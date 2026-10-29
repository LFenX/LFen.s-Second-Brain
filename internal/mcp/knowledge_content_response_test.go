package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ragflow/internal/entity"
)

func contentCandidateRPC(t *testing.T, id any, result *sdk.CallToolResult) []byte {
	t.Helper()
	wire, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func contentCandidateServeBytes(t *testing.T, wire []byte) *httptest.ResponseRecorder {
	t.Helper()
	handler := contentKnowledgeBoundedHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public")
		for len(wire) > 0 {
			n := min(len(wire), 4096)
			_, _ = w.Write(wire[:n])
			wire = wire[n:]
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "http://localhost/mcp", nil))
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private result is cacheable")
	}
	if response.Body.Len() > contentKnowledgeMaxResponseBytes || response.Header().Get("Content-Length") != strconv.Itoa(response.Body.Len()) {
		t.Fatal("actual HTTP body length is not bounded and accurate")
	}
	return response
}

func contentCandidateAssertCapacityHTTP(t *testing.T, wire []byte) {
	t.Helper()
	response := contentCandidateServeBytes(t, wire)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		Error struct {
			Data struct {
				Code string `json:"code"`
			} `json:"data"`
		} `json:"error"`
	}
	if json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Error.Data.Code != "capacity_exceeded" {
		t.Fatal("missing controlled HTTP JSON-RPC capacity refusal")
	}
	if bytes.Equal(response.Body.Bytes(), wire) {
		t.Fatal("oversized response escaped")
	}
}

func TestContentKnowledgeCandidateActualHTTPBudget(t *testing.T) {
	for _, test := range []struct{ name, text string }{
		{"double copy exceeds wire budget", strings.Repeat("a", contentKnowledgeMaxResponseBytes*3/5)},
		{"JSON text escaping exceeds wire budget", strings.Repeat(`"`, contentKnowledgeMaxResponseBytes/4)},
	} {
		t.Run(test.name, func(t *testing.T) {
			semantic, err := json.Marshal(map[string]string{"text": test.text})
			if err != nil || len(semantic) >= contentKnowledgeMaxResponseBytes {
				t.Fatal("test must have semantic body below budget")
			}
			wire := contentCandidateRPC(t, 1, contentKnowledgeEncodedResult(semantic, false))
			if len(wire) <= contentKnowledgeMaxResponseBytes {
				t.Fatal("test must exceed the actual full HTTP JSON-RPC budget")
			}
			contentCandidateAssertCapacityHTTP(t, wire)
		})
	}
	// A valid CallToolResult below 1 MiB still overflows after JSON-RPC id/framing.
	result := &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: strings.Repeat("x", contentKnowledgeMaxResponseBytes-200)}}}
	encoded, _ := json.Marshal(result)
	wire := contentCandidateRPC(t, strings.Repeat("id", 512), result)
	if len(encoded) >= contentKnowledgeMaxResponseBytes || len(wire) <= contentKnowledgeMaxResponseBytes {
		t.Fatal("invalid JSON-RPC-overhead boundary fixture")
	}
	contentCandidateAssertCapacityHTTP(t, wire)
}

func TestContentKnowledgeCandidateHTTPBudgetBoundaryAndNormalData(t *testing.T) {
	fixture := contentCandidateActual(t)
	tool := contentCandidateTools(t)["knowledge_read_local"]
	result := contentKnowledgeResult(tool, "read", &fixture.Reads[0], nil)
	if result.IsError {
		t.Fatal("actual normal data rejected")
	}
	wire := contentCandidateRPC(t, 7, result)
	response := contentCandidateServeBytes(t, wire)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), wire) {
		t.Fatal("normal complete serialized data changed")
	}
	padding := append(append([]byte(nil), wire...), bytes.Repeat([]byte(" "), contentKnowledgeMaxResponseBytes-len(wire))...)
	atLimit := contentCandidateServeBytes(t, padding)
	if atLimit.Code != http.StatusOK || !bytes.Equal(atLimit.Body.Bytes(), padding) {
		t.Fatal("exact 1 MiB response not preserved")
	}
	contentCandidateAssertCapacityHTTP(t, append(padding, ' '))
}

// Constructed size-only search data exercises the real tool encoder; it is
// never an owning source/index/native-session proof or an exported fixture.
func TestContentKnowledgeCandidateToolResultBudget(t *testing.T) {
	fixture := contentCandidateActual(t)
	tool := contentCandidateTools(t)["knowledge_search_local"]
	search := fixture.Search
	hit := search.Hits[0]
	hit.Text = strings.Repeat("\x00", 8192)
	digest := sha256.Sum256([]byte(hit.Text))
	textSHA := hex.EncodeToString(digest[:])
	hit.Evidence.Range = entity.KnowledgeContentRange{StartByte: 0, EndByte: 8192, SHA256: &textSHA}
	search.Hits = nil
	for i := 0; i < 18; i++ {
		copy := hit
		copy.Evidence.RowID = fmt.Sprintf("%064x", i+1)
		search.Hits = append(search.Hits, copy)
	}
	search.RequestedLimit, search.ReturnedCount = 18, 18
	body, _ := json.Marshal(map[string]any{"operation": "search", "contract_version": entity.KnowledgeContentVersion, "request_id": strings.Repeat("1", 32), "data": &search})
	var value any
	if json.Unmarshal(body, &value) != nil || tool.output.Validate(&value) != nil {
		t.Fatal("size fixture must satisfy the closed output shape")
	}
	encoded, _ := json.Marshal(contentKnowledgeEncodedResult(body, false))
	if len(body) >= contentKnowledgeMaxResponseBytes || len(encoded) <= contentKnowledgeMaxResponseBytes {
		t.Fatal("size fixture must overflow only after full SDK result encoding")
	}
	result := contentKnowledgeResult(tool, "search", &search, nil)
	var controlled map[string]any
	raw, _ := json.Marshal(result.StructuredContent)
	if !result.IsError || json.Unmarshal(raw, &controlled) != nil || controlled["data"] != nil || controlled["code"] != "capacity_exceeded" {
		t.Fatal("tool result overflow did not become controlled null-data capacity error")
	}
	if bytes, _ := json.Marshal(result); len(bytes) > contentKnowledgeMaxResponseBytes {
		t.Fatal("controlled tool result exceeds budget")
	}
}

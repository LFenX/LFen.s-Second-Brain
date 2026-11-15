//go:build integration && linux

package elasticsearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

func TestKnowledgeAccessRealESFilterAndDeniedFrequency(t *testing.T) {
	if os.Getenv("LFEN_POC_SYNTHETIC_ONLY") != "1" || os.Getenv("LFEN_ACCESS_ES_GAPS") != "root-approved-isolated-es" {
		t.Skip("isolated synthetic access ES scope not granted")
	}
	index := os.Getenv("LFEN_ACCESS_ES_INDEX_REQUIRED")
	if !regexp.MustCompile("^ragflow_[0-9a-f]{32}$").MatchString(index) {
		t.Fatal("ACCESS_ES_INDEX_REQUIRED: bind one new Root-owned synthetic index before activation")
	}
	auth := readKnowledgeAccessESAuth(t, index)
	t.Cleanup(func() { auth.Username = ""; auth.Password = "" })
	domain := strings.TrimPrefix(index, "ragflow_")
	t.Logf("SYNTHETIC_ES_INDEX_SCOPE %s", index)
	if err := common.InitLogger("warn", common.FileOutput{}, ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	t.Cleanup(transport.CloseIdleConnections)
	var mu sync.Mutex
	record := false
	var queries []map[string]any
	client, err := es.NewClient(es.Config{DisableRetry: true, Addresses: []string{auth.Endpoint}, Username: auth.Username, Password: auth.Password, Transport: lexicalRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "kcls-es:9200" || r.URL.Scheme != "http" || r.URL.User != nil {
			return nil, errors.New("non-synthetic HTTP blocked")
		}
		if r.URL.Path != "/_bulk" && r.URL.Path != "/"+index && !strings.HasPrefix(r.URL.Path, "/"+index+"/") {
			return nil, errors.New("outside synthetic index scope")
		}
		mu.Lock()
		capture := record && strings.HasSuffix(r.URL.Path, "/_search")
		mu.Unlock()
		if capture {
			raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil || len(raw) > 1<<20 {
				return nil, errors.New("synthetic query capture bound")
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var query map[string]any
			if json.Unmarshal(raw, &query) != nil {
				return nil, errors.New("synthetic query JSON")
			}
			mu.Lock()
			queries = append(queries, query)
			mu.Unlock()
		}
		response, err := transport.RoundTrip(r)
		if response != nil && response.StatusCode >= 300 && response.StatusCode < 400 {
			response.Body.Close()
			return nil, errors.New("synthetic ES redirect blocked")
		}
		return response, err
	})})
	if err != nil {
		t.Fatal("synthetic ES client")
	}
	engine := &Engine{client: client}
	// Root holds this exact name before activation; existing indices are never adopted.
	exists, err := client.Indices.Exists([]string{index}, client.Indices.Exists.WithContext(ctx))
	if err != nil {
		t.Fatal("synthetic index absence probe failed")
	}
	status := exists.StatusCode
	if err := exists.Body.Close(); err != nil || status != http.StatusNotFound {
		t.Fatal("synthetic index must be proven absent before any write or cleanup")
	}
	// Atomic create prevents PrepareKnowledgeLexical from adopting a raced-in index.
	createBody, err := json.Marshal(map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": map[string]any{"properties": map[string]any{"tag_feas": map[string]any{"type": "rank_features"}}},
	})
	if err != nil {
		t.Fatal("synthetic index create body")
	}
	created, err := client.Indices.Create(index, client.Indices.Create.WithContext(ctx), client.Indices.Create.WithBody(bytes.NewReader(createBody)))
	if err != nil {
		t.Fatal("synthetic index create outcome unresolved; Root retains the exact scope name")
	}
	var createAck struct {
		Acknowledged bool   `json:"acknowledged"`
		Index        string `json:"index"`
	}
	if err := knowledgeDecode(created, &createAck); err != nil || !createAck.Acknowledged || createAck.Index != index {
		t.Fatal("synthetic index ownership not established; no automatic cleanup")
	}
	// Register deletion only after a successful atomic create of the absent exact index.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		response, err := client.Indices.Delete([]string{index}, client.Indices.Delete.WithContext(cleanup))
		if err != nil {
			t.Error("synthetic index cleanup failed")
			return
		}
		if response.StatusCode == http.StatusNotFound {
			if err := response.Body.Close(); err != nil {
				t.Error("synthetic index absence receipt close failed")
			}
			return
		}
		var deletion struct {
			Acknowledged bool `json:"acknowledged"`
		}
		if err := knowledgeDecode(response, &deletion); err != nil || !deletion.Acknowledged {
			t.Error("synthetic index cleanup outcome unresolved")
			return
		}
		absent, err := client.Indices.Exists([]string{index}, client.Indices.Exists.WithContext(cleanup))
		if err != nil {
			t.Error("synthetic index cleanup absence probe failed")
			return
		}
		status := absent.StatusCode
		if err := absent.Body.Close(); err != nil || status != http.StatusNotFound {
			t.Error("synthetic index cleanup absence unproven")
		}
	})
	if err := engine.PrepareKnowledgeLexical(ctx, index, domain, ""); err != nil {
		t.Fatal("synthetic index preparation")
	}
	requestJSON := func(method, suffix string, body any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal("synthetic request JSON")
		}
		req, err := http.NewRequestWithContext(ctx, method, "http://kcls-es:9200/"+index+suffix, bytes.NewReader(raw))
		if err != nil {
			t.Fatal("synthetic request")
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Perform(req)
		if err != nil {
			t.Fatal("synthetic ES transport")
		}
		defer response.Body.Close()
		var reply map[string]any
		if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&reply) != nil {
			t.Fatal("synthetic ES response")
		}
		return reply
	}
	makeRow := func(id, binding, tokens string) map[string]any {
		return map[string]any{"id": id, "kb_id": domain, "doc_id": common.GenerateUUID(), "knowledge_binding_kwd": binding, "knowledge_ref": "{\"synthetic_access_gap\":true}", "available_int": 0, "content_with_weight": tokens, "content_ltks": tokens}
	}
	idA, idB, deniedID := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("0", 64)
	bindingA, bindingB, deniedBinding := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	allowed := []map[string]any{
		makeRow(idA, bindingA, "lfenaclq "+strings.Repeat("padding ", 512)),
		makeRow(idB, bindingB, strings.Repeat("lfenaclq ", 32)+"padding"),
	}
	rows := append(append([]map[string]any{}, allowed...), makeRow(deniedID, deniedBinding, strings.Repeat("lfenaclq ", 128)))
	if err := engine.CreateKnowledgeLexicalRows(ctx, index, domain, rows); err != nil {
		t.Fatal("synthetic initial rows")
	}
	// Dynamic fields exist after the first real rows; assert the actual required mapping.
	mappingResponse, err := client.Indices.GetMapping(client.Indices.GetMapping.WithContext(ctx), client.Indices.GetMapping.WithIndex(index))
	if err != nil {
		t.Fatal("synthetic ID mapping read failed")
	}
	var actualMapping map[string]struct {
		Mappings struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"mappings"`
	}
	if err := knowledgeDecode(mappingResponse, &actualMapping); err != nil || len(actualMapping) != 1 {
		t.Fatal("synthetic ID mapping response")
	}
	indexMapping, ok := actualMapping[index]
	idMapping := indexMapping.Mappings.Properties["id"]
	if !ok || idMapping["type"] != "keyword" || idMapping["doc_values"] == false || idMapping["index"] == false || idMapping["normalizer"] != nil {
		t.Fatal("ACCESS_ES_ID_KEYWORD_MAPPING_REQUIRED")
	}
	if above, ok := idMapping["ignore_above"].(float64); ok && above < 64 {
		t.Fatal("ACCESS_ES_ID_KEYWORD_MAPPING_REQUIRED")
	}
	requestJSON("POST", "/_refresh", map[string]any{})
	// A real relevance control proves that postfiltering the global top1
	// would discard the denied row instead of finding the permitted A row.
	control := requestJSON("POST", "/_search", map[string]any{"size": 3, "track_total_hits": true, "query": map[string]any{"term": map[string]any{"content_ltks": "lfenaclq"}}, "sort": []any{map[string]any{"_score": "desc"}}})
	hits := control["hits"].(map[string]any)["hits"].([]any)
	if len(hits) != 3 || hits[0].(map[string]any)["_id"] != deniedID || hits[1].(map[string]any)["_id"] != idB {
		t.Fatal("fixture does not establish denied high score or conflicting relevance order")
	}
	countMatchingDocs := func() float64 {
		reply := requestJSON("POST", "/_count", map[string]any{"query": map[string]any{"term": map[string]any{"content_ltks": "lfenaclq"}}})
		return reply["count"].(float64)
	}
	beforeFrequency := countMatchingDocs()
	if beforeFrequency != 3 {
		t.Fatal("baseline term document frequency")
	}
	runScoped := func(limit int) []string {
		t.Helper()
		mu.Lock()
		record = true
		queries = nil
		mu.Unlock()
		result, err := engine.Search(ctx, &types.SearchRequest{KnowledgeAccessStable: true, IndexNames: []string{index}, KbIDs: []string{domain}, Limit: limit, SelectFields: []string{"id", "knowledge_binding_kwd", "content_with_weight"}, Filter: map[string]any{"available_int": 0, "knowledge_available_exact": 0, "or": []map[string]any{{"knowledge_binding_kwd": bindingA, "id": []string{idA}}, {"knowledge_binding_kwd": bindingB, "id": []string{idB}}}}, MatchExprs: []any{&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "lfenaclq", TopN: limit}}, RankFeature: map[string]float64{}})
		mu.Lock()
		record = false
		captured := append([]map[string]any(nil), queries...)
		mu.Unlock()
		if err != nil || result == nil || len(result.Chunks) != limit || len(captured) != 1 {
			t.Fatal("real scoped search or query capture failed")
		}
		query := captured[0]
		sortJSON, _ := json.Marshal(query["sort"])
		if query["size"] != float64(limit) || string(sortJSON) != "[{\"id\":{\"order\":\"asc\"}}]" {
			t.Fatal("ID sort/limit was not sent to ES")
		}
		paired := false
		var inspect func(any)
		inspect = func(value any) {
			switch value := value.(type) {
			case map[string]any:
				if boolQuery, ok := value["bool"].(map[string]any); ok {
					if clauses, ok := boolQuery["should"].([]any); ok && len(clauses) == 2 {
						pairA, pairB := false, false
						for _, clause := range clauses {
							raw, _ := json.Marshal(clause)
							s := string(raw)
							pairA = pairA || strings.Contains(s, bindingA) && strings.Contains(s, idA) && !strings.Contains(s, bindingB) && !strings.Contains(s, idB)
							pairB = pairB || strings.Contains(s, bindingB) && strings.Contains(s, idB) && !strings.Contains(s, bindingA) && !strings.Contains(s, idA)
						}
						paired = paired || pairA && pairB
					}
				}
				for _, item := range value {
					inspect(item)
				}
			case []any:
				for _, item := range value {
					inspect(item)
				}
			}
		}
		inspect(query["query"])
		if !paired {
			t.Fatal("final ES query did not preserve the exact paired OR")
		}
		ids := make([]string, len(result.Chunks))
		for i, row := range result.Chunks {
			id, ok := row["id"].(string)
			if !ok || id != []string{idA, idB}[i] {
				t.Fatal("permission filter/order was applied after top-k or scores reordered IDs")
			}
			ids[i] = id
		}
		return ids
	}
	runScoped(1)
	baseline := runScoped(2)
	moreDeniedBinding := strings.Repeat("d", 64)
	moreDenied := make([]map[string]any, 16)
	for i := range moreDenied {
		moreDenied[i] = makeRow(fmt.Sprintf("%064x", i+16), moreDeniedBinding, strings.Repeat("lfenaclq ", 64+i))
	}
	if err := engine.CreateKnowledgeLexicalRows(ctx, index, domain, moreDenied); err != nil {
		t.Fatal("synthetic denied frequency perturbation")
	}
	requestJSON("POST", "/_refresh", map[string]any{})
	afterFrequency := countMatchingDocs()
	if afterFrequency != beforeFrequency+float64(len(moreDenied)) {
		t.Fatal("denied corpus term document frequency did not actually change")
	}
	for _, row := range allowed {
		actual, err := engine.knowledgeGet(ctx, index, row["id"].(string))
		if err != nil || !knowledgeEquivalent(actual, row) {
			t.Fatal("allowed row changed during denied corpus perturbation")
		}
	}
	runScoped(1)
	after := runScoped(2)
	if strings.Join(baseline, ",") != strings.Join(after, ",") {
		t.Fatal("denied corpus frequency influenced permitted order")
	}
	t.Log("REAL_ES_PAIRED_FILTER_BEFORE_LIMIT_AND_DENIED_FREQUENCY_STABLE")
	// Numeric ES scoring remains internal. The shared service DTO/carrier
	// no-score/no-global-count assertions are separate owning tests.
}

const esAccessEndpoint = "http://kcls-es:9200"
const esAccessContainerID = "6405fc5082380e456c8ed32769c97b1f4859068b9a4e2f6c7e9a9279b9d3810a"

var errESAccessRAM = errors.New("es_private_ram_refused")

// This input belongs to this named integration test. Credential values are
// separate from the public lease bytes whose digest selects the RAM packet.
type esAccessLease struct {
	Purpose         string    `json:"purpose"`
	RunID           string    `json:"run_id"`
	Endpoint        string    `json:"endpoint"`
	ContainerID     string    `json:"container_id"`
	Index           string    `json:"index"`
	AuthMode        string    `json:"auth_mode"`
	ModeProven      bool      `json:"mode_proven"`
	EffectiveConfig bool      `json:"effective_config_proven"`
	AnonymousProven bool      `json:"anonymous_proven"`
	ModeEvidenceSHA string    `json:"mode_evidence_sha256"`
	SyntheticOnly   bool      `json:"synthetic_only"`
	FreshCreatorRun string    `json:"fresh_creator_run_id"`
	NotBefore       time.Time `json:"not_before"`
	Expires         time.Time `json:"expires_at"`
	TestELFSHA      string    `json:"test_elf_sha256"`
	TestELFBytes    int64     `json:"test_elf_bytes"`
}

type esAccessCredentials struct {
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (credentials *esAccessCredentials) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if entity.KnowledgeParsedDecode(raw, &fields) != nil || len(fields) != 3 {
		return errESAccessRAM
	}
	defer func() {
		for _, value := range fields {
			clear(value)
		}
	}()
	for _, name := range []string{"endpoint", "username", "password"} {
		value, ok := fields[name]
		if !ok || len(bytes.TrimSpace(value)) == 0 || bytes.TrimSpace(value)[0] != '"' {
			return errESAccessRAM
		}
	}
	type plainCredentials esAccessCredentials
	var decoded plainCredentials
	if entity.KnowledgeParsedDecode(raw, &decoded) != nil {
		return errESAccessRAM
	}
	*credentials = esAccessCredentials(decoded)
	return nil
}

type esAccessInput struct {
	Lease json.RawMessage      `json:"lease"`
	ES    *esAccessCredentials `json:"es"`
}

func esAccessSHA(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateESAccessInput(raw []byte, expected, index, pipe string, expires, now time.Time) (esAccessLease, esAccessCredentials, error) {
	var input esAccessInput
	var lease esAccessLease
	if len(raw) == 0 || len(raw) > 128<<10 || !entity.ValidKnowledgeLexicalHash(expected) ||
		entity.KnowledgeParsedDecode(raw, &input) != nil || input.ES == nil ||
		esAccessSHA(input.Lease) != expected || entity.KnowledgeParsedDecode(input.Lease, &lease) != nil {
		return lease, esAccessCredentials{}, errESAccessRAM
	}
	var leaseFields map[string]json.RawMessage
	if entity.KnowledgeParsedDecode(input.Lease, &leaseFields) != nil || len(leaseFields) != 16 {
		return lease, esAccessCredentials{}, errESAccessRAM
	}
	for _, name := range []string{"purpose", "run_id", "endpoint", "container_id", "index", "auth_mode", "mode_proven", "effective_config_proven", "anonymous_proven", "mode_evidence_sha256", "synthetic_only", "fresh_creator_run_id", "not_before", "expires_at", "test_elf_sha256", "test_elf_bytes"} {
		if _, ok := leaseFields[name]; !ok {
			return lease, esAccessCredentials{}, errESAccessRAM
		}
	}
	if value := bytes.TrimSpace(leaseFields["anonymous_proven"]); !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
		return lease, esAccessCredentials{}, errESAccessRAM
	}
	if lease.Purpose != "ROOT_ACCESS02_S1_ES_GAPS_AUTH04" || !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(lease.RunID) ||
		lease.Endpoint != esAccessEndpoint || input.ES.Endpoint != lease.Endpoint || lease.ContainerID != esAccessContainerID ||
		lease.Index != index || !regexp.MustCompile(`^ragflow_[0-9a-f]{32}$`).MatchString(index) ||
		pipe != "/dev/shm/lfen-es04-"+lease.RunID+"/input" || !lease.SyntheticOnly || lease.FreshCreatorRun != lease.RunID ||
		!lease.ModeProven || !lease.EffectiveConfig || !entity.ValidKnowledgeLexicalHash(lease.ModeEvidenceSHA) ||
		lease.NotBefore.IsZero() || now.Before(lease.NotBefore) || !lease.Expires.Equal(expires) ||
		!expires.After(now.Add(80*time.Second)) || expires.Sub(lease.NotBefore) > 10*time.Minute ||
		!entity.ValidKnowledgeLexicalHash(lease.TestELFSHA) || lease.TestELFBytes < 1 || lease.TestELFBytes > 1<<30 {
		return lease, esAccessCredentials{}, errESAccessRAM
	}
	credentials := *input.ES
	switch lease.AuthMode {
	case "basic":
		if lease.AnonymousProven || len(credentials.Username) < 1 || len(credentials.Username) > 256 ||
			len(credentials.Password) < 1 || len(credentials.Password) > 4096 ||
			strings.ContainsAny(credentials.Username, ":\x00\r\n") || strings.ContainsAny(credentials.Password, "\x00\r\n") {
			return lease, esAccessCredentials{}, errESAccessRAM
		}
	case "anonymous_root_proven":
		if !lease.AnonymousProven || credentials.Username != "" || credentials.Password != "" {
			return lease, esAccessCredentials{}, errESAccessRAM
		}
	default:
		return lease, esAccessCredentials{}, errESAccessRAM
	}
	return lease, credentials, nil
}

// The existing MySQL RAM channel's same-file/owner/mode checks are kept private
// to this test package. No opener goroutine or credential environment is used.
func readESAccessRAM(name string, expires time.Time) ([]byte, error) {
	deadline := time.Now().Add(5 * time.Second)
	if expires.Before(deadline) {
		deadline = expires
	}
	live := func() bool { return time.Now().Before(deadline) }
	if !live() || !filepath.IsAbs(name) || filepath.Clean(name) != name ||
		!regexp.MustCompile(`^/dev/shm/lfen-es04-[0-9a-f]{16}/input$`).MatchString(name) {
		return nil, errESAccessRAM
	}
	before, err := os.Lstat(name)
	if err != nil || before.Mode()&os.ModeNamedPipe == 0 || before.Mode()&os.ModeSymlink != 0 || before.Mode().Perm() != 0600 || !live() {
		return nil, errESAccessRAM
	}
	parent, err := os.Lstat(filepath.Dir(name))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm() != 0700 || !live() {
		return nil, errESAccessRAM
	}
	a, aok := before.Sys().(*syscall.Stat_t)
	b, bok := parent.Sys().(*syscall.Stat_t)
	if !aok || !bok || a.Uid != uint32(os.Geteuid()) || b.Uid != uint32(os.Geteuid()) {
		return nil, errESAccessRAM
	}
	stream, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errESAccessRAM
	}
	defer stream.Close()
	after, err := stream.Stat()
	if err != nil || !os.SameFile(before, after) || !live() || stream.SetReadDeadline(deadline) != nil || !live() {
		return nil, errESAccessRAM
	}
	raw, err := io.ReadAll(io.LimitReader(stream, (128<<10)+1))
	if err != nil || !live() || len(raw) < 1 || len(raw) > 128<<10 {
		clear(raw)
		return nil, errESAccessRAM
	}
	return raw, nil
}

func readKnowledgeAccessESAuth(t *testing.T, index string) *esAccessCredentials {
	t.Helper()
	expected, pipe := os.Getenv("LFEN_ACCESS_ES_SCOPE_SHA256"), os.Getenv("LFEN_ACCESS_ES_RAM_PIPE")
	expires, err := time.Parse(time.RFC3339Nano, os.Getenv("LFEN_ACCESS_ES_EXPIRES_AT"))
	if err != nil || !expires.After(time.Now()) || !entity.ValidKnowledgeLexicalHash(expected) {
		t.Fatal("ES_AUTH_RAM_BOOTSTRAP_REFUSED")
	}
	raw, err := readESAccessRAM(pipe, expires)
	if err != nil {
		t.Fatal("ES_AUTH_RAM_INPUT_REFUSED")
	}
	defer clear(raw)
	lease, credentials, err := validateESAccessInput(raw, expected, index, pipe, expires, time.Now())
	if err != nil {
		t.Fatal("ES_AUTH_RAM_SCOPE_REFUSED")
	}
	exe, err := os.Open("/proc/self/exe")
	if err != nil {
		t.Fatal("ES_AUTH_TEST_ELF_OPEN_REFUSED")
	}
	defer exe.Close()
	info, err := exe.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != lease.TestELFBytes {
		t.Fatal("ES_AUTH_TEST_ELF_SIZE_REFUSED")
	}
	digest := sha256.New()
	n, err := io.Copy(digest, io.LimitReader(exe, lease.TestELFBytes+1))
	if err != nil || n != lease.TestELFBytes || hex.EncodeToString(digest.Sum(nil)) != lease.TestELFSHA {
		t.Fatal("ES_AUTH_TEST_ELF_PIN_REFUSED")
	}
	if !lease.Expires.After(time.Now().Add(70 * time.Second)) {
		t.Fatal("ES_AUTH_ORIGINAL_SCOPE_REMAINING_REFUSED")
	}
	// Raw buffers and our references are cleared. SDK/basic-auth string copies
	// are bounded by this joined test process; physical erasure is not claimed.
	return &credentials
}

func TestKnowledgeAccessESAuthInputCodec(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	lease := esAccessLease{Purpose: "ROOT_ACCESS02_S1_ES_GAPS_AUTH04", RunID: strings.Repeat("a", 16), Endpoint: esAccessEndpoint, ContainerID: esAccessContainerID, Index: "ragflow_" + strings.Repeat("b", 32), AuthMode: "basic", ModeProven: true, EffectiveConfig: true, ModeEvidenceSHA: strings.Repeat("c", 64), SyntheticOnly: true, FreshCreatorRun: strings.Repeat("a", 16), NotBefore: now.Add(-time.Second), Expires: now.Add(2 * time.Minute), TestELFSHA: strings.Repeat("d", 64), TestELFBytes: 1024}
	credentials := esAccessCredentials{Endpoint: esAccessEndpoint, Username: "synthetic-user-only", Password: "synthetic-password-only"}
	check := func(l esAccessLease, c esAccessCredentials) error {
		leaseRaw, _ := json.Marshal(l)
		raw, _ := json.Marshal(esAccessInput{Lease: leaseRaw, ES: &c})
		defer clear(raw)
		_, _, err := validateESAccessInput(raw, esAccessSHA(leaseRaw), lease.Index, "/dev/shm/lfen-es04-"+lease.RunID+"/input", lease.Expires, now)
		return err
	}
	if check(lease, credentials) != nil {
		t.Fatal("synthetic_basic_codec_rejected")
	}
	for _, change := range []func(*esAccessLease, *esAccessCredentials){
		func(_ *esAccessLease, c *esAccessCredentials) { c.Password = "" },
		func(_ *esAccessLease, c *esAccessCredentials) { c.Endpoint = "http://userinfo:forbidden@kcls-es:9200" },
		func(l *esAccessLease, _ *esAccessCredentials) { l.ModeProven = false },
		func(l *esAccessLease, _ *esAccessCredentials) { l.AuthMode = "" },
		func(l *esAccessLease, c *esAccessCredentials) {
			l.AuthMode = "anonymous_root_proven"
			c.Username = ""
			c.Password = ""
		},
		func(l *esAccessLease, _ *esAccessCredentials) { l.Index = "ragflow_*" },
	} {
		l, c := lease, credentials
		change(&l, &c)
		if check(l, c) == nil {
			t.Fatal("synthetic_invalid_auth_codec_accepted")
		}
	}
	lease.AuthMode, lease.AnonymousProven = "anonymous_root_proven", true
	credentials.Username, credentials.Password = "", ""
	if check(lease, credentials) != nil {
		t.Fatal("synthetic_explicit_anonymous_codec_rejected")
	}
}

func TestKnowledgeAccessESAuthBothRequestPaths(t *testing.T) {
	seen := 0
	client, err := es.NewClient(es.Config{Addresses: []string{esAccessEndpoint}, Username: "synthetic-user-only", Password: "synthetic-password-only", DisableRetry: true, Transport: lexicalRoundTripper(func(r *http.Request) (*http.Response, error) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "synthetic-user-only" || password != "synthetic-password-only" || r.URL.User != nil || r.URL.Host != "kcls-es:9200" {
			return nil, errors.New("synthetic_auth_request_refused")
		}
		seen++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Elastic-Product": []string{"Elasticsearch"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})})
	if err != nil {
		t.Fatal("synthetic_auth_client_refused")
	}
	response, err := client.Indices.Exists([]string{"ragflow_" + strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal("synthetic_sdk_auth_missing")
	}
	response.Body.Close()
	req, _ := http.NewRequest(http.MethodPost, esAccessEndpoint+"/ragflow_"+strings.Repeat("a", 32)+"/_count", strings.NewReader("{}"))
	rawResponse, err := client.Perform(req)
	if err != nil {
		t.Fatal("synthetic_direct_auth_missing")
	}
	rawResponse.Body.Close()
	if seen != 2 {
		t.Fatal("synthetic_auth_paths_not_both_exercised")
	}
}

package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/handler"
	"ragflow/internal/server"
	"ragflow/internal/service/dataset"
)

// Exercise the production Router.Setup -> DatasetService -> protected official
// stateless transport chain with a synthetic signed session and user-only DB.
func TestRouterContentMCPTransport(t *testing.T) {
	// The auth path uses package-global configuration. Isolate its synthetic
	// initialization in this test binary rather than adding a production seam.
	const child = "LFEN_CONTENT_MCP_TRANSPORT_UNIT_CHILD"
	if os.Getenv(child) != "1" {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRouterContentMCPTransport$", "-test.count=1")
		cmd.Env = []string{child + "=1", "LANG=C", "TZ=UTC"}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated content MCP transport test: %v\n%s", err, output)
		}
		return
	}
	gin.SetMode(gin.TestMode)
	previousLogger := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previousLogger })

	const secret = "content-mcp-router-unit-secret"
	t.Setenv(common.EnvRAGFlowSecretKey, secret)
	configPath := filepath.Join(t.TempDir(), "service_conf.yaml")
	if err := os.WriteFile(configPath, []byte("general:\n  mode: release\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Explicit synthetic configuration; no default config discovery or clients.
	if err := server.Init(configPath); err != nil {
		t.Fatal(err)
	}

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatal(err)
	}
	accessToken, status := strings.Repeat("a", 32), "1"
	if err := db.Create(&entity.User{
		ID: "content-mcp-unit-owner", Email: "content-mcp@example.test", Nickname: "Unit owner",
		AccessToken: &accessToken, Status: &status, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0",
	}).Error; err != nil {
		t.Fatal(err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })

	// Discovery and GET may authenticate users; any business SQL or write fails.
	var businessSQL atomic.Int64
	refuseBusiness := func(tx *gorm.DB) {
		businessSQL.Add(1)
		tx.AddError(errors.New("content MCP transport must not dispatch business operations"))
	}
	if err := db.Callback().Query().Before("gorm:query").Register("content_mcp_user_only", func(tx *gorm.DB) {
		if tx.Statement.Table != (entity.User{}).TableName() {
			refuseBusiness(tx)
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, register := range []func(string, func(*gorm.DB)) error{
		db.Callback().Create().Before("gorm:create").Register,
		db.Callback().Update().Before("gorm:update").Register,
		db.Callback().Delete().Before("gorm:delete").Register,
		db.Callback().Raw().Before("gorm:raw").Register,
		db.Callback().Row().Before("gorm:row").Register,
	} {
		if err := register("content_mcp_no_business", refuseBusiness); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if got := businessSQL.Load(); got != 0 {
			t.Errorf("unexpected business SQL or write attempts: %d", got)
		}
	})

	signed, err := common.DumpAccessToken(accessToken, secret)
	if err != nil {
		t.Fatal(err)
	}
	const endpoint = "/api/v1/knowledge/content/mcp"
	const profile = "knowledge-content-lexical/0.2.0-candidate"
	engine := gin.New()
	r := &Router{authHandler: handler.NewAuthHandler(), datasetsHandler: handler.NewDatasetsHandler(&dataset.DatasetService{}, nil)}
	r.Setup(engine)
	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		if route.Path == endpoint {
			registered[route.Method] = true
		}
	}
	if len(registered) != 2 || !registered[http.MethodGet] || !registered[http.MethodPost] {
		t.Fatalf("content MCP methods=%v; want exact GET and POST", registered)
	}
	request := func(t *testing.T, method, body, authorization, selectedProfile, origin string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://localhost"+endpoint, strings.NewReader(body))
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("MCP-Protocol-Version", "2025-11-25")
		req.Header.Set("Authorization", authorization)
		req.Header.Set("X-Knowledge-MCP-Profile", selectedProfile)
		req.Header.Set("Origin", origin)
		req.Header.Set("X-API-Key", "unit-api-key-must-not-authorize")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		if response.Header().Get("Mcp-Session-Id") != "" || len(response.Header().Values("Set-Cookie")) != 0 {
			t.Fatal("stateless MCP transport issued a session or cookie")
		}
		return response
	}
	for _, tc := range []struct {
		name, authorization, selectedProfile, origin string
		status                                       int
	}{
		{"authenticated", "Bearer " + signed, profile, "http://localhost", http.StatusMethodNotAllowed},
		{"origin absent", "Bearer " + signed, profile, "", http.StatusMethodNotAllowed},
		{"cross origin", "Bearer " + signed, profile, "https://other.example.test", http.StatusForbidden},
		{"invalid origin before auth", "", "", "null", http.StatusForbidden},
		{"missing native session", "", profile, "http://localhost", http.StatusUnauthorized},
		{"invalid native session", "Bearer invalid-unit-session", profile, "http://localhost", http.StatusUnauthorized},
		{"missing profile", "Bearer " + signed, "", "http://localhost", http.StatusUnauthorized},
		{"wrong profile", "Bearer " + signed, "local_lexical/0.1.0", "http://localhost", http.StatusUnauthorized},
	} {
		t.Run("GET/"+tc.name, func(t *testing.T) {
			response := request(t, http.MethodGet, "", tc.authorization, tc.selectedProfile, tc.origin)
			if response.Code != tc.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("GET status=%d cache=%q", response.Code, response.Header().Get("Cache-Control"))
			}
			if tc.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != http.MethodPost {
				t.Fatal("official stateless GET refusal must advertise POST")
			}
			if tc.status == http.StatusMethodNotAllowed && response.Body.String() != "Method Not Allowed\n" {
				t.Fatal("GET must reach the official stateless refusal after protection")
			}
		})
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"content-router-unit","version":"1"}}}`, http.StatusOK},
		{"initialized", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, http.StatusAccepted},
		{"tools/list", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, http.StatusOK},
	} {
		t.Run("POST/"+tc.name, func(t *testing.T) {
			response := request(t, http.MethodPost, tc.body, "Bearer "+signed, profile, "http://localhost")
			if response.Code != tc.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("POST %s status=%d", tc.name, response.Code)
			}
			if tc.status == http.StatusAccepted {
				if response.Body.Len() != 0 {
					t.Fatal("accepted notification must have an empty body")
				}
				return
			}
			var rpc struct {
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &rpc); err != nil || len(rpc.Result) == 0 || len(rpc.Error) != 0 {
				t.Fatal("discovery must preserve its JSON-RPC result")
			}
			if tc.name == "tools/list" {
				var listed struct {
					Tools []struct {
						Name         string          `json:"name"`
						InputSchema  json.RawMessage `json:"inputSchema"`
						OutputSchema json.RawMessage `json:"outputSchema"`
					} `json:"tools"`
				}
				if err := json.Unmarshal(rpc.Result, &listed); err != nil || len(listed.Tools) != 3 {
					t.Fatal("exact three read-only tools must remain discoverable")
				}
				names := map[string]bool{}
				for _, tool := range listed.Tools {
					names[tool.Name] = true
					if len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 {
						t.Fatal("tool schemas must remain available")
					}
				}
				for _, name := range []string{"knowledge_inspect_local_index", "knowledge_search_local", "knowledge_read_local"} {
					if !names[name] {
						t.Fatalf("missing read-only tool %s", name)
					}
				}
			}
		})
	}
	for _, method := range []string{http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		t.Run("unsupported/"+method, func(t *testing.T) {
			if response := request(t, method, "", "Bearer "+signed, profile, "http://localhost"); response.Code != http.StatusNotFound {
				t.Fatalf("unsupported public method %s status=%d; want existing unregistered404 boundary", method, response.Code)
			}
		})
	}
}

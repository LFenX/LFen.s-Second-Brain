package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
)

type accessAuthTrace struct{ bytes.Buffer }

func (w *accessAuthTrace) Printf(format string, args ...any) { fmt.Fprintf(&w.Buffer, format, args...) }

// Only session decoding is substituted; the native SQL read and HTTP
// middleware chain are real. No Redis or remote service is required.
type accessAuthResolver struct {
	db                *gorm.DB
	native, api, beta int
}

func (r *accessAuthResolver) ResolveAccessToken(ctx context.Context, token string) (*service.AuthIdentity, common.ErrorCode, error) {
	r.native++
	u, err := dao.NewUserDAO().GetByAccessToken(ctx, r.db, token)
	if err != nil {
		return nil, common.CodeUnauthorized, err
	}
	return &service.AuthIdentity{User: u, TenantID: u.ID}, common.CodeSuccess, nil
}
func (r *accessAuthResolver) ResolveAPIToken(ctx context.Context, token string) (*service.AuthIdentity, common.ErrorCode, error) {
	r.api++
	row, err := dao.NewAPITokenDAO().GetByAPIToken(ctx, r.db, token)
	if err != nil || row == nil {
		return nil, common.CodeUnauthorized, errors.New("synthetic API unavailable")
	}
	return &service.AuthIdentity{User: &entity.User{ID: row.TenantID}, TenantID: row.TenantID}, common.CodeSuccess, nil
}
func (r *accessAuthResolver) ResolveBetaAPIToken(context.Context, string) (*service.AuthIdentity, common.ErrorCode, error) {
	r.beta++
	return nil, common.CodeUnauthorized, errors.New("synthetic beta unavailable")
}

func TestKnowledgeAccessNativeOnlyHTTPAuthKeepsAuthorizationPrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	trace := &accessAuthTrace{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.New(trace, logger.Config{LogLevel: logger.Info})})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err = db.AutoMigrate(&entity.User{}, &entity.APIToken{}); err != nil {
		t.Fatal(err)
	}
	const marker = "Synthetic_Authorization_Sentinel_No_Real_Credential"
	const native = marker + "_Native"
	const api = marker + "_API"
	valid, nativeToken := "1", native
	if err = db.Create(&entity.User{ID: strings.Repeat("1", 32), Nickname: "Synthetic", Email: "synthetic@invalid", AccessToken: &nativeToken, Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&entity.APIToken{Token: api, TenantID: strings.Repeat("1", 32)}).Error; err != nil {
		t.Fatal(err)
	}
	core, appLogs := observer.New(zap.DebugLevel)
	oldLogger, oldSugar, oldStatus := common.Logger, common.Sugar, local.GetAdminStatus()
	common.Logger = zap.New(core)
	common.Sugar = common.Logger.Sugar()
	local.InitAdminStatus()
	local.SetAdminStatus(local.Available, "")
	t.Cleanup(func() {
		local.SetAdminStatus(oldStatus.Status, oldStatus.Reason)
		common.Logger, common.Sugar = oldLogger, oldSugar
	})
	for _, test := range []struct {
		name, method        string
		tokens              []string
		status, nativeCalls int
	}{
		{"wrong method", "GET", []string{native}, 405, 0},
		{"missing", "POST", nil, 401, 0},
		{"empty", "POST", []string{""}, 401, 0},
		{"blank", "POST", []string{" \t "}, 401, 0},
		{"native success", "POST", []string{native}, 204, 1},
		{"native failure", "POST", []string{marker + "_Invalid"}, 401, 1},
		{"API token", "POST", []string{api}, 401, 1},
		{"beta token", "POST", []string{marker + "_Beta"}, 401, 1},
		{"duplicate same", "POST", []string{native, native}, 401, 0},
		{"duplicate different", "POST", []string{native, marker + "_Extra"}, 401, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := &accessAuthResolver{db: db}
			h := &AuthHandler{userService: resolver}
			engine := gin.New()
			reached := false
			engine.Any("/access", h.KnowledgeAccessAuth("read"), func(c *gin.Context) {
				reached = true
				if c.GetString("user_id") != strings.Repeat("1", 32) || c.GetBool("auth_via_api_token") {
					t.Error("native identity lost")
				}
				c.Status(204)
			})
			trace.Reset()
			appLogs.TakeAll()
			req := httptest.NewRequest(test.method, "/access", nil)
			for i, token := range test.tokens {
				header := "Authorization"
				if i > 0 {
					header = "aUtHoRiZaTiOn"
				}
				req.Header.Add(header, token)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != test.status || resolver.native != test.nativeCalls || resolver.api != 0 || resolver.beta != 0 || reached != (test.status == 204) {
				t.Fatalf("status=%d native=%d api=%d beta=%d handler=%t", rec.Code, resolver.native, resolver.api, resolver.beta, reached)
			}
			if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Knowledge-Access-Version") != entity.KnowledgeAccessVersion || len(rec.Header().Get("X-Request-ID")) != 32 {
				t.Fatal("missing private response headers")
			}
			if test.status == 405 && rec.Header().Get("Allow") != "POST" {
				t.Fatal("missing Allow")
			}
			if strings.Contains(trace.String(), marker) || strings.Contains(rec.Body.String(), marker) || strings.Contains(fmt.Sprint(rec.Header()), marker) {
				t.Fatal("authorization escaped into SQL diagnostics or response")
			}
			for _, entry := range appLogs.All() {
				if strings.Contains(fmt.Sprint(entry), marker) {
					t.Fatal("authorization escaped into application log")
				}
			}
			if test.status != 204 {
				if err := entity.KnowledgeAccessDecode(rec.Body.Bytes(), "ReadResponse", nil); err != nil {
					t.Fatal(err)
				}
			}
			if test.status == 401 {
				var body entity.KnowledgeAccessError
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "unauthenticated" || !body.OutcomeKnown || body.RetrySameKey {
					t.Fatal("authentication failure must use the closed unauthenticated response")
				}
			}
		})
	}
	// The ordinary entry still invokes the API DAO and emits ordinary SQL
	// diagnostics. This control prevents globally silencing existing logging.
	resolver := &accessAuthResolver{db: db}
	h := &AuthHandler{userService: resolver}
	engine := gin.New()
	engine.POST("/ordinary", h.AuthMiddleware(), func(c *gin.Context) { c.Status(204) })
	trace.Reset()
	req := httptest.NewRequest("POST", "/ordinary", nil)
	req.Header.Set("Authorization", api)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 204 || resolver.api != 1 || !strings.Contains(trace.String(), api) {
		t.Fatal("ordinary API fallback or SQL diagnostic control changed")
	}
}

func TestKnowledgeAccessHandlerRejectsAPITokenContext(t *testing.T) {
	engine := gin.New()
	h := &DatasetsHandler{}
	engine.POST("/access", func(c *gin.Context) {
		c.Set("auth_via_api_token", true)
		c.Set("knowledge_access_request_id", strings.Repeat("2", 32))
	}, h.KnowledgeAccess("read"))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest("POST", "/access", nil))
	if rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "unauthenticated" || body["data"] != nil {
		t.Fatal("API token context entered business service")
	}
}

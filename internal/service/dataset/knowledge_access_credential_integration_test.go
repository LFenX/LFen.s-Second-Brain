//go:build integration && linux

package dataset

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/mcp"
)

type credentialMySQLInput struct {
	Purpose        string    `json:"purpose"`
	ReferenceID    string    `json:"reference_id"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	RunID          string    `json:"run_id"`
	ExpiresAt      time.Time `json:"expires_at"`
	Schema         string    `json:"schema"`
	Address        string    `json:"address"`
	ServerUUID     string    `json:"server_uuid"`
	DSN            string    `json:"dsn"`
	Token          string    `json:"token"`
	UserID         string    `json:"user_id"`
	DomainID       string    `json:"domain_id"`
}

// Root supplies a fresh disposable fixture through one private stdin pipe.
// This test has no DDL, fixture writes, native session, storage or model client.
// Closing this process's own SQL pool does not stop the shared MySQL service.
func TestKnowledgeAccessCredentialMySQLPrimaryFailures(t *testing.T) {
	stat, err := os.Stdin.Stat()
	if err != nil || stat.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("credential integration requires a private input pipe")
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 65537))
	if err != nil || len(raw) > 65536 {
		t.Fatal("credential integration input refused")
	}
	defer clear(raw)
	var input credentialMySQLInput
	if entity.KnowledgeParsedDecode(raw, &input) != nil || input.Purpose != "root-synthetic-credential-mysql-integration" || input.ReferenceID != "KCRL-20261116-44" || input.ManifestSHA256 != "df9a53d5bbbf483bc76ce72bda9e4643e937c6bd5b1a4f2e866b9a65745b8547" || !isKnowledgeID(input.RunID, 32) || !isKnowledgeID(input.UserID, 32) || !isKnowledgeID(input.DomainID, 32) || input.Schema != "second_brain_s1_acceptance_20261116" || !time.Now().Before(input.ExpiresAt) || input.ExpiresAt.After(time.Now().Add(3*time.Minute)) {
		t.Fatal("credential integration lease refused")
	}
	defer func() { input.Token = ""; input.DSN = "" }()
	parsed, err := mysqlDriver.ParseDSN(input.DSN)
	host, port, endpointErr := net.SplitHostPort(input.Address)
	if err != nil || endpointErr != nil || net.ParseIP(host) == nil || port != "3306" || parsed.Net != "tcp" || parsed.Addr != input.Address || parsed.DBName != input.Schema || !parsed.ParseTime || parsed.InterpolateParams || parsed.MultiStatements || parsed.TLSConfig != "" {
		t.Fatal("credential integration database endpoint refused")
	}
	parsed.Timeout = 5 * time.Second
	parsed.ReadTimeout = 5 * time.Second
	parsed.WriteTimeout = 5 * time.Second
	parsed.Loc = time.UTC
	pool, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		t.Fatal("credential integration pool failed")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(1)
	ctx, cancel := context.WithDeadline(t.Context(), input.ExpiresAt)
	defer cancel()
	var schema, server, version string
	var readOnly int
	if pool.QueryRowContext(ctx, "SELECT DATABASE(),@@server_uuid,@@version,@@read_only").Scan(&schema, &server, &version, &readOnly) != nil || schema != input.Schema || server != input.ServerUUID || !strings.HasPrefix(version, "8.") || readOnly != 0 {
		t.Fatal("credential integration primary identity failed")
	}
	db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		t.Fatal("credential integration GORM failed")
	}
	oldDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = oldDB }()
	const audience = "http://localhost:29398/api/v1/knowledge/access/mcp"
	t.Setenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE", audience)
	identity, err := accessParseCredential("Bearer " + input.Token)
	if err != nil {
		t.Fatal("credential integration bearer refused")
	}
	var row entity.KnowledgeAccessCredentialRecord
	query := db.WithContext(ctx).Where("id = ?", identity.ID)
	dry := query.Session(&gorm.Session{DryRun: true}).Take(&entity.KnowledgeAccessCredentialRecord{})
	if dry.Error != nil || !strings.Contains(dry.Statement.SQL.String(), "id = ?") || strings.Contains(dry.Statement.SQL.String(), identity.ID) || len(dry.Statement.Vars) != 1 {
		t.Fatal("credential integration UUID SQL is not bound")
	}
	if query.Take(&row).Error != nil || row.SubjectUserID != input.UserID || row.DomainID != input.DomainID || row.Audience != audience || len(row.VerifierSHA256) != 32 || subtle.ConstantTimeCompare(row.VerifierSHA256, identity.VerifierSHA256) != 1 || row.RevokedAt != nil || !time.Now().Before(row.ExpiresAt) || accessCredentialIntegrity(db.WithContext(ctx), row) != nil {
		t.Fatal("credential integration real row or creation receipt failed")
	}
	var account struct{ Email string }
	if db.WithContext(ctx).Table("user").Select("email").Where("id = ?", input.UserID).Take(&account).Error != nil || !strings.HasSuffix(account.Email, "@fixtures.invalid") {
		t.Fatal("credential integration fixture ownership failed")
	}
	var before int64
	if db.WithContext(ctx).Model(&entity.KnowledgeAccessAuditRecord{}).Where("credential_id = ?", row.ID).Count(&before).Error != nil {
		t.Fatal("credential integration audit baseline failed")
	}
	service := &DatasetService{}
	handler, err := mcp.NewKnowledgeAccessHandler(service, func(err error) mcp.KnowledgeAccessFailure {
		var failure *KnowledgeAccessError
		if errors.As(err, &failure) {
			return mcp.KnowledgeAccessFailure{Status: failure.Status, Code: failure.Code, OutcomeKnown: failure.OutcomeKnown, RetrySameKey: failure.RetrySameKey}
		}
		return mcp.KnowledgeAccessFailure{Status: 503, Code: "unavailable", RetrySameKey: true}
	}, audience)
	if err != nil {
		t.Fatal("credential integration managed handler failed")
	}
	call := func(token string, want int, code string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, audience, strings.NewReader(`{"jsonrpc":"2.0","id":"synthetic-primary-probe","method":"tools/list","params":{}}`))
		request = request.WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Mcp-Protocol-Version", "2025-11-25")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		body := response.Body.Bytes()
		var value entity.KnowledgeAccessError
		var outer map[string]json.RawMessage
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" || len(body) > 1<<20 || json.Unmarshal(body, &value) != nil || json.Unmarshal(body, &outer) != nil || value.Code != code || value.Data != nil || value.Operation != "mcpTransport" || outer["result"] != nil || bytes.Contains(body, []byte(input.Token)) {
			t.Fatal("credential integration transport failure mapping failed")
		}
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal("credential integration synthetic secret failed")
	}
	defer clear(secret)
	wrong := "kac1_" + row.ID + "." + base64.RawURLEncoding.EncodeToString(secret)
	if wrong == input.Token {
		t.Fatal("credential integration replacement secret collided")
	}
	call(wrong, 401, "unauthenticated")
	unknown := "kac1_" + input.RunID + "." + base64.RawURLEncoding.EncodeToString(secret)
	if input.RunID == row.ID {
		t.Fatal("credential integration unknown ID collided")
	}
	call(unknown, 401, "unauthenticated")
	var after int64
	if db.WithContext(ctx).Model(&entity.KnowledgeAccessAuditRecord{}).Where("credential_id = ?", row.ID).Count(&after).Error != nil || after != before {
		t.Fatal("credential integration rejected auth created a reservation")
	}
	if pool.Close() != nil {
		t.Fatal("credential integration own pool close failed")
	}
	call(input.Token, 503, "unavailable")
	t.Log(`{"state":"Passed_RealMySQLCredentialPrimaryFailures","knownWrongSecret":401,"unknownCredential":401,"ownClosedPrimaryPool":503,"credentialVerifierBytes":32,"boundUUIDSQL":true,"durableCreationReceipt":true,"sharedDatabaseStopped":false,"fixtureWrites":0,"models":0}`)
}

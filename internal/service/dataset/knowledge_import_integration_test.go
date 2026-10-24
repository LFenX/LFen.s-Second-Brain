//go:build integration

package dataset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/server/config"
	"ragflow/internal/storage"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This integration suite creates its own database and synthetic buckets. It never
// loads application/model credentials or changes the running application's storage.
type knowledgeImportTestConfig struct {
	MySQLDSN      string `json:"mysql_dsn"`
	MinioHost     string `json:"minio_host"`
	MinioUser     string `json:"minio_user"`
	MinioPassword string `json:"minio_password"`
	EvidenceDir   string `json:"evidence_dir"`
	RestoreDB     string `json:"restore_db"`
}

type knowledgeImportIntegration struct {
	service    *DatasetService
	db         *gorm.DB
	store      storage.Storage
	config     knowledgeImportTestConfig
	database   string
	blockedKey atomic.Value
	blocked    atomic.Int64
}

type knowledgeImportBackupObject struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Bytes  []byte `json:"bytes"`
}

type knowledgeImportHTTPGuard struct {
	base      http.RoundTripper
	minioHost string
	denied    atomic.Int64
}

func (g *knowledgeImportHTTPGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != g.minioHost {
		g.denied.Add(1)
		return nil, errors.New("external HTTP is prohibited in the synthetic C2 suite")
	}
	return g.base.RoundTrip(req)
}

func readKnowledgeImportTestConfig(t *testing.T) knowledgeImportTestConfig {
	t.Helper()
	filename := os.Getenv("LFEN_C2_TEST_CONFIG")
	if filename == "" {
		t.Skip("LFEN_C2_TEST_CONFIG is unset; opt-in MySQL/MinIO integration suite")
	}
	if os.Getenv("LFEN_POC_SYNTHETIC_ONLY") != "1" {
		t.Fatal("LFEN_POC_SYNTHETIC_ONLY=1 is required")
	}
	var cfg knowledgeImportTestConfig
	raw, err := os.ReadFile(filename)
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.MySQLDSN == "" || cfg.MinioHost == "" || cfg.EvidenceDir == "" {
		t.Fatal("invalid private C2 integration configuration")
	}
	return cfg
}

func openKnowledgeImportMySQL(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open isolated integration database (details omitted to protect DSN)")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(12)
	pool.SetMaxIdleConns(12)
	t.Cleanup(func() { _ = pool.Close() })
	return db
}

func setupKnowledgeImportIntegration(t *testing.T) *knowledgeImportIntegration {
	t.Helper()
	cfg := readKnowledgeImportTestConfig(t)
	dsn, err := mysqldriver.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		t.Fatal("invalid private MySQL DSN")
	}
	dsn.DBName = ""
	dsn.ParseTime = true
	dsn.Timeout = 5 * time.Second
	dsn.ReadTimeout = 30 * time.Second
	dsn.WriteTimeout = 30 * time.Second
	admin := openKnowledgeImportMySQL(t, dsn.FormatDSN())
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	database := "lfen_c2_test_" + hex.EncodeToString(random)
	if err := admin.Exec("CREATE DATABASE `" + database + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error; err != nil {
		t.Fatal("cannot create task-owned synthetic database")
	}
	dsn.DBName = database
	db := openKnowledgeImportMySQL(t, dsn.FormatDSN())
	pushServiceDB(t, db)
	if err := db.AutoMigrate(
		&entity.Knowledgebase{}, &entity.Document{}, &entity.File{}, &entity.File2Document{},
		&entity.User{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.Connector2Kb{},
		&entity.Task{}, &entity.IngestionTask{}, &entity.IngestionTaskLog{},
		&entity.KnowledgeGroup{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeNode{},
		&entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeSourceRegistration{},
		&entity.KnowledgeSourceBinding{}, &entity.KnowledgeImportPreview{}, &entity.KnowledgeImport{},
	); err != nil {
		t.Fatal(err)
	}
	suite := &knowledgeImportIntegration{db: db, config: cfg, database: database}
	suite.blockedKey.Store("")
	upstream, err := url.Parse("http://" + cfg.MinioHost)
	if err != nil {
		t.Fatal("invalid private MinIO endpoint")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pattern := suite.blockedKey.Load().(string)
		if r.Method == http.MethodPut && pattern != "" && strings.Contains(r.URL.Path, pattern) {
			suite.blocked.Add(1)
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "<Error><Code>AccessDenied</Code><Message>Synthetic request-scoped storage failure</Message><RequestId>synthetic</RequestId></Error>")
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	store, err := storage.NewMinioStorage(config.MinioConfig{Host: strings.TrimPrefix(server.URL, "http://"), User: cfg.MinioUser, Password: cfg.MinioPassword})
	if err != nil {
		t.Fatal("cannot create isolated MinIO client")
	}
	suite.store = store
	oldStore := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(store)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(oldStore) })
	suite.service = NewDatasetService()
	valid := "1"
	if err := db.Create(&entity.UserTenant{ID: strings.Repeat("c", 32), UserID: knowledgeTestOwner, TenantID: knowledgeTestOwner, InvitedBy: knowledgeTestOwner, Role: "owner", Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.EvidenceDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeKnowledgeIntegrationEvidence(t, filepath.Join(cfg.EvidenceDir, "database.json"), map[string]any{"database": database, "synthetic_only": true, "retained_for_recovery_check": true})
	return suite
}

func writeKnowledgeIntegrationEvidence(t *testing.T, filename string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil || os.WriteFile(filename, append(raw, '\n'), 0600) != nil {
		t.Fatal("cannot save private integration evidence")
	}
}

func (s *knowledgeImportIntegration) domain(t *testing.T) string {
	t.Helper()
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(random)
	valid := "1"
	if err := s.db.Create(&entity.Knowledgebase{ID: id, CreatedBy: knowledgeTestOwner, TenantID: knowledgeTestOwner, Name: "Synthetic integration", Permission: "me", Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	registerKnowledgeTest(t, s.service, id)
	return id
}

func (s *knowledgeImportIntegration) registration(t *testing.T, domain, key string) *entity.KnowledgeSourceRegistration {
	t.Helper()
	reg, created, err := s.service.CreateKnowledgeRegistration(t.Context(), knowledgeTestOwner, domain, entity.CreateKnowledgeRegistrationRequest{RegistrationKey: key, Name: "Synthetic", SourceKind: "text_file", InputMode: "inline_text", IncludePaths: []string{"web/card.tsx"}, ComponentSide: "frontend", Language: "tsx"})
	if err != nil || !created {
		t.Fatalf("synthetic registration failed: created=%t err=%v", created, err)
	}
	return reg
}

type knowledgeConcurrentResult struct {
	receipt *entity.KnowledgeImportReceipt
	created bool
	err     error
}

func concurrentKnowledgeCommits(s *DatasetService, domain, registration string, requests []entity.CommitKnowledgeImportRequest) []knowledgeConcurrentResult {
	results := make([]knowledgeConcurrentResult, len(requests))
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(len(requests))
	done.Add(len(requests))
	for i := range requests {
		go func(i int) {
			defer done.Done()
			ready.Done()
			<-start
			results[i].receipt, results[i].created, results[i].err = s.CommitKnowledgeImport(context.Background(), knowledgeTestOwner, domain, registration, requests[i])
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	return results
}

func assertKnowledgeImportCounts(t *testing.T, db *gorm.DB, domain, registration string, documents, revisions, receipts int64) {
	t.Helper()
	for _, check := range []struct {
		model any
		field string
		id    string
		want  int64
	}{
		{&entity.Document{}, "kb_id", domain, documents},
		{&entity.KnowledgeSourceBinding{}, "registration_id", registration, documents},
		{&entity.KnowledgeSourceRevision{}, "domain_id", domain, revisions},
		{&entity.KnowledgeImport{}, "registration_id", registration, receipts},
	} {
		var count int64
		if err := db.Model(check.model).Where(check.field+" = ?", check.id).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("%T count=%d want=%d err=%v", check.model, count, check.want, err)
		}
	}
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", domain).Take(&kb).Error; err != nil || kb.DocNum != documents || kb.ChunkNum != 0 || kb.TokenNum != 0 {
		t.Fatalf("native counters inconsistent: documents=%d err=%v", kb.DocNum, err)
	}
}

func TestKnowledgeImportMySQL(t *testing.T) {
	suite := setupKnowledgeImportIntegration(t)
	minioURL, _ := url.Parse("http://" + suite.config.MinioHost)
	guard := &knowledgeImportHTTPGuard{base: http.DefaultTransport, minioHost: minioURL.Hostname()}
	http.DefaultTransport = guard
	t.Cleanup(func() { http.DefaultTransport = guard.base })
	t.Run("Concurrency", func(t *testing.T) {
		for round := 0; round < 5; round++ {
			t.Run(fmt.Sprintf("round_%d", round), func(t *testing.T) {
				domain := suite.domain(t)
				req := entity.CreateKnowledgeRegistrationRequest{RegistrationKey: "same", Name: "Synthetic", SourceKind: "text_file", InputMode: "inline_text", IncludePaths: []string{"web/card.tsx"}, ComponentSide: "frontend", Language: "tsx"}
				var registrations [8]*entity.KnowledgeSourceRegistration
				var errs [8]error
				var created [8]bool
				start := make(chan struct{})
				var done sync.WaitGroup
				done.Add(8)
				for i := range 8 {
					go func() {
						defer done.Done()
						<-start
						registrations[i], created[i], errs[i] = suite.service.CreateKnowledgeRegistration(t.Context(), knowledgeTestOwner, domain, req)
					}()
				}
				close(start)
				done.Wait()
				newCount := 0
				for i := range 8 {
					if errs[i] != nil || registrations[i] == nil || registrations[i].ID != registrations[0].ID {
						t.Fatalf("concurrent registration %d failed: %v", i, errs[i])
					}
					if created[i] {
						newCount++
					}
				}
				if newCount != 1 {
					t.Fatalf("registrations created=%d", newCount)
				}
				reg := registrations[0]
				p := previewKnowledgeTest(t, suite.service, reg, "// synthetic first\n")
				requests := make([]entity.CommitKnowledgeImportRequest, 8)
				for i := range requests {
					requests[i] = entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "same-key"}
				}
				results := concurrentKnowledgeCommits(suite.service, domain, reg.ID, requests)
				newCount = 0
				for i, r := range results {
					if r.err != nil || r.receipt == nil || r.receipt.Status != "succeeded" || r.receipt.ID != results[0].receipt.ID {
						t.Fatalf("same-key commit %d: %v", i, r.err)
					}
					if r.created {
						newCount++
					}
				}
				if newCount != 1 {
					t.Fatalf("same-key publications=%d", newCount)
				}
				first := results[0].receipt
				assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 1, 1)
				p = previewKnowledgeTest(t, suite.service, reg, "// synthetic second\n")
				for i := range requests {
					requests[i] = entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: fmt.Sprintf("different_%d", i)}
				}
				results = concurrentKnowledgeCommits(suite.service, domain, reg.ID, requests)
				newCount = 0
				for _, r := range results {
					if r.err == nil {
						if r.receipt.Status != "succeeded" {
							t.Fatal("unexpected failed publish")
						}
						newCount++
					} else {
						assertKnowledgeStatus(t, r.err, 409)
					}
				}
				if newCount != 1 {
					t.Fatalf("single-preview consumers=%d", newCount)
				}
				assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 2, 2)
				for i := range requests {
					p = previewKnowledgeTest(t, suite.service, reg, fmt.Sprintf("// competing update %d\n", i))
					requests[i] = entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: fmt.Sprintf("competing_%d", i)}
				}
				results = concurrentKnowledgeCommits(suite.service, domain, reg.ID, requests)
				newCount = 0
				for _, r := range results {
					if r.err == nil {
						newCount++
						if r.receipt.Status != "succeeded" || *r.receipt.Items[0].SourceID != *first.Items[0].SourceID {
							t.Fatal("source identity changed")
						}
					} else {
						assertKnowledgeStatus(t, r.err, 409)
					}
				}
				if newCount != 1 {
					t.Fatalf("competing successful updates=%d", newCount)
				}
				assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 3, 3)
				old, err := suite.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, domain, *first.Items[0].RootNodeID, nil, nil)
				if err != nil || old.Text != "// synthetic first\n" {
					t.Fatalf("old revision changed: %v", err)
				}
			})
		}
	})
	t.Run("Failure", func(t *testing.T) {
		for _, pattern := range []string{"/.knowledge-import/", "/.objects/"} {
			t.Run(strings.Trim(pattern, "/."), func(t *testing.T) {
				domain := suite.domain(t)
				reg := suite.registration(t, domain, "failure")
				first := commitKnowledgeTest(t, suite.service, reg, previewKnowledgeTest(t, suite.service, reg, "// old synthetic bytes\n"), "first")
				p := previewKnowledgeTest(t, suite.service, reg, "// new synthetic bytes\n")
				suite.blockedKey.Store(pattern)
				before := suite.blocked.Load()
				req := entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "fault"}
				failed, created, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
				suite.blockedKey.Store("")
				if err != nil || !created || failed.Status != "failed" || failed.Items[0].SourceID != nil || suite.blocked.Load() <= before {
					t.Fatalf("fault did not produce failed receipt: %v", err)
				}
				again, created, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
				if err != nil || created || again.ID != failed.ID || again.Status != "failed" {
					t.Fatalf("failure replay failed: %v", err)
				}
				assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 1, 2)
				old, err := suite.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, domain, *first.Items[0].RootNodeID, nil, nil)
				if err != nil || old.Text != "// old synthetic bytes\n" {
					t.Fatalf("fault changed original: %v", err)
				}
				retry := commitKnowledgeTest(t, suite.service, reg, previewKnowledgeTest(t, suite.service, reg, "// new synthetic bytes\n"), "retry")
				if *retry.Items[0].SourceID != *first.Items[0].SourceID {
					t.Fatal("retry duplicated source")
				}
				assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 2, 3)
			})
		}
		t.Run("database_publication", func(t *testing.T) {
			domain := suite.domain(t)
			reg := suite.registration(t, domain, "database")
			first := commitKnowledgeTest(t, suite.service, reg, previewKnowledgeTest(t, suite.service, reg, "// old\n"), "first")
			p := previewKnowledgeTest(t, suite.service, reg, "// changed\n")
			name := "c2_synthetic_publication_fault"
			if err := suite.db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == "knowledge_source_revision" {
					tx.AddError(errors.New("synthetic database publication fault"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			failed, created, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "fault"})
			_ = suite.db.Callback().Create().Remove(name)
			if err != nil || !created || failed.Status != "failed" {
				t.Fatalf("database failure receipt: %v", err)
			}
			assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 1, 2)
			current, err := knowledgeImportCurrent(suite.db, domain, *first.Items[0].SourceID)
			if err != nil || current.ID != *first.Items[0].SourceRevisionID {
				t.Fatal("database fault changed current revision")
			}
		})
		t.Run("receipt_persistence", func(t *testing.T) {
			domain := suite.domain(t)
			reg := suite.registration(t, domain, "receipt")
			p := previewKnowledgeTest(t, suite.service, reg, "// synthetic receipt fault\n")
			name := "c2_synthetic_receipt_fault"
			if err := suite.db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == "knowledge_import" {
					tx.AddError(errors.New("synthetic receipt persistence fault"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			req := entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "same-key"}
			_, created, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
			_ = suite.db.Callback().Create().Remove(name)
			if err == nil || created {
				t.Fatal("missing receipt persistence error")
			}
			assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 0, 0, 0)
			var staged entity.KnowledgeImportPreview
			if err := suite.db.Where("id = ?", p.ID).Take(&staged).Error; err != nil || staged.ConsumedTaskID != nil {
				t.Fatal("failed transaction consumed preview")
			}
			replayed, created, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
			if err != nil || !created || replayed.Status != "succeeded" {
				t.Fatalf("uncertain-result same-key retry: %v", err)
			}
			assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 1, 1)
		})
		t.Run("client_timeout", func(t *testing.T) {
			domain := suite.domain(t)
			reg := suite.registration(t, domain, "timeout")
			p := previewKnowledgeTest(t, suite.service, reg, "// synthetic timeout\n")
			req := entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "same-key"}
			committed := make(chan string, 1)
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receipt, _, err := suite.service.CommitKnowledgeImport(r.Context(), knowledgeTestOwner, domain, reg.ID, req)
				if err != nil {
					committed <- ""
					return
				}
				committed <- receipt.ID
				<-release
				_ = json.NewEncoder(w).Encode(receipt)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
			done := make(chan error, 1)
			go func() {
				response, err := http.DefaultClient.Do(request)
				if response != nil {
					_ = response.Body.Close()
				}
				done <- err
			}()
			id := <-committed
			err := <-done
			close(release)
			if id == "" || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("client did not lose committed response")
			}
			again, created, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
			if err != nil || created || again.ID != id {
				t.Fatalf("timeout replay duplicated receipt: %v", err)
			}
			assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 1, 1, 1)
		})
	})
	t.Run("Permission", func(t *testing.T) {
		domain := suite.domain(t)
		reg := suite.registration(t, domain, "revocation")
		p := previewKnowledgeTest(t, suite.service, reg, "// private synthetic bytes\n")
		if err := suite.db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "0").Error; err != nil {
			t.Fatal(err)
		}
		req := entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "revoked"}
		_, _, err := suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
		assertKnowledgeStatus(t, err, 404)
		_, err = suite.service.GetKnowledgeImportPreview(t.Context(), knowledgeTestOwner, domain, reg.ID, p.ID)
		assertKnowledgeStatus(t, err, 404)
		assertKnowledgeImportCounts(t, suite.db, domain, reg.ID, 0, 0, 0)
		if err := suite.db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		first := commitKnowledgeTest(t, suite.service, reg, p, "revoked")
		if err := suite.db.Model(&entity.Knowledgebase{}).Where("id = ?", domain).Update("created_by", strings.Repeat("b", 32)).Error; err != nil {
			t.Fatal(err)
		}
		_, _, err = suite.service.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, req)
		assertKnowledgeStatus(t, err, 404)
		_, err = suite.service.GetKnowledgeImport(t.Context(), knowledgeTestOwner, domain, reg.ID, first.ID)
		assertKnowledgeStatus(t, err, 404)
		_, err = suite.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, domain, *first.Items[0].RootNodeID, nil, nil)
		assertKnowledgeStatus(t, err, 404)
	})
	t.Run("Persistence", func(t *testing.T) {
		domain := suite.domain(t)
		reg := suite.registration(t, domain, "recovery")
		text := "// 合成恢复 🧠\r\n"
		first := commitKnowledgeTest(t, suite.service, reg, previewKnowledgeTest(t, suite.service, reg, text), "first")
		second := commitKnowledgeTest(t, suite.service, reg, previewKnowledgeTest(t, suite.service, reg, text+"// v2\n"), "second")
		var revision entity.KnowledgeSourceRevision
		if err := suite.db.Where("id = ?", *first.Items[0].SourceRevisionID).Take(&revision).Error; err != nil {
			t.Fatal(err)
		}
		var objects []knowledgeImportBackupObject
		// Native original files and immutable snapshots use distinct buckets.
		for _, bucket := range []string{domain, revision.FolderID} {
			keys, err := suite.store.ListObjects(t.Context(), bucket)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range keys {
				content, err := suite.store.Get(t.Context(), bucket, key)
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(content)
				objects = append(objects, knowledgeImportBackupObject{Bucket: bucket, Key: key, SHA256: hex.EncodeToString(digest[:]), Bytes: content})
			}
		}
		if len(objects) == 0 {
			t.Fatal("synthetic recovery backup has no objects")
		}
		writeKnowledgeIntegrationEvidence(t, filepath.Join(suite.config.EvidenceDir, "objects.private.json"), objects)
		writeKnowledgeIntegrationEvidence(t, filepath.Join(suite.config.EvidenceDir, "recovery.json"), map[string]any{"database": suite.database, "domain_id": domain, "registration_id": reg.ID, "first": first, "second": second, "original_text": text, "updated_text": text + "// v2\n"})
	})
	for _, model := range []any{&entity.IngestionTask{}, &entity.Task{}} {
		var count int64
		if err := suite.db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("unexpected automatic processing: %T count=%d err=%v", model, count, err)
		}
	}
	if guard.denied.Load() != 0 {
		t.Fatal("unexpected attempt to use external default HTTP transport")
	}
	t.Logf("synthetic_database=%s blocked_storage_requests=%d prohibited_default_http_attempts=%d ingestion_tasks=0", suite.database, suite.blocked.Load(), guard.denied.Load())
}

func TestKnowledgeImportRestoredMySQL(t *testing.T) {
	cfg := readKnowledgeImportTestConfig(t)
	if !strings.HasPrefix(cfg.RestoreDB, "lfen_c2_test_restore_") || strings.Trim(cfg.RestoreDB, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
		t.Fatal("restore_db must name a task-owned lfen_c2_test_restore_ database")
	}
	dsn, err := mysqldriver.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		t.Fatal("invalid private restore DSN")
	}
	dsn.DBName, dsn.ParseTime = cfg.RestoreDB, true
	db := openKnowledgeImportMySQL(t, dsn.FormatDSN())
	pushServiceDB(t, db)
	store, err := storage.NewMinioStorage(config.MinioConfig{Host: cfg.MinioHost, User: cfg.MinioUser, Password: cfg.MinioPassword})
	if err != nil {
		t.Fatal("cannot reopen original synthetic object storage")
	}
	var evidence struct {
		DomainID       string                        `json:"domain_id"`
		RegistrationID string                        `json:"registration_id"`
		First          entity.KnowledgeImportReceipt `json:"first"`
		Second         entity.KnowledgeImportReceipt `json:"second"`
		OriginalText   string                        `json:"original_text"`
		UpdatedText    string                        `json:"updated_text"`
	}
	raw, err := os.ReadFile(filepath.Join(cfg.EvidenceDir, "recovery.json"))
	if err != nil || json.Unmarshal(raw, &evidence) != nil {
		t.Fatal("missing synthetic recovery fixture")
	}
	var objects []knowledgeImportBackupObject
	raw, err = os.ReadFile(filepath.Join(cfg.EvidenceDir, "objects.private.json"))
	if err != nil || json.Unmarshal(raw, &objects) != nil || len(objects) == 0 {
		t.Fatal("missing synthetic object backup")
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	restoreBucket := "lfen-c2-restore-" + hex.EncodeToString(random)
	for _, object := range objects {
		digest := sha256.Sum256(object.Bytes)
		if hex.EncodeToString(digest[:]) != object.SHA256 {
			t.Fatal("backup object checksum mismatch")
		}
		if object.Bucket == "" {
			t.Fatal("backup omitted original bucket identity")
		}
		if err := store.Put(t.Context(), restoreBucket, object.Bucket+"/"+object.Key, object.Bytes); err != nil {
			t.Fatal("cannot restore synthetic object")
		}
	}
	restoredStore, err := storage.NewMinioStorage(config.MinioConfig{Host: cfg.MinioHost, User: cfg.MinioUser, Password: cfg.MinioPassword, Bucket: restoreBucket})
	if err != nil {
		t.Fatal("cannot open restored object namespace")
	}
	old := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(restoredStore)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(old) })
	service := NewDatasetService()
	for _, check := range []struct {
		receipt entity.KnowledgeImportReceipt
		text    string
	}{{evidence.First, evidence.OriginalText}, {evidence.Second, evidence.UpdatedText}} {
		receipt, err := service.GetKnowledgeImport(t.Context(), knowledgeTestOwner, evidence.DomainID, evidence.RegistrationID, check.receipt.ID)
		if err != nil || receipt.Status != "succeeded" || receipt.ID != check.receipt.ID {
			t.Fatalf("restored receipt unreadable: %v", err)
		}
		content, err := service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, evidence.DomainID, *receipt.Items[0].RootNodeID, nil, nil)
		if err != nil || content.Text != check.text || content.SourceRevisionID != *check.receipt.Items[0].SourceRevisionID {
			t.Fatalf("restored fixed revision differs: %v", err)
		}
	}
	assertKnowledgeImportCounts(t, db, evidence.DomainID, evidence.RegistrationID, 1, 2, 2)
	writeKnowledgeIntegrationEvidence(t, filepath.Join(cfg.EvidenceDir, "restored.json"), map[string]any{"restore_database": cfg.RestoreDB, "restore_bucket": restoreBucket, "restored_object_count": len(objects), "fixed_revisions_read": 2, "receipts_read": 2, "original_database_and_bucket_untouched": true})
	t.Log("task-owned SQL dump and object backup restored into separate namespaces; both immutable revisions and receipts remain readable")
}

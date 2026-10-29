//go:build integration

package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"net/http"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	es "ragflow/internal/engine/elasticsearch"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/server/config"
	"ragflow/internal/service/nlp"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
	"strings"
	"testing"
	"time"
)

type lexicalHTTPGuard struct {
	base    http.RoundTripper
	blocked *int
}

func (g lexicalHTTPGuard) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "http" || r.URL.Host != "kcls-es:9200" {
		*g.blocked++
		return nil, errors.New("non-synthetic HTTP blocked")
	}
	return g.base.RoundTrip(r)
}

type lexicalRealFaultEngine struct {
	engine.DocEngine
	native lexicalEngine
	fail   bool
}

func (e *lexicalRealFaultEngine) PrepareKnowledgeLexical(c context.Context, i, d, l string) error {
	return e.native.PrepareKnowledgeLexical(c, i, d, l)
}
func (e *lexicalRealFaultEngine) VerifyKnowledgeLexicalRows(c context.Context, i, d string, r []map[string]any) error {
	return e.native.VerifyKnowledgeLexicalRows(c, i, d, r)
}
func (e *lexicalRealFaultEngine) CreateKnowledgeLexicalRows(c context.Context, i, d string, r []map[string]any) error {
	if e.fail && len(r) > 0 {
		if err := e.native.CreateKnowledgeLexicalRows(c, i, d, r[:1]); err != nil {
			return err
		}
		return errors.New("synthetic fault after actual ES first row")
	}
	return e.native.CreateKnowledgeLexicalRows(c, i, d, r)
}

func TestKnowledgeLocalLexicalRealSQLAndES(t *testing.T) {
	if os.Getenv("LFEN_POC_SYNTHETIC_ONLY") != "1" {
		t.Skip("isolated synthetic environment not selected")
	}
	if err := common.InitLogger("warn", common.FileOutput{}, ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := sql.Open("mysql", "root@tcp(kcls-mysql:3306)/?parseTime=true&timeout=5s")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	dbName := "lfen_kcls_test_" + common.GenerateUUID()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+dbName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.Open("root@tcp(kcls-mysql:3306)/"+dbName+"?parseTime=true&timeout=5s"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	pushServiceDB(t, db)
	models := []any{&entity.Document{}, &entity.Knowledgebase{}, &entity.KnowledgeSourceRevision{}, &entity.Task{}, &entity.IngestionTask{}, &entity.IngestionTaskLog{}, &entity.File2Document{}, &entity.File{}, &entity.User{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.API4Conversation{}, &entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}, &entity.UserCanvas{}, &entity.KnowledgeGroup{}, &entity.KnowledgeNode{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeDirectoryState{}, &entity.KnowledgeSourceLifecycle{}, &entity.KnowledgeSourceFence{}, &entity.KnowledgeRetiredPosition{}, &entity.KnowledgeLifecycleCommandRecord{}, &entity.KnowledgeLexicalPolicyRecord{}, &entity.KnowledgeLexicalOperationRecord{}, &entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}, &entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureNodeRecord{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	valid := "1"
	token := "synthetic-session-access-token-for-isolated-tests"
	if err := db.Create(&entity.User{ID: knowledgeTestOwner, Nickname: "Synthetic", Email: "synthetic@example.invalid", AccessToken: &token, Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: knowledgeTestDomain, TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "Synthetic lexical integration", Permission: "me", Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.UserTenant{ID: common.GenerateUUID(), UserID: knowledgeTestOwner, TenantID: knowledgeTestOwner, Role: "owner", InvitedBy: knowledgeTestOwner, Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	previousStore := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previousStore) })
	blocked := 0
	previousHTTP := http.DefaultTransport
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	http.DefaultTransport = lexicalHTTPGuard{base: transport, blocked: &blocked}
	t.Cleanup(func() { http.DefaultTransport = previousHTTP })
	native, err := es.NewEngine(ctx, config.ElasticsearchConfig{Hosts: "http://kcls-es:9200"})
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	s := NewDatasetService()
	s.docEngine = native
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	ctx = context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: knowledgeTestOwner, SHA256: lexicalSHA([]byte(token))})
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: "/usr/share/infinity/resource", MinSize: 1, MaxSize: 2}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tokenizer.Close)
	if err := nlp.InitQueryBuilderFromTokenizer("/usr/share/infinity/resource"); err != nil {
		t.Fatal(err)
	}
	oldResolver := component.DefaultEmbedderResolver
	modelAttempts := 0
	component.DefaultEmbedderResolver = func(context.Context, string, string) (component.Embedder, string, error) {
		modelAttempts++
		return nil, "", errors.New("model attempts forbidden")
	}
	t.Cleanup(func() { component.DefaultEmbedderResolver = oldResolver })
	policy, err := s.GetKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain)
	if err != nil {
		t.Fatal(err)
	}
	lexicalSaveEvidence(t, "getLocalPolicy", "policy-default", policy)
	receipt, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "enable", ExpectedFoundationVersion: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	lexicalSaveEvidence(t, "changeLocalPolicy", "policy-enabled", receipt)
	selection := lexicalFixtureSelection(t, s, ctx, store)
	inspect := entity.KnowledgeLexicalInspectRequest{Version: entity.KnowledgeLocalLexicalVersion, Selection: selection}
	unindexed, err := s.InspectKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, inspect)
	if err != nil || unindexed.Readiness.Kind != "unindexed" {
		t.Fatalf("unindexed: %+v %v", unindexed, err)
	}
	lexicalSaveEvidence(t, "inspectIndex", "unindexed", unindexed)
	job, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "index", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	lexicalSaveEvidence(t, "createIndex", "job-created", job)
	building, err := s.InspectKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, inspect)
	if err != nil || building.Readiness.Kind != "building" {
		t.Fatalf("building: %+v %v", building, err)
	}
	lexicalSaveEvidence(t, "inspectIndex", "building", building)
	var record entity.KnowledgeLexicalJobRecord
	if err := db.Where("id = ?", job.ID).Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	record.State = "building"
	record.Claim = common.GenerateUUID()
	record.LeaseUntil = time.Now().Add(time.Minute)
	if err := db.Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	fault := &lexicalRealFaultEngine{DocEngine: native, native: native, fail: true}
	s.docEngine = fault
	record.AttemptCount = 3
	s.runKnowledgeLocalLexical(ctx, record)
	var publishedCount int64
	if err := db.Model(&entity.KnowledgeLexicalPublicationRecord{}).Count(&publishedCount).Error; err != nil || publishedCount != 0 {
		t.Fatalf("half publication: %d %v", publishedCount, err)
	}
	failed, err := s.InspectKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, inspect)
	if err != nil || failed.Readiness.Kind != "failed" {
		t.Fatalf("failed: %+v %v", failed, err)
	}
	lexicalSaveEvidence(t, "inspectIndex", "failed", failed)
	fault.fail = false
	job, err = s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "index-retry", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	record = entity.KnowledgeLexicalJobRecord{}
	if err := db.Where("id = ?", job.ID).Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	record.State = "building"
	record.Claim = common.GenerateUUID()
	record.LeaseUntil = time.Now().Add(time.Minute)
	if err := db.Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.buildKnowledgeLocalLexical(ctx, record); err != nil {
		t.Fatal(err)
	}
	finished, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID)
	if err != nil || finished.State != "published" {
		t.Fatalf("published: %+v %v", finished, err)
	}
	lexicalSaveEvidence(t, "getIndexJob", "job-published", finished)
	ready, err := s.InspectKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, inspect)
	if err != nil || ready.Readiness.Kind != "ready" {
		t.Fatalf("ready: %+v %v", ready, err)
	}
	lexicalSaveEvidence(t, "inspectIndex", "ready", ready)
	query := entity.KnowledgeLexicalSearchRequest{Version: entity.KnowledgeLocalLexicalVersion, Index: *finished.Index, Selection: selection, Query: "星栈", Limit: 8}
	hits, err := s.SearchKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, query)
	if err != nil || len(hits.Hits) == 0 {
		t.Fatalf("real lexical search: %+v %v", hits, err)
	}
	lexicalSaveEvidence(t, "search", "search", hits)
	var publication entity.KnowledgeLexicalPublicationRecord
	if err := db.Where("id = ?", finished.Index.ID).Take(&publication).Error; err != nil {
		t.Fatal(err)
	}
	manifest, err := lexicalPublication(publication)
	if err != nil {
		t.Fatal(err)
	}
	crlf := false
	for i, row := range manifest.Rows {
		read, err := s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: lexicalEvidence(*finished.Index, row)})
		if err != nil || lexicalSHA([]byte(read.Text)) != row.TextSHA256 || int64(len(read.Text)) != row.ByteLength {
			t.Fatalf("whole row read: %+v %v", read, err)
		}
		if strings.Contains(read.Text, "\r\n") {
			crlf = true
		}
		lexicalSaveEvidence(t, "read", "read-"+row.ID, read)
		if i == 0 {
			tampered := read.Evidence
			tampered.FragmentSHA256 = strings.Repeat("0", 64)
			_, err := s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: tampered})
			lexicalAssertCode(t, err, "integrity_error")
			lexicalSaveError(t, "tampered-evidence", err)
		}
	}
	if !crlf {
		t.Fatal("CRLF source was not round-tripped")
	}
	query.Query = "qzxv_synthetic_no_match_987654321"
	zero, err := s.SearchKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, query)
	if err != nil || len(zero.Hits) != 0 {
		t.Fatalf("zero match: %+v %v", zero, err)
	}
	lexicalSaveEvidence(t, "search", "ready-zero-hit", zero)
	if err := native.UpdateChunks(ctx, map[string]any{"doc_id": manifest.Rows[0].Target.Source.SourceID}, map[string]any{"available_int": 1}, "ragflow_"+knowledgeTestOwner, knowledgeTestDomain); err != nil {
		t.Fatal(err)
	}
	rows, err := s.lexicalExpectedRows(ctx, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+knowledgeTestOwner, knowledgeTestDomain, rows); err != nil {
		t.Fatal(err)
	}
	// Remove one synthetic immutable node to exercise coverage-gap failure, then
	// restore its exact row before continuing the fixed-revision checks.
	var gapTarget entity.KnowledgeLexicalTarget
	for _, target := range selection.Targets {
		if target.Representation.Kind == "structure_view" {
			gapTarget = target
			break
		}
	}
	var missing entity.KnowledgeStructureNodeRecord
	if err := db.Where("view_id = ? AND source_id = ? AND kind = ?", gapTarget.Representation.ID, gapTarget.Source.SourceID, "passage").Take(&missing).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&missing).Error; err != nil {
		t.Fatal(err)
	}
	gapSelection := entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{gapTarget}}
	gapJob, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "coverage-gap", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: gapSelection})
	if err != nil {
		t.Fatal(err)
	}
	gapRecord := entity.KnowledgeLexicalJobRecord{}
	db.Where("id = ?", gapJob.ID).Take(&gapRecord)
	gapRecord.State = "building"
	gapRecord.Claim = common.GenerateUUID()
	gapRecord.LeaseUntil = time.Now().Add(time.Minute)
	if err := db.Save(&gapRecord).Error; err != nil {
		t.Fatal(err)
	}
	s.runKnowledgeLocalLexical(ctx, gapRecord)
	gap, err := s.InspectKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalInspectRequest{Version: entity.KnowledgeLocalLexicalVersion, Selection: gapSelection})
	if err != nil || gap.Readiness.Kind != "coverage_gap" {
		t.Fatalf("coverage gap: %+v %v", gap, err)
	}
	lexicalSaveEvidence(t, "inspectIndex", "coverage-gap", gap)
	if err := db.Create(&missing).Error; err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Path    string `json:"path"`
		Content string `json:"content_utf8"`
	}
	if err := json.Unmarshal([]byte(lexicalFixturesJSON), &fixtures); err != nil {
		t.Fatal(err)
	}
	var revised string
	for _, f := range fixtures {
		if f.Path == "field-guide-v2.md" {
			revised = f.Content
		}
	}
	var oldRow lexicalManifestRow
	for _, r := range manifest.Rows {
		if r.Target.Source.SourceID == gapTarget.Source.SourceID {
			oldRow = r
			break
		}
	}
	var doc entity.Document
	if err := db.Where("id = ?", oldRow.Target.Source.SourceID).Take(&doc).Error; err != nil || doc.Location == nil {
		t.Fatalf("fixed source: %v", err)
	}
	if err := store.Put(ctx, knowledgeTestDomain, *doc.Location, []byte(revised)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: doc.ID, ExpectedRevisionID: oldRow.Target.Source.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	newSelection := entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{{Source: entity.KnowledgeLexicalSourceRef{DomainID: knowledgeTestDomain, SourceID: doc.ID, RevisionID: snapshot.Source.ID, SHA256: snapshot.Source.SHA256}, Representation: entity.KnowledgeLexicalRepresentationRef{Kind: "source_revision", ID: snapshot.Source.ID}}}}
	newJob, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "new-revision", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: newSelection})
	if err != nil {
		t.Fatal(err)
	}
	newRecord := entity.KnowledgeLexicalJobRecord{}
	if err := db.Where("id = ?", newJob.ID).Take(&newRecord).Error; err != nil {
		t.Fatal(err)
	}
	newRecord.State = "building"
	newRecord.Claim = common.GenerateUUID()
	newRecord.LeaseUntil = time.Now().Add(time.Minute)
	if err := db.Save(&newRecord).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.buildKnowledgeLocalLexical(ctx, newRecord); err != nil {
		t.Fatal(err)
	}
	newFinished, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, newJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	var newPublication entity.KnowledgeLexicalPublicationRecord
	if err := db.Where("id = ?", newFinished.Index.ID).Take(&newPublication).Error; err != nil {
		t.Fatal(err)
	}
	newManifest, err := lexicalPublication(newPublication)
	if err != nil || len(newManifest.Rows) != 1 {
		t.Fatalf("new revision manifest: %v", err)
	}
	oldRead, err := s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: lexicalEvidence(*finished.Index, oldRow)})
	if err != nil || oldRead.Currentness != "historical" || !strings.Contains(oldRead.Text, "bufferLimit = 12;") {
		t.Fatalf("old fixed revision: %+v %v", oldRead, err)
	}
	newRead, err := s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: lexicalEvidence(*newFinished.Index, newManifest.Rows[0])})
	if err != nil || !strings.Contains(newRead.Text, "bufferLimit = 18;") || newRead.Evidence.RowID == oldRead.Evidence.RowID {
		t.Fatalf("new fixed revision: %+v %v", newRead, err)
	}
	lexicalSaveEvidence(t, "read", "old-fixed-revision", oldRead)
	lexicalSaveEvidence(t, "read", "new-fixed-revision", newRead)
	mixedJob, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID)
	if err != nil || mixedJob.Readiness.Currentness != "mixed" {
		t.Fatalf("published mixed snapshot: %+v %v", mixedJob, err)
	}
	lexicalSaveEvidence(t, "getIndexJob", "job-mixed", mixedJob)
	// Change a trusted, actual SQL language value to exercise a different plan
	// for the identical source/revision/representation set. Native assets stay
	// read-only and the producer remains the measured executable/resources hash.
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", knowledgeTestDomain).Take(&kb).Error; err != nil {
		t.Fatal(err)
	}
	oldLanguage := lexicalLanguage(&kb)
	newLanguage := "English"
	if oldLanguage == newLanguage {
		newLanguage = ""
	}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestDomain).Update("language", newLanguage).Error; err != nil {
		t.Fatal(err)
	}
	planJob, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "new-plan", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	planRecord := entity.KnowledgeLexicalJobRecord{}
	if err := db.Where("id = ?", planJob.ID).Take(&planRecord).Error; err != nil {
		t.Fatal(err)
	}
	planRecord.State = "building"
	planRecord.Claim = common.GenerateUUID()
	planRecord.LeaseUntil = time.Now().Add(time.Minute)
	if err := db.Save(&planRecord).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.buildKnowledgeLocalLexical(ctx, planRecord); err != nil {
		t.Fatal(err)
	}
	planFinished, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, planJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	var planPublication entity.KnowledgeLexicalPublicationRecord
	if err := db.Where("id = ?", planFinished.Index.ID).Take(&planPublication).Error; err != nil {
		t.Fatal(err)
	}
	planManifest, err := lexicalPublication(planPublication)
	if err != nil {
		t.Fatal(err)
	}
	if planManifest.SelectionSHA256 != manifest.SelectionSHA256 || planManifest.Producer != manifest.Producer || planManifest.Profile == manifest.Profile || planManifest.Language != newLanguage {
		t.Fatal("plan identity not isolated")
	}
	oldByMember := map[string]lexicalManifestRow{}
	for _, r := range manifest.Rows {
		oldByMember[r.Member.Node.ID] = r
	}
	for _, r := range planManifest.Rows {
		old := oldByMember[r.Member.Node.ID]
		if old.ID == r.ID || old.Binding == r.Binding || old.Plan == r.Plan || old.Reference != r.Reference || old.TextSHA256 != r.TextSHA256 {
			t.Fatal("same fixed member plan isolation failed")
		}
	}
	for _, m := range []*lexicalManifest{manifest, planManifest} {
		expected, err := s.lexicalExpectedRows(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+knowledgeTestOwner, knowledgeTestDomain, expected); err != nil {
			t.Fatal(err)
		}
	}
	lexicalSaveEvidence(t, "verification", "old-plan-manifest", manifest)
	lexicalSaveEvidence(t, "verification", "new-plan-manifest", planManifest)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestDomain).Update("language", oldLanguage).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "disable", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.InspectKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, inspect)
	if err != nil || stale.Readiness.Kind != "stale" {
		t.Fatalf("stale: %+v %v", stale, err)
	}
	lexicalSaveEvidence(t, "inspectIndex", "stale", stale)
	_, err = s.SearchKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, query)
	lexicalAssertCode(t, err, "policy_disabled")
	lexicalSaveError(t, "policy-disabled", err)
	if err := db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: lexicalEvidence(*finished.Index, manifest.Rows[0])})
	lexicalAssertCode(t, err, "not_found")
	lexicalSaveError(t, "membership-revoked", err)
	if blocked != 0 || modelAttempts != 0 {
		t.Fatalf("prohibited attempts: http=%d models=%d", blocked, modelAttempts)
	}
	lexicalSaveEvidence(t, "verification", "runtime-facts", map[string]any{"database": dbName, "sql": "MySQL8", "engine": "Elasticsearch8", "r1_parser": "real deterministic Build+Bind; publication fixture transaction", "object_storage": "in-memory storage; native FileCommit immutable snapshots", "published_rows": len(manifest.Rows), "coverage_bytes": manifest.Coverage.CoveredBytes, "CRLF": crlf, "model_attempts": modelAttempts, "blocked_external_http_attempts": blocked, "partial_real_ES_fault": "first row exists; SQL publication count 0; new-key equivalent retry published full set", "coverage_gap_fault": "synthetic node removed and exact SQL row restored", "fixed_revisions": "old R1 retains 12; new bounded C1 root has 18; distinct row IDs", "durability": "tmpfs lifetime only", "native_session_signature": "not exercised; trusted synthetic service context"})
	t.Logf("real_mysql_database=%s real_es=true rows=%d coverage_bytes=%d CRLF=true model_attempts=0 external_http_attempts=0", dbName, len(manifest.Rows), manifest.Coverage.CoveredBytes)
}

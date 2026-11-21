package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	models "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/storage"

	"go.uber.org/zap"
)

type knowledgeSkillEmbedCapture struct {
	*models.DummyModel
	texts []string
}

func (d *knowledgeSkillEmbedCapture) Embed(_ context.Context, _ *string, req models.EmbedRequest, _ *models.APIConfig, _ *models.EmbeddingConfig, _ *common.ModelUsage) ([]models.EmbeddingData, error) {
	d.texts = append(d.texts, req.Texts...)
	result := make([]models.EmbeddingData, len(req.Texts))
	for i := range result {
		result[i] = models.EmbeddingData{Embedding: []float64{1, 0}, Index: i}
	}
	return result, nil
}

type knowledgeSkillIndexCapture struct {
	fakeChatDocEngine
	documents []any
}

func (e *knowledgeSkillIndexCapture) IndexDocument(_ context.Context, _ string, _ string, body any) error {
	e.documents = append(e.documents, body)
	return nil
}

func seedKnowledgeRestrictedSkillSpace(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.SkillSpace{}, &entity.SkillSearchConfig{}, &entity.TenantModel{}, &entity.TenantModelInstance{}, &entity.TenantModelProvider{}); err != nil {
		t.Fatal(err)
	}
	valid, anchor, did, loc := "1", "private-anchor", "source-doc", "payload"
	for _, row := range []any{
		&entity.UserTenant{ID: "copy-owner", TenantID: "copy-owner", UserID: "copy-owner", Role: "owner", Status: &valid},
		&entity.UserTenant{ID: "source-owner", TenantID: "source-owner", UserID: "source-owner", Role: "owner", Status: &valid},
		&entity.UserTenant{ID: "source-in-copy", TenantID: "copy-owner", UserID: "source-owner", Role: "normal", Status: &valid},
		&entity.Tenant{ID: "copy-owner"},
		&entity.Knowledgebase{ID: "private-domain", TenantID: "source-owner", CreatedBy: "source-owner", Name: "Private", Permission: "me", Status: &valid, KnowledgeEnabled: true},
		&entity.File{ID: "root", ParentID: "root", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "/", Type: "folder"},
		&entity.File{ID: "skills-root", ParentID: "root", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "skills", Type: "folder", SourceType: "system"},
		&entity.File{ID: "private-space", ParentID: "skills-root", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "Collision", Type: "folder", SourceType: "skill_space"},
		&entity.File{ID: anchor, ParentID: "private-space", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "anchor.txt", Type: "txt"},
		&entity.File{ID: "private-skill", ParentID: "private-space", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "PrivateSkill", Type: "folder"},
		&entity.File{ID: "private-version", ParentID: "private-skill", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "1.0.0", Type: "folder"},
		&entity.File{ID: "private-payload", ParentID: "private-version", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "SKILL.md", Type: "txt", Location: &loc},
		&entity.Document{ID: did, KbID: "private-domain", CreatedBy: "source-owner", Name: sptr("anchor.txt"), Status: &valid, ParserConfig: entity.JSONMap{}},
		&entity.File2Document{ID: "supported-legacy-link", FileID: &anchor, DocumentID: &did},
		&entity.SkillSpace{ID: "old-space", TenantID: "copy-owner", Name: "Collision", FolderID: "private-space", Status: "1"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	store := storage.NewMemoryStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })
	if err := store.Put(t.Context(), "private-version", loc, []byte("---\nname: Synthetic private source sentinel\ndescription: Synthetic private source sentinel\n---\nSynthetic private source body\n")); err != nil {
		t.Fatal(err)
	}
	if err := permission.NewDatabaseChecker(db).CheckResource(permission.WithIdentity(t.Context(), "copy-owner", ""), permission.ResourceRef{Kind: permission.ResourceKindFolder, ID: "private-space"}, permission.OperationRead); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("fixture exact folder guard=%v", err)
	}
}

func TestKnowledgeSkillSpaceCacheRevalidatesOrigin(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	for _, row := range []entity.File{{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: "folder"}, {ID: "old-root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "skills", Type: "folder"}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSkillSpaceService(nil)
	first, err := svc.getSkillsFolderID(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: "domain", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true, KnowledgeFolderID: &first}).Error; err != nil {
		t.Fatal(err)
	}
	second, err := svc.getSkillsFolderID(t.Context(), "owner")
	t.Logf("cached root after registered-origin binding=%s err=%v", second, err)
	if err == nil && second == first {
		t.Error("cached skills identity bypassed current source policy")
	}
}

func TestKnowledgeSkillSpaceUpdateHonorsKnownSourceOwner(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	seedKnowledgeRestrictedSkillSpace(t)
	_, code, err := NewSkillSpaceService(nil).UpdateSpace(permission.WithIdentity(t.Context(), "copy-owner", "copy-owner"), "old-space", &UpdateSpaceRequest{Name: sptr("Changed private folder")})
	var f entity.File
	if e := dao.DB.Where("id = ?", "private-space").Take(&f).Error; e != nil {
		t.Fatal(e)
	}
	t.Logf("native folder read denied; SkillSpace.Update code=%v err=%v persisted_name=%s", code, err, f.Name)
	if err == nil || f.Name != "Collision" {
		t.Error("skill space mutation bypassed known source-owner intersection")
	}
}

func TestKnowledgeSkillReindexStopsBeforeReadingOrModelUse(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	seedKnowledgeRestrictedSkillSpace(t)
	provider := dao.GetModelProviderManager().FindProvider("OpenAI")
	if provider == nil {
		t.Fatal("OpenAI catalog missing")
	}
	original := provider.ModelDriver
	capture := &knowledgeSkillEmbedCapture{DummyModel: models.NewDummyModel(nil, models.URLSuffix{})}
	provider.ModelDriver = capture
	t.Cleanup(func() { provider.ModelDriver = original })
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "probe-provider", TenantID: "copy-owner", UserID: "copy-owner", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "probe-instance", ProviderID: "probe-provider", InstanceName: "default", APIKey: "", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "probe-model", ProviderID: "probe-provider", InstanceID: "probe-instance", ModelName: "synthetic-embedding", ModelType: int(entity.ModelTypeEmbedding), Status: "active", Extra: `{"max_tokens":8192}`},
	} {
		if err := dao.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	engine := &knowledgeSkillIndexCapture{}
	got, err := NewSkillIndexerService().ReindexAll(permission.WithIdentity(t.Context(), "copy-owner", "copy-owner"), "old-space", engine, "probe-model")
	privatePayload := false
	for _, value := range capture.texts {
		if strings.Contains(value, "Synthetic private source sentinel") {
			privatePayload = true
		}
	}
	indexedBody := false
	for _, v := range engine.documents {
		if m, ok := v.(map[string]any); ok {
			body, _ := m["content"].(string)
			indexedBody = indexedBody || strings.Contains(body, "Synthetic private source body")
		}
	}
	t.Logf("ReindexAll result=%v err=%v in_process_stub_calls=%d private_payload_seen=%t indexed_documents=%d indexed_body_seen=%t", got, err, len(capture.texts), privatePayload, len(engine.documents), indexedBody)
	if privatePayload || len(capture.texts) != 0 || len(engine.documents) != 0 {
		t.Error("known private source entered skill model/index pipeline")
	}
}

func TestKnowledgeOrdinarySkillReindexControl(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	seedKnowledgeRestrictedSkillSpace(t)
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "private-domain").Update("knowledge_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	provider := dao.GetModelProviderManager().FindProvider("OpenAI")
	if provider == nil {
		t.Fatal("OpenAI catalog missing")
	}
	original := provider.ModelDriver
	capture := &knowledgeSkillEmbedCapture{DummyModel: models.NewDummyModel(nil, models.URLSuffix{})}
	provider.ModelDriver = capture
	t.Cleanup(func() { provider.ModelDriver = original })
	for _, row := range []any{&entity.TenantModelProvider{ID: "probe-provider", TenantID: "copy-owner", UserID: "copy-owner", ProviderName: "OpenAI"}, &entity.TenantModelInstance{ID: "probe-instance", ProviderID: "probe-provider", InstanceName: "default", APIKey: "", Status: "active", Extra: "{}"}, &entity.TenantModel{ID: "probe-model", ProviderID: "probe-provider", InstanceID: "probe-instance", ModelName: "synthetic-embedding", ModelType: int(entity.ModelTypeEmbedding), Status: "active", Extra: `{"max_tokens":8192}`}} {
		if err := dao.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	engine := &knowledgeSkillIndexCapture{}
	got, err := NewSkillIndexerService().ReindexAll(permission.WithIdentity(t.Context(), "copy-owner", "copy-owner"), "old-space", engine, "probe-model")
	if err != nil || len(capture.texts) == 0 || len(engine.documents) != 1 {
		t.Fatalf("ordinary control result=%v err=%v stub_calls=%d indexed=%d", got, err, len(capture.texts), len(engine.documents))
	}
	t.Logf("ordinary reindex succeeds with in-process stub only: stub_calls=%d indexed=%d", len(capture.texts), len(engine.documents))
}

func TestKnowledgeSkillBackgroundDeleteRetainsSourceFacts(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	seedKnowledgeRestrictedSkillSpace(t)
	if err := dao.DB.Model(&entity.SkillSpace{}).Where("id = ?", "old-space").Update("status", "2").Error; err != nil {
		t.Fatal(err)
	}
	NewSkillSpaceService(nil).asyncDeleteSpace(t.Context(), "old-space", "private-space", "copy-owner", nil)
	var n int64
	if err := dao.DB.Model(&entity.File{}).Where("id IN ?", []string{"private-space", "private-anchor", "private-skill", "private-version", "private-payload"}).Count(&n).Error; err != nil || n != 5 {
		t.Fatalf("background delete lost source tree count=%d err=%v", n, err)
	}
	if err := permission.CheckKnowledgeFileTransfer(t.Context(), dao.DB, []string{"private-payload"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("background delete lost origin policy=%v", err)
	}
	var space entity.SkillSpace
	if err := dao.DB.Where("id = ?", "old-space").Take(&space).Error; err != nil {
		t.Fatal(err)
	}
	if space.Status != "2" {
		t.Fatalf("denied background cleanup changed status=%s", space.Status)
	}
}

func TestKnowledgeSkillSpaceScopeAndConsumersRejectManagedSources(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	seedKnowledgeRestrictedSkillSpace(t)
	ctx := permission.WithIdentity(t.Context(), "copy-owner", "copy-owner")
	spaces := NewSkillSpaceService(nil)
	listed, code, err := spaces.ListSpaces(ctx)
	if err != nil || code != common.CodeSuccess || len(listed["spaces"].([]map[string]interface{})) != 0 {
		t.Fatalf("restricted skills scope leaked: code=%d err=%v", code, err)
	}
	if _, _, err := spaces.GetSpace(ctx, "old-space"); err == nil {
		t.Fatal("restricted skill space was readable")
	}
	if _, _, err := spaces.GetSpaceByFolderID(ctx, "private-space"); err == nil {
		t.Fatal("folder lookup bypassed skill scope")
	}
	if _, err := spaces.DeleteSpace(ctx, "old-space", nil); err == nil {
		t.Fatal("restricted space deletion reached background cleanup")
	}
	if _, _, err := NewSkillSearchService().Search(ctx, &SearchRequest{SpaceID: "old-space", Query: "synthetic"}, nil); err == nil {
		t.Fatal("restricted skill search reached its engine")
	}
	indexer := NewSkillIndexerService()
	skill := SkillInfo{Name: "Synthetic", FolderID: "private-skill"}
	for name, run := range map[string]func() error{
		"default index": func() error { return indexer.IndexSkill(ctx, "default", skill, nil, "") },
		"default batch": func() error {
			return indexer.BatchIndexSkills(ctx, "default", []SkillInfo{skill}, nil, "")
		},
		"default update": func() error { return indexer.UpdateSkillVersion(ctx, "default", skill, nil, "") },
		"internal index": func() error { return indexer.indexSkill(ctx, "copy-owner", "default", skill, nil, "") },
		"internal batch": func() error {
			return indexer.batchIndexSkills(ctx, "copy-owner", "default", []SkillInfo{skill}, nil, "")
		},
	} {
		if err := run(); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
			t.Errorf("%s bypassed known source: %v", name, err)
		}
	}
	var configRows int64
	if err := dao.DB.Model(&entity.SkillSearchConfig{}).Count(&configRows).Error; err != nil || configRows != 0 {
		t.Fatalf("denied consumption changed configuration: count=%d err=%v", configRows, err)
	}
}

type knowledgeSkillSearchCapture struct{ fakeChatDocEngine }

func (knowledgeSkillSearchCapture) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}

func (knowledgeSkillSearchCapture) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return &types.SearchResult{Chunks: []map[string]interface{}{
		{"skill_id": "protected-index-entry", "folder_id": "private-skill", "name": "Private", "content": "synthetic private", "score": 1.0},
		{"skill_id": "ordinary-index-entry", "folder_id": "ordinary-skill", "name": "Ordinary", "content": "synthetic ordinary", "score": 1.0},
	}}, nil
}

func TestKnowledgeSkillSearchFiltersKnownOldIndexOriginsBeforeCounts(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	seedKnowledgeRestrictedSkillSpace(t)
	result, code, err := NewSkillSearchService().Search(permission.WithIdentity(t.Context(), "copy-owner", "copy-owner"), &SearchRequest{SpaceID: "default", Page: 1, PageSize: 1}, knowledgeSkillSearchCapture{})
	if err != nil || code != common.CodeSuccess || result.Total != 1 || len(result.Skills) != 1 || result.Skills[0].SkillID != "ordinary-index-entry" {
		t.Fatalf("old index origin filtering: result=%+v code=%d err=%v", result, code, err)
	}
}

func TestKnowledgeSkillSpaceCreationPreservesManagedTrees(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	factory := storage.GetStorageFactory()
	previousStorage := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previousStorage) })
	for _, mode := range []string{"managed_child", "managed_root_alias", "ordinary_control"} {
		t.Run(mode, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			if err := db.AutoMigrate(&entity.SkillSpace{}, &entity.SkillSearchConfig{}); err != nil {
				t.Fatal(err)
			}
			valid, raw, copyID := "1", "raw-private", "copy-doc"
			primary := "collision-folder"
			rootSource := "system"
			if mode == "managed_root_alias" {
				primary = "skills-root"
				rootSource = "knowledgebase"
			}
			rows := []any{
				&entity.UserTenant{ID: "owner-membership", UserID: "owner", TenantID: "owner", Role: "owner", Status: &valid},
				&entity.UserTenant{ID: "reader-membership", UserID: "reader", TenantID: "owner", Role: "normal", Status: &valid},
				&entity.Tenant{ID: "owner"},
				&entity.Knowledgebase{ID: "target", TenantID: "owner", CreatedBy: "owner", Name: "Shared", Permission: "team", Status: &valid},
				&entity.File{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: "folder"},
				&entity.File{ID: "skills-root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "skills", Type: "folder", SourceType: rootSource},
				&entity.File{ID: "collision-folder", ParentID: "skills-root", TenantID: "owner", CreatedBy: "owner", Name: "Collision", Type: "folder", SourceType: "skill_space"},
				&entity.File{ID: raw, ParentID: "collision-folder", TenantID: "owner", CreatedBy: "owner", Name: "private.txt", Type: "txt", SourceType: "local"},
				&entity.Document{ID: copyID, KbID: "target", CreatedBy: "owner", Name: sptr("private-copy.txt"), Status: &valid, ParserConfig: entity.JSONMap{}},
				&entity.File2Document{ID: "old-raw-copy", FileID: &raw, DocumentID: &copyID},
			}
			if mode != "ordinary_control" {
				rows = append(rows, &entity.Knowledgebase{ID: "registered", TenantID: "owner", CreatedBy: "owner", Name: "Private", Permission: "me", Status: &valid, KnowledgeEnabled: true, KnowledgeFolderID: &primary})
			}
			for _, row := range rows {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := permission.CheckDatasetModelUse(t.Context(), db, []string{"target"}, "generation"); (mode != "ordinary_control") != errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
				t.Fatalf("precondition guard=%v", err)
			}
			got, code, err := NewSkillSpaceService(nil).CreateSpace(permission.WithIdentity(t.Context(), "owner", "owner"), &CreateSpaceRequest{Name: "Collision"})
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("CreateSpace result=%v code=%v err=%v", got, code, err)
			}
			var rawRows, oldFolderRows int64
			db.Model(&entity.File{}).Where("id = ?", raw).Count(&rawRows)
			db.Model(&entity.File{}).Where("id = ?", "collision-folder").Count(&oldFolderRows)
			after := permission.CheckDatasetModelUse(t.Context(), db, []string{"target"}, "generation")
			t.Logf("CreateSpace code=%v rawRows=%d oldFolderRows=%d targetModelGuard=%v", code, rawRows, oldFolderRows, after)
			if mode != "ordinary_control" {
				if rawRows != 1 || oldFolderRows != 1 {
					t.Errorf("space creation removed known managed tree")
				}
				if !errors.Is(after, permission.ErrKnowledgeModelUseDenied) {
					t.Errorf("space creation released known source model restriction: %v", after)
				}
			} else if rawRows != 0 || oldFolderRows != 0 {
				t.Errorf("ordinary name cleanup changed")
			}
		})
	}
}

package tool

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/service/nlp"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type knowledgeEnhancerProbe struct {
	retrievalEnhancer
	calls int
}

func (p *knowledgeEnhancerProbe) FilterDocuments(context.Context, map[string]any, string, *modelModule.ChatModel, []string, []string) ([]string, error) {
	p.calls++
	return nil, errors.New("synthetic model enhancement reached")
}

func TestKnowledgeAgentRetrievalStopsBeforeModelEnhancement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatal(err)
	}
	valid := string(entity.StatusValid)
	kb := entity.Knowledgebase{ID: "synthetic-private", TenantID: "owner", CreatedBy: "owner", Name: "Synthetic",
		Permission: "me", Status: &valid, KnowledgeEnabled: true}
	if err := db.Create(&kb).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.UserTenant{ID: "synthetic-member", UserID: "owner", TenantID: "owner", Role: "owner", InvitedBy: "owner", Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	constructors := 0
	probe := &knowledgeEnhancerProbe{}
	adapter := &NLPRetrievalAdapter{svc: &nlp.RetrievalService{}, kbDAO: fakeKnowledgebaseLookup{kbs: []*entity.Knowledgebase{&kb}}, enhancer: probe,
		modelConstructors: RetrievalModelConstructors{
			Embedding: func(context.Context, string, string) (*modelModule.EmbeddingModel, error) {
				constructors++
				return modelModule.NewEmbeddingModel(&modelModule.DummyModel{}, nil, &modelModule.APIConfig{}, 512), nil
			},
			Chat: func(context.Context, string, string) (*modelModule.ChatModel, error) {
				constructors++
				return &modelModule.ChatModel{}, nil
			},
		}}
	state := runtime.NewCanvasState("synthetic-run", "synthetic-session")
	state.Sys["user_id"] = "owner"
	ctx := runtime.WithState(t.Context(), state)
	_, err = adapter.Search(ctx, db, RetrievalRequest{Query: "synthetic", DatasetIDs: []string{kb.ID}, MetaDataFilter: map[string]any{"method": "auto"}, CrossLanguages: []string{"English"}})
	if !errors.Is(err, permission.ErrKnowledgeModelUseDenied) || constructors != 0 || probe.calls != 0 {
		t.Fatalf("policy=%v constructors=%d enhancement_calls=%d", err, constructors, probe.calls)
	}
}

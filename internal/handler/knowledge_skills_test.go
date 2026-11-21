package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestKnowledgeSkillIndexHandlerChecksSourcesBeforeIndexInitialization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}, &entity.File{}, &entity.Document{}, &entity.File2Document{}, &entity.Knowledgebase{}, &entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	previous := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previous })
	valid, folder := "1", "protected-folder"
	for _, row := range []any{
		&entity.UserTenant{ID: "owner-membership", UserID: "owner", TenantID: "owner", Role: "owner", Status: &valid},
		&entity.Knowledgebase{ID: "domain", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true, KnowledgeFolderID: &folder},
		&entity.File{ID: folder, ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "Private", Type: "folder"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		setAuthIdentity(c, &service.AuthIdentity{User: &entity.User{ID: "owner"}, TenantID: "owner"})
		c.Next()
	})
	r.POST("/index", NewSkillSearchHandler(nil, nil).IndexSkills)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/index", strings.NewReader(`{"space_id":"default","embd_id":"not-configured-synthetic","skills":[{"id":"synthetic","name":"Synthetic","folder_id":"protected-folder","content":"synthetic source"}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var response struct{ Code int }
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Code != 404 {
		t.Fatalf("native index did not reject before dependencies: code=%d err=%v", response.Code, err)
	}
}

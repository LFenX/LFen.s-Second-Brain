package file

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

type privateCommitTrace struct{ bytes.Buffer }

func (w *privateCommitTrace) Printf(format string, args ...any) {
	fmt.Fprintf(&w.Buffer, format, args...)
}

func TestKnowledgeAccessCommitReaderUsesScopedSilentDB(t *testing.T) {
	trace := &privateCommitTrace{}
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.New(trace, logger.Config{LogLevel: logger.Info})})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err = db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}, &entity.User{}); err != nil {
		t.Fatal(err)
	}
	location, hash := ".objects/synthetic-hash", "synthetic-hash"
	if err = db.Create(&entity.FileCommit{ID: "synthetic-commit", FolderID: "synthetic-folder", AuthorID: "synthetic-user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&entity.FileCommitItem{ID: "synthetic-item", CommitID: "synthetic-commit", FileID: "synthetic-file", NewHash: &hash, NewLocation: &location}).Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	if err = store.Put(t.Context(), "synthetic-folder", location, []byte("synthetic exact bytes")); err != nil {
		t.Fatal(err)
	}
	oldStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(store)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(oldStorage) })
	trace.Reset()
	ctx := dao.KnowledgeAccessPrivateReadContext(storage.WithPrivateRead(t.Context()))
	body, err := ReadCommitFileContent(ctx, dao.KnowledgeAccessReadDB(ctx, db), "synthetic-folder", "synthetic-commit", "synthetic-file")
	if err != nil || string(body) != "synthetic exact bytes" || trace.Len() != 0 {
		t.Fatal("private SQL reader leaked or changed bytes")
	}
	_, err = ReadCommitFileContent(ctx, dao.KnowledgeAccessReadDB(ctx, db), "synthetic-folder", "missing-synthetic-commit", "synthetic-file")
	if err == nil || trace.Len() != 0 {
		t.Fatal("failed private SQL lookup leaked")
	}
	oldDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = oldDB })
	body, err = NewFileCommitService().GetCommitFileContent(t.Context(), "synthetic-folder", "synthetic-commit", "synthetic-file")
	if err != nil || string(body) != "synthetic exact bytes" || !strings.Contains(trace.String(), "synthetic-commit") {
		t.Fatal("ordinary DB path no longer uses the same core/diagnostics")
	}
	// Native session lookup is also silent only under the internal marker.
	valid, token := "1", "synthetic-session-value"
	if err = db.Create(&entity.User{ID: "synthetic-user", Nickname: "Synthetic", Email: "synthetic@invalid", Status: &valid, AccessToken: &token}).Error; err != nil {
		t.Fatal(err)
	}
	trace.Reset()
	if _, err = dao.NewUserDAO().GetByAccessToken(ctx, db, token); err != nil || trace.Len() != 0 {
		t.Fatal("private native session lookup leaked")
	}
	if _, err = dao.NewUserDAO().GetByAccessToken(t.Context(), db, token); err != nil || trace.Len() == 0 {
		t.Fatal("ordinary native lookup behavior changed")
	}
}

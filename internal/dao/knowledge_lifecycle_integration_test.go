//go:build integration

package dao

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestKnowledgeLifecycleMySQLMigrationAndExactPositions(t *testing.T) {
	input := os.Getenv("LFEN_TEST_MYSQL_CONFIG")
	if input == "" {
		t.Skip("explicit synthetic MySQL configuration required")
	}
	var config struct {
		Host     string `json:"host"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if json.Unmarshal([]byte(input), &config) != nil || config.Host == "" || config.User == "" || config.Password == "" {
		t.Fatal("invalid private SQL configuration")
	}
	dsn := mysqldriver.NewConfig()
	dsn.User, dsn.Passwd, dsn.Net, dsn.Addr, dsn.ParseTime, dsn.Loc = config.User, config.Password, "tcp", config.Host, true, time.UTC
	admin, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		t.Fatal("SQL configuration failed")
	}
	defer admin.Close()
	name := "lfen_r6_lifecycle_" + common.GenerateUUID()[:16]
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatal("synthetic database creation failed")
	}
	t.Logf("retained_synthetic_database=%s", name)
	dsn.DBName = name
	db, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("synthetic SQL open failed")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.AutoMigrate(&entity.Document{}, &entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	domain, source := strings.Repeat("a", 32), strings.Repeat("b", 32)
	valid := "1"
	if err := db.Create(&entity.Document{ID: source, KbID: domain, Status: &valid, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatal(err)
	}
	head := entity.KnowledgeSourceRevision{ID: strings.Repeat("c", 32), DomainID: domain, DocumentID: source, InputHash: strings.Repeat("d", 64), IsCurrent: true}
	if err := db.Create(&head).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateKnowledgeLifecycle(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var state entity.KnowledgeSourceLifecycle
	if err := db.Where("source_id = ?", source).Take(&state).Error; err != nil || state.Version != 1 {
		t.Fatal("initial generation", err)
	}
	if err := db.Model(&state).Updates(map[string]any{"version": entity.KnowledgeMaxLifecycleVersion, "state": "archived"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLifecycle(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("source_id = ?", source).Take(&state).Error; err != nil || state.Version != entity.KnowledgeMaxLifecycleVersion || state.State != "archived" {
		t.Fatal("BIGINT state was reset", err)
	}
	paths := []string{"docs/A.md", "docs/a.md", "docs/Café.md", "docs/Cafe\u0301.md", "docs/尾空格.md", "docs/尾空格.md "}
	for _, path := range paths {
		digest, err := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-lifecycle-position/0.7.0", "registration_id": strings.Repeat("e", 32), "relative_path": path})
		if err != nil {
			t.Fatal(err)
		}
		row := entity.KnowledgeRetiredPosition{ID: digest, DomainID: domain, RegistrationID: strings.Repeat("e", 32), RelativePath: path, SourceID: source}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal("raw position digest collapsed", err)
		}
		var got entity.KnowledgeRetiredPosition
		if err := db.Where("id = ?", digest).Take(&got).Error; err != nil || got.RelativePath != path {
			t.Fatal("raw position not preserved", err)
		}
	}
	for _, key := range []string{"Case", "case", "café", "cafe\u0301"} {
		digest := entity.KnowledgeID("knowledge-lifecycle-command-key/0.7.0", source, domain, "archive", key)
		if err := db.Create(&entity.KnowledgeLifecycleCommandRecord{ID: common.GenerateUUID(), KeySHA256: digest, OwnerID: source, DomainID: domain, SourceID: source, Action: "archive", IdempotencyKey: key, RequestSHA256: strings.Repeat("f", 64), Payload: "{}"}).Error; err != nil {
			t.Fatal("private key scope collapsed", err)
		}
	}
	if err := db.Model(&head).Update("is_current", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLifecycle(t.Context(), db); err == nil {
		t.Fatal("zero head accepted")
	}
	if err := db.Model(&head).Update("is_current", true).Error; err != nil {
		t.Fatal(err)
	}
	second := head
	second.ID = fmt.Sprintf("%032x", 123)
	second.InputHash = strings.Repeat("0", 64)
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLifecycle(t.Context(), db); err == nil {
		t.Fatal("multiple heads accepted")
	}
}

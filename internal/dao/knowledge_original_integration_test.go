//go:build integration

package dao

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestKnowledgeOriginalExactKeysMySQL(t *testing.T) {
	input := os.Getenv("LFEN_TEST_MYSQL_CONFIG")
	if input == "" {
		t.Skip("integration requires explicit LFEN_TEST_MYSQL_CONFIG")
	}
	var config struct {
		Host     string `json:"host"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if json.Unmarshal([]byte(input), &config) != nil || config.Host == "" || config.User == "" || config.Password == "" {
		t.Fatal("invalid explicit SQL integration configuration")
	}
	dsn := drivermysql.NewConfig()
	dsn.User, dsn.Passwd, dsn.Net, dsn.Addr, dsn.ParseTime, dsn.Loc = config.User, config.Password, "tcp", config.Host, true, time.UTC
	admin, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		t.Fatal("SQL integration configuration failed")
	}
	defer admin.Close()
	name := "lfen_r4_exact_" + common.GenerateUUID()[:16]
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatal(err)
	}
	// Retained synthetic database only; never drop an application database.
	t.Logf("retained_test_database=%s", name)
	dsn.DBName = name
	db, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("could not open synthetic SQL database")
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, model := range []any{&entity.Knowledgebase{}, &entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}} {
		if err := db.AutoMigrate(model); err != nil {
			t.Fatal(err)
		}
	}
	domain := strings.Repeat("a", 32)
	reg := entity.KnowledgeSourceRegistration{ID: strings.Repeat("b", 32), DomainID: domain, RegistrationKey: "Exact:Key", Name: "Synthetic exact", SourceKind: "original_document", InputMode: "original_upload", IncludePaths: []string{"docs/Café.pdf"}, ComponentSide: "non_code", ConfigHash: strings.Repeat("c", 64)}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatal(err)
	}
	binding := entity.KnowledgeSourceBinding{ID: strings.Repeat("d", 32), RegistrationID: reg.ID, RelativePath: "docs/Café.pdf", DocumentID: strings.Repeat("e", 32), FileID: strings.Repeat("f", 32)}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateKnowledgeFoundation(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var kept entity.KnowledgeSourceRegistration
	if err := db.Where("id = ?", reg.ID).Take(&kept).Error; err != nil || kept.RegistrationKey != reg.RegistrationKey || kept.ConfigHash != reg.ConfigHash {
		t.Fatal("migration changed registration identity/value", err)
	}
	other := reg
	other.ID = common.GenerateUUID()
	other.RegistrationKey = "exact:key"
	if err := db.Create(&other).Error; err != nil {
		t.Fatal("case-distinct config keys collapsed", err)
	}
	for _, relative := range []string{"docs/Cafe\u0301.pdf", "docs/café.pdf"} {
		row := binding
		row.ID, row.DocumentID, row.FileID = common.GenerateUUID(), common.GenerateUUID(), common.GenerateUUID()
		row.RelativePath = relative
		if err := db.Create(&row).Error; err != nil {
			t.Fatal("exact Unicode/case paths collapsed", err)
		}
	}
	for _, key := range []string{"Retry:Key", "retry:key"} {
		upload := entity.KnowledgeOriginalUploadRecord{ID: common.GenerateUUID(), DomainID: domain, OwnerID: domain, RegistrationID: reg.ID, IdempotencyKey: key, RequestSHA256: strings.Repeat("a", 64), Payload: "{}", State: "awaiting_bytes", DeclaredBytes: 1, ExpiresAt: time.Now().Add(30 * time.Minute)}
		if err := db.Create(&upload).Error; err != nil {
			t.Fatal("case-distinct upload keys collapsed", err)
		}
		receipt := entity.KnowledgeOriginalImportRecord{ID: common.GenerateUUID(), RegistrationID: reg.ID, IdempotencyKey: key, UploadID: upload.ID, ManifestSHA256: strings.Repeat("b", 64), Receipt: "{}"}
		if err := db.Create(&receipt).Error; err != nil {
			t.Fatal("case-distinct confirmation keys collapsed", err)
		}
	}
	for _, column := range []struct{ table, name string }{{"knowledge_source_registration", "registration_key"}, {"knowledge_source_binding", "relative_path"}, {"knowledge_original_upload", "idempotency_key"}, {"knowledge_original_import", "idempotency_key"}} {
		var collation string
		if err := db.Raw("SELECT COLLATION_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", column.table, column.name).Scan(&collation).Error; err != nil || collation != "utf8mb4_bin" {
			t.Fatalf("%s.%s collation=%s err=%v", column.table, column.name, collation, err)
		}
	}
	var slots int64
	if err := db.Model(&entity.KnowledgeOriginalReceiveSlot{}).Count(&slots).Error; err != nil || slots != 2 {
		t.Fatal("repeat migration changed global slots", err)
	}
}

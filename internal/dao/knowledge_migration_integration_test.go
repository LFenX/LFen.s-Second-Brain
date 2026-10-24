//go:build integration

package dao

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestKnowledgeMigrationMySQLRepeatable(t *testing.T) {
	filename := os.Getenv("LFEN_C2_TEST_CONFIG")
	if filename == "" {
		t.Skip("LFEN_C2_TEST_CONFIG is unset; requires task-owned MySQL schema")
	}
	if os.Getenv("LFEN_POC_SYNTHETIC_ONLY") != "1" {
		t.Fatal("explicit synthetic-only opt-in is required")
	}
	var cfg struct {
		MySQLDSN string `json:"mysql_dsn"`
	}
	raw, err := os.ReadFile(filename)
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.MySQLDSN == "" {
		t.Fatal("invalid private migration test configuration")
	}
	dsn, err := mysqldriver.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		t.Fatal("invalid private DSN")
	}
	dsn.DBName, dsn.ParseTime, dsn.Timeout = "", true, 5*time.Second
	open := func() *gorm.DB {
		db, err := gorm.Open(newMigrationAwareDialector(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal("cannot connect to isolated migration test database")
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pool.Close() })
		return db
	}
	admin := open()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	database := "lfen_c2_test_migrate_" + hex.EncodeToString(random)
	if err := admin.Exec("CREATE DATABASE `" + database + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error; err != nil {
		t.Fatal("cannot create task-owned migration database")
	}
	dsn.DBName = database
	db := open()
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.File{}, &entity.File2Document{}, &entity.Document{}); err != nil {
		t.Fatal(err)
	}
	valid := "1"
	row := entity.Knowledgebase{ID: strings.Repeat("d", 32), TenantID: strings.Repeat("e", 32), CreatedBy: strings.Repeat("e", 32), Name: "Synthetic pre-extension dataset", Permission: "me", EmbdID: "synthetic-no-provider", Status: &valid}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate the native schema before this extension, retaining all native rows.
	for _, column := range []string{"KnowledgeEnabled", "KnowledgeCategory", "KnowledgePolicyVersion", "KnowledgeFolderID"} {
		if err := db.Migrator().DropColumn(&entity.Knowledgebase{}, column); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := migrateKnowledgeFoundation(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var after entity.Knowledgebase
	if err := db.Where("id = ?", row.ID).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Name != row.Name || after.Permission != row.Permission || after.EmbdID != row.EmbdID || after.KnowledgeEnabled || after.KnowledgePolicyVersion != 0 {
		t.Fatal("repeat migration changed pre-extension native metadata")
	}
	for _, model := range []any{&entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeImportPreview{}, &entity.KnowledgeImport{}} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("missing extension table %T", model)
		}
	}
	for _, check := range []struct {
		model  any
		fields []string
	}{
		{&entity.KnowledgeSourceRegistration{}, []string{"domain_id", "registration_key"}},
		{&entity.KnowledgeSourceBinding{}, []string{"registration_id", "relative_path"}},
		{&entity.KnowledgeImport{}, []string{"registration_id", "idempotency_key"}},
	} {
		indexes, err := db.Migrator().GetIndexes(check.model)
		if err != nil {
			t.Fatal(err)
		}
		matches := 0
		for _, index := range indexes {
			unique, known := index.Unique()
			if !known || !unique {
				continue
			}
			columns := index.Columns()
			if len(columns) == len(check.fields) && strings.Join(columns, ",") == strings.Join(check.fields, ",") {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("unique index set %v has %d matches", check.fields, matches)
		}
	}
	t.Logf("synthetic_migration_database=%s extension_migrations=2 native_metadata_unchanged=true", database)
}

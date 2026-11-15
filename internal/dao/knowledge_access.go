package dao

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ragflow/internal/entity"
)

type knowledgeAccessPrivateReadKey struct{}

// KnowledgeAccessPrivateReadContext affects logging only. It confers no
// session, principal, tenant, scope or permission.
func KnowledgeAccessPrivateReadContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, knowledgeAccessPrivateReadKey{}, true)
}
func KnowledgeAccessReadDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	query := db.WithContext(ctx)
	if private, _ := ctx.Value(knowledgeAccessPrivateReadKey{}).(bool); private {
		query = query.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	}
	return query
}

func migrateKnowledgeAccess(ctx context.Context, db *gorm.DB) error {
	for _, model := range []any{&entity.KnowledgeAccessTenantStateRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessAuditRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge access: %w", err)
		}
	}
	for _, index := range []struct {
		table, name string
		columns     []string
	}{{"knowledge_access_module", "idx_ka_module_1", []string{"domain_id", "id"}}, {"knowledge_access_audit", "idx_ka_audit_command_key", []string{"actor_user_id", "command_operation", "command_key"}}} {
		if err := ensureKnowledgeAccessUnique(ctx, db, index.table, index.name, index.columns); err != nil {
			return err
		}
	}
	if db.Dialector.Name() == "mysql" {
		var collation string
		if err := db.WithContext(ctx).Raw("SELECT COLLATION_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", "knowledge_access_audit", "command_key").Scan(&collation).Error; err != nil {
			return err
		}
		if collation != "ascii_bin" {
			if err := db.WithContext(ctx).Exec("ALTER TABLE knowledge_access_audit MODIFY command_key VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL").Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// KnowledgeAccessTransaction uses fresh primary reads. Authorization callers
// acquire tenant state first, then current native/resource/source rows.
func KnowledgeAccessTransaction(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	var options *sql.TxOptions
	if db.Dialector.Name() != "sqlite" {
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return db.WithContext(ctx).Transaction(fn, options)
}

func LockKnowledgeAccessTenants(tx *gorm.DB, tenantIDs []string, shared bool) (map[string]int64, error) {
	ids := append([]string(nil), tenantIDs...)
	sort.Strings(ids)
	out := map[string]int64{}
	strength := "UPDATE"
	if shared {
		strength = "SHARE"
	}
	for _, id := range ids {
		if _, exists := out[id]; exists {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entity.KnowledgeAccessTenantStateRecord{TenantID: id}).Error; err != nil {
			return nil, err
		}
		var row entity.KnowledgeAccessTenantStateRecord
		if err := tx.Clauses(clause.Locking{Strength: strength}).Where("tenant_id = ?", id).Take(&row).Error; err != nil {
			return nil, err
		}
		if row.Epoch < 0 || row.Epoch > entity.KnowledgeAccessMaxGeneration {
			return nil, fmt.Errorf("invalid access epoch")
		}
		out[id] = row.Epoch
	}
	return out, nil
}

func BumpKnowledgeAccessEpoch(tx *gorm.DB, tenant string) error {
	result := tx.Model(&entity.KnowledgeAccessTenantStateRecord{}).Where("tenant_id = ? AND epoch < ?", tenant, entity.KnowledgeAccessMaxGeneration).UpdateColumn("epoch", gorm.Expr("epoch + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("access epoch exhausted or missing")
	}
	return nil
}

func ensureKnowledgeAccessUnique(ctx context.Context, db *gorm.DB, table, name string, columns []string) error {
	equivalent := false
	if db.Dialector.Name() == "mysql" {
		var err error
		equivalent, err = hasUniqueIndex(ctx, db, table, columns)
		if err != nil {
			return err
		}
	} else {
		indexes, err := db.Migrator().GetIndexes(table)
		if err != nil {
			return err
		}
		want := append([]string(nil), columns...)
		sort.Strings(want)
		for _, index := range indexes {
			unique, known := index.Unique()
			actual := append([]string(nil), index.Columns()...)
			sort.Strings(actual)
			if known && unique && len(want) == len(actual) && strings.Join(want, "\x00") == strings.Join(actual, "\x00") {
				equivalent = true
			}
		}
	}
	if equivalent {
		return nil
	}
	if db.Migrator().HasIndex(table, name) {
		return fmt.Errorf("knowledge access unique index conflict")
	}
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = "`" + column + "`"
	}
	return db.WithContext(ctx).Exec("CREATE UNIQUE INDEX `" + name + "` ON `" + table + "` (" + strings.Join(quoted, ",") + ")").Error
}

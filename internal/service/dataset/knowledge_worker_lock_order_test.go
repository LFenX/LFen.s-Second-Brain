package dataset

import (
	"context"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/dao"
)

func TestKnowledgeWorkerLocksAccessEpochBeforeDomain(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(context.Context, string, func(*gorm.DB) error) error
	}{{"structure", structureInternalTransaction}, {"batch", batchInternalTransaction}} {
		t.Run(test.name, func(t *testing.T) {
			setupKnowledgeTest(t)
			epochSeen, domainSeen := false, false
			const callback = "test:worker_epoch_order"
			if err := dao.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "knowledge_access_tenant_state" {
					epochSeen = true
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "knowledgebase" && len(tx.Statement.Selects) == 0 {
					domainSeen = true
					if !epochSeen {
						t.Error("worker attempts domain row before access epoch")
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				dao.DB.Callback().Create().Remove(callback)
				dao.DB.Callback().Query().Remove(callback)
			})
			if err := test.run(t.Context(), knowledgeTestDomain, func(tx *gorm.DB) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if !epochSeen || !domainSeen {
				t.Fatal("worker must hold both access epoch and domain row")
			}
		})
	}
}

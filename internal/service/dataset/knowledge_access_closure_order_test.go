//go:build linux

package dataset

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Observe actual SQL projections, including fence hashing, independently of
// object-store Get. A blocked proof must prevent every derived body query.
func TestKnowledgeAccessClosureBeforeBodiesAcrossOwningEntries(t *testing.T) {
	contentLexicalTokenizer(t)
	for _, entry := range []string{"setModule", "createGrant", "inspect", "read", "search", "readFinal", "searchFinal"} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/blocked=%t", entry, blocked), func(t *testing.T) {
				f := accessTestSetup(t, true)
				selected := []entity.KnowledgeAccessTarget{}
				for _, target := range f.Scope.Targets {
					if target.Representation.Kind == "parsed_representation" {
						selected = append(selected, target)
					}
				}
				if len(selected) != 1 {
					t.Fatal("one parsed target required")
				}
				f.enable(t, selected, "read_search")
				view := f.inspect(t, "read_search", nil)
				requestID := common.GenerateUUID()
				var verified *verifiedAccessActor
				if strings.HasSuffix(entry, "Final") {
					operation := strings.TrimSuffix(entry, "Final")
					var err error
					verified, err = f.Service.accessOpenView(f.Recipient, accessTestRecipient, view.ViewID, operation, requestID)
					if err != nil {
						t.Fatal(err)
					}
				}
				accessTestSession(t, accessTestOther)
				run := func() error {
					if verified != nil {
						return f.Service.accessFinalPermit(f.Recipient, verified, strings.TrimSuffix(entry, "Final"), requestID)
					}
					actor, ctx := accessTestRecipient, f.Recipient
					var request any
					switch entry {
					case "setModule":
						actor, ctx = knowledgeTestOwner, f.Owner
						request = entity.KnowledgeAccessSetModuleRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: &f.Module.ModuleID, ExpectedVersion: f.Module.Version, Name: "Changed synthetic module", Enabled: true, Scope: f.Module.Scope, Confirmed: true, IdempotencyKey: "Synthetic_Module_Order_0002"}
					case "createGrant":
						actor, ctx = knowledgeTestOwner, f.Owner
						request = entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, ExpectedModuleVersion: f.Module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: accessTestOther}, Mode: "read_search", Delivery: "web_and_mcp", ExpiresAt: accessTime(time.Now().Add(time.Hour)), Confirmed: true, IdempotencyKey: "Synthetic_Grant_Order_0002"}
					case "inspect":
						request = entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: "read_search", Delivery: "web_only"}
					case "read":
						request = accessTestReadRequest(view.ViewID, 0, 0)
					case "search":
						request = entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "合成", Limit: 2}
					}
					status, _ := f.Service.KnowledgeAccess(ctx, actor, entry, requestID, []byte(accessJSON(request)))
					if status != 200 && status != 201 {
						return fmt.Errorf("operation rejected: %d", status)
					}
					return nil
				}
				events := []string{}
				proofReads, bodyReads := 0, 0
				const callback = "access:repair:closure-before-body"
				if err := dao.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table != "knowledge_document_representation" {
						return
					}
					body := len(tx.Statement.Selects) == 0
					for _, column := range tx.Statement.Selects {
						column = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(column, "`", "")))
						if column == "*" || strings.HasSuffix(column, ".*") || column == "reading_text" || strings.HasSuffix(column, ".reading_text") {
							body = true
						}
					}
					if slices.Contains(tx.Statement.Omits, "reading_text") {
						body = false
					}
					if body {
						bodyReads++
						events = append(events, "body")
					} else {
						proofReads++
						events = append(events, "proof")
						if blocked {
							tx.AddError(errors.New("synthetic dependency proof unavailable"))
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { dao.DB.Callback().Query().Remove(callback) })
				err := run()
				if blocked {
					if err == nil || proofReads == 0 || bodyReads != 0 {
						t.Fatalf("failed proof did not close body path: err=%v events=%v", err, events)
					}
				} else {
					if err != nil || proofReads == 0 || bodyReads == 0 || slices.Index(events, "body") < slices.Index(events, "proof") {
						t.Fatalf("proof did not precede actual body: err=%v events=%v", err, events)
					}
				}
			})
		}
	}
}

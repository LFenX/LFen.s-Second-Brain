package dataset

import (
	"errors"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/storage"
)

func TestKnowledgeArchivePreservesExistingCopyRestrictions(t *testing.T) {
	for _, mode := range []string{"raw", "uploaded", "snapshotted"} {
		t.Run(mode, func(t *testing.T) {
			s, store := setupKnowledgeTest(t)
			registerKnowledgeTest(t, s, knowledgeTestDomain)
			var kb entity.Knowledgebase
			if err := dao.DB.Where("id = ?", knowledgeTestDomain).Take(&kb).Error; err != nil {
				t.Fatal(err)
			}
			fid, sourceDoc := "f2222222222222222222222222222222", "22222222222222222222222222222222"
			body := []byte("synthetic private bytes\n")
			location, bucket := "raw.txt", *kb.KnowledgeFolderID
			if mode == "raw" {
				if err := dao.DB.Create(&entity.File{ID: fid, ParentID: bucket, TenantID: kb.TenantID, CreatedBy: kb.CreatedBy, Name: "private.txt", Type: "txt", SourceType: "local", Location: &location}).Error; err != nil {
					t.Fatal(err)
				}
				if err := store.Put(t.Context(), bucket, location, body); err != nil {
					t.Fatal(err)
				}
			} else {
				uploadKnowledgeTest(t, store, sourceDoc, "private.txt", body)
				bucket, location = knowledgeTestDomain, "synthetic/"+sourceDoc
				if mode == "snapshotted" {
					if _, err := s.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: sourceDoc}); err != nil {
						t.Fatal(err)
					}
				}
			}
			valid, copyDoc, reader := "1", "33333333333333333333333333333333", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			ensureDatasetTestMembership(t, reader, knowledgeTestOwner, "normal")
			if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestOther).Update("permission", "team").Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Create(&entity.Document{ID: copyDoc, KbID: knowledgeTestOther, ParserConfig: entity.JSONMap{}, Status: &valid}).Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Create(&entity.File2Document{ID: "existing-copy", FileID: &fid, DocumentID: &copyDoc}).Error; err != nil {
				t.Fatal(err)
			}
			refs := []permission.ResourceRef{{Kind: permission.ResourceKindFile, ID: fid}, {Kind: permission.ResourceKindDataset, ID: knowledgeTestOther}}
			assertRestrictions := func() {
				for _, ref := range refs {
					if err := permission.NewDatabaseChecker(dao.DB).CheckResource(permission.WithIdentity(t.Context(), reader, ""), ref, permission.OperationRead); err == nil {
						t.Fatalf("member read allowed: %+v", ref)
					}
					if err := permission.NewDatabaseChecker(dao.DB).CheckResource(permission.WithIdentity(t.Context(), knowledgeTestOwner, ""), ref, permission.OperationRead); err != nil {
						t.Fatalf("owner read lost: %+v err=%v", ref, err)
					}
				}
				for _, purpose := range []string{"embedding", "rerank", "generation", "remote_parse", "wiki", "memory"} {
					if err := permission.CheckDatasetModelUse(t.Context(), dao.DB, []string{knowledgeTestOther}, purpose); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
						t.Fatalf("copy purpose=%s err=%v", purpose, err)
					}
				}
			}
			assertRestrictions()
			if _, _, err := s.DeleteDatasets(permission.WithIdentity(t.Context(), knowledgeTestOwner, knowledgeTestOwner), []string{knowledgeTestDomain}, false); err != nil {
				t.Fatal(err)
			}
			assertRestrictions()
			if err := permission.CheckDatasetModelUse(t.Context(), dao.DB, []string{knowledgeTestDomain}, "wiki"); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
				t.Fatalf("old source backlog reopened: %v", err)
			}
			if err := permission.CheckKnowledgeIngestion(t.Context(), dao.DB, []string{knowledgeTestDomain}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
				t.Fatalf("old source queue reopened: %v", err)
			}
			content, err := store.Get(t.Context(), bucket, location)
			if err != nil || string(content) != string(body) {
				t.Fatalf("archive changed original bytes: err=%v", err)
			}
			var archived entity.Knowledgebase
			if err := dao.DB.Where("id = ?", kb.ID).Take(&archived).Error; err != nil || archived.Status == nil || *archived.Status != "0" || !archived.KnowledgeEnabled {
				t.Fatalf("lost tombstone: %+v err=%v", archived, err)
			}
			_, err = s.GetKnowledgeDomain(t.Context(), knowledgeTestOwner, kb.ID)
			assertKnowledgeStatus(t, err, 404)
			if _, err := s.RegisterKnowledgeDomain(t.Context(), knowledgeTestOwner, kb.ID, entity.RegisterKnowledgeDomainRequest{Category: "project"}); err == nil {
				t.Fatal("archived domain was reactivated")
			}
		})
	}
}

func TestKnowledgeRetirementReloadsCurrentRegistrationBeforeStorage(t *testing.T) {
	s, _ := setupKnowledgeTest(t)
	// This is the native dataset snapshot a caller obtained before registration.
	stale := &entity.Knowledgebase{ID: knowledgeTestDomain}
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(nil)
	t.Cleanup(func() { factory.SetStorage(previous) })
	if err := s.deleteDataset(t.Context(), stale); err != nil {
		t.Fatalf("retirement used stale policy or touched storage: %v", err)
	}
	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", stale.ID).Take(&kb).Error; err != nil || kb.Status == nil || *kb.Status != "0" || !kb.KnowledgeEnabled {
		t.Fatalf("current registration was purged: %+v err=%v", kb, err)
	}
}

func TestKnowledgeRetirementRechecksActorUnderLock(t *testing.T) {
	s, _ := setupKnowledgeTest(t)
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	other := permission.WithIdentity(t.Context(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", knowledgeTestOwner)
	if err := s.deleteDatasetAuthorized(other, &entity.Knowledgebase{ID: knowledgeTestDomain}, permission.UserIDFromContext(other)); err == nil {
		t.Fatal("unauthorized retirement allowed")
	}
	if _, err := s.GetKnowledgeDomain(t.Context(), knowledgeTestOwner, knowledgeTestDomain); err != nil {
		t.Fatalf("rejected retirement changed domain: %v", err)
	}
}

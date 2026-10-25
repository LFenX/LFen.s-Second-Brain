package dataset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestKnowledgeOriginalScratchKeepsActiveAndCleansExpired(t *testing.T) {
	s, reg, _ := originalFlowFixture(t)
	first, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest("scratch:first"))
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := s.beginOriginalTransfer(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, first.UploadID, -1, "", "", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	root, absolute, err := openOriginalScratchRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	active := "transfer-" + first.UploadID + "-1"
	stale := "transfer-" + strings.Repeat("f", 32) + "-8"
	for _, name := range []string{active, stale} {
		if err := os.Mkdir(filepath.Join(absolute, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(absolute, name, "body"), []byte("synthetic temporary bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(absolute, stale, "outside-link")); err != nil {
		t.Fatal(err)
	}
	if err := cleanOriginalScratch(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(absolute, active, "body")); err != nil {
		t.Fatal("active staging removed", err)
	}
	if _, err := os.Lstat(filepath.Join(absolute, stale)); !os.IsNotExist(err) {
		t.Fatal("expired staging retained", err)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "untouched" {
		t.Fatal("cleanup followed a symlink", err)
	}
	if err := dao.DB.Model(&entity.KnowledgeOriginalReceiveSlot{}).Where("id = ?", transfer.slot).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := cleanOriginalScratch(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(absolute, active)); !os.IsNotExist(err) {
		t.Fatal("expired active directory was not reclaimed", err)
	}
}

func TestKnowledgeOriginalScratchDoesNotAdoptForeignData(t *testing.T) {
	_, _, _ = originalFlowFixture(t)
	name := os.Getenv("LFEN_KNOWLEDGE_ORIGINAL_STAGING_ROOT")
	if err := os.Mkdir(name, 0700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(name, "keep.txt")
	if err := os.WriteFile(data, []byte("not a staging root"), 0600); err != nil {
		t.Fatal(err)
	}
	if root, _, err := openOriginalScratchRoot(); err == nil {
		root.Close()
		t.Fatal("nonempty unmarked directory adopted")
	}
	if content, err := os.ReadFile(data); err != nil || string(content) != "not a staging root" {
		t.Fatal("unclaimed data changed", err)
	}
}

package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestKnowledgeOriginalRegistrationAndSourceDigestVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/knowledge-original-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Vectors []struct {
			Registration struct {
				entity.KnowledgeOriginalRegistrationRequest
				DomainID string `json:"domain_id"`
				OwnerID  string `json:"owner_id"`
			} `json:"registration"`
			Source             entity.KnowledgeOriginalRevision `json:"source_input"`
			RegistrationDigest struct {
				SHA256 string `json:"sha256"`
			} `json:"registration_digest"`
			SourceDigest struct {
				SHA256 string `json:"sha256"`
			} `json:"source_input_digest"`
		} `json:"digest_vectors"`
	}
	if err := json.Unmarshal(raw, &examples); err != nil || len(examples.Vectors) == 0 {
		t.Fatal("frozen vectors missing", err)
	}
	for _, v := range examples.Vectors {
		if got := originalRegistrationDigest(v.Registration.DomainID, v.Registration.OwnerID, v.Registration.KnowledgeOriginalRegistrationRequest); got != v.RegistrationDigest.SHA256 {
			t.Fatalf("registration digest=%s want=%s", got, v.RegistrationDigest.SHA256)
		}
		if got := originalInputHash(v.Source.SHA256, v.Source.OriginalFilename, v.Source.MediaType, v.Source.RelativePath, v.Source.Classification); got != v.SourceDigest.SHA256 {
			t.Fatalf("source digest=%s want=%s", got, v.SourceDigest.SHA256)
		}
	}
}

func TestKnowledgeOriginalRangeVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/knowledge-original-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Cases []struct {
			Name  string `json:"name"`
			Kind  string `json:"kind"`
			Input struct {
				Authorized bool   `json:"authorized"`
				Method     string `json:"method"`
				Range      string `json:"range"`
				IfRange    string `json:"ifRange"`
				ETag       string `json:"etag"`
				Length     int64  `json:"length"`
			} `json:"input"`
			Expected json.RawMessage `json:"expected"`
		} `json:"semantic_cases"`
	}
	if err := json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, v := range examples.Cases {
		if v.Kind != "range" || !v.Input.Authorized {
			continue
		}
		count++
		t.Run(v.Name, func(t *testing.T) {
			var expected struct {
				Status int    `json:"status"`
				Start  *int64 `json:"start"`
				End    *int64 `json:"end"`
			}
			if err := json.Unmarshal(v.Expected, &expected); err != nil {
				t.Fatal(err)
			}
			status, offset, length, err := originalRange(v.Input.Length, v.Input.Range, v.Input.IfRange, v.Input.ETag, v.Input.Method == "HEAD")
			if status != expected.Status {
				t.Fatalf("status=%d want=%d err=%v", status, expected.Status, err)
			}
			if status == 416 {
				assertOriginalCode(t, err, 416, "range_not_satisfiable")
			} else if err != nil {
				t.Fatal(err)
			}
			if expected.Start != nil && (offset != *expected.Start || expected.End == nil || offset+length-1 != *expected.End) {
				t.Fatalf("range=%d+%d", offset, length)
			}
		})
	}
	if count < 5 {
		t.Fatal("authorized range vectors not executed")
	}
}

func TestKnowledgeOriginalSameBytesKeepDistinctSourceIdentities(t *testing.T) {
	s, reg, _ := originalFlowFixture(t)
	body := []byte("%PDF-1.7\nshared bytes separate sources\n%%EOF")
	first := confirmOriginalFlow(t, s, reg, prepareOriginalFlow(t, s, reg, originalTestRequest("first:source"), body), "first:save")
	second, created, err := s.CreateKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, entity.KnowledgeOriginalRegistrationRequest{RegistrationKey: "other:registration", Name: "Other exact scope", SourceKind: "original_document", InputMode: "original_upload", IncludePaths: []string{"docs/sample.pdf"}, DefaultClassification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code", Language: ""}})
	if err != nil || !created {
		t.Fatal(err)
	}
	other := confirmOriginalFlow(t, s, second, prepareOriginalFlow(t, s, second, originalTestRequest("other:source"), body), "other:save")
	if first.Source.SHA256 != other.Source.SHA256 || first.Source.SourceID == other.Source.SourceID || first.Source.SourceRevisionID == other.Source.SourceRevisionID {
		t.Fatal("content digest replaced source identity")
	}
}

func TestKnowledgeOriginalPageByteBudgetAndCursorScope(t *testing.T) {
	s, _, _ := originalFlowFixture(t)
	project, err := s.CreateKnowledgeGroup(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "Synthetic original pages"})
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 24)
	for i := range paths {
		paths[i] = fmt.Sprintf("docs/%s%02d.pdf", strings.Repeat("路", 130), i)
	}
	classification := entity.KnowledgeDirectoryClassification{GroupID: &project.ID, ComponentSide: "non_code", Language: ""}
	reg, _, err := s.CreateKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeOriginalRegistrationRequest{RegistrationKey: "pages:unicode", Name: "Synthetic page budget", SourceKind: "original_document", InputMode: "original_upload", IncludePaths: paths, DefaultClassification: classification})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("%PDF-1.7\nsynthetic page test\n%%EOF")
	var first *entity.KnowledgeOriginalReceipt
	for i, relative := range paths {
		req := originalTestRequest(fmt.Sprintf("pages:create:%d", i))
		req.RelativePath, req.Classification = relative, classification
		req.OriginalFilename = fmt.Sprintf("%s%02d.pdf", strings.Repeat("原", 240), i)
		receipt := confirmOriginalFlow(t, s, reg, prepareOriginalFlow(t, s, reg, req, body), fmt.Sprintf("pages:confirm:%d", i))
		if first == nil {
			first = receipt
		}
	}
	page, err := s.ListKnowledgeOriginalRevisions(t.Context(), knowledgeTestOwner, reg.DomainID, &project.ID, "")
	if err != nil || page.NextCursor == nil || len(page.Items) == 0 || len(page.Items) >= 20 {
		t.Fatalf("Unicode byte budget was not exercised: page=%+v err=%v", page, err)
	}
	_, err = s.ListKnowledgeOriginalRevisions(t.Context(), knowledgeTestOwner, reg.DomainID, nil, *page.NextCursor)
	assertOriginalCode(t, err, 400, "knowledge_invalid_request")
	seen := map[string]bool{}
	for pages := 0; ; pages++ {
		if pages >= 24 {
			t.Fatal("pagination failed to terminate")
		}
		encoded, err := json.Marshal(page)
		if err != nil || len(encoded) > 31744 {
			t.Fatalf("metadata exceeds envelope budget: %d %v", len(encoded), err)
		}
		for _, item := range page.Items {
			if seen[item.SourceRevisionID] {
				t.Fatal("cursor repeated a source revision")
			}
			seen[item.SourceRevisionID] = true
		}
		if page.NextCursor == nil {
			break
		}
		page, err = s.ListKnowledgeOriginalRevisions(t.Context(), knowledgeTestOwner, reg.DomainID, &project.ID, *page.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != len(paths) {
		t.Fatalf("missing sources: %d", len(seen))
	}
	entry := entity.KnowledgeProjectManifestEntry{RegistrationID: reg.RegistrationID, RelativePath: first.Source.RelativePath, SourceID: first.Source.SourceID, SourceRevisionID: first.Source.SourceRevisionID, RootNodeID: strings.Repeat("f", 64), SHA256: first.Source.SHA256, ByteLength: first.Source.ByteLength, Classification: classification}
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		return validateProjectEntries(tx, reg.DomainID, project.ID, []entity.KnowledgeProjectManifestEntry{entry})
	})
	var conflict *KnowledgeDirectoryError
	if !errors.As(err, &conflict) || conflict.Status != 409 || conflict.Code != "knowledge_batch_conflict" || conflict.Reason == nil || *conflict.Reason != "project_membership_changed" {
		t.Fatalf("0.4 original rejection=%v", err)
	}
}

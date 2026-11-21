//go:build linux

package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

func contentLexicalTokenizer(t *testing.T) {
	t.Helper()
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: "/usr/share/infinity/resource", MinSize: 1, MaxSize: 2}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tokenizer.Close)
	if err := nlp.InitQueryBuilderFromTokenizer("/usr/share/infinity/resource"); err != nil {
		t.Fatal(err)
	}
}

func contentLexicalSession(t *testing.T, s *DatasetService, local, content bool) context.Context {
	t.Helper()
	if err := dao.DB.AutoMigrate(&entity.KnowledgeStructureOperationRecord{}, &entity.KnowledgeContentPolicyRecord{}, &entity.KnowledgeLexicalPolicyRecord{}, &entity.KnowledgeLexicalOperationRecord{}, &entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}); err != nil {
		t.Fatal(err)
	}
	token, valid := "synthetic-content-lexical-native-session", "1"
	if err := dao.DB.Save(&entity.User{ID: knowledgeTestOwner, Nickname: "Synthetic", Email: "synthetic@example.invalid", AccessToken: &token, Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), lexicalSessionKey{}, lexicalSession{UserID: knowledgeTestOwner, SHA256: lexicalSHA([]byte(token))})
	if local {
		if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "enable-local-lexical", ExpectedFoundationVersion: 1, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if content {
		current, err := s.KnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.ChangeKnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: current.ContentPolicyVersion, ParseEnabled: current.ParseEnabled, LexicalEnabled: true, Confirmed: true, IdempotencyKey: "enable-content-lexical"}); err != nil {
			t.Fatal(err)
		}
	}
	return ctx
}

func contentLexicalR2(t *testing.T, body string) (*DatasetService, *documentBound, entity.KnowledgeContentTarget) {
	t.Helper()
	s, req := documentSetupTest(t, true)
	value, _, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil {
		t.Fatal(err)
	}
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", value.(*entity.KnowledgeDocumentTask).TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	fixed, err := documentDecode[documentInputs](row.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	result := documentEmptyPDFProjection()
	if body != "" {
		reading := entity.KnowledgeDocumentReadingRange{EndByte: int64(len(body)), TextSHA256: lexicalSHA([]byte(body))}
		result.ReadingText = body
		for i := range result.Payloads {
			result.Payloads[i].ReadingRange, result.Payloads[i].TextStatus, result.Payloads[i].TextReason = reading, "available", nil
		}
		page := result.Payloads[1]
		passage := page
		passage.Kind, passage.Attributes, passage.Ordinal, passage.CoordinatePrecision = "passage", json.RawMessage(`{"chunk_index":0}`), 0, "derived_range"
		passage.ParentRef = &entity.KnowledgeDocumentNaturalRef{Kind: page.Kind, Locator: page.Locator}
		passage.Locator = entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &page.Locator, ChunkIndex: 0}
		passage.OwnReadingRange = &reading
		result.Payloads = append(result.Payloads, passage)
	}
	// The projection is constructed. This proves owning publication/index
	// adaptation and nested evidence integrity, not actual PDF extraction quality.
	bound, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.documentPublish(t.Context(), claim, bound); err != nil {
		t.Fatal(err)
	}
	rep := bound.Representation
	target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: rep.DomainID, SourceID: rep.Original.SourceID, RevisionID: rep.Original.SourceRevisionID, SHA256: rep.Original.SHA256}, "document_representation", rep.RepresentationID, &rep.ProducerSHA256)
	return s, bound, target
}

func TestKnowledgeContentLexicalNestedR2UsesSameTokenizerAndDerivedEvidence(t *testing.T) {
	contentLexicalTokenizer(t)
	body := "合成内页证据\r\n规则 A\n"
	s, bound, target := contentLexicalR2(t, body)
	ctx := contentLexicalSession(t, s, true, true)
	prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	if err != nil || len(prepared.Rows) != 1 || prepared.Coverage[0].CoveredBytes != int64(len(body)) {
		t.Fatal(prepared, err)
	}
	row := prepared.Rows[0]
	ref := row.Reference
	if ref.Version != entity.KnowledgeContentVersion || ref.CoordinateSpace != "derived_utf8" || ref.Address.Target.Source.SHA256 == ref.ReadingSHA256 || ref.ReadingSHA256 != lexicalSHA([]byte(body)) || *ref.Address.Node.ID != bound.Nodes[2].ID || ref.Locator.Document.ParentLocator.Scheme != "pdf_page_v1" || ref.OriginalPosition.Kind != "pdf_page" || ref.OriginalPosition.PageNumber != 1 {
		t.Fatal("original identity or nested natural locator lost", ref)
	}
	oldResolver, attempts := component.DefaultEmbedderResolver, 0
	component.DefaultEmbedderResolver = func(context.Context, string, string) (component.Embedder, string, error) {
		attempts++
		return nil, "", errors.New("model call forbidden")
	}
	t.Cleanup(func() { component.DefaultEmbedderResolver = oldResolver })
	job := entity.KnowledgeLexicalJobRecord{DomainID: knowledgeTestDomain, Language: prepared.Policies.Language, CreatedAt: time.Now()}
	rows, _, err := lexicalTokenizeInputs(ctx, job, prepared.Inputs)
	if err != nil || len(rows) != 1 || rows[0]["content_with_weight"] != body || rows[0]["knowledge_ref"] != lexicalJSON(ref) || rows[0]["available_int"] != 0 || attempts != 0 {
		t.Fatal(rows, attempts, err)
	}
	for key := range rows[0] {
		if key == "mom_id" || strings.HasPrefix(key, "q_") && strings.HasSuffix(key, "_vec") {
			t.Fatal("unexpected model/parent field", key)
		}
	}
	native := &lexicalTestEngine{rows: map[string]map[string]any{}}
	if err := native.CreateKnowledgeLexicalRows(ctx, "ragflow_"+knowledgeTestOwner, knowledgeTestDomain, rows); err != nil {
		t.Fatal(err)
	}
	if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+knowledgeTestOwner, knowledgeTestDomain, rows); err != nil {
		t.Fatal(err)
	}
	window, err := s.ReadKnowledgeContentNode(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: ref.Address, StartByte: &ref.Range.StartByte, EndByte: &ref.Range.EndByte})
	if err != nil || window.Text != body || *window.Range.SHA256 != *ref.Range.SHA256 {
		t.Fatal(window, err)
	}
	if err := s.verifyKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, prepared); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{"text", "reference"} {
		tampered := *prepared
		tampered.Inputs = append([]lexicalTokenInput{}, prepared.Inputs...)
		if changed == "text" {
			tampered.Inputs[0].Text = strings.Repeat("x", len(body))
		} else {
			tampered.Inputs[0].Reference = strings.Replace(tampered.Inputs[0].Reference, "derived_utf8", "source_utf8", 1)
		}
		if rows, _, err := lexicalTokenizeInputs(ctx, job, tampered.Inputs); err == nil || rows != nil {
			t.Fatal("changed tokenizer input accepted", changed, err)
		}
		lexicalAssertCode(t, s.verifyKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, &tampered), "integrity_error")
	}
	t.Log("real_native_tokenizer=true es=substitute queue_admission=false sql_publication=false model_attempts=0")
}

func TestKnowledgeContentLexicalActualParsedChildUsesAllPassages(t *testing.T) {
	contentLexicalTokenizer(t)
	var original strings.Builder
	original.WriteString("编号,正文\r\n")
	for i := range 107 {
		fmt.Fprintf(&original, "%d,合成段 %03d\r\n", i, i)
	}
	s, _, req, _ := parsedExecutionFixture(t, "lexical.csv", []byte(original.String()), true)
	req.MaxUnitBytes = 32
	task, claim := parsedExecutionClaim(t, s, req)
	s.runKnowledgeParsed(t.Context(), claim)
	done := parsedExecutionCurrent(t, s, task.TaskID)
	if done.State != "succeeded" || done.Result == nil {
		failure := "<none>"
		if done.FailureCode != nil {
			failure = *done.FailureCode
		}
		t.Fatalf("actual parser task state=%s failure_code=%s", done.State, failure)
	}
	ctx := contentLexicalSession(t, s, true, true)
	prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{*done.Result}}, true)
	if err != nil || len(prepared.Rows) <= 20 || prepared.Coverage[0].IndexedUnits != int64(len(prepared.Rows)) {
		t.Fatal(prepared, err)
	}
	rows := append([]contentLexicalManifestRow{}, prepared.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Reference.Range.StartByte < rows[j].Reference.Range.StartByte })
	var rebuilt strings.Builder
	for _, row := range rows {
		ref := row.Reference
		window, err := s.ReadKnowledgeContentNode(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: ref.Address, StartByte: &ref.Range.StartByte, EndByte: &ref.Range.EndByte})
		if err != nil || lexicalSHA([]byte(window.Text)) != *ref.Range.SHA256 || row.ByteLength > 32 {
			t.Fatal(row, window, err)
		}
		rebuilt.WriteString(window.Text)
	}
	coverage := prepared.Coverage[0]
	if coverage.ExpectedBytes != int64(rebuilt.Len()) || coverage.ReadingSHA256 != lexicalSHA([]byte(rebuilt.String())) || !coverage.ReadingTextComplete || coverage.OriginalCoverage != "not_asserted" {
		t.Fatal("coverage reflects only a paged subset or conflates original bytes", coverage)
	}
	t.Logf("actual_owned_child=true fixed_passages=%d complete_derived_bytes=%d original_coverage=not_asserted", len(rows), rebuilt.Len())
}

func TestKnowledgeContentLexicalIndependentDefaultOffPolicies(t *testing.T) {
	contentLexicalTokenizer(t)
	s, _, target := contentLexicalR2(t, "已有 R2 合成正文")
	ctx := contentLexicalSession(t, s, false, false)
	_, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	lexicalAssertCode(t, err, "policy_disabled")
	if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "local-only", ExpectedFoundationVersion: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, err = s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	lexicalAssertCode(t, err, "policy_disabled")
	if _, _, err := s.ChangeKnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, LexicalEnabled: true, Confirmed: true, IdempotencyKey: "lexical-without-parse"}); err != nil {
		t.Fatal(err)
	}
	policy, err := s.KnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || policy.ParseEnabled || !policy.LexicalEnabled {
		t.Fatal("local lexical permission implied parse permission", policy, err)
	}
	prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	if err != nil || prepared == nil {
		t.Fatal(prepared, err)
	}
	_, err = s.prepareKnowledgeContentLexical(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	lexicalAssertCode(t, err, "unauthenticated")
	_, err = s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target, target}}, true)
	lexicalAssertCode(t, err, "invalid_request")
}

func TestKnowledgeContentLexicalRejectsFixedDamageAndRevocation(t *testing.T) {
	contentLexicalTokenizer(t)
	for _, failure := range []string{"reading", "manifest", "passage", "parent", "extra_node", "original_sha", "producer", "session", "membership", "content_policy_during_read"} {
		t.Run(failure, func(t *testing.T) {
			s, bound, target := contentLexicalR2(t, "合成完整性正文\r\n")
			ctx := contentLexicalSession(t, s, true, true)
			prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
			if err != nil {
				t.Fatal(err)
			}
			code := "integrity_error"
			switch failure {
			case "reading":
				err = dao.DB.Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("id = ?", *target.Representation.ID).Update("reading_text", "tampered").Error
			case "manifest":
				manifest := bound.Manifest
				manifest.Nodes[1].NaturalRef.Locator.PageNumber = 2
				err = dao.DB.Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("id = ?", *target.Representation.ID).Update("manifest", lexicalJSON(manifest)).Error
			case "passage":
				err = dao.DB.Model(&entity.KnowledgeDocumentNodeRecord{}).Where("id = ?", bound.Nodes[2].ID).Update("payload", strings.Replace(bound.Nodes[2].Payload, `"start_byte":0`, `"start_byte":1`, 1)).Error
			case "parent":
				err = dao.DB.Model(&entity.KnowledgeDocumentNodeRecord{}).Where("id = ?", bound.Nodes[2].ID).Update("parent_id", bound.Nodes[0].ID).Error
			case "extra_node":
				row := bound.Nodes[2]
				row.ID = strings.Repeat("e", 64)
				err = dao.DB.Create(&row).Error
			case "original_sha":
				prepared.Selection.Targets[0].Source.SHA256 = strings.Repeat("f", 64)
			case "producer":
				prepared.Selection.Targets[0].Representation.ProducerSHA256 = contentString(strings.Repeat("b", 64))
			case "session":
				code = "unauthenticated"
				err = dao.DB.Model(&entity.User{}).Where("id = ?", knowledgeTestOwner).Update("access_token", "revoked-synthetic-session").Error
			case "membership":
				code = "not_found"
				err = dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "0").Error
			case "content_policy_during_read":
				code = "policy_disabled"
				callback, revoked := "synthetic:content_lexical_revoke_during_read", false
				err = dao.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
					if revoked || tx.Statement.Table != "knowledge_document_representation" || !slices.Contains(tx.Statement.Selects, "reading_text") {
						return
					}
					revoked = true
					if _, _, err := s.ChangeKnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: prepared.Policies.ContentVersion, Confirmed: true, IdempotencyKey: "revoke-during-index-plan"}); err != nil {
						t.Fatal(err)
					}
				})
				t.Cleanup(func() { dao.DB.Callback().Query().Remove(callback) })
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, prepared.Selection, true)
			lexicalAssertCode(t, err, code)
			if result != nil {
				t.Fatal("units escaped after damaged source or revoked authority")
			}
		})
	}
}

func TestKnowledgeContentLexicalEmptyAndPartialCoverageRemainExplicit(t *testing.T) {
	contentLexicalTokenizer(t)
	t.Run("empty_physical_page", func(t *testing.T) {
		s, _, target := contentLexicalR2(t, "")
		ctx := contentLexicalSession(t, s, true, true)
		prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
		if err != nil || len(prepared.Rows) != 0 || prepared.Coverage[0].TextIndexState != "empty" || prepared.Coverage[0].UnsearchableNodes != 1 || prepared.Coverage[0].OriginalCoverage != "not_asserted" {
			t.Fatal(prepared, err)
		}
	})
	t.Run("partial_parser_content", func(t *testing.T) {
		s, prior, _ := parsedPersistenceFixture(t)
		original := []byte("A,B\n甲,乙\n1,2\n")
		result := parser.NormalizeParseResult(parser.ParseResult{JSON: []parser.DocItem{parser.TextItem{Text: "可检索内容\r\n"}, parser.ImageItem{Text: "alt is not OCR", Image: "excluded-image-payload"}, parser.RawItem{M: map[string]any{"text": "excluded-unknown-content"}}, parser.TextItem{Text: "第二段"}}})
		bound, err := bindKnowledgeParsed(prior.Representation.Source, original, strings.Repeat("a", 64), result, 32)
		if err != nil {
			t.Fatal(err)
		}
		parsedStore(t, bound, strings.Repeat("b", 32))
		ctx := contentLexicalSession(t, s, true, true)
		target := entity.KnowledgeContentTarget{Source: bound.Representation.Source, Representation: bound.Representation.Ref}
		prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
		if err != nil || len(prepared.Rows) == 0 {
			t.Fatal(prepared, err)
		}
		coverage := prepared.Coverage[0]
		if !coverage.ReadingTextComplete || coverage.RepresentationState != "partial" || coverage.UnsearchableNodes != 2 || coverage.OriginalCoverage != "not_asserted" || strings.Contains(lexicalJSON(prepared.Inputs), "excluded-") {
			t.Fatal("derived coverage promoted partial content to original completeness", coverage)
		}
	})
}

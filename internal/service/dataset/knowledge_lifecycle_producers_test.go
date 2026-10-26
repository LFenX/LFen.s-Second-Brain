package dataset

import (
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func lifecycleTestABA(t *testing.T, s *DatasetService, source string) {
	t.Helper()
	lifecycleTestAction(t, s, source, "archive", lifecycleTestCommand(lifecycleTestDetail(t, s, source), "archive"))
	lifecycleTestAction(t, s, source, "restore", lifecycleTestCommand(lifecycleTestDetail(t, s, source), "restore"))
}

func TestKnowledgeLifecycleDirectoryObservationAndPreviewFences(t *testing.T) {
	for _, mode := range []string{"observation", "preview", "unfenced_observation", "unfenced_preview"} {
		t.Run(mode, func(t *testing.T) {
			f := setupDirectoryTest(t)
			first := f.commit(t, f.preview(t, f.observe(t, "web/card.tsx"), f.reg.DefaultClassification), "initial")
			source := *first.Items[0].SourceID
			obs := f.observe(t, "web/card.tsx")
			var preview *entity.KnowledgeDirectoryPreview
			if strings.HasSuffix(mode, "preview") {
				preview = f.preview(t, obs, f.reg.DefaultClassification)
			}
			if strings.HasPrefix(mode, "unfenced_") {
				preparation := obs.ObservationID
				if preview != nil {
					preparation = preview.PreviewID
				}
				if err := dao.DB.Where("preparation_id = ?", preparation).Delete(&entity.KnowledgeSourceFence{}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				lifecycleTestABA(t, f.service, source)
			}
			if preview == nil {
				_, err := f.service.CreateKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryPreviewRequest{ObservationID: obs.ObservationID, ObservationSHA256: obs.ObservationSHA256, RelativePath: "web/card.tsx", Classification: f.reg.DefaultClassification})
				assertDirectoryError(t, err, 409, "revision_changed")
			} else {
				_, _, err := f.service.CommitKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryCommitRequest{PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: "pending"})
				assertDirectoryError(t, err, 409, "revision_changed")
			}
		})
	}
}

func TestKnowledgeLifecycleBatchAcceptanceAndWorkerFences(t *testing.T) {
	for _, mode := range []string{"before_accept", "after_accept", "unfenced_worker"} {
		t.Run(mode, func(t *testing.T) {
			f := setupBatchTest(t)
			first := f.commit(t, f.preview(t, f.observe(t, "web/card.tsx"), f.reg.DefaultClassification), "initial")
			source := *first.Items[0].SourceID
			p := prepareBatchTest(t, f, "web/card.tsx")
			if mode == "before_accept" {
				lifecycleTestABA(t, f.service, source)
				_, _, err := f.service.AcceptKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, batchRequest(p, "pending"))
				assertDirectoryError(t, err, 409, "revision_changed")
				return
			}
			task := acceptBatchTest(t, f, p, "pending")
			if mode == "after_accept" {
				lifecycleTestABA(t, f.service, source)
			} else {
				if err := dao.DB.Where("preparation_id = ?", p.Items[0].ItemID).Delete(&entity.KnowledgeSourceFence{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			done := runBatchTest(t, f, task.TaskID)
			if done.Status != "failed" {
				t.Fatal(done)
			}
			items, err := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
			if err != nil || len(items.Items) != 1 || items.Items[0].Code != "revision_changed" {
				t.Fatal(items, err)
			}
		})
	}
}

func TestKnowledgeLifecycleOriginalUploadFenceNeverResamples(t *testing.T) {
	for _, mode := range []string{"state_aba", "unfenced"} {
		t.Run(mode, func(t *testing.T) {
			s, reg, _ := originalFlowFixture(t)
			body := []byte("%PDF-1.7\nsynthetic\n%%EOF")
			first := confirmOriginalFlow(t, s, reg, prepareOriginalFlow(t, s, reg, originalTestRequest("first"), body), "initial")
			request := originalTestRequest("pending")
			request.ExpectedRevisionID = &first.Source.SourceRevisionID
			ready := prepareOriginalFlow(t, s, reg, request, body)
			if mode == "state_aba" {
				lifecycleTestABA(t, s, first.Source.SourceID)
			} else {
				if err := dao.DB.Where("preparation_id = ?", ready.UploadID).Delete(&entity.KnowledgeSourceFence{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := s.ConfirmKnowledgeOriginalImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeOriginalConfirmRequest{UploadID: ready.UploadID, ManifestSHA256: *ready.ManifestSHA256, IdempotencyKey: "confirm_pending", Confirmed: true})
			assertOriginalCode(t, err, 409, "revision_changed")
		})
	}
}

func TestKnowledgeLifecycleWebExplicitPreviewAndFixedMetadata(t *testing.T) {
	s, reg, store := webFlowFixture(t)
	raw := []byte(`<rss version="2.0"><channel><item><guid>source</guid><description><![CDATA[<pre>synthetic code</pre>]]></description></item></channel></rss>`)
	ready := webReadyFixture(t, s, reg, "capture", raw)
	first, _ := webConfirmFixture(t, s, reg, webPreviewFixture(t, s, reg, ready, "first"), "confirm")
	items, err := s.ListKnowledgeWebReceiptItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, first.ImportTaskID, "")
	if err != nil {
		t.Fatal(err)
	}
	source := items.Items[0].SourceID
	pending := webPreviewFixture(t, s, reg, ready, "pending")
	pitems, err := s.ListKnowledgeWebPreviewItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, pending.PreviewID, "")
	if err != nil {
		t.Fatal(err)
	}
	lifecycleTestABA(t, s, source)
	_, _, err = s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeWebConfirm{PreviewID: pending.PreviewID, ManifestSHA256: pending.ManifestSHA256, IdempotencyKey: "pending_confirm", Confirmed: true, AcknowledgedEntryKeys: []string{pitems.Items[0].EntryKey}})
	assertOriginalCode(t, err, 409, "revision_changed")
	newPreview := webPreviewFixture(t, s, reg, ready, "fresh_explicit")
	_, _ = webConfirmFixture(t, s, reg, newPreview, "fresh_confirm")
	beforeReads := store.wholeReads + store.streamReads
	v := lifecycleTestDetail(t, s, source)
	class := v.Current.Classification
	class.ComponentSide = "shared"
	meta := lifecycleTestAction(t, s, source, "reclassify", entity.KnowledgeLifecycleReclassify{KnowledgeLifecycleCommand: lifecycleTestCommand(v, "metadata"), Classification: class})
	if store.wholeReads+store.streamReads != beforeReads {
		t.Fatal("web metadata read object body")
	}
	fixed, err := s.GetKnowledgeWebRevision(t.Context(), knowledgeTestOwner, reg.DomainID, meta.After.SourceRevisionID)
	if err != nil || fixed.CaptureID != ready.CaptureID || fixed.Classification.ComponentSide != "shared" {
		t.Fatal(fixed, err)
	}
	var roots int64
	dao.DB.Model(&entity.KnowledgeNode{}).Count(&roots)
	if roots != 0 {
		t.Fatal("web metadata created C1 root")
	}
	if v.Actions["move"].Supported {
		t.Fatal("web move exposed")
	}
}

func TestKnowledgeLifecycleProjectGenerationAndOrganization(t *testing.T) {
	for _, mode := range []string{"generation_aba", "unfenced", "reclassify", "historical_body"} {
		t.Run(mode, func(t *testing.T) {
			f := setupBatchTest(t)
			project, err := f.service.CreateKnowledgeGroup(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "Synthetic"})
			if err != nil {
				t.Fatal(err)
			}
			f.reg.DefaultClassification.GroupID = &project.ID
			p := prepareBatchTest(t, f, "web/card.tsx")
			task := runBatchTest(t, f, acceptBatchTest(t, f, p, "first").TaskID)
			items, err := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
			if err != nil {
				t.Fatal(err)
			}
			source := *items.Items[0].SourceID
			if mode == "historical_body" {
				f.write(t, "web/card.tsx", "// new body\n")
				next := prepareBatchTest(t, f, "web/card.tsx")
				runBatchTest(t, f, acceptBatchTest(t, f, next, "next").TaskID)
			}
			preview, err := f.service.PrepareKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPreviewRequest{ProjectID: project.ID, TaskID: task.TaskID})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "generation_aba" {
				lifecycleTestABA(t, f.service, source)
			}
			if mode == "unfenced" {
				if err := dao.DB.Where("preparation_id = ?", preview.PreviewID).Delete(&entity.KnowledgeSourceFence{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode == "reclassify" {
				v := lifecycleTestDetail(t, f.service, source)
				class := v.Current.Classification
				class.ComponentSide = "shared"
				lifecycleTestAction(t, f.service, source, "reclassify", entity.KnowledgeLifecycleReclassify{KnowledgeLifecycleCommand: lifecycleTestCommand(v, "class"), Classification: class})
			}
			_, created, err := f.service.PublishKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPublicationRequest{ProjectID: project.ID, PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: "publish"})
			if mode == "historical_body" {
				if err != nil || !created {
					t.Fatal("explicit historical body with same organization rejected", err)
				}
			} else if err == nil {
				t.Fatal("stale project published", mode)
			}
		})
	}
}

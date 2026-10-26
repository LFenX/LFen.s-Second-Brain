package dataset

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupStructureReadTest(t *testing.T, body []byte) (*DatasetService, *entity.KnowledgeStructureView, string) {
	t.Helper()
	s, store := setupKnowledgeTest(t)
	models := []any{&entity.KnowledgeStructurePolicyRecord{}, &entity.KnowledgeStructureOperationRecord{}, &entity.KnowledgeStructureTaskRecord{}, &entity.KnowledgeStructureLeaseRecord{}, &entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureHeadRecord{}, &entity.KnowledgeStructureNodeRecord{}, &entity.KnowledgeStructureRelationRecord{}, &entity.KnowledgeStructureArtifactRecord{}, &entity.KnowledgeStructureCursorRecord{}, &entity.KnowledgeProjectManifestHeadRecord{}}
	if err := dao.DB.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	project, err := s.CreateKnowledgeGroup(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "Synthetic structure"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := strings.Repeat("2", 32)
	uploadKnowledgeTest(t, store, sourceID, "sample.md", body)
	snapshot, err := s.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: sourceID, RelativePath: "docs/sample.md", GroupID: &project.ID, ComponentSide: "non_code", Language: "markdown"})
	if err != nil {
		t.Fatal(err)
	}
	viewID := entity.KnowledgeID("synthetic structure read view")
	nodeID := entity.KnowledgeID("synthetic structure read root")
	producer, err := entity.KnowledgeStructureProducerDigest([]entity.KnowledgeStructureAlias{})
	if err != nil {
		t.Fatal(err)
	}
	class := entity.KnowledgeStructureClassification{GroupID: &project.ID, ComponentSide: "non_code", Language: "markdown"}
	view := &entity.KnowledgeStructureView{ViewID: viewID, DomainID: knowledgeTestDomain, OwnerID: knowledgeTestOwner, ProjectID: project.ID, ManifestID: strings.Repeat("3", 32), ManifestSHA256: strings.Repeat("4", 64), FoundationPolicyVersion: 1, StructurePolicyVersion: 1, ProducerSHA256: producer, ProducerFormat: "code-structure-v1", Aliases: []entity.KnowledgeStructureAlias{}, Members: []entity.KnowledgeStructureViewMember{{SourceID: sourceID, SourceRevisionID: snapshot.Source.ID, SourceNodeID: snapshot.Root.ID, RootStructureNodeID: nodeID, LifecycleVersion: 1, Parser: "markdown", SHA256: snapshot.Source.SHA256, ByteLength: int64(len(body)), RelativePath: "docs/sample.md", Classification: class}}, SourceCount: 1, NodeCount: 1, SourceBytes: int64(len(body)), PublishedAt: time.Now().UTC().Format(time.RFC3339Nano), LexicalIndex: "not_created", VectorIndex: "not_created"}
	raw, _ := json.Marshal(view)
	if err := dao.DB.Create(&entity.KnowledgeStructureViewRecord{ID: viewID, DomainID: knowledgeTestDomain, OwnerID: knowledgeTestOwner, ProjectID: project.ID, Published: true, Payload: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.KnowledgeStructureHeadRecord{ProjectID: project.ID, DomainID: knowledgeTestDomain, OwnerID: knowledgeTestOwner, ViewID: viewID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.KnowledgeProjectManifestHeadRecord{ProjectID: project.ID, DomainID: knowledgeTestDomain, ManifestID: view.ManifestID}).Error; err != nil {
		t.Fatal(err)
	}
	node := entity.KnowledgeStructureNode{StructureNodeID: nodeID, LogicalNodeID: entity.KnowledgeID("synthetic logical root"), ViewID: viewID, SourceID: sourceID, SourceRevisionID: snapshot.Source.ID, SourceNodeID: snapshot.Root.ID, Kind: "file", Name: "sample.md", LogicalKey: "file", IdentityStability: "file", StartByte: 0, EndByte: int64(len(body)), StartLine: 1, EndLine: knowledgeLastLine(body), Parser: "markdown", Classification: class, CoordinateSpace: "source_utf8", ProducerSHA256: producer}
	raw, _ = json.Marshal(node)
	if err := dao.DB.Create(&entity.KnowledgeStructureNodeRecord{ID: nodeID, OwnerID: knowledgeTestOwner, DomainID: knowledgeTestDomain, ViewID: viewID, SourceID: sourceID, SourceRevisionID: snapshot.Source.ID, Kind: "file", StartByte: 0, EndByte: node.EndByte, FragmentSHA256: directorySHA(body), Payload: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	return s, view, nodeID
}

func TestKnowledgeStructureFixedReadWindows(t *testing.T) {
	body := append([]byte("\xef\xbb\xbf"+strings.Repeat("a", 65532)), []byte("😀\r\n尾行\t")...)
	s, view, node := setupStructureReadTest(t, body)
	first, err := s.ReadKnowledgeStructureNode(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, node, nil, nil)
	if err != nil || first.EndByte != 65535 || first.Complete || first.NextStartByte == nil || !bytes.Equal([]byte(first.Text), body[:65535]) {
		t.Fatal(first, err)
	}
	tail, err := s.ReadKnowledgeStructureNode(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, node, first.NextStartByte, nil)
	if err != nil || !tail.Complete || tail.NextStartByte != nil || !bytes.Equal(append([]byte(first.Text), []byte(tail.Text)...), body) {
		t.Fatal("fixed bytes changed across windows", tail, err)
	}
	for name, values := range map[string][2]*int64{
		"end_without_start": {nil, structureInt(10)}, "split_rune": {structureInt(65536), structureInt(65539)}, "over_budget": {structureInt(0), structureInt(65537)}, "outside_node": {structureInt(0), structureInt(int64(len(body)) + 1)}, "negative": {structureInt(-1), nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.ReadKnowledgeStructureNode(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, node, values[0], values[1])
			assertKnowledgeStatus(t, err, 400)
		})
	}
	// Archiving and disabling processing preserve authorized fixed history.
	if err := dao.DB.Model(&entity.KnowledgeSourceLifecycle{}).Where("source_id = ?", view.Members[0].SourceID).Updates(map[string]any{"state": "archived", "version": 2}).Error; err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadKnowledgeStructureNode(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, node, nil, nil)
	if err != nil || after.FragmentSHA256 != first.FragmentSHA256 {
		t.Fatal("archive revoked fixed history", err)
	}
	_, err = s.GetKnowledgeStructureView(t.Context(), "intruder", view.DomainID, view.ViewID)
	assertKnowledgeStatus(t, err, 404)
	if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ?", knowledgeTestOwner).Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeStructureNode(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, node, nil, nil)
	assertKnowledgeStatus(t, err, 404)
}

func structureInt(value int64) *int64 { return &value }

func TestKnowledgeStructureCursorScopeAndContainmentProjection(t *testing.T) {
	s, view, root := setupStructureReadTest(t, []byte("# One\r\ntext😀\n# Two\n"))
	for i := int64(0); i < 3; i++ {
		id := entity.KnowledgeID("synthetic child", string(rune('a'+i)))
		node := entity.KnowledgeStructureNode{StructureNodeID: id, ViewID: view.ViewID, SourceID: view.Members[0].SourceID, SourceRevisionID: view.Members[0].SourceRevisionID, SourceNodeID: view.Members[0].SourceNodeID, ParentID: &root, Kind: "passage", StartByte: i, EndByte: i + 1}
		raw, _ := json.Marshal(node)
		if err := dao.DB.Create(&entity.KnowledgeStructureNodeRecord{ID: id, OwnerID: knowledgeTestOwner, DomainID: view.DomainID, ViewID: view.ViewID, SourceID: node.SourceID, SourceRevisionID: node.SourceRevisionID, ParentID: root, Kind: "passage", StartByte: i, EndByte: i + 1, FragmentSHA256: directorySHA([]byte{'#'}), Payload: string(raw)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListKnowledgeStructureNodes(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, view.Members[0].SourceID, root, "", 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatal(first, err)
	}
	second, err := s.ListKnowledgeStructureNodes(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, view.Members[0].SourceID, root, *first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].StructureNodeID == first.Items[0].StructureNodeID {
		t.Fatal(second, err)
	}
	_, err = s.ListKnowledgeStructureNodes(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, view.Members[0].SourceID, root, *first.NextCursor, 2)
	assertKnowledgeStatus(t, err, 400)
	_, err = s.ListKnowledgeStructureNodes(t.Context(), "intruder", view.DomainID, view.ViewID, view.Members[0].SourceID, root, "bad cursor", 2)
	assertKnowledgeStatus(t, err, 404)
	relations, err := s.ListKnowledgeStructureRelations(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, root, "out", "contains", "", 1)
	if err != nil || len(relations.Items) != 1 || relations.NextCursor == nil || relations.Items[0].Reason != "syntax_containment" {
		t.Fatal(relations, err)
	}
	next, err := s.ListKnowledgeStructureRelations(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, root, "out", "contains", *relations.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].RelationID <= relations.Items[0].RelationID {
		t.Fatal(next, err)
	}
	// No second containment graph is materialized in relation storage.
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeStructureRelationRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
	var cursor entity.KnowledgeStructureCursorRecord
	if err := dao.DB.Where("id = ?", *first.NextCursor).Take(&cursor).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&cursor).Update("expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.ListKnowledgeStructureNodes(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, view.Members[0].SourceID, root, *first.NextCursor, 1)
	assertKnowledgeStatus(t, err, 400)
}

func TestKnowledgeStructureHeadReportsArchiveAndPolicyWithoutHidingHistory(t *testing.T) {
	s, view, root := setupStructureReadTest(t, []byte("# Synthetic\n"))
	head, err := s.GetKnowledgeStructureHead(t.Context(), knowledgeTestOwner, view.DomainID, view.ProjectID)
	if err != nil || head.Currentness != "current" || len(head.Reasons) != 0 {
		t.Fatal(head, err)
	}
	if err := dao.DB.Create(&entity.KnowledgeStructurePolicyRecord{DomainID: view.DomainID, OwnerID: knowledgeTestOwner, Version: 2, Enabled: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.KnowledgeSourceLifecycle{}).Where("source_id = ?", view.Members[0].SourceID).Updates(map[string]any{"state": "archived", "version": 2}).Error; err != nil {
		t.Fatal(err)
	}
	head, err = s.GetKnowledgeStructureHead(t.Context(), knowledgeTestOwner, view.DomainID, view.ProjectID)
	if err != nil || head.Currentness != "stale" || len(head.AffectedSources) != 1 || !strings.Contains(strings.Join(head.Reasons, ","), "source_archived") || !strings.Contains(strings.Join(head.Reasons, ","), "policy_changed") {
		t.Fatal(head, err)
	}
	if _, err = s.ReadKnowledgeStructureNode(t.Context(), knowledgeTestOwner, view.DomainID, view.ViewID, root, nil, nil); err != nil {
		t.Fatal("stale processing view lost authorized history", err)
	}
}

package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Reading a fixed representation checks current native/domain/source authority
// independently of its head currentness or the local processing permission.
func (d *DatasetService) documentReadRepresentation(ctx context.Context, owner, domain, id string) (*entity.KnowledgeDocumentRepresentation, error) {
	var value *entity.KnowledgeDocumentRepresentation
	err := d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		if !isKnowledgeID(id, 64) {
			return documentError(404, "not_found")
		}
		var row entity.KnowledgeDocumentRepresentationRecord
		if err := tx.Omit("reading_text", "manifest").Where("id = ? AND owner_id = ? AND domain_id = ?", id, owner, domain).Take(&row).Error; err != nil {
			return err
		}
		rep, err := documentDecode[entity.KnowledgeDocumentRepresentation](row.Payload)
		if err != nil {
			return err
		}
		if rep.RepresentationID != id || rep.OwnerID != owner || rep.DomainID != domain || rep.Original.SourceID != row.SourceID || rep.Original.SourceRevisionID != row.SourceRevisionID {
			return documentError(409, "document_integrity")
		}
		source, err := loadOriginalRevision(tx, domain, rep.Original.SourceRevisionID)
		if err != nil {
			return err
		}
		if source.DocumentID != rep.Original.SourceID || source.SHA256 != rep.Original.SHA256 || source.ByteLength != rep.Original.ByteLength || source.OriginalRegistrationID != rep.Original.RegistrationID {
			return documentError(409, "document_integrity")
		}
		digest, err := entity.KnowledgeDocumentRepresentationDigest(*rep)
		if err != nil || digest != id {
			return documentError(409, "document_integrity")
		}
		value = rep
		return nil
	})
	return value, err
}

func (d *DatasetService) GetKnowledgeDocumentRepresentation(ctx context.Context, owner, domain, id string) (*entity.KnowledgeDocumentRepresentation, error) {
	return d.documentReadRepresentation(ctx, owner, domain, id)
}

func documentReadNode(ctx context.Context, owner, domain string, rep *entity.KnowledgeDocumentRepresentation, id string) (*entity.KnowledgeDocumentNode, error) {
	if !isKnowledgeID(id, 64) {
		return nil, documentError(404, "not_found")
	}
	var row entity.KnowledgeDocumentNodeRecord
	if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND representation_id = ?", id, owner, domain, rep.RepresentationID).Take(&row).Error; err != nil {
		return nil, documentNormalize(err)
	}
	node, err := documentDecode[entity.KnowledgeDocumentNode](row.Payload)
	if err != nil {
		return nil, err
	}
	if node.DocumentNodeID != id || node.RepresentationID != rep.RepresentationID || node.SourceID != rep.Original.SourceID || node.SourceRevisionID != rep.Original.SourceRevisionID || node.OriginalSHA256 != rep.Original.SHA256 || node.ProducerSHA256 != rep.ProducerSHA256 {
		return nil, documentError(409, "document_integrity")
	}
	sha, err := entity.KnowledgeDocumentPayloadDigest(node.Payload)
	if err != nil || sha != node.NodePayloadSHA256 {
		return nil, documentError(409, "document_integrity")
	}
	nodeID, err := entity.KnowledgeDocumentNodeDigest(rep.RepresentationID, node.SourceRevisionID, sha)
	if err != nil || nodeID != id {
		return nil, documentError(409, "document_integrity")
	}
	return node, nil
}

func (d *DatasetService) GetKnowledgeDocumentNode(ctx context.Context, owner, domain, repID, nodeID string) (*entity.KnowledgeDocumentNode, error) {
	rep, err := d.documentReadRepresentation(ctx, owner, domain, repID)
	if err != nil {
		return nil, err
	}
	node, err := documentReadNode(ctx, owner, domain, rep, nodeID)
	if err != nil {
		return nil, err
	}
	if _, err := d.documentReadRepresentation(ctx, owner, domain, repID); err != nil {
		return nil, err
	}
	return node, nil
}

func (d *DatasetService) ListKnowledgeDocumentNodes(ctx context.Context, owner, domain, repID, parentID, cursorID string, limit int) (*entity.KnowledgeDocumentNodePage, error) {
	rep, err := d.documentReadRepresentation(ctx, owner, domain, repID)
	if err != nil {
		return nil, err
	}
	if _, err := documentReadNode(ctx, owner, domain, rep, parentID); err != nil {
		return nil, err
	}
	scope, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-document-node-cursor/0.1.0", "owner_id": owner, "domain_id": domain, "representation_id": repID, "parent_id": parentID, "limit": limit})
	cursor, err := structureLoadReadCursor(ctx, owner, domain, scope, cursorID, limit)
	if err != nil {
		var old *KnowledgeError
		if errors.As(err, &old) && old.Status == 400 {
			return nil, documentError(400, "invalid_request")
		}
		return nil, documentNormalize(err)
	}
	var rows []entity.KnowledgeDocumentNodeRecord
	if err := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND representation_id = ? AND parent_id = ?", owner, domain, repID, parentID).Order("ordinal ASC, id ASC").Offset(cursor.Offset).Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, documentNormalize(err)
	}
	page := &entity.KnowledgeDocumentNodePage{RepresentationID: repID, SourceID: rep.Original.SourceID, SourceRevisionID: rep.Original.SourceRevisionID, ParentID: parentID, Items: []entity.KnowledgeDocumentNode{}}
	for _, row := range rows[:min(limit, len(rows))] {
		node, err := documentReadNode(ctx, owner, domain, rep, row.ID)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, *node)
		raw, err := json.Marshal(page)
		if err != nil {
			return nil, documentError(503, "unavailable")
		}
		if len(raw) > 240<<10 {
			page.Items = page.Items[:len(page.Items)-1]
			break
		}
	}
	if len(rows) > len(page.Items) {
		if len(page.Items) == 0 {
			return nil, documentError(503, "unavailable")
		}
		cursor.Offset += len(page.Items)
		page.NextCursor, err = structureSaveReadCursor(ctx, owner, domain, scope, cursor)
	}
	if err != nil {
		return nil, documentNormalize(err)
	}
	if _, err := d.documentReadRepresentation(ctx, owner, domain, repID); err != nil {
		return nil, err
	}
	return page, nil
}

func (d *DatasetService) ReadKnowledgeDocumentNode(ctx context.Context, owner, domain, repID, nodeID string, start, end *int64) (*entity.KnowledgeDocumentContentWindow, error) {
	rep, err := d.documentReadRepresentation(ctx, owner, domain, repID)
	if err != nil {
		return nil, err
	}
	node, err := documentReadNode(ctx, owner, domain, rep, nodeID)
	if err != nil {
		return nil, err
	}
	var row entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.WithContext(ctx).Select("id", "reading_text").Where("id = ? AND owner_id = ? AND domain_id = ?", repID, owner, domain).Take(&row).Error; err != nil {
		return nil, documentNormalize(err)
	}
	text := row.ReadingText
	if int64(len(text)) != rep.ReadingByteLength || !utf8.ValidString(text) || directorySHA([]byte(text)) != rep.ReadingTextSHA256 || !documentRangeValid(node.Payload.ReadingRange, text) {
		return nil, documentError(409, "document_integrity")
	}
	r := node.Payload.ReadingRange
	a, b := r.StartByte, r.EndByte
	if start != nil {
		a = *start
	}
	if end != nil {
		b = *end
	} else {
		b = min(b, a+65536)
		for b > a && b < int64(len(text)) && !utf8.RuneStart(text[b]) {
			b--
		}
	}
	if a < r.StartByte || b < a || b > r.EndByte || b-a > 65536 || a > b || a < int64(len(text)) && !utf8.RuneStart(text[a]) || b < int64(len(text)) && !utf8.RuneStart(text[b]) || a == b && r.EndByte > r.StartByte {
		return nil, documentError(400, "invalid_request")
	}
	fragment := text[a:b]
	complete := b == r.EndByte
	var next *int64
	if !complete {
		n := b
		next = &n
	}
	value := &entity.KnowledgeDocumentContentWindow{RepresentationID: repID, DocumentNodeID: nodeID, SourceID: rep.Original.SourceID, SourceRevisionID: rep.Original.SourceRevisionID, OriginalSHA256: rep.Original.SHA256, ReadingTextSHA256: rep.ReadingTextSHA256, NodeTextSHA256: r.TextSHA256, FragmentSHA256: directorySHA([]byte(fragment)), CoordinateSpace: "derived_utf8", TextStatus: node.Payload.TextStatus, StartByte: a, EndByte: b, Text: fragment, Complete: complete, NextStartByte: next}
	if _, err := d.documentReadRepresentation(ctx, owner, domain, repID); err != nil {
		return nil, err
	}
	return value, nil
}

func (d *DatasetService) GetKnowledgeDocumentHead(ctx context.Context, owner, domain, source string) (*entity.KnowledgeDocumentHead, error) {
	var value *entity.KnowledgeDocumentHead
	err := d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		data, err := loadLifecycleSource(tx, kb, source)
		if err != nil {
			return err
		}
		if data.head.RepresentationKind != "original_only" || data.registration == nil || data.registration.SourceKind != "original_document" {
			return documentError(404, "not_found")
		}
		policy, err := documentPolicyRecord(tx, owner, domain)
		if err != nil {
			return err
		}
		head, err := documentActiveHead(tx, owner, domain, source)
		if err != nil {
			return err
		}
		value = &entity.KnowledgeDocumentHead{DomainID: domain, SourceID: source, CurrentSourceRevisionID: data.head.ID, CurrentLifecycleVersion: data.state.Version, FoundationPolicyVersion: kb.KnowledgePolicyVersion, DocumentPolicyVersion: policy.Version, Currentness: "none", Reasons: []string{}, CheckedAt: structureTimestamp(now)}
		var latest entity.KnowledgeStructureTaskRecord
		e := tx.Select("id").Where("owner_id = ? AND domain_id = ? AND protocol = ? AND target_kind = ? AND target_id = ?", owner, domain, documentTaskProtocol, "original_document", source).Order("created_at DESC, id DESC").Take(&latest).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil {
			value.LatestTaskID = &latest.ID
		}
		if head == nil {
			return nil
		}
		var row entity.KnowledgeDocumentRepresentationRecord
		if err := tx.Omit("reading_text", "manifest").Where("id = ? AND domain_id = ? AND owner_id = ? AND source_id = ?", *head, domain, owner, source).Take(&row).Error; err != nil {
			return err
		}
		rep, err := documentDecode[entity.KnowledgeDocumentRepresentation](row.Payload)
		if err != nil {
			return err
		}
		if _, err := loadOriginalRevision(tx, domain, rep.Original.SourceRevisionID); err != nil {
			return err
		}
		value.ActiveRepresentationID = head
		value.ActiveSourceRevisionID = &rep.Original.SourceRevisionID
		value.Currentness = "current"
		if rep.Original.SourceRevisionID != data.head.ID || rep.Original.SHA256 != data.head.SHA256 {
			value.Reasons = append(value.Reasons, "source_changed")
		}
		if rep.SourceSnapshot.LifecycleVersion != data.state.Version {
			value.Reasons = append(value.Reasons, "lifecycle_changed")
		}
		classification := entity.KnowledgeDirectoryClassification{GroupID: data.head.GroupID, ComponentSide: data.head.ComponentSide, Language: data.head.Language}
		a, _ := structureJSON(classification)
		b, _ := structureJSON(rep.SourceSnapshot.Classification)
		if a != b {
			value.Reasons = append(value.Reasons, "classification_changed")
		}
		if rep.SourceSnapshot.RelativePath != data.head.RelativePath {
			value.Reasons = append(value.Reasons, "position_changed")
		}
		if data.state.State != "active" {
			value.Reasons = append(value.Reasons, "source_archived")
		}
		if rep.FoundationPolicyVersion != kb.KnowledgePolicyVersion || rep.DocumentPolicyVersion != policy.Version {
			value.Reasons = append(value.Reasons, "policy_changed")
		}
		profile, err := documentProfile(rep.ProfileID)
		if err != nil || profile.ProducerSHA256 != rep.ProducerSHA256 {
			value.Reasons = append(value.Reasons, "producer_changed")
		}
		if len(value.Reasons) > 0 {
			value.Currentness = "stale"
		}
		return nil
	})
	return value, err
}

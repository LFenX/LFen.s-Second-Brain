package dataset

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

// persistKnowledgeParsed runs inside the caller's owned publication transaction.
// The task checkpoint supplies current permission/source/policy/lease fences.
// This storage primitive neither admits a task nor updates the R2 head.
func persistKnowledgeParsed(tx *gorm.DB, owner, domain, task string, bound *parsedBound, now time.Time) error {
	if bound == nil || bound.Representation.Source.DomainID != domain || !isKnowledgeID(owner, 32) || !isKnowledgeID(task, 32) || entity.KnowledgeParsedValidateBound(bound.Representation, bound.ReadingText, bound.Nodes, bound.Manifest) != nil {
		return contentError(409, "content_integrity")
	}
	rep := bound.Representation
	payload, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(bound.Manifest)
	if err != nil {
		return err
	}
	expected := entity.KnowledgeDocumentRepresentationRecord{ID: *rep.Ref.ID, Protocol: entity.KnowledgeParsedProtocol, OwnerID: owner, DomainID: domain, SourceID: rep.Source.SourceID, SourceRevisionID: rep.Source.RevisionID, TaskID: task, Payload: string(payload), ReadingText: bound.ReadingText, Manifest: string(manifest), PublishedAt: now}
	var prior entity.KnowledgeDocumentRepresentationRecord
	err = tx.Where("id = ?", expected.ID).Take(&prior).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	reuse := err == nil
	if reuse {
		if prior.Protocol != expected.Protocol || prior.OwnerID != owner || prior.DomainID != domain || prior.SourceID != expected.SourceID || prior.SourceRevisionID != expected.SourceRevisionID || prior.Payload != expected.Payload || prior.ReadingText != expected.ReadingText || prior.Manifest != expected.Manifest {
			return contentError(409, "content_integrity")
		}
	} else if err := tx.Create(&expected).Error; err != nil {
		return err
	}
	rows := make([]entity.KnowledgeDocumentNodeRecord, 0, len(bound.Nodes))
	for _, node := range bound.Nodes {
		raw, err := json.Marshal(node)
		if err != nil {
			return err
		}
		parent := ""
		if node.Payload.ParentLocalIndex != nil {
			parent = bound.Nodes[*node.Payload.ParentLocalIndex].NodeID
		}
		row := entity.KnowledgeDocumentNodeRecord{ID: node.NodeID, OwnerID: owner, DomainID: domain, RepresentationID: expected.ID, ParentID: parent, Ordinal: node.Payload.Ordinal, Payload: string(raw)}
		if reuse {
			var old entity.KnowledgeDocumentNodeRecord
			if err := tx.Where("id = ?", row.ID).Take(&old).Error; err != nil {
				return contentError(409, "content_integrity")
			}
			if old.OwnerID != row.OwnerID || old.DomainID != row.DomainID || old.RepresentationID != row.RepresentationID || old.ParentID != row.ParentID || old.Ordinal != row.Ordinal || old.Payload != row.Payload {
				return contentError(409, "content_integrity")
			}
		} else {
			rows = append(rows, row)
		}
	}
	if reuse {
		var count int64
		if err := tx.Model(&entity.KnowledgeDocumentNodeRecord{}).Where("representation_id = ?", expected.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(bound.Nodes)) {
			return contentError(409, "content_integrity")
		}
		return nil
	}
	return tx.CreateInBatches(rows, 128).Error
}

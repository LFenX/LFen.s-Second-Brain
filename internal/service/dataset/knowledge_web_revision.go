package dataset

import (
	"context"
	"encoding/json"
	"time"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

type webRevisionData struct {
	view          entity.KnowledgeWebRevision
	entry         entity.KnowledgeWebEntry
	entryRecord   entity.KnowledgeWebEntryRecord
	observation   entity.KnowledgeWebObservationRecord
	capture       entity.KnowledgeWebCapture
	captureRecord entity.KnowledgeWebCaptureRecord
}

func loadWebRevision(tx *gorm.DB, user, domain, id string) (*webRevisionData, error) {
	if !isKnowledgeID(id, 32) {
		return nil, webError(404, "read", "knowledge_not_found")
	}
	var source entity.KnowledgeSourceRevision
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").Where("knowledge_source_revision.id = ? AND knowledge_source_revision.domain_id = ? AND knowledge_source_revision.representation_kind = ? AND document.status = ?", id, domain, "web_capture", "1").Take(&source).Error; err != nil {
		return nil, err
	}
	regRow, reg, err := loadWebRegistration(tx, user, domain, source.OriginalRegistrationID, "")
	if err != nil {
		return nil, err
	}
	var fixed entity.KnowledgeWebRevisionRecord
	if err := tx.Where("id = ? AND registration_id = ?", id, reg.RegistrationID).Take(&fixed).Error; err != nil {
		return nil, err
	}
	var view entity.KnowledgeWebRevision
	if json.Unmarshal([]byte(fixed.Payload), &view) != nil || view.DomainID != domain || view.SourceRevisionID != id || view.SourceID != source.DocumentID || view.RegistrationID != reg.RegistrationID || view.EntryKey != fixed.EntryKey || view.CaptureID != fixed.CaptureID || view.SourceInputSHA256 != source.InputHash || view.RepresentationKind != "web_capture" || view.RootNodeID != nil || len(view.Representations) != 2 || view.Representations[0].SHA256 != source.SHA256 || view.Representations[0].ByteLength != source.ByteLength || source.FolderID != regRow.FolderID {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	if err := webClassification(tx, domain, view.Classification); err != nil {
		return nil, err
	}
	data := &webRevisionData{view: view}
	if err := tx.Where("id = ? AND entry_key = ?", fixed.EntryRecordID, view.EntryKey).Take(&data.entryRecord).Error; err != nil {
		return nil, err
	}
	entry, err := readWebEntry(&data.entryRecord, view.CaptureID, view.ObservationID, view.EntryKey)
	if err != nil {
		return nil, err
	}
	data.entry = *entry
	if directoryJSON(entry.Metadata) != directoryJSON(view.Metadata) || entry.ReferencesSHA256 != view.ReferencesSHA256 || directoryJSON(entry.Representations) != directoryJSON(view.Representations) {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	if err := tx.Where("id = ? AND capture_id = ?", view.ObservationID, view.CaptureID).Take(&data.observation).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("id = ? AND domain_id = ? AND owner_id = ? AND registration_id = ?", view.CaptureID, domain, user, reg.RegistrationID).Take(&data.captureRecord).Error; err != nil {
		return nil, err
	}
	capture, err := readWebCapture(&data.captureRecord)
	if err != nil {
		return nil, err
	}
	if capture.RawSHA256 == nil || capture.RawByteLength == nil || *capture.RawSHA256 != view.CaptureSHA256 || data.captureRecord.ObjectFolderID != regRow.FolderID {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	data.capture = *capture
	data.view.IsCurrent = source.IsCurrent
	return data, nil
}

func (d *DatasetService) webFixedData(ctx context.Context, user, domain, id string) (*webRevisionData, error) {
	var data *webRevisionData
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		var err error
		data, err = loadWebRevision(tx, user, domain, id)
		return err
	})
	return data, err
}

// GetKnowledgeWebRevision reads a fixed native revision independently of
// Capture/observation TTL and the source's current head.
func (d *DatasetService) GetKnowledgeWebRevision(ctx context.Context, user, domain, id string) (*entity.KnowledgeWebRevision, error) {
	data, err := d.webFixedData(ctx, user, domain, id)
	if err != nil {
		return nil, err
	}
	return &data.view, nil
}

// ListKnowledgeWebRevisions pages live current heads; it is not a frozen view of
// the complete catalogue. Fixed links use GetKnowledgeWebRevision instead.
func (d *DatasetService) ListKnowledgeWebRevisions(ctx context.Context, user, domain, registration string, group *string, cursor string) (*entity.KnowledgeWebRevisionPage, error) {
	var result *entity.KnowledgeWebRevisionPage
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		if registration != "" {
			if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
				return err
			}
		}
		if group != nil {
			if err := webClassification(tx, domain, entity.KnowledgeDirectoryClassification{GroupID: group, ComponentSide: "non_code", Language: ""}); err != nil {
				return err
			}
		}
		scope := entity.KnowledgeID("web_revisions", registration, directoryJSON(group))
		position, err := webCursorPosition(tx, user, domain, scope, cursor, now)
		if err != nil {
			return err
		}
		if position.LastID != "" && !isKnowledgeID(position.LastID, 32) {
			return webError(400, "read", "knowledge_invalid_request")
		}
		query := tx.Model(&entity.KnowledgeSourceRevision{}).Select("knowledge_source_revision.id").Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").Where("knowledge_source_revision.domain_id = ? AND knowledge_source_revision.representation_kind = ? AND knowledge_source_revision.is_current = ? AND document.status = ?", domain, "web_capture", true, "1")
		if registration != "" {
			query = query.Where("knowledge_source_revision.original_registration_id = ?", registration)
		}
		if group != nil {
			query = query.Where("knowledge_source_revision.group_id = ?", *group)
		}
		if position.LastID != "" {
			query = query.Where("knowledge_source_revision.id > ?", position.LastID)
		}
		var ids []string
		if err := query.Order("knowledge_source_revision.id ASC").Limit(21).Pluck("knowledge_source_revision.id", &ids).Error; err != nil {
			return err
		}
		result = &entity.KnowledgeWebRevisionPage{Items: []entity.KnowledgeWebRevision{}}
		for _, id := range ids {
			if len(result.Items) == 20 {
				break
			}
			data, err := loadWebRevision(tx, user, domain, id)
			if err != nil {
				return err
			}
			candidate := append(result.Items, data.view)
			if len(directoryJSON(entity.KnowledgeWebRevisionPage{Items: candidate, NextCursor: directoryString("00000000000000000000000000000000")})) > 31744 {
				break
			}
			result.Items = candidate
		}
		if len(result.Items) == 0 && len(ids) > 0 {
			return webError(413, "read", "web_budget_exceeded")
		}
		if len(result.Items) < len(ids) {
			result.NextCursor, err = webSaveCursor(tx, user, domain, registration, webCursor{Scope: scope, LastID: result.Items[len(result.Items)-1].SourceRevisionID}, now.Add(30*time.Minute), now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

// ListKnowledgeWebRevisionReferences pages the fixed revision's references.
func (d *DatasetService) ListKnowledgeWebRevisionReferences(ctx context.Context, user, domain, id, cursor string) (*entity.KnowledgeWebReferencePage, error) {
	var result *entity.KnowledgeWebReferencePage
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		data, err := loadWebRevision(tx, user, domain, id)
		if err != nil {
			return err
		}
		refs, err := webEntryReferences(&data.entryRecord, &data.entry)
		if err != nil {
			return err
		}
		page, err := webArrayPage(tx, user, domain, data.view.RegistrationID, entity.KnowledgeID("web_revision_refs", id, data.view.ReferencesSHA256), cursor, refs, now.Add(30*time.Minute), now)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeWebReferencePage{Items: page.Items, NextCursor: page.NextCursor}
		return nil
	})
	return result, err
}

// ReadKnowledgeWebRevisionContent validates bytes for the requested fixed ID,
// then rechecks current authorization before returning any content.
func (d *DatasetService) ReadKnowledgeWebRevisionContent(ctx context.Context, user, domain, id, kind string) (*KnowledgeWebContent, error) {
	data, err := d.webFixedData(ctx, user, domain, id)
	if err != nil {
		return nil, err
	}
	rep, err := webRepresentation(&data.entry, kind)
	if err != nil {
		return nil, err
	}
	raw, err := webReadPacked(ctx, &data.entryRecord, &data.observation, rep, 0, rep.ByteLength)
	if err != nil {
		return nil, err
	}
	if _, err := d.webFixedData(ctx, user, domain, id); err != nil {
		return nil, err
	}
	filename := "decoded.txt"
	if kind == "safe_reading" {
		filename = "safe-reading.html"
	}
	return &KnowledgeWebContent{Raw: raw, SHA256: rep.SHA256, Filename: filename}, nil
}

// ReadKnowledgeWebRevisionWindow reads an aligned interval of a fixed revision.
func (d *DatasetService) ReadKnowledgeWebRevisionWindow(ctx context.Context, user, domain, id, kind string, start, end int64) (*entity.KnowledgeWebWindow, error) {
	data, err := d.webFixedData(ctx, user, domain, id)
	if err != nil {
		return nil, err
	}
	rep, err := webRepresentation(&data.entry, kind)
	if err != nil {
		return nil, err
	}
	if start < 0 || end < start || end > rep.ByteLength || end-start > 4096 {
		return nil, webError(416, "read", "web_window_invalid")
	}
	raw, err := webReadPacked(ctx, &data.entryRecord, &data.observation, rep, start, end)
	if err != nil {
		return nil, err
	}
	if _, err := d.webFixedData(ctx, user, domain, id); err != nil {
		return nil, err
	}
	view := &entity.KnowledgeWebWindow{DomainID: domain, CaptureID: data.view.CaptureID, ObservationID: data.view.ObservationID, EntryKey: data.view.EntryKey, SourceRevisionID: &id, Representation: *rep, StartByte: start, EndByte: end, Text: string(raw)}
	if end < rep.ByteLength {
		view.NextStartByte = &end
	}
	return view, nil
}

// DownloadKnowledgeWebRevisionCapture authorizes the whole original Capture as
// owner-only evidence. It does not infer a future entry-scoped MCP permission.
func (d *DatasetService) DownloadKnowledgeWebRevisionCapture(ctx context.Context, user, domain, id string) (*KnowledgeWebContent, error) {
	data, err := d.webFixedData(ctx, user, domain, id)
	if err != nil {
		return nil, err
	}
	raw, err := webReadObject(ctx, data.captureRecord.ObjectFolderID, *data.capture.RawSHA256, *data.capture.RawByteLength)
	if err != nil {
		return nil, err
	}
	if _, err := d.webFixedData(ctx, user, domain, id); err != nil {
		return nil, err
	}
	return &KnowledgeWebContent{Raw: raw, SHA256: *data.capture.RawSHA256, Filename: "capture-snapshot.bin"}, nil
}

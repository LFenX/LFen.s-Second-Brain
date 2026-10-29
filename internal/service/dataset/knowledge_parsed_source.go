package dataset

import (
	"context"
	"errors"
	"path"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

func parsedSourceRow(tx *gorm.DB, kb *entity.Knowledgebase, ref entity.KnowledgeContentSourceRef) (*entity.KnowledgeSourceRevision, error) {
	if ref.Validate(kb.ID) != nil {
		return nil, documentError(400, "invalid_request")
	}
	var source entity.KnowledgeSourceRevision
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").Where("knowledge_source_revision.id = ? AND knowledge_source_revision.domain_id = ? AND knowledge_source_revision.document_id = ? AND document.status = ?", ref.RevisionID, kb.ID, ref.SourceID, "1").Take(&source).Error; err != nil {
		return nil, err
	}
	if source.SHA256 != ref.SHA256 || source.ByteLength < 0 || source.ByteLength > entity.KnowledgeMaxSourceBytes {
		return nil, documentError(409, "content_integrity")
	}
	if source.RepresentationKind == "original_only" {
		var reg entity.KnowledgeSourceRegistration
		var binding entity.KnowledgeSourceBinding
		if err := tx.Where("id = ? AND domain_id = ? AND source_kind = ? AND input_mode = ?", source.OriginalRegistrationID, kb.ID, "parsed_file", "native_snapshot").Take(&reg).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("document_id = ? AND registration_id = ? AND file_id = ? AND relative_path = ?", source.DocumentID, reg.ID, source.FileID, source.RelativePath).Take(&binding).Error; err != nil {
			return nil, documentError(409, "content_integrity")
		}
	} else if source.RepresentationKind != "text" {
		return nil, documentError(415, "unsupported_source")
	}
	var native entity.File
	if kb.KnowledgeFolderID == nil || source.FolderID != *kb.KnowledgeFolderID {
		return nil, documentError(409, "content_integrity")
	}
	if err := tx.Where("id = ? AND parent_id = ? AND tenant_id = ? AND source_type = ?", source.FileID, source.FolderID, kb.TenantID, "knowledgebase").Take(&native).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func parsedSourceInput(registration, sha, filename, relative string, classification entity.KnowledgeDirectoryClassification) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-native-source/0.2.0", "registration_id": registration, "sha256": sha, "filename": filename, "relative_path": relative, "classification": classification})
}

func parsedNativeFolder(tx *gorm.DB, kb *entity.Knowledgebase) error {
	if kb.KnowledgeFolderID == nil {
		return documentError(409, "source_changed")
	}
	var folder entity.File
	if err := tx.Where("id = ? AND tenant_id = ? AND type = ? AND source_type = ?", *kb.KnowledgeFolderID, kb.TenantID, "folder", "knowledgebase").Take(&folder).Error; err != nil {
		return err
	}
	var shared int64
	if err := tx.Model(&entity.Knowledgebase{}).Where("id <> ? AND knowledge_folder_id = ?", kb.ID, folder.ID).Count(&shared).Error; err != nil {
		return err
	}
	if shared != 0 {
		return documentError(409, "source_changed")
	}
	return nil
}

func (d *DatasetService) parsedAuthorizedSource(ctx context.Context, owner, domain string, ref entity.KnowledgeContentSourceRef) (*entity.KnowledgeSourceRevision, error) {
	kb, err := d.knowledgeBase(ctx, owner, domain, true)
	if err != nil {
		return nil, err
	}
	return parsedSourceRow(dao.DB.WithContext(ctx), kb, ref)
}

func (d *DatasetService) GetKnowledgeParsedSource(ctx context.Context, owner, domain, sourceID string) (*entity.KnowledgeParsedSourceState, error) {
	if !isKnowledgeID(sourceID, 32) {
		return nil, documentError(404, "not_found")
	}
	var value *entity.KnowledgeParsedSourceState
	err := d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, _ time.Time) error {
		data, err := loadLifecycleSource(tx, kb, sourceID)
		if err != nil {
			return err
		}
		source, err := parsedSourceRow(tx, kb, contentSource(data.head))
		if err != nil {
			return err
		}
		filename := source.OriginalFilename
		if source.RepresentationKind == "text" {
			filename = path.Base(source.RelativePath)
		}
		profile, err := parsedLocalProfile(string(utility.GetFileType(filename)))
		if err != nil {
			return err
		}
		value = &entity.KnowledgeParsedSourceState{Source: contentSource(source), NativeFileID: source.FileID, RepresentationKind: source.RepresentationKind, OriginalFilename: filename, RelativePath: source.RelativePath, Classification: entity.KnowledgeDirectoryClassification{GroupID: source.GroupID, ComponentSide: source.ComponentSide, Language: source.Language}, ByteLength: source.ByteLength, LifecycleVersion: data.state.Version, State: data.state.State, FoundationPolicyVersion: kb.KnowledgePolicyVersion, Profile: profile}
		return nil
	})
	return value, err
}

func parsedOriginalBytes(ctx context.Context, source *entity.KnowledgeSourceRevision) ([]byte, error) {
	var commit entity.FileCommit
	var item entity.FileCommitItem
	if err := dao.DB.WithContext(ctx).Where("id = ? AND folder_id = ?", source.CommitID, source.FolderID).Take(&commit).Error; err != nil {
		return nil, err
	}
	if err := dao.DB.WithContext(ctx).Where("id = ? AND commit_id = ? AND file_id = ?", source.ID, source.CommitID, source.FileID).Take(&item).Error; err != nil {
		return nil, err
	}
	if item.NewHash == nil || *item.NewHash != source.SHA256 || item.NewLocation == nil || *item.NewLocation != ".objects/"+source.SHA256 {
		return nil, documentError(409, "original_integrity")
	}
	content, err := file.NewFileCommitService().GetCommitFileContent(ctx, source.FolderID, source.CommitID, source.FileID)
	if err != nil {
		return nil, documentError(503, "original_unavailable")
	}
	if int64(len(content)) != source.ByteLength || directorySHA(content) != source.SHA256 {
		return nil, documentError(409, "original_integrity")
	}
	return content, nil
}

func parsedNativeSource(tx *gorm.DB, kb *entity.Knowledgebase, id string) (*entity.Document, *entity.File, error) {
	var doc entity.Document
	var native entity.File
	if err := parsedNativeFolder(tx, kb); err != nil {
		return nil, nil, err
	}
	if err := tx.Where("id = ? AND kb_id = ? AND status = ?", id, kb.ID, "1").Take(&doc).Error; err != nil {
		return nil, nil, err
	}
	if err := tx.Model(&entity.File{}).Joins("JOIN file2document ON file2document.file_id = file.id").Where("file2document.document_id = ? AND file.tenant_id = ? AND file.parent_id = ? AND file.source_type = ? AND file.type NOT IN ?", id, kb.TenantID, *kb.KnowledgeFolderID, "knowledgebase", []string{"folder", "virtual"}).Order("file.id ASC").Take(&native).Error; err != nil {
		return nil, nil, err
	}
	if doc.Name == nil || doc.Location == nil || native.Location == nil || *doc.Name != native.Name || !isKnowledgeImportPath(native.Name) || path.Base(native.Name) != native.Name || *doc.Location != *native.Location || doc.Size != native.Size || doc.Size < 0 || doc.Size > entity.KnowledgeMaxSourceBytes || !parser.KnowledgeParsedLocalFormat(string(utility.GetFileType(native.Name))) {
		return nil, nil, documentError(415, "unsupported_source")
	}
	return &doc, &native, nil
}

// SnapshotKnowledgeParsedSource attaches already uploaded native bytes through
// the existing FileCommit chain. It creates no C1 root and starts no parser.
func (d *DatasetService) SnapshotKnowledgeParsedSource(ctx context.Context, owner, domain string, req entity.KnowledgeParsedSnapshotRequest) (*entity.KnowledgeSourceRevision, error) {
	if !isKnowledgeID(req.DocumentID, 32) || req.ExpectedRevisionID != "" && !isKnowledgeID(req.ExpectedRevisionID, 32) || !structureVersion(req.ExpectedPolicyVersion) || !isKnowledgeImportPath(req.RelativePath) {
		return nil, documentError(400, "invalid_request")
	}
	var observedDoc *entity.Document
	var observedFile *entity.File
	var fence *entity.KnowledgeSourceFence
	var result *entity.KnowledgeSourceRevision
	err := d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, _ time.Time) error {
		if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion {
			return documentError(409, "policy_changed")
		}
		if err := knowledgeImportClassification(tx, domain, req.Classification.GroupID, req.Classification.ComponentSide, req.Classification.Language); err != nil {
			return documentError(400, "invalid_request")
		}
		var err error
		observedDoc, observedFile, err = parsedNativeSource(tx, kb, req.DocumentID)
		if err != nil {
			return err
		}
		if utility.GetFileType(req.RelativePath) != utility.GetFileType(observedFile.Name) {
			return documentError(400, "invalid_request")
		}
		current, err := lifecycleHead(tx, domain, req.DocumentID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		currentID := ""
		if current != nil {
			currentID = current.ID
		}
		if currentID != req.ExpectedRevisionID {
			return documentError(409, "source_changed")
		}
		source := ""
		if current != nil {
			source = req.DocumentID
		}
		fence, err = lifecycleObserveSource(tx, domain, "", req.RelativePath, source)
		return err
	})
	if err != nil {
		return nil, err
	}
	store := storage.GetStorageFactory().GetStorage()
	if store == nil {
		return nil, documentError(503, "original_unavailable")
	}
	content, err := store.Get(ctx, domain, *observedDoc.Location)
	if err != nil {
		return nil, documentError(503, "original_unavailable")
	}
	if int64(len(content)) != observedDoc.Size || int64(len(content)) > entity.KnowledgeMaxSourceBytes {
		return nil, documentError(409, "original_integrity")
	}
	err = d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion {
			return documentError(409, "policy_changed")
		}
		doc, native, err := parsedNativeSource(tx, kb, req.DocumentID)
		if err != nil {
			return err
		}
		if doc.Size != observedDoc.Size || *doc.Location != *observedDoc.Location || *doc.Name != *observedDoc.Name || native.ID != observedFile.ID {
			return documentError(409, "source_changed")
		}
		current, err := lifecycleHead(tx, domain, req.DocumentID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		source := ""
		if current != nil {
			source = req.DocumentID
		}
		if err := lifecycleVerifyFence(tx, fence, source); err != nil {
			return documentError(409, "source_changed")
		}
		if current != nil && (current.ID != req.ExpectedRevisionID || current.RepresentationKind != "original_only") {
			return documentError(409, "source_changed")
		}
		key := "parsed_" + doc.ID
		var reg entity.KnowledgeSourceRegistration
		err = tx.Where("domain_id = ? AND registration_key = ?", domain, key).Take(&reg).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			reg = entity.KnowledgeSourceRegistration{ID: common.GenerateUUID(), DomainID: domain, RegistrationKey: key, Name: *doc.Name, SourceKind: "parsed_file", InputMode: "native_snapshot", IncludePaths: []string{req.RelativePath}, GroupID: req.Classification.GroupID, ComponentSide: req.Classification.ComponentSide, Language: req.Classification.Language, ConfigHash: directorySHA([]byte(key)), CreatedAt: now}
			if err := tx.Create(&reg).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if reg.SourceKind != "parsed_file" || reg.InputMode != "native_snapshot" || len(reg.IncludePaths) != 1 || reg.IncludePaths[0] != req.RelativePath {
			return documentError(409, "source_changed")
		}
		if current != nil && (current.OriginalRegistrationID != reg.ID || current.RelativePath != req.RelativePath || !sameOptionalID(current.GroupID, req.Classification.GroupID) || current.ComponentSide != req.Classification.ComponentSide || current.Language != req.Classification.Language) {
			return documentError(409, "source_changed")
		}
		hash := directorySHA(content)
		input, err := parsedSourceInput(reg.ID, hash, *doc.Name, req.RelativePath, req.Classification)
		if err != nil {
			return err
		}
		if err := lifecyclePublishState(tx, domain, doc.ID, current == nil, current == nil || current.InputHash != input); err != nil {
			return err
		}
		if current != nil && current.InputHash == input {
			result = current
			return nil
		}
		var previous entity.KnowledgeSourceRevision
		priorErr := tx.Where("domain_id = ? AND document_id = ? AND input_hash = ?", domain, doc.ID, input).Take(&previous).Error
		if priorErr != nil && !errors.Is(priorErr, gorm.ErrRecordNotFound) {
			return priorErr
		}
		if priorErr == nil {
			if previous.RepresentationKind != "original_only" || previous.OriginalRegistrationID != reg.ID || previous.FileID != native.ID || previous.SHA256 != hash || previous.ByteLength != int64(len(content)) {
				return documentError(409, "content_integrity")
			}
			if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND is_current = ?", domain, doc.ID, true).Update("is_current", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&previous).Where("id = ?", previous.ID).Update("is_current", true).Error; err != nil {
				return err
			}
			previous.IsCurrent, result = true, &previous
			return nil
		}
		commit, item, err := file.NewFileCommitService().RecordSourceSnapshot(ctx, tx, native, owner, content)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeSourceRevision{ID: item.ID, DomainID: domain, DocumentID: doc.ID, FileID: native.ID, CommitID: commit.ID, FolderID: native.ParentID, GroupID: req.Classification.GroupID, RelativePath: req.RelativePath, ComponentSide: req.Classification.ComponentSide, Language: req.Classification.Language, SHA256: hash, InputHash: input, ByteLength: int64(len(content)), IsCurrent: true, RepresentationKind: "original_only", OriginalFilename: path.Base(*doc.Name), OriginalMediaType: "application/octet-stream", OriginalRegistrationID: reg.ID}
		if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND is_current = ?", domain, doc.ID, true).Update("is_current", false).Error; err != nil {
			return err
		}
		if err := tx.Create(result).Error; err != nil {
			return err
		}
		if current == nil {
			return tx.Create(&entity.KnowledgeSourceBinding{ID: common.GenerateUUID(), RegistrationID: reg.ID, RelativePath: req.RelativePath, DocumentID: doc.ID, FileID: native.ID}).Error
		}
		return nil
	})
	return result, err
}

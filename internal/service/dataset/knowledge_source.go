package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"
	"ragflow/internal/utility"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const knowledgeMaxSourceBytes int64 = entity.KnowledgeMaxSourceBytes
const knowledgeMaxContentBytes int64 = 64 * 1024

func knowledgeRoot(source *entity.KnowledgeSourceRevision) entity.KnowledgeNode {
	logical := entity.KnowledgeID("knowledge-logical/1", source.DomainID, source.DocumentID, "file", ".")
	return entity.KnowledgeNode{ID: entity.KnowledgeID("knowledge-node/1", logical, source.ID, "source-v1"), LogicalID: logical,
		DomainID: source.DomainID, SourceRevisionID: source.ID, Kind: "file", LogicalKey: ".", StructureVersion: "source-v1",
		Name: path.Base(source.RelativePath), StartByte: 0, EndByte: source.ByteLength, StartLine: 1, EndLine: source.LineCount}
}

func knowledgeRelativePath(value string) bool {
	if !validKnowledgeText(value, 512) || utf8.RuneCountInString(path.Base(value)) > 256 || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\:") || path.Clean(value) != value || value == "." {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}
	return true
}

// SnapshotKnowledgeSource attaches an already-uploaded native UTF-8 document.
// It never starts parsing, embedding or knowledge compilation.
func (d *DatasetService) SnapshotKnowledgeSource(ctx context.Context, userID, domainID string, req entity.SnapshotKnowledgeSourceRequest) (*entity.KnowledgeSnapshot, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	if !isKnowledgeID(req.DocumentID, 32) || (req.ExpectedRevisionID != "" && !isKnowledgeID(req.ExpectedRevisionID, 32)) || (req.GroupID != nil && !isKnowledgeID(*req.GroupID, 32)) {
		return nil, knowledgeInvalid("Source IDs must be 32 lowercase hexadecimal characters")
	}
	var document entity.Document
	if err := dao.DB.WithContext(ctx).Where("id = ? AND kb_id = ? AND status = ?", req.DocumentID, kb.ID, string(entity.StatusValid)).Take(&document).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, knowledgeNotFound()
		}
		return nil, err
	}
	var originalCount int64
	if err := dao.DB.WithContext(ctx).Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND representation_kind IN ?", kb.ID, document.ID, []string{"original_only", "web_capture"}).Count(&originalCount).Error; err != nil {
		return nil, err
	}
	if originalCount > 0 {
		return nil, knowledgeError(415, "knowledge_source_unsupported", "This source has no C1 text representation")
	}
	if document.Location == nil || document.Name == nil || document.Size < 0 || document.Size > knowledgeMaxSourceBytes {
		return nil, knowledgeInvalid("Source must have readable content of at most 2 MiB")
	}
	if req.RelativePath == "" {
		req.RelativePath = *document.Name
	}
	if !knowledgeRelativePath(req.RelativePath) {
		return nil, knowledgeInvalid("relative_path must be a normalized relative POSIX path with a basename of at most 256 characters")
	}
	if req.ComponentSide == "" {
		req.ComponentSide = "non_code"
	}
	if !validKnowledgeSide(req.ComponentSide) || utf8.RuneCountInString(req.Language) > 32 {
		return nil, knowledgeInvalid("Invalid source classification")
	}
	req.Language = strings.TrimSpace(req.Language)
	if req.Language != "" && !validKnowledgeText(req.Language, 32) {
		return nil, knowledgeInvalid("Invalid language")
	}
	if req.GroupID != nil {
		if _, err := d.knowledgeGroup(ctx, kb.ID, *req.GroupID); err != nil {
			return nil, err
		}
	}
	var nativeFile entity.File
	if err := dao.DB.WithContext(ctx).Model(&entity.File{}).
		Joins("JOIN file2document ON file2document.file_id = file.id").
		Where("file2document.document_id = ? AND file.tenant_id = ? AND file.type NOT IN ?", document.ID, kb.TenantID, []string{"folder", "virtual"}).
		Order("file.id ASC").Take(&nativeFile).Error; err != nil {
		return nil, knowledgeDBError(err, "source file lookup")
	}
	var folder entity.File
	if err := dao.DB.WithContext(ctx).Where("id = ? AND tenant_id = ? AND type = ?", nativeFile.ParentID, kb.TenantID, "folder").Take(&folder).Error; err != nil {
		return nil, knowledgeDBError(err, "source folder lookup")
	}
	if kb.KnowledgeFolderID == nil || folder.SourceType != string(entity.FileSourceKnowledgebase) {
		return nil, knowledgeConflict("Source file is outside the registered native folder")
	}
	var otherDomain int64
	if err := dao.DB.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id <> ? AND knowledge_folder_id = ?", kb.ID, folder.ID).Count(&otherDomain).Error; err != nil {
		return nil, knowledgeDBError(err, "source folder ownership lookup")
	}
	if otherDomain > 0 {
		return nil, knowledgeConflict("Source file belongs to another native domain folder")
	}
	store := storage.GetStorageFactory().GetStorage()
	if store == nil {
		return nil, knowledgeError(503, "knowledge_storage_unavailable", "Source storage unavailable")
	}
	content, err := store.Get(ctx, kb.ID, *document.Location)
	if err != nil {
		return nil, knowledgeError(503, "knowledge_storage_unavailable", "Source could not be read")
	}
	if int64(len(content)) > knowledgeMaxSourceBytes || !utility.IsKnowledgeTextSource(req.RelativePath, content) || !utility.IsKnowledgeTextSource(*document.Name, content) {
		return nil, knowledgeError(415, "knowledge_source_unsupported", "Foundation v1 supports UTF-8 text/code sources only")
	}
	var result entity.KnowledgeSnapshot
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND knowledge_enabled = ? AND created_by = ? AND status = ?", kb.ID, true, userID, string(entity.StatusValid)).Take(&locked).Error; err != nil {
			return knowledgeDBError(err, "domain lock")
		}
		return publishKnowledgeSource(ctx, tx, kb, &document, &nativeFile, folder.ID, userID, content, req, &result)
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func knowledgeSourceInputHash(hash string, req entity.SnapshotKnowledgeSourceRequest) string {
	groupID := ""
	if req.GroupID != nil {
		groupID = *req.GroupID
	}
	return entity.KnowledgeID("knowledge-source/1", hash, req.RelativePath, groupID, req.ComponentSide, req.Language)
}

// publishKnowledgeSource is the shared C1/C2 publication path. The caller holds the domain lock.
func publishKnowledgeSource(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase, document *entity.Document, nativeFile *entity.File, folderID, userID string, content []byte, req entity.SnapshotKnowledgeSourceRequest, result *entity.KnowledgeSnapshot) error {
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	inputHash := knowledgeSourceInputHash(hash, req)
	var current entity.KnowledgeSourceRevision
	currentErr := tx.Where("domain_id = ? AND document_id = ? AND is_current = ?", kb.ID, document.ID, true).Take(&current).Error
	if currentErr != nil && !errors.Is(currentErr, gorm.ErrRecordNotFound) {
		return currentErr
	}
	if currentErr == nil && (current.RepresentationKind == "original_only" || current.RepresentationKind == "web_capture") {
		return knowledgeError(415, "knowledge_source_unsupported", "Original-only sources have no C1 text representation")
	}
	if currentErr == nil && current.InputHash == inputHash {
		current.RootNodeID = knowledgeRoot(&current).ID
		result.Source, result.Root = current, knowledgeRoot(&current)
		return nil
	}
	if current.ID != req.ExpectedRevisionID {
		return knowledgeConflict("expected_revision_id does not match the current source revision")
	}
	var source entity.KnowledgeSourceRevision
	err := tx.Where("domain_id = ? AND document_id = ? AND input_hash = ?", kb.ID, document.ID, inputHash).Take(&source).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		commit, item, err := file.NewFileCommitService().RecordSourceSnapshot(ctx, tx, nativeFile, userID, content)
		if err != nil {
			return err
		}
		source = entity.KnowledgeSourceRevision{ID: item.ID, DomainID: kb.ID, DocumentID: document.ID, FileID: nativeFile.ID,
			CommitID: commit.ID, FolderID: folderID, GroupID: req.GroupID, RelativePath: req.RelativePath, ComponentSide: req.ComponentSide,
			Language: req.Language, SHA256: hash, InputHash: inputHash, ByteLength: int64(len(content)), LineCount: knowledgeLastLine(content), IsCurrent: true}
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		root := knowledgeRoot(&source)
		if err := tx.Create(&root).Error; err != nil {
			return err
		}
		result.Created = true
	}
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND id <> ?", kb.ID, document.ID, source.ID).Update("is_current", false).Error; err != nil {
		return err
	}
	if !source.IsCurrent {
		if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("id = ?", source.ID).Update("is_current", true).Error; err != nil {
			return err
		}
		source.IsCurrent = true
	}
	source.RootNodeID = knowledgeRoot(&source).ID
	result.Source, result.Root = source, knowledgeRoot(&source)
	return nil
}

func (d *DatasetService) knowledgeSource(ctx context.Context, domainID, revisionID string) (*entity.KnowledgeSourceRevision, error) {
	var source entity.KnowledgeSourceRevision
	err := dao.DB.WithContext(ctx).Model(&entity.KnowledgeSourceRevision{}).
		Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").
		Where("knowledge_source_revision.id = ? AND knowledge_source_revision.domain_id = ? AND document.status = ?", revisionID, domainID, string(entity.StatusValid)).
		Take(&source).Error
	if err != nil {
		return nil, knowledgeDBError(err, "source lookup")
	}
	if source.RepresentationKind == "original_only" || source.RepresentationKind == "web_capture" {
		return nil, knowledgeError(415, "knowledge_source_unsupported", "This source has no C1 text representation")
	}
	source.RootNodeID = knowledgeRoot(&source).ID
	return &source, nil
}

func (d *DatasetService) ListKnowledgeSources(ctx context.Context, userID, domainID, groupID, cursor string, limit int) (*entity.KnowledgePage[entity.KnowledgeSourceRevision], error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	// A subquery keeps cursor/order columns unambiguous in the outer query.
	validDocuments := dao.DB.WithContext(ctx).Model(&entity.Document{}).Select("id").Where("kb_id = ? AND status = ?", kb.ID, string(entity.StatusValid))
	query := dao.DB.WithContext(ctx).Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND is_current = ? AND representation_kind = ? AND document_id IN (?)", kb.ID, true, "text", validDocuments)
	if groupID != "" {
		if _, err := d.knowledgeGroup(ctx, kb.ID, groupID); err != nil {
			return nil, err
		}
		query = query.Where("group_id = ?", groupID)
	}
	scope := knowledgeCursor{DomainID: kb.ID, Kind: "sources", Parent: groupID}
	query, err = knowledgePageQuery(query, cursor, scope, limit)
	if err != nil {
		return nil, err
	}
	items := []entity.KnowledgeSourceRevision{}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	for i := range items {
		items[i].RootNodeID = knowledgeRoot(&items[i]).ID
	}
	page := &entity.KnowledgePage[entity.KnowledgeSourceRevision]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = knowledgeNextCursor(scope, items[limit-1].ID)
	}
	return page, nil
}

func (d *DatasetService) GetKnowledgeSource(ctx context.Context, userID, domainID, revisionID string) (*entity.KnowledgeSourceRevision, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	return d.knowledgeSource(ctx, kb.ID, revisionID)
}

func knowledgeLastLine(content []byte) int64 {
	if len(content) == 0 {
		return 1
	}
	return int64(bytes.Count(content[:len(content)-1], []byte{'\n'})) + 1
}

func (d *DatasetService) knowledgeNode(ctx context.Context, domainID, nodeID string) (*entity.KnowledgeNode, *entity.KnowledgeSourceRevision, error) {
	var node entity.KnowledgeNode
	if err := dao.DB.WithContext(ctx).Where("id = ? AND domain_id = ?", nodeID, domainID).Take(&node).Error; err != nil {
		return nil, nil, knowledgeDBError(err, "node lookup")
	}
	source, err := d.knowledgeSource(ctx, domainID, node.SourceRevisionID)
	if err != nil {
		return nil, nil, err
	}
	return &node, source, nil
}

func knowledgeSourceBytes(ctx context.Context, source *entity.KnowledgeSourceRevision) ([]byte, error) {
	if source.RepresentationKind == "original_only" || source.RepresentationKind == "web_capture" {
		return nil, knowledgeError(415, "knowledge_source_unsupported", "This source has no C1 text representation")
	}
	content, err := file.NewFileCommitService().GetCommitFileContent(ctx, source.FolderID, source.CommitID, source.FileID)
	if err != nil {
		return nil, knowledgeError(503, "knowledge_storage_unavailable", "Source revision could not be read")
	}
	digest := sha256.Sum256(content)
	if int64(len(content)) != source.ByteLength || hex.EncodeToString(digest[:]) != source.SHA256 {
		return nil, knowledgeError(409, "knowledge_source_integrity", "Source revision integrity check failed")
	}
	return content, nil
}

func knowledgeRange(content []byte, start, end int64) bool {
	if start < 0 || end < start || end > int64(len(content)) {
		return false
	}
	return (start == int64(len(content)) || utf8.RuneStart(content[start])) && (end == int64(len(content)) || utf8.RuneStart(content[end]))
}

func (d *DatasetService) CreateKnowledgeNode(ctx context.Context, userID, domainID string, req entity.CreateKnowledgeNodeRequest) (*entity.KnowledgeNode, bool, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, false, err
	}
	if !isKnowledgeID(req.SourceRevisionID, 32) || !isKnowledgeID(req.ParentID, 64) {
		return nil, false, knowledgeInvalid("Node references must use lowercase hexadecimal source and parent IDs")
	}
	parent, source, err := d.knowledgeNode(ctx, kb.ID, req.ParentID)
	if err != nil {
		return nil, false, err
	}
	if req.SourceRevisionID != source.ID {
		return nil, false, knowledgeNotFound()
	}
	if !source.IsCurrent {
		return nil, false, knowledgeConflict("Historical source structures are read-only")
	}
	switch req.Kind {
	case "section", "symbol", "passage":
	default:
		return nil, false, knowledgeInvalid("Unsupported manual node kind")
	}
	if !validKnowledgeText(req.LogicalKey, 256) || !validKnowledgeText(strings.TrimSpace(req.Name), 256) {
		return nil, false, knowledgeInvalid("Invalid node name or logical_key")
	}
	if req.StartByte < parent.StartByte || req.EndByte > parent.EndByte {
		return nil, false, knowledgeInvalid("Node range exceeds its parent")
	}
	ancestor := parent
	depth := 2
	for ancestor.ParentID != nil {
		depth++
		if depth > 16 {
			return nil, false, knowledgeInvalid("Node depth exceeds 16")
		}
		ancestor, _, err = d.knowledgeNode(ctx, kb.ID, *ancestor.ParentID)
		if err != nil {
			return nil, false, err
		}
	}
	content, err := knowledgeSourceBytes(ctx, source)
	if err != nil {
		return nil, false, err
	}
	if !knowledgeRange(content, req.StartByte, req.EndByte) {
		return nil, false, knowledgeInvalid("Byte range must follow UTF-8 boundaries")
	}
	logical := entity.KnowledgeID("knowledge-logical/1", kb.ID, source.DocumentID, req.Kind, req.LogicalKey)
	id := entity.KnowledgeID("knowledge-node/1", logical, source.ID, "manual-v1")
	startLine := int64(bytes.Count(content[:req.StartByte], []byte{'\n'})) + 1
	endLine := startLine
	if req.EndByte > req.StartByte {
		endLine = int64(bytes.Count(content[:req.EndByte-1], []byte{'\n'})) + 1
	}
	node := &entity.KnowledgeNode{ID: id, LogicalID: logical, DomainID: kb.ID, SourceRevisionID: source.ID, ParentID: &parent.ID,
		Kind: req.Kind, LogicalKey: req.LogicalKey, StructureVersion: "manual-v1", Name: strings.TrimSpace(req.Name),
		StartByte: req.StartByte, EndByte: req.EndByte, StartLine: startLine, EndLine: endLine}
	var existing entity.KnowledgeNode
	created := false
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND created_by = ? AND knowledge_enabled = ? AND status = ?", kb.ID, userID, true, string(entity.StatusValid)).Take(&locked).Error; err != nil {
			return knowledgeDBError(err, "domain lock")
		}
		var current entity.KnowledgeSourceRevision
		if err := tx.Where("id = ? AND domain_id = ? AND is_current = ?", source.ID, kb.ID, true).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return knowledgeConflict("Historical source structures are read-only")
			}
			return err
		}
		err := tx.Where("id = ? AND domain_id = ?", id, kb.ID).Take(&existing).Error
		if err == nil {
			if existing.ParentID == nil || *existing.ParentID != parent.ID || existing.Name != node.Name || existing.StartByte != node.StartByte || existing.EndByte != node.EndByte {
				return knowledgeConflict("Node identity already has immutable content")
			}
			node = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(node).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return node, created, nil
}

func (d *DatasetService) GetKnowledgeNode(ctx context.Context, userID, domainID, nodeID string) (*entity.KnowledgeNode, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	node, _, err := d.knowledgeNode(ctx, kb.ID, nodeID)
	return node, err
}

func (d *DatasetService) ListKnowledgeNodes(ctx context.Context, userID, domainID, revisionID, parentID, cursor string, limit int) (*entity.KnowledgePage[entity.KnowledgeNode], error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	if _, err := d.knowledgeSource(ctx, kb.ID, revisionID); err != nil {
		return nil, err
	}
	query := dao.DB.WithContext(ctx).Model(&entity.KnowledgeNode{}).Where("domain_id = ? AND source_revision_id = ?", kb.ID, revisionID)
	if parentID == "" {
		query = query.Where("parent_id IS NULL")
	} else {
		parent, _, err := d.knowledgeNode(ctx, kb.ID, parentID)
		if err != nil {
			return nil, err
		}
		if parent.SourceRevisionID != revisionID {
			return nil, knowledgeNotFound()
		}
		query = query.Where("parent_id = ?", parentID)
	}
	scope := knowledgeCursor{DomainID: kb.ID, Kind: "nodes", Parent: parentID, Revision: revisionID}
	query, err = knowledgePageQuery(query, cursor, scope, limit)
	if err != nil {
		return nil, err
	}
	items := []entity.KnowledgeNode{}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	page := &entity.KnowledgePage[entity.KnowledgeNode]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = knowledgeNextCursor(scope, items[limit-1].ID)
	}
	return page, nil
}

func (d *DatasetService) ReadKnowledgeNode(ctx context.Context, userID, domainID, nodeID string, start, end *int64) (*entity.KnowledgeNodeContent, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	node, source, err := d.knowledgeNode(ctx, kb.ID, nodeID)
	if err != nil {
		return nil, err
	}
	from, to := node.StartByte, node.EndByte
	if start != nil {
		from = *start
	}
	if end != nil {
		to = *end
	}
	if from < node.StartByte || to > node.EndByte || to < from {
		return nil, knowledgeInvalid("Requested range exceeds this node")
	}
	if to-from > knowledgeMaxContentBytes {
		return nil, knowledgeError(413, "knowledge_range_too_large", "Read at most 65536 bytes per request")
	}
	content, err := knowledgeSourceBytes(ctx, source)
	if err != nil {
		return nil, err
	}
	if !knowledgeRange(content, from, to) {
		return nil, knowledgeInvalid("Byte range must follow UTF-8 boundaries")
	}
	return &entity.KnowledgeNodeContent{NodeID: node.ID, SourceID: source.DocumentID, SourceRevisionID: source.ID,
		SHA256: source.SHA256, StartByte: from, EndByte: to, Text: string(content[from:to]), Encoding: "utf-8"}, nil
}

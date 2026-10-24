package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"

	"github.com/zeebo/xxh3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var knowledgeImportKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func isKnowledgeImportPath(value string) bool {
	if !knowledgeRelativePath(value) {
		return false
	}
	for _, part := range strings.Split(strings.ToLower(value), "/") {
		if strings.HasPrefix(part, ".env") || slices.Contains([]string{".git", ".svn", "node_modules", "vendor", "dist", "build", ".local-data", ".ssh", ".aws", "credentials", "id_rsa", "id_ed25519"}, part) {
			return false
		}
	}
	return !slices.Contains([]string{".pem", ".key", ".p12", ".pfx"}, strings.ToLower(path.Ext(value)))
}

func lockKnowledgeImportDomain(ctx context.Context, tx *gorm.DB, userID, domainID string) (*entity.Knowledgebase, error) {
	var kb entity.Knowledgebase
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND created_by = ? AND knowledge_enabled = ? AND permission = ? AND status = ?", domainID, userID, true, "me", "1").Take(&kb).Error; err != nil {
		return nil, knowledgeDBError(err, "import domain")
	}
	var count int64
	if err := tx.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ? AND status = ?", userID, kb.TenantID, "1").Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 || kb.KnowledgeFolderID == nil {
		return nil, knowledgeNotFound()
	}
	return &kb, nil
}

func knowledgeImportClassification(tx *gorm.DB, domainID string, groupID *string, side, language string) error {
	if !validKnowledgeSide(side) || (language != "" && !validKnowledgeText(language, 32)) || language != strings.TrimSpace(language) {
		return knowledgeInvalid("Invalid source classification")
	}
	if groupID != nil {
		if !isKnowledgeID(*groupID, 32) {
			return knowledgeInvalid("Invalid group ID")
		}
		var group entity.KnowledgeGroup
		if err := tx.Where("id = ? AND domain_id = ?", *groupID, domainID).Take(&group).Error; err != nil {
			return knowledgeDBError(err, "import group")
		}
	}
	return nil
}

func importRegistration(tx *gorm.DB, domainID, id string) (*entity.KnowledgeSourceRegistration, error) {
	if !isKnowledgeID(id, 32) {
		return nil, knowledgeNotFound()
	}
	var reg entity.KnowledgeSourceRegistration
	if err := tx.Where("id = ? AND domain_id = ?", id, domainID).Take(&reg).Error; err != nil {
		return nil, knowledgeDBError(err, "source registration")
	}
	return &reg, nil
}

func (d *DatasetService) CreateKnowledgeRegistration(ctx context.Context, userID, domainID string, req entity.CreateKnowledgeRegistrationRequest) (*entity.KnowledgeSourceRegistration, bool, error) {
	if _, err := d.knowledgeBase(ctx, userID, domainID, true); err != nil {
		return nil, false, err
	}
	if !knowledgeImportKey.MatchString(req.RegistrationKey) || len(req.RegistrationKey) > 64 || !validKnowledgeText(req.Name, 128) || req.SourceKind != "text_file" || req.InputMode != "inline_text" || len(req.IncludePaths) < 1 || len(req.IncludePaths) > 16 {
		return nil, false, knowledgeInvalid("Only an explicit manual text scope is supported")
	}
	paths := slices.Clone(req.IncludePaths)
	slices.Sort(paths)
	for i, p := range paths {
		if !isKnowledgeImportPath(p) || (i > 0 && p == paths[i-1]) {
			return nil, false, knowledgeInvalid("Invalid or excluded source path")
		}
	}
	req.IncludePaths = paths
	encoded, _ := json.Marshal(req)
	digest := sha256.Sum256(encoded)
	configHash := hex.EncodeToString(digest[:])
	var result entity.KnowledgeSourceRegistration
	created := false
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockKnowledgeImportDomain(ctx, tx, userID, domainID); err != nil {
			return err
		}
		if err := knowledgeImportClassification(tx, domainID, req.GroupID, req.ComponentSide, req.Language); err != nil {
			return err
		}
		err := tx.Where("domain_id = ? AND registration_key = ?", domainID, req.RegistrationKey).Take(&result).Error
		if err == nil {
			if result.ConfigHash != configHash {
				return knowledgeConflict("Registration key already has different configuration")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result = entity.KnowledgeSourceRegistration{ID: common.GenerateUUID(), DomainID: domainID, RegistrationKey: req.RegistrationKey, Name: req.Name, SourceKind: req.SourceKind, InputMode: req.InputMode, IncludePaths: paths, GroupID: req.GroupID, ComponentSide: req.ComponentSide, Language: req.Language, ConfigHash: configHash, CreatedAt: time.Now().UTC()}
		created = true
		return tx.Create(&result).Error
	})
	return &result, created, err
}

func (d *DatasetService) GetKnowledgeRegistration(ctx context.Context, userID, domainID, id string) (*entity.KnowledgeSourceRegistration, error) {
	if _, err := d.knowledgeBase(ctx, userID, domainID, true); err != nil {
		return nil, err
	}
	return importRegistration(dao.DB.WithContext(ctx), domainID, id)
}

func knowledgeImportManifest(preview *entity.KnowledgeImportPreview) string {
	group := ""
	if preview.GroupID != nil {
		group = *preview.GroupID
	}
	return entity.KnowledgeID("knowledge-import-manifest/0.2.0", preview.RegistrationID, preview.RelativePath, preview.SHA256, group, preview.ComponentSide, preview.Language, preview.ExpectedRevisionID, strconv.FormatInt(preview.PolicyVersion, 10))
}

func knowledgePreviewView(p *entity.KnowledgeImportPreview, domainID string) *entity.KnowledgeImportPreviewView {
	var expected *string
	if p.ExpectedRevisionID != "" {
		v := p.ExpectedRevisionID
		expected = &v
	}
	return &entity.KnowledgeImportPreviewView{ID: p.ID, RegistrationID: p.RegistrationID, DomainID: domainID, PolicyVersion: p.PolicyVersion, ManifestSHA256: p.ManifestSHA256, ExpiresAt: p.ExpiresAt, ConsumedTaskID: p.ConsumedTaskID, Items: []entity.KnowledgeImportCandidate{{ItemKey: p.RelativePath, RelativePath: p.RelativePath, GroupID: p.GroupID, ComponentSide: p.ComponentSide, Language: p.Language, Action: p.Action, SourceID: p.DocumentID, ExpectedRevisionID: expected, SHA256: p.SHA256, ByteLength: p.ByteLength}}}
}

func (d *DatasetService) PreviewKnowledgeImport(ctx context.Context, userID, domainID, registrationID string, req entity.PreviewKnowledgeImportRequest) (*entity.KnowledgeImportPreviewView, error) {
	if _, err := d.knowledgeBase(ctx, userID, domainID, true); err != nil {
		return nil, err
	}
	if !isKnowledgeImportPath(req.RelativePath) {
		return nil, knowledgeInvalid("Invalid or excluded source path")
	}
	content := []byte(req.Text)
	if int64(len(content)) > entity.KnowledgeMaxSourceBytes {
		return nil, knowledgeError(413, "knowledge_source_too_large", "Source exceeds 2 MiB")
	}
	if !utility.IsKnowledgeTextSource(req.RelativePath, content) || strings.Contains(req.Text, "-----BEGIN PRIVATE KEY-----") || strings.Contains(req.Text, "-----BEGIN RSA PRIVATE KEY-----") {
		return nil, knowledgeError(415, "knowledge_source_unsupported", "Only non-secret UTF-8 text/code is supported")
	}
	digest := sha256.Sum256(content)
	p := entity.KnowledgeImportPreview{ID: common.GenerateUUID(), RegistrationID: registrationID, RelativePath: req.RelativePath, GroupID: req.GroupID, ComponentSide: req.ComponentSide, Language: req.Language, Content: req.Text, SHA256: hex.EncodeToString(digest[:]), ByteLength: int64(len(content)), Action: "add", ExpiresAt: time.Now().UTC().Add(entity.KnowledgeImportPreviewTTL)}
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(ctx, tx, userID, domainID)
		if err != nil {
			return err
		}
		reg, err := importRegistration(tx, domainID, registrationID)
		if err != nil {
			return err
		}
		if !slices.Contains(reg.IncludePaths, req.RelativePath) {
			return knowledgeInvalid("Path is outside the registered selection")
		}
		if err := knowledgeImportClassification(tx, domainID, req.GroupID, req.ComponentSide, req.Language); err != nil {
			return err
		}
		p.PolicyVersion = kb.KnowledgePolicyVersion
		var binding entity.KnowledgeSourceBinding
		err = tx.Where("registration_id = ? AND relative_path = ?", registrationID, req.RelativePath).Take(&binding).Error
		if err == nil {
			current, err := knowledgeImportCurrent(tx, domainID, binding.DocumentID)
			if err != nil {
				return err
			}
			p.DocumentID = &binding.DocumentID
			p.ExpectedRevisionID = current.ID
			p.Action = "update"
			if current.InputHash == knowledgeSourceInputHash(p.SHA256, entity.SnapshotKnowledgeSourceRequest{GroupID: p.GroupID, RelativePath: p.RelativePath, ComponentSide: p.ComponentSide, Language: p.Language}) {
				p.Action = "unchanged"
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		p.ManifestSHA256 = knowledgeImportManifest(&p)
		return tx.Create(&p).Error
	})
	if err != nil {
		return nil, err
	}
	return knowledgePreviewView(&p, domainID), nil
}

func knowledgeImportCurrent(tx *gorm.DB, domainID, documentID string) (*entity.KnowledgeSourceRevision, error) {
	var doc entity.Document
	if err := tx.Where("id = ? AND kb_id = ? AND status = ?", documentID, domainID, "1").Take(&doc).Error; err != nil {
		return nil, knowledgeDBError(err, "import document")
	}
	var revision entity.KnowledgeSourceRevision
	if err := tx.Where("domain_id = ? AND document_id = ? AND is_current = ?", domainID, documentID, true).Take(&revision).Error; err != nil {
		return nil, knowledgeDBError(err, "import current revision")
	}
	return &revision, nil
}

func importPreview(tx *gorm.DB, registrationID, previewID string) (*entity.KnowledgeImportPreview, error) {
	if !isKnowledgeID(previewID, 32) {
		return nil, knowledgeNotFound()
	}
	var p entity.KnowledgeImportPreview
	if err := tx.Where("id = ? AND registration_id = ?", previewID, registrationID).Take(&p).Error; err != nil {
		return nil, knowledgeDBError(err, "import preview")
	}
	return &p, nil
}

func (d *DatasetService) GetKnowledgeImportPreview(ctx context.Context, userID, domainID, registrationID, previewID string) (*entity.KnowledgeImportPreviewView, error) {
	if _, err := d.GetKnowledgeRegistration(ctx, userID, domainID, registrationID); err != nil {
		return nil, err
	}
	p, err := importPreview(dao.DB.WithContext(ctx), registrationID, previewID)
	if err != nil {
		return nil, err
	}
	return knowledgePreviewView(p, domainID), nil
}

func (d *DatasetService) GetKnowledgeImport(ctx context.Context, userID, domainID, registrationID, taskID string) (*entity.KnowledgeImportReceipt, error) {
	if _, err := d.GetKnowledgeRegistration(ctx, userID, domainID, registrationID); err != nil {
		return nil, err
	}
	if !isKnowledgeID(taskID, 32) {
		return nil, knowledgeNotFound()
	}
	var task entity.KnowledgeImport
	if err := dao.DB.WithContext(ctx).Where("id = ? AND registration_id = ?", taskID, registrationID).Take(&task).Error; err != nil {
		return nil, knowledgeDBError(err, "import receipt")
	}
	var result entity.KnowledgeImportReceipt
	if err := json.Unmarshal([]byte(task.Receipt), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *DatasetService) CommitKnowledgeImport(ctx context.Context, userID, domainID, registrationID string, req entity.CommitKnowledgeImportRequest) (*entity.KnowledgeImportReceipt, bool, error) {
	if _, err := d.GetKnowledgeRegistration(ctx, userID, domainID, registrationID); err != nil {
		return nil, false, err
	}
	if !isKnowledgeID(req.PreviewID, 32) || !isKnowledgeID(req.ManifestSHA256, 64) || len(req.IdempotencyKey) < 1 || len(req.IdempotencyKey) > 128 || !knowledgeImportKey.MatchString(req.IdempotencyKey) {
		return nil, false, knowledgeInvalid("Invalid preview, manifest or idempotency key")
	}
	var result entity.KnowledgeImportReceipt
	created := false
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(ctx, tx, userID, domainID)
		if err != nil {
			return err
		}
		reg, err := importRegistration(tx, domainID, registrationID)
		if err != nil {
			return err
		}
		var task entity.KnowledgeImport
		err = tx.Where("registration_id = ? AND idempotency_key = ?", registrationID, req.IdempotencyKey).Take(&task).Error
		if err == nil {
			if task.PreviewID != req.PreviewID || task.ManifestSHA256 != req.ManifestSHA256 {
				return knowledgeConflict("Idempotency key is bound to another request")
			}
			return json.Unmarshal([]byte(task.Receipt), &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		p, err := importPreview(tx, registrationID, req.PreviewID)
		if err != nil {
			return err
		}
		if p.ConsumedTaskID != nil || !time.Now().Before(p.ExpiresAt) || p.PolicyVersion != kb.KnowledgePolicyVersion || p.ManifestSHA256 != req.ManifestSHA256 || knowledgeImportManifest(p) != p.ManifestSHA256 {
			return knowledgeConflict("Preview expired, consumed or no longer matches the current policy/manifest")
		}
		if !slices.Contains(reg.IncludePaths, p.RelativePath) {
			return knowledgeConflict("Selection changed")
		}
		if err := knowledgeImportClassification(tx, domainID, p.GroupID, p.ComponentSide, p.Language); err != nil {
			return err
		}
		var binding entity.KnowledgeSourceBinding
		err = tx.Where("registration_id = ? AND relative_path = ?", registrationID, p.RelativePath).Take(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if p.DocumentID != nil {
				return knowledgeConflict("Source mapping changed")
			}
			binding = entity.KnowledgeSourceBinding{ID: common.GenerateUUID(), RegistrationID: registrationID, RelativePath: p.RelativePath, DocumentID: common.GenerateUUID(), FileID: common.GenerateUUID()}
		} else if err != nil {
			return err
		} else {
			current, err := knowledgeImportCurrent(tx, domainID, binding.DocumentID)
			if err != nil {
				return err
			}
			if p.DocumentID == nil || *p.DocumentID != binding.DocumentID || current.ID != p.ExpectedRevisionID {
				return knowledgeConflict("Source revision changed after preview")
			}
		}
		result = entity.KnowledgeImportReceipt{ID: common.GenerateUUID(), RegistrationID: registrationID, DomainID: domainID, PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, Status: "succeeded", CreatedAt: time.Now().UTC()}
		item := entity.KnowledgeImportResult{ItemKey: p.RelativePath, RelativePath: p.RelativePath, GroupID: p.GroupID, ComponentSide: p.ComponentSide, Language: p.Language, SaveStatus: "saved", CatalogueStatus: "ready", StructureStatus: "deferred", FullTextStatus: "disabled", VectorStatus: "disabled", SHA256: p.SHA256, ByteLength: p.ByteLength, Code: "ok"}
		if err := tx.SavePoint("knowledge_import_publish").Error; err != nil {
			return err
		}
		snapshot, publishErr := publishKnowledgeImport(ctx, tx, kb, userID, &binding, p)
		if publishErr != nil {
			if err := tx.RollbackTo("knowledge_import_publish").Error; err != nil {
				return err
			}
			result.Status = "failed"
			item.SaveStatus = "failed"
			item.CatalogueStatus = "failed"
			item.Code = "knowledge_import_publish_failed"
		} else {
			item.SourceID = &snapshot.Source.DocumentID
			item.SourceRevisionID = &snapshot.Source.ID
			item.RootNodeID = &snapshot.Root.ID
			if p.Action == "unchanged" {
				item.SaveStatus = "unchanged"
			}
		}
		result.Items = []entity.KnowledgeImportResult{item}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		task = entity.KnowledgeImport{ID: result.ID, RegistrationID: registrationID, IdempotencyKey: req.IdempotencyKey, PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, Receipt: string(encoded), CreatedAt: result.CreatedAt}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		if err := tx.Model(p).Update("consumed_task_id", task.ID).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return &result, created, err
}

// publishKnowledgeImport stages immutable bytes, then publishes native rows and the shared C1 snapshot.
func publishKnowledgeImport(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase, userID string, binding *entity.KnowledgeSourceBinding, p *entity.KnowledgeImportPreview) (*entity.KnowledgeSnapshot, error) {
	store := storage.GetStorageFactory().GetStorage()
	if store == nil {
		return nil, errors.New("storage unavailable")
	}
	content := []byte(p.Content)
	digest := sha256.Sum256(content)
	if int64(len(content)) != p.ByteLength || hex.EncodeToString(digest[:]) != p.SHA256 || !utility.IsKnowledgeTextSource(p.RelativePath, content) {
		return nil, errors.New("staged content mismatch")
	}
	if p.Action == "unchanged" {
		current, err := knowledgeImportCurrent(tx, kb.ID, binding.DocumentID)
		if err != nil {
			return nil, err
		}
		root := knowledgeRoot(current)
		current.RootNodeID = root.ID
		return &entity.KnowledgeSnapshot{Source: *current, Root: root}, nil
	}
	var folder entity.File
	if err := tx.Where("id = ? AND tenant_id = ? AND type = ? AND source_type = ?", *kb.KnowledgeFolderID, kb.TenantID, "folder", "knowledgebase").Take(&folder).Error; err != nil {
		return nil, err
	}
	location := ".knowledge-import/" + binding.DocumentID + "/" + p.SHA256
	if err := store.Put(ctx, kb.ID, location, content); err != nil {
		return nil, err
	}
	name := path.Base(p.RelativePath)
	valid := "1"
	nativeDigest := xxh3.Hash128(content).Bytes()
	nativeHash := hex.EncodeToString(nativeDigest[:])
	var doc entity.Document
	var native entity.File
	if p.DocumentID == nil {
		doc = entity.Document{ID: binding.DocumentID, KbID: kb.ID, CreatedBy: userID, Name: &name, Location: &location, Size: p.ByteLength, ContentHash: &nativeHash, Status: &valid, ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: "txt", Suffix: strings.TrimPrefix(path.Ext(name), ".")}
		native = entity.File{ID: binding.FileID, ParentID: folder.ID, TenantID: kb.TenantID, CreatedBy: userID, Name: name, Location: &location, Size: p.ByteLength, Type: "txt", SourceType: "knowledgebase"}
		for _, row := range []any{&doc, &native, &entity.File2Document{ID: common.GenerateUUID(), FileID: &native.ID, DocumentID: &doc.ID}, binding} {
			if err := tx.Create(row).Error; err != nil {
				return nil, err
			}
		}
		if err := tx.Model(kb).Update("doc_num", gorm.Expr("doc_num + 1")).Error; err != nil {
			return nil, err
		}
	} else {
		if err := tx.Where("id = ? AND kb_id = ? AND status = ?", binding.DocumentID, kb.ID, "1").Take(&doc).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("id = ? AND parent_id = ? AND tenant_id = ?", binding.FileID, folder.ID, kb.TenantID).Take(&native).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&doc).Updates(map[string]any{"location": location, "size": p.ByteLength, "content_hash": nativeHash}).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&native).Updates(map[string]any{"location": location, "size": p.ByteLength}).Error; err != nil {
			return nil, err
		}
	}
	var snapshot entity.KnowledgeSnapshot
	req := entity.SnapshotKnowledgeSourceRequest{DocumentID: doc.ID, ExpectedRevisionID: p.ExpectedRevisionID, RelativePath: p.RelativePath, GroupID: p.GroupID, ComponentSide: p.ComponentSide, Language: p.Language}
	if err := publishKnowledgeSource(ctx, tx, kb, &doc, &native, folder.ID, userID, content, req, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

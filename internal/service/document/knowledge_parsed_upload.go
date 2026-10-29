package document

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"path"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

type KnowledgeParsedUploadError struct {
	Status int
	Code   string
}

func (e *KnowledgeParsedUploadError) Error() string { return e.Code }

type parsedUploadAuthority struct {
	actor, domain string
	request       entity.KnowledgeParsedUploadRequest
	failure       error
}

func (a *parsedUploadAuthority) reject(status int, code string) error {
	a.failure = &KnowledgeParsedUploadError{Status: status, Code: code}
	return a.failure
}

func (a *parsedUploadAuthority) verify(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase) error {
	if kb == nil || kb.ID != a.domain || kb.CreatedBy != a.actor || !kb.KnowledgeEnabled || kb.Permission != "me" || kb.Status == nil || *kb.Status != "1" || kb.KnowledgeFolderID == nil {
		return a.reject(404, "not_found")
	}
	var members int64
	if err := tx.WithContext(ctx).Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ? AND status = ?", a.actor, kb.TenantID, "1").Count(&members).Error; err != nil {
		return a.reject(503, "unavailable")
	}
	if members == 0 {
		return a.reject(404, "not_found")
	}
	if kb.KnowledgePolicyVersion != a.request.ExpectedPolicyVersion {
		return a.reject(409, "foundation_policy_changed")
	}
	var policy entity.KnowledgeContentPolicyRecord
	err := tx.WithContext(ctx).Where("domain_id = ?", kb.ID).Take(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return a.reject(409, "content_policy_disabled")
	}
	if err != nil {
		return a.reject(503, "unavailable")
	}
	if policy.OwnerID != a.actor || policy.Version < 1 || policy.Version > entity.KnowledgeMaxLifecycleVersion {
		return a.reject(409, "content_integrity")
	}
	if policy.Version != a.request.ExpectedContentPolicyVersion {
		return a.reject(409, "content_policy_changed")
	}
	if !policy.ParseEnabled {
		return a.reject(409, "content_policy_disabled")
	}
	return nil
}

func (a *parsedUploadAuthority) validateBlob(filename string, blob []byte) error {
	if !utf8.ValidString(filename) || filename == "" || len(filename) > 255 || path.Base(filename) != filename || strings.ContainsAny(filename, "\\\x00\r\n") || strings.HasPrefix(strings.ToLower(filename), ".env") || !parser.KnowledgeParsedLocalFormat(string(utility.GetFileType(filename))) {
		return a.reject(415, "unsupported_source")
	}
	if int64(len(blob)) > entity.KnowledgeMaxSourceBytes {
		return a.reject(413, "capacity_exceeded")
	}
	return nil
}

func (a *parsedUploadAuthority) readFile(header *multipart.FileHeader) ([]byte, error) {
	if header == nil || header.Size < 0 || header.Size > entity.KnowledgeMaxSourceBytes {
		return nil, a.reject(413, "capacity_exceeded")
	}
	if err := a.validateBlob(header.Filename, nil); err != nil {
		return nil, err
	}
	file, err := header.Open()
	if err != nil {
		return nil, a.reject(400, "invalid_request")
	}
	defer file.Close()
	blob, err := io.ReadAll(io.LimitReader(file, entity.KnowledgeMaxSourceBytes+1))
	if err != nil {
		return nil, a.reject(503, "unavailable")
	}
	if err := a.validateBlob(header.Filename, blob); err != nil {
		return nil, err
	}
	return blob, nil
}

// UploadKnowledgeParsedLocalDocument is the explicit unified-content ingress.
// It shares native storage, document counters, file linkage and rollback with
// ordinary upload, and checks its independent local permission under the same
// domain lock as the insert. It neither parses nor enqueues ingestion work.
func (s *DocumentService) UploadKnowledgeParsedLocalDocument(ctx context.Context, actor, domain string, req entity.KnowledgeParsedUploadRequest, header *multipart.FileHeader) (*entity.KnowledgeParsedUploadReceipt, error) {
	if !entity.ValidKnowledgeLexicalID(actor) || !entity.ValidKnowledgeLexicalID(domain) || !req.Confirmed || req.ExpectedPolicyVersion < 1 || req.ExpectedPolicyVersion > entity.KnowledgeMaxLifecycleVersion || req.ExpectedContentPolicyVersion < 1 || req.ExpectedContentPolicyVersion > entity.KnowledgeMaxLifecycleVersion || header == nil {
		return nil, &KnowledgeParsedUploadError{Status: 400, Code: "invalid_request"}
	}
	authority := &parsedUploadAuthority{actor: actor, domain: domain, request: req}
	rows, messages := s.uploadLocalDocuments(ctx, &entity.Knowledgebase{ID: domain}, actor, []*multipart.FileHeader{header}, "", nil, authority)
	if authority.failure != nil {
		return nil, authority.failure
	}
	if len(messages) != 0 || len(rows) != 1 {
		return nil, &KnowledgeParsedUploadError{Status: 503, Code: "unavailable"}
	}
	id, idOK := rows[0]["id"].(string)
	name, nameOK := rows[0]["name"].(string)
	size, sizeOK := rows[0]["size"].(int64)
	if !idOK || !nameOK || !sizeOK || !entity.ValidKnowledgeLexicalID(id) {
		return nil, &KnowledgeParsedUploadError{Status: 503, Code: "unavailable"}
	}
	var document entity.Document
	if err := dao.DB.WithContext(ctx).Where("id = ? AND kb_id = ? AND created_by = ?", id, domain, actor).Take(&document).Error; err != nil {
		return nil, &KnowledgeParsedUploadError{Status: 503, Code: "unavailable"}
	}
	return &entity.KnowledgeParsedUploadReceipt{SourceID: id, OriginalFilename: name, ByteLength: size}, nil
}

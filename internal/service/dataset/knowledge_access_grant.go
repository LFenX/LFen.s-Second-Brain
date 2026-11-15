package dataset

import (
	"bytes"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func accessGrantIntegrity(tx *gorm.DB, row entity.KnowledgeAccessGrantRecord) error {
	var definition entity.KnowledgeAccessCreateGrantRequest
	if len(row.DefinitionJSON) > 32768 || entity.KnowledgeAccessDecode([]byte(row.DefinitionJSON), "CreateGrantRequest", &definition) != nil {
		return accessError(409, "integrity_error")
	}
	expires, err := time.Parse(time.RFC3339Nano, definition.ExpiresAt)
	if err != nil || definition.DomainID != row.DomainID || definition.ModuleID != row.ModuleID || definition.ExpectedModuleVersion != row.ModuleVersion || definition.Subject.Kind != row.SubjectKind || definition.Subject.ID != row.SubjectID || definition.Mode != row.Mode || definition.Delivery != row.Delivery || !expires.UTC().Truncate(time.Millisecond).Equal(row.ExpiresAt) {
		return accessError(409, "integrity_error")
	}
	var receipt entity.KnowledgeAccessAuditRecord
	if err := accessLocked(tx, true).Where("id = ? AND tenant_id = ?", row.CreateCommandID, row.TenantID).Take(&receipt).Error; err != nil {
		return err
	}
	if receipt.ActorUserID == nil || *receipt.ActorUserID != row.IssuedBy || receipt.DomainID == nil || *receipt.DomainID != row.DomainID || receipt.ModuleID == nil || *receipt.ModuleID != row.ModuleID || receipt.CommandOperation == nil || *receipt.CommandOperation != "createGrant" || receipt.CommandKey == nil || *receipt.CommandKey != definition.IdempotencyKey || receipt.CommandResourceID == nil || *receipt.CommandResourceID != row.ID || receipt.CommandState == nil || *receipt.CommandState != "completed" || !bytes.Equal(receipt.CommandRequestSHA256, accessCommandDigest(row.IssuedBy, "createGrant", &definition)) {
		return accessError(409, "integrity_error")
	}
	return nil
}

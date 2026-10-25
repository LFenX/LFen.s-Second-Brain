package entity

import "time"

// KnowledgeOriginalUploadRecord keeps the immutable request and upload state.
// Payload contains only the bounded public metadata projection, never bytes.
type KnowledgeOriginalUploadRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	DomainID       string `gorm:"size:32;not null;index:idx_original_upload_scope"`
	OwnerID        string `gorm:"size:32;not null;index:idx_original_upload_scope"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_original_upload_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_original_upload_key"`
	RequestSHA256  string `gorm:"size:64;not null"`
	Payload        string `gorm:"type:text;not null"`
	State          string `gorm:"size:24;not null;index:idx_original_upload_scope"`
	DeclaredBytes  int64  `gorm:"not null"`
	Epoch          int64  `gorm:"not null;default:0"`
	LeaseUntil     *time.Time
	ObjectFolderID string `gorm:"size:32;not null;default:''"`
	ObjectLocation string `gorm:"size:128;not null;default:''"`
	NativeHash     string `gorm:"size:32;not null;default:''"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time `gorm:"not null;index"`
}

func (KnowledgeOriginalUploadRecord) TableName() string { return "knowledge_original_upload" }

// KnowledgeOriginalReceiveSlot bounds active transfer leases across API processes.
type KnowledgeOriginalReceiveSlot struct {
	ID         int    `gorm:"primaryKey;autoIncrement:false"`
	UploadID   string `gorm:"size:32;not null;default:''"`
	DomainID   string `gorm:"size:32;not null;default:''"`
	OwnerID    string `gorm:"size:32;not null;default:''"`
	Epoch      int64  `gorm:"not null;default:0"`
	LeaseUntil *time.Time
}

func (KnowledgeOriginalReceiveSlot) TableName() string { return "knowledge_original_receive_slot" }

// KnowledgeOriginalImportRecord preserves a fixed confirmation receipt.
type KnowledgeOriginalImportRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_original_import_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_original_import_key"`
	UploadID       string `gorm:"size:32;not null;uniqueIndex:idx_original_import_upload"`
	ManifestSHA256 string `gorm:"size:64;not null"`
	Receipt        string `gorm:"type:text;not null"`
	CreatedAt      time.Time
}

func (KnowledgeOriginalImportRecord) TableName() string { return "knowledge_original_import" }

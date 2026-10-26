package entity

import "time"

// KnowledgeWebRegistrationRecord extends the native source registration with
// immutable web configuration and its dedicated native folder.
type KnowledgeWebRegistrationRecord struct {
	ID        string `gorm:"primaryKey;size:32"`
	DomainID  string `gorm:"size:32;not null;index"`
	OwnerID   string `gorm:"size:32;not null"`
	FolderID  string `gorm:"size:32;not null"`
	Payload   string `gorm:"type:text;not null"`
	CreatedAt time.Time
}

func (KnowledgeWebRegistrationRecord) TableName() string { return "knowledge_web_registration" }

// KnowledgeWebCaptureRecord preserves request identity separately from mutable
// state. An online Attempted value is never reset, including after interruption.
type KnowledgeWebCaptureRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	DomainID       string `gorm:"size:32;not null;index:idx_web_capture_scope"`
	OwnerID        string `gorm:"size:32;not null;index:idx_web_capture_scope"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_web_capture_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_web_capture_key"`
	RequestSHA256  string `gorm:"size:64;not null"`
	Request        string `gorm:"type:text;not null"`
	Payload        string `gorm:"type:text;not null"`
	State          string `gorm:"size:24;not null;index:idx_web_capture_scope"`
	ReservedBytes  int64  `gorm:"not null"`
	Epoch          int64  `gorm:"not null;default:0"`
	Attempted      bool   `gorm:"not null;default:false"`
	LeaseUntil     *time.Time
	ObjectFolderID string `gorm:"size:32;not null;default:''"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time `gorm:"not null;index"`
}

func (KnowledgeWebCaptureRecord) TableName() string { return "knowledge_web_capture" }

// KnowledgeWebSlot bounds acquisition and parsing across API processes.
type KnowledgeWebSlot struct {
	ID         int    `gorm:"primaryKey;autoIncrement:false"`
	DomainID   string `gorm:"size:32;not null;default:''"`
	OwnerID    string `gorm:"size:32;not null;default:''"`
	CaptureID  string `gorm:"size:32;not null;default:''"`
	Operation  string `gorm:"size:16;not null;default:''"`
	Epoch      int64  `gorm:"not null;default:0"`
	LeaseUntil *time.Time
}

func (KnowledgeWebSlot) TableName() string { return "knowledge_web_slot" }

type KnowledgeWebObservationRecord struct {
	ID               string `gorm:"primaryKey;size:32"`
	CaptureID        string `gorm:"size:32;not null;uniqueIndex:idx_web_observation_capture"`
	Payload          string `gorm:"type:text;not null"`
	Epoch            int64  `gorm:"not null;default:0"`
	LeaseUntil       *time.Time
	ObjectFolderID   string `gorm:"size:32;not null;default:''"`
	ObjectSHA256     string `gorm:"size:64;not null;default:''"`
	ObjectByteLength int64  `gorm:"not null;default:0"`
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

func (KnowledgeWebObservationRecord) TableName() string { return "knowledge_web_observation" }

// KnowledgeWebEntryRecord stores bounded metadata and object locators, never an
// executable document. Source IDs in Payload are fixed at observation time.
type KnowledgeWebEntryRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	ObservationID  string `gorm:"size:32;not null;uniqueIndex:idx_web_entry_key"`
	EntryKey       string `gorm:"size:64;not null;uniqueIndex:idx_web_entry_key"`
	Payload        string `gorm:"type:text;not null"`
	References     string `gorm:"type:longtext;not null"`
	ObjectFolderID string `gorm:"size:32;not null"`
	NativeHash     string `gorm:"size:32;not null"`
	DecodedOffset  int64  `gorm:"not null"`
	ReadingOffset  int64  `gorm:"not null"`
}

func (KnowledgeWebEntryRecord) TableName() string { return "knowledge_web_entry" }

type KnowledgeWebPreviewRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_web_preview_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_web_preview_key"`
	CaptureID      string `gorm:"size:32;not null;index"`
	RequestSHA256  string `gorm:"size:64;not null"`
	Request        string `gorm:"type:text;not null"`
	Payload        string `gorm:"type:text;not null"`
	Items          string `gorm:"type:longtext;not null"`
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

func (KnowledgeWebPreviewRecord) TableName() string { return "knowledge_web_preview" }

type KnowledgeWebReceiptRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_web_receipt_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_web_receipt_key"`
	PreviewID      string `gorm:"size:32;not null;uniqueIndex:idx_web_receipt_preview"`
	RequestSHA256  string `gorm:"size:64;not null"`
	Payload        string `gorm:"type:text;not null"`
	Items          string `gorm:"type:longtext;not null"`
	CreatedAt      time.Time
}

func (KnowledgeWebReceiptRecord) TableName() string { return "knowledge_web_receipt" }

// KnowledgeWebRevisionRecord supplements the native KnowledgeSourceRevision,
// whose identity remains the FileCommitItem ID. The payload is immutable.
type KnowledgeWebRevisionRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;index"`
	CaptureID      string `gorm:"size:32;not null"`
	EntryKey       string `gorm:"size:64;not null;index"`
	EntryRecordID  string `gorm:"size:32;not null"`
	Payload        string `gorm:"type:text;not null"`
	CreatedAt      time.Time
}

func (KnowledgeWebRevisionRecord) TableName() string { return "knowledge_web_revision" }

// KnowledgeWebBinding fixes a feed identity to its native binding even when
// its decoded representation changes between HTML and plain text.
type KnowledgeWebBinding struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_web_binding_entry"`
	EntryKey       string `gorm:"size:64;not null;uniqueIndex:idx_web_binding_entry"`
}

func (KnowledgeWebBinding) TableName() string { return "knowledge_web_binding" }

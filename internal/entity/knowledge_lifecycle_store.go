package entity

import "time"

// KnowledgeSourceLifecycle owns state and a monotonic generation, never a head.
type KnowledgeSourceLifecycle struct {
	SourceID  string `gorm:"primaryKey;size:32"`
	DomainID  string `gorm:"size:32;not null;index:idx_lifecycle_scope"`
	State     string `gorm:"size:16;not null;index:idx_lifecycle_scope"`
	Version   int64  `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (KnowledgeSourceLifecycle) TableName() string { return "knowledge_source_lifecycle" }

// KnowledgeRetiredPosition reserves exact old positions independently of the
// single current native binding. Raw bytes are retained to detect digest collisions.
type KnowledgeRetiredPosition struct {
	ID             string `gorm:"primaryKey;size:64"`
	DomainID       string `gorm:"size:32;not null;index"`
	RegistrationID string `gorm:"size:32;not null"`
	RelativePath   string `gorm:"size:512;not null"`
	SourceID       string `gorm:"size:32;not null;index"`
	CreatedAt      time.Time
}

func (KnowledgeRetiredPosition) TableName() string { return "knowledge_retired_position" }

// KnowledgeSourceFence distinguishes a missing pre-deployment preparation from
// an explicitly observed empty position (SourceID empty, Version zero).
type KnowledgeSourceFence struct {
	ID             string `gorm:"primaryKey;size:64"`
	DomainID       string `gorm:"size:32;not null;index"`
	Kind           string `gorm:"size:32;not null"`
	PreparationID  string `gorm:"size:32;not null;index"`
	ItemKey        string `gorm:"size:512;not null"`
	RegistrationID string `gorm:"size:32;not null"`
	RelativePath   string `gorm:"size:512;not null"`
	SourceID       string `gorm:"size:32;not null"`
	Version        int64  `gorm:"not null"`
	CreatedAt      time.Time
}

func (KnowledgeSourceFence) TableName() string { return "knowledge_source_fence" }

type KnowledgeLifecycleCommandRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	KeySHA256      string `gorm:"size:64;not null;uniqueIndex:idx_lifecycle_command_key"`
	OwnerID        string `gorm:"size:32;not null;index"`
	DomainID       string `gorm:"size:32;not null;index"`
	SourceID       string `gorm:"size:32;not null"`
	Action         string `gorm:"size:16;not null"`
	IdempotencyKey string `gorm:"size:128;not null"`
	RequestSHA256  string `gorm:"size:64;not null"`
	Payload        string `gorm:"type:text;not null"`
	CreatedAt      time.Time
}

func (KnowledgeLifecycleCommandRecord) TableName() string { return "knowledge_lifecycle_command" }

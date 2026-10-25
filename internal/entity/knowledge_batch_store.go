package entity

import "time"

// KnowledgeBatchPreviewRecord owns the immutable selection, never the original bytes.
type KnowledgeBatchPreviewRecord struct {
	ID             string  `gorm:"primaryKey;size:32"`
	DomainID       string  `gorm:"size:32;not null;index:idx_knowledge_batch_preview_scope"`
	UserID         string  `gorm:"size:32;not null;index:idx_knowledge_batch_preview_scope"`
	RegistrationID string  `gorm:"size:32;not null"`
	Payload        string  `gorm:"type:longtext;not null"`
	ConsumedTaskID *string `gorm:"size:32"`
	CreatedAt      time.Time
	ExpiresAt      time.Time `gorm:"index"`
}

func (KnowledgeBatchPreviewRecord) TableName() string { return "knowledge_batch_preview" }

// KnowledgeBatchItemRecord keeps each bounded body separate from task metadata.
type KnowledgeBatchItemRecord struct {
	ID          string  `gorm:"primaryKey;size:32"`
	PreviewID   string  `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_item_order"`
	Ordinal     int64   `gorm:"not null;uniqueIndex:idx_knowledge_batch_item_order"`
	TaskID      *string `gorm:"size:32;index:idx_knowledge_batch_item_task"`
	Status      string  `gorm:"size:16;not null;index:idx_knowledge_batch_item_task"`
	PreviewJSON string  `gorm:"type:longtext;not null"`
	ResultJSON  string  `gorm:"type:longtext;not null"`
	Content     string  `gorm:"type:longtext;not null"`
	Identity    string  `gorm:"type:text;not null"`
}

func (KnowledgeBatchItemRecord) TableName() string { return "knowledge_batch_item" }

type KnowledgeBatchTaskRecord struct {
	ID             string     `gorm:"primaryKey;size:32"`
	DomainID       string     `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_task_key;index:idx_knowledge_batch_domain_lease"`
	UserID         string     `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_task_key"`
	RegistrationID string     `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_task_key"`
	IdempotencyKey string     `gorm:"size:128;not null;uniqueIndex:idx_knowledge_batch_task_key"`
	PreviewID      string     `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_task_preview"`
	RequestJSON    string     `gorm:"type:text;not null"`
	Payload        string     `gorm:"type:longtext;not null"`
	Status         string     `gorm:"size:20;not null;index:idx_knowledge_batch_domain_lease"`
	LeaseEpoch     int64      `gorm:"not null;default:0"`
	LeaseOwner     string     `gorm:"size:32;not null;default:''"`
	LeaseUntil     *time.Time `gorm:"index:idx_knowledge_batch_domain_lease"`
	CreatedAt      time.Time  `gorm:"index"`
	DeadlineAt     time.Time  `gorm:"index"`
}

func (KnowledgeBatchTaskRecord) TableName() string { return "knowledge_batch_task" }

// KnowledgeBatchViewRecord materializes a fixed task/history list or project preview.
type KnowledgeBatchViewRecord struct {
	ID         string  `gorm:"primaryKey;size:32"`
	DomainID   string  `gorm:"size:32;not null;index:idx_knowledge_batch_view_scope"`
	UserID     string  `gorm:"size:32;not null;index:idx_knowledge_batch_view_scope"`
	ScopeID    string  `gorm:"size:32;not null;index:idx_knowledge_batch_view_scope"`
	Kind       string  `gorm:"size:24;not null;index:idx_knowledge_batch_view_scope"`
	Payload    string  `gorm:"type:longtext;not null"`
	ConsumedID *string `gorm:"size:32"`
	CreatedAt  time.Time
	ExpiresAt  time.Time `gorm:"index"`
}

func (KnowledgeBatchViewRecord) TableName() string { return "knowledge_batch_view" }

type KnowledgeProjectManifestRecord struct {
	ID             string    `gorm:"primaryKey;size:32"`
	DomainID       string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_manifest_key;index:idx_knowledge_manifest_project"`
	UserID         string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_manifest_key"`
	ProjectID      string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_manifest_key;index:idx_knowledge_manifest_project"`
	IdempotencyKey string    `gorm:"size:128;not null;uniqueIndex:idx_knowledge_manifest_key"`
	PreviewID      string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_manifest_preview"`
	RequestJSON    string    `gorm:"type:text;not null"`
	Payload        string    `gorm:"type:longtext;not null"`
	Receipt        string    `gorm:"type:text;not null"`
	CreatedAt      time.Time `gorm:"index"`
}

func (KnowledgeProjectManifestRecord) TableName() string { return "knowledge_project_manifest" }

type KnowledgeProjectManifestHeadRecord struct {
	ProjectID  string `gorm:"primaryKey;size:32"`
	DomainID   string `gorm:"size:32;not null;index"`
	ManifestID string `gorm:"size:32;not null"`
}

func (KnowledgeProjectManifestHeadRecord) TableName() string {
	return "knowledge_project_manifest_head"
}

type KnowledgeBatchCommandRecord struct {
	ID             string `gorm:"primaryKey;size:32"`
	DomainID       string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_command_key"`
	UserID         string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_command_key"`
	ScopeID        string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_batch_command_key"`
	Kind           string `gorm:"size:16;not null;uniqueIndex:idx_knowledge_batch_command_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_knowledge_batch_command_key"`
	RequestJSON    string `gorm:"type:text;not null"`
	CreatedAt      time.Time
}

func (KnowledgeBatchCommandRecord) TableName() string { return "knowledge_batch_command" }

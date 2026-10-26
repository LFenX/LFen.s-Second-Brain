package entity

import "time"

// KnowledgeStructurePolicyRecord grants deterministic processing independently
// of the foundation policy; it never grants ingestion or model access.
type KnowledgeStructurePolicyRecord struct {
	DomainID  string `gorm:"primaryKey;size:32"`
	OwnerID   string `gorm:"size:32;not null"`
	Version   int64  `gorm:"not null"`
	Enabled   bool   `gorm:"not null"`
	UpdatedAt time.Time
}

func (KnowledgeStructurePolicyRecord) TableName() string { return "knowledge_structure_policy" }

// KnowledgeStructureOperationRecord stores one immutable admitted outcome,
// including semantic rejections, for each typed operation/key scope.
type KnowledgeStructureOperationRecord struct {
	ID            string `gorm:"primaryKey;size:64"`
	OwnerID       string `gorm:"size:32;not null"`
	DomainID      string `gorm:"size:32;not null;index"`
	Kind          string `gorm:"size:16;not null"`
	Key           string `gorm:"size:128;not null"`
	TaskID        string `gorm:"size:32;not null"`
	RequestSHA256 string `gorm:"size:64;not null"`
	Status        int    `gorm:"not null"`
	OutcomeKind   string `gorm:"size:16;not null"`
	Payload       string `gorm:"type:longtext;not null"`
	CreatedAt     time.Time
}

func (KnowledgeStructureOperationRecord) TableName() string { return "knowledge_structure_operation" }

type KnowledgeStructureTaskRecord struct {
	ID             string    `gorm:"primaryKey;size:32"`
	Protocol       string    `gorm:"size:40;not null;default:''"`
	TargetKind     string    `gorm:"size:32;not null;default:''"`
	TargetID       string    `gorm:"size:32;not null;default:''"`
	Execution      string    `gorm:"type:longtext"`
	OwnerID        string    `gorm:"size:32;not null;index:idx_structure_task_scope"`
	DomainID       string    `gorm:"size:32;not null;index:idx_structure_task_scope"`
	ProjectID      string    `gorm:"size:32;not null;index:idx_structure_task_scope"`
	RequestSHA256  string    `gorm:"size:64;not null"`
	ProducerSHA256 string    `gorm:"size:64;not null"`
	Revision       int64     `gorm:"not null"`
	AttemptCount   int64     `gorm:"not null"`
	State          string    `gorm:"size:24;not null;index:idx_structure_task_work"`
	Payload        string    `gorm:"type:longtext;not null"`
	Inputs         string    `gorm:"type:longtext;not null"`
	CreatedAt      time.Time `gorm:"index:idx_structure_task_scope;index:idx_structure_task_work"`
	UpdatedAt      time.Time
	DeadlineAt     time.Time
	CompletedAt    *time.Time
}

func (KnowledgeStructureTaskRecord) TableName() string { return "knowledge_structure_task" }

// KnowledgeStructureLeaseRecord fences execution attempts across API processes.
type KnowledgeStructureLeaseRecord struct {
	DomainID  string `gorm:"primaryKey;size:32"`
	TaskID    string `gorm:"size:32;not null"`
	RunnerID  string `gorm:"size:32;not null"`
	Epoch     int64  `gorm:"not null"`
	ExpiresAt time.Time
	UpdatedAt time.Time
}

func (KnowledgeStructureLeaseRecord) TableName() string { return "knowledge_structure_lease" }

type KnowledgeStructureViewRecord struct {
	ID          string `gorm:"primaryKey;size:64"`
	OwnerID     string `gorm:"size:32;not null"`
	DomainID    string `gorm:"size:32;not null;index"`
	ProjectID   string `gorm:"size:32;not null"`
	TaskID      string `gorm:"size:32;not null"`
	Published   bool   `gorm:"not null"`
	Payload     string `gorm:"type:longtext;not null"`
	CreatedAt   time.Time
	PublishedAt time.Time
}

func (KnowledgeStructureViewRecord) TableName() string { return "knowledge_structure_view" }

// KnowledgeStructureHeadRecord points to a published view, never a source head.
type KnowledgeStructureHeadRecord struct {
	ProjectID string `gorm:"primaryKey;size:32"`
	DomainID  string `gorm:"size:32;not null;index"`
	OwnerID   string `gorm:"size:32;not null"`
	ViewID    string `gorm:"size:64;not null"`
	UpdatedAt time.Time
}

func (KnowledgeStructureHeadRecord) TableName() string { return "knowledge_structure_head" }

type KnowledgeStructureNodeRecord struct {
	ID               string `gorm:"primaryKey;size:64"`
	OwnerID          string `gorm:"size:32;not null"`
	DomainID         string `gorm:"size:32;not null;index:idx_structure_node_page"`
	ViewID           string `gorm:"size:64;not null;index:idx_structure_node_page"`
	SourceID         string `gorm:"size:32;not null;index:idx_structure_node_page"`
	SourceRevisionID string `gorm:"size:32;not null"`
	ParentID         string `gorm:"size:64;not null;index:idx_structure_node_page"`
	Kind             string `gorm:"size:32;not null;index:idx_structure_node_page"`
	StartByte        int64  `gorm:"not null;index:idx_structure_node_page"`
	EndByte          int64  `gorm:"not null;index:idx_structure_node_page"`
	FragmentSHA256   string `gorm:"size:64;not null"`
	Payload          string `gorm:"type:longtext;not null"`
}

func (KnowledgeStructureNodeRecord) TableName() string { return "knowledge_structure_node" }

type KnowledgeStructureRelationRecord struct {
	ID         string `gorm:"primaryKey;size:64"`
	OwnerID    string `gorm:"size:32;not null"`
	DomainID   string `gorm:"size:32;not null;index:idx_structure_relation_from;index:idx_structure_relation_to"`
	ViewID     string `gorm:"size:64;not null;index:idx_structure_relation_from;index:idx_structure_relation_to"`
	FromNodeID string `gorm:"size:64;not null;index:idx_structure_relation_from"`
	ToNodeID   string `gorm:"size:64;not null;index:idx_structure_relation_to"`
	Kind       string `gorm:"size:32;not null"`
	Payload    string `gorm:"type:longtext;not null"`
}

func (KnowledgeStructureRelationRecord) TableName() string { return "knowledge_structure_relation" }

// KnowledgeStructureArtifactRecord contains positions/facts, never source bytes.
type KnowledgeStructureArtifactRecord struct {
	ID             string `gorm:"primaryKey;size:64"`
	OwnerID        string `gorm:"size:32;not null"`
	DomainID       string `gorm:"size:32;not null;index"`
	SourceID       string `gorm:"size:32;not null"`
	ContentSHA256  string `gorm:"size:64;not null"`
	ProducerSHA256 string `gorm:"size:64;not null"`
	Parser         string `gorm:"size:16;not null"`
	Payload        string `gorm:"type:longtext;not null"`
	CreatedAt      time.Time
}

func (KnowledgeStructureArtifactRecord) TableName() string { return "knowledge_structure_artifact" }

// KnowledgeStructureCursorRecord binds an opaque cursor to its first-page
// deadline and exact authorized scope. Continuations never renew its deadline.
type KnowledgeStructureCursorRecord struct {
	ID          string `gorm:"primaryKey;size:32"`
	OwnerID     string `gorm:"size:32;not null"`
	DomainID    string `gorm:"size:32;not null"`
	ScopeSHA256 string `gorm:"size:64;not null"`
	Payload     string `gorm:"type:text;not null"`
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

func (KnowledgeStructureCursorRecord) TableName() string { return "knowledge_structure_cursor" }

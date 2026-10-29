package entity

import "time"

// KnowledgeDocumentPolicyRecord grants only explicitly requested local document
// processing. It is independent of the code structure and model policies.
type KnowledgeDocumentPolicyRecord struct {
	DomainID  string `gorm:"primaryKey;size:32"`
	OwnerID   string `gorm:"size:32;not null"`
	Version   int64  `gorm:"not null"`
	Enabled   bool   `gorm:"not null"`
	UpdatedAt time.Time
}

func (KnowledgeDocumentPolicyRecord) TableName() string { return "knowledge_document_policy" }

type KnowledgeDocumentOperationRow struct {
	ID            string `gorm:"primaryKey;size:64"`
	OwnerID       string `gorm:"size:32;not null"`
	DomainID      string `gorm:"size:32;not null;index"`
	Kind          string `gorm:"size:16;not null"`
	Key           string `gorm:"size:128;not null"`
	TaskID        string `gorm:"size:32;not null"`
	RequestSHA256 string `gorm:"size:64;not null"`
	Status        int    `gorm:"not null"`
	OutcomeKind   string `gorm:"size:24;not null"`
	Payload       string `gorm:"type:longtext;not null"`
	CreatedAt     time.Time
}

func (KnowledgeDocumentOperationRow) TableName() string { return "knowledge_document_operation" }

// KnowledgeDocumentRepresentationRecord and its nodes become visible in one
// publication transaction. Derived text is distinct from the immutable original.
type KnowledgeDocumentRepresentationRecord struct {
	Protocol         string `gorm:"size:40;not null;default:document_structure/0.1.0"`
	ID               string `gorm:"primaryKey;size:64"`
	OwnerID          string `gorm:"size:32;not null"`
	DomainID         string `gorm:"size:32;not null;index"`
	SourceID         string `gorm:"size:32;not null;index"`
	SourceRevisionID string `gorm:"size:32;not null"`
	TaskID           string `gorm:"size:32;not null"`
	Payload          string `gorm:"type:longtext;not null"`
	ReadingText      string `gorm:"type:longtext;not null"`
	Manifest         string `gorm:"type:longtext;not null"`
	PublishedAt      time.Time
}

func (KnowledgeDocumentRepresentationRecord) TableName() string {
	return "knowledge_document_representation"
}

type KnowledgeDocumentHeadRecord struct {
	SourceID         string `gorm:"primaryKey;size:32"`
	DomainID         string `gorm:"size:32;not null;index"`
	OwnerID          string `gorm:"size:32;not null"`
	RepresentationID string `gorm:"size:64;not null"`
	UpdatedAt        time.Time
}

func (KnowledgeDocumentHeadRecord) TableName() string { return "knowledge_document_head" }

type KnowledgeDocumentNodeRecord struct {
	ID               string `gorm:"primaryKey;size:64"`
	OwnerID          string `gorm:"size:32;not null"`
	DomainID         string `gorm:"size:32;not null;index:idx_document_node_page"`
	RepresentationID string `gorm:"size:64;not null;index:idx_document_node_page"`
	ParentID         string `gorm:"size:64;not null;index:idx_document_node_page"`
	Ordinal          int64  `gorm:"not null;index:idx_document_node_page"`
	Payload          string `gorm:"type:longtext;not null"`
}

func (KnowledgeDocumentNodeRecord) TableName() string { return "knowledge_document_node" }

package entity

import "time"

const KnowledgeSourceContractVersion = "0.2.0"
const KnowledgeImportPreviewTTL = 10 * time.Minute
const KnowledgeImportRequestBytes int64 = 16 * 1024 * 1024

// KnowledgeSourceRegistration identifies a manual input scope, not a new domain.
type KnowledgeSourceRegistration struct {
	ID              string    `gorm:"primaryKey;size:32" json:"registration_id"`
	DomainID        string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_registration_key" json:"domain_id"`
	RegistrationKey string    `gorm:"size:128;not null;uniqueIndex:idx_knowledge_registration_key" json:"registration_key"`
	Name            string    `gorm:"size:128;not null" json:"name"`
	SourceKind      string    `gorm:"size:32;not null" json:"source_kind"`
	InputMode       string    `gorm:"size:32;not null" json:"input_mode"`
	IncludePaths    []string  `gorm:"serializer:json;type:text;not null" json:"include_paths"`
	GroupID         *string   `gorm:"size:32" json:"group_id"`
	ComponentSide   string    `gorm:"size:16;not null" json:"component_side"`
	Language        string    `gorm:"size:32;not null" json:"language"`
	ConfigHash      string    `gorm:"size:64;not null" json:"-"`
	CreatedAt       time.Time `json:"created_at"`
}

func (KnowledgeSourceRegistration) TableName() string { return "knowledge_source_registration" }

type CreateKnowledgeRegistrationRequest struct {
	RegistrationKey string   `json:"registration_key"`
	Name            string   `json:"name"`
	SourceKind      string   `json:"source_kind"`
	InputMode       string   `json:"input_mode"`
	IncludePaths    []string `json:"include_paths"`
	GroupID         *string  `json:"group_id"`
	ComponentSide   string   `json:"component_side"`
	Language        string   `json:"language"`
}

type PreviewKnowledgeImportRequest struct {
	RelativePath  string  `json:"relative_path"`
	Text          string  `json:"text"`
	GroupID       *string `json:"group_id"`
	ComponentSide string  `json:"component_side"`
	Language      string  `json:"language"`
}

// KnowledgeSourceBinding keeps one native Document across successful imports.
type KnowledgeSourceBinding struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_binding_key"`
	RelativePath   string `gorm:"size:512;not null;uniqueIndex:idx_knowledge_binding_key"`
	DocumentID     string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_binding_document"`
	FileID         string `gorm:"size:32;not null"`
	CreatedAt      time.Time
}

func (KnowledgeSourceBinding) TableName() string { return "knowledge_source_binding" }

// KnowledgeImportPreview holds private temporary text; public views omit it.
type KnowledgeImportPreview struct {
	ID                 string  `gorm:"primaryKey;size:32"`
	RegistrationID     string  `gorm:"size:32;not null;index"`
	PolicyVersion      int64   `gorm:"not null"`
	RelativePath       string  `gorm:"size:512;not null"`
	GroupID            *string `gorm:"size:32"`
	ComponentSide      string  `gorm:"size:16;not null"`
	Language           string  `gorm:"size:32;not null"`
	Content            string  `gorm:"type:longtext;not null"`
	SHA256             string  `gorm:"size:64;not null"`
	ByteLength         int64   `gorm:"not null"`
	ExpectedRevisionID string  `gorm:"size:32;not null"`
	DocumentID         *string `gorm:"size:32"`
	Action             string  `gorm:"size:16;not null"`
	ManifestSHA256     string  `gorm:"size:64;not null"`
	ConsumedTaskID     *string `gorm:"size:32"`
	ExpiresAt          time.Time
	CreatedAt          time.Time
}

func (KnowledgeImportPreview) TableName() string { return "knowledge_import_preview" }

type KnowledgeImport struct {
	ID             string `gorm:"primaryKey;size:32"`
	RegistrationID string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_import_key"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:idx_knowledge_import_key"`
	PreviewID      string `gorm:"size:32;not null;uniqueIndex:idx_knowledge_import_preview"`
	ManifestSHA256 string `gorm:"size:64;not null"`
	Receipt        string `gorm:"type:longtext;not null"`
	CreatedAt      time.Time
}

func (KnowledgeImport) TableName() string { return "knowledge_import" }

type CommitKnowledgeImportRequest struct {
	PreviewID      string `json:"preview_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	IdempotencyKey string `json:"idempotency_key"`
}

type KnowledgeImportCandidate struct {
	ItemKey            string  `json:"item_key"`
	RelativePath       string  `json:"relative_path"`
	GroupID            *string `json:"group_id"`
	ComponentSide      string  `json:"component_side"`
	Language           string  `json:"language"`
	Action             string  `json:"action"`
	SourceID           *string `json:"source_id"`
	ExpectedRevisionID *string `json:"expected_revision_id"`
	SHA256             string  `json:"sha256"`
	ByteLength         int64   `json:"byte_length"`
}
type KnowledgeImportPreviewView struct {
	ID             string                     `json:"preview_id"`
	RegistrationID string                     `json:"registration_id"`
	DomainID       string                     `json:"domain_id"`
	PolicyVersion  int64                      `json:"policy_version"`
	ManifestSHA256 string                     `json:"manifest_sha256"`
	ExpiresAt      time.Time                  `json:"expires_at"`
	ConsumedTaskID *string                    `json:"consumed_task_id"`
	Items          []KnowledgeImportCandidate `json:"items"`
}
type KnowledgeImportResult struct {
	ItemKey          string  `json:"item_key"`
	RelativePath     string  `json:"relative_path"`
	GroupID          *string `json:"group_id"`
	ComponentSide    string  `json:"component_side"`
	Language         string  `json:"language"`
	SaveStatus       string  `json:"save_status"`
	CatalogueStatus  string  `json:"catalogue_status"`
	StructureStatus  string  `json:"structure_status"`
	FullTextStatus   string  `json:"full_text_status"`
	VectorStatus     string  `json:"vector_status"`
	SourceID         *string `json:"source_id"`
	SourceRevisionID *string `json:"source_revision_id"`
	RootNodeID       *string `json:"root_node_id"`
	SHA256           string  `json:"sha256"`
	ByteLength       int64   `json:"byte_length"`
	Code             string  `json:"code"`
}
type KnowledgeImportReceipt struct {
	ID             string                  `json:"import_task_id"`
	RegistrationID string                  `json:"registration_id"`
	DomainID       string                  `json:"domain_id"`
	PreviewID      string                  `json:"preview_id"`
	ManifestSHA256 string                  `json:"manifest_sha256"`
	Status         string                  `json:"status"`
	ModelCalls     int                     `json:"model_calls"`
	Items          []KnowledgeImportResult `json:"items"`
	CreatedAt      time.Time               `json:"created_at"`
}

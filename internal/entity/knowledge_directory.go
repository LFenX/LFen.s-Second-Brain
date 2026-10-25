package entity

import (
	_ "embed"
	"time"
)

const KnowledgeDirectoryContractVersion = "0.3.0"

//go:embed knowledge_directory_capabilities.json
var KnowledgeDirectoryCapabilitiesJSON []byte

type KnowledgeDirectoryClassification struct {
	GroupID       *string `json:"group_id"`
	ComponentSide string  `json:"component_side"`
	Language      string  `json:"language"`
}

type KnowledgeDirectoryRoot struct {
	RootID          string  `json:"root_id"`
	DisplayName     string  `json:"display_name"`
	RootVersion     int64   `json:"root_version"`
	RuntimePlatform string  `json:"runtime_platform"`
	Ready           bool    `json:"ready"`
	Reason          *string `json:"reason"`
}

type KnowledgeDirectoryRoots struct {
	DomainID string                   `json:"domain_id"`
	Items    []KnowledgeDirectoryRoot `json:"items"`
}

type KnowledgeDirectoryRegistrationRequest struct {
	RegistrationKey       string                           `json:"registration_key"`
	Name                  string                           `json:"name"`
	SourceKind            string                           `json:"source_kind"`
	InputMode             string                           `json:"input_mode"`
	RootID                string                           `json:"root_id"`
	ScopeDirectories      []string                         `json:"scope_directories"`
	DefaultClassification KnowledgeDirectoryClassification `json:"default_classification"`
}

type KnowledgeDirectoryRegistration struct {
	RegistrationKey       string                           `json:"registration_key"`
	Name                  string                           `json:"name"`
	SourceKind            string                           `json:"source_kind"`
	InputMode             string                           `json:"input_mode"`
	RootID                string                           `json:"root_id"`
	ScopeDirectories      []string                         `json:"scope_directories"`
	DefaultClassification KnowledgeDirectoryClassification `json:"default_classification"`
	RegistrationID        string                           `json:"registration_id"`
	DomainID              string                           `json:"domain_id"`
	CreatedAt             string                           `json:"created_at"`
}

type KnowledgeDirectoryListingRequest struct {
	RelativeDirectory string `json:"relative_directory"`
	NamePrefix        string `json:"name_prefix"`
}

type KnowledgeDirectoryEntry struct {
	RelativePath string  `json:"relative_path"`
	EntryKind    string  `json:"entry_kind"`
	ByteLength   *int64  `json:"byte_length"`
	Selectable   bool    `json:"selectable"`
	Reason       *string `json:"reason"`
}

type KnowledgeDirectoryListing struct {
	ListingID         string                    `json:"listing_id"`
	DomainID          string                    `json:"domain_id"`
	RegistrationID    string                    `json:"registration_id"`
	RootID            string                    `json:"root_id"`
	RootVersion       int64                     `json:"root_version"`
	RelativeDirectory string                    `json:"relative_directory"`
	NamePrefix        string                    `json:"name_prefix"`
	CapturedAt        string                    `json:"captured_at"`
	ExpiresAt         string                    `json:"expires_at"`
	Complete          bool                      `json:"complete"`
	Reason            *string                   `json:"reason"`
	TotalEntries      int64                     `json:"total_entries"`
	Items             []KnowledgeDirectoryEntry `json:"items"`
	NextCursor        *string                   `json:"next_cursor"`
}

type KnowledgeDirectoryObservationRequest struct {
	SelectedPaths []string `json:"selected_paths"`
}

type KnowledgeDirectoryObservationItem struct {
	RelativePath       string  `json:"relative_path"`
	State              string  `json:"state"`
	Eligible           bool    `json:"eligible"`
	Reason             *string `json:"reason"`
	ByteLength         *int64  `json:"byte_length"`
	SHA256             *string `json:"sha256"`
	SourceID           *string `json:"source_id"`
	ExpectedRevisionID *string `json:"expected_revision_id"`
	ChangeKind         string  `json:"change_kind"`
}

type KnowledgeDirectoryObservation struct {
	ObservationID         string                              `json:"observation_id"`
	DomainID              string                              `json:"domain_id"`
	RegistrationID        string                              `json:"registration_id"`
	RootID                string                              `json:"root_id"`
	RootVersion           int64                               `json:"root_version"`
	PolicyVersion         int64                               `json:"policy_version"`
	DefaultClassification KnowledgeDirectoryClassification    `json:"default_classification"`
	SelectedPaths         []string                            `json:"selected_paths"`
	Complete              bool                                `json:"complete"`
	ObservationSHA256     string                              `json:"observation_sha256"`
	CapturedAt            string                              `json:"captured_at"`
	ExpiresAt             string                              `json:"expires_at"`
	Items                 []KnowledgeDirectoryObservationItem `json:"items"`
}

type KnowledgeDirectoryPreviewRequest struct {
	ObservationID     string                           `json:"observation_id"`
	ObservationSHA256 string                           `json:"observation_sha256"`
	RelativePath      string                           `json:"relative_path"`
	Classification    KnowledgeDirectoryClassification `json:"classification"`
}

type KnowledgeDirectoryPreviewItem struct {
	RelativePath       string                           `json:"relative_path"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	Action             string                           `json:"action"`
	SourceID           *string                          `json:"source_id"`
	ExpectedRevisionID *string                          `json:"expected_revision_id"`
	SHA256             string                           `json:"sha256"`
	ByteLength         int64                            `json:"byte_length"`
}

type KnowledgeDirectoryPreview struct {
	PreviewID         string                          `json:"preview_id"`
	DomainID          string                          `json:"domain_id"`
	RegistrationID    string                          `json:"registration_id"`
	RootID            string                          `json:"root_id"`
	RootVersion       int64                           `json:"root_version"`
	PolicyVersion     int64                           `json:"policy_version"`
	ObservationID     string                          `json:"observation_id"`
	ObservationSHA256 string                          `json:"observation_sha256"`
	ManifestSHA256    string                          `json:"manifest_sha256"`
	CapturedAt        string                          `json:"captured_at"`
	ExpiresAt         string                          `json:"expires_at"`
	ConsumedTaskID    *string                         `json:"consumed_task_id"`
	Items             []KnowledgeDirectoryPreviewItem `json:"items"`
}

type KnowledgeDirectoryContentWindow struct {
	PreviewID  string `json:"preview_id"`
	Offset     int64  `json:"offset"`
	ByteLength int64  `json:"byte_length"`
	TotalBytes int64  `json:"total_bytes"`
	SHA256     string `json:"sha256"`
	Text       string `json:"text"`
}

type KnowledgeDirectoryCommitRequest struct {
	PreviewID      string `json:"preview_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	IdempotencyKey string `json:"idempotency_key"`
}

type KnowledgeDirectoryResult struct {
	RelativePath     string                           `json:"relative_path"`
	Classification   KnowledgeDirectoryClassification `json:"classification"`
	SaveStatus       string                           `json:"save_status"`
	CatalogueStatus  string                           `json:"catalogue_status"`
	StructureStatus  string                           `json:"structure_status"`
	FullTextStatus   string                           `json:"full_text_status"`
	VectorStatus     string                           `json:"vector_status"`
	SourceID         *string                          `json:"source_id"`
	SourceRevisionID *string                          `json:"source_revision_id"`
	RootNodeID       *string                          `json:"root_node_id"`
	SHA256           string                           `json:"sha256"`
	ByteLength       int64                            `json:"byte_length"`
	Code             string                           `json:"code"`
}

type KnowledgeDirectoryReceipt struct {
	ImportTaskID   string                     `json:"import_task_id"`
	DomainID       string                     `json:"domain_id"`
	RegistrationID string                     `json:"registration_id"`
	PreviewID      string                     `json:"preview_id"`
	ObservationID  string                     `json:"observation_id"`
	ManifestSHA256 string                     `json:"manifest_sha256"`
	Status         string                     `json:"status"`
	ModelCalls     int64                      `json:"model_calls"`
	Mode           string                     `json:"mode"`
	Items          []KnowledgeDirectoryResult `json:"items"`
	CreatedAt      string                     `json:"created_at"`
}

// KnowledgeDirectoryRegistration extends the native registration without changing its wire shape.
type KnowledgeDirectoryRegistrationBinding struct {
	RegistrationID string `gorm:"primaryKey;size:32"`
	RootID         string `gorm:"size:32;not null"`
}

func (KnowledgeDirectoryRegistrationBinding) TableName() string {
	return "knowledge_directory_registration"
}

// KnowledgeDirectoryRootPin prevents a trusted root ID from being rebound to another physical root.
type KnowledgeDirectoryRootPin struct {
	ID         string `gorm:"primaryKey;size:32"`
	DomainID   string `gorm:"size:32;not null"`
	UserID     string `gorm:"size:32;not null"`
	PathHash   string `gorm:"size:64;not null"`
	Identity   string `gorm:"size:256;not null"`
	ConfigHash string `gorm:"size:64;not null"`
	Version    int64  `gorm:"not null"`
	CreatedAt  time.Time
}

func (KnowledgeDirectoryRootPin) TableName() string { return "knowledge_directory_root_pin" }

// KnowledgeDirectoryState stores immutable short-lived metadata; preview bytes stay in the native preview table.
type KnowledgeDirectoryState struct {
	ID             string    `gorm:"primaryKey;size:32"`
	DomainID       string    `gorm:"size:32;not null;index:idx_directory_quota"`
	UserID         string    `gorm:"size:32;not null;index:idx_directory_quota"`
	RegistrationID string    `gorm:"size:32;not null;index"`
	Kind           string    `gorm:"size:16;not null;index:idx_directory_quota"`
	Payload        string    `gorm:"type:longtext;not null"`
	PrivateData    string    `gorm:"type:longtext;not null"`
	ExpiresAt      time.Time `gorm:"not null;index"`
	CreatedAt      time.Time
}

func (KnowledgeDirectoryState) TableName() string { return "knowledge_directory_state" }

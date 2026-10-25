package entity

import _ "embed"

const KnowledgeOriginalContractVersion = "0.5.0"
const KnowledgeOriginalMaxBytes int64 = 64 * 1024 * 1024

//go:embed knowledge_original_capabilities.json
var KnowledgeOriginalCapabilitiesJSON []byte

// KnowledgeOriginalRegistrationRequest is the frozen C2 0.5.0 CreateRegistration projection.
type KnowledgeOriginalRegistrationRequest struct {
	RegistrationKey       string                           `json:"registration_key"`
	Name                  string                           `json:"name"`
	SourceKind            string                           `json:"source_kind"`
	InputMode             string                           `json:"input_mode"`
	IncludePaths          []string                         `json:"include_paths"`
	DefaultClassification KnowledgeDirectoryClassification `json:"default_classification"`
}

// KnowledgeOriginalRegistration is the frozen C2 0.5.0 Registration projection.
type KnowledgeOriginalRegistration struct {
	RegistrationKey       string                           `json:"registration_key"`
	Name                  string                           `json:"name"`
	SourceKind            string                           `json:"source_kind"`
	InputMode             string                           `json:"input_mode"`
	IncludePaths          []string                         `json:"include_paths"`
	DefaultClassification KnowledgeDirectoryClassification `json:"default_classification"`
	RegistrationID        string                           `json:"registration_id"`
	DomainID              string                           `json:"domain_id"`
	ConfigSHA256          string                           `json:"config_sha256"`
	CreatedAt             string                           `json:"created_at"`
}

// KnowledgeOriginalUploadRequest is the frozen C2 0.5.0 CreateUpload projection.
type KnowledgeOriginalUploadRequest struct {
	OriginalFilename   string                           `json:"original_filename"`
	RelativePath       string                           `json:"relative_path"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	DeclaredByteLength int64                            `json:"declared_byte_length"`
	DeclaredMediaType  string                           `json:"declared_media_type"`
	ExpectedSHA256     *string                          `json:"expected_sha256"`
	ExpectedRevisionID *string                          `json:"expected_revision_id"`
	IdempotencyKey     string                           `json:"idempotency_key"`
}

// KnowledgeOriginalUpload is the frozen C2 0.5.0 Upload projection.
type KnowledgeOriginalUpload struct {
	UploadID           string                           `json:"upload_id"`
	DomainID           string                           `json:"domain_id"`
	OwnerID            string                           `json:"owner_id"`
	RegistrationID     string                           `json:"registration_id"`
	IdempotencyKey     string                           `json:"idempotency_key"`
	RequestSHA256      string                           `json:"request_sha256"`
	OriginalFilename   string                           `json:"original_filename"`
	RelativePath       string                           `json:"relative_path"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	DeclaredByteLength int64                            `json:"declared_byte_length"`
	DeclaredMediaType  string                           `json:"declared_media_type"`
	ExpectedSHA256     *string                          `json:"expected_sha256"`
	ExpectedRevisionID *string                          `json:"expected_revision_id"`
	SourceID           *string                          `json:"source_id"`
	PolicyVersion      int64                            `json:"policy_version"`
	State              string                           `json:"state"`
	ReceivedBytes      int64                            `json:"received_bytes"`
	ByteLength         *int64                           `json:"byte_length"`
	SHA256             *string                          `json:"sha256"`
	MediaType          *string                          `json:"media_type"`
	Action             *string                          `json:"action"`
	ManifestSHA256     *string                          `json:"manifest_sha256"`
	ImportTaskID       *string                          `json:"import_task_id"`
	Code               *string                          `json:"code"`
	CreatedAt          string                           `json:"created_at"`
	ExpiresAt          string                           `json:"expires_at"`
	Revision           int64                            `json:"revision"`
	UpdatedAt          string                           `json:"updated_at"`
}

// KnowledgeOriginalConfirmRequest is the frozen C2 0.5.0 Confirm projection.
type KnowledgeOriginalConfirmRequest struct {
	UploadID       string `json:"upload_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	IdempotencyKey string `json:"idempotency_key"`
	Confirmed      bool   `json:"confirmed"`
}

// KnowledgeOriginalRevision is the frozen C2 0.5.0 OriginalRevision projection.
type KnowledgeOriginalRevision struct {
	DomainID           string                           `json:"domain_id"`
	RegistrationID     string                           `json:"registration_id"`
	SourceID           string                           `json:"source_id"`
	SourceRevisionID   string                           `json:"source_revision_id"`
	OriginalFilename   string                           `json:"original_filename"`
	RelativePath       string                           `json:"relative_path"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	SHA256             string                           `json:"sha256"`
	ByteLength         int64                            `json:"byte_length"`
	MediaType          string                           `json:"media_type"`
	RepresentationKind string                           `json:"representation_kind"`
	RootNodeID         *string                          `json:"root_node_id"`
	TextAvailable      bool                             `json:"text_available"`
	ParseStatus        string                           `json:"parse_status"`
	StructureStatus    string                           `json:"structure_status"`
	FullTextStatus     string                           `json:"full_text_status"`
	VectorStatus       string                           `json:"vector_status"`
	IsCurrent          bool                             `json:"is_current"`
	CreatedAt          string                           `json:"created_at"`
}

// KnowledgeOriginalReceipt is the frozen C2 0.5.0 Receipt projection.
type KnowledgeOriginalReceipt struct {
	ImportTaskID   string                    `json:"import_task_id"`
	DomainID       string                    `json:"domain_id"`
	RegistrationID string                    `json:"registration_id"`
	UploadID       string                    `json:"upload_id"`
	IdempotencyKey string                    `json:"idempotency_key"`
	ManifestSHA256 string                    `json:"manifest_sha256"`
	Status         string                    `json:"status"`
	SaveStatus     string                    `json:"save_status"`
	ModelCalls     int64                     `json:"model_calls"`
	Source         KnowledgeOriginalRevision `json:"source"`
	CreatedAt      string                    `json:"created_at"`
}

// KnowledgeOriginalTarget is the frozen C2 0.5.0 Target projection.
type KnowledgeOriginalTarget struct {
	DomainID       string                     `json:"domain_id"`
	RegistrationID string                     `json:"registration_id"`
	RelativePath   string                     `json:"relative_path"`
	Source         *KnowledgeOriginalRevision `json:"source"`
}

// KnowledgeOriginalPage is the frozen C2 0.5.0 OriginalPage projection.
type KnowledgeOriginalPage struct {
	DomainID   string                      `json:"domain_id"`
	Items      []KnowledgeOriginalRevision `json:"items"`
	NextCursor *string                     `json:"next_cursor"`
}

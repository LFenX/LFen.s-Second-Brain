package entity

import "time"

// KnowledgeContentPolicyRecord grants derived local work only. Foundation,
// native ingestion and the lexical master switch remain separate authorities.
type KnowledgeContentPolicyRecord struct {
	DomainID       string `gorm:"primaryKey;size:32"`
	OwnerID        string `gorm:"size:32;not null"`
	Version        int64  `gorm:"not null"`
	ParseEnabled   bool   `gorm:"not null"`
	LexicalEnabled bool   `gorm:"not null"`
	UpdatedAt      time.Time
}

func (KnowledgeContentPolicyRecord) TableName() string { return "knowledge_content_policy" }

type KnowledgeContentPolicy struct {
	DomainID                string `json:"domain_id"`
	FoundationPolicyVersion int64  `json:"foundation_policy_version"`
	ContentPolicyVersion    int64  `json:"content_policy_version"`
	ParseEnabled            bool   `json:"parse_enabled"`
	LexicalEnabled          bool   `json:"lexical_enabled"`
}

type KnowledgeContentPolicyRequest struct {
	ExpectedPolicyVersion        int64  `json:"expected_policy_version"`
	ExpectedContentPolicyVersion int64  `json:"expected_content_policy_version"`
	ParseEnabled                 bool   `json:"parse_enabled"`
	LexicalEnabled               bool   `json:"lexical_enabled"`
	Confirmed                    bool   `json:"confirmed"`
	IdempotencyKey               string `json:"idempotency_key"`
}

type KnowledgeParsedProfile struct {
	ProfileID      string `json:"profile_id"`
	Format         string `json:"format"`
	ProducerSHA256 string `json:"producer_sha256"`
	RuntimeReady   bool   `json:"runtime_ready"`
}

type KnowledgeParsedSnapshotRequest struct {
	DocumentID            string                           `json:"source_id"`
	ExpectedRevisionID    string                           `json:"expected_revision_id"`
	ExpectedPolicyVersion int64                            `json:"expected_policy_version"`
	RelativePath          string                           `json:"relative_path"`
	Classification        KnowledgeDirectoryClassification `json:"classification"`
}

type KnowledgeParsedUploadRequest struct {
	ExpectedPolicyVersion        int64 `json:"expected_policy_version"`
	ExpectedContentPolicyVersion int64 `json:"expected_content_policy_version"`
	Confirmed                    bool  `json:"confirmed"`
}

type KnowledgeParsedUploadReceipt struct {
	SourceID         string `json:"source_id"`
	OriginalFilename string `json:"original_filename"`
	ByteLength       int64  `json:"byte_length"`
}

type KnowledgeParsedSourceState struct {
	Source                  KnowledgeContentSourceRef        `json:"source_ref"`
	NativeFileID            string                           `json:"native_file_id"`
	RepresentationKind      string                           `json:"representation_kind"`
	OriginalFilename        string                           `json:"original_filename"`
	RelativePath            string                           `json:"relative_path"`
	Classification          KnowledgeDirectoryClassification `json:"classification"`
	ByteLength              int64                            `json:"byte_length"`
	LifecycleVersion        int64                            `json:"lifecycle_version"`
	State                   string                           `json:"state"`
	FoundationPolicyVersion int64                            `json:"foundation_policy_version"`
	Profile                 *KnowledgeParsedProfile          `json:"profile"`
}

type KnowledgeParsedTaskRequest struct {
	Source                       KnowledgeContentSourceRef `json:"source_ref"`
	ExpectedLifecycleVersion     int64                     `json:"expected_lifecycle_version"`
	ExpectedPolicyVersion        int64                     `json:"expected_policy_version"`
	ExpectedContentPolicyVersion int64                     `json:"expected_content_policy_version"`
	ExpectedProducerSHA256       string                    `json:"expected_producer_sha256"`
	MaxUnitBytes                 int64                     `json:"max_unit_bytes"`
	Confirmed                    bool                      `json:"confirmed"`
	IdempotencyKey               string                    `json:"idempotency_key"`
}

type KnowledgeParsedTask struct {
	TaskID          string                     `json:"task_id"`
	OwnerID         string                     `json:"owner_id"`
	DomainID        string                     `json:"domain_id"`
	OriginalRequest KnowledgeParsedTaskRequest `json:"original_request"`
	RequestSHA256   string                     `json:"request_sha256"`
	ProducerSHA256  string                     `json:"producer_sha256"`
	Revision        int64                      `json:"revision"`
	AttemptCount    int64                      `json:"attempt_count"`
	State           string                     `json:"state"`
	Phase           string                     `json:"phase"`
	Result          *KnowledgeContentTarget    `json:"result"`
	FailureCode     *string                    `json:"failure_code"`
	CreatedAt       string                     `json:"created_at"`
	UpdatedAt       string                     `json:"updated_at"`
	DeadlineAt      string                     `json:"deadline_at"`
	CompletedAt     *string                    `json:"completed_at"`
}

type KnowledgeParsedCancelRequest struct {
	ExpectedTaskRevision int64  `json:"expected_task_revision"`
	IdempotencyKey       string `json:"idempotency_key"`
}

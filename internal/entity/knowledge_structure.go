// Code generated from frozen Structure 0.1.0 schemas; DO NOT EDIT.
package entity

import "encoding/json"

const KnowledgeStructureContractVersion = "0.1.0"

type KnowledgeStructureClassification struct {
	GroupID       *string `json:"group_id"`
	ComponentSide string  `json:"component_side"`
	Language      string  `json:"language"`
}

type KnowledgeStructureAlias struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
}

type KnowledgeStructureMember struct {
	SourceID         string `json:"source_id"`
	SourceRevisionID string `json:"source_revision_id"`
	LifecycleVersion int64  `json:"lifecycle_version"`
	Parser           string `json:"parser"`
}

type KnowledgeStructurePolicy struct {
	DomainID                string   `json:"domain_id"`
	FoundationPolicyVersion int64    `json:"foundation_policy_version"`
	StructurePolicyVersion  int64    `json:"structure_policy_version"`
	Enabled                 bool     `json:"enabled"`
	Mode                    string   `json:"mode"`
	AutomaticIngestion      bool     `json:"automatic_ingestion"`
	AllowedModelUses        []string `json:"allowed_model_uses"`
	ExternalModels          bool     `json:"external_models"`
	LocalInference          bool     `json:"local_inference"`
	LexicalIndex            string   `json:"lexical_index"`
	VectorIndex             string   `json:"vector_index"`
}

type KnowledgeStructurePolicyChangeRequest struct {
	ExpectedPolicyVersion          int64  `json:"expected_policy_version"`
	ExpectedStructurePolicyVersion int64  `json:"expected_structure_policy_version"`
	Enabled                        bool   `json:"enabled"`
	Confirmed                      bool   `json:"confirmed"`
	IdempotencyKey                 string `json:"idempotency_key"`
}

type KnowledgeStructurePolicySnapshot struct {
	StructurePolicyVersion int64 `json:"structure_policy_version"`
	Enabled                bool  `json:"enabled"`
}

type KnowledgeStructurePolicyReceipt struct {
	CommandID       string                                `json:"command_id"`
	Kind            string                                `json:"kind"`
	DomainID        string                                `json:"domain_id"`
	OwnerID         string                                `json:"owner_id"`
	IdempotencyKey  string                                `json:"idempotency_key"`
	OriginalRequest KnowledgeStructurePolicyChangeRequest `json:"original_request"`
	RequestSHA256   string                                `json:"request_sha256"`
	Before          KnowledgeStructurePolicySnapshot      `json:"before"`
	After           KnowledgeStructurePolicySnapshot      `json:"after"`
	Outcome         string                                `json:"outcome"`
	CompletedAt     string                                `json:"completed_at"`
	ModelCalls      int                                   `json:"model_calls"`
}

type KnowledgeStructureTaskRequest struct {
	ProjectID                      string                     `json:"project_id"`
	ManifestID                     string                     `json:"manifest_id"`
	ExpectedActiveViewID           *string                    `json:"expected_active_view_id"`
	ExpectedPolicyVersion          int64                      `json:"expected_policy_version"`
	ExpectedStructurePolicyVersion int64                      `json:"expected_structure_policy_version"`
	Members                        []KnowledgeStructureMember `json:"members"`
	Aliases                        []KnowledgeStructureAlias  `json:"aliases"`
	Confirmed                      bool                       `json:"confirmed"`
	IdempotencyKey                 string                     `json:"idempotency_key"`
}

type KnowledgeStructureCancelRequest struct {
	ExpectedTaskRevision int64  `json:"expected_task_revision"`
	Confirmed            bool   `json:"confirmed"`
	IdempotencyKey       string `json:"idempotency_key"`
}

type KnowledgeStructureDiagnostic struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	StartByte *int64 `json:"start_byte"`
	EndByte   *int64 `json:"end_byte"`
}

type KnowledgeStructureTaskMember struct {
	SourceID         string                         `json:"source_id"`
	SourceRevisionID string                         `json:"source_revision_id"`
	LifecycleVersion int64                          `json:"lifecycle_version"`
	Parser           string                         `json:"parser"`
	State            string                         `json:"state"`
	Diagnostics      []KnowledgeStructureDiagnostic `json:"diagnostics"`
	NodeCount        int64                          `json:"node_count"`
	RelationCount    int64                          `json:"relation_count"`
}

type KnowledgeStructureTask struct {
	TaskID          string                         `json:"task_id"`
	OwnerID         string                         `json:"owner_id"`
	DomainID        string                         `json:"domain_id"`
	ProjectID       string                         `json:"project_id"`
	OriginalRequest KnowledgeStructureTaskRequest  `json:"original_request"`
	RequestSHA256   string                         `json:"request_sha256"`
	ProducerSHA256  string                         `json:"producer_sha256"`
	Revision        int64                          `json:"revision"`
	AttemptCount    int64                          `json:"attempt_count"`
	State           string                         `json:"state"`
	CreatedAt       string                         `json:"created_at"`
	UpdatedAt       string                         `json:"updated_at"`
	DeadlineAt      string                         `json:"deadline_at"`
	TotalMembers    int64                          `json:"total_members"`
	FinishedMembers int64                          `json:"finished_members"`
	Members         []KnowledgeStructureTaskMember `json:"members"`
	ResultViewID    *string                        `json:"result_view_id"`
	FailureCode     *string                        `json:"failure_code"`
	ModelCalls      int                            `json:"model_calls"`
	LocalInference  int                            `json:"local_inference"`
	ExternalFetches int                            `json:"external_fetches"`
	CompletedAt     *string                        `json:"completed_at"`
}

type KnowledgeStructureCancelReceipt struct {
	CommandID       string                          `json:"command_id"`
	Kind            string                          `json:"kind"`
	DomainID        string                          `json:"domain_id"`
	OwnerID         string                          `json:"owner_id"`
	TaskID          string                          `json:"task_id"`
	IdempotencyKey  string                          `json:"idempotency_key"`
	OriginalRequest KnowledgeStructureCancelRequest `json:"original_request"`
	RequestSHA256   string                          `json:"request_sha256"`
	BeforeState     string                          `json:"before_state"`
	AfterState      string                          `json:"after_state"`
	TaskRevision    int64                           `json:"task_revision"`
	Outcome         string                          `json:"outcome"`
	CompletedAt     string                          `json:"completed_at"`
	ModelCalls      int                             `json:"model_calls"`
}

type KnowledgeStructureViewMember struct {
	SourceID            string                           `json:"source_id"`
	SourceRevisionID    string                           `json:"source_revision_id"`
	SourceNodeID        string                           `json:"source_node_id"`
	RootStructureNodeID string                           `json:"root_structure_node_id"`
	LifecycleVersion    int64                            `json:"lifecycle_version"`
	Parser              string                           `json:"parser"`
	SHA256              string                           `json:"sha256"`
	ByteLength          int64                            `json:"byte_length"`
	RelativePath        string                           `json:"relative_path"`
	Classification      KnowledgeStructureClassification `json:"classification"`
}

type KnowledgeStructureView struct {
	ViewID                  string                         `json:"view_id"`
	DomainID                string                         `json:"domain_id"`
	OwnerID                 string                         `json:"owner_id"`
	ProjectID               string                         `json:"project_id"`
	ManifestID              string                         `json:"manifest_id"`
	ManifestSHA256          string                         `json:"manifest_sha256"`
	FoundationPolicyVersion int64                          `json:"foundation_policy_version"`
	StructurePolicyVersion  int64                          `json:"structure_policy_version"`
	ProducerSHA256          string                         `json:"producer_sha256"`
	ProducerFormat          string                         `json:"producer_format"`
	Aliases                 []KnowledgeStructureAlias      `json:"aliases"`
	Members                 []KnowledgeStructureViewMember `json:"members"`
	SourceCount             int64                          `json:"source_count"`
	NodeCount               int64                          `json:"node_count"`
	RelationCount           int64                          `json:"relation_count"`
	SourceBytes             int64                          `json:"source_bytes"`
	PublishedAt             string                         `json:"published_at"`
	ModelCalls              int                            `json:"model_calls"`
	LocalInference          int                            `json:"local_inference"`
	LexicalIndex            string                         `json:"lexical_index"`
	VectorIndex             string                         `json:"vector_index"`
}

type KnowledgeStructureHead struct {
	DomainID          string                        `json:"domain_id"`
	ProjectID         string                        `json:"project_id"`
	ViewID            *string                       `json:"view_id"`
	CurrentManifestID *string                       `json:"current_manifest_id"`
	Currentness       string                        `json:"currentness"`
	Reasons           []string                      `json:"reasons"`
	CheckedAt         string                        `json:"checked_at"`
	AffectedSources   []KnowledgeStructureHeadIssue `json:"affected_sources"`
}

type KnowledgeStructureNode struct {
	StructureNodeID   string                           `json:"structure_node_id"`
	LogicalNodeID     string                           `json:"logical_node_id"`
	ViewID            string                           `json:"view_id"`
	SourceID          string                           `json:"source_id"`
	SourceRevisionID  string                           `json:"source_revision_id"`
	SourceNodeID      string                           `json:"source_node_id"`
	ParentID          *string                          `json:"parent_id"`
	Kind              string                           `json:"kind"`
	Name              string                           `json:"name"`
	LogicalKey        string                           `json:"logical_key"`
	IdentityStability string                           `json:"identity_stability"`
	StartByte         int64                            `json:"start_byte"`
	EndByte           int64                            `json:"end_byte"`
	StartLine         int64                            `json:"start_line"`
	EndLine           int64                            `json:"end_line"`
	Parser            string                           `json:"parser"`
	Classification    KnowledgeStructureClassification `json:"classification"`
	CoordinateSpace   string                           `json:"coordinate_space"`
	ProducerSHA256    string                           `json:"producer_sha256"`
}

type KnowledgeStructureContent struct {
	ViewID           string `json:"view_id"`
	StructureNodeID  string `json:"structure_node_id"`
	SourceNodeID     string `json:"source_node_id"`
	SourceID         string `json:"source_id"`
	SourceRevisionID string `json:"source_revision_id"`
	SourceSHA256     string `json:"source_sha256"`
	FragmentSHA256   string `json:"fragment_sha256"`
	CoordinateSpace  string `json:"coordinate_space"`
	StartByte        int64  `json:"start_byte"`
	EndByte          int64  `json:"end_byte"`
	Text             string `json:"text"`
	Complete         bool   `json:"complete"`
	NextStartByte    *int64 `json:"next_start_byte"`
}

type KnowledgeStructureEvidence struct {
	SourceID         string `json:"source_id"`
	SourceRevisionID string `json:"source_revision_id"`
	StartByte        int64  `json:"start_byte"`
	EndByte          int64  `json:"end_byte"`
	FragmentSHA256   string `json:"fragment_sha256"`
}

type KnowledgeStructureRelation struct {
	RelationID         string                     `json:"relation_id"`
	ViewID             string                     `json:"view_id"`
	Kind               string                     `json:"kind"`
	FromNodeID         string                     `json:"from_node_id"`
	ToNodeID           *string                    `json:"to_node_id"`
	Resolution         string                     `json:"resolution"`
	Reason             string                     `json:"reason"`
	Reference          string                     `json:"reference"`
	ReferenceTruncated bool                       `json:"reference_truncated"`
	Evidence           KnowledgeStructureEvidence `json:"evidence"`
}

type KnowledgeStructureNodePage struct {
	ViewID     string                   `json:"view_id"`
	SourceID   string                   `json:"source_id"`
	ParentID   *string                  `json:"parent_id"`
	Items      []KnowledgeStructureNode `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

type KnowledgeStructureRelationPage struct {
	ViewID     string                       `json:"view_id"`
	NodeID     string                       `json:"node_id"`
	Direction  string                       `json:"direction"`
	Kind       *string                      `json:"kind"`
	Items      []KnowledgeStructureRelation `json:"items"`
	NextCursor *string                      `json:"next_cursor"`
}

type KnowledgeStructureTaskPage struct {
	DomainID   string                   `json:"domain_id"`
	ProjectID  string                   `json:"project_id"`
	CapturedAt string                   `json:"captured_at"`
	Items      []KnowledgeStructureTask `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

type KnowledgeStructureLimits struct {
	MaxFiles                  int `json:"max_files"`
	MaxFileBytes              int `json:"max_file_bytes"`
	MaxInputBytes             int `json:"max_input_bytes"`
	MaxNodesPerFile           int `json:"max_nodes_per_file"`
	MaxNodes                  int `json:"max_nodes"`
	MaxRelations              int `json:"max_relations"`
	MaxDepth                  int `json:"max_depth"`
	PassageTargetBytes        int `json:"passage_target_bytes"`
	PassageMaxBytes           int `json:"passage_max_bytes"`
	ContentMaxBytes           int `json:"content_max_bytes"`
	PageMaxItems              int `json:"page_max_items"`
	MetadataResponseMaxBytes  int `json:"metadata_response_max_bytes"`
	ContentResponseMaxBytes   int `json:"content_response_max_bytes"`
	RequestMaxBytes           int `json:"request_max_bytes"`
	CursorTtlSeconds          int `json:"cursor_ttl_seconds"`
	LeaseSeconds              int `json:"lease_seconds"`
	LeaseRenewSeconds         int `json:"lease_renew_seconds"`
	MaxAttempts               int `json:"max_attempts"`
	TaskDeadlineSeconds       int `json:"task_deadline_seconds"`
	AttemptSoftTimeoutSeconds int `json:"attempt_soft_timeout_seconds"`
	TsParseTimeoutSeconds     int `json:"ts_parse_timeout_seconds"`
	ClientWaitSeconds         int `json:"client_wait_seconds"`
	RecoverySlotMaxBytes      int `json:"recovery_slot_max_bytes"`
	RecoverySlots             int `json:"recovery_slots"`
}

type KnowledgeStructureProducers struct {
	Format               string `json:"format"`
	Go                   string `json:"go"`
	GoToolchain          string `json:"go_toolchain"`
	TreeSitterGo         string `json:"tree_sitter_go"`
	TreeSitterTypescript string `json:"tree_sitter_typescript"`
	Goldmark             string `json:"goldmark"`
	CoordinateSpace      string `json:"coordinate_space"`
	ParseCtx             bool   `json:"parse_ctx"`
	RemoteImages         bool   `json:"remote_images"`
	OverlapBytes         int    `json:"overlap_bytes"`
}

type KnowledgeStructureCapabilities struct {
	ContractVersion          string                      `json:"contract_version"`
	StructureContractVersion string                      `json:"structure_contract_version"`
	RuntimeReady             bool                        `json:"runtime_ready"`
	OwnerOnly                bool                        `json:"owner_only"`
	Mode                     string                      `json:"mode"`
	ParserIDs                []string                    `json:"parser_ids"`
	PolicyDefaultEnabled     bool                        `json:"policy_default_enabled"`
	ModelCalls               int                         `json:"model_calls"`
	LocalInference           int                         `json:"local_inference"`
	ExternalFetches          int                         `json:"external_fetches"`
	AutomaticIngestion       bool                        `json:"automatic_ingestion"`
	LexicalIndex             string                      `json:"lexical_index"`
	VectorIndex              string                      `json:"vector_index"`
	PartialPublication       bool                        `json:"partial_publication"`
	Producers                KnowledgeStructureProducers `json:"producers"`
	Limits                   KnowledgeStructureLimits    `json:"limits"`
}

type KnowledgeStructureHeadIssue struct {
	SourceID string   `json:"source_id"`
	Reasons  []string `json:"reasons"`
}

type KnowledgeStructureRejectionReceipt struct {
	RejectionID     string          `json:"rejection_id"`
	Operation       string          `json:"operation"`
	OwnerID         string          `json:"owner_id"`
	DomainID        string          `json:"domain_id"`
	TaskID          *string         `json:"task_id"`
	IdempotencyKey  string          `json:"idempotency_key"`
	OriginalRequest json.RawMessage `json:"original_request"`
	RequestSHA256   string          `json:"request_sha256"`
	Reason          string          `json:"reason"`
	RecordedAt      string          `json:"recorded_at"`
	Accepted        bool            `json:"accepted"`
	ModelCalls      int             `json:"model_calls"`
}

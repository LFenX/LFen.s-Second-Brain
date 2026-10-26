package entity

import "encoding/json"

const KnowledgeDocumentContractVersion = "0.1.0"
const KnowledgeDocumentReadingFormat = "rich_document_reading_v1"
const KnowledgeDocumentCoordinateSpace = "derived_utf8"
const KnowledgeDocumentPDFProfile = "pdf-plain-pages/1"
const KnowledgeDocumentPPTXProfile = "pptx-ooxml-slides/1"
const KnowledgeDocumentDOCXProfile = "docx-ooxml-blocks/1"

type KnowledgeDocumentClassification = KnowledgeDirectoryClassification

type KnowledgeDocumentOriginalRef struct {
	DomainID           string  `json:"domain_id"`
	RegistrationID     string  `json:"registration_id"`
	SourceID           string  `json:"source_id"`
	SourceRevisionID   string  `json:"source_revision_id"`
	SHA256             string  `json:"sha256"`
	ByteLength         int64   `json:"byte_length"`
	MediaType          string  `json:"media_type"`
	OriginalFilename   string  `json:"original_filename"`
	RepresentationKind string  `json:"representation_kind"`
	RootNodeID         *string `json:"root_node_id"`
	TextAvailable      bool    `json:"text_available"`
}
type KnowledgeDocumentSourceSnapshot struct {
	LifecycleVersion int64                            `json:"lifecycle_version"`
	RelativePath     string                           `json:"relative_path"`
	Classification   KnowledgeDirectoryClassification `json:"classification"`
}
type KnowledgeDocumentConfiguration struct {
	Extraction               string `json:"extraction"`
	ReadingFormat            string `json:"reading_format"`
	NormalizeDOCXLineSpacing bool   `json:"normalize_docx_line_spacing"`
	TrimText                 bool   `json:"trim_text"`
	RemoteResources          bool   `json:"remote_resources"`
	OCR                      bool   `json:"ocr"`
	Models                   bool   `json:"models"`
	MediaOutput              string `json:"media_output"`
}
type KnowledgeDocumentProducer struct {
	ProfileID            string `json:"profile_id"`
	Format               string `json:"format"`
	EngineID             string `json:"engine_id"`
	EngineVersion        string `json:"engine_version"`
	EngineBuildSHA256    string `json:"engine_build_sha256"`
	AdapterVersion       string `json:"adapter_version"`
	ReadingFormatVersion string `json:"reading_format_version"`
	ConfigurationSHA256  string `json:"configuration_sha256"`
}
type KnowledgeDocumentProfile struct {
	ProfileID      string                         `json:"profile_id"`
	Format         string                         `json:"format"`
	MediaType      string                         `json:"media_type"`
	Configuration  KnowledgeDocumentConfiguration `json:"configuration"`
	Producer       KnowledgeDocumentProducer      `json:"producer"`
	ProducerSHA256 string                         `json:"producer_sha256"`
	RuntimeReady   bool                           `json:"runtime_ready"`
}
type KnowledgeDocumentLimits struct {
	MaxOriginals             int64 `json:"max_originals"`
	MaxOriginalBytes         int64 `json:"max_original_bytes"`
	MaxOfficeEntries         int64 `json:"max_office_entries"`
	MaxExpandedBytes         int64 `json:"max_expanded_bytes"`
	MaxXMLPartBytes          int64 `json:"max_xml_part_bytes"`
	MaxXMLTotalBytes         int64 `json:"max_xml_total_bytes"`
	MaxXMLDepth              int64 `json:"max_xml_depth"`
	MaxIntermediateBytes     int64 `json:"max_intermediate_bytes"`
	MaxPackBytes             int64 `json:"max_pack_bytes"`
	MaxPDFPages              int64 `json:"max_pdf_pages"`
	MaxPPTXSlides            int64 `json:"max_pptx_slides"`
	MaxNodes                 int64 `json:"max_nodes"`
	MaxDepth                 int64 `json:"max_depth"`
	MaxReadingBytes          int64 `json:"max_reading_bytes"`
	PassageTargetBytes       int64 `json:"passage_target_bytes"`
	PassageMaxBytes          int64 `json:"passage_max_bytes"`
	PassageOverlapBytes      int64 `json:"passage_overlap_bytes"`
	ContentMaxBytes          int64 `json:"content_max_bytes"`
	PageMaxItems             int64 `json:"page_max_items"`
	MetadataResponseMaxBytes int64 `json:"metadata_response_max_bytes"`
	ContentResponseMaxBytes  int64 `json:"content_response_max_bytes"`
	RequestMaxBytes          int64 `json:"request_max_bytes"`
	CursorTTLSeconds         int64 `json:"cursor_ttl_seconds"`
	LeaseSeconds             int64 `json:"lease_seconds"`
	LeaseRenewSeconds        int64 `json:"lease_renew_seconds"`
	AttemptTimeoutSeconds    int64 `json:"attempt_timeout_seconds"`
	MaxAttempts              int64 `json:"max_attempts"`
	TaskDeadlineSeconds      int64 `json:"task_deadline_seconds"`
	ClientWaitSeconds        int64 `json:"client_wait_seconds"`
	PollIntervalSeconds      int64 `json:"poll_interval_seconds"`
	RecoverySlotMaxBytes     int64 `json:"recovery_slot_max_bytes"`
	RecoverySlots            int64 `json:"recovery_slots"`
	StderrMaxBytes           int64 `json:"stderr_max_bytes"`
	RSSSamplingIntervalMs    int64 `json:"rss_sampling_interval_ms"`
	MaxRSSGrowthBytes        int64 `json:"max_rss_growth_bytes"`
}
type KnowledgeDocumentCapabilities struct {
	ContractVersion                  string                                      `json:"contract_version"`
	DocumentStructureContractVersion string                                      `json:"document_structure_contract_version"`
	RuntimeReady                     bool                                        `json:"runtime_ready"`
	OwnerOnly                        bool                                        `json:"owner_only"`
	Mode                             string                                      `json:"mode"`
	DocumentPolicyDefaultEnabled     bool                                        `json:"document_policy_default_enabled"`
	CodeStructurePermissionRequired  bool                                        `json:"code_structure_permission_required"`
	AutomaticProcessing              bool                                        `json:"automatic_processing"`
	ModelCalls                       int64                                       `json:"model_calls"`
	LocalInference                   int64                                       `json:"local_inference"`
	ExternalFetches                  int64                                       `json:"external_fetches"`
	LexicalIndex                     string                                      `json:"lexical_index"`
	VectorIndex                      string                                      `json:"vector_index"`
	PartialPublication               bool                                        `json:"partial_publication"`
	ExecutionQuota                   KnowledgeDocumentCapabilitiesExecutionQuota `json:"execution_quota"`
	Execution                        KnowledgeDocumentCapabilitiesExecution      `json:"execution"`
	Profiles                         []KnowledgeDocumentProfile                  `json:"profiles"`
	Limits                           KnowledgeDocumentLimits                     `json:"limits"`
}
type KnowledgeDocumentPolicy struct {
	DomainID                        string   `json:"domain_id"`
	FoundationPolicyVersion         int64    `json:"foundation_policy_version"`
	DocumentPolicyVersion           int64    `json:"document_policy_version"`
	Enabled                         bool     `json:"enabled"`
	Mode                            string   `json:"mode"`
	CodeStructurePermissionRequired bool     `json:"code_structure_permission_required"`
	AllowedModelUses                []string `json:"allowed_model_uses"`
	ExternalModels                  bool     `json:"external_models"`
	LocalInference                  bool     `json:"local_inference"`
	AutomaticProcessing             bool     `json:"automatic_processing"`
	LexicalIndex                    string   `json:"lexical_index"`
	VectorIndex                     string   `json:"vector_index"`
}
type KnowledgeDocumentPolicyChangeRequest struct {
	ExpectedPolicyVersion         int64  `json:"expected_policy_version"`
	ExpectedDocumentPolicyVersion int64  `json:"expected_document_policy_version"`
	Enabled                       bool   `json:"enabled"`
	Confirmed                     bool   `json:"confirmed"`
	IdempotencyKey                string `json:"idempotency_key"`
}
type KnowledgeDocumentPolicySnapshot struct {
	DocumentPolicyVersion int64 `json:"document_policy_version"`
	Enabled               bool  `json:"enabled"`
}
type KnowledgeDocumentDomainTarget struct {
	Kind     string `json:"kind"`
	DomainID string `json:"domain_id"`
}
type KnowledgeDocumentOriginalTarget struct {
	Kind                     string `json:"kind"`
	SourceID                 string `json:"source_id"`
	SourceRevisionID         string `json:"source_revision_id"`
	OriginalSHA256           string `json:"original_sha256"`
	ExpectedLifecycleVersion int64  `json:"expected_lifecycle_version"`
}
type KnowledgeDocumentTaskTarget struct {
	Kind   string `json:"kind"`
	TaskID string `json:"task_id"`
}
type KnowledgeDocumentTaskRequest struct {
	Target                         KnowledgeDocumentOriginalTarget `json:"target"`
	ExpectedPolicyVersion          int64                           `json:"expected_policy_version"`
	ExpectedDocumentPolicyVersion  int64                           `json:"expected_document_policy_version"`
	ExpectedActiveRepresentationID *string                         `json:"expected_active_representation_id"`
	ProfileID                      string                          `json:"profile_id"`
	ExpectedProducerSHA256         string                          `json:"expected_producer_sha256"`
	Confirmed                      bool                            `json:"confirmed"`
	IdempotencyKey                 string                          `json:"idempotency_key"`
}
type KnowledgeDocumentCancelRequest struct {
	ExpectedTaskRevision int64  `json:"expected_task_revision"`
	Confirmed            bool   `json:"confirmed"`
	IdempotencyKey       string `json:"idempotency_key"`
}
type KnowledgeDocumentPolicyReceipt struct {
	RecordKind      string                               `json:"record_kind"`
	CommandID       string                               `json:"command_id"`
	OwnerID         string                               `json:"owner_id"`
	DomainID        string                               `json:"domain_id"`
	Target          KnowledgeDocumentDomainTarget        `json:"target"`
	IdempotencyKey  string                               `json:"idempotency_key"`
	OriginalRequest KnowledgeDocumentPolicyChangeRequest `json:"original_request"`
	RequestSHA256   string                               `json:"request_sha256"`
	Before          KnowledgeDocumentPolicySnapshot      `json:"before"`
	After           KnowledgeDocumentPolicySnapshot      `json:"after"`
	Outcome         string                               `json:"outcome"`
	CompletedAt     string                               `json:"completed_at"`
	ModelCalls      int64                                `json:"model_calls"`
}
type KnowledgeDocumentTaskProgress struct {
	UnitKind       string `json:"unit_kind"`
	CompletedUnits int64  `json:"completed_units"`
	TotalUnits     *int64 `json:"total_units"`
}
type KnowledgeDocumentTaskDiagnostic struct {
	Code          string                    `json:"code"`
	Message       string                    `json:"message"`
	NativeLocator *KnowledgeDocumentLocator `json:"native_locator"`
}
type KnowledgeDocumentTask struct {
	RecordKind             string                            `json:"record_kind"`
	TaskID                 string                            `json:"task_id"`
	OwnerID                string                            `json:"owner_id"`
	DomainID               string                            `json:"domain_id"`
	OriginalRequest        KnowledgeDocumentTaskRequest      `json:"original_request"`
	Original               KnowledgeDocumentOriginalRef      `json:"original"`
	SourceSnapshot         KnowledgeDocumentSourceSnapshot   `json:"source_snapshot"`
	RequestSHA256          string                            `json:"request_sha256"`
	ProducerSHA256         string                            `json:"producer_sha256"`
	Revision               int64                             `json:"revision"`
	State                  string                            `json:"state"`
	Phase                  string                            `json:"phase"`
	AttemptCount           int64                             `json:"attempt_count"`
	CreatedAt              string                            `json:"created_at"`
	UpdatedAt              string                            `json:"updated_at"`
	DeadlineAt             string                            `json:"deadline_at"`
	CompletedAt            *string                           `json:"completed_at"`
	Progress               *KnowledgeDocumentTaskProgress    `json:"progress"`
	Diagnostics            []KnowledgeDocumentTaskDiagnostic `json:"diagnostics"`
	ResultRepresentationID *string                           `json:"result_representation_id"`
	FailureCode            *string                           `json:"failure_code"`
	ModelCalls             int64                             `json:"model_calls"`
	LocalInference         int64                             `json:"local_inference"`
	ExternalFetches        int64                             `json:"external_fetches"`
}
type KnowledgeDocumentCancelReceipt struct {
	RecordKind      string                         `json:"record_kind"`
	CommandID       string                         `json:"command_id"`
	OwnerID         string                         `json:"owner_id"`
	DomainID        string                         `json:"domain_id"`
	Target          KnowledgeDocumentTaskTarget    `json:"target"`
	IdempotencyKey  string                         `json:"idempotency_key"`
	OriginalRequest KnowledgeDocumentCancelRequest `json:"original_request"`
	RequestSHA256   string                         `json:"request_sha256"`
	BeforeState     string                         `json:"before_state"`
	AfterState      string                         `json:"after_state"`
	TaskRevision    int64                          `json:"task_revision"`
	Outcome         string                         `json:"outcome"`
	CompletedAt     string                         `json:"completed_at"`
	ModelCalls      int64                          `json:"model_calls"`
}
type KnowledgeDocumentTaskRejectionReceipt struct {
	RecordKind      string                          `json:"record_kind"`
	RejectionID     string                          `json:"rejection_id"`
	OwnerID         string                          `json:"owner_id"`
	DomainID        string                          `json:"domain_id"`
	IdempotencyKey  string                          `json:"idempotency_key"`
	RequestSHA256   string                          `json:"request_sha256"`
	Reason          string                          `json:"reason"`
	RecordedAt      string                          `json:"recorded_at"`
	Accepted        bool                            `json:"accepted"`
	ModelCalls      int64                           `json:"model_calls"`
	Operation       string                          `json:"operation"`
	Target          KnowledgeDocumentOriginalTarget `json:"target"`
	OriginalRequest KnowledgeDocumentTaskRequest    `json:"original_request"`
}
type KnowledgeDocumentPolicyRejectionReceipt struct {
	RecordKind      string                               `json:"record_kind"`
	RejectionID     string                               `json:"rejection_id"`
	OwnerID         string                               `json:"owner_id"`
	DomainID        string                               `json:"domain_id"`
	IdempotencyKey  string                               `json:"idempotency_key"`
	RequestSHA256   string                               `json:"request_sha256"`
	Reason          string                               `json:"reason"`
	RecordedAt      string                               `json:"recorded_at"`
	Accepted        bool                                 `json:"accepted"`
	ModelCalls      int64                                `json:"model_calls"`
	Operation       string                               `json:"operation"`
	Target          KnowledgeDocumentDomainTarget        `json:"target"`
	OriginalRequest KnowledgeDocumentPolicyChangeRequest `json:"original_request"`
}
type KnowledgeDocumentCancelRejectionReceipt struct {
	RecordKind      string                         `json:"record_kind"`
	RejectionID     string                         `json:"rejection_id"`
	OwnerID         string                         `json:"owner_id"`
	DomainID        string                         `json:"domain_id"`
	IdempotencyKey  string                         `json:"idempotency_key"`
	RequestSHA256   string                         `json:"request_sha256"`
	Reason          string                         `json:"reason"`
	RecordedAt      string                         `json:"recorded_at"`
	Accepted        bool                           `json:"accepted"`
	ModelCalls      int64                          `json:"model_calls"`
	Operation       string                         `json:"operation"`
	Target          KnowledgeDocumentTaskTarget    `json:"target"`
	OriginalRequest KnowledgeDocumentCancelRequest `json:"original_request"`
}
type KnowledgeDocumentHead struct {
	DomainID                string   `json:"domain_id"`
	SourceID                string   `json:"source_id"`
	ActiveRepresentationID  *string  `json:"active_representation_id"`
	ActiveSourceRevisionID  *string  `json:"active_source_revision_id"`
	CurrentSourceRevisionID string   `json:"current_source_revision_id"`
	CurrentLifecycleVersion int64    `json:"current_lifecycle_version"`
	FoundationPolicyVersion int64    `json:"foundation_policy_version"`
	DocumentPolicyVersion   int64    `json:"document_policy_version"`
	LatestTaskID            *string  `json:"latest_task_id"`
	Currentness             string   `json:"currentness"`
	Reasons                 []string `json:"reasons"`
	CheckedAt               string   `json:"checked_at"`
}
type KnowledgeDocumentRepresentation struct {
	RepresentationID        string                          `json:"representation_id"`
	OwnerID                 string                          `json:"owner_id"`
	DomainID                string                          `json:"domain_id"`
	Original                KnowledgeDocumentOriginalRef    `json:"original"`
	SourceSnapshot          KnowledgeDocumentSourceSnapshot `json:"source_snapshot"`
	FoundationPolicyVersion int64                           `json:"foundation_policy_version"`
	DocumentPolicyVersion   int64                           `json:"document_policy_version"`
	Format                  string                          `json:"format"`
	ProfileID               string                          `json:"profile_id"`
	Configuration           KnowledgeDocumentConfiguration  `json:"configuration"`
	Producer                KnowledgeDocumentProducer       `json:"producer"`
	ProducerSHA256          string                          `json:"producer_sha256"`
	Quality                 string                          `json:"quality"`
	RootDocumentNodeID      string                          `json:"root_document_node_id"`
	NodeCount               int64                           `json:"node_count"`
	PageCount               *int64                          `json:"page_count"`
	SlideCount              *int64                          `json:"slide_count"`
	UnsupportedBlockCount   int64                           `json:"unsupported_block_count"`
	TextCoverage            string                          `json:"text_coverage"`
	CoordinateSpace         string                          `json:"coordinate_space"`
	ReadingFormat           string                          `json:"reading_format"`
	ReadingTextSHA256       string                          `json:"reading_text_sha256"`
	ReadingByteLength       int64                           `json:"reading_byte_length"`
	StructureManifestSHA256 string                          `json:"structure_manifest_sha256"`
	PublishedAt             string                          `json:"published_at"`
	ModelCalls              int64                           `json:"model_calls"`
	LocalInference          int64                           `json:"local_inference"`
	ExternalFetches         int64                           `json:"external_fetches"`
	LexicalIndex            string                          `json:"lexical_index"`
	VectorIndex             string                          `json:"vector_index"`
}
type KnowledgeDocumentNodePage struct {
	RepresentationID string                  `json:"representation_id"`
	SourceID         string                  `json:"source_id"`
	SourceRevisionID string                  `json:"source_revision_id"`
	ParentID         string                  `json:"parent_id"`
	Items            []KnowledgeDocumentNode `json:"items"`
	NextCursor       *string                 `json:"next_cursor"`
}
type KnowledgeDocumentContentWindow struct {
	RepresentationID  string `json:"representation_id"`
	DocumentNodeID    string `json:"document_node_id"`
	SourceID          string `json:"source_id"`
	SourceRevisionID  string `json:"source_revision_id"`
	OriginalSHA256    string `json:"original_sha256"`
	ReadingTextSHA256 string `json:"reading_text_sha256"`
	NodeTextSHA256    string `json:"node_text_sha256"`
	FragmentSHA256    string `json:"fragment_sha256"`
	CoordinateSpace   string `json:"coordinate_space"`
	TextStatus        string `json:"text_status"`
	StartByte         int64  `json:"start_byte"`
	EndByte           int64  `json:"end_byte"`
	Text              string `json:"text"`
	Complete          bool   `json:"complete"`
	NextStartByte     *int64 `json:"next_start_byte"`
}
type KnowledgeDocumentError struct {
	Code                             string  `json:"code"`
	Message                          string  `json:"message"`
	ContractVersion                  string  `json:"contract_version"`
	DocumentStructureContractVersion string  `json:"document_structure_contract_version"`
	RequestID                        string  `json:"request_id"`
	Data                             any     `json:"data"`
	OutcomeKnown                     bool    `json:"outcome_known"`
	RejectionID                      *string `json:"rejection_id"`
}
type KnowledgeDocumentCapabilitiesResponse struct {
	Code                             int64                         `json:"code"`
	Message                          string                        `json:"message"`
	ContractVersion                  string                        `json:"contract_version"`
	DocumentStructureContractVersion string                        `json:"document_structure_contract_version"`
	RequestID                        string                        `json:"request_id"`
	Data                             KnowledgeDocumentCapabilities `json:"data"`
}
type KnowledgeDocumentPolicyResponse struct {
	Code                             int64                   `json:"code"`
	Message                          string                  `json:"message"`
	ContractVersion                  string                  `json:"contract_version"`
	DocumentStructureContractVersion string                  `json:"document_structure_contract_version"`
	RequestID                        string                  `json:"request_id"`
	Data                             KnowledgeDocumentPolicy `json:"data"`
}
type KnowledgeDocumentPolicyReceiptResponse struct {
	Code                             int64                          `json:"code"`
	Message                          string                         `json:"message"`
	ContractVersion                  string                         `json:"contract_version"`
	DocumentStructureContractVersion string                         `json:"document_structure_contract_version"`
	RequestID                        string                         `json:"request_id"`
	Data                             KnowledgeDocumentPolicyReceipt `json:"data"`
}
type KnowledgeDocumentTaskResponse struct {
	Code                             int64                 `json:"code"`
	Message                          string                `json:"message"`
	ContractVersion                  string                `json:"contract_version"`
	DocumentStructureContractVersion string                `json:"document_structure_contract_version"`
	RequestID                        string                `json:"request_id"`
	Data                             KnowledgeDocumentTask `json:"data"`
}
type KnowledgeDocumentOperationRecordResponse struct {
	Code                             int64                            `json:"code"`
	Message                          string                           `json:"message"`
	ContractVersion                  string                           `json:"contract_version"`
	DocumentStructureContractVersion string                           `json:"document_structure_contract_version"`
	RequestID                        string                           `json:"request_id"`
	Data                             KnowledgeDocumentOperationRecord `json:"data"`
}
type KnowledgeDocumentCancelReceiptResponse struct {
	Code                             int64                          `json:"code"`
	Message                          string                         `json:"message"`
	ContractVersion                  string                         `json:"contract_version"`
	DocumentStructureContractVersion string                         `json:"document_structure_contract_version"`
	RequestID                        string                         `json:"request_id"`
	Data                             KnowledgeDocumentCancelReceipt `json:"data"`
}
type KnowledgeDocumentHeadResponse struct {
	Code                             int64                 `json:"code"`
	Message                          string                `json:"message"`
	ContractVersion                  string                `json:"contract_version"`
	DocumentStructureContractVersion string                `json:"document_structure_contract_version"`
	RequestID                        string                `json:"request_id"`
	Data                             KnowledgeDocumentHead `json:"data"`
}
type KnowledgeDocumentRepresentationResponse struct {
	Code                             int64                           `json:"code"`
	Message                          string                          `json:"message"`
	ContractVersion                  string                          `json:"contract_version"`
	DocumentStructureContractVersion string                          `json:"document_structure_contract_version"`
	RequestID                        string                          `json:"request_id"`
	Data                             KnowledgeDocumentRepresentation `json:"data"`
}
type KnowledgeDocumentNodePageResponse struct {
	Code                             int64                     `json:"code"`
	Message                          string                    `json:"message"`
	ContractVersion                  string                    `json:"contract_version"`
	DocumentStructureContractVersion string                    `json:"document_structure_contract_version"`
	RequestID                        string                    `json:"request_id"`
	Data                             KnowledgeDocumentNodePage `json:"data"`
}
type KnowledgeDocumentNodeResponse struct {
	Code                             int64                 `json:"code"`
	Message                          string                `json:"message"`
	ContractVersion                  string                `json:"contract_version"`
	DocumentStructureContractVersion string                `json:"document_structure_contract_version"`
	RequestID                        string                `json:"request_id"`
	Data                             KnowledgeDocumentNode `json:"data"`
}
type KnowledgeDocumentContentWindowResponse struct {
	Code                             int64                          `json:"code"`
	Message                          string                         `json:"message"`
	ContractVersion                  string                         `json:"contract_version"`
	DocumentStructureContractVersion string                         `json:"document_structure_contract_version"`
	RequestID                        string                         `json:"request_id"`
	Data                             KnowledgeDocumentContentWindow `json:"data"`
}
type KnowledgeDocumentOperationQuery struct {
	Kind           string `json:"kind"`
	IdempotencyKey string `json:"idempotency_key"`
}
type KnowledgeDocumentNodeQuery struct {
	ParentID string  `json:"parent_id"`
	Limit    *int64  `json:"limit,omitempty"`
	Cursor   *string `json:"cursor,omitempty"`
}
type KnowledgeDocumentContentQuery struct {
	StartByte *int64 `json:"start_byte,omitempty"`
	EndByte   *int64 `json:"end_byte,omitempty"`
}
type KnowledgeDocumentCapabilitiesExecutionQuota struct {
	DomainRunning  int64 `json:"domain_running"`
	DomainQueued   int64 `json:"domain_queued"`
	SharedWithCode bool  `json:"shared_with_code"`
}
type KnowledgeDocumentCapabilitiesExecution struct {
	Mode                 string `json:"mode"`
	CancelAction         string `json:"cancel_action"`
	CancellationComplete string `json:"cancellation_complete"`
	HardMemoryIsolation  bool   `json:"hard_memory_isolation"`
}

type KnowledgeDocumentRejectionReceipt struct {
	RecordKind      string          `json:"record_kind"`
	RejectionID     string          `json:"rejection_id"`
	OwnerID         string          `json:"owner_id"`
	DomainID        string          `json:"domain_id"`
	Operation       string          `json:"operation"`
	Target          json.RawMessage `json:"target"`
	IdempotencyKey  string          `json:"idempotency_key"`
	OriginalRequest json.RawMessage `json:"original_request"`
	RequestSHA256   string          `json:"request_sha256"`
	Reason          string          `json:"reason"`
	RecordedAt      string          `json:"recorded_at"`
	Accepted        bool            `json:"accepted"`
	ModelCalls      int64           `json:"model_calls"`
}

// KnowledgeDocumentOperationRecord retains the original union JSON without adding a wrapper on the wire.
type KnowledgeDocumentOperationRecord struct {
	Raw json.RawMessage `json:"-"`
}

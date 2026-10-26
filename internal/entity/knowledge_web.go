// Code generated from the frozen C2 0.6.0 schema; DO NOT EDIT.

package entity

const KnowledgeWebContractVersion = "0.6.0"

type KnowledgeWebNetworkScope struct {
	Enabled        bool     `json:"enabled"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// KnowledgeWebProducerVersions is the frozen C2 0.6.0 ProducerVersions projection.
type KnowledgeWebProducerVersions struct {
	NormalizerVersion     string `json:"normalizer_version"`
	ConverterVersion      string `json:"converter_version"`
	SanitizerVersion      string `json:"sanitizer_version"`
	ResourcePolicyVersion string `json:"resource_policy_version"`
}

// KnowledgeWebCreateRegistration is the frozen C2 0.6.0 CreateRegistration projection.
type KnowledgeWebCreateRegistration struct {
	RegistrationKey       string                           `json:"registration_key"`
	Name                  string                           `json:"name"`
	SourceKind            string                           `json:"source_kind"`
	OriginURL             string                           `json:"origin_url"`
	Producer              string                           `json:"producer"`
	DefaultClassification KnowledgeDirectoryClassification `json:"default_classification"`
	NetworkScope          KnowledgeWebNetworkScope         `json:"network_scope"`
}

// KnowledgeWebRegistration is the frozen C2 0.6.0 Registration projection.
type KnowledgeWebRegistration struct {
	RegistrationKey       string                           `json:"registration_key"`
	Name                  string                           `json:"name"`
	SourceKind            string                           `json:"source_kind"`
	OriginURL             string                           `json:"origin_url"`
	Producer              string                           `json:"producer"`
	DefaultClassification KnowledgeDirectoryClassification `json:"default_classification"`
	NetworkScope          KnowledgeWebNetworkScope         `json:"network_scope"`
	DomainID              string                           `json:"domain_id"`
	OwnerID               string                           `json:"owner_id"`
	RegistrationID        string                           `json:"registration_id"`
	ConfigSHA256          string                           `json:"config_sha256"`
	ScopeSHA256           string                           `json:"scope_sha256"`
	CreatedAt             string                           `json:"created_at"`
	PolicyVersion         int64                            `json:"policy_version"`
}

// KnowledgeWebTarget is the frozen C2 0.6.0 Target projection.
type KnowledgeWebTarget struct {
	DomainID              string                            `json:"domain_id"`
	RegistrationID        string                            `json:"registration_id"`
	EntryKey              string                            `json:"entry_key"`
	SourceID              *string                           `json:"source_id"`
	CurrentRevisionID     *string                           `json:"current_revision_id"`
	CurrentInputSHA256    *string                           `json:"current_input_sha256"`
	CurrentClassification *KnowledgeDirectoryClassification `json:"current_classification"`
}

// KnowledgeWebCreateLocalCapture is the frozen C2 0.6.0 CreateLocalCapture projection.
type KnowledgeWebCreateLocalCapture struct {
	RegistrationConfigSHA256 string  `json:"registration_config_sha256"`
	IdempotencyKey           string  `json:"idempotency_key"`
	AcquisitionMode          string  `json:"acquisition_mode"`
	OriginalFilename         string  `json:"original_filename"`
	DeclaredByteLength       int64   `json:"declared_byte_length"`
	DeclaredMediaType        string  `json:"declared_media_type"`
	ExpectedSHA256           *string `json:"expected_sha256"`
	DeclaredFinalURL         *string `json:"declared_final_url"`
	ClaimedCapturedAt        *string `json:"claimed_captured_at"`
}

// KnowledgeWebCreateFetchCapture is the frozen C2 0.6.0 CreateFetchCapture projection.
type KnowledgeWebCreateFetchCapture struct {
	RegistrationConfigSHA256 string `json:"registration_config_sha256"`
	IdempotencyKey           string `json:"idempotency_key"`
	AcquisitionMode          string `json:"acquisition_mode"`
}

// KnowledgeWebFailure is the frozen C2 0.6.0 Failure projection.
type KnowledgeWebFailure struct {
	Stage              string `json:"stage"`
	MachineCode        string `json:"machine_code"`
	UpstreamHTTPStatus *int64 `json:"upstream_http_status"`
	OutcomeKnown       bool   `json:"outcome_known"`
}

// KnowledgeWebRedirect is the frozen C2 0.6.0 Redirect projection.
type KnowledgeWebRedirect struct {
	FromURL string `json:"from_url"`
	ToURL   string `json:"to_url"`
	Status  int64  `json:"status"`
}

// KnowledgeWebProvenance is the frozen C2 0.6.0 Provenance projection.
type KnowledgeWebProvenance struct {
	Kind                    string                 `json:"kind"`
	RequestedURL            string                 `json:"requested_url"`
	FinalURL                *string                `json:"final_url"`
	ClaimedCapturedAt       *string                `json:"claimed_captured_at"`
	ServerReceivedAt        *string                `json:"server_received_at"`
	FetchStartedAt          *string                `json:"fetch_started_at"`
	FetchCompletedAt        *string                `json:"fetch_completed_at"`
	UpstreamHTTPStatus      *int64                 `json:"upstream_http_status"`
	RedirectChain           []KnowledgeWebRedirect `json:"redirect_chain"`
	ResponseContentType     *string                `json:"response_content_type"`
	ResponseContentEncoding *string                `json:"response_content_encoding"`
	ResponseEtag            *string                `json:"response_etag"`
	ResponseLastModified    *string                `json:"response_last_modified"`
}

// KnowledgeWebCapture is the frozen C2 0.6.0 Capture projection.
type KnowledgeWebCapture struct {
	DomainID                 string                       `json:"domain_id"`
	OwnerID                  string                       `json:"owner_id"`
	RegistrationID           string                       `json:"registration_id"`
	CaptureID                string                       `json:"capture_id"`
	IdempotencyKey           string                       `json:"idempotency_key"`
	RequestSHA256            string                       `json:"request_sha256"`
	RegistrationConfigSHA256 string                       `json:"registration_config_sha256"`
	ScopeSHA256              string                       `json:"scope_sha256"`
	AcquisitionMode          string                       `json:"acquisition_mode"`
	State                    string                       `json:"state"`
	Revision                 int64                        `json:"revision"`
	PolicyVersion            int64                        `json:"policy_version"`
	ProducerVersions         KnowledgeWebProducerVersions `json:"producer_versions"`
	CreatedAt                string                       `json:"created_at"`
	UpdatedAt                string                       `json:"updated_at"`
	ExpiresAt                string                       `json:"expires_at"`
	AttemptStartedAt         *string                      `json:"attempt_started_at"`
	AttemptDeadlineAt        *string                      `json:"attempt_deadline_at"`
	OriginalFilename         *string                      `json:"original_filename"`
	DeclaredByteLength       *int64                       `json:"declared_byte_length"`
	DeclaredMediaType        *string                      `json:"declared_media_type"`
	ExpectedSHA256           *string                      `json:"expected_sha256"`
	ReceivedBytes            int64                        `json:"received_bytes"`
	RawSHA256                *string                      `json:"raw_sha256"`
	RawByteLength            *int64                       `json:"raw_byte_length"`
	MediaType                *string                      `json:"media_type"`
	Provenance               KnowledgeWebProvenance       `json:"provenance"`
	ObservationID            *string                      `json:"observation_id"`
	Failure                  *KnowledgeWebFailure         `json:"failure"`
}

// KnowledgeWebAcquire is the frozen C2 0.6.0 Acquire projection.
type KnowledgeWebAcquire struct {
	ScopeSHA256             string `json:"scope_sha256"`
	ExpectedCaptureRevision int64  `json:"expected_capture_revision"`
	Confirmed               bool   `json:"confirmed"`
}

// KnowledgeWebObserve is the frozen C2 0.6.0 Observe projection.
type KnowledgeWebObserve struct {
	NormalizerVersion string `json:"normalizer_version"`
}

// KnowledgeWebObservation is the frozen C2 0.6.0 Observation projection.
type KnowledgeWebObservation struct {
	DomainID                 string                       `json:"domain_id"`
	RegistrationID           string                       `json:"registration_id"`
	CaptureID                string                       `json:"capture_id"`
	ObservationID            string                       `json:"observation_id"`
	CaptureSHA256            string                       `json:"capture_sha256"`
	ProducerVersions         KnowledgeWebProducerVersions `json:"producer_versions"`
	State                    string                       `json:"state"`
	Revision                 int64                        `json:"revision"`
	ObservationSHA256        *string                      `json:"observation_sha256"`
	CreatedAt                string                       `json:"created_at"`
	ExpiresAt                string                       `json:"expires_at"`
	InputEntryCount          *int64                       `json:"input_entry_count"`
	EntryCount               *int64                       `json:"entry_count"`
	EligibleCount            *int64                       `json:"eligible_count"`
	DuplicateCount           *int64                       `json:"duplicate_count"`
	CaptureTraversalComplete bool                         `json:"capture_traversal_complete"`
	DecodedBytes             int64                        `json:"decoded_bytes"`
	ReadingBytes             int64                        `json:"reading_bytes"`
	MetadataBytes            int64                        `json:"metadata_bytes"`
	ReferenceBytes           int64                        `json:"reference_bytes"`
	ReferenceCount           int64                        `json:"reference_count"`
	Warnings                 []string                     `json:"warnings"`
	Failure                  *KnowledgeWebFailure         `json:"failure"`
}

// KnowledgeWebRepresentation is the frozen C2 0.6.0 Representation projection.
type KnowledgeWebRepresentation struct {
	Kind                  string  `json:"kind"`
	MediaType             string  `json:"media_type"`
	Encoding              string  `json:"encoding"`
	ByteLength            int64   `json:"byte_length"`
	SHA256                string  `json:"sha256"`
	ConverterVersion      string  `json:"converter_version"`
	SanitizerVersion      *string `json:"sanitizer_version"`
	ResourcePolicyVersion *string `json:"resource_policy_version"`
}

// KnowledgeWebAuthor is the frozen C2 0.6.0 Author projection.
type KnowledgeWebAuthor struct {
	Name  string  `json:"name"`
	URI   *string `json:"uri"`
	Email *string `json:"email"`
}

// KnowledgeWebMetadata is the frozen C2 0.6.0 Metadata projection.
type KnowledgeWebMetadata struct {
	Title             string               `json:"title"`
	Authors           []KnowledgeWebAuthor `json:"authors"`
	Categories        []string             `json:"categories"`
	Published         *string              `json:"published"`
	Updated           *string              `json:"updated"`
	CanonicalDeclared *string              `json:"canonical_declared"`
	CanonicalResolved *string              `json:"canonical_resolved"`
	EffectiveBase     string               `json:"effective_base"`
	BodyCompleteness  string               `json:"body_completeness"`
}

// KnowledgeWebEntrySummary is the frozen C2 0.6.0 EntrySummary projection.
type KnowledgeWebEntrySummary struct {
	EntryKey          string  `json:"entry_key"`
	IdentityBasis     string  `json:"identity_basis"`
	IdentityStrength  string  `json:"identity_strength"`
	Title             string  `json:"title"`
	BodyCompleteness  string  `json:"body_completeness"`
	Eligible          bool    `json:"eligible"`
	IneligibleReason  *string `json:"ineligible_reason"`
	SourceID          *string `json:"source_id"`
	CurrentRevisionID *string `json:"current_revision_id"`
	DecodedByteLength int64   `json:"decoded_byte_length"`
	ReadingByteLength int64   `json:"reading_byte_length"`
	SourceInputSHA256 *string `json:"source_input_sha256"`
	FirstPosition     int64   `json:"first_position"`
}

// KnowledgeWebEntry is the frozen C2 0.6.0 Entry projection.
type KnowledgeWebEntry struct {
	EntryKey          string                       `json:"entry_key"`
	IdentityBasis     string                       `json:"identity_basis"`
	IdentityStrength  string                       `json:"identity_strength"`
	Title             string                       `json:"title"`
	BodyCompleteness  string                       `json:"body_completeness"`
	Eligible          bool                         `json:"eligible"`
	IneligibleReason  *string                      `json:"ineligible_reason"`
	SourceID          *string                      `json:"source_id"`
	CurrentRevisionID *string                      `json:"current_revision_id"`
	DecodedByteLength int64                        `json:"decoded_byte_length"`
	ReadingByteLength int64                        `json:"reading_byte_length"`
	SourceInputSHA256 *string                      `json:"source_input_sha256"`
	FirstPosition     int64                        `json:"first_position"`
	ObservationID     string                       `json:"observation_id"`
	CaptureID         string                       `json:"capture_id"`
	IdentityValue     string                       `json:"identity_value"`
	Positions         []int64                      `json:"positions"`
	Metadata          KnowledgeWebMetadata         `json:"metadata"`
	MetadataSHA256    string                       `json:"metadata_sha256"`
	ReferencesSHA256  string                       `json:"references_sha256"`
	ReferenceCount    int64                        `json:"reference_count"`
	Representations   []KnowledgeWebRepresentation `json:"representations"`
	ProducerVersions  KnowledgeWebProducerVersions `json:"producer_versions"`
}

// KnowledgeWebReference is the frozen C2 0.6.0 Reference projection.
type KnowledgeWebReference struct {
	ReferenceID       string  `json:"reference_id"`
	Relation          string  `json:"relation"`
	RawHref           string  `json:"raw_href"`
	BaseURL           string  `json:"base_url"`
	ResolvedURL       *string `json:"resolved_url"`
	Fragment          *string `json:"fragment"`
	Label             string  `json:"label"`
	DeclaredMediaType *string `json:"declared_media_type"`
	OpenAllowed       bool    `json:"open_allowed"`
}

// KnowledgeWebSelection is the frozen C2 0.6.0 Selection projection.
type KnowledgeWebSelection struct {
	EntryKey           string                           `json:"entry_key"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	ExpectedRevisionID *string                          `json:"expected_revision_id"`
}

// KnowledgeWebCreatePreview is the frozen C2 0.6.0 CreatePreview projection.
type KnowledgeWebCreatePreview struct {
	CaptureID         string                  `json:"capture_id"`
	ObservationID     string                  `json:"observation_id"`
	ObservationSHA256 string                  `json:"observation_sha256"`
	IdempotencyKey    string                  `json:"idempotency_key"`
	Selection         []KnowledgeWebSelection `json:"selection"`
}

// KnowledgeWebPreviewItem is the frozen C2 0.6.0 PreviewItem projection.
type KnowledgeWebPreviewItem struct {
	EntryKey           string                           `json:"entry_key"`
	Title              string                           `json:"title"`
	SourceID           *string                          `json:"source_id"`
	ExpectedRevisionID *string                          `json:"expected_revision_id"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	SourceInputSHA256  string                           `json:"source_input_sha256"`
	Action             string                           `json:"action"`
	DecodedSHA256      string                           `json:"decoded_sha256"`
	ReadingSHA256      string                           `json:"reading_sha256"`
	BodyCompleteness   string                           `json:"body_completeness"`
	Acknowledgement    string                           `json:"acknowledgement"`
}

// KnowledgeWebPreview is the frozen C2 0.6.0 Preview projection.
type KnowledgeWebPreview struct {
	DomainID                 string                       `json:"domain_id"`
	OwnerID                  string                       `json:"owner_id"`
	RegistrationID           string                       `json:"registration_id"`
	CaptureID                string                       `json:"capture_id"`
	CaptureSHA256            string                       `json:"capture_sha256"`
	ObservationID            string                       `json:"observation_id"`
	ObservationSHA256        string                       `json:"observation_sha256"`
	PreviewID                string                       `json:"preview_id"`
	IdempotencyKey           string                       `json:"idempotency_key"`
	RequestSHA256            string                       `json:"request_sha256"`
	RegistrationConfigSHA256 string                       `json:"registration_config_sha256"`
	ScopeSHA256              string                       `json:"scope_sha256"`
	ProducerVersions         KnowledgeWebProducerVersions `json:"producer_versions"`
	PolicyVersion            int64                        `json:"policy_version"`
	ManifestSHA256           string                       `json:"manifest_sha256"`
	SelectionSHA256          string                       `json:"selection_sha256"`
	SelectedCount            int64                        `json:"selected_count"`
	SelectedByteLength       int64                        `json:"selected_byte_length"`
	State                    string                       `json:"state"`
	Revision                 int64                        `json:"revision"`
	CreatedAt                string                       `json:"created_at"`
	ExpiresAt                string                       `json:"expires_at"`
	ImportTaskID             *string                      `json:"import_task_id"`
}

// KnowledgeWebConfirm is the frozen C2 0.6.0 Confirm projection.
type KnowledgeWebConfirm struct {
	PreviewID             string   `json:"preview_id"`
	ManifestSHA256        string   `json:"manifest_sha256"`
	IdempotencyKey        string   `json:"idempotency_key"`
	Confirmed             bool     `json:"confirmed"`
	AcknowledgedEntryKeys []string `json:"acknowledged_entry_keys"`
}

// KnowledgeWebReceiptItem is the frozen C2 0.6.0 ReceiptItem projection.
type KnowledgeWebReceiptItem struct {
	EntryKey          string                           `json:"entry_key"`
	SourceID          string                           `json:"source_id"`
	SourceRevisionID  string                           `json:"source_revision_id"`
	Action            string                           `json:"action"`
	Classification    KnowledgeDirectoryClassification `json:"classification"`
	SourceInputSHA256 string                           `json:"source_input_sha256"`
	DecodedSHA256     string                           `json:"decoded_sha256"`
	ReadingSHA256     string                           `json:"reading_sha256"`
}

// KnowledgeWebReceipt is the frozen C2 0.6.0 Receipt projection.
type KnowledgeWebReceipt struct {
	DomainID             string                       `json:"domain_id"`
	OwnerID              string                       `json:"owner_id"`
	RegistrationID       string                       `json:"registration_id"`
	CaptureID            string                       `json:"capture_id"`
	CaptureSHA256        string                       `json:"capture_sha256"`
	ObservationID        string                       `json:"observation_id"`
	ObservationSHA256    string                       `json:"observation_sha256"`
	PreviewID            string                       `json:"preview_id"`
	PreviewRequestSHA256 string                       `json:"preview_request_sha256"`
	ProducerVersions     KnowledgeWebProducerVersions `json:"producer_versions"`
	PolicyVersion        int64                        `json:"policy_version"`
	ImportTaskID         string                       `json:"import_task_id"`
	IdempotencyKey       string                       `json:"idempotency_key"`
	RequestSHA256        string                       `json:"request_sha256"`
	ManifestSHA256       string                       `json:"manifest_sha256"`
	SelectionSHA256      string                       `json:"selection_sha256"`
	SelectedCount        int64                        `json:"selected_count"`
	Status               string                       `json:"status"`
	SavedCount           int64                        `json:"saved_count"`
	UnchangedCount       int64                        `json:"unchanged_count"`
	IndexStatus          string                       `json:"index_status"`
	ModelCalls           int64                        `json:"model_calls"`
	CreatedAt            string                       `json:"created_at"`
}

// KnowledgeWebRevision is the frozen C2 0.6.0 WebRevision projection.
type KnowledgeWebRevision struct {
	DomainID           string                           `json:"domain_id"`
	RegistrationID     string                           `json:"registration_id"`
	SourceID           string                           `json:"source_id"`
	SourceRevisionID   string                           `json:"source_revision_id"`
	EntryKey           string                           `json:"entry_key"`
	IdentityBasis      string                           `json:"identity_basis"`
	IdentityValue      string                           `json:"identity_value"`
	RelativePath       string                           `json:"relative_path"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	RepresentationKind string                           `json:"representation_kind"`
	RootNodeID         *string                          `json:"root_node_id"`
	SourceInputSHA256  string                           `json:"source_input_sha256"`
	CaptureID          string                           `json:"capture_id"`
	CaptureSHA256      string                           `json:"capture_sha256"`
	ObservationID      string                           `json:"observation_id"`
	Positions          []int64                          `json:"positions"`
	Metadata           KnowledgeWebMetadata             `json:"metadata"`
	MetadataSHA256     string                           `json:"metadata_sha256"`
	ReferencesSHA256   string                           `json:"references_sha256"`
	ReferenceCount     int64                            `json:"reference_count"`
	Representations    []KnowledgeWebRepresentation     `json:"representations"`
	ProducerVersions   KnowledgeWebProducerVersions     `json:"producer_versions"`
	TextAvailable      bool                             `json:"text_available"`
	StructureStatus    string                           `json:"structure_status"`
	FullTextStatus     string                           `json:"full_text_status"`
	VectorStatus       string                           `json:"vector_status"`
	IsCurrent          bool                             `json:"is_current"`
	CreatedAt          string                           `json:"created_at"`
}

// KnowledgeWebWindow is the frozen C2 0.6.0 Window projection.
type KnowledgeWebWindow struct {
	DomainID         string                     `json:"domain_id"`
	CaptureID        string                     `json:"capture_id"`
	ObservationID    string                     `json:"observation_id"`
	EntryKey         string                     `json:"entry_key"`
	SourceRevisionID *string                    `json:"source_revision_id"`
	Representation   KnowledgeWebRepresentation `json:"representation"`
	StartByte        int64                      `json:"start_byte"`
	EndByte          int64                      `json:"end_byte"`
	NextStartByte    *int64                     `json:"next_start_byte"`
	Text             string                     `json:"text"`
}

// KnowledgeWebCapabilities is the frozen C2 0.6.0 Capabilities projection.
type KnowledgeWebCapabilities struct {
	ContractVersion       string                       `json:"contract_version"`
	SourceContractVersion string                       `json:"source_contract_version"`
	RuntimeReady          bool                         `json:"runtime_ready"`
	AcquisitionModes      []string                     `json:"acquisition_modes"`
	Formats               []string                     `json:"formats"`
	ProducerVersions      KnowledgeWebProducerVersions `json:"producer_versions"`
	Limits                struct {
		MaxRawBytes              int64 `json:"max_raw_bytes"`
		MaxCaptureEntries        int64 `json:"max_capture_entries"`
		MaxParseDepth            int64 `json:"max_parse_depth"`
		MaxParseNodes            int64 `json:"max_parse_nodes"`
		MaxSelected              int64 `json:"max_selected"`
		MaxSelectionBytes        int64 `json:"max_selection_bytes"`
		MaxEntryDecodedBytes     int64 `json:"max_entry_decoded_bytes"`
		MaxEntryReadingBytes     int64 `json:"max_entry_reading_bytes"`
		MaxEntryMetadataBytes    int64 `json:"max_entry_metadata_bytes"`
		MaxEntryReferences       int64 `json:"max_entry_references"`
		MaxEntryReferenceBytes   int64 `json:"max_entry_reference_bytes"`
		MaxReferenceJSONBytes    int64 `json:"max_reference_json_bytes"`
		MaxCaptureDecodedBytes   int64 `json:"max_capture_decoded_bytes"`
		MaxCaptureReadingBytes   int64 `json:"max_capture_reading_bytes"`
		MaxCaptureMetadataBytes  int64 `json:"max_capture_metadata_bytes"`
		MaxCaptureReferences     int64 `json:"max_capture_references"`
		MaxCaptureReferenceBytes int64 `json:"max_capture_reference_bytes"`
		MaxJSONBytes             int64 `json:"max_json_bytes"`
		MaxPageItems             int64 `json:"max_page_items"`
		MaxWindowBytes           int64 `json:"max_window_bytes"`
		CaptureTTLSeconds        int64 `json:"capture_ttl_seconds"`
		PreviewTTLSeconds        int64 `json:"preview_ttl_seconds"`
		UploadTimeoutSeconds     int64 `json:"upload_timeout_seconds"`
		FetchTimeoutSeconds      int64 `json:"fetch_timeout_seconds"`
		ParseTimeoutSeconds      int64 `json:"parse_timeout_seconds"`
		LeaseSeconds             int64 `json:"lease_seconds"`
		RenewSeconds             int64 `json:"renew_seconds"`
		MaxOrigins               int64 `json:"max_origins"`
		MaxRedirects             int64 `json:"max_redirects"`
		MaxHeaderBytes           int64 `json:"max_header_bytes"`
		MaxOpenCaptures          int64 `json:"max_open_captures"`
		MaxReservedRawBytes      int64 `json:"max_reserved_raw_bytes"`
		MaxActivePerOwnerDomain  int64 `json:"max_active_per_owner_domain"`
		MaxActiveDeployment      int64 `json:"max_active_deployment"`
	} `json:"limits"`
	NetworkConfirmationRequired bool  `json:"network_confirmation_required"`
	NetworkRetriesPerCapture    int64 `json:"network_retries_per_capture"`
	AutomaticIngestion          bool  `json:"automatic_ingestion"`
	AutomaticPrune              bool  `json:"automatic_prune"`
	ModelCalls                  int64 `json:"model_calls"`
}

// KnowledgeWebEntryPage is the frozen C2 0.6.0 EntryPage projection.
type KnowledgeWebEntryPage struct {
	Items      []KnowledgeWebEntrySummary `json:"items"`
	NextCursor *string                    `json:"next_cursor"`
}

// KnowledgeWebReferencePage is the frozen C2 0.6.0 ReferencePage projection.
type KnowledgeWebReferencePage struct {
	Items      []KnowledgeWebReference `json:"items"`
	NextCursor *string                 `json:"next_cursor"`
}

// KnowledgeWebPreviewItemPage is the frozen C2 0.6.0 PreviewItemPage projection.
type KnowledgeWebPreviewItemPage struct {
	Items      []KnowledgeWebPreviewItem `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
}

// KnowledgeWebReceiptItemPage is the frozen C2 0.6.0 ReceiptItemPage projection.
type KnowledgeWebReceiptItemPage struct {
	Items      []KnowledgeWebReceiptItem `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
}

// KnowledgeWebRevisionPage is the frozen C2 0.6.0 RevisionPage projection.
type KnowledgeWebRevisionPage struct {
	Items      []KnowledgeWebRevision `json:"items"`
	NextCursor *string                `json:"next_cursor"`
}

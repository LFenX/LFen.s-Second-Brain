package entity

const KnowledgeContentLexicalProtocol = "content_lexical/0.2.0"
const KnowledgeLocalLexicalProtocol = "local_lexical/0.1.0"

type KnowledgeContentLexicalSelection struct {
	Kind    string                   `json:"kind"`
	Targets []KnowledgeContentTarget `json:"targets"`
}

type KnowledgeContentLexicalPolicy struct {
	DomainID          string `json:"domain_id"`
	LocalVersion      int64  `json:"local_policy_version"`
	ContentVersion    int64  `json:"content_policy_version"`
	FoundationVersion int64  `json:"foundation_policy_version"`
	LocalEnabled      bool   `json:"local_lexical_enabled"`
	ContentEnabled    bool   `json:"content_lexical_enabled"`
	Enabled           bool   `json:"enabled"`
	Mode              string `json:"mode"`
}

type KnowledgeContentLexicalProfile struct {
	SHA256          string   `json:"profile_sha256"`
	ProducerSHA256  string   `json:"tokenizer_producer_sha256"`
	Language        string   `json:"language"`
	Strategies      []string `json:"strategies"`
	Engine          string   `json:"engine"`
	Execution       string   `json:"execution_profile"`
	MaxUnitBytes    int64    `json:"max_unit_bytes"`
	ParentExpansion string   `json:"parent_expansion"`
	ModelsEnabled   bool     `json:"models_enabled"`
	RemoteFetches   bool     `json:"remote_fetches"`
}

type KnowledgeContentLexicalCoverage struct {
	Target              KnowledgeContentTarget `json:"target"`
	Scope               string                 `json:"scope"`
	CoordinateSpace     string                 `json:"coordinate_space"`
	ReadingSHA256       string                 `json:"reading_text_sha256"`
	ExpectedBytes       int64                  `json:"expected_reading_bytes"`
	CoveredBytes        int64                  `json:"covered_reading_bytes"`
	IndexedUnits        int64                  `json:"indexed_unit_count"`
	ReadingTextComplete bool                   `json:"reading_text_complete"`
	RepresentationState string                 `json:"representation_state"`
	UnsearchableNodes   int64                  `json:"unsearchable_node_count"`
	OriginalCoverage    string                 `json:"original_coverage"`
	TextIndexState      string                 `json:"text_index_state"`
}

type KnowledgeContentLexicalEvidence struct {
	Version             string                             `json:"evidence_version"`
	Index               KnowledgeLexicalIndexRef           `json:"index_ref"`
	RowID               string                             `json:"index_row_id"`
	Address             KnowledgeContentAddress            `json:"address"`
	CoordinateSpace     string                             `json:"coordinate_space"`
	ReadingSHA256       string                             `json:"reading_text_sha256"`
	Range               KnowledgeContentRange              `json:"range"`
	Locator             KnowledgeContentLocator            `json:"locator"`
	OriginalPosition    *KnowledgeDocumentOriginalPosition `json:"original_position"`
	CoordinatePrecision string                             `json:"coordinate_precision"`
	Assembly            string                             `json:"assembly"`
}

type KnowledgeContentLexicalInspectRequest struct {
	Version   string                           `json:"contract_version"`
	Selection KnowledgeContentLexicalSelection `json:"selection"`
}

type KnowledgeContentLexicalCreateRequest struct {
	Version                   string                           `json:"contract_version"`
	Key                       string                           `json:"idempotency_key"`
	ExpectedLocalVersion      int64                            `json:"expected_local_policy_version"`
	ExpectedContentVersion    int64                            `json:"expected_content_policy_version"`
	ExpectedFoundationVersion int64                            `json:"expected_foundation_policy_version"`
	ExpectedProfileSHA256     string                           `json:"expected_profile_sha256"`
	Selection                 KnowledgeContentLexicalSelection `json:"selection"`
}

type KnowledgeContentLexicalJobRequest struct {
	Version string `json:"contract_version"`
	ID      string `json:"job_id"`
}

type KnowledgeContentLexicalSearchRequest struct {
	Version   string                           `json:"contract_version"`
	Index     KnowledgeLexicalIndexRef         `json:"index_ref"`
	Selection KnowledgeContentLexicalSelection `json:"selection"`
	Query     string                           `json:"query"`
	Limit     int                              `json:"limit"`
}

type KnowledgeContentLexicalReadRequest struct {
	Version  string                          `json:"contract_version"`
	Evidence KnowledgeContentLexicalEvidence `json:"evidence_ref"`
}

type KnowledgeContentLexicalReadiness struct {
	Kind            string                            `json:"kind"`
	Publication     string                            `json:"publication"`
	Selection       KnowledgeContentLexicalSelection  `json:"selection"`
	SelectionSHA256 string                            `json:"selection_sha256"`
	Currentness     string                            `json:"snapshot_currentness"`
	Index           *KnowledgeLexicalIndexRef         `json:"index_ref"`
	Coverage        []KnowledgeContentLexicalCoverage `json:"coverage"`
	Diagnostics     []string                          `json:"diagnostic_codes"`
}

type KnowledgeContentLexicalInspectData struct {
	Policy    KnowledgeContentLexicalPolicy    `json:"effective_policy"`
	Profile   KnowledgeContentLexicalProfile   `json:"profile"`
	Readiness KnowledgeContentLexicalReadiness `json:"readiness"`
}

type KnowledgeContentLexicalJob struct {
	ID                string                           `json:"job_id"`
	Key               string                           `json:"idempotency_key"`
	RequestSHA256     string                           `json:"request_sha256"`
	State             string                           `json:"state"`
	Selection         KnowledgeContentLexicalSelection `json:"selection"`
	LocalVersion      int64                            `json:"captured_local_policy_version"`
	ContentVersion    int64                            `json:"captured_content_policy_version"`
	FoundationVersion int64                            `json:"captured_foundation_policy_version"`
	ProfileSHA256     string                           `json:"captured_profile_sha256"`
	ProducerSHA256    string                           `json:"captured_tokenizer_producer_sha256"`
	Index             *KnowledgeLexicalIndexRef        `json:"index_ref"`
	Readiness         KnowledgeContentLexicalReadiness `json:"readiness"`
	CreatedAt         string                           `json:"created_at"`
	UpdatedAt         string                           `json:"updated_at"`
}

type KnowledgeContentLexicalBinding struct {
	Index             KnowledgeLexicalIndexRef         `json:"index_ref"`
	Selection         KnowledgeContentLexicalSelection `json:"selection"`
	SelectionSHA256   string                           `json:"selection_sha256"`
	LocalVersion      int64                            `json:"local_policy_version"`
	ContentVersion    int64                            `json:"content_policy_version"`
	FoundationVersion int64                            `json:"foundation_policy_version"`
	ProfileSHA256     string                           `json:"profile_sha256"`
	ProducerSHA256    string                           `json:"tokenizer_producer_sha256"`
	Engine            string                           `json:"engine"`
	Execution         string                           `json:"execution_profile"`
	ParentExpansion   string                           `json:"parent_expansion"`
	PublishedAt       string                           `json:"published_at"`
}

type KnowledgeContentLexicalHit struct {
	Evidence    KnowledgeContentLexicalEvidence `json:"evidence_ref"`
	Text        string                          `json:"text"`
	Score       float64                         `json:"score"`
	MatchType   string                          `json:"match_type"`
	Currentness string                          `json:"source_currentness"`
}

type KnowledgeContentLexicalSearchData struct {
	Binding         KnowledgeContentLexicalBinding    `json:"binding"`
	Coverage        []KnowledgeContentLexicalCoverage `json:"coverage"`
	Query           string                            `json:"query"`
	RequestedLimit  int                               `json:"requested_limit"`
	ReturnedCount   int                               `json:"returned_count"`
	Hits            []KnowledgeContentLexicalHit      `json:"hits"`
	TotalMatches    *int                              `json:"total_matches"`
	Pagination      string                            `json:"pagination"`
	Ranking         string                            `json:"ranking"`
	ParentExpansion string                            `json:"parent_expansion"`
}

type KnowledgeContentLexicalReadData struct {
	Evidence    KnowledgeContentLexicalEvidence `json:"evidence_ref"`
	Text        string                          `json:"text"`
	Encoding    string                          `json:"encoding"`
	ByteLength  int                             `json:"byte_length"`
	Currentness string                          `json:"source_currentness"`
	Complete    bool                            `json:"complete"`
	Context     KnowledgeContentLexicalContext  `json:"context"`
}

type KnowledgeContentLexicalTypedMember struct {
	Node          KnowledgeContentNode `json:"node"`
	PayloadSHA256 string               `json:"payload_sha256"`
	TextAssembly  string               `json:"text_assembly"`
}

type KnowledgeContentLexicalContext struct {
	Node                *KnowledgeContentNode                `json:"node"`
	NodeOmissionReason  *string                              `json:"node_omission_reason"`
	TypedMembers        []KnowledgeContentLexicalTypedMember `json:"typed_members"`
	Scope               string                               `json:"typed_members_scope"`
	MembersComplete     bool                                 `json:"typed_members_complete"`
	OmittedMembers      int                                  `json:"omitted_member_count"`
	OmissionReasons     []string                             `json:"omission_reasons"`
	ManifestSHA256      *string                              `json:"manifest_sha256"`
	Membership          string                               `json:"membership"`
	RepresentationState string                               `json:"representation_state"`
	ParentExpansion     string                               `json:"parent_expansion"`
	MetadataBytes       int                                  `json:"metadata_byte_length"`
}

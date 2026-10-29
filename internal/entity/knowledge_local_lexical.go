package entity

const KnowledgeLocalLexicalVersion = "0.1.0-candidate"
const KnowledgeLocalLexicalMaxUnit = 8192

type KnowledgeLexicalSourceRef struct {
	DomainID   string `json:"domain_id"`
	SourceID   string `json:"source_id"`
	RevisionID string `json:"source_revision_id"`
	SHA256     string `json:"source_sha256"`
}
type KnowledgeLexicalRepresentationRef struct {
	Kind           string  `json:"kind"`
	ID             string  `json:"id"`
	ProducerSHA256 *string `json:"producer_sha256"`
}
type KnowledgeLexicalTarget struct {
	Source         KnowledgeLexicalSourceRef         `json:"source_ref"`
	Representation KnowledgeLexicalRepresentationRef `json:"representation_ref"`
}
type KnowledgeLexicalSelection struct {
	Kind    string                   `json:"kind"`
	Targets []KnowledgeLexicalTarget `json:"targets"`
}
type KnowledgeLexicalIndexRef struct {
	ID             string `json:"index_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}
type KnowledgeLexicalRange struct {
	Start int64 `json:"start_byte"`
	End   int64 `json:"end_byte"`
}
type KnowledgeLexicalNodeRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type KnowledgeLexicalEvidence struct {
	Version         string                            `json:"evidence_version"`
	Index           KnowledgeLexicalIndexRef          `json:"index_ref"`
	RowID           string                            `json:"index_row_id"`
	Source          KnowledgeLexicalSourceRef         `json:"source_ref"`
	Representation  KnowledgeLexicalRepresentationRef `json:"representation_ref"`
	Node            KnowledgeLexicalNodeRef           `json:"node_ref"`
	CoordinateSpace string                            `json:"coordinate_space"`
	Range           KnowledgeLexicalRange             `json:"range"`
	FragmentSHA256  string                            `json:"fragment_sha256"`
}
type KnowledgeLexicalPolicy struct {
	DomainID           string   `json:"domain_id"`
	Version            int64    `json:"local_policy_version"`
	FoundationVersion  int64    `json:"foundation_policy_version"`
	Enabled            bool     `json:"enabled"`
	Mode               string   `json:"mode"`
	ReadAuthority      string   `json:"read_authority"`
	AcceptedInputs     []string `json:"accepted_inputs"`
	AllowedModelUses   []string `json:"allowed_model_uses"`
	ExternalModels     bool     `json:"external_models"`
	LocalInference     bool     `json:"local_inference"`
	RemoteFetches      bool     `json:"remote_fetches"`
	AutomaticIngestion bool     `json:"automatic_ingestion"`
	VectorIndex        string   `json:"vector_index"`
}
type KnowledgeLexicalPolicyChange struct {
	Version                   string `json:"contract_version"`
	Key                       string `json:"idempotency_key"`
	ExpectedVersion           int64  `json:"expected_local_policy_version"`
	ExpectedFoundationVersion int64  `json:"expected_foundation_policy_version"`
	Enabled                   bool   `json:"enabled"`
}
type KnowledgeLexicalPolicyReceipt struct {
	Key           string                 `json:"idempotency_key"`
	RequestSHA256 string                 `json:"request_sha256"`
	Outcome       string                 `json:"outcome"`
	Policy        KnowledgeLexicalPolicy `json:"policy"`
}
type KnowledgeLexicalInspectRequest struct {
	Version   string                    `json:"contract_version"`
	Selection KnowledgeLexicalSelection `json:"selection"`
}
type KnowledgeLexicalCreateRequest struct {
	Version                   string                    `json:"contract_version"`
	Key                       string                    `json:"idempotency_key"`
	ExpectedVersion           int64                     `json:"expected_local_policy_version"`
	ExpectedFoundationVersion int64                     `json:"expected_foundation_policy_version"`
	Selection                 KnowledgeLexicalSelection `json:"selection"`
}
type KnowledgeLexicalSearchRequest struct {
	Version   string                    `json:"contract_version"`
	Index     KnowledgeLexicalIndexRef  `json:"index_ref"`
	Selection KnowledgeLexicalSelection `json:"selection"`
	Query     string                    `json:"query"`
	Limit     int                       `json:"limit"`
}
type KnowledgeLexicalReadRequest struct {
	Version  string                   `json:"contract_version"`
	Evidence KnowledgeLexicalEvidence `json:"evidence_ref"`
}
type KnowledgeLexicalMissingRange struct {
	Source KnowledgeLexicalSourceRef `json:"source_ref"`
	Range  KnowledgeLexicalRange     `json:"range"`
}
type KnowledgeLexicalCoverage struct {
	SelectedSources int                            `json:"selected_source_count"`
	CoveredSources  int                            `json:"covered_source_count"`
	ExpectedBytes   int64                          `json:"expected_source_bytes"`
	CoveredBytes    int64                          `json:"covered_source_bytes"`
	TokenlessBytes  int64                          `json:"tokenless_source_bytes"`
	IndexedUnits    int                            `json:"indexed_unit_count"`
	Complete        bool                           `json:"complete"`
	Missing         []KnowledgeLexicalMissingRange `json:"missing_ranges"`
}
type KnowledgeLexicalReadiness struct {
	Kind            string                    `json:"kind"`
	Publication     string                    `json:"publication"`
	Index           *KnowledgeLexicalIndexRef `json:"index_ref"`
	Selection       KnowledgeLexicalSelection `json:"selection"`
	SelectionSHA256 string                    `json:"selection_sha256"`
	Currentness     string                    `json:"snapshot_currentness"`
	Coverage        *KnowledgeLexicalCoverage `json:"coverage"`
	Diagnostics     []string                  `json:"diagnostic_codes"`
}
type KnowledgeLexicalReadinessData struct {
	Policy    KnowledgeLexicalPolicy    `json:"policy"`
	Readiness KnowledgeLexicalReadiness `json:"readiness"`
}
type KnowledgeLexicalBinding struct {
	Index             KnowledgeLexicalIndexRef  `json:"index_ref"`
	Selection         KnowledgeLexicalSelection `json:"selection"`
	SelectionSHA256   string                    `json:"selection_sha256"`
	PolicyVersion     int64                     `json:"local_policy_version"`
	FoundationVersion int64                     `json:"foundation_policy_version"`
	ProfileSHA256     string                    `json:"profile_sha256"`
	Engine            string                    `json:"engine"`
	ExecutionProfile  string                    `json:"execution_profile"`
	ParentExpansion   string                    `json:"parent_expansion"`
	UnitPolicy        string                    `json:"unit_policy"`
	PublishedAt       string                    `json:"published_at"`
}
type KnowledgeLexicalHit struct {
	Evidence    KnowledgeLexicalEvidence `json:"evidence_ref"`
	Text        string                   `json:"text"`
	Score       float64                  `json:"score"`
	MatchType   string                   `json:"match_type"`
	Currentness string                   `json:"source_currentness"`
}
type KnowledgeLexicalSearchData struct {
	Binding         KnowledgeLexicalBinding  `json:"binding"`
	Coverage        KnowledgeLexicalCoverage `json:"coverage"`
	Readiness       string                   `json:"readiness"`
	Query           string                   `json:"query"`
	RequestedLimit  int                      `json:"requested_limit"`
	ReturnedCount   int                      `json:"returned_count"`
	Hits            []KnowledgeLexicalHit    `json:"hits"`
	TotalMatches    *int                     `json:"total_matches"`
	Pagination      string                   `json:"pagination"`
	Ranking         string                   `json:"ranking"`
	ParentExpansion string                   `json:"parent_expansion"`
}
type KnowledgeLexicalReadData struct {
	Evidence    KnowledgeLexicalEvidence `json:"evidence_ref"`
	Text        string                   `json:"text"`
	Encoding    string                   `json:"encoding"`
	ByteLength  int                      `json:"byte_length"`
	Currentness string                   `json:"source_currentness"`
	Complete    bool                     `json:"complete"`
}
type KnowledgeLexicalJob struct {
	ID                string                    `json:"job_id"`
	Key               string                    `json:"idempotency_key"`
	RequestSHA256     string                    `json:"request_sha256"`
	State             string                    `json:"state"`
	Selection         KnowledgeLexicalSelection `json:"selection"`
	PolicyVersion     int64                     `json:"captured_local_policy_version"`
	FoundationVersion int64                     `json:"captured_foundation_policy_version"`
	Index             *KnowledgeLexicalIndexRef `json:"index_ref"`
	Readiness         KnowledgeLexicalReadiness `json:"readiness"`
	CreatedAt         string                    `json:"created_at"`
	UpdatedAt         string                    `json:"updated_at"`
}

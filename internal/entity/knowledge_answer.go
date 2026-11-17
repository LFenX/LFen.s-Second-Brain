package entity

const KnowledgeAnswerVersion = "0.1.0-candidate"

type KnowledgeAnswerScope struct {
	Index     KnowledgeLexicalIndexRef         `json:"index_ref"`
	Selection KnowledgeContentLexicalSelection `json:"selection"`
}

type KnowledgeAnswerModelRef struct {
	ID     string `json:"model_id"`
	SHA256 string `json:"configuration_sha256"`
}

type KnowledgeAnswerInspectRequest struct {
	Version string               `json:"contract_version"`
	Scope   KnowledgeAnswerScope `json:"scope"`
	ModelID string               `json:"model_id"`
}

type KnowledgeAnswerPolicyGetRequest struct {
	Version string `json:"contract_version"`
}

type KnowledgeAnswerPolicySetRequest struct {
	Version               string                   `json:"contract_version"`
	ExpectedVersion       int64                    `json:"expected_version"`
	Enabled               bool                     `json:"enabled"`
	Scope                 *KnowledgeAnswerScope    `json:"scope"`
	Model                 *KnowledgeAnswerModelRef `json:"model_ref"`
	Confirmed             bool                     `json:"confirmed"`
	ExternalSendConfirmed bool                     `json:"external_send_confirmed"`
	Key                   string                   `json:"idempotency_key"`
}

type KnowledgeAnswerRequest struct {
	Version         string  `json:"contract_version"`
	ExpectedVersion int64   `json:"expected_policy_version"`
	Question        string  `json:"question"`
	Query           *string `json:"retrieval_query"`
	Key             string  `json:"idempotency_key"`
}

type KnowledgeAnswerPolicy struct {
	DomainID     string                   `json:"domain_id"`
	Version      int64                    `json:"version"`
	Enabled      bool                     `json:"enabled"`
	Scope        *KnowledgeAnswerScope    `json:"scope"`
	ScopeSHA256  *string                  `json:"scope_sha256"`
	Model        *KnowledgeAnswerModelRef `json:"model_ref"`
	ConsentActor *string                  `json:"consent_actor"`
	ConsentTime  *string                  `json:"consent_time"`
}

type KnowledgeAnswerLimits struct {
	Targets       int `json:"max_targets"`
	Retrieval     int `json:"retrieval_limit"`
	ContextBytes  int `json:"max_context_utf8_bytes"`
	QuestionBytes int `json:"max_question_utf8_bytes"`
	OutputTokens  int `json:"max_output_tokens"`
	AnswerBytes   int `json:"max_answer_utf8_bytes"`
}

func KnowledgeAnswerFixedLimits() KnowledgeAnswerLimits {
	return KnowledgeAnswerLimits{32, 5, 32768, 4096, 1024, 8192}
}

type KnowledgeAnswerInspect struct {
	PolicyVersion int64                   `json:"policy_version"`
	Scope         KnowledgeAnswerScope    `json:"scope"`
	ScopeSHA256   string                  `json:"scope_sha256"`
	Model         KnowledgeAnswerModelRef `json:"model_ref"`
	ModelName     string                  `json:"model_display_name"`
	Destination   string                  `json:"destination_origin"`
	Readiness     string                  `json:"readiness"`
	Limits        KnowledgeAnswerLimits   `json:"limits"`
	ModelCalls    int                     `json:"model_calls"`
}

type KnowledgeAnswerCitation struct {
	ID         string                          `json:"id"`
	Evidence   KnowledgeContentLexicalEvidence `json:"evidence_ref"`
	Text       string                          `json:"text"`
	ByteLength int                             `json:"byte_length"`
	SHA256     string                          `json:"sha256"`
}

type KnowledgeAnswerUsage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Total      int `json:"total_tokens"`
}

type KnowledgeAnswerResult struct {
	ID            string                    `json:"answer_id"`
	State         string                    `json:"state"`
	PolicyVersion int64                     `json:"policy_version"`
	ScopeSHA256   string                    `json:"scope_sha256"`
	Model         KnowledgeAnswerModelRef   `json:"model_ref"`
	Answer        string                    `json:"answer"`
	Insufficient  bool                      `json:"insufficient_evidence"`
	Citations     []KnowledgeAnswerCitation `json:"citations"`
	Usage         *KnowledgeAnswerUsage     `json:"usage"`
	ProviderCalls int                       `json:"provider_calls"`
	Replayed      bool                      `json:"replayed"`
}

type KnowledgeAnswerRunning struct {
	ID            string                  `json:"answer_id"`
	State         string                  `json:"state"`
	PolicyVersion int64                   `json:"policy_version"`
	ScopeSHA256   string                  `json:"scope_sha256"`
	Model         KnowledgeAnswerModelRef `json:"model_ref"`
	ProviderCalls int                     `json:"provider_calls"`
	Replayed      bool                    `json:"replayed"`
}

type KnowledgeAnswerModelOutput struct {
	Answer       string   `json:"answer"`
	Insufficient bool     `json:"insufficient_evidence"`
	EvidenceIDs  []string `json:"evidence_ids"`
}

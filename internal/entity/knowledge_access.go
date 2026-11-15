package entity

import "encoding/json"

const KnowledgeAccessVersion = "0.3.0-candidate"
const KnowledgeAccessMaxGeneration int64 = 9007199254740991

type KnowledgeAccessSetPolicyRequest struct {
	ContractVersion string `json:"contract_version"`
	DomainID        string `json:"domain_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Enabled         bool   `json:"enabled"`
	Confirmed       bool   `json:"confirmed"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type KnowledgeAccessPolicy struct {
	DomainID string `json:"domain_id"`
	Version  int64  `json:"version"`
	Enabled  bool   `json:"enabled"`
}

type KnowledgeAccessError struct {
	Operation       string `json:"operation"`
	ContractVersion string `json:"contract_version"`
	RequestID       string `json:"request_id"`
	Data            any    `json:"data"`
	Code            string `json:"code"`
	Message         string `json:"message"`
	OutcomeKnown    bool   `json:"outcome_known"`
	RetrySameKey    bool   `json:"retry_same_key"`
}

type KnowledgeAccessSetModuleRequest struct {
	ContractVersion string                     `json:"contract_version"`
	DomainID        string                     `json:"domain_id"`
	ModuleID        *string                    `json:"module_id"`
	ExpectedVersion int64                      `json:"expected_version"`
	Name            string                     `json:"name"`
	Enabled         bool                       `json:"enabled"`
	Scope           KnowledgeAccessModuleScope `json:"scope"`
	Confirmed       bool                       `json:"confirmed"`
	IdempotencyKey  string                     `json:"idempotency_key"`
}

type KnowledgeAccessModuleScope struct {
	Kind        string                          `json:"kind"`
	Publication KnowledgeAccessOwnerPublication `json:"publication"`
	Targets     []KnowledgeAccessTarget         `json:"targets"`
}

type KnowledgeAccessOwnerPublication struct {
	ID             string `json:"id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type KnowledgeAccessTarget struct {
	Source         KnowledgeAccessSource         `json:"source"`
	Representation KnowledgeAccessRepresentation `json:"representation"`
}

type KnowledgeAccessSource struct {
	DomainID         string `json:"domain_id"`
	SourceID         string `json:"source_id"`
	SourceRevisionID string `json:"source_revision_id"`
	SourceSHA256     string `json:"source_sha256"`
}

type KnowledgeAccessRepresentation struct {
	Kind           string  `json:"kind"`
	ID             string  `json:"id"`
	ProducerSHA256 *string `json:"producer_sha256"`
}

type KnowledgeAccessModuleAdmin struct {
	ModuleID string                     `json:"module_id"`
	DomainID string                     `json:"domain_id"`
	Name     string                     `json:"name"`
	Version  int64                      `json:"version"`
	Enabled  bool                       `json:"enabled"`
	Scope    KnowledgeAccessModuleScope `json:"scope"`
	Grants   []KnowledgeAccessGrant     `json:"grants"`
	Policy   KnowledgeAccessPolicy      `json:"policy"`
}

type KnowledgeAccessGrant struct {
	GrantID        string                 `json:"grant_id"`
	DomainID       string                 `json:"domain_id"`
	ModuleID       string                 `json:"module_id"`
	ModuleVersion  int64                  `json:"module_version"`
	Subject        KnowledgeAccessSubject `json:"subject"`
	Mode           string                 `json:"mode"`
	Delivery       string                 `json:"delivery"`
	IssuedAt       string                 `json:"issued_at"`
	ExpiresAt      string                 `json:"expires_at"`
	RevokedAt      *string                `json:"revoked_at"`
	IssuedByUserID string                 `json:"issued_by_user_id"`
}

type KnowledgeAccessSubject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type KnowledgeAccessGetModuleAdminRequest struct {
	ContractVersion string `json:"contract_version"`
	DomainID        string `json:"domain_id"`
	ModuleID        string `json:"module_id"`
}

type KnowledgeAccessListModulesRequest struct {
	ContractVersion string  `json:"contract_version"`
	TenantID        string  `json:"tenant_id"`
	DomainID        *string `json:"domain_id"`
}

type KnowledgeAccessListModulesData struct {
	Items               []KnowledgeAccessModuleSummary `json:"items"`
	Pagination          string                         `json:"pagination"`
	OwnedDomainPolicies []KnowledgeAccessPolicy        `json:"owned_domain_policies"`
}

type KnowledgeAccessModuleSummary struct {
	ModuleID            string   `json:"module_id"`
	DomainID            string   `json:"domain_id"`
	Name                string   `json:"name"`
	Version             int64    `json:"version"`
	TargetCount         int64    `json:"target_count"`
	Modes               []string `json:"modes"`
	MCPDeliveryAllowed  bool     `json:"mcp_delivery_allowed"`
	Enabled             bool     `json:"enabled"`
	AdministrativeOwner bool     `json:"administrative_owner"`
}

type KnowledgeAccessCreateGrantRequest struct {
	ContractVersion       string                 `json:"contract_version"`
	DomainID              string                 `json:"domain_id"`
	ModuleID              string                 `json:"module_id"`
	ExpectedModuleVersion int64                  `json:"expected_module_version"`
	Subject               KnowledgeAccessSubject `json:"subject"`
	Mode                  string                 `json:"mode"`
	Delivery              string                 `json:"delivery"`
	ExpiresAt             string                 `json:"expires_at"`
	Confirmed             bool                   `json:"confirmed"`
	IdempotencyKey        string                 `json:"idempotency_key"`
}

type KnowledgeAccessRevokeGrantRequest struct {
	ContractVersion string `json:"contract_version"`
	DomainID        string `json:"domain_id"`
	GrantID         string `json:"grant_id"`
	Confirmed       bool   `json:"confirmed"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type KnowledgeAccessRevoked struct {
	ID        string `json:"id"`
	RevokedAt string `json:"revoked_at"`
}

type KnowledgeAccessInspectRequest struct {
	ContractVersion string    `json:"contract_version"`
	DomainID        string    `json:"domain_id"`
	ModuleID        string    `json:"module_id"`
	Mode            string    `json:"mode"`
	Delivery        string    `json:"delivery"`
	SourceIDs       *[]string `json:"source_ids"`
}

type KnowledgeAccessInspectData struct {
	ViewID        string                              `json:"view_id"`
	DomainID      string                              `json:"domain_id"`
	ModuleID      string                              `json:"module_id"`
	ModuleVersion int64                               `json:"module_version"`
	Mode          string                              `json:"mode"`
	Delivery      string                              `json:"delivery"`
	ExpiresAt     string                              `json:"expires_at"`
	Targets       []KnowledgeAccessTargetCatalogEntry `json:"targets"`
	SourceCount   int64                               `json:"source_count"`
	UnitCount     int64                               `json:"unit_count"`
	Readiness     string                              `json:"readiness"`
	Pagination    string                              `json:"pagination"`
}

type KnowledgeAccessTargetCatalogEntry struct {
	Target    KnowledgeAccessTarget `json:"target"`
	UnitCount int64                 `json:"unit_count"`
}

type KnowledgeAccessSearchRequest struct {
	ContractVersion string `json:"contract_version"`
	ViewID          string `json:"view_id"`
	Query           string `json:"query"`
	Limit           int64  `json:"limit"`
}

type KnowledgeAccessSearchData struct {
	ViewID          string               `json:"view_id"`
	Query           string               `json:"query"`
	RequestedLimit  int64                `json:"requested_limit"`
	ReturnedCount   int64                `json:"returned_count"`
	Hits            []KnowledgeAccessHit `json:"hits"`
	TotalMatches    any                  `json:"total_matches"`
	Pagination      string               `json:"pagination"`
	Ranking         string               `json:"ranking"`
	ParentExpansion string               `json:"parent_expansion"`
}

type KnowledgeAccessHit struct {
	EvidenceRef KnowledgeAccessEvidenceRef `json:"evidence_ref"`
	Citation    KnowledgeAccessCitation    `json:"citation"`
	Text        string                     `json:"text"`
	ByteLength  int64                      `json:"byte_length"`
	RankOrdinal int64                      `json:"rank_ordinal"`
}

type KnowledgeAccessEvidenceRef struct {
	ContractVersion string `json:"contract_version"`
	ViewID          string `json:"view_id"`
	EvidenceID      string `json:"evidence_id"`
}

type KnowledgeAccessCitation struct {
	Target          KnowledgeAccessTarget `json:"target"`
	NodeID          string                `json:"node_id"`
	CoordinateSpace string                `json:"coordinate_space"`
	Range           KnowledgeAccessRange  `json:"range"`
	DisplayName     string                `json:"display_name"`
	PositionLabel   string                `json:"position_label"`
	RevisionBinding string                `json:"revision_binding"`
}

type KnowledgeAccessRange struct {
	StartByte int64  `json:"start_byte"`
	EndByte   int64  `json:"end_byte"`
	SHA256    string `json:"sha256"`
}

type KnowledgeAccessReadRequest struct {
	ContractVersion string          `json:"contract_version"`
	Selector        json.RawMessage `json:"selector"`
}

type KnowledgeAccessUnitReadSelector struct {
	Kind        string `json:"kind"`
	ViewID      string `json:"view_id"`
	TargetIndex int64  `json:"target_index"`
	UnitOrdinal int64  `json:"unit_ordinal"`
}

type KnowledgeAccessEvidenceReadSelector struct {
	Kind        string                     `json:"kind"`
	EvidenceRef KnowledgeAccessEvidenceRef `json:"evidence_ref"`
}

type KnowledgeAccessReadData struct {
	EvidenceRef     KnowledgeAccessEvidenceRef  `json:"evidence_ref"`
	Citation        KnowledgeAccessCitation     `json:"citation"`
	Text            string                      `json:"text"`
	ByteLength      int64                       `json:"byte_length"`
	Encoding        string                      `json:"encoding"`
	Complete        bool                        `json:"complete"`
	TypedContext    KnowledgeAccessTypedContext `json:"typed_context"`
	Attachments     string                      `json:"attachments"`
	ParentExpansion string                      `json:"parent_expansion"`
	UnitLocator     KnowledgeAccessUnitLocator  `json:"unit_locator"`
}

type KnowledgeAccessTypedContext struct {
	Members         []KnowledgeAccessTypedMember `json:"members"`
	MembersComplete bool                         `json:"members_complete"`
	OmissionReasons []string                     `json:"omission_reasons"`
}

type KnowledgeAccessTypedMember struct {
	NodeID        string               `json:"node_id"`
	Kind          string               `json:"kind"`
	Range         KnowledgeAccessRange `json:"range"`
	TextStatus    string               `json:"text_status"`
	PayloadSHA256 string               `json:"payload_sha256"`
}

type KnowledgeAccessUnitLocator struct {
	ViewID          string `json:"view_id"`
	TargetIndex     int64  `json:"target_index"`
	UnitOrdinal     int64  `json:"unit_ordinal"`
	TargetUnitCount int64  `json:"target_unit_count"`
	NextUnitOrdinal *int64 `json:"next_unit_ordinal"`
}

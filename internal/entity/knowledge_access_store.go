package entity

import "time"

type KnowledgeAccessTenantStateRecord struct {
	TenantID string `gorm:"size:32;primaryKey"`
	Epoch    int64  `gorm:"not null;default:0"`
}

func (KnowledgeAccessTenantStateRecord) TableName() string { return "knowledge_access_tenant_state" }

type KnowledgeAccessDepartmentRecord struct {
	ID        string    `gorm:"size:32;primaryKey"`
	TenantID  string    `gorm:"size:32;not null;index:idx_ka_department_tenant"`
	Name      string    `gorm:"size:128;not null"`
	Version   int64     `gorm:"not null"`
	Enabled   bool      `gorm:"not null"`
	CreatedBy string    `gorm:"size:32;not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (KnowledgeAccessDepartmentRecord) TableName() string { return "knowledge_access_department" }

type KnowledgeAccessDepartmentMemberRecord struct {
	ID           string                           `gorm:"size:32;primaryKey"`
	DepartmentID string                           `gorm:"size:32;not null"`
	UserID       string                           `gorm:"size:32;not null"`
	CreatedAt    time.Time                        `gorm:"not null"`
	Department   *KnowledgeAccessDepartmentRecord `gorm:"foreignKey:DepartmentID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (KnowledgeAccessDepartmentMemberRecord) TableName() string {
	return "knowledge_access_department_member"
}

type KnowledgeAccessDomainPolicyRecord struct {
	DomainID  string    `gorm:"size:32;primaryKey"`
	TenantID  string    `gorm:"size:32;not null;index:idx_ka_policy_tenant"`
	Enabled   bool      `gorm:"not null;default:false"`
	Version   int64     `gorm:"not null"`
	UpdatedBy string    `gorm:"size:32;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (KnowledgeAccessDomainPolicyRecord) TableName() string { return "knowledge_access_domain_policy" }

// The owning migration installs UNIQUE(domain_id,id) as idx_ka_module_1.
type KnowledgeAccessModuleRecord struct {
	ID          string    `gorm:"size:32;primaryKey"`
	DomainID    string    `gorm:"size:32;not null"`
	TenantID    string    `gorm:"size:32;not null;index:idx_ka_module_tenant"`
	Name        string    `gorm:"size:128;not null"`
	Version     int64     `gorm:"not null"`
	Enabled     bool      `gorm:"not null"`
	ScopeJSON   string    `gorm:"type:text;not null"`
	ScopeSHA256 string    `gorm:"size:64;not null"`
	CreatedBy   string    `gorm:"size:32;not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (KnowledgeAccessModuleRecord) TableName() string { return "knowledge_access_module" }

type KnowledgeAccessGrantRecord struct {
	ID              string    `gorm:"size:32;primaryKey"`
	TenantID        string    `gorm:"size:32;not null"`
	DomainID        string    `gorm:"size:32;not null"`
	ModuleID        string    `gorm:"size:32;not null;index:idx_ka_grant_subject,priority:1"`
	SubjectKind     string    `gorm:"size:16;not null;index:idx_ka_grant_subject,priority:2"`
	SubjectID       string    `gorm:"size:32;not null;index:idx_ka_grant_subject,priority:3"`
	Mode            string    `gorm:"size:16;not null"`
	Delivery        string    `gorm:"size:16;not null"`
	IssuedBy        string    `gorm:"size:32;not null"`
	DefinitionJSON  string    `gorm:"type:text;not null"`
	IssuedAt        time.Time `gorm:"not null"`
	ExpiresAt       time.Time `gorm:"not null;index:idx_ka_grant_expiry"`
	RevokedAt       *time.Time
	ModuleVersion   int64                       `gorm:"not null"`
	CreateCommandID string                      `gorm:"size:32;not null"`
	CreationReceipt *KnowledgeAccessAuditRecord `gorm:"foreignKey:CreateCommandID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (KnowledgeAccessGrantRecord) TableName() string { return "knowledge_access_grant" }

type KnowledgeAccessViewRecord struct {
	ID                  string    `gorm:"size:32;primaryKey"`
	TenantID            string    `gorm:"size:32;not null"`
	SubjectUserID       string    `gorm:"size:32;not null;index:idx_ka_view_subject_domain,priority:1"`
	CredentialID        *string   `gorm:"size:32;index:idx_ka_view_credential"`
	GrantID             string    `gorm:"size:32;not null"`
	DomainID            string    `gorm:"size:32;not null;index:idx_ka_view_subject_domain,priority:2"`
	ModuleID            string    `gorm:"size:32;not null"`
	ModuleVersion       int64     `gorm:"not null"`
	TenantEpoch         int64     `gorm:"not null"`
	PrivateManifestJSON string    `gorm:"type:text;not null"`
	CaptureJSON         string    `gorm:"type:text;not null"`
	IssuedAt            time.Time `gorm:"not null"`
	ExpiresAt           time.Time `gorm:"not null;index:idx_ka_view_expiry"`
}

func (KnowledgeAccessViewRecord) TableName() string { return "knowledge_access_view" }

// UNIQUE(actor_user_id, command_operation, command_key) is installed as
// idx_ka_audit_command_key by the DAO migration, reconciling full column sets.
type KnowledgeAccessAuditRecord struct {
	ID                   string    `gorm:"size:32;primaryKey"`
	TenantID             string    `gorm:"size:32;not null;index:idx_ka_audit_tenant_time,priority:1"`
	ActorUserID          *string   `gorm:"size:32"`
	CredentialID         *string   `gorm:"size:32"`
	DomainID             *string   `gorm:"size:32;index:idx_ka_audit_domain_time,priority:1"`
	ModuleID             *string   `gorm:"size:32"`
	Action               string    `gorm:"size:48;not null"`
	Outcome              string    `gorm:"size:16;not null"`
	RequestID            string    `gorm:"size:32;not null"`
	DefinitionDigest     *string   `gorm:"size:64"`
	BeforeDefinitionJSON *string   `gorm:"type:text"`
	AfterDefinitionJSON  *string   `gorm:"type:text"`
	OccurredAt           time.Time `gorm:"not null;index:idx_ka_audit_tenant_time,priority:2;index:idx_ka_audit_domain_time,priority:2"`
	CommandOperation     *string   `gorm:"size:32"`
	CommandKey           *string   `gorm:"size:128"`
	CommandRequestSHA256 []byte    `gorm:"type:binary(32)"`
	CommandScopeKind     *string   `gorm:"size:16"`
	CommandScopeID       *string   `gorm:"size:32"`
	CommandState         *string   `gorm:"size:16"`
	CommandResourceID    *string   `gorm:"size:32;index:idx_ka_audit_resource"`
	CommandHTTPStatus    *int
	CommandResultJSON    *string `gorm:"type:mediumtext"`
	CommandCommittedAt   *time.Time
	CommandCompactedAt   *time.Time
}

func (KnowledgeAccessAuditRecord) TableName() string { return "knowledge_access_audit" }

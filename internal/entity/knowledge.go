package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// KnowledgeContractVersion identifies the frozen foundation HTTP contract.
const KnowledgeContractVersion = "1.0.0"

// KnowledgeMaxSourceBytes bounds the unparsed foundation snapshot, not future import pipelines.
const KnowledgeMaxSourceBytes int64 = 2 * 1024 * 1024

// KnowledgeGroup is an organizational view, separate from source node parents.
type KnowledgeGroup struct {
	ID            string  `gorm:"primaryKey;size:32" json:"id"`
	DomainID      string  `gorm:"size:32;not null;index:idx_knowledge_group_parent" json:"domain_id"`
	ParentID      *string `gorm:"size:32;index:idx_knowledge_group_parent" json:"parent_id"`
	Kind          string  `gorm:"size:16;not null" json:"kind"`
	Name          string  `gorm:"size:256;not null" json:"name"`
	ComponentSide string  `gorm:"size:16;not null;default:'non_code'" json:"component_side"`
	BaseModel
}

func (KnowledgeGroup) TableName() string { return "knowledge_group" }

// KnowledgeSourceRevision projects an immutable native FileCommitItem into a domain.
// DocumentID is also the stable source_id. Raw content remains in native storage.
type KnowledgeSourceRevision struct {
	ID            string  `gorm:"primaryKey;size:32" json:"source_revision_id"`
	DomainID      string  `gorm:"size:32;not null;index;uniqueIndex:idx_knowledge_source_input" json:"domain_id"`
	DocumentID    string  `gorm:"size:32;not null;index;uniqueIndex:idx_knowledge_source_input" json:"source_id"`
	FileID        string  `gorm:"size:32;not null" json:"native_file_id"`
	CommitID      string  `gorm:"size:32;not null" json:"native_commit_id"`
	FolderID      string  `gorm:"size:32;not null" json:"-"`
	GroupID       *string `gorm:"size:32;index" json:"group_id"`
	RelativePath  string  `gorm:"size:512;not null" json:"relative_path"`
	ComponentSide string  `gorm:"size:16;not null" json:"component_side"`
	Language      string  `gorm:"size:32;not null;default:''" json:"language"`
	SHA256        string  `gorm:"size:64;not null" json:"sha256"`
	InputHash     string  `gorm:"size:64;not null;uniqueIndex:idx_knowledge_source_input" json:"-"`
	ByteLength    int64   `gorm:"not null" json:"byte_length"`
	LineCount     int64   `gorm:"not null" json:"line_count"`
	IsCurrent     bool    `gorm:"not null;default:true;index" json:"is_current"`
	RootNodeID    string  `gorm:"-" json:"root_node_id"`
	BaseModel
}

func (KnowledgeSourceRevision) TableName() string { return "knowledge_source_revision" }

// KnowledgeNode identifies a source range in one immutable source revision.
// ParentID always refers to a node in the same domain and source revision.
type KnowledgeNode struct {
	ID               string  `gorm:"primaryKey;size:64" json:"node_id"`
	LogicalID        string  `gorm:"size:64;not null;index" json:"logical_node_id"`
	DomainID         string  `gorm:"size:32;not null;index:idx_knowledge_node_scope" json:"domain_id"`
	SourceRevisionID string  `gorm:"size:32;not null;index:idx_knowledge_node_scope" json:"source_revision_id"`
	ParentID         *string `gorm:"size:64;index:idx_knowledge_node_scope" json:"parent_id"`
	Kind             string  `gorm:"size:16;not null" json:"kind"`
	LogicalKey       string  `gorm:"size:256;not null" json:"logical_key"`
	StructureVersion string  `gorm:"size:64;not null" json:"structure_version"`
	Name             string  `gorm:"size:256;not null" json:"name"`
	StartByte        int64   `gorm:"not null" json:"start_byte"`
	EndByte          int64   `gorm:"not null" json:"end_byte"`
	StartLine        int64   `gorm:"not null" json:"start_line"`
	EndLine          int64   `gorm:"not null" json:"end_line"`
	BaseModel
}

func (KnowledgeNode) TableName() string { return "knowledge_node" }

// KnowledgeID hashes a length-unambiguous JSON array. IDs are opaque to clients.
func KnowledgeID(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

type KnowledgePolicy struct {
	Version            int64    `json:"version"`
	Access             string   `json:"access"`
	IndexModes         []string `json:"index_modes"`
	AllowedModelUses   []string `json:"allowed_model_uses"`
	ExternalModels     bool     `json:"external_models"`
	AutomaticIngestion bool     `json:"automatic_ingestion"`
}

// KnowledgeLocalPolicy deliberately exposes no model or ingestion opt-in in v1.
func KnowledgeLocalPolicy(version int64) KnowledgePolicy {
	return KnowledgePolicy{
		Version: version, Access: "owner_only", IndexModes: []string{"catalogue"},
		AllowedModelUses: []string{}, ExternalModels: false, AutomaticIngestion: false,
	}
}

type KnowledgeDomain struct {
	ID              string          `json:"domain_id"`
	Name            string          `json:"name"`
	Category        string          `json:"category"`
	OwnerID         string          `json:"owner_id"`
	ContractVersion string          `json:"contract_version"`
	Policy          KnowledgePolicy `json:"policy"`
}

type RegisterKnowledgeDomainRequest struct {
	Category string `json:"category"`
}

type CreateKnowledgeGroupRequest struct {
	ParentID      *string `json:"parent_id"`
	Kind          string  `json:"kind"`
	Name          string  `json:"name"`
	ComponentSide string  `json:"component_side"`
}

type SnapshotKnowledgeSourceRequest struct {
	DocumentID         string  `json:"document_id"`
	ExpectedRevisionID string  `json:"expected_revision_id"`
	GroupID            *string `json:"group_id"`
	RelativePath       string  `json:"relative_path"`
	ComponentSide      string  `json:"component_side"`
	Language           string  `json:"language"`
}

type CreateKnowledgeNodeRequest struct {
	SourceRevisionID string `json:"source_revision_id"`
	ParentID         string `json:"parent_id"`
	Kind             string `json:"kind"`
	LogicalKey       string `json:"logical_key"`
	Name             string `json:"name"`
	StartByte        int64  `json:"start_byte"`
	EndByte          int64  `json:"end_byte"`
}

type CheckKnowledgePolicyRequest struct {
	Purpose string `json:"purpose"`
}

type KnowledgePolicyDecision struct {
	Purpose         string `json:"purpose"`
	Allowed         bool   `json:"allowed"`
	Reason          string `json:"reason"`
	PolicyVersion   int64  `json:"policy_version"`
	ContractVersion string `json:"contract_version"`
}

type KnowledgeNodeContent struct {
	NodeID           string `json:"node_id"`
	SourceID         string `json:"source_id"`
	SourceRevisionID string `json:"source_revision_id"`
	SHA256           string `json:"source_sha256"`
	StartByte        int64  `json:"start_byte"`
	EndByte          int64  `json:"end_byte"`
	Text             string `json:"text"`
	Encoding         string `json:"encoding"`
}

// KnowledgePage uses a scope-bound cursor; every page is authorized again.
type KnowledgePage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

type KnowledgeSnapshot struct {
	Source  KnowledgeSourceRevision `json:"source"`
	Root    KnowledgeNode           `json:"root"`
	Created bool                    `json:"created"`
}

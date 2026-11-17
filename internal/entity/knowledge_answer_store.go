package entity

import "time"

type KnowledgeAnswerPolicyRecord struct {
	DomainID     string `gorm:"size:32;primaryKey"`
	OwnerID      string `gorm:"size:32;not null"`
	Version      int64  `gorm:"not null"`
	Enabled      bool   `gorm:"not null;default:false"`
	ScopeJSON    string `gorm:"type:mediumtext;not null"`
	ScopeSHA256  string `gorm:"size:64;not null"`
	ModelID      string `gorm:"size:32;not null"`
	ModelSHA256  string `gorm:"size:64;not null"`
	ConsentActor string `gorm:"size:32;not null"`
	UpdatedAt    time.Time
}

func (KnowledgeAnswerPolicyRecord) TableName() string { return "knowledge_answer_policy" }

// Policy commands and answer dispatches share one durable deduplication ledger.
// No API key, question or uncited context is stored in this table.
type KnowledgeAnswerRequestRecord struct {
	ID            string    `gorm:"size:32;primaryKey"`
	OwnerID       string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_answer_key,priority:1"`
	DomainID      string    `gorm:"size:32;not null;uniqueIndex:idx_knowledge_answer_key,priority:2"`
	Operation     string    `gorm:"size:16;not null;uniqueIndex:idx_knowledge_answer_key,priority:3"`
	KeySHA256     string    `gorm:"size:64;not null;uniqueIndex:idx_knowledge_answer_key,priority:4"`
	RequestSHA256 string    `gorm:"size:64;not null"`
	PolicyVersion int64     `gorm:"not null"`
	ScopeSHA256   string    `gorm:"size:64;not null"`
	State         string    `gorm:"size:16;not null"`
	ProviderCalls int       `gorm:"not null"`
	FailureCode   string    `gorm:"size:32;not null"`
	ResponseJSON  string    `gorm:"type:mediumtext;not null"`
	CreatedAt     time.Time `gorm:"not null;index:idx_knowledge_answer_created"`
	UpdatedAt     time.Time
}

func (KnowledgeAnswerRequestRecord) TableName() string { return "knowledge_answer_request" }

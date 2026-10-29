package entity

import "time"

type KnowledgeLexicalPolicyRecord struct {
	DomainID  string `gorm:"primaryKey;size:32"`
	OwnerID   string `gorm:"size:32;not null"`
	Version   int64  `gorm:"not null"`
	Enabled   bool   `gorm:"not null"`
	UpdatedAt time.Time
}

func (KnowledgeLexicalPolicyRecord) TableName() string { return "knowledge_local_lexical_policy" }

type KnowledgeLexicalOperationRecord struct {
	ID            string `gorm:"primaryKey;size:64"`
	OwnerID       string `gorm:"size:32;not null"`
	DomainID      string `gorm:"size:32;not null;index"`
	Kind          string `gorm:"size:16;not null"`
	Key           string `gorm:"size:128;not null"`
	RequestSHA256 string `gorm:"size:64;not null"`
	Payload       string `gorm:"type:longtext;not null"`
	CreatedAt     time.Time
}

func (KnowledgeLexicalOperationRecord) TableName() string { return "knowledge_local_lexical_operation" }

type KnowledgeLexicalJobRecord struct {
	Protocol          string    `gorm:"size:40;not null;default:local_lexical/0.1.0"`
	ID                string    `gorm:"primaryKey;size:32"`
	OwnerID           string    `gorm:"size:32;not null"`
	DomainID          string    `gorm:"size:32;not null;index:idx_local_lexical_job_scope"`
	Key               string    `gorm:"size:128;not null"`
	RequestSHA256     string    `gorm:"size:64;not null"`
	SelectionSHA256   string    `gorm:"size:64;not null;index:idx_local_lexical_job_scope"`
	Selection         string    `gorm:"type:longtext;not null"`
	PolicyVersion     int64     `gorm:"not null"`
	FoundationVersion int64     `gorm:"not null"`
	ContentVersion    int64     `gorm:"not null;default:0"`
	ProducerSHA256    string    `gorm:"size:64;not null;default:''"`
	ProfileSHA256     string    `gorm:"size:64;not null;default:''"`
	ContentInputs     string    `gorm:"type:longtext"`
	Language          string    `gorm:"type:text;not null"`
	SessionSHA256     string    `gorm:"size:64;not null"`
	State             string    `gorm:"size:16;not null;index:idx_local_lexical_job_work"`
	AttemptCount      int64     `gorm:"not null"`
	Claim             string    `gorm:"size:32;not null"`
	LeaseUntil        time.Time `gorm:"not null;default:'1970-01-01 00:00:01'"`
	Manifest          string    `gorm:"type:longtext;not null"`
	IndexID           string    `gorm:"size:64;not null"`
	Diagnostic        string    `gorm:"size:32;not null"`
	CreatedAt         time.Time `gorm:"index:idx_local_lexical_job_scope"`
	UpdatedAt         time.Time
}

func (KnowledgeLexicalJobRecord) TableName() string { return "knowledge_local_lexical_job" }

// Publication payload bytes and their row entries are immutable after insert.
type KnowledgeLexicalPublicationRecord struct {
	Protocol        string    `gorm:"size:40;not null;default:local_lexical/0.1.0"`
	ID              string    `gorm:"primaryKey;size:64"`
	OwnerID         string    `gorm:"size:32;not null"`
	DomainID        string    `gorm:"size:32;not null;index:idx_local_lexical_publication"`
	SelectionSHA256 string    `gorm:"size:64;not null;index:idx_local_lexical_publication"`
	ManifestSHA256  string    `gorm:"size:64;not null"`
	Payload         string    `gorm:"type:longtext;not null"`
	CreatedAt       time.Time `gorm:"index:idx_local_lexical_publication"`
}

func (KnowledgeLexicalPublicationRecord) TableName() string {
	return "knowledge_local_lexical_publication"
}

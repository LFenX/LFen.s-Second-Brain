package entity

import _ "embed"

const KnowledgeBatchContractVersion = "0.4.0"

//go:embed knowledge_batch_capabilities.json
var KnowledgeBatchCapabilitiesJSON []byte

type KnowledgeBatchSelectionItem struct {
	RelativePath   string                           `json:"relative_path"`
	Classification KnowledgeDirectoryClassification `json:"classification"`
}

type KnowledgeBatchPreviewRequest struct {
	ObservationID     string                        `json:"observation_id"`
	ObservationSHA256 string                        `json:"observation_sha256"`
	SelectedItems     []KnowledgeBatchSelectionItem `json:"selected_items"`
}

type KnowledgeBatchPreviewItem struct {
	ItemID             string                           `json:"item_id"`
	Ordinal            int64                            `json:"ordinal"`
	RelativePath       string                           `json:"relative_path"`
	Classification     KnowledgeDirectoryClassification `json:"classification"`
	SourceID           *string                          `json:"source_id"`
	ExpectedRevisionID *string                          `json:"expected_revision_id"`
	SHA256             string                           `json:"sha256"`
	ByteLength         int64                            `json:"byte_length"`
	Action             string                           `json:"action"`
}

type KnowledgeBatchPreview struct {
	PreviewID         string                      `json:"preview_id"`
	DomainID          string                      `json:"domain_id"`
	RegistrationID    string                      `json:"registration_id"`
	ObservationID     string                      `json:"observation_id"`
	ObservationSHA256 string                      `json:"observation_sha256"`
	RootID            string                      `json:"root_id"`
	RootVersion       int64                       `json:"root_version"`
	PolicyVersion     int64                       `json:"policy_version"`
	ManifestSHA256    string                      `json:"manifest_sha256"`
	Mode              string                      `json:"mode"`
	CreatedAt         string                      `json:"created_at"`
	ExpiresAt         string                      `json:"expires_at"`
	ByteLength        int64                       `json:"byte_length"`
	Items             []KnowledgeBatchPreviewItem `json:"items"`
	ConsumedTaskID    *string                     `json:"consumed_task_id"`
}

type KnowledgeBatchContentWindow struct {
	PreviewID    string `json:"preview_id"`
	ItemID       string `json:"item_id"`
	RelativePath string `json:"relative_path"`
	SHA256       string `json:"sha256"`
	TotalBytes   int64  `json:"total_bytes"`
	Offset       int64  `json:"offset"`
	ByteLength   int64  `json:"byte_length"`
	Text         string `json:"text"`
	NextOffset   *int64 `json:"next_offset"`
}

type KnowledgeBatchAcceptRequest struct {
	PreviewID        string   `json:"preview_id"`
	ManifestSHA256   string   `json:"manifest_sha256"`
	IdempotencyKey   string   `json:"idempotency_key"`
	ConfirmedItemIDs []string `json:"confirmed_item_ids"`
	RetryOfTaskID    *string  `json:"retry_of_task_id"`
}

type KnowledgeBatchCounts struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Running   int64 `json:"running"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Cancelled int64 `json:"cancelled"`
}

type KnowledgeBatchTask struct {
	TaskID            string               `json:"task_id"`
	DomainID          string               `json:"domain_id"`
	RegistrationID    string               `json:"registration_id"`
	PreviewID         string               `json:"preview_id"`
	ManifestSHA256    string               `json:"manifest_sha256"`
	IdempotencyKey    string               `json:"idempotency_key"`
	ConfirmedItemIDs  []string             `json:"confirmed_item_ids"`
	RetryOfTaskID     *string              `json:"retry_of_task_id"`
	Status            string               `json:"status"`
	Revision          int64                `json:"revision"`
	Counts            KnowledgeBatchCounts `json:"counts"`
	CancelRequested   bool                 `json:"cancel_requested"`
	Mode              string               `json:"mode"`
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
	ExecutionDeadline string               `json:"execution_deadline"`
	TerminalAt        *string              `json:"terminal_at"`
}

type KnowledgeBatchTaskItem struct {
	ItemID           string                           `json:"item_id"`
	Ordinal          int64                            `json:"ordinal"`
	RelativePath     string                           `json:"relative_path"`
	Classification   KnowledgeDirectoryClassification `json:"classification"`
	SHA256           string                           `json:"sha256"`
	ByteLength       int64                            `json:"byte_length"`
	Status           string                           `json:"status"`
	SaveStatus       *string                          `json:"save_status"`
	SourceID         *string                          `json:"source_id"`
	SourceRevisionID *string                          `json:"source_revision_id"`
	RootNodeID       *string                          `json:"root_node_id"`
	CatalogueStatus  string                           `json:"catalogue_status"`
	StructureStatus  string                           `json:"structure_status"`
	FullTextStatus   string                           `json:"full_text_status"`
	VectorStatus     string                           `json:"vector_status"`
	Code             string                           `json:"code"`
}

type KnowledgeBatchTaskPage struct {
	DomainID       string               `json:"domain_id"`
	RegistrationID string               `json:"registration_id"`
	ListSnapshotID string               `json:"list_snapshot_id"`
	TotalTasks     int64                `json:"total_tasks"`
	CapturedAt     string               `json:"captured_at"`
	ExpiresAt      string               `json:"expires_at"`
	Items          []KnowledgeBatchTask `json:"items"`
	NextCursor     *string              `json:"next_cursor"`
}

type KnowledgeBatchTaskItemsPage struct {
	TaskID         string                   `json:"task_id"`
	RegistrationID string                   `json:"registration_id"`
	ResultRevision int64                    `json:"result_revision"`
	Status         string                   `json:"status"`
	Counts         KnowledgeBatchCounts     `json:"counts"`
	Items          []KnowledgeBatchTaskItem `json:"items"`
	NextCursor     *string                  `json:"next_cursor"`
}

type KnowledgeBatchCancellationRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type KnowledgeProjectManifestPreviewRequest struct {
	ProjectID          string  `json:"project_id"`
	TaskID             string  `json:"task_id"`
	ExpectedManifestID *string `json:"expected_manifest_id"`
}

type KnowledgeProjectManifestEntry struct {
	RegistrationID   string                           `json:"registration_id"`
	RelativePath     string                           `json:"relative_path"`
	SourceID         string                           `json:"source_id"`
	SourceRevisionID string                           `json:"source_revision_id"`
	RootNodeID       string                           `json:"root_node_id"`
	SHA256           string                           `json:"sha256"`
	ByteLength       int64                            `json:"byte_length"`
	Classification   KnowledgeDirectoryClassification `json:"classification"`
	OriginTaskID     string                           `json:"origin_task_id"`
}

type KnowledgeProjectManifestPreviewPage struct {
	PreviewID          string                          `json:"preview_id"`
	DomainID           string                          `json:"domain_id"`
	ProjectID          string                          `json:"project_id"`
	TaskID             string                          `json:"task_id"`
	TaskManifestSHA256 string                          `json:"task_manifest_sha256"`
	ExpectedManifestID *string                         `json:"expected_manifest_id"`
	ManifestSHA256     string                          `json:"manifest_sha256"`
	Coverage           string                          `json:"coverage"`
	CapturedAtomically bool                            `json:"captured_atomically"`
	CreatedAt          string                          `json:"created_at"`
	ExpiresAt          string                          `json:"expires_at"`
	TotalEntries       int64                           `json:"total_entries"`
	Added              int64                           `json:"added"`
	Replaced           int64                           `json:"replaced"`
	Carried            int64                           `json:"carried"`
	Items              []KnowledgeProjectManifestEntry `json:"items"`
	NextCursor         *string                         `json:"next_cursor"`
	ConsumedManifestID *string                         `json:"consumed_manifest_id"`
}

type KnowledgeProjectManifestPublicationRequest struct {
	ProjectID      string `json:"project_id"`
	PreviewID      string `json:"preview_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	IdempotencyKey string `json:"idempotency_key"`
}

type KnowledgeProjectManifestReceipt struct {
	DomainID           string  `json:"domain_id"`
	ProjectID          string  `json:"project_id"`
	ManifestID         string  `json:"manifest_id"`
	PreviewID          string  `json:"preview_id"`
	TaskID             string  `json:"task_id"`
	ManifestSHA256     string  `json:"manifest_sha256"`
	IdempotencyKey     string  `json:"idempotency_key"`
	ExpectedManifestID *string `json:"expected_manifest_id"`
	Coverage           string  `json:"coverage"`
	CapturedAtomically bool    `json:"captured_atomically"`
	TotalEntries       int64   `json:"total_entries"`
	PublishedAt        string  `json:"published_at"`
}

type KnowledgeProjectManifestHead struct {
	DomainID   string  `json:"domain_id"`
	ProjectID  string  `json:"project_id"`
	ManifestID *string `json:"manifest_id"`
}

type KnowledgeProjectManifestVersionPage struct {
	DomainID           string                          `json:"domain_id"`
	ProjectID          string                          `json:"project_id"`
	ManifestID         string                          `json:"manifest_id"`
	PreviewID          string                          `json:"preview_id"`
	TaskID             string                          `json:"task_id"`
	ManifestSHA256     string                          `json:"manifest_sha256"`
	ExpectedManifestID *string                         `json:"expected_manifest_id"`
	Coverage           string                          `json:"coverage"`
	CapturedAtomically bool                            `json:"captured_atomically"`
	TotalEntries       int64                           `json:"total_entries"`
	PublishedAt        string                          `json:"published_at"`
	Items              []KnowledgeProjectManifestEntry `json:"items"`
	NextCursor         *string                         `json:"next_cursor"`
}

type KnowledgeProjectManifestHistoryPage struct {
	DomainID       string                            `json:"domain_id"`
	ProjectID      string                            `json:"project_id"`
	ListSnapshotID string                            `json:"list_snapshot_id"`
	TotalManifests int64                             `json:"total_manifests"`
	CapturedAt     string                            `json:"captured_at"`
	ExpiresAt      string                            `json:"expires_at"`
	Items          []KnowledgeProjectManifestReceipt `json:"items"`
	NextCursor     *string                           `json:"next_cursor"`
}

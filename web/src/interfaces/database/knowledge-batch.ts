import type { DirectoryClassification } from './knowledge-directory';

export type BatchCapabilities = {
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  status: 'Frozen';
  implemented: true;
  task_execution: 'durable_bounded_batch';
  prerequisite_directory_version: '0.3.0';
  mode: 'catalogue_only';
  max_items: 32;
  max_file_bytes: 2097152;
  max_batch_bytes: 67108864;
  max_manifest_entries: 1024;
  model_calls: 0;
  external_models: false;
  automatic_ingestion: false;
  allowed_model_uses: Array<string>;
};

export type BatchClassification = DirectoryClassification;

export type BatchPreviewItem = {
  item_id: string;
  ordinal: number;
  relative_path: string;
  classification: BatchClassification;
  source_id: string | null;
  expected_revision_id: string | null;
  sha256: string;
  byte_length: number;
  action: 'add' | 'update' | 'unchanged';
};

export type BatchPreview = {
  preview_id: string;
  domain_id: string;
  registration_id: string;
  observation_id: string;
  observation_sha256: string;
  root_id: string;
  root_version: number;
  policy_version: number;
  manifest_sha256: string;
  mode: 'catalogue_only';
  created_at: string;
  expires_at: string;
  byte_length: number;
  items: Array<BatchPreviewItem>;
  consumed_task_id: string | null;
};

export type BatchContentWindow = {
  preview_id: string;
  item_id: string;
  relative_path: string;
  sha256: string;
  total_bytes: number;
  offset: number;
  byte_length: number;
  text: string;
  next_offset: number | null;
};

export type BatchCounts = {
  total: number;
  pending: number;
  running: number;
  succeeded: number;
  failed: number;
  cancelled: number;
};

export type BatchTask = {
  task_id: string;
  domain_id: string;
  registration_id: string;
  preview_id: string;
  manifest_sha256: string;
  idempotency_key: string;
  confirmed_item_ids: Array<string>;
  retry_of_task_id: string | null;
  status:
    | 'queued'
    | 'running'
    | 'cancelling'
    | 'succeeded'
    | 'partial_failed'
    | 'failed'
    | 'cancelled';
  revision: number;
  counts: BatchCounts;
  cancel_requested: boolean;
  mode: 'catalogue_only';
  created_at: string;
  updated_at: string;
  execution_deadline: string;
  terminal_at: string | null;
};

export type BatchTaskItem = {
  item_id: string;
  ordinal: number;
  relative_path: string;
  classification: BatchClassification;
  sha256: string;
  byte_length: number;
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled';
  save_status: 'created' | 'updated' | 'unchanged' | null;
  source_id: string | null;
  source_revision_id: string | null;
  root_node_id: string | null;
  catalogue_status: 'pending' | 'registered' | 'failed' | 'cancelled';
  structure_status: 'deferred';
  full_text_status: 'disabled';
  vector_status: 'disabled';
  code:
    | 'awaiting_execution'
    | 'running'
    | 'saved'
    | 'source_changed'
    | 'revision_changed'
    | 'root_changed'
    | 'policy_changed'
    | 'root_unready'
    | 'permission_revoked'
    | 'save_failed'
    | 'execution_expired'
    | 'cancelled';
};

export type BatchTaskPage = {
  domain_id: string;
  registration_id: string;
  list_snapshot_id: string;
  total_tasks: number;
  captured_at: string;
  expires_at: string;
  items: Array<BatchTask>;
  next_cursor: string | null;
};

export type BatchTaskItemsPage = {
  task_id: string;
  registration_id: string;
  result_revision: number;
  status:
    | 'queued'
    | 'running'
    | 'cancelling'
    | 'succeeded'
    | 'partial_failed'
    | 'failed'
    | 'cancelled';
  counts: BatchCounts;
  items: Array<BatchTaskItem>;
  next_cursor: string | null;
};

export type BatchManifestEntry = {
  registration_id: string;
  relative_path: string;
  source_id: string;
  source_revision_id: string;
  root_node_id: string;
  sha256: string;
  byte_length: number;
  classification: BatchClassification;
  origin_task_id: string;
};

export type BatchManifestPreviewPage = {
  preview_id: string;
  domain_id: string;
  project_id: string;
  task_id: string;
  task_manifest_sha256: string;
  expected_manifest_id: string | null;
  manifest_sha256: string;
  coverage: 'explicit_selection';
  captured_atomically: false;
  created_at: string;
  expires_at: string;
  total_entries: number;
  added: number;
  replaced: number;
  carried: number;
  items: Array<BatchManifestEntry>;
  next_cursor: string | null;
  consumed_manifest_id: string | null;
};

export type BatchManifestReceipt = {
  domain_id: string;
  project_id: string;
  manifest_id: string;
  preview_id: string;
  task_id: string;
  manifest_sha256: string;
  idempotency_key: string;
  expected_manifest_id: string | null;
  coverage: 'explicit_selection';
  captured_atomically: false;
  total_entries: number;
  published_at: string;
};

export type BatchManifestHead = {
  domain_id: string;
  project_id: string;
  manifest_id: string | null;
};

export type BatchManifestVersionPage = {
  domain_id: string;
  project_id: string;
  manifest_id: string;
  preview_id: string;
  task_id: string;
  manifest_sha256: string;
  expected_manifest_id: string | null;
  coverage: 'explicit_selection';
  captured_atomically: false;
  total_entries: number;
  published_at: string;
  items: Array<BatchManifestEntry>;
  next_cursor: string | null;
};

export type BatchManifestHistoryPage = {
  domain_id: string;
  project_id: string;
  list_snapshot_id: string;
  total_manifests: number;
  captured_at: string;
  expires_at: string;
  items: Array<BatchManifestReceipt>;
  next_cursor: string | null;
};

export type BatchError = {
  code:
    | 'knowledge_batch_invalid'
    | 'knowledge_batch_not_found'
    | 'knowledge_batch_version_mismatch'
    | 'knowledge_batch_conflict'
    | 'knowledge_batch_expired'
    | 'knowledge_batch_too_large'
    | 'knowledge_batch_range'
    | 'knowledge_batch_capacity'
    | 'knowledge_batch_unavailable';
  message: string;
  reason:
    | 'observation_incomplete'
    | 'observation_mismatch'
    | 'ineligible'
    | 'source_changed'
    | 'revision_changed'
    | 'root_changed'
    | 'policy_changed'
    | 'preview_consumed'
    | 'idempotency_conflict'
    | 'confirmation_mismatch'
    | 'result_changed'
    | 'task_not_succeeded'
    | 'project_head_changed'
    | 'project_membership_changed'
    | 'cursor_invalid'
    | 'retry_parent_invalid'
    | 'root_unready'
    | 'result_unknown'
    | null;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchNativeUnauthorized = {
  code: number;
  message: string;
  data?: unknown;
  [key: string]: unknown;
};

export type BatchCapabilitiesEnvelope = {
  code: 0;
  message: string;
  data: BatchCapabilities;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchBatchPreviewEnvelope = {
  code: 0;
  message: string;
  data: BatchPreview;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchContentWindowEnvelope = {
  code: 0;
  message: string;
  data: BatchContentWindow;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchTaskEnvelope = {
  code: 0;
  message: string;
  data: BatchTask;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchTaskPageEnvelope = {
  code: 0;
  message: string;
  data: BatchTaskPage;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchTaskItemsPageEnvelope = {
  code: 0;
  message: string;
  data: BatchTaskItemsPage;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchManifestPreviewPageEnvelope = {
  code: 0;
  message: string;
  data: BatchManifestPreviewPage;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchManifestReceiptEnvelope = {
  code: 0;
  message: string;
  data: BatchManifestReceipt;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchManifestHeadEnvelope = {
  code: 0;
  message: string;
  data: BatchManifestHead;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchManifestVersionPageEnvelope = {
  code: 0;
  message: string;
  data: BatchManifestVersionPage;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

export type BatchManifestHistoryPageEnvelope = {
  code: 0;
  message: string;
  data: BatchManifestHistoryPage;
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  request_id: string;
};

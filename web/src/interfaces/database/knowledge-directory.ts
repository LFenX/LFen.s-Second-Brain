export type DirectoryClassification = {
  group_id: string | null;
  component_side: 'frontend' | 'backend' | 'shared' | 'cli' | 'non_code';
  language: string;
};

export type DirectoryRoot = {
  root_id: string;
  display_name: string;
  root_version: number;
  runtime_platform: 'linux' | 'windows';
  ready: boolean;
  reason: 'platform_unverified' | 'unavailable' | null;
};

export type DirectoryRoots = {
  domain_id: string;
  items: DirectoryRoot[];
};

export type DirectoryRegistration = {
  registration_key: string;
  name: string;
  source_kind: 'controlled_directory';
  input_mode: 'server_read';
  root_id: string;
  scope_directories: string[];
  default_classification: DirectoryClassification;
  registration_id: string;
  domain_id: string;
  created_at: string;
};

export type DirectoryEntry = {
  relative_path: string;
  entry_kind: 'file' | 'directory' | 'blocked';
  byte_length: number | null;
  selectable: boolean;
  reason:
    | 'link'
    | 'hardlink'
    | 'not_regular'
    | 'unreadable'
    | 'identity_unverified'
    | null;
};

export type DirectoryListing = {
  listing_id: string;
  domain_id: string;
  registration_id: string;
  root_id: string;
  root_version: number;
  relative_directory: string;
  name_prefix: string;
  captured_at: string;
  expires_at: string;
  complete: boolean;
  reason:
    | 'limit_reached'
    | 'read_error'
    | 'timeout'
    | 'changed_during_read'
    | null;
  total_entries: number;
  items: DirectoryEntry[];
  next_cursor: string | null;
};

export type DirectoryObservationItem = {
  relative_path: string;
  state: 'verified' | 'ineligible' | 'missing' | 'unobserved';
  eligible: boolean;
  reason:
    | 'unsupported_format'
    | 'too_large'
    | 'invalid_utf8'
    | 'nul_byte'
    | 'secret_marker'
    | 'not_regular'
    | 'link'
    | 'hardlink'
    | 'identity_unverified'
    | 'missing'
    | 'unreadable'
    | 'changed_during_read'
    | 'timeout'
    | 'budget_exhausted'
    | 'platform_unverified'
    | null;
  byte_length: number | null;
  sha256: string | null;
  source_id: string | null;
  expected_revision_id: string | null;
  change_kind:
    | 'add'
    | 'update'
    | 'unchanged'
    | 'blocked'
    | 'missing_proposal'
    | 'unobserved';
};

export type DirectoryObservation = {
  observation_id: string;
  domain_id: string;
  registration_id: string;
  root_id: string;
  root_version: number;
  policy_version: number;
  default_classification: DirectoryClassification;
  selected_paths: string[];
  complete: boolean;
  observation_sha256: string;
  captured_at: string;
  expires_at: string;
  items: DirectoryObservationItem[];
};

export type DirectoryPreviewItem = {
  relative_path: string;
  classification: DirectoryClassification;
  action: 'add' | 'update' | 'unchanged';
  source_id: string | null;
  expected_revision_id: string | null;
  sha256: string;
  byte_length: number;
};

export type DirectoryPreview = {
  preview_id: string;
  domain_id: string;
  registration_id: string;
  root_id: string;
  root_version: number;
  policy_version: number;
  observation_id: string;
  observation_sha256: string;
  manifest_sha256: string;
  captured_at: string;
  expires_at: string;
  consumed_task_id: string | null;
  items: [DirectoryPreviewItem];
};

export type DirectoryContentWindow = {
  preview_id: string;
  offset: number;
  byte_length: number;
  total_bytes: number;
  sha256: string;
  text: string;
};

export type DirectoryResult = {
  relative_path: string;
  classification: DirectoryClassification;
  save_status: 'created' | 'updated' | 'unchanged' | 'failed';
  catalogue_status: 'registered' | 'failed';
  structure_status: 'deferred';
  full_text_status: 'disabled';
  vector_status: 'disabled';
  source_id: string | null;
  source_revision_id: string | null;
  root_node_id: string | null;
  sha256: string;
  byte_length: number;
  code: 'saved' | 'save_failed';
};

export type DirectoryCapabilities = {
  contract: 'C2';
  version: '0.3.0';
  status: 'Frozen';
  implemented: true;
  task_execution: 'synchronous_single_item';
  source_kind: 'controlled_directory';
  input_mode: 'server_read';
  mode: 'catalogue_only';
  structure: 'deferred';
  full_text: 'disabled';
  vector: 'disabled';
  model_calls: 0;
  external_models: false;
  automatic_ingestion: false;
  allowed_model_uses: never[];
  text_extensions: [
    '.txt',
    '.md',
    '.markdown',
    '.ts',
    '.tsx',
    '.js',
    '.jsx',
    '.go',
    '.css',
    '.scss',
    '.json',
    '.yaml',
    '.yml',
    '.html',
    '.htm',
    '.sql',
    '.sh',
    '.c',
    '.cpp',
    '.h',
    '.hpp',
    '.java',
    '.rs',
    '.py',
  ];
  limits: {
    roots_per_domain: 32;
    scope_directories: 16;
    listing_entries: 512;
    listing_page_size: 50;
    selected_paths: 32;
    source_bytes: 2097152;
    observation_read_bytes: 67108864;
    request_json_bytes: 32768;
    response_json_bytes: 262144;
    content_window_bytes: 65536;
    content_response_json_bytes: 1048576;
    listing_timeout_seconds: 10;
    observation_timeout_seconds: 30;
    preview_read_timeout_seconds: 10;
    confirmation_read_timeout_seconds: 10;
    key_lookup_wait_seconds: 10;
    snapshot_ttl_seconds: 600;
    preview_max_ttl_seconds: 600;
    unconsumed_preview_metadata_hours: 24;
    active_listings: 8;
    active_observations: 4;
    active_previews: 4;
    path_utf8_bytes: 512;
    path_depth: 32;
    name_prefix_utf8_bytes: 128;
  };
  receipt_retention: 'no_automatic_cleanup';
};

export type DirectoryError = {
  code:
    | 'knowledge_directory_invalid'
    | 'knowledge_directory_not_found'
    | 'knowledge_directory_version_mismatch'
    | 'knowledge_directory_conflict'
    | 'knowledge_directory_expired'
    | 'knowledge_directory_too_large'
    | 'knowledge_directory_unsupported'
    | 'knowledge_directory_range'
    | 'knowledge_directory_capacity'
    | 'knowledge_directory_unavailable';
  reason:
    | 'root_changed'
    | 'policy_changed'
    | 'source_changed'
    | 'revision_changed'
    | 'observation_incomplete'
    | 'observation_mismatch'
    | 'ineligible'
    | 'preview_consumed'
    | 'idempotency_conflict'
    | 'root_unready'
    | 'result_unknown'
    | null;
  message: string;
  data: null;
  contract_version: '1.0.0';
  source_contract_version: '0.3.0';
  request_id: string;
};

export type DirectorySavedResult = DirectoryResult & {
  save_status: 'created' | 'updated' | 'unchanged';
  source_id: string;
  source_revision_id: string;
  root_node_id: string;
};
export type DirectoryFailedResult = DirectoryResult & {
  save_status: 'failed';
  source_id: null;
  source_revision_id: null;
  root_node_id: null;
};
type DirectoryReceiptBase = {
  import_task_id: string;
  domain_id: string;
  registration_id: string;
  preview_id: string;
  observation_id: string;
  manifest_sha256: string;
  model_calls: 0;
  mode: 'catalogue_only';
  created_at: string;
};
export type DirectoryReceipt = DirectoryReceiptBase &
  (
    | { status: 'succeeded'; items: [DirectorySavedResult] }
    | { status: 'failed'; items: [DirectoryFailedResult] }
  );

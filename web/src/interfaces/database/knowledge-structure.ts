import type {
  StructurePolicyChangeRequest,
  StructureTaskRequest,
  StructureCancelRequest,
} from '@/interfaces/request/knowledge-structure';

export type StructureID = string;

export type StructureHash = string;

export type StructureKey = string;

export type StructureVersion = number;

export type StructureTimestamp = string;

export type StructureCursor = StructureID | null;

export type StructureParser = 'typescript' | 'tsx' | 'go' | 'markdown';

export type StructureClassification = {
  group_id: string | null;
  component_side: 'frontend' | 'backend' | 'shared' | 'cli' | 'non_code';
  language: string;
};

export type StructureAlias = {
  prefix: string;
  target: string;
};

export type StructureMember = {
  source_id: StructureID;
  source_revision_id: StructureID;
  lifecycle_version: StructureVersion;
  parser: StructureParser;
};

export type StructurePolicy = {
  domain_id: StructureID;
  foundation_policy_version: StructureVersion;
  structure_policy_version: StructureVersion;
  enabled: boolean;
  mode: 'structure_only';
  automatic_ingestion: false;
  allowed_model_uses: string[];
  external_models: false;
  local_inference: false;
  lexical_index: 'not_created';
  vector_index: 'not_created';
};

export type StructurePolicySnapshot = {
  structure_policy_version: StructureVersion;
  enabled: boolean;
};

export type StructurePolicyReceipt = {
  command_id: StructureID;
  kind: 'policy';
  domain_id: StructureID;
  owner_id: StructureID;
  idempotency_key: StructureKey;
  original_request: StructurePolicyChangeRequest;
  request_sha256: StructureHash;
  before: StructurePolicySnapshot;
  after: StructurePolicySnapshot;
  outcome: 'applied' | 'unchanged';
  completed_at: StructureTimestamp;
  model_calls: 0;
};

export type StructureTaskState =
  | 'queued'
  | 'running'
  | 'cancel_requested'
  | 'succeeded'
  | 'failed'
  | 'cancelled';

export type StructureMemberState =
  | 'pending'
  | 'processing'
  | 'staged'
  | 'published'
  | 'reused'
  | 'failed'
  | 'cancelled';

export type StructureDiagnostic = {
  code: string;
  message: string;
  start_byte: number | null;
  end_byte: number | null;
};

export type StructureTaskMember = {
  source_id: StructureID;
  source_revision_id: StructureID;
  lifecycle_version: StructureVersion;
  parser: StructureParser;
  state: StructureMemberState;
  diagnostics: StructureDiagnostic[];
  node_count: number;
  relation_count: number;
};

export type StructureTask = {
  task_id: StructureID;
  owner_id: StructureID;
  domain_id: StructureID;
  project_id: StructureID;
  original_request: StructureTaskRequest;
  request_sha256: StructureHash;
  producer_sha256: StructureHash;
  revision: StructureVersion;
  attempt_count: number;
  state: StructureTaskState;
  created_at: StructureTimestamp;
  updated_at: StructureTimestamp;
  deadline_at: StructureTimestamp;
  total_members: number;
  finished_members: number;
  members: StructureTaskMember[];
  result_view_id: StructureHash | null;
  failure_code: string | null;
  model_calls: 0;
  local_inference: 0;
  external_fetches: 0;
  completed_at: StructureTimestamp | null;
};

export type StructureCancelReceipt = {
  command_id: StructureID;
  kind: 'cancel';
  domain_id: StructureID;
  owner_id: StructureID;
  task_id: StructureID;
  idempotency_key: StructureKey;
  original_request: StructureCancelRequest;
  request_sha256: StructureHash;
  before_state: StructureTaskState;
  after_state: StructureTaskState;
  task_revision: StructureVersion;
  outcome: 'requested' | 'unchanged';
  completed_at: StructureTimestamp;
  model_calls: 0;
};

export type StructureCommandReceipt =
  | StructurePolicyReceipt
  | StructureCancelReceipt
  | StructureCommandRejection;

export type StructureViewMember = {
  source_id: StructureID;
  source_revision_id: StructureID;
  source_node_id: StructureHash;
  root_structure_node_id: StructureHash;
  lifecycle_version: StructureVersion;
  parser: StructureParser;
  sha256: StructureHash;
  byte_length: number;
  relative_path: string;
  classification: StructureClassification;
};

export type StructureView = {
  view_id: StructureHash;
  domain_id: StructureID;
  owner_id: StructureID;
  project_id: StructureID;
  manifest_id: StructureID;
  manifest_sha256: StructureHash;
  foundation_policy_version: StructureVersion;
  structure_policy_version: StructureVersion;
  producer_sha256: StructureHash;
  producer_format: 'code-structure-v1';
  aliases: StructureAlias[];
  members: StructureViewMember[];
  source_count: number;
  node_count: number;
  relation_count: number;
  source_bytes: number;
  published_at: StructureTimestamp;
  model_calls: 0;
  local_inference: 0;
  lexical_index: 'not_created';
  vector_index: 'not_created';
};

export type StructureHead = {
  domain_id: StructureID;
  project_id: StructureID;
  view_id: StructureHash | null;
  current_manifest_id: StructureID | null;
  currentness: 'none' | 'current' | 'stale';
  reasons: (
    | 'manifest_changed'
    | 'source_changed'
    | 'lifecycle_changed'
    | 'classification_changed'
    | 'position_changed'
    | 'policy_changed'
    | 'source_archived'
  )[];
  checked_at: StructureTimestamp;
  affected_sources: StructureHeadIssue[];
};

export type StructureNode = {
  structure_node_id: StructureHash;
  logical_node_id: StructureHash;
  view_id: StructureHash;
  source_id: StructureID;
  source_revision_id: StructureID;
  source_node_id: StructureHash;
  parent_id: StructureHash | null;
  kind: 'file' | 'symbol' | 'section' | 'passage';
  name: string;
  logical_key: string;
  identity_stability: 'file' | 'name_scope' | 'position_based' | 'range_based';
  start_byte: number;
  end_byte: number;
  start_line: number;
  end_line: number;
  parser: StructureParser;
  classification: StructureClassification;
  coordinate_space: 'source_utf8';
  producer_sha256: StructureHash;
};

export type StructureContent = {
  view_id: StructureHash;
  structure_node_id: StructureHash;
  source_node_id: StructureHash;
  source_id: StructureID;
  source_revision_id: StructureID;
  source_sha256: StructureHash;
  fragment_sha256: StructureHash;
  coordinate_space: 'source_utf8';
  start_byte: number;
  end_byte: number;
  text: string;
  complete: boolean;
  next_start_byte: number | null;
};

export type StructureEvidence = {
  source_id: StructureID;
  source_revision_id: StructureID;
  start_byte: number;
  end_byte: number;
  fragment_sha256: StructureHash;
};

export type StructureRelationKind =
  | 'contains'
  | 'imports_file'
  | 'exports_symbol'
  | 'references_type'
  | 'calls_local';

export type StructureRelation = {
  relation_id: StructureHash;
  view_id: StructureHash;
  kind: StructureRelationKind;
  from_node_id: StructureHash;
  to_node_id: StructureHash | null;
  resolution: 'resolved' | 'unresolved';
  reason:
    | 'syntax_containment'
    | 'explicit_relative'
    | 'explicit_alias'
    | 'named_export'
    | 'lexical_binding'
    | 'external_package'
    | 'outside_selection'
    | 'ambiguous'
    | 'dynamic_import'
    | 'dynamic_call'
    | 'wildcard_export'
    | 'cycle'
    | 'unsupported_target'
    | 'go_package_unresolved'
    | 'reference_too_long';
  reference: string;
  reference_truncated: boolean;
  evidence: StructureEvidence;
};

export type StructureNodePage = {
  view_id: StructureHash;
  source_id: StructureID;
  parent_id: StructureHash | null;
  items: StructureNode[];
  next_cursor: StructureCursor;
};

export type StructureRelationPage = {
  view_id: StructureHash;
  node_id: StructureHash;
  direction: 'in' | 'out' | 'both';
  kind: StructureRelationKind | null;
  items: StructureRelation[];
  next_cursor: StructureCursor;
};

export type StructureTaskPage = {
  domain_id: StructureID;
  project_id: StructureID;
  captured_at: StructureTimestamp;
  items: StructureTask[];
  next_cursor: StructureCursor;
};

export type StructureLimits = {
  max_files: 32;
  max_file_bytes: 2097152;
  max_input_bytes: 16777216;
  max_nodes_per_file: 8192;
  max_nodes: 32768;
  max_relations: 65536;
  max_depth: 16;
  passage_target_bytes: 4096;
  passage_max_bytes: 8192;
  content_max_bytes: 65536;
  page_max_items: 20;
  metadata_response_max_bytes: 262144;
  content_response_max_bytes: 524288;
  request_max_bytes: 32768;
  cursor_ttl_seconds: 600;
  lease_seconds: 30;
  lease_renew_seconds: 10;
  max_attempts: 3;
  task_deadline_seconds: 120;
  attempt_soft_timeout_seconds: 30;
  ts_parse_timeout_seconds: 5;
  client_wait_seconds: 25;
  recovery_slot_max_bytes: 16384;
  recovery_slots: 8;
};

export type StructureProducers = {
  format: 'code-structure-v1';
  go: 'standard-go-parser';
  go_toolchain: 'go1.27.3';
  tree_sitter_go: 'v0.24.0';
  tree_sitter_typescript: 'v0.23.2';
  goldmark: 'v1.7.1';
  coordinate_space: 'source_utf8';
  parse_ctx: false;
  remote_images: false;
  overlap_bytes: 0;
};

export type StructureCapabilities = {
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  runtime_ready: boolean;
  owner_only: true;
  mode: 'structure_only';
  parser_ids: ['typescript', 'tsx', 'go', 'markdown'];
  policy_default_enabled: false;
  model_calls: 0;
  local_inference: 0;
  external_fetches: 0;
  automatic_ingestion: false;
  lexical_index: 'not_created';
  vector_index: 'not_created';
  partial_publication: false;
  producers: StructureProducers;
  limits: StructureLimits;
};

export type StructureError = {
  code:
    | 'invalid_request'
    | 'not_found'
    | 'contract_mismatch'
    | 'policy_disabled'
    | 'policy_changed'
    | 'revision_changed'
    | 'manifest_changed'
    | 'view_changed'
    | 'idempotency_conflict'
    | 'task_revision_changed'
    | 'capacity_exceeded'
    | 'source_unavailable'
    | 'lifecycle_integrity'
    | 'structure_integrity'
    | 'unavailable'
    | 'unsupported_source';
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: null;
  outcome_known: boolean;
  rejection_id: StructureID | null;
};

export type StructureHeadIssue = {
  source_id: StructureID;
  reasons: (
    | 'manifest_changed'
    | 'source_changed'
    | 'lifecycle_changed'
    | 'classification_changed'
    | 'position_changed'
    | 'policy_changed'
    | 'source_archived'
  )[];
};

export type StructureRejectionReceipt = {
  rejection_id: StructureID;
  operation: 'task' | 'policy' | 'cancel';
  owner_id: StructureID;
  domain_id: StructureID;
  task_id: StructureID | null;
  idempotency_key: StructureKey;
  original_request:
    | StructureTaskRequest
    | StructurePolicyChangeRequest
    | StructureCancelRequest;
  request_sha256: StructureHash;
  reason:
    | 'policy_disabled'
    | 'policy_changed'
    | 'revision_changed'
    | 'manifest_changed'
    | 'view_changed'
    | 'task_revision_changed'
    | 'capacity_exceeded'
    | 'unsupported_source';
  recorded_at: StructureTimestamp;
  accepted: false;
  model_calls: 0;
};

export type StructureTaskRejection = StructureRejectionReceipt & {
  operation: 'task';
};

export type StructureCommandRejection = StructureRejectionReceipt & {
  operation: 'policy' | 'cancel';
};

export type StructureTaskOutcome = StructureTask | StructureTaskRejection;

export type StructureCapabilitiesResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureCapabilities;
};

export type StructurePolicyResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructurePolicy;
};

export type StructurePolicyReceiptResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructurePolicyReceipt;
};

export type StructureCommandReceiptResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureCommandReceipt;
};

export type StructureTaskResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureTask;
};

export type StructureTaskOutcomeResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureTaskOutcome;
};

export type StructureTaskPageResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureTaskPage;
};

export type StructureCancelReceiptResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureCancelReceipt;
};

export type StructureViewResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureView;
};

export type StructureHeadResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureHead;
};

export type StructureNodePageResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureNodePage;
};

export type StructureNodeResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureNode;
};

export type StructureContentResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureContent;
};

export type StructureRelationPageResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  structure_contract_version: '0.1.0';
  request_id: StructureID;
  data: StructureRelationPage;
};

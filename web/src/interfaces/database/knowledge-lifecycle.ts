import type {
  LifecycleStateCommand,
  LifecycleReclassifyCommand,
  LifecycleMoveCommand,
} from '@/interfaces/request/knowledge-lifecycle';

export type LifecycleID = string;

export type LifecycleSHA256 = string;

export type LifecycleVersion = number;

export type LifecycleClassification = {
  group_id: LifecycleID | null;
  component_side: 'frontend' | 'backend' | 'shared' | 'cli' | 'non_code';
  language: string;
};

export type LifecycleSnapshot = {
  source_revision_id: LifecycleID;
  source_input_sha256: LifecycleSHA256;
  lifecycle_version: LifecycleVersion;
  state: 'active' | 'archived';
  relative_path: string;
  classification: LifecycleClassification;
  sha256: LifecycleSHA256;
  byte_length: number;
};

export type LifecycleActionCapability = {
  supported: boolean;
  allowed: boolean;
  reason:
    | 'source_archived'
    | 'action_unsupported'
    | 'target_not_registered'
    | 'target_occupied'
    | 'target_media_mismatch'
    | 'invalid_target'
    | null;
};

export type LifecycleDetail = {
  owner_id: LifecycleID;
  domain_id: LifecycleID;
  source_id: LifecycleID;
  native_file_id: LifecycleID;
  registration_id: LifecycleID | null;
  source_kind:
    | 'text_file'
    | 'controlled_directory'
    | 'original_document'
    | 'web_page'
    | 'web_feed'
    | null;
  representation_kind: 'text' | 'original_only' | 'web_capture';
  policy_version: LifecycleVersion;
  current: LifecycleSnapshot;
  actions: {
    reclassify: LifecycleActionCapability;
    move: LifecycleActionCapability;
    archive: LifecycleActionCapability;
    restore: LifecycleActionCapability;
  };
};

export type LifecyclePage = {
  items: LifecycleDetail[];
  next_cursor: LifecycleID | null;
};

export type LifecycleReceipt = {
  command_id: LifecycleID;
  owner_id: LifecycleID;
  domain_id: LifecycleID;
  source_id: LifecycleID;
  action: 'reclassify' | 'move' | 'archive' | 'restore';
  idempotency_key: string;
  request_sha256: LifecycleSHA256;
  request:
    | LifecycleStateCommand
    | LifecycleReclassifyCommand
    | LifecycleMoveCommand;
  result: 'applied' | 'unchanged';
  before: LifecycleSnapshot;
  after: LifecycleSnapshot;
  policy_version: LifecycleVersion;
  created_at: string;
  model_calls: 0;
};

export type LifecycleTargetCheck = {
  domain_id: LifecycleID;
  source_id: LifecycleID;
  registration_id: LifecycleID | null;
  current_source_revision_id: LifecycleID;
  lifecycle_version: LifecycleVersion;
  policy_version: LifecycleVersion;
  relative_path: string;
  allowed: boolean;
  reason:
    | 'source_archived'
    | 'action_unsupported'
    | 'target_not_registered'
    | 'target_occupied'
    | 'target_media_mismatch'
    | 'invalid_target'
    | null;
  reserved: false;
};

export type LifecycleCapabilities = {
  source_contract_version: '0.7.0';
  runtime_ready: boolean;
  owner_only: true;
  actions: ['reclassify', 'move', 'archive', 'restore'];
  max_lifecycle_version: 9007199254740991;
  max_json_bytes: 32768;
  max_page_items: 20;
  cursor_ttl_seconds: 600;
  server_command_timeout_seconds: 10;
  client_wait_seconds: 25;
  max_recovery_slot_bytes: 8192;
  max_local_recovery_slots: 8;
  pending_preparations: 'unfenced_rejected_on_consume';
  web_fence_start: 'explicit_preview';
  archive_is_revocation: false;
  fixed_history_preserved: true;
  automatic_ingestion: false;
  physical_gc: false;
  model_calls: 0;
};

export type LifecycleCapabilitiesResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.7.0';
  request_id: LifecycleID;
  data: LifecycleCapabilities;
};

export type LifecycleDetailResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.7.0';
  request_id: LifecycleID;
  data: LifecycleDetail;
};

export type LifecyclePageResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.7.0';
  request_id: LifecycleID;
  data: LifecyclePage;
};

export type LifecycleTargetCheckResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.7.0';
  request_id: LifecycleID;
  data: LifecycleTargetCheck;
};

export type LifecycleReceiptResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.7.0';
  request_id: LifecycleID;
  data: LifecycleReceipt;
};

export type LifecycleError = {
  code:
    | 'knowledge_invalid_request'
    | 'knowledge_unauthorized'
    | 'knowledge_not_found'
    | 'knowledge_contract_version'
    | 'knowledge_source_contract_version'
    | 'policy_changed'
    | 'revision_changed'
    | 'idempotency_conflict'
    | 'lifecycle_source_archived'
    | 'lifecycle_action_unsupported'
    | 'lifecycle_position_occupied'
    | 'lifecycle_version_exhausted'
    | 'lifecycle_integrity'
    | 'lifecycle_budget_exceeded'
    | 'lifecycle_result_unknown';
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.7.0';
  request_id: LifecycleID;
  data: null;
  outcome_known: boolean;
};

import type {
  CreateOriginalUpload,
  OriginalRegistrationRequest,
} from '@/interfaces/request/knowledge-original';

export type OriginalClassification = {
  group_id: string | null;
  component_side: 'frontend' | 'backend' | 'shared' | 'cli' | 'non_code';
  language: string;
};
export type OriginalRegistration = OriginalRegistrationRequest & {
  registration_id: string;
  domain_id: string;
  config_sha256: string;
  created_at: string;
};
export type OriginalUploadState =
  | 'awaiting_bytes'
  | 'receiving'
  | 'ready'
  | 'interrupted'
  | 'rejected'
  | 'cancelled'
  | 'expired'
  | 'committed';
export type OriginalSaveStatus = 'created' | 'updated' | 'unchanged';
export type OriginalUpload = CreateOriginalUpload & {
  upload_id: string;
  domain_id: string;
  owner_id: string;
  registration_id: string;
  request_sha256: string;
  source_id: string | null;
  policy_version: number;
  state: OriginalUploadState;
  received_bytes: number;
  byte_length: number | null;
  sha256: string | null;
  media_type: string | null;
  action: OriginalSaveStatus | null;
  manifest_sha256: string | null;
  import_task_id: string | null;
  code: string | null;
  created_at: string;
  expires_at: string;
  revision: number;
  updated_at: string;
};
export type OriginalRevision = {
  domain_id: string;
  registration_id: string;
  source_id: string;
  source_revision_id: string;
  original_filename: string;
  relative_path: string;
  classification: OriginalClassification;
  sha256: string;
  byte_length: number;
  media_type: string;
  representation_kind: 'original_only';
  root_node_id: null;
  text_available: false;
  parse_status: 'not_parsed';
  structure_status: 'not_built';
  full_text_status: 'not_built';
  vector_status: 'not_built';
  is_current: boolean;
  created_at: string;
};
export type OriginalReceipt = {
  import_task_id: string;
  domain_id: string;
  registration_id: string;
  upload_id: string;
  idempotency_key: string;
  manifest_sha256: string;
  status: 'succeeded';
  save_status: OriginalSaveStatus;
  model_calls: 0;
  source: OriginalRevision;
  created_at: string;
};
export type OriginalTarget = {
  domain_id: string;
  registration_id: string;
  relative_path: string;
  source: OriginalRevision | null;
};
export type OriginalPage = {
  domain_id: string;
  items: OriginalRevision[];
  next_cursor: string | null;
};
export type OriginalCapabilities = {
  contract_version: '1.0.0';
  source_contract_version: '0.5.0';
  runtime_ready: boolean;
  max_file_bytes: 67108864;
  max_open_uploads_per_user_domain: 2;
  reserved_bytes_per_user_domain: 134217728;
  receiving_per_user_domain: 1;
  receiving_deployment: 2;
  upload_ttl_seconds: 1800;
  receive_timeout_seconds: 120;
  lease_seconds: 30;
  heartbeat_seconds: 10;
  json_max_bytes: 32768;
  supported_formats: ['pdf', 'pptx', 'docx', 'utf8_text_code'];
  resumable_parts: false;
  parses_automatically: false;
  model_calls: 0;
  storage_engine: 'minio';
  reading: 'authenticated_fixed_revision_single_range';
};

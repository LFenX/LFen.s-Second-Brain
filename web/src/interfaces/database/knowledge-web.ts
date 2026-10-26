import type {
  WebCreateCapture,
  WebCreatePreview,
  WebConfirm,
} from '@/interfaces/request/knowledge-web';

export type WebClassification = {
  group_id: string | null;
  component_side: 'frontend' | 'backend' | 'shared' | 'cli' | 'non_code';
  language: string;
};

export type WebProducerVersions = {
  normalizer_version: 'web-normalizer/0.6.0';
  converter_version: 'web-reading/0.6.0';
  sanitizer_version: 'web-sanitizer/0.6.0';
  resource_policy_version: 'web-resources/0.6.0';
};

export type WebNetworkScope =
  | {
      enabled: false;
      allowed_origins: [];
    }
  | {
      enabled: true;
      allowed_origins: string[];
    };

export type WebRegistration = {
  registration_key: string;
  name: string;
  source_kind: 'web_page' | 'web_feed';
  origin_url: string;
  producer: 'direct' | 'rsshub' | 'other_feed' | 'browser_export';
  default_classification: WebClassification;
  network_scope: WebNetworkScope;
  domain_id: string;
  owner_id: string;
  registration_id: string;
  config_sha256: string;
  scope_sha256: string;
  created_at: string;
  policy_version: number;
};

export type WebTarget = {
  domain_id: string;
  registration_id: string;
  entry_key: string;
  source_id: string | null;
  current_revision_id: string | null;
  current_input_sha256: string | null;
  current_classification: WebClassification | null;
};

export type WebFailure = {
  stage:
    | 'create'
    | 'upload'
    | 'fetch'
    | 'observe'
    | 'preview'
    | 'confirm'
    | 'read';
  machine_code:
    | 'knowledge_unauthorized'
    | 'knowledge_not_found'
    | 'knowledge_invalid_request'
    | 'knowledge_contract_version'
    | 'knowledge_source_contract_version'
    | 'registration_conflict'
    | 'idempotency_conflict'
    | 'policy_changed'
    | 'revision_changed'
    | 'manifest_changed'
    | 'web_state_conflict'
    | 'web_capture_busy'
    | 'web_capture_expired'
    | 'web_capture_cancelled'
    | 'web_capture_rejected'
    | 'web_capture_interrupted'
    | 'web_capture_quota_exceeded'
    | 'web_slots_busy'
    | 'web_length_mismatch'
    | 'web_sha256_mismatch'
    | 'web_too_large'
    | 'web_format_unsupported'
    | 'web_encoding_unsupported'
    | 'web_range_unsupported'
    | 'web_window_invalid'
    | 'web_remote_blocked'
    | 'web_redirect_denied'
    | 'web_compression_rejected'
    | 'web_upstream_http_error'
    | 'web_fetch_timeout'
    | 'web_upload_timeout'
    | 'web_parse_timeout'
    | 'web_parse_rejected'
    | 'web_budget_exceeded'
    | 'web_entry_conflict'
    | 'web_observation_not_ready'
    | 'web_preview_expired'
    | 'web_preview_cancelled'
    | 'web_preview_consumed'
    | 'web_capture_integrity'
    | 'web_storage_unavailable'
    | 'web_result_unknown'
    | 'knowledge_internal';
  upstream_http_status: number | null;
  outcome_known: boolean;
};

export type WebRedirect = {
  from_url: string;
  to_url: string;
  status: 301 | 302 | 303 | 307 | 308;
};

export type WebProvenance = {
  kind: 'user_supplied' | 'server_http';
  requested_url: string;
  final_url: string | null;
  claimed_captured_at: string | null;
  server_received_at: string | null;
  fetch_started_at: string | null;
  fetch_completed_at: string | null;
  upstream_http_status: 200 | null;
  redirect_chain: WebRedirect[];
  response_content_type: string | null;
  response_content_encoding: 'identity' | null;
  response_etag: string | null;
  response_last_modified: string | null;
};

export type WebCapture = {
  domain_id: string;
  owner_id: string;
  registration_id: string;
  capture_id: string;
  idempotency_key: string;
  request_sha256: string;
  registration_config_sha256: string;
  scope_sha256: string;
  acquisition_mode: 'local_snapshot' | 'explicit_fetch';
  state:
    | 'awaiting_bytes'
    | 'awaiting_fetch'
    | 'receiving'
    | 'fetching'
    | 'ready'
    | 'interrupted'
    | 'rejected'
    | 'cancelled'
    | 'expired';
  revision: number;
  policy_version: number;
  producer_versions: WebProducerVersions;
  created_at: string;
  updated_at: string;
  expires_at: string;
  attempt_started_at: string | null;
  attempt_deadline_at: string | null;
  original_filename: string | null;
  declared_byte_length: number | null;
  declared_media_type: string | null;
  expected_sha256: string | null;
  received_bytes: number;
  raw_sha256: string | null;
  raw_byte_length: number | null;
  media_type:
    | 'text/html'
    | 'application/rss+xml'
    | 'application/atom+xml'
    | null;
  provenance: WebProvenance;
  observation_id: string | null;
  failure: WebFailure | null;
};

export type WebObservation = {
  domain_id: string;
  registration_id: string;
  capture_id: string;
  observation_id: string;
  capture_sha256: string;
  producer_versions: WebProducerVersions;
  state: 'parsing' | 'ready' | 'interrupted' | 'rejected' | 'expired';
  revision: number;
  observation_sha256: string | null;
  created_at: string;
  expires_at: string;
  input_entry_count: number | null;
  entry_count: number | null;
  eligible_count: number | null;
  duplicate_count: number | null;
  capture_traversal_complete: boolean;
  decoded_bytes: number;
  reading_bytes: number;
  metadata_bytes: number;
  reference_bytes: number;
  reference_count: number;
  warnings: string[];
  failure: WebFailure | null;
};

export type WebRepresentation = {
  kind: 'decoded_content' | 'safe_reading';
  media_type: 'text/plain' | 'text/html' | 'application/xhtml+xml';
  encoding: 'utf-8';
  byte_length: number;
  sha256: string;
  converter_version: 'web-reading/0.6.0';
  sanitizer_version: 'web-sanitizer/0.6.0' | null;
  resource_policy_version: 'web-resources/0.6.0' | null;
};

export type WebAuthor = {
  name: string;
  uri: string | null;
  email: string | null;
};

export type WebMetadata = {
  title: string;
  authors: WebAuthor[];
  categories: string[];
  published: string | null;
  updated: string | null;
  canonical_declared: string | null;
  canonical_resolved: string | null;
  effective_base: string;
  body_completeness: 'summary' | 'unknown' | 'external_reference';
};

export type WebEntrySummary = {
  entry_key: string;
  identity_basis: 'page' | 'rss_guid' | 'atom_id' | 'weak_url' | 'missing';
  identity_strength: 'strong' | 'weak' | 'missing';
  title: string;
  body_completeness: 'summary' | 'unknown' | 'external_reference';
  eligible: boolean;
  ineligible_reason:
    | 'identity_missing'
    | 'body_missing'
    | 'reference_missing'
    | null;
  source_id: string | null;
  current_revision_id: string | null;
  decoded_byte_length: number;
  reading_byte_length: number;
  source_input_sha256: string | null;
  first_position: number;
};

export type WebEntry = {
  entry_key: string;
  identity_basis: 'page' | 'rss_guid' | 'atom_id' | 'weak_url' | 'missing';
  identity_strength: 'strong' | 'weak' | 'missing';
  title: string;
  body_completeness: 'summary' | 'unknown' | 'external_reference';
  eligible: boolean;
  ineligible_reason:
    | 'identity_missing'
    | 'body_missing'
    | 'reference_missing'
    | null;
  source_id: string | null;
  current_revision_id: string | null;
  decoded_byte_length: number;
  reading_byte_length: number;
  source_input_sha256: string | null;
  first_position: number;
  observation_id: string;
  capture_id: string;
  identity_value: string;
  positions: number[];
  metadata: WebMetadata;
  metadata_sha256: string;
  references_sha256: string;
  reference_count: number;
  representations: WebRepresentation[];
  producer_versions: WebProducerVersions;
};

export type WebReference = {
  reference_id: string;
  relation: 'link' | 'image' | 'enclosure' | 'external_content';
  raw_href: string;
  base_url: string;
  resolved_url: string | null;
  fragment: string | null;
  label: string;
  declared_media_type: string | null;
  open_allowed: boolean;
};

export type WebPreviewItem = {
  entry_key: string;
  title: string;
  source_id: string | null;
  expected_revision_id: string | null;
  classification: WebClassification;
  source_input_sha256: string;
  action: 'created' | 'updated' | 'unchanged';
  decoded_sha256: string;
  reading_sha256: string;
  body_completeness: 'summary' | 'unknown' | 'external_reference';
  acknowledgement:
    | 'summary_only'
    | 'unknown_completeness'
    | 'external_reference_only';
};

export type WebPreview = {
  domain_id: string;
  owner_id: string;
  registration_id: string;
  capture_id: string;
  capture_sha256: string;
  observation_id: string;
  observation_sha256: string;
  preview_id: string;
  idempotency_key: string;
  request_sha256: string;
  registration_config_sha256: string;
  scope_sha256: string;
  producer_versions: WebProducerVersions;
  policy_version: number;
  manifest_sha256: string;
  selection_sha256: string;
  selected_count: number;
  selected_byte_length: number;
  state: 'ready' | 'committed' | 'cancelled' | 'expired';
  revision: number;
  created_at: string;
  expires_at: string;
  import_task_id: string | null;
};

export type WebReceiptItem = {
  entry_key: string;
  source_id: string;
  source_revision_id: string;
  action: 'created' | 'updated' | 'unchanged';
  classification: WebClassification;
  source_input_sha256: string;
  decoded_sha256: string;
  reading_sha256: string;
};

export type WebReceipt = {
  domain_id: string;
  owner_id: string;
  registration_id: string;
  capture_id: string;
  capture_sha256: string;
  observation_id: string;
  observation_sha256: string;
  preview_id: string;
  preview_request_sha256: string;
  producer_versions: WebProducerVersions;
  policy_version: number;
  import_task_id: string;
  idempotency_key: string;
  request_sha256: string;
  manifest_sha256: string;
  selection_sha256: string;
  selected_count: number;
  status: 'completed';
  saved_count: number;
  unchanged_count: number;
  index_status: 'not_built';
  model_calls: 0;
  created_at: string;
};

export type WebRevision = {
  domain_id: string;
  registration_id: string;
  source_id: string;
  source_revision_id: string;
  entry_key: string;
  identity_basis: 'page' | 'rss_guid' | 'atom_id' | 'weak_url';
  identity_value: string;
  relative_path: string;
  classification: WebClassification;
  representation_kind: 'web_capture';
  root_node_id: null;
  source_input_sha256: string;
  capture_id: string;
  capture_sha256: string;
  observation_id: string;
  positions: number[];
  metadata: WebMetadata;
  metadata_sha256: string;
  references_sha256: string;
  reference_count: number;
  representations: WebRepresentation[];
  producer_versions: WebProducerVersions;
  text_available: boolean;
  structure_status: 'not_built';
  full_text_status: 'not_built';
  vector_status: 'not_built';
  is_current: boolean;
  created_at: string;
};

export type WebWindow = {
  domain_id: string;
  capture_id: string;
  observation_id: string;
  entry_key: string;
  source_revision_id: string | null;
  representation: WebRepresentation;
  start_byte: number;
  end_byte: number;
  next_start_byte: number | null;
  text: string;
};

export type WebCapabilities = {
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  runtime_ready: boolean;
  acquisition_modes: ['local_snapshot', 'explicit_fetch'];
  formats: ['html', 'rss2', 'atom_feed'];
  producer_versions: WebProducerVersions;
  limits: {
    max_raw_bytes: 2097152;
    max_capture_entries: 1000;
    max_parse_depth: 64;
    max_parse_nodes: 50000;
    max_selected: 32;
    max_selection_bytes: 8388608;
    max_entry_decoded_bytes: 2097152;
    max_entry_reading_bytes: 2097152;
    max_entry_metadata_bytes: 8192;
    max_entry_references: 256;
    max_entry_reference_bytes: 524288;
    max_reference_json_bytes: 16384;
    max_capture_decoded_bytes: 4194304;
    max_capture_reading_bytes: 8388608;
    max_capture_metadata_bytes: 1048576;
    max_capture_references: 4096;
    max_capture_reference_bytes: 2097152;
    max_json_bytes: 32768;
    max_page_items: 20;
    max_window_bytes: 4096;
    capture_ttl_seconds: 1800;
    preview_ttl_seconds: 600;
    upload_timeout_seconds: 30;
    fetch_timeout_seconds: 15;
    parse_timeout_seconds: 10;
    lease_seconds: 30;
    renew_seconds: 5;
    max_origins: 4;
    max_redirects: 3;
    max_header_bytes: 16384;
    max_open_captures: 2;
    max_reserved_raw_bytes: 4194304;
    max_active_per_owner_domain: 1;
    max_active_deployment: 2;
  };
  network_confirmation_required: true;
  network_retries_per_capture: 0;
  automatic_ingestion: false;
  automatic_prune: false;
  model_calls: 0;
};

export type WebRecoverySlot = {
  slot_version: 'web-recovery/0.6';
  owner_id: string;
  domain_id: string;
  registration_id: string;
  capture_create_key: string;
  capture_create_request: WebCreateCapture;
  capture_id: string | null;
  capture_sha256: string | null;
  producer_versions: WebProducerVersions;
  observation_id: string | null;
  preview_create_key: string | null;
  preview_create_request: WebCreatePreview | null;
  preview_id: string | null;
  preview_request_sha256: string | null;
  confirmation_request: WebConfirm | null;
  receipt_id: string | null;
};

export type WebEntryPage = {
  items: WebEntrySummary[];
  next_cursor: string | null;
};

export type WebReferencePage = {
  items: WebReference[];
  next_cursor: string | null;
};

export type WebPreviewItemPage = {
  items: WebPreviewItem[];
  next_cursor: string | null;
};

export type WebReceiptItemPage = {
  items: WebReceiptItem[];
  next_cursor: string | null;
};

export type WebRevisionPage = {
  items: WebRevision[];
  next_cursor: string | null;
};

export type WebCapabilitiesEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebCapabilities;
};

export type WebRegistrationEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebRegistration;
};

export type WebTargetEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebTarget;
};

export type WebCaptureEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebCapture;
};

export type WebObservationEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebObservation;
};

export type WebEntryPageEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebEntryPage;
};

export type WebEntryEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebEntry;
};

export type WebReferencePageEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebReferencePage;
};

export type WebPreviewEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebPreview;
};

export type WebPreviewItemPageEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebPreviewItemPage;
};

export type WebReceiptEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebReceipt;
};

export type WebReceiptItemPageEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebReceiptItemPage;
};

export type WebWebRevisionEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebRevision;
};

export type WebRevisionPageEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebRevisionPage;
};

export type WebWindowEnvelope = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: WebWindow;
};

export type WebError = {
  code:
    | 'knowledge_unauthorized'
    | 'knowledge_not_found'
    | 'knowledge_invalid_request'
    | 'knowledge_contract_version'
    | 'knowledge_source_contract_version'
    | 'registration_conflict'
    | 'idempotency_conflict'
    | 'policy_changed'
    | 'revision_changed'
    | 'manifest_changed'
    | 'web_state_conflict'
    | 'web_capture_busy'
    | 'web_capture_expired'
    | 'web_capture_cancelled'
    | 'web_capture_rejected'
    | 'web_capture_interrupted'
    | 'web_capture_quota_exceeded'
    | 'web_slots_busy'
    | 'web_length_mismatch'
    | 'web_sha256_mismatch'
    | 'web_too_large'
    | 'web_format_unsupported'
    | 'web_encoding_unsupported'
    | 'web_range_unsupported'
    | 'web_window_invalid'
    | 'web_remote_blocked'
    | 'web_redirect_denied'
    | 'web_compression_rejected'
    | 'web_upstream_http_error'
    | 'web_fetch_timeout'
    | 'web_upload_timeout'
    | 'web_parse_timeout'
    | 'web_parse_rejected'
    | 'web_budget_exceeded'
    | 'web_entry_conflict'
    | 'web_observation_not_ready'
    | 'web_preview_expired'
    | 'web_preview_cancelled'
    | 'web_preview_consumed'
    | 'web_capture_integrity'
    | 'web_storage_unavailable'
    | 'web_result_unknown'
    | 'knowledge_internal';
  message: string;
  contract_version: '1.0.0';
  source_contract_version: '0.6.0';
  request_id: string;
  data: null;
  failure: WebFailure | null;
};

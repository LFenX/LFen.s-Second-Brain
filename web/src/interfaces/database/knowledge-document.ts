// Types follow the frozen Document Structure 0.1.0 OpenAPI; runtime validation remains strict.
import type {
  DocumentPolicyChangeRequest,
  DocumentTaskRequest,
  DocumentCancelRequest,
} from '@/interfaces/request/knowledge-document';

export type DocumentID32 = string;

export type DocumentHash64 = string;

export type DocumentKey = string;

export type DocumentVersion = number;

export type DocumentCount = number;

export type DocumentTimestamp = string;

export type DocumentFormat = 'pdf' | 'pptx' | 'docx';

export type DocumentProfileId =
  | 'pdf-plain-pages/1'
  | 'pptx-ooxml-slides/1'
  | 'docx-ooxml-blocks/1';

export type DocumentOperationKind = 'task' | 'policy' | 'cancel';

export type DocumentEmpty = {};

export type DocumentOfficePart = string;

export type DocumentRelativePath = string;

export type DocumentOriginalFilename = string;

export type DocumentClassification = {
  group_id: DocumentID32 | null;
  component_side: 'frontend' | 'backend' | 'shared' | 'cli' | 'non_code';
  language: string;
};

export type DocumentMediaType =
  | 'application/pdf'
  | 'application/vnd.openxmlformats-officedocument.presentationml.presentation'
  | 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';

export type DocumentOriginalRef = {
  domain_id: DocumentID32;
  registration_id: DocumentID32;
  source_id: DocumentID32;
  source_revision_id: DocumentID32;
  sha256: DocumentHash64;
  byte_length: number;
  media_type: DocumentMediaType;
  original_filename: DocumentOriginalFilename;
  representation_kind: 'original_only';
  root_node_id: null;
  text_available: false;
};

export type DocumentSourceSnapshot = {
  lifecycle_version: DocumentVersion;
  relative_path: DocumentRelativePath;
  classification: DocumentClassification;
};

export type DocumentProcessingConfiguration = {
  extraction: 'pdf_plain_text' | 'office_ooxml_source';
  reading_format: 'rich_document_reading_v1';
  normalize_docx_line_spacing: false;
  trim_text: false;
  remote_resources: false;
  ocr: false;
  models: false;
  media_output: 'metadata_only';
};

export type DocumentProducer = unknown &
  unknown &
  unknown & {
    profile_id: DocumentProfileId;
    format: DocumentFormat;
    engine_id: 'pdf_oxide' | 'go_ooxml';
    engine_version: string;
    engine_build_sha256: DocumentHash64;
    adapter_version: string;
    reading_format_version: 'rich_document_reading_v1';
    configuration_sha256: DocumentHash64;
  };

export type DocumentProfile = unknown &
  unknown &
  unknown & {
    profile_id: DocumentProfileId;
    format: DocumentFormat;
    media_type: DocumentMediaType;
    configuration: DocumentProcessingConfiguration;
    producer: DocumentProducer;
    producer_sha256: DocumentHash64;
    runtime_ready: boolean;
  };

export type DocumentLimits = {
  max_originals: 1;
  max_original_bytes: 8388608;
  max_office_entries: 4096;
  max_expanded_bytes: 16777216;
  max_xml_part_bytes: 4194304;
  max_xml_total_bytes: 16777216;
  max_xml_depth: 64;
  max_intermediate_bytes: 16777216;
  max_pack_bytes: 16777216;
  max_pdf_pages: 128;
  max_pptx_slides: 128;
  max_nodes: 8192;
  max_depth: 16;
  max_reading_bytes: 4194304;
  passage_target_bytes: 4096;
  passage_max_bytes: 8192;
  passage_overlap_bytes: 0;
  content_max_bytes: 65536;
  page_max_items: 20;
  metadata_response_max_bytes: 262144;
  content_response_max_bytes: 524288;
  request_max_bytes: 32768;
  cursor_ttl_seconds: 600;
  lease_seconds: 30;
  lease_renew_seconds: 10;
  attempt_timeout_seconds: 30;
  max_attempts: 3;
  task_deadline_seconds: 120;
  client_wait_seconds: 25;
  poll_interval_seconds: 2;
  recovery_slot_max_bytes: 16384;
  recovery_slots: 8;
  stderr_max_bytes: 4096;
  rss_sampling_interval_ms: 50;
  max_rss_growth_bytes: 268435456;
};

export type DocumentCapabilities = {
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  runtime_ready: boolean;
  owner_only: true;
  mode: 'document_structure_only';
  document_policy_default_enabled: false;
  code_structure_permission_required: false;
  automatic_processing: false;
  model_calls: 0;
  local_inference: 0;
  external_fetches: 0;
  lexical_index: 'not_created';
  vector_index: 'not_created';
  partial_publication: false;
  execution_quota: {
    domain_running: 1;
    domain_queued: 1;
    shared_with_code: true;
  };
  execution: {
    mode: 'owned_subprocess';
    cancel_action: 'terminate_owned_process_group_and_wait';
    cancellation_complete: 'after_exit_and_cleanup';
    hard_memory_isolation: false;
  };
  profiles: unknown & unknown & unknown & DocumentProfile[];
  limits: DocumentLimits;
};

export type DocumentPolicy = {
  domain_id: DocumentID32;
  foundation_policy_version: DocumentVersion;
  document_policy_version: DocumentVersion;
  enabled: boolean;
  mode: 'document_structure_only';
  code_structure_permission_required: false;
  allowed_model_uses: string[];
  external_models: false;
  local_inference: false;
  automatic_processing: false;
  lexical_index: 'not_created';
  vector_index: 'not_created';
};

export type DocumentPolicySnapshot = {
  document_policy_version: DocumentVersion;
  enabled: boolean;
};

export type DocumentDomainTarget = {
  kind: 'domain';
  domain_id: DocumentID32;
};

export type DocumentOriginalDocumentTarget = {
  kind: 'original_document';
  source_id: DocumentID32;
  source_revision_id: DocumentID32;
  original_sha256: DocumentHash64;
  expected_lifecycle_version: DocumentVersion;
};

export type DocumentTaskTarget = {
  kind: 'task';
  task_id: DocumentID32;
};

export type DocumentPolicyReceipt = {
  record_kind: 'policy_receipt';
  command_id: DocumentID32;
  owner_id: DocumentID32;
  domain_id: DocumentID32;
  target: DocumentDomainTarget;
  idempotency_key: DocumentKey;
  original_request: DocumentPolicyChangeRequest;
  request_sha256: DocumentHash64;
  before: DocumentPolicySnapshot;
  after: DocumentPolicySnapshot;
  outcome: 'applied' | 'unchanged';
  completed_at: DocumentTimestamp;
  model_calls: 0;
};

export type DocumentTaskState =
  | 'queued'
  | 'running'
  | 'cancel_requested'
  | 'succeeded'
  | 'failed'
  | 'cancelled';

export type DocumentTaskPhase =
  | 'queued'
  | 'reading_original'
  | 'extracting'
  | 'projecting'
  | 'staged'
  | 'publishing'
  | 'stopping'
  | 'cleaning'
  | 'done';

export type DocumentTaskFailure =
  | 'original_unavailable'
  | 'original_integrity'
  | 'unsupported_document'
  | 'encrypted_document'
  | 'source_structure_unavailable'
  | 'invalid_source_structure'
  | 'budget_exceeded'
  | 'native_timeout'
  | 'native_process_failed'
  | 'source_changed'
  | 'lifecycle_changed'
  | 'policy_changed'
  | 'head_changed'
  | 'producer_changed'
  | 'document_integrity'
  | 'cleanup_failed';

export type DocumentTaskProgress = unknown & {
  unit_kind: 'page' | 'slide' | 'source_block';
  completed_units: number;
  total_units: number | null;
};

export type DocumentOfficeSegment = {
  namespace: string;
  name: string;
  index: DocumentCount;
};

export type DocumentNaturalLocator =
  | {
      scheme: 'document_v1';
    }
  | {
      scheme: 'pdf_page_v1';
      page_number: number;
    }
  | {
      scheme: 'office_package_v1';
      part: DocumentOfficePart;
      segments: DocumentOfficeSegment[];
    };

export type DocumentDerivedPassageLocator = {
  scheme: 'derived_passage_v1';
  parent_locator: DocumentNaturalLocator;
  chunk_index: number;
};

export type DocumentLocator =
  | DocumentNaturalLocator
  | DocumentDerivedPassageLocator;

export type DocumentNodeKind =
  | 'document'
  | 'page'
  | 'slide'
  | 'section'
  | 'heading'
  | 'paragraph'
  | 'text_box'
  | 'text_block'
  | 'table'
  | 'row'
  | 'cell'
  | 'list'
  | 'list_item'
  | 'media'
  | 'passage'
  | 'unsupported_block';

export type DocumentNaturalNodeRef = unknown & {
  kind: DocumentNodeKind;
  locator: DocumentLocator;
};

export type DocumentSemanticParentRef = DocumentNaturalNodeRef & {
  kind?:
    | 'document'
    | 'page'
    | 'slide'
    | 'section'
    | 'heading'
    | 'paragraph'
    | 'text_box'
    | 'text_block'
    | 'table'
    | 'row'
    | 'cell'
    | 'list'
    | 'list_item'
    | 'media'
    | 'unsupported_block';
};

export type DocumentOriginalPosition =
  | {
      kind: 'pdf_page';
      page_number: number;
    }
  | {
      kind: 'pptx_slide';
      slide_number: number;
    }
  | {
      kind: 'document_only';
    };

export type DocumentTextStatus =
  | 'available'
  | 'empty'
  | 'unsupported_or_requires_ocr'
  | 'unsupported';

export type DocumentTextReason =
  | 'no_extractable_text'
  | 'media_not_ocr'
  | 'unsupported_source_block'
  | null;

export type DocumentReadingRange = {
  start_byte: number;
  end_byte: number;
  text_sha256: DocumentHash64;
};

export type DocumentTaskDiagnostic = {
  code:
    | 'unsupported_source_block'
    | 'no_extractable_text'
    | 'media_not_ocr'
    | 'source_structure_unavailable'
    | 'invalid_source_structure'
    | 'budget_exceeded'
    | 'native_timeout'
    | 'native_process_failed'
    | 'stop_pending'
    | 'cleanup_pending'
    | 'cleanup_failed'
    | 'recovery_wait'
    | 'original_integrity'
    | 'document_integrity';
  message: string;
  native_locator: DocumentNaturalLocator | null;
};

export type DocumentTask = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    record_kind: 'task';
    task_id: DocumentID32;
    owner_id: DocumentID32;
    domain_id: DocumentID32;
    original_request: DocumentTaskRequest;
    original: DocumentOriginalRef & {
      byte_length?: number;
    };
    source_snapshot: DocumentSourceSnapshot;
    request_sha256: DocumentHash64;
    producer_sha256: DocumentHash64;
    revision: DocumentVersion;
    state: DocumentTaskState;
    phase: DocumentTaskPhase;
    attempt_count: number;
    created_at: DocumentTimestamp;
    updated_at: DocumentTimestamp;
    deadline_at: DocumentTimestamp;
    completed_at: DocumentTimestamp | null;
    progress: DocumentTaskProgress | null;
    diagnostics: DocumentTaskDiagnostic[];
    result_representation_id: DocumentHash64 | null;
    failure_code: DocumentTaskFailure | null;
    model_calls: 0;
    local_inference: 0;
    external_fetches: 0;
  };

export type DocumentCancelReceipt = unknown &
  unknown & {
    record_kind: 'cancel_receipt';
    command_id: DocumentID32;
    owner_id: DocumentID32;
    domain_id: DocumentID32;
    target: DocumentTaskTarget;
    idempotency_key: DocumentKey;
    original_request: DocumentCancelRequest;
    request_sha256: DocumentHash64;
    before_state: DocumentTaskState;
    after_state: DocumentTaskState;
    task_revision: DocumentVersion;
    outcome: 'requested' | 'unchanged';
    completed_at: DocumentTimestamp;
    model_calls: 0;
  };

export type DocumentRejectionReason =
  | 'foundation_policy_changed'
  | 'document_policy_disabled'
  | 'document_policy_changed'
  | 'source_revision_changed'
  | 'lifecycle_changed'
  | 'source_archived'
  | 'head_changed'
  | 'producer_changed'
  | 'capacity_exceeded'
  | 'unsupported_source'
  | 'task_revision_changed';

export type DocumentTaskRejectionReceipt = {
  record_kind: 'rejection';
  rejection_id: DocumentID32;
  owner_id: DocumentID32;
  domain_id: DocumentID32;
  idempotency_key: DocumentKey;
  request_sha256: DocumentHash64;
  reason:
    | 'foundation_policy_changed'
    | 'document_policy_disabled'
    | 'document_policy_changed'
    | 'source_revision_changed'
    | 'lifecycle_changed'
    | 'source_archived'
    | 'head_changed'
    | 'producer_changed'
    | 'capacity_exceeded'
    | 'unsupported_source';
  recorded_at: DocumentTimestamp;
  accepted: false;
  model_calls: 0;
  operation: 'task';
  target: DocumentOriginalDocumentTarget;
  original_request: DocumentTaskRequest;
};

export type DocumentPolicyRejectionReceipt = {
  record_kind: 'rejection';
  rejection_id: DocumentID32;
  owner_id: DocumentID32;
  domain_id: DocumentID32;
  idempotency_key: DocumentKey;
  request_sha256: DocumentHash64;
  reason: 'foundation_policy_changed' | 'document_policy_changed';
  recorded_at: DocumentTimestamp;
  accepted: false;
  model_calls: 0;
  operation: 'policy';
  target: DocumentDomainTarget;
  original_request: DocumentPolicyChangeRequest;
};

export type DocumentCancelRejectionReceipt = {
  record_kind: 'rejection';
  rejection_id: DocumentID32;
  owner_id: DocumentID32;
  domain_id: DocumentID32;
  idempotency_key: DocumentKey;
  request_sha256: DocumentHash64;
  reason: 'task_revision_changed';
  recorded_at: DocumentTimestamp;
  accepted: false;
  model_calls: 0;
  operation: 'cancel';
  target: DocumentTaskTarget;
  original_request: DocumentCancelRequest;
};

export type DocumentRejectionReceipt =
  | DocumentTaskRejectionReceipt
  | DocumentPolicyRejectionReceipt
  | DocumentCancelRejectionReceipt;

export type DocumentOperationRecord =
  | DocumentTask
  | DocumentPolicyReceipt
  | DocumentCancelReceipt
  | DocumentRejectionReceipt;

export type DocumentHead = unknown &
  unknown &
  unknown &
  unknown & {
    domain_id: DocumentID32;
    source_id: DocumentID32;
    active_representation_id: DocumentHash64 | null;
    active_source_revision_id: DocumentID32 | null;
    current_source_revision_id: DocumentID32;
    current_lifecycle_version: DocumentVersion;
    foundation_policy_version: DocumentVersion;
    document_policy_version: DocumentVersion;
    latest_task_id: DocumentID32 | null;
    currentness: 'none' | 'current' | 'stale';
    reasons: (
      | 'source_changed'
      | 'lifecycle_changed'
      | 'classification_changed'
      | 'position_changed'
      | 'source_archived'
      | 'policy_changed'
      | 'producer_changed'
    )[];
    checked_at: DocumentTimestamp;
  };

export type DocumentPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'document';
    attributes: DocumentEmpty;
    locator: {
      scheme: 'document_v1';
    };
    parent_ref: null;
    ordinal: 0;
    name: string;
    name_truncated: boolean;
    original_position: {
      kind: 'document_only';
    };
    coordinate_precision: 'document';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentPagePayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'page';
    attributes: {
      page_number: number;
      page_content_classification: 'unknown';
    };
    locator: {
      scheme: 'pdf_page_v1';
      page_number: number;
    };
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: {
      kind: 'pdf_page';
      page_number: number;
    };
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentSlidePayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'slide';
    attributes: {
      slide_number: number;
      hidden: boolean;
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: {
      kind: 'pptx_slide';
      slide_number: number;
    };
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentSectionPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'section';
    attributes: DocumentEmpty;
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentHeadingPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'heading';
    attributes: {
      level: number | null;
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: DocumentReadingRange;
  };

export type DocumentParagraphPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'paragraph';
    attributes: DocumentEmpty;
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: DocumentReadingRange;
  };

export type DocumentTextBoxPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'text_box';
    attributes: DocumentEmpty;
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentTextBlockPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'text_block';
    attributes: DocumentEmpty;
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: DocumentReadingRange;
  };

export type DocumentTablePayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'table';
    attributes: {
      row_count: number;
      max_cells_per_row: number;
      layout_precision: 'source_array';
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentRowPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'row';
    attributes: {
      row_index: number;
      cell_count: number;
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentCellPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'cell';
    attributes: {
      row_index: number;
      cell_index: number;
      row_span: null;
      column_span: null;
      is_header: null;
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentListPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'list';
    attributes: {
      ordered: boolean | null;
      start_number: number | null;
      source_level: number | null;
      numbering_id: string | null;
      number_format: string | null;
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentListItemPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'list_item';
    attributes: {
      item_index: number;
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentMediaPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'media';
    attributes: unknown &
      unknown & {
        placement: 'block' | 'inline';
        inline_anchor_byte: number | null;
        media_type: string | null;
        byte_length: number | null;
        origin: 'embedded' | 'external_reference' | 'unknown';
        ocr_status: 'not_requested';
      };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentPassagePayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'passage';
    attributes: {
      chunk_index: number;
    };
    locator: DocumentDerivedPassageLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision: 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: DocumentReadingRange;
  };

export type DocumentUnsupportedBlockPayload = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    kind: 'unsupported_block';
    attributes: {
      native_type: string;
      reason: 'unsupported_source_block';
    };
    locator: DocumentNaturalLocator;
    parent_ref: DocumentSemanticParentRef;
    ordinal: number;
    name: string;
    name_truncated: boolean;
    original_position: DocumentOriginalPosition;
    coordinate_precision:
      | 'document'
      | 'physical_page'
      | 'slide'
      | 'source_path'
      | 'derived_range';
    text_status: DocumentTextStatus;
    text_reason: DocumentTextReason;
    reading_range: DocumentReadingRange;
    own_reading_range: null;
  };

export type DocumentNodePayload =
  | DocumentPayload
  | DocumentPagePayload
  | DocumentSlidePayload
  | DocumentSectionPayload
  | DocumentHeadingPayload
  | DocumentParagraphPayload
  | DocumentTextBoxPayload
  | DocumentTextBlockPayload
  | DocumentTablePayload
  | DocumentRowPayload
  | DocumentCellPayload
  | DocumentListPayload
  | DocumentListItemPayload
  | DocumentMediaPayload
  | DocumentPassagePayload
  | DocumentUnsupportedBlockPayload;

export type DocumentManifestEntry = {
  natural_ref: DocumentNaturalNodeRef;
  node_payload_sha256: DocumentHash64;
};

export type DocumentStructureManifest = {
  nodes: DocumentManifestEntry[];
};

export type DocumentNode = unknown & {
  representation_id: DocumentHash64;
  document_node_id: DocumentHash64;
  source_id: DocumentID32;
  source_revision_id: DocumentID32;
  original_sha256: DocumentHash64;
  producer_sha256: DocumentHash64;
  parent_id: DocumentHash64 | null;
  node_payload_sha256: DocumentHash64;
  payload: DocumentNodePayload;
};

export type DocumentRepresentation = unknown &
  unknown &
  unknown &
  unknown &
  unknown &
  unknown & {
    representation_id: DocumentHash64;
    owner_id: DocumentID32;
    domain_id: DocumentID32;
    original: DocumentOriginalRef & {
      byte_length?: number;
    };
    source_snapshot: DocumentSourceSnapshot;
    foundation_policy_version: DocumentVersion;
    document_policy_version: DocumentVersion;
    format: DocumentFormat;
    profile_id: DocumentProfileId;
    configuration: DocumentProcessingConfiguration;
    producer: DocumentProducer;
    producer_sha256: DocumentHash64;
    quality: 'complete';
    root_document_node_id: DocumentHash64;
    node_count: number;
    page_count: number | null;
    slide_count: number | null;
    unsupported_block_count: number;
    text_coverage: 'supported_profile_blocks';
    coordinate_space: 'derived_utf8';
    reading_format: 'rich_document_reading_v1';
    reading_text_sha256: DocumentHash64;
    reading_byte_length: number;
    structure_manifest_sha256: DocumentHash64;
    published_at: DocumentTimestamp;
    model_calls: 0;
    local_inference: 0;
    external_fetches: 0;
    lexical_index: 'not_created';
    vector_index: 'not_created';
  };

export type DocumentNodePage = {
  representation_id: DocumentHash64;
  source_id: DocumentID32;
  source_revision_id: DocumentID32;
  parent_id: DocumentHash64;
  items: DocumentNode[];
  next_cursor: DocumentID32 | null;
};

export type DocumentContentWindow = unknown &
  unknown & {
    representation_id: DocumentHash64;
    document_node_id: DocumentHash64;
    source_id: DocumentID32;
    source_revision_id: DocumentID32;
    original_sha256: DocumentHash64;
    reading_text_sha256: DocumentHash64;
    node_text_sha256: DocumentHash64;
    fragment_sha256: DocumentHash64;
    coordinate_space: 'derived_utf8';
    text_status: DocumentTextStatus;
    start_byte: number;
    end_byte: number;
    text: string;
    complete: boolean;
    next_start_byte: number | null;
  };

export type DocumentError = unknown & {
  code:
    | 'invalid_request'
    | 'unauthorized'
    | 'not_found'
    | 'contract_mismatch'
    | 'foundation_policy_changed'
    | 'document_policy_disabled'
    | 'document_policy_changed'
    | 'source_revision_changed'
    | 'lifecycle_changed'
    | 'source_archived'
    | 'head_changed'
    | 'producer_changed'
    | 'capacity_exceeded'
    | 'unsupported_source'
    | 'task_revision_changed'
    | 'idempotency_conflict'
    | 'original_integrity'
    | 'document_integrity'
    | 'unavailable';
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: null;
  outcome_known: boolean;
  rejection_id: DocumentID32 | null;
};

export type DocumentError400 = DocumentError & {
  code?: 'invalid_request';
  outcome_known?: false;
  rejection_id?: null;
};

export type DocumentError401 = DocumentError & {
  code?: 'unauthorized';
  outcome_known?: false;
  rejection_id?: null;
};

export type DocumentError404 = DocumentError & {
  code?: 'not_found';
  outcome_known?: false;
  rejection_id?: null;
};

export type DocumentError409 = DocumentError & {
  code?:
    | 'foundation_policy_changed'
    | 'document_policy_disabled'
    | 'document_policy_changed'
    | 'source_revision_changed'
    | 'lifecycle_changed'
    | 'source_archived'
    | 'head_changed'
    | 'producer_changed'
    | 'unsupported_source'
    | 'task_revision_changed'
    | 'idempotency_conflict'
    | 'original_integrity'
    | 'document_integrity';
};

export type DocumentError412 = DocumentError & {
  code?: 'contract_mismatch';
  outcome_known?: false;
  rejection_id?: null;
};

export type DocumentError429 = DocumentError & {
  code?: 'capacity_exceeded';
  outcome_known?: true;
  rejection_id?: DocumentID32;
};

export type DocumentError503 = DocumentError & {
  code?: 'unavailable';
  outcome_known?: false;
  rejection_id?: null;
};

export type DocumentCapabilitiesResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentCapabilities;
};

export type DocumentPolicyResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentPolicy;
};

export type DocumentPolicyReceiptResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentPolicyReceipt;
};

export type DocumentTaskResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentTask;
};

export type DocumentOperationRecordResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentOperationRecord;
};

export type DocumentCancelReceiptResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentCancelReceipt;
};

export type DocumentHeadResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentHead;
};

export type DocumentRepresentationResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentRepresentation;
};

export type DocumentNodePageResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentNodePage;
};

export type DocumentNodeResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentNode;
};

export type DocumentContentWindowResponse = {
  code: 0;
  message: string;
  contract_version: '1.0.0';
  document_structure_contract_version: '0.1.0';
  request_id: DocumentID32;
  data: DocumentContentWindow;
};

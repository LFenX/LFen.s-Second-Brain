import type {
  DocumentLocator,
  DocumentOriginalPosition,
  DocumentNodePayload,
} from './knowledge-document';
import type { StructureClassification } from './knowledge-structure';

export const KnowledgeContentVersion = '0.2.0-candidate' as const;
export type ContentSourceRef = {
  domain_id: string;
  source_id: string;
  source_revision_id: string;
  source_sha256: string;
};
export type ContentRepresentationRef = {
  kind:
    | 'source_revision'
    | 'structure_view'
    | 'document_representation'
    | 'parsed_representation'
    | 'parser_result';
  id: string | null;
  producer_sha256: string | null;
};
export type ContentTarget = {
  source_ref: ContentSourceRef;
  representation_ref: ContentRepresentationRef;
};
export type ContentNodeRef = {
  kind: 'c1_node' | 'r1_node' | 'r2_node' | 'parsed_node' | 'parser_item';
  id: string | null;
  item_index: number | null;
};
export type ContentAddress = {
  target: ContentTarget;
  node_ref: ContentNodeRef;
};
export type ContentRange = {
  start_byte: number;
  end_byte: number;
  sha256: string | null;
};
export type ContentLocator = {
  kind: 'source_utf8' | 'document' | 'parsed_item' | 'parser_item';
  source_utf8: {
    start_byte: number;
    end_byte: number;
    start_line: number;
    end_line: number;
  } | null;
  document: DocumentLocator | null;
  item_index: number | null;
};
export type ContentText = {
  coordinate_space: 'source_utf8' | 'derived_utf8' | 'item_utf8';
  range: ContentRange | null;
  own_range: ContentRange | null;
  status:
    | 'available'
    | 'empty'
    | 'unsupported'
    | 'unsupported_or_requires_ocr'
    | 'unavailable';
  reason: string | null;
  value: string | null;
};
type DocumentAttributes<K extends DocumentNodePayload['kind']> = Extract<
  DocumentNodePayload,
  { kind: K }
>['attributes'];
export type ContentDocumentAttributes = {
  empty: Record<string, never> | null;
  page: DocumentAttributes<'page'> | null;
  slide: DocumentAttributes<'slide'> | null;
  heading: DocumentAttributes<'heading'> | null;
  table: DocumentAttributes<'table'> | null;
  row: DocumentAttributes<'row'> | null;
  cell: DocumentAttributes<'cell'> | null;
  list: DocumentAttributes<'list'> | null;
  list_item: DocumentAttributes<'list_item'> | null;
  media: DocumentAttributes<'media'> | null;
  passage: DocumentAttributes<'passage'> | null;
  unsupported_block: DocumentAttributes<'unsupported_block'> | null;
};
export type ContentTableData = {
  grid: Array<Array<{
    text: string;
    col_span?: number;
    row_span?: number;
    covered?: boolean;
  }> | null>;
  header_rows: number;
  caption?: string;
  page?: number;
};
export type ContentAttributes = {
  source: {
    logical_id: string;
    logical_key: string;
    structure_version: string;
  } | null;
  structure: {
    logical_node_id: string;
    logical_key: string;
    identity_stability: string;
    parser: string;
    classification: StructureClassification;
  } | null;
  document: ContentDocumentAttributes | null;
  parser: {
    item_type: 'text' | 'table' | 'image' | 'opaque';
    ck_type: string;
    table: ContentTableData | null;
    sheet: string;
    positions: Array<number[] | null> | null;
    alt_text: string | null;
    payload_present: boolean;
    text_assembly: string | null;
  } | null;
};
export type ContentNode = {
  ref: ContentNodeRef;
  parent_ref: ContentNodeRef | null;
  kind: string;
  name: string;
  name_truncated: boolean;
  ordinal: number | null;
  locator: ContentLocator;
  original_position: DocumentOriginalPosition | null;
  coordinate_precision:
    | 'source_bytes'
    | 'source_path'
    | 'physical_page'
    | 'document'
    | 'derived_range'
    | 'item_only';
  text: ContentText;
  attributes: ContentAttributes;
  attachment_ref: ContentSourceRef | null;
};
export type ContentRepresentation = {
  projection_version: typeof KnowledgeContentVersion;
  binding: 'bound' | 'unbound';
  source_ref: ContentSourceRef | null;
  representation_ref: ContentRepresentationRef;
  root_ref: ContentNodeRef | null;
  state:
    | 'ready'
    | 'empty'
    | 'partial'
    | 'unavailable'
    | 'normalization_required';
  reason: string | null;
  coordinate_space: '' | 'source_utf8' | 'derived_utf8' | 'item_utf8';
  node_count: number | null;
};
export type ContentNodeResult = {
  projection_version: typeof KnowledgeContentVersion;
  target: ContentTarget;
  node: ContentNode;
};
export type ContentNodePage = {
  projection_version: typeof KnowledgeContentVersion;
  target: ContentTarget;
  parent_ref: ContentNodeRef | null;
  items: ContentNode[];
  next_cursor: string | null;
};
export type ContentWindow = {
  projection_version: typeof KnowledgeContentVersion;
  address: ContentAddress;
  locator: ContentLocator;
  coordinate_space: ContentText['coordinate_space'];
  range: ContentRange;
  text_status: ContentText['status'];
  text_reason: string | null;
  text: string;
  complete: boolean;
  next_start_byte: number | null;
};
export type ContentCapabilities = {
  projection_version: typeof KnowledgeContentVersion;
  owner_only: true;
  parser_registrations: Array<{
    file_type: string;
    base_result: 'structured' | 'image_payload' | 'configuration_only';
  }>;
  fixed_representation_kinds: Array<
    | 'source_revision'
    | 'structure_view'
    | 'document_representation'
    | 'parsed_representation'
  >;
  coordinates: ContentText['coordinate_space'][];
  parser_registration_is_extraction_proof: false;
  unbound_parser_items_publishable: false;
  max_content_bytes: 65536;
  max_page_items: 100;
  max_parsed_representations_page_items: 20;
  parsed_profiles: ContentParsedProfile[];
  parsed_publication_runtime_ready: boolean;
  parsed_policy_default_enabled: false;
  max_parsed_original_bytes: 2097152;
};
export type ContentOperation =
  | 'capabilities'
  | 'representation'
  | 'parsed-representations'
  | 'content-policy'
  | 'content-policy-change'
  | 'parsed-source-upload'
  | 'parsed-source-snapshot'
  | 'parsed-source'
  | 'parsed-task-create'
  | 'parsed-task-get'
  | 'parsed-task-cancel'
  | 'node'
  | 'nodes'
  | 'read';
export type ContentResults = {
  capabilities: ContentCapabilities;
  representation: ContentRepresentation;
  'parsed-representations': ContentParsedRepresentationPage;
  'content-policy': ContentPolicy;
  'content-policy-change': ContentPolicy;
  'parsed-source-upload': ContentParsedUploadReceipt;
  'parsed-source-snapshot': ContentParsedSnapshot;
  'parsed-source': ContentParsedSource;
  'parsed-task-create': ContentParsedTask;
  'parsed-task-get': ContentParsedTask;
  'parsed-task-cancel': ContentParsedTask;
  node: ContentNodeResult;
  nodes: ContentNodePage;
  read: ContentWindow;
};
export type ContentEnvelope<O extends ContentOperation> = {
  operation: O;
  contract_version: typeof KnowledgeContentVersion;
  request_id: string;
  data: ContentResults[O];
};
export type ContentError = {
  operation: ContentOperation;
  contract_version: typeof KnowledgeContentVersion;
  request_id: string;
  data: null;
  code:
    | 'invalid_request'
    | 'unauthenticated'
    | 'not_found'
    | 'content_integrity'
    | 'contract_mismatch'
    | 'capacity_exceeded'
    | 'unsupported'
    | 'unavailable'
    | 'foundation_policy_changed'
    | 'content_policy_changed'
    | 'content_policy_disabled'
    | 'source_changed'
    | 'source_archived'
    | 'lifecycle_changed'
    | 'producer_changed'
    | 'idempotency_conflict'
    | 'task_changed'
    | 'policy_changed'
    | 'unsupported_source'
    | 'original_integrity'
    | 'original_unavailable';
  message: string;
  outcome_known?: boolean;
  rejection_id?: string | null;
};

export type ContentParsedRepresentationPage = {
  projection_version: typeof KnowledgeContentVersion;
  source_ref: ContentSourceRef;
  items: ContentTarget[];
  next_cursor: string | null;
};

export type ContentPolicy = {
  domain_id: string;
  foundation_policy_version: number;
  content_policy_version: number;
  parse_enabled: boolean;
  lexical_enabled: boolean;
};
export type ContentParsedProfile = {
  profile_id: 'knowledge-parsed-local/1';
  format: string;
  producer_sha256: string;
  runtime_ready: boolean;
};
export type ContentParsedUploadReceipt = {
  source_id: string;
  original_filename: string;
  byte_length: number;
};
export type ContentParsedSnapshot = {
  source_ref: ContentSourceRef;
  representation_kind: 'original_only';
  source_kind: 'parsed_file';
};
export type ContentParsedSource = {
  source_ref: ContentSourceRef;
  native_file_id: string;
  representation_kind: 'text' | 'original_only';
  original_filename: string;
  relative_path: string;
  classification: StructureClassification;
  byte_length: number;
  lifecycle_version: number;
  state: 'active' | 'archived';
  foundation_policy_version: number;
  profile: ContentParsedProfile | null;
};
export type ContentParsedTask = {
  task_id: string;
  owner_id: string;
  domain_id: string;
  original_request: import('../request/knowledge-consumption').ContentParsedTaskRequest;
  request_sha256: string;
  producer_sha256: string;
  revision: number;
  attempt_count: number;
  state:
    | 'queued'
    | 'running'
    | 'cancel_requested'
    | 'cancelled'
    | 'failed'
    | 'succeeded';
  phase: 'queued' | 'reading_original' | 'extracting' | 'cleaning' | 'done';
  result: ContentTarget | null;
  failure_code: string | null;
  created_at: string;
  updated_at: string;
  deadline_at: string;
  completed_at: string | null;
};

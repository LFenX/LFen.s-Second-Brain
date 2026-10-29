import type {
  ContentSourceRef,
  ContentTarget,
  ContentAddress,
  ContentLocator,
  ContentNode,
} from './knowledge-consumption';

export type LocalLexicalSourceRef = ContentSourceRef;
export type LocalLexicalTarget = ContentTarget;

export type LocalLexicalPolicy = {
  domain_id: string;
  local_policy_version: number;
  foundation_policy_version: number;
  enabled: boolean;
  mode: 'owner_only_lexical';
  read_authority: 'existing_foundation_owner';
  accepted_inputs: Array<
    'c1_text_root_at_most_8192_bytes' | 'r1_published_passage'
  >;
  allowed_model_uses: [];
  external_models: false;
  local_inference: false;
  remote_fetches: false;
  automatic_ingestion: false;
  vector_index: 'not_created';
};

export type LocalLexicalPolicyReceipt = {
  idempotency_key: string;
  request_sha256: string;
  outcome: 'changed' | 'no_change';
  policy: LocalLexicalPolicy;
};

export type LocalLexicalSelection = {
  kind: 'fixed_representations';
  targets: Array<ContentTarget>;
};

export type LocalLexicalIndexRef = {
  index_id: string;
  manifest_sha256: string;
};

export type LocalLexicalRange = {
  start_byte: number;
  end_byte: number;
  sha256: string;
};

export type LocalLexicalEvidenceRef = {
  evidence_version: '0.2.0-candidate';
  index_ref: LocalLexicalIndexRef;
  index_row_id: string;
  address: ContentAddress;
  coordinate_space: 'source_utf8' | 'derived_utf8';
  reading_text_sha256: string;
  range: LocalLexicalRange;
  locator: ContentLocator;
  original_position: NonNullable<ContentNode['original_position']> | null;
  coordinate_precision: 'source_bytes' | 'derived_range';
  assembly: 'c1_root_subrange_exact_bytes' | 'existing_passage_full_range';
};

export type LocalLexicalEffectivePolicy = {
  domain_id: string;
  local_policy_version: number;
  content_policy_version: number;
  foundation_policy_version: number;
  local_lexical_enabled: boolean;
  content_lexical_enabled: boolean;
  enabled: boolean;
  mode: 'owner_only_local_lexical';
};

export type LocalLexicalProfile = {
  profile_sha256: string;
  tokenizer_producer_sha256: string;
  language: string;
  strategies: [
    'c1_exact_utf8_ranges',
    'r1_existing_passage',
    'r2_existing_passage',
    'parsed_existing_passage',
  ];
  engine: 'elasticsearch';
  execution_profile: 'local_lexical_no_models/0.2.0';
  max_unit_bytes: 8192;
  parent_expansion: 'disabled';
  models_enabled: false;
  remote_fetches: false;
};

export type LocalLexicalCoverage = {
  target: ContentTarget;
  scope: 'selected_representation_reading_utf8';
  coordinate_space: 'source_utf8' | 'derived_utf8';
  reading_text_sha256: string;
  expected_reading_bytes: number;
  covered_reading_bytes: number;
  indexed_unit_count: number;
  reading_text_complete: true;
  representation_state: 'ready' | 'partial' | 'empty';
  unsearchable_node_count: number;
  original_coverage: 'source_utf8_exact' | 'not_asserted';
  text_index_state: 'empty' | 'available';
};

export type LocalLexicalReadiness = {
  kind: 'unindexed' | 'empty' | 'building' | 'failed' | 'ready' | 'stale';
  publication: 'none' | 'published';
  selection: LocalLexicalSelection;
  selection_sha256: string;
  snapshot_currentness: 'current' | 'historical' | 'mixed';
  index_ref: LocalLexicalIndexRef | null;
  coverage: Array<LocalLexicalCoverage>;
  diagnostic_codes: Array<
    | 'none'
    | 'building'
    | 'policy_disabled'
    | 'policy_changed'
    | 'profile_changed'
    | 'source_changed'
    | 'empty_content'
    | 'unit_oversize'
    | 'unsupported_representation'
    | 'source_unavailable'
    | 'integrity_error'
    | 'coverage_gap'
    | 'index_failed'
  >;
};

export type LocalLexicalReadinessData = {
  effective_policy: LocalLexicalEffectivePolicy;
  profile: LocalLexicalProfile;
  readiness: LocalLexicalReadiness;
};

export type LocalLexicalJob = {
  job_id: string;
  idempotency_key: string;
  request_sha256: string;
  state: 'queued' | 'building' | 'published' | 'failed';
  selection: LocalLexicalSelection;
  captured_local_policy_version: number;
  captured_content_policy_version: number;
  captured_foundation_policy_version: number;
  captured_profile_sha256: string;
  captured_tokenizer_producer_sha256: string;
  index_ref: LocalLexicalIndexRef | null;
  readiness: LocalLexicalReadiness;
  created_at: string;
  updated_at: string;
};

export type LocalLexicalBinding = {
  index_ref: LocalLexicalIndexRef;
  selection: LocalLexicalSelection;
  selection_sha256: string;
  local_policy_version: number;
  content_policy_version: number;
  foundation_policy_version: number;
  profile_sha256: string;
  tokenizer_producer_sha256: string;
  engine: 'elasticsearch';
  execution_profile: 'local_lexical_no_models/0.2.0';
  parent_expansion: 'disabled';
  published_at: string;
};

export type LocalLexicalHit = {
  evidence_ref: LocalLexicalEvidenceRef;
  text: string;
  score: number;
  match_type: 'lexical';
  source_currentness: 'current' | 'historical';
};

export type LocalLexicalSearchData = {
  binding: LocalLexicalBinding;
  coverage: Array<LocalLexicalCoverage>;
  query: string;
  requested_limit: number;
  returned_count: number;
  hits: Array<LocalLexicalHit>;
  total_matches: null;
  pagination: 'none';
  ranking: 'native_lexical_only';
  parent_expansion: 'disabled';
};

export type LocalLexicalTypedMember = {
  node: ContentNode;
  payload_sha256: string;
  text_assembly:
    | 'existing_reading_range'
    | 'row_major_uncovered_cells_tab_lf_v1';
};

export type LocalLexicalContext = {
  node: ContentNode | null;
  node_omission_reason: 'metadata_budget' | null;
  typed_members: Array<LocalLexicalTypedMember>;
  typed_members_scope: 'whole_nodes_contained_in_evidence_range';
  typed_members_complete: boolean;
  omitted_member_count: number;
  omission_reasons: Array<
    | 'typed_members_not_defined_for_source_family'
    | 'representation_contains_unsearchable_nodes'
    | 'whole_member_outside_evidence'
    | 'unsearchable_member'
    | 'empty_member_not_anchored'
    | 'member_limit'
    | 'metadata_budget'
  >;
  manifest_sha256: string | null;
  membership:
    | 'stored_c1_root'
    | 'validated_r1_passage'
    | 'validated_document_manifest'
    | 'validated_parsed_manifest';
  representation_state: 'ready' | 'partial' | 'empty';
  parent_expansion: 'disabled';
  metadata_byte_length: number;
};

export type LocalLexicalReadData = {
  evidence_ref: LocalLexicalEvidenceRef;
  text: string;
  encoding: 'utf-8';
  byte_length: number;
  source_currentness: 'current' | 'historical';
  complete: true;
  context: LocalLexicalContext;
};

export type LocalLexicalData = {
  getLocalPolicy: LocalLexicalPolicy;
  changeLocalPolicy: LocalLexicalPolicyReceipt;
  inspectIndex: LocalLexicalReadinessData;
  createIndex: LocalLexicalJob;
  getIndexJob: LocalLexicalJob;
  search: LocalLexicalSearchData;
  read: LocalLexicalReadData;
};
export type LocalLexicalOperation = keyof LocalLexicalData;
export type LocalLexicalEnvelope<O extends LocalLexicalOperation> = {
  operation: O;
  contract_version: O extends 'getLocalPolicy' | 'changeLocalPolicy'
    ? '0.1.0-candidate'
    : '0.2.0-candidate';
  request_id: string;
  data: LocalLexicalData[O];
};
export type LocalLexicalError = {
  operation: LocalLexicalOperation;
  contract_version: '0.1.0-candidate' | '0.2.0-candidate';
  request_id: string;
  data: null;
  code: string;
  message: string;
  outcome_known: boolean;
  retry_same_key: boolean;
};

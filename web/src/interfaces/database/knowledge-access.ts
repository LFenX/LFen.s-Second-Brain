export const KnowledgeAccessVersion = '0.3.0-candidate' as const;

export type AccessCredential = {
  credential_id: AccessID;
  domain_id: AccessID;
  module_id: AccessID;
  module_version: number;
  mode: AccessMode;
  client_label: string;
  issued_at: AccessTime;
  expires_at: AccessTime;
  revoked_at: AccessTime | null;
  last_used_at: AccessTime | null;
};
export type AccessCredentialMintData = {
  credential: AccessCredential;
  token: string | null;
  delivery: 'issued_once' | 'unavailable_after_first_response';
};
export type AccessListCredentialsData = {
  items: AccessCredential[];
  pagination: 'none';
};

export type AccessCitation = {
  target: AccessTarget;
  node_id: AccessOwningNodeID;
  coordinate_space: 'source_utf8' | 'derived_utf8';
  range: AccessRange;
  display_name: string;
  position_label: string;
  revision_binding: 'fixed';
};

export type AccessDelivery = 'web_only' | 'web_and_mcp';

export type AccessError = {
  operation:
    | 'setDepartment'
    | 'listDepartments'
    | 'setPolicy'
    | 'setModule'
    | 'getModuleAdmin'
    | 'listModules'
    | 'createGrant'
    | 'revokeGrant'
    | 'createCredential'
    | 'revokeCredential'
    | 'listCredentials'
    | 'inspect'
    | 'search'
    | 'read'
    | 'mcpTransport';
  contract_version: AccessVersion;
  request_id: AccessID;
  data: null;
  code:
    | 'invalid_request'
    | 'unauthenticated'
    | 'action_denied'
    | 'not_found'
    | 'version_conflict'
    | 'scope_changed'
    | 'view_stale'
    | 'index_stale'
    | 'index_unindexed'
    | 'scope_not_supported'
    | 'idempotency_conflict'
    | 'contract_mismatch'
    | 'capacity_exceeded'
    | 'rate_limited'
    | 'integrity_error'
    | 'unavailable';
  message: 'Knowledge access operation could not be completed';
  outcome_known: boolean;
  retry_same_key: boolean;
};

export type AccessEvidenceReadSelector = {
  kind: 'evidence';
  evidence_ref: AccessEvidenceRef;
};

export type AccessEvidenceRef = {
  contract_version: AccessVersion;
  view_id: AccessID;
  evidence_id: AccessHash;
};

export type AccessGeneration = number;
export type AccessDepartment = {
  department_id: AccessID;
  tenant_id: AccessID;
  name: string;
  version: AccessGeneration;
  enabled: boolean;
  member_count: number | null;
  user_ids: AccessID[] | null;
};
export type AccessListDepartmentsData = {
  items: AccessDepartment[];
  pagination: 'none';
};

export type AccessGrant = {
  grant_id: AccessID;
  domain_id: AccessID;
  module_id: AccessID;
  module_version: number;
  subject: AccessSubject;
  mode: AccessMode;
  delivery: AccessDelivery;
  issued_at: AccessTime;
  expires_at: AccessTime;
  revoked_at: AccessTime | null;
  issued_by_user_id: AccessID;
};

export type AccessHash = string;

export type AccessHit = {
  evidence_ref: AccessEvidenceRef;
  citation: AccessCitation;
  text: string;
  byte_length: number;
  rank_ordinal: number;
};

export type AccessID = string;

export type AccessInspectData = {
  view_id: AccessID;
  domain_id: AccessID;
  module_id: AccessID;
  module_version: number;
  mode: AccessMode;
  delivery: AccessDelivery;
  expires_at: AccessTime;
  targets: AccessTargetCatalogEntry[];
  source_count: number;
  unit_count: number;
  readiness: 'ready';
  pagination: 'none';
};

export type AccessKey = string;

export type AccessListModulesData = {
  items: AccessModuleSummary[];
  pagination: 'none';
  owned_domain_policies: AccessPolicy[];
};

export type AccessMode = 'read' | 'read_search';

export type AccessModuleAdmin = {
  module_id: AccessID;
  domain_id: AccessID;
  name: string;
  version: number;
  enabled: boolean;
  scope: AccessModuleScope;
  grants: AccessGrant[];
  policy: AccessPolicy;
};

export type AccessModuleScope = {
  kind: 'fixed_published_targets';
  publication: AccessOwnerPublication;
  targets: AccessTarget[];
};

export type AccessModuleSummary = {
  module_id: AccessID;
  domain_id: AccessID;
  name: string;
  version: number;
  target_count: number;
  modes: AccessMode[];
  mcp_delivery_allowed: boolean;
  enabled: boolean;
  administrative_owner: boolean;
};

export type AccessOwnerPublication = {
  id: AccessHash;
  manifest_sha256: AccessHash;
};

export type AccessOwningNodeID = string;

export type AccessPolicy = {
  domain_id: AccessID;
  version: AccessGeneration;
  enabled: boolean;
};

export type AccessRange = {
  start_byte: number;
  end_byte: number;
  sha256: AccessHash;
};

export type AccessReadData = {
  evidence_ref: AccessEvidenceRef;
  citation: AccessCitation;
  text: string;
  byte_length: number;
  encoding: 'utf-8';
  complete: true;
  typed_context: AccessTypedContext;
  attachments: 'not_exposed';
  parent_expansion: 'disabled';
  unit_locator: AccessUnitLocator;
};

export type AccessRepresentation =
  | { kind: 'source_revision'; id: string; producer_sha256: null }
  | {
      kind:
        | 'structure_view'
        | 'document_representation'
        | 'parsed_representation';
      id: string;
      producer_sha256: string;
    };

export type AccessRevoked = { id: AccessID; revoked_at: AccessTime };

export type AccessSearchData = {
  view_id: AccessID;
  query: string;
  requested_limit: number;
  returned_count: number;
  hits: AccessHit[];
  total_matches: null;
  pagination: 'none';
  ranking: 'scoped_lexical_stable_id';
  parent_expansion: 'disabled';
};

export type AccessSource = {
  domain_id: AccessID;
  source_id: AccessID;
  source_revision_id: AccessID;
  source_sha256: AccessHash;
};

export type AccessSubject = { kind: 'user' | 'department'; id: AccessID };

export type AccessTarget = {
  source: AccessSource;
  representation: AccessRepresentation;
};

export type AccessTargetCatalogEntry = {
  target: AccessTarget;
  unit_count: number;
};

export type AccessTime = string;

export type AccessTypedContext = {
  members: AccessTypedMember[];
  members_complete: boolean;
  omission_reasons: (
    | 'typed_payload_not_exposed_in_access_v03'
    | 'whole_member_outside_evidence'
    | 'unsearchable_member'
    | 'member_budget'
    | 'metadata_budget'
  )[];
};

export type AccessTypedMember = {
  node_id: AccessOwningNodeID;
  kind: string;
  range: AccessRange;
  text_status: 'available' | 'empty';
  payload_sha256: AccessHash;
};

export type AccessUnitLocator = {
  view_id: AccessID;
  target_index: number;
  unit_ordinal: number;
  target_unit_count: number;
  next_unit_ordinal: number | null;
};

export type AccessUnitReadSelector = {
  kind: 'unit';
  view_id: AccessID;
  target_index: number;
  unit_ordinal: number;
};

export type AccessVersion = '0.3.0-candidate';

export interface AccessActor {
  user: string;
  tenant: string;
  session: number;
  auth: 'native';
}
export interface AccessResults {
  createCredential: AccessCredentialMintData;
  revokeCredential: AccessRevoked;
  listCredentials: AccessListCredentialsData;
  setDepartment: AccessDepartment;
  listDepartments: AccessListDepartmentsData;
  setPolicy: AccessPolicy;
  setModule: AccessModuleAdmin;
  getModuleAdmin: AccessModuleAdmin;
  listModules: AccessListModulesData;
  createGrant: AccessGrant;
  revokeGrant: AccessRevoked;
  inspect: AccessInspectData;
  search: AccessSearchData;
  read: AccessReadData;
}

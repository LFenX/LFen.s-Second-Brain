import type {
  ContentAddress,
  ContentNodeRef,
  ContentTarget,
  ContentSourceRef,
  KnowledgeContentVersion,
} from '../database/knowledge-consumption';
export type ContentScope = { owner: string; domain: string };
export type ContentNodeQuery = {
  target: ContentTarget;
  parent_ref: ContentNodeRef | null;
  cursor: string;
  limit: number;
};
export type ContentReadRequest = {
  address: ContentAddress;
  start_byte: number | null;
  end_byte: number | null;
};
export type ContentPolicyRequest = {
  expected_policy_version: number;
  expected_content_policy_version: number;
  parse_enabled: boolean;
  lexical_enabled: boolean;
  confirmed: true;
  idempotency_key: string;
};
export type ContentParsedUploadRequest = Pick<
  ContentPolicyRequest,
  'expected_policy_version' | 'expected_content_policy_version' | 'confirmed'
>;
export type ContentParsedSnapshotRequest = {
  source_id: string;
  expected_revision_id: string;
  expected_policy_version: number;
  relative_path: string;
  classification: import('../database/knowledge-structure').StructureClassification;
};
export type ContentParsedTaskRequest = {
  source_ref: ContentSourceRef;
  expected_lifecycle_version: number;
  expected_policy_version: number;
  expected_content_policy_version: number;
  expected_producer_sha256: string;
  max_unit_bytes: number;
  confirmed: true;
  idempotency_key: string;
};
export type ContentParsedCancelRequest = {
  expected_task_revision: number;
  idempotency_key: string;
};
type VersionedRequest<T> = {
  contract_version: typeof KnowledgeContentVersion;
  request: T;
};
export type ContentRequests = {
  capabilities: undefined;
  'content-policy': undefined;
  'content-policy-change': VersionedRequest<ContentPolicyRequest>;
  'parsed-source-upload': VersionedRequest<ContentParsedUploadRequest>;
  'parsed-source-snapshot': VersionedRequest<ContentParsedSnapshotRequest>;
  'parsed-source': {
    contract_version: typeof KnowledgeContentVersion;
    source_id: string;
  };
  'parsed-task-create': VersionedRequest<ContentParsedTaskRequest>;
  'parsed-task-get': {
    contract_version: typeof KnowledgeContentVersion;
    task_id: string;
  };
  'parsed-task-cancel': VersionedRequest<ContentParsedCancelRequest> & {
    task_id: string;
  };
  'parsed-representations': {
    contract_version: typeof KnowledgeContentVersion;
    query: ContentParsedRepresentationQuery;
  };
  representation: {
    contract_version: typeof KnowledgeContentVersion;
    target: ContentTarget;
  };
  node: {
    contract_version: typeof KnowledgeContentVersion;
    address: ContentAddress;
  };
  nodes: {
    contract_version: typeof KnowledgeContentVersion;
    query: ContentNodeQuery;
  };
  read: {
    contract_version: typeof KnowledgeContentVersion;
    read: ContentReadRequest;
  };
};

export type ContentParsedRepresentationQuery = {
  source_ref: ContentSourceRef;
  cursor: string;
  limit: number;
};

// Types follow the frozen Document Structure 0.1.0 OpenAPI; runtime validation remains strict.
import type {
  DocumentVersion,
  DocumentKey,
  DocumentOriginalDocumentTarget,
  DocumentHash64,
  DocumentProfileId,
  DocumentOperationKind,
  DocumentID32,
} from '@/interfaces/database/knowledge-document';

export type DocumentPolicyChangeRequest = {
  expected_policy_version: DocumentVersion;
  expected_document_policy_version: DocumentVersion;
  enabled: boolean;
  confirmed: true;
  idempotency_key: DocumentKey;
};

export type DocumentTaskRequest = {
  target: DocumentOriginalDocumentTarget;
  expected_policy_version: DocumentVersion;
  expected_document_policy_version: DocumentVersion;
  expected_active_representation_id: DocumentHash64 | null;
  profile_id: DocumentProfileId;
  expected_producer_sha256: DocumentHash64;
  confirmed: true;
  idempotency_key: DocumentKey;
};

export type DocumentCancelRequest = {
  expected_task_revision: DocumentVersion;
  confirmed: true;
  idempotency_key: DocumentKey;
};

export type DocumentOperationQuery = {
  kind: DocumentOperationKind;
  idempotency_key: DocumentKey;
};

export type DocumentNodeQuery = {
  parent_id: DocumentHash64;
  limit?: number;
  cursor?: DocumentID32;
};

export type DocumentContentQuery = {
  start_byte?: number;
  end_byte?: number;
};

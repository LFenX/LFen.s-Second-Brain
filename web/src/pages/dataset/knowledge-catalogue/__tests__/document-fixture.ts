import { webcrypto, randomUUID } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-document-structure-v1.examples.json';
import type {
  DocumentTask,
  DocumentRepresentation,
  DocumentNode,
  DocumentContentWindow,
  DocumentPolicy,
  DocumentPolicyReceipt,
  DocumentCancelReceipt,
  DocumentHead,
  DocumentCapabilities,
  DocumentRejectionReceipt,
} from '@/interfaces/database/knowledge-document';
import type { DocumentOperation } from '@/utils/knowledge-document';
Object.defineProperty(globalThis.crypto, 'subtle', {
  configurable: true,
  value: webcrypto.subtle,
});
Object.defineProperty(globalThis.crypto, 'randomUUID', {
  configurable: true,
  value: randomUUID,
});
export function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value));
}
export function sample<T>(name: string): T {
  const v = Examples.positive.find((v) => v.name === name);
  if (!v) throw Error('Missing document fixture ' + name);
  return clone(v.value) as T;
}
export const Task = clone(Examples.fixtures.trees.docx.task) as DocumentTask;
export const Rep = sample<{ data: DocumentRepresentation }>(
  'response_representation_docx',
).data;
export const Scope = {
  owner: Examples.fixtures.owner_id,
  domain: Examples.fixtures.domain_id,
};
export const SourceScope = { ...Scope, source: Task.original.source_id };
export const ViewScope = {
  ...Scope,
  source: Rep.original.source_id,
  revision: Rep.original.source_revision_id,
  sha: Rep.original.sha256,
  representation: Rep.representation_id,
};
export const Nodes = clone(
  Examples.fixtures.trees.docx.nodes,
) as DocumentNode[];
export const Root = Nodes[0];
export const Policy = sample<{ data: DocumentPolicy }>('response_policy').data;
export const Caps = sample<{ data: DocumentCapabilities }>(
  'response_caps',
).data;
export const PolicyReceipt = clone(
  Examples.fixtures.policy_receipt,
) as DocumentPolicyReceipt;
export const CancelReceipt = clone(
  Examples.fixtures.cancel_receipt,
) as DocumentCancelReceipt;
export const Head = sample<{ data: DocumentHead }>(
  'response_head_current',
).data;
export const Content = sample<{ data: DocumentContentWindow }>(
  'response_content_docx',
).data;
export const TaskOperation: Extract<DocumentOperation, { kind: 'task' }> = {
  kind: 'task',
  scope: { ...Scope, source: Task.original.source_id },
  body: Task.original_request,
};
export const PolicyOperation: Extract<DocumentOperation, { kind: 'policy' }> = {
  kind: 'policy',
  scope: Scope,
  body: PolicyReceipt.original_request,
};
export const CancelOperation: Extract<DocumentOperation, { kind: 'cancel' }> = {
  kind: 'cancel',
  scope: { ...Scope, task: CancelReceipt.target.task_id },
  body: CancelReceipt.original_request,
};
export const Rejections = clone(Examples.fixtures.rejections) as Record<
  'task' | 'policy' | 'cancel',
  DocumentRejectionReceipt
>;
export function wire<T>(data: T, status = 200) {
  const request_id = '1'.repeat(32);
  return {
    status,
    headers: {
      'x-knowledge-contract-version': '1.0.0',
      'x-knowledge-document-structure-contract-version': '0.1.0',
      'x-request-id': request_id,
      'cache-control': 'no-store',
    },
    data: {
      code: 0,
      message: '',
      contract_version: '1.0.0',
      document_structure_contract_version: '0.1.0',
      request_id,
      data,
    },
  };
}

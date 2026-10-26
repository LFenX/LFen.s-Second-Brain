import { webcrypto, randomUUID } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-structure-v1.examples.json';
import type {
  StructureTask,
  StructureView,
  StructureNode,
  StructureContent,
  StructurePolicy,
  StructurePolicyReceipt,
  StructureCancelReceipt,
  StructureRejectionReceipt,
  StructureHead,
  StructureRelation,
  StructureCapabilities,
} from '@/interfaces/database/knowledge-structure';
import type { StructureOperation } from '@/utils/knowledge-structure';
import { structureRequestDigest } from '@/utils/knowledge-structure';
Object.defineProperty(globalThis.crypto, 'subtle', {
  configurable: true,
  value: webcrypto.subtle,
});
Object.defineProperty(globalThis.crypto, 'randomUUID', {
  configurable: true,
  value: randomUUID,
});
export function sample<T>(name: string): T {
  const item = Examples.positive.find((value) => value.name === name);
  if (!item) throw Error('Missing structure fixture ' + name);
  return JSON.parse(JSON.stringify(item.value)) as T;
}
export const Task = sample<StructureTask>('task'),
  View = sample<StructureView>('view'),
  Node = sample<StructureNode>('node'),
  Content = sample<StructureContent>('content'),
  Policy = sample<StructurePolicy>('policy'),
  PolicyReceipt = sample<StructurePolicyReceipt>('policyreceipt'),
  CancelReceipt = sample<StructureCancelReceipt>('cancelreceipt'),
  Head = sample<StructureHead>('head'),
  Relation = sample<StructureRelation>('relation'),
  Capabilities = sample<StructureCapabilities>('capabilities');
export const Scope = { owner: Task.owner_id, domain: Task.domain_id },
  ProjectScope = { ...Scope, project: Task.project_id },
  ViewScope = { ...Scope, view: View.view_id };
export const TaskOperation = {
    kind: 'task',
    scope: ProjectScope,
    body: Task.original_request,
  } satisfies StructureOperation,
  PolicyOperation = {
    kind: 'policy',
    scope: Scope,
    body: PolicyReceipt.original_request,
  } satisfies StructureOperation,
  CancelOperation = {
    kind: 'cancel',
    scope: { ...Scope, task: Task.task_id },
    body: CancelReceipt.original_request,
  } satisfies StructureOperation;
export function running(patch: Partial<StructureTask> = {}): StructureTask {
  return {
    ...sample<StructureTask>('task'),
    state: 'running',
    result_view_id: null,
    completed_at: null,
    finished_members: 0,
    members: Task.members.map((member) => ({ ...member, state: 'processing' })),
    ...patch,
  };
}
export async function rejection(
  operation: StructureOperation,
  patch: Partial<StructureRejectionReceipt> = {},
): Promise<StructureRejectionReceipt> {
  return {
    rejection_id: 'f'.repeat(32),
    operation: operation.kind,
    owner_id: operation.scope.owner,
    domain_id: operation.scope.domain,
    task_id: operation.kind === 'cancel' ? operation.scope.task : null,
    idempotency_key: operation.body.idempotency_key,
    original_request: operation.body,
    request_sha256: await structureRequestDigest(
      operation.kind,
      operation.scope,
      operation.body,
    ),
    reason: 'policy_changed',
    recorded_at: Task.created_at,
    accepted: false,
    model_calls: 0,
    ...patch,
  };
}
export function wire(value: unknown, status = 200) {
  const requestId = '1'.repeat(32);
  return {
    status,
    data: {
      code: 0,
      message: '',
      contract_version: '1.0.0',
      structure_contract_version: '0.1.0',
      request_id: requestId,
      data: value,
    },
    headers: {
      'x-knowledge-contract-version': '1.0.0',
      'x-knowledge-structure-contract-version': '0.1.0',
      'cache-control': 'no-store',
      'x-request-id': requestId,
    },
  };
}
export const Entry = View.members[2];
export function manifestEntry(member = Entry) {
  return {
    registration_id: '1'.repeat(32),
    relative_path: member.relative_path,
    source_id: member.source_id,
    source_revision_id: member.source_revision_id,
    root_node_id: member.source_node_id,
    sha256: member.sha256,
    byte_length: member.byte_length,
    classification: member.classification,
    origin_task_id: '2'.repeat(32),
  };
}

import type {
  StructureAlias,
  StructureTask,
  StructureView,
  StructureNode,
  StructureRelation,
  StructureContent,
  StructurePolicyReceipt,
  StructureCancelReceipt,
  StructureRejectionReceipt,
} from '@/interfaces/database/knowledge-structure';
import type {
  StructureTaskRequest,
  StructurePolicyChangeRequest,
  StructureCancelRequest,
} from '@/interfaces/request/knowledge-structure';
import dayjs from 'dayjs';
import { encodeWebValue, webSha256 } from './knowledge-web';
import {
  StructureId,
  StructureKey,
  StructureLimits,
  StructureProducers,
  structureJsonBytes,
  validStructureData,
  structureBytes,
} from './knowledge-structure-schema';

export type StructureScope = { owner: string; domain: string };
export type StructureProjectScope = StructureScope & { project: string };
export type StructureViewScope = StructureScope & { view: string };
export type StructureWriteKind = 'task' | 'policy' | 'cancel';
export type StructureWriteBody =
  | StructureTaskRequest
  | StructurePolicyChangeRequest
  | StructureCancelRequest;
export type StructureOperation =
  | { kind: 'task'; scope: StructureProjectScope; body: StructureTaskRequest }
  | {
      kind: 'policy';
      scope: StructureScope;
      body: StructurePolicyChangeRequest;
    }
  | {
      kind: 'cancel';
      scope: StructureScope & { task: string };
      body: StructureCancelRequest;
    };
export type StructureRecoverySlot = {
  version: '0.1.0';
  owner: string;
  domain: string;
  target: string;
  kind: StructureWriteKind;
  key: string;
  body: StructureWriteBody;
  request_sha256: string;
  saved_at: string;
  task_id?: string;
};
export type StructureOutcome =
  | StructureTask
  | StructurePolicyReceipt
  | StructureCancelReceipt
  | StructureRejectionReceipt;
const Encoder = new TextEncoder();
export const structureTerminal = (task: StructureTask) =>
  ['succeeded', 'failed', 'cancelled'].includes(task.state);
export const structureRejected = (
  outcome: StructureOutcome,
): outcome is StructureRejectionReceipt => 'rejection_id' in outcome;
export const canonicalStructureAliases = (aliases: StructureAlias[]) =>
  [...aliases].sort((a, b) =>
    a.prefix < b.prefix ? -1 : a.prefix > b.prefix ? 1 : 0,
  );
export function structureDigest(value: object): Promise<string> {
  return webSha256(Encoder.encode(encodeWebValue(value)));
}
export function structureRequestProjection(
  kind: StructureWriteKind,
  scope: StructureScope & { task?: string },
  body: StructureWriteBody,
): object {
  const request = Object.fromEntries(
    Object.entries(body).filter(([key]) => key !== 'idempotency_key'),
  );
  return {
    kind: `knowledge-structure-${kind === 'task' ? 'task' : kind}/0.1.0`,
    owner_id: scope.owner,
    domain_id: scope.domain,
    ...(kind === 'cancel' ? { task_id: scope.task } : {}),
    ...request,
    ...(kind === 'task'
      ? {
          members: [...(body as StructureTaskRequest).members].sort((a, b) =>
            a.source_id < b.source_id ? -1 : a.source_id > b.source_id ? 1 : 0,
          ),
          aliases: canonicalStructureAliases(
            (body as StructureTaskRequest).aliases,
          ),
        }
      : {}),
  };
}
export const structureRequestDigest = (
  kind: StructureWriteKind,
  scope: StructureScope & { task?: string },
  body: StructureWriteBody,
) => structureDigest(structureRequestProjection(kind, scope, body));
export function structureProducerProjection(aliases: StructureAlias[]): object {
  return {
    kind: 'knowledge-structure-producer/0.1.0',
    producers: StructureProducers,
    limits: StructureLimits,
    aliases: canonicalStructureAliases(aliases),
  };
}
export function structureViewProjection(view: StructureView): object {
  return {
    kind: 'knowledge-structure-view/0.1.0',
    owner_id: view.owner_id,
    domain_id: view.domain_id,
    project_id: view.project_id,
    manifest_id: view.manifest_id,
    manifest_sha256: view.manifest_sha256,
    foundation_policy_version: view.foundation_policy_version,
    structure_policy_version: view.structure_policy_version,
    producer_sha256: view.producer_sha256,
    members: [...view.members]
      .sort((a, b) =>
        a.source_id < b.source_id ? -1 : a.source_id > b.source_id ? 1 : 0,
      )
      .map(
        ({
          source_id,
          source_revision_id,
          lifecycle_version,
          sha256,
          classification,
          relative_path,
        }) => ({
          source_id,
          source_revision_id,
          lifecycle_version,
          sha256,
          classification,
          relative_path,
        }),
      ),
  };
}
export function structureLogicalProjection(
  domain: string,
  node: StructureNode,
): object {
  return {
    kind: 'knowledge-structure-logical/0.1.0',
    domain_id: domain,
    source_id: node.source_id,
    node_kind: node.kind,
    logical_key: node.logical_key,
  };
}
export function structureNodeProjection(node: StructureNode): object {
  return {
    kind: 'knowledge-structure-node/0.1.0',
    view_id: node.view_id,
    logical_node_id: node.logical_node_id,
    source_revision_id: node.source_revision_id,
    start_byte: node.start_byte,
    end_byte: node.end_byte,
  };
}
export function structureRelationProjection(
  relation: StructureRelation,
): object {
  const { view_id, kind, from_node_id, to_node_id, reference, evidence } =
    relation;
  return {
    kind: 'knowledge-structure-relation/0.1.0',
    view_id,
    relation_kind: kind,
    from_node_id,
    to_node_id,
    reference,
    evidence,
  };
}
export async function checkStructureOutcome(
  scope: StructureScope & { task?: string; project?: string },
  outcome: StructureOutcome,
  operation?: StructureOperation,
): Promise<void> {
  const rejected = structureRejected(outcome);
  const kind = rejected
    ? outcome.operation
    : 'task_id' in outcome && 'state' in outcome
      ? 'task'
      : (outcome as StructurePolicyReceipt | StructureCancelReceipt).kind;
  const schema = rejected
    ? 'RejectionReceipt'
    : kind === 'task'
      ? 'Task'
      : kind === 'policy'
        ? 'PolicyReceipt'
        : 'CancelReceipt';
  if (
    !validStructureData(schema, outcome) ||
    outcome.owner_id !== scope.owner ||
    outcome.domain_id !== scope.domain
  )
    throw Error('Structure outcome scope mismatch');
  const taskId =
    kind === 'cancel' ? (outcome as StructureCancelReceipt).task_id : undefined;
  if (kind === 'cancel' && (!taskId || (scope.task && scope.task !== taskId)))
    throw Error('Cancel task identity mismatch');
  if (
    kind === 'task' &&
    scope.project &&
    (outcome.original_request as StructureTaskRequest).project_id !==
      scope.project
  )
    throw Error('Task project mismatch');
  if (operation && kind !== operation.kind)
    throw Error('Structure operation mismatch');
  if (!rejected && kind === 'policy') {
    const receipt = outcome as StructurePolicyReceipt;
    if (
      receipt.before.structure_policy_version !==
        receipt.original_request.expected_structure_policy_version ||
      receipt.after.enabled !== receipt.original_request.enabled
    )
      throw Error('Policy receipt expectation mismatch');
  }
  if (!rejected && kind === 'cancel') {
    const receipt = outcome as StructureCancelReceipt;
    const changed = receipt.outcome === 'requested';
    if (
      receipt.task_revision !==
        receipt.original_request.expected_task_revision + (changed ? 1 : 0) ||
      (changed
        ? !['queued', 'running'].includes(receipt.before_state) ||
          receipt.after_state !== 'cancel_requested'
        : receipt.before_state !== receipt.after_state)
    )
      throw Error('Cancel receipt expectation mismatch');
  }
  const digest = await structureRequestDigest(
    kind,
    { ...scope, task: taskId },
    outcome.original_request,
  );
  if (digest !== outcome.request_sha256)
    throw Error('Structure request digest mismatch');
  if (
    operation &&
    (operation.body.idempotency_key !==
      outcome.original_request.idempotency_key ||
      (await structureRequestDigest(
        operation.kind,
        operation.scope,
        operation.body,
      )) !== digest)
  )
    throw Error('Structure original request mismatch');
  if (kind === 'task' && !rejected) {
    const task = outcome as StructureTask;
    if (scope.task && task.task_id !== scope.task)
      throw Error('Task identity mismatch');
    if (
      (await structureDigest(
        structureProducerProjection(task.original_request.aliases),
      )) !== task.producer_sha256
    )
      throw Error('Producer identity mismatch');
    const members = new Map(
      task.original_request.members.map((member) => [member.source_id, member]),
    );
    if (
      members.size !== task.members.length ||
      new Set(task.members.map((member) => member.source_id)).size !==
        members.size ||
      task.members.some((member) => {
        const expected = members.get(member.source_id);
        return (
          !expected ||
          expected.source_revision_id !== member.source_revision_id ||
          expected.lifecycle_version !== member.lifecycle_version ||
          expected.parser !== member.parser
        );
      })
    )
      throw Error('Task members mismatch');
  }
}
export async function checkStructureView(
  scope: StructureViewScope,
  view: StructureView,
): Promise<void> {
  if (
    view.owner_id !== scope.owner ||
    view.domain_id !== scope.domain ||
    view.view_id !== scope.view ||
    (await structureDigest(structureViewProjection(view))) !== view.view_id ||
    (await structureDigest(structureProducerProjection(view.aliases))) !==
      view.producer_sha256 ||
    view.members.reduce((sum, member) => sum + member.byte_length, 0) !==
      view.source_bytes
  )
    throw Error('Structure view identity mismatch');
}
export async function checkStructureNode(
  scope: StructureViewScope,
  node: StructureNode,
): Promise<void> {
  if (
    node.view_id !== scope.view ||
    (await structureDigest(structureLogicalProjection(scope.domain, node))) !==
      node.logical_node_id ||
    (await structureDigest(structureNodeProjection(node))) !==
      node.structure_node_id
  )
    throw Error('Structure node identity mismatch');
}
export async function checkStructureContent(
  node: StructureNode,
  sourceSha: string,
  content: StructureContent,
): Promise<void> {
  if (
    [
      'view_id',
      'structure_node_id',
      'source_node_id',
      'source_id',
      'source_revision_id',
    ].some(
      (key) =>
        content[key as keyof StructureContent] !==
        node[key as keyof StructureNode],
    ) ||
    content.source_sha256 !== sourceSha ||
    content.start_byte < node.start_byte ||
    content.end_byte > node.end_byte ||
    content.complete !== (content.end_byte === node.end_byte) ||
    content.end_byte < content.start_byte ||
    structureBytes(content.text) !== content.end_byte - content.start_byte ||
    (!content.complete &&
      (content.next_start_byte !== content.end_byte ||
        content.end_byte <= content.start_byte)) ||
    (await webSha256(Encoder.encode(content.text))) !== content.fragment_sha256
  )
    throw Error('Structure content integrity mismatch');
}
export function structureBytePosition(
  text: string,
  byteOffset: number,
): { lineNumber: number; column: number } {
  const bytes = Encoder.encode(text);
  if (
    !Number.isSafeInteger(byteOffset) ||
    byteOffset < 0 ||
    byteOffset > bytes.length
  )
    throw Error('Invalid byte position');
  const prefix = new TextDecoder('utf-8', {
    fatal: true,
    ignoreBOM: true,
  }).decode(bytes.slice(0, byteOffset));
  const lines = prefix.split(/\r\n|\r|\n/);
  return {
    lineNumber: lines.length,
    column: lines[lines.length - 1].length + 1,
  };
}
function slotPrefix(owner: string) {
  return `knowledge-structure:0.1.0:${owner}:`;
}
export function structureSlotKey(slot: StructureRecoverySlot): string {
  return (
    slotPrefix(slot.owner) +
    [slot.domain, slot.kind, slot.target, slot.key].join(':')
  );
}
export function structureSlotOperation(
  slot: StructureRecoverySlot,
): StructureOperation {
  const scope = { owner: slot.owner, domain: slot.domain };
  if (slot.kind === 'task')
    return {
      kind: 'task',
      scope: { ...scope, project: slot.target },
      body: slot.body as StructureTaskRequest,
    };
  if (slot.kind === 'cancel')
    return {
      kind: 'cancel',
      scope: { ...scope, task: slot.target },
      body: slot.body as StructureCancelRequest,
    };
  return {
    kind: 'policy',
    scope,
    body: slot.body as StructurePolicyChangeRequest,
  };
}
async function validateSlot(slot: StructureRecoverySlot): Promise<void> {
  if (
    !slot ||
    Object.keys(slot).some(
      (key) =>
        ![
          'version',
          'owner',
          'domain',
          'target',
          'kind',
          'key',
          'body',
          'request_sha256',
          'saved_at',
          'task_id',
        ].includes(key),
    ) ||
    slot.version !== '0.1.0' ||
    !StructureId.test(slot.owner) ||
    !StructureId.test(slot.domain) ||
    !StructureId.test(slot.target) ||
    !StructureKey.test(slot.key) ||
    !['task', 'policy', 'cancel'].includes(slot.kind) ||
    slot.key !== slot.body?.idempotency_key ||
    !validStructureData(
      slot.kind === 'task'
        ? 'TaskRequest'
        : slot.kind === 'policy'
          ? 'PolicyChangeRequest'
          : 'CancelRequest',
      slot.body,
    ) ||
    (slot.kind === 'policy' && slot.target !== slot.domain) ||
    (slot.kind === 'task' &&
      (slot.body as StructureTaskRequest).project_id !== slot.target) ||
    (slot.task_id !== undefined &&
      (slot.kind !== 'task' || !StructureId.test(slot.task_id))) ||
    !validStructureData('Timestamp', slot.saved_at) ||
    structureJsonBytes(slot) > 16384 ||
    (await structureRequestDigest(
      slot.kind,
      structureSlotOperation(slot).scope,
      slot.body,
    )) !== slot.request_sha256
  )
    throw Error('structure_storage_corrupt');
}
export async function readStructureSlots(
  storage: Storage,
  owner: string,
): Promise<StructureRecoverySlot[]> {
  if (!StructureId.test(owner)) throw Error('structure_storage_corrupt');
  const slots: StructureRecoverySlot[] = [];
  for (let index = 0; index < storage.length; index++) {
    const key = storage.key(index);
    if (!key?.startsWith(slotPrefix(owner))) continue;
    const slot = JSON.parse(
      storage.getItem(key) ?? 'null',
    ) as StructureRecoverySlot;
    await validateSlot(slot);
    if (slot.owner !== owner || key !== structureSlotKey(slot))
      throw Error('structure_storage_corrupt');
    slots.push(slot);
  }
  if (slots.length > 8) throw Error('structure_storage_full');
  return slots;
}
export async function saveStructureSlot(
  storage: Storage,
  operation: StructureOperation,
): Promise<StructureRecoverySlot> {
  const slots = await readStructureSlots(storage, operation.scope.owner);
  if (slots.length >= 8) throw Error('structure_storage_full');
  const slot: StructureRecoverySlot = {
    version: '0.1.0',
    owner: operation.scope.owner,
    domain: operation.scope.domain,
    target:
      operation.kind === 'task'
        ? operation.scope.project
        : operation.kind === 'cancel'
          ? operation.scope.task
          : operation.scope.domain,
    kind: operation.kind,
    key: operation.body.idempotency_key,
    body: JSON.parse(JSON.stringify(operation.body)),
    request_sha256: await structureRequestDigest(
      operation.kind,
      operation.scope,
      operation.body,
    ),
    saved_at: dayjs().toISOString(),
  };
  await validateSlot(slot);
  const key = structureSlotKey(slot);
  if (storage.getItem(key) !== null) throw Error('structure_storage_conflict');
  const serialized = JSON.stringify(slot);
  storage.setItem(key, serialized);
  if (storage.getItem(key) !== serialized)
    throw Error('structure_storage_failed');
  return slot;
}
export async function acceptStructureSlot(
  storage: Storage,
  slot: StructureRecoverySlot,
  outcome: StructureOutcome,
): Promise<void> {
  await validateSlot(slot);
  const operation = structureSlotOperation(slot);
  await checkStructureOutcome(operation.scope, outcome, operation);
  if (structureRejected(outcome)) return;
  if (slot.kind === 'task' && !structureTerminal(outcome as StructureTask)) {
    const next = { ...slot, task_id: (outcome as StructureTask).task_id };
    await validateSlot(next);
    const serialized = JSON.stringify(next);
    storage.setItem(structureSlotKey(slot), serialized);
    if (storage.getItem(structureSlotKey(slot)) !== serialized)
      throw Error('structure_storage_failed');
  } else storage.removeItem(structureSlotKey(slot));
}
export async function abandonStructureSlot(
  storage: Storage,
  slot: StructureRecoverySlot,
  rejection: StructureRejectionReceipt,
): Promise<void> {
  if (!structureRejected(rejection)) throw Error('Fixed rejection required');
  await validateSlot(slot);
  await checkStructureOutcome(
    structureSlotOperation(slot).scope,
    rejection,
    structureSlotOperation(slot),
  );
  storage.removeItem(structureSlotKey(slot));
}

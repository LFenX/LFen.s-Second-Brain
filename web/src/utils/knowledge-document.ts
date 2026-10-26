import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import type {
  DocumentTask,
  DocumentOperationRecord,
  DocumentRepresentation,
  DocumentNode,
  DocumentContentWindow,
  DocumentOriginalRef,
  DocumentRejectionReceipt,
} from '@/interfaces/database/knowledge-document';
import type {
  DocumentTaskRequest,
  DocumentPolicyChangeRequest,
  DocumentCancelRequest,
  DocumentContentQuery,
} from '@/interfaces/request/knowledge-document';
import { encodeWebValue } from './knowledge-web';
import {
  DocumentId,
  DocumentKey,
  parseDocumentJson,
  documentDigest,
  documentJsonBytes,
  documentSha,
  documentBytes,
  validDocumentData,
} from './knowledge-document-schema';
import dayjs from 'dayjs';

export type DocumentScope = { owner: string; domain: string };
export type DocumentSourceScope = DocumentScope & { source: string };
export type DocumentViewScope = DocumentSourceScope & {
  revision: string;
  sha: string;
  representation: string;
};
export type DocumentWriteKind = 'task' | 'policy' | 'cancel';
export type DocumentWriteBody =
  | DocumentTaskRequest
  | DocumentPolicyChangeRequest
  | DocumentCancelRequest;
export type DocumentOperation =
  | { kind: 'task'; scope: DocumentSourceScope; body: DocumentTaskRequest }
  | { kind: 'policy'; scope: DocumentScope; body: DocumentPolicyChangeRequest }
  | {
      kind: 'cancel';
      scope: DocumentScope & { task: string };
      body: DocumentCancelRequest;
    };
export type DocumentOutcome = DocumentOperationRecord;
export type DocumentRecoverySlot = {
  version: '0.1.0';
  owner: string;
  domain: string;
  target: string;
  kind: DocumentWriteKind;
  key: string;
  body: DocumentWriteBody;
  request_sha256: string;
  saved_at: string;
  task_id?: string;
};
export const sameDocumentValue = (a: unknown, b: unknown) =>
  encodeWebValue(a) === encodeWebValue(b);
export const documentTerminal = (task: DocumentTask) =>
  ['succeeded', 'failed', 'cancelled'].includes(task.state);
export function latestDocumentTask(
  current: DocumentTask | null,
  next: DocumentTask,
): DocumentTask {
  if (!current) return next;
  if (
    current.task_id !== next.task_id ||
    current.request_sha256 !== next.request_sha256
  )
    throw Error('document_integrity');
  if (next.revision < current.revision) return current;
  if (next.revision === current.revision && !sameDocumentValue(current, next))
    throw Error('document_integrity');
  return next;
}
export const documentRejected = (
  outcome: DocumentOutcome,
): outcome is DocumentRejectionReceipt => outcome.record_kind === 'rejection';

export function documentRequestProjection(
  kind: DocumentWriteKind,
  scope: DocumentScope & { task?: string },
  body: DocumentWriteBody,
): object {
  const request = Object.fromEntries(
    Object.entries(body).filter(([key]) => key !== 'idempotency_key'),
  );
  return {
    kind: `knowledge-document-${kind}/0.1.0`,
    owner_id: scope.owner,
    domain_id: scope.domain,
    ...(kind === 'cancel' ? { task_id: scope.task } : {}),
    request,
  };
}
export const documentRequestDigest = (
  kind: DocumentWriteKind,
  scope: DocumentScope & { task?: string },
  body: DocumentWriteBody,
) => documentDigest(documentRequestProjection(kind, scope, body));
export async function checkDocumentOutcome(
  scope: DocumentScope,
  outcome: DocumentOutcome,
  operation?: DocumentOperation,
): Promise<void> {
  if (
    !(await validDocumentData('OperationRecord', outcome)) ||
    outcome.owner_id !== scope.owner ||
    outcome.domain_id !== scope.domain
  )
    throw Error('document_integrity');
  const kind =
    outcome.record_kind === 'task'
      ? 'task'
      : outcome.record_kind === 'policy_receipt'
        ? 'policy'
        : outcome.record_kind === 'cancel_receipt'
          ? 'cancel'
          : outcome.operation;
  const target =
    kind === 'task'
      ? (outcome.original_request as DocumentTaskRequest).target.source_id
      : kind === 'cancel'
        ? 'target' in outcome
          ? (outcome.target as { task_id: string }).task_id
          : ''
        : scope.domain;
  const digest = await documentRequestDigest(
    kind,
    { ...scope, ...(kind === 'cancel' ? { task: target } : {}) },
    outcome.original_request,
  );
  if (outcome.request_sha256 !== digest) throw Error('document_integrity');
  if (
    operation &&
    (operation.kind !== kind ||
      !sameDocumentValue(operation.body, outcome.original_request) ||
      (operation.kind === 'task' && operation.scope.source !== target) ||
      (operation.kind === 'cancel' && operation.scope.task !== target))
  )
    throw Error('document_integrity');
}
export function sameDocumentOriginal(
  ref: DocumentOriginalRef,
  original: OriginalRevision,
): boolean {
  return [
    'domain_id',
    'registration_id',
    'source_id',
    'source_revision_id',
    'sha256',
    'byte_length',
    'media_type',
    'original_filename',
    'representation_kind',
    'root_node_id',
    'text_available',
  ].every(
    (key) =>
      ref[key as keyof DocumentOriginalRef] ===
      original[key as keyof OriginalRevision],
  );
}
export async function checkDocumentNode(
  representation: DocumentRepresentation,
  node: DocumentNode,
): Promise<void> {
  if (
    !(await validDocumentData('DocumentNode', node)) ||
    node.representation_id !== representation.representation_id ||
    node.source_id !== representation.original.source_id ||
    node.source_revision_id !== representation.original.source_revision_id ||
    node.original_sha256 !== representation.original.sha256 ||
    node.producer_sha256 !== representation.producer_sha256 ||
    node.payload.reading_range.end_byte > representation.reading_byte_length ||
    (node.payload.kind === 'document' &&
      node.document_node_id !== representation.root_document_node_id)
  )
    throw Error('document_integrity');
}
export async function checkDocumentContent(
  representation: DocumentRepresentation,
  node: DocumentNode,
  content: DocumentContentWindow,
  query: DocumentContentQuery,
): Promise<void> {
  const r = node.payload.reading_range;
  if (
    !(await validDocumentData('ContentWindow', content)) ||
    [
      'representation_id',
      'document_node_id',
      'source_id',
      'source_revision_id',
      'original_sha256',
    ].some(
      (key) =>
        content[key as keyof DocumentContentWindow] !==
        node[key as keyof DocumentNode],
    ) ||
    content.reading_text_sha256 !== representation.reading_text_sha256 ||
    content.node_text_sha256 !== r.text_sha256 ||
    content.text_status !== node.payload.text_status ||
    content.start_byte !== (query.start_byte ?? r.start_byte) ||
    (query.end_byte !== undefined && content.end_byte !== query.end_byte) ||
    content.start_byte < r.start_byte ||
    content.end_byte > r.end_byte ||
    content.complete !== (content.end_byte === r.end_byte)
  )
    throw Error('document_integrity');
  if (
    content.start_byte === r.start_byte &&
    content.end_byte === r.end_byte &&
    (await documentSha(content.text)) !== r.text_sha256
  )
    throw Error('document_integrity');
  if (
    content.start_byte === 0 &&
    content.end_byte === representation.reading_byte_length &&
    (await documentSha(content.text)) !== representation.reading_text_sha256
  )
    throw Error('document_integrity');
}
export function documentOwnQuery(
  node: DocumentNode,
  start?: number,
): DocumentContentQuery | null {
  const range = node.payload.own_reading_range;
  if (!range || range.start_byte === range.end_byte) return null;
  const at = start ?? range.start_byte;
  if (at < range.start_byte || at >= range.end_byte) return null;
  return range.end_byte - at <= 65536
    ? { start_byte: at, end_byte: range.end_byte }
    : { start_byte: at };
}
export function documentSlice(
  text: string,
  start: number,
  end: number,
): string {
  const bytes = new TextEncoder().encode(text);
  if (start < 0 || end < start || end > bytes.length)
    throw Error('document_integrity');
  return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
    bytes.slice(start, end),
  );
}
export async function appendDocumentContent(
  previous: string,
  next: DocumentContentWindow,
  range: { start_byte: number; end_byte: number; text_sha256: string },
): Promise<string> {
  if (
    next.start_byte !== range.start_byte + documentBytes(previous) ||
    next.end_byte > range.end_byte
  )
    throw Error('document_integrity');
  const text = previous + next.text;
  if (
    next.end_byte === range.end_byte &&
    (await documentSha(text)) !== range.text_sha256
  )
    throw Error('document_integrity');
  return text;
}
export function documentFixedLink(scope: DocumentViewScope, node?: string) {
  return (
    `/dataset/knowledge-catalogue/${scope.domain}?` +
    new URLSearchParams({
      document_structure: '1',
      document_source: scope.source,
      document_revision: scope.revision,
      document_original_sha: scope.sha,
      document_representation: scope.representation,
      ...(node ? { document_node: node } : {}),
    })
  );
}

function slotPrefix(owner: string) {
  return `knowledge-document-structure:0.1.0:${owner}:`;
}
export function documentSlotKey(slot: DocumentRecoverySlot): string {
  return (
    slotPrefix(slot.owner) +
    [slot.domain, slot.kind, slot.target, slot.key].join(':')
  );
}
export function documentSlotOperation(
  slot: DocumentRecoverySlot,
): DocumentOperation {
  const scope = { owner: slot.owner, domain: slot.domain };
  if (slot.kind === 'task')
    return {
      kind: 'task',
      scope: { ...scope, source: slot.target },
      body: slot.body as DocumentTaskRequest,
    };
  if (slot.kind === 'cancel')
    return {
      kind: 'cancel',
      scope: { ...scope, task: slot.target },
      body: slot.body as DocumentCancelRequest,
    };
  return {
    kind: 'policy',
    scope,
    body: slot.body as DocumentPolicyChangeRequest,
  };
}
async function validateSlot(slot: DocumentRecoverySlot): Promise<void> {
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
    !DocumentId.test(slot.owner) ||
    !DocumentId.test(slot.domain) ||
    !DocumentId.test(slot.target) ||
    !DocumentKey.test(slot.key) ||
    !['task', 'policy', 'cancel'].includes(slot.kind) ||
    slot.key !== slot.body?.idempotency_key ||
    !(await validDocumentData(
      slot.kind === 'task'
        ? 'TaskRequest'
        : slot.kind === 'policy'
          ? 'PolicyChangeRequest'
          : 'CancelRequest',
      slot.body,
    )) ||
    (slot.kind === 'policy' && slot.target !== slot.domain) ||
    (slot.kind === 'task' &&
      (slot.body as DocumentTaskRequest).target.source_id !== slot.target) ||
    (slot.task_id !== undefined &&
      (slot.kind !== 'task' || !DocumentId.test(slot.task_id))) ||
    !(await validDocumentData('Timestamp', slot.saved_at)) ||
    documentJsonBytes(slot) > 16384 ||
    (await documentRequestDigest(
      slot.kind,
      documentSlotOperation(slot).scope,
      slot.body,
    )) !== slot.request_sha256
  )
    throw Error('document_storage_corrupt');
}
export async function readDocumentSlots(
  storage: Storage,
  owner: string,
): Promise<DocumentRecoverySlot[]> {
  if (!DocumentId.test(owner)) throw Error('document_storage_corrupt');
  const slots: DocumentRecoverySlot[] = [];
  for (let index = 0; index < storage.length; index++) {
    const key = storage.key(index);
    if (!key?.startsWith(slotPrefix(owner))) continue;
    const slot = parseDocumentJson(
      storage.getItem(key) ?? 'null',
    ) as DocumentRecoverySlot;
    await validateSlot(slot);
    if (slot.owner !== owner || key !== documentSlotKey(slot))
      throw Error('document_storage_corrupt');
    slots.push(slot);
  }
  if (slots.length > 8) throw Error('document_storage_full');
  return slots;
}
export async function saveDocumentSlot(
  storage: Storage,
  operation: DocumentOperation,
): Promise<DocumentRecoverySlot> {
  const slots = await readDocumentSlots(storage, operation.scope.owner);
  if (slots.length >= 8) throw Error('document_storage_full');
  const slot: DocumentRecoverySlot = {
    version: '0.1.0',
    owner: operation.scope.owner,
    domain: operation.scope.domain,
    target:
      operation.kind === 'task'
        ? operation.scope.source
        : operation.kind === 'cancel'
          ? operation.scope.task
          : operation.scope.domain,
    kind: operation.kind,
    key: operation.body.idempotency_key,
    body: JSON.parse(JSON.stringify(operation.body)),
    request_sha256: await documentRequestDigest(
      operation.kind,
      operation.scope,
      operation.body,
    ),
    saved_at: dayjs().toISOString(),
  };
  await validateSlot(slot);
  if (
    slots.some(
      (value) =>
        value.domain === slot.domain &&
        value.kind === slot.kind &&
        value.key === slot.key,
    )
  )
    throw Error('document_storage_conflict');
  const key = documentSlotKey(slot);
  if (storage.getItem(key) !== null) throw Error('document_storage_conflict');
  const serialized = JSON.stringify(slot);
  storage.setItem(key, serialized);
  if (storage.getItem(key) !== serialized)
    throw Error('document_storage_failed');
  return slot;
}
export async function acceptDocumentSlot(
  storage: Storage,
  slot: DocumentRecoverySlot,
  outcome: DocumentOutcome,
): Promise<void> {
  await validateSlot(slot);
  const operation = documentSlotOperation(slot);
  await checkDocumentOutcome(operation.scope, outcome, operation);
  if (documentRejected(outcome)) return;
  if (slot.kind === 'task' && !documentTerminal(outcome as DocumentTask)) {
    const next = { ...slot, task_id: (outcome as DocumentTask).task_id };
    await validateSlot(next);
    const serialized = JSON.stringify(next);
    storage.setItem(documentSlotKey(slot), serialized);
    if (storage.getItem(documentSlotKey(slot)) !== serialized)
      throw Error('document_storage_failed');
  } else storage.removeItem(documentSlotKey(slot));
}
export async function abandonDocumentSlot(
  storage: Storage,
  slot: DocumentRecoverySlot,
  rejection: DocumentRejectionReceipt,
): Promise<void> {
  if (!documentRejected(rejection)) throw Error('Fixed rejection required');
  await validateSlot(slot);
  await checkDocumentOutcome(
    documentSlotOperation(slot).scope,
    rejection,
    documentSlotOperation(slot),
  );
  storage.removeItem(documentSlotKey(slot));
}

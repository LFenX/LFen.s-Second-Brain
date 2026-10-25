import type {
  BatchPreview,
  BatchTask,
  BatchTaskItem,
  BatchTaskPage,
  BatchTaskItemsPage,
  BatchContentWindow,
  BatchManifestEntry,
  BatchManifestPreviewPage,
  BatchManifestVersionPage,
  BatchManifestHistoryPage,
  BatchManifestReceipt,
} from '@/interfaces/database/knowledge-batch';
import type {
  BatchAcceptRequest,
  BatchPreviewRequest,
  BatchManifestPublicationRequest,
} from '@/interfaces/request/knowledge-batch';
import type { DirectoryObservation } from '@/interfaces/database/knowledge-directory';
import { knowledgeImportStorageKey } from './knowledge-import';
import { sameDirectoryClassification } from './knowledge-directory';
import { compareDirectoryPaths } from './knowledge-directory-schema';
import { validBatchData, batchTerminal } from './knowledge-batch-schema';
import dayjs from 'dayjs';

export type BatchPending = {
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  domain_id: string;
  registration_id: string;
  request: BatchAcceptRequest;
  task_id: string | null;
};
export type ManifestPending = {
  contract_version: '1.0.0';
  source_contract_version: '0.4.0';
  domain_id: string;
  request: BatchManifestPublicationRequest;
  expected_manifest_id: string | null;
  task_id: string;
};
export type BatchSavedItem = BatchTaskItem & {
  status: 'succeeded';
  source_id: string;
  source_revision_id: string;
  root_node_id: string;
};
export const batchSaved = (item: BatchTaskItem): item is BatchSavedItem =>
  item.status === 'succeeded' &&
  !!item.source_id &&
  !!item.source_revision_id &&
  !!item.root_node_id;
export function batchStorageKey(
  domain: string,
  authorization: string,
  userId?: string,
) {
  return knowledgeImportStorageKey(domain, authorization, userId).then(
    (key) => 'batch:0.4.0:' + key,
  );
}
const Id = /^[a-f0-9]{32}$/;
export const batchPending = (
  domain: string,
  registration: string,
  request: BatchAcceptRequest,
  taskId: string | null = null,
): BatchPending => ({
  contract_version: '1.0.0',
  source_contract_version: '0.4.0',
  domain_id: domain,
  registration_id: registration,
  request,
  task_id: taskId,
});
export const manifestPending = (
  domain: string,
  request: BatchManifestPublicationRequest,
  expected: string | null,
  taskId: string,
): ManifestPending => ({
  contract_version: '1.0.0',
  source_contract_version: '0.4.0',
  domain_id: domain,
  request,
  expected_manifest_id: expected,
  task_id: taskId,
});
function validPending(
  value: unknown,
  kind: 'task' | 'publication',
  domain: string,
) {
  if (!value || typeof value !== 'object') return false;
  const v = value as Record<string, any>;
  if (
    v.contract_version !== '1.0.0' ||
    v.source_contract_version !== '0.4.0' ||
    v.domain_id !== domain ||
    !Id.test(domain)
  )
    return false;
  const keys =
    kind === 'task'
      ? [
          'contract_version',
          'source_contract_version',
          'domain_id',
          'registration_id',
          'request',
          'task_id',
        ]
      : [
          'contract_version',
          'source_contract_version',
          'domain_id',
          'request',
          'expected_manifest_id',
          'task_id',
        ];
  if (Object.keys(v).sort().join(',') !== keys.sort().join(',')) return false;
  return kind === 'task'
    ? Id.test(v.registration_id) &&
        (v.task_id === null || Id.test(v.task_id)) &&
        validBatchData('AcceptRequest', v.request)
    : (v.expected_manifest_id === null || Id.test(v.expected_manifest_id)) &&
        Id.test(v.task_id) &&
        validBatchData('ManifestPublicationRequest', v.request);
}
export function loadBatchRecovery(
  key: string,
  kind: 'task',
  domain: string,
): BatchPending | null;
export function loadBatchRecovery(
  key: string,
  kind: 'publication',
  domain: string,
): ManifestPending | null;
export function loadBatchRecovery(
  key: string,
  kind: 'task' | 'publication',
  domain: string,
): BatchPending | ManifestPending | null {
  try {
    const value = JSON.parse(localStorage.getItem(key + ':' + kind) || 'null');
    if (value === null) return null;
    if (!validPending(value, kind, domain))
      throw Error('Invalid recovery metadata');
    return value;
  } catch {
    try {
      localStorage.removeItem(key + ':' + kind);
    } catch {
      /* storage failure cannot authorize a POST */
    }
    return null;
  }
}
export function saveBatchRecovery(
  key: string,
  kind: 'task' | 'publication',
  value: BatchPending | ManifestPending,
) {
  if (!validPending(value, kind, value.domain_id))
    throw Error('Invalid recovery metadata');
  localStorage.setItem(key + ':' + kind, JSON.stringify(value));
}
export function clearBatchRecovery(key: string, kind: 'task' | 'publication') {
  localStorage.removeItem(key + ':' + kind);
}
export function batchTaskMatches(task: BatchTask, request: BatchAcceptRequest) {
  return (
    [
      'preview_id',
      'manifest_sha256',
      'idempotency_key',
      'retry_of_task_id',
    ].every(
      (key) =>
        task[key as keyof BatchTask] ===
        request[key as keyof BatchAcceptRequest],
    ) &&
    JSON.stringify(task.confirmed_item_ids) ===
      JSON.stringify(request.confirmed_item_ids)
  );
}
export function batchReceiptMatches(
  receipt: BatchManifestReceipt,
  pending: ManifestPending,
) {
  return (
    receipt.domain_id === pending.domain_id &&
    receipt.task_id === pending.task_id &&
    receipt.expected_manifest_id === pending.expected_manifest_id &&
    ['project_id', 'preview_id', 'manifest_sha256', 'idempotency_key'].every(
      (key) =>
        receipt[key as keyof BatchManifestReceipt] ===
        pending.request[key as keyof BatchManifestPublicationRequest],
    )
  );
}
export function batchPreviewMatches(
  value: BatchPreview,
  request: BatchPreviewRequest,
  observation: DirectoryObservation,
) {
  if (
    !observation.complete ||
    value.observation_id !== observation.observation_id ||
    value.observation_sha256 !== observation.observation_sha256 ||
    value.domain_id !== observation.domain_id ||
    value.registration_id !== observation.registration_id ||
    value.root_id !== observation.root_id ||
    value.root_version !== observation.root_version ||
    value.policy_version !== observation.policy_version ||
    value.items.length !== request.selected_items.length ||
    dayjs(value.expires_at).isAfter(dayjs(observation.expires_at))
  )
    return false;
  return value.items.every((item, n) => {
    const selected = request.selected_items[n],
      observed = observation.items.find(
        (i) => i.relative_path === selected.relative_path,
      );
    return (
      !!observed?.eligible &&
      item.relative_path === selected.relative_path &&
      sameDirectoryClassification(
        item.classification,
        selected.classification,
      ) &&
      item.sha256 === observed.sha256 &&
      item.byte_length === observed.byte_length &&
      item.source_id === observed.source_id &&
      item.expected_revision_id === observed.expected_revision_id
    );
  });
}
export function batchWindowMatches(
  value: BatchContentWindow,
  preview: BatchPreview,
  itemId: string,
  offset: number,
) {
  const item = preview.items.find((i) => i.item_id === itemId);
  return (
    !!item &&
    value.preview_id === preview.preview_id &&
    value.item_id === itemId &&
    value.relative_path === item.relative_path &&
    value.offset === offset &&
    value.total_bytes === item.byte_length &&
    value.sha256 === item.sha256
  );
}
function sameFields(
  left: Record<string, unknown>,
  right: Record<string, unknown>,
  keys: string[],
) {
  return keys.every(
    (key) => JSON.stringify(left[key]) === JSON.stringify(right[key]),
  );
}
function countsEqual(a: BatchTask['counts'], b: BatchTask['counts']) {
  return [
    'total',
    'pending',
    'running',
    'succeeded',
    'failed',
    'cancelled',
  ].every((key) => a[key as keyof typeof a] === b[key as keyof typeof b]);
}
export function latestBatchTask(previous: BatchTask | null, next: BatchTask) {
  if (!previous) return next;
  if (
    !sameFields(previous, next, [
      'task_id',
      'domain_id',
      'registration_id',
      'preview_id',
      'manifest_sha256',
      'idempotency_key',
      'confirmed_item_ids',
      'retry_of_task_id',
      'created_at',
      'execution_deadline',
    ])
  )
    throw Error('Task identity changed');
  if (next.revision < previous.revision) return previous;
  if (batchTerminal(previous.status) && next.revision !== previous.revision)
    throw Error('Terminal task revision changed');
  if (
    (next.revision === previous.revision || batchTerminal(previous.status)) &&
    (next.status !== previous.status ||
      !countsEqual(next.counts, previous.counts) ||
      next.cancel_requested !== previous.cancel_requested ||
      next.terminal_at !== previous.terminal_at)
  )
    throw Error('Task revision changed');
  return next;
}
export function appendBatchTaskPage(
  pages: BatchTaskPage[],
  next: BatchTaskPage,
) {
  if (
    pages[0] &&
    !sameFields(pages[0], next, [
      'domain_id',
      'registration_id',
      'list_snapshot_id',
      'total_tasks',
      'captured_at',
      'expires_at',
    ])
  )
    throw Error('Task list changed');
  const all = [...pages, next],
    items = all.flatMap((p) => p.items);
  if (
    new Set(items.map((i) => i.task_id)).size !== items.length ||
    items.length > next.total_tasks ||
    (!next.next_cursor && items.length !== next.total_tasks)
  )
    throw Error('Invalid task page');
  return all;
}
export function appendBatchResultPage(
  pages: BatchTaskItemsPage[],
  next: BatchTaskItemsPage,
  task: BatchTask,
) {
  const first = pages[0];
  if (
    next.task_id !== task.task_id ||
    next.registration_id !== task.registration_id ||
    (first &&
      (first.result_revision !== next.result_revision ||
        first.status !== next.status ||
        !countsEqual(first.counts, next.counts)))
  )
    throw Error('Result revision changed');
  const all = [...pages, next],
    items = all.flatMap((p) => p.items);
  if (
    next.counts.total !== task.confirmed_item_ids.length ||
    items.some(
      (i, n) => i.ordinal !== n || task.confirmed_item_ids[n] !== i.item_id,
    ) ||
    items.length > next.counts.total ||
    (!next.next_cursor && items.length !== next.counts.total)
  )
    throw Error('Invalid result page');
  if (!next.next_cursor) {
    const c = next.counts;
    if (
      !['pending', 'running', 'succeeded', 'failed', 'cancelled'].every(
        (status) =>
          items.filter((i) => i.status === status).length ===
          c[status as keyof typeof c],
      )
    )
      throw Error('Result counts changed');
  }
  return all;
}
export function appendBatchManifestPage<
  T extends BatchManifestPreviewPage | BatchManifestVersionPage,
>(pages: T[], next: T) {
  const first = pages[0];
  if (
    first &&
    !sameFields(
      first,
      next,
      Object.keys(first).filter((k) => !['items', 'next_cursor'].includes(k)),
    )
  )
    throw Error('Manifest changed');
  const all = [...pages, next],
    items = all.flatMap((p) => p.items),
    keys = items.map((i) => i.registration_id + '/' + i.relative_path);
  if (
    keys.some((k, n) => n > 0 && compareDirectoryPaths(keys[n - 1], k) >= 0) ||
    new Set(items.map((i) => i.source_id)).size !== items.length ||
    items.length > next.total_entries ||
    (!next.next_cursor && items.length !== next.total_entries)
  )
    throw Error('Invalid manifest page');
  return all;
}
export function appendBatchHistoryPage(
  pages: BatchManifestHistoryPage[],
  next: BatchManifestHistoryPage,
) {
  if (
    pages[0] &&
    !sameFields(pages[0], next, [
      'domain_id',
      'project_id',
      'list_snapshot_id',
      'total_manifests',
      'captured_at',
      'expires_at',
    ])
  )
    throw Error('History changed');
  const all = [...pages, next],
    items = all.flatMap((p) => p.items);
  if (
    new Set(items.map((i) => i.manifest_id)).size !== items.length ||
    items.length > next.total_manifests ||
    (!next.next_cursor && items.length !== next.total_manifests)
  )
    throw Error('Invalid history page');
  return all;
}
export function batchRetryPaths(
  parent: BatchTask,
  items: BatchTaskItem[],
  full: boolean,
) {
  if (
    !['partial_failed', 'failed', 'cancelled'].includes(parent.status) ||
    items.length !== parent.counts.total ||
    items.some(
      (i, n) => i.item_id !== parent.confirmed_item_ids[n] || i.ordinal !== n,
    )
  )
    throw Error('Invalid retry parent');
  const paths = items
    .filter((i) => full || i.status !== 'succeeded')
    .map((i) => i.relative_path)
    .sort(compareDirectoryPaths);
  if (!paths.length) throw Error('Empty retry');
  return paths;
}
export function batchRetrySelection(
  parent: BatchTask,
  parentPaths: string[],
  unfinished: string[],
  selected: string[],
  domain: string,
  registration: string,
) {
  if (
    parent.domain_id !== domain ||
    parent.registration_id !== registration ||
    !['partial_failed', 'failed', 'cancelled'].includes(parent.status) ||
    !selected.length ||
    new Set(selected).size !== selected.length ||
    parentPaths.length !== parent.counts.total
  )
    return false;
  return (
    (selected.length === parentPaths.length &&
      selected.every((p) => parentPaths.includes(p))) ||
    selected.every((p) => unfinished.includes(p))
  );
}
export function batchProjectMatches(
  page: BatchManifestPreviewPage,
  entries: BatchManifestEntry[],
  task: BatchTask,
  results: BatchTaskItem[],
  base: BatchManifestEntry[],
) {
  if (
    task.status !== 'succeeded' ||
    page.task_id !== task.task_id ||
    page.domain_id !== task.domain_id ||
    page.task_manifest_sha256 !== task.manifest_sha256 ||
    results.length !== task.counts.total ||
    results.some(
      (i, n) => !batchSaved(i) || i.item_id !== task.confirmed_item_ids[n],
    )
  )
    return false;
  const expected = new Map(base.map((i) => [i.source_id, i]));
  for (const item of results) {
    if (!batchSaved(item)) return false;
    expected.set(item.source_id, {
      registration_id: task.registration_id,
      relative_path: item.relative_path,
      source_id: item.source_id,
      source_revision_id: item.source_revision_id,
      root_node_id: item.root_node_id,
      sha256: item.sha256,
      byte_length: item.byte_length,
      classification: item.classification,
      origin_task_id: task.task_id,
    });
  }
  return (
    entries.length === page.total_entries &&
    entries.length === expected.size &&
    entries.every((i) => {
      const before = expected.get(i.source_id);
      return (
        !!before &&
        sameFields(before, i, [
          'registration_id',
          'relative_path',
          'source_id',
          'source_revision_id',
          'root_node_id',
          'sha256',
          'byte_length',
          'origin_task_id',
        ]) &&
        sameDirectoryClassification(before.classification, i.classification)
      );
    })
  );
}

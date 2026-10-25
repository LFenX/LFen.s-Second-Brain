import { createHash, webcrypto } from 'node:crypto';
import Examples from '../../../docs/knowledge-batch-import-v1.examples.json';
import { validBatchData } from './knowledge-batch-schema';
import {
  batchPending,
  manifestPending,
  batchStorageKey,
  loadBatchRecovery,
  saveBatchRecovery,
  batchTaskMatches,
  batchReceiptMatches,
  batchProjectMatches,
  batchRetrySelection,
  appendBatchResultPage,
  appendBatchManifestPage,
  latestBatchTask,
  batchPreviewMatches,
  batchWindowMatches,
} from './knowledge-batch';
import {
  makeObservation,
  makePreview,
  acceptRequest,
  makeTask,
  makeResults,
  makeManifest,
  makeReceipt,
  digest,
  batchTokens,
  manifestTokens,
  Registration,
  ProjectId,
  DomainId,
  Id,
  content,
} from '@/pages/dataset/knowledge-import/__tests__/batch-fixture';

beforeAll(() =>
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  }),
);
beforeEach(() => localStorage.clear());
describe('frozen 0.4 examples', () => {
  it.each(Examples.schema_cases)('$id', (item) =>
    expect(validBatchData(item.schema, item.data)).toBe(item.valid),
  );
  it.each(Examples.semantic_cases)('$id', (item) => {
    const v = item.data as Record<string, any>;
    let actual = false;
    if (item.kind === 'task') actual = validBatchData('Task', v);
    if (item.kind === 'preview') actual = validBatchData('BatchPreview', v);
    if (item.kind === 'item') actual = validBatchData('TaskItem', v);
    if (item.kind === 'manifest')
      actual = validBatchData('ManifestPreviewPage', v);
    if (item.kind === 'consent')
      actual =
        validBatchData('BatchPreview', v.preview) &&
        validBatchData('AcceptRequest', v.request) &&
        v.request.preview_id === v.preview.preview_id &&
        v.request.manifest_sha256 === v.preview.manifest_sha256 &&
        JSON.stringify(v.request.confirmed_item_ids) ===
          JSON.stringify(
            v.preview.items.map((i: { item_id: string }) => i.item_id),
          );
    if (item.kind === 'task_recovery')
      actual =
        validBatchData('Task', v.task) && batchTaskMatches(v.task, v.request);
    if (item.kind === 'manifest_recovery')
      actual = batchReceiptMatches(
        v.receipt,
        manifestPending(
          v.domain_id,
          v.request,
          v.expected_manifest_id,
          v.task_id,
        ),
      );
    if (item.kind === 'error')
      actual = validBatchData('Error', v.body, v.http_status);
    if (item.kind === 'window') actual = validBatchData('ContentWindow', v);
    if (item.kind === 'retry')
      actual =
        v.parent_user_id === v.user_id &&
        v.retry_of_task_id === v.parent.task_id &&
        batchRetrySelection(
          v.parent,
          v.parent_paths,
          v.unfinished_paths,
          v.selected_paths,
          v.domain_id,
          v.registration_id,
        );
    if (item.kind === 'project_from_task')
      actual = batchProjectMatches(
        v.manifest_preview,
        v.manifest_preview.items,
        v.task,
        v.results,
        v.base_manifest?.items || [],
      );
    expect(actual).toBe(item.valid);
  });
  it.each(Examples.digest_vectors)(
    'rebuilds $id from input rather than hashing declared tokens',
    (vector) => {
      const tokens =
        vector.kind === 'batch'
          ? batchTokens(
              vector.input as unknown as import('@/interfaces/database/knowledge-batch').BatchPreview,
              'user_id' in vector ? vector.user_id : undefined,
            )
          : manifestTokens(
              vector.input as unknown as import('@/interfaces/database/knowledge-batch').BatchManifestPreviewPage,
            );
      expect(tokens).toEqual(vector.tokens);
      expect(digest(tokens)).toBe(vector.sha256);
      const bytes = Buffer.from(
        tokens
          .map((t) => {
            const s = t === null ? '' : String(t);
            return Buffer.byteLength(s, 'utf8') + ':' + s;
          })
          .join(''),
        'utf8',
      );
      expect(bytes.toString('hex')).toBe(vector.encoded_utf8_hex);
    },
  );
  it.each(Examples.raw_byte_vectors)('preserves $id raw bytes', (vector) => {
    const bytes = Buffer.from(vector.hex, 'hex'),
      text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
        bytes,
      );
    expect(Buffer.from(text, 'utf8')).toEqual(bytes);
    expect(bytes.length).toBe(vector.byte_length);
    expect(createHash('sha256').update(bytes).digest('hex')).toBe(
      vector.sha256,
    );
  });
});
it('binds preview and fixed content to the exact observation, classification and item', () => {
  const observation = makeObservation(),
    preview = makePreview(observation),
    request = {
      observation_id: observation.observation_id,
      observation_sha256: observation.observation_sha256,
      selected_items: preview.items.map((i) => ({
        relative_path: i.relative_path,
        classification: i.classification,
      })),
    };
  expect(batchPreviewMatches(preview, request, observation)).toBe(true);
  expect(
    batchPreviewMatches({ ...preview, root_version: 3 }, request, observation),
  ).toBe(false);
  expect(
    batchPreviewMatches(
      preview,
      {
        ...request,
        selected_items: [
          {
            ...request.selected_items[0],
            classification: {
              group_id: null,
              component_side: 'backend',
              language: '',
            },
          },
          request.selected_items[1],
        ],
      },
      observation,
    ),
  ).toBe(false);
  expect(
    batchWindowMatches(
      content(preview, preview.items[0].item_id),
      preview,
      preview.items[0].item_id,
      0,
    ),
  ).toBe(true);
  expect(
    batchWindowMatches(
      content(preview, preview.items[0].item_id),
      preview,
      preview.items[1].item_id,
      0,
    ),
  ).toBe(false);
});
it('keeps separate durable recovery slots and explicit null bases isolated by user/domain/version', async () => {
  const key = await batchStorageKey(DomainId, 'synthetic-token', 'owner'),
    preview = makePreview(),
    request = acceptRequest(preview),
    pending = batchPending(DomainId, Registration.registration_id, request);
  saveBatchRecovery(key, 'task', pending);
  const publication = manifestPending(
    DomainId,
    {
      project_id: ProjectId,
      preview_id: Id(30),
      manifest_sha256: 'a'.repeat(64),
      idempotency_key: 'original-publish',
    },
    null,
    Id(9),
  );
  saveBatchRecovery(key, 'publication', publication);
  expect(loadBatchRecovery(key, 'task', DomainId)).toEqual(pending);
  expect(loadBatchRecovery(key, 'publication', DomainId)).toEqual(publication);
  expect(
    JSON.parse(localStorage.getItem(key + ':publication')!),
  ).toHaveProperty('expected_manifest_id', null);
  expect(localStorage.getItem(key + ':task')).not.toMatch(
    /relative_path|text|synthetic-token/,
  );
  expect(await batchStorageKey(DomainId, 'renewed-token', 'owner')).toBe(key);
  expect(
    loadBatchRecovery(
      await batchStorageKey(DomainId, 'synthetic-token', 'other'),
      'task',
      DomainId,
    ),
  ).toBeNull();
  expect(
    loadBatchRecovery(
      await batchStorageKey('d'.repeat(32), 'synthetic-token', 'owner'),
      'task',
      'd'.repeat(32),
    ),
  ).toBeNull();
  const corrupted = { ...publication } as Partial<typeof publication>;
  delete corrupted.expected_manifest_id;
  localStorage.setItem(key + ':publication', JSON.stringify(corrupted));
  expect(loadBatchRecovery(key, 'publication', DomainId)).toBeNull();
});
it('merges 32 results only in ordinal order at one revision and rejects drift, duplicates and false totals', () => {
  const paths = Array.from(
      { length: 32 },
      (_, i) => 'web/file-' + String(i).padStart(2, '0') + '.ts',
    ),
    preview = makePreview(makeObservation(paths)),
    task = makeTask(preview, 'succeeded'),
    rows = makeResults(task, preview);
  const page = (start: number, end: number, next: string | null) => ({
    task_id: task.task_id,
    registration_id: task.registration_id,
    result_revision: task.revision,
    status: task.status,
    counts: task.counts,
    items: rows.slice(start, end),
    next_cursor: next,
  });
  let pages: import('@/interfaces/database/knowledge-batch').BatchTaskItemsPage[] =
    [];
  for (const [start, end, next] of [
    [0, 10, 'p2'],
    [10, 20, 'p3'],
    [20, 30, 'p4'],
    [30, 32, null],
  ] as const)
    pages = appendBatchResultPage(pages, page(start, end, next), task);
  expect(pages.flatMap((p) => p.items)).toHaveLength(32);
  expect(() =>
    appendBatchResultPage(
      [page(0, 10, 'p2')],
      { ...page(10, 20, 'p3'), result_revision: task.revision + 1 },
      task,
    ),
  ).toThrow();
  expect(() =>
    appendBatchResultPage([page(0, 10, 'p2')], page(0, 10, null), task),
  ).toThrow();
});
it('does not regress task revisions or mutate a terminal result', () => {
  const preview = makePreview(),
    queued = makeTask(preview),
    done = {
      ...makeTask(preview, 'succeeded'),
      created_at: queued.created_at,
      execution_deadline: queued.execution_deadline,
    };
  expect(latestBatchTask(done, queued)).toBe(done);
  expect(() => latestBatchTask(done, { ...done, revision: 6 })).toThrow();
  expect(() =>
    latestBatchTask(queued, { ...queued, idempotency_key: 'foreign' }),
  ).toThrow();
});
it('preserves the complete project union and immutable pagination while binding publication recovery', () => {
  const preview = makePreview(),
    task = makeTask(preview, 'succeeded'),
    results = makeResults(task, preview),
    manifest = makeManifest(task, results, {
      project_id: ProjectId,
      task_id: task.task_id,
      expected_manifest_id: null,
    });
  expect(batchProjectMatches(manifest, manifest.items, task, results, [])).toBe(
    true,
  );
  expect(
    batchProjectMatches(
      { ...manifest, total_entries: 1 },
      manifest.items.slice(0, 1),
      task,
      results,
      [],
    ),
  ).toBe(false);
  expect(appendBatchManifestPage([], manifest)).toHaveLength(1);
  const receipt = makeReceipt(manifest),
    pending = manifestPending(
      DomainId,
      {
        project_id: ProjectId,
        preview_id: manifest.preview_id,
        manifest_sha256: manifest.manifest_sha256,
        idempotency_key: receipt.idempotency_key,
      },
      null,
      task.task_id,
    );
  expect(batchReceiptMatches(receipt, pending)).toBe(true);
  expect(
    batchReceiptMatches({ ...receipt, expected_manifest_id: Id(99) }, pending),
  ).toBe(false);
  expect(batchReceiptMatches({ ...receipt, task_id: Id(98) }, pending)).toBe(
    false,
  );
});

import Examples from '../../../../docs/knowledge-document-structure-v1.examples.json';
import {
  Scope,
  Task,
  TaskOperation,
  PolicyOperation,
  CancelOperation,
  PolicyReceipt,
  CancelReceipt,
  Rejections,
  Rep,
  Nodes,
  clone,
} from '@/pages/dataset/knowledge-catalogue/__tests__/document-fixture';
import {
  saveDocumentSlot,
  readDocumentSlots,
  acceptDocumentSlot,
  abandonDocumentSlot,
  documentSlotKey,
  documentRequestDigest,
  documentOwnQuery,
  appendDocumentContent,
  documentSlice,
  checkDocumentOutcome,
  sameDocumentOriginal,
  latestDocumentTask,
} from '../knowledge-document';
import { documentSha } from '../knowledge-document-schema';
import type { DocumentOperation } from '../knowledge-document';
import type {
  DocumentContentWindow,
  DocumentNode,
} from '@/interfaces/database/knowledge-document';
beforeEach(() => localStorage.clear());
it('does not downgrade a task revision or accept changed visible fields at the same revision', () => {
  const newer = { ...Task, revision: Task.revision + 1 };
  expect(latestDocumentTask(newer, Task)).toBe(newer);
  expect(() =>
    latestDocumentTask(Task, { ...Task, updated_at: '2026-10-27T00:00:00Z' }),
  ).toThrow('document_integrity');
});
it.each([TaskOperation, PolicyOperation, CancelOperation])(
  'persists complete $kind scope/body/key before transport',
  async (op) => {
    const slot = await saveDocumentSlot(localStorage, op);
    expect(slot.body).toEqual(op.body);
    expect(slot.request_sha256).toBe(
      await documentRequestDigest(op.kind, op.scope, op.body),
    );
    expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([slot]);
    expect(await readDocumentSlots(localStorage, 'f'.repeat(32))).toEqual([]);
  },
);
it('keeps another protocol storage untouched', async () => {
  localStorage.setItem(
    'knowledge-structure:0.1.0:' + Scope.owner + ':legacy',
    'broken',
  );
  await saveDocumentSlot(localStorage, TaskOperation);
  expect(
    localStorage.getItem(
      'knowledge-structure:0.1.0:' + Scope.owner + ':legacy',
    ),
  ).toBe('broken');
});
it('does not replace an existing first body or key', async () => {
  const slot = await saveDocumentSlot(localStorage, CancelOperation);
  await expect(
    saveDocumentSlot(localStorage, CancelOperation),
  ).rejects.toThrow();
  expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([slot]);
});
it('blocks the ninth slot without eviction', async () => {
  for (let i = 0; i < 8; i++)
    await saveDocumentSlot(localStorage, {
      ...PolicyOperation,
      body: { ...PolicyOperation.body, idempotency_key: 'synthetic_' + i },
    });
  await expect(saveDocumentSlot(localStorage, TaskOperation)).rejects.toThrow(
    'document_storage_full',
  );
  expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(8);
});
it('blocks writes when an owned slot is corrupt', async () => {
  const slot = await saveDocumentSlot(localStorage, TaskOperation);
  localStorage.setItem(documentSlotKey(slot), '{');
  await expect(
    saveDocumentSlot(localStorage, PolicyOperation),
  ).rejects.toThrow();
});
it('blocks corrupt digest and does not clear the slot', async () => {
  const slot = await saveDocumentSlot(localStorage, TaskOperation);
  localStorage.setItem(
    documentSlotKey(slot),
    JSON.stringify({ ...slot, request_sha256: 'f'.repeat(64) }),
  );
  await expect(readDocumentSlots(localStorage, Scope.owner)).rejects.toThrow();
  expect(localStorage.getItem(documentSlotKey(slot))).not.toBeNull();
});
it('requires storage write and readback to succeed', async () => {
  const broken = {
    length: 0,
    key: () => null,
    getItem: () => null,
    setItem: () => {},
    removeItem: () => {},
    clear: () => {},
  } as Storage;
  await expect(saveDocumentSlot(broken, TaskOperation)).rejects.toThrow(
    'document_storage_failed',
  );
});
it('tracks accepted task until terminal and retains it after a cancel receipt', async () => {
  const task = await saveDocumentSlot(localStorage, TaskOperation);
  await acceptDocumentSlot(localStorage, task, Task);
  const cancel = await saveDocumentSlot(localStorage, CancelOperation);
  await acceptDocumentSlot(localStorage, cancel, CancelReceipt);
  expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([
    { ...task, task_id: Task.task_id },
  ]);
  await acceptDocumentSlot(localStorage, task, {
    ...Task,
    state: 'succeeded',
    phase: 'done',
    attempt_count: 1,
    completed_at: Task.updated_at,
    result_representation_id: Rep.representation_id,
  });
  expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([]);
});
it('resolves policy receipt independently of current policy', async () => {
  const slot = await saveDocumentSlot(localStorage, PolicyOperation);
  await acceptDocumentSlot(localStorage, slot, PolicyReceipt);
  expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(0);
});
it.each(['task', 'policy', 'cancel'] as const)(
  'retains a full %s rejection until explicit abandonment',
  async (kind) => {
    const rejection = Rejections[kind];
    const op = {
      kind,
      scope: {
        ...Scope,
        ...(kind === 'task'
          ? {
              source:
                Rejections.task.target.kind === 'original_document'
                  ? Rejections.task.target.source_id
                  : '',
            }
          : {}),
        ...(kind === 'cancel'
          ? {
              task:
                Rejections.cancel.target.kind === 'task'
                  ? Rejections.cancel.target.task_id
                  : '',
            }
          : {}),
      },
      body: rejection.original_request,
    } as DocumentOperation;
    const slot = await saveDocumentSlot(localStorage, op);
    await acceptDocumentSlot(localStorage, slot, rejection);
    expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(1);
    await abandonDocumentSlot(localStorage, slot, rejection);
    expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([]);
  },
);
it.each([401, 404, 412, 503])(
  'cannot abandon using an HTTP%s hint',
  async (status) => {
    const slot = await saveDocumentSlot(localStorage, TaskOperation);
    await expect(
      abandonDocumentSlot(localStorage, slot, {
        status,
        outcome_known: true,
        rejection_id: 'f'.repeat(32),
      } as any),
    ).rejects.toThrow();
    expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(1);
  },
);
it('includes cancel path task in identity', async () => {
  const slot = await saveDocumentSlot(localStorage, CancelOperation);
  const other = {
    ...CancelOperation,
    scope: { ...Scope, task: 'f'.repeat(32) },
  };
  await expect(
    checkDocumentOutcome(Scope, CancelReceipt, other),
  ).rejects.toThrow();
  expect(slot.request_sha256).not.toBe(
    await documentRequestDigest('cancel', other.scope, other.body),
  );
});
it('rejects wrong owner without clearing the durable request', async () => {
  const slot = await saveDocumentSlot(localStorage, TaskOperation);
  await expect(
    acceptDocumentSlot(localStorage, slot, {
      ...Task,
      owner_id: 'f'.repeat(32),
    }),
  ).rejects.toThrow();
  expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(1);
});
it('reads own title range without following aggregate next', () => {
  const heading = {
    ...Nodes.find((n) => n.payload.kind === 'heading')!,
    payload: Examples.positive.find(
      (v) => v.name === 'heading_own_range_excludes_section_body',
    )!.value,
  } as DocumentNode;
  expect(documentOwnQuery(heading)).toEqual({
    start_byte: heading.payload.own_reading_range!.start_byte,
    end_byte: heading.payload.own_reading_range!.end_byte,
  });
  expect(heading.payload.own_reading_range!.end_byte).toBeLessThan(
    heading.payload.reading_range.end_byte,
  );
  expect(
    documentOwnQuery(heading, heading.payload.own_reading_range!.end_byte),
  ).toBeNull();
});
it('does not request empty own text or null container text', () => {
  expect(documentOwnQuery(Nodes[0])).toBeNull();
  const heading = clone(
    Nodes.find((n) => n.payload.kind === 'heading')!,
  ) as any;
  heading.payload.own_reading_range.end_byte =
    heading.payload.own_reading_range.start_byte;
  expect(documentOwnQuery(heading)).toBeNull();
});
it('lets the server select safe UTF8 boundary before an explicit final own endpoint', () => {
  const n = clone(Nodes.find((n) => n.payload.kind === 'paragraph')!) as any;
  n.payload.own_reading_range = {
    start_byte: 5,
    end_byte: 80005,
    text_sha256: 'a'.repeat(64),
  };
  expect(documentOwnQuery(n)).toEqual({ start_byte: 5 });
  expect(documentOwnQuery(n, 65538)).toEqual({
    start_byte: 65538,
    end_byte: 80005,
  });
});
it('checks full own SHA only after contiguous complete bytes', async () => {
  const range = {
    start_byte: 2,
    end_byte: 7,
    text_sha256: await documentSha('a—b'),
  };
  const first = {
      start_byte: 2,
      end_byte: 3,
      text: 'a',
    } as DocumentContentWindow,
    next = { start_byte: 3, end_byte: 7, text: '—b' } as DocumentContentWindow;
  expect(await appendDocumentContent('', first, range)).toBe('a');
  expect(await appendDocumentContent('a', next, range)).toBe('a—b');
  await expect(appendDocumentContent('x', next, range)).rejects.toThrow();
  await expect(appendDocumentContent('', next, range)).rejects.toThrow();
});
it('rejects offsets splitting Unicode and preserves exact raw whitespace', () => {
  expect(documentSlice('a—b', 1, 4)).toBe('—');
  expect(() => documentSlice('a—b', 1, 2)).toThrow();
  expect(documentSlice(' \n\t', 0, 3)).toBe(' \n\t');
});
it('does not treat an original subset as matching another SHA', () => {
  expect(
    sameDocumentOriginal(Rep.original, {
      ...Rep.original,
      sha256: 'f'.repeat(64),
    } as any),
  ).toBe(false);
});

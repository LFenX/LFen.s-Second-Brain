import Examples from '../../../../docs/knowledge-structure-v1.examples.json';
import {
  validStructureData,
  validStructureQuery,
  validStructureAliases,
  structureAliasTarget,
} from '../knowledge-structure-schema';
import {
  structureDigest,
  structureRequestDigest,
  structureViewProjection,
  structureLogicalProjection,
  structureNodeProjection,
  structureRelationProjection,
  structureProducerProjection,
  checkStructureOutcome,
  checkStructureView,
  checkStructureNode,
  checkStructureContent,
  structureBytePosition,
  saveStructureSlot,
  readStructureSlots,
  acceptStructureSlot,
  abandonStructureSlot,
  structureSlotKey,
} from '../knowledge-structure';
import {
  Task,
  View,
  Node,
  Content,
  Relation,
  Scope,
  ViewScope,
  TaskOperation,
  PolicyOperation,
  CancelOperation,
  PolicyReceipt,
  CancelReceipt,
  rejection,
  running,
} from '@/pages/dataset/knowledge-catalogue/__tests__/structure-fixture';
beforeEach(() => localStorage.clear());
it.each(Examples.positive)(
  'accepts frozen example $name',
  ({ schema, value }) => expect(validStructureData(schema, value)).toBe(true),
);
it.each(Examples.negative)(
  'rejects frozen example $name',
  ({ schema, value }) => expect(validStructureData(schema, value)).toBe(false),
);
it.each(Object.entries(Examples.digest_vectors))(
  'matches frozen typed-LP projection %s',
  async (_name, vector) =>
    expect(await structureDigest(vector.projection)).toBe(vector.sha256),
);
it('constructs all eight frozen identity projections', async () => {
  expect(
    await structureRequestDigest('task', Scope, Task.original_request),
  ).toBe(Examples.digest_vectors.task.sha256);
  expect(
    await structureRequestDigest(
      'policy',
      Scope,
      PolicyReceipt.original_request,
    ),
  ).toBe(Examples.digest_vectors.policy_command.sha256);
  expect(
    await structureRequestDigest(
      'cancel',
      { ...Scope, task: Task.task_id },
      CancelReceipt.original_request,
    ),
  ).toBe(Examples.digest_vectors.cancel_command.sha256);
  for (const [value, expected] of [
    [
      structureProducerProjection(View.aliases),
      Examples.digest_vectors.producer.sha256,
    ],
    [structureViewProjection(View), View.view_id],
    [structureLogicalProjection(Scope.domain, Node), Node.logical_node_id],
    [structureNodeProjection(Node), Node.structure_node_id],
    [structureRelationProjection(Relation), Relation.relation_id],
  ] as const)
    expect(await structureDigest(value)).toBe(expected);
  await checkStructureView(ViewScope, View);
  await checkStructureNode(ViewScope, Node);
  await checkStructureContent(Node, Content.source_sha256, Content);
});
it('preserves original order and keys while canonicalizing the digest only', async () => {
  const body = {
    ...Task.original_request,
    members: [...Task.original_request.members].reverse(),
    idempotency_key: 'Changed_KEY',
  };
  expect(await structureRequestDigest('task', Scope, body)).toBe(
    Task.request_sha256,
  );
  const operation = { ...TaskOperation, body } as typeof TaskOperation,
    slot = await saveStructureSlot(localStorage, operation);
  expect(slot.body).toEqual(body);
  expect(slot.key).toBe('Changed_KEY');
  expect(body.members[0]).toEqual(Task.original_request.members.at(-1));
  await checkStructureOutcome(Scope, Task, {
    ...TaskOperation,
    body: { ...body, idempotency_key: Task.original_request.idempotency_key },
  });
});
it('validates duplicate members, exact aliases, UTF8 path limits and query bounds', () => {
  expect(
    validStructureData('TaskRequest', {
      ...Task.original_request,
      members: [
        Task.original_request.members[0],
        Task.original_request.members[0],
      ],
    }),
  ).toBe(false);
  expect(validStructureAliases([{ prefix: '@/', target: 'frontend/' }])).toBe(
    true,
  );
  expect(
    validStructureAliases([
      { prefix: '@/', target: '' },
      { prefix: '@/x/', target: '' },
    ]),
  ).toBe(false);
  for (const value of [
    'frontend',
    '../',
    '/a/',
    'C:/',
    'a\\b/',
    'a//b/',
    'a/./b/',
  ])
    expect(structureAliasTarget(value)).toBe(false);
  for (const value of ['', 'frontend/', '中文/'])
    expect(structureAliasTarget(value)).toBe(true);
  expect(validStructureQuery('content', { end_byte: 2 })).toBe(false);
  expect(
    validStructureQuery('content', { start_byte: 0, end_byte: 65537 }),
  ).toBe(false);
  expect(
    validStructureQuery('node', {
      source_id: Node.source_id,
      parent_id: Node.parent_id,
      limit: 20,
    }),
  ).toBe(true);
  expect(
    validStructureQuery('node', { source_id: Node.source_id, cursor: '' }),
  ).toBe(false);
  expect(
    validStructureQuery('relation', {
      node_id: Node.structure_node_id,
      limit: 21,
    }),
  ).toBe(false);
});
it('keeps BOM, CRLF and emoji byte positions separate from UTF16 columns', () => {
  const text = '\ufeffA😀\r\n中';
  expect(structureBytePosition(text, 3)).toEqual({ lineNumber: 1, column: 2 });
  expect(structureBytePosition(text, 8)).toEqual({ lineNumber: 1, column: 5 });
  expect(structureBytePosition(text, 10)).toEqual({ lineNumber: 2, column: 1 });
  expect(() => structureBytePosition(text, 6)).toThrow();
});
it('refuses content with wrong digest, source identity, incomplete progress or a changed node range', async () => {
  for (const patch of [
    { fragment_sha256: '0'.repeat(64) },
    { source_revision_id: '0'.repeat(32) },
    { start_byte: Content.start_byte + 1 },
    { complete: false, next_start_byte: Content.start_byte },
  ])
    await expect(
      checkStructureContent(Node, Content.source_sha256, {
        ...Content,
        ...patch,
      }),
    ).rejects.toThrow();
});
it.each([TaskOperation, PolicyOperation, CancelOperation])(
  'stores and isolates each $kind recovery kind',
  async (operation) => {
    const slot = await saveStructureSlot(localStorage, operation);
    expect((await readStructureSlots(localStorage, Scope.owner))[0]).toEqual(
      slot,
    );
    expect(await readStructureSlots(localStorage, '0'.repeat(32))).toEqual([]);
    expect(slot.target).toBe(
      operation.kind === 'cancel'
        ? operation.scope.task
        : operation.kind === 'task'
          ? operation.scope.project
          : Scope.domain,
    );
    await expect(
      abandonStructureSlot(
        localStorage,
        slot,
        await rejection(operation, { request_sha256: '0'.repeat(64) }),
      ),
    ).rejects.toThrow();
    expect(localStorage.getItem(structureSlotKey(slot))).not.toBeNull();
    await abandonStructureSlot(localStorage, slot, await rejection(operation));
    expect(await readStructureSlots(localStorage, Scope.owner)).toEqual([]);
  },
);
it('binds cancellation to its task without losing original task recovery', async () => {
  const taskSlot = await saveStructureSlot(localStorage, TaskOperation),
    cancelSlot = await saveStructureSlot(localStorage, CancelOperation);
  await acceptStructureSlot(localStorage, taskSlot, running());
  expect(
    (await readStructureSlots(localStorage, Scope.owner)).find(
      (slot) => slot.kind === 'task',
    )?.task_id,
  ).toBe(Task.task_id);
  await abandonStructureSlot(
    localStorage,
    cancelSlot,
    await rejection(CancelOperation),
  );
  expect(
    (await readStructureSlots(localStorage, Scope.owner)).map(
      (slot) => slot.kind,
    ),
  ).toEqual(['task']);
  await acceptStructureSlot(localStorage, taskSlot, Task);
  expect(await readStructureSlots(localStorage, Scope.owner)).toEqual([]);
});
it('blocks corrupt/full/oversized or unwritable storage without eviction', async () => {
  localStorage.setItem('old-protocol-slot', 'untouched');
  for (let index = 0; index < 8; index++)
    await saveStructureSlot(localStorage, {
      ...PolicyOperation,
      body: { ...PolicyOperation.body, idempotency_key: 'slot_' + index },
    });
  await expect(saveStructureSlot(localStorage, TaskOperation)).rejects.toThrow(
    'structure_storage_full',
  );
  expect(localStorage.length).toBe(9);
  expect(localStorage.getItem('old-protocol-slot')).toBe('untouched');
  localStorage.setItem(
    'knowledge-structure:0.1.0:' + Scope.owner + ':corrupt',
    '{}',
  );
  await expect(
    saveStructureSlot(localStorage, TaskOperation),
  ).rejects.toThrow();
  localStorage.clear();
  const storage = {
    length: 0,
    key: () => null,
    getItem: () => null,
    setItem: () => {
      throw Error('quota');
    },
  } as unknown as Storage;
  await expect(saveStructureSlot(storage, TaskOperation)).rejects.toThrow(
    'quota',
  );
  const long = {
    ...TaskOperation.body,
    aliases: Array.from({ length: 8 }, (_, index) => ({
      prefix: 'alias' + index + '/',
      target: '中'.repeat(169) + '/',
    })),
  };
  expect(new TextEncoder().encode(JSON.stringify(long)).length).toBeGreaterThan(
    4096,
  );
});

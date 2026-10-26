import request from '@/utils/next-request';
import service from '../knowledge-structure-service';
import foundation from '../knowledge-foundation-service';
import lifecycle from '../knowledge-lifecycle-service';
import batch from '../knowledge-batch-service';
import {
  Scope,
  ProjectScope,
  ViewScope,
  Task,
  View,
  Node,
  Content,
  Policy,
  TaskOperation,
  CancelOperation,
  rejection,
  wire,
  manifestEntry,
  Entry,
  sample,
} from '@/pages/dataset/knowledge-catalogue/__tests__/structure-fixture';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock;
type Wire = { headers: Record<string, string>; data: Record<string, unknown> };
beforeEach(() => Http.mockReset());
afterEach(() => jest.restoreAllMocks());
it('recognizes fixed task and command rejection unions with code 0', async () => {
  const task = await rejection(TaskOperation),
    cancel = await rejection(CancelOperation);
  Http.mockResolvedValueOnce(wire(task)).mockResolvedValueOnce(wire(cancel));
  expect((await service.taskByKey(Scope, task.idempotency_key)).data).toEqual(
    task,
  );
  expect(
    (await service.commandByKey(Scope, 'cancel', cancel.idempotency_key)).data,
  ).toEqual(cancel);
  Http.mockResolvedValue(wire({ ...task, request_sha256: '0'.repeat(64) }));
  await expect(
    service.taskByKey(Scope, task.idempotency_key),
  ).rejects.toMatchObject({ code: 'structure_invalid_response' });
});
it('rejects cross-owner/domain/project/view/source and mismatched digest without exposing data', async () => {
  for (const patch of [
    { owner_id: '0'.repeat(32) },
    { domain_id: '0'.repeat(32) },
    { request_sha256: '0'.repeat(64) },
    { producer_sha256: '0'.repeat(64) },
    { project_id: '0'.repeat(32) },
  ]) {
    Http.mockResolvedValue(wire({ ...Task, ...patch }));
    await expect(service.task(Scope, Task.task_id)).rejects.toMatchObject({
      code: 'structure_invalid_response',
    });
  }
  Http.mockResolvedValue(wire({ ...View, view_id: '0'.repeat(64) }));
  await expect(service.view(ViewScope)).rejects.toThrow();
  Http.mockResolvedValue(wire({ ...Node, source_id: '0'.repeat(32) }));
  await expect(
    service.node(ViewScope, Node.structure_node_id),
  ).rejects.toThrow();
});
it('requires no-store, both headers, matching request ID, schema and budgets', async () => {
  for (const mutate of [
    (value: Wire) => Reflect.deleteProperty(value.headers, 'cache-control'),
    (value: Wire) =>
      (value.headers['x-knowledge-structure-contract-version'] = '9'),
    (value: Wire) => (value.headers['x-request-id'] = '2'.repeat(32)),
    (value: Wire) => Object.assign(value.data, { extra: true }),
  ]) {
    const value = wire(Policy);
    mutate(value);
    Http.mockResolvedValue(value);
    await expect(service.policy(Scope)).rejects.toThrow();
  }
  const value = wire({ ...Policy, padding: 'x'.repeat(262144) });
  Http.mockResolvedValue(value);
  await expect(service.policy(Scope)).rejects.toThrow();
});
it('keeps native 401, version 412, 404 and unknown transport distinct without interpreting hints as proof', async () => {
  Http.mockRejectedValueOnce({ response: { status: 401, data: {} } });
  await expect(service.policy(Scope)).rejects.toMatchObject({ status: 401 });
  for (const [code, status] of [
    ['not_found', 404],
    ['contract_mismatch', 412],
    ['policy_disabled', 409],
    ['capacity_exceeded', 429],
    ['unavailable', 503],
  ] as const) {
    const response = wire(null, status);
    response.data = {
      ...response.data,
      code,
      message: '',
      outcome_known: true,
      rejection_id: 'f'.repeat(32),
    } as never;
    Http.mockRejectedValueOnce({ response });
    await expect(service.taskByKey(Scope, 'same-key')).rejects.toMatchObject({
      status,
      code,
      outcomeKnown: true,
      rejectionId: 'f'.repeat(32),
    });
  }
  Http.mockRejectedValueOnce(Error('timeout'));
  await expect(service.taskByKey(Scope, 'same-key')).rejects.toMatchObject({
    status: 0,
    code: 'structure_result_unknown',
  });
});
it('does not retry or shrink invalid explicit byte windows and checks fragment integrity', async () => {
  await expect(
    service.content(ViewScope, Node, Content.source_sha256, { end_byte: 155 }),
  ).rejects.toMatchObject({ status: 400 });
  expect(Http).not.toHaveBeenCalled();
  Http.mockResolvedValueOnce(wire(Content));
  await service.content(ViewScope, Node, Content.source_sha256, {
    start_byte: 80,
    end_byte: 155,
  });
  expect(Http.mock.calls[0][0].params).toEqual({
    start_byte: 80,
    end_byte: 155,
  });
  Http.mockResolvedValueOnce(
    wire({ ...Content, fragment_sha256: '0'.repeat(64) }),
  );
  await expect(
    service.content(ViewScope, Node, Content.source_sha256),
  ).rejects.toThrow();
  expect(Http).toHaveBeenCalledTimes(2);
});
function prepareFixture() {
  jest.spyOn(foundation, 'domain').mockResolvedValue({
    domain_id: Scope.domain,
    owner_id: Scope.owner,
    policy: { version: 1 },
  } as never);
  jest.spyOn(batch, 'head').mockResolvedValue({
    data: {
      domain_id: Scope.domain,
      project_id: ProjectScope.project,
      manifest_id: Task.original_request.manifest_id,
    },
    httpStatus: 200,
  });
  jest
    .spyOn(service, 'policy')
    .mockResolvedValue({ data: Policy, httpStatus: 200 });
  jest
    .spyOn(service, 'head')
    .mockResolvedValue({ data: sample('head-none'), httpStatus: 200 });
  jest.spyOn(lifecycle, 'detail').mockResolvedValue({
    data: {
      owner_id: Scope.owner,
      domain_id: Scope.domain,
      source_id: Entry.source_id,
      representation_kind: 'text',
      current: { ...Entry, state: 'active', lifecycle_version: 5 },
    },
    httpStatus: 200,
  } as never);
  jest.spyOn(foundation, 'source').mockResolvedValue({
    domain_id: Scope.domain,
    source_id: Entry.source_id,
    source_revision_id: Entry.source_revision_id,
    root_node_id: Entry.source_node_id,
    sha256: Entry.sha256,
    byte_length: Entry.byte_length,
    relative_path: Entry.relative_path,
    ...Entry.classification,
  } as never);
}
it('takes generation only from exact active lifecycle details and binds the fixed C1 root', async () => {
  prepareFixture();
  const result = await service.prepare(
    ProjectScope,
    Task.original_request.manifest_id,
    [manifestEntry()],
    [{ prefix: '@/', target: 'frontend/' }],
  );
  expect(result.request.members).toEqual([
    {
      source_id: Entry.source_id,
      source_revision_id: Entry.source_revision_id,
      lifecycle_version: 5,
      parser: 'tsx',
    },
  ]);
  expect(result.request.aliases).toEqual([
    { prefix: '@/', target: 'frontend/' },
  ]);
  expect(result.request.expected_active_view_id).toBeNull();
  expect(result.request).not.toHaveProperty('idempotency_key');
  expect(result.request).not.toHaveProperty('confirmed');
  const detail = await lifecycle.detail({ ...Scope, source: Entry.source_id });
  (lifecycle.detail as jest.Mock).mockResolvedValue({
    ...detail,
    data: {
      ...detail.data,
      current: { ...detail.data.current, relative_path: 'other.tsx' },
    },
  });
  await expect(
    service.prepare(
      ProjectScope,
      Task.original_request.manifest_id,
      [manifestEntry()],
      [],
    ),
  ).rejects.toMatchObject({ code: 'revision_changed' });
});
it('blocks stale manifest, archived/nontext source, parser suffix and size overflow before POST', async () => {
  prepareFixture();
  (batch.head as jest.Mock).mockResolvedValue({
    data: { manifest_id: '0'.repeat(32) },
  });
  await expect(
    service.prepare(
      ProjectScope,
      Task.original_request.manifest_id,
      [manifestEntry()],
      [],
    ),
  ).rejects.toMatchObject({ code: 'manifest_changed' });
  for (const entry of [
    { ...manifestEntry(), relative_path: 'FILE.TS' },
    { ...manifestEntry(), byte_length: 2097153 },
  ])
    await expect(
      service.prepare(
        ProjectScope,
        Task.original_request.manifest_id,
        [entry],
        [],
      ),
    ).rejects.toMatchObject({ status: 400 });
  expect(Http).not.toHaveBeenCalled();
});

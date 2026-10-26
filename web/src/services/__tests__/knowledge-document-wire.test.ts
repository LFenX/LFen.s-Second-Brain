import request from '@/utils/next-request';
import service from '../knowledge-document-service';
import {
  Scope,
  SourceScope,
  ViewScope,
  Task,
  Rep,
  Root,
  Nodes,
  Content,
  Caps,
  Policy,
  PolicyReceipt,
  CancelReceipt,
  Head,
  wire,
  clone,
} from '@/pages/dataset/knowledge-catalogue/__tests__/document-fixture';
import Examples from '../../../../docs/knowledge-document-structure-v1.examples.json';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock,
  Base = '/api/v1/datasets/' + Scope.domain + '/knowledge',
  ViewPath = Base + '/document-representations/' + Rep.representation_id;
const ContentNode = Nodes.find(
  (n) => n.document_node_id === Content.document_node_id,
)!;
const Page = {
  representation_id: Rep.representation_id,
  source_id: Rep.original.source_id,
  source_revision_id: Rep.original.source_revision_id,
  parent_id: Root.document_node_id,
  items: Nodes.filter((n) => n.parent_id === Root.document_node_id),
  next_cursor: null,
};
const Operations = [
  {
    name: 'DC01',
    path: '/api/v1/knowledge/document-structure-contract',
    value: Caps,
    run: () => service.contract(),
  },
  {
    name: 'DC02',
    path: Base + '/document-structure-policy',
    value: Policy,
    run: () => service.policy(Scope),
  },
  {
    name: 'DC03',
    post: true,
    path: Base + '/document-structure-policy/changes',
    value: PolicyReceipt,
    run: () => service.changePolicy(Scope, PolicyReceipt.original_request),
  },
  {
    name: 'DC04',
    post: true,
    path: Base + '/document-tasks',
    value: Task,
    run: () => service.createTask(SourceScope, Task.original_request),
  },
  {
    name: 'DC05',
    path: Base + '/document-operations/by-key',
    value: Task,
    params: {
      kind: 'task',
      idempotency_key: Task.original_request.idempotency_key,
    },
    run: () =>
      service.byKey(Scope, {
        kind: 'task',
        idempotency_key: Task.original_request.idempotency_key,
      }),
  },
  {
    name: 'DC06',
    path: Base + '/document-tasks/' + Task.task_id,
    value: Task,
    run: () => service.task(Scope, Task.task_id),
  },
  {
    name: 'DC07',
    post: true,
    path: Base + '/document-tasks/' + CancelReceipt.target.task_id + '/cancel',
    value: CancelReceipt,
    run: () =>
      service.cancel(
        { ...Scope, task: CancelReceipt.target.task_id },
        CancelReceipt.original_request,
      ),
  },
  {
    name: 'DC08',
    path: Base + '/sources/' + Head.source_id + '/document-head',
    value: Head,
    run: () => service.head({ ...Scope, source: Head.source_id }),
  },
  {
    name: 'DC09',
    path: ViewPath,
    value: Rep,
    run: () => service.representation(ViewScope),
  },
  {
    name: 'DC10',
    path: ViewPath + '/nodes',
    value: Page,
    params: { parent_id: Root.document_node_id, limit: 20 },
    run: () =>
      service.nodes(ViewScope, Rep, Root, {
        parent_id: Root.document_node_id,
        limit: 20,
      }),
  },
  {
    name: 'DC11',
    path: ViewPath + '/nodes/' + Root.document_node_id,
    value: Root,
    run: () => service.node(ViewScope, Rep, Root.document_node_id),
  },
  {
    name: 'DC12',
    path: ViewPath + '/nodes/' + ContentNode.document_node_id + '/content',
    value: Content,
    params: {},
    run: () => service.content(ViewScope, Rep, ContentNode),
  },
];
beforeEach(() => Http.mockReset());
it.each(Operations)('uses formal $name', async (op) => {
  Http.mockResolvedValue(wire(op.value, op.post ? 201 : 200));
  const result = await op.run();
  expect(result.data).toEqual(op.value);
  const config = Http.mock.calls[0][0];
  expect(config).toMatchObject({
    url: op.path,
    method: op.post ? 'post' : 'get',
    timeout: 25000,
    headers: {
      'X-Knowledge-Contract-Version': '1.0.0',
      'X-Knowledge-Document-Structure-Contract-Version': '0.1.0',
    },
  });
  expect(config.params).toEqual(op.params);
  expect(config.headers).not.toHaveProperty(
    'X-Knowledge-Structure-Contract-Version',
  );
  expect(config.headers).not.toHaveProperty(
    'X-Knowledge-Source-Contract-Version',
  );
  if (op.post) expect(JSON.parse(config.data).confirmed).toBe(true);
  expect(
    config.transformResponse[0](JSON.stringify(wire(op.value).data)),
  ).toEqual(wire(op.value).data);
});
it.each(Operations.filter((v) => v.post))(
  'accepts replay200 and refuses202 $name',
  async (op) => {
    Http.mockResolvedValueOnce(wire(op.value, 200)).mockResolvedValueOnce(
      wire(op.value, 202),
    );
    expect((await op.run()).httpStatus).toBe(200);
    await expect(op.run()).rejects.toMatchObject({ status: 502 });
  },
);
it('rejects a foreign representation without switching to current head', async () => {
  Http.mockResolvedValue(wire(Rep));
  await expect(
    service.representation({ ...ViewScope, sha: 'f'.repeat(64) }),
  ).rejects.toMatchObject({ status: 502 });
  expect(Http).toHaveBeenCalledTimes(1);
});
it('refuses clipped explicit content endpoints', async () => {
  Http.mockResolvedValue(wire(Content));
  await expect(
    service.content(ViewScope, Rep, ContentNode, {
      start_byte: Content.start_byte,
      end_byte: Content.end_byte - 1,
    }),
  ).rejects.toThrow();
});
it('preserves unknown results for missing responses', async () => {
  Http.mockRejectedValue({});
  await expect(
    service.createTask(SourceScope, Task.original_request),
  ).rejects.toMatchObject({ status: 0, code: 'document_result_unknown' });
});
it('requires bounded versioned401 envelope and rejects raw native401', async () => {
  const value = Examples.positive.find((v) => v.schema === 'Error401')!
    .value as any;
  const r = wire(null, 401);
  Http.mockRejectedValueOnce({
    response: {
      ...r,
      data: value,
      headers: { ...r.headers, 'x-request-id': value.request_id },
    },
  });
  await expect(service.policy(Scope)).rejects.toMatchObject({
    status: 401,
    code: 'unauthorized',
  });
  Http.mockRejectedValueOnce({
    response: { status: 401, data: { message: 'login' } },
  });
  await expect(service.policy(Scope)).rejects.toMatchObject({ status: 502 });
});
it('rejects wrong or missing response headers', async () => {
  const r = wire(Policy);
  delete (r.headers as any)['cache-control'];
  Http.mockResolvedValue(r);
  await expect(service.policy(Scope)).rejects.toThrow();
});
it('rejects changed producer without accepting a guessed profile', async () => {
  const r = clone(Caps);
  r.profiles[0].producer_sha256 = 'f'.repeat(64);
  Http.mockResolvedValue(wire(r));
  await expect(service.contract()).rejects.toThrow();
});
it('rejects unknown query or over-budget range before transport', async () => {
  await expect(
    service.content(ViewScope, Rep, ContentNode, {
      end_byte: 70000,
      start_byte: 0,
    }),
  ).rejects.toMatchObject({ status: 400 });
  expect(Http).not.toHaveBeenCalled();
});

import request from '@/utils/next-request';
import service from '../knowledge-structure-service';
import {
  Scope,
  ProjectScope,
  ViewScope,
  Task,
  View,
  Node,
  Content,
  Relation,
  Policy,
  PolicyReceipt,
  CancelReceipt,
  Head,
  Capabilities,
  wire,
} from '@/pages/dataset/knowledge-catalogue/__tests__/structure-fixture';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock,
  Base = '/api/v1/datasets/' + Scope.domain + '/knowledge',
  ViewPath = Base + '/structure-views/' + View.view_id;
beforeEach(() => Http.mockReset());
const Operations = [
  {
    name: 'capabilities',
    path: '/api/v1/knowledge/structure-contract',
    value: Capabilities,
    run: () => service.contract(),
  },
  {
    name: 'policy',
    path: Base + '/structure-policy',
    value: Policy,
    run: () => service.policy(Scope),
  },
  {
    name: 'policy change',
    post: true,
    path: Base + '/structure-policy/changes',
    value: PolicyReceipt,
    run: () => service.changePolicy(Scope, PolicyReceipt.original_request),
  },
  {
    name: 'command by key',
    path: Base + '/structure-commands/by-key',
    value: CancelReceipt,
    params: { kind: 'cancel', idempotency_key: CancelReceipt.idempotency_key },
    run: () =>
      service.commandByKey(Scope, 'cancel', CancelReceipt.idempotency_key),
  },
  {
    name: 'create task',
    post: true,
    path: Base + '/structure-tasks',
    value: Task,
    run: () => service.createTask(ProjectScope, Task.original_request),
  },
  {
    name: 'task page',
    path: Base + '/structure-tasks',
    value: {
      domain_id: Scope.domain,
      project_id: Task.project_id,
      captured_at: Task.created_at,
      items: [Task],
      next_cursor: null,
    },
    params: { project_id: Task.project_id, limit: 20 },
    run: () =>
      service.tasks(ProjectScope, { project_id: Task.project_id, limit: 20 }),
  },
  {
    name: 'task by key',
    path: Base + '/structure-tasks/by-key',
    value: Task,
    params: { idempotency_key: Task.original_request.idempotency_key },
    run: () => service.taskByKey(Scope, Task.original_request.idempotency_key),
  },
  {
    name: 'task',
    path: Base + '/structure-tasks/' + Task.task_id,
    value: Task,
    run: () => service.task(Scope, Task.task_id),
  },
  {
    name: 'cancel',
    post: true,
    path: Base + '/structure-tasks/' + Task.task_id + '/cancel',
    value: CancelReceipt,
    run: () =>
      service.cancel(
        { ...Scope, task: Task.task_id },
        CancelReceipt.original_request,
      ),
  },
  {
    name: 'head',
    path: Base + '/projects/' + Task.project_id + '/structure-head',
    value: Head,
    run: () => service.head(ProjectScope),
  },
  {
    name: 'view',
    path: ViewPath,
    value: View,
    run: () => service.view(ViewScope),
  },
  {
    name: 'nodes',
    path: ViewPath + '/nodes',
    value: {
      view_id: View.view_id,
      source_id: Node.source_id,
      parent_id: Node.parent_id,
      items: [Node],
      next_cursor: null,
    },
    params: { source_id: Node.source_id, parent_id: Node.parent_id, limit: 20 },
    run: () =>
      service.nodes(ViewScope, {
        source_id: Node.source_id,
        parent_id: Node.parent_id!,
        limit: 20,
      }),
  },
  {
    name: 'node',
    path: ViewPath + '/nodes/' + Node.structure_node_id,
    value: Node,
    run: () => service.node(ViewScope, Node.structure_node_id),
  },
  {
    name: 'content',
    path: ViewPath + '/nodes/' + Node.structure_node_id + '/content',
    value: Content,
    params: {},
    run: () => service.content(ViewScope, Node, Content.source_sha256),
  },
  {
    name: 'relations',
    path: ViewPath + '/relations',
    value: {
      view_id: View.view_id,
      node_id: Relation.from_node_id,
      direction: 'out',
      kind: Relation.kind,
      items: [Relation],
      next_cursor: null,
    },
    params: {
      node_id: Relation.from_node_id,
      direction: 'out',
      kind: Relation.kind,
      limit: 20,
    },
    run: () =>
      service.relations(ViewScope, {
        node_id: Relation.from_node_id,
        direction: 'out',
        kind: Relation.kind,
        limit: 20,
      }),
  },
];
it.each(Operations)(
  'uses the frozen HTTP operation $name',
  async (operation) => {
    Http.mockResolvedValue(wire(operation.value, operation.post ? 201 : 200));
    const result = await operation.run();
    expect(result.data).toEqual(operation.value);
    const config = Http.mock.calls[0][0];
    expect(config.url).toBe(operation.path);
    expect(config.method).toBe(operation.post ? 'post' : 'get');
    expect(config.timeout).toBe(25000);
    expect(config.headers).toMatchObject({
      'X-Knowledge-Contract-Version': '1.0.0',
      'X-Knowledge-Structure-Contract-Version': '0.1.0',
    });
    expect(config.params).toEqual(operation.params);
    if (operation.post)
      expect(JSON.parse(config.data)).toHaveProperty('confirmed', true);
  },
);
it.each(Operations.filter((value) => value.post))(
  'accepts replay 200 and refuses 202 for $name',
  async (operation) => {
    Http.mockResolvedValueOnce(
      wire(operation.value, 200),
    ).mockResolvedValueOnce(wire(operation.value, 202));
    expect((await operation.run()).httpStatus).toBe(200);
    await expect(operation.run()).rejects.toMatchObject({
      code: 'structure_invalid_response',
    });
  },
);

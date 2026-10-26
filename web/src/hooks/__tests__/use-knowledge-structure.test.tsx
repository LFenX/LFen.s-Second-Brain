import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service from '@/services/knowledge-structure-service';
import { getAuthorization } from '@/utils/authorization-util';
import { getKnowledgeSessionScope } from '../use-knowledge-foundation';
import {
  KnowledgeStructureKeys,
  scopedStructure,
  useStructureNodes,
  useStructureTask,
  useStructureActions,
} from '../use-knowledge-structure';
import {
  Scope,
  ProjectScope,
  ViewScope,
  Node,
  Task,
  View,
  running,
} from '@/pages/dataset/knowledge-catalogue/__tests__/structure-fixture';
type ReactNode = import('react').ReactNode;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-structure-session'),
}));
jest.mock('@/services/knowledge-structure-service', () => {
  const actual = jest.requireActual('@/services/knowledge-structure-service');
  return {
    ...actual,
    __esModule: true,
    default: { ...actual.default, nodes: jest.fn(), task: jest.fn() },
  };
});
function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    client,
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  };
}
beforeEach(() => {
  jest.clearAllMocks();
  (getAuthorization as jest.Mock).mockReturnValue(
    'synthetic-structure-session',
  );
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    value: 'visible',
  });
});
afterEach(() => jest.useRealTimers());
it('keys every fixed identity and paged filter independently from the current head', () => {
  const session = getKnowledgeSessionScope(),
    query = { source_id: Node.source_id, limit: 20 };
  const values = [
    KnowledgeStructureKeys.nodes(session, ViewScope, query),
    KnowledgeStructureKeys.nodes(session, ViewScope, {
      ...query,
      parent_id: Node.parent_id!,
    }),
    KnowledgeStructureKeys.nodes(session, ViewScope, { ...query, limit: 1 }),
    KnowledgeStructureKeys.nodes(session, ViewScope, {
      ...query,
      cursor: 'f'.repeat(32),
    }),
    KnowledgeStructureKeys.nodes(
      session,
      { ...ViewScope, owner: 'f'.repeat(32) },
      query,
    ),
    KnowledgeStructureKeys.nodes(
      session,
      { ...ViewScope, view: 'f'.repeat(64) },
      query,
    ),
    KnowledgeStructureKeys.nodes(session + 1, ViewScope, query),
    KnowledgeStructureKeys.head(session, ProjectScope),
  ];
  expect(new Set(values.map((value) => JSON.stringify(value))).size).toBe(
    values.length,
  );
});
it('follows a nonempty cursor after a short page and keeps exact parent/source/limit', async () => {
  const next = {
    ...Node,
    structure_node_id: 'f'.repeat(64),
    start_byte: Node.start_byte + 1,
  };
  (service.nodes as jest.Mock).mockImplementation(async (_scope, query) => ({
    data: {
      view_id: View.view_id,
      source_id: Node.source_id,
      parent_id: Node.parent_id,
      items: [query.cursor ? next : Node],
      next_cursor: query.cursor ? null : 'e'.repeat(32),
    },
  }));
  const { result } = renderHook(
    () => ({
      ...useStructureNodes(ViewScope, {
        source_id: Node.source_id,
        parent_id: Node.parent_id!,
        limit: 20,
      }),
    }),
    wrapper(),
  );
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(result.current.hasNextPage).toBe(true);
  await act(async () => {
    await result.current.fetchNextPage();
  });
  expect(service.nodes).toHaveBeenLastCalledWith(
    ViewScope,
    {
      source_id: Node.source_id,
      parent_id: Node.parent_id,
      limit: 20,
      cursor: 'e'.repeat(32),
    },
    expect.any(AbortSignal),
  );
  await waitFor(() => expect(result.current.data?.pages).toHaveLength(2));
});
it('rejects repeated nodes/cursors rather than inventing page completion', async () => {
  (service.nodes as jest.Mock).mockResolvedValue({
    data: {
      view_id: View.view_id,
      source_id: Node.source_id,
      parent_id: Node.parent_id,
      items: [Node],
      next_cursor: 'e'.repeat(32),
    },
  });
  const { result } = renderHook(
    () => ({
      ...useStructureNodes(ViewScope, {
        source_id: Node.source_id,
        parent_id: Node.parent_id!,
      }),
    }),
    wrapper(),
  );
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.isError).toBe(true));
});
it('drops late responses from an old session before exposing them', async () => {
  const session = getKnowledgeSessionScope();
  let complete!: (value: string) => void;
  const pending = scopedStructure(
    session,
    () =>
      new Promise<string>((resolve) => {
        complete = resolve;
      }),
  );
  (getAuthorization as jest.Mock).mockReturnValue('new-synthetic-session');
  complete('private-old-owner');
  await expect(pending).rejects.toMatchObject({ status: 401 });
});
it('polls no faster than two seconds, stops hidden and unmounted, then stops after 25 seconds', async () => {
  jest.useFakeTimers();
  (service.task as jest.Mock).mockResolvedValue({
    data: running(),
    httpStatus: 200,
  });
  const { result, unmount } = renderHook(
    () => useStructureTask(Scope, Task.task_id),
    wrapper(),
  );
  await act(async () => {
    await jest.advanceTimersByTimeAsync(1);
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  await act(async () => {
    await jest.advanceTimersByTimeAsync(1900);
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  await act(async () => {
    await jest.advanceTimersByTimeAsync(100);
  });
  expect(service.task).toHaveBeenCalledTimes(2);
  await act(async () => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'hidden',
    });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  const hidden = (service.task as jest.Mock).mock.calls.length;
  await act(async () => {
    await jest.advanceTimersByTimeAsync(5000);
  });
  expect(service.task).toHaveBeenCalledTimes(hidden);
  await act(async () => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await act(async () => {
    await jest.advanceTimersByTimeAsync(26000);
  });
  expect(result.current.waiting).toBe(false);
  const waited = (service.task as jest.Mock).mock.calls.length;
  await act(async () => {
    await jest.advanceTimersByTimeAsync(4000);
  });
  expect(service.task).toHaveBeenCalledTimes(waited);
  unmount();
  await act(async () => {
    await jest.advanceTimersByTimeAsync(4000);
  });
  expect(service.task).toHaveBeenCalledTimes(waited);
});
it('stops polling at terminal state and preserves fixed view cache on head refresh', async () => {
  jest.useFakeTimers();
  (service.task as jest.Mock).mockResolvedValue({
    data: Task,
    httpStatus: 200,
  });
  const context = wrapper(),
    { result, unmount } = renderHook(
      () => useStructureTask(Scope, Task.task_id),
      context,
    );
  await act(async () => {
    await jest.advanceTimersByTimeAsync(2);
  });
  expect(result.current.data?.state).toBe('succeeded');
  await act(async () => {
    await jest.advanceTimersByTimeAsync(6000);
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  unmount();
  jest.useRealTimers();
  const session = getKnowledgeSessionScope();
  context.client.setQueryData(
    KnowledgeStructureKeys.view(session, ViewScope),
    View,
  );
  const action = renderHook(() => useStructureActions(Scope), context);
  await act(async () => {
    await action.result.current.refresh(ProjectScope.project);
  });
  expect(
    context.client.getQueryData(
      KnowledgeStructureKeys.view(session, ViewScope),
    ),
  ).toEqual(View);
});

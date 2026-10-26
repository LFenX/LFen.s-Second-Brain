import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service from '@/services/knowledge-document-service';
import { getAuthorization } from '@/utils/authorization-util';
import { getKnowledgeSessionScope } from '../use-knowledge-foundation';
import {
  KnowledgeDocumentKeys,
  scopedDocument,
  useDocumentChildren,
  useDocumentTask,
  useDocumentActions,
} from '../use-knowledge-document';
import {
  Scope,
  ViewScope,
  Task,
  Rep,
  Root,
  Nodes,
} from '@/pages/dataset/knowledge-catalogue/__tests__/document-fixture';
type ReactNode = import('react').ReactNode;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'document-session'),
}));
jest.mock('@/services/knowledge-document-service', () => {
  const actual = jest.requireActual('@/services/knowledge-document-service');
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
  (getAuthorization as jest.Mock).mockReturnValue('document-session');
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    value: 'visible',
  });
});
afterEach(() => jest.useRealTimers());
it('aborts actions on unmount and discards even an adapter that resolves late', async () => {
  let resolve: (v: string) => void = () => {},
    signal: AbortSignal | undefined;
  const { result, unmount } = renderHook(
    () => useDocumentActions(Scope),
    wrapper(),
  );
  const pending = result.current.run((s) => {
    signal = s;
    return new Promise<string>((r) => {
      resolve = r;
    });
  });
  const rejected = expect(pending).rejects.toMatchObject({
    code: 'document_scope_closed',
  });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  unmount();
  expect(signal?.aborted).toBe(true);
  resolve('private result');
  await rejected;
  await expect(
    result.current.run(async () => 'new work'),
  ).rejects.toMatchObject({ code: 'document_scope_closed' });
});
it('isolates fixed identity, owner, session, parent, limit and byte window keys', () => {
  const session = getKnowledgeSessionScope();
  const keys = [
    KnowledgeDocumentKeys.view(session, ViewScope),
    KnowledgeDocumentKeys.view(session + 1, ViewScope),
    KnowledgeDocumentKeys.view(session, {
      ...ViewScope,
      owner: 'f'.repeat(32),
    }),
    KnowledgeDocumentKeys.view(session, {
      ...ViewScope,
      revision: 'f'.repeat(32),
    }),
    KnowledgeDocumentKeys.view(session, { ...ViewScope, sha: 'f'.repeat(64) }),
    KnowledgeDocumentKeys.children(
      session,
      ViewScope,
      Root.document_node_id,
      20,
    ),
    KnowledgeDocumentKeys.children(
      session,
      ViewScope,
      Root.document_node_id,
      1,
    ),
    KnowledgeDocumentKeys.content(session, ViewScope, Root, { start_byte: 1 }),
    KnowledgeDocumentKeys.content(session, ViewScope, Root, { start_byte: 2 }),
  ];
  expect(new Set(keys.map((key) => JSON.stringify(key))).size).toBe(
    keys.length,
  );
});
it('rejects a delayed result after session changes', async () => {
  const session = getKnowledgeSessionScope();
  let resolve: (v: string) => void = () => {};
  const result = scopedDocument(
    session,
    () =>
      new Promise<string>((r) => {
        resolve = r;
      }),
  );
  (getAuthorization as jest.Mock).mockReturnValue('another-owner');
  resolve('private');
  await expect(result).rejects.toMatchObject({ status: 401 });
});
it('follows short child pages only on demand and retains source scope', async () => {
  const children = Nodes.filter(
    (n) => n.parent_id === Nodes[1].document_node_id,
  );
  (service.nodes as jest.Mock).mockImplementation(
    async (_scope, _rep, _parent, q) => ({
      data: {
        items: [children[q.cursor ? 1 : 0]],
        next_cursor: q.cursor ? null : 'e'.repeat(32),
      },
    }),
  );
  const { result } = renderHook(
    () => ({ ...useDocumentChildren(ViewScope, Rep, Nodes[1]) }),
    wrapper(),
  );
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(service.nodes).toHaveBeenCalledTimes(1);
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.data?.pages).toHaveLength(2));
  expect((service.nodes as jest.Mock).mock.calls[1][3]).toEqual({
    parent_id: Nodes[1].document_node_id,
    limit: 20,
    cursor: 'e'.repeat(32),
  });
});
it('polls no faster than2seconds, stops at25seconds and waits for explicit resume', async () => {
  jest.useFakeTimers();
  (service.task as jest.Mock).mockResolvedValue({ data: Task });
  const { result, unmount } = renderHook(
    () => ({ ...useDocumentTask(Scope, Task.task_id) }),
    wrapper(),
  );
  await act(async () => {
    await Promise.resolve();
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  await act(async () => {
    jest.advanceTimersByTime(1999);
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  await act(async () => {
    jest.advanceTimersByTime(1);
  });
  expect(service.task).toHaveBeenCalledTimes(2);
  await act(async () => {
    jest.advanceTimersByTime(23000);
  });
  expect(result.current.waiting).toBe(false);
  const count = (service.task as jest.Mock).mock.calls.length;
  await act(async () => {
    jest.advanceTimersByTime(60000);
  });
  expect(service.task).toHaveBeenCalledTimes(count);
  unmount();
});
it('stops hidden task polling and aborts its request', async () => {
  jest.useFakeTimers();
  let signal: AbortSignal | undefined;
  (service.task as jest.Mock).mockImplementation((_s, _t, s) => {
    signal = s;
    return new Promise(() => {});
  });
  const { unmount } = renderHook(
    () => useDocumentTask(Scope, Task.task_id),
    wrapper(),
  );
  await act(async () => {
    await Promise.resolve();
  });
  act(() => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'hidden',
    });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await act(async () => {
    jest.advanceTimersByTime(10000);
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  expect(signal?.aborted).toBe(true);
  unmount();
});
it('does not poll terminal tasks', async () => {
  jest.useFakeTimers();
  (service.task as jest.Mock).mockResolvedValue({
    data: {
      ...Task,
      state: 'cancelled',
      phase: 'done',
      completed_at: Task.updated_at,
    },
  });
  const { unmount } = renderHook(
    () => useDocumentTask(Scope, Task.task_id),
    wrapper(),
  );
  await act(async () => {
    await Promise.resolve();
  });
  await act(async () => {
    jest.advanceTimersByTime(10000);
  });
  expect(service.task).toHaveBeenCalledTimes(1);
  unmount();
});

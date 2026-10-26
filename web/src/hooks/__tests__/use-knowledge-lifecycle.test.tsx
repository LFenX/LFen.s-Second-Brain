type ReactNode = import('react').ReactNode;
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service from '@/services/knowledge-lifecycle-service';
import { getAuthorization } from '@/utils/authorization-util';
import {
  useLifecycleList,
  useLifecycleActions,
  KnowledgeLifecycleKeys,
  scopedLifecycle,
} from '../use-knowledge-lifecycle';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from '../use-knowledge-foundation';
import { KnowledgeWebKeys } from '../use-knowledge-web';
import { KnowledgeOriginalKeys } from '../use-knowledge-original';
import {
  Scope,
  Owner,
  Domain,
  detail,
} from '@/pages/dataset/knowledge-catalogue/__tests__/lifecycle-fixture';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-lifecycle-session'),
}));
jest.mock('@/services/knowledge-lifecycle-service', () => {
  const actual = jest.requireActual('@/services/knowledge-lifecycle-service');
  return {
    ...actual,
    __esModule: true,
    default: { ...actual.default, list: jest.fn() },
  };
});
const List = service.list as jest.Mock;
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
  List.mockReset();
  (getAuthorization as jest.Mock).mockReturnValue(
    'synthetic-lifecycle-session',
  );
});
it('isolates full filters including empty and omitted language, scope, state and page limit', () => {
  const session = getKnowledgeSessionScope(),
    keys = [
      {},
      { language: '' },
      { group_scope: 'ungrouped' as const },
      { group_scope: 'exact' as const, group_id: '3'.repeat(32) },
      { state: 'archived' as const },
      { limit: 1 },
    ].map((query) =>
      JSON.stringify(
        KnowledgeLifecycleKeys.list(session, Owner, Domain, query),
      ),
    );
  expect(new Set(keys).size).toBe(6);
  expect(KnowledgeLifecycleKeys.detail(session, Scope)).not.toEqual(
    KnowledgeLifecycleKeys.detail(session, { ...Scope, owner: '3'.repeat(32) }),
  );
});
it('reuses all original filters for short cursor pages and rejects duplicate ordered sources', async () => {
  const first = detail(),
    second = { ...first, source_id: 'e'.repeat(32) };
  List.mockImplementation(async (_owner, _domain, query) => ({
    data: query.cursor
      ? { items: [second], next_cursor: null }
      : { items: [first], next_cursor: '9'.repeat(32) },
  }));
  const options = {
    state: 'active' as const,
    group_scope: 'ungrouped' as const,
    language: '',
    limit: 20,
  };
  const { result } = renderHook(
    () => ({ ...useLifecycleList(Owner, Domain, options) }),
    { wrapper: wrapper().wrapper },
  );
  await waitFor(() => expect(result.current.data?.pages).toHaveLength(1));
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.data?.pages).toHaveLength(2));
  expect(List.mock.calls[1][2]).toEqual({ ...options, cursor: '9'.repeat(32) });
  List.mockResolvedValue({
    data: { items: [first, first], next_cursor: null },
  });
  await act(async () => {
    await result.current.refetch();
  });
  await waitFor(() => expect(result.current.isError).toBe(true));
});
it('invalidates active list families and current detail without invalidating fixed revision caches', async () => {
  const { client, wrapper: Wrapper } = wrapper(),
    session = getKnowledgeSessionScope(),
    group = '3'.repeat(32);
  const lists = [
    KnowledgeLifecycleKeys.list(session, Owner, Domain, { state: 'archived' }),
    KnowledgeKeys.sources(session, Domain, group),
    KnowledgeOriginalKeys.list(session, Domain, group),
    KnowledgeWebKeys.revisions(session, Domain, Owner, group),
  ];
  const fixed = [
    KnowledgeKeys.source(session, Domain, 'c'.repeat(32)),
    KnowledgeOriginalKeys.revision(session, Domain, 'c'.repeat(32)),
    KnowledgeWebKeys.revision(session, Domain, Owner, 'c'.repeat(32)),
  ];
  for (const key of [...lists, ...fixed]) client.setQueryData(key, {});
  const { result } = renderHook(() => useLifecycleActions(Scope), {
    wrapper: Wrapper,
  });
  await act(async () => {
    await result.current.refreshCurrent();
  });
  for (const key of lists)
    expect(client.getQueryState(key)?.isInvalidated).toBe(true);
  for (const key of fixed)
    expect(client.getQueryState(key)?.isInvalidated).toBe(false);
  expect(KnowledgeKeys.sources(session, Domain, group)).toEqual([
    'knowledge',
    session,
    Domain,
    'sources',
    group,
  ]);
  expect(KnowledgeWebKeys.revisions(session, Domain, Owner, group)).toEqual([
    'knowledge-web',
    session,
    Domain,
    Owner,
    'revisions',
    group,
    undefined,
  ]);
});
it('rejects late results after native session replacement and never executes stale work', async () => {
  const session = getKnowledgeSessionScope();
  let finish!: (value: number) => void;
  const pending = scopedLifecycle(
    session,
    () =>
      new Promise<number>((resolve) => {
        finish = resolve;
      }),
  );
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-next-session');
  getKnowledgeSessionScope();
  finish(1);
  await expect(pending).rejects.toMatchObject({ status: 401 });
  const work = jest.fn(async () => 1);
  await expect(scopedLifecycle(session, work)).rejects.toMatchObject({
    status: 401,
  });
  expect(work).not.toHaveBeenCalled();
});

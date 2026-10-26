import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import service, {
  LifecycleRequestError,
} from '@/services/knowledge-lifecycle-service';
import type { LifecycleListQuery } from '@/interfaces/request/knowledge-lifecycle';
import type { LifecycleScope } from '@/utils/knowledge-lifecycle';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from './use-knowledge-foundation';
import { KnowledgeOriginalKeys } from './use-knowledge-original';
import { KnowledgeWebKeys } from './use-knowledge-web';

export const KnowledgeLifecycleKeys = {
  contract: (session: number) =>
    ['knowledge-lifecycle', session, 'contract'] as const,
  scope: (session: number, owner: string, domain: string) =>
    ['knowledge-lifecycle', session, owner, domain] as const,
  lists: (session: number, owner: string, domain: string) =>
    [...KnowledgeLifecycleKeys.scope(session, owner, domain), 'lists'] as const,
  list: (
    session: number,
    owner: string,
    domain: string,
    query: LifecycleListQuery,
  ) =>
    [
      ...KnowledgeLifecycleKeys.lists(session, owner, domain),
      query.state ?? 'active',
      query.group_scope ?? 'all',
      query.group_id,
      query.component_side,
      query.language,
      query.limit ?? 20,
    ] as const,
  detail: (session: number, scope: LifecycleScope) =>
    [
      ...KnowledgeLifecycleKeys.scope(session, scope.owner, scope.domain),
      'detail',
      scope.source,
    ] as const,
  movePaths: (
    session: number,
    scope: LifecycleScope,
    registration: string | null,
  ) =>
    [
      ...KnowledgeLifecycleKeys.scope(session, scope.owner, scope.domain),
      'move-paths',
      scope.source,
      registration,
    ] as const,
  action: (session: number, scope: LifecycleScope) =>
    [
      ...KnowledgeLifecycleKeys.scope(session, scope.owner, scope.domain),
      'action',
      scope.source,
    ] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
} as const;
export function lifecycleScopeFailure(error: unknown) {
  return (
    error instanceof KnowledgeRequestError &&
    ([401, 404].includes(error.status) ||
      [
        'knowledge_contract_version',
        'knowledge_source_contract_version',
        'policy_changed',
      ].includes(error.code))
  );
}
export async function scopedLifecycle<T>(
  session: number,
  work: () => Promise<T>,
) {
  if (session !== getKnowledgeSessionScope())
    throw new LifecycleRequestError(401, 'knowledge_unauthorized');
  const value = await work();
  if (session !== getKnowledgeSessionScope())
    throw new LifecycleRequestError(401, 'knowledge_unauthorized');
  return value;
}
export function useLifecycleContract() {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeLifecycleKeys.contract(session),
    queryFn: async ({ signal }) =>
      (await scopedLifecycle(session, () => service.contract(signal))).data,
  });
}
export function useLifecycleDetail(scope: LifecycleScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeLifecycleKeys.detail(session, scope),
    queryFn: async ({ signal }) =>
      (await scopedLifecycle(session, () => service.detail(scope, signal)))
        .data,
  });
}
export function useLifecycleMovePaths(
  detail: import('@/interfaces/database/knowledge-lifecycle').LifecycleDetail,
) {
  const session = getKnowledgeSessionScope(),
    scope = {
      owner: detail.owner_id,
      domain: detail.domain_id,
      source: detail.source_id,
    };
  return useQuery({
    ...Options,
    queryKey: KnowledgeLifecycleKeys.movePaths(
      session,
      scope,
      detail.registration_id,
    ),
    queryFn: ({ signal }) =>
      scopedLifecycle(session, () => service.movePaths(detail, signal)),
  });
}
export function useLifecycleList(
  owner: string,
  domain: string,
  query: LifecycleListQuery,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeLifecycleKeys.list(session, owner, domain, query),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedLifecycle(session, () =>
          service.list(
            owner,
            domain,
            { ...query, ...(pageParam ? { cursor: pageParam } : {}) },
            signal,
          ),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      const ids = new Set<string>(),
        cursors = new Set<string>();
      let previous = '';
      for (const page of data.pages) {
        for (const value of page.items) {
          if (
            ids.has(value.source_id) ||
            (previous && previous >= value.source_id)
          )
            throw Error('Nonadvancing lifecycle source order');
          ids.add(value.source_id);
          previous = value.source_id;
        }
        if (page.next_cursor) {
          if (cursors.has(page.next_cursor))
            throw Error('Repeated lifecycle cursor');
          cursors.add(page.next_cursor);
        }
      }
      return data;
    },
  });
}
export function useLifecycleActions(scope: LifecycleScope) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const mutation = useMutation({
    ...Options,
    mutationKey: KnowledgeLifecycleKeys.action(session, scope),
    mutationFn: (work: () => Promise<unknown>) =>
      scopedLifecycle(session, work),
  });
  async function run<T>(work: () => Promise<T>) {
    return (await mutation.mutateAsync(work)) as T;
  }
  async function refreshCurrent() {
    await Promise.all([
      client.invalidateQueries({
        queryKey: KnowledgeLifecycleKeys.lists(
          session,
          scope.owner,
          scope.domain,
        ),
      }),
      client.invalidateQueries({
        queryKey: KnowledgeLifecycleKeys.detail(session, scope),
      }),
      client.invalidateQueries({
        queryKey: KnowledgeKeys.sourceLists(session, scope.domain),
      }),
      client.invalidateQueries({
        queryKey: KnowledgeOriginalKeys.lists(session, scope.domain),
      }),
      client.invalidateQueries({
        queryKey: KnowledgeWebKeys.revisionLists(
          session,
          scope.domain,
          scope.owner,
        ),
      }),
    ]);
  }
  function clear() {
    client.removeQueries({
      queryKey: KnowledgeLifecycleKeys.scope(
        session,
        scope.owner,
        scope.domain,
      ),
    });
  }
  return { run, refreshCurrent, clear };
}

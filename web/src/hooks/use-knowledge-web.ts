import { useCallback, useEffect, useRef } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import service, {
  WebRequestError,
  type WebScope,
} from '@/services/knowledge-web-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import type {
  WebCapture,
  WebObservation,
} from '@/interfaces/database/knowledge-web';
import { latestWebResource } from '@/utils/knowledge-web';
import { compareWebBytes } from '@/utils/knowledge-web-schema';
import { getKnowledgeSessionScope } from './use-knowledge-foundation';

export const KnowledgeWebKeys = {
  contract: (session: number) =>
    ['knowledge-web', session, 'contract'] as const,
  scope: (session: number, domain: string, owner: string) =>
    ['knowledge-web', session, domain, owner] as const,
  action: (session: number, scope: WebScope) =>
    [
      ...KnowledgeWebKeys.scope(session, scope.domain, scope.owner),
      'action',
      scope.registration,
    ] as const,
  resource: (session: number, scope: WebScope, kind: string, id: string) =>
    [
      ...KnowledgeWebKeys.scope(session, scope.domain, scope.owner),
      scope.registration,
      kind,
      id,
    ] as const,
  revisions: (
    session: number,
    domain: string,
    owner: string,
    group?: string,
    registration?: string,
  ) =>
    [
      ...KnowledgeWebKeys.scope(session, domain, owner),
      'revisions',
      group,
      registration,
    ] as const,
  revisionLists: (session: number, domain: string, owner: string) =>
    [...KnowledgeWebKeys.scope(session, domain, owner), 'revisions'] as const,
  revision: (session: number, domain: string, owner: string, id: string) =>
    [
      ...KnowledgeWebKeys.scope(session, domain, owner),
      'revision',
      id,
    ] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
} as const;
export function webScopeFailure(error: unknown) {
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
export async function scopedWeb<T>(session: number, work: () => Promise<T>) {
  if (session !== getKnowledgeSessionScope())
    throw new WebRequestError(401, 'knowledge_unauthorized');
  const value = await work();
  if (session !== getKnowledgeSessionScope())
    throw new WebRequestError(401, 'knowledge_unauthorized');
  return value;
}
export function useWebContract() {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeWebKeys.contract(session),
    queryFn: async ({ signal }) =>
      (await scopedWeb(session, () => service.contract(signal))).data,
  });
}
export function useWebActions(scope: WebScope) {
  const session = getKnowledgeSessionScope();
  const mutation = useMutation({
    ...Options,
    mutationKey: KnowledgeWebKeys.action(session, scope),
    mutationFn: (work: () => Promise<unknown>) => scopedWeb(session, work),
  });
  async function run<T>(work: () => Promise<T>) {
    return (await mutation.mutateAsync(work)) as T;
  }
  return { run, reset: mutation.reset };
}
export function useWebResource<T extends WebCapture | WebObservation>(
  scope: WebScope,
  kind: 'capture' | 'observation',
  id: string,
  read: (signal?: AbortSignal) => Promise<{ data: T }>,
  onFailure?: (error: unknown) => void,
) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    latest = useRef<T | null>(null),
    failures = useRef(0),
    identity = useRef(''),
    handler = useRef(onFailure),
    loader = useRef(read);
  handler.current = onFailure;
  loader.current = read;
  const context = [
    session,
    scope.domain,
    scope.owner,
    scope.registration,
    kind,
    id,
  ].join(':');
  const { domain, owner, registration } = scope;
  if (identity.current !== context) {
    identity.current = context;
    latest.current = null;
    failures.current = 0;
  }
  const query = useQuery({
    ...Options,
    queryKey: KnowledgeWebKeys.resource(session, scope, kind, id),
    queryFn: async ({ signal }) => {
      try {
        const result = await scopedWeb(session, () => loader.current(signal));
        latest.current = latestWebResource(latest.current, result.data);
        failures.current = 0;
        return latest.current;
      } catch (error) {
        failures.current +=
          error instanceof WebRequestError && [0, 503].includes(error.status)
            ? 1
            : 3;
        throw error;
      }
    },
    refetchInterval: (value) =>
      latest.current &&
      ['receiving', 'fetching', 'parsing'].includes(latest.current.state) &&
      !webScopeFailure(value.state.error) &&
      failures.current < 3
        ? 2000
        : false,
    refetchIntervalInBackground: false,
  });
  useEffect(() => {
    if (query.error && webScopeFailure(query.error))
      handler.current?.(query.error);
  }, [query.error]);
  const adopt = useCallback(
    (value: T) => {
      latest.current = latestWebResource(latest.current, value);
      client.setQueryData(
        KnowledgeWebKeys.resource(
          session,
          { domain, owner, registration },
          kind,
          id,
        ),
        latest.current,
      );
      return latest.current;
    },
    [client, session, domain, owner, registration, kind, id],
  );
  function recheck() {
    failures.current = 0;
    return query.refetch();
  }
  return { ...query, adopt, recheck, pollingStopped: failures.current >= 3 };
}
export function useWebRevisions(
  domain: string,
  owner: string,
  group?: string,
  registration?: string,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeWebKeys.revisions(
      session,
      domain,
      owner,
      group,
      registration,
    ),
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam, signal }) => {
      const page = (
        await scopedWeb(session, () =>
          service.revisions(
            domain,
            {
              group_id: group,
              registration_id: registration,
              cursor: pageParam,
            },
            signal,
          ),
        )
      ).data;
      if (
        page.items.some(
          (item) =>
            item.domain_id !== domain ||
            (group && item.classification.group_id !== group) ||
            (registration && item.registration_id !== registration),
        )
      )
        throw Error('Invalid revision page scope');
      return page;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      const ids = new Set<string>(),
        cursors = new Set<string>();
      for (const page of data.pages) {
        for (const item of page.items) {
          if (
            ids.has(item.source_revision_id) ||
            item.domain_id !== domain ||
            (group && item.classification.group_id !== group) ||
            (registration && item.registration_id !== registration)
          )
            throw Error('Invalid revision page');
          ids.add(item.source_revision_id);
        }
        if (page.next_cursor) {
          if (cursors.has(page.next_cursor))
            throw Error('Repeated revision cursor');
          cursors.add(page.next_cursor);
        }
      }
      return data;
    },
  });
}
export function useWebRevision(domain: string, owner: string, id: string) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeWebKeys.revision(session, domain, owner, id),
    queryFn: async ({ signal }) =>
      (await scopedWeb(session, () => service.revision(domain, id, signal)))
        .data,
  });
}
export async function collectWebPages<T>(
  load: (
    cursor: string | null,
  ) => Promise<{ data: { items: T[]; next_cursor: string | null } }>,
  identity: (value: T) => string,
  maximum: number,
  ordered = false,
) {
  const result: T[] = [],
    ids = new Set<string>(),
    cursors = new Set<string>();
  let cursor: string | null = null;
  do {
    const { data } = await load(cursor);
    for (const item of data.items) {
      const id = identity(item);
      if (
        ids.has(id) ||
        (ordered &&
          result.length > 0 &&
          compareWebBytes(identity(result[result.length - 1]), id) >= 0)
      )
        throw Error('Invalid paginated items');
      ids.add(id);
      result.push(item);
      if (result.length > maximum) throw Error('Page budget exceeded');
    }
    cursor = data.next_cursor;
    if (cursor) {
      if (cursors.has(cursor) || data.items.length === 0)
        throw Error('Nonadvancing cursor');
      cursors.add(cursor);
    }
  } while (cursor);
  return result;
}

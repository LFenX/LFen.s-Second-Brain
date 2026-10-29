import { useContext, useEffect, useMemo } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import type {
  ContentAddress,
  ContentNode,
  ContentNodePage,
  ContentSourceRef,
  ContentParsedRepresentationPage,
  ContentNodeRef,
  ContentTarget,
  ContentParsedTask,
} from '@/interfaces/database/knowledge-consumption';
import type {
  ContentScope,
  ContentPolicyRequest,
  ContentParsedUploadRequest,
  ContentParsedSnapshotRequest,
  ContentParsedTaskRequest,
  ContentParsedCancelRequest,
} from '@/interfaces/request/knowledge-consumption';
import service from '@/services/knowledge-consumption-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  contentCanonical,
  contentTargetKey,
  ContentLimits,
  checkContentTask,
  contentTaskTerminal,
} from '@/utils/knowledge-consumption';
import { CatalogueScopeFailure } from '@/pages/dataset/knowledge-catalogue/state';

export const KnowledgeConsumptionKeys = {
  scope: (session: number, scope: ContentScope) =>
    ['knowledge-consumption', session, scope.owner, scope.domain] as const,
  policy: (session: number, scope: ContentScope) =>
    [...KnowledgeConsumptionKeys.scope(session, scope), 'policy'] as const,
  parsedSource: (session: number, scope: ContentScope, source: string) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'parsed-source',
      source,
    ] as const,
  parsedTask: (
    session: number,
    scope: ContentScope,
    task: string,
    source: string,
  ) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'parsed-task',
      task,
      source,
    ] as const,
  action: (session: number, scope: ContentScope, action: string) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'action',
      action,
    ] as const,
  capabilities: (session: number, scope: ContentScope) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'capabilities',
    ] as const,
  parsedRepresentations: (
    session: number,
    scope: ContentScope,
    source: ContentSourceRef,
  ) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'parsed-representations',
      contentCanonical(source),
    ] as const,
  representation: (
    session: number,
    scope: ContentScope,
    target: ContentTarget,
  ) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'representation',
      contentTargetKey(target),
    ] as const,
  node: (session: number, scope: ContentScope, address: ContentAddress) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'node',
      contentCanonical(address),
    ] as const,
  nodes: (
    session: number,
    scope: ContentScope,
    target: ContentTarget,
    parent: ContentNodeRef | null,
  ) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'nodes',
      contentTargetKey(target),
      contentCanonical(parent),
    ] as const,
  read: (
    session: number,
    scope: ContentScope,
    address: ContentAddress,
    start: number,
  ) =>
    [
      ...KnowledgeConsumptionKeys.scope(session, scope),
      'read',
      contentCanonical(address),
      start,
    ] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;
export async function scopedKnowledgeContent<T>(
  session: number,
  work: () => Promise<T>,
) {
  const check = () => {
    if (session !== getKnowledgeSessionScope())
      throw new KnowledgeRequestError(401, 'unauthenticated');
  };
  check();
  try {
    const data = await work();
    check();
    return data;
  } catch (error) {
    check();
    throw error;
  }
}
export function useContentScopeError(error: unknown) {
  const failure = useContext(CatalogueScopeFailure);
  useEffect(() => {
    if (
      error instanceof KnowledgeRequestError &&
      [401, 403, 404, 409].includes(error.status)
    )
      failure?.(error);
  }, [error, failure]);
}
export function useContentBoundary(scope: ContentScope) {
  const client = useQueryClient(),
    session = getKnowledgeSessionScope();
  const { owner, domain } = scope;
  const key = useMemo(
    () => KnowledgeConsumptionKeys.scope(session, { owner, domain }),
    [session, owner, domain],
  );
  useEffect(
    () => () => {
      void client.cancelQueries({ queryKey: key });
      client.removeQueries({ queryKey: key });
    },
    [client, key],
  );
  return session;
}
export function useContentCapabilities(scope: ContentScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.capabilities(session, scope),
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, () =>
        service.capabilities(scope, signal),
      ),
  });
}
export function useContentRepresentation(
  scope: ContentScope,
  target: ContentTarget,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.representation(session, scope, target),
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, () =>
        service.representation(scope, target, signal),
      ),
  });
}
export function useContentNode(scope: ContentScope, address: ContentAddress) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.node(session, scope, address),
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, () =>
        service.node(scope, address, signal),
      ),
  });
}
export function useContentNodes(
  scope: ContentScope,
  target: ContentTarget,
  parent: ContentNodeRef | null,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.nodes(session, scope, target, parent),
    initialPageParam: '',
    getNextPageParam: (page: ContentNodePage) => page.next_cursor ?? undefined,
    queryFn: ({ pageParam, signal }) =>
      scopedKnowledgeContent(session, () =>
        service.nodes(
          scope,
          {
            target,
            parent_ref: parent,
            cursor: pageParam,
            limit: Math.min(20, ContentLimits.page),
          },
          signal,
        ),
      ),
  });
}
export function useContentRead(
  scope: ContentScope,
  address: ContentAddress,
  node: ContentNode,
  start: number,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.read(session, scope, address, start),
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, () =>
        service.read(
          scope,
          { address, start_byte: start, end_byte: null },
          node,
          signal,
        ),
      ),
  });
}

export function useContentParsedRepresentations(
  scope: ContentScope,
  source: ContentSourceRef,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.parsedRepresentations(
      session,
      scope,
      source,
    ),
    initialPageParam: '',
    getNextPageParam: (page: ContentParsedRepresentationPage) =>
      page.next_cursor ?? undefined,
    queryFn: ({ pageParam, signal }) =>
      scopedKnowledgeContent(session, () =>
        service.parsedRepresentations(
          scope,
          { source_ref: source, cursor: pageParam, limit: 20 },
          signal,
        ),
      ),
  });
}

export function useContentPolicy(scope: ContentScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.policy(session, scope),
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, () => service.policy(scope, signal)),
  });
}
export function useContentParsedSource(scope: ContentScope, source: string) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeConsumptionKeys.parsedSource(session, scope, source),
    enabled: !!source,
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, () =>
        service.parsedSource(scope, source, signal),
      ),
  });
}
async function latestContentTask(
  scope: ContentScope,
  next: ContentParsedTask,
  old?: ContentParsedTask,
) {
  if (!old) return next;
  if (old.revision > next.revision) {
    await checkContentTask(scope, old, { previous: next });
    return old;
  }
  await checkContentTask(scope, next, { previous: old });
  return next;
}
export function useContentParsedTask(
  scope: ContentScope,
  task: string,
  source: string,
) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    key = KnowledgeConsumptionKeys.parsedTask(session, scope, task, source);
  return useQuery({
    ...Options,
    queryKey: key,
    enabled: !!task && !!source,
    queryFn: ({ signal }) =>
      scopedKnowledgeContent(session, async () =>
        latestContentTask(
          scope,
          await service.parsedTask(scope, task, source, signal),
          client.getQueryData<ContentParsedTask>(key),
        ),
      ),
    refetchInterval: (query) =>
      query.state.error ||
      !query.state.data ||
      contentTaskTerminal(query.state.data)
        ? false
        : 2000,
    refetchIntervalInBackground: false,
  });
}
function useContentAction<I, O>(
  scope: ContentScope,
  action: string,
  work: (input: I) => Promise<O>,
) {
  const session = getKnowledgeSessionScope(),
    active = useMemo(
      () => ({
        current: true,
        session,
        owner: scope.owner,
        domain: scope.domain,
      }),
      [session, scope.owner, scope.domain],
    );
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, [active]);
  return useMutation({
    mutationKey: KnowledgeConsumptionKeys.action(session, scope, action),
    retry: false,
    mutationFn: (input: I) =>
      scopedKnowledgeContent(session, async () => {
        const check = () => {
          if (!active.current)
            throw new KnowledgeRequestError(0, 'content_scope_changed');
        };
        check();
        try {
          const data = await work(input);
          check();
          return data;
        } catch (error) {
          check();
          throw error;
        }
      }),
  });
}
export const useContentPolicyChange = (scope: ContentScope) =>
  useContentAction(scope, 'policy-change', (request: ContentPolicyRequest) =>
    service.changePolicy(scope, request),
  );
export const useContentParsedUpload = (scope: ContentScope) =>
  useContentAction(
    scope,
    'parsed-upload',
    (input: { request: ContentParsedUploadRequest; file: File }) =>
      service.uploadParsedSource(scope, input.request, input.file),
  );
export const useContentParsedSnapshot = (scope: ContentScope) =>
  useContentAction(
    scope,
    'parsed-snapshot',
    (request: ContentParsedSnapshotRequest) =>
      service.snapshotParsedSource(scope, request),
  );
export const useContentParsedTaskCreate = (scope: ContentScope) =>
  useContentAction(
    scope,
    'parsed-task-create',
    (request: ContentParsedTaskRequest) =>
      service.createParsedTask(scope, request),
  );
export const useContentParsedTaskCancel = (scope: ContentScope) =>
  useContentAction(
    scope,
    'parsed-task-cancel',
    (input: { task: ContentParsedTask; request: ContentParsedCancelRequest }) =>
      service.cancelParsedTask(scope, input.task, input.request),
  );

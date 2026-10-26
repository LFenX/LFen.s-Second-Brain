import { useEffect, useState } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import service, {
  StructureRequestError,
  type StructureResponse,
} from '@/services/knowledge-structure-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { getKnowledgeSessionScope } from './use-knowledge-foundation';
import type {
  StructureScope,
  StructureProjectScope,
  StructureViewScope,
  StructureOperation,
  StructureOutcome,
} from '@/utils/knowledge-structure';
import { structureTerminal } from '@/utils/knowledge-structure';
import type {
  StructureNode,
  StructureAlias,
} from '@/interfaces/database/knowledge-structure';
import type {
  StructureNodeQuery,
  StructureRelationQuery,
  StructureContentQuery,
} from '@/interfaces/request/knowledge-structure';
import type { BatchManifestEntry } from '@/interfaces/database/knowledge-batch';
import { appendBatchManifestPage } from '@/utils/knowledge-batch';

export const KnowledgeStructureKeys = {
  root: () => ['knowledge-structure'] as const,
  contract: (session: number) =>
    [...KnowledgeStructureKeys.root(), session, 'contract'] as const,
  scope: (session: number, scope: StructureScope) =>
    [
      ...KnowledgeStructureKeys.root(),
      session,
      scope.owner,
      scope.domain,
    ] as const,
  policy: (session: number, scope: StructureScope) =>
    [...KnowledgeStructureKeys.scope(session, scope), 'policy'] as const,
  head: (session: number, scope: StructureProjectScope) =>
    [
      ...KnowledgeStructureKeys.scope(session, scope),
      'head',
      scope.project,
    ] as const,
  task: (session: number, scope: StructureScope, task: string) =>
    [...KnowledgeStructureKeys.scope(session, scope), 'task', task] as const,
  tasks: (session: number, scope: StructureProjectScope, limit: number) =>
    [
      ...KnowledgeStructureKeys.scope(session, scope),
      'tasks',
      scope.project,
      limit,
    ] as const,
  byKey: (
    session: number,
    scope: StructureScope,
    kind: string,
    key: string,
    target: string,
  ) =>
    [
      ...KnowledgeStructureKeys.scope(session, scope),
      'by-key',
      kind,
      target,
      key,
    ] as const,
  view: (session: number, scope: StructureViewScope) =>
    [
      ...KnowledgeStructureKeys.scope(session, scope),
      'view',
      scope.view,
    ] as const,
  nodes: (
    session: number,
    scope: StructureViewScope,
    query: StructureNodeQuery,
  ) =>
    [
      ...KnowledgeStructureKeys.view(session, scope),
      'nodes',
      query.source_id,
      query.parent_id ?? null,
      query.limit ?? 20,
      query.cursor ?? null,
    ] as const,
  node: (session: number, scope: StructureViewScope, node: string) =>
    [...KnowledgeStructureKeys.view(session, scope), 'node', node] as const,
  content: (
    session: number,
    scope: StructureViewScope,
    node: StructureNode,
    sha: string,
    query: StructureContentQuery,
  ) =>
    [
      ...KnowledgeStructureKeys.node(session, scope, node.structure_node_id),
      'content',
      node.source_id,
      node.source_revision_id,
      sha,
      query.start_byte ?? null,
      query.end_byte ?? null,
    ] as const,
  relations: (
    session: number,
    scope: StructureViewScope,
    query: StructureRelationQuery,
  ) =>
    [
      ...KnowledgeStructureKeys.view(session, scope),
      'relations',
      query.node_id,
      query.direction ?? 'both',
      query.kind ?? null,
      query.limit ?? 20,
      query.cursor ?? null,
    ] as const,
  manifestHead: (session: number, scope: StructureProjectScope) =>
    [
      ...KnowledgeStructureKeys.scope(session, scope),
      'manifest-head',
      scope.project,
    ] as const,
  manifest: (session: number, scope: StructureProjectScope, manifest: string) =>
    [
      ...KnowledgeStructureKeys.scope(session, scope),
      'manifest',
      scope.project,
      manifest,
    ] as const,
  action: (session: number, scope: StructureScope) =>
    [...KnowledgeStructureKeys.scope(session, scope), 'action'] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;
export function structureScopeFailure(error: unknown): boolean {
  return error instanceof KnowledgeRequestError && error.status === 401;
}
export async function scopedStructure<T>(
  session: number,
  work: () => Promise<T>,
): Promise<T> {
  if (session !== getKnowledgeSessionScope())
    throw new StructureRequestError(401, 'knowledge_unauthorized');
  try {
    const value = await work();
    if (session !== getKnowledgeSessionScope())
      throw new StructureRequestError(401, 'knowledge_unauthorized');
    return value;
  } catch (error) {
    if (session !== getKnowledgeSessionScope())
      throw new StructureRequestError(401, 'knowledge_unauthorized');
    throw error;
  }
}
export function useStructurePrivateError(error: unknown) {
  const client = useQueryClient();
  useEffect(() => {
    if (structureScopeFailure(error))
      client.removeQueries({ queryKey: KnowledgeStructureKeys.root() });
  }, [client, error]);
}
export function useStructureContract() {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.contract(session),
    queryFn: async ({ signal }) =>
      (await scopedStructure(session, () => service.contract(signal))).data,
  });
}
export function useStructurePolicy(scope: StructureScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.policy(session, scope),
    queryFn: async ({ signal }) =>
      (await scopedStructure(session, () => service.policy(scope, signal)))
        .data,
  });
}
export function useStructureHead(scope: StructureProjectScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.head(session, scope),
    queryFn: async ({ signal }) =>
      (await scopedStructure(session, () => service.head(scope, signal))).data,
  });
}
export function useStructureView(scope: StructureViewScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.view(session, scope),
    queryFn: async ({ signal }) =>
      (await scopedStructure(session, () => service.view(scope, signal))).data,
  });
}
export function useStructureNode(scope: StructureViewScope, node: string) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.node(session, scope, node),
    queryFn: async ({ signal }) =>
      (await scopedStructure(session, () => service.node(scope, node, signal)))
        .data,
  });
}
export function useStructureContent(
  scope: StructureViewScope,
  node: StructureNode,
  sha: string,
  query: StructureContentQuery,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.content(session, scope, node, sha, query),
    queryFn: async ({ signal }) =>
      (
        await scopedStructure(session, () =>
          service.content(scope, node, sha, query, signal),
        )
      ).data,
  });
}
function orderedPages<T>(
  pages: { items: T[]; next_cursor: string | null }[],
  compare: (a: T, b: T) => number,
  identity: (value: T) => string,
) {
  const ids = new Set<string>(),
    cursors = new Set<string>();
  let previous: T | undefined;
  for (const page of pages) {
    for (const item of page.items) {
      const key = identity(item);
      if (
        ids.has(key) ||
        (previous !== undefined && compare(previous, item) >= 0)
      )
        throw new StructureRequestError(502, 'structure_invalid_response');
      ids.add(key);
      previous = item;
    }
    if (page.next_cursor) {
      if (cursors.has(page.next_cursor))
        throw new StructureRequestError(502, 'structure_invalid_response');
      cursors.add(page.next_cursor);
    }
  }
}
export function useStructureNodes(
  scope: StructureViewScope,
  query: StructureNodeQuery,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.nodes(session, scope, query),
    initialPageParam: query.cursor,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedStructure(session, () =>
          service.nodes(
            scope,
            { ...query, ...(pageParam ? { cursor: pageParam } : {}) },
            signal,
          ),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      orderedPages(
        data.pages,
        (a, b) =>
          a.start_byte - b.start_byte ||
          a.end_byte - b.end_byte ||
          (a.kind < b.kind ? -1 : a.kind > b.kind ? 1 : 0) ||
          (a.structure_node_id < b.structure_node_id
            ? -1
            : a.structure_node_id > b.structure_node_id
              ? 1
              : 0),
        (node) => node.structure_node_id,
      );
      return data;
    },
  });
}
export function useStructureRelations(
  scope: StructureViewScope,
  query: StructureRelationQuery,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.relations(session, scope, query),
    initialPageParam: query.cursor,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedStructure(session, () =>
          service.relations(
            scope,
            { ...query, ...(pageParam ? { cursor: pageParam } : {}) },
            signal,
          ),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      orderedPages(
        data.pages,
        (a, b) =>
          a.relation_id < b.relation_id
            ? -1
            : a.relation_id > b.relation_id
              ? 1
              : 0,
        (relation) => relation.relation_id,
      );
      return data;
    },
  });
}
export function useStructureTasks(scope: StructureProjectScope, limit = 20) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.tasks(session, scope, limit),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedStructure(session, () =>
          service.tasks(
            scope,
            {
              project_id: scope.project,
              limit,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
            signal,
          ),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      orderedPages(
        data.pages,
        (a, b) =>
          a.created_at > b.created_at
            ? -1
            : a.created_at < b.created_at
              ? 1
              : a.task_id > b.task_id
                ? -1
                : a.task_id < b.task_id
                  ? 1
                  : 0,
        (task) => task.task_id,
      );
      const captured = data.pages[0]?.captured_at;
      if (data.pages.some((page) => page.captured_at !== captured))
        throw new StructureRequestError(502, 'structure_invalid_response');
      return data;
    },
  });
}
export function useStructureTask(scope: StructureScope, task: string) {
  const session = getKnowledgeSessionScope(),
    [visible, setVisible] = useState(document.visibilityState === 'visible'),
    [waiting, setWaiting] = useState(true),
    [cycle, setCycle] = useState(0);
  useEffect(() => {
    const changed = () => setVisible(document.visibilityState === 'visible');
    document.addEventListener('visibilitychange', changed);
    return () => document.removeEventListener('visibilitychange', changed);
  }, []);
  useEffect(() => {
    if (!visible || !waiting) return;
    const timeout = window.setTimeout(() => setWaiting(false), 25000);
    return () => window.clearTimeout(timeout);
  }, [visible, waiting, cycle, task]);
  const result = useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.task(session, scope, task),
    queryFn: async ({ signal }) =>
      (await scopedStructure(session, () => service.task(scope, task, signal)))
        .data,
    refetchInterval: (query) =>
      visible &&
      waiting &&
      !query.state.error &&
      !(query.state.data && structureTerminal(query.state.data))
        ? 2000
        : false,
    refetchIntervalInBackground: false,
  });
  const resume = () => {
    setCycle((current) => current + 1);
    setWaiting(true);
  };
  return {
    ...result,
    waiting: waiting && !(result.data && structureTerminal(result.data)),
    resume,
  };
}
export function useStructureManifestHead(scope: StructureProjectScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.manifestHead(session, scope),
    queryFn: async ({ signal }) =>
      (
        await scopedStructure(session, () =>
          service.manifestHead(scope, signal),
        )
      ).data,
  });
}
export function useStructureManifest(
  scope: StructureProjectScope,
  manifest: string,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeStructureKeys.manifest(session, scope, manifest),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedStructure(session, () =>
          service.manifest(scope, manifest, pageParam, signal),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      data.pages.reduce<
        import('@/interfaces/database/knowledge-batch').BatchManifestVersionPage[]
      >((pages, page) => appendBatchManifestPage(pages, page), []);
      return data;
    },
  });
}
export function useStructureActions(scope: StructureScope) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const mutation = useMutation({
    ...Options,
    mutationKey: KnowledgeStructureKeys.action(session, scope),
    mutationFn: (work: () => Promise<unknown>) =>
      scopedStructure(session, work),
  });
  async function run<T>(work: () => Promise<T>): Promise<T> {
    return (await mutation.mutateAsync(work)) as T;
  }
  const write = (operation: StructureOperation) =>
    run<StructureResponse<StructureOutcome>>(() =>
      operation.kind === 'task'
        ? service.createTask(operation.scope, operation.body)
        : operation.kind === 'policy'
          ? service.changePolicy(operation.scope, operation.body)
          : service.cancel(operation.scope, operation.body),
    );
  const recover = (kind: 'task' | 'policy' | 'cancel', key: string) =>
    run<StructureResponse<StructureOutcome>>(() =>
      kind === 'task'
        ? service.taskByKey(scope, key)
        : service.commandByKey(scope, kind, key),
    );
  const task = (taskId: string) => run(() => service.task(scope, taskId));
  const prepare = (
    project: StructureProjectScope,
    manifest: string,
    entries: BatchManifestEntry[],
    aliases: StructureAlias[],
  ) => run(() => service.prepare(project, manifest, entries, aliases));
  const refresh = async (project?: string) => {
    await Promise.all([
      client.invalidateQueries({
        queryKey: KnowledgeStructureKeys.policy(session, scope),
      }),
      ...(project
        ? [
            client.invalidateQueries({
              queryKey: KnowledgeStructureKeys.head(session, {
                ...scope,
                project,
              }),
            }),
            client.invalidateQueries({
              queryKey: KnowledgeStructureKeys.manifestHead(session, {
                ...scope,
                project,
              }),
            }),
            client.invalidateQueries({
              queryKey: KnowledgeStructureKeys.tasks(
                session,
                { ...scope, project },
                20,
              ),
            }),
          ]
        : []),
    ]);
  };
  return {
    run,
    write,
    recover,
    task,
    prepare,
    refresh,
    isPending: mutation.isPending,
    error: mutation.error,
  };
}

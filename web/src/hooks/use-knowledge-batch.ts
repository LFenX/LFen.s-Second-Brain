import { useEffect, useRef } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import service, {
  BatchRequestError,
  type BatchResponse,
} from '@/services/knowledge-batch-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import type { BatchTask } from '@/interfaces/database/knowledge-batch';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from './use-knowledge-foundation';
import { batchTerminal } from '@/utils/knowledge-batch-schema';
import { latestBatchTask } from '@/utils/knowledge-batch';

export const KnowledgeBatchKeys = {
  contract: (session: number) =>
    ['knowledge-batch', session, 'contract'] as const,
  scope: (session: number, domain: string) =>
    ['knowledge-batch', session, domain] as const,
  registration: (session: number, domain: string, reg: string) =>
    ['knowledge-batch', session, domain, 'registration', reg] as const,
  action: (session: number, domain: string) =>
    ['knowledge-batch', session, domain, 'action'] as const,
  tasks: (session: number, domain: string, reg: string) =>
    ['knowledge-batch', session, domain, 'registration', reg, 'tasks'] as const,
  task: (session: number, domain: string, reg: string, task: string) =>
    [
      'knowledge-batch',
      session,
      domain,
      'registration',
      reg,
      'task',
      task,
    ] as const,
  items: (
    session: number,
    domain: string,
    reg: string,
    task: string,
    revision: number,
  ) =>
    [
      'knowledge-batch',
      session,
      domain,
      'registration',
      reg,
      'task',
      task,
      'items',
      revision,
    ] as const,
  head: (session: number, domain: string, project: string) =>
    ['knowledge-batch', session, domain, 'project', project, 'head'] as const,
  history: (session: number, domain: string, project: string) =>
    [
      'knowledge-batch',
      session,
      domain,
      'project',
      project,
      'history',
    ] as const,
  manifest: (
    session: number,
    domain: string,
    project: string,
    manifest: string,
  ) =>
    [
      'knowledge-batch',
      session,
      domain,
      'project',
      project,
      'manifest',
      manifest,
    ] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
} as const;
export function batchScopeFailure(error: unknown) {
  return (
    error instanceof KnowledgeRequestError &&
    ([401, 403, 404].includes(error.status) ||
      [
        'knowledge_batch_version_mismatch',
        'knowledge_directory_version_mismatch',
        'knowledge_contract_version',
      ].includes(error.code))
  );
}
async function scoped<T>(
  session: number,
  work: () => Promise<BatchResponse<T>>,
): Promise<BatchResponse<T>> {
  if (session !== getKnowledgeSessionScope())
    throw new BatchRequestError(401, 'knowledge_unauthorized');
  const value = await work();
  if (session !== getKnowledgeSessionScope())
    throw new BatchRequestError(401, 'knowledge_unauthorized');
  return value;
}
export function useBatchActions(domain: string) {
  const session = getKnowledgeSessionScope(),
    mutation = useMutation({
      ...Options,
      mutationKey: KnowledgeBatchKeys.action(session, domain),
      mutationFn: (work: () => Promise<unknown>) =>
        scoped(session, work as () => Promise<BatchResponse<unknown>>),
    });
  async function run<T>(work: () => Promise<BatchResponse<T>>) {
    return (await mutation.mutateAsync(work)) as BatchResponse<T>;
  }
  return { run, reset: mutation.reset };
}
export function useBatchContract() {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeBatchKeys.contract(session),
    queryFn: async ({ signal }) =>
      (await scoped(session, () => service.contract(signal))).data,
  });
}
export function useBatchTask(domain: string, reg: string, taskId: string) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    failures = useRef(0),
    latest = useRef<BatchTask | null>(null),
    invalidated = useRef('');
  const query = useQuery({
    ...Options,
    queryKey: KnowledgeBatchKeys.task(session, domain, reg, taskId),
    queryFn: async ({ signal }) => {
      try {
        const result = await scoped(session, () =>
          service.task(domain, reg, taskId, signal),
        );
        latest.current = latestBatchTask(latest.current, result.data);
        failures.current = 0;
        return latest.current;
      } catch (error) {
        failures.current += 1;
        throw error;
      }
    },
    refetchInterval: (value) =>
      !value.state.data ||
      batchTerminal(value.state.data.status) ||
      batchScopeFailure(value.state.error) ||
      failures.current >= 3
        ? false
        : 2000,
    refetchIntervalInBackground: false,
  });
  useEffect(() => {
    const task = query.data,
      key = task ? task.task_id + ':' + task.revision : '';
    if (task && task.counts.succeeded > 0 && invalidated.current !== key) {
      invalidated.current = key;
      void client.invalidateQueries({
        queryKey: KnowledgeKeys.domainScope(session, domain),
      });
    }
  }, [query.data, client, domain, session]);
  function recheck() {
    failures.current = 0;
    return query.refetch();
  }
  function adopt(value: BatchTask) {
    latest.current = latestBatchTask(latest.current, value);
    client.setQueryData(
      KnowledgeBatchKeys.task(session, domain, reg, taskId),
      latest.current,
    );
  }
  return { ...query, recheck, adopt, pollingStopped: failures.current >= 3 };
}
export function useBatchTasks(domain: string, reg: string) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    key = KnowledgeBatchKeys.tasks(session, domain, reg);
  const query = useInfiniteQuery({
    ...Options,
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scoped(session, () =>
          service.tasks(domain, reg, pageParam, signal),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  function restart() {
    return client.resetQueries({ queryKey: key, exact: true });
  }
  return { ...query, restart };
}
export function useBatchItems(domain: string, reg: string, task: BatchTask) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    key = KnowledgeBatchKeys.items(
      session,
      domain,
      reg,
      task.task_id,
      task.revision,
    );
  const query = useInfiniteQuery({
    ...Options,
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scoped(session, () =>
          service.items(domain, reg, task.task_id, pageParam, signal),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  function restart() {
    return client.resetQueries({ queryKey: key, exact: true });
  }
  return { ...query, restart };
}
export function useBatchHead(domain: string, project: string) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeBatchKeys.head(session, domain, project),
    queryFn: async ({ signal }) =>
      (await scoped(session, () => service.head(domain, project, signal))).data,
  });
}
export function useBatchHistory(domain: string, project: string) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    key = KnowledgeBatchKeys.history(session, domain, project);
  const query = useInfiniteQuery({
    ...Options,
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scoped(session, () =>
          service.history(domain, project, pageParam, signal),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  function restart() {
    return client.resetQueries({ queryKey: key, exact: true });
  }
  return { ...query, restart };
}
export function useBatchManifest(
  domain: string,
  project: string,
  manifest: string,
) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    key = KnowledgeBatchKeys.manifest(session, domain, project, manifest);
  const query = useInfiniteQuery({
    ...Options,
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scoped(session, () =>
          service.manifest(domain, project, manifest, pageParam, signal),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
  function restart() {
    return client.resetQueries({ queryKey: key, exact: true });
  }
  return { ...query, restart };
}

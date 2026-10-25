import { useCallback, useEffect, useRef } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import service, {
  OriginalRequestError,
} from '@/services/knowledge-original-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import type { OriginalUpload } from '@/interfaces/database/knowledge-original';
import { latestOriginalUpload } from '@/utils/knowledge-original';
import { getKnowledgeSessionScope } from './use-knowledge-foundation';

export const KnowledgeOriginalKeys = {
  contract: (session: number) =>
    ['knowledge-original', session, 'contract'] as const,
  scope: (session: number, domain: string) =>
    ['knowledge-original', session, domain] as const,
  action: (session: number, domain: string) =>
    ['knowledge-original', session, domain, 'action'] as const,
  registration: (session: number, domain: string, reg: string) =>
    ['knowledge-original', session, domain, 'registration', reg] as const,
  target: (session: number, domain: string, reg: string, path: string) =>
    ['knowledge-original', session, domain, 'target', reg, path] as const,
  upload: (session: number, domain: string, reg: string, upload: string) =>
    ['knowledge-original', session, domain, 'upload', reg, upload] as const,
  receipt: (session: number, domain: string, reg: string, receipt: string) =>
    ['knowledge-original', session, domain, 'receipt', reg, receipt] as const,
  lists: (session: number, domain: string) =>
    ['knowledge-original', session, domain, 'lists'] as const,
  list: (session: number, domain: string, group?: string) =>
    ['knowledge-original', session, domain, 'lists', group] as const,
  revision: (session: number, domain: string, revision: string) =>
    ['knowledge-original', session, domain, 'revision', revision] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
} as const;
export function originalScopeFailure(error: unknown) {
  return (
    error instanceof KnowledgeRequestError &&
    ([401, 403, 404].includes(error.status) ||
      [
        'knowledge_contract_version',
        'knowledge_source_contract_version',
        'policy_changed',
      ].includes(error.code))
  );
}
export async function scopedOriginal<T>(
  session: number,
  work: () => Promise<T>,
): Promise<T> {
  if (session !== getKnowledgeSessionScope())
    throw new OriginalRequestError(401, 'knowledge_unauthorized');
  const value = await work();
  if (session !== getKnowledgeSessionScope())
    throw new OriginalRequestError(401, 'knowledge_unauthorized');
  return value;
}
export function useOriginalActions(domain: string) {
  const session = getKnowledgeSessionScope();
  const mutation = useMutation({
    ...Options,
    mutationKey: KnowledgeOriginalKeys.action(session, domain),
    mutationFn: (work: () => Promise<unknown>) => scopedOriginal(session, work),
  });
  async function run<T>(work: () => Promise<T>) {
    return (await mutation.mutateAsync(work)) as T;
  }
  return { run, reset: mutation.reset };
}
export function useOriginalContract() {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeOriginalKeys.contract(session),
    queryFn: async ({ signal }) =>
      (await scopedOriginal(session, () => service.contract(signal))).data,
  });
}
export function useOriginalUpload(
  domain: string,
  reg: string,
  uploadId: string,
  onFailure?: (error: unknown) => void,
) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const latest = useRef<OriginalUpload | null>(null),
    failures = useRef(0),
    context = useRef(''),
    failureHandler = useRef(onFailure);
  failureHandler.current = onFailure;
  const identity = session + ':' + domain + ':' + reg + ':' + uploadId;
  if (context.current !== identity) {
    context.current = identity;
    latest.current = null;
    failures.current = 0;
  }
  const query = useQuery({
    ...Options,
    queryKey: KnowledgeOriginalKeys.upload(session, domain, reg, uploadId),
    queryFn: async ({ signal }) => {
      try {
        const result = await scopedOriginal(session, () =>
          service.upload(domain, reg, uploadId, signal),
        );
        latest.current = latestOriginalUpload(latest.current, result.data);
        failures.current = 0;
        return latest.current;
      } catch (error) {
        if (
          error instanceof OriginalRequestError &&
          (error.status === 0 || error.status === 503)
        )
          failures.current += 1;
        else failures.current = 3;
        throw error;
      }
    },
    refetchInterval: (value) =>
      value.state.data?.state === 'receiving' &&
      !originalScopeFailure(value.state.error) &&
      failures.current < 3
        ? 2000
        : false,
    refetchIntervalInBackground: false,
  });
  useEffect(() => {
    if (query.error && originalScopeFailure(query.error))
      failureHandler.current?.(query.error);
  }, [query.error]);
  const adopt = useCallback(
    (value: OriginalUpload) => {
      latest.current = latestOriginalUpload(latest.current, value);
      client.setQueryData(
        KnowledgeOriginalKeys.upload(session, domain, reg, uploadId),
        latest.current,
      );
      return latest.current;
    },
    [client, session, domain, reg, uploadId],
  );
  function recheck() {
    failures.current = 0;
    return query.refetch();
  }
  return { ...query, adopt, recheck, pollingStopped: failures.current >= 3 };
}
export function useOriginalList(domain: string, group?: string) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    key = KnowledgeOriginalKeys.list(session, domain, group);
  const query = useInfiniteQuery({
    ...Options,
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedOriginal(session, () =>
          service.list(domain, group, pageParam, signal),
        )
      ).data,
    getNextPageParam: (last) => last.next_cursor || undefined,
    select: (data) => {
      const all = data.pages.flatMap((page) => page.items);
      const cursors = data.pages
        .map((page) => page.next_cursor)
        .filter(Boolean);
      if (
        new Set(cursors).size !== cursors.length ||
        new Set(all.map((item) => item.source_revision_id)).size !==
          all.length ||
        all.some(
          (item, n) =>
            (group && item.classification.group_id !== group) ||
            (n > 0 && all[n - 1].source_revision_id >= item.source_revision_id),
        )
      )
        throw new OriginalRequestError(502, 'original_invalid_response');
      return data;
    },
  });
  return {
    ...query,
    restart: () => client.resetQueries({ queryKey: key, exact: true }),
  };
}
export function useOriginalRevision(domain: string, revision: string) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeOriginalKeys.revision(session, domain, revision),
    queryFn: async ({ signal }) =>
      (
        await scopedOriginal(session, () =>
          service.revision(domain, revision, signal),
        )
      ).data,
  });
}

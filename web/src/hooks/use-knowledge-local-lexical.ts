import { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import service, {
  LocalLexicalRequestError,
} from '@/services/knowledge-local-lexical-service';
import contentService, {
  ContentRequestError,
} from '@/services/knowledge-consumption-service';
import { KnowledgeConsumptionKeys } from '@/hooks/use-knowledge-consumption';
import {
  contentSchemaValid,
  contentBytes,
  parseContentJSON,
} from '@/utils/knowledge-consumption';
import type { ContentPolicy } from '@/interfaces/database/knowledge-consumption';
import type {
  LocalLexicalEvidenceRef,
  LocalLexicalIndexRef,
  LocalLexicalJob,
  LocalLexicalPolicyReceipt,
  LocalLexicalSelection,
} from '@/interfaces/database/knowledge-local-lexical';
import type {
  LocalLexicalReadBody,
  LocalLexicalScope,
  LocalLexicalSearchBody,
  LocalLexicalWrite,
} from '@/interfaces/request/knowledge-local-lexical';
import {
  checkLocalLexicalRequest,
  localLexicalCanonical,
  localLexicalSelectionKey,
  LocalLexicalVersion,
} from '@/utils/knowledge-local-lexical';
import { getKnowledgeSessionScope } from './use-knowledge-foundation';

export const LocalLexicalKeys = {
  root: () => ['knowledge-content-lexical', '0.2.0-candidate'] as const,
  scope: (session: number, scope: LocalLexicalScope) =>
    [...LocalLexicalKeys.root(), session, scope.owner, scope.domain] as const,
  policy: (session: number, scope: LocalLexicalScope) =>
    [...LocalLexicalKeys.scope(session, scope), 'policy'] as const,
  readiness: (
    session: number,
    scope: LocalLexicalScope,
    selection: LocalLexicalSelection,
  ) =>
    [
      ...LocalLexicalKeys.scope(session, scope),
      'readiness',
      localLexicalSelectionKey(selection),
    ] as const,
  job: (session: number, scope: LocalLexicalScope, job: string) =>
    [...LocalLexicalKeys.scope(session, scope), 'job', job] as const,
  searches: (session: number, scope: LocalLexicalScope) =>
    [...LocalLexicalKeys.scope(session, scope), 'search'] as const,
  reads: (session: number, scope: LocalLexicalScope) =>
    [...LocalLexicalKeys.scope(session, scope), 'read'] as const,
  search: (
    session: number,
    scope: LocalLexicalScope,
    selection: LocalLexicalSelection,
    index: LocalLexicalIndexRef,
    query: string,
    limit: number,
  ) =>
    [
      ...LocalLexicalKeys.searches(session, scope),
      localLexicalSelectionKey(selection),
      index.index_id,
      index.manifest_sha256,
      query,
      limit,
    ] as const,
  read: (
    session: number,
    scope: LocalLexicalScope,
    evidence: LocalLexicalEvidenceRef,
  ) =>
    [
      ...LocalLexicalKeys.reads(session, scope),
      localLexicalCanonical(evidence),
    ] as const,
  action: (session: number, scope: LocalLexicalScope) =>
    [...LocalLexicalKeys.scope(session, scope), 'action'] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;
async function scoped<T>(session: number, work: () => Promise<T>): Promise<T> {
  const check = () => {
    if (session !== getKnowledgeSessionScope())
      throw new LocalLexicalRequestError(401, 'unauthenticated');
  };
  check();
  try {
    const result = await work();
    check();
    return result;
  } catch (error) {
    check();
    throw error;
  }
}
export function useLocalLexicalPolicy(scope: LocalLexicalScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: LocalLexicalKeys.policy(session, scope),
    queryFn: ({ signal }) =>
      scoped(session, () => service.policy(scope, signal)),
  });
}
export function useLocalLexicalReadiness(
  scope: LocalLexicalScope,
  selection: LocalLexicalSelection,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: LocalLexicalKeys.readiness(session, scope, selection),
    queryFn: async ({ signal }) => {
      const result = await scoped(session, () =>
        service.readiness(
          scope,
          { contract_version: LocalLexicalVersion, selection },
          signal,
        ),
      );
      return result;
    },
  });
}
export function useLocalLexicalJob(scope: LocalLexicalScope, job: string) {
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
    const timer = window.setTimeout(() => setWaiting(false), 25000);
    return () => window.clearTimeout(timer);
  }, [visible, waiting, cycle, job]);
  const query = useQuery({
    ...Options,
    queryKey: LocalLexicalKeys.job(session, scope, job),
    queryFn: ({ signal }) =>
      scoped(session, () => service.job(scope, job, signal)),
    refetchInterval: (value) =>
      visible &&
      waiting &&
      !value.state.error &&
      !['published', 'failed'].includes(value.state.data?.state ?? '')
        ? 2000
        : false,
    refetchIntervalInBackground: false,
  });
  const resume = () => {
    setCycle((value) => value + 1);
    setWaiting(true);
    void query.refetch();
  };
  return { ...query, waiting, resume };
}
type StoredWrite = {
  version: 2;
  owner: string;
  domain: string;
  write: LocalLexicalWrite;
};
function checkWrite(write: LocalLexicalWrite, scope: LocalLexicalScope) {
  if (
    !write ||
    typeof write !== 'object' ||
    Object.keys(write).sort().join(',') !== 'body,operation' ||
    !['changeLocalPolicy', 'createIndex', 'changeContentPolicy'].includes(
      write.operation,
    )
  )
    throw new LocalLexicalRequestError(400, 'invalid_request', '', true, false);
  if (write.operation === 'changeContentPolicy') {
    if (
      !/^[a-f0-9]{32}$/.test(scope.owner) ||
      !/^[a-f0-9]{32}$/.test(scope.domain) ||
      !contentSchemaValid('HTTPContentPolicyRequest', write.body) ||
      contentBytes(JSON.stringify(write.body)) > 32768
    )
      throw new LocalLexicalRequestError(
        400,
        'invalid_request',
        '',
        true,
        false,
      );
  } else checkLocalLexicalRequest(write.operation, scope, write.body);
}
export function useLocalLexicalActions(scope: LocalLexicalScope) {
  const { owner, domain } = scope;
  const session = getKnowledgeSessionScope(),
    client = useQueryClient(),
    mounted = useRef(true),
    lock = useRef(false),
    saved = useRef<LocalLexicalWrite | null>(null),
    [pending, setPending] = useState<LocalLexicalWrite | null>(null),
    [lastJob, setLastJob] = useState<LocalLexicalJob | null>(null),
    [storageReady, setStorageReady] = useState(false),
    [storageError, setStorageError] = useState<unknown>(null);
  const storageKey =
    'knowledge-content-lexical:2:' +
    session +
    ':' +
    scope.owner +
    ':' +
    scope.domain;
  const current = () =>
    mounted.current && session === getKnowledgeSessionScope();
  useEffect(() => {
    mounted.current = true;
    saved.current = null;
    setPending(null);
    setLastJob(null);
    setStorageReady(false);
    setStorageError(null);
    try {
      const raw = sessionStorage.getItem(storageKey);
      if (raw) {
        const value = parseContentJSON(raw) as StoredWrite;
        if (
          value.version !== 2 ||
          value.owner !== owner ||
          value.domain !== domain ||
          !['changeLocalPolicy', 'createIndex', 'changeContentPolicy'].includes(
            value.write?.operation,
          ) ||
          Object.keys(value).sort().join(',') !== 'domain,owner,version,write'
        )
          throw Error('Invalid saved request');
        checkWrite(value.write, { owner, domain });
        saved.current = value.write;
        setPending(value.write);
      }
      setStorageReady(true);
    } catch {
      setStorageError(
        new LocalLexicalRequestError(0, 'local_lexical_storage_failed'),
      );
    }
    return () => {
      mounted.current = false;
      void client.cancelQueries({
        queryKey: LocalLexicalKeys.scope(session, { owner, domain }),
      });
      client.removeQueries({
        queryKey: LocalLexicalKeys.scope(session, { owner, domain }),
      });
    };
  }, [client, session, owner, domain, storageKey]);
  const serialize = (write: LocalLexicalWrite) =>
    JSON.stringify({
      version: 2,
      owner: scope.owner,
      domain: scope.domain,
      write,
    } satisfies StoredWrite);
  const clear = (write: LocalLexicalWrite) => {
    if (!current()) return;
    if (sessionStorage.getItem(storageKey) !== serialize(write))
      throw new LocalLexicalRequestError(0, 'local_lexical_storage_failed');
    sessionStorage.removeItem(storageKey);
    saved.current = null;
    setPending(null);
  };
  const mutation = useMutation({
    ...Options,
    mutationKey: LocalLexicalKeys.action(session, scope),
    mutationFn: (write: LocalLexicalWrite) =>
      scoped<LocalLexicalPolicyReceipt | LocalLexicalJob | ContentPolicy>(
        session,
        () =>
          write.operation === 'changeLocalPolicy'
            ? service.changePolicy(scope, write.body)
            : write.operation === 'createIndex'
              ? service.createJob(scope, write.body)
              : contentService.changePolicy(scope, write.body.request),
      ),
  });
  const refresh = async () => {
    if (!current()) return;
    await client.invalidateQueries({
      queryKey: LocalLexicalKeys.scope(session, scope),
    });
    await client.invalidateQueries({
      queryKey: KnowledgeConsumptionKeys.policy(session, scope),
    });
  };
  const send = async (write: LocalLexicalWrite) => {
    if (lock.current || !current())
      throw new LocalLexicalRequestError(0, 'local_lexical_pending_request');
    lock.current = true;
    try {
      const result = await mutation.mutateAsync(write);
      clear(write);
      if (current() && 'job_id' in result) setLastJob(result);
      await refresh();
      return result;
    } catch (error) {
      if (
        current() &&
        (error instanceof LocalLexicalRequestError ||
          error instanceof ContentRequestError) &&
        error.outcomeKnown &&
        error.code !== 'idempotency_conflict'
      )
        clear(write);
      throw error;
    } finally {
      lock.current = false;
    }
  };
  const submit = async (write: LocalLexicalWrite) => {
    if (!storageReady || saved.current || lock.current)
      throw new LocalLexicalRequestError(0, 'local_lexical_pending_request');
    checkWrite(write, scope);
    const snapshot = JSON.parse(serialize(write)) as StoredWrite;
    try {
      sessionStorage.setItem(storageKey, serialize(snapshot.write));
    } catch {
      throw new LocalLexicalRequestError(0, 'local_lexical_storage_failed');
    }
    saved.current = snapshot.write;
    setPending(snapshot.write);
    return send(snapshot.write);
  };
  const resend = async () => {
    if (
      !storageReady ||
      !saved.current ||
      sessionStorage.getItem(storageKey) !== serialize(saved.current)
    )
      throw new LocalLexicalRequestError(0, 'local_lexical_storage_failed');
    return send(saved.current);
  };
  const search = (body: LocalLexicalSearchBody) =>
    client.fetchQuery({
      ...Options,
      queryKey: LocalLexicalKeys.search(
        session,
        scope,
        body.selection,
        body.index_ref,
        body.query,
        body.limit,
      ),
      queryFn: ({ signal }) =>
        scoped(session, () => service.search(scope, body, signal)),
    });
  const read = (body: LocalLexicalReadBody) =>
    client.fetchQuery({
      ...Options,
      queryKey: LocalLexicalKeys.read(session, scope, body.evidence_ref),
      queryFn: ({ signal }) =>
        scoped(session, () => service.read(scope, body, signal)),
    });
  return {
    pending,
    lastJob,
    storageReady,
    storageError,
    submit,
    resend,
    refresh,
    search,
    read,
    busy: mutation.isPending,
    error: mutation.error,
    session,
  };
}
export type LocalLexicalActions = ReturnType<typeof useLocalLexicalActions>;

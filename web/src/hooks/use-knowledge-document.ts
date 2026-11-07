import { useEffect, useRef, useState } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import service, {
  DocumentRequestError,
  type DocumentResponse,
} from '@/services/knowledge-document-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { getKnowledgeSessionScope } from './use-knowledge-foundation';
import type {
  DocumentRepresentation,
  DocumentNode,
  DocumentTask,
} from '@/interfaces/database/knowledge-document';
import type { DocumentContentQuery } from '@/interfaces/request/knowledge-document';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import {
  documentTerminal,
  latestDocumentTask,
  type DocumentScope,
  type DocumentSourceScope,
  type DocumentViewScope,
  type DocumentOperation,
  type DocumentOutcome,
} from '@/utils/knowledge-document';

export const KnowledgeDocumentKeys = {
  root: () => ['knowledge-document', '0.1.0'] as const,
  contract: (session: number) =>
    [...KnowledgeDocumentKeys.root(), session, 'contract'] as const,
  scope: (session: number, s: DocumentScope) =>
    [...KnowledgeDocumentKeys.root(), session, s.owner, s.domain] as const,
  policy: (session: number, s: DocumentScope) =>
    [...KnowledgeDocumentKeys.scope(session, s), 'policy'] as const,
  originals: (session: number, s: DocumentScope) =>
    [...KnowledgeDocumentKeys.scope(session, s), 'originals'] as const,
  head: (session: number, s: DocumentSourceScope) =>
    [...KnowledgeDocumentKeys.scope(session, s), 'head', s.source] as const,
  task: (session: number, s: DocumentScope, task: string) =>
    [...KnowledgeDocumentKeys.scope(session, s), 'task', task] as const,
  view: (session: number, s: DocumentViewScope) =>
    [
      ...KnowledgeDocumentKeys.scope(session, s),
      'representation',
      s.source,
      s.revision,
      s.sha,
      s.representation,
    ] as const,
  node: (session: number, s: DocumentViewScope, node: string) =>
    [...KnowledgeDocumentKeys.view(session, s), 'node', node] as const,
  children: (
    session: number,
    s: DocumentViewScope,
    parent: string,
    limit: number,
  ) =>
    [
      ...KnowledgeDocumentKeys.view(session, s),
      'children',
      parent,
      limit,
    ] as const,
  content: (
    session: number,
    s: DocumentViewScope,
    node: DocumentNode,
    q: DocumentContentQuery,
  ) =>
    [
      ...KnowledgeDocumentKeys.node(session, s, node.document_node_id),
      'content',
      node.payload.reading_range.text_sha256,
      q.start_byte ?? null,
      q.end_byte ?? null,
    ] as const,
  action: (session: number, s: DocumentScope) =>
    [...KnowledgeDocumentKeys.scope(session, s), 'action'] as const,
  original: (session: number, s: DocumentViewScope) =>
    [...KnowledgeDocumentKeys.view(session, s), 'original'] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;
export const documentScopeFailure = (e: unknown) =>
  e instanceof KnowledgeRequestError && [401, 403, 404].includes(e.status);
export async function scopedDocument<T>(
  session: number,
  work: () => Promise<T>,
): Promise<T> {
  if (session !== getKnowledgeSessionScope())
    throw new DocumentRequestError(401, 'unauthorized');
  try {
    const result = await work();
    if (session !== getKnowledgeSessionScope())
      throw new DocumentRequestError(401, 'unauthorized');
    return result;
  } catch (error) {
    if (session !== getKnowledgeSessionScope())
      throw new DocumentRequestError(401, 'unauthorized');
    throw error;
  }
}
export function useDocumentPrivateError(error: unknown) {
  const client = useQueryClient();
  useEffect(() => {
    if (documentScopeFailure(error)) {
      void client.cancelQueries({ queryKey: KnowledgeDocumentKeys.root() });
      client.removeQueries({ queryKey: KnowledgeDocumentKeys.root() });
    }
  }, [client, error]);
}
export function useDocumentContract() {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.contract(session),
    queryFn: async ({ signal }) =>
      (await scopedDocument(session, () => service.contract(signal))).data,
  });
}
export function useDocumentPolicy(s: DocumentScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.policy(session, s),
    queryFn: async ({ signal }) =>
      (await scopedDocument(session, () => service.policy(s, signal))).data,
  });
}
export function useDocumentOriginals(s: DocumentScope) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.originals(session, s),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedDocument(session, () =>
          service.originals(s, pageParam, signal),
        )
      ).data,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  });
}
export function useDocumentHead(s: DocumentSourceScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.head(session, s),
    queryFn: async ({ signal }) =>
      (await scopedDocument(session, () => service.head(s, signal))).data,
  });
}
export function useDocumentRepresentation(s: DocumentViewScope) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.view(session, s),
    queryFn: async ({ signal }) =>
      (await scopedDocument(session, () => service.representation(s, signal)))
        .data,
  });
}
export function useDocumentNode(
  s: DocumentViewScope,
  rep: DocumentRepresentation,
  node: string,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.node(session, s, node),
    queryFn: async ({ signal }) =>
      (await scopedDocument(session, () => service.node(s, rep, node, signal)))
        .data,
  });
}
export function useDocumentFixedOriginal(
  s: DocumentViewScope,
  rep: DocumentRepresentation,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.original(session, s),
    queryFn: async ({ signal }) =>
      (
        await scopedDocument(session, () =>
          service.fixedOriginal(s, rep, signal),
        )
      ).data,
  });
}
export function useDocumentChildren(
  s: DocumentViewScope,
  rep: DocumentRepresentation,
  parent: DocumentNode,
) {
  const session = getKnowledgeSessionScope();
  return useInfiniteQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.children(
      session,
      s,
      parent.document_node_id,
      20,
    ),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      (
        await scopedDocument(session, () =>
          service.nodes(
            s,
            rep,
            parent,
            {
              parent_id: parent.document_node_id,
              limit: 20,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
            signal,
          ),
        )
      ).data,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    select: (data) => {
      const seen = new Set<string>();
      let ordinal = -1;
      for (const page of data.pages)
        for (const node of page.items) {
          if (
            seen.has(node.document_node_id) ||
            node.payload.ordinal <= ordinal
          )
            throw new DocumentRequestError(502, 'document_integrity');
          seen.add(node.document_node_id);
          ordinal = node.payload.ordinal;
        }
      return data;
    },
  });
}
export function useDocumentContent(
  s: DocumentViewScope,
  rep: DocumentRepresentation,
  node: DocumentNode,
  q: DocumentContentQuery,
) {
  const session = getKnowledgeSessionScope();
  return useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.content(session, s, node, q),
    queryFn: async ({ signal }) =>
      (
        await scopedDocument(session, () =>
          service.content(s, rep, node, q, signal),
        )
      ).data,
  });
}
export function useDocumentTask(s: DocumentScope, task: string) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const { owner, domain } = s;
  const latest = useRef<{ key: string; value: DocumentTask | null }>({
      key: '',
      value: null,
    }),
    identity = [session, s.owner, s.domain, task].join(':');
  if (latest.current.key !== identity)
    latest.current = { key: identity, value: null };
  const [visible, setVisible] = useState(
      document.visibilityState === 'visible',
    ),
    [waiting, setWaiting] = useState(true),
    [cycle, setCycle] = useState(0);
  useEffect(() => {
    const changed = () => setVisible(document.visibilityState === 'visible');
    document.addEventListener('visibilitychange', changed);
    return () => document.removeEventListener('visibilitychange', changed);
  }, []);
  useEffect(() => {
    if (!waiting) return;
    const timer = window.setTimeout(() => setWaiting(false), 25000);
    return () => window.clearTimeout(timer);
  }, [waiting, cycle, task]);
  useEffect(() => {
    if (!visible || !waiting)
      void client.cancelQueries({
        queryKey: KnowledgeDocumentKeys.task(session, { owner, domain }, task),
      });
  }, [client, session, owner, domain, task, visible, waiting]);
  const query = useQuery({
    ...Options,
    queryKey: KnowledgeDocumentKeys.task(session, s, task),
    enabled:
      visible &&
      waiting &&
      (!latest.current.value || !documentTerminal(latest.current.value)),
    queryFn: async ({ signal }) => {
      const data = (
        await scopedDocument(session, () => service.task(s, task, signal))
      ).data;
      const value = latestDocumentTask(latest.current.value, data);
      latest.current = { key: identity, value };
      return value;
    },
    refetchInterval: (q) =>
      visible &&
      waiting &&
      !q.state.error &&
      (!q.state.data || !documentTerminal(q.state.data))
        ? 2000
        : false,
    refetchIntervalInBackground: false,
  });
  const resume = () => {
    setCycle((value) => value + 1);
    setWaiting(true);
  };
  return { ...query, waiting, resume };
}
export function useDocumentActions(s: DocumentScope) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const controllers = useRef(new Set<AbortController>()),
    mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    const ownedControllers = controllers.current;
    return () => {
      mounted.current = false;
      for (const controller of ownedControllers) controller.abort();
      ownedControllers.clear();
    };
  }, [session, s.owner, s.domain]);
  const mutation = useMutation({
    ...Options,
    mutationKey: KnowledgeDocumentKeys.action(session, s),
    mutationFn: (work: () => Promise<unknown>) => scopedDocument(session, work),
  });
  async function run<T>(work: (signal: AbortSignal) => Promise<T>): Promise<T> {
    if (!mounted.current)
      throw new DocumentRequestError(0, 'document_scope_closed');
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      const value = await mutation.mutateAsync(() => {
        if (controller.signal.aborted)
          throw new DocumentRequestError(0, 'document_scope_closed');
        return work(controller.signal);
      });
      if (controller.signal.aborted || !mounted.current)
        throw new DocumentRequestError(0, 'document_scope_closed');
      return value as T;
    } finally {
      controllers.current.delete(controller);
    }
  }
  const write = (op: DocumentOperation) =>
    run<DocumentResponse<DocumentOutcome>>((signal) =>
      op.kind === 'task'
        ? service.createTask(op.scope, op.body, signal)
        : op.kind === 'policy'
          ? service.changePolicy(op.scope, op.body, signal)
          : service.cancel(op.scope, op.body, signal),
    );
  const recover = (kind: 'task' | 'policy' | 'cancel', key: string) =>
    run((signal) => service.byKey(s, { kind, idempotency_key: key }, signal));
  const task = (taskId: string) =>
    run((signal) => service.task(s, taskId, signal));
  const prepare = (source: DocumentSourceScope, selected: OriginalRevision) =>
    run((signal) => service.prepare(source, selected, signal));
  const original = (revision: string) =>
    run((signal) => service.original(s, revision, signal));
  const download = (view: DocumentViewScope, rep: DocumentRepresentation) =>
    run((signal) => service.download(view, rep, signal));
  const refresh = async (source?: string) => {
    await Promise.all([
      client.invalidateQueries(
        { queryKey: KnowledgeDocumentKeys.policy(session, s) },
        { throwOnError: true },
      ),
      ...(source
        ? [
            client.invalidateQueries(
              {
                queryKey: KnowledgeDocumentKeys.head(session, { ...s, source }),
              },
              { throwOnError: true },
            ),
          ]
        : []),
    ]);
  };
  return { write, recover, task, prepare, original, download, refresh, run };
}

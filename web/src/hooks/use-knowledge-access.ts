import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import {
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';
import type {
  AccessActor,
  AccessInspectData,
  AccessModuleSummary,
  AccessReadData,
  AccessSearchData,
  AccessMode,
  AccessEvidenceRef,
} from '@/interfaces/database/knowledge-access';
import { KnowledgeAccessVersion } from '@/interfaces/database/knowledge-access';
import type { AccessRequests } from '@/interfaces/request/knowledge-access';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import nativeService from '@/services/knowledge-access-native-service';
import {
  nativeSessionRevision,
  subscribeNativeSession,
} from '@/utils/native-session-scope';
import { accessCanonical } from '@/utils/knowledge-access-schema';
import { KnowledgeAccessKeys as Keys } from './knowledge-access-keys';

const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: true,
} as const;
async function metadata<T>(
  actor: AccessActor,
  signal: AbortSignal,
  request: () => Promise<T>,
) {
  const check = () => {
    if (signal.aborted || actor.session !== nativeSessionRevision())
      throw new AccessRequestError(401, 'unauthenticated', true);
  };
  check();
  const value = await request();
  check();
  return value;
}
export function clearSharedAccessContent(client: QueryClient) {
  const predicate = (query: { queryKey: readonly unknown[] }) =>
    Keys.isConsumer(query.queryKey);
  void client.cancelQueries({ predicate });
  client.removeQueries({ predicate });
}
export function useAccessIdentity(enabled = true) {
  const session = useSyncExternalStore(
    subscribeNativeSession,
    nativeSessionRevision,
    nativeSessionRevision,
  );
  const client = useQueryClient();
  useLayoutEffect(() => {
    clearSharedAccessContent(client);
    return () => clearSharedAccessContent(client);
  }, [client, session]);
  const query = useQuery({
    ...Options,
    queryKey: Keys.identity(session),
    enabled,
    queryFn: async ({ signal }) => {
      const value = await nativeService.identity(signal);
      if (nativeSessionRevision() !== session || signal.aborted)
        throw new AccessRequestError(401, 'unauthenticated', true);
      return value;
    },
  });
  return { ...query, session };
}
export function useAccessDepartments(actor: AccessActor) {
  return useQuery({
    ...Options,
    queryKey: Keys.departments(actor),
    queryFn: ({ signal }) =>
      metadata(actor, signal, () =>
        service.listDepartments(
          { contract_version: KnowledgeAccessVersion, tenant_id: actor.tenant },
          signal,
        ),
      ),
  });
}
export function useAccessDepartmentMembers(actor: AccessActor) {
  return useQuery({
    ...Options,
    queryKey: Keys.members(actor),
    queryFn: ({ signal }) =>
      metadata(actor, signal, () => nativeService.members(actor, signal)),
  });
}
export function useAccessModules(actor: AccessActor, domain: string | null) {
  return useQuery({
    ...Options,
    queryKey: Keys.modules(actor, domain),
    queryFn: ({ signal }) =>
      metadata(actor, signal, () =>
        service.listModules(
          {
            contract_version: KnowledgeAccessVersion,
            tenant_id: actor.tenant,
            domain_id: domain,
          },
          signal,
        ),
      ),
  });
}
export function useAccessModuleAdmin(
  actor: AccessActor,
  domain: string,
  module: string,
) {
  return useQuery({
    ...Options,
    queryKey: Keys.admin(actor, domain, module),
    enabled: !!module,
    queryFn: ({ signal }) =>
      metadata(actor, signal, () =>
        service.getModuleAdmin(
          {
            contract_version: KnowledgeAccessVersion,
            domain_id: domain,
            module_id: module,
          },
          signal,
        ),
      ),
  });
}
type ReaderState = {
  scope: string;
  view: AccessInspectData | null;
  results: AccessSearchData | null;
  evidence: AccessReadData | null;
  error: unknown;
  busy: boolean;
};
function emptyReader(scope: string): ReaderState {
  return {
    scope,
    view: null,
    results: null,
    evidence: null,
    error: null,
    busy: false,
  };
}
export function useSharedAccessReader(
  actor: AccessActor,
  module: AccessModuleSummary | null,
) {
  const client = useQueryClient();
  const session = useSyncExternalStore(
    subscribeNativeSession,
    nativeSessionRevision,
    nativeSessionRevision,
  );
  const boundary = accessCanonical([
    actor,
    session,
    module?.domain_id,
    module?.module_id,
    module?.version,
    module?.modes,
    module?.enabled,
  ]);
  const current = useRef({ boundary, cycle: 0, mounted: true });
  current.current.boundary = boundary;
  const request = useRef<AbortController | null>(null);
  const activeView = useRef<AccessInspectData | null>(null);
  const empty = () => emptyReader(boundary);
  const [state, setState] = useState<ReaderState>(empty);
  const { user, tenant, session: actorSession } = actor;
  const moduleId = module?.module_id ?? '';
  const consumerKey = useMemo(
    () =>
      Keys.consumer(
        { user, tenant, session: actorSession, auth: 'native' },
        moduleId,
      ),
    [user, tenant, actorSession, moduleId],
  );
  const clear = useCallback(
    (error: unknown = null) => {
      current.current.cycle += 1;
      request.current?.abort();
      activeView.current = null;
      void client.cancelQueries({ queryKey: consumerKey });
      client.removeQueries({ queryKey: consumerKey });
      if (current.current.mounted)
        setState({ ...emptyReader(boundary), error });
    },
    [boundary, client, consumerKey],
  );
  function forgetOutput() {
    current.current.cycle += 1;
    request.current?.abort();
    void client.cancelQueries({ queryKey: consumerKey });
    client.removeQueries({
      predicate: (query) =>
        accessSamePrefix(query.queryKey, consumerKey) &&
        query.queryKey.at(-1) !== 'view',
    });
    setState((value) => ({
      ...value,
      results: null,
      evidence: null,
      error: null,
      busy: false,
    }));
  }
  useLayoutEffect(() => {
    const lifetime = current.current;
    lifetime.mounted = true;
    clear();
    return () => {
      lifetime.mounted = false;
      lifetime.cycle += 1;
      request.current?.abort();
      activeView.current = null;
      void client.cancelQueries({ queryKey: consumerKey });
      client.removeQueries({ queryKey: consumerKey });
    };
  }, [client, clear, consumerKey]);
  useEffect(
    () =>
      client.getQueryCache().subscribe((event) => {
        const view = activeView.current;
        if (
          event.type === 'removed' &&
          view &&
          accessSamePrefix(event.query.queryKey, consumerKey) &&
          event.query.queryKey[7] === view.module_version &&
          event.query.queryKey[8] === view.view_id &&
          event.query.queryKey.at(-1) === 'view'
        )
          clear(new AccessRequestError(409, 'scope_changed', true));
      }),
    [client, clear, consumerKey],
  );
  const view = state.scope === boundary ? state.view : null;
  useEffect(() => {
    if (!view) return;
    const timer = setTimeout(
      () => clear(new AccessRequestError(409, 'view_stale', true)),
      Math.max(0, Date.parse(view.expires_at) - Date.now()),
    );
    return () => clearTimeout(timer);
  }, [view, clear]);
  async function run<T>(
    work: (signal: AbortSignal, check: () => void) => Promise<T>,
    commit: (value: T) => void,
  ) {
    const expected = boundary,
      cycle = ++current.current.cycle;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const check = () => {
      if (
        !current.current.mounted ||
        current.current.boundary !== expected ||
        current.current.cycle !== cycle ||
        controller.signal.aborted ||
        nativeSessionRevision() !== actor.session
      )
        throw new AccessRequestError(409, 'scope_changed', true);
    };
    setState((value) => ({
      ...value,
      scope: boundary,
      results: null,
      evidence: null,
      error: null,
      busy: true,
    }));
    try {
      check();
      const value = await work(controller.signal, check);
      check();
      commit(value);
      setState((previous) => ({ ...previous, busy: false }));
    } catch (error) {
      if (
        current.current.mounted &&
        current.current.boundary === expected &&
        current.current.cycle === cycle
      )
        clear(error);
    }
  }
  async function inspect(mode: AccessMode, sourceIds: string[] | null = null) {
    clear();
    if (
      !module?.enabled ||
      (mode === 'read_search' && !module.modes.includes('read_search')) ||
      !module.modes.length
    )
      return;
    await run(
      async (signal, check) => {
        const data = await service.inspect(
          {
            contract_version: KnowledgeAccessVersion,
            domain_id: module.domain_id,
            module_id: module.module_id,
            mode,
            delivery: 'web_only',
            source_ids: sourceIds,
          },
          signal,
        );
        check();
        if (
          data.module_version !== module.version ||
          Date.parse(data.expires_at) <= Date.now()
        )
          throw new AccessRequestError(409, 'view_stale', true);
        return data;
      },
      (value) => {
        activeView.current = value;
        client.setQueryData(
          Keys.view(actor, module.module_id, module.version, value.view_id),
          value,
        );
        setState({ ...empty(), view: value });
      },
    );
  }
  function liveView() {
    const value = activeView.current;
    if (
      !value ||
      state.scope !== boundary ||
      Date.parse(value.expires_at) <= Date.now()
    ) {
      clear(new AccessRequestError(409, 'view_stale', true));
      return null;
    }
    return value;
  }
  async function read(selector: AccessRequests['read']['selector']) {
    const value = liveView();
    if (!value) return;
    await run(
      (signal, check) =>
        client.fetchQuery({
          queryKey: Keys.read(
            actor,
            value.module_id,
            value.module_version,
            value.view_id,
            accessCanonical(selector),
          ),
          retry: false,
          staleTime: 0,
          gcTime: 0,
          queryFn: async () => {
            check();
            const result = await service.read(
              { contract_version: KnowledgeAccessVersion, selector },
              value,
              signal,
            );
            check();
            return result;
          },
        }),
      (data) => setState((previous) => ({ ...previous, evidence: data })),
    );
  }
  async function search(query: string, limit = 20) {
    const value = liveView();
    if (!value || value.mode !== 'read_search') return;
    await run(
      (signal, check) =>
        client.fetchQuery({
          queryKey: Keys.search(
            actor,
            value.module_id,
            value.module_version,
            value.view_id,
            query,
            limit,
          ),
          retry: false,
          staleTime: 0,
          gcTime: 0,
          queryFn: async () => {
            check();
            const result = await service.search(
              {
                contract_version: KnowledgeAccessVersion,
                view_id: value.view_id,
                query,
                limit,
              },
              value,
              signal,
            );
            check();
            return result;
          },
        }),
      (data) => setState((previous) => ({ ...previous, results: data })),
    );
  }
  const reread = (evidence: AccessEvidenceRef) =>
    read({ kind: 'evidence', evidence_ref: evidence });
  return {
    ...(state.scope === boundary ? state : empty()),
    view,
    inspect,
    read,
    reread,
    search,
    clear,
    forgetOutput,
  };
}

function accessSamePrefix(key: readonly unknown[], prefix: readonly unknown[]) {
  return prefix.every((value, index) => key[index] === value);
}

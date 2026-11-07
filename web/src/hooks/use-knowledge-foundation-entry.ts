import { useLayoutEffect, useRef, useState } from 'react';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import type {
  ComponentSide,
  KnowledgeDomain,
  KnowledgeGroup,
} from '@/interfaces/database/knowledge-foundation';
import type { CreateKnowledgeGroupRequest } from '@/interfaces/request/knowledge-foundation';
import service, {
  KnowledgeRequestError,
} from '@/services/knowledge-foundation-service';
import { validKnowledgeData } from '@/utils/knowledge-foundation-schema';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from './use-knowledge-foundation';

export interface KnowledgeEntryIssue {
  kind: 'failure' | 'unknown';
  code: string;
  requestId: string;
}

const IdPattern = /^[a-f0-9]{32}$/;
const EntryKeys = {
  datasets: (epoch: number, ownerId: string) =>
    ['knowledge-entry-datasets', epoch, ownerId] as const,
};

export function useKnowledgeEntryDatasets(ownerId: string) {
  return useInfiniteQuery({
    queryKey: EntryKeys.datasets(getKnowledgeSessionScope(), ownerId),
    initialPageParam: 1,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    queryFn: ({ pageParam, signal }) =>
      service.datasets(ownerId, pageParam, signal),
    getNextPageParam: (page, pages) =>
      pages.length * 20 < page.total ? pages.length + 1 : undefined,
  });
}
function invalidReceipt(): never {
  throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
}
function issueOf(error: unknown): KnowledgeEntryIssue {
  const failure = error as Partial<KnowledgeRequestError>;
  const known = [400, 401, 403, 404, 409, 413, 415, 422].includes(
    failure.status || 0,
  );
  return {
    kind:
      known && failure.code !== 'knowledge_contract_version'
        ? 'failure'
        : 'unknown',
    code: String(failure.code || 'knowledge_unavailable'),
    requestId: failure.requestId || '',
  };
}

interface EntryState {
  busy: boolean;
  issue: KnowledgeEntryIssue | null;
}
interface AttemptContext<S> {
  current: () => boolean;
  patch: (next: Partial<S>) => void;
  state: () => S;
  epoch: number;
}
function useEntryState<S extends EntryState>(
  ownerId: string,
  domainId: string,
  initial: S,
) {
  const epoch = getKnowledgeSessionScope();
  const scope = JSON.stringify([epoch, ownerId, domainId]);
  const currentScope = useRef(scope);
  currentScope.current = scope;
  const revision = useRef(0);
  const alive = useRef(false);
  const mutex = useRef(false);
  const [record, setRecord] = useState({ scope, value: initial });
  const state = useRef(record);
  if (state.current.scope !== scope) state.current = { scope, value: initial };
  useLayoutEffect(() => {
    alive.current = true;
    revision.current += 1;
    mutex.current = false;
    return () => {
      alive.current = false;
      revision.current += 1;
    };
  }, [scope]);
  const read = () => state.current.value;
  async function run<R>(
    work: (context: AttemptContext<S>) => Promise<R>,
  ): Promise<R | undefined> {
    const generation = revision.current;
    const current = () =>
      alive.current &&
      currentScope.current === scope &&
      revision.current === generation &&
      getKnowledgeSessionScope() === epoch;
    if (!ownerId || mutex.current || !current()) return;
    const patch = (next: Partial<S>) => {
      if (!current()) return;
      state.current = {
        scope,
        value: { ...state.current.value, ...next },
      };
      setRecord(state.current);
    };
    mutex.current = true;
    patch({ busy: true, issue: null } as Partial<S>);
    try {
      const result = await work({ current, patch, state: read, epoch });
      return current() ? result : undefined;
    } catch (error) {
      patch({ issue: issueOf(error) } as Partial<S>);
    } finally {
      if (current()) {
        mutex.current = false;
        patch({ busy: false } as Partial<S>);
      }
    }
  }
  return { state: record.scope === scope ? record.value : initial, read, run };
}

function assertDomain(
  value: KnowledgeDomain,
  ownerId: string,
  domainId: string,
  category?: KnowledgeDomain['category'],
) {
  if (
    !validKnowledgeData('Domain', value) ||
    value.owner_id !== ownerId ||
    value.domain_id !== domainId ||
    (category && value.category !== category)
  )
    invalidReceipt();
  return value;
}

interface DomainAttempt {
  datasetId: string | null;
  name: string;
  category: KnowledgeDomain['category'];
}
interface DomainEntryState extends EntryState {
  attempt: DomainAttempt | null;
  receipt: KnowledgeDomain | null;
  canRetry: boolean;
  uncertain: boolean;
}
const InitialDomainState: DomainEntryState = {
  busy: false,
  issue: null,
  attempt: null,
  receipt: null,
  canRetry: false,
  uncertain: false,
};

export function useKnowledgeDomainEntry(
  ownerId: string,
  refreshNative: () => Promise<unknown>,
) {
  const client = useQueryClient();
  const entry = useEntryState(ownerId, '', InitialDomainState);
  const refreshDatasets = async (context: AttemptContext<DomainEntryState>) => {
    await client.invalidateQueries({
      queryKey: EntryKeys.datasets(context.epoch, ownerId),
      exact: true,
    });
    if (context.current()) await refreshNative();
  };
  const confirm = async (
    context: AttemptContext<DomainEntryState>,
    id: string,
    category?: KnowledgeDomain['category'],
  ) => {
    const value = await service.domain(id);
    if (!context.current()) return;
    assertDomain(value, ownerId, id, category);
    await client.invalidateQueries({
      queryKey: KnowledgeKeys.domains(context.epoch),
      exact: true,
    });
    if (!context.current()) return;
    await client.invalidateQueries({
      queryKey: KnowledgeKeys.domain(context.epoch, id),
      exact: true,
    });
    context.patch({
      receipt: value,
      issue: null,
      canRetry: false,
      uncertain: false,
    });
  };
  const register = async (
    context: AttemptContext<DomainEntryState>,
    attempt: DomainAttempt,
  ) => {
    if (!attempt.datasetId || !context.current()) return;
    context.patch({ uncertain: true });
    let value: KnowledgeDomain;
    try {
      value = await service.registerDomain(attempt.datasetId, {
        category: attempt.category,
      });
    } catch (error) {
      if (issueOf(error).kind === 'failure')
        context.patch({ uncertain: false });
      throw error;
    }
    if (!context.current()) return;
    assertDomain(value, ownerId, attempt.datasetId, attempt.category);
    context.patch({ uncertain: false });
    await confirm(context, attempt.datasetId, attempt.category);
  };
  const create = (name: string, category: KnowledgeDomain['category']) => {
    if (entry.read().attempt || entry.read().receipt) return Promise.resolve();
    return entry.run(async (context) => {
      const normalized = name.trim();
      if (
        !normalized ||
        new TextEncoder().encode(normalized).length > 128 ||
        !validKnowledgeData('RegisterDomain', { category })
      ) {
        throw new KnowledgeRequestError(400, 'knowledge_entry_name');
      }
      const attempt: DomainAttempt = {
        datasetId: null,
        name: normalized,
        category,
      };
      context.patch({ attempt, canRetry: false, uncertain: true });
      let value: unknown;
      try {
        value = await service.createDataset(normalized);
      } catch (error) {
        if (issueOf(error).kind === 'failure')
          context.patch({ attempt: null, uncertain: false });
        throw error;
      }
      if (!context.current()) return;
      const dataset = value as {
        id?: string;
        name?: string;
        created_by?: string;
        permission?: string;
      } | null;
      if (
        !dataset ||
        !IdPattern.test(dataset.id || '') ||
        dataset.created_by !== ownerId ||
        dataset.permission !== 'me' ||
        typeof dataset.name !== 'string' ||
        !dataset.name
      )
        invalidReceipt();
      const known = { ...attempt, datasetId: dataset.id!, name: dataset.name! };
      context.patch({ attempt: known, uncertain: false });
      await refreshDatasets(context);
      if (context.current()) await register(context, known);
    });
  };
  const registerExisting = (
    id: string,
    category: KnowledgeDomain['category'],
  ) => {
    const pending = entry.read().attempt;
    if (pending?.datasetId || entry.read().receipt) return Promise.resolve();
    return entry.run(async (context) => {
      if (!IdPattern.test(id)) invalidReceipt();
      const attempt = {
        datasetId: id,
        name: '',
        category: pending?.category || category,
      };
      context.patch({ attempt, canRetry: false });
      await register(context, attempt);
    });
  };
  const check = () =>
    entry.run(async (context) => {
      const attempt = context.state().attempt;
      if (!attempt?.datasetId) {
        await refreshDatasets(context);
        context.patch({
          issue: {
            kind: 'unknown',
            code: 'native_result_unknown',
            requestId: '',
          },
        });
        return;
      }
      try {
        await confirm(context, attempt.datasetId, attempt.category);
      } catch (error) {
        if ((error as KnowledgeRequestError).status === 404) {
          context.patch({ canRetry: true });
        }
        throw error;
      }
    });
  const retryRegistration = () =>
    entry.run(async (context) => {
      const state = context.state();
      if (!state.canRetry || !state.attempt?.datasetId) return;
      context.patch({ canRetry: false });
      await register(context, state.attempt);
    });
  const selectDomain = (id: string) => {
    const state = entry.read();
    if (state.attempt && !state.receipt && state.uncertain)
      return Promise.resolve();
    return entry.run(async (context) => {
      if (!IdPattern.test(id)) invalidReceipt();
      await confirm(context, id);
    });
  };
  const clearCompleted = () => {
    if (!entry.read().receipt) return Promise.resolve();
    return entry.run(async (context) => {
      context.patch({
        attempt: null,
        receipt: null,
        canRetry: false,
        uncertain: false,
      });
    });
  };
  return {
    ...entry.state,
    create,
    registerExisting,
    check,
    retryRegistration,
    selectDomain,
    clearCompleted,
    isLocked: () => {
      const state = entry.read();
      return (
        state.busy || (!!state.attempt && !state.receipt && state.uncertain)
      );
    },
    locked: entry.state.busy || (!!entry.state.attempt && !entry.state.receipt),
  };
}

interface GroupEntryState extends EntryState {
  attempt: CreateKnowledgeGroupRequest | null;
  receipt: KnowledgeGroup | null;
  origin: 'created' | 'existing' | null;
}
const InitialGroupState: GroupEntryState = {
  busy: false,
  issue: null,
  attempt: null,
  receipt: null,
  origin: null,
};

export function useRootProjectCreate(
  ownerId: string,
  domainId: string,
  canCreate: () => boolean = () => true,
) {
  const client = useQueryClient();
  const allowed = useRef(canCreate);
  allowed.current = canCreate;
  const entry = useEntryState(ownerId, domainId, InitialGroupState);
  const assertGroup = (value: KnowledgeGroup) => {
    if (
      !validKnowledgeData('Group', value) ||
      value.domain_id !== domainId ||
      value.parent_id !== null ||
      value.kind !== 'project'
    )
      invalidReceipt();
    return value;
  };
  const authorize = async (context: AttemptContext<GroupEntryState>) => {
    const domain = await service.domain(domainId);
    if (!context.current()) return false;
    assertDomain(domain, ownerId, domainId);
    return true;
  };
  const refresh = (context: AttemptContext<GroupEntryState>) =>
    client.invalidateQueries({
      queryKey: KnowledgeKeys.groups(context.epoch, domainId, undefined),
      exact: true,
    });
  const create = (name: string, side: ComponentSide) => {
    if (!allowed.current() || entry.read().attempt || entry.read().receipt)
      return Promise.resolve();
    return entry.run(async (context) => {
      const normalized = name.trim();
      if (
        !normalized ||
        [...normalized].length > 256 ||
        normalized.includes('\0')
      ) {
        throw new KnowledgeRequestError(400, 'knowledge_entry_name');
      }
      if (!(await authorize(context))) return;
      if (!allowed.current()) return;
      const request: CreateKnowledgeGroupRequest = {
        parent_id: null,
        kind: 'project',
        name: normalized,
        component_side: side,
      };
      context.patch({ attempt: request });
      let group: KnowledgeGroup;
      try {
        group = await service.createGroup(domainId, request);
      } catch (error) {
        if (issueOf(error).kind === 'failure') context.patch({ attempt: null });
        throw error;
      }
      if (!context.current()) return;
      assertGroup(group);
      if (group.name !== request.name || group.component_side !== side)
        invalidReceipt();
      await refresh(context);
      context.patch({ receipt: group, origin: 'created', issue: null });
    });
  };
  const reconcile = (id: string) =>
    entry.run(async (context) => {
      if (!IdPattern.test(id) || !(await authorize(context))) return;
      const seen = new Set<string>();
      let cursor: string | undefined;
      do {
        const page = await service.groups(domainId, { cursor, limit: 200 });
        if (!context.current()) return;
        const found = page.items.find((group) => group.id === id);
        if (found) {
          assertGroup(found);
          await refresh(context);
          context.patch({ receipt: found, origin: 'existing', issue: null });
          return;
        }
        cursor = page.next_cursor || undefined;
        if (cursor && seen.has(cursor)) invalidReceipt();
        if (cursor) seen.add(cursor);
      } while (cursor);
      throw new KnowledgeRequestError(404, 'knowledge_not_found');
    });
  const select = (onSelect: (group: KnowledgeGroup) => void) =>
    entry.run(async (context) => {
      const group = context.state().receipt;
      if (context.state().origin === 'created' && !allowed.current()) return;
      if (group && context.current()) onSelect(group);
    });
  return {
    ...entry.state,
    create,
    reconcile,
    select,
    locked: entry.state.busy || (!!entry.state.attempt && !entry.state.receipt),
  };
}

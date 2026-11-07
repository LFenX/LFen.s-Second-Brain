import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  useDocumentActions,
  scopedDocument,
  documentScopeFailure,
} from '@/hooks/use-knowledge-document';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import {
  abandonDocumentSlot,
  acceptDocumentSlot,
  readDocumentSlots,
  saveDocumentSlot,
  documentRejected,
  documentTerminal,
  documentSlotKey,
  documentSlotOperation,
  type DocumentScope,
  type DocumentOperation,
  type DocumentRecoverySlot,
  type DocumentOutcome,
} from '@/utils/knowledge-document';
import type { DocumentTask } from '@/interfaces/database/knowledge-document';

export function useDocumentFlow(
  scope: DocumentScope,
  storage: Storage = localStorage,
) {
  const actions = useDocumentActions(scope),
    session = getKnowledgeSessionScope();
  const scopeIdentity = [session, scope.owner, scope.domain].join(':');
  const activeScope = useRef(scopeIdentity);
  activeScope.current = scopeIdentity;
  const taskRevisions = useRef(new Map<string, number>());
  const terminalRefreshes = useRef(new Map<string, Promise<void>>());
  const slotEpoch = useRef(0);
  const slotStorage = useMemo(
    () =>
      new Proxy(storage, {
        get(target, property) {
          const method = Reflect.get(target, property, target);
          if (typeof method !== 'function') return method;
          return (...args: unknown[]) => {
            const result = Reflect.apply(method, target, args);
            if (property === 'setItem' || property === 'removeItem')
              slotEpoch.current++;
            return result;
          };
        },
      }),
    [storage],
  );
  const [slots, setSlots] = useState<DocumentRecoverySlot[]>([]),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [preparationEpoch, setPreparationEpoch] = useState(0),
    [outcomes, setOutcomes] = useState<Record<string, DocumentOutcome>>({}),
    [rejections, setRejections] = useState<Record<string, boolean>>({});
  const lock = useRef(false),
    mounted = useRef(true),
    actionsRef = useRef(actions);
  actionsRef.current = actions;
  const current = useCallback(
    () =>
      mounted.current &&
      activeScope.current === scopeIdentity &&
      session === getKnowledgeSessionScope(),
    [scopeIdentity, session],
  );
  const reportError = useCallback((issue: unknown) => {
    if (documentScopeFailure(issue)) {
      setSlots([]);
      setOutcomes({});
      setRejections({});
      setReady(false);
    }
    setError(issue);
  }, []);
  const invalidatePreparation = useCallback(
    () => setPreparationEpoch((value) => value + 1),
    [],
  );
  const refresh = useCallback(
    async (source?: string) => {
      invalidatePreparation();
      await actionsRef.current.refresh(source);
    },
    [invalidatePreparation],
  );
  const rememberTask = useCallback(
    (task: DocumentTask) => {
      if (
        !current() ||
        task.owner_id !== scope.owner ||
        task.domain_id !== scope.domain
      )
        return false;
      const key = [scopeIdentity, task.task_id].join(':');
      const revision = taskRevisions.current.get(key);
      if (revision !== undefined && task.revision < revision) return false;
      taskRevisions.current.set(key, task.revision);
      return true;
    },
    [current, scope.owner, scope.domain, scopeIdentity],
  );
  const refreshTaskHead = useCallback(
    async (task: DocumentTask) => {
      const taskKey = [scopeIdentity, task.task_id].join(':');
      if (!current() || taskRevisions.current.get(taskKey) !== task.revision)
        return;
      const key = [taskKey, task.revision].join(':');
      let pending = terminalRefreshes.current.get(key);
      if (!pending) {
        pending = Promise.resolve().then(async () => {
          if (current() && taskRevisions.current.get(taskKey) === task.revision)
            await refresh(task.original.source_id);
        });
        terminalRefreshes.current.set(key, pending);
      }
      try {
        await pending;
      } catch (issue) {
        if (terminalRefreshes.current.get(key) === pending)
          terminalRefreshes.current.delete(key);
        throw issue;
      }
    },
    [current, refresh, scopeIdentity],
  );
  const refreshReceived = useCallback(
    async (value: DocumentOutcome, source?: string) => {
      if (!current()) return;
      try {
        if (value.record_kind === 'task' && documentTerminal(value))
          await refreshTaskHead(value);
        else await refresh(source);
      } catch (issue) {
        if (documentScopeFailure(issue) || !current()) throw issue;
        // The query observer retains the refresh error; this outcome is confirmed.
      }
    },
    [current, refresh, refreshTaskHead],
  );
  const reload = useCallback(
    async (permitted = current) => {
      const epoch = slotEpoch.current;
      const currentSnapshot = () => permitted() && slotEpoch.current === epoch;
      const all = await scopedDocument(session, () =>
        readDocumentSlots(storage, scope.owner),
      );
      if (currentSnapshot()) {
        setSlots((values) =>
          currentSnapshot()
            ? all.filter((slot) => slot.domain === scope.domain)
            : values,
        );
        setReady((value) => (currentSnapshot() ? true : value));
      }
    },
    [session, storage, scope.owner, scope.domain, current],
  );
  const accept = useCallback(
    async (
      slot: DocumentRecoverySlot,
      value: DocumentOutcome,
      fromGet: boolean,
    ) => {
      if (!current() || (value.record_kind === 'task' && !rememberTask(value)))
        return false;
      const taskKey =
        value.record_kind === 'task'
          ? [scopeIdentity, value.task_id].join(':')
          : null;
      const revision = value.record_kind === 'task' ? value.revision : null;
      const permitted = () =>
        current() &&
        (taskKey === null || taskRevisions.current.get(taskKey) === revision);
      const commitStorage = new Proxy(slotStorage, {
        get(target, property) {
          const method = Reflect.get(target, property, target);
          if (typeof method !== 'function') return method;
          return (...args: unknown[]) => {
            if (
              (property === 'setItem' || property === 'removeItem') &&
              !permitted()
            )
              throw Error('document_result_superseded');
            return Reflect.apply(method, target, args);
          };
        },
      });
      try {
        await scopedDocument(session, () =>
          acceptDocumentSlot(commitStorage, slot, value),
        );
        if (!permitted()) return false;
        const key = documentSlotKey(slot);
        setOutcomes((values) =>
          permitted() ? { ...values, [key]: value } : values,
        );
        setRejections((values) =>
          permitted()
            ? { ...values, [key]: fromGet && documentRejected(value) }
            : values,
        );
        await reload(permitted);
        return permitted();
      } catch (issue) {
        if (documentScopeFailure(issue)) throw issue;
        if (!permitted()) return false;
        throw issue;
      }
    },
    [session, slotStorage, current, reload, rememberTask, scopeIdentity],
  );
  const recoverOne = useCallback(
    async (slot: DocumentRecoverySlot) => {
      const response =
        slot.kind === 'task' && slot.task_id
          ? await actionsRef.current.task(slot.task_id)
          : await actionsRef.current.recover(slot.kind, slot.key);
      const accepted = await accept(slot, response.data, true);
      if (accepted && !documentRejected(response.data))
        await refreshReceived(
          response.data,
          slot.kind === 'task' ? slot.target : undefined,
        );
      return response.data;
    },
    [accept, refreshReceived],
  );
  useEffect(() => {
    mounted.current = true;
    setReady(false);
    setSlots([]);
    setOutcomes({});
    setRejections({});
    setError(null);
    let active = true;
    async function restore() {
      try {
        const epoch = slotEpoch.current;
        const currentSnapshot = () =>
          active && current() && slotEpoch.current === epoch;
        const all = await scopedDocument(session, () =>
          readDocumentSlots(storage, scope.owner),
        );
        if (!currentSnapshot()) return;
        const own = all.filter((slot) => slot.domain === scope.domain);
        setSlots((values) => (currentSnapshot() ? own : values));
        setReady((value) => (currentSnapshot() ? true : value));
        for (const slot of own) {
          if (!active || !current()) return;
          try {
            await recoverOne(slot);
          } catch (issue) {
            if (active && current()) reportError(issue);
          }
        }
      } catch (issue) {
        if (active && current()) {
          setReady(false);
          reportError(issue);
        }
      }
    }
    void restore();
    return () => {
      active = false;
      mounted.current = false;
    };
  }, [
    scope.owner,
    scope.domain,
    session,
    storage,
    current,
    recoverOne,
    reportError,
  ]);
  async function exclusive<T>(work: () => Promise<T>): Promise<T> {
    if (!current()) throw Error('document_scope_closed');
    if (lock.current) throw Error('document_operation_pending');
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      return await work();
    } catch (issue) {
      if (current()) reportError(issue);
      throw issue;
    } finally {
      lock.current = false;
      if (current()) setBusy(false);
    }
  }
  async function writeSaved(
    slot: DocumentRecoverySlot,
    operation: DocumentOperation,
  ): Promise<DocumentOutcome> {
    try {
      const response = await actions.write(operation);
      const accepted = await accept(slot, response.data, false);
      if (accepted)
        await refreshReceived(
          response.data,
          operation.kind === 'task' ? operation.scope.source : undefined,
        );
      return response.data;
    } catch (issue) {
      if (documentScopeFailure(issue) || !current()) throw issue;
      try {
        return await recoverOne(slot);
      } catch (lookupIssue) {
        if (documentScopeFailure(lookupIssue)) throw lookupIssue;
        throw issue;
      }
    }
  }
  const submit = (operation: DocumentOperation) =>
    exclusive(async () => {
      if (!ready) throw Error('document_storage_failed');
      if (
        operation.scope.owner !== scope.owner ||
        operation.scope.domain !== scope.domain
      )
        throw Error('document_scope_mismatch');
      const existing = await scopedDocument(session, () =>
        readDocumentSlots(storage, scope.owner),
      );
      const target =
        operation.kind === 'task'
          ? operation.scope.source
          : operation.kind === 'cancel'
            ? operation.scope.task
            : operation.scope.domain;
      if (
        existing.some(
          (slot) =>
            slot.domain === operation.scope.domain &&
            slot.kind === operation.kind &&
            slot.target === target &&
            !slot.task_id,
        )
      )
        throw Error('document_original_key_required');
      const slot = await scopedDocument(session, () =>
        saveDocumentSlot(slotStorage, operation),
      );
      await reload();
      return writeSaved(slot, operation);
    });
  const recover = (slot: DocumentRecoverySlot) =>
    exclusive(() => recoverOne(slot));
  const resend = (slot: DocumentRecoverySlot) =>
    exclusive(async () => {
      const existing = await scopedDocument(session, () =>
        readDocumentSlots(storage, scope.owner),
      );
      if (
        !existing.some(
          (value) =>
            documentSlotKey(value) === documentSlotKey(slot) &&
            value.request_sha256 === slot.request_sha256,
        )
      )
        throw Error('document_storage_corrupt');
      const operation = documentSlotOperation(slot);
      return writeSaved(slot, operation);
    });
  const abandon = (slot: DocumentRecoverySlot) =>
    exclusive(async () => {
      const key = documentSlotKey(slot),
        outcome = outcomes[key];
      if (!rejections[key] || !outcome || !documentRejected(outcome))
        throw Error('Fixed GET rejection required');
      await scopedDocument(session, () =>
        abandonDocumentSlot(slotStorage, slot, outcome),
      );
      await reload();
      await refresh(slot.kind === 'task' ? slot.target : undefined);
    });
  const reconcile = async (task: DocumentTask) => {
    if (!rememberTask(task)) return;
    const slot = slots.find(
      (item) =>
        item.kind === 'task' &&
        item.body.idempotency_key === task.original_request.idempotency_key,
    );
    if (slot && !(await accept(slot, task, false))) return;
    if (current() && documentTerminal(task)) await refreshReceived(task);
  };
  return {
    slots,
    ready,
    preparationEpoch,
    invalidatePreparation,
    busy,
    error,
    outcomes,
    rejections,
    submit,
    recover,
    resend,
    abandon,
    reconcile,
    prepare: actions.prepare,
    refresh,
  };
}
export type DocumentFlow = ReturnType<typeof useDocumentFlow>;

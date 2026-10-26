import { useCallback, useEffect, useRef, useState } from 'react';
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
    () => mounted.current && session === getKnowledgeSessionScope(),
    [session],
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
  const reload = useCallback(async () => {
    const all = await scopedDocument(session, () =>
      readDocumentSlots(storage, scope.owner),
    );
    if (current()) {
      setSlots(all.filter((slot) => slot.domain === scope.domain));
      setReady(true);
    }
  }, [session, storage, scope.owner, scope.domain, current]);
  const accept = useCallback(
    async (
      slot: DocumentRecoverySlot,
      value: DocumentOutcome,
      fromGet: boolean,
    ) => {
      await scopedDocument(session, () =>
        acceptDocumentSlot(storage, slot, value),
      );
      if (current()) {
        const key = documentSlotKey(slot);
        setOutcomes((values) => ({ ...values, [key]: value }));
        setRejections((values) => ({
          ...values,
          [key]: fromGet && documentRejected(value),
        }));
        await reload();
      }
    },
    [session, storage, current, reload],
  );
  const recoverOne = useCallback(
    async (slot: DocumentRecoverySlot) => {
      const response =
        slot.kind === 'task' && slot.task_id
          ? await actionsRef.current.task(slot.task_id)
          : await actionsRef.current.recover(slot.kind, slot.key);
      await accept(slot, response.data, true);
      if (!documentRejected(response.data))
        await refresh(slot.kind === 'task' ? slot.target : undefined);
      return response.data;
    },
    [accept, refresh],
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
        const all = await scopedDocument(session, () =>
          readDocumentSlots(storage, scope.owner),
        );
        if (!active || !current()) return;
        const own = all.filter((slot) => slot.domain === scope.domain);
        setSlots(own);
        setReady(true);
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
      await accept(slot, response.data, false);
      await refresh(
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
        saveDocumentSlot(storage, operation),
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
        abandonDocumentSlot(storage, slot, outcome),
      );
      await reload();
      await refresh(slot.kind === 'task' ? slot.target : undefined);
    });
  const reconcile = async (task: DocumentTask) => {
    const slot = slots.find(
      (item) =>
        item.kind === 'task' &&
        item.body.idempotency_key === task.original_request.idempotency_key,
    );
    if (slot) await accept(slot, task, false);
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

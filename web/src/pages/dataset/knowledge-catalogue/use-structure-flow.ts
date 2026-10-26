import { useCallback, useEffect, useRef, useState } from 'react';
import {
  useStructureActions,
  scopedStructure,
  structureScopeFailure,
} from '@/hooks/use-knowledge-structure';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import {
  abandonStructureSlot,
  acceptStructureSlot,
  readStructureSlots,
  saveStructureSlot,
  structureRejected,
  structureSlotKey,
  structureSlotOperation,
  type StructureScope,
  type StructureOperation,
  type StructureRecoverySlot,
  type StructureOutcome,
} from '@/utils/knowledge-structure';
import type { StructureTask } from '@/interfaces/database/knowledge-structure';

export function useStructureFlow(
  scope: StructureScope,
  storage: Storage = localStorage,
) {
  const actions = useStructureActions(scope),
    session = getKnowledgeSessionScope();
  const [slots, setSlots] = useState<StructureRecoverySlot[]>([]),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [preparationEpoch, setPreparationEpoch] = useState(0),
    [outcomes, setOutcomes] = useState<Record<string, StructureOutcome>>({}),
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
    if (structureScopeFailure(issue)) {
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
    async (project?: string) => {
      invalidatePreparation();
      await actionsRef.current.refresh(project);
    },
    [invalidatePreparation],
  );
  const reload = useCallback(async () => {
    const all = await scopedStructure(session, () =>
      readStructureSlots(storage, scope.owner),
    );
    if (current()) {
      setSlots(all.filter((slot) => slot.domain === scope.domain));
      setReady(true);
    }
  }, [session, storage, scope.owner, scope.domain, current]);
  const accept = useCallback(
    async (
      slot: StructureRecoverySlot,
      value: StructureOutcome,
      fromGet: boolean,
    ) => {
      await scopedStructure(session, () =>
        acceptStructureSlot(storage, slot, value),
      );
      if (current()) {
        const key = structureSlotKey(slot);
        setOutcomes((values) => ({ ...values, [key]: value }));
        setRejections((values) => ({
          ...values,
          [key]: fromGet && structureRejected(value),
        }));
        await reload();
      }
    },
    [session, storage, current, reload],
  );
  const recoverOne = useCallback(
    async (slot: StructureRecoverySlot) => {
      const response =
        slot.kind === 'task' && slot.task_id
          ? await actionsRef.current.task(slot.task_id)
          : await actionsRef.current.recover(slot.kind, slot.key);
      await accept(slot, response.data, true);
      if (!structureRejected(response.data))
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
        const all = await scopedStructure(session, () =>
          readStructureSlots(storage, scope.owner),
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
    if (lock.current) throw Error('structure_operation_pending');
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
    slot: StructureRecoverySlot,
    operation: StructureOperation,
  ): Promise<StructureOutcome> {
    try {
      const response = await actions.write(operation);
      await accept(slot, response.data, false);
      await refresh(
        operation.kind === 'task' ? operation.scope.project : undefined,
      );
      return response.data;
    } catch (issue) {
      if (structureScopeFailure(issue) || !current()) throw issue;
      try {
        return await recoverOne(slot);
      } catch (lookupIssue) {
        if (structureScopeFailure(lookupIssue)) throw lookupIssue;
        throw issue;
      }
    }
  }
  const submit = (operation: StructureOperation) =>
    exclusive(async () => {
      if (!ready) throw Error('structure_storage_failed');
      if (
        operation.scope.owner !== scope.owner ||
        operation.scope.domain !== scope.domain
      )
        throw Error('structure_scope_mismatch');
      const existing = await scopedStructure(session, () =>
        readStructureSlots(storage, scope.owner),
      );
      const target =
        operation.kind === 'task'
          ? operation.scope.project
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
        throw Error('structure_original_key_required');
      const slot = await scopedStructure(session, () =>
        saveStructureSlot(storage, operation),
      );
      await reload();
      return writeSaved(slot, operation);
    });
  const recover = (slot: StructureRecoverySlot) =>
    exclusive(() => recoverOne(slot));
  const resend = (slot: StructureRecoverySlot) =>
    exclusive(async () => {
      const existing = await scopedStructure(session, () =>
        readStructureSlots(storage, scope.owner),
      );
      if (
        !existing.some(
          (value) =>
            structureSlotKey(value) === structureSlotKey(slot) &&
            value.request_sha256 === slot.request_sha256,
        )
      )
        throw Error('structure_storage_corrupt');
      const operation = structureSlotOperation(slot);
      return writeSaved(slot, operation);
    });
  const abandon = (slot: StructureRecoverySlot) =>
    exclusive(async () => {
      const key = structureSlotKey(slot),
        outcome = outcomes[key];
      if (!rejections[key] || !outcome || !structureRejected(outcome))
        throw Error('Fixed GET rejection required');
      await scopedStructure(session, () =>
        abandonStructureSlot(storage, slot, outcome),
      );
      await reload();
      await refresh(slot.kind === 'task' ? slot.target : undefined);
    });
  const reconcile = async (task: StructureTask) => {
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
export type StructureFlow = ReturnType<typeof useStructureFlow>;

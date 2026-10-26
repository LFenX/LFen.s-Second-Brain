import { useEffect, useRef, useState } from 'react';
import {
  useLifecycleActions,
  lifecycleScopeFailure,
} from '@/hooks/use-knowledge-lifecycle';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import service, {
  LifecycleRequestError,
} from '@/services/knowledge-lifecycle-service';
import type {
  LifecycleDetail,
  LifecycleReceipt,
} from '@/interfaces/database/knowledge-lifecycle';
import type {
  LifecycleAction,
  LifecycleCommand,
} from '@/interfaces/request/knowledge-lifecycle';
import {
  readLifecycleRecovery,
  saveLifecycleRecovery,
  removeLifecycleRecovery,
  lifecycleRequestDigest,
  lifecycleNewKey,
  lifecycleExpected,
  equalLifecycle,
  type LifecycleRecovery,
  type LifecycleScope,
} from '@/utils/knowledge-lifecycle';
import { validLifecycleCommand } from '@/utils/knowledge-lifecycle-schema';

export function useLifecycleFlow(
  scope: LifecycleScope,
  onScopeFailure: (error: unknown) => void,
) {
  const session = getKnowledgeSessionScope(),
    actions = useLifecycleActions(scope),
    live = useRef(true),
    busyRef = useRef(false),
    onFailure = useRef(onScopeFailure);
  onFailure.current = onScopeFailure;
  const [initial] = useState(() => {
    try {
      return {
        value: readLifecycleRecovery(scope.owner, scope.domain),
        error: null as unknown,
      };
    } catch (error) {
      return { value: null, error };
    }
  });
  const [pending, setPending] = useState<LifecycleRecovery | null>(
      initial.value,
    ),
    pendingRef = useRef(initial.value),
    [receipt, setReceipt] = useState<LifecycleReceipt | null>(null),
    [error, setError] = useState<unknown>(initial.error),
    [busy, setBusy] = useState(false),
    [blocked, setBlocked] = useState(false),
    [checkedMissing, setCheckedMissing] = useState(false),
    [knownRejected, setKnownRejected] = useState(false),
    [consentEpoch, setConsentEpoch] = useState(0);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);
  function assertCurrent() {
    if (!live.current || session !== getKnowledgeSessionScope())
      throw new LifecycleRequestError(401, 'knowledge_unauthorized');
  }
  function fail(issue: unknown, lookup = false) {
    setError(issue);
    setConsentEpoch((value) => value + 1);
    if (
      lifecycleScopeFailure(issue) &&
      !(
        lookup &&
        issue instanceof LifecycleRequestError &&
        issue.status === 404
      )
    ) {
      setBlocked(true);
      setReceipt(null);
      actions.clear();
      onFailure.current(issue);
    }
  }
  async function run(work: () => Promise<void>, lookup = false) {
    if (busyRef.current || initial.error || blocked) return;
    busyRef.current = true;
    setBusy(true);
    setError(null);
    try {
      await actions.run(work);
    } catch (issue) {
      if (live.current) fail(issue, lookup);
    } finally {
      busyRef.current = false;
      if (live.current) setBusy(false);
    }
  }
  async function save(value: LifecycleRecovery) {
    await saveLifecycleRecovery(value);
    assertCurrent();
    pendingRef.current = value;
    setPending(value);
  }
  async function verifySlot(value: LifecycleRecovery) {
    if (
      value.source_id !== scope.source ||
      value.owner_id !== scope.owner ||
      value.domain_id !== scope.domain ||
      (await lifecycleRequestDigest(scope, value.action, value.request)) !==
        value.request_sha256
    )
      throw Error('Recovery original context mismatch');
  }
  async function accept(value: LifecycleReceipt, original: LifecycleRecovery) {
    if (
      value.request_sha256 !== original.request_sha256 ||
      value.idempotency_key !== original.request.idempotency_key ||
      value.action !== original.action ||
      !equalLifecycle(value.request, original.request)
    )
      throw Error('Recovered another command');
    assertCurrent();
    await save({ ...original, command_id: value.command_id });
    setReceipt(value);
    setKnownRejected(false);
    setCheckedMissing(false);
    setConsentEpoch((version) => version + 1);
    await actions.refreshCurrent();
  }
  async function send(action: LifecycleAction, request: LifecycleCommand) {
    assertCurrent();
    if (action === 'reclassify' && 'classification' in request)
      return service.reclassify(scope, request);
    if (action === 'move' && 'relative_path' in request)
      return service.move(scope, request);
    if (action === 'archive' || action === 'restore')
      return service[action](scope, request);
    throw Error('Command shape mismatch');
  }
  async function submit(
    detail: LifecycleDetail,
    action: LifecycleAction,
    target: object = {},
    confirmed = false,
  ) {
    return run(async () => {
      const existing = readLifecycleRecovery(scope.owner, scope.domain);
      if (existing?.command_id === null)
        throw Error('Resolve the existing original request first');
      if (
        !confirmed ||
        !detail.actions[action].allowed ||
        detail.source_id !== scope.source ||
        detail.owner_id !== scope.owner ||
        detail.domain_id !== scope.domain
      )
        throw Error('Explicit current action is required');
      const request = {
        ...target,
        ...lifecycleExpected(detail),
        idempotency_key: lifecycleNewKey(),
        confirmed: true as const,
      };
      if (!validLifecycleCommand(action, request))
        throw new LifecycleRequestError(
          400,
          'knowledge_invalid_request',
          '',
          true,
        );
      const value: LifecycleRecovery = {
        slot_version: 1,
        session_scope: session,
        owner_id: scope.owner,
        domain_id: scope.domain,
        source_id: scope.source,
        action,
        request,
        request_sha256: await lifecycleRequestDigest(scope, action, request),
        command_id: null,
      };
      await save(value);
      setReceipt(null);
      setKnownRejected(false);
      setCheckedMissing(false);
      try {
        await accept((await send(action, request)).data, value);
      } catch (issue) {
        if (
          issue instanceof LifecycleRequestError &&
          issue.outcomeKnown &&
          !['idempotency_conflict', 'lifecycle_integrity'].includes(issue.code)
        )
          setKnownRejected(true);
        throw issue;
      }
    });
  }
  async function recover() {
    return run(async () => {
      const original = pendingRef.current;
      if (!original) return;
      await verifySlot(original);
      setCheckedMissing(false);
      try {
        const result = original.command_id
          ? await service.receipt(scope, original.command_id)
          : await service.byKey(scope, original.action, original.request);
        await accept(result.data, original);
      } catch (issue) {
        if (issue instanceof LifecycleRequestError && issue.status === 404) {
          // Missing-by-key is ambiguous. Only current authorization enables an explicit resend.
          try {
            await service.detail(scope);
          } catch (authorization) {
            fail(authorization);
            throw authorization;
          }
          assertCurrent();
          setCheckedMissing(true);
        }
        throw issue;
      }
    }, true);
  }
  async function resend() {
    return run(async () => {
      const original = pendingRef.current;
      if (!original || !checkedMissing || original.command_id !== null)
        throw Error('GET the original result first');
      await verifySlot(original);
      await service.detail(scope);
      try {
        await accept(
          (await send(original.action, original.request)).data,
          original,
        );
      } catch (issue) {
        if (
          issue instanceof LifecycleRequestError &&
          issue.outcomeKnown &&
          !['idempotency_conflict', 'lifecycle_integrity'].includes(issue.code)
        )
          setKnownRejected(true);
        throw issue;
      }
    });
  }
  async function abandon() {
    return run(async () => {
      const original = pendingRef.current;
      if (
        !original ||
        !knownRejected ||
        !checkedMissing ||
        original.command_id !== null
      )
        throw Error('A checked determinate rejection is required');
      await verifySlot(original);
      try {
        await accept(
          (await service.byKey(scope, original.action, original.request)).data,
          original,
        );
        return;
      } catch (issue) {
        if (!(issue instanceof LifecycleRequestError && issue.status === 404))
          throw issue;
      }
      await service.detail(scope);
      assertCurrent();
      removeLifecycleRecovery(scope.owner, scope.domain);
      pendingRef.current = null;
      setPending(null);
      setKnownRejected(false);
      setCheckedMissing(false);
      setConsentEpoch((value) => value + 1);
      await actions.refreshCurrent();
    });
  }
  return {
    pending,
    receipt,
    error,
    busy,
    blocked,
    corrupt: !!initial.error,
    elsewhere:
      pending?.command_id === null && pending.source_id !== scope.source,
    unresolved: pending?.command_id === null,
    canResend: checkedMissing && pending?.command_id === null,
    canAbandon: checkedMissing && knownRejected,
    consentEpoch,
    submit,
    recover,
    resend,
    abandon,
  };
}
export type LifecycleFlow = ReturnType<typeof useLifecycleFlow>;

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  useKnowledgeImportActions,
  KnowledgeImportKeys,
} from '@/hooks/use-knowledge-sources';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from '@/hooks/use-knowledge-foundation';
import type {
  KnowledgeImportPreview,
  KnowledgeImportReceipt,
  KnowledgeRegistration,
} from '@/interfaces/database/knowledge-sources';
import type {
  KnowledgePreviewRequest,
  KnowledgeRegistrationRequest,
} from '@/interfaces/request/knowledge-sources';
import type { KnowledgeSourceResult } from '@/services/knowledge-sources-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  checkKnowledgePreview,
  checkKnowledgeReceipt,
  knowledgeTextDigest,
  loadKnowledgeConfirmation,
  saveKnowledgeConfirmation,
  type KnowledgePendingConfirmation,
} from '@/utils/knowledge-import';

export function useImportFlow(
  domainId: string,
  storageKey: string,
  onScopeFailure: (error: unknown) => void,
) {
  const actions = useKnowledgeImportActions(domainId);
  const Actions = useRef(actions);
  Actions.current = actions;
  const client = useQueryClient();
  const [pending, setPending] = useState<KnowledgePendingConfirmation | null>(
    () => loadKnowledgeConfirmation(storageKey),
  );
  const [phase, setPhase] = useState(pending ? 'unknown' : 'registration');
  const [registration, setRegistration] =
    useState<KnowledgeRegistration | null>(null);
  const [registrationAttempt, setRegistrationAttempt] =
    useState<KnowledgeRegistrationRequest | null>(null);
  const [preview, setPreview] = useState<KnowledgeImportPreview | null>(null);
  const [receipt, setReceipt] =
    useState<KnowledgeSourceResult<KnowledgeImportReceipt> | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [scopeFailure, setScopeFailure] = useState<unknown>(null);
  const [canRepreview, setCanRepreview] = useState(false);
  const [busy, setBusy] = useState(false);
  const Busy = useRef(false);
  const Alive = useRef(true);
  useEffect(() => {
    Alive.current = true;
    return () => {
      Alive.current = false;
    };
  }, []);

  const hideScope = useCallback(
    (failure: KnowledgeRequestError) => {
      setScopeFailure(failure);
      onScopeFailure(failure);
      setRegistration(null);
      setPreview(null);
      setReceipt(null);
      setRegistrationAttempt(null);
      Object.values(Actions.current).forEach((action) => action.reset());
      const session = getKnowledgeSessionScope();
      client.removeQueries({
        queryKey: KnowledgeKeys.domainScope(session, domainId),
      });
      client.removeQueries({
        queryKey: KnowledgeImportKeys.scope(session, domainId),
      });
    },
    [client, domainId, onScopeFailure],
  );

  function fail(failure: unknown) {
    if (!Alive.current) return;
    setError(failure);
    if (
      failure instanceof KnowledgeRequestError &&
      ([401, 403, 404].includes(failure.status) ||
        [
          'knowledge_source_contract_version',
          'knowledge_contract_version',
        ].includes(failure.code))
    )
      hideScope(failure);
  }

  async function run(work: () => Promise<void>) {
    if (Busy.current) return;
    Busy.current = true;
    setBusy(true);
    setError(null);
    try {
      await work();
    } catch (failure) {
      fail(failure);
    } finally {
      Busy.current = false;
      if (Alive.current) setBusy(false);
    }
  }

  function acceptReceipt(
    result: KnowledgeSourceResult<KnowledgeImportReceipt>,
    expected: KnowledgeImportPreview,
  ) {
    checkKnowledgeReceipt(expected, result.data);
    if (!Alive.current) return;
    setPreview(expected);
    setReceipt(result);
    setPhase('receipt');
    setCanRepreview(false);
    actions.preview.reset();
  }

  const register = (request: KnowledgeRegistrationRequest) =>
    run(async () => {
      setRegistrationAttempt(request);
      try {
        const result = await actions.register.mutateAsync(request);
        const actual = result.data;
        if (
          actual.registration_key !== request.registration_key ||
          actual.name !== request.name ||
          actual.group_id !== request.group_id ||
          actual.component_side !== request.component_side ||
          actual.language !== request.language ||
          JSON.stringify([...actual.include_paths].sort()) !==
            JSON.stringify([...request.include_paths].sort())
        )
          throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
        if (!Alive.current) return;
        setRegistration(actual);
        setRegistrationAttempt(null);
        setPhase('file');
      } catch (failure) {
        if (
          failure instanceof KnowledgeRequestError &&
          failure.status >= 400 &&
          failure.status < 500
        )
          setRegistrationAttempt(null);
        throw failure;
      }
    });

  const makePreview = (request: KnowledgePreviewRequest) =>
    run(async () => {
      if (!registration || pending) return;
      setPreview(null);
      setPhase('file');
      const digest = await knowledgeTextDigest(request.text);
      const result = await actions.preview.mutateAsync({
        registrationId: registration.registration_id,
        request,
      });
      checkKnowledgePreview(result.data, request, digest);
      if (!Alive.current) return;
      setPreview(result.data);
      setPhase('preview');
    });

  const confirm = () =>
    run(async () => {
      if (!registration || !preview || phase !== 'preview' || pending) return;
      const next = {
        registrationId: registration.registration_id,
        request: {
          preview_id: preview.preview_id,
          manifest_sha256: preview.manifest_sha256,
          idempotency_key: crypto.randomUUID(),
        },
      };
      // Persist before the first POST. A browser close or lost response must not
      // create a different confirmation request on recovery.
      saveKnowledgeConfirmation(storageKey, next);
      setPending(next);
      setPhase('submitting');
      try {
        acceptReceipt(await actions.confirm.mutateAsync(next), preview);
      } catch (failure) {
        setPhase(
          failure instanceof KnowledgeRequestError && failure.status === 409
            ? 'conflict'
            : 'unknown',
        );
        throw failure;
      }
    });

  const recover = () =>
    run(async () => {
      if (!pending) return;
      setCanRepreview(false);
      setPhase('unknown');
      const reg = await actions.registration.mutateAsync(
        pending.registrationId,
      );
      const expected = await actions.readPreview.mutateAsync({
        registrationId: pending.registrationId,
        previewId: pending.request.preview_id,
      });
      if (expected.manifest_sha256 !== pending.request.manifest_sha256)
        throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
      if (!Alive.current) return;
      setRegistration(reg);
      const readConsumed = async (value: KnowledgeImportPreview) => {
        if (!value.consumed_task_id) return false;
        const valueReceipt = await actions.receipt.mutateAsync({
          registrationId: pending.registrationId,
          taskId: value.consumed_task_id,
        });
        acceptReceipt({ data: valueReceipt, httpStatus: 200 }, value);
        return true;
      };
      if (await readConsumed(expected)) return;
      try {
        acceptReceipt(await actions.confirm.mutateAsync(pending), expected);
      } catch (failure) {
        if (
          failure instanceof KnowledgeRequestError &&
          failure.status === 409 &&
          failure.code === 'knowledge_conflict'
        ) {
          // Never infer a reason from message. After the original POST is
          // rejected, re-read consumption before offering a new preview.
          const latest = await actions.readPreview.mutateAsync({
            registrationId: pending.registrationId,
            previewId: pending.request.preview_id,
          });
          if (latest.manifest_sha256 !== pending.request.manifest_sha256)
            throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
          if (await readConsumed(latest)) return;
          setPhase('conflict');
          setCanRepreview(true);
        }
        throw failure;
      }
    });

  const startAnother = () => {
    if (Busy.current || !registration || (phase !== 'receipt' && !canRepreview))
      return;
    sessionStorage.removeItem(storageKey);
    setPending(null);
    setReceipt(null);
    setPreview(null);
    setError(null);
    setCanRepreview(false);
    setPhase('file');
  };
  const invalidatePreview = () => {
    if (!pending && phase === 'preview') {
      setPreview(null);
      setPhase('file');
    }
  };
  return {
    phase,
    registration,
    registrationAttempt,
    preview,
    receipt,
    pending,
    error,
    scopeFailure,
    busy,
    canRepreview,
    register,
    makePreview,
    confirm,
    recover,
    startAnother,
    invalidatePreview,
    hideScope,
  };
}

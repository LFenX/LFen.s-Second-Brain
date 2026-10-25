import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import dayjs from 'dayjs';
import {
  useDirectoryActions,
  KnowledgeDirectoryKeys,
} from '@/hooks/use-knowledge-directory';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from '@/hooks/use-knowledge-foundation';
import service, {
  DirectoryRequestError,
  type DirectoryResponse,
} from '@/services/knowledge-directory-service';
import type {
  DirectoryRegistration,
  DirectoryObservation,
  DirectoryPreview,
  DirectoryReceipt,
  DirectoryContentWindow,
} from '@/interfaces/database/knowledge-directory';
import type {
  DirectoryRegistrationRequest,
  DirectoryPreviewRequest,
} from '@/interfaces/request/knowledge-directory';
import {
  loadDirectoryPending,
  saveDirectoryPending,
  directoryObservationMatches,
  directoryPreviewMatches,
  directoryReceiptMatches,
  directoryWindowMatches,
  sameDirectoryClassification,
  type DirectoryPending,
} from '@/utils/knowledge-directory';

export function useDirectoryFlow(
  domain: string,
  storageKey: string,
  onScopeFailure: (error: unknown) => void,
) {
  const actions = useDirectoryActions(domain),
    Actions = useRef(actions);
  Actions.current = actions;
  const client = useQueryClient(),
    Alive = useRef(true),
    Busy = useRef(false);
  const [pending, setPending] = useState<DirectoryPending | null>(() =>
    loadDirectoryPending(storageKey),
  );
  const [registration, setRegistration] =
    useState<DirectoryRegistration | null>(null);
  const [attempt, setAttempt] = useState<DirectoryRegistrationRequest | null>(
    null,
  );
  const [observation, setObservation] = useState<DirectoryObservation | null>(
    null,
  );
  const [preview, setPreview] = useState<DirectoryPreview | null>(null);
  const [window, setWindow] = useState<DirectoryContentWindow | null>(null);
  const [receipt, setReceipt] =
    useState<DirectoryResponse<DirectoryReceipt> | null>(null);
  const [phase, setPhase] = useState(pending ? 'unknown' : 'registration');
  const [error, setError] = useState<unknown>(null),
    [blocked, setBlocked] = useState<unknown>(null);
  const [busy, setBusy] = useState(false),
    [canRestart, setCanRestart] = useState(false);
  useEffect(() => {
    Alive.current = true;
    return () => {
      Alive.current = false;
    };
  }, []);
  function clearPrivate() {
    setRegistration(null);
    setAttempt(null);
    setObservation(null);
    setPreview(null);
    setWindow(null);
    setReceipt(null);
    Actions.current.reset();
    client.removeQueries({
      queryKey: KnowledgeDirectoryKeys.scope(
        getKnowledgeSessionScope(),
        domain,
      ),
    });
    client.removeQueries({
      queryKey: KnowledgeKeys.domainScope(getKnowledgeSessionScope(), domain),
    });
  }
  function fail(failure: unknown) {
    if (!Alive.current) return;
    setError(failure);
    if (
      failure instanceof KnowledgeRequestError &&
      ([401, 403, 404].includes(failure.status) ||
        failure.code === 'knowledge_directory_version_mismatch')
    ) {
      clearPrivate();
      setBlocked(failure);
      onScopeFailure(failure);
    }
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
  const register = (request: DirectoryRegistrationRequest) =>
    run(async () => {
      setAttempt(request);
      try {
        const result = await Actions.current.run(() =>
          service.register(domain, request),
        );
        if (!Alive.current) return;
        setRegistration(result.data);
        setAttempt(null);
        setPhase('selection');
      } catch (failure) {
        if (
          failure instanceof DirectoryRequestError &&
          failure.status >= 400 &&
          failure.status < 500
        )
          setAttempt(null);
        throw failure;
      }
    });
  const observe = (paths: string[]) =>
    run(async () => {
      if (!registration || pending) return;
      setObservation(null);
      setPreview(null);
      setWindow(null);
      const result = await Actions.current.run(() =>
        service.observe(domain, registration.registration_id, {
          selected_paths: paths,
        }),
      );
      if (
        !directoryObservationMatches(result.data, paths) ||
        result.data.root_id !== registration.root_id
      )
        throw new DirectoryRequestError(
          502,
          'knowledge_directory_invalid_response',
        );
      if (!Alive.current) return;
      setObservation(result.data);
      setPhase('observation');
    });
  const makePreview = (request: DirectoryPreviewRequest) =>
    run(async () => {
      if (!registration || !observation || pending) return;
      setPreview(null);
      setWindow(null);
      setPhase('observation');
      const result = await Actions.current.run(() =>
        service.preview(domain, registration.registration_id, request),
      );
      if (!directoryPreviewMatches(result.data, request, observation))
        throw new DirectoryRequestError(
          502,
          'knowledge_directory_invalid_response',
        );
      const content = await Actions.current.run(() =>
        service.content(
          domain,
          registration.registration_id,
          result.data.preview_id,
          0,
          Math.max(1, Math.min(65536, result.data.items[0].byte_length)),
        ),
      );
      if (!directoryWindowMatches(content.data, result.data, 0))
        throw new DirectoryRequestError(
          502,
          'knowledge_directory_invalid_response',
        );
      if (!Alive.current) return;
      setPreview(result.data);
      setWindow(content.data);
      setPhase('preview');
    });
  const readWindow = (offset = 0) =>
    run(async () => {
      if (!registration || !preview || pending) return;
      setWindow(null);
      const length = Math.max(
        1,
        Math.min(65536, preview.items[0].byte_length - offset),
      );
      const result = await Actions.current.run(() =>
        service.content(
          domain,
          registration.registration_id,
          preview.preview_id,
          offset,
          length,
        ),
      );
      if (!directoryWindowMatches(result.data, preview, offset))
        throw new DirectoryRequestError(
          502,
          'knowledge_directory_invalid_response',
        );
      if (Alive.current) setWindow(result.data);
    });
  function accept(
    result: DirectoryResponse<DirectoryReceipt>,
    expected: DirectoryPending,
  ) {
    if (!directoryReceiptMatches(result.data, expected, preview))
      throw new DirectoryRequestError(
        502,
        'knowledge_directory_invalid_response',
      );
    if (!Alive.current) return;
    setReceipt(result);
    setWindow(null);
    setPreview(null);
    setPhase('receipt');
    setCanRestart(false);
    Actions.current.reset();
  }
  async function postOriginal(value: DirectoryPending) {
    try {
      accept(
        await Actions.current.run(() =>
          service.confirm(domain, value.registrationId, value.request),
        ),
        value,
      );
    } catch (failure) {
      if (Alive.current) {
        setPhase('unknown');
        setWindow(null);
        setCanRestart(
          failure instanceof DirectoryRequestError &&
            (failure.status === 410 ||
              (failure.status === 409 &&
                [
                  'root_changed',
                  'policy_changed',
                  'source_changed',
                  'revision_changed',
                  'observation_incomplete',
                  'observation_mismatch',
                  'ineligible',
                ].includes(failure.reason || ''))),
        );
      }
      throw failure;
    }
  }
  const confirm = () =>
    run(async () => {
      if (
        !registration ||
        !preview ||
        pending ||
        phase !== 'preview' ||
        !window
      )
        return;
      if (!dayjs(preview.expires_at).isAfter(dayjs()))
        throw new DirectoryRequestError(410, 'knowledge_directory_expired');
      const value = {
        registrationId: registration.registration_id,
        request: {
          preview_id: preview.preview_id,
          manifest_sha256: preview.manifest_sha256,
          idempotency_key: crypto.randomUUID(),
        },
      };
      saveDirectoryPending(storageKey, value);
      setPending(value);
      setPhase('submitting');
      await postOriginal(value);
    });
  const recover = () =>
    run(async () => {
      if (!pending) return;
      // A result check is GET-only, even if no visible result exists.
      const result = await Actions.current.run(() =>
        service.byKey(domain, pending.registrationId, pending.request),
      );
      accept(result, pending);
    });
  const resend = () =>
    run(async () => {
      if (!pending) return;
      setCanRestart(false);
      setPhase('submitting');
      await postOriginal(pending);
    });
  function invalidatePreview() {
    if (pending) return;
    setPreview(null);
    setWindow(null);
    if (observation) setPhase('observation');
  }
  const restart = () =>
    run(async () => {
      if (pending && phase !== 'receipt' && !canRestart) return;
      const restored = pending
        ? (
            await Actions.current.run(() =>
              service.registration(domain, pending.registrationId),
            )
          ).data
        : registration;
      if (
        registration &&
        restored &&
        (registration.root_id !== restored.root_id ||
          registration.registration_key !== restored.registration_key ||
          registration.name !== restored.name ||
          !sameDirectoryClassification(
            registration.default_classification,
            restored.default_classification,
          ) ||
          JSON.stringify([...registration.scope_directories].sort()) !==
            JSON.stringify([...restored.scope_directories].sort()))
      )
        throw new DirectoryRequestError(
          502,
          'knowledge_directory_invalid_response',
        );
      if (!Alive.current) return;
      sessionStorage.removeItem(storageKey);
      setPending(null);
      setRegistration(restored);
      setReceipt(null);
      setObservation(null);
      setPreview(null);
      setWindow(null);
      setCanRestart(false);
      setPhase(restored ? 'selection' : 'registration');
    });
  function changeSelection() {
    if (pending) return;
    setObservation(null);
    setPreview(null);
    setWindow(null);
    setPhase('selection');
  }
  return {
    registration,
    attempt,
    observation,
    preview,
    window,
    receipt,
    pending,
    phase,
    error,
    blocked,
    busy,
    canRestart,
    register,
    observe,
    makePreview,
    readWindow,
    confirm,
    recover,
    resend,
    restart,
    invalidatePreview,
    changeSelection,
    fail,
  };
}

import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import service, {
  type WebScope,
  WebRequestError,
} from '@/services/knowledge-web-service';
import {
  useWebActions,
  collectWebPages,
  KnowledgeWebKeys,
  webScopeFailure,
} from '@/hooks/use-knowledge-web';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import type {
  WebRegistration,
  WebCapture,
  WebObservation,
  WebEntrySummary,
  WebEntry,
  WebReference,
  WebPreview,
  WebPreviewItem,
  WebReceipt,
  WebReceiptItem,
  WebRecoverySlot,
} from '@/interfaces/database/knowledge-web';
import type {
  WebCreateRegistration,
  WebCreateCapture,
  WebCreateLocalCapture,
  WebSelection,
} from '@/interfaces/request/knowledge-web';
import {
  webStorageKey,
  readWebSlot,
  writeWebSlot,
  newWebSlot,
  webNewKey,
  webSha256,
  webDigest,
  equalWebValue,
  latestWebResource,
  checkWebEntry,
  checkWebPreview,
  checkWebReceipt,
  type WebRegistrationPending,
} from '@/utils/knowledge-web';
import {
  validWebData,
  WebVersions,
  compareWebBytes,
  webJsonBytes,
} from '@/utils/knowledge-web-schema';

type UnknownStage =
  | 'registration'
  | 'capture'
  | 'upload'
  | 'acquire'
  | 'observe'
  | 'preview'
  | 'confirm'
  | null;
export function useWebFlow(domain: string, owner: string) {
  const storage = webStorageKey(domain, owner),
    session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const [registration, setRegistration] = useState<WebRegistration | null>(
      null,
    ),
    [capture, setCapture] = useState<WebCapture | null>(null),
    [observation, setObservation] = useState<WebObservation | null>(null),
    [entries, setEntries] = useState<WebEntrySummary[]>([]),
    [preview, setPreview] = useState<WebPreview | null>(null),
    [previewItems, setPreviewItems] = useState<WebPreviewItem[]>([]),
    [receipt, setReceipt] = useState<WebReceipt | null>(null),
    [receiptItems, setReceiptItems] = useState<WebReceiptItem[]>([]);
  const [slot, setSlot] = useState<WebRecoverySlot | null>(null),
    [registrationPending, setRegistrationPending] =
      useState<WebRegistrationPending | null>(null),
    [corrupt, setCorrupt] = useState(false),
    [unknown, setUnknown] = useState<UnknownStage>(null),
    [canResend, setCanResend] = useState(false),
    [canRevise, setCanRevise] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [blocked, setBlocked] = useState(false),
    [progress, setProgress] = useState(0);
  const scope: WebScope = {
      domain,
      owner,
      registration:
        registration?.registration_id || slot?.registration_id || '',
    },
    actions = useWebActions(scope);
  const running = useRef(false),
    active = useRef(true),
    file = useRef<File | null>(null),
    slotRef = useRef<WebRecoverySlot | null>(null),
    captureRef = useRef<WebCapture | null>(null),
    observationRef = useRef<WebObservation | null>(null),
    registrationRef = useRef<WebRegistration | null>(null);
  const missingOriginal = useRef(false);
  useEffect(() => {
    active.current = true;
    try {
      const stored = readWebSlot(storage, domain, owner),
        raw = localStorage.getItem(storage + ':registration');
      if (stored && raw) throw Error('Overlapping recovery records');
      if (stored) {
        slotRef.current = stored;
        setSlot(stored);
        setUnknown(
          stored.confirmation_request
            ? 'confirm'
            : stored.preview_create_request
              ? 'preview'
              : 'capture',
        );
      }
      if (raw) {
        if (new TextEncoder().encode(raw).length > 32768)
          throw Error('Registration recovery budget');
        const value = JSON.parse(raw) as WebRegistrationPending;
        if (
          Object.keys(value).sort().join(',') !==
            'config_sha256,domain_id,kind,owner_id,request' ||
          value.kind !== 'registration' ||
          value.owner_id !== owner ||
          value.domain_id !== domain ||
          !validWebData('CreateRegistration', value.request)
        )
          throw Error('Invalid registration recovery');
        setRegistrationPending(value);
        setUnknown('registration');
      }
    } catch (issue) {
      setCorrupt(true);
      setError(issue);
    }
    return () => {
      active.current = false;
      file.current = null;
    };
  }, [storage, domain, owner]);
  function assertCurrent() {
    if (!active.current || session !== getKnowledgeSessionScope())
      throw new WebRequestError(401, 'knowledge_unauthorized');
  }
  function save(value: WebRecoverySlot) {
    assertCurrent();
    writeWebSlot(storage, value);
    slotRef.current = value;
    setSlot(value);
  }
  function clearVisible(issue: unknown) {
    setBlocked(true);
    setError(issue);
    setRegistration(null);
    captureRef.current = null;
    setCapture(null);
    observationRef.current = null;
    setObservation(null);
    setEntries([]);
    setPreview(null);
    setPreviewItems([]);
    setReceipt(null);
    setReceiptItems([]);
    file.current = null;
    client.removeQueries({
      queryKey: KnowledgeWebKeys.scope(session, domain, owner),
    });
  }
  async function run(
    work: () => Promise<void>,
    _stage: UnknownStage = null,
    lookup = false,
  ) {
    if (running.current || corrupt || blocked) return;
    running.current = true;
    setBusy(true);
    setError(null);
    setCanResend(false);
    missingOriginal.current = false;
    try {
      await actions.run(work);
      if (!active.current) return;
    } catch (issue) {
      if (!active.current) return;
      setError(issue);
      if (
        lookup &&
        missingOriginal.current &&
        issue instanceof WebRequestError &&
        issue.status === 404
      )
        setCanResend(true);
      else if (webScopeFailure(issue)) clearVisible(issue);
      if (
        issue instanceof WebRequestError &&
        issue.failure?.outcome_known &&
        ['capture', 'preview', 'confirm'].includes(_stage || '') &&
        [400, 409, 410, 413, 415, 416, 422].includes(issue.status)
      )
        setCanRevise(true);
    } finally {
      running.current = false;
      if (active.current) setBusy(false);
    }
  }
  async function originalLookup<T>(work: () => Promise<T>) {
    try {
      return await work();
    } catch (issue) {
      if (issue instanceof WebRequestError && issue.status === 404)
        missingOriginal.current = true;
      throw issue;
    }
  }
  function acceptCapture(value: WebCapture, checked = false) {
    assertCurrent();
    if (
      value.domain_id !== domain ||
      value.owner_id !== owner ||
      value.registration_id !== scope.registration
    )
      throw Error('Capture scope mismatch');
    const next = latestWebResource(captureRef.current, value);
    captureRef.current = next;
    setCapture(next);
    const pending = slotRef.current;
    if (pending)
      save({
        ...pending,
        capture_id: next.capture_id,
        capture_sha256: next.raw_sha256,
        observation_id: next.observation_id || pending.observation_id,
      });
    if (checked && (unknown === 'upload' || unknown === 'acquire'))
      setUnknown(null);
    return next;
  }
  async function acceptObservation(value: WebObservation) {
    assertCurrent();
    const cap = captureRef.current;
    if (
      !cap ||
      value.capture_id !== cap.capture_id ||
      value.capture_sha256 !== cap.raw_sha256 ||
      value.expires_at !== cap.expires_at ||
      !equalWebValue(value.producer_versions, cap.producer_versions)
    )
      throw Error('Observation scope mismatch');
    const next = latestWebResource(observationRef.current, value);
    observationRef.current = next;
    setObservation(next);
    if (slotRef.current)
      save({ ...slotRef.current, observation_id: next.observation_id });
    if (next.state === 'ready') {
      const list = await collectWebPages(
        (cursor) => service.entries(scope, cap.capture_id, cursor),
        (item) => item.entry_key,
        1000,
        true,
      );
      if (
        list.length !== next.entry_count ||
        list.filter((item) => item.eligible).length !== next.eligible_count
      )
        throw Error('Observation count mismatch');
      assertCurrent();
      setEntries(list);
    }
  }
  function adoptRegistration(value: WebRegistration) {
    assertCurrent();
    registrationRef.current = value;
    setRegistration(value);
  }
  async function openRegistration(id: string) {
    return run(async () => {
      if (slotRef.current || registrationPending)
        throw Error('Recovery pending');
      const value = (await service.registration({ ...scope, registration: id }))
        .data;
      adoptRegistration(value);
      setUnknown(null);
    });
  }
  async function createRegistration(request: WebCreateRegistration) {
    return run(async () => {
      if (slotRef.current || registrationPending)
        throw Error('Recovery pending');
      if (!validWebData('CreateRegistration', request))
        throw new WebRequestError(400, 'knowledge_invalid_request');
      const pending: WebRegistrationPending = {
        kind: 'registration',
        owner_id: owner,
        domain_id: domain,
        request,
        config_sha256: await webDigest('registration', request),
      };
      if (webJsonBytes(pending) > 32768)
        throw Error('Registration recovery budget');
      localStorage.setItem(storage + ':registration', JSON.stringify(pending));
      setRegistrationPending(pending);
      setUnknown('registration');
      const value = (await service.createRegistration(scope, request)).data;
      adoptRegistration(value);
      localStorage.removeItem(storage + ':registration');
      setRegistrationPending(null);
      setUnknown(null);
    }, 'registration');
  }
  async function createCapture(request: WebCreateCapture, selected?: File) {
    return run(async () => {
      if (!registration || slotRef.current) throw Error('Preparation pending');
      if (request.registration_config_sha256 !== registration.config_sha256)
        throw Error('Configuration mismatch');
      if (request.acquisition_mode === 'local_snapshot') {
        if (
          !selected ||
          selected.name !== request.original_filename ||
          selected.size !== request.declared_byte_length ||
          (await webSha256(await selected.arrayBuffer())) !==
            request.expected_sha256
        )
          throw Error('Local snapshot mismatch');
        file.current = selected;
      }
      const pending = newWebSlot(
        owner,
        domain,
        registration.registration_id,
        request,
      );
      save(pending);
      setUnknown('capture');
      acceptCapture((await service.createCapture(scope, request)).data);
      setUnknown(null);
    }, 'capture');
  }
  async function upload(selected?: File) {
    return run(async () => {
      const cap = captureRef.current,
        request = slotRef.current?.capture_create_request;
      if (
        !cap ||
        !request ||
        request.acquisition_mode !== 'local_snapshot' ||
        !['awaiting_bytes', 'interrupted'].includes(cap.state)
      )
        throw Error('Upload state mismatch');
      const original = selected || file.current;
      if (
        !original ||
        original.name !== request.original_filename ||
        original.size !== request.declared_byte_length ||
        (await webSha256(await original.arrayBuffer())) !==
          request.expected_sha256
      )
        throw Error('Select the exact original snapshot');
      file.current = original;
      setProgress(0);
      setUnknown('upload');
      acceptCapture(
        (
          await service.upload(
            scope,
            cap.capture_id,
            original,
            undefined,
            (event) => setProgress(event.loaded),
          )
        ).data,
      );
      setUnknown(null);
    }, 'upload');
  }
  async function acquire(confirmed: boolean) {
    return run(async () => {
      const cap = captureRef.current;
      if (
        !confirmed ||
        !cap ||
        cap.acquisition_mode !== 'explicit_fetch' ||
        cap.state !== 'awaiting_fetch' ||
        cap.attempt_started_at !== null ||
        unknown
      )
        throw Error('Explicit acquisition required');
      setUnknown('acquire');
      acceptCapture(
        (
          await service.acquire(scope, cap.capture_id, {
            scope_sha256: cap.scope_sha256,
            expected_capture_revision: cap.revision,
            confirmed: true,
          })
        ).data,
      );
      setUnknown(null);
    }, 'acquire');
  }
  async function observe() {
    return run(async () => {
      const cap = captureRef.current;
      if (!cap || cap.state !== 'ready' || unknown)
        throw Error('Capture is not ready');
      setUnknown('observe');
      await acceptObservation(
        (
          await service.observe(scope, cap.capture_id, {
            normalizer_version: WebVersions.normalizer_version,
          })
        ).data,
      );
      setUnknown(null);
    }, 'observe');
  }
  async function selectedArtifacts(
    request: NonNullable<WebRecoverySlot['preview_create_request']>,
  ) {
    const details: WebEntry[] = [],
      references: Record<string, WebReference[]> = {};
    for (const selected of request.selection) {
      const entry = (
        await service.entry(scope, request.capture_id, selected.entry_key)
      ).data;
      await checkWebEntry(
        entry,
        registrationRef.current!.default_classification,
      );
      details.push(entry);
      const refs = await collectWebPages(
        (cursor) =>
          service.entryReferences(
            scope,
            request.capture_id,
            entry.entry_key,
            cursor,
          ),
        (item) => item.reference_id,
        256,
      );
      if (
        refs.length !== entry.reference_count ||
        (await webDigest('references', { references: refs })) !==
          entry.references_sha256
      )
        throw Error('Reference digest mismatch');
      for (const ref of refs)
        if ((await webDigest('reference', ref)) !== ref.reference_id)
          throw Error('Reference identity mismatch');
      references[entry.entry_key] = refs;
    }
    return { details, references };
  }
  async function loadPreview(value: WebPreview, pending: WebRecoverySlot) {
    const request = pending.preview_create_request;
    if (!request) throw Error('Missing original preview request');
    if (
      value.registration_config_sha256 !==
        pending.capture_create_request.registration_config_sha256 ||
      value.scope_sha256 !== registrationRef.current!.scope_sha256 ||
      value.capture_id !== pending.capture_id ||
      value.capture_sha256 !== pending.capture_sha256 ||
      value.policy_version !== registrationRef.current!.policy_version
    )
      throw Error('Preview binding mismatch');
    const items = await collectWebPages(
      (cursor) => service.previewItems(scope, value.preview_id, cursor),
      (item) => item.entry_key,
      32,
      true,
    );
    if (
      items.length !== value.selected_count ||
      items.length !== request.selection.length ||
      (await webDigest('preview_manifest', { ...value, items })) !==
        value.manifest_sha256
    )
      throw Error('Preview manifest mismatch');
    for (let i = 0; i < items.length; i++)
      if (
        items[i].entry_key !== request.selection[i].entry_key ||
        items[i].expected_revision_id !==
          request.selection[i].expected_revision_id ||
        !equalWebValue(
          items[i].classification,
          request.selection[i].classification,
        )
      )
        throw Error('Preview selection mismatch');
    if (value.state === 'ready') {
      const { details, references } = await selectedArtifacts(request);
      await checkWebPreview(value, request, items, details, references);
    }
    assertCurrent();
    setPreview((current) => latestWebResource(current, value));
    setPreviewItems(items);
    save({
      ...pending,
      preview_id: value.preview_id,
      preview_request_sha256: value.request_sha256,
    });
    return items;
  }
  async function createPreview(selection: WebSelection[]) {
    return run(async () => {
      const pending = slotRef.current,
        obs = observationRef.current,
        cap = captureRef.current;
      if (
        !pending ||
        !obs ||
        obs.state !== 'ready' ||
        !obs.observation_sha256 ||
        !cap ||
        cap.state !== 'ready' ||
        pending.preview_create_request ||
        unknown
      )
        throw Error('Preview is not available');
      const request = {
        capture_id: cap.capture_id,
        observation_id: obs.observation_id,
        observation_sha256: obs.observation_sha256,
        idempotency_key: webNewKey(),
        selection: [...selection].sort((a, b) =>
          compareWebBytes(a.entry_key, b.entry_key),
        ),
      };
      if (
        !validWebData('CreatePreview', request) ||
        webJsonBytes(request) > 32768
      )
        throw new WebRequestError(400, 'knowledge_invalid_request');
      const next = {
        ...pending,
        preview_create_key: request.idempotency_key,
        preview_create_request: request,
      };
      save(next);
      setUnknown('preview');
      await loadPreview(
        (await service.createPreview(scope, request)).data,
        next,
      );
      setUnknown(null);
    }, 'preview');
  }
  async function loadReceipt(value: WebReceipt, pending: WebRecoverySlot) {
    const request = pending.confirmation_request;
    if (!request || !pending.preview_create_request)
      throw Error('Missing original confirmation');
    const fixed = (
      await service.previewByKey(scope, pending.preview_create_request)
    ).data;
    // Consumed previews remain recoverable after the temporary capture expires.
    const items = await loadPreview(fixed, pending),
      results = await collectWebPages(
        (cursor) => service.receiptItems(scope, value.import_task_id, cursor),
        (item) => item.entry_key,
        32,
        true,
      );
    await checkWebReceipt(value, request, fixed, results, items);
    assertCurrent();
    setReceipt(value);
    setReceiptItems(results);
    save({ ...slotRef.current!, receipt_id: value.import_task_id });
    setUnknown(null);
  }
  async function confirm(acknowledged: string[], confirmed: boolean) {
    return run(async () => {
      const pending = slotRef.current;
      if (
        !pending ||
        !preview ||
        preview.state !== 'ready' ||
        !confirmed ||
        dayjs(preview.expires_at).isBefore(dayjs()) ||
        pending.confirmation_request ||
        unknown
      )
        throw Error('Confirmation is not available');
      const keys = [...acknowledged].sort(compareWebBytes);
      if (
        !equalWebValue(
          keys,
          previewItems.map((item) => item.entry_key),
        )
      )
        throw Error('Every item needs explicit acknowledgement');
      const request = {
        preview_id: preview.preview_id,
        manifest_sha256: preview.manifest_sha256,
        idempotency_key: webNewKey(),
        confirmed: true as const,
        acknowledged_entry_keys: keys,
      };
      const next = { ...pending, confirmation_request: request };
      save(next);
      setUnknown('confirm');
      await loadReceipt((await service.confirm(scope, request)).data, next);
    }, 'confirm');
  }
  async function recover() {
    return run(
      async () => {
        if (registrationPending) {
          const value = (
            await originalLookup(() =>
              service.registrationByKey(scope, registrationPending.request),
            )
          ).data;
          if (value.config_sha256 !== registrationPending.config_sha256)
            throw Error('Registration mismatch');
          adoptRegistration(value);
          localStorage.removeItem(storage + ':registration');
          setRegistrationPending(null);
          setUnknown(null);
          return;
        }
        const pending = slotRef.current;
        if (!pending) return;
        // This lookup uses the slot's registration before React adopts the result.
        const fixedScope = {
          domain,
          owner,
          registration: pending.registration_id,
        };
        const reg = (await service.registration(fixedScope)).data;
        if (
          reg.config_sha256 !==
          pending.capture_create_request.registration_config_sha256
        )
          throw Error('Configuration mismatch');
        adoptRegistration(reg);
        if (pending.confirmation_request) {
          await loadReceipt(
            (
              await originalLookup(() =>
                service.receiptByKey(fixedScope, pending.confirmation_request!),
              )
            ).data,
            pending,
          );
          return;
        }
        const value = (
          await originalLookup(() =>
            service.captureByKey(fixedScope, pending.capture_create_request),
          )
        ).data;
        acceptCapture(value);
        if (pending.preview_create_request) {
          await loadPreview(
            (
              await originalLookup(() =>
                service.previewByKey(
                  fixedScope,
                  pending.preview_create_request!,
                ),
              )
            ).data,
            slotRef.current!,
          );
          setUnknown(null);
          return;
        }
        if (value.state === 'ready') {
          try {
            await acceptObservation(
              (await service.observation(fixedScope, value.capture_id)).data,
            );
          } catch (issue) {
            if (
              !(
                issue instanceof WebRequestError &&
                issue.status === 404 &&
                pending.observation_id === null
              )
            )
              throw issue;
          }
        }
        setUnknown(null);
      },
      unknown || 'capture',
      true,
    );
  }
  async function resend() {
    return run(async () => {
      if (!canResend) throw Error('Check the original result first');
      if (registrationPending) {
        const result = (
          await service.createRegistration(scope, registrationPending.request)
        ).data;
        adoptRegistration(result);
        localStorage.removeItem(storage + ':registration');
        setRegistrationPending(null);
        setUnknown(null);
        return;
      }
      const pending = slotRef.current;
      if (!pending) throw Error('No original request');
      if (pending.confirmation_request)
        await loadReceipt(
          (await service.confirm(scope, pending.confirmation_request)).data,
          pending,
        );
      else if (pending.preview_create_request) {
        await loadPreview(
          (await service.createPreview(scope, pending.preview_create_request))
            .data,
          pending,
        );
        setUnknown(null);
      } else {
        acceptCapture(
          (await service.createCapture(scope, pending.capture_create_request))
            .data,
        );
        setUnknown(null);
      }
    }, unknown);
  }
  async function recheckCapture() {
    return run(async () => {
      const cap = captureRef.current;
      if (!cap) throw Error('No capture');
      acceptCapture((await service.capture(scope, cap.capture_id)).data);
      setUnknown(null);
    });
  }
  async function recheckObservation() {
    return run(async () => {
      const cap = captureRef.current;
      if (!cap) throw Error('No capture');
      await acceptObservation(
        (await service.observation(scope, cap.capture_id)).data,
      );
      setUnknown(null);
    }, 'observe');
  }
  async function cancelCapture() {
    return run(async () => {
      const cap = captureRef.current;
      if (!cap || unknown) throw Error('Check the result first');
      acceptCapture((await service.cancelCapture(scope, cap.capture_id)).data);
    });
  }
  async function changeSelection() {
    return run(async () => {
      const pending = slotRef.current;
      if (!pending || !preview || pending.confirmation_request || unknown)
        throw Error('Check the result first');
      if (preview.state === 'ready')
        await service.cancelPreview(scope, preview.preview_id);
      save({
        ...pending,
        preview_create_key: null,
        preview_create_request: null,
        preview_id: null,
        preview_request_sha256: null,
      });
      setPreview(null);
      setPreviewItems([]);
    });
  }
  async function revisePreparation() {
    return run(async () => {
      const pending = slotRef.current;
      if (!canRevise || !pending) throw Error('A known rejection is required');
      try {
        if (pending.confirmation_request) {
          await loadReceipt(
            (await service.receiptByKey(scope, pending.confirmation_request))
              .data,
            pending,
          );
          setCanRevise(false);
          return;
        }
        if (pending.preview_create_request) {
          await loadPreview(
            (await service.previewByKey(scope, pending.preview_create_request))
              .data,
            pending,
          );
          setUnknown(null);
          setCanRevise(false);
          return;
        }
        acceptCapture(
          (await service.captureByKey(scope, pending.capture_create_request))
            .data,
        );
        setUnknown(null);
        setCanRevise(false);
        return;
      } catch (issue) {
        if (!(issue instanceof WebRequestError && issue.status === 404))
          throw issue;
      }
      if (pending.confirmation_request && pending.preview_id)
        await service.cancelPreview(scope, pending.preview_id);
      if (pending.preview_create_request) {
        save({
          ...pending,
          preview_create_key: null,
          preview_create_request: null,
          preview_id: null,
          preview_request_sha256: null,
          confirmation_request: null,
        });
        setPreview(null);
        setPreviewItems([]);
      } else {
        localStorage.removeItem(storage);
        slotRef.current = null;
        setSlot(null);
      }
      setUnknown(null);
      setCanRevise(false);
      setCanResend(false);
    });
  }
  function startAnother() {
    if (
      unknown ||
      busy ||
      corrupt ||
      blocked ||
      (!receipt &&
        capture &&
        !['expired', 'cancelled', 'rejected', 'interrupted'].includes(
          capture.state,
        ))
    )
      return;
    localStorage.removeItem(storage);
    slotRef.current = null;
    setSlot(null);
    captureRef.current = null;
    setCapture(null);
    observationRef.current = null;
    setObservation(null);
    setEntries([]);
    setPreview(null);
    setPreviewItems([]);
    setReceipt(null);
    setReceiptItems([]);
    file.current = null;
    setError(null);
    setCanResend(false);
  }
  return {
    scope,
    registration,
    capture,
    observation,
    entries,
    preview,
    previewItems,
    receipt,
    receiptItems,
    slot,
    registrationPending,
    corrupt,
    unknown,
    canResend,
    canRevise,
    busy,
    error,
    blocked,
    progress,
    openRegistration,
    createRegistration,
    createCapture,
    upload,
    acquire,
    observe,
    createPreview,
    confirm,
    recover,
    resend,
    recheckCapture,
    recheckObservation,
    cancelCapture,
    changeSelection,
    revisePreparation,
    startAnother,
    acceptCapture,
    acceptObservation,
    scopeFailure: clearVisible,
  };
}
export type WebFlow = ReturnType<typeof useWebFlow>;
export type WebLocalFields = Omit<
  WebCreateLocalCapture,
  | 'registration_config_sha256'
  | 'idempotency_key'
  | 'acquisition_mode'
  | 'original_filename'
  | 'declared_byte_length'
  | 'expected_sha256'
>;

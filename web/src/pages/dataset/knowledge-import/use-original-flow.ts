import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type {
  OriginalClassification,
  OriginalRegistration,
  OriginalUpload,
  OriginalReceipt,
} from '@/interfaces/database/knowledge-original';
import type {
  OriginalRegistrationRequest,
  CreateOriginalUpload,
} from '@/interfaces/request/knowledge-original';
import service, {
  OriginalRequestError,
} from '@/services/knowledge-original-service';
import foundationService, {
  KnowledgeContractVersion,
  KnowledgeRequestError,
} from '@/services/knowledge-foundation-service';
import { validKnowledgeData } from '@/utils/knowledge-foundation-schema';
import {
  useOriginalActions,
  KnowledgeOriginalKeys,
  originalScopeFailure,
} from '@/hooks/use-knowledge-original';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from '@/hooks/use-knowledge-foundation';
import {
  OriginalMaxBytes,
  OriginalMediaTypes,
  validOriginalData,
} from '@/utils/knowledge-original-schema';
import {
  originalScope,
  originalStorageKey,
  originalDigest,
  originalUnexpired,
  latestOriginalUpload,
  checkOriginalUpload,
  checkOriginalReceipt,
  loadOriginalRecovery,
  saveOriginalRecovery,
  clearOriginalRecovery,
  type OriginalCreatePending,
  type OriginalConfirmPending,
} from '@/utils/knowledge-original';

class OriginalRecoveryChanged extends Error {}

export function useOriginalFlow(
  domain: string,
  owner: string,
  storageKey: string,
) {
  const client = useQueryClient(),
    actions = useOriginalActions(domain);
  const [initial] = useState(() => {
    try {
      if (storageKey !== originalStorageKey(domain, owner))
        throw Error('Original recovery scope mismatch');
      return {
        create: loadOriginalRecovery(storageKey, 'create', domain, owner),
        confirm: loadOriginalRecovery(storageKey, 'confirm', domain, owner),
        error: null as unknown,
      };
    } catch (error) {
      return { create: null, confirm: null, error };
    }
  });
  const [createPending, setCreatePending] =
    useState<OriginalCreatePending | null>(initial.create);
  const [confirmPending, setConfirmPending] =
    useState<OriginalConfirmPending | null>(initial.confirm);
  const [registration, setRegistration] = useState<OriginalRegistration | null>(
    null,
  );
  const [upload, setUpload] = useState<OriginalUpload | null>(null);
  const [receipt, setReceipt] = useState<OriginalReceipt | null>(null);
  const [error, setError] = useState<unknown>(initial.error),
    [busy, setBusy] = useState(false);
  const [blocked, setBlocked] = useState(false),
    [unknown, setUnknown] = useState(!!initial.create || !!initial.confirm);
  const [canResend, setCanResend] = useState(false),
    [canReset, setCanReset] = useState(false);
  const [progress, setProgress] = useState(0),
    [cancelling, setCancelling] = useState(false);
  const [operation, setOperation] = useState(0),
    [freshRegistration, setFreshRegistration] = useState(false),
    [recoveryChanged, setRecoveryChanged] = useState(false),
    [lockingUnavailable, setLockingUnavailable] = useState(false);
  const [scope] = useState(() => ({
    domain,
    owner,
    storageKey,
    session: getKnowledgeSessionScope(),
  }));
  const view = useRef(scope);
  view.current = {
    domain,
    owner,
    storageKey,
    session: getKnowledgeSessionScope(),
  };
  const epoch = useRef(0),
    lostRecovery = useRef(false),
    lostScope = useRef(false),
    recovery = useRef({ create: initial.create, confirm: initial.confirm }),
    completedReceipt = useRef<OriginalReceipt | null>(null);
  if (
    scope.domain !== domain ||
    scope.owner !== owner ||
    scope.storageKey !== storageKey ||
    scope.session !== view.current.session
  ) {
    lostScope.current = true;
    lostRecovery.current = true;
  }
  const Alive = useRef(true),
    Busy = useRef(false),
    Latest = useRef<OriginalUpload | null>(null);
  const Controller = useRef<AbortController | null>(null),
    Cancelling = useRef(false),
    Action = useRef(actions);
  const Confirmation = useRef(confirmPending);
  const RejectedConfirmation = useRef<string | null>(null);
  Confirmation.current = confirmPending;
  Action.current = actions;
  const isCurrent = useCallback(
    () =>
      Alive.current &&
      !lostRecovery.current &&
      epoch.current === operation &&
      scope.domain === view.current.domain &&
      scope.owner === view.current.owner &&
      scope.storageKey === view.current.storageKey &&
      scope.session === view.current.session &&
      scope.session === getKnowledgeSessionScope(),
    [operation, scope],
  );
  useEffect(() => {
    Alive.current = true;
    return () => {
      Alive.current = false;
      Controller.current?.abort();
    };
  }, []);
  const fail = useCallback(
    (value: unknown) => {
      if (!isCurrent()) return;
      setError(value);
      if (
        value instanceof OriginalRecoveryChanged ||
        originalScopeFailure(value)
      ) {
        lostRecovery.current = true;
        completedReceipt.current = null;
        Controller.current?.abort();
        setRecoveryChanged(true);
        RejectedConfirmation.current = null;
        setCanReset(false);
        setCanResend(false);
        setBlocked(true);
        setRegistration(null);
        setUpload(null);
        setReceipt(null);
        if (originalScopeFailure(value))
          client.removeQueries({
            queryKey: KnowledgeOriginalKeys.scope(
              getKnowledgeSessionScope(),
              domain,
            ),
          });
      }
    },
    [client, domain, isCurrent],
  );
  const checkRecovery = useCallback(() => {
    if (!isCurrent()) throw Error('Inactive original operation');
    const stored = {
      create: loadOriginalRecovery(storageKey, 'create', domain, owner),
      confirm: loadOriginalRecovery(storageKey, 'confirm', domain, owner),
    };
    if (JSON.stringify(stored) !== JSON.stringify(recovery.current))
      throw new OriginalRecoveryChanged(
        'Original recovery changed in another view',
      );
  }, [isCurrent, storageKey, domain, owner]);
  useEffect(() => {
    const check = (event: StorageEvent | Event) => {
      if (
        event instanceof StorageEvent &&
        event.key !== null &&
        event.key !== storageKey + ':create' &&
        event.key !== storageKey + ':confirm'
      )
        return;
      if (!isCurrent() || initial.error) return;
      try {
        checkRecovery();
      } catch (issue) {
        fail(issue);
      }
    };
    window.addEventListener('storage', check);
    window.addEventListener('focus', check);
    return () => {
      window.removeEventListener('storage', check);
      window.removeEventListener('focus', check);
    };
  }, [checkRecovery, isCurrent, fail, initial.error, storageKey]);
  const rememberUpload = useCallback(
    (value: OriginalUpload) => {
      if (!isCurrent()) return value;
      const pending = recovery.current.create;
      if (
        pending?.kind !== 'upload' ||
        pending.registration_id !== value.registration_id ||
        (pending.upload_id !== null && pending.upload_id !== value.upload_id)
      )
        return value;
      checkRecovery();
      Latest.current = latestOriginalUpload(Latest.current, value);
      setUpload(Latest.current);
      if (!Confirmation.current && !Busy.current) setUnknown(false);
      return Latest.current;
    },
    [checkRecovery, isCurrent],
  );
  async function exclusive(work: () => Promise<void>) {
    if (!isCurrent()) throw Error('Inactive original operation');
    const locks = navigator.locks;
    if (!locks || typeof locks.request !== 'function') {
      setLockingUnavailable(true);
      throw Error('Browser recovery locking unavailable');
    }
    setLockingUnavailable(false);
    let entered = false;
    try {
      await locks.request(storageKey + ':write', async () => {
        entered = true;
        checkRecovery();
        await work();
      });
    } catch (issue) {
      if (!entered && isCurrent()) setLockingUnavailable(true);
      throw issue;
    }
  }
  async function request<T>(work: () => Promise<T>) {
    checkRecovery();
    const result = await Action.current.run(() => {
      checkRecovery();
      return work();
    });
    checkRecovery();
    return result;
  }
  async function run(work: () => Promise<void>) {
    if (Busy.current || Cancelling.current || initial.error || !isCurrent())
      return false;
    Busy.current = true;
    setBusy(true);
    setError(null);
    try {
      await exclusive(work);
      return true;
    } catch (issue) {
      if ((issue as { code?: string }).code !== 'ERR_CANCELED') fail(issue);
      else if (Alive.current && !Cancelling.current) setUnknown(true);
      return false;
    } finally {
      Busy.current = false;
      if (Alive.current) setBusy(false);
    }
  }
  function saveCreate(value: OriginalCreatePending) {
    checkRecovery();
    saveOriginalRecovery(storageKey, 'create', value);
    recovery.current.create = value;
    setCreatePending(value);
    checkRecovery();
  }
  function saveConfirm(value: OriginalConfirmPending) {
    checkRecovery();
    saveOriginalRecovery(storageKey, 'confirm', value);
    recovery.current.confirm = value;
    setConfirmPending(value);
    checkRecovery();
  }
  function clearRecovery(kind: 'create' | 'confirm') {
    checkRecovery();
    clearOriginalRecovery(storageKey, kind);
    recovery.current[kind] = null;
    if (kind === 'create') setCreatePending(null);
    else setConfirmPending(null);
    checkRecovery();
  }
  function rememberReceipt(value: OriginalReceipt) {
    checkRecovery();
    completedReceipt.current = value;
    setReceipt(value);
  }
  function checkCompletedOperation() {
    const saved = completedReceipt.current;
    if (!saved || saved.import_task_id !== receipt?.import_task_id)
      throw Error('Restore the completed original receipt first');
    const pending = recovery.current;
    if (pending.confirm)
      checkOriginalReceipt(
        saved,
        domain,
        pending.confirm.registration_id,
        pending.confirm.request,
      );
    if (
      pending.create &&
      (pending.create.registration_id !== saved.registration_id ||
        (pending.create.kind === 'upload' &&
          pending.create.upload_id !== saved.upload_id))
    )
      throw Error('Resolve the other original request first');
  }
  function finishPreparation(isNew: boolean) {
    epoch.current += 1;
    setOperation(epoch.current);
    setFreshRegistration(isNew);
    completedReceipt.current = null;
    setUpload(null);
    Latest.current = null;
    setReceipt(null);
    setUnknown(false);
    setCanResend(false);
    setCanReset(false);
    RejectedConfirmation.current = null;
    setBlocked(false);
    setProgress(0);
  }
  function acceptRegistration(value: OriginalRegistration) {
    setRegistration(value);
    setBlocked(false);
    setUnknown(false);
    setCanResend(false);
  }
  async function acceptUpload(
    value: OriginalUpload,
    original: Extract<OriginalCreatePending, { kind: 'upload' }>,
  ) {
    await checkOriginalUpload(
      value,
      original.request,
      domain,
      owner,
      original.registration_id,
    );
    if (original.upload_id && value.upload_id !== original.upload_id)
      throw Error('Recovered another upload');
    const next = rememberUpload(value);
    saveCreate({ ...original, upload_id: next.upload_id });
    setUnknown(false);
    setBlocked(false);
    setCanResend(false);
    return next;
  }
  function acceptReceipt(
    value: OriginalReceipt,
    original: OriginalConfirmPending,
  ) {
    checkOriginalReceipt(
      value,
      domain,
      original.registration_id,
      original.request,
      Latest.current || undefined,
    );
    if (
      original.import_task_id &&
      original.import_task_id !== value.import_task_id
    )
      throw Error('Recovered another receipt');
    saveConfirm({ ...original, import_task_id: value.import_task_id });
    rememberReceipt(value);
    RejectedConfirmation.current = null;
    setCanReset(false);
    setUnknown(false);
    setBlocked(false);
    setCanResend(false);
    void client.invalidateQueries({
      queryKey: KnowledgeOriginalKeys.lists(getKnowledgeSessionScope(), domain),
    });
    void client.invalidateQueries({
      queryKey: KnowledgeOriginalKeys.target(
        getKnowledgeSessionScope(),
        domain,
        original.registration_id,
        value.source.relative_path,
      ),
    });
    void client.invalidateQueries({
      queryKey: KnowledgeKeys.domainScope(getKnowledgeSessionScope(), domain),
    });
  }
  function mayCreate() {
    if (
      confirmPending ||
      (createPending && (createPending.kind === 'upload' || !registration))
    )
      throw Error('Resolve the original request first');
  }
  async function sendConfirmation(original: OriginalConfirmPending) {
    RejectedConfirmation.current = null;
    setCanReset(false);
    try {
      const result = await request(() =>
        service.confirm(domain, original.registration_id, original.request),
      );
      acceptReceipt(result.data, original);
    } catch (issue) {
      if (
        issue instanceof OriginalRequestError &&
        issue.status === 409 &&
        issue.code === 'revision_changed'
      )
        RejectedConfirmation.current = original.request.idempotency_key;
      throw issue;
    }
  }
  async function findConfirmation(original: OriginalConfirmPending) {
    try {
      const result = await request(() => service.receiptByKey(original));
      acceptReceipt(result.data, original);
      return true;
    } catch (issue) {
      if (
        issue instanceof OriginalRequestError &&
        issue.status === 404 &&
        issue.code === 'knowledge_not_found'
      )
        return false;
      throw issue;
    }
  }
  async function readRetryScope() {
    let currentDomain;
    try {
      currentDomain = await request(() => foundationService.domain(domain));
    } catch (issue) {
      if (
        issue instanceof KnowledgeRequestError &&
        issue.status === 502 &&
        issue.code === 'knowledge_invalid_response'
      )
        throw new OriginalRequestError(409, 'policy_changed');
      throw issue;
    }
    if (currentDomain.domain_id !== domain || currentDomain.owner_id !== owner)
      throw new OriginalRequestError(404, 'knowledge_not_found');
    if (currentDomain.contract_version !== KnowledgeContractVersion)
      throw new OriginalRequestError(409, 'knowledge_contract_version');
    if (!validKnowledgeData('Domain', currentDomain))
      throw new OriginalRequestError(409, 'policy_changed');
  }
  async function readCreateByKey<T>(work: () => Promise<T>): Promise<T | null> {
    try {
      return await request(work);
    } catch (issue) {
      if (
        !(issue instanceof OriginalRequestError) ||
        issue.status !== 404 ||
        issue.code !== 'knowledge_not_found'
      )
        throw issue;
      await readRetryScope();
      setBlocked(false);
      setUnknown(true);
      setCanResend(true);
      setCanReset(false);
      return null;
    }
  }
  async function readConfirmationContext(original: OriginalConfirmPending) {
    if (
      createPending?.kind !== 'upload' ||
      createPending.registration_id !== original.registration_id ||
      createPending.upload_id !== original.request.upload_id
    )
      throw Error('Restore the original upload request first');
    const currentDomain = await request(() => foundationService.domain(domain));
    if (currentDomain.domain_id !== domain || currentDomain.owner_id !== owner)
      throw new OriginalRequestError(404, 'knowledge_not_found');
    const reg = (
      await request(() =>
        service.registration(domain, original.registration_id, owner),
      )
    ).data;
    const value = (
      await request(() =>
        service.upload(
          domain,
          original.registration_id,
          original.request.upload_id,
        ),
      )
    ).data;
    await checkOriginalUpload(
      value,
      createPending.request,
      domain,
      owner,
      original.registration_id,
    );
    checkRecovery();
    if (
      value.upload_id !== original.request.upload_id ||
      (['ready', 'committed'].includes(value.state) &&
        value.manifest_sha256 !== original.request.manifest_sha256)
    )
      throw Error('Recovered another confirmation upload');
    return { registration: reg, upload: value };
  }
  async function committedConfirmation(
    value: OriginalUpload,
    original: OriginalConfirmPending,
  ) {
    if (value.state !== 'committed') return false;
    if (!value.import_task_id) throw Error('Committed upload has no receipt');
    acceptReceipt(
      (
        await request(() =>
          service.receipt(domain, value.registration_id, value.import_task_id!),
        )
      ).data,
      original,
    );
    return true;
  }
  const openRegistration = (reg: string) =>
    run(async () => {
      if (
        confirmPending ||
        (createPending &&
          (createPending.kind === 'upload' ||
            createPending.registration_id !== reg))
      )
        throw Error('Resolve the original request first');
      const result = await request(() =>
        service.registration(domain, reg, owner),
      );
      if (
        createPending?.kind === 'registration' &&
        result.data.config_sha256 !== createPending.config_sha256
      )
        throw Error('Registration configuration changed');
      acceptRegistration(result.data);
      saveCreate({
        ...originalScope(domain, owner),
        kind: 'registration',
        registration_key: result.data.registration_key,
        config_sha256: result.data.config_sha256,
        registration_id: result.data.registration_id,
      });
    });
  const register = (input: OriginalRegistrationRequest) =>
    run(async () => {
      if (confirmPending || createPending?.kind === 'upload')
        throw Error('Resolve the original request first');
      if (createPending && !registration && !canResend)
        throw Error('Check the original registration key before resending');
      const digest = await originalDigest('registration', {
        ...input,
        domain_id: domain,
        owner_id: owner,
      });
      if (
        createPending &&
        (createPending.config_sha256 !== digest ||
          createPending.registration_key !== input.registration_key)
      )
        throw Error('Re-enter the original registration configuration');
      if (createPending && !registration) await readRetryScope();
      const original: OriginalCreatePending = {
        ...originalScope(domain, owner),
        kind: 'registration',
        registration_key: input.registration_key,
        config_sha256: digest,
        registration_id: null,
      };
      saveCreate(original);
      setUnknown(true);
      const result = await request(() =>
        service.register(domain, owner, input),
      );
      saveCreate({ ...original, registration_id: result.data.registration_id });
      acceptRegistration(result.data);
    });
  async function putOriginal(
    file: File,
    current: OriginalUpload,
    original: Extract<OriginalCreatePending, { kind: 'upload' }>,
  ) {
    if (
      !['awaiting_bytes', 'interrupted'].includes(current.state) ||
      file.name !== current.original_filename ||
      file.size !== current.declared_byte_length ||
      !originalUnexpired(current)
    )
      throw Error('File does not match the retryable upload');
    setProgress(0);
    setUnknown(true);
    const controller = new AbortController();
    Controller.current = controller;
    try {
      const result = await request(() =>
        service.put(
          domain,
          current.registration_id,
          current.upload_id,
          file,
          (event) => {
            if (isCurrent())
              setProgress(
                Math.max(0, Math.min(100, (event.loaded / file.size) * 100)),
              );
          },
          controller.signal,
        ),
      );
      await acceptUpload(result.data, original);
    } finally {
      if (Controller.current === controller) Controller.current = null;
    }
  }
  const prepare = (
    file: File,
    path: string,
    classification: OriginalClassification,
  ) =>
    run(async () => {
      mayCreate();
      if (!registration || file.size < 1 || file.size > OriginalMaxBytes)
        throw new OriginalRequestError(413, 'upload_too_large');
      const target = (
        await request(() =>
          service.target(domain, registration.registration_id, path),
        )
      ).data;
      const input: CreateOriginalUpload = {
        original_filename: file.name,
        relative_path: path,
        classification,
        declared_byte_length: file.size,
        declared_media_type: OriginalMediaTypes.includes(file.type)
          ? file.type
          : 'application/octet-stream',
        expected_sha256: null,
        expected_revision_id: target.source?.source_revision_id || null,
        idempotency_key: crypto.randomUUID(),
      };
      if (!validOriginalData('CreateUpload', input))
        throw new OriginalRequestError(400, 'knowledge_invalid_request');
      const original: Extract<OriginalCreatePending, { kind: 'upload' }> = {
        ...originalScope(domain, owner),
        kind: 'upload',
        registration_id: registration.registration_id,
        request: input,
        request_sha256: await originalDigest('request', {
          ...input,
          domain_id: domain,
          owner_id: owner,
          registration_id: registration.registration_id,
        }),
        upload_id: null,
      };
      saveCreate(original);
      setUnknown(true);
      setCanReset(false);
      let value: OriginalUpload;
      try {
        value = (
          await request(() =>
            service.create(domain, registration.registration_id, owner, input),
          )
        ).data;
      } catch (issue) {
        if (
          issue instanceof OriginalRequestError &&
          [
            'knowledge_invalid_request',
            'upload_too_large',
            'upload_quota_exceeded',
            'upload_format_unsupported',
            'revision_changed',
          ].includes(issue.code)
        )
          setCanReset(true);
        throw issue;
      }
      value = await acceptUpload(value, original);
      await putOriginal(file, value, {
        ...original,
        upload_id: value.upload_id,
      });
    });
  const retransmit = (file: File) =>
    run(async () => {
      if (!upload || createPending?.kind !== 'upload' || confirmPending) return;
      const current = await acceptUpload(
        (await request(() => service.uploadByKey(createPending))).data,
        createPending,
      );
      await putOriginal(file, current, createPending);
    });
  const confirm = (checked: boolean) =>
    run(async () => {
      if (
        !checked ||
        !upload ||
        upload.state !== 'ready' ||
        !upload.manifest_sha256 ||
        !originalUnexpired(upload) ||
        confirmPending ||
        unknown
      )
        throw Error('Explicit confirmation of a ready upload is required');
      const original: OriginalConfirmPending = {
        ...originalScope(domain, owner),
        registration_id: upload.registration_id,
        request: {
          upload_id: upload.upload_id,
          manifest_sha256: upload.manifest_sha256,
          idempotency_key: crypto.randomUUID(),
          confirmed: true,
        },
        import_task_id: null,
      };
      saveConfirm(original);
      setUnknown(true);
      await sendConfirmation(original);
    });
  const recover = () =>
    run(async () => {
      setCanResend(false);
      if (confirmPending) {
        setCanReset(false);
        if (await findConfirmation(confirmPending)) return;
        const context = await readConfirmationContext(confirmPending);
        const current = rememberUpload(context.upload);
        setRegistration(context.registration);
        if (await committedConfirmation(current, confirmPending)) return;
        setBlocked(false);
        setUnknown(true);
        setCanResend(true);
        const rejected =
          RejectedConfirmation.current ===
          confirmPending.request.idempotency_key;
        setCanReset(rejected);
        setError(
          new OriginalRequestError(
            rejected ? 409 : 404,
            rejected ? 'revision_changed' : 'knowledge_not_found',
          ),
        );
        return;
      }
      try {
        if (createPending?.kind === 'registration') {
          const result = await readCreateByKey(() =>
            service.registrationByKey(
              domain,
              owner,
              createPending.registration_key,
              createPending.config_sha256,
            ),
          );
          if (!result) return;
          saveCreate({
            ...createPending,
            registration_id: result.data.registration_id,
          });
          acceptRegistration(result.data);
          return;
        }
        if (createPending?.kind === 'upload') {
          const result = await readCreateByKey(() =>
            service.uploadByKey(createPending),
          );
          if (!result) return;
          const value = await acceptUpload(result.data, createPending);
          const reg = await request(() =>
            service.registration(domain, value.registration_id, owner),
          );
          acceptRegistration(reg.data);
          if (value.state === 'committed' && value.import_task_id)
            rememberReceipt(
              (
                await request(() =>
                  service.receipt(
                    domain,
                    value.registration_id,
                    value.import_task_id!,
                  ),
                )
              ).data,
            );
        }
      } catch (issue) {
        if (
          issue instanceof OriginalRequestError &&
          [0, 404, 503].includes(issue.status)
        )
          setCanResend(true);
        throw issue;
      }
    });
  const resend = () =>
    run(async () => {
      if (!canResend) return;
      setUnknown(true);
      setCanResend(false);
      if (confirmPending) {
        const context = await readConfirmationContext(confirmPending);
        rememberUpload(context.upload);
        setRegistration(context.registration);
        setBlocked(false);
        await sendConfirmation(confirmPending);
        return;
      }
      if (createPending?.kind === 'upload') {
        await readRetryScope();
        await acceptUpload(
          (
            await request(() =>
              service.create(
                domain,
                createPending.registration_id,
                owner,
                createPending.request,
              ),
            )
          ).data,
          createPending,
        );
      }
    });
  async function cancel() {
    if (Cancelling.current || !isCurrent() || !upload || receipt) return;
    Cancelling.current = true;
    setCancelling(true);
    Controller.current?.abort();
    try {
      await exclusive(async () => {
        const result = await request(() =>
          service.cancel(domain, upload.registration_id, upload.upload_id),
        );
        const value = rememberUpload(result.data);
        setUnknown(false);
        setError(null);
        if (value.state === 'committed' && value.import_task_id) {
          const saved = (
            await request(() =>
              service.receipt(
                domain,
                value.registration_id,
                value.import_task_id!,
              ),
            )
          ).data;
          if (saved.upload_id !== value.upload_id)
            throw Error('Cancellation returned another receipt');
          if (confirmPending) acceptReceipt(saved, confirmPending);
          else rememberReceipt(saved);
        }
      });
    } catch (issue) {
      fail(issue);
      setUnknown(true);
    } finally {
      Cancelling.current = false;
      if (Alive.current) setCancelling(false);
    }
  }
  const startAnother = () =>
    run(async () => {
      if (receipt) checkCompletedOperation();
      let recoveredRegistration: OriginalRegistration | null = null;
      if (confirmPending && !receipt) {
        if (
          !canReset ||
          RejectedConfirmation.current !==
            confirmPending.request.idempotency_key
        )
          throw Error('Check the rejected original confirmation first');
        setCanReset(false);
        setCanResend(false);
        if (await findConfirmation(confirmPending)) return;
        const context = await readConfirmationContext(confirmPending);
        let current = rememberUpload(context.upload);
        recoveredRegistration = context.registration;
        if (await committedConfirmation(current, confirmPending)) return;
        if (!['rejected', 'cancelled', 'expired'].includes(current.state)) {
          current = rememberUpload(
            (
              await request(() =>
                service.cancel(
                  domain,
                  current.registration_id,
                  current.upload_id,
                ),
              )
            ).data,
          );
          if (await committedConfirmation(current, confirmPending)) return;
        }
        if (!['rejected', 'cancelled', 'expired'].includes(current.state))
          throw Error('Cancel the original upload before new preparation');
      }
      if (
        !receipt &&
        !canReset &&
        (!upload ||
          !['rejected', 'cancelled', 'expired'].includes(upload.state))
      )
        throw Error('Resolve or cancel the original upload first');
      const reg =
        recoveredRegistration ||
        registration ||
        (createPending?.registration_id
          ? (
              await request(() =>
                service.registration(
                  domain,
                  createPending.registration_id!,
                  owner,
                ),
              )
            ).data
          : null);
      if (!reg) throw Error('Restore the registration first');
      clearRecovery('confirm');
      saveCreate({
        ...originalScope(domain, owner),
        kind: 'registration',
        registration_key: reg.registration_key,
        config_sha256: reg.config_sha256,
        registration_id: reg.registration_id,
      });
      setRegistration(reg);
      finishPreparation(false);
    });
  const startNewRegistration = () => {
    if (!completedReceipt.current || !receipt || blocked || unknown)
      return Promise.resolve(false);
    return run(async () => {
      checkCompletedOperation();
      clearRecovery('confirm');
      clearRecovery('create');
      setRegistration(null);
      finishPreparation(true);
    });
  };
  const reauthorize = () => {
    if (
      !isCurrent() ||
      Busy.current ||
      Cancelling.current ||
      createPending ||
      confirmPending
    )
      return;
    setBlocked(false);
    setError(null);
    Action.current.reset();
  };
  const active = isCurrent();
  return {
    registration: active ? registration : null,
    upload: active ? upload : null,
    receipt: active ? receipt : null,
    createPending: active ? createPending : null,
    confirmPending: active ? confirmPending : null,
    busy,
    error: error || (active ? null : new Error('Inactive original view')),
    blocked: !active || blocked,
    unknown,
    canResend,
    canReset,
    progress,
    cancelling,
    freshRegistration,
    recoveryChanged: recoveryChanged || lostScope.current,
    lockingUnavailable,
    corruptRecovery: !!initial.error,
    fail,
    rememberUpload,
    openRegistration,
    register,
    prepare,
    retransmit,
    confirm,
    recover,
    resend,
    cancel,
    startAnother,
    startNewRegistration,
    reauthorize,
  };
}
export type OriginalFlow = ReturnType<typeof useOriginalFlow>;

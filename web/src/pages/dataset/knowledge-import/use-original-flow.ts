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
import foundationService from '@/services/knowledge-foundation-service';
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

export function useOriginalFlow(
  domain: string,
  owner: string,
  storageKey: string,
) {
  const client = useQueryClient(),
    actions = useOriginalActions(domain);
  const [initial] = useState(() => {
    try {
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
  useEffect(() => {
    Alive.current = true;
    return () => {
      Alive.current = false;
      Controller.current?.abort();
    };
  }, []);
  const fail = useCallback(
    (value: unknown) => {
      if (!Alive.current) return;
      setError(value);
      if (originalScopeFailure(value)) {
        RejectedConfirmation.current = null;
        setCanReset(false);
        setCanResend(false);
        setBlocked(true);
        setRegistration(null);
        setUpload(null);
        setReceipt(null);
        client.removeQueries({
          queryKey: KnowledgeOriginalKeys.scope(
            getKnowledgeSessionScope(),
            domain,
          ),
        });
      }
    },
    [client, domain],
  );
  const rememberUpload = useCallback((value: OriginalUpload) => {
    if (!Alive.current) return value;
    Latest.current = latestOriginalUpload(Latest.current, value);
    setUpload(Latest.current);
    if (!Confirmation.current && !Busy.current) setUnknown(false);
    return Latest.current;
  }, []);
  async function request<T>(work: () => Promise<T>) {
    if (!Alive.current) throw Error('Inactive original view');
    const result = await Action.current.run(work);
    if (!Alive.current) throw Error('Inactive original view');
    return result;
  }
  async function run(work: () => Promise<void>) {
    if (Busy.current || initial.error) return;
    Busy.current = true;
    setBusy(true);
    setError(null);
    try {
      await work();
    } catch (issue) {
      if ((issue as { code?: string }).code !== 'ERR_CANCELED') fail(issue);
      else if (Alive.current && !Cancelling.current) setUnknown(true);
    } finally {
      Busy.current = false;
      if (Alive.current) setBusy(false);
    }
  }
  function saveCreate(value: OriginalCreatePending) {
    saveOriginalRecovery(storageKey, 'create', value);
    setCreatePending(value);
  }
  function saveConfirm(value: OriginalConfirmPending) {
    saveOriginalRecovery(storageKey, 'confirm', value);
    setConfirmPending(value);
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
    setReceipt(value);
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
            if (Alive.current)
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
          const result = await request(() =>
            service.registrationByKey(
              domain,
              owner,
              createPending.registration_key,
              createPending.config_sha256,
            ),
          );
          saveCreate({
            ...createPending,
            registration_id: result.data.registration_id,
          });
          acceptRegistration(result.data);
          return;
        }
        if (createPending?.kind === 'upload') {
          const value = await acceptUpload(
            (await request(() => service.uploadByKey(createPending))).data,
            createPending,
          );
          const reg = await request(() =>
            service.registration(domain, value.registration_id, owner),
          );
          acceptRegistration(reg.data);
          if (value.state === 'committed' && value.import_task_id)
            setReceipt(
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
      if (createPending?.kind === 'upload')
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
    });
  async function cancel() {
    if (Cancelling.current || !upload || receipt) return;
    Cancelling.current = true;
    setCancelling(true);
    Controller.current?.abort();
    try {
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
        else setReceipt(saved);
      }
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
      clearOriginalRecovery(storageKey, 'confirm');
      saveCreate({
        ...originalScope(domain, owner),
        kind: 'registration',
        registration_key: reg.registration_key,
        config_sha256: reg.config_sha256,
        registration_id: reg.registration_id,
      });
      setConfirmPending(null);
      setUpload(null);
      Latest.current = null;
      setReceipt(null);
      setUnknown(false);
      setCanResend(false);
      setCanReset(false);
      RejectedConfirmation.current = null;
      setBlocked(false);
      setRegistration(reg);
      setProgress(0);
    });
  const reauthorize = () =>
    run(async () => {
      if (createPending || confirmPending) return;
      setBlocked(false);
      setError(null);
      Action.current.reset();
    });
  return {
    registration,
    upload,
    receipt,
    createPending,
    confirmPending,
    busy,
    error,
    blocked,
    unknown,
    canResend,
    canReset,
    progress,
    cancelling,
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
    reauthorize,
  };
}
export type OriginalFlow = ReturnType<typeof useOriginalFlow>;

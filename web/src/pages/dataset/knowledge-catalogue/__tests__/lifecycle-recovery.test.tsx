type ReactNode = import('react').ReactNode;
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import request from '@/utils/next-request';
import { getAuthorization } from '@/utils/authorization-util';
import { useLifecycleFlow } from '../use-lifecycle-flow';
import { useOriginalFlow } from '../../knowledge-import/use-original-flow';
import originalService, {
  OriginalRequestError,
} from '@/services/knowledge-original-service';
import foundationService, {
  KnowledgeRequestError,
} from '@/services/knowledge-foundation-service';
import {
  originalScope,
  originalStorageKey,
  loadOriginalRecovery,
  saveOriginalRecovery,
} from '@/utils/knowledge-original';
import * as OriginalFixture from '../../knowledge-import/__tests__/original-fixture';
import {
  createLifecycleBackend,
  Scope,
  Owner,
  Domain,
  detail,
  recovery,
} from './lifecycle-fixture';
import {
  saveLifecycleRecovery,
  lifecycleStorageKey,
  readLifecycleRecovery,
} from '@/utils/knowledge-lifecycle';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-lifecycle-session'),
}));
const Http = request as unknown as jest.Mock;
function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}
let backend: import('./lifecycle-fixture').LifecycleBackend;
beforeEach(() => {
  localStorage.clear();
  backend = createLifecycleBackend();
  Http.mockReset().mockImplementation(backend.http);
  (getAuthorization as jest.Mock).mockReturnValue(
    'synthetic-lifecycle-session',
  );
});
afterEach(() => jest.restoreAllMocks());
it('persists the original before the first POST and GET-recovers a lost result after remount without another POST', async () => {
  backend.losePost = true;
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) => {
    if (config.method === 'post')
      expect(readLifecycleRecovery(Owner, Domain)?.request).toEqual(
        JSON.parse(config.data),
      );
    return original(config);
  });
  const first = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await first.result.current.submit(detail(), 'archive', {}, true);
  });
  expect(first.result.current.unresolved).toBe(true);
  expect(first.result.current.receipt).toBeNull();
  const key = first.result.current.pending!.request.idempotency_key;
  first.unmount();
  const next = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  expect(next.result.current.pending?.request.idempotency_key).toBe(key);
  await act(async () => {
    await next.result.current.recover();
  });
  expect(next.result.current.error).toBeNull();
  expect(next.result.current.receipt?.after.state).toBe('archived');
  expect(next.result.current.unresolved).toBe(false);
  expect(backend.posts).toHaveLength(1);
});
it('keeps missing-by-key unknown and only explicitly resends the identical request after checking current access', async () => {
  backend.conflict = true;
  const { result } = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await result.current.submit(detail(), 'archive', {}, true);
  });
  expect(result.current.unresolved).toBe(true);
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.canResend).toBe(true);
  const original = result.current.pending!.request;
  backend.conflict = false;
  await act(async () => {
    await result.current.resend();
  });
  expect(result.current.error).toBeNull();
  expect(backend.posts).toHaveLength(2);
  expect(backend.posts[1].request).toEqual(original);
});
it('requires a determinate rejection plus a checked missing result before explicit abandonment and new preparation', async () => {
  backend.conflict = true;
  const { result } = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await result.current.submit(detail(), 'archive', {}, true);
  });
  await act(async () => {
    await result.current.abandon();
  });
  expect(result.current.pending).not.toBeNull();
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.canAbandon).toBe(true);
  await act(async () => {
    await result.current.abandon();
  });
  expect(result.current.pending).toBeNull();
  expect(localStorage.getItem(lifecycleStorageKey(Owner, Domain))).toBeNull();
  backend.conflict = false;
  await act(async () => {
    await result.current.submit(detail(), 'archive', {}, true);
  });
  expect(result.current.receipt).not.toBeNull();
  expect(backend.posts[0].request.idempotency_key).not.toBe(
    backend.posts[1].request.idempotency_key,
  );
});
it('never treats unknown404 or timeout as permission to abandon or generate a new command key', async () => {
  backend.losePost = true;
  backend.missingResults = true;
  const { result } = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await result.current.submit(detail(), 'archive', {}, true);
  });
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.canAbandon).toBe(false);
  const key = result.current.pending!.request.idempotency_key;
  await act(async () => {
    await result.current.submit(detail(), 'restore', {}, true);
  });
  expect(backend.posts).toHaveLength(1);
  expect(result.current.pending?.request.idempotency_key).toBe(key);
});
it('blocks new POSTs on corrupt slots, full eight-domain capacity and failed local storage', async () => {
  localStorage.setItem(lifecycleStorageKey(Owner, Domain), 'broken');
  const corrupt = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await corrupt.result.current.submit(detail(), 'archive', {}, true);
  });
  expect(backend.posts).toHaveLength(0);
  corrupt.unmount();
  localStorage.clear();
  const value = await recovery();
  for (let n = 1; n <= 8; n++)
    await saveLifecycleRecovery({
      ...value,
      domain_id: n.toString(16).padStart(32, '0'),
    });
  const full = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await full.result.current.submit(detail(), 'archive', {}, true);
  });
  expect(backend.posts).toHaveLength(0);
  full.unmount();
  localStorage.clear();
  const write = jest
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw Error('Quota');
    });
  const quota = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await quota.result.current.submit(detail(), 'archive', {}, true);
  });
  expect(backend.posts).toHaveLength(0);
  write.mockRestore();
});
it('retains the owner slot and clears visible results on lost access', async () => {
  backend.losePost = true;
  const fail = jest.fn(),
    { result } = renderHook(() => useLifecycleFlow(Scope, fail), {
      wrapper: wrapper(),
    });
  await act(async () => {
    await result.current.submit(detail(), 'archive', {}, true);
  });
  backend.denied = true;
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.blocked).toBe(true);
  expect(result.current.receipt).toBeNull();
  expect(fail).toHaveBeenCalled();
  expect(
    localStorage.getItem(lifecycleStorageKey(Owner, Domain)),
  ).not.toBeNull();
});
it('hides a late successful command response after the native session changes, retaining the original key', async () => {
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => {
      finish = resolve;
    }),
    original = Http.getMockImplementation()!;
  Http.mockImplementation(async (config) => {
    const value = await original(config);
    if (config.method === 'post') await gate;
    return value;
  });
  const failed = jest.fn(),
    { result } = renderHook(() => useLifecycleFlow(Scope, failed), {
      wrapper: wrapper(),
    });
  let submitting!: Promise<void>;
  act(() => {
    submitting = result.current.submit(detail(), 'archive', {}, true);
  });
  await waitFor(() => expect(backend.posts).toHaveLength(1));
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-replaced-session');
  await act(async () => {
    finish();
    await submitting;
  });
  expect(result.current.receipt).toBeNull();
  expect(result.current.blocked).toBe(true);
  expect(failed).toHaveBeenCalled();
  expect(readLifecycleRecovery(Owner, Domain)?.command_id).toBeNull();
});
it('serializes competing views in one domain and preserves the winning pending original', async () => {
  backend.losePost = true;
  const first = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
      wrapper: wrapper(),
    }),
    second = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
      wrapper: wrapper(),
    });
  await act(async () => {
    await Promise.all([
      first.result.current.submit(detail(), 'archive', {}, true),
      second.result.current.submit(detail(), 'archive', {}, true),
    ]);
  });
  expect(backend.posts).toHaveLength(1);
  expect(readLifecycleRecovery(Owner, Domain)?.request.idempotency_key).toBe(
    first.result.current.pending?.request.idempotency_key,
  );
});
it('a fixed archive receipt can be replayed after restoration without applying archive again', async () => {
  const { result } = renderHook(() => useLifecycleFlow(Scope, jest.fn()), {
    wrapper: wrapper(),
  });
  await act(async () => {
    await result.current.submit(detail(), 'archive', {}, true);
  });
  const saved = result.current.pending!;
  backend.current = {
    ...backend.current,
    current: {
      ...backend.current.current,
      state: 'active',
      lifecycle_version: 7,
    },
  };
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.receipt?.after.state).toBe('archived');
  expect(backend.current.current.state).toBe('active');
  expect(backend.posts).toHaveLength(1);
  expect(result.current.pending?.command_id).toBe(saved.command_id);
});
async function oldConfirmation(
  issue = new OriginalRequestError(409, 'revision_changed'),
) {
  const upload = await OriginalFixture.makeUpload(),
    registration = await OriginalFixture.makeRegistration(),
    key = originalStorageKey(OriginalFixture.DomainId, OriginalFixture.OwnerId),
    pending = {
      ...originalScope(OriginalFixture.DomainId, OriginalFixture.OwnerId),
      kind: 'upload' as const,
      registration_id: OriginalFixture.RegistrationId,
      request: OriginalFixture.createRequest(),
      request_sha256: upload.request_sha256,
      upload_id: upload.upload_id,
    };
  saveOriginalRecovery(key, 'create', pending);
  jest
    .spyOn(originalService, 'uploadByKey')
    .mockResolvedValue({ data: upload, httpStatus: 200 });
  const byRegistration = jest
      .spyOn(originalService, 'registration')
      .mockResolvedValue({ data: registration, httpStatus: 200 }),
    byDomain = jest.spyOn(foundationService, 'domain').mockResolvedValue({
      domain_id: OriginalFixture.DomainId,
      owner_id: OriginalFixture.OwnerId,
      name: 'synthetic old confirmation',
      category: 'custom',
      contract_version: '1.0.0',
      policy: {
        version: 1,
        access: 'owner_only',
        index_modes: ['catalogue'],
        allowed_model_uses: [],
        external_models: false,
        automatic_ingestion: false,
      },
    }),
    byId = jest
      .spyOn(originalService, 'upload')
      .mockResolvedValue({ data: upload, httpStatus: 200 }),
    post = jest.spyOn(originalService, 'confirm').mockRejectedValue(issue),
    byReceipt = jest
      .spyOn(originalService, 'receiptByKey')
      .mockRejectedValue(new OriginalRequestError(404, 'knowledge_not_found')),
    cancelled = await OriginalFixture.makeUpload('cancelled', { revision: 3 }),
    cancel = jest
      .spyOn(originalService, 'cancel')
      .mockResolvedValue({ data: cancelled, httpStatus: 200 });
  const hook = renderHook(
    () =>
      useOriginalFlow(OriginalFixture.DomainId, OriginalFixture.OwnerId, key),
    { wrapper: wrapper() },
  );
  await act(async () => {
    await hook.result.current.recover();
  });
  await act(async () => {
    await hook.result.current.confirm(true);
  });
  return {
    ...hook,
    key,
    upload,
    registration,
    post,
    byReceipt,
    byDomain,
    byRegistration,
    byId,
    cancel,
  };
}
it('exits a known old 0.5 rejection only after receipt and access checks, cancels the old quota, then fully uploads a fresh session', async () => {
  const flow = await oldConfirmation(),
    { result } = flow;
  expect(result.current.canReset).toBe(false);
  const original = result.current.confirmPending!.request;
  await act(async () => {
    await result.current.recover();
  });
  expect(flow.byReceipt).toHaveBeenCalledWith(result.current.confirmPending);
  expect(flow.byDomain).toHaveBeenCalledWith(OriginalFixture.DomainId);
  expect(flow.byRegistration).toHaveBeenLastCalledWith(
    OriginalFixture.DomainId,
    OriginalFixture.RegistrationId,
    OriginalFixture.OwnerId,
  );
  expect(flow.byId).toHaveBeenCalledWith(
    OriginalFixture.DomainId,
    OriginalFixture.RegistrationId,
    flow.upload.upload_id,
  );
  expect(result.current.blocked).toBe(false);
  expect(result.current.canReset).toBe(true);
  expect(result.current.unknown).toBe(true);
  expect(result.current.upload?.state).toBe('ready');
  expect(result.current.confirmPending!.request).toEqual(original);
  await act(async () => {
    await result.current.startAnother();
  });
  expect(flow.byReceipt).toHaveBeenCalledTimes(2);
  expect(flow.cancel).toHaveBeenCalledWith(
    OriginalFixture.DomainId,
    OriginalFixture.RegistrationId,
    flow.upload.upload_id,
  );
  expect(result.current.confirmPending).toBeNull();
  expect(result.current.createPending?.kind).toBe('registration');
  expect(result.current.blocked).toBe(false);
  expect(result.current.unknown).toBe(false);
  const nextId = '2'.repeat(32),
    target = jest.spyOn(originalService, 'target').mockResolvedValue({
      data: {
        domain_id: OriginalFixture.DomainId,
        registration_id: OriginalFixture.RegistrationId,
        relative_path: flow.upload.relative_path,
        source: null,
      },
      httpStatus: 200,
    }),
    create = jest
      .spyOn(originalService, 'create')
      .mockImplementation(async (_domain, _reg, _owner, input) => ({
        data: await OriginalFixture.makeUpload(
          'awaiting_bytes',
          { upload_id: nextId },
          input,
        ),
        httpStatus: 201,
      })),
    put = jest.spyOn(originalService, 'put').mockImplementation(async () => ({
      data: await OriginalFixture.makeUpload(
        'ready',
        { upload_id: nextId },
        create.mock.calls[0][3],
      ),
      httpStatus: 200,
    }));
  expect(create).not.toHaveBeenCalled();
  expect(put).not.toHaveBeenCalled();
  const file = OriginalFixture.file();
  await act(async () => {
    await result.current.prepare(
      file,
      flow.upload.relative_path,
      OriginalFixture.Classification,
    );
  });
  expect(result.current.error).toBeNull();
  expect(target).toHaveBeenCalledTimes(1);
  expect(create.mock.calls[0][3].idempotency_key).not.toBe(
    OriginalFixture.createRequest().idempotency_key,
  );
  expect(put).toHaveBeenCalledWith(
    OriginalFixture.DomainId,
    OriginalFixture.RegistrationId,
    nextId,
    file,
    expect.any(Function),
    expect.any(AbortSignal),
  );
  expect(result.current.upload?.upload_id).toBe(nextId);
  expect(flow.post).toHaveBeenCalledTimes(1);
});
it('keeps the authorized old ready upload visibly cancellable after recovery', async () => {
  const flow = await oldConfirmation();
  await act(async () => {
    await flow.result.current.recover();
  });
  expect(flow.result.current.blocked).toBe(false);
  expect(flow.result.current.upload?.state).toBe('ready');
  await act(async () => {
    await flow.result.current.cancel();
  });
  expect(flow.result.current.upload?.state).toBe('cancelled');
  expect(flow.result.current.confirmPending).not.toBeNull();
  expect(flow.cancel).toHaveBeenCalledTimes(1);
});
it.each([
  [0, 'original_result_unknown'],
  [503, 'original_result_unknown'],
  [401, 'knowledge_unauthorized'],
])(
  'does not replace an old confirmation after an unknown or unauthorized POST (%s)',
  async (status, code) => {
    const flow = await oldConfirmation(new OriginalRequestError(status, code)),
      original = flow.result.current.confirmPending!.request;
    await act(async () => {
      await flow.result.current.recover();
    });
    expect(flow.result.current.canReset).toBe(false);
    await act(async () => {
      await flow.result.current.startAnother();
    });
    expect(flow.result.current.confirmPending?.request).toEqual(original);
    expect(
      loadOriginalRecovery(
        flow.key,
        'confirm',
        OriginalFixture.DomainId,
        OriginalFixture.OwnerId,
      ),
    ).toMatchObject({ request: original });
    expect(flow.cancel).not.toHaveBeenCalled();
    expect(flow.post).toHaveBeenCalledTimes(1);
  },
);
it('loses rejection proof on refresh and requires original GET plus explicit identical resend before enabling the old transition exit', async () => {
  const flow = await oldConfirmation(),
    original = flow.result.current.confirmPending!.request;
  flow.unmount();
  const next = renderHook(
    () =>
      useOriginalFlow(
        OriginalFixture.DomainId,
        OriginalFixture.OwnerId,
        flow.key,
      ),
    { wrapper: wrapper() },
  );
  await act(async () => {
    await next.result.current.recover();
  });
  expect(next.result.current.canResend).toBe(true);
  expect(next.result.current.canReset).toBe(false);
  await act(async () => {
    await next.result.current.startAnother();
  });
  expect(next.result.current.confirmPending?.request).toEqual(original);
  expect(flow.cancel).not.toHaveBeenCalled();
  await act(async () => {
    await next.result.current.resend();
  });
  expect(flow.post).toHaveBeenLastCalledWith(
    OriginalFixture.DomainId,
    OriginalFixture.RegistrationId,
    original,
  );
  expect(next.result.current.canReset).toBe(false);
  await act(async () => {
    await next.result.current.recover();
  });
  expect(next.result.current.canReset).toBe(true);
  expect(next.result.current.confirmPending?.request).toEqual(original);
  expect(flow.post).toHaveBeenCalledTimes(2);
});
it.each(['domain', 'registration', 'upload'])(
  'isolates old recovery state when the current %s authorization is lost',
  async (scope) => {
    const flow = await oldConfirmation(),
      original = flow.result.current.confirmPending!.request;
    await act(async () => {
      await flow.result.current.recover();
    });
    expect(flow.result.current.canReset).toBe(true);
    if (scope === 'domain')
      flow.byDomain.mockRejectedValue(
        new KnowledgeRequestError(401, 'knowledge_unauthorized'),
      );
    else if (scope === 'registration')
      flow.byRegistration.mockRejectedValue(
        new OriginalRequestError(404, 'knowledge_not_found'),
      );
    else
      flow.byId.mockRejectedValue(
        new OriginalRequestError(404, 'knowledge_not_found'),
      );
    await act(async () => {
      await flow.result.current.startAnother();
    });
    expect(flow.result.current.blocked).toBe(true);
    expect(flow.result.current.canReset).toBe(false);
    expect(flow.result.current.canResend).toBe(false);
    expect(flow.result.current.upload).toBeNull();
    expect(flow.result.current.registration).toBeNull();
    expect(flow.result.current.receipt).toBeNull();
    expect(flow.result.current.confirmPending?.request).toEqual(original);
    expect(flow.cancel).not.toHaveBeenCalled();
  },
);
it.each(['recover', 'startAnother'] as const)(
  'preserves a successful old receipt found by %s without cancelling or preparing another upload',
  async (action) => {
    const flow = await oldConfirmation();
    if (action === 'startAnother')
      await act(async () => {
        await flow.result.current.recover();
      });
    const saved = OriginalFixture.makeReceipt(
      flow.upload,
      flow.result.current.confirmPending!.request,
    );
    flow.byReceipt.mockResolvedValue({ data: saved, httpStatus: 200 });
    await act(async () => {
      await flow.result.current[action]();
    });
    expect(flow.result.current.receipt).toEqual(saved);
    expect(flow.result.current.confirmPending?.import_task_id).toBe(
      saved.import_task_id,
    );
    expect(flow.result.current.canReset).toBe(false);
    expect(flow.cancel).not.toHaveBeenCalled();
    expect(flow.post).toHaveBeenCalledTimes(1);
  },
);
it('preserves an old commit that wins cancellation instead of clearing the original slot', async () => {
  const flow = await oldConfirmation();
  await act(async () => {
    await flow.result.current.recover();
  });
  const committed = await OriginalFixture.makeUpload('committed', {
      revision: 3,
      created_at: flow.upload.created_at,
      expires_at: flow.upload.expires_at,
    }),
    saved = OriginalFixture.makeReceipt(
      flow.upload,
      flow.result.current.confirmPending!.request,
    );
  flow.cancel.mockResolvedValue({ data: committed, httpStatus: 200 });
  const byId = jest
    .spyOn(originalService, 'receipt')
    .mockResolvedValue({ data: saved, httpStatus: 200 });
  await act(async () => {
    await flow.result.current.startAnother();
  });
  expect(flow.result.current.error).toBeNull();
  expect(byId).toHaveBeenCalledWith(
    OriginalFixture.DomainId,
    OriginalFixture.RegistrationId,
    saved.import_task_id,
  );
  expect(flow.result.current.receipt).toEqual(saved);
  expect(flow.result.current.createPending?.kind).toBe('upload');
  expect(flow.result.current.confirmPending?.import_task_id).toBe(
    saved.import_task_id,
  );
});
it('retains the old confirmation and upload slot when cancellation loses its response', async () => {
  const flow = await oldConfirmation(),
    original = flow.result.current.confirmPending!.request;
  await act(async () => {
    await flow.result.current.recover();
  });
  flow.cancel.mockRejectedValue(
    new OriginalRequestError(503, 'original_result_unknown'),
  );
  await act(async () => {
    await flow.result.current.startAnother();
  });
  expect(flow.result.current.confirmPending?.request).toEqual(original);
  expect(flow.result.current.createPending?.kind).toBe('upload');
  expect(flow.result.current.canReset).toBe(false);
  expect(flow.result.current.unknown).toBe(true);
  expect(flow.post).toHaveBeenCalledTimes(1);
});

import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service, {
  OriginalRequestError,
} from '@/services/knowledge-original-service';
import foundation, {
  KnowledgeRequestError,
} from '@/services/knowledge-foundation-service';
import OriginalImport from '../original';
import { useOriginalFlow } from '../use-original-flow';
import { KnowledgeOriginalKeys } from '@/hooks/use-knowledge-original';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from '@/hooks/use-knowledge-foundation';
import {
  originalScope,
  originalStorageKey,
  saveOriginalRecovery,
  loadOriginalRecovery,
  originalRegistrationRequest,
} from '@/utils/knowledge-original';
import {
  DomainId,
  OwnerId,
  RegistrationId,
  Capabilities,
  makeRegistration,
  makeUpload,
  makeReceipt,
  createRequest,
  confirmRequest,
  file,
} from './original-fixture';

type Domain =
  import('@/interfaces/database/knowledge-foundation').KnowledgeDomain;
type Registration =
  import('@/interfaces/database/knowledge-original').OriginalRegistration;
type Receipt =
  import('@/interfaces/database/knowledge-original').OriginalReceipt;
type Upload = import('@/interfaces/database/knowledge-original').OriginalUpload;
type Response<T> =
  import('@/services/knowledge-original-service').OriginalResponse<T>;
type Progress = import('axios').AxiosProgressEvent;

let mockAuthorization = 'synthetic-original-new-registration';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(() => {
    throw Error('Unstubbed HTTP is forbidden');
  }),
}));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { getUserInfoObject: () => ({ name: 'Synthetic owner' }) },
  getAuthorization: () => mockAuthorization,
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/ui/button', () => ({
  Button: ({ variant: _variant, ...props }: Record<string, unknown>) => {
    void _variant;
    return <button {...props} />;
  },
}));
jest.mock('@/components/file-uploader', () => ({
  FileUploader: () => (
    <input aria-label="synthetic-original-file" type="file" />
  ),
}));

const NativeDomain: Domain = {
  domain_id: DomainId,
  owner_id: OwnerId,
  name: 'Synthetic original domain',
  category: 'project',
  contract_version: '1.0.0',
  policy: {
    version: 1,
    access: 'owner_only',
    index_modes: ['catalogue'],
    allowed_model_uses: [],
    external_models: false,
    automatic_ingestion: false,
  },
};
const StorageKey = originalStorageKey(DomainId, OwnerId);
const NewRegistrationId = '2'.repeat(32);
const FreshPath = 'synthetic/fresh-docx.docx';
const ok = <T,>(data: T, httpStatus = 200): Response<T> => ({
  data,
  httpStatus,
});
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

let registration: Registration, upload: Upload, receipt: Receipt;
let api: {
  registration: jest.SpyInstance;
  registrationByKey: jest.SpyInstance;
  receiptByKey: jest.SpyInstance;
  register: jest.SpyInstance;
  create: jest.SpyInstance;
  put: jest.SpyInstance;
  cancel: jest.SpyInstance;
};
let lockNames: string[];
let lockDepth: number;
function installLocks() {
  const tails = new Map<string, Promise<unknown>>();
  lockNames = [];
  lockDepth = 0;
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: (name: string, callback: () => Promise<unknown>) => {
        lockNames.push(name);
        const previous = tails.get(name) || Promise.resolve();
        const next = previous
          .catch(() => {})
          .then(async () => {
            lockDepth++;
            try {
              return await callback();
            } finally {
              lockDepth--;
            }
          });
        tails.set(name, next);
        return next;
      },
    },
  });
}
function wrapper(
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  }),
) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}
function mountFlow() {
  return renderHook(() => useOriginalFlow(DomainId, OwnerId, StorageKey), {
    wrapper: wrapper(),
  });
}
async function seedCompleted(withConfirmation = true) {
  const scope = originalScope(DomainId, OwnerId);
  saveOriginalRecovery(StorageKey, 'create', {
    ...scope,
    kind: 'upload',
    registration_id: RegistrationId,
    request: createRequest(),
    request_sha256: upload.request_sha256,
    upload_id: upload.upload_id,
  });
  if (withConfirmation)
    saveOriginalRecovery(StorageKey, 'confirm', {
      ...scope,
      registration_id: RegistrationId,
      request: confirmRequest(upload),
      import_task_id: receipt.import_task_id,
    });
}
function recoverySnapshot() {
  return {
    create: localStorage.getItem(StorageKey + ':create'),
    confirm: localStorage.getItem(StorageKey + ':confirm'),
  };
}
async function recoveredFlow(withConfirmation = true) {
  await seedCompleted(withConfirmation);
  const view = mountFlow();
  await act(async () => {
    await view.result.current.recover();
  });
  expect(view.result.current.receipt?.source).toEqual(receipt.source);
  return view;
}
async function enterFreshForm() {
  await seedCompleted();
  render(<OriginalImport domain={DomainId} />, { wrapper: wrapper() });
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeOriginal.checkResult',
    }),
  );
  const saved = await screen.findByRole('region', {
    name: 'knowledgeOriginal.receipt',
  });
  expect(
    within(saved).getByRole('button', {
      name: 'knowledgeOriginal.continueRegistration',
    }),
  ).toBeInTheDocument();
  fireEvent.click(
    within(saved).getByRole('button', {
      name: 'knowledgeOriginal.startNewRegistration',
    }),
  );
  return screen.findByRole('form', { name: 'knowledgeOriginal.registration' });
}

beforeEach(async () => {
  jest.restoreAllMocks();
  localStorage.clear();
  mockAuthorization = 'synthetic-original-new-registration';
  installLocks();
  registration = await makeRegistration();
  upload = await makeUpload('committed');
  receipt = makeReceipt(upload);
  jest.spyOn(service, 'contract').mockResolvedValue(ok(Capabilities));
  jest.spyOn(foundation, 'domain').mockResolvedValue(NativeDomain);
  jest
    .spyOn(foundation, 'groups')
    .mockResolvedValue({ items: [], next_cursor: '' });
  api = {
    registration: jest
      .spyOn(service, 'registration')
      .mockResolvedValue(ok(registration)),
    registrationByKey: jest
      .spyOn(service, 'registrationByKey')
      .mockImplementation(async () => ok(registration)),
    receiptByKey: jest
      .spyOn(service, 'receiptByKey')
      .mockResolvedValue(ok(receipt)),
    register: jest
      .spyOn(service, 'register')
      .mockImplementation(async (_domain, _owner, input) => {
        registration = await makeRegistration({
          ...input,
          registration_id: NewRegistrationId,
        });
        return ok(registration, 201);
      }),
    create: jest
      .spyOn(service, 'create')
      .mockRejectedValue(Error('No unexpected upload POST')),
    put: jest
      .spyOn(service, 'put')
      .mockRejectedValue(Error('No unexpected byte PUT')),
    cancel: jest
      .spyOn(service, 'cancel')
      .mockRejectedValue(Error('No cancellation expected')),
  };
  jest.spyOn(service, 'uploadByKey').mockResolvedValue(ok(upload));
  jest
    .spyOn(service, 'target')
    .mockImplementation(async (domain, reg, relative) =>
      ok({
        domain_id: domain,
        registration_id: reg,
        relative_path: relative,
        source: null,
      }),
    );
  jest.spyOn(service, 'upload').mockResolvedValue(ok(upload));
  jest.spyOn(service, 'receipt').mockResolvedValue(ok(receipt));
});
afterEach(() => {
  jest.restoreAllMocks();
});

it('offers separate new-registration and old-registration actions after a GET-recovered receipt; creates a fresh key and path without changing old fixed references', async () => {
  const fixed = JSON.parse(JSON.stringify(receipt));
  localStorage.setItem(
    'synthetic-fixed-publication',
    JSON.stringify(fixed.source),
  );
  const form = await enterFreshForm();
  const key = within(form).getByLabelText(
    'knowledgeOriginal.registrationKey',
  ) as HTMLInputElement;
  expect(key.value).not.toBe(registration.registration_key);
  expect(key.value).not.toBe('');
  expect(
    screen.queryByRole('region', { name: 'knowledgeOriginal.receipt' }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByLabelText('knowledgeOriginal.registrationId'),
  ).not.toBeInTheDocument();
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.name'), {
    target: { value: 'Fresh synthetic DOCX registration' },
  });
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.paths'), {
    target: { value: FreshPath },
  });
  const submittedKey = key.value;
  fireEvent.submit(form);
  await screen.findByRole('form', { name: 'knowledgeOriginal.prepare' });
  expect(api.register).toHaveBeenCalledTimes(1);
  expect(api.register.mock.calls[0]).toEqual([
    DomainId,
    OwnerId,
    expect.objectContaining({
      registration_key: submittedKey,
      include_paths: [FreshPath],
    }),
  ]);
  expect(
    loadOriginalRecovery(StorageKey, 'create', DomainId, OwnerId)
      ?.registration_id,
  ).toBe(NewRegistrationId);
  expect(receipt).toEqual(fixed);
  expect(localStorage.getItem('synthetic-fixed-publication')).toBe(
    JSON.stringify(fixed.source),
  );
  expect(api.cancel).not.toHaveBeenCalled();
});

it('keeps the existing continue action bound to the old registration and its registered paths', async () => {
  const view = await recoveredFlow();
  await act(async () => {
    await view.result.current.startAnother();
  });
  expect(view.result.current.registration?.registration_id).toBe(
    RegistrationId,
  );
  expect(view.result.current.registration?.include_paths).toEqual(
    registration.include_paths,
  );
  expect(view.result.current.receipt).toBeNull();
  expect(api.register).not.toHaveBeenCalled();
});

it('makes a repeated explicit new action a local no-op and removes only this completed owner/domain operation', async () => {
  const view = await recoveredFlow();
  const otherKey = originalStorageKey('3'.repeat(32), OwnerId);
  localStorage.setItem(otherKey + ':create', 'other-domain-sentinel');
  const start = view.result.current.startNewRegistration;
  await act(async () => {
    await Promise.all([start(), start()]);
  });
  expect(recoverySnapshot()).toEqual({ create: null, confirm: null });
  expect(localStorage.getItem(otherKey + ':create')).toBe(
    'other-domain-sentinel',
  );
  expect(view.result.current.registration).toBeNull();
  expect(view.result.current.receipt).toBeNull();
  expect(api.register).not.toHaveBeenCalled();
  expect(api.cancel).not.toHaveBeenCalled();
});

it('restores a committed upload without a confirm slot and still starts a fresh registration safely', async () => {
  const view = await recoveredFlow(false);
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual({ create: null, confirm: null });
  expect(view.result.current.receipt).toBeNull();
  expect(api.register).not.toHaveBeenCalled();
});

it('does not discard an unknown confirmation or issue a replacement POST after an unconfirmed GET result', async () => {
  await seedCompleted();
  api.receiptByKey.mockRejectedValue(
    new OriginalRequestError(404, 'knowledge_not_found'),
  );
  jest
    .spyOn(service, 'upload')
    .mockResolvedValue(ok(await makeUpload('ready')));
  const before = recoverySnapshot();
  const view = mountFlow();
  await act(async () => {
    await view.result.current.recover();
    await view.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(view.result.current.receipt).toBeNull();
  expect(api.register).not.toHaveBeenCalled();
  expect(api.cancel).not.toHaveBeenCalled();
});

it('refuses new/reset and a second lookup while the original result lookup is busy', async () => {
  await seedCompleted();
  const pending = deferred<Response<Receipt>>();
  api.receiptByKey.mockReturnValue(pending.promise);
  const before = recoverySnapshot(),
    view = mountFlow();
  let work!: Promise<unknown>;
  act(() => {
    work = view.result.current.recover();
  });
  await waitFor(() => expect(api.receiptByKey).toHaveBeenCalledTimes(1));
  await act(async () => {
    await view.result.current.startNewRegistration();
    await view.result.current.recover();
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(api.receiptByKey).toHaveBeenCalledTimes(1);
  await act(async () => {
    pending.resolve(ok(receipt));
    await work;
  });
  expect(view.result.current.receipt?.import_task_id).toBe(
    receipt.import_task_id,
  );
});

it('ignores old upload/query-error callbacks and old action closures after a fresh registration begins', async () => {
  const view = await recoveredFlow();
  const staleUpload = view.result.current.rememberUpload,
    staleFailure = view.result.current.fail,
    staleRecover = view.result.current.recover;
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  const reads = api.receiptByKey.mock.calls.length;
  await act(async () => {
    staleUpload(upload);
    staleFailure(new OriginalRequestError(401, 'knowledge_unauthorized'));
    await staleRecover();
  });
  expect(view.result.current.receipt).toBeNull();
  expect(view.result.current.upload).toBeNull();
  expect(view.result.current.registration).toBeNull();
  expect(view.result.current.error).toBeNull();
  expect(recoverySnapshot()).toEqual({ create: null, confirm: null });
  expect(api.receiptByKey).toHaveBeenCalledTimes(reads);
});

it.each([1, 2])(
  'keeps the confirmed receipt and refuses new POST when clearing recovery slot %i fails, then permits an explicit retry',
  async (failureAt) => {
    const view = await recoveredFlow();
    const remove = Storage.prototype.removeItem;
    let count = 0;
    const broken = jest
      .spyOn(Storage.prototype, 'removeItem')
      .mockImplementation(function (this: Storage, key: string) {
        if (key.startsWith(StorageKey) && ++count === failureAt)
          throw new DOMException('Synthetic storage refusal', 'SecurityError');
        return remove.call(this, key);
      });
    await act(async () => {
      await view.result.current.startNewRegistration();
    });
    expect(view.result.current.receipt?.import_task_id).toBe(
      receipt.import_task_id,
    );
    expect(view.result.current.error).toBeTruthy();
    expect(api.register).not.toHaveBeenCalled();
    broken.mockRestore();
    await act(async () => {
      await view.result.current.startNewRegistration();
    });
    expect(view.result.current.receipt).toBeNull();
    expect(recoverySnapshot()).toEqual({ create: null, confirm: null });
  },
);

it('retains the new form key and sends no POST on storage write failure; retry uses that same key', async () => {
  const form = await enterFreshForm();
  const key = (
    within(form).getByLabelText(
      'knowledgeOriginal.registrationKey',
    ) as HTMLInputElement
  ).value;
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.name'), {
    target: { value: 'Fresh synthetic' },
  });
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.paths'), {
    target: { value: FreshPath },
  });
  const set = Storage.prototype.setItem;
  const broken = jest
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(function (this: Storage, name: string, value: string) {
      if (name === StorageKey + ':create')
        throw new DOMException('Synthetic quota', 'QuotaExceededError');
      return set.call(this, name, value);
    });
  fireEvent.submit(form);
  await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
  expect(api.register).not.toHaveBeenCalled();
  expect(
    (
      within(form).getByLabelText(
        'knowledgeOriginal.registrationKey',
      ) as HTMLInputElement
    ).value,
  ).toBe(key);
  broken.mockRestore();
  fireEvent.submit(form);
  await screen.findByRole('form', { name: 'knowledgeOriginal.prepare' });
  expect(api.register.mock.calls[0][2].registration_key).toBe(key);
});

it('recovers a lost new registration response by its exact key across remount without resending POST', async () => {
  const view = await recoveredFlow();
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  const input = {
    registration_key: 'fresh:lost-result',
    name: 'Fresh synthetic',
    source_kind: 'original_document' as const,
    input_mode: 'original_upload' as const,
    include_paths: [FreshPath],
    default_classification: registration.default_classification,
  };
  api.register.mockImplementationOnce(async () => {
    registration = await makeRegistration({
      ...input,
      registration_id: NewRegistrationId,
    });
    throw Error('Synthetic response lost');
  });
  await act(async () => {
    await view.result.current.register(input);
  });
  const stored = loadOriginalRecovery(StorageKey, 'create', DomainId, OwnerId);
  expect(stored?.kind).toBe('registration');
  await act(async () => {
    await view.result.current.register(input);
    await view.result.current.startNewRegistration();
  });
  expect(api.register).toHaveBeenCalledTimes(1);
  view.unmount();
  const resumed = mountFlow();
  await act(async () => {
    await resumed.result.current.recover();
  });
  expect(api.registrationByKey.mock.calls.at(-1)).toEqual([
    DomainId,
    OwnerId,
    input.registration_key,
    registration.config_sha256,
  ]);
  expect(resumed.result.current.registration?.registration_id).toBe(
    NewRegistrationId,
  );
  expect(api.register).toHaveBeenCalledTimes(1);
});

it.each(['scope', 'scope ABA', 'session', 'unmount'])(
  'does not adopt a late old result after %s changes',
  async (change) => {
    await seedCompleted();
    const pending = deferred<Response<Receipt>>();
    api.receiptByKey.mockReturnValue(pending.promise);
    const view = renderHook(
      ({ domain, owner }) =>
        useOriginalFlow(domain, owner, originalStorageKey(domain, owner)),
      {
        initialProps: { domain: DomainId, owner: OwnerId },
        wrapper: wrapper(),
      },
    );
    let work!: Promise<unknown>;
    act(() => {
      work = view.result.current.recover();
    });
    await waitFor(() => expect(api.receiptByKey).toHaveBeenCalledTimes(1));
    const before = recoverySnapshot();
    if (change === 'scope' || change === 'scope ABA')
      view.rerender({ domain: '4'.repeat(32), owner: '5'.repeat(32) });
    if (change === 'scope ABA')
      view.rerender({ domain: DomainId, owner: OwnerId });
    if (change === 'session') {
      mockAuthorization = 'synthetic-other-session';
      view.rerender({ domain: DomainId, owner: OwnerId });
    }
    if (change === 'unmount') view.unmount();
    await act(async () => {
      pending.resolve(ok(receipt));
      await work;
    });
    expect(recoverySnapshot()).toEqual(before);
    if (change !== 'unmount') expect(view.result.current.receipt).toBeNull();
    expect(api.register).not.toHaveBeenCalled();
  },
);

it('rejects an external tab slot replacement and late result without overwriting that replacement', async () => {
  await seedCompleted();
  const pending = deferred<Response<Receipt>>();
  api.receiptByKey.mockReturnValue(pending.promise);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const fixedKey = KnowledgeOriginalKeys.revision(
    getKnowledgeSessionScope(),
    DomainId,
    receipt.source.source_revision_id,
  );
  client.setQueryData(fixedKey, receipt.source);
  const view = renderHook(
    () => useOriginalFlow(DomainId, OwnerId, StorageKey),
    { wrapper: wrapper(client) },
  );
  let work!: Promise<unknown>;
  act(() => {
    work = view.result.current.recover();
  });
  await waitFor(() => expect(api.receiptByKey).toHaveBeenCalledTimes(1));
  const replacement = {
    ...originalScope(DomainId, OwnerId),
    kind: 'registration' as const,
    registration_key: 'other:tab:new',
    config_sha256: '6'.repeat(64),
    registration_id: null,
  };
  act(() => {
    saveOriginalRecovery(StorageKey, 'create', replacement);
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: StorageKey + ':create',
        storageArea: localStorage,
      }),
    );
  });
  const changed = recoverySnapshot();
  await act(async () => {
    pending.resolve(ok(receipt));
    await work;
    await view.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual(changed);
  expect(view.result.current.receipt).toBeNull();
  expect(api.register).not.toHaveBeenCalled();
  expect(client.getQueryData(fixedKey)).toEqual(receipt.source);
});

it('serializes two same-scope views and refuses the stale reset rather than deleting a new unresolved registration', async () => {
  const first = await recoveredFlow();
  const second = mountFlow();
  await act(async () => {
    await second.result.current.recover();
  });
  await act(async () => {
    await first.result.current.startNewRegistration();
  });
  const input = {
    registration_key: 'new:held',
    name: 'New held registration',
    source_kind: 'original_document' as const,
    input_mode: 'original_upload' as const,
    include_paths: [FreshPath],
    default_classification: registration.default_classification,
  };
  const pending = deferred<Response<Registration>>();
  api.register.mockReturnValueOnce(pending.promise);
  let work!: Promise<unknown>;
  act(() => {
    work = first.result.current.register(input);
  });
  await waitFor(() => expect(api.register).toHaveBeenCalledTimes(1));
  const saved = recoverySnapshot();
  let reset!: Promise<unknown>;
  act(() => {
    reset = second.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual(saved);
  await act(async () => {
    pending.reject(Error('Synthetic response unknown'));
    await work;
    await reset;
  });
  expect(recoverySnapshot()).toEqual(saved);
  expect(api.register).toHaveBeenCalledTimes(1);
  expect(second.result.current.receipt).toBeNull();
  expect(lockNames.length).toBeGreaterThan(1);
});

it('fails closed before a new POST or recovery clear when scoped browser locks are unavailable', async () => {
  const view = await recoveredFlow();
  const before = recoverySnapshot();
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: undefined,
  });
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(view.result.current.receipt?.import_task_id).toBe(
    receipt.import_task_id,
  );
  expect(view.result.current.error).toBeTruthy();
  expect(api.register).not.toHaveBeenCalled();
});

it('does not start fresh while a new upload is still unresolved', async () => {
  const view = mountFlow();
  await act(async () => {
    await view.result.current.openRegistration(RegistrationId);
  });
  const pending = deferred<Response<Upload>>();
  jest.spyOn(service, 'create').mockReturnValue(pending.promise);
  let work!: Promise<unknown>;
  act(() => {
    work = view.result.current.prepare(
      file(),
      registration.include_paths[0],
      registration.default_classification,
    );
  });
  await waitFor(() => expect(service.create).toHaveBeenCalledTimes(1));
  const before = recoverySnapshot();
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual(before);
  await act(async () => {
    pending.reject(Error('Synthetic upload result unknown'));
    await work;
  });
  expect(view.result.current.receipt).toBeNull();
  expect(api.register).not.toHaveBeenCalled();
});

it('keeps saved fixed-original reading independent of browser recovery locks', async () => {
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: undefined,
  });
  const revision = jest
    .spyOn(service, 'revision')
    .mockResolvedValue(ok(receipt.source));
  const download = jest
    .spyOn(service, 'download')
    .mockResolvedValue(new Blob(['synthetic-only']));
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: jest.fn(() => 'blob:synthetic-original'),
  });
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: jest.fn(),
  });
  jest.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const query = new URLSearchParams({
    mode: 'original',
    original_revision: receipt.source.source_revision_id,
    original_source: receipt.source.source_id,
    original_sha: receipt.source.sha256,
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[
          '/dataset/knowledge-import/' + DomainId + '?' + query.toString(),
        ]}
      >
        <OriginalImport domain={DomainId} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.fixedDetails' });
  expect(revision).toHaveBeenCalledTimes(1);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.download' }),
  );
  await waitFor(() => expect(download).toHaveBeenCalledTimes(1));
  expect(api.receiptByKey).not.toHaveBeenCalled();
  expect(api.register).not.toHaveBeenCalled();
  expect(lockNames).toHaveLength(0);
});

it('does not erase a different unresolved upload beside a recovered old confirm receipt, through either action', async () => {
  await seedCompleted();
  const pending = await makeUpload('awaiting_bytes', {
    upload_id: '7'.repeat(32),
  });
  saveOriginalRecovery(StorageKey, 'create', {
    ...originalScope(DomainId, OwnerId),
    kind: 'upload',
    registration_id: RegistrationId,
    request: createRequest(),
    request_sha256: pending.request_sha256,
    upload_id: pending.upload_id,
  });
  const view = mountFlow();
  await act(async () => {
    await view.result.current.recover();
  });
  const before = recoverySnapshot();
  await act(async () => {
    await view.result.current.startNewRegistration();
    await view.result.current.startAnother();
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(view.result.current.receipt?.import_task_id).toBe(
    receipt.import_task_id,
  );
  expect(api.cancel).not.toHaveBeenCalled();
  expect(api.register).not.toHaveBeenCalled();
});

it('ignores the old transfer progress and cancel callback after a completed upload starts a new collection', async () => {
  const view = mountFlow();
  await act(async () => {
    await view.result.current.openRegistration(RegistrationId);
  });
  let ready!: Upload;
  let transferProgress!: (event: Progress) => void;
  jest
    .spyOn(service, 'create')
    .mockImplementation(async (_domain, _reg, _owner, input) =>
      ok(await makeUpload('awaiting_bytes', {}, input), 201),
    );
  jest
    .spyOn(service, 'put')
    .mockImplementation(async (_domain, _reg, _upload, _file, progress) => {
      transferProgress = progress;
      const input = loadOriginalRecovery(
        StorageKey,
        'create',
        DomainId,
        OwnerId,
      );
      if (input?.kind !== 'upload') throw Error('Missing real upload recovery');
      ready = await makeUpload('ready', {}, input.request);
      return ok(ready);
    });
  jest
    .spyOn(service, 'confirm')
    .mockImplementation(async (_domain, _reg, input) =>
      ok(makeReceipt(ready, input), 201),
    );
  await act(async () => {
    await view.result.current.prepare(
      file(),
      registration.include_paths[0],
      registration.default_classification,
    );
  });
  const oldCancel = view.result.current.cancel;
  await act(async () => {
    await view.result.current.confirm(true);
  });
  expect(view.result.current.receipt).toBeTruthy();
  await act(async () => {
    await view.result.current.startNewRegistration();
    transferProgress({ loaded: 6, bytes: 6, lengthComputable: false });
    await oldCancel();
  });
  expect(view.result.current.progress).toBe(0);
  expect(view.result.current.upload).toBeNull();
  expect(view.result.current.receipt).toBeNull();
  expect(api.cancel).not.toHaveBeenCalled();
});

it('keeps the read-only original list independent of recovery locks', async () => {
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: undefined,
  });
  const list = jest
    .spyOn(service, 'list')
    .mockResolvedValue(
      ok({ domain_id: DomainId, items: [receipt.source], next_cursor: null }),
    );
  render(<OriginalImport domain={DomainId} />, { wrapper: wrapper() });
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeOriginal.originals' }),
  );
  await waitFor(() => expect(list).toHaveBeenCalledTimes(1));
  expect(
    await screen.findByRole('link', {
      name: new RegExp(receipt.source.original_filename),
    }),
  ).toBeInTheDocument();
  expect(api.register).not.toHaveBeenCalled();
  expect(lockNames).toHaveLength(0);
});

it('does not create an empty fresh form when reading recovery storage fails, and permits a normal reread', async () => {
  await seedCompleted();
  const get = Storage.prototype.getItem;
  const broken = jest
    .spyOn(Storage.prototype, 'getItem')
    .mockImplementation(function (this: Storage, key: string) {
      if (key.startsWith(StorageKey))
        throw new DOMException(
          'Synthetic storage read refusal',
          'SecurityError',
        );
      return get.call(this, key);
    });
  render(<OriginalImport domain={DomainId} />, { wrapper: wrapper() });
  expect(
    await screen.findByText('knowledgeOriginal.corruptRecovery'),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole('form', { name: 'knowledgeOriginal.registration' }),
  ).not.toBeInTheDocument();
  broken.mockRestore();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.reloadRecovery' }),
  );
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeOriginal.checkResult',
    }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.receipt' });
  expect(api.register).not.toHaveBeenCalled();
});

it('retains completed progress when the browser exposes locks but rejects the acquisition', async () => {
  const view = await recoveredFlow();
  const before = recoverySnapshot();
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: jest
        .fn()
        .mockRejectedValue(
          new DOMException('Synthetic browser policy refusal', 'SecurityError'),
        ),
    },
  });
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(view.result.current.receipt?.import_task_id).toBe(
    receipt.import_task_id,
  );
  expect(view.result.current.lockingUnavailable).toBe(true);
  expect(api.register).not.toHaveBeenCalled();
});

it('keeps an unmounted new POST recoverable by its original key when its response arrives late', async () => {
  const view = await recoveredFlow();
  await act(async () => {
    await view.result.current.startNewRegistration();
  });
  const input = {
    registration_key: 'fresh:late-response',
    name: 'Fresh late synthetic',
    source_kind: 'original_document' as const,
    input_mode: 'original_upload' as const,
    include_paths: [FreshPath],
    default_classification: registration.default_classification,
  };
  const pending = deferred<Response<Registration>>();
  api.register.mockReturnValueOnce(pending.promise);
  let work!: Promise<unknown>;
  act(() => {
    work = view.result.current.register(input);
  });
  await waitFor(() => expect(api.register).toHaveBeenCalledTimes(1));
  const before = recoverySnapshot();
  view.unmount();
  registration = await makeRegistration({
    ...input,
    registration_id: NewRegistrationId,
  });
  await act(async () => {
    pending.resolve(ok(registration, 201));
    await work;
  });
  expect(recoverySnapshot()).toEqual(before);
  const resumed = mountFlow();
  await act(async () => {
    await resumed.result.current.recover();
  });
  expect(resumed.result.current.registration?.registration_id).toBe(
    NewRegistrationId,
  );
  expect(api.registrationByKey.mock.calls.at(-1)?.[2]).toBe(
    input.registration_key,
  );
  expect(api.register).toHaveBeenCalledTimes(1);
});

it('preserves an in-flight confirmation and its idempotency key while fresh registration is refused', async () => {
  const view = mountFlow();
  await act(async () => {
    await view.result.current.openRegistration(RegistrationId);
  });
  let ready!: Upload;
  jest
    .spyOn(service, 'create')
    .mockImplementation(async (_domain, _reg, _owner, input) =>
      ok(await makeUpload('awaiting_bytes', {}, input), 201),
    );
  jest.spyOn(service, 'put').mockImplementation(async () => {
    const pending = loadOriginalRecovery(
      StorageKey,
      'create',
      DomainId,
      OwnerId,
    );
    if (pending?.kind !== 'upload') throw Error('Missing real upload recovery');
    ready = await makeUpload('ready', {}, pending.request);
    return ok(ready);
  });
  await act(async () => {
    await view.result.current.prepare(
      file(),
      registration.include_paths[0],
      registration.default_classification,
    );
  });
  const pending = deferred<Response<Receipt>>();
  const confirm = jest
    .spyOn(service, 'confirm')
    .mockReturnValue(pending.promise);
  let work!: Promise<unknown>;
  act(() => {
    work = view.result.current.confirm(true);
  });
  await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
  const before = recoverySnapshot();
  await act(async () => {
    await view.result.current.startNewRegistration();
    await view.result.current.confirm(true);
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(confirm).toHaveBeenCalledTimes(1);
  await act(async () => {
    pending.reject(Error('Synthetic result unknown'));
    await work;
  });
  expect(recoverySnapshot()).toEqual(before);
  expect(view.result.current.receipt).toBeNull();
  const input = confirm.mock.calls[0][2];
  api.receiptByKey.mockResolvedValue(ok(makeReceipt(ready, input)));
  await act(async () => {
    await view.result.current.recover();
  });
  expect(view.result.current.receipt?.idempotency_key).toBe(
    input.idempotency_key,
  );
  expect(confirm).toHaveBeenCalledTimes(1);
});

type MissingKind = 'registration' | 'upload';
async function seedMissingResult(kind: MissingKind) {
  const pending =
    kind === 'registration'
      ? {
          ...originalScope(DomainId, OwnerId),
          kind: 'registration' as const,
          registration_key: registration.registration_key,
          config_sha256: registration.config_sha256,
          registration_id: null,
        }
      : {
          ...originalScope(DomainId, OwnerId),
          kind: 'upload' as const,
          registration_id: RegistrationId,
          request: createRequest(),
          request_sha256: (await makeUpload('awaiting_bytes')).request_sha256,
          upload_id: null,
        };
  saveOriginalRecovery(StorageKey, 'create', pending);
  const lookup =
    kind === 'registration'
      ? api.registrationByKey
      : jest.mocked(service.uploadByKey);
  lookup.mockRejectedValue(
    new OriginalRequestError(404, 'knowledge_not_found'),
  );
  return pending;
}
function mountWithCachedDomain() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(
    KnowledgeKeys.domain(getKnowledgeSessionScope(), DomainId),
    NativeDomain,
  );
  return renderHook(() => useOriginalFlow(DomainId, OwnerId, StorageKey), {
    wrapper: wrapper(client),
  });
}

it.each(['registration', 'upload'] as const)(
  'keeps a missing %s by-key result unknown and permits only an explicit exact-request retry after fresh scope reads under the lock',
  async (kind) => {
    const pending = await seedMissingResult(kind);
    const before = recoverySnapshot();
    const input = originalRegistrationRequest(registration);
    const currentDomain = jest.mocked(foundation.domain);
    currentDomain.mockImplementation(async (id) => {
      expect(id).toBe(DomainId);
      expect(lockDepth).toBe(1);
      expect(lockNames.at(-1)).toBe(StorageKey + ':write');
      return NativeDomain;
    });
    const view = mountWithCachedDomain();
    await act(async () => {
      await view.result.current.recover();
    });
    expect(view.result.current.createPending).toEqual(pending);
    expect(view.result.current.canResend).toBe(true);
    expect(view.result.current.unknown).toBe(true);
    expect(view.result.current.blocked).toBe(false);
    expect(view.result.current.recoveryChanged).toBe(false);
    expect(currentDomain).toHaveBeenCalledTimes(1);
    expect(recoverySnapshot()).toEqual(before);
    expect(api.register).not.toHaveBeenCalled();
    expect(api.create).not.toHaveBeenCalled();
    expect(api.put).not.toHaveBeenCalled();
    if (pending.kind === 'registration') {
      await act(async () => {
        await view.result.current.register(input);
      });
      expect(api.register.mock.calls).toEqual([[DomainId, OwnerId, input]]);
      expect(api.create).not.toHaveBeenCalled();
      const saved = loadOriginalRecovery(
        StorageKey,
        'create',
        DomainId,
        OwnerId,
      );
      expect(saved).toMatchObject({
        registration_key: pending.registration_key,
        config_sha256: pending.config_sha256,
        registration_id: NewRegistrationId,
      });
    } else {
      const retryUpload = await makeUpload(
        'awaiting_bytes',
        {},
        pending.request,
      );
      api.create.mockResolvedValue(ok(retryUpload, 201));
      await act(async () => {
        await view.result.current.resend();
      });
      expect(api.create.mock.calls).toEqual([
        [DomainId, RegistrationId, OwnerId, pending.request],
      ]);
      expect(api.register).not.toHaveBeenCalled();
      expect(view.result.current.upload?.upload_id).toBe(retryUpload.upload_id);
      expect(
        loadOriginalRecovery(StorageKey, 'create', DomainId, OwnerId),
      ).toEqual({
        ...pending,
        upload_id: retryUpload.upload_id,
      });
    }
    expect(currentDomain).toHaveBeenCalledTimes(2);
    expect(api.put).not.toHaveBeenCalled();
  },
);

it.each(['registration', 'upload'] as const)(
  'keeps a %s by-key 404 closed when a fresh domain read is unavailable despite a cached visible domain',
  async (kind) => {
    await seedMissingResult(kind);
    const before = recoverySnapshot();
    jest
      .mocked(foundation.domain)
      .mockRejectedValue(new KnowledgeRequestError(404, 'knowledge_not_found'));
    const view = mountWithCachedDomain();
    await act(async () => {
      await view.result.current.recover();
      await view.result.current.resend();
      await view.result.current.register(
        originalRegistrationRequest(registration),
      );
    });
    expect(foundation.domain).toHaveBeenCalledTimes(1);
    expect(view.result.current.recoveryChanged).toBe(true);
    expect(view.result.current.blocked).toBe(true);
    expect(view.result.current.createPending).toBeNull();
    expect(view.result.current.canResend).toBe(false);
    expect(recoverySnapshot()).toEqual(before);
    expect(api.register).not.toHaveBeenCalled();
    expect(api.create).not.toHaveBeenCalled();
    expect(api.put).not.toHaveBeenCalled();
  },
);

it('refuses a changed registration key or payload after a missing by-key result, preserving the original digest and scope', async () => {
  const pending = await seedMissingResult('registration');
  const before = recoverySnapshot();
  const input = originalRegistrationRequest(registration);
  const view = mountFlow();
  await act(async () => {
    await view.result.current.recover();
  });
  for (const changed of [
    { ...input, registration_key: input.registration_key + ':different' },
    { ...input, include_paths: [FreshPath] },
    {
      ...input,
      default_classification: {
        ...input.default_classification,
        group_id: 'e'.repeat(32),
      },
    },
  ]) {
    await act(async () => {
      await view.result.current.register(changed);
    });
    expect(view.result.current.createPending).toEqual(pending);
    expect(view.result.current.canResend).toBe(true);
    expect(recoverySnapshot()).toEqual(before);
    expect(api.register).not.toHaveBeenCalled();
  }
  expect(foundation.domain).toHaveBeenCalledTimes(1);
  await act(async () => {
    await view.result.current.register(input);
  });
  expect(api.register.mock.calls).toEqual([[DomainId, OwnerId, input]]);
});

it.each([
  ['unauthorized', new KnowledgeRequestError(401, 'knowledge_unauthorized')],
  ['unavailable domain', new KnowledgeRequestError(404, 'knowledge_not_found')],
  ['another owner', { ...NativeDomain, owner_id: 'e'.repeat(32) }],
  ['another domain', { ...NativeDomain, domain_id: 'e'.repeat(32) }],
  ['contract change', { ...NativeDomain, contract_version: '2.0.0' }],
  [
    'policy change',
    { ...NativeDomain, policy: { ...NativeDomain.policy, version: 2 } },
  ],
  [
    'service-rejected policy DTO',
    new KnowledgeRequestError(502, 'knowledge_invalid_response'),
  ],
] as const)(
  'rechecks the current scope before explicit replay and closes on %s',
  async (_name, changedScope) => {
    await seedMissingResult('registration');
    const before = recoverySnapshot();
    const view = mountWithCachedDomain();
    await act(async () => {
      await view.result.current.recover();
    });
    expect(view.result.current.canResend).toBe(true);
    const scope = jest.mocked(foundation.domain);
    scope.mockImplementation(async () => {
      expect(lockDepth).toBe(1);
      if (changedScope instanceof Error) throw changedScope;
      return changedScope as unknown as Domain;
    });
    await act(async () => {
      await view.result.current.register(
        originalRegistrationRequest(registration),
      );
    });
    expect(scope).toHaveBeenCalledTimes(2);
    expect(view.result.current.recoveryChanged).toBe(true);
    expect(view.result.current.blocked).toBe(true);
    expect(view.result.current.createPending).toBeNull();
    expect(view.result.current.canResend).toBe(false);
    expect(recoverySnapshot()).toEqual(before);
    expect(api.register).not.toHaveBeenCalled();
    expect(api.create).not.toHaveBeenCalled();
    expect(api.put).not.toHaveBeenCalled();
  },
);

it('refuses upload replay when another tab replaces the slot during the fresh domain read', async () => {
  await seedMissingResult('upload');
  const view = mountFlow();
  await act(async () => {
    await view.result.current.recover();
  });
  expect(view.result.current.canResend).toBe(true);
  const scope = deferred<Domain>();
  jest.mocked(foundation.domain).mockReturnValue(scope.promise);
  let replay!: Promise<unknown>;
  act(() => {
    replay = view.result.current.resend();
  });
  await waitFor(() => expect(foundation.domain).toHaveBeenCalledTimes(2));
  const replacement = {
    ...originalScope(DomainId, OwnerId),
    kind: 'registration' as const,
    registration_key: 'other:tab:while-checking-scope',
    config_sha256: '6'.repeat(64),
    registration_id: null,
  };
  act(() => {
    saveOriginalRecovery(StorageKey, 'create', replacement);
    window.dispatchEvent(
      new StorageEvent('storage', { key: StorageKey + ':create' }),
    );
  });
  const replaced = recoverySnapshot();
  await act(async () => {
    scope.resolve(NativeDomain);
    await replay;
  });
  expect(view.result.current.recoveryChanged).toBe(true);
  expect(view.result.current.canResend).toBe(false);
  expect(recoverySnapshot()).toEqual(replaced);
  expect(api.create).not.toHaveBeenCalled();
  expect(api.register).not.toHaveBeenCalled();
});

it('does not consume an ordinary registration-by-ID 404 after a successful upload-by-key lookup', async () => {
  await seedMissingResult('upload');
  jest
    .mocked(service.uploadByKey)
    .mockResolvedValue(ok(await makeUpload('awaiting_bytes')));
  api.registration.mockRejectedValue(
    new OriginalRequestError(404, 'knowledge_not_found'),
  );
  const view = mountFlow();
  await act(async () => {
    await view.result.current.recover();
  });
  expect(view.result.current.recoveryChanged).toBe(true);
  expect(view.result.current.blocked).toBe(true);
  expect(view.result.current.canResend).toBe(false);
  expect(foundation.domain).not.toHaveBeenCalled();
  expect(api.create).not.toHaveBeenCalled();
  expect(api.register).not.toHaveBeenCalled();
});

it('B28 regression: rejects unsupported CSV registration locally with accurate input feedback and permits a corrected DOCX submission', async () => {
  render(<OriginalImport domain={DomainId} />, { wrapper: wrapper() });
  const form = await screen.findByRole('form', {
    name: 'knowledgeOriginal.registration',
  });
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.name'), {
    target: { value: 'Synthetic validation' },
  });
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.paths'), {
    target: { value: 'synthetic/unsupported.csv' },
  });
  const key = (
    within(form).getByLabelText(
      'knowledgeOriginal.registrationKey',
    ) as HTMLInputElement
  ).value;
  fireEvent.submit(form);
  await screen.findByText('knowledgeOriginal.invalidInput');
  expect(
    screen.queryByText('knowledgeOriginal.unconfirmed'),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: 'knowledgeOriginal.checkResult' }),
  ).not.toBeInTheDocument();
  expect(api.register).not.toHaveBeenCalled();
  expect(localStorage.getItem(StorageKey + ':create')).toBeNull();
  fireEvent.change(within(form).getByLabelText('knowledgeOriginal.paths'), {
    target: { value: FreshPath },
  });
  fireEvent.submit(form);
  await screen.findByRole('form', { name: 'knowledgeOriginal.prepare' });
  expect(api.register).toHaveBeenCalledTimes(1);
  expect(api.register.mock.calls[0][2]).toEqual(
    expect.objectContaining({
      registration_key: key,
      include_paths: [FreshPath],
    }),
  );
});

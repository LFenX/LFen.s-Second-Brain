import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react';
import {
  QueryClient as RuntimeQueryClient,
  QueryClientProvider,
} from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useOriginalFlow } from '@/pages/dataset/knowledge-import/use-original-flow';
import { useContentParsedUpload } from '@/hooks/use-knowledge-consumption';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import { getAuthorization } from '@/utils/authorization-util';
import { getKbDetail } from '@/services/knowledge-service';
import original from '@/services/knowledge-original-service';
import content from '@/services/knowledge-consumption-service';
import {
  originalScope,
  originalStorageKey,
  saveOriginalRecovery,
} from '@/utils/knowledge-original';
import {
  DomainId,
  OwnerId,
  RegistrationId,
  makeRegistration,
  makeUpload,
  makeReceipt,
  createRequest,
  confirmRequest,
} from '@/pages/dataset/knowledge-import/__tests__/original-fixture';
type OriginalUpload =
  import('@/interfaces/database/knowledge-original').OriginalUpload;

jest.mock('@/components/list-filter-bar/use-handle-filter-submit', () => ({}));
jest.mock('@/components/ui/message', () => ({ __esModule: true, default: {} }));
jest.mock('@/utils/dataset-util', () => ({ isDatasetId: () => true }));
jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (s: string) => s },
}));
jest.mock('@/hooks/logic-hooks', () => ({}));
jest.mock('@/hooks/route-hook', () => ({}));
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(() => {
    throw Error('No network allowed');
  }),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-counter-session'),
}));
jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {},
  getKbDetail: jest.fn(),
}));

const OtherDomain = '9'.repeat(32);
const Scope = { domain: DomainId, owner: OwnerId };
const StorageKey = originalStorageKey(DomainId, OwnerId);
type TestQueryClient = import('@tanstack/react-query').QueryClient;
const clients: TestQueryClient[] = [];
let counts: Record<string, { files: number; bytes: number }>;
let upload: OriginalUpload;
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}
function Counter({ domain }: { domain: string }) {
  const query = useFetchKnowledgeBaseConfiguration();
  return (
    <output data-testid={domain}>
      {query.data.document_count}|{query.data.size}
    </output>
  );
}
function mount() {
  const client = new RuntimeQueryClient({
    defaultOptions: {
      queries: { retry: false, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  });
  clients.push(client);
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = render(
    <>
      {[DomainId, OtherDomain].map((domain) => (
        <MemoryRouter key={domain} initialEntries={['/dataset/' + domain]}>
          <Routes>
            <Route path="/dataset/:id" element={<Counter domain={domain} />} />
          </Routes>
        </MemoryRouter>
      ))}
    </>,
    { wrapper },
  );
  return { client, wrapper, view };
}
async function ready() {
  await waitFor(() =>
    expect(screen.getByTestId(DomainId)).toHaveTextContent('2|239'),
  );
  await waitFor(() =>
    expect(screen.getByTestId(OtherDomain)).toHaveTextContent('7|700'),
  );
}
function assertUnchanged() {
  expect(screen.getByTestId(DomainId)).toHaveTextContent('2|239');
  expect(screen.getByTestId(OtherDomain)).toHaveTextContent('7|700');
  expect(
    (getKbDetail as jest.Mock).mock.calls.filter(([id]) => id === DomainId),
  ).toHaveLength(1);
  expect(
    (getKbDetail as jest.Mock).mock.calls.filter(([id]) => id === OtherDomain),
  ).toHaveLength(1);
}
async function seed(withConfirmation: boolean) {
  saveOriginalRecovery(StorageKey, 'create', {
    ...originalScope(DomainId, OwnerId),
    kind: 'upload',
    registration_id: RegistrationId,
    request: createRequest(),
    request_sha256: upload.request_sha256,
    upload_id: upload.upload_id,
  });
  if (withConfirmation)
    saveOriginalRecovery(StorageKey, 'confirm', {
      ...originalScope(DomainId, OwnerId),
      registration_id: RegistrationId,
      request: confirmRequest(upload),
      import_task_id: null,
    });
}
beforeEach(async () => {
  jest.restoreAllMocks();
  localStorage.clear();
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-counter-session');
  (getKbDetail as jest.Mock).mockReset();
  counts = {
    [DomainId]: { files: 2, bytes: 239 },
    [OtherDomain]: { files: 7, bytes: 700 },
  };
  (getKbDetail as jest.Mock).mockImplementation(async (id: string) => ({
    data: {
      code: 0,
      data: { id, document_count: counts[id].files, size: counts[id].bytes },
    },
  }));
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: async (_name: string, work: () => Promise<unknown>) => work(),
    },
  });
  upload = await makeUpload('ready');
  jest
    .spyOn(original, 'uploadByKey')
    .mockImplementation(async () => ({ data: upload, httpStatus: 200 }));
  jest
    .spyOn(original, 'registration')
    .mockResolvedValue({ data: await makeRegistration(), httpStatus: 200 });
});
afterEach(() => {
  for (const client of clients.splice(0)) client.clear();
  jest.restoreAllMocks();
});

it.each([
  'confirm',
  'recover-confirm',
  'recover-committed',
  'new-revision',
] as const)(
  'B28 regression: original %s refreshes mounted native counts in only the receipt domain without inventing a file',
  async (mode) => {
    if (mode === 'recover-committed') upload = await makeUpload('committed');
    if (mode === 'new-revision')
      upload = await makeUpload('ready', { action: 'updated' });
    const expectedFiles = mode === 'new-revision' ? 2 : 3;
    const saved = () => {
      counts[DomainId] = { files: expectedFiles, bytes: 317 };
    };
    jest
      .spyOn(original, 'confirm')
      .mockImplementation(async (_domain, _registration, request) => {
        saved();
        return { data: makeReceipt(upload, request), httpStatus: 201 };
      });
    jest.spyOn(original, 'receiptByKey').mockImplementation(async () => {
      saved();
      return { data: makeReceipt(upload), httpStatus: 200 };
    });
    jest.spyOn(original, 'receipt').mockImplementation(async () => {
      saved();
      return { data: makeReceipt(upload), httpStatus: 200 };
    });
    await seed(mode === 'recover-confirm');
    const mounted = mount();
    const hook = renderHook(
      () => useOriginalFlow(DomainId, OwnerId, StorageKey),
      { wrapper: mounted.wrapper },
    );
    await ready();
    await act(async () => {
      await hook.result.current.recover();
    });
    if (mode === 'confirm' || mode === 'new-revision')
      await act(async () => {
        await hook.result.current.confirm(true);
      });
    expect(hook.result.current.receipt?.status).toBe('succeeded');
    await waitFor(() =>
      expect(screen.getByTestId(DomainId)).toHaveTextContent(
        expectedFiles + '|317',
      ),
    );
    expect(screen.getByTestId(OtherDomain)).toHaveTextContent('7|700');
    expect(
      (getKbDetail as jest.Mock).mock.calls.filter(
        ([id]) => id === OtherDomain,
      ),
    ).toHaveLength(1);
    if (mode !== 'recover-committed') {
      await act(async () => {
        await hook.result.current.recover();
      });
      expect(screen.getByTestId(DomainId)).toHaveTextContent(
        expectedFiles + '|317',
      );
    }
  },
);

it.each(['domain', 'session', 'unmount'] as const)(
  'B28 regression: late original receipt after %s change cannot refresh either native counter',
  async (mode) => {
    const pending = deferred<{
      data: import('@/interfaces/database/knowledge-original').OriginalReceipt;
      httpStatus: number;
    }>();
    const read = jest
      .spyOn(original, 'receiptByKey')
      .mockReturnValue(pending.promise);
    await seed(true);
    const mounted = mount();
    const hook = renderHook(
      ({ domain }) =>
        useOriginalFlow(domain, OwnerId, originalStorageKey(domain, OwnerId)),
      { wrapper: mounted.wrapper, initialProps: { domain: DomainId } },
    );
    await ready();
    let work!: Promise<unknown>;
    act(() => {
      work = hook.result.current.recover();
    });
    await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
    if (mode === 'domain') hook.rerender({ domain: OtherDomain });
    else if (mode === 'session') {
      (getAuthorization as jest.Mock).mockReturnValue('synthetic-new-session');
      getKnowledgeSessionScope();
      hook.rerender({ domain: DomainId });
    } else hook.unmount();
    counts[DomainId] = { files: 3, bytes: 317 };
    await act(async () => {
      pending.resolve({ data: makeReceipt(upload), httpStatus: 200 });
      await work;
    });
    assertUnchanged();
  },
);

it('B28 regression: parsed upload refreshes the mounted native counter and preserves the unrelated domain', async () => {
  jest.spyOn(content, 'uploadParsedSource').mockImplementation(async () => {
    counts[DomainId] = { files: 3, bytes: 317 };
    return {
      source_id: '8'.repeat(32),
      original_filename: 'synthetic.csv',
      byte_length: 58,
    };
  });
  const mounted = mount();
  const hook = renderHook(() => useContentParsedUpload(Scope), {
    wrapper: mounted.wrapper,
  });
  await ready();
  await act(async () => {
    await hook.result.current.mutateAsync({
      file: new File(['item,quantity'], 'synthetic.csv'),
      request: {
        expected_policy_version: 1,
        expected_content_policy_version: 2,
        confirmed: true,
      },
    });
  });
  await waitFor(() =>
    expect(screen.getByTestId(DomainId)).toHaveTextContent('3|317'),
  );
  expect(screen.getByTestId(OtherDomain)).toHaveTextContent('7|700');
  expect(
    (getKbDetail as jest.Mock).mock.calls.filter(([id]) => id === OtherDomain),
  ).toHaveLength(1);
});

it.each(['domain', 'session', 'unmount'] as const)(
  'B28 regression: late parsed upload after %s change cannot refresh either native counter',
  async (mode) => {
    const pending = deferred<{
      source_id: string;
      original_filename: string;
      byte_length: number;
    }>();
    const save = jest
      .spyOn(content, 'uploadParsedSource')
      .mockReturnValue(pending.promise);
    const mounted = mount();
    const hook = renderHook(
      ({ domain }) => useContentParsedUpload({ ...Scope, domain }),
      { wrapper: mounted.wrapper, initialProps: { domain: DomainId } },
    );
    await ready();
    let work!: Promise<unknown>;
    act(() => {
      work = hook.result.current
        .mutateAsync({
          file: new File(['synthetic'], 'synthetic.csv'),
          request: {
            expected_policy_version: 1,
            expected_content_policy_version: 2,
            confirmed: true,
          },
        })
        .catch((error) => error);
    });
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    if (mode === 'domain') hook.rerender({ domain: OtherDomain });
    else if (mode === 'session') {
      (getAuthorization as jest.Mock).mockReturnValue('synthetic-new-session');
      getKnowledgeSessionScope();
      hook.rerender({ domain: DomainId });
    } else hook.unmount();
    counts[DomainId] = { files: 3, bytes: 317 };
    await act(async () => {
      pending.resolve({
        source_id: '8'.repeat(32),
        original_filename: 'synthetic.csv',
        byte_length: 58,
      });
      await work;
    });
    assertUnchanged();
  },
);

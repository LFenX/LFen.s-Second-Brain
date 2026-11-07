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
import { webcrypto, randomUUID } from 'node:crypto';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useKnowledgeSources } from '@/hooks/use-knowledge-foundation';
import { useImportFlow } from '@/pages/dataset/knowledge-import/use-import-flow';
import sources from '@/services/knowledge-sources-service';
import foundation from '@/services/knowledge-foundation-service';
import { getKbDetail } from '@/services/knowledge-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { getAuthorization } from '@/utils/authorization-util';
import {
  RegistrationRequest,
  Registration,
  PreviewRequest,
  makePreview,
  makeReceipt,
} from '@/pages/dataset/knowledge-import/__tests__/fixture';
import {
  DomainId,
  Source,
} from '@/pages/dataset/knowledge-catalogue/__tests__/fixture';

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
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'display-synthetic-session'),
}));
jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {},
  getKbDetail: jest.fn(),
}));
jest.mock('@/services/knowledge-foundation-service', () => {
  const actual = jest.requireActual('@/services/knowledge-foundation-service');
  return { ...actual, __esModule: true, default: { sources: jest.fn() } };
});
jest.mock('@/services/knowledge-sources-service', () => ({
  __esModule: true,
  default: {
    register: jest.fn(),
    preview: jest.fn(),
    confirm: jest.fn(),
    registration: jest.fn(),
    readPreview: jest.fn(),
    receipt: jest.fn(),
  },
}));
const OtherDomain = 'd'.repeat(32);
type TestQueryClient = import('@tanstack/react-query').QueryClient;
const clients: TestQueryClient[] = [];
let counts: Record<string, { files: number; size: number }>;
let consumed: boolean;
function saved() {
  counts[DomainId] = { files: 3, size: 317 };
}
function Counters({ domain }: { domain: string }) {
  const native = useFetchKnowledgeBaseConfiguration();
  const catalogue = useKnowledgeSources(domain);
  const total = catalogue.data?.pages.flatMap((p) => p.items).length;
  return (
    <p data-testid={'counters-' + domain}>
      {native.data.document_count}|{native.data.size}|{total}
    </p>
  );
}
function mount() {
  const client = new RuntimeQueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
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
            <Route path="/dataset/:id" element={<Counters domain={domain} />} />
          </Routes>
        </MemoryRouter>
      ))}
    </>,
    { wrapper },
  );
  const hook = renderHook(
    () => useImportFlow(DomainId, 'display-import-pending', jest.fn()),
    { wrapper },
  );
  return { ...hook, view, client };
}
async function ready() {
  await waitFor(() =>
    expect(screen.getByTestId('counters-' + DomainId)).toHaveTextContent(
      '2|239|2',
    ),
  );
  await waitFor(() =>
    expect(screen.getByTestId('counters-' + OtherDomain)).toHaveTextContent(
      '7|700|7',
    ),
  );
}
async function prepare(hook: ReturnType<typeof mount>) {
  await act(async () => {
    await hook.result.current.register(RegistrationRequest);
  });
  await act(async () => {
    await hook.result.current.makePreview(PreviewRequest);
  });
}
function assertOtherDomainUnchanged() {
  expect(screen.getByTestId('counters-' + OtherDomain)).toHaveTextContent(
    '7|700|7',
  );
  expect(
    (getKbDetail as jest.Mock).mock.calls.filter(([id]) => id === OtherDomain),
  ).toHaveLength(1);
  expect(
    (foundation.sources as jest.Mock).mock.calls.filter(
      ([id]) => id === OtherDomain,
    ),
  ).toHaveLength(1);
}
beforeAll(() => {
  Object.defineProperty(globalThis.crypto, 'subtle', {
    configurable: true,
    value: webcrypto.subtle,
  });
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    configurable: true,
    value: randomUUID,
  });
});
beforeEach(() => {
  jest.clearAllMocks();
  sessionStorage.clear();
  consumed = false;
  (getAuthorization as jest.Mock).mockReturnValue('display-synthetic-session');
  counts = {
    [DomainId]: { files: 2, size: 239 },
    [OtherDomain]: { files: 7, size: 700 },
  };
  (getKbDetail as jest.Mock).mockImplementation(async (id: string) => ({
    data: {
      code: 0,
      data: { id, document_count: counts[id].files, size: counts[id].size },
    },
  }));
  (foundation.sources as jest.Mock).mockImplementation(async (id: string) => ({
    items: Array.from({ length: counts[id].files }, (_, i) => ({
      ...Source,
      domain_id: id,
      source_id: (i + 1).toString(16).padStart(32, '0'),
    })),
    next_cursor: null,
  }));
  (sources.register as jest.Mock).mockResolvedValue({
    data: Registration,
    httpStatus: 201,
  });
  (sources.preview as jest.Mock).mockResolvedValue({
    data: makePreview(),
    httpStatus: 201,
  });
  (sources.registration as jest.Mock).mockResolvedValue(Registration);
  (sources.readPreview as jest.Mock).mockImplementation(async () => ({
    ...makePreview(),
    consumed_task_id: consumed ? makeReceipt().import_task_id : null,
  }));
  (sources.confirm as jest.Mock).mockImplementation(async () => {
    saved();
    consumed = true;
    return { data: makeReceipt(), httpStatus: 201 };
  });
  (sources.receipt as jest.Mock).mockImplementation(async () => makeReceipt());
});
afterEach(() => {
  for (const client of clients.splice(0)) client.clear();
});
it('refreshes the mounted native counts and catalogue after first successful confirmation', async () => {
  const hook = mount();
  await ready();
  await prepare(hook);
  await act(async () => {
    await hook.result.current.confirm();
  });
  await waitFor(() =>
    expect(screen.getByTestId('counters-' + DomainId)).toHaveTextContent(
      '3|317|3',
    ),
  );
  expect(sources.confirm).toHaveBeenCalledTimes(1);
  assertOtherDomainUnchanged();
});
it('refreshes both mounted observers after original-key replay without replacing the request', async () => {
  (sources.confirm as jest.Mock).mockRejectedValueOnce(
    new KnowledgeRequestError(0, 'knowledge_result_unknown'),
  );
  const hook = mount();
  await ready();
  await prepare(hook);
  await act(async () => {
    await hook.result.current.confirm();
  });
  expect(hook.result.current.phase).toBe('unknown');
  await act(async () => {
    await hook.result.current.recover();
  });
  await waitFor(() =>
    expect(screen.getByTestId('counters-' + DomainId)).toHaveTextContent(
      '3|317|3',
    ),
  );
  expect(sources.confirm).toHaveBeenCalledTimes(2);
  expect((sources.confirm as jest.Mock).mock.calls[1]).toEqual(
    (sources.confirm as jest.Mock).mock.calls[0],
  );
  assertOtherDomainUnchanged();
});
it('refreshes catalogue and native counts on consumed-preview receipt recovery without a replacement POST', async () => {
  (sources.confirm as jest.Mock).mockImplementationOnce(async () => {
    saved();
    consumed = true;
    throw new KnowledgeRequestError(0, 'knowledge_result_unknown');
  });
  const hook = mount();
  await ready();
  await prepare(hook);
  await act(async () => {
    await hook.result.current.confirm();
  });
  await act(async () => {
    await hook.result.current.recover();
  });
  expect(hook.result.current.phase).toBe('receipt');
  await waitFor(() =>
    expect(screen.getByTestId('counters-' + DomainId)).toHaveTextContent(
      '3|317|3',
    ),
  );
  expect(sources.confirm).toHaveBeenCalledTimes(1);
  expect(sources.receipt).toHaveBeenCalledTimes(1);
  assertOtherDomainUnchanged();
});
it('does not refresh either mounted observer for a failed receipt', async () => {
  (sources.confirm as jest.Mock).mockResolvedValue({
    data: makeReceipt(makePreview(), true),
    httpStatus: 201,
  });
  const hook = mount();
  await ready();
  await prepare(hook);
  await act(async () => {
    await hook.result.current.confirm();
  });
  expect(screen.getByTestId('counters-' + DomainId)).toHaveTextContent(
    '2|239|2',
  );
  expect(
    (getKbDetail as jest.Mock).mock.calls.filter(([id]) => id === DomainId),
  ).toHaveLength(1);
  expect(
    (foundation.sources as jest.Mock).mock.calls.filter(
      ([id]) => id === DomainId,
    ),
  ).toHaveLength(1);
  assertOtherDomainUnchanged();
});
it('refreshes both observers when a conflict replay rechecks consumption and reads the saved receipt', async () => {
  (sources.confirm as jest.Mock)
    .mockRejectedValueOnce(
      new KnowledgeRequestError(0, 'knowledge_result_unknown'),
    )
    .mockImplementationOnce(async () => {
      saved();
      consumed = true;
      throw new KnowledgeRequestError(409, 'knowledge_conflict');
    });
  const hook = mount();
  await ready();
  await prepare(hook);
  await act(async () => {
    await hook.result.current.confirm();
  });
  await act(async () => {
    await hook.result.current.recover();
  });
  expect(hook.result.current.phase).toBe('receipt');
  await waitFor(() =>
    expect(screen.getByTestId('counters-' + DomainId)).toHaveTextContent(
      '3|317|3',
    ),
  );
  expect(sources.confirm).toHaveBeenCalledTimes(2);
  expect(sources.receipt).toHaveBeenCalledTimes(1);
  expect((sources.confirm as jest.Mock).mock.calls[1]).toEqual(
    (sources.confirm as jest.Mock).mock.calls[0],
  );
  assertOtherDomainUnchanged();
});
it('does not refresh the mounted domain after an old-session confirmation resolves late', async () => {
  let finish!: (value: unknown) => void;
  (sources.confirm as jest.Mock).mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const hook = mount();
  await ready();
  await prepare(hook);
  let pending!: Promise<void>;
  act(() => {
    pending = hook.result.current.confirm();
  });
  await waitFor(() => expect(sources.confirm).toHaveBeenCalledTimes(1));
  (getAuthorization as jest.Mock).mockReturnValue(
    'different-display-synthetic-session',
  );
  saved();
  await act(async () => {
    finish({ data: makeReceipt(), httpStatus: 201 });
    await pending;
  });
  expect(
    (getKbDetail as jest.Mock).mock.calls.filter(([id]) => id === DomainId),
  ).toHaveLength(1);
  expect(
    (foundation.sources as jest.Mock).mock.calls.filter(
      ([id]) => id === DomainId,
    ),
  ).toHaveLength(1);
  assertOtherDomainUnchanged();
});

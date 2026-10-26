type ReactNode = import('react').ReactNode;
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service, { WebRequestError } from '@/services/knowledge-web-service';
import { getAuthorization } from '@/utils/authorization-util';
import {
  useWebResource,
  useWebRevisions,
  KnowledgeWebKeys,
  scopedWeb,
  collectWebPages,
} from '../use-knowledge-web';
import { getKnowledgeSessionScope } from '../use-knowledge-foundation';
import {
  fixture,
  Scope,
  DomainId,
  OwnerId,
  CaptureId,
} from '@/pages/dataset/knowledge-import/__tests__/web-fixture';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-web-session'),
}));
jest.mock('@/services/knowledge-web-service', () => {
  const actual = jest.requireActual('@/services/knowledge-web-service');
  return {
    ...actual,
    __esModule: true,
    default: { ...actual.default, capture: jest.fn(), revisions: jest.fn() },
  };
});
const Capture = service.capture as jest.Mock,
  Revisions = service.revisions as jest.Mock;
function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    client,
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  };
}
beforeEach(() => {
  Capture.mockReset();
  Revisions.mockReset();
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-web-session');
});
afterEach(() => jest.useRealTimers());
it('polls foreground mutable states and stops after three state failures until manual recheck', async () => {
  jest.useFakeTimers();
  const f = await fixture(),
    fetching = { ...f.capture, state: 'fetching', revision: 1 };
  Capture.mockResolvedValueOnce({ data: fetching }).mockRejectedValue(
    new WebRequestError(503, 'web_storage_unavailable'),
  );
  const { result, unmount } = renderHook(
    () =>
      useWebResource(Scope, 'capture', CaptureId, () =>
        service.capture(Scope, CaptureId),
      ),
    { wrapper: wrapper().wrapper },
  );
  await waitFor(() => expect(result.current.data?.state).toBe('fetching'));
  for (let i = 0; i < 3; i++)
    await act(async () => {
      await jest.advanceTimersByTimeAsync(2050);
    });
  expect(Capture).toHaveBeenCalledTimes(4);
  expect(result.current.pollingStopped).toBe(true);
  await act(async () => {
    await jest.advanceTimersByTimeAsync(10000);
  });
  expect(Capture).toHaveBeenCalledTimes(4);
  Capture.mockResolvedValue({ data: { ...f.capture, revision: 2 } });
  await act(async () => {
    await result.current.recheck();
  });
  await waitFor(() => expect(result.current.data?.state).toBe('ready'));
  expect(result.current.pollingStopped).toBe(false);
  unmount();
});
it('preserves the newest revision, rejects equal-revision contradictions and clears authorization failures', async () => {
  const f = await fixture(),
    failed = jest.fn();
  Capture.mockResolvedValue({ data: f.capture });
  const { result } = renderHook(
    () =>
      useWebResource(
        Scope,
        'capture',
        CaptureId,
        () => service.capture(Scope, CaptureId),
        failed,
      ),
    { wrapper: wrapper().wrapper },
  );
  await waitFor(() => expect(result.current.data).toEqual(f.capture));
  act(() => {
    result.current.adopt({
      ...f.capture,
      state: 'cancelled',
      revision: f.capture.revision + 1,
    });
  });
  await waitFor(() => expect(result.current.data?.state).toBe('cancelled'));
  Capture.mockResolvedValue({ data: f.capture });
  await act(async () => {
    await result.current.recheck();
  });
  await waitFor(() => expect(result.current.data?.state).toBe('cancelled'));
  expect(() =>
    result.current.adopt({ ...f.capture, revision: f.capture.revision + 1 }),
  ).toThrow();
  Capture.mockRejectedValue(new WebRequestError(401, 'knowledge_unauthorized'));
  await act(async () => {
    await result.current.recheck();
  });
  await waitFor(() => expect(failed).toHaveBeenCalled());
});
it('separates session, owner, domain, registration and filters and refuses a result after session replacement', async () => {
  const before = getKnowledgeSessionScope();
  expect(
    KnowledgeWebKeys.resource(before, Scope, 'capture', CaptureId),
  ).not.toEqual(
    KnowledgeWebKeys.resource(
      before,
      { ...Scope, owner: '8'.repeat(32) },
      'capture',
      CaptureId,
    ),
  );
  let resolve!: (value: number) => void;
  const promise = scopedWeb(
    before,
    () =>
      new Promise<number>((done) => {
        resolve = done;
      }),
  );
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-next-session');
  getKnowledgeSessionScope();
  resolve(1);
  await expect(promise).rejects.toMatchObject({ status: 401 });
});
it('follows short pages until the nullable cursor ends and rejects duplicates and cursor loops', async () => {
  const calls: (string | null)[] = [],
    load = jest.fn(async (cursor: string | null) => {
      calls.push(cursor);
      return {
        data: cursor
          ? { items: [{ id: 'b' }], next_cursor: null }
          : { items: [{ id: 'a' }], next_cursor: '1'.repeat(32) },
      };
    });
  expect(await collectWebPages(load, (item) => item.id, 32, true)).toHaveLength(
    2,
  );
  expect(calls).toEqual([null, '1'.repeat(32)]);
  await expect(
    collectWebPages(
      async () => ({
        data: { items: [{ id: 'a' }], next_cursor: '1'.repeat(32) },
      }),
      (item) => item.id,
      32,
    ),
  ).rejects.toThrow();
});
it('loads revisions lazily with group and registration filters and rejects crossed domains', async () => {
  const f = await fixture();
  Revisions.mockResolvedValue({
    data: { items: [f.revision], next_cursor: null },
  });
  const { result } = renderHook(
    () => ({
      ...useWebRevisions(DomainId, OwnerId, undefined, Scope.registration),
    }),
    { wrapper: wrapper().wrapper },
  );
  await waitFor(() =>
    expect(result.current.data?.pages[0].items).toHaveLength(1),
  );
  expect(Revisions.mock.calls[0][1]).toMatchObject({
    registration_id: Scope.registration,
  });
  Revisions.mockResolvedValue({
    data: {
      items: [{ ...f.revision, domain_id: '8'.repeat(32) }],
      next_cursor: null,
    },
  });
  await act(async () => {
    const value = await result.current.refetch();
    expect(value.error).toBeInstanceOf(Error);
  });
  await waitFor(() => expect(result.current.isError).toBe(true));
});

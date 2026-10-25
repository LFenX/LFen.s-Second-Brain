type ReactNode = import('react').ReactNode;
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service, {
  OriginalRequestError,
} from '@/services/knowledge-original-service';
import { getAuthorization } from '@/utils/authorization-util';
import {
  useOriginalUpload,
  useOriginalList,
  KnowledgeOriginalKeys,
  scopedOriginal,
} from '../use-knowledge-original';
import { getKnowledgeSessionScope } from '../use-knowledge-foundation';
import {
  DomainId,
  RegistrationId,
  UploadId,
  makeUpload,
  makeRevision,
} from '@/pages/dataset/knowledge-import/__tests__/original-fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-session'),
}));
jest.mock('@/services/knowledge-original-service', () => {
  const actual = jest.requireActual('@/services/knowledge-original-service');
  return {
    ...actual,
    __esModule: true,
    default: { ...actual.default, upload: jest.fn(), list: jest.fn() },
  };
});
const Upload = service.upload as jest.Mock,
  List = service.list as jest.Mock;
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
  Upload.mockReset();
  List.mockReset();
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-session');
});
afterEach(() => jest.useRealTimers());
it('polls receiving uploads every two foreground seconds and stops after three network/503 failures until an explicit GET check', async () => {
  jest.useFakeTimers();
  const receiving = await makeUpload('receiving');
  Upload.mockResolvedValueOnce({
    data: receiving,
    httpStatus: 200,
  }).mockRejectedValue(
    new OriginalRequestError(503, 'original_storage_unavailable'),
  );
  const { result, unmount } = renderHook(
    () => useOriginalUpload(DomainId, RegistrationId, UploadId),
    { wrapper: wrapper().wrapper },
  );
  await waitFor(() => expect(result.current.data?.state).toBe('receiving'));
  for (let n = 0; n < 3; n += 1)
    await act(async () => {
      await jest.advanceTimersByTimeAsync(2050);
    });
  expect(Upload).toHaveBeenCalledTimes(4);
  expect(result.current.pollingStopped).toBe(true);
  await act(async () => {
    await jest.advanceTimersByTimeAsync(10000);
  });
  expect(Upload).toHaveBeenCalledTimes(4);
  Upload.mockResolvedValue({
    data: { ...receiving, state: 'interrupted', revision: 2 },
    httpStatus: 200,
  });
  await act(async () => {
    await result.current.recheck();
  });
  await waitFor(() => expect(result.current.data?.state).toBe('interrupted'));
  expect(Upload).toHaveBeenCalledTimes(5);
  expect(result.current.pollingStopped).toBe(false);
  unmount();
  await act(async () => {
    await jest.advanceTimersByTimeAsync(4000);
  });
  expect(Upload).toHaveBeenCalledTimes(5);
});
it('adopts newer PUT state without allowing an older in-flight GET to overwrite it', async () => {
  const ready = await makeUpload('ready', { revision: 4 });
  let resolve!: (value: unknown) => void;
  Upload.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { result } = renderHook(
    () => useOriginalUpload(DomainId, RegistrationId, UploadId),
    { wrapper: wrapper().wrapper },
  );
  await waitFor(() => expect(Upload).toHaveBeenCalledTimes(1));
  act(() => {
    result.current.adopt(ready);
  });
  await act(async () => {
    resolve({
      data: { ...ready, state: 'receiving', revision: 3 },
      httpStatus: 200,
    });
  });
  await waitFor(() => expect(result.current.data?.revision).toBe(4));
  expect(result.current.data?.state).toBe('ready');
  expect(() => result.current.adopt({ ...ready, received_bytes: 1 })).toThrow();
});
it.each([401, 404, 409])(
  'stops and reports scope failures immediately for %i',
  async (status) => {
    jest.useFakeTimers();
    const receiving = await makeUpload('receiving'),
      failure = jest.fn();
    Upload.mockResolvedValueOnce({
      data: receiving,
      httpStatus: 200,
    }).mockRejectedValue(
      new OriginalRequestError(
        status,
        status === 409 ? 'policy_changed' : 'knowledge_not_found',
      ),
    );
    const { result } = renderHook(
      () => useOriginalUpload(DomainId, RegistrationId, UploadId, failure),
      { wrapper: wrapper().wrapper },
    );
    await waitFor(() => expect(result.current.data).toBeDefined());
    await act(async () => {
      await jest.advanceTimersByTimeAsync(2050);
    });
    expect(failure).toHaveBeenCalledTimes(1);
    await act(async () => {
      await jest.advanceTimersByTimeAsync(6000);
    });
    expect(Upload).toHaveBeenCalledTimes(2);
  },
);
it('rejects an old-session response after the native authorization changes', async () => {
  let resolve!: (value: unknown) => void;
  const session = getKnowledgeSessionScope();
  const response = scopedOriginal(
    session,
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  (getAuthorization as jest.Mock).mockReturnValue('another-synthetic-session');
  resolve({ data: await makeUpload(), httpStatus: 200 });
  await expect(response).rejects.toMatchObject({ status: 401 });
  const work = jest.fn();
  await expect(scopedOriginal(session, work)).rejects.toMatchObject({
    status: 401,
  });
  expect(work).not.toHaveBeenCalled();
});
it('keeps list group filters and treats repeated revisions/cursors as invalid rather than complete', async () => {
  const group = '4'.repeat(32),
    source = makeRevision(await makeUpload());
  source.classification = { ...source.classification, group_id: group };
  List.mockResolvedValue({
    data: { domain_id: DomainId, items: [source], next_cursor: 'next' },
    httpStatus: 200,
  });
  const { result } = renderHook(() => useOriginalList(DomainId, group), {
    wrapper: wrapper().wrapper,
  });
  await waitFor(() => expect(result.current.data).toBeDefined());
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(List.mock.calls[1].slice(0, 3)).toEqual([DomainId, group, 'next']);
  expect(KnowledgeOriginalKeys.list(1, DomainId, group)).toEqual([
    'knowledge-original',
    1,
    DomainId,
    'lists',
    group,
  ]);
});

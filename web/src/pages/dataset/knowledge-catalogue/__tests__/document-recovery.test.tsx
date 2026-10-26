import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service, {
  DocumentRequestError,
} from '@/services/knowledge-document-service';
import { useDocumentFlow } from '../use-document-flow';
import {
  Scope,
  Task,
  TaskOperation,
  PolicyOperation,
  PolicyReceipt,
  Rejections,
} from './document-fixture';
import {
  readDocumentSlots,
  saveDocumentSlot,
  documentSlotKey,
} from '@/utils/knowledge-document';
type ReactNode = import('react').ReactNode;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'document-flow-session'),
}));
jest.mock('@/services/knowledge-document-service', () => {
  const actual = jest.requireActual('@/services/knowledge-document-service');
  return {
    ...actual,
    __esModule: true,
    default: {
      ...actual.default,
      createTask: jest.fn(),
      changePolicy: jest.fn(),
      byKey: jest.fn(),
      task: jest.fn(),
    },
  };
});
function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  };
}
beforeEach(() => {
  localStorage.clear();
  jest.clearAllMocks();
});
it('recovers a lost POST once by its complete original key and keeps Task tracking', async () => {
  (service.createTask as jest.Mock).mockRejectedValue(
    new DocumentRequestError(0, 'document_result_unknown'),
  );
  (service.byKey as jest.Mock).mockResolvedValue({
    data: Task,
    httpStatus: 200,
  });
  const { result } = renderHook(() => useDocumentFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.ready).toBe(true));
  await act(async () => {
    await result.current.submit(TaskOperation);
  });
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.byKey).toHaveBeenCalledWith(
    Scope,
    { kind: 'task', idempotency_key: Task.original_request.idempotency_key },
    expect.any(AbortSignal),
  );
  expect((await readDocumentSlots(localStorage, Scope.owner))[0].task_id).toBe(
    Task.task_id,
  );
});
it('retains the slot for503 followed by404; no automatic replacement POST', async () => {
  (service.createTask as jest.Mock).mockRejectedValue(
    new DocumentRequestError(503, 'unavailable'),
  );
  (service.byKey as jest.Mock).mockRejectedValue(
    new DocumentRequestError(404, 'not_found'),
  );
  const { result } = renderHook(() => useDocumentFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.ready).toBe(true));
  await act(async () => {
    await expect(result.current.submit(TaskOperation)).rejects.toThrow();
  });
  expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(1);
  expect(service.createTask).toHaveBeenCalledTimes(1);
});
it('does not abandon from a409 hint when by-key is unavailable', async () => {
  (service.changePolicy as jest.Mock).mockRejectedValue(
    new DocumentRequestError(
      409,
      'document_policy_changed',
      '',
      true,
      'e'.repeat(32),
    ),
  );
  (service.byKey as jest.Mock).mockRejectedValue(
    new DocumentRequestError(503, 'unavailable'),
  );
  const { result } = renderHook(() => useDocumentFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.ready).toBe(true));
  await act(async () => {
    await expect(result.current.submit(PolicyOperation)).rejects.toThrow();
  });
  const slot = (await readDocumentSlots(localStorage, Scope.owner))[0];
  await act(async () => {
    await expect(result.current.abandon(slot)).rejects.toThrow(
      'Fixed GET rejection required',
    );
  });
  expect(await readDocumentSlots(localStorage, Scope.owner)).toHaveLength(1);
});
it('allows explicit abandonment only after a matching complete GET rejection', async () => {
  const rejection = Rejections.policy,
    op = {
      kind: 'policy' as const,
      scope: Scope,
      body: rejection.original_request as import('@/interfaces/request/knowledge-document').DocumentPolicyChangeRequest,
    };
  (service.changePolicy as jest.Mock).mockRejectedValue(
    new DocumentRequestError(409, 'document_policy_changed'),
  );
  (service.byKey as jest.Mock).mockResolvedValue({
    data: rejection,
    httpStatus: 200,
  });
  const { result } = renderHook(() => useDocumentFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.ready).toBe(true));
  await act(async () => {
    await result.current.submit(op);
  });
  const slot = result.current.slots[0];
  expect(result.current.rejections[documentSlotKey(slot)]).toBe(true);
  await act(async () => {
    await result.current.abandon(slot);
  });
  expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([]);
});
it('restores by GET without automatically resending any saved policy request', async () => {
  await saveDocumentSlot(localStorage, PolicyOperation);
  (service.byKey as jest.Mock).mockResolvedValue({
    data: PolicyReceipt,
    httpStatus: 200,
  });
  renderHook(() => useDocumentFlow(Scope), wrapper());
  await waitFor(() => expect(service.byKey).toHaveBeenCalledTimes(1));
  await waitFor(async () =>
    expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([]),
  );
  expect(service.changePolicy).not.toHaveBeenCalled();
});
it('blocks a new write when storage is corrupt', async () => {
  localStorage.setItem(
    'knowledge-document-structure:0.1.0:' + Scope.owner + ':bad',
    '{',
  );
  const { result } = renderHook(() => useDocumentFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.error).toBeTruthy());
  await act(async () => {
    await expect(result.current.submit(TaskOperation)).rejects.toThrow();
  });
  expect(service.createTask).not.toHaveBeenCalled();
});

import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import service, {
  StructureRequestError,
} from '@/services/knowledge-structure-service';
import { getAuthorization } from '@/utils/authorization-util';
import { useStructureFlow } from '../use-structure-flow';
import {
  saveStructureSlot,
  readStructureSlots,
  structureSlotKey,
} from '@/utils/knowledge-structure';
import {
  Scope,
  Task,
  TaskOperation,
  PolicyOperation,
  CancelOperation,
  PolicyReceipt,
  CancelReceipt,
  rejection,
  running,
} from './structure-fixture';
type ReactNode = import('react').ReactNode;
type Operation = import('@/utils/knowledge-structure').StructureOperation;
type Outcome = import('@/utils/knowledge-structure').StructureOutcome;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-structure-recovery'),
}));
jest.mock('@/services/knowledge-structure-service', () => {
  const actual = jest.requireActual('@/services/knowledge-structure-service');
  return {
    ...actual,
    __esModule: true,
    default: Object.fromEntries(
      Object.entries(actual.default).map(([key, value]) => [
        key,
        typeof value === 'function' ? jest.fn() : value,
      ]),
    ),
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
const write = (operation: Operation) =>
  service[
    operation.kind === 'task'
      ? 'createTask'
      : operation.kind === 'policy'
        ? 'changePolicy'
        : 'cancel'
  ] as jest.Mock;
const lookup = (operation: Operation) =>
  service[
    operation.kind === 'task' ? 'taskByKey' : 'commandByKey'
  ] as jest.Mock;
beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  (getAuthorization as jest.Mock).mockReturnValue(
    'synthetic-structure-recovery',
  );
});
it.each([TaskOperation, PolicyOperation, CancelOperation])(
  'persists the complete $kind request before POST and restores by its original key',
  async (operation) => {
    const expected: Outcome =
      operation.kind === 'task'
        ? running()
        : operation.kind === 'policy'
          ? PolicyReceipt
          : CancelReceipt;
    lookup(operation).mockRejectedValue(
      new StructureRequestError(404, 'not_found'),
    );
    write(operation).mockImplementation(async () => {
      const stored = await readStructureSlots(localStorage, Scope.owner);
      expect(stored).toHaveLength(1);
      expect(stored[0].body).toEqual(operation.body);
      throw new StructureRequestError(0, 'structure_result_unknown');
    });
    const first = renderHook(() => useStructureFlow(Scope), wrapper());
    await waitFor(() => expect(first.result.current.ready).toBe(true));
    await act(async () => {
      await expect(first.result.current.submit(operation)).rejects.toThrow();
    });
    expect(first.result.current.slots).toHaveLength(1);
    await act(async () => {
      await expect(
        first.result.current.submit({
          ...operation,
          body: { ...operation.body, idempotency_key: 'new-key' },
        } as Operation),
      ).rejects.toThrow('structure_original_key_required');
    });
    expect(write(operation)).toHaveBeenCalledTimes(1);
    first.unmount();
    expect(lookup(operation)).toHaveBeenCalledTimes(1);
    lookup(operation)
      .mockReset()
      .mockResolvedValue({ data: expected, httpStatus: 200 });
    const second = renderHook(() => useStructureFlow(Scope), wrapper());
    await waitFor(() => expect(lookup(operation)).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(Object.values(second.result.current.outcomes)).toHaveLength(1),
    );
    if (operation.kind === 'task') {
      expect(second.result.current.slots[0].task_id).toBe(Task.task_id);
    } else expect(second.result.current.slots).toHaveLength(0);
    expect(write(operation)).toHaveBeenCalledTimes(1);
  },
);
it.each([401, 404, 409, 412, 429, 503])(
  'retains the original slot for HTTP %i even with outcome/rejection hints',
  async (status) => {
    const slot = await saveStructureSlot(localStorage, TaskOperation);
    (service.taskByKey as jest.Mock).mockRejectedValue(
      new StructureRequestError(
        status,
        'policy_changed',
        '',
        true,
        'f'.repeat(32),
      ),
    );
    const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.slots).toHaveLength(status === 401 ? 0 : 1);
    expect(result.current.rejections[structureSlotKey(slot)]).toBeUndefined();
    await act(async () => {
      await expect(result.current.abandon(slot)).rejects.toThrow(
        'Fixed GET rejection required',
      );
    });
    expect(await readStructureSlots(localStorage, Scope.owner)).toHaveLength(1);
  },
);
it.each([TaskOperation, PolicyOperation, CancelOperation])(
  'permits abandoning $kind only after a full matching GET rejection',
  async (operation) => {
    const slot = await saveStructureSlot(localStorage, operation),
      proof = await rejection(operation);
    lookup(operation).mockResolvedValue({ data: proof, httpStatus: 200 });
    const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
    await waitFor(() =>
      expect(result.current.rejections[structureSlotKey(slot)]).toBe(true),
    );
    await act(async () => {
      await result.current.abandon(slot);
    });
    expect(result.current.slots).toEqual([]);
    expect(write(operation)).not.toHaveBeenCalled();
  },
);
it('rejects a fake fixed rejection with the wrong digest and preserves cancel/task separation', async () => {
  const taskSlot = await saveStructureSlot(localStorage, TaskOperation),
    cancelSlot = await saveStructureSlot(localStorage, CancelOperation);
  (service.taskByKey as jest.Mock).mockResolvedValue({
    data: running(),
    httpStatus: 200,
  });
  (service.commandByKey as jest.Mock).mockResolvedValue({
    data: await rejection(CancelOperation, { request_sha256: '0'.repeat(64) }),
    httpStatus: 200,
  });
  const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.error).not.toBeNull());
  expect(
    result.current.rejections[structureSlotKey(cancelSlot)],
  ).toBeUndefined();
  expect(result.current.slots.map((slot) => slot.kind)).toEqual([
    'task',
    'cancel',
  ]);
  (service.commandByKey as jest.Mock).mockResolvedValue({
    data: await rejection(CancelOperation),
    httpStatus: 200,
  });
  await act(async () => {
    await result.current.recover(cancelSlot);
  });
  await act(async () => {
    await result.current.abandon(cancelSlot);
  });
  expect(result.current.slots).toHaveLength(1);
  expect(result.current.slots[0].task_id).toBe(Task.task_id);
  expect(localStorage.getItem(structureSlotKey(taskSlot))).not.toBeNull();
});
it('blocks POST on corrupt or failed storage and preserves old-protocol data', async () => {
  localStorage.setItem('old-protocol', 'keep');
  localStorage.setItem(
    'knowledge-structure:0.1.0:' + Scope.owner + ':corrupt',
    '{',
  );
  const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.error).not.toBeNull());
  expect(result.current.ready).toBe(false);
  await act(async () => {
    await expect(result.current.submit(TaskOperation)).rejects.toThrow();
  });
  expect(service.createTask).not.toHaveBeenCalled();
  expect(localStorage.getItem('old-protocol')).toBe('keep');
});
it('ignores a late recovered receipt after session change and leaves physical owner slots intact', async () => {
  const slot = await saveStructureSlot(localStorage, PolicyOperation);
  let finish!: (value: { data: Outcome; httpStatus: number }) => void;
  (service.commandByKey as jest.Mock).mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
  await waitFor(() => expect(service.commandByKey).toHaveBeenCalled());
  await act(async () => {
    (getAuthorization as jest.Mock).mockReturnValue('different-session');
    finish({ data: PolicyReceipt, httpStatus: 200 });
  });
  expect(result.current.outcomes).toEqual({});
  expect(localStorage.getItem(structureSlotKey(slot))).not.toBeNull();
});
it('immediately queries the original key after a lost POST response and accepts a fixed result', async () => {
  (service.createTask as jest.Mock).mockRejectedValue(
    new StructureRequestError(0, 'structure_result_unknown'),
  );
  (service.taskByKey as jest.Mock).mockResolvedValue({
    data: running(),
    httpStatus: 200,
  });
  const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.ready).toBe(true));
  await act(async () => {
    expect(await result.current.submit(TaskOperation)).toMatchObject({
      task_id: Task.task_id,
    });
  });
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.taskByKey).toHaveBeenCalledWith(
    Scope,
    TaskOperation.body.idempotency_key,
  );
  expect(result.current.slots[0].task_id).toBe(Task.task_id);
});
it('resends the immutable saved original body/key only after an explicit action', async () => {
  const slot = await saveStructureSlot(localStorage, PolicyOperation);
  (service.commandByKey as jest.Mock).mockRejectedValue(
    new StructureRequestError(404, 'not_found'),
  );
  const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
  await waitFor(() => expect(result.current.error).not.toBeNull());
  expect(service.changePolicy).not.toHaveBeenCalled();
  (service.changePolicy as jest.Mock).mockResolvedValue({
    data: PolicyReceipt,
    httpStatus: 200,
  });
  await act(async () => {
    await result.current.resend(slot);
  });
  expect(service.changePolicy).toHaveBeenCalledWith(
    PolicyOperation.scope,
    PolicyOperation.body,
  );
  expect(result.current.slots).toEqual([]);
});
it('requires a verified rejection before a new key and resets the preparation epoch on abandonment', async () => {
  const slot = await saveStructureSlot(localStorage, PolicyOperation);
  (service.commandByKey as jest.Mock).mockResolvedValue({
    data: await rejection(PolicyOperation),
    httpStatus: 200,
  });
  const { result } = renderHook(() => useStructureFlow(Scope), wrapper());
  await waitFor(() =>
    expect(result.current.rejections[structureSlotKey(slot)]).toBe(true),
  );
  await act(async () => {
    await expect(
      result.current.submit({
        ...PolicyOperation,
        body: { ...PolicyOperation.body, idempotency_key: 'new-key' },
      }),
    ).rejects.toThrow('structure_original_key_required');
  });
  await act(async () => {
    await result.current.abandon(slot);
  });
  expect(result.current.preparationEpoch).toBeGreaterThan(0);
  expect(service.changePolicy).not.toHaveBeenCalled();
});

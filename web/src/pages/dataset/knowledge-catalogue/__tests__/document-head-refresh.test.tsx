import { useState } from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import {
  QueryClient as RuntimeQueryClient,
  QueryClientProvider,
  focusManager,
} from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { useDocumentFlow } from '@/pages/dataset/knowledge-catalogue/use-document-flow';
import { DocumentTaskPanel } from '@/pages/dataset/knowledge-catalogue/document-tasks';
import { useDocumentHead } from '@/hooks/use-knowledge-document';
import service, {
  DocumentRequestError,
} from '@/services/knowledge-document-service';
import { getAuthorization } from '@/utils/authorization-util';
import { validDocumentData } from '@/utils/knowledge-document-schema';
import {
  documentRequestDigest,
  documentSlotKey,
  readDocumentSlots,
} from '@/utils/knowledge-document';
import {
  Scope,
  Task,
  Rep,
  Head,
  TaskOperation,
  clone,
} from '@/pages/dataset/knowledge-catalogue/__tests__/document-fixture';
type DocumentTask =
  import('@/interfaces/database/knowledge-document').DocumentTask;
type DocumentScope = import('@/utils/knowledge-document').DocumentScope;

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'display-document-session'),
}));
jest.mock('@/utils/knowledge-document-schema', () => {
  const actual = jest.requireActual('@/utils/knowledge-document-schema');
  return { ...actual, validDocumentData: jest.fn(actual.validDocumentData) };
});
jest.mock('@/services/knowledge-document-service', () => {
  const actual = jest.requireActual('@/services/knowledge-document-service');
  return {
    ...actual,
    __esModule: true,
    default: {
      ...actual.default,
      head: jest.fn(),
      task: jest.fn(),
      createTask: jest.fn(),
      byKey: jest.fn(),
    },
  };
});
const OtherSource = 'f'.repeat(32);
type TestQueryClient = import('@tanstack/react-query').QueryClient;
type TestDocumentFlow =
  import('@/pages/dataset/knowledge-catalogue/use-document-flow').DocumentFlow;
const clients: TestQueryClient[] = [];
let serverTask: DocumentTask;
let published: boolean;
let exposed: { flow: TestDocumentFlow; setTask: (task: string) => void };
function HeadReader({
  scope,
  source,
  label,
}: {
  scope: DocumentScope;
  source: string;
  label: string;
}) {
  const head = useDocumentHead({ ...scope, source });
  return (
    <p data-testid={label}>
      {head.data?.currentness}:{head.data?.active_representation_id || 'none'}
      {head.isError ? ':head-error' : ''}
    </p>
  );
}
function Harness({
  scope = Scope,
  known = false,
}: {
  scope?: DocumentScope;
  known?: boolean;
}) {
  const flow = useDocumentFlow(scope);
  const [task, setTask] = useState<string | null>(known ? Task.task_id : null);
  exposed = { flow, setTask };
  return (
    <>
      <HeadReader scope={scope} source={Task.original.source_id} label="head" />
      <HeadReader scope={scope} source={OtherSource} label="other-head" />
      {task && <DocumentTaskPanel scope={scope} task={task} flow={flow} />}
    </>
  );
}
function mount(known = false) {
  const client = new RuntimeQueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  });
  clients.push(client);
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(<Harness known={known} />, { wrapper });
  return { ...view, client, wrapper };
}
async function ready() {
  await waitFor(() => expect(exposed.flow.ready).toBe(true));
  await waitFor(() =>
    expect(screen.getByTestId('head')).toHaveTextContent('none:none'),
  );
}
function completed(): DocumentTask {
  return {
    ...clone(Task),
    state: 'succeeded',
    phase: 'done',
    attempt_count: 1,
    revision: Task.revision + 1,
    completed_at: Task.updated_at,
    result_representation_id: Rep.representation_id,
    failure_code: null,
  };
}
function publish() {
  published = true;
  serverTask = completed();
}
async function start() {
  await act(async () => {
    const value = await exposed.flow.submit(TaskOperation);
    if (value.record_kind !== 'task') throw Error('Expected task');
    exposed.setTask(value.task_id);
  });
}
async function current() {
  await waitFor(
    () =>
      expect(screen.getByTestId('head')).toHaveTextContent(
        'current:' + Rep.representation_id,
      ),
    { timeout: 3600 },
  );
}
const ownReads = () =>
  (service.head as jest.Mock).mock.calls.filter(
    ([scope]) => scope.source === Task.original.source_id,
  );
beforeAll(() => {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    value: 'visible',
  });
  focusManager.setFocused(true);
});
afterAll(() => focusManager.setFocused(undefined));
beforeEach(async () => {
  localStorage.clear();
  jest.clearAllMocks();
  (validDocumentData as jest.Mock).mockImplementation(
    jest.requireActual('@/utils/knowledge-document-schema').validDocumentData,
  );
  published = false;
  (getAuthorization as jest.Mock).mockReturnValue('display-document-session');
  serverTask = clone(Task);
  expect(await validDocumentData('OperationRecord', serverTask)).toBe(true);
  expect(await validDocumentData('OperationRecord', completed())).toBe(true);
  (service.head as jest.Mock).mockImplementation(
    async (scope: DocumentScope & { source: string }) => ({
      data: {
        ...clone(Head),
        domain_id: scope.domain,
        source_id: scope.source,
        currentness:
          published &&
          scope.source === Task.original.source_id &&
          scope.domain === Scope.domain
            ? 'current'
            : 'none',
        active_representation_id:
          published &&
          scope.source === Task.original.source_id &&
          scope.domain === Scope.domain
            ? Rep.representation_id
            : null,
        active_source_revision_id:
          published && scope.source === Task.original.source_id
            ? Task.original.source_revision_id
            : null,
      },
      httpStatus: 200,
    }),
  );
  (service.task as jest.Mock).mockImplementation(async () => ({
    data: clone(serverTask),
    httpStatus: 200,
  }));
  (service.createTask as jest.Mock).mockImplementation(async () => ({
    data: clone(serverTask),
    httpStatus: 201,
  }));
  (service.byKey as jest.Mock).mockImplementation(async () => ({
    data: clone(serverTask),
    httpStatus: 200,
  }));
});
afterEach(() => {
  for (const client of clients.splice(0)) client.clear();
});
it('updates the mounted head when the original task poll succeeds, without navigation or a second POST', async () => {
  const view = mount();
  await ready();
  await start();
  publish();
  await current();
  const reads = ownReads().length;
  await act(async () => {
    await exposed.flow.reconcile(clone(serverTask));
    await exposed.flow.reconcile({
      ...clone(serverTask),
      revision: serverTask.revision - 1,
    });
    await exposed.flow.reconcile(clone(serverTask));
  });
  view.rerender(<Harness />);
  expect(ownReads()).toHaveLength(reads);
  expect(
    (service.head as jest.Mock).mock.calls.filter(
      ([s]) => s.source === OtherSource,
    ),
  ).toHaveLength(1);
  expect(service.createTask).toHaveBeenCalledTimes(1);
});
it('refreshes the mounted head after lost-response recovery continues polling to success', async () => {
  (service.createTask as jest.Mock).mockRejectedValueOnce(
    new DocumentRequestError(0, 'document_result_unknown'),
  );
  mount();
  await ready();
  await start();
  publish();
  await current();
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.byKey).toHaveBeenCalledTimes(1);
});
it('refreshes a known task without a recovery slot when polling reaches success', async () => {
  mount(true);
  await ready();
  expect(exposed.flow.slots).toHaveLength(0);
  publish();
  await current();
  expect(service.createTask).not.toHaveBeenCalled();
});
it('does not repeat head refresh when an immediately successful POST is observed again by TaskPanel', async () => {
  (service.createTask as jest.Mock).mockImplementation(async () => {
    publish();
    return { data: clone(serverTask), httpStatus: 201 };
  });
  mount();
  await ready();
  await start();
  await current();
  await waitFor(() => expect(service.task).toHaveBeenCalled());
  await act(async () => {
    await exposed.flow.reconcile(clone(serverTask));
    await exposed.flow.reconcile(clone(serverTask));
  });
  expect(ownReads()).toHaveLength(2);
  expect(service.createTask).toHaveBeenCalledTimes(1);
});
it('does not repeat an already terminal head refresh after original-key GET recovery', async () => {
  (service.createTask as jest.Mock).mockRejectedValueOnce(
    new DocumentRequestError(0, 'document_result_unknown'),
  );
  (service.byKey as jest.Mock).mockImplementation(async () => {
    publish();
    return { data: clone(serverTask), httpStatus: 200 };
  });
  mount();
  await ready();
  await start();
  await current();
  await waitFor(() => expect(service.task).toHaveBeenCalled());
  await act(async () => {
    await exposed.flow.reconcile(clone(serverTask));
  });
  expect(ownReads()).toHaveLength(2);
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.byKey).toHaveBeenCalledTimes(1);
});
it('ignores a late callback from an unmounted flow while a head observer stays mounted', async () => {
  const view = mount();
  await ready();
  const old = exposed.flow;
  view.rerender(
    <HeadReader scope={Scope} source={Task.original.source_id} label="head" />,
  );
  await waitFor(() =>
    expect(screen.getByTestId('head')).toHaveTextContent('none:none'),
  );
  const reads = ownReads().length;
  publish();
  await act(async () => {
    await old.reconcile(clone(serverTask));
  });
  expect(ownReads()).toHaveLength(reads);
  expect(screen.getByTestId('head')).toHaveTextContent('none:none');
});
it('ignores old-scope callbacks after the same flow hook changes domain', async () => {
  const view = mount();
  await ready();
  const old = exposed.flow;
  const next = { ...Scope, domain: 'e'.repeat(32) };
  view.rerender(<Harness scope={next} />);
  await waitFor(() => expect(exposed.flow.ready).toBe(true));
  await waitFor(() =>
    expect(screen.getByTestId('head')).toHaveTextContent('none:none'),
  );
  const reads = (service.head as jest.Mock).mock.calls.length;
  publish();
  await act(async () => {
    await old.reconcile(clone(serverTask));
  });
  expect((service.head as jest.Mock).mock.calls).toHaveLength(reads);
  expect(screen.getByTestId('head')).toHaveTextContent('none:none');
});
it('does not refresh a head when the session changed before the late terminal callback', async () => {
  mount();
  await ready();
  const old = exposed.flow;
  const reads = ownReads().length;
  (getAuthorization as jest.Mock).mockReturnValue(
    'different-document-synthetic-session',
  );
  publish();
  await act(async () => {
    await old.reconcile(clone(serverTask));
  });
  expect(ownReads()).toHaveLength(reads);
  expect(service.createTask).not.toHaveBeenCalled();
});
it('V2: does not resurrect a slot or roll back an outcome when an older validation finishes after terminal acceptance', async () => {
  mount();
  await ready();
  await act(async () => {
    await exposed.flow.submit(TaskOperation);
  });
  expect(exposed.flow.slots).toHaveLength(1);
  const key = documentSlotKey(exposed.flow.slots[0]);
  let releaseOld = () => {},
    signalPaused = () => {};
  const paused = new Promise<void>((resolve) => {
    signalPaused = resolve;
  });
  const resume = new Promise<void>((resolve) => {
    releaseOld = resolve;
  });
  const actual = jest.requireActual('@/utils/knowledge-document-schema');
  let held = false;
  (validDocumentData as jest.Mock).mockImplementation(
    async (name: string, value: DocumentTask) => {
      const valid = await actual.validDocumentData(name, value);
      if (
        !held &&
        name === 'OperationRecord' &&
        value.task_id === Task.task_id &&
        value.revision === Task.revision
      ) {
        held = true;
        signalPaused();
        await resume;
      }
      return valid;
    },
  );
  const older = exposed.flow.reconcile(clone(Task));
  await paused;
  try {
    publish();
    await act(async () => {
      await exposed.flow.reconcile(clone(serverTask));
    });
    await current();
    expect(localStorage.getItem(key)).toBeNull();
    expect(exposed.flow.outcomes[key]).toEqual(serverTask);
  } finally {
    await act(async () => {
      releaseOld();
      await older;
    });
  }
  expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([]);
  expect(exposed.flow.slots).toEqual([]);
  expect(exposed.flow.outcomes[key]).toEqual(serverTask);
  expect(screen.getByTestId('head')).toHaveTextContent(
    'current:' + Rep.representation_id,
  );
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.byKey).not.toHaveBeenCalled();
});
it('V2: retries an actual failed QueryClient head refetch at the same revision while preserving the acknowledged POST', async () => {
  mount();
  await ready();
  const originalHead = (service.head as jest.Mock).getMockImplementation();
  let rejectNextHead = true;
  (service.head as jest.Mock).mockImplementation(async (...args: unknown[]) => {
    if (
      rejectNextHead &&
      (args[0] as { source: string }).source === Task.original.source_id
    ) {
      rejectNextHead = false;
      throw new DocumentRequestError(503, 'unavailable');
    }
    return originalHead?.(...args);
  });
  (service.createTask as jest.Mock).mockImplementation(async () => {
    publish();
    return { data: clone(serverTask), httpStatus: 201 };
  });
  await act(async () => {
    const acknowledged = await exposed.flow.submit(TaskOperation);
    expect(acknowledged).toEqual(serverTask);
  });
  await waitFor(() =>
    expect(screen.getByTestId('head')).toHaveTextContent(':head-error'),
  );
  expect(Object.values(exposed.flow.outcomes)).toEqual([serverTask]);
  expect(exposed.flow.error).toBeNull();
  expect(await readDocumentSlots(localStorage, Scope.owner)).toEqual([]);
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.byKey).not.toHaveBeenCalled();
  expect(ownReads()).toHaveLength(2);
  await act(async () => {
    await exposed.flow.reconcile(clone(serverTask));
  });
  await current();
  expect(screen.getByTestId('head')).not.toHaveTextContent(':head-error');
  expect(ownReads()).toHaveLength(3);
  await act(async () => {
    await exposed.flow.reconcile(clone(serverTask));
    await exposed.flow.reconcile(clone(serverTask));
  });
  expect(ownReads()).toHaveLength(3);
  expect(service.createTask).toHaveBeenCalledTimes(1);
  expect(service.byKey).not.toHaveBeenCalled();
});
it('V3: does not install another task obsolete slot from a full snapshot captured before its terminal commit', async () => {
  const other = clone(Task);
  other.task_id = 'd'.repeat(32);
  other.original.source_id = OtherSource;
  other.original.source_revision_id = 'e'.repeat(32);
  other.original_request.target.source_id = OtherSource;
  other.original_request.target.source_revision_id =
    other.original.source_revision_id;
  other.original_request.idempotency_key = 'synthetic_task_docx_other';
  other.request_sha256 = await documentRequestDigest(
    'task',
    { ...Scope, source: OtherSource },
    other.original_request,
  );
  const terminal: DocumentTask = {
    ...clone(other),
    state: 'succeeded',
    phase: 'done',
    attempt_count: 1,
    revision: other.revision + 1,
    completed_at: other.updated_at,
    result_representation_id: 'c'.repeat(64),
    failure_code: null,
  };
  expect(await validDocumentData('OperationRecord', other)).toBe(true);
  expect(await validDocumentData('OperationRecord', terminal)).toBe(true);
  const originalHead = (service.head as jest.Mock).getMockImplementation();
  let otherPublished = false;
  (service.head as jest.Mock).mockImplementation(
    async (scope: DocumentScope & { source: string }, ...args: unknown[]) => {
      if (scope.source === OtherSource && otherPublished)
        return {
          data: {
            ...clone(Head),
            source_id: OtherSource,
            currentness: 'current',
            active_representation_id: terminal.result_representation_id,
            active_source_revision_id: other.original.source_revision_id,
          },
          httpStatus: 200,
        };
      return originalHead?.(scope, ...args);
    },
  );
  (service.createTask as jest.Mock).mockImplementation(
    async (scope: DocumentScope & { source: string }) => ({
      data: clone(scope.source === OtherSource ? other : Task),
      httpStatus: 201,
    }),
  );
  mount();
  await ready();
  await act(async () => {
    await exposed.flow.submit(TaskOperation);
    await exposed.flow.submit({
      kind: 'task',
      scope: { ...Scope, source: OtherSource },
      body: clone(other.original_request),
    });
  });
  const firstSlot = exposed.flow.slots.find(
    (slot) => slot.target === Task.original.source_id,
  );
  const otherSlot = exposed.flow.slots.find(
    (slot) => slot.target === OtherSource,
  );
  if (!firstSlot || !otherSlot) throw Error('Expected both saved task slots');
  expect(firstSlot.task_id).toBe(Task.task_id);
  expect(otherSlot.task_id).toBe(other.task_id);
  const firstKey = documentSlotKey(firstSlot),
    otherKey = documentSlotKey(otherSlot);
  let releaseRead = () => {},
    signalCaptured = () => {};
  const captured = new Promise<void>((resolve) => {
    signalCaptured = resolve;
  });
  const resume = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  const actual = jest.requireActual('@/utils/knowledge-document-schema');
  let held = false;
  (validDocumentData as jest.Mock).mockImplementation(
    async (name: string, value: { idempotency_key?: string }) => {
      const valid = await actual.validDocumentData(name, value);
      if (
        !held &&
        name === 'TaskRequest' &&
        value.idempotency_key === other.original_request.idempotency_key
      ) {
        held = true;
        signalCaptured();
        await resume;
      }
      return valid;
    },
  );
  let recovering: Promise<unknown> = Promise.resolve();
  await act(async () => {
    recovering = exposed.flow.recover(firstSlot);
    await captured;
  });
  try {
    otherPublished = true;
    await act(async () => {
      await exposed.flow.reconcile(terminal);
    });
    expect(localStorage.getItem(otherKey)).toBeNull();
    expect(exposed.flow.slots.map(documentSlotKey)).toEqual([firstKey]);
    expect(exposed.flow.outcomes[otherKey]).toEqual(terminal);
  } finally {
    await act(async () => {
      releaseRead();
      await recovering;
    });
  }
  expect(localStorage.getItem(otherKey)).toBeNull();
  expect(
    (await readDocumentSlots(localStorage, Scope.owner)).map(documentSlotKey),
  ).toEqual([firstKey]);
  expect(exposed.flow.slots.map(documentSlotKey)).toEqual([firstKey]);
  expect(exposed.flow.outcomes[otherKey]).toEqual(terminal);
  expect(screen.getByTestId('other-head')).toHaveTextContent(
    'current:' + terminal.result_representation_id,
  );
  expect(service.createTask).toHaveBeenCalledTimes(2);
  expect(service.byKey).not.toHaveBeenCalled();
});

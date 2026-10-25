import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import {
  QueryClient,
  QueryClientProvider,
  focusManager,
} from '@tanstack/react-query';
import { webcrypto } from 'node:crypto';
import request from '@/utils/next-request';
import KnowledgeImport from '..';
import { createBatchBackend, DomainId, Registration } from './batch-fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { getUserInfoObject: () => ({ id: 'owner' }) },
  getAuthorization: () => 'synthetic-batch-session',
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
const Http = request as unknown as jest.Mock;
let backend = createBatchBackend();
beforeAll(() => {
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  });
});
beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  backend = createBatchBackend();
  Http.mockReset().mockImplementation(backend.handler);
  focusManager.setFocused(true);
});
afterEach(() => {
  jest.useRealTimers();
  focusManager.setFocused(undefined);
});
function mount(extra = '') {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[
          '/dataset/knowledge-import/' +
            DomainId +
            '?mode=batch&registration=' +
            Registration.registration_id +
            extra,
        ]}
      >
        <Routes>
          <Route
            path="/dataset/knowledge-import/:id"
            element={<KnowledgeImport />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
async function prepare(paths = ['web/card.tsx', 'web/empty.ts']) {
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeBatch.openRegistration',
    }),
  );
  fireEvent.change(
    await screen.findByLabelText('knowledgeDirectory.exactSelection'),
    { target: { value: paths.join('\n') } },
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.observe' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeBatch.prepare' }),
  );
  await screen.findByRole('region', { name: 'knowledgeBatch.preview' });
}
async function approve() {
  const region = screen.getByRole('region', { name: 'knowledgeBatch.preview' });
  for (const item of backend.lastPreview!.items) {
    fireEvent.click(
      within(region).getByRole('button', {
        name: 'knowledgeBatch.readItem · ' + item.relative_path,
      }),
    );
    const box = within(region).getByLabelText(
      'knowledgeBatch.approveItem · ' + item.relative_path,
    );
    await waitFor(() => expect(box).toBeEnabled());
    fireEvent.click(box);
  }
}
async function accept() {
  await approve();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeBatch.accept' }),
  );
}

it('requires successful fixed reading and active per-item approval before one durable acceptance', async () => {
  backend.taskStatus = 'succeeded';
  mount();
  await prepare();
  const button = screen.getByRole('button', { name: 'knowledgeBatch.accept' });
  expect(button).toBeDisabled();
  expect(backend.acceptRequests).toHaveLength(0);
  backend.badWindow = true;
  fireEvent.click(
    screen.getByRole('button', {
      name: 'knowledgeBatch.readItem · web/card.tsx',
    }),
  );
  await screen.findByRole('alert');
  expect(
    screen.getByLabelText('knowledgeBatch.approveItem · web/card.tsx'),
  ).toBeDisabled();
  backend.badWindow = false;
  await accept();
  await screen.findByRole('heading', {
    name: 'knowledgeBatch.taskStates.succeeded',
  });
  expect(backend.acceptRequests).toHaveLength(1);
  expect(backend.acceptRequests[0].confirmed_item_ids).toEqual(
    backend.lastPreview!.items.map((i) => i.item_id),
  );
  expect(
    await screen.findAllByRole('link', {
      name: 'knowledgeImport.readRevision',
    }),
  ).toHaveLength(2);
  expect(backend.publishRequests).toHaveLength(0);
});

it('recovers a committed lost response after remount using GET and the original key without another POST', async () => {
  backend.acceptMode = 'timeout-after';
  backend.taskStatus = 'succeeded';
  const view = mount();
  await prepare();
  await accept();
  await screen.findByText('knowledgeBatch.pending');
  const original = JSON.parse(JSON.stringify(backend.acceptRequests[0]));
  view.unmount();
  mount();
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeBatch.checkOriginal' }),
  );
  await screen.findByRole('heading', {
    name: 'knowledgeBatch.taskStates.succeeded',
  });
  expect(backend.acceptRequests).toEqual([original]);
  expect(backend.registrationPosts).toBe(0);
  const stored = Object.values(localStorage).join('');
  expect(stored).not.toContain('web/card.tsx');
  expect(stored).not.toContain('synthetic-batch-session');
});

it('keeps an unknown acceptance unresolved until explicit replay of the identical request', async () => {
  backend.acceptMode = 'timeout-before';
  mount();
  await prepare();
  await accept();
  await screen.findByText('knowledgeBatch.pending');
  const original = JSON.parse(JSON.stringify(backend.acceptRequests[0]));
  const check = screen.getByRole('button', {
    name: 'knowledgeBatch.checkOriginal',
  });
  await waitFor(() => expect(check).toBeEnabled());
  fireEvent.click(check);
  await screen.findByText('knowledgeBatch.errors.notVisible');
  expect(backend.acceptRequests).toHaveLength(1);
  expect(
    screen.queryByRole('button', { name: 'knowledgeBatch.newBatch' }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.reload' }),
  );
  backend.acceptMode = 'success';
  backend.taskStatus = 'succeeded';
  fireEvent.click(
    await screen.findByLabelText('knowledgeDirectory.resendConsent'),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.resend' }),
  );
  await screen.findByRole('heading', {
    name: 'knowledgeBatch.taskStates.succeeded',
  });
  expect(backend.acceptRequests).toEqual([original, original]);
});

it('clears every previous approval when the explicit selection produces a replacement preview', async () => {
  mount();
  await prepare();
  await approve();
  const old = backend.lastPreview!.preview_id;
  fireEvent.click(screen.getByLabelText('web/empty.ts'));
  expect(
    screen.queryByRole('region', { name: 'knowledgeBatch.preview' }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeBatch.prepare' }),
  );
  await screen.findByRole('region', { name: 'knowledgeBatch.preview' });
  expect(backend.lastPreview!.preview_id).not.toBe(old);
  expect(
    screen.getByLabelText('knowledgeBatch.approveItem · web/card.tsx'),
  ).not.toBeChecked();
  expect(
    screen.getByRole('button', { name: 'knowledgeBatch.accept' }),
  ).toBeDisabled();
  expect(backend.acceptRequests).toHaveLength(0);
});

it('keeps successful fixed links on partial failure and retries only unfinished paths with a fresh preview', async () => {
  backend.taskStatus = 'partial_failed';
  mount();
  await prepare();
  await accept();
  await screen.findByRole('heading', {
    name: 'knowledgeBatch.taskStates.partial_failed',
  });
  expect(
    await screen.findAllByRole('link', {
      name: 'knowledgeImport.readRevision',
    }),
  ).toHaveLength(1);
  expect(
    screen.queryByRole('button', { name: 'knowledgeBatch.prepareProject' }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeBatch.retryUnfinished' }),
  );
  await screen.findByText('knowledgeBatch.retryParent', { exact: false });
  expect(
    await screen.findByLabelText('knowledgeDirectory.exactSelection'),
  ).toHaveValue('web/empty.ts');
  expect(backend.acceptRequests).toHaveLength(1);
});

it('cancels only after explicit consent and preserves the success rows', async () => {
  mount();
  await prepare();
  await accept();
  const consent = await screen.findByLabelText('knowledgeBatch.cancelConsent');
  const cancel = screen.getByRole('button', {
    name: 'knowledgeBatch.cancel',
  });
  expect(cancel).toBeDisabled();
  fireEvent.click(consent);
  fireEvent.click(cancel);
  await screen.findByRole('heading', {
    name: 'knowledgeBatch.taskStates.cancelled',
  });
  expect(backend.cancelRequests).toHaveLength(1);
  expect(backend.acceptRequests).toHaveLength(1);
});

it('stops automatic polling after three transient failures and never resubmits or cancels on unmount', async () => {
  jest.useFakeTimers();
  const view = mount();
  await prepare();
  await accept();
  await screen.findByRole('heading', {
    name: 'knowledgeBatch.taskStates.queued',
  });
  backend.taskFailures = 3;
  await act(async () => {
    await jest.advanceTimersByTimeAsync(6500);
  });
  await screen.findByText('knowledgeBatch.manualCheck');
  const reads = Http.mock.calls.filter(([c]) =>
    c.url.endsWith('/batch-tasks/' + backend.lastTaskId),
  ).length;
  await act(async () => {
    await jest.advanceTimersByTimeAsync(8000);
  });
  expect(
    Http.mock.calls.filter(([c]) =>
      c.url.endsWith('/batch-tasks/' + backend.lastTaskId),
    ),
  ).toHaveLength(reads);
  view.unmount();
  expect(backend.cancelRequests).toHaveLength(0);
  expect(backend.acceptRequests).toHaveLength(1);
});

import {
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
import {
  createBatchBackend,
  DomainId,
  ProjectId,
  Registration,
  makePreview,
  makeTask,
  makeResults,
} from './batch-fixture';

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
let backend = createBatchBackend(),
  taskId = '';
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
  const preview = makePreview(),
    task = makeTask(preview, 'succeeded');
  taskId = task.task_id;
  backend.records.set(taskId, {
    task,
    preview,
    results: makeResults(task, preview),
  });
  backend.finish('succeeded', taskId);
  Http.mockReset().mockImplementation(backend.handler);
  focusManager.setFocused(false);
});
afterEach(() => focusManager.setFocused(undefined));
function mount() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  });
  const route =
    '/dataset/knowledge-import/' +
    DomainId +
    '?mode=batch&batch_tab=projects&registration=' +
    Registration.registration_id +
    '&task=' +
    taskId +
    '&project=' +
    ProjectId;
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[route]}>
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
async function prepare() {
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeBatch.prepareProject',
    }),
  );
  return screen.findByRole('region', {
    name: 'knowledgeBatch.manifestPreview',
  });
}
async function publish() {
  fireEvent.click(screen.getByLabelText('knowledgeBatch.publicationConsent'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeBatch.publish' }),
  );
}

it('requires separate project confirmation and publishes fixed C1 tuples without another file import', async () => {
  mount();
  await prepare();
  expect(backend.publishRequests).toHaveLength(0);
  expect(backend.lastManifest!.expected_manifest_id).toBeNull();
  expect(
    screen.getByLabelText('knowledgeBatch.publicationConsent'),
  ).not.toBeChecked();
  expect(
    screen.getByRole('button', { name: 'knowledgeBatch.publish' }),
  ).toBeDisabled();
  const original = backend.lastManifest!.items.map((i) => i.source_revision_id);
  await publish();
  await screen.findByText('knowledgeBatch.published');
  const fixed = await screen.findByRole('region', {
    name: 'knowledgeBatch.fixedManifest',
  });
  expect(
    within(fixed).getAllByRole('link', {
      name: 'knowledgeImport.readRevision',
    }),
  ).toHaveLength(original.length);
  expect(
    [...backend.versions.values()][0].items.map((i) => i.source_revision_id),
  ).toEqual(original);
  expect(backend.acceptRequests).toHaveLength(0);
});

it('persists an explicit null base and recovers a lost publication through its independent GET slot', async () => {
  backend.publicationMode = 'timeout-after';
  const view = mount();
  await prepare();
  await publish();
  await screen.findByText('knowledgeBatch.publicationPending');
  const original = JSON.parse(JSON.stringify(backend.publishRequests[0]));
  expect(Object.values(localStorage).join('')).toContain(
    '"expected_manifest_id":null',
  );
  view.unmount();
  mount();
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeBatch.checkPublication',
    }),
  );
  await screen.findByText('knowledgeBatch.published');
  expect(backend.publishRequests).toEqual([original]);
  expect(backend.acceptRequests).toHaveLength(0);
});

it('keeps the original failed CAS request and requires a fresh preview and confirmation for a changed base', async () => {
  backend.publicationMode = 'cas';
  mount();
  await prepare();
  await publish();
  await screen.findByText('knowledgeBatch.reasons.project_head_changed');
  expect(backend.publishRequests).toHaveLength(1);
  const original = JSON.parse(JSON.stringify(backend.publishRequests[0]));
  backend.publicationMode = 'success';
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeBatch.prepareProject' }),
  );
  await screen.findByRole('region', { name: 'knowledgeBatch.manifestPreview' });
  expect(backend.lastManifest!.expected_manifest_id).toBe(backend.head);
  expect(backend.publishRequests).toEqual([original]);
  expect(
    screen.getByLabelText('knowledgeBatch.publicationConsent'),
  ).not.toBeChecked();
});

it('reads 50+1 fixed entries and 20+1 history records after the acquisition root is revoked', async () => {
  backend.seedProject(51, 21);
  backend.rootRevoked = true;
  mount();
  const fixed = await screen.findByRole('region', {
    name: 'knowledgeBatch.fixedManifest',
  });
  await waitFor(() =>
    expect(
      within(fixed).getAllByRole('link', {
        name: 'knowledgeImport.readRevision',
      }),
    ).toHaveLength(50),
  );
  fireEvent.click(
    within(fixed).getByRole('button', { name: 'knowledgeCatalogue.more' }),
  );
  await waitFor(() =>
    expect(
      within(fixed).getAllByRole('link', {
        name: 'knowledgeImport.readRevision',
      }),
    ).toHaveLength(51),
  );
  const history = screen.getByRole('region', {
    name: 'knowledgeBatch.history',
  });
  expect(
    within(history)
      .getAllByRole('button')
      .filter((b) => b.textContent?.includes(' · ')),
  ).toHaveLength(20);
  fireEvent.click(
    within(history).getByRole('button', { name: 'knowledgeCatalogue.more' }),
  );
  await waitFor(() =>
    expect(
      within(history)
        .getAllByRole('button')
        .filter((b) => b.textContent?.includes(' · ')),
    ).toHaveLength(21),
  );
  expect(backend.registrationReads).toBe(0);
  expect(backend.publishRequests).toHaveLength(0);
});

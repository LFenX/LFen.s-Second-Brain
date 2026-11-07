import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useLocation } from 'react-router';
import request from '@/utils/next-request';
import Datasets from '../index';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'synthetic-entry-ui',
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
let MockOwner = 'b'.repeat(32);
jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({ data: { id: MockOwner }, loading: false }),
}));
let MockNativePage: Record<string, unknown>;
const MockShowNative = jest.fn();
jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchNextKnowledgeListByPage: () => MockNativePage,
}));
jest.mock('@/hooks/logic-hooks', () => ({
  useGoToPreviousPageOnEmpty: () => undefined,
}));
jest.mock('../hooks', () => ({
  useSaveKnowledge: () => ({ visible: false, showModal: MockShowNative }),
}));
jest.mock('../use-select-owners', () => ({ useSelectOwners: () => [] }));
jest.mock('../use-rename-dataset', () => ({
  useRenameDataset: () => ({ datasetRenameVisible: false }),
}));
jest.mock('../dataset-card', () => ({
  DatasetCard: ({ dataset }: { dataset: { id: string } }) => (
    <div data-testid="native-card-probe">{dataset.id}</div>
  ),
}));
jest.mock('../dataset-creating-dialog', () => ({
  DatasetCreatingDialog: () => <div />,
}));
jest.mock('@/components/rename-dialog', () => ({
  RenameDialog: () => <div />,
}));
jest.mock('@/components/list-filter-bar', () => ({
  __esModule: true,
  default: ({ children }: { children: unknown }) => (
    <div>{children as never}</div>
  ),
}));
jest.mock('@/components/card-container', () => ({
  CardContainer: ({ children }: { children: unknown }) => (
    <div>{children as never}</div>
  ),
}));
jest.mock('@/components/ui/ragflow-pagination', () => ({
  RAGFlowPagination: () => <div data-testid="native-pagination-probe" />,
}));
jest.mock('@/components/empty/empty', () => ({
  EmptyAppCard: ({ onClick }: { onClick: () => void }) => (
    <button onClick={onClick}>native-create-probe</button>
  ),
}));

const Http = request as unknown as jest.Mock;
const Id = 'a'.repeat(32),
  Owner = 'b'.repeat(32);
const Domain = {
  domain_id: Id,
  name: 'Synthetic domain',
  category: 'project',
  owner_id: Owner,
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
const Native = {
  id: Id,
  name: Domain.name,
  permission: 'me',
  created_by: Owner,
  document_count: 0,
  chunk_count: 0,
  token_num: 0,
};
const respond = (data: unknown) => ({
  status: 200,
  data: { code: 0, data, contract_version: '1.0.0', request_id: 'synthetic' },
});
let Registered = false;
function transport(config: { method: string; url: string }) {
  if (config.method === 'post') return respond(Native);
  if (config.method === 'put') {
    Registered = true;
    return respond(Domain);
  }
  if (config.url.endsWith('/knowledge/domains'))
    return respond({ items: Registered ? [Domain] : [], next_cursor: '' });
  if (config.url.endsWith('/knowledge/domain')) return respond(Domain);
  return { status: 200, data: { code: 0, data: [Native], total_datasets: 1 } };
}
function LocationProbe() {
  const location = useLocation();
  return (
    <output data-testid="location">
      {location.pathname + location.search}
    </output>
  );
}
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const tree = () => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/datasets?page=3&keywords=kept']}>
        <Datasets />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(tree());
  return { ...view, rerenderScope: () => view.rerender(tree()) };
}
beforeEach(() => {
  MockOwner = Owner;
  Registered = false;
  MockShowNative.mockReset();
  Http.mockReset().mockImplementation(transport);
  MockNativePage = {
    kbs: [],
    loading: false,
    searchString: '',
    pagination: { current: 3, pageSize: 20 },
    refetch: jest.fn(async () => ({})),
    setPagination: jest.fn(),
    setSearchString: jest.fn(),
    setFilterValue: jest.fn(),
  };
});

it.each(['empty', 'loading', 'list'])(
  'keeps one opt-in entry in %s state without eager queries or replacing native actions',
  (mode) => {
    if (mode === 'loading') MockNativePage.loading = true;
    if (mode === 'list') MockNativePage.kbs = [Native];
    mount();
    expect(
      screen.getAllByRole('button', { name: 'knowledgeDomain.entry' }),
    ).toHaveLength(1);
    expect(Http).not.toHaveBeenCalled();
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/datasets?page=3&keywords=kept',
    );
    if (mode === 'empty') {
      fireEvent.click(screen.getByText('native-create-probe'));
      expect(MockShowNative).toHaveBeenCalledTimes(1);
    }
    if (mode === 'list')
      expect(screen.getByTestId('native-card-probe')).toHaveTextContent(Id);
  },
);

it('offers a normal from-zero UI, confirms the same KB and keeps existing route/filter/page', async () => {
  mount();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  fireEvent.change(await screen.findByLabelText('knowledgeDomain.name'), {
    target: { value: 'Synthetic domain' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  );
  await screen.findByRole('region', { name: 'knowledgeDomain.ready' });
  expect(Http.mock.calls.find(([c]) => c.method === 'post')[0].data).toEqual({
    name: 'Synthetic domain',
    permission: 'me',
    parse_type: 1,
    parser_id: 'general',
  });
  expect(MockNativePage.refetch).toHaveBeenCalledWith({ throwOnError: true });
  expect(
    screen.getByRole('link', { name: 'knowledgeDomain.original' }),
  ).toHaveAttribute('href', `/dataset/knowledge-import/${Id}?mode=original`);
  expect(
    screen.getByRole('link', { name: 'knowledgeDomain.directory' }),
  ).toHaveAttribute('href', `/dataset/knowledge-import/${Id}?mode=directory`);
  expect(MockShowNative).not.toHaveBeenCalled();
  expect(screen.getByTestId('location')).toHaveTextContent(
    '/datasets?page=3&keywords=kept',
  );
  expect(MockNativePage.setPagination).not.toHaveBeenCalled();
});

it('holds a busy dialog and scopes while two click attempts wait for one native response', async () => {
  let release!: (value: unknown) => void;
  Http.mockImplementation((config) =>
    config.method === 'post'
      ? new Promise((resolve) => {
          release = resolve;
        })
      : transport(config),
  );
  mount();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  fireEvent.change(await screen.findByLabelText('knowledgeDomain.name'), {
    target: { value: 'Synthetic domain' },
  });
  const create = screen.getByRole('button', { name: 'knowledgeDomain.create' });
  fireEvent.click(create);
  fireEvent.click(create);
  fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  expect(screen.getByLabelText('knowledgeDomain.category')).toBeDisabled();
  expect(screen.getByLabelText('knowledgeDomain.select')).toBeDisabled();
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(1);
  await act(async () => {
    release(respond(Native));
  });
  await screen.findByRole('region', { name: 'knowledgeDomain.ready' });
});

it('shows unknown native state and recovers by selecting a distinct real ID instead of resending POST', async () => {
  const other = { ...Native, id: 'd'.repeat(32) };
  Http.mockImplementation((config) => {
    if (config.method === 'post')
      return Promise.reject(new Error('synthetic response loss'));
    if (config.url === '/api/v1/datasets')
      return {
        status: 200,
        data: { code: 0, data: [Native, other], total_datasets: 2 },
      };
    if (config.url.includes(other.id))
      return respond({ ...Domain, domain_id: other.id });
    return transport(config);
  });
  mount();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  fireEvent.change(await screen.findByLabelText('knowledgeDomain.name'), {
    target: { value: 'Synthetic domain' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  );
  await screen.findByText('knowledgeDomain.nativeUnknown');
  expect(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  ).toBeDisabled();
  fireEvent.click(
    await screen.findByRole('button', {
      name: `${Native.name} · ${other.id.slice(-8)}`,
    }),
  );
  await screen.findByRole('region', { name: 'knowledgeDomain.ready' });
  expect(
    screen.getByRole('link', { name: 'knowledgeDomain.original' }),
  ).toHaveAttribute(
    'href',
    `/dataset/knowledge-import/${other.id}?mode=original`,
  );
  await waitFor(() =>
    expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(
      1,
    ),
  );
});

it('clears the old owner draft and suppresses its late native result in the actual entry UI', async () => {
  let release!: (value: unknown) => void;
  Http.mockImplementation((config) =>
    config.method === 'post'
      ? new Promise((resolve) => {
          release = resolve;
        })
      : transport(config),
  );
  const view = mount();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  fireEvent.change(await screen.findByLabelText('knowledgeDomain.name'), {
    target: { value: 'Old owner synthetic draft' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  );
  MockOwner = 'e'.repeat(32);
  view.rerenderScope();
  await waitFor(() =>
    expect(screen.getByLabelText('knowledgeDomain.name')).toHaveValue(''),
  );
  await act(async () => {
    release(respond(Native));
  });
  expect(Http.mock.calls.filter(([c]) => c.method === 'put')).toHaveLength(0);
  expect(
    screen.queryByRole('region', { name: 'knowledgeDomain.ready' }),
  ).not.toBeInTheDocument();
});

it('opens a fresh intentional creation after closing a confirmed domain, without an automatic POST', async () => {
  mount();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  fireEvent.change(await screen.findByLabelText('knowledgeDomain.name'), {
    target: { value: 'Synthetic domain' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  );
  await screen.findByRole('region', { name: 'knowledgeDomain.ready' });
  fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  expect(await screen.findByLabelText('knowledgeDomain.name')).toHaveValue('');
  expect(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  ).toBeEnabled();
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(1);
});

it('retains a confirmed native ID across dismissing a definite registration refusal', async () => {
  Http.mockImplementation((config) =>
    config.method === 'put'
      ? Promise.reject({
          response: {
            status: 409,
            data: { code: 'knowledge_domain_not_empty' },
          },
        })
      : transport(config),
  );
  mount();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  fireEvent.change(await screen.findByLabelText('knowledgeDomain.name'), {
    target: { value: 'Synthetic domain' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  );
  await screen.findByText('knowledgeDomain.rejected');
  fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDomain.entry' }),
  );
  expect(
    await screen.findByText('knowledgeDomain.nativeCreated'),
  ).toBeInTheDocument();
  expect(
    screen.getByRole('button', { name: 'knowledgeDomain.create' }),
  ).toBeDisabled();
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(1);
});

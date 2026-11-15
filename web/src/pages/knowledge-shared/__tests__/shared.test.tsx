import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import nativeService from '@/services/knowledge-access-native-service';
import { SharedKnowledgeReader } from '../reader';
import SharedKnowledgePage from '../index';
import notification from '@/utils/notification';
jest.mock('@/utils/notification', () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
import {
  actor,
  deferred,
  evidence,
  moduleSummary,
  searchResult,
  view,
  wrapper,
  Tenant,
  User,
} from '@/services/__tests__/knowledge-access-fixtures';
type AccessReadData =
  import('@/interfaces/database/knowledge-access').AccessReadData;
type AccessInspectData =
  import('@/interfaces/database/knowledge-access').AccessInspectData;
import { KnowledgeAccessKeys } from '@/hooks/knowledge-access-keys';
import { nativeSessionRevision } from '@/utils/native-session-scope';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils/register-server', () => ({
  registerNextServer: () => ({ post: jest.fn() }),
}));
jest.mock('@/services/knowledge-access-service', () => {
  const actual = jest.requireActual('@/services/knowledge-access-service');
  return {
    __esModule: true,
    ...actual,
    default: {
      inspect: jest.fn(),
      read: jest.fn(),
      search: jest.fn(),
      listModules: jest.fn(),
    },
  };
});
jest.mock('@/services/knowledge-access-native-service', () => ({
  __esModule: true,
  default: { identity: jest.fn() },
}));
const api = jest.mocked(service),
  native = jest.mocked(nativeService);
beforeEach(() => {
  jest.clearAllMocks();
  api.inspect.mockResolvedValue(view());
  api.read.mockResolvedValue(evidence());
});

it.each(['actor', 'module', 'late_actor'])(
  'reopens %s scope with empty query and a fresh subset without automatic search',
  async (change) => {
    const user = actor(),
      module = {
        ...moduleSummary(),
        modes: ['read', 'read_search'] as ('read' | 'read_search')[],
      };
    api.inspect.mockImplementation(async () => ({
      ...view(),
      mode: 'read_search',
    }));
    const { Wrapper, client } = wrapper();
    const page = render(
      <SharedKnowledgeReader actor={user} module={module} />,
      { wrapper: Wrapper },
    );
    fireEvent.change(screen.getByLabelText('knowledgeAccess.mode'), {
      target: { value: 'read_search' },
    });
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
    );
    fireEvent.change(
      await screen.findByRole('textbox', { name: 'knowledgeAccess.query' }),
      { target: { value: 'SYNTHETIC_OLD_SCOPE_QUERY' } },
    );
    fireEvent.click(screen.getByRole('checkbox'));
    expect(
      screen.getByRole('button', { name: 'knowledgeAccess.applySubset' }),
    ).toBeDisabled();
    const nextUser =
      change === 'module' ? user : { ...user, user: '1'.repeat(32) };
    const nextModule = change === 'module' ? { ...module, version: 2 } : module;
    const late = deferred<AccessInspectData>();
    if (change === 'late_actor') {
      api.inspect.mockReturnValueOnce(late.promise);
      fireEvent.click(
        screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
      );
      await waitFor(() => expect(api.inspect).toHaveBeenCalledTimes(2));
    }
    page.rerender(
      <SharedKnowledgeReader actor={nextUser} module={nextModule} />,
    );
    if (change === 'late_actor')
      await act(async () => {
        late.resolve({ ...view(), mode: 'read_search' });
      });
    expect(
      screen.queryByRole('textbox', { name: 'knowledgeAccess.query' }),
    ).not.toBeInTheDocument();
    expect(
      client.getQueryCache().findAll({
        predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
      }),
    ).toHaveLength(0);
    api.inspect.mockResolvedValueOnce({
      ...view(),
      view_id: '2'.repeat(32),
      mode: 'read_search',
      module_version: nextModule.version,
    });
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
    );
    expect(
      await screen.findByRole('textbox', { name: 'knowledgeAccess.query' }),
    ).toHaveValue('');
    expect(screen.getByRole('checkbox')).toBeChecked();
    expect(
      screen.getByRole('button', { name: 'knowledgeAccess.readFirst' }),
    ).toBeEnabled();
    expect(
      screen.queryByRole('button', { name: 'knowledgeAccess.applySubset' }),
    ).not.toBeInTheDocument();
    expect(api.search).not.toHaveBeenCalled();
  },
);

it('renders a fresh read-only directory and exact ref reread without a search control or call', async () => {
  const { Wrapper } = wrapper();
  render(<SharedKnowledgeReader actor={actor()} module={moduleSummary()} />, {
    wrapper: Wrapper,
  });
  expect(
    screen.queryByRole('form', { name: 'knowledgeAccess.search' }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole('option', { name: 'knowledgeAccess.readSearch' }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.readFirst' }),
  );
  const article = await screen.findByRole('article', {
    name: 'knowledgeAccess.evidence',
  });
  expect(article.textContent).toContain(evidence().text);
  expect(api.read.mock.calls[0][0].selector).toMatchObject({
    kind: 'unit',
    target_index: 0,
    unit_ordinal: 0,
  });
  fireEvent.click(
    within(article).getByRole('button', { name: 'knowledgeAccess.reread' }),
  );
  await waitFor(() => expect(api.read).toHaveBeenCalledTimes(2));
  expect(api.read.mock.calls[1][0].selector).toEqual({
    kind: 'evidence',
    evidence_ref: evidence().evidence_ref,
  });
  expect(api.search).not.toHaveBeenCalled();
  expect(api.inspect.mock.calls[0][0]).toMatchObject({
    mode: 'read',
    delivery: 'web_only',
    source_ids: null,
  });
  expect(
    screen.queryByText(/publication|B_ONLY_NOT_AUTHORIZED/),
  ).not.toBeInTheDocument();
});

it('escapes citation and body text and exposes only bounded typed summaries', async () => {
  const value = evidence();
  value.text = '<img src=x onerror="bad()">';
  value.citation.display_name = '<script>bad()</script>';
  api.read.mockResolvedValueOnce(value);
  const { Wrapper } = wrapper();
  const rendered = render(
    <SharedKnowledgeReader actor={actor()} module={moduleSummary()} />,
    { wrapper: Wrapper },
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.readFirst' }),
  );
  const article = await screen.findByRole('article', {
    name: 'knowledgeAccess.evidence',
  });
  expect(article.textContent).toContain(value.text);
  expect(article.textContent).toContain(value.citation.display_name);
  expect(rendered.container.querySelector('img,script,a[download]')).toBeNull();
  expect(article.textContent).not.toContain(
    value.typed_context.members[0].payload_sha256,
  );
  expect(article.textContent).toContain('knowledgeAccess.emptyMember');
});

it('permission denial during ref reread removes text, catalogue counts and references', async () => {
  const { Wrapper, client } = wrapper();
  render(<SharedKnowledgeReader actor={actor()} module={moduleSummary()} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.readFirst' }),
  );
  await screen.findByRole('article', { name: 'knowledgeAccess.evidence' });
  api.read.mockRejectedValueOnce(
    new AccessRequestError(404, 'not_found', true),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.reread' }),
  );
  await screen.findByRole('alert');
  expect(
    screen.queryByRole('article', { name: 'knowledgeAccess.evidence' }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByText('knowledgeAccess.catalogueCount'),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: 'knowledgeAccess.readFirst' }),
  ).not.toBeInTheDocument();
  expect(
    client.getQueryCache().findAll({
      predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
    }),
  ).toHaveLength(0);
});

it('explicit read_search mode allows scoped search and a hit is read with its exact reference', async () => {
  const catalogue = { ...view(), mode: 'read_search' as const };
  api.inspect.mockResolvedValueOnce(catalogue);
  api.search.mockResolvedValueOnce(searchResult(catalogue));
  const { Wrapper } = wrapper();
  render(
    <SharedKnowledgeReader
      actor={actor()}
      module={{ ...moduleSummary(), modes: ['read', 'read_search'] }}
    />,
    { wrapper: Wrapper },
  );
  fireEvent.change(screen.getByLabelText('knowledgeAccess.mode'), {
    target: { value: 'read_search' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  const form = await screen.findByRole('form', {
    name: 'knowledgeAccess.search',
  });
  fireEvent.change(within(form).getByRole('textbox'), {
    target: { value: '合成A' },
  });
  fireEvent.submit(form);
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.readCitation' }),
  );
  await screen.findByRole('article', { name: 'knowledgeAccess.evidence' });
  expect(api.search.mock.calls[0][0]).toMatchObject({
    view_id: catalogue.view_id,
    query: '合成A',
    limit: 20,
  });
  expect(api.read.mock.calls[0][0].selector).toEqual({
    kind: 'evidence',
    evidence_ref: searchResult(catalogue).hits[0].evidence_ref,
  });
});

it('a changed module version does not render a late response or leave old content in cache', async () => {
  const { Wrapper, client } = wrapper(),
    user = actor(),
    module = moduleSummary();
  const delayed = deferred<AccessReadData>();
  api.read.mockReturnValueOnce(delayed.promise);
  const page = render(<SharedKnowledgeReader actor={user} module={module} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.readFirst' }),
  );
  await waitFor(() => expect(api.read).toHaveBeenCalledTimes(1));
  page.rerender(
    <SharedKnowledgeReader actor={user} module={{ ...module, version: 2 }} />,
  );
  await act(async () => delayed.resolve(evidence()));
  expect(
    screen.queryByRole('article', { name: 'knowledgeAccess.evidence' }),
  ).not.toBeInTheDocument();
  expect(
    client.getQueryCache().findAll({
      predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
    }),
  ).toHaveLength(0);
});

it('the recipient page lists granted modules only and never infers consume authority from owner metadata', async () => {
  native.identity.mockResolvedValue({
    user: User,
    name: 'Synthetic recipient',
    tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
  });
  api.listModules.mockResolvedValue({
    pagination: 'none',
    owned_domain_policies: [],
    items: [
      moduleSummary(),
      {
        ...moduleSummary(),
        module_id: '2'.repeat(32),
        name: 'Owner metadata only',
        administrative_owner: true,
        modes: [],
      },
    ],
  });
  const { Wrapper } = wrapper();
  render(<SharedKnowledgePage />, { wrapper: Wrapper });
  const select = await screen.findByLabelText('knowledgeAccess.module');
  expect(
    within(select).getByRole('option', { name: 'Synthetic shared A' }),
  ).toBeInTheDocument();
  expect(
    within(select).queryByRole('option', { name: 'Owner metadata only' }),
  ).not.toBeInTheDocument();
  expect(api.inspect).not.toHaveBeenCalled();
  fireEvent.change(select, { target: { value: moduleSummary().module_id } });
  expect(
    await screen.findByRole('button', {
      name: 'knowledgeAccess.openCatalogue',
    }),
  ).toBeInTheDocument();
});

it('continues whole-unit reading using the exact next ordinal and no search', async () => {
  const catalogue = view();
  catalogue.targets[0].unit_count = 2;
  catalogue.unit_count = 2;
  const first = evidence();
  first.unit_locator.target_unit_count = 2;
  first.unit_locator.next_unit_ordinal = 1;
  const next = evidence();
  next.unit_locator.target_unit_count = 2;
  next.unit_locator.unit_ordinal = 1;
  api.inspect.mockResolvedValueOnce(catalogue);
  api.read.mockResolvedValueOnce(first).mockResolvedValueOnce(next);
  const { Wrapper } = wrapper();
  render(<SharedKnowledgeReader actor={actor()} module={moduleSummary()} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.readFirst' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeAccess.nextUnit' }),
  );
  await screen.findByRole('article', { name: 'knowledgeAccess.evidence' });
  expect(api.read.mock.calls[1][0].selector).toEqual({
    kind: 'unit',
    view_id: catalogue.view_id,
    target_index: 0,
    unit_ordinal: 1,
  });
  expect(
    screen.queryByRole('button', { name: 'knowledgeAccess.nextUnit' }),
  ).not.toBeInTheDocument();
  expect(api.search).not.toHaveBeenCalled();
});

it.each([false, true])(
  'an explicit narrowed source selection does not silently refill its parent scope (denied=%s)',
  async (denied) => {
    const catalogue = view(),
      second = JSON.parse(JSON.stringify(catalogue.targets[0]));
    second.target.source.source_id = '000000000000000000000000000000ca';
    catalogue.targets.push(second);
    catalogue.source_count = 2;
    catalogue.unit_count = 2;
    api.inspect.mockResolvedValueOnce(catalogue);
    if (denied)
      api.inspect.mockRejectedValueOnce(
        new AccessRequestError(409, 'scope_not_supported', true),
      );
    else
      api.inspect.mockResolvedValueOnce({ ...view(), view_id: '4'.repeat(32) });
    const { Wrapper } = wrapper();
    render(
      <SharedKnowledgeReader
        actor={actor()}
        module={{ ...moduleSummary(), target_count: 2 }}
      />,
      { wrapper: Wrapper },
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
    );
    await screen.findAllByRole('button', { name: 'knowledgeAccess.readFirst' });
    fireEvent.click(screen.getAllByRole('checkbox')[1]);
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeAccess.applySubset' }),
    );
    await waitFor(() => expect(api.inspect).toHaveBeenCalledTimes(2));
    expect(api.inspect.mock.calls[1][0].source_ids).toEqual([
      catalogue.targets[0].target.source.source_id,
    ]);
    if (denied) {
      await screen.findByRole('alert');
      expect(
        screen.queryByText('knowledgeAccess.catalogueCount'),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole('button', { name: 'knowledgeAccess.readFirst' }),
      ).not.toBeInTheDocument();
    } else {
      expect(
        await screen.findByRole('button', {
          name: 'knowledgeAccess.readFirst',
        }),
      ).toBeEnabled();
    }
    expect(api.inspect).toHaveBeenCalledTimes(2);
  },
);

it('a catalogue entry with zero whole units cannot issue a unit-zero read', async () => {
  const catalogue = view();
  catalogue.targets[0].unit_count = 0;
  catalogue.unit_count = 0;
  api.inspect.mockResolvedValueOnce(catalogue);
  const { Wrapper } = wrapper();
  render(<SharedKnowledgeReader actor={actor()} module={moduleSummary()} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
  );
  expect(
    await screen.findByRole('button', { name: 'knowledgeAccess.readFirst' }),
  ).toBeDisabled();
  expect(api.read).not.toHaveBeenCalled();
});

it.each([false, true])(
  'copies only the current own account ID and reports clipboard failure=%s',
  async (failed) => {
    const writeText = jest.fn();
    if (failed) writeText.mockRejectedValue(Error('synthetic copy failure'));
    else writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    native.identity.mockResolvedValue({
      user: User,
      name: 'Synthetic self',
      tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
      get access_token() {
        throw Error('must not read credential');
      },
    } as any);
    api.listModules.mockResolvedValue({
      pagination: 'none',
      owned_domain_policies: [],
      items: [],
    });
    const { Wrapper } = wrapper();
    render(<SharedKnowledgePage />, { wrapper: Wrapper });
    const own = await screen.findByRole('region', {
      name: 'knowledgeAccess.ownAccountID',
    });
    expect(own).toHaveTextContent(User);
    fireEvent.click(
      within(own).getByRole('button', {
        name: 'knowledgeAccess.copyAccountID',
      }),
    );
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(User));
    expect(writeText).toHaveBeenCalledTimes(1);
    if (failed)
      await waitFor(() =>
        expect(notification.error).toHaveBeenCalledWith({
          message: 'knowledgeAccess.copyIDFailed',
        }),
      );
    else
      await waitFor(() =>
        expect(notification.success).toHaveBeenCalledWith({
          message: 'knowledgeAccess.accountIDCopied',
        }),
      );
  },
);

it('fix04 shared identity transition removes both old sibling panels and clears module selection without duplicate keys', async () => {
  const errors = jest.spyOn(console, 'error');
  try {
    native.identity.mockResolvedValue({
      user: User,
      name: 'Synthetic self',
      tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
    });
    api.listModules.mockResolvedValue({
      pagination: 'none',
      owned_domain_policies: [],
      items: [moduleSummary()],
    });
    const { Wrapper, client } = wrapper();
    render(<SharedKnowledgePage />, { wrapper: Wrapper });
    const oldAccount = await screen.findByRole('region', {
      name: 'knowledgeAccess.ownAccountID',
    });
    fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
      target: { value: moduleSummary().module_id },
    });
    await screen.findByRole('button', {
      name: 'knowledgeAccess.openCatalogue',
    });
    const nextUser = '1'.repeat(32);
    act(() =>
      client.setQueryData(
        KnowledgeAccessKeys.identity(nativeSessionRevision()),
        {
          user: nextUser,
          name: 'Synthetic next',
          tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
        },
      ),
    );
    await waitFor(() =>
      expect(
        screen.getAllByRole('region', { name: 'knowledgeAccess.ownAccountID' }),
      ).toHaveLength(1),
    );
    expect(oldAccount).not.toBeInTheDocument();
    expect(
      screen.getByRole('region', { name: 'knowledgeAccess.ownAccountID' }),
    ).toHaveTextContent(nextUser);
    expect(await screen.findByLabelText('knowledgeAccess.module')).toHaveValue(
      '',
    );
    expect(
      screen.queryByRole('button', { name: 'knowledgeAccess.openCatalogue' }),
    ).not.toBeInTheDocument();
    expect(
      errors.mock.calls.filter((call) => String(call[0]).includes('same key')),
    ).toHaveLength(0);
    expect(api.inspect).not.toHaveBeenCalled();
  } finally {
    errors.mockRestore();
  }
});

it.each([
  ['identity', false],
  ['identity', true],
  ['unmount', false],
  ['unmount', true],
] as const)(
  'fix04 pending clipboard after %s ignores late failure=%s',
  async (transition, failed) => {
    const pending = deferred<void>();
    const writeText = jest.fn(() => pending.promise);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    native.identity.mockResolvedValue({
      user: User,
      name: 'Synthetic self',
      tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
      get access_token() {
        throw Error('must not read credential');
      },
    } as any);
    api.listModules.mockResolvedValue({
      pagination: 'none',
      owned_domain_policies: [],
      items: [],
    });
    const { Wrapper, client } = wrapper();
    const page = render(<SharedKnowledgePage />, { wrapper: Wrapper });
    const oldAccount = await screen.findByRole('region', {
      name: 'knowledgeAccess.ownAccountID',
    });
    fireEvent.click(
      within(oldAccount).getByRole('button', {
        name: 'knowledgeAccess.copyAccountID',
      }),
    );
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith(User);
    if (transition === 'identity') {
      act(() =>
        client.setQueryData(
          KnowledgeAccessKeys.identity(nativeSessionRevision()),
          {
            user: '2'.repeat(32),
            name: 'Synthetic next',
            tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
          },
        ),
      );
      await waitFor(() =>
        expect(
          screen
            .getAllByRole('region', { name: 'knowledgeAccess.ownAccountID' })
            .some((node) => node.textContent?.includes('2'.repeat(32))),
        ).toBe(true),
      );
    } else page.unmount();
    await act(async () => {
      if (failed) pending.reject(Error('synthetic late clipboard failure'));
      else pending.resolve();
    });
    expect(notification.success).not.toHaveBeenCalled();
    expect(notification.error).not.toHaveBeenCalled();
    expect(writeText).toHaveBeenCalledTimes(1);
  },
);

import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { useTranslation } from 'react-i18next';
import En from '@/locales/en';
import Zh from '@/locales/zh';
import { KnowledgeBaseProvider } from '../../contexts/knowledge-base-context';
type IDataset = import('@/interfaces/database/dataset').IDataset;
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import nativeService from '@/services/knowledge-access-native-service';
import KnowledgeAccessPage from '../index';
import { AccessGrants } from '../grants';
import { nativeSessionRevision } from '@/utils/native-session-scope';
import { KnowledgeAccessKeys } from '@/hooks/knowledge-access-keys';
import {
  Domain,
  ModuleId,
  Tenant,
  User,
  admin,
  example,
  grant,
  moduleSummary,
  wrapper,
} from '@/services/__tests__/knowledge-access-fixtures';

jest.mock('react-i18next', () => ({
  useTranslation: jest.fn(() => ({ t: (key: string) => key })),
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
      listModules: jest.fn(),
      getModuleAdmin: jest.fn(),
      setPolicy: jest.fn(),
      setModule: jest.fn(),
      createGrant: jest.fn(),
      revokeGrant: jest.fn(),
      inspect: jest.fn(),
      read: jest.fn(),
      search: jest.fn(),
    },
  };
});
jest.mock('@/services/knowledge-access-native-service', () => ({
  __esModule: true,
  default: { identity: jest.fn() },
}));
const api = jest.mocked(service),
  native = jest.mocked(nativeService);
const Owner = '0000000000000000000000000000000b';
beforeAll(() => {
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    configurable: true,
    value: () => require('node:crypto').randomUUID(),
  });
});
function mount(choice = true) {
  const { Wrapper, client } = wrapper();
  const state = choice
    ? {
        accessPublication: {
          domain: Domain,
          scope: example('EX01').scope,
          names: {
            [admin().scope.targets[0].source.source_id]: 'synthetic-A.csv',
          },
        },
      }
    : undefined;
  return {
    client,
    ...render(
      <MemoryRouter
        initialEntries={[
          { pathname: '/dataset/knowledge-access/' + Domain, state },
        ]}
      >
        <KnowledgeBaseProvider
          knowledgeBase={{ tenant_id: Tenant } as IDataset}
          loading={false}
        >
          <Routes>
            <Route
              path="/dataset/knowledge-access/:id"
              element={<KnowledgeAccessPage />}
            />
          </Routes>
        </KnowledgeBaseProvider>
      </MemoryRouter>,
      { wrapper: Wrapper },
    ),
  };
}
beforeEach(() => {
  jest.clearAllMocks();
  jest
    .mocked(useTranslation)
    .mockReturnValue({ t: (key: string) => key } as any);
  sessionStorage.clear();
  native.identity.mockResolvedValue({
    user: Owner,
    name: 'Synthetic owner',
    tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
  });
  api.getModuleAdmin.mockResolvedValue(admin());
  api.listModules.mockResolvedValue({
    pagination: 'none',
    owned_domain_policies: [{ domain_id: Domain, version: 0, enabled: false }],
    items: [{ ...moduleSummary(), administrative_owner: true, modes: [] }],
  });
});

it.each([
  ['zh', 'sharing'],
  ['en', 'sharing'],
  ['zh', 'module'],
  ['en', 'module'],
  ['zh', 'grant'],
  ['en', 'grant'],
])(
  'shows the actual %s delivery-boundary warning inside the existing %s confirmation',
  async (language, action) => {
    const copy = (language === 'zh' ? Zh : En).translation.knowledgeAccess
      .revocationBoundary;
    jest.mocked(useTranslation).mockReturnValue({
      t: (key: string) =>
        key === 'knowledgeAccess.revocationBoundary' ? copy : key,
    } as any);
    if (action === 'sharing')
      api.listModules.mockResolvedValue({
        pagination: 'none',
        owned_domain_policies: [
          { domain_id: Domain, version: 1, enabled: true },
        ],
        items: [{ ...moduleSummary(), administrative_owner: true, modes: [] }],
      });
    mount();
    let scope: HTMLElement;
    if (action === 'sharing') {
      scope = await screen.findByRole('region', {
        name: 'knowledgeAccess.policy',
      });
      expect(within(scope).getByRole('note')).toHaveTextContent(copy);
      expect(within(scope).getAllByRole('checkbox')).toHaveLength(1);
      fireEvent.click(
        within(scope).getByLabelText('knowledgeAccess.confirmPolicy'),
      );
      expect(
        within(scope).getByRole('button', {
          name: 'knowledgeAccess.disablePolicy',
        }),
      ).toBeEnabled();
    } else {
      fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
        target: { value: ModuleId },
      });
      if (action === 'module') {
        scope = await screen.findByRole('form', {
          name: 'knowledgeAccess.editModule',
        });
        expect(within(scope).queryByRole('note')).not.toBeInTheDocument();
        fireEvent.click(
          within(scope).getByLabelText('knowledgeAccess.moduleEnabled'),
        );
        expect(within(scope).getByRole('note')).toHaveTextContent(copy);
        fireEvent.click(
          within(scope).getByLabelText('knowledgeAccess.confirmModule'),
        );
        expect(
          within(scope).getByRole('button', {
            name: 'knowledgeAccess.saveModule',
          }),
        ).toBeEnabled();
      } else {
        scope = await screen.findByRole('listitem');
        expect(within(scope).getByRole('note')).toHaveTextContent(copy);
        expect(within(scope).getAllByRole('checkbox')).toHaveLength(1);
        fireEvent.click(
          within(scope).getByLabelText('knowledgeAccess.confirmRevoke'),
        );
        expect(
          within(scope).getByRole('button', {
            name: 'knowledgeAccess.revokeGrant',
          }),
        ).toBeEnabled();
      }
    }
    expect(copy).toMatch(
      language === 'zh' ? /无法追回.*接收者/ : /cannot be recalled.*recipients/,
    );
    expect(copy).toMatch(language === 'zh' ? /后续请求/ : /later requests/);
    expect(
      screen.getByText('knowledgeAccess.tenantRenewalWarning'),
    ).toBeInTheDocument();
    expect(api.setPolicy).not.toHaveBeenCalled();
    expect(api.setModule).not.toHaveBeenCalled();
    expect(api.revokeGrant).not.toHaveBeenCalled();
  },
);

it('missing policy is visibly off, explicit policy CAS uses zero, and owner metadata never reads content', async () => {
  api.setPolicy.mockImplementation(async (body) => {
    api.listModules.mockResolvedValue({
      pagination: 'none',
      items: [],
      owned_domain_policies: [{ domain_id: Domain, version: 1, enabled: true }],
    });
    return { domain_id: Domain, version: 1, enabled: body.enabled };
  });
  mount();
  await screen.findByText('knowledgeAccess.policyOff');
  const enable = screen.getByRole('button', {
    name: 'knowledgeAccess.enablePolicy',
  });
  expect(enable).toBeDisabled();
  fireEvent.click(screen.getByLabelText('knowledgeAccess.confirmPolicy'));
  fireEvent.click(enable);
  await waitFor(() => expect(api.setPolicy).toHaveBeenCalledTimes(1));
  expect(api.setPolicy.mock.calls[0][0]).toMatchObject({
    expected_version: 0,
    enabled: true,
    confirmed: true,
  });
  expect(api.setPolicy.mock.calls[0][0].idempotency_key).toMatch(
    /^[A-Za-z0-9_-]{16,128}$/,
  );
  await screen.findByText('knowledgeAccess.policyOn');
  expect(api.inspect).not.toHaveBeenCalled();
  expect(api.read).not.toHaveBeenCalled();
  expect(api.search).not.toHaveBeenCalled();
});

it('creates only the exact published target selection and leaves the module disabled by default', async () => {
  api.setModule.mockResolvedValue(admin());
  mount();
  const form = await screen.findByRole('form', {
    name: 'knowledgeAccess.newModule',
  });
  expect(
    within(form).getByLabelText('knowledgeAccess.moduleEnabled'),
  ).not.toBeChecked();
  fireEvent.change(within(form).getByLabelText('knowledgeAccess.moduleName'), {
    target: { value: 'Synthetic fixed A' },
  });
  fireEvent.click(within(form).getByLabelText('knowledgeAccess.confirmModule'));
  fireEvent.submit(form);
  await waitFor(() => expect(api.setModule).toHaveBeenCalledTimes(1));
  expect(api.setModule.mock.calls[0][0]).toMatchObject({
    module_id: null,
    expected_version: 0,
    enabled: false,
    scope: example('EX01').scope,
  });
  expect(api.createGrant).not.toHaveBeenCalled();
  expect(api.setPolicy).not.toHaveBeenCalled();
});

it('a native identity without owner policy receives no management forms', async () => {
  api.listModules.mockResolvedValue({
    pagination: 'none',
    owned_domain_policies: [],
    items: [],
  });
  mount();
  await screen.findByText('knowledgeAccess.ownerOnly');
  expect(
    screen.queryByRole('form', { name: 'knowledgeAccess.newModule' }),
  ).not.toBeInTheDocument();
  expect(api.getModuleAdmin).not.toHaveBeenCalled();
});

it('a normal native domain creator grants an exact account ID through createGrant and explicitly revokes it', async () => {
  api.createGrant.mockResolvedValue(grant());
  api.revokeGrant.mockResolvedValue({
    id: grant().grant_id,
    revoked_at: '2030-01-01T00:00:01Z',
  });
  mount();
  fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
    target: { value: ModuleId },
  });
  const form = await screen.findByRole('form', {
    name: 'knowledgeAccess.createGrant',
  });
  await waitFor(() =>
    expect(
      within(form).getByLabelText('knowledgeAccess.recipientID'),
    ).toBeEnabled(),
  );
  fireEvent.change(within(form).getByLabelText('knowledgeAccess.recipientID'), {
    target: { value: User },
  });
  fireEvent.click(within(form).getByLabelText('knowledgeAccess.confirmGrant'));
  fireEvent.submit(form);
  await waitFor(() => expect(api.createGrant).toHaveBeenCalledTimes(1));
  expect(api.createGrant.mock.calls[0][0]).toMatchObject({
    expected_module_version: 1,
    subject: { kind: 'user', id: User },
    mode: 'read',
    delivery: 'web_only',
  });
  expect(nativeService).not.toHaveProperty('members');
  expect(
    within(form).queryByRole('option', { name: 'Synthetic recipient' }),
  ).not.toBeInTheDocument();
  await waitFor(() =>
    expect(
      screen.getByLabelText('knowledgeAccess.confirmRevoke'),
    ).toBeEnabled(),
  );
  fireEvent.click(screen.getByLabelText('knowledgeAccess.confirmRevoke'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.revokeGrant' }),
  );
  await waitFor(() => expect(api.revokeGrant).toHaveBeenCalledTimes(1));
  expect(api.revokeGrant.mock.calls[0][0]).toMatchObject({
    domain_id: Domain,
    grant_id: grant().grant_id,
    confirmed: true,
  });
});

it('a known policy conflict requires explicit latest-state review and never retries an updated CAS automatically', async () => {
  api.setPolicy.mockImplementation(async () => {
    api.listModules.mockResolvedValue({
      pagination: 'none',
      items: [],
      owned_domain_policies: [
        { domain_id: Domain, version: 5, enabled: false },
      ],
    });
    throw new AccessRequestError(409, 'version_conflict', true);
  });
  mount();
  await screen.findByText('knowledgeAccess.policyOff');
  fireEvent.click(screen.getByLabelText('knowledgeAccess.confirmPolicy'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.enablePolicy' }),
  );
  await screen.findByRole('button', { name: 'knowledgeAccess.reviewLatest' });
  expect(
    screen.getByRole('button', { name: 'knowledgeAccess.enablePolicy' }),
  ).toBeDisabled();
  expect(api.setPolicy).toHaveBeenCalledTimes(1);
  expect(api.setPolicy.mock.calls[0][0].expected_version).toBe(0);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.reviewLatest' }),
  );
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'knowledgeAccess.reviewLatest' }),
    ).not.toBeInTheDocument(),
  );
  expect(api.setPolicy).toHaveBeenCalledTimes(1);
});

it.each([-1, 31])(
  'a grant expiry %s days away is rejected locally without a write',
  async (days) => {
    mount();
    fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
      target: { value: ModuleId },
    });
    const form = await screen.findByRole('form', {
      name: 'knowledgeAccess.createGrant',
    });
    await waitFor(() =>
      expect(
        within(form).getByLabelText('knowledgeAccess.recipientID'),
      ).toBeEnabled(),
    );
    fireEvent.change(
      within(form).getByLabelText('knowledgeAccess.recipientID'),
      {
        target: { value: User },
      },
    );
    fireEvent.change(within(form).getByLabelText('knowledgeAccess.expiry'), {
      target: {
        value: new Date(Date.now() + days * 86400000)
          .toISOString()
          .slice(0, 16),
      },
    });
    fireEvent.click(
      within(form).getByLabelText('knowledgeAccess.confirmGrant'),
    );
    fireEvent.submit(form);
    await screen.findByText('knowledgeAccess.invalidExpiry');
    expect(api.createGrant).not.toHaveBeenCalled();
  },
);

it.each([
  '',
  'not-an-id',
  'A'.repeat(32),
  '1'.repeat(31),
  '1'.repeat(33),
  ' ' + User,
])('rejects invalid recipient ID %p without a grant request', async (value) => {
  mount();
  fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
    target: { value: ModuleId },
  });
  const form = await screen.findByRole('form', {
    name: 'knowledgeAccess.createGrant',
  });
  fireEvent.change(within(form).getByLabelText('knowledgeAccess.recipientID'), {
    target: { value },
  });
  fireEvent.submit(form);
  expect(api.createGrant).not.toHaveBeenCalled();
  expect(
    within(form).getByRole('button', { name: 'knowledgeAccess.createGrant' }),
  ).toBeDisabled();
});

it('shows the exact grant review and changing the recipient clears consent before read_search creation', async () => {
  api.createGrant.mockResolvedValue({ ...grant(), mode: 'read_search' });
  mount();
  fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
    target: { value: ModuleId },
  });
  const form = await screen.findByRole('form', {
    name: 'knowledgeAccess.createGrant',
  });
  const input = within(form).getByLabelText('knowledgeAccess.recipientID');
  fireEvent.change(input, { target: { value: '1'.repeat(32) } });
  fireEvent.click(within(form).getByLabelText('knowledgeAccess.confirmGrant'));
  fireEvent.change(input, { target: { value: User } });
  expect(
    within(form).getByLabelText('knowledgeAccess.confirmGrant'),
  ).not.toBeChecked();
  fireEvent.change(within(form).getByLabelText('knowledgeAccess.mode'), {
    target: { value: 'read_search' },
  });
  const review = within(form).getByRole('group', {
    name: 'knowledgeAccess.grantReview',
  });
  expect(review).toHaveTextContent(User);
  expect(review).toHaveTextContent('knowledgeAccess.grantScope');
  expect(review).toHaveTextContent('knowledgeAccess.readSearch');
  expect(review).toHaveTextContent('knowledgeAccess.webOnly');
  fireEvent.click(within(form).getByLabelText('knowledgeAccess.confirmGrant'));
  fireEvent.submit(form);
  await waitFor(() => expect(api.createGrant).toHaveBeenCalledTimes(1));
  expect(api.createGrant.mock.calls[0][0]).toMatchObject({
    domain_id: Domain,
    module_id: ModuleId,
    expected_module_version: 1,
    subject: { kind: 'user', id: User },
    mode: 'read_search',
    delivery: 'web_only',
  });
});

it.each(['user', 'tenant', 'session', 'domain', 'module', 'version'])(
  'clears recipient input and confirmation on %s scope change',
  (scope) => {
    const user = {
      user: Owner,
      tenant: Tenant,
      session: nativeSessionRevision(),
      auth: 'native' as const,
    };
    const module = admin();
    const command = { locked: false, start: jest.fn() } as any;
    const page = render(
      <AccessGrants actor={user} module={module} command={command} />,
    );
    fireEvent.change(screen.getByLabelText('knowledgeAccess.recipientID'), {
      target: { value: User },
    });
    fireEvent.click(screen.getByLabelText('knowledgeAccess.confirmGrant'));
    expect(screen.getByLabelText('knowledgeAccess.confirmGrant')).toBeChecked();
    const nextUser = {
      ...user,
      ...(scope === 'user'
        ? { user: '1'.repeat(32) }
        : scope === 'tenant'
          ? { tenant: '2'.repeat(32) }
          : scope === 'session'
            ? { session: user.session + 1 }
            : {}),
    };
    const nextModule = {
      ...module,
      ...(scope === 'domain'
        ? { domain_id: '3'.repeat(32) }
        : scope === 'module'
          ? { module_id: '4'.repeat(32) }
          : scope === 'version'
            ? { version: 2 }
            : {}),
    };
    page.rerender(
      <AccessGrants actor={nextUser} module={nextModule} command={command} />,
    );
    expect(screen.getByLabelText('knowledgeAccess.recipientID')).toHaveValue(
      '',
    );
    expect(
      screen.getByLabelText('knowledgeAccess.confirmGrant'),
    ).not.toBeChecked();
    fireEvent.submit(
      screen.getByRole('form', { name: 'knowledgeAccess.createGrant' }),
    );
    expect(command.start).not.toHaveBeenCalled();
  },
);

it('fix04 owner module version transition unmounts both old siblings and clears edit and grant consent without duplicate keys', async () => {
  const errors = jest.spyOn(console, 'error');
  try {
    const { client } = mount();
    fireEvent.change(await screen.findByLabelText('knowledgeAccess.module'), {
      target: { value: ModuleId },
    });
    const oldEditor = await screen.findByRole('form', {
      name: 'knowledgeAccess.editModule',
    });
    const oldGrant = await screen.findByRole('form', {
      name: 'knowledgeAccess.createGrant',
    });
    fireEvent.change(
      within(oldEditor).getByLabelText('knowledgeAccess.moduleName'),
      { target: { value: 'SYNTHETIC_OLD_EDIT' } },
    );
    fireEvent.click(
      within(oldEditor).getByLabelText('knowledgeAccess.confirmModule'),
    );
    fireEvent.change(
      within(oldGrant).getByLabelText('knowledgeAccess.recipientID'),
      { target: { value: User } },
    );
    fireEvent.click(
      within(oldGrant).getByLabelText('knowledgeAccess.confirmGrant'),
    );
    act(() =>
      client.setQueryData(
        KnowledgeAccessKeys.admin(
          {
            user: Owner,
            tenant: Tenant,
            session: nativeSessionRevision(),
            auth: 'native',
          },
          Domain,
          ModuleId,
        ),
        { ...admin(), version: 2 },
      ),
    );
    await waitFor(() =>
      expect(
        screen.getAllByRole('form', { name: 'knowledgeAccess.editModule' }),
      ).toHaveLength(1),
    );
    expect(oldEditor).not.toBeInTheDocument();
    expect(oldGrant).not.toBeInTheDocument();
    const editor = screen.getByRole('form', {
      name: 'knowledgeAccess.editModule',
    });
    expect(
      within(editor).getByLabelText('knowledgeAccess.moduleName'),
    ).not.toHaveValue('SYNTHETIC_OLD_EDIT');
    expect(
      within(editor).getByLabelText('knowledgeAccess.confirmModule'),
    ).not.toBeChecked();
    const grantForm = screen.getByRole('form', {
      name: 'knowledgeAccess.createGrant',
    });
    expect(
      within(grantForm).getByLabelText('knowledgeAccess.recipientID'),
    ).toHaveValue('');
    expect(
      within(grantForm).getByLabelText('knowledgeAccess.confirmGrant'),
    ).not.toBeChecked();
    expect(
      errors.mock.calls.filter((call) => String(call[0]).includes('same key')),
    ).toHaveLength(0);
    expect(api.setModule).not.toHaveBeenCalled();
    expect(api.createGrant).not.toHaveBeenCalled();
  } finally {
    errors.mockRestore();
  }
});

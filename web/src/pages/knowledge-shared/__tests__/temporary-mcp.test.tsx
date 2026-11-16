import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import {
  knowledgeAccessMCPEndpoint,
  MintMCPCredential,
  OwnMCPCredentials,
} from '../temporary-mcp';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import {
  actor,
  Domain,
  ModuleId,
  moduleSummary,
  view,
  wrapper,
} from '@/services/__tests__/knowledge-access-fixtures';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils/register-server', () => ({
  registerNextServer: () => ({ post: jest.fn() }),
}));
jest.mock('@/services/knowledge-access-native-service', () => ({
  __esModule: true,
  default: { identity: jest.fn() },
}));
jest.mock('@/services/knowledge-access-service', () => ({
  __esModule: true,
  ...jest.requireActual('@/services/knowledge-access-service'),
  default: {
    inspect: jest.fn(),
    createCredential: jest.fn(),
    revokeCredential: jest.fn(),
    listCredentials: jest.fn(),
  },
}));
const api = jest.mocked(service),
  copy = jest.fn();
const token = () => 'kac1_' + '6'.repeat(32) + '.' + 'A'.repeat(43);
const metadata = () => ({
  credential_id: '6'.repeat(32),
  domain_id: Domain,
  module_id: ModuleId,
  module_version: 1,
  mode: 'read' as const,
  client_label: 'Synthetic SDK',
  issued_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 3600000).toISOString(),
  revoked_at: null,
  last_used_at: null,
});
const moduleValue = () => ({ ...moduleSummary(), mcp_delivery_allowed: true });
beforeEach(() => {
  jest.clearAllMocks();
  sessionStorage.clear();
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: copy.mockResolvedValue(undefined) },
  });
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    configurable: true,
    value: () => require('node:crypto').randomUUID(),
  });
  api.inspect.mockImplementation(async () => ({
    ...view(),
    delivery: 'web_and_mcp',
  }));
  api.createCredential.mockImplementation(async () => ({
    credential: metadata(),
    token: token(),
    delivery: 'issued_once',
  }));
});
async function mint() {
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.prepareMCP' }),
  );
  await screen.findByRole('group', {
    name: 'knowledgeAccess.credentialReview',
  });
  fireEvent.change(screen.getByLabelText('knowledgeAccess.clientLabel'), {
    target: { value: 'Synthetic SDK' },
  });
  fireEvent.click(screen.getByLabelText('knowledgeAccess.confirmCredential'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.createCredential' }),
  );
}

it('reviews a native MCP view and reveals/copies the first secret only on explicit actions', async () => {
  const { Wrapper, client } = wrapper();
  render(<MintMCPCredential actor={actor()} module={moduleValue()} />, {
    wrapper: Wrapper,
  });
  expect(screen.getByLabelText('knowledgeAccess.credentialTTL')).toHaveValue(
    3600,
  );
  await mint();
  await screen.findByRole('button', { name: 'knowledgeAccess.copyToken' });
  expect(api.inspect).toHaveBeenCalledWith(
    expect.objectContaining({
      domain_id: Domain,
      module_id: ModuleId,
      delivery: 'web_and_mcp',
      source_ids: null,
    }),
    expect.any(AbortSignal),
  );
  expect(api.createCredential).toHaveBeenCalledWith(
    expect.objectContaining({
      ttl_seconds: 3600,
      client_label: 'Synthetic SDK',
      confirmed: true,
    }),
  );
  expect(screen.queryByTestId('one-time-token')).toBeNull();
  expect(copy).not.toHaveBeenCalled();
  expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(
    token(),
  );
  expect(JSON.stringify({ ...sessionStorage })).not.toContain(token());
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.copyToken' }),
  );
  await waitFor(() => expect(copy).toHaveBeenCalledTimes(1));
  expect(copy).toHaveBeenCalledWith(token());
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.showToken' }),
  );
  expect(screen.getByTestId('one-time-token')).toHaveTextContent(token());
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.closeToken' }),
  );
  expect(screen.queryByTestId('one-time-token')).toBeNull();
  expect(
    screen.queryByRole('button', { name: 'knowledgeAccess.copyToken' }),
  ).toBeNull();
  expect(
    screen.getByText('knowledgeAccess.tokenUnavailable'),
  ).toBeInTheDocument();
});
it('requires a fresh scope review after a source subset change and removes any first token', async () => {
  const { Wrapper } = wrapper();
  render(<MintMCPCredential actor={actor()} module={moduleValue()} />, {
    wrapper: Wrapper,
  });
  await mint();
  await screen.findByRole('button', { name: 'knowledgeAccess.copyToken' });
  const source = screen.getByRole('checkbox', { name: /^[a-f0-9]{32} \/ / });
  fireEvent.click(source);
  expect(
    screen.queryByRole('group', { name: 'knowledgeAccess.credentialReview' }),
  ).toBeNull();
  expect(
    screen.queryByRole('button', { name: 'knowledgeAccess.copyToken' }),
  ).toBeNull();
  expect(
    screen.getByRole('button', { name: 'knowledgeAccess.prepareMCP' }),
  ).toBeDisabled();
  fireEvent.click(source);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.prepareMCP' }),
  );
  await screen.findByRole('group', {
    name: 'knowledgeAccess.credentialReview',
  });
  expect(api.inspect.mock.calls[1][0].source_ids).toEqual([
    view().targets[0].target.source.source_id,
  ]);
  expect(
    screen.getByLabelText('knowledgeAccess.confirmCredential'),
  ).not.toBeChecked();
});
it('resolves a lost first reply with its original request and displays metadata-only recovery', async () => {
  const { Wrapper } = wrapper();
  api.createCredential
    .mockRejectedValueOnce(new AccessRequestError(0, 'unavailable', null, true))
    .mockResolvedValueOnce({
      credential: metadata(),
      token: null,
      delivery: 'unavailable_after_first_response',
    });
  render(<MintMCPCredential actor={actor()} module={moduleValue()} />, {
    wrapper: Wrapper,
  });
  await mint();
  await screen.findByRole('button', { name: 'knowledgeAccess.checkOriginal' });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.checkOriginal' }),
  );
  await screen.findByText('knowledgeAccess.tokenUnavailable');
  expect(api.createCredential.mock.calls[0][0]).toEqual(
    api.createCredential.mock.calls[1][0],
  );
  expect(copy).not.toHaveBeenCalled();
  expect(
    screen.queryByRole('button', { name: 'knowledgeAccess.showToken' }),
  ).toBeNull();
});
it('drops first token and view references on actor change', async () => {
  const user = actor(),
    { Wrapper } = wrapper(),
    page = render(<MintMCPCredential actor={user} module={moduleValue()} />, {
      wrapper: Wrapper,
    });
  await mint();
  await screen.findByRole('button', { name: 'knowledgeAccess.showToken' });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.showToken' }),
  );
  page.rerender(
    <MintMCPCredential
      actor={{ ...user, user: '7'.repeat(32) }}
      module={moduleValue()}
    />,
  );
  expect(screen.queryByTestId('one-time-token')).toBeNull();
  expect(
    screen.queryByRole('group', { name: 'knowledgeAccess.credentialReview' }),
  ).toBeNull();
  expect(
    screen.getByRole('button', { name: 'knowledgeAccess.createCredential' }),
  ).toBeDisabled();
});
it('expires its native scope before a later mint without a background credential write', async () => {
  const { Wrapper } = wrapper();
  api.inspect.mockImplementationOnce(async () => ({
    ...view(),
    delivery: 'web_and_mcp',
    expires_at: new Date(Date.now() + 600).toISOString(),
  }));
  render(<MintMCPCredential actor={actor()} module={moduleValue()} />, {
    wrapper: Wrapper,
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.prepareMCP' }),
  );
  await screen.findByRole('group', {
    name: 'knowledgeAccess.credentialReview',
  });
  await waitFor(
    () =>
      expect(
        screen.queryByRole('group', {
          name: 'knowledgeAccess.credentialReview',
        }),
      ).toBeNull(),
    { timeout: 2000 },
  );
  expect(api.createCredential).not.toHaveBeenCalled();
});
it('keeps safe self metadata/revoke available without any content module or inspect', async () => {
  const { Wrapper } = wrapper(),
    item = metadata();
  api.listCredentials.mockResolvedValueOnce({
    items: [item],
    pagination: 'none',
  });
  api.revokeCredential.mockResolvedValueOnce({
    id: item.credential_id,
    revoked_at: new Date().toISOString(),
  });
  render(<OwnMCPCredentials actor={actor()} />, { wrapper: Wrapper });
  fireEvent.change(screen.getByLabelText('knowledgeAccess.credentialDomain'), {
    target: { value: Domain },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.listCredentials' }),
  );
  await screen.findByRole('button', {
    name: 'knowledgeAccess.revokeCredential',
  });
  expect(api.inspect).not.toHaveBeenCalled();
  expect(api.listCredentials).toHaveBeenCalledWith(
    expect.objectContaining({ domain_id: Domain, idempotency_key: null }),
    expect.any(AbortSignal),
  );
  expect(screen.queryByTestId('one-time-token')).toBeNull();
  fireEvent.click(
    screen.getByLabelText('knowledgeAccess.confirmRevokeCredential'),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeAccess.revokeCredential' }),
  );
  await waitFor(() =>
    expect(api.revokeCredential).toHaveBeenCalledWith(
      expect.objectContaining({
        credential_id: item.credential_id,
        confirmed: true,
      }),
    ),
  );
  expect(api.createCredential).not.toHaveBeenCalled();
});
it('does not offer minting without an explicit MCP delivery grant', () => {
  const { Wrapper } = wrapper();
  render(<MintMCPCredential actor={actor()} module={moduleSummary()} />, {
    wrapper: Wrapper,
  });
  expect(
    screen.queryByRole('button', { name: 'knowledgeAccess.createCredential' }),
  ).toBeNull();
  expect(api.inspect).not.toHaveBeenCalled();
});
it('shows only a fixed same-origin HTTPS or loopback endpoint and has no URL/token export', () => {
  for (const origin of [
    'http://localhost',
    'http://127.0.0.1:29398',
    'http://[::1]:29398',
    'https://knowledge.example',
  ])
    expect(knowledgeAccessMCPEndpoint(origin)).toBe(
      origin + '/api/v1/knowledge/access/mcp',
    );
  for (const origin of [
    'http://knowledge.example',
    'https://user:password@knowledge.example',
    'https://knowledge.example/?query=1',
    'https://knowledge.example/#fragment',
    'https://knowledge.example/path',
    'https://KNOWLEDGE.EXAMPLE',
  ])
    expect(knowledgeAccessMCPEndpoint(origin)).toBeNull();
});

type ReactNode = import('react').ReactNode;
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import request from '@/utils/next-request';
import { getAuthorization } from '@/utils/authorization-util';
import {
  useKnowledgeDomainEntry,
  useRootProjectCreate,
} from '../use-knowledge-foundation-entry';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from '../use-knowledge-foundation';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'synthetic-entry'),
}));
const Http = request as unknown as jest.Mock;
const Owner = 'b'.repeat(32),
  Id = 'a'.repeat(32),
  GroupId = 'c'.repeat(32);
const Native = {
  id: Id,
  name: 'Synthetic',
  created_by: Owner,
  permission: 'me',
};
const Domain = {
  domain_id: Id,
  name: 'Synthetic',
  owner_id: Owner,
  category: 'project',
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
const Group = {
  id: GroupId,
  domain_id: Id,
  parent_id: null,
  kind: 'project',
  name: 'Same name',
  component_side: 'shared',
};
const envelope = (data: unknown) => ({
  code: 0,
  data,
  message: 'OK',
  contract_version: '1.0.0',
  request_id: 'synthetic',
});
const respond = (data: unknown, status = 200) => ({
  status,
  data: envelope(data),
});
function environment() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidate = jest.spyOn(client, 'invalidateQueries');
  return {
    client,
    invalidate,
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  };
}
function successful(config: { method: string; url: string }) {
  if (config.method === 'post' && config.url === '/api/v1/datasets')
    return respond(Native);
  if (config.method === 'post') return respond(Group, 201);
  return respond(Domain);
}
beforeEach(() => {
  Http.mockReset().mockImplementation(successful);
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-entry');
});

it('creates a private native KB without a model field, registers that ID, confirms by GET and refreshes exact scopes', async () => {
  const env = environment(),
    refresh = jest.fn(async () => undefined);
  const { result } = renderHook(() => useKnowledgeDomainEntry(Owner, refresh), {
    wrapper: env.wrapper,
  });
  const epoch = getKnowledgeSessionScope();
  await act(async () => {
    await result.current.create(' Synthetic ', 'project');
  });
  expect(Http.mock.calls.map(([config]) => config.method)).toEqual([
    'post',
    'put',
    'get',
  ]);
  expect(Http.mock.calls[0][0]).toMatchObject({
    url: '/api/v1/datasets',
    data: {
      name: 'Synthetic',
      permission: 'me',
      parse_type: 1,
      parser_id: 'general',
    },
  });
  expect(Object.keys(Http.mock.calls[0][0].data).sort()).toEqual([
    'name',
    'parse_type',
    'parser_id',
    'permission',
  ]);
  expect(Http.mock.calls[1][0]).toMatchObject({
    url: `/api/v1/datasets/${Id}/knowledge/domain`,
    data: { category: 'project' },
    headers: { 'X-Knowledge-Contract-Version': '1.0.0' },
  });
  expect(result.current.receipt).toEqual(Domain);
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(env.invalidate).toHaveBeenCalledWith({
    queryKey: KnowledgeKeys.domains(epoch),
    exact: true,
  });
  expect(env.invalidate).toHaveBeenCalledWith({
    queryKey: KnowledgeKeys.domain(epoch, Id),
    exact: true,
  });
});

it('uses a synchronous mutex so double-clicking cannot issue two native POSTs', async () => {
  let release!: (value: unknown) => void;
  Http.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const { result } = renderHook(
    () => useKnowledgeDomainEntry(Owner, async () => undefined),
    { wrapper: environment().wrapper },
  );
  let pending!: Promise<unknown>;
  act(() => {
    pending = result.current.create('Synthetic', 'project');
    void result.current.create('Synthetic', 'project');
  });
  expect(Http).toHaveBeenCalledTimes(1);
  await act(async () => {
    release(respond(Native));
    await pending;
  });
  expect(
    Http.mock.calls.filter(([config]) => config.method === 'post'),
  ).toHaveLength(1);
  expect(result.current.receipt).toEqual(Domain);
});

it('does not repeat an unknown native POST; read-only checking and explicit real-ID selection recover', async () => {
  Http.mockRejectedValueOnce(new Error('synthetic lost response'));
  const refresh = jest.fn(async () => undefined);
  const { result } = renderHook(() => useKnowledgeDomainEntry(Owner, refresh), {
    wrapper: environment().wrapper,
  });
  await act(async () => {
    await result.current.create('Synthetic', 'project');
  });
  expect(result.current.issue?.kind).toBe('unknown');
  expect(result.current.attempt?.datasetId).toBeNull();
  await act(async () => {
    await result.current.clearCompleted();
  });
  expect(result.current.isLocked()).toBe(true);
  await act(async () => {
    await result.current.create('Synthetic', 'project');
    await result.current.check();
  });
  expect(Http).toHaveBeenCalledTimes(1);
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(result.current.locked).toBe(true);
  await act(async () => {
    await result.current.registerExisting(Id, 'custom');
  });
  expect(
    Http.mock.calls.filter(([config]) => config.method === 'post'),
  ).toHaveLength(1);
  expect(Http.mock.calls[1][0].data).toEqual({ category: 'project' });
  expect(result.current.receipt).toEqual(Domain);
});

it('retains the KB ID after unknown PUT and recovers only by GET then explicit same-category PUT', async () => {
  let puts = 0,
    gets = 0;
  Http.mockImplementation((config) => {
    if (config.method === 'put' && puts++ === 0)
      return Promise.reject(new Error('synthetic lost PUT'));
    if (config.method === 'get' && gets++ === 0)
      return Promise.reject({
        response: { status: 404, data: { code: 'knowledge_not_found' } },
      });
    return successful(config);
  });
  const { result } = renderHook(
    () => useKnowledgeDomainEntry(Owner, async () => undefined),
    { wrapper: environment().wrapper },
  );
  await act(async () => {
    await result.current.create('Synthetic', 'project');
  });
  expect(result.current.attempt?.datasetId).toBe(Id);
  expect(result.current.receipt).toBeNull();
  await act(async () => {
    await result.current.retryRegistration();
  });
  expect(puts).toBe(1);
  await act(async () => {
    await result.current.check();
  });
  expect(result.current.canRetry).toBe(true);
  expect(result.current.isLocked()).toBe(true);
  await act(async () => {
    await result.current.retryRegistration();
  });
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(1);
  expect(
    Http.mock.calls
      .filter(([c]) => c.method === 'put')
      .map(([c]) => [c.url, c.data]),
  ).toEqual(
    Array(2).fill([
      `/api/v1/datasets/${Id}/knowledge/domain`,
      { category: 'project' },
    ]),
  );
  expect(result.current.receipt).toEqual(Domain);
});

it.each([
  ['accepted without completion', { status: 202, data: envelope(Native) }],
  ['wrong native owner', respond({ ...Native, created_by: 'other-owner' })],
  ['missing native ID', respond({ ...Native, id: undefined })],
])('does not continue native creation for %s', async (_name, response) => {
  Http.mockResolvedValueOnce(response);
  const { result } = renderHook(
    () => useKnowledgeDomainEntry(Owner, async () => undefined),
    { wrapper: environment().wrapper },
  );
  await act(async () => {
    await result.current.create('Synthetic', 'project');
  });
  expect(result.current.issue?.kind).toBe('unknown');
  expect(result.current.receipt).toBeNull();
  expect(Http).toHaveBeenCalledTimes(1);
});

it('shows explicit registration rejection without losing the native ID or changing category', async () => {
  Http.mockImplementation((config) =>
    config.method === 'put'
      ? Promise.reject({
          response: {
            status: 409,
            data: {
              code: 'knowledge_domain_not_empty',
              request_id: 'synthetic-rejection',
            },
          },
        })
      : successful(config),
  );
  const { result } = renderHook(
    () => useKnowledgeDomainEntry(Owner, async () => undefined),
    { wrapper: environment().wrapper },
  );
  await act(async () => {
    await result.current.create('Synthetic', 'project');
  });
  expect(result.current.issue).toMatchObject({
    kind: 'failure',
    code: 'knowledge_domain_not_empty',
  });
  expect(result.current.attempt?.datasetId).toBe(Id);
  await act(async () => {
    await result.current.create('Another', 'personal');
  });
  expect(Http).toHaveBeenCalledTimes(2);
});

it.each(['contract', 'owner', 'policy'])(
  'does not mark a domain ready from an invalid %s receipt',
  async (variant) => {
    const data =
      variant === 'owner'
        ? { ...Domain, owner_id: 'other' }
        : variant === 'policy'
          ? { ...Domain, policy: { ...Domain.policy, external_models: true } }
          : Domain;
    Http.mockImplementation((config) =>
      config.method === 'put'
        ? {
            status: 200,
            data: {
              ...envelope(data),
              contract_version: variant === 'contract' ? '2.0.0' : '1.0.0',
            },
          }
        : successful(config),
    );
    const { result } = renderHook(
      () => useKnowledgeDomainEntry(Owner, async () => undefined),
      { wrapper: environment().wrapper },
    );
    await act(async () => {
      await result.current.create('Synthetic', 'project');
    });
    expect(result.current.receipt).toBeNull();
    expect(result.current.issue?.kind).toBe('unknown');
  },
);

it.each(['owner', 'session', 'unmount'])(
  'suppresses native late results after %s change',
  async (change) => {
    let release!: (value: unknown) => void;
    Http.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    const env = environment(),
      refresh = jest.fn(async () => undefined);
    const { result, rerender, unmount } = renderHook(
      ({ owner }) => useKnowledgeDomainEntry(owner, refresh),
      { wrapper: env.wrapper, initialProps: { owner: Owner } },
    );
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.create('Synthetic', 'project');
    });
    if (change === 'owner') rerender({ owner: 'new-owner' });
    if (change === 'session')
      (getAuthorization as jest.Mock).mockReturnValue('new-synthetic-session');
    if (change === 'unmount') unmount();
    await act(async () => {
      release(respond(Native));
      await pending;
    });
    expect(Http).toHaveBeenCalledTimes(1);
    expect(refresh).not.toHaveBeenCalled();
    expect(env.invalidate).not.toHaveBeenCalled();
  },
);

it('creates only a root shared project and refreshes its exact group list, then waits for explicit selection', async () => {
  const env = environment(),
    selected = jest.fn();
  const { result } = renderHook(() => useRootProjectCreate(Owner, Id), {
    wrapper: env.wrapper,
  });
  await act(async () => {
    await result.current.create(' Same name ', 'shared');
  });
  expect(Http.mock.calls[1][0]).toMatchObject({
    method: 'post',
    data: {
      parent_id: null,
      kind: 'project',
      name: 'Same name',
      component_side: 'shared',
    },
  });
  expect(env.invalidate).toHaveBeenCalledWith({
    queryKey: KnowledgeKeys.groups(getKnowledgeSessionScope(), Id, undefined),
    exact: true,
  });
  expect(selected).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.select(selected);
  });
  expect(selected).toHaveBeenCalledWith(Group);
});

it('reconciles an unknown group POST by exact real ID across same-name paginated results without another POST', async () => {
  const other = { ...Group, id: 'd'.repeat(32) };
  Http.mockImplementation((config) => {
    if (config.method === 'post')
      return Promise.reject(new Error('synthetic group timeout'));
    if (config.url.endsWith('/groups'))
      return respond(
        config.params.cursor
          ? { items: [Group], next_cursor: '' }
          : { items: [other], next_cursor: 'opaque-next' },
      );
    return respond(Domain);
  });
  const { result } = renderHook(() => useRootProjectCreate(Owner, Id), {
    wrapper: environment().wrapper,
  });
  await act(async () => {
    await result.current.create('Same name', 'shared');
  });
  expect(result.current.issue?.kind).toBe('unknown');
  await act(async () => {
    await result.current.create('Same name', 'shared');
    await result.current.reconcile(GroupId);
  });
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(1);
  expect(result.current.receipt?.id).toBe(GroupId);
  expect(result.current.origin).toBe('existing');
});

it('does not accept a malformed 201 group or install a late group into a different domain', async () => {
  let release!: (value: unknown) => void;
  Http.mockImplementation((config) =>
    config.method === 'post'
      ? new Promise((resolve) => {
          release = resolve;
        })
      : respond(Domain),
  );
  const env = environment();
  const { result, rerender } = renderHook(
    ({ domain }) => useRootProjectCreate(Owner, domain),
    { wrapper: env.wrapper, initialProps: { domain: Id } },
  );
  let pending!: Promise<unknown>;
  await act(async () => {
    pending = result.current.create('Same name', 'shared');
    await Promise.resolve();
  });
  rerender({ domain: 'e'.repeat(32) });
  await act(async () => {
    release(respond(Group, 201));
    await pending;
  });
  expect(result.current.receipt).toBeNull();
  expect(env.invalidate).not.toHaveBeenCalled();
  Http.mockImplementation((config) =>
    config.method === 'post'
      ? respond({ ...Group, parent_id: GroupId }, 201)
      : respond(Domain),
  );
  rerender({ domain: Id });
  await act(async () => {
    await result.current.create('Same name', 'shared');
  });
  expect(result.current.receipt).toBeNull();
  expect(result.current.issue?.kind).toBe('unknown');
});

it('checks the native UTF-8 byte limit before any write', async () => {
  const { result } = renderHook(
    () => useKnowledgeDomainEntry(Owner, async () => undefined),
    { wrapper: environment().wrapper },
  );
  await act(async () => {
    await result.current.create('脑'.repeat(43), 'project');
  });
  expect(Http).not.toHaveBeenCalled();
  expect(result.current.issue?.kind).toBe('failure');
});

it('rechecks parent permission after the domain read before issuing a group POST', async () => {
  let allowed = true;
  let release!: (value: unknown) => void;
  Http.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const { result } = renderHook(
    () => useRootProjectCreate(Owner, Id, () => allowed),
    { wrapper: environment().wrapper },
  );
  let pending!: Promise<unknown>;
  act(() => {
    pending = result.current.create('Same name', 'shared');
  });
  allowed = false;
  await act(async () => {
    release(respond(Domain));
    await pending;
  });
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(0);
  expect(result.current.receipt).toBeNull();
});

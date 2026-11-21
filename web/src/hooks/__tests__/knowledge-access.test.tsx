type AccessInspectData =
  import('@/interfaces/database/knowledge-access').AccessInspectData;
type AccessReadData =
  import('@/interfaces/database/knowledge-access').AccessReadData;
import { act, renderHook, waitFor } from '@testing-library/react';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import {
  clearSharedAccessContent,
  useAccessModules,
  useSharedAccessReader,
} from '../use-knowledge-access';
import { KnowledgeAccessKeys } from '../knowledge-access-keys';
import { notifyNativeSessionChanged } from '@/utils/native-session-scope';
import authorization from '@/utils/authorization-util';
import {
  actor,
  deferred,
  evidence,
  moduleSummary,
  view,
  wrapper,
} from '@/services/__tests__/knowledge-access-fixtures';

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
const api = jest.mocked(service);
beforeEach(() => {
  jest.clearAllMocks();
  api.inspect.mockResolvedValue(view());
  api.read.mockResolvedValue(evidence());
});
afterEach(() => jest.useRealTimers());

it('a fresh read-only reader opens unit zero without search and rereads the exact returned ref', async () => {
  const { client, Wrapper } = wrapper(),
    user = actor(),
    module = moduleSummary();
  const hook = renderHook(() => useSharedAccessReader(user, module), {
    wrapper: Wrapper,
  });
  await act(async () => {
    await hook.result.current.inspect('read');
  });
  await act(async () => {
    await hook.result.current.read({
      kind: 'unit',
      view_id: hook.result.current.view!.view_id,
      target_index: 0,
      unit_ordinal: 0,
    });
  });
  expect(hook.result.current.evidence).toEqual(evidence());
  expect(api.read.mock.calls[0][0].selector).toMatchObject({
    kind: 'unit',
    target_index: 0,
    unit_ordinal: 0,
  });
  await act(async () => {
    await hook.result.current.reread(
      hook.result.current.evidence!.evidence_ref,
    );
  });
  expect(api.read.mock.calls[1][0].selector).toEqual({
    kind: 'evidence',
    evidence_ref: evidence().evidence_ref,
  });
  await act(async () => {
    await hook.result.current.search('合成A');
  });
  expect(api.search).not.toHaveBeenCalled();
  hook.unmount();
  expect(
    client.getQueryCache().findAll({
      predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
    }),
  ).toHaveLength(0);
});

it.each(['user', 'tenant', 'version', 'disabled', 'permission'])(
  'clears text, counts, refs and QueryClient on %s change and refuses a late response',
  async (change) => {
    const { client, Wrapper } = wrapper();
    const initial = { user: actor(), module: moduleSummary() };
    const hook = renderHook(
      ({ user, module }) => useSharedAccessReader(user, module),
      { wrapper: Wrapper, initialProps: initial },
    );
    await act(async () => {
      await hook.result.current.inspect('read');
    });
    const delayed = deferred<AccessReadData>();
    api.read.mockReturnValueOnce(delayed.promise);
    let reading!: Promise<void>;
    act(() => {
      reading = hook.result.current.read({
        kind: 'unit',
        view_id: hook.result.current.view!.view_id,
        target_index: 0,
        unit_ordinal: 0,
      });
    });
    const next = { user: { ...initial.user }, module: { ...initial.module } };
    if (change === 'user') next.user.user = '1'.repeat(32);
    if (change === 'tenant') next.user.tenant = '2'.repeat(32);
    if (change === 'version') next.module.version = 2;
    if (change === 'disabled') next.module.enabled = false;
    if (change === 'permission') next.module.modes = [];
    hook.rerender(next);
    expect(hook.result.current.view).toBeNull();
    expect(hook.result.current.evidence).toBeNull();
    await act(async () => {
      delayed.resolve(evidence());
      await reading;
    });
    expect(hook.result.current.evidence).toBeNull();
    expect(hook.result.current.results).toBeNull();
    expect(
      client.getQueryCache().findAll({
        predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
      }),
    ).toHaveLength(0);
    hook.unmount();
  },
);

it.each([401, 404, 409])(
  'denial %s discards the complete view and never leaves a partial response',
  async (status) => {
    const { client, Wrapper } = wrapper();
    const hook = renderHook(
      () => useSharedAccessReader(actor(), moduleSummary()),
      { wrapper: Wrapper },
    );
    await act(async () => {
      await hook.result.current.inspect('read');
    });
    await act(async () => {
      await hook.result.current.read({
        kind: 'unit',
        view_id: hook.result.current.view!.view_id,
        target_index: 0,
        unit_ordinal: 0,
      });
    });
    api.read.mockRejectedValueOnce(
      new AccessRequestError(status, 'not_found', true),
    );
    await act(async () => {
      await hook.result.current.reread(evidence().evidence_ref);
    });
    expect(hook.result.current.view).toBeNull();
    expect(hook.result.current.evidence).toBeNull();
    expect(
      client.getQueryCache().findAll({
        predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
      }),
    ).toHaveLength(0);
    hook.unmount();
  },
);

it('expires at the exact view boundary and requires another explicit inspect', async () => {
  jest.useFakeTimers();
  const catalogue = view();
  catalogue.expires_at = new Date(Date.now() + 5000).toISOString();
  api.inspect.mockResolvedValueOnce(catalogue);
  const { Wrapper, client } = wrapper();
  const hook = renderHook(
    () => useSharedAccessReader(actor(), moduleSummary()),
    { wrapper: Wrapper },
  );
  await act(async () => {
    await hook.result.current.inspect('read');
  });
  await act(async () => {
    await hook.result.current.read({
      kind: 'unit',
      view_id: catalogue.view_id,
      target_index: 0,
      unit_ordinal: 0,
    });
  });
  act(() => jest.advanceTimersByTime(5000));
  expect(hook.result.current.view).toBeNull();
  expect(hook.result.current.evidence).toBeNull();
  expect(api.inspect).toHaveBeenCalledTimes(1);
  expect(
    client.getQueryCache().findAll({
      predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
    }),
  ).toHaveLength(0);
  hook.unmount();
});

it('native session changes synchronously invalidate output and late inspect results', async () => {
  const { Wrapper, client } = wrapper();
  const user = actor();
  const delayed = deferred<AccessInspectData>();
  api.inspect.mockReturnValueOnce(delayed.promise);
  const hook = renderHook(() => useSharedAccessReader(user, moduleSummary()), {
    wrapper: Wrapper,
  });
  let inspecting!: Promise<void>;
  act(() => {
    inspecting = hook.result.current.inspect('read');
  });
  act(() => notifyNativeSessionChanged());
  await act(async () => {
    delayed.resolve(view());
    await inspecting;
  });
  expect(hook.result.current.view).toBeNull();
  expect(hook.result.current.evidence).toBeNull();
  expect(
    client.getQueryCache().findAll({
      predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
    }),
  ).toHaveLength(0);
  hook.unmount();
});

it('a coarse permission invalidation removes a still-mounted reader and prevents stale cache release', async () => {
  const { Wrapper, client } = wrapper();
  const hook = renderHook(
    () => useSharedAccessReader(actor(), moduleSummary()),
    { wrapper: Wrapper },
  );
  await act(async () => {
    await hook.result.current.inspect('read');
  });
  await act(async () => {
    await hook.result.current.read({
      kind: 'unit',
      view_id: hook.result.current.view!.view_id,
      target_index: 0,
      unit_ordinal: 0,
    });
  });
  act(() => clearSharedAccessContent(client));
  expect(hook.result.current.view).toBeNull();
  expect(hook.result.current.evidence).toBeNull();
  hook.unmount();
});

it('late module metadata from an old native session cannot enter its query cache', async () => {
  const { Wrapper, client } = wrapper();
  const user = actor();
  const delayed = deferred<any>();
  api.listModules.mockReturnValueOnce(delayed.promise);
  const hook = renderHook(() => useAccessModules(user, null), {
    wrapper: Wrapper,
  });
  await waitFor(() => expect(api.listModules).toHaveBeenCalledTimes(1));
  act(() => notifyNativeSessionChanged());
  await act(async () => {
    delayed.resolve({
      items: [moduleSummary()],
      pagination: 'none',
      owned_domain_policies: [],
    });
  });
  await waitFor(() => expect(hook.result.current.isError).toBe(true));
  expect(
    client.getQueryData(KnowledgeAccessKeys.modules(user, null)),
  ).toBeUndefined();
  hook.unmount();
});

it.each(['logout', 'token replacement'])(
  'the existing native %s mutation invalidates the shared reader',
  async (change) => {
    const { Wrapper, client } = wrapper(),
      user = actor();
    const hook = renderHook(
      () => useSharedAccessReader(user, moduleSummary()),
      { wrapper: Wrapper },
    );
    await act(async () => {
      await hook.result.current.inspect('read');
    });
    await act(async () => {
      await hook.result.current.read({
        kind: 'unit',
        view_id: hook.result.current.view!.view_id,
        target_index: 0,
        unit_ordinal: 0,
      });
    });
    act(() => {
      if (change === 'logout') authorization.removeAll();
      else
        authorization.saveLoginCredentials({
          userInfo: JSON.stringify(authorization.getUserInfoObject()),
          authorization: 'Bearer synthetic-session-only',
          token: 'synthetic-session-only',
        });
    });
    expect(hook.result.current.view).toBeNull();
    expect(hook.result.current.evidence).toBeNull();
    expect(
      client.getQueryCache().findAll({
        predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
      }),
    ).toHaveLength(0);
    hook.unmount();
    authorization.removeAll();
  },
);

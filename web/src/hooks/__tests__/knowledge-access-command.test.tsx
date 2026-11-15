type AccessCommand = import('../use-knowledge-access-command').AccessCommand;
import { act, renderHook } from '@testing-library/react';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import { KnowledgeAccessVersion } from '@/interfaces/database/knowledge-access';
import {
  useAccessCommand,
  accessCommandKey,
} from '../use-knowledge-access-command';
import { KnowledgeAccessKeys } from '../knowledge-access-keys';
import { notifyNativeSessionChanged } from '@/utils/native-session-scope';
import {
  Domain,
  User,
  actor,
  admin,
  deferred,
  example,
  grant,
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
      setPolicy: jest.fn(),
      setModule: jest.fn(),
      createGrant: jest.fn(),
      revokeGrant: jest.fn(),
    },
  };
});
jest.mock('@/services/knowledge-access-native-service', () => ({
  __esModule: true,
  default: { identity: jest.fn() },
}));
const api = jest.mocked(service);
const policy = (): AccessCommand => ({
  operation: 'setPolicy',
  body: {
    contract_version: KnowledgeAccessVersion,
    domain_id: Domain,
    expected_version: 0,
    enabled: true,
    confirmed: true,
    idempotency_key: 'synthetic-policy-key-0001',
  },
});
function command(operation: string): AccessCommand {
  if (operation === 'setModule') return { operation, body: example('EX01') };
  if (operation === 'createGrant')
    return {
      operation,
      body: {
        ...example('EX10'),
        subject: { kind: 'user', id: User },
        delivery: 'web_only',
      },
    };
  if (operation === 'revokeGrant')
    return {
      operation,
      body: {
        contract_version: KnowledgeAccessVersion,
        domain_id: Domain,
        grant_id: grant().grant_id,
        confirmed: true,
        idempotency_key: 'synthetic-revoke-key-0001',
      },
    };
  return policy();
}
beforeEach(() => {
  jest.clearAllMocks();
  sessionStorage.clear();
});

it.each(['setPolicy', 'setModule', 'createGrant', 'revokeGrant'] as const)(
  '%s preserves its immutable body/key across lost replies, remount and exact replay',
  async (operation) => {
    const user = actor(),
      original = command(operation),
      { Wrapper, client } = wrapper();
    const invoke = api[operation] as jest.Mock;
    invoke.mockRejectedValueOnce(
      new AccessRequestError(503, 'unavailable', false, true),
    );
    let hook = renderHook(() => useAccessCommand(user, Domain), {
      wrapper: Wrapper,
    });
    await act(async () => {
      await hook.result.current.start(original);
    });
    expect(hook.result.current.pending).toEqual(original);
    expect(Object.isFrozen(hook.result.current.pending?.body)).toBe(true);
    expect(hook.result.current.locked).toBe(true);
    await act(async () => {
      await hook.result.current.start({
        ...original,
        body: { ...original.body, idempotency_key: 'different-new-key-0001' },
      } as AccessCommand);
    });
    expect(invoke).toHaveBeenCalledTimes(1);
    hook.unmount();
    hook = renderHook(() => useAccessCommand(user, Domain), {
      wrapper: Wrapper,
    });
    expect(hook.result.current.pending).toEqual(original);
    client.setQueryData(KnowledgeAccessKeys.view(user, 'x', 1, 'v'), view());
    invoke.mockResolvedValueOnce(
      operation === 'setModule'
        ? admin()
        : operation === 'createGrant'
          ? grant()
          : {},
    );
    await act(async () => {
      await hook.result.current.recover();
    });
    expect(invoke.mock.calls[1][0]).toEqual(invoke.mock.calls[0][0]);
    expect(hook.result.current.pending).toBeNull();
    expect(hook.result.current.completed).toBe(true);
    expect(sessionStorage.getItem(accessCommandKey(user, Domain))).toBeNull();
    expect(
      client.getQueryCache().findAll({
        predicate: (query) => KnowledgeAccessKeys.isConsumer(query.queryKey),
      }),
    ).toHaveLength(0);
    hook.unmount();
  },
);

it('a known CAS conflict reloads metadata and requires explicit review before a new request', async () => {
  const user = actor(),
    { Wrapper, client } = wrapper();
  const invalidate = jest.spyOn(client, 'invalidateQueries');
  api.setPolicy.mockRejectedValueOnce(
    new AccessRequestError(409, 'version_conflict', true, false),
  );
  const hook = renderHook(() => useAccessCommand(user, Domain), {
    wrapper: Wrapper,
  });
  await act(async () => {
    await hook.result.current.start(policy());
  });
  expect(hook.result.current.pending).toBeNull();
  expect(hook.result.current.needsReview).toBe(true);
  expect(invalidate).toHaveBeenCalledWith({
    queryKey: KnowledgeAccessKeys.actor(user),
  });
  expect(api.setPolicy.mock.calls[0][0].expected_version).toBe(0);
  await act(async () => {
    await hook.result.current.start(policy());
  });
  expect(api.setPolicy).toHaveBeenCalledTimes(1);
  await act(async () => {
    await hook.result.current.reviewLatest();
  });
  expect(hook.result.current.locked).toBe(false);
  expect(api.setPolicy).toHaveBeenCalledTimes(1);
  hook.unmount();
});

it('late completion under another identity cannot clear the old receipt or render in the new actor', async () => {
  const initial = actor(),
    next = { ...initial, user: '1'.repeat(32) },
    { Wrapper } = wrapper();
  const delayed = deferred<any>();
  api.setPolicy.mockReturnValueOnce(delayed.promise);
  const hook = renderHook(({ user }) => useAccessCommand(user, Domain), {
    wrapper: Wrapper,
    initialProps: { user: initial },
  });
  let saving!: Promise<boolean>;
  act(() => {
    saving = hook.result.current.start(policy());
  });
  hook.rerender({ user: next });
  await act(async () => {
    delayed.resolve({ domain_id: Domain, version: 1, enabled: true });
    await saving;
  });
  expect(hook.result.current.completed).toBe(false);
  expect(hook.result.current.pending).toBeNull();
  expect(
    sessionStorage.getItem(accessCommandKey(initial, Domain)),
  ).not.toBeNull();
  expect(sessionStorage.getItem(accessCommandKey(next, Domain))).toBeNull();
  hook.unmount();
});

it('native session loss retains the original command for explicit recovery by the same verified user', async () => {
  const user = actor(),
    { Wrapper } = wrapper(),
    delayed = deferred<any>();
  api.setPolicy.mockReturnValueOnce(delayed.promise);
  const hook = renderHook(() => useAccessCommand(user, Domain), {
    wrapper: Wrapper,
  });
  let saving!: Promise<boolean>;
  act(() => {
    saving = hook.result.current.start(policy());
  });
  act(() => notifyNativeSessionChanged());
  await act(async () => {
    delayed.resolve({});
    await saving;
  });
  expect(sessionStorage.getItem(accessCommandKey(user, Domain))).not.toBeNull();
  expect(hook.result.current.completed).toBe(false);
  hook.unmount();
});

it('corrupt pending storage fails closed without a new key or request', async () => {
  const user = actor(),
    { Wrapper } = wrapper();
  sessionStorage.setItem(accessCommandKey(user, Domain), '{');
  const hook = renderHook(() => useAccessCommand(user, Domain), {
    wrapper: Wrapper,
  });
  expect(hook.result.current.storageBlocked).toBe(true);
  await act(async () => {
    await hook.result.current.start(policy());
    await hook.result.current.recover();
  });
  expect(api.setPolicy).not.toHaveBeenCalled();
  hook.unmount();
});

it('a changed pending slot cannot delete or replay a different command', async () => {
  const user = actor(),
    { Wrapper } = wrapper();
  api.setPolicy.mockRejectedValueOnce(
    new AccessRequestError(503, 'unavailable', false, true),
  );
  const hook = renderHook(() => useAccessCommand(user, Domain), {
    wrapper: Wrapper,
  });
  await act(async () => {
    await hook.result.current.start(policy());
  });
  const replacement = policy();
  replacement.body.idempotency_key = 'replacement-command-key-0001';
  sessionStorage.setItem(
    accessCommandKey(user, Domain),
    JSON.stringify(replacement),
  );
  await act(async () => {
    await hook.result.current.recover();
  });
  expect(api.setPolicy).toHaveBeenCalledTimes(1);
  expect(hook.result.current.storageBlocked).toBe(true);
  expect(
    JSON.parse(sessionStorage.getItem(accessCommandKey(user, Domain))!),
  ).toEqual(replacement);
  hook.unmount();
});

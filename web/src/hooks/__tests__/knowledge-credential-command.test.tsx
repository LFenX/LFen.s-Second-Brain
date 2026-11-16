import { act, renderHook } from '@testing-library/react';
type CredentialCommand =
  import('../use-knowledge-credential-command').CredentialCommand;
import {
  credentialCommandKey,
  useCredentialCommand,
} from '../use-knowledge-credential-command';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import { KnowledgeAccessVersion } from '@/interfaces/database/knowledge-access';
import { KnowledgeAccessKeys } from '../knowledge-access-keys';
import { notifyNativeSessionChanged } from '@/utils/native-session-scope';
import {
  actor,
  deferred,
  Domain,
  ModuleId,
  view,
  wrapper,
} from '@/services/__tests__/knowledge-access-fixtures';

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
  default: { createCredential: jest.fn(), revokeCredential: jest.fn() },
}));
const api = jest.mocked(service);
const token = () => 'kac1_' + '6'.repeat(32) + '.' + 'A'.repeat(43);
const metadata = () => ({
  credential_id: '6'.repeat(32),
  domain_id: Domain,
  module_id: ModuleId,
  module_version: 1,
  mode: 'read' as const,
  client_label: 'Synthetic SDK',
  issued_at: '2030-01-01T00:00:00Z',
  expires_at: '2030-01-01T01:00:00Z',
  revoked_at: null,
  last_used_at: null,
});
const command = (): CredentialCommand => ({
  operation: 'createCredential',
  scope: { domain: Domain, module: ModuleId, version: 1 },
  body: {
    contract_version: KnowledgeAccessVersion,
    view_id: view().view_id,
    mode: 'read',
    ttl_seconds: null,
    client_label: 'Synthetic SDK',
    confirmed: true,
    idempotency_key: 'Synthetic_Credential_Hook01',
  },
});
beforeEach(() => {
  jest.clearAllMocks();
  sessionStorage.clear();
});

it('delivers only the first token directly, keeping storage, hook state and query caches safe', async () => {
  const user = actor(),
    deliver = jest.fn(),
    { Wrapper, client } = wrapper(),
    value = {
      credential: metadata(),
      token: token(),
      delivery: 'issued_once' as const,
    };
  api.createCredential.mockResolvedValueOnce(value);
  const hook = renderHook(() => useCredentialCommand(user, Domain, deliver), {
    wrapper: Wrapper,
  });
  await act(async () => {
    expect(await hook.result.current.start(command())).toBe(true);
  });
  expect(deliver).toHaveBeenCalledTimes(1);
  expect(deliver).toHaveBeenCalledWith(token(), metadata());
  expect(value.token).toBeNull();
  expect(hook.result.current.receipt).toEqual(metadata());
  expect(hook.result.current.delivery).toBe('issued_once');
  expect(JSON.stringify(hook.result.current)).not.toContain(token());
  expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(
    token(),
  );
  expect(sessionStorage.getItem(credentialCommandKey(user, Domain))).toBeNull();
  expect(client.getMutationCache().getAll()).toHaveLength(0);
});
it('resolves a lost mint with the exact saved body/key after remount and never recovers its token', async () => {
  const user = actor(),
    original = command(),
    deliver = jest.fn(),
    { Wrapper } = wrapper();
  api.createCredential.mockRejectedValueOnce(
    new AccessRequestError(503, 'unavailable', false, true),
  );
  let hook = renderHook(() => useCredentialCommand(user, Domain, deliver), {
    wrapper: Wrapper,
  });
  await act(async () => {
    expect(await hook.result.current.start(original)).toBe(false);
  });
  expect(hook.result.current.pending).toEqual(original);
  expect(
    sessionStorage.getItem(credentialCommandKey(user, Domain)),
  ).not.toContain(token());
  await act(async () => {
    expect(
      await hook.result.current.start({
        ...original,
        body: { ...original.body, idempotency_key: 'Synthetic_Blind_New_Key' },
      } as CredentialCommand),
    ).toBe(false);
  });
  expect(api.createCredential).toHaveBeenCalledTimes(1);
  hook.unmount();
  api.createCredential.mockResolvedValueOnce({
    credential: metadata(),
    token: null,
    delivery: 'unavailable_after_first_response',
  });
  hook = renderHook(() => useCredentialCommand(user, Domain, deliver), {
    wrapper: Wrapper,
  });
  await act(async () => {
    expect(await hook.result.current.recover()).toBe(true);
  });
  expect(api.createCredential.mock.calls[1][0]).toEqual(original.body);
  expect(deliver).not.toHaveBeenCalled();
  expect(hook.result.current.delivery).toBe('unavailable_after_first_response');
  expect(hook.result.current.pending).toBeNull();
});
it.each(['unmount', 'native session change'])(
  'does not deliver a late first token after %s',
  async (change) => {
    const user = actor(),
      deliver = jest.fn(),
      { Wrapper } = wrapper(),
      late = deferred<any>();
    api.createCredential.mockReturnValueOnce(late.promise);
    const hook = renderHook(() => useCredentialCommand(user, Domain, deliver), {
      wrapper: Wrapper,
    });
    let run: Promise<boolean>;
    act(() => {
      run = hook.result.current.start(command());
    });
    if (change === 'unmount') hook.unmount();
    else act(() => notifyNativeSessionChanged());
    const value = {
      credential: metadata(),
      token: token(),
      delivery: 'issued_once',
    };
    await act(async () => {
      late.resolve(value);
      await run;
    });
    expect(deliver).not.toHaveBeenCalled();
    expect(value.token).toBeNull();
    expect(
      sessionStorage.getItem(credentialCommandKey(user, Domain)),
    ).not.toBeNull();
  },
);
it('keeps a mismatched mint scope unresolved and never delivers the rejected token', async () => {
  const user = actor(),
    deliver = jest.fn(),
    { Wrapper } = wrapper();
  api.createCredential.mockResolvedValueOnce({
    credential: { ...metadata(), module_id: '7'.repeat(32) },
    token: token(),
    delivery: 'issued_once',
  });
  const hook = renderHook(() => useCredentialCommand(user, Domain, deliver), {
    wrapper: Wrapper,
  });
  await act(async () => {
    expect(await hook.result.current.start(command())).toBe(false);
  });
  expect(deliver).not.toHaveBeenCalled();
  expect(hook.result.current.pending).toEqual(command());
  expect(hook.result.current.receipt).toBeNull();
  expect(hook.result.current.locked).toBe(true);
});
it('clears shared content for known authorization loss and requires explicit review', async () => {
  const user = actor(),
    deliver = jest.fn(),
    { Wrapper, client } = wrapper();
  client.setQueryData(
    KnowledgeAccessKeys.view(user, ModuleId, 1, view().view_id),
    view(),
  );
  api.createCredential.mockRejectedValueOnce(
    new AccessRequestError(409, 'scope_changed', true),
  );
  const hook = renderHook(() => useCredentialCommand(user, Domain, deliver), {
    wrapper: Wrapper,
  });
  await act(async () => {
    await hook.result.current.start(command());
  });
  expect(
    client.getQueryCache().findAll({
      predicate: (q) => KnowledgeAccessKeys.isConsumer(q.queryKey),
    }),
  ).toHaveLength(0);
  expect(hook.result.current.pending).toBeNull();
  expect(hook.result.current.needsReview).toBe(true);
  expect(hook.result.current.locked).toBe(true);
  expect(deliver).not.toHaveBeenCalled();
});
it('fails closed before a write if safe command storage is unavailable', async () => {
  const user = actor(),
    { Wrapper } = wrapper(),
    write = jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage unavailable');
    });
  try {
    const hook = renderHook(
      () => useCredentialCommand(user, Domain, jest.fn()),
      { wrapper: Wrapper },
    );
    await act(async () => {
      expect(await hook.result.current.start(command())).toBe(false);
    });
    expect(api.createCredential).not.toHaveBeenCalled();
    expect(hook.result.current.blocked).toBe(true);
  } finally {
    write.mockRestore();
  }
});
it('retries an unknown revoke using one exact credential ID and command key', async () => {
  const user = actor(),
    { Wrapper } = wrapper(),
    original: CredentialCommand = {
      operation: 'revokeCredential',
      scope: { domain: Domain, module: ModuleId, version: 1 },
      body: {
        contract_version: KnowledgeAccessVersion,
        credential_id: metadata().credential_id,
        confirmed: true,
        idempotency_key: 'Synthetic_Credential_Revoke_Hook01',
      },
    };
  api.revokeCredential
    .mockRejectedValueOnce(new AccessRequestError(0, 'unavailable', null, true))
    .mockResolvedValueOnce({
      id: metadata().credential_id,
      revoked_at: '2030-01-01T00:30:00Z',
    });
  const hook = renderHook(() => useCredentialCommand(user, Domain, jest.fn()), {
    wrapper: Wrapper,
  });
  await act(async () => {
    await hook.result.current.start(original);
  });
  await act(async () => {
    expect(await hook.result.current.recover()).toBe(true);
  });
  expect(api.revokeCredential.mock.calls[0][0]).toEqual(
    api.revokeCredential.mock.calls[1][0],
  );
  expect(hook.result.current.pending).toBeNull();
});

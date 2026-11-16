import nativeService from '../knowledge-access-native-service';
import userService, { listTenant, listTenantUser } from '../user-service';
import { Tenant, User, deferred } from './knowledge-access-fixtures';
jest.mock('@/utils/register-server', () => ({
  registerNextServer: () => ({ post: jest.fn() }),
}));
jest.mock('../user-service', () => ({
  __esModule: true,
  default: { userInfo: jest.fn() },
  listTenant: jest.fn(),
  listTenantUser: jest.fn(),
}));
const identity = {
  user: User,
  name: 'Synthetic self',
  tenants: [{ id: Tenant, name: 'Synthetic team', role: 'normal' }],
};
beforeEach(() => jest.clearAllMocks());
it('uses only current native profile ID and team display fields without requesting a member roster or reading credentials', async () => {
  const user = {
    id: User,
    nickname: 'Synthetic self',
    get access_token() {
      throw Error('must not read secret');
    },
    get password() {
      throw Error('must not read password');
    },
  };
  jest
    .mocked(userService.userInfo)
    .mockResolvedValue({ data: { code: 0, data: user } });
  jest.mocked(listTenant).mockResolvedValue({
    data: {
      code: 0,
      data: [
        {
          tenant_id: Tenant,
          nickname: 'Synthetic team',
          role: 'normal',
          email: 'ignored@example.invalid',
          llm_id: 'PRIVATE_MODEL_FIELD',
        },
      ],
    },
  } as any);
  await expect(nativeService.identity()).resolves.toEqual(identity);
  expect(listTenantUser).not.toHaveBeenCalled();
});
it('rejects an aborted or late identity response', async () => {
  const signal = new AbortController(),
    result = deferred<any>();
  jest.mocked(userService.userInfo).mockReturnValueOnce(result.promise);
  jest
    .mocked(listTenant)
    .mockResolvedValueOnce({ data: { code: 0, data: [] } } as any);
  const reading = nativeService.identity(signal.signal);
  signal.abort();
  result.resolve({ data: { code: 0, data: { id: User } } });
  await expect(reading).rejects.toMatchObject({ code: 'scope_changed' });
  expect(listTenantUser).not.toHaveBeenCalled();
});

it('uses the exact native tenant roster and projects only active member labels', async () => {
  const member = (
    userID: string,
    role = 'normal',
    active = '1',
    flags: Partial<{
      is_authenticated: unknown;
      is_anonymous: unknown;
      status: string;
    }> = {},
  ) => ({
    user_id: userID,
    nickname: 'Synthetic member',
    role,
    status: '1',
    is_active: active,
    is_authenticated: true,
    is_anonymous: false,
    ...flags,
    get password() {
      throw Error('secret accessed');
    },
    get access_token() {
      throw Error('secret accessed');
    },
  });
  const rosterResponse = {
    data: {
      code: 0,
      data: [
        member('b'.repeat(32)),
        member('c'.repeat(32), 'invite'),
        member('d'.repeat(32), 'normal', '0'),
        member('e'.repeat(32), 'normal', '1', { is_authenticated: false }),
        member('f'.repeat(32), 'normal', '1', { is_anonymous: true }),
        member('1'.repeat(32), 'normal', '1', { status: '0' }),
        member('2'.repeat(32), 'normal', '1', {
          is_authenticated: '1',
          is_anonymous: '0',
        }),
        member('3'.repeat(32), 'admin'),
      ],
    },
  } as any;
  jest.mocked(listTenantUser).mockResolvedValueOnce(rosterResponse);
  await expect(
    nativeService.members({ user: User, tenant: Tenant }),
  ).resolves.toEqual([
    { id: User, name: '' },
    { id: 'b'.repeat(32), name: 'Synthetic member' },
    { id: '3'.repeat(32), name: 'Synthetic member' },
  ]);
  expect(listTenantUser).toHaveBeenCalledWith(Tenant);
});
it('discards a late native roster after cancellation', async () => {
  const pending = deferred<any>(),
    abort = new AbortController();
  jest.mocked(listTenantUser).mockReturnValueOnce(pending.promise);
  const request = nativeService.members(
    { user: User, tenant: Tenant },
    abort.signal,
  );
  abort.abort();
  pending.resolve({ data: { code: 0, data: [] } });
  await expect(request).rejects.toMatchObject({ code: 'scope_changed' });
});

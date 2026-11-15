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

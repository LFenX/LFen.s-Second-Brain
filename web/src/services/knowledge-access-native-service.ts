import userService, { listTenant } from './user-service';
import type { ITenant } from '@/interfaces/database/user-setting';
import { AccessRequestError } from './knowledge-access-service';

export interface AccessIdentity {
  user: string;
  name: string;
  tenants: { id: string; name: string; role: string }[];
}
const id = /^[a-f0-9]{32}$/;
function check(signal?: AbortSignal) {
  if (signal?.aborted) throw new AccessRequestError(0, 'scope_changed', true);
}
const nativeService = {
  async identity(signal?: AbortSignal): Promise<AccessIdentity> {
    check(signal);
    const [userResponse, tenantResponse] = await Promise.all([
      userService.userInfo(),
      listTenant(),
    ]);
    check(signal);
    const user = userResponse.data;
    const tenants = tenantResponse.data;
    if (
      user?.code !== 0 ||
      tenants?.code !== 0 ||
      !id.test(user.data?.id) ||
      !Array.isArray(tenants.data)
    )
      throw new AccessRequestError(401, 'unauthenticated', true);
    const rows = (tenants.data as ITenant[]).filter((item) =>
      id.test(item.tenant_id),
    );
    return {
      user: user.data.id,
      name: String(user.data.nickname || '当前用户'),
      tenants: rows.map((item) => ({
        id: item.tenant_id,
        name: item.nickname || '当前团队',
        role: item.role,
      })),
    };
  },
};
export default nativeService;

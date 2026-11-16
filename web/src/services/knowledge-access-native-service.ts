import userService, { listTenant, listTenantUser } from './user-service';
import type { ITenant, ITenantUser } from '@/interfaces/database/user-setting';
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
  async members(
    actor: { user: string; tenant: string },
    signal?: AbortSignal,
  ): Promise<{ id: string; name: string }[]> {
    check(signal);
    if (!id.test(actor.user) || !id.test(actor.tenant))
      throw new AccessRequestError(400, 'invalid_request', true);
    const response = await listTenantUser(actor.tenant);
    check(signal);
    if (response.data?.code !== 0 || !Array.isArray(response.data.data))
      throw new AccessRequestError(0, 'unavailable', true);
    const values = (response.data.data as ITenantUser[]).filter(
      (item) =>
        id.test(item.user_id) &&
        ['owner', 'admin', 'normal'].includes(item.role) &&
        item.status === '1' &&
        item.is_active === '1' &&
        item.is_authenticated === '1' &&
        item.is_anonymous === '0',
    );
    const members = values
      .filter((item) => item.user_id !== actor.user)
      .map((item) => ({
        id: item.user_id,
        name: typeof item.nickname === 'string' ? item.nickname : '',
      }));
    if (new Set(members.map((item) => item.id)).size !== members.length)
      throw new AccessRequestError(502, 'invalid_response', true);
    return [{ id: actor.user, name: '' }, ...members];
  },
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

import { AxiosError } from 'axios';
import request from '@/utils/next-request';
import service from '../knowledge-lifecycle-service';
import authorization, { redirectToLogin } from '@/utils/authorization-util';
import {
  Scope,
  detail,
  command,
  receipt,
  response,
  failure,
} from '@/pages/dataset/knowledge-catalogue/__tests__/lifecycle-fixture';
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));
jest.mock('@/utils/notification', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { removeAll: jest.fn() },
  getAuthorization: () => 'synthetic-lifecycle-token',
  redirectToLogin: jest.fn(),
}));
it('keeps exact original JSON and empty classification fields through real Axios interceptors with native authentication', async () => {
  const value = detail(),
    input = { ...command(value), idempotency_key: 'AbC_001-Z' },
    result = await receipt(value, 'archive', input),
    previous = request.defaults.adapter,
    adapter = jest.fn(async (config) => ({
      ...response(result, 201),
      statusText: 'Synthetic',
      config,
    }));
  request.defaults.adapter = adapter;
  try {
    await service.archive(Scope, input);
    const config = adapter.mock.calls[0][0];
    expect(JSON.parse(config.data)).toEqual(input);
    expect(config.headers.get('Authorization')).toBe(
      'synthetic-lifecycle-token',
    );
    expect(config.headers.get('X-Knowledge-Source-Contract-Version')).toBe(
      '0.7.0',
    );
    expect(config.timeout).toBe(25000);
    expect(config.data).not.toContain('tenant_id');
  } finally {
    request.defaults.adapter = previous;
  }
});
it('does not clear authentication for a known409 or503; native401 retains the existing cleanup', async () => {
  const previous = request.defaults.adapter;
  let status = 409;
  request.defaults.adapter = async (config) => {
    const result = {
      headers: {},
      ...failure(
        status,
        status === 409
          ? 'revision_changed'
          : status === 401
            ? 'knowledge_unauthorized'
            : 'lifecycle_result_unknown',
        status === 409,
      ).response,
      statusText: 'Synthetic',
      config,
    };
    throw new AxiosError(
      'Synthetic',
      'ERR_BAD_RESPONSE',
      config,
      undefined,
      result,
    );
  };
  try {
    await expect(service.detail(Scope)).rejects.toMatchObject({
      status: 409,
      outcomeKnown: true,
    });
    status = 503;
    await expect(service.detail(Scope)).rejects.toMatchObject({
      status: 503,
      outcomeKnown: false,
    });
    expect(authorization.removeAll).not.toHaveBeenCalled();
    status = 401;
    await expect(service.detail(Scope)).rejects.toMatchObject({ status: 401 });
    expect(authorization.removeAll).toHaveBeenCalledTimes(1);
    expect(redirectToLogin).toHaveBeenCalledTimes(1);
  } finally {
    request.defaults.adapter = previous;
  }
});

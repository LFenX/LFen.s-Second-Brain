import { AxiosError } from 'axios';
import request from '@/utils/next-request';
import service from '../knowledge-web-service';
import authorization, {
  getAuthorization,
  redirectToLogin,
} from '@/utils/authorization-util';
import {
  fixture,
  Scope,
  CaptureId,
  Raw,
  file,
  response,
  failure,
} from '@/pages/dataset/knowledge-import/__tests__/web-fixture';
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
  getAuthorization: jest.fn(() => 'synthetic-web-token'),
  redirectToLogin: jest.fn(),
}));
it('preserves an enhanced File and original JSON through real Axios interceptors, with existing authentication', async () => {
  const f = await fixture(),
    original = file(),
    signal = new AbortController().signal,
    progress = jest.fn();
  Object.defineProperty(original, 'preview', {
    value: 'blob:synthetic',
    enumerable: true,
  });
  const adapter = jest.fn(async (config) => ({
      ...response(f.capture),
      config,
      statusText: 'Synthetic',
    })),
    previous = request.defaults.adapter;
  request.defaults.adapter = adapter;
  try {
    await service.upload(Scope, CaptureId, original, signal, progress);
    await service.createCapture(Scope, f.captureRequest);
    const first = adapter.mock.calls[0][0];
    expect(first.data).toBe(original);
    expect(Array.from(new Uint8Array(await first.data.arrayBuffer()))).toEqual(
      Array.from(Raw),
    );
    expect(first.headers.get('Authorization')).toBe(getAuthorization());
    expect(first.headers.get('Content-Type')).toBe('application/octet-stream');
    expect(first.signal).toBe(signal);
    expect(first.onUploadProgress).toBe(progress);
    expect(first.timeout).toBe(25000);
    expect(adapter.mock.calls[1][0].timeout).toBe(25000);
    expect(JSON.parse(adapter.mock.calls[1][0].data)).toEqual(f.captureRequest);
  } finally {
    request.defaults.adapter = previous;
  }
});
it('never logs out for upstream502; native401 still runs the existing cleanup and redirect', async () => {
  const previous = request.defaults.adapter;
  let status = 502;
  request.defaults.adapter = async (config) => {
    const result = {
      ...failure(
        status,
        status === 401 ? 'knowledge_unauthorized' : 'web_upstream_http_error',
        status === 401 ? null : 401,
      ).response,
      config,
      statusText: 'Synthetic',
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
    await expect(service.capture(Scope, CaptureId)).rejects.toMatchObject({
      status: 502,
      code: 'web_upstream_http_error',
    });
    expect(authorization.removeAll).not.toHaveBeenCalled();
    expect(redirectToLogin).not.toHaveBeenCalled();
    status = 401;
    await expect(service.capture(Scope, CaptureId)).rejects.toMatchObject({
      status: 401,
      code: 'knowledge_unauthorized',
    });
    expect(authorization.removeAll).toHaveBeenCalledTimes(1);
    expect(redirectToLogin).toHaveBeenCalledTimes(1);
  } finally {
    request.defaults.adapter = previous;
  }
});

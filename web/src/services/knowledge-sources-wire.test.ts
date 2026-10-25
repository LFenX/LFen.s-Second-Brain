import request from '@/utils/next-request';
import service from './knowledge-sources-service';
import {
  Registration,
  makePreview,
  makeReceipt,
  c2Envelope,
} from '@/pages/dataset/knowledge-import/__tests__/fixture';

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

it('keeps frozen confirmation JSON through the real shared interceptors and Axios serializer', async () => {
  const preview = makePreview();
  const commit = {
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: 'synthetic-wire-confirmation',
  };
  const adapter = jest.fn(async (config) => ({
    data: c2Envelope(makeReceipt(preview)),
    status: 201,
    statusText: 'Created',
    headers: {},
    config,
  }));
  const originalAdapter = request.defaults.adapter;
  request.defaults.adapter = adapter;
  localStorage.setItem('Authorization', 'synthetic-wire-token');
  try {
    await service.confirm(
      Registration.domain_id,
      Registration.registration_id,
      commit,
    );
    expect(adapter).toHaveBeenCalledTimes(1);
    const wire = adapter.mock.calls[0][0];
    expect(JSON.parse(wire.data)).toEqual(commit);
    expect(wire.headers.get('Content-Type')).toBe('application/json');
    expect(wire.headers.get('X-Knowledge-Contract-Version')).toBe('1.0.0');
    expect(wire.headers.get('X-Knowledge-Source-Contract-Version')).toBe(
      '0.2.0',
    );
    expect(wire.headers.get('Authorization')).toBe('synthetic-wire-token');
  } finally {
    request.defaults.adapter = originalAdapter;
    localStorage.removeItem('Authorization');
  }
});

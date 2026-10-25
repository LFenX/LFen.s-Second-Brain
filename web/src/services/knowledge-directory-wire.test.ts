import request from '@/utils/next-request';
import service from './knowledge-directory-service';
import {
  Registration,
  makeObservation,
  makePreview,
  makeReceipt,
  respond,
} from '@/pages/dataset/knowledge-import/__tests__/directory-fixture';

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

it('preserves observation_sha256 and manifest_sha256 through real shared Axios interceptors and serialization', async () => {
  const observation = makeObservation(),
    preview = makePreview(observation);
  const input = {
    observation_id: observation.observation_id,
    observation_sha256: observation.observation_sha256,
    relative_path: 'web/card.tsx',
    classification: observation.default_classification,
  };
  const commit = {
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: 'synthetic-wire',
  };
  const adapter = jest.fn(async (config) => ({
    ...respond(
      config.url.endsWith('/imports') ? makeReceipt(preview) : preview,
      201,
    ),
    statusText: 'Created',
    headers: {},
    config,
  }));
  const originalAdapter = request.defaults.adapter;
  request.defaults.adapter = adapter;
  localStorage.setItem('Authorization', 'synthetic-directory-wire-token');
  try {
    await service.preview(
      Registration.domain_id,
      Registration.registration_id,
      input,
    );
    await service.confirm(
      Registration.domain_id,
      Registration.registration_id,
      commit,
    );
    expect(adapter).toHaveBeenCalledTimes(2);
    expect(JSON.parse(adapter.mock.calls[0][0].data)).toEqual(input);
    expect(JSON.parse(adapter.mock.calls[1][0].data)).toEqual(commit);
    for (const [wire] of adapter.mock.calls) {
      expect(wire.headers.get('Content-Type')).toBe('application/json');
      expect(wire.headers.get('X-Knowledge-Contract-Version')).toBe('1.0.0');
      expect(wire.headers.get('X-Knowledge-Source-Contract-Version')).toBe(
        '0.3.0',
      );
      expect(wire.headers.get('Authorization')).toBe(
        'synthetic-directory-wire-token',
      );
      expect(wire.method).toBe('post');
    }
  } finally {
    request.defaults.adapter = originalAdapter;
    localStorage.removeItem('Authorization');
  }
});

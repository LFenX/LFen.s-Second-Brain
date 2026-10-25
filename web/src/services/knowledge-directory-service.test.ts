import request from '@/utils/next-request';
import service from './knowledge-directory-service';
import {
  copy,
  Registration,
  RegistrationRequest,
  Capabilities,
  Root,
  makeListing,
  makeObservation,
  makePreview,
  makeReceipt,
  windowFor,
  respond,
  reject,
} from '@/pages/dataset/knowledge-import/__tests__/directory-fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
const Http = request as unknown as jest.Mock;
const Domain = Registration.domain_id,
  Reg = Registration.registration_id;
const Base =
  '/api/v1/datasets/' + Domain + '/knowledge/directory-registrations/' + Reg;
beforeEach(() => Http.mockReset());

it('uses exactly the fourteen frozen operations, both version headers and synchronous response statuses', async () => {
  const listing = makeListing(),
    observation = makeObservation(),
    preview = makePreview(observation),
    receipt = makeReceipt(preview);
  const input = {
    observation_id: observation.observation_id,
    observation_sha256: observation.observation_sha256,
    relative_path: 'web/card.tsx',
    classification: observation.default_classification,
  };
  const commit = {
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: 'synthetic-original',
  };
  const cases = [
    [
      () => service.contract(),
      Capabilities,
      200,
      '/api/v1/knowledge/directory-import-contract',
      'get',
    ],
    [
      () => service.roots(Domain),
      { domain_id: Domain, items: [Root] },
      200,
      '/api/v1/datasets/' + Domain + '/knowledge/directory-roots',
      'get',
    ],
    [
      () => service.register(Domain, RegistrationRequest),
      Registration,
      201,
      Base.slice(0, -33),
      'post',
    ],
    [() => service.registration(Domain, Reg), Registration, 200, Base, 'get'],
    [
      () =>
        service.createListing(Domain, Reg, {
          relative_directory: 'web',
          name_prefix: '',
        }),
      listing,
      201,
      Base + '/listings',
      'post',
    ],
    [
      () =>
        service.listing(Domain, Reg, listing.listing_id, 'synthetic-cursor'),
      listing,
      200,
      Base + '/listings/' + listing.listing_id,
      'get',
    ],
    [
      () =>
        service.observe(Domain, Reg, {
          selected_paths: observation.selected_paths,
        }),
      observation,
      201,
      Base + '/observations',
      'post',
    ],
    [
      () => service.observation(Domain, Reg, observation.observation_id),
      observation,
      200,
      Base + '/observations/' + observation.observation_id,
      'get',
    ],
    [
      () => service.preview(Domain, Reg, input),
      preview,
      201,
      Base + '/previews',
      'post',
    ],
    [
      () => service.readPreview(Domain, Reg, preview.preview_id),
      preview,
      200,
      Base + '/previews/' + preview.preview_id,
      'get',
    ],
    [
      () => service.content(Domain, Reg, preview.preview_id, 0, 65536),
      windowFor(preview),
      200,
      Base + '/previews/' + preview.preview_id + '/content',
      'get',
    ],
    [
      () => service.confirm(Domain, Reg, commit),
      receipt,
      201,
      Base + '/imports',
      'post',
    ],
    [
      () => service.receipt(Domain, Reg, receipt.import_task_id),
      receipt,
      200,
      Base + '/imports/' + receipt.import_task_id,
      'get',
    ],
    [
      () => service.byKey(Domain, Reg, commit),
      receipt,
      200,
      Base + '/imports/by-key/synthetic-original',
      'get',
    ],
  ] as const;
  expect(Object.keys(service)).toHaveLength(14);
  for (const [call, data, status, url, method] of cases) {
    Http.mockResolvedValueOnce(respond(data, status));
    expect((await call()).httpStatus).toBe(status);
    const config = Http.mock.calls.at(-1)![0];
    expect(config.url).toBe(url);
    expect(config.method).toBe(method);
    expect(config.headers).toEqual({
      'X-Knowledge-Contract-Version': '1.0.0',
      'X-Knowledge-Source-Contract-Version': '0.3.0',
      ...(method === 'post' ? { 'Content-Type': 'application/json' } : {}),
    });
    expect(config.timeout).toBe(45000);
    if (method === 'get') expect(config.data).toBeUndefined();
  }
  expect(Http).toHaveBeenCalledTimes(14);
});
it('accepts idempotent registration replay with equivalent classification and scope property order', async () => {
  Http.mockResolvedValueOnce(
    respond({
      ...Registration,
      default_classification: {
        language: 'tsx',
        component_side: 'frontend',
        group_id: null,
      },
    }),
  );
  await expect(
    service.register(Domain, RegistrationRequest),
  ).resolves.toMatchObject({ httpStatus: 200 });
});
it.each(['ReviewCandidate', 'Draft', 'Unknown'])(
  'rejects non-frozen contract status %s',
  async (status) => {
    Http.mockResolvedValueOnce(respond({ ...Capabilities, status }));
    await expect(service.contract()).rejects.toMatchObject({ status: 502 });
  },
);
it('rejects mismatched versions, scopes, extra fields and unsupported successful HTTP statuses', async () => {
  Http.mockResolvedValueOnce({
    ...respond(Capabilities),
    data: { ...respond(Capabilities).data, source_contract_version: '0.2.0' },
  });
  await expect(service.contract()).rejects.toMatchObject({
    status: 409,
    code: 'knowledge_directory_version_mismatch',
  });
  Http.mockResolvedValueOnce(
    respond({ ...Registration, domain_id: 'f'.repeat(32) }),
  );
  await expect(service.registration(Domain, Reg)).rejects.toMatchObject({
    status: 502,
  });
  Http.mockResolvedValueOnce(
    respond({ ...Registration, absolute_root: '/synthetic-private-root' }),
  );
  await expect(service.registration(Domain, Reg)).rejects.toMatchObject({
    status: 502,
  });
  Http.mockResolvedValueOnce(respond(Registration, 202));
  await expect(service.registration(Domain, Reg)).rejects.toMatchObject({
    status: 502,
  });
  Http.mockResolvedValueOnce(
    respond({ ...Registration, root_id: 'f'.repeat(32) }, 201),
  );
  await expect(
    service.register(Domain, RegistrationRequest),
  ).rejects.toMatchObject({ status: 502 });
});
it('keeps HTTP 201 failed receipts distinct from successful saves', async () => {
  const preview = makePreview(),
    receipt = makeReceipt(preview, true);
  Http.mockResolvedValueOnce(respond(receipt, 201));
  const result = await service.confirm(Domain, Reg, {
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: 'synthetic-failed',
  });
  expect(result).toEqual({ httpStatus: 201, data: receipt });
  expect(result.data.items[0]).toMatchObject({
    source_id: null,
    source_revision_id: null,
    root_node_id: null,
  });
});
it.each([
  [400, 'knowledge_directory_invalid', null],
  [404, 'knowledge_directory_not_found', null],
  [409, 'knowledge_directory_conflict', 'source_changed'],
  [410, 'knowledge_directory_expired', null],
  [413, 'knowledge_directory_too_large', null],
  [415, 'knowledge_directory_unsupported', null],
  [416, 'knowledge_directory_range', null],
  [429, 'knowledge_directory_capacity', null],
  [500, 'knowledge_directory_unavailable', 'result_unknown'],
  [503, 'knowledge_directory_unavailable', 'result_unknown'],
])(
  'preserves frozen HTTP %s, code and reason',
  async (status, code, reason) => {
    Http.mockRejectedValueOnce(
      reject(status as number, code as string, reason as string | null),
    );
    await expect(service.registration(Domain, Reg)).rejects.toMatchObject({
      status,
      code,
      reason,
    });
    expect(Http).toHaveBeenCalledTimes(1);
  },
);
it('accepts native 401, refuses invalid error mappings and never guesses transport outcomes', async () => {
  Http.mockRejectedValueOnce({
    response: { status: 401, data: copy('valid-NativeUnauthorized') },
  });
  await expect(service.contract()).rejects.toMatchObject({
    status: 401,
    code: 'knowledge_unauthorized',
  });
  Http.mockRejectedValueOnce(
    reject(404, 'knowledge_directory_conflict', 'source_changed'),
  );
  await expect(service.contract()).rejects.toMatchObject({ status: 502 });
  Http.mockRejectedValueOnce(new Error('Synthetic timeout'));
  await expect(service.contract()).rejects.toMatchObject({
    status: 0,
    reason: 'result_unknown',
  });
});
it('rejects unsafe requests, 33 paths and invalid content ranges before transport', async () => {
  await expect(
    service.observe(Domain, Reg, { selected_paths: ['../private'] }),
  ).rejects.toMatchObject({ status: 400 });
  await expect(
    service.observe(Domain, Reg, {
      selected_paths: Array.from(
        { length: 33 },
        (_, i) => 'web/file' + i + '.ts',
      ),
    }),
  ).rejects.toMatchObject({ status: 400 });
  await expect(
    service.content(Domain, Reg, 'f'.repeat(32), 0, 65537),
  ).rejects.toMatchObject({ status: 400 });
  await expect(
    service.content(Domain, Reg, 'f'.repeat(32), -1, 1),
  ).rejects.toMatchObject({ status: 400 });
  expect(() =>
    service.byKey(Domain, Reg, {
      preview_id: 'bad',
      manifest_sha256: 'a'.repeat(64),
      idempotency_key: 'synthetic',
    }),
  ).toThrow();
  expect(Http).not.toHaveBeenCalled();
});
it('shrinks only the GET content length at a split UTF-8 boundary, with four bounded attempts', async () => {
  const preview = makePreview();
  Http.mockRejectedValueOnce(reject(416, 'knowledge_directory_range'))
    .mockRejectedValueOnce(reject(416, 'knowledge_directory_range'))
    .mockRejectedValueOnce(reject(416, 'knowledge_directory_range'))
    .mockResolvedValueOnce(respond(windowFor(preview, 0, 3)));
  await service.content(Domain, Reg, preview.preview_id, 0, 6);
  expect(Http.mock.calls.map(([config]) => config.params)).toEqual(
    [6, 5, 4, 3].map((length) => ({ offset: 0, length })),
  );
  expect(Http.mock.calls.every(([config]) => config.method === 'get')).toBe(
    true,
  );
  Http.mockReset().mockRejectedValue(reject(416, 'knowledge_directory_range'));
  await expect(
    service.content(Domain, Reg, preview.preview_id, 0, 65536),
  ).rejects.toMatchObject({ status: 416 });
  expect(Http).toHaveBeenCalledTimes(4);
});
it('returns an original by-key receipt after preview expiration without any preview, disk or POST operation', async () => {
  const preview = makePreview(),
    receipt = makeReceipt(preview),
    commit = {
      preview_id: preview.preview_id,
      manifest_sha256: preview.manifest_sha256,
      idempotency_key: 'synthetic-original',
    };
  Http.mockResolvedValueOnce(respond(receipt));
  await expect(service.byKey(Domain, Reg, commit)).resolves.toMatchObject({
    data: receipt,
  });
  expect(Http).toHaveBeenCalledTimes(1);
  expect(Http.mock.calls[0][0].method).toBe('get');
  Http.mockResolvedValueOnce(
    respond({ ...receipt, manifest_sha256: 'f'.repeat(64) }),
  );
  await expect(service.byKey(Domain, Reg, commit)).rejects.toMatchObject({
    status: 502,
  });
});
it.each(['frontend', 'backend', 'shared', 'cli', 'non_code'] as const)(
  'sends explicit %s classification without inheriting defaults or generating source identities',
  async (component_side) => {
    const observation = makeObservation(),
      input = {
        observation_id: observation.observation_id,
        observation_sha256: observation.observation_sha256,
        relative_path: 'web/card.tsx',
        classification: { group_id: null, component_side, language: '' },
      };
    Http.mockResolvedValueOnce(respond(makePreview(observation, input), 201));
    const result = await service.preview(Domain, Reg, input);
    expect(JSON.parse(Http.mock.calls[0][0].data)).toEqual(input);
    expect(result.data.items[0].classification).toEqual(input.classification);
    expect(result.data.items[0].source_id).toBeNull();
  },
);
it('enforces the 32KiB serialized JSON budget before transport even for individually valid paths', async () => {
  const selected_paths = Array.from(
    { length: 32 },
    (_, i) => '"'.repeat(510) + String(i).padStart(2, '0'),
  );
  expect(Buffer.byteLength(JSON.stringify({ selected_paths }))).toBeGreaterThan(
    32768,
  );
  await expect(
    service.observe(Domain, Reg, { selected_paths }),
  ).rejects.toMatchObject({
    status: 413,
    code: 'knowledge_directory_too_large',
  });
  expect(Http).not.toHaveBeenCalled();
});

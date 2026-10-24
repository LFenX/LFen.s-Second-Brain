import request from '@/utils/next-request';
import service from './knowledge-sources-service';
import {
  RegistrationRequest,
  Registration,
  PreviewRequest,
  ImportCapabilities,
  makePreview,
  makeReceipt,
  c2Envelope,
} from '@/pages/dataset/knowledge-import/__tests__/fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
const Http = request as unknown as jest.Mock;
beforeEach(() => Http.mockReset());

it('uses the existing HTTP transport, both version headers and only the seven frozen operations', async () => {
  const preview = makePreview();
  const receipt = makeReceipt(preview);
  const commit = {
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: 'synthetic-first',
  };
  for (const [index, data] of [
    ImportCapabilities,
    Registration,
    Registration,
    preview,
    preview,
    receipt,
    receipt,
  ].entries())
    Http.mockResolvedValueOnce({
      status: index === 3 ? 201 : 200,
      data: c2Envelope(data),
    });
  await service.contract();
  await service.register(Registration.domain_id, RegistrationRequest);
  await service.registration(
    Registration.domain_id,
    Registration.registration_id,
  );
  await service.preview(
    Registration.domain_id,
    Registration.registration_id,
    PreviewRequest,
  );
  await service.readPreview(
    Registration.domain_id,
    Registration.registration_id,
    preview.preview_id,
  );
  await service.confirm(
    Registration.domain_id,
    Registration.registration_id,
    commit,
  );
  await service.receipt(
    Registration.domain_id,
    Registration.registration_id,
    receipt.import_task_id,
  );
  expect(Http).toHaveBeenCalledTimes(7);
  for (const [config] of Http.mock.calls) {
    expect(config.url).toMatch(
      /^\/api\/v1\/(knowledge\/import-contract|datasets\/[^/]+\/knowledge\/registrations)/,
    );
    expect(config.headers).toEqual({
      'X-Knowledge-Contract-Version': '1.0.0',
      'X-Knowledge-Source-Contract-Version': '0.2.0',
    });
    expect(config.headers.Authorization).toBeUndefined();
    expect(config.timeout).toBe(30000);
  }
});

it.each(['ReviewCandidate', '0.1.0-draft', 'Unknown'])(
  'stops for non-Frozen capability %s',
  async (status) => {
    Http.mockResolvedValue({
      status: 200,
      data: c2Envelope({ ...ImportCapabilities, status }),
    });
    await expect(service.contract()).rejects.toMatchObject({
      status: 409,
      code: 'knowledge_source_contract_version',
    });
  },
);

it.each(['frontend', 'backend', 'shared', 'cli', 'non_code'] as const)(
  'round-trips explicit %s without inheriting the registration defaults',
  async (componentSide) => {
    const input = {
      ...PreviewRequest,
      group_id: null,
      component_side: componentSide,
      language: '',
    };
    Http.mockResolvedValueOnce({
      status: 201,
      data: c2Envelope(makePreview(input)),
    });
    const result = await service.preview(
      Registration.domain_id,
      Registration.registration_id,
      input,
    );
    expect(Http.mock.calls[0][0].data).toEqual(input);
    expect(result.data.items[0]).toMatchObject({
      group_id: null,
      component_side: componentSide,
      language: '',
    });
  },
);

it('preserves HTTP 201 failed receipts without treating envelope code zero as import success', async () => {
  const preview = makePreview();
  const failed = makeReceipt(preview, true);
  Http.mockResolvedValue({ status: 201, data: c2Envelope(failed) });
  const result = await service.confirm(
    Registration.domain_id,
    Registration.registration_id,
    {
      preview_id: preview.preview_id,
      manifest_sha256: preview.manifest_sha256,
      idempotency_key: 'failure',
    },
  );
  expect(result.httpStatus).toBe(201);
  expect(result.data.status).toBe('failed');
  expect(result.data.items[0].source_id).toBeNull();
});

it('rejects undeclared HTTP statuses and invalid preview timestamps', async () => {
  Http.mockResolvedValueOnce({ status: 202, data: c2Envelope(makePreview()) });
  await expect(
    service.preview(
      Registration.domain_id,
      Registration.registration_id,
      PreviewRequest,
    ),
  ).rejects.toMatchObject({ status: 502 });
  Http.mockResolvedValueOnce({
    status: 201,
    data: c2Envelope({ ...makePreview(), expires_at: 'invalid' }),
  });
  await expect(
    service.preview(
      Registration.domain_id,
      Registration.registration_id,
      PreviewRequest,
    ),
  ).rejects.toMatchObject({ status: 502 });
});

it.each([401, 404, 409, 413, 415, 500])(
  'preserves HTTP %i without guessing from message',
  async (status) => {
    Http.mockRejectedValue({
      response: {
        status,
        data: {
          code: 'knowledge_conflict',
          message: 'Unstable text',
          request_id: 'request',
        },
      },
    });
    await expect(
      service.registration(
        Registration.domain_id,
        Registration.registration_id,
      ),
    ).rejects.toMatchObject({
      status,
      code: 'knowledge_conflict',
      requestId: 'request',
    });
    expect(Http).toHaveBeenCalledTimes(1);
  },
);

it('handles native integer auth errors and transport timeouts', async () => {
  Http.mockResolvedValueOnce({
    status: 200,
    data: { code: 401, message: 'synthetic', data: null },
  });
  await expect(service.contract()).rejects.toMatchObject({ status: 401 });
  Http.mockRejectedValueOnce({ code: 'ECONNABORTED' });
  await expect(service.contract()).rejects.toMatchObject({
    status: 0,
    code: 'knowledge_unavailable',
  });
});

it('checks UTF-8 body limits before requests and leaves the server authoritative for formats', async () => {
  await expect(
    service.register(Registration.domain_id, {
      ...RegistrationRequest,
      include_paths: Array.from(
        { length: 16 },
        (_, index) => `${index}/${'中'.repeat(500)}.txt`,
      ),
    }),
  ).rejects.toMatchObject({ status: 413, code: 'knowledge_body_too_large' });
  await expect(
    service.preview(Registration.domain_id, Registration.registration_id, {
      ...PreviewRequest,
      text: '中'.repeat(699051),
    }),
  ).rejects.toMatchObject({ status: 413, code: 'knowledge_source_too_large' });
  expect(Http).not.toHaveBeenCalled();
  Http.mockRejectedValueOnce({
    response: { status: 415, data: { code: 'knowledge_source_unsupported' } },
  });
  await expect(
    service.preview(Registration.domain_id, Registration.registration_id, {
      ...PreviewRequest,
      relative_path: 'synthetic.csv',
    }),
  ).rejects.toMatchObject({ status: 415 });
  expect(Http).toHaveBeenCalledTimes(1);
});

it('rejects extra identity or processing fields, malformed receipts and wrong response scope', async () => {
  const unsupportedRequest = { ...PreviewRequest, source_id: 'synthetic' };
  await expect(
    service.preview(
      Registration.domain_id,
      Registration.registration_id,
      unsupportedRequest,
    ),
  ).rejects.toMatchObject({ status: 400 });
  expect(Http).not.toHaveBeenCalled();
  Http.mockResolvedValueOnce({
    status: 200,
    data: c2Envelope({ ...Registration, domain_id: 'f'.repeat(32) }),
  });
  await expect(
    service.registration(Registration.domain_id, Registration.registration_id),
  ).rejects.toMatchObject({ status: 502 });
  const failed = makeReceipt(makePreview(), true);
  Http.mockResolvedValueOnce({
    status: 200,
    data: c2Envelope({
      ...failed,
      items: [{ ...failed.items[0], source_id: '1'.repeat(32) }],
    }),
  });
  await expect(
    service.receipt(
      Registration.domain_id,
      Registration.registration_id,
      failed.import_task_id,
    ),
  ).rejects.toMatchObject({ status: 502 });
});

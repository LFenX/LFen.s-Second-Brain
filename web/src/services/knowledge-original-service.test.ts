import request from '@/utils/next-request';
import service from './knowledge-original-service';
import { originalScope } from '@/utils/knowledge-original';
import {
  DomainId,
  OwnerId,
  RegistrationId,
  UploadId,
  Bytes,
  Capabilities,
  blob,
  makeRegistration,
  makeUpload,
  makeRevision,
  makeReceipt,
  createRequest,
  confirmRequest,
  respond,
  binaryResponse,
  failure,
} from '@/pages/dataset/knowledge-import/__tests__/original-fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock;
beforeEach(() => Http.mockReset());
it('routes all 17 frozen operations with isolated headers and exact serialized request fields', async () => {
  const reg = await makeRegistration(),
    upload = await makeUpload(),
    receipt = makeReceipt(upload),
    source = makeRevision(upload);
  const create = {
    ...originalScope(DomainId, OwnerId),
    kind: 'upload' as const,
    registration_id: RegistrationId,
    request: createRequest(),
    request_sha256: upload.request_sha256,
    upload_id: UploadId,
  };
  const pending = {
    ...originalScope(DomainId, OwnerId),
    registration_id: RegistrationId,
    request: confirmRequest(upload),
    import_task_id: receipt.import_task_id,
  };
  const replies = [
    Capabilities,
    reg,
    reg,
    reg,
    {
      domain_id: DomainId,
      registration_id: RegistrationId,
      relative_path: upload.relative_path,
      source: null,
    },
    upload,
    upload,
    upload,
    upload,
    upload,
    receipt,
    receipt,
    receipt,
    { domain_id: DomainId, items: [source], next_cursor: null },
    source,
  ];
  Http.mockImplementation(async (config) =>
    config.url.endsWith('/content') && config.method !== 'put'
      ? binaryResponse(source)
      : respond(replies.shift()),
  );
  const {
    registration_key,
    name,
    source_kind,
    input_mode,
    include_paths,
    default_classification,
  } = reg;
  await service.contract();
  await service.register(DomainId, OwnerId, {
    registration_key,
    name,
    source_kind,
    input_mode,
    include_paths,
    default_classification,
  });
  await service.registrationByKey(
    DomainId,
    OwnerId,
    reg.registration_key,
    reg.config_sha256,
  );
  await service.registration(DomainId, RegistrationId, OwnerId);
  await service.target(DomainId, RegistrationId, upload.relative_path);
  await service.create(DomainId, RegistrationId, OwnerId, create.request);
  await service.uploadByKey(create);
  await service.upload(DomainId, RegistrationId, UploadId);
  await service.cancel(DomainId, RegistrationId, UploadId);
  await service.put(DomainId, RegistrationId, UploadId, blob());
  await service.confirm(DomainId, RegistrationId, pending.request);
  await service.receiptByKey(pending);
  await service.receipt(DomainId, RegistrationId, receipt.import_task_id);
  await service.list(DomainId);
  await service.revision(DomainId, source.source_revision_id);
  await service.content(source);
  await service.head(source);
  expect(Http).toHaveBeenCalledTimes(17);
  for (const [config] of Http.mock.calls) {
    expect(config.url).toMatch(/^\/api\/v1\//);
    expect(config.headers['X-Knowledge-Contract-Version']).toBe('1.0.0');
    expect(config.headers['X-Knowledge-Source-Contract-Version']).toBe('0.5.0');
    expect(config.validateStatus).toBeUndefined();
  }
  expect(JSON.parse(Http.mock.calls[5][0].data)).toEqual(create.request);
  expect(JSON.parse(Http.mock.calls[10][0].data)).toEqual(pending.request);
  expect(Http.mock.calls[9][0].transformRequest[0]()).toBeInstanceOf(Blob);
});
it.each([401, 404, 409, 410, 413, 415, 429, 503])(
  'preserves structured HTTP %i failures and native 401 handling',
  async (status) => {
    const code = (
      {
        401: 'knowledge_unauthorized',
        404: 'knowledge_not_found',
        409: 'upload_busy',
        410: 'upload_expired',
        413: 'upload_too_large',
        415: 'upload_format_unsupported',
        429: 'upload_slots_busy',
        503: 'result_unknown',
      } as Record<number, string>
    )[status];
    Http.mockRejectedValue(failure(status, code));
    await expect(service.contract()).rejects.toMatchObject({ status, code });
  },
);
it('rejects wrong scope, request digest, same-key different creation and mismatched recovery receipts', async () => {
  const upload = await makeUpload();
  Http.mockResolvedValue(
    respond({ ...upload, request_sha256: '0'.repeat(64) }),
  );
  await expect(
    service.upload(DomainId, RegistrationId, UploadId),
  ).rejects.toThrow();
  Http.mockResolvedValue(respond(upload));
  await expect(
    service.create(
      DomainId,
      RegistrationId,
      OwnerId,
      createRequest({ original_filename: 'different.pdf' }),
    ),
  ).rejects.toThrow();
  const pending = {
    ...originalScope(DomainId, OwnerId),
    registration_id: RegistrationId,
    request: confirmRequest(upload),
    import_task_id: null,
  };
  Http.mockResolvedValue(
    respond({ ...makeReceipt(upload), upload_id: '0'.repeat(32) }),
  );
  await expect(service.receiptByKey(pending)).rejects.toThrow();
});
it('rejects metadata over 32KiB before reaching the request layer', async () => {
  const upload = await makeUpload(),
    request = createRequest({ original_filename: '原'.repeat(40000) + '.pdf' });
  await expect(
    service.create(DomainId, RegistrationId, OwnerId, request),
  ).rejects.toThrow();
  expect(Http).not.toHaveBeenCalled();
  Http.mockResolvedValue(respond(upload));
});
it('verifies complete Blob SHA and rejects MIME, ETag, length, status, HTML and byte corruption', async () => {
  const source = makeRevision(await makeUpload());
  Http.mockResolvedValue(binaryResponse(source));
  await expect(service.download(source)).resolves.toHaveProperty(
    'size',
    Bytes.length,
  );
  const invalidHeaders: Record<string, string>[] = [
    { 'content-type': 'text/html' },
    { etag: 'W/"sha256-' + source.sha256 + '"' },
    { 'content-length': '5' },
    { 'content-length': '06' },
  ];
  for (const headers of invalidHeaders) {
    Http.mockResolvedValue(binaryResponse(source, blob(), 200, headers));
    await expect(service.download(source)).rejects.toThrow();
  }
  Http.mockResolvedValue(
    binaryResponse(source, blob(new Uint8Array(Bytes.length))),
  );
  await expect(service.download(source)).rejects.toMatchObject({
    code: 'original_integrity',
  });
  Http.mockResolvedValue(
    binaryResponse(source, blob(), 206, { 'content-range': 'bytes 0-5/6' }),
  );
  await expect(service.download(source)).rejects.toThrow();
});
it('validates ranges, accepts If-Range full fallback only as a full body and handles empty HEAD errors', async () => {
  const source = makeRevision(await makeUpload());
  Http.mockResolvedValue(
    binaryResponse(source, blob(Bytes.slice(1, 3)), 206, {
      'content-range': 'bytes 1-2/6',
    }),
  );
  await expect(
    service.content(source, undefined, 'bytes=1-2'),
  ).resolves.toHaveProperty('status', 206);
  await expect(
    service.content(source, undefined, 'bytes=2-3'),
  ).rejects.toThrow();
  Http.mockResolvedValue(binaryResponse(source));
  await expect(
    service.content(source, undefined, 'bytes=1-2', 'old'),
  ).resolves.toHaveProperty('status', 200);
  Http.mockResolvedValue(
    binaryResponse(source, blob(new Uint8Array()), 200, {
      'content-length': '6',
    }),
  );
  await expect(
    service.head(source, undefined, 'invalid range', 'weak'),
  ).resolves.toHaveProperty('status', 200);
  const error = failure(409, 'original_integrity');
  error.response.data = '' as any;
  Http.mockRejectedValue(error);
  await expect(service.head(source)).rejects.toMatchObject({
    status: 409,
    code: 'original_integrity',
  });
});
it('decodes only bounded JSON error Blobs and never writes an error file', async () => {
  const source = makeRevision(await makeUpload()),
    error = failure(503, 'original_storage_unavailable');
  Http.mockRejectedValue({
    response: {
      ...error.response,
      data: blob(
        new TextEncoder().encode(JSON.stringify(error.response.data)),
        'application/json',
      ),
    },
  });
  await expect(service.download(source)).rejects.toMatchObject({
    status: 503,
    code: 'original_storage_unavailable',
  });
  Http.mockRejectedValue({
    response: {
      ...error.response,
      data: blob(new Uint8Array(32769), 'application/json'),
    },
  });
  await expect(service.download(source)).rejects.toMatchObject({
    code: 'original_invalid_response',
  });
});

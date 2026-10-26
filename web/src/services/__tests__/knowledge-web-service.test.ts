import request from '@/utils/next-request';
import service, { WebRequestError } from '../knowledge-web-service';
import {
  fixture,
  Scope,
  DomainId,
  CaptureId,
  PreviewId,
  ReceiptId,
  Raw,
  Decoded,
  Reading,
  Encoder,
  response,
  binaryResponse,
  blob,
  failure,
  file,
} from '@/pages/dataset/knowledge-import/__tests__/web-fixture';
import { webDigest, webSha256 } from '@/utils/knowledge-web';
type Fixture =
  import('@/pages/dataset/knowledge-import/__tests__/web-fixture').WebFixture;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock;
beforeEach(() => Http.mockReset());
const Operations: Record<
  string,
  (f: Fixture) => {
    work: () => Promise<unknown>;
    data: unknown;
    path: string;
    method: string;
    binary?: boolean;
  }
> = {
  contract: (f) => ({
    work: () => service.contract(),
    data: f.capabilities,
    path: '/knowledge/web-import-contract',
    method: 'get',
  }),
  createRegistration: (f) => ({
    work: () => service.createRegistration(Scope, f.registrationRequest),
    data: f.registration,
    path: '/web-registrations',
    method: 'post',
  }),
  registrationByKey: (f) => ({
    work: () => service.registrationByKey(Scope, f.registrationRequest),
    data: f.registration,
    path: '/web-registrations/by-key',
    method: 'get',
  }),
  registration: (f) => ({
    work: () => service.registration(Scope),
    data: f.registration,
    path: '/web-registrations/' + Scope.registration,
    method: 'get',
  }),
  target: (f) => ({
    work: () => service.target(Scope, f.entry.entry_key),
    data: { ...f.registration, ...{} },
    path: '/target',
    method: 'get',
  }),
  createCapture: (f) => ({
    work: () => service.createCapture(Scope, f.captureRequest),
    data: f.capture,
    path: '/captures',
    method: 'post',
  }),
  captureByKey: (f) => ({
    work: () => service.captureByKey(Scope, f.captureRequest),
    data: f.capture,
    path: '/captures/by-key',
    method: 'get',
  }),
  capture: (f) => ({
    work: () => service.capture(Scope, CaptureId),
    data: f.capture,
    path: '/captures/' + CaptureId,
    method: 'get',
  }),
  cancelCapture: (f) => ({
    work: () => service.cancelCapture(Scope, CaptureId),
    data: f.capture,
    path: '/captures/' + CaptureId,
    method: 'delete',
  }),
  upload: (f) => ({
    work: () => service.upload(Scope, CaptureId, file()),
    data: f.capture,
    path: '/content',
    method: 'put',
  }),
  captureContent: (f) => ({
    work: () => service.captureContent(Scope, f.capture),
    data: binaryResponse(Raw, f.capture.raw_sha256!),
    path: '/captures/' + CaptureId + '/content',
    method: 'get',
    binary: true,
  }),
  acquire: (f) => ({
    work: () =>
      service.acquire(Scope, CaptureId, {
        scope_sha256: f.capture.scope_sha256,
        expected_capture_revision: 1,
        confirmed: true,
      }),
    data: f.capture,
    path: '/acquire',
    method: 'post',
  }),
  observe: (f) => ({
    work: () =>
      service.observe(Scope, CaptureId, {
        normalizer_version: 'web-normalizer/0.6.0',
      }),
    data: f.observation,
    path: '/observation',
    method: 'post',
  }),
  observation: (f) => ({
    work: () => service.observation(Scope, CaptureId),
    data: f.observation,
    path: '/observation',
    method: 'get',
  }),
  entries: (f) => ({
    work: () => service.entries(Scope, CaptureId),
    data: { items: [f.summary], next_cursor: null },
    path: '/observation/entries',
    method: 'get',
  }),
  entry: (f) => ({
    work: () => service.entry(Scope, CaptureId, f.entry.entry_key),
    data: f.entry,
    path: '/entries/' + f.entry.entry_key,
    method: 'get',
  }),
  entryReferences: (f) => ({
    work: () => service.entryReferences(Scope, CaptureId, f.entry.entry_key),
    data: { items: f.references, next_cursor: null },
    path: '/references',
    method: 'get',
  }),
  entryContent: (f) => ({
    work: () =>
      service.representation({ scope: Scope, entry: f.entry }, 'safe_reading'),
    data: binaryResponse(
      Encoder.encode(Reading),
      f.entry.representations[1].sha256,
    ),
    path: '/safe_reading/content',
    method: 'get',
    binary: true,
  }),
  entryWindow: (f) => ({
    work: () =>
      service.window(
        { scope: Scope, entry: f.entry },
        'decoded_content',
        0,
        Encoder.encode(Decoded).length,
      ),
    data: {
      domain_id: DomainId,
      capture_id: CaptureId,
      observation_id: f.observation.observation_id,
      entry_key: f.entry.entry_key,
      source_revision_id: null,
      representation: f.entry.representations[0],
      start_byte: 0,
      end_byte: Encoder.encode(Decoded).length,
      next_start_byte: null,
      text: Decoded,
    },
    path: '/decoded_content/window',
    method: 'get',
  }),
  createPreview: (f) => ({
    work: () => service.createPreview(Scope, f.previewRequest),
    data: f.preview,
    path: '/previews',
    method: 'post',
  }),
  previewByKey: (f) => ({
    work: () => service.previewByKey(Scope, f.previewRequest),
    data: f.preview,
    path: '/previews/by-key',
    method: 'get',
  }),
  preview: (f) => ({
    work: () => service.preview(Scope, PreviewId),
    data: f.preview,
    path: '/previews/' + PreviewId,
    method: 'get',
  }),
  cancelPreview: (f) => ({
    work: () => service.cancelPreview(Scope, PreviewId),
    data: f.preview,
    path: '/previews/' + PreviewId,
    method: 'delete',
  }),
  previewItems: (f) => ({
    work: () => service.previewItems(Scope, PreviewId),
    data: { items: [f.previewItem], next_cursor: null },
    path: '/items',
    method: 'get',
  }),
  confirm: (f) => ({
    work: () => service.confirm(Scope, f.confirm),
    data: f.receipt,
    path: '/imports',
    method: 'post',
  }),
  receiptByKey: (f) => ({
    work: () => service.receiptByKey(Scope, f.confirm),
    data: f.receipt,
    path: '/imports/by-key',
    method: 'get',
  }),
  receipt: (f) => ({
    work: () => service.receipt(Scope, ReceiptId),
    data: f.receipt,
    path: '/imports/' + ReceiptId,
    method: 'get',
  }),
  receiptItems: (f) => ({
    work: () => service.receiptItems(Scope, ReceiptId),
    data: { items: [f.receiptItem], next_cursor: null },
    path: '/items',
    method: 'get',
  }),
  revisions: (f) => ({
    work: () => service.revisions(DomainId),
    data: { items: [f.revision], next_cursor: null },
    path: '/web-revisions',
    method: 'get',
  }),
  revision: (f) => ({
    work: () => service.revision(DomainId, f.revision.source_revision_id),
    data: f.revision,
    path: '/web-revisions/' + f.revision.source_revision_id,
    method: 'get',
  }),
  revisionReferences: (f) => ({
    work: () =>
      service.revisionReferences(DomainId, f.revision.source_revision_id),
    data: { items: f.references, next_cursor: null },
    path: '/references',
    method: 'get',
  }),
  revisionContent: (f) => ({
    work: () =>
      service.representation({ revision: f.revision }, 'safe_reading'),
    data: binaryResponse(
      Encoder.encode(Reading),
      f.revision.representations[1].sha256,
    ),
    path: '/safe_reading/content',
    method: 'get',
    binary: true,
  }),
  revisionWindow: (f) => ({
    work: () =>
      service.window(
        { revision: f.revision },
        'decoded_content',
        0,
        Encoder.encode(Decoded).length,
      ),
    data: {
      domain_id: DomainId,
      capture_id: CaptureId,
      observation_id: f.observation.observation_id,
      entry_key: f.entry.entry_key,
      source_revision_id: f.revision.source_revision_id,
      representation: f.entry.representations[0],
      start_byte: 0,
      end_byte: Encoder.encode(Decoded).length,
      next_start_byte: null,
      text: Decoded,
    },
    path: '/decoded_content/window',
    method: 'get',
  }),
  revisionCapture: (f) => ({
    work: () => service.revisionCapture(f.revision),
    data: binaryResponse(Raw, f.revision.capture_sha256),
    path: '/capture/content',
    method: 'get',
    binary: true,
  }),
};
it.each(Object.keys(Operations))('0.6 operation: %s', async (name) => {
  const f = await fixture(),
    operation = Operations[name](f);
  if (name === 'target')
    operation.data = {
      domain_id: DomainId,
      registration_id: Scope.registration,
      entry_key: f.entry.entry_key,
      source_id: null,
      current_revision_id: null,
      current_input_sha256: null,
      current_classification: null,
    };
  Http.mockResolvedValue(
    operation.binary ? operation.data : response(operation.data),
  );
  await operation.work();
  const config = Http.mock.calls[0][0];
  expect(config.url.endsWith(operation.path)).toBe(true);
  expect(config.method).toBe(operation.method);
  expect(config.url).toMatch(/^\/api\/v1\//);
  expect(config.headers['X-Knowledge-Source-Contract-Version']).toBe('0.6.0');
  expect(config.headers['X-Knowledge-Contract-Version']).toBe('1.0.0');
  expect(config.headers.Range).toBeUndefined();
  expect(config.timeout).toBe(25000);
});
it('covers the frozen 34 operations', () =>
  expect(Object.keys(Operations)).toHaveLength(34));
it('serializes original keys and SHA fields exactly, preserving case and explicit null', async () => {
  const f = await fixture(),
    input = { ...f.captureRequest, idempotency_key: 'Case:Sensitive.001' };
  const cap = { ...f.capture, idempotency_key: input.idempotency_key };
  cap.request_sha256 = await webDigest('capture_request', {
    registration_id: Scope.registration,
    registration_config_sha256: input.registration_config_sha256,
    acquisition_mode: 'local_snapshot',
    local_declaration: {
      original_filename: input.original_filename,
      declared_byte_length: input.declared_byte_length,
      declared_media_type: input.declared_media_type,
      expected_sha256: input.expected_sha256,
      declared_final_url: input.declared_final_url,
      claimed_captured_at: input.claimed_captured_at,
    },
  });
  Http.mockResolvedValue(response(cap, 201));
  await service.createCapture(Scope, input);
  expect(JSON.parse(Http.mock.calls[0][0].data)).toEqual(input);
});
it('rejects scope, hash, version, JSON budget and unexpected fields rather than adopting responses', async () => {
  const f = await fixture();
  for (const value of [
    { ...f.capture, owner_id: '8'.repeat(32) },
    { ...f.capture, request_sha256: '0'.repeat(64) },
    { ...f.capture, extra: true },
  ]) {
    Http.mockResolvedValue(response(value));
    await expect(service.capture(Scope, CaptureId)).rejects.toBeInstanceOf(
      WebRequestError,
    );
  }
  const oversized = response(f.capture);
  oversized.data.message = '中'.repeat(12000);
  Http.mockResolvedValue(oversized);
  await expect(service.capture(Scope, CaptureId)).rejects.toThrow();
  Http.mockResolvedValue({
    ...response(f.capture),
    data: { ...response(f.capture).data, source_contract_version: '0.5.0' },
  });
  await expect(service.capture(Scope, CaptureId)).rejects.toMatchObject({
    status: 409,
  });
});
it('bounds binary error parsing and keeps upstream 401/403/404 as API502 failures', async () => {
  for (const status of [401, 403, 404]) {
    Http.mockRejectedValue(failure(502, 'web_upstream_http_error', status));
    await expect(service.capture(Scope, CaptureId)).rejects.toMatchObject({
      status: 502,
      code: 'web_upstream_http_error',
      failure: { upstream_http_status: status },
    });
  }
  const issue = failure();
  issue.response.data = blob(
    new Uint8Array(32769),
    'application/json',
  ) as never;
  Http.mockRejectedValue(issue);
  await expect(service.capture(Scope, CaptureId)).rejects.toMatchObject({
    code: 'web_invalid_response',
  });
});
it('requires octet-stream, exact length and SHA, security headers and 200; supports empty content', async () => {
  const f = await fixture(),
    good = binaryResponse(
      Encoder.encode(Reading),
      f.entry.representations[1].sha256,
    );
  for (const bad of [
    { ...good, status: 206 },
    { ...good, headers: { ...good.headers, 'content-length': '1' } },
    { ...good, headers: { ...good.headers, 'content-type': 'text/html' } },
    {
      ...good,
      headers: { ...good.headers, 'x-content-sha256': '0'.repeat(64) },
    },
    { ...good, headers: { ...good.headers, 'content-security-policy': '' } },
  ]) {
    Http.mockResolvedValue(bad);
    await expect(
      service.representation({ scope: Scope, entry: f.entry }, 'safe_reading'),
    ).rejects.toThrow();
  }
  const empty = {
    ...f.entry,
    representations: [
      f.entry.representations[0],
      {
        ...f.entry.representations[1],
        byte_length: 0,
        sha256: await webSha256(new Uint8Array()),
      },
    ],
  };
  Http.mockResolvedValue(
    binaryResponse(new Uint8Array(), empty.representations[1].sha256),
  );
  expect(
    (
      await service.representation(
        { scope: Scope, entry: empty },
        'safe_reading',
      )
    ).size,
  ).toBe(0);
});

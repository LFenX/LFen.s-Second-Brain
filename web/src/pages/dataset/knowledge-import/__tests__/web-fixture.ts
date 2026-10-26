import { webcrypto, randomUUID } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-web-import-v1.examples.json';
import type * as Data from '@/interfaces/database/knowledge-web';
import type * as Request from '@/interfaces/request/knowledge-web';
import {
  webDigest,
  webSha256,
  webCaptureProjection,
  webSourceProjection,
  webStoredJsonBytes,
} from '@/utils/knowledge-web';

Object.defineProperty(globalThis.crypto, 'subtle', {
  configurable: true,
  value: webcrypto.subtle,
});
Object.defineProperty(globalThis.crypto, 'randomUUID', {
  configurable: true,
  value: randomUUID,
});
export const DomainId = 'a'.repeat(32),
  OwnerId = 'b'.repeat(32),
  RegistrationId = 'c'.repeat(32),
  CaptureId = 'd'.repeat(32),
  ObservationId = 'e'.repeat(32),
  PreviewId = 'f'.repeat(32),
  ReceiptId = '3'.repeat(32);
export const Scope = {
  domain: DomainId,
  owner: OwnerId,
  registration: RegistrationId,
};
export const Encoder = new TextEncoder(),
  Raw = Encoder.encode(
    '<rss version="2.0"><channel><title>合成Feed</title><item><guid> A:大脑/é </guid><title>合成代码</title><description>合成正文</description></item></channel></rss>',
  ),
  Decoded = 'const value = "<b>中文</b>";\n',
  Reading =
    '<article><pre><code>const value = &quot;&lt;b&gt;中文&lt;/b&gt;&quot;;</code></pre></article>';
export function blob(
  bytes: Uint8Array = Raw,
  type = 'application/octet-stream',
) {
  const value = new Blob([bytes as BlobPart], { type });
  Object.defineProperty(value, 'arrayBuffer', {
    value: async () => bytes.slice().buffer,
  });
  Object.defineProperty(value, 'text', {
    value: async () => new TextDecoder().decode(bytes),
  });
  return value;
}
export function file(bytes: Uint8Array = Raw, name = 'snapshot.xml') {
  const value = new File([bytes as BlobPart], name, {
    type: 'application/rss+xml',
  });
  Object.defineProperty(value, 'arrayBuffer', {
    value: async () => bytes.slice().buffer,
  });
  return value;
}
export function response(data: unknown, status = 200) {
  return {
    status,
    data: {
      code: 0,
      message: '',
      contract_version: '1.0.0',
      source_contract_version: '0.6.0',
      request_id: '9'.repeat(32),
      data,
    },
    headers: { 'cache-control': 'no-store' },
  };
}
export function binaryResponse(bytes: Uint8Array, sha: string) {
  return {
    status: 200,
    data: blob(bytes),
    headers: {
      'content-type': 'application/octet-stream',
      'content-length': String(bytes.length),
      'x-content-sha256': sha,
      'x-knowledge-contract-version': '1.0.0',
      'x-knowledge-source-contract-version': '0.6.0',
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
      'content-disposition': 'attachment; filename="synthetic.bin"',
      'content-security-policy': "default-src 'none'; sandbox",
    },
  };
}
export function failure(
  status = 503,
  code = 'web_result_unknown',
  upstream: number | null = null,
) {
  return {
    response: {
      status,
      headers: { 'content-type': 'application/json' },
      data: {
        code,
        message: 'synthetic failure',
        contract_version: '1.0.0',
        source_contract_version: '0.6.0',
        request_id: '9'.repeat(32),
        data: null,
        failure: {
          stage: 'fetch',
          machine_code: code,
          upstream_http_status: upstream,
          outcome_known: false,
        },
      },
    },
  };
}
export async function fixture() {
  const samples = JSON.parse(
      JSON.stringify(Examples.samples),
    ) as typeof Examples.samples,
    stamp = new Date(Math.floor(Date.now() / 1000) * 1000),
    created = stamp.toISOString(),
    expires = new Date(stamp.valueOf() + 1800000).toISOString();
  const registration = samples.registration as Data.WebRegistration;
  registration.created_at = created;
  const registrationRequest =
    samples.registration_request as Request.WebCreateRegistration;
  const captureRequest: Request.WebCreateLocalCapture = {
    ...(samples.local_capture_request as Request.WebCreateLocalCapture),
    original_filename: 'snapshot.xml',
    declared_byte_length: Raw.length,
    declared_media_type: 'application/rss+xml',
    expected_sha256: await webSha256(Raw),
  };
  const capture: Data.WebCapture = {
    ...(samples.local_capture_ready as Data.WebCapture),
    created_at: created,
    updated_at: created,
    expires_at: expires,
    original_filename: captureRequest.original_filename,
    declared_byte_length: Raw.length,
    declared_media_type: 'application/rss+xml',
    expected_sha256: captureRequest.expected_sha256,
    received_bytes: Raw.length,
    raw_byte_length: Raw.length,
    raw_sha256: captureRequest.expected_sha256,
    media_type: 'application/rss+xml',
  };
  capture.request_sha256 = await webDigest(
    'capture_request',
    webCaptureProjection(RegistrationId, captureRequest),
  );
  const reference = samples.reference as Data.WebReference;
  reference.reference_id = await webDigest('reference', reference);
  const references = [reference];
  const entry = samples.entry as Data.WebEntry;
  entry.representations[0] = {
    ...entry.representations[0],
    byte_length: Encoder.encode(Decoded).length,
    sha256: await webSha256(Encoder.encode(Decoded)),
  };
  entry.representations[1] = {
    ...entry.representations[1],
    byte_length: Encoder.encode(Reading).length,
    sha256: await webSha256(Encoder.encode(Reading)),
  };
  entry.decoded_byte_length = entry.representations[0].byte_length;
  entry.reading_byte_length = entry.representations[1].byte_length;
  entry.metadata_sha256 = await webDigest('metadata', entry.metadata);
  entry.references_sha256 = await webDigest('references', { references });
  entry.source_input_sha256 = await webDigest(
    'source_input',
    webSourceProjection(entry, registration.default_classification),
  );
  const observation: Data.WebObservation = {
    ...(samples.observation as Data.WebObservation),
    capture_sha256: capture.raw_sha256!,
    created_at: created,
    expires_at: expires,
    decoded_bytes: entry.decoded_byte_length,
    reading_bytes: entry.reading_byte_length,
    metadata_bytes: webStoredJsonBytes(entry.metadata),
    reference_bytes: webStoredJsonBytes(references),
  };
  observation.observation_sha256 = await webDigest('observation', {
    capture_sha256: capture.raw_sha256,
    producer_versions: capture.producer_versions,
    entries: [
      {
        entry_key: entry.entry_key,
        positions: entry.positions,
        source_input_sha256: entry.source_input_sha256,
        eligible: true,
      },
    ],
  });
  const {
    observation_id: _o,
    capture_id: _c,
    identity_value: _i,
    positions: _p,
    metadata: _m,
    metadata_sha256: _ms,
    references_sha256: _rs,
    reference_count: _rc,
    representations: _r,
    producer_versions: _v,
    ...summary
  } = entry;
  void [_o, _c, _i, _p, _m, _ms, _rs, _rc, _r, _v];
  const previewRequest: Request.WebCreatePreview = {
    ...(samples.preview_request as Request.WebCreatePreview),
    observation_sha256: observation.observation_sha256!,
  };
  const previewItem: Data.WebPreviewItem = {
    ...(samples.preview_item as Data.WebPreviewItem),
    source_input_sha256: entry.source_input_sha256!,
    decoded_sha256: entry.representations[0].sha256,
    reading_sha256: entry.representations[1].sha256,
  };
  const preview: Data.WebPreview = {
    ...(samples.preview as Data.WebPreview),
    capture_sha256: capture.raw_sha256!,
    observation_sha256: observation.observation_sha256!,
    request_sha256: await webDigest('selection_request', previewRequest),
    created_at: created,
    expires_at: new Date(stamp.valueOf() + 600000).toISOString(),
    selected_byte_length:
      entry.decoded_byte_length +
      entry.reading_byte_length +
      webStoredJsonBytes(entry.metadata) +
      webStoredJsonBytes(references),
  };
  preview.manifest_sha256 = await webDigest('preview_manifest', {
    ...preview,
    items: [previewItem],
  });
  const confirm: Request.WebConfirm = {
    ...(samples.confirm as Request.WebConfirm),
    manifest_sha256: preview.manifest_sha256,
  };
  const receipt: Data.WebReceipt = {
    ...(samples.receipt as Data.WebReceipt),
    capture_sha256: capture.raw_sha256!,
    observation_sha256: observation.observation_sha256!,
    preview_request_sha256: preview.request_sha256,
    manifest_sha256: preview.manifest_sha256,
    request_sha256: await webDigest('confirmation', confirm),
    created_at: created,
  };
  const receiptItem: Data.WebReceiptItem = {
    ...(samples.receipt_item as Data.WebReceiptItem),
    source_input_sha256: entry.source_input_sha256!,
    decoded_sha256: entry.representations[0].sha256,
    reading_sha256: entry.representations[1].sha256,
  };
  const revision: Data.WebRevision = {
    ...(samples.web_revision as Data.WebRevision),
    capture_sha256: capture.raw_sha256!,
    source_input_sha256: entry.source_input_sha256!,
    metadata: entry.metadata,
    metadata_sha256: entry.metadata_sha256,
    references_sha256: entry.references_sha256,
    representations: entry.representations,
    created_at: created,
  };
  return {
    registration,
    registrationRequest,
    captureRequest,
    capture,
    observation,
    entry,
    summary,
    references,
    previewRequest,
    preview,
    previewItem,
    confirm,
    receipt,
    receiptItem,
    revision,
    capabilities: {
      ...samples.capabilities,
      runtime_ready: true,
    } as Data.WebCapabilities,
  };
}
export type WebFixture = Awaited<ReturnType<typeof fixture>>;

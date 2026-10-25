import { webcrypto, createHash, randomUUID } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-original-import-v1.examples.json';
import type {
  OriginalUpload,
  OriginalUploadState,
  OriginalRevision,
  OriginalReceipt,
  OriginalRegistration,
} from '@/interfaces/database/knowledge-original';
import type {
  CreateOriginalUpload,
  ConfirmOriginalImport,
} from '@/interfaces/request/knowledge-original';
import {
  originalDigest,
  originalRegistrationRequest,
} from '@/utils/knowledge-original';

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
  UploadId = 'd'.repeat(32);
export const Bytes = new Uint8Array([0, 255, 13, 10, 65, 66]);
export const Digest = createHash('sha256').update(Bytes).digest('hex');
export const Classification = {
  group_id: null,
  component_side: 'non_code' as const,
  language: '',
};
export const Capabilities = {
  ...Examples.samples.capabilities,
  runtime_ready: true,
};
export function blob(bytes: Uint8Array = Bytes, type = 'application/pdf') {
  const data = new Blob([bytes as BlobPart], { type });
  Object.defineProperty(data, 'arrayBuffer', {
    value: async () => bytes.slice().buffer,
  });
  Object.defineProperty(data, 'text', {
    value: async () => new TextDecoder().decode(bytes),
  });
  return data;
}
export function file(
  name = 'original.pdf',
  bytes: Uint8Array = Bytes,
  type = 'application/pdf',
) {
  return new File([bytes as BlobPart], name, { type });
}
export async function makeRegistration(
  patch: Partial<OriginalRegistration> = {},
): Promise<OriginalRegistration> {
  const value: OriginalRegistration = {
    ...Examples.samples.registration,
    source_kind: 'original_document',
    input_mode: 'original_upload',
    default_classification: Classification,
    registration_id: RegistrationId,
    domain_id: DomainId,
    config_sha256: '0'.repeat(64),
    created_at: '2026-10-26T01:00:00Z',
    ...patch,
  };
  value.config_sha256 = await originalDigest('registration', {
    ...originalRegistrationRequest(value),
    domain_id: value.domain_id,
    owner_id: OwnerId,
  });
  return value;
}
export function createRequest(
  patch: Partial<CreateOriginalUpload> = {},
): CreateOriginalUpload {
  return {
    original_filename: 'original.pdf',
    relative_path: 'docs/合成.pdf',
    classification: Classification,
    declared_byte_length: Bytes.length,
    declared_media_type: 'application/pdf',
    expected_sha256: null,
    expected_revision_id: null,
    idempotency_key: 'create:original:1',
    ...patch,
  };
}
export async function makeUpload(
  state: OriginalUploadState = 'ready',
  patch: Partial<OriginalUpload> = {},
  input = createRequest(),
): Promise<OriginalUpload> {
  const created = new Date(Math.floor(Date.now() / 1000) * 1000);
  const stamp = (date: Date) => date.toISOString().replace('.000Z', 'Z');
  const ready = state === 'ready' || state === 'committed';
  const value: OriginalUpload = {
    ...input,
    upload_id: UploadId,
    domain_id: DomainId,
    owner_id: OwnerId,
    registration_id: RegistrationId,
    request_sha256: '',
    source_id: null,
    policy_version: 1,
    state,
    received_bytes: ready ? input.declared_byte_length : 0,
    byte_length: ready ? input.declared_byte_length : null,
    sha256: ready ? Digest : null,
    media_type: ready ? 'application/pdf' : null,
    action: ready ? 'created' : null,
    manifest_sha256: null,
    import_task_id: state === 'committed' ? '1'.repeat(32) : null,
    code: null,
    created_at: stamp(created),
    updated_at: stamp(created),
    expires_at: stamp(new Date(created.valueOf() + 1800000)),
    revision: ready ? 2 : 1,
    ...patch,
  };
  value.request_sha256 = await originalDigest('request', value);
  if (ready) value.manifest_sha256 = await originalDigest('preview', value);
  return value;
}
export function makeRevision(
  upload: OriginalUpload,
  patch: Partial<OriginalRevision> = {},
): OriginalRevision {
  return {
    domain_id: upload.domain_id,
    registration_id: upload.registration_id,
    source_id: 'e'.repeat(32),
    source_revision_id: 'f'.repeat(32),
    original_filename: upload.original_filename,
    relative_path: upload.relative_path,
    classification: upload.classification,
    sha256: upload.sha256!,
    byte_length: upload.byte_length!,
    media_type: upload.media_type!,
    representation_kind: 'original_only',
    root_node_id: null,
    text_available: false,
    parse_status: 'not_parsed',
    structure_status: 'not_built',
    full_text_status: 'not_built',
    vector_status: 'not_built',
    is_current: true,
    created_at: upload.created_at,
    ...patch,
  };
}
export function confirmRequest(upload: OriginalUpload): ConfirmOriginalImport {
  return {
    upload_id: upload.upload_id,
    manifest_sha256: upload.manifest_sha256!,
    idempotency_key: 'confirm:original:1',
    confirmed: true,
  };
}
export function makeReceipt(
  upload: OriginalUpload,
  input = confirmRequest(upload),
): OriginalReceipt {
  return {
    import_task_id: '1'.repeat(32),
    domain_id: upload.domain_id,
    registration_id: upload.registration_id,
    upload_id: upload.upload_id,
    idempotency_key: input.idempotency_key,
    manifest_sha256: input.manifest_sha256,
    status: 'succeeded',
    save_status: upload.action!,
    model_calls: 0,
    source: makeRevision(upload),
    created_at: upload.updated_at,
  };
}
export function respond(data: unknown, status = 200) {
  return {
    status,
    data: {
      code: 0,
      message: '',
      data,
      contract_version: '1.0.0',
      source_contract_version: '0.5.0',
      request_id: '9'.repeat(32),
    },
  };
}
export function failure(status: number, code: string) {
  return {
    response: {
      status,
      headers: {
        'content-type': 'application/json',
        'x-request-id': '9'.repeat(32),
        'x-knowledge-error-code': code,
      },
      data: {
        code,
        message: '',
        data: null,
        contract_version: '1.0.0',
        source_contract_version: '0.5.0',
        request_id: '9'.repeat(32),
      },
    },
  };
}
export function binaryResponse(
  source: OriginalRevision,
  data = blob(),
  status = 200,
  extra: Record<string, string> = {},
) {
  return {
    status,
    data,
    headers: {
      'content-type': source.media_type,
      'content-length': String(data.size),
      etag: '"sha256-' + source.sha256 + '"',
      'accept-ranges': 'bytes',
      'cache-control': 'no-store',
      'content-disposition': 'attachment; filename="original.pdf"',
      'x-content-type-options': 'nosniff',
      'x-knowledge-contract-version': '1.0.0',
      'x-knowledge-source-contract-version': '0.5.0',
      ...extra,
    },
  };
}

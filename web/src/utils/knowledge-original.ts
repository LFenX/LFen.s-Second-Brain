import dayjs from 'dayjs';
import type {
  OriginalRegistration,
  OriginalUpload,
  OriginalReceipt,
  OriginalRevision,
  OriginalClassification,
} from '@/interfaces/database/knowledge-original';
import type {
  OriginalRegistrationRequest,
  CreateOriginalUpload,
  ConfirmOriginalImport,
} from '@/interfaces/request/knowledge-original';
import Contract from '../../../docs/knowledge-original-import-v1.openapi.json';
import {
  OriginalId,
  OriginalSha,
  OriginalKey,
  OriginalMaxBytes,
  validOriginalData,
} from './knowledge-original-schema';

const Encoder = new TextEncoder();
export async function originalSha256(bytes: ArrayBuffer | Uint8Array) {
  const hash = await crypto.subtle.digest('SHA-256', bytes as BufferSource);
  return Array.from(new Uint8Array(hash), (b) =>
    b.toString(16).padStart(2, '0'),
  ).join('');
}
function field(value: Record<string, any>, path: string): unknown {
  return path
    .split('.')
    .reduce<any>(
      (current, key) => (key === 'length' ? current.length : current[key]),
      value,
    );
}
export async function originalDigest(
  kind: 'registration' | 'request' | 'preview',
  value: Record<string, any>,
) {
  const projection = Contract['x-digest-projection'][kind];
  const tokens: unknown[] = [projection.prefix];
  for (const path of projection.fields) {
    if (path === 'include_paths[]') tokens.push(...value.include_paths);
    else tokens.push(field(value, path));
  }
  const encoded = tokens.map((token) => {
    if (token === undefined) throw Error('Missing digest field');
    const text = token === null ? '' : String(token);
    return Encoder.encode(Encoder.encode(text).length + ':' + text);
  });
  const bytes = new Uint8Array(
    encoded.reduce((size, item) => size + item.length, 0),
  );
  let offset = 0;
  for (const item of encoded) {
    bytes.set(item, offset);
    offset += item.length;
  }
  return originalSha256(bytes);
}
export function sameOriginalClassification(
  a: OriginalClassification,
  b: OriginalClassification,
) {
  return (
    a.group_id === b.group_id &&
    a.component_side === b.component_side &&
    a.language === b.language
  );
}
export function originalRegistrationRequest(
  reg: OriginalRegistration,
): OriginalRegistrationRequest {
  return {
    registration_key: reg.registration_key,
    name: reg.name,
    source_kind: reg.source_kind,
    input_mode: reg.input_mode,
    include_paths: reg.include_paths,
    default_classification: reg.default_classification,
  };
}
export function originalUploadRequest(
  upload: OriginalUpload,
): CreateOriginalUpload {
  return {
    original_filename: upload.original_filename,
    relative_path: upload.relative_path,
    classification: upload.classification,
    declared_byte_length: upload.declared_byte_length,
    declared_media_type: upload.declared_media_type,
    expected_sha256: upload.expected_sha256,
    expected_revision_id: upload.expected_revision_id,
    idempotency_key: upload.idempotency_key,
  };
}
export async function checkOriginalUpload(
  upload: OriginalUpload,
  request: CreateOriginalUpload,
  domain: string,
  owner: string,
  registration: string,
) {
  if (
    upload.domain_id !== domain ||
    upload.owner_id !== owner ||
    upload.registration_id !== registration ||
    upload.idempotency_key !== request.idempotency_key ||
    (await originalDigest('request', {
      ...request,
      domain_id: domain,
      owner_id: owner,
      registration_id: registration,
    })) !== upload.request_sha256 ||
    (await originalDigest('request', upload)) !== upload.request_sha256 ||
    !sameOriginalClassification(upload.classification, request.classification)
  )
    throw Error('Upload request identity changed');
  if (
    (upload.state === 'ready' || upload.state === 'committed') &&
    (await originalDigest('preview', upload)) !== upload.manifest_sha256
  )
    throw Error('Original preview digest changed');
  return upload;
}
export function latestOriginalUpload(
  previous: OriginalUpload | null,
  next: OriginalUpload,
) {
  if (!previous) return next;
  const fixed = [
    'upload_id',
    'domain_id',
    'owner_id',
    'registration_id',
    'idempotency_key',
    'request_sha256',
    'original_filename',
    'relative_path',
    'classification',
    'declared_byte_length',
    'declared_media_type',
    'expected_sha256',
    'expected_revision_id',
    'source_id',
    'policy_version',
    'created_at',
    'expires_at',
  ];
  for (const key of fixed)
    if (
      key === 'classification'
        ? !sameOriginalClassification(
            previous.classification,
            next.classification,
          )
        : previous[key as keyof OriginalUpload] !==
          next[key as keyof OriginalUpload]
    )
      throw Error('Upload identity changed');
  if (next.revision < previous.revision) return previous;
  if (
    (['rejected', 'cancelled', 'expired', 'committed'].includes(
      previous.state,
    ) &&
      next.state !== previous.state) ||
    (previous.state === 'ready' &&
      !['ready', 'cancelled', 'expired', 'committed'].includes(next.state)) ||
    (previous.state !== 'awaiting_bytes' && next.state === 'awaiting_bytes')
  )
    throw Error('Upload state reopened');
  if (
    next.revision === previous.revision &&
    Object.keys(previous).some(
      (key) =>
        key !== 'classification' &&
        previous[key as keyof OriginalUpload] !==
          next[key as keyof OriginalUpload],
    )
  )
    throw Error('Conflicting upload revision');
  if (
    previous.manifest_sha256 &&
    (next.state === 'ready' || next.state === 'committed') &&
    ['sha256', 'byte_length', 'media_type', 'action', 'manifest_sha256'].some(
      (key) =>
        previous[key as keyof OriginalUpload] !==
        next[key as keyof OriginalUpload],
    )
  )
    throw Error('Ready preview changed');
  return next;
}
export function checkOriginalReceipt(
  receipt: OriginalReceipt,
  domain: string,
  registration: string,
  request: ConfirmOriginalImport,
  upload?: OriginalUpload,
) {
  if (
    receipt.domain_id !== domain ||
    receipt.registration_id !== registration ||
    receipt.upload_id !== request.upload_id ||
    receipt.manifest_sha256 !== request.manifest_sha256 ||
    receipt.idempotency_key !== request.idempotency_key
  )
    throw Error('Receipt identity changed');
  if (upload) {
    const source = receipt.source;
    if (
      receipt.save_status !== upload.action ||
      source.original_filename !== upload.original_filename ||
      source.relative_path !== upload.relative_path ||
      source.sha256 !== upload.sha256 ||
      source.byte_length !== upload.byte_length ||
      source.media_type !== upload.media_type ||
      !sameOriginalClassification(
        source.classification,
        upload.classification,
      ) ||
      (upload.source_id && source.source_id !== upload.source_id) ||
      (upload.action === 'unchanged' &&
        source.source_revision_id !== upload.expected_revision_id)
    )
      throw Error('Receipt source changed');
  }
  return receipt;
}
type RecoveryScope = {
  contract_version: '1.0.0';
  source_contract_version: '0.5.0';
  domain_id: string;
  owner_id: string;
};
export type OriginalCreatePending = RecoveryScope &
  (
    | {
        kind: 'registration';
        registration_key: string;
        config_sha256: string;
        registration_id: string | null;
      }
    | {
        kind: 'upload';
        registration_id: string;
        request: CreateOriginalUpload;
        request_sha256: string;
        upload_id: string | null;
      }
  );
export type OriginalConfirmPending = RecoveryScope & {
  registration_id: string;
  request: ConfirmOriginalImport;
  import_task_id: string | null;
};
export function originalScope(domain: string, owner: string): RecoveryScope {
  if (!OriginalId.test(domain) || !OriginalId.test(owner))
    throw Error('Invalid native owner');
  return {
    contract_version: '1.0.0',
    source_contract_version: '0.5.0',
    domain_id: domain,
    owner_id: owner,
  };
}
export function originalStorageKey(domain: string, owner: string) {
  originalScope(domain, owner);
  return 'lfen-original:0.5.0:' + owner + ':' + domain;
}
function validRecovery(
  value: any,
  kind: 'create' | 'confirm',
  domain: string,
  owner: string,
) {
  if (
    !value ||
    value.contract_version !== '1.0.0' ||
    value.source_contract_version !== '0.5.0' ||
    value.domain_id !== domain ||
    value.owner_id !== owner ||
    !OriginalId.test(domain) ||
    !OriginalId.test(owner)
  )
    return false;
  const scope = [
    'contract_version',
    'source_contract_version',
    'domain_id',
    'owner_id',
  ];
  const keys =
    kind === 'confirm'
      ? [...scope, 'registration_id', 'request', 'import_task_id']
      : value.kind === 'registration'
        ? [
            ...scope,
            'kind',
            'registration_key',
            'config_sha256',
            'registration_id',
          ]
        : [
            ...scope,
            'kind',
            'registration_id',
            'request',
            'request_sha256',
            'upload_id',
          ];
  if (Object.keys(value).sort().join(',') !== keys.sort().join(','))
    return false;
  if (kind === 'confirm')
    return (
      OriginalId.test(value.registration_id) &&
      (value.import_task_id === null ||
        OriginalId.test(value.import_task_id)) &&
      validOriginalData('Confirm', value.request)
    );
  if (value.kind === 'registration')
    return (
      OriginalKey.test(value.registration_key) &&
      value.registration_key.length <= 64 &&
      OriginalSha.test(value.config_sha256) &&
      (value.registration_id === null || OriginalId.test(value.registration_id))
    );
  return (
    value.kind === 'upload' &&
    OriginalId.test(value.registration_id) &&
    validOriginalData('CreateUpload', value.request) &&
    OriginalSha.test(value.request_sha256) &&
    (value.upload_id === null || OriginalId.test(value.upload_id))
  );
}
export function saveOriginalRecovery(
  key: string,
  kind: 'create' | 'confirm',
  value: OriginalCreatePending | OriginalConfirmPending,
) {
  const text = JSON.stringify(value);
  if (
    !validRecovery(value, kind, value.domain_id, value.owner_id) ||
    Encoder.encode(text).length > 32768
  )
    throw Error('Invalid recovery metadata');
  localStorage.setItem(key + ':' + kind, text);
}
export function loadOriginalRecovery(
  key: string,
  kind: 'create',
  domain: string,
  owner: string,
): OriginalCreatePending | null;
export function loadOriginalRecovery(
  key: string,
  kind: 'confirm',
  domain: string,
  owner: string,
): OriginalConfirmPending | null;
export function loadOriginalRecovery(
  key: string,
  kind: 'create' | 'confirm',
  domain: string,
  owner: string,
): OriginalCreatePending | OriginalConfirmPending | null {
  const text = localStorage.getItem(key + ':' + kind);
  if (!text) return null;
  if (Encoder.encode(text).length > 32768)
    throw Error('Invalid recovery metadata');
  const value = JSON.parse(text);
  if (!validRecovery(value, kind, domain, owner))
    throw Error('Invalid recovery metadata');
  return value;
}
export function clearOriginalRecovery(key: string, kind: 'create' | 'confirm') {
  localStorage.removeItem(key + ':' + kind);
}
export const originalUnexpired = (upload: OriginalUpload) =>
  dayjs(upload.expires_at).isAfter(dayjs());
export function originalRevisionLink(source: OriginalRevision) {
  const params = new URLSearchParams({
    mode: 'original',
    original_revision: source.source_revision_id,
    original_source: source.source_id,
    original_sha: source.sha256,
  });
  return (
    '/dataset/knowledge-import/' + source.domain_id + '?' + params.toString()
  );
}
export async function verifyOriginalBlob(blob: Blob, source: OriginalRevision) {
  if (
    blob.size < 1 ||
    blob.size > OriginalMaxBytes ||
    blob.size !== source.byte_length ||
    (await originalSha256(await blob.arrayBuffer())) !== source.sha256
  )
    throw Error('Original download integrity failed');
  return blob;
}

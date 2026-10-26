import Contract from '../../../docs/knowledge-web-import-v1.openapi.json';
import type {
  WebCapture,
  WebRegistration,
  WebEntry,
  WebClassification,
  WebPreview,
  WebPreviewItem,
  WebReceipt,
  WebReceiptItem,
  WebRevision,
  WebRecoverySlot,
  WebReference,
} from '@/interfaces/database/knowledge-web';
import type {
  WebCreateCapture,
  WebCreateRegistration,
  WebCreatePreview,
  WebConfirm,
} from '@/interfaces/request/knowledge-web';
import {
  compareWebBytes,
  validWebData,
  WebId,
  WebKey,
  WebVersions,
  webJsonBytes,
} from './knowledge-web-schema';

const Encoder = new TextEncoder();
function lp(value: string) {
  return Encoder.encode(value).length + ':' + value;
}
export function encodeWebValue(value: unknown): string {
  if (value === null) return lp('n');
  if (typeof value === 'string') {
    if (
      new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
        Encoder.encode(value),
      ) !== value
    )
      throw Error('Invalid Unicode');
    return lp('s') + lp(value);
  }
  if (typeof value === 'boolean') return lp('b') + lp(value ? '1' : '0');
  if (typeof value === 'number' && Number.isSafeInteger(value))
    return lp('i') + lp(String(value));
  if (Array.isArray(value))
    return (
      lp('a') + lp(String(value.length)) + value.map(encodeWebValue).join('')
    );
  if (
    value &&
    typeof value === 'object' &&
    Object.getPrototypeOf(value) === Object.prototype
  ) {
    const keys = Object.keys(value).sort(compareWebBytes);
    return (
      lp('o') +
      lp(String(keys.length)) +
      keys
        .map(
          (key) =>
            lp(key) + encodeWebValue((value as Record<string, unknown>)[key]),
        )
        .join('')
    );
  }
  throw Error('Unsupported typed-LP value');
}
export async function webSha256(value: BufferSource) {
  return Array.from(
    new Uint8Array(await crypto.subtle.digest('SHA-256', value)),
  )
    .map((n) => n.toString(16).padStart(2, '0'))
    .join('');
}
export type WebDigestKind = keyof (typeof Contract)['x-digest-projection'];
export function webDigestEncoding(
  kind: WebDigestKind,
  value: Record<string, unknown>,
) {
  const rule = Contract['x-digest-projection'][kind];
  return (
    lp(rule.prefix) +
    encodeWebValue(
      Object.fromEntries(rule.fields.map((field) => [field, value[field]])),
    )
  );
}
export function webDigest(kind: WebDigestKind, value: object) {
  return webSha256(
    Encoder.encode(webDigestEncoding(kind, value as Record<string, unknown>)),
  );
}
export function equalWebValue(a: unknown, b: unknown) {
  return encodeWebValue(a) === encodeWebValue(b);
}
export function webRegistrationRequest(
  value: WebRegistration,
): WebCreateRegistration {
  const {
    registration_key,
    name,
    source_kind,
    origin_url,
    producer,
    default_classification,
    network_scope,
  } = value;
  return {
    registration_key,
    name,
    source_kind,
    origin_url,
    producer,
    default_classification,
    network_scope,
  };
}
export function webCaptureProjection(
  registration: string,
  value: WebCreateCapture | WebCapture,
) {
  const local = value.acquisition_mode === 'local_snapshot';
  return {
    registration_id: registration,
    registration_config_sha256: value.registration_config_sha256,
    acquisition_mode: value.acquisition_mode,
    local_declaration: local
      ? {
          original_filename: value.original_filename,
          declared_byte_length: value.declared_byte_length,
          declared_media_type: value.declared_media_type,
          expected_sha256: value.expected_sha256,
          declared_final_url:
            'provenance' in value
              ? value.provenance.final_url
              : value.declared_final_url,
          claimed_captured_at:
            'provenance' in value
              ? value.provenance.claimed_captured_at
              : value.claimed_captured_at,
        }
      : null,
  };
}
export function webSourceProjection(
  value: WebEntry | WebRevision,
  classification: WebClassification,
) {
  const decoded = value.representations.find(
      (item) => item.kind === 'decoded_content',
    )!,
    reading = value.representations.find(
      (item) => item.kind === 'safe_reading',
    )!;
  return {
    entry_key: value.entry_key,
    decoded_sha256: decoded.sha256,
    decoded_byte_length: decoded.byte_length,
    decoded_media_type: decoded.media_type,
    reading_sha256: reading.sha256,
    reading_byte_length: reading.byte_length,
    producer_versions: value.producer_versions,
    metadata_sha256: value.metadata_sha256,
    references_sha256: value.references_sha256,
    classification,
  };
}
export async function checkWebEntry(
  value: WebEntry | WebRevision,
  classification?: WebClassification,
) {
  if (
    (await webDigest('entry_key', value)) !== value.entry_key ||
    (await webDigest('metadata', value.metadata)) !== value.metadata_sha256
  )
    throw Error('Invalid entry digest');
  if (
    classification &&
    (await webDigest(
      'source_input',
      webSourceProjection(value, classification),
    )) !== value.source_input_sha256
  )
    throw Error('Invalid input digest');
}
export function latestWebResource<T extends { revision: number }>(
  current: T | null,
  next: T,
): T {
  if (!current) return next;
  if (next.revision < current.revision) return current;
  if (next.revision === current.revision && !equalWebValue(current, next))
    throw Error('Contradictory resource revision');
  return next;
}
export function webStoredJsonBytes(value: unknown) {
  return Encoder.encode(
    JSON.stringify(value).replace(
      /[<>&\u2028\u2029]/g,
      (c) => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'),
    ),
  ).length;
}
export async function checkWebPreview(
  preview: WebPreview,
  request: WebCreatePreview,
  items: WebPreviewItem[],
  entries: WebEntry[],
  references: Record<string, WebReference[]>,
) {
  if (
    preview.capture_id !== request.capture_id ||
    preview.observation_id !== request.observation_id ||
    preview.observation_sha256 !== request.observation_sha256 ||
    preview.idempotency_key !== request.idempotency_key ||
    preview.selected_count !== request.selection.length ||
    items.length !== request.selection.length ||
    (await webDigest('selection_request', request)) !==
      preview.request_sha256 ||
    (await webDigest('selection', request)) !== preview.selection_sha256
  )
    throw Error('Preview request mismatch');
  let bytes = 0;
  for (let i = 0; i < items.length; i++) {
    const item = items[i],
      selected = request.selection[i],
      entry = entries.find((entry) => entry.entry_key === selected.entry_key);
    if (
      !entry ||
      item.entry_key !== selected.entry_key ||
      !equalWebValue(item.classification, selected.classification) ||
      item.expected_revision_id !== selected.expected_revision_id ||
      item.decoded_sha256 !== entry.representations[0].sha256 ||
      item.reading_sha256 !== entry.representations[1].sha256 ||
      item.body_completeness !== entry.body_completeness ||
      item.source_input_sha256 !==
        (await webDigest(
          'source_input',
          webSourceProjection(entry, selected.classification),
        ))
    )
      throw Error('Preview item mismatch');
    if (!references[entry.entry_key]) throw Error('Missing references');
    bytes +=
      entry.decoded_byte_length +
      entry.reading_byte_length +
      webStoredJsonBytes(entry.metadata) +
      webStoredJsonBytes(references[entry.entry_key]);
  }
  if (
    bytes !== preview.selected_byte_length ||
    (await webDigest('preview_manifest', { ...preview, items })) !==
      preview.manifest_sha256
  )
    throw Error('Preview manifest mismatch');
}
export async function checkWebReceipt(
  value: WebReceipt,
  request: WebConfirm,
  preview: WebPreview,
  items: WebReceiptItem[],
  expected: WebPreviewItem[],
) {
  if (
    value.preview_id !== request.preview_id ||
    value.manifest_sha256 !== request.manifest_sha256 ||
    value.idempotency_key !== request.idempotency_key ||
    value.request_sha256 !== (await webDigest('confirmation', request)) ||
    value.preview_request_sha256 !== preview.request_sha256 ||
    value.capture_id !== preview.capture_id ||
    value.capture_sha256 !== preview.capture_sha256 ||
    value.observation_id !== preview.observation_id ||
    value.observation_sha256 !== preview.observation_sha256 ||
    value.policy_version !== preview.policy_version ||
    !equalWebValue(value.producer_versions, preview.producer_versions) ||
    preview.state !== 'committed' ||
    preview.import_task_id !== value.import_task_id ||
    value.selection_sha256 !== preview.selection_sha256 ||
    value.selected_count !== expected.length ||
    items.length !== expected.length ||
    !equalWebValue(
      request.acknowledged_entry_keys,
      expected.map((item) => item.entry_key),
    )
  )
    throw Error('Receipt mismatch');
  for (let i = 0; i < items.length; i++)
    for (const key of [
      'entry_key',
      'classification',
      'source_input_sha256',
      'action',
      'decoded_sha256',
      'reading_sha256',
    ] as const)
      if (!equalWebValue(items[i][key], expected[i][key]))
        throw Error('Receipt item mismatch');
  for (let i = 0; i < items.length; i++)
    if (
      (expected[i].source_id !== null &&
        items[i].source_id !== expected[i].source_id) ||
      (expected[i].action === 'unchanged' &&
        items[i].source_revision_id !== expected[i].expected_revision_id)
    )
      throw Error('Receipt revision binding mismatch');
}
export function webStorageKey(domain: string, owner: string) {
  if (!WebId.test(domain) || !WebId.test(owner))
    throw Error('Invalid recovery scope');
  return 'knowledge-web:0.6:' + owner + ':' + domain;
}
export type WebRegistrationPending = {
  kind: 'registration';
  owner_id: string;
  domain_id: string;
  request: WebCreateRegistration;
  config_sha256: string;
};
export function readWebSlot(
  key: string,
  domain: string,
  owner: string,
): WebRecoverySlot | null {
  const raw = localStorage.getItem(key);
  if (!raw) return null;
  if (Encoder.encode(raw).length > 32768)
    throw Error('Recovery slot exceeds budget');
  const value = JSON.parse(raw);
  if (
    !validWebData('RecoverySlot', value) ||
    value.domain_id !== domain ||
    value.owner_id !== owner
  )
    throw Error('Invalid recovery slot');
  return value;
}
export function writeWebSlot(key: string, value: WebRecoverySlot) {
  if (!validWebData('RecoverySlot', value) || webJsonBytes(value) > 32768)
    throw Error('Invalid recovery slot');
  localStorage.setItem(key, JSON.stringify(value));
}
export function newWebSlot(
  owner: string,
  domain: string,
  reg: string,
  request: WebCreateCapture,
): WebRecoverySlot {
  return {
    slot_version: 'web-recovery/0.6',
    owner_id: owner,
    domain_id: domain,
    registration_id: reg,
    capture_create_key: request.idempotency_key,
    capture_create_request: request,
    capture_id: null,
    capture_sha256: null,
    producer_versions: WebVersions,
    observation_id: null,
    preview_create_key: null,
    preview_create_request: null,
    preview_id: null,
    preview_request_sha256: null,
    confirmation_request: null,
    receipt_id: null,
  };
}
export function webNewKey(): string {
  const value = crypto.randomUUID();
  if (!WebKey.test(value)) throw Error('Invalid generated key');
  return value;
}
export function webRevisionLink(
  domain: string,
  value: Pick<
    WebRevision,
    'source_id' | 'source_revision_id' | 'source_input_sha256'
  >,
) {
  return (
    '/dataset/knowledge-import/' +
    domain +
    '?' +
    new URLSearchParams({
      mode: 'web',
      web_revision: value.source_revision_id,
      web_source: value.source_id,
      web_input_sha: value.source_input_sha256,
    })
  );
}
export function webCaptureAction(
  mode: string,
  state: string,
  action: string,
  hasAttempt = false,
) {
  if (
    (action === 'acquire' && mode !== 'explicit_fetch') ||
    (action === 'upload' && mode !== 'local_snapshot')
  )
    return 'mode_conflict';
  if (action === 'acquire' && hasAttempt) return 'replay_no_fetch';
  if (['expired', 'cancelled'].includes(state)) return 'expired_or_cancelled';
  return action === 'acquire'
    ? state === 'awaiting_fetch'
      ? 'start'
      : 'state_conflict'
    : ['awaiting_bytes', 'interrupted'].includes(state)
      ? 'start'
      : 'state_conflict';
}

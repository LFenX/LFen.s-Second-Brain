import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import dayjs from 'dayjs';
import Contract from '../../../docs/knowledge-web-import-v1.openapi.json';

export const WebId = /^[0-9a-f]{32}$/;
export const WebSha = /^[0-9a-f]{64}$/;
export const WebKey = /^[A-Za-z0-9._:-]{1,128}$/;
export const WebLimits = Contract['x-budget'];
export const WebVersions = {
  normalizer_version: 'web-normalizer/0.6.0',
  converter_version: 'web-reading/0.6.0',
  sanitizer_version: 'web-sanitizer/0.6.0',
  resource_policy_version: 'web-resources/0.6.0',
} as const;
const Validator = new Ajv({ strict: false });
addFormats(Validator);
const Schemas = Object.fromEntries(
  Object.keys(Contract.components.schemas).map((name) => [
    name,
    Validator.compile({
      components: Contract.components,
      $ref: '#/components/schemas/' + name,
    }),
  ]),
);
const Encoder = new TextEncoder();
export function webJsonBytes(value: unknown) {
  return Encoder.encode(JSON.stringify(value)).length;
}
export function compareWebBytes(a: string, b: string) {
  const left = Encoder.encode(a),
    right = Encoder.encode(b);
  for (let i = 0; i < Math.min(left.length, right.length); i++)
    if (left[i] !== right[i]) return left[i] - right[i];
  return left.length - right.length;
}
export function webSorted(values: string[]) {
  return values.every(
    (value, i) => !i || compareWebBytes(values[i - 1], value) < 0,
  );
}
export function webHttpUrl(value: string) {
  try {
    const url = new URL(value);
    return (
      /^https?:$/.test(url.protocol) &&
      !url.username &&
      !url.password &&
      !Array.from(value).some(
        (character) =>
          character.codePointAt(0)! <= 32 || character.codePointAt(0) === 127,
      )
    );
  } catch {
    return false;
  }
}
export function webOriginAllowed(value: string, origins: string[]) {
  return webHttpUrl(value) && origins.includes(new URL(value).origin);
}
export function validWebStructure(schema: string, value: unknown) {
  return !!Schemas[schema]?.(value);
}
function representations(value: Record<string, any>) {
  const items = value.representations;
  return (
    items.length === 2 &&
    items[0].kind === 'decoded_content' &&
    items[1].kind === 'safe_reading' &&
    items.every(
      (item: Record<string, any>) =>
        item.converter_version === WebVersions.converter_version &&
        (item.kind === 'decoded_content'
          ? item.sanitizer_version === null &&
            item.resource_policy_version === null
          : item.sanitizer_version === WebVersions.sanitizer_version &&
            item.resource_policy_version ===
              WebVersions.resource_policy_version),
    )
  );
}
export function validWebData(
  schema: string,
  input: unknown,
  httpStatus?: number,
): boolean {
  if (!validWebStructure(schema, input)) return false;
  const name = schema.endsWith('Envelope') ? schema.slice(0, -8) : schema;
  const value = (
    schema.endsWith('Envelope') ? (input as { data: unknown }).data : input
  ) as Record<string, any>;
  if (name.endsWith('Page'))
    return value.items.every((item: unknown) =>
      validWebData(
        (
          {
            EntryPage: 'EntrySummary',
            ReferencePage: 'Reference',
            PreviewItemPage: 'PreviewItem',
            ReceiptItemPage: 'ReceiptItem',
            RevisionPage: 'WebRevision',
          } as Record<string, string>
        )[name],
        item,
      ),
    );
  if (name === 'Error')
    return (
      httpStatus === undefined ||
      ([
        400, 401, 404, 409, 410, 413, 415, 416, 422, 429, 502, 503, 504,
      ].includes(httpStatus) &&
        (value.failure === null || value.failure.machine_code === value.code) &&
        (httpStatus !== 401 || value.code === 'knowledge_unauthorized') &&
        (value.code !== 'web_upstream_http_error' || httpStatus === 502))
    );
  if (name === 'CreateRegistration' || name === 'Registration') {
    if (!webHttpUrl(value.origin_url)) return false;
    const origins = value.network_scope.allowed_origins;
    if (
      !webSorted(origins) ||
      !origins.every(
        (origin: string) =>
          webHttpUrl(origin) && new URL(origin).origin === origin,
      )
    )
      return false;
    return (
      !value.network_scope.enabled ||
      webOriginAllowed(value.origin_url, origins)
    );
  }
  if (name === 'CreateLocalCapture')
    return (
      !/[/\\]/u.test(value.original_filename) &&
      !Array.from(value.original_filename as string).some(
        (character) =>
          character.codePointAt(0)! < 32 || character.codePointAt(0) === 127,
      ) &&
      (value.declared_final_url === null ||
        webHttpUrl(value.declared_final_url))
    );
  if (name === 'CreateCapture')
    return validWebData(
      value.acquisition_mode === 'local_snapshot'
        ? 'CreateLocalCapture'
        : 'CreateFetchCapture',
      value,
    );
  if (name === 'CreatePreview')
    return webSorted(
      value.selection.map((item: { entry_key: string }) => item.entry_key),
    );
  if (name === 'Confirm') return webSorted(value.acknowledged_entry_keys);
  if (name === 'Capture') {
    if (
      dayjs(value.expires_at).valueOf() - dayjs(value.created_at).valueOf() !==
        1800000 ||
      dayjs(value.updated_at).isBefore(value.created_at)
    )
      return false;
    if (
      value.acquisition_mode === 'local_snapshot' &&
      (value.original_filename === null ||
        value.declared_byte_length === null ||
        value.declared_media_type === null ||
        value.received_bytes > value.declared_byte_length ||
        value.provenance.kind !== 'user_supplied')
    )
      return false;
    if (
      value.acquisition_mode === 'explicit_fetch' &&
      (value.original_filename !== null ||
        value.declared_byte_length !== null ||
        value.declared_media_type !== null ||
        value.expected_sha256 !== null ||
        value.provenance.kind !== 'server_http')
    )
      return false;
    if (
      value.state === 'ready' &&
      (value.raw_sha256 === null ||
        value.raw_byte_length === null ||
        value.media_type === null ||
        value.received_bytes !== value.raw_byte_length ||
        value.failure !== null)
    )
      return false;
    return (
      value.state !== 'ready' ||
      value.acquisition_mode !== 'local_snapshot' ||
      (value.raw_byte_length === value.declared_byte_length &&
        (value.expected_sha256 === null ||
          value.expected_sha256 === value.raw_sha256))
    );
  }
  if (name === 'Observation')
    return (
      value.state !== 'ready' ||
      (value.observation_sha256 !== null &&
        value.capture_traversal_complete &&
        value.entry_count !== null &&
        value.input_entry_count !== null &&
        value.eligible_count <= value.entry_count &&
        value.entry_count <= value.input_entry_count &&
        value.duplicate_count === value.input_entry_count - value.entry_count &&
        value.failure === null)
    );
  if (name === 'EntrySummary' || name === 'Entry') {
    if (
      value.eligible !== (value.ineligible_reason === null) ||
      value.eligible !== (value.source_input_sha256 !== null) ||
      (value.source_id === null) !== (value.current_revision_id === null)
    )
      return false;
    if (name === 'Entry')
      return (
        representations(value) &&
        value.positions[0] === value.first_position &&
        value.positions.every(
          (n: number, i: number) => !i || value.positions[i - 1] < n,
        ) &&
        value.representations[0].byte_length === value.decoded_byte_length &&
        value.representations[1].byte_length === value.reading_byte_length &&
        webJsonBytes(value.metadata) <= 8192
      );
  }
  if (name === 'WebRevision')
    return (
      representations(value) &&
      webJsonBytes(value.metadata) <= 8192 &&
      value.positions.every(
        (n: number, i: number) => !i || value.positions[i - 1] < n,
      )
    );
  if (name === 'Preview')
    return (
      (value.state === 'committed') === (value.import_task_id !== null) &&
      dayjs(value.expires_at).isAfter(value.created_at) &&
      dayjs(value.expires_at).valueOf() - dayjs(value.created_at).valueOf() <=
        600000
    );
  if (name === 'PreviewItem')
    return (
      value.acknowledgement ===
        (
          {
            summary: 'summary_only',
            unknown: 'unknown_completeness',
            external_reference: 'external_reference_only',
          } as Record<string, string>
        )[value.body_completeness] &&
      (value.source_id === null) === (value.expected_revision_id === null) &&
      (value.action !== 'created' || value.source_id === null)
    );
  if (name === 'Receipt')
    return value.saved_count + value.unchanged_count === value.selected_count;
  if (name === 'Window')
    return (
      value.end_byte >= value.start_byte &&
      value.end_byte <= value.representation.byte_length &&
      value.end_byte - value.start_byte <= 4096 &&
      Encoder.encode(value.text).length === value.end_byte - value.start_byte &&
      value.next_start_byte ===
        (value.end_byte < value.representation.byte_length
          ? value.end_byte
          : null) &&
      (value.end_byte > value.start_byte ||
        value.representation.byte_length === 0)
    );
  if (name === 'Reference')
    return (
      webJsonBytes(value) <= 16384 &&
      (!value.open_allowed ||
        (value.resolved_url !== null && webHttpUrl(value.resolved_url)))
    );
  if (name === 'RecoverySlot') {
    if (
      value.capture_create_key !==
        value.capture_create_request.idempotency_key ||
      !validWebData('CreateCapture', value.capture_create_request)
    )
      return false;
    if (
      (value.preview_create_key === null) !==
      (value.preview_create_request === null)
    )
      return false;
    if (
      value.preview_create_request &&
      (value.preview_create_key !==
        value.preview_create_request.idempotency_key ||
        value.capture_id !== value.preview_create_request.capture_id ||
        value.observation_id !== value.preview_create_request.observation_id ||
        !validWebData('CreatePreview', value.preview_create_request))
    )
      return false;
    return (
      value.confirmation_request === null ||
      (value.preview_id === value.confirmation_request.preview_id &&
        validWebData('Confirm', value.confirmation_request))
    );
  }
  return true;
}

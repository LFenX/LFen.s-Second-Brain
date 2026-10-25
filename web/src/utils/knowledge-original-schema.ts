import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import dayjs from 'dayjs';
import Contract from '../../../docs/knowledge-original-import-v1.openapi.json';
import {
  compareDirectoryPaths,
  validDirectoryPath,
} from './knowledge-directory-schema';

export const OriginalMaxBytes = 67108864;
export const OriginalId = /^[a-f0-9]{32}$/;
export const OriginalSha = /^[a-f0-9]{64}$/;
export const OriginalKey = /^[A-Za-z0-9._:-]{1,128}$/;
export const OriginalMediaTypes = Contract['x-media-rules'].declarations;
const Encoder = new TextEncoder();
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
export function originalFamily(name: string): string | null {
  const extension = /\.[^./]+$/.exec(name)?.[0].toLowerCase();
  if (extension === '.pdf') return 'application/pdf';
  if (extension === '.docx')
    return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';
  if (extension === '.pptx')
    return 'application/vnd.openxmlformats-officedocument.presentationml.presentation';
  return extension &&
    Contract['x-media-rules'].text_extensions.includes(extension)
    ? 'text/plain'
    : null;
}
export function validOriginalFilename(name: string) {
  const characters = Array.from(name);
  return (
    characters.length >= 1 &&
    characters.length <= 256 &&
    !/[/\\:]/u.test(name) &&
    !characters.some(
      (character) =>
        character.codePointAt(0)! < 32 || character.codePointAt(0) === 127,
    ) &&
    name !== '.' &&
    name !== '..' &&
    new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      Encoder.encode(name),
    ) === name
  );
}
function names(value: Record<string, any>, actual = false) {
  const media = originalFamily(value.original_filename);
  return (
    validOriginalFilename(value.original_filename) &&
    validDirectoryPath(value.relative_path) &&
    !!media &&
    originalFamily(value.relative_path) === media &&
    (actual
      ? value.media_type === media
      : value.declared_media_type === 'application/octet-stream' ||
        value.declared_media_type === media)
  );
}
export function validOriginalStructure(
  schema: string,
  input: unknown,
): boolean {
  return !!Schemas[schema]?.(input);
}
export function validOriginalData(
  schema: string,
  input: unknown,
  httpStatus?: number,
): boolean {
  if (!validOriginalStructure(schema, input)) return false;
  const name = schema.endsWith('Envelope') ? schema.slice(0, -8) : schema;
  const value = (
    schema.endsWith('Envelope') ? (input as { data: unknown }).data : input
  ) as Record<string, any>;
  if (name === 'Error' && httpStatus !== undefined)
    return (
      (Contract['x-error-http-statuses'] as Record<string, number>)[
        value.code
      ] === httpStatus
    );
  if (name === 'CreateRegistration' || name === 'Registration')
    return value.include_paths.every(
      (path: string, i: number) =>
        validDirectoryPath(path) &&
        !!originalFamily(path) &&
        (!i || compareDirectoryPaths(value.include_paths[i - 1], path) < 0),
    );
  if (name === 'CreateUpload') return names(value);
  if (name === 'Upload') {
    const ttl =
      dayjs(value.expires_at).valueOf() - dayjs(value.created_at).valueOf();
    if (
      !names(value) ||
      ttl !== 1800000 ||
      dayjs(value.updated_at).isBefore(value.created_at) ||
      value.received_bytes > value.declared_byte_length
    )
      return false;
    if (value.state === 'ready' || value.state === 'committed') {
      if (
        !names(value, true) ||
        value.byte_length !== value.declared_byte_length ||
        value.received_bytes !== value.byte_length ||
        (value.expected_sha256 !== null &&
          value.expected_sha256 !== value.sha256)
      )
        return false;
      if (
        value.action === 'created'
          ? value.source_id !== null || value.expected_revision_id !== null
          : value.source_id === null || value.expected_revision_id === null
      )
        return false;
    }
  }
  if (name === 'OriginalRevision') return names(value, true);
  if (name === 'Receipt')
    return (
      value.source.domain_id === value.domain_id &&
      value.source.registration_id === value.registration_id &&
      validOriginalData('OriginalRevision', value.source)
    );
  if (name === 'Target')
    return (
      value.source === null ||
      (validOriginalData('OriginalRevision', value.source) &&
        value.source.domain_id === value.domain_id &&
        value.source.registration_id === value.registration_id &&
        value.source.relative_path === value.relative_path &&
        value.source.is_current)
    );
  if (name === 'OriginalPage')
    return value.items.every(
      (item: Record<string, any>, i: number) =>
        item.domain_id === value.domain_id &&
        item.is_current &&
        validOriginalData('OriginalRevision', item) &&
        (!i || value.items[i - 1].source_revision_id < item.source_revision_id),
    );
  return true;
}

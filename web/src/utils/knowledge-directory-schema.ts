import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import dayjs from 'dayjs';
import Contract from '../../../docs/knowledge-directory-import-v1.openapi.json';

const Encoder = new TextEncoder();
const ExcludedSegments = new Set([
  '.git',
  '.svn',
  'node_modules',
  'vendor',
  'dist',
  'build',
  '.local-data',
  '.ssh',
  '.aws',
  'credentials',
  'id_rsa',
  'id_ed25519',
]);
export function compareDirectoryPaths(left: string, right: string) {
  const a = Encoder.encode(left);
  const b = Encoder.encode(right);
  for (let i = 0; i < Math.min(a.length, b.length); i += 1)
    if (a[i] !== b[i]) return a[i] - b[i];
  return a.length - b.length;
}
export function validDirectoryPath(value: string, directory = false) {
  if (directory && value === '.') return true;
  if (
    !value ||
    Encoder.encode(value).length > 512 ||
    /[\\:*?[\]{}]/u.test(value) ||
    Array.from(value).some((character) => {
      const code = character.codePointAt(0)!;
      return code < 32 || code === 127;
    }) ||
    new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      Encoder.encode(value),
    ) !== value
  )
    return false;
  const parts = value.split('/');
  return (
    parts.length <= 32 &&
    parts.every((part) => {
      const lower = part.toLowerCase();
      return (
        !!part &&
        part !== '.' &&
        part !== '..' &&
        part.trim() === part &&
        !/[. ]$/.test(part) &&
        !/^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(part) &&
        !lower.startsWith('.env') &&
        !ExcludedSegments.has(lower) &&
        !/\.(pem|key|p12|pfx)$/i.test(part)
      );
    })
  );
}
const Validator = new Ajv({ strict: false });
addFormats(Validator);
Validator.addFormat('knowledge-relative-file', {
  type: 'string',
  validate: (value: string) => validDirectoryPath(value),
});
Validator.addFormat('knowledge-relative-directory', {
  type: 'string',
  validate: (value: string) => validDirectoryPath(value, true),
});
const Schemas = Object.fromEntries(
  Object.keys(Contract.components.schemas).map((name) => [
    name,
    Validator.compile({
      components: Contract.components,
      $ref: '#/components/schemas/' + name,
    }),
  ]),
);
function sortedUnique(paths: string[]) {
  return paths.every(
    (path, i) => !i || compareDirectoryPaths(paths[i - 1], path) < 0,
  );
}
export function validDirectoryData(
  schema: string,
  data: unknown,
  httpStatus?: number,
): boolean {
  if (!Schemas[schema]?.(data)) return false;
  const name = schema.endsWith('Response') ? schema.slice(0, -8) : schema;
  const value = (
    schema.endsWith('Response') ? (data as { data: unknown }).data : data
  ) as Record<string, any>;
  if (name === 'Listing') {
    if (
      !sortedUnique(
        value.items.map(
          (item: { relative_path: string }) => item.relative_path,
        ),
      ) ||
      value.items.length > value.total_entries
    )
      return false;
  }
  if (name === 'Observation') {
    const paths: string[] = value.selected_paths;
    const actual: string[] = value.items.map(
      (item: { relative_path: string }) => item.relative_path,
    );
    if (
      !sortedUnique(paths) ||
      !sortedUnique(actual) ||
      JSON.stringify(paths) !== JSON.stringify(actual)
    )
      return false;
    const complete = value.items.every((item: { state: string }) =>
      ['verified', 'ineligible'].includes(item.state),
    );
    if (value.complete !== complete) return false;
  }
  if (['Listing', 'Observation', 'Preview'].includes(name)) {
    const age =
      dayjs(value.expires_at).valueOf() - dayjs(value.captured_at).valueOf();
    if (!(age > 0 && age <= 600000)) return false;
  }
  if (name === 'RegistrationRequest' || name === 'Registration') {
    const paths: string[] = value.scope_directories;
    if (
      paths.some((p, i) =>
        paths.some((q, j) => i !== j && (q === '.' || p.startsWith(q + '/'))),
      )
    )
      return false;
  }
  if (
    name === 'ListingRequest' &&
    Encoder.encode(value.name_prefix).length > 128
  )
    return false;
  if (
    name === 'ContentWindow' &&
    (Encoder.encode(value.text).length !== value.byte_length ||
      value.offset + value.byte_length > value.total_bytes)
  )
    return false;
  if (name === 'Error' && httpStatus !== undefined) {
    const codes = Contract['x-http-error-codes'] as Record<string, string[]>;
    if (!codes[String(httpStatus)]?.includes(value.code)) return false;
    if (httpStatus === 500 && value.reason !== 'result_unknown') return false;
  }
  return true;
}

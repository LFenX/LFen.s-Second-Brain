import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import Contract from '../../../docs/knowledge-source-lifecycle-v1.openapi.json';
import type {
  LifecycleAction,
  LifecycleListQuery,
} from '@/interfaces/request/knowledge-lifecycle';

export const LifecycleId = /^[a-f0-9]{32}$/;
export const LifecycleSha = /^[a-f0-9]{64}$/;
export const LifecycleKey = /^[A-Za-z0-9_-]{1,128}$/;
export const LifecycleActions = [
  'reclassify',
  'move',
  'archive',
  'restore',
] as const;
const Encoder = new TextEncoder(),
  Validator = new Ajv({ strict: false });
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
export const lifecycleJsonBytes = (value: unknown) =>
  Encoder.encode(JSON.stringify(value)).length;
export function lifecycleScalar(value: string) {
  return (
    new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      Encoder.encode(value),
    ) === value
  );
}
const EdgeSpaces = new Set([
  9,
  10,
  11,
  12,
  13,
  32,
  0x85,
  0xa0,
  0x1680,
  ...Array.from({ length: 11 }, (_, i) => 0x2000 + i),
  0x2028,
  0x2029,
  0x202f,
  0x205f,
  0x3000,
]);
export function lifecycleLanguage(value: string) {
  return (
    Array.from(value).length <= 32 &&
    lifecycleScalar(value) &&
    !value.includes('\0') &&
    !EdgeSpaces.has(Array.from(value)[0]?.codePointAt(0) ?? -1) &&
    !EdgeSpaces.has(Array.from(value).at(-1)?.codePointAt(0) ?? -1)
  );
}
export function lifecyclePath(value: string) {
  return (
    typeof value === 'string' &&
    lifecycleScalar(value) &&
    Array.from(value).length >= 1 &&
    Array.from(value).length <= 512 &&
    !/[\\:\p{Cc}]/u.test(value) &&
    !value.startsWith('/') &&
    !value.split('/').some((part) => !part || part === '.' || part === '..')
  );
}
export function validLifecycleStructure(schema: string, value: unknown) {
  return !!Schemas[schema]?.(value);
}
export function validLifecycleData(
  schema: string,
  input: unknown,
  status?: number,
): boolean {
  if (!validLifecycleStructure(schema, input)) return false;
  const name = schema.endsWith('Response') ? schema.slice(0, -8) : schema,
    value = (
      schema.endsWith('Response') ? (input as { data: unknown }).data : input
    ) as Record<string, any>;
  if (name === 'Error') {
    const rule = (
      Contract['x-error-mapping'] as Record<string, (number | boolean)[]>
    )[value.code];
    return (
      !!rule &&
      (status === undefined || rule[0] === status) &&
      rule[1] === value.outcome_known
    );
  }
  if (name === 'Classification') return lifecycleLanguage(value.language);
  if (name === 'Snapshot')
    return (
      lifecyclePath(value.relative_path) &&
      validLifecycleData('Classification', value.classification)
    );
  if (name === 'ReclassifyCommand')
    return validLifecycleData('Classification', value.classification);
  if (name === 'MoveCommand') return lifecyclePath(value.relative_path);
  if (name === 'Detail') return validLifecycleData('Snapshot', value.current);
  if (name === 'Page')
    return value.items.every((item: unknown) =>
      validLifecycleData('Detail', item),
    );
  if (name === 'Receipt')
    return (
      validLifecycleData('Snapshot', value.before) &&
      validLifecycleData('Snapshot', value.after) &&
      validLifecycleCommand(value.action, value.request)
    );
  if (name === 'TargetCheck') return lifecyclePath(value.relative_path);
  return true;
}
export function lifecycleCommandSchema(action: LifecycleAction) {
  return action === 'reclassify'
    ? 'ReclassifyCommand'
    : action === 'move'
      ? 'MoveCommand'
      : 'StateCommand';
}
export function validLifecycleCommand(
  action: LifecycleAction,
  request: unknown,
) {
  return (
    LifecycleActions.includes(action) &&
    validLifecycleData(lifecycleCommandSchema(action), request)
  );
}
export function validLifecycleQuery(query: LifecycleListQuery) {
  if (
    Object.keys(query).some(
      (key) =>
        ![
          'state',
          'group_scope',
          'group_id',
          'component_side',
          'language',
          'limit',
          'cursor',
        ].includes(key),
    )
  )
    return false;
  if (
    (query.state !== undefined &&
      !['active', 'archived'].includes(query.state)) ||
    (query.group_scope !== undefined &&
      !['all', 'ungrouped', 'exact'].includes(query.group_scope))
  )
    return false;
  if (
    (query.group_scope === 'exact') !== (query.group_id !== undefined) ||
    (query.group_id !== undefined && !LifecycleId.test(query.group_id))
  )
    return false;
  return (
    (query.component_side === undefined ||
      ['frontend', 'backend', 'shared', 'cli', 'non_code'].includes(
        query.component_side,
      )) &&
    (query.language === undefined || lifecycleLanguage(query.language)) &&
    (query.limit === undefined ||
      (Number.isSafeInteger(query.limit) &&
        query.limit >= 1 &&
        query.limit <= 20)) &&
    (query.cursor === undefined || LifecycleId.test(query.cursor))
  );
}

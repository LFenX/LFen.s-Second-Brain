import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import Contract from '../../../docs/knowledge-structure-v1.openapi.json';
import type {
  StructureAlias,
  StructureParser,
} from '@/interfaces/database/knowledge-structure';

export const StructureId = /^[a-f0-9]{32}$/;
export const StructureHash = /^[a-f0-9]{64}$/;
export const StructureKey = /^[A-Za-z0-9_-]{1,128}$/;
export const StructureLimits = Contract['x-rules'].limits;
export const StructureProducers = Contract['x-rules'].producers;
export function structureParser(path: string): StructureParser | null {
  if (path.endsWith('.tsx')) return 'tsx';
  if (path.endsWith('.ts')) return 'typescript';
  if (path.endsWith('.go')) return 'go';
  if (path.endsWith('.md') || path.endsWith('.markdown')) return 'markdown';
  return null;
}
const Encoder = new TextEncoder();
export const structureBytes = (value: string) => Encoder.encode(value).length;
export const structureJsonBytes = (value: unknown) =>
  structureBytes(JSON.stringify(value));
export function structureScalar(value: string): boolean {
  return (
    new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      Encoder.encode(value),
    ) === value
  );
}
export function structurePath(value: string): boolean {
  return (
    structureScalar(value) &&
    structureBytes(value) <= 512 &&
    value.length > 0 &&
    !/[\\:\p{Cc}]/u.test(value) &&
    !value.startsWith('/') &&
    !value.split('/').some((part) => !part || part === '.' || part === '..')
  );
}
export function structureAliasTarget(value: string): boolean {
  return (
    value === '' ||
    (value.endsWith('/') &&
      structurePath(value.slice(0, -1)) &&
      structureBytes(value) <= 512)
  );
}
export function validStructureAliases(aliases: StructureAlias[]): boolean {
  const prefixes = aliases.map((alias) => alias.prefix).sort();
  return (
    aliases.length <= 8 &&
    aliases.every((alias) => validStructureSchema('Alias', alias)) &&
    prefixes.every(
      (prefix, index) => index === 0 || !prefix.startsWith(prefixes[index - 1]),
    )
  );
}
const Validator = new Ajv({ strict: false, allErrors: false });
addFormats(Validator);
Validator.addKeyword({
  keyword: 'x-max-utf8-bytes',
  type: 'string',
  schemaType: 'number',
  validate: (limit: number, value: string) => structureBytes(value) <= limit,
});
Validator.addFormat('knowledge-structure-alias-target', structureAliasTarget);
Validator.addFormat('knowledge-structure-relative-file', structurePath);
Validator.addSchema({
  $id: 'knowledge-structure-0.1.0',
  components: Contract.components,
});
const Schemas = new Map<string, ReturnType<typeof Validator.compile>>();
export function validStructureSchema(schema: string, value: unknown): boolean {
  if (!Object.hasOwn(Contract.components.schemas, schema)) return false;
  let validate = Schemas.get(schema);
  if (!validate) {
    validate = Validator.compile({
      $ref: 'knowledge-structure-0.1.0#/components/schemas/' + schema,
    });
    Schemas.set(schema, validate);
  }
  return !!validate(value);
}
function scalarTree(value: unknown): boolean {
  if (typeof value === 'string') return structureScalar(value);
  if (Array.isArray(value)) return value.every(scalarTree);
  return (
    !value ||
    typeof value !== 'object' ||
    Object.entries(value).every(
      ([key, item]) => structureScalar(key) && scalarTree(item),
    )
  );
}
export function validStructureData(schema: string, input: unknown): boolean {
  if (!validStructureSchema(schema, input) || !scalarTree(input)) return false;
  const name = schema.endsWith('Response') ? schema.slice(0, -8) : schema;
  const value = (
    schema.endsWith('Response') ? (input as { data: unknown }).data : input
  ) as Record<string, any>;
  if (name === 'TaskRequest')
    return (
      new Set(
        value.members.map((member: { source_id: string }) => member.source_id),
      ).size === value.members.length && validStructureAliases(value.aliases)
    );
  if (name === 'View')
    return (
      validStructureAliases(value.aliases) &&
      value.source_count === value.members.length &&
      new Set(
        value.members.map((member: { source_id: string }) => member.source_id),
      ).size === value.members.length &&
      value.members.every((member: unknown) =>
        validStructureData('ViewMember', member),
      )
    );
  if (name === 'ViewMember')
    return (
      structurePath(value.relative_path) &&
      validStructureData('Classification', value.classification)
    );
  if (name === 'Classification')
    return (
      !/[\p{Cc}]/u.test(value.language) &&
      value.language === value.language.trim()
    );
  if (name === 'Node')
    return (
      value.end_byte >= value.start_byte &&
      value.end_line >= value.start_line &&
      validStructureData('Classification', value.classification)
    );
  if (name === 'Content')
    return (
      value.end_byte >= value.start_byte &&
      structureBytes(value.text) === value.end_byte - value.start_byte &&
      (value.complete
        ? value.next_start_byte === null
        : value.next_start_byte === value.end_byte)
    );
  if (name === 'Evidence') return value.end_byte >= value.start_byte;
  if (name === 'Relation')
    return validStructureData('Evidence', value.evidence);
  if (name === 'Task')
    return (
      validStructureData('TaskRequest', value.original_request) &&
      value.project_id === value.original_request.project_id &&
      value.total_members === value.members.length &&
      value.finished_members <= value.total_members &&
      value.members.every((member: unknown) =>
        validStructureData('TaskMember', member),
      )
    );
  if (name === 'TaskMember')
    return value.diagnostics.every(
      (item: { start_byte: number | null; end_byte: number | null }) =>
        item.start_byte === null
          ? item.end_byte === null
          : item.end_byte !== null && item.end_byte >= item.start_byte,
    );
  if (
    name === 'RejectionReceipt' ||
    name === 'TaskRejection' ||
    name === 'CommandRejection'
  )
    return (
      validStructureData(
        value.operation === 'task'
          ? 'TaskRequest'
          : value.operation === 'policy'
            ? 'PolicyChangeRequest'
            : 'CancelRequest',
        value.original_request,
      ) && value.idempotency_key === value.original_request.idempotency_key
    );
  if (name === 'TaskOutcome')
    return validStructureData(
      'rejection_id' in value ? 'TaskRejection' : 'Task',
      value,
    );
  if (name === 'CommandReceipt')
    return validStructureData(
      'rejection_id' in value
        ? 'CommandRejection'
        : value.kind === 'policy'
          ? 'PolicyReceipt'
          : 'CancelReceipt',
      value,
    );
  if (name === 'PolicyReceipt')
    return (
      value.idempotency_key === value.original_request.idempotency_key &&
      (value.outcome === 'unchanged'
        ? value.before.enabled === value.after.enabled &&
          value.before.structure_policy_version ===
            value.after.structure_policy_version
        : value.before.enabled !== value.after.enabled &&
          value.after.structure_policy_version ===
            value.before.structure_policy_version + 1)
    );
  if (name === 'CancelReceipt')
    return value.idempotency_key === value.original_request.idempotency_key;
  if (name === 'NodePage' || name === 'RelationPage' || name === 'TaskPage')
    return value.items.every((item: unknown) =>
      validStructureData(
        name === 'NodePage'
          ? 'Node'
          : name === 'RelationPage'
            ? 'Relation'
            : 'Task',
        item,
      ),
    );
  return true;
}
export function validStructureQuery(
  kind: 'task' | 'node' | 'relation' | 'content',
  query: Record<string, unknown>,
): boolean {
  const keys =
    kind === 'task'
      ? ['project_id', 'limit', 'cursor']
      : kind === 'node'
        ? ['source_id', 'parent_id', 'limit', 'cursor']
        : kind === 'relation'
          ? ['node_id', 'direction', 'kind', 'limit', 'cursor']
          : ['start_byte', 'end_byte'];
  if (
    Object.keys(query).some(
      (key) =>
        !keys.includes(key) || query[key] === undefined || query[key] === '',
    )
  )
    return false;
  if (kind === 'content')
    return (
      Object.values(query).every(
        (v) =>
          Number.isSafeInteger(v) && Number(v) >= 0 && Number(v) <= 2097152,
      ) &&
      (query.end_byte === undefined ||
        (query.start_byte !== undefined &&
          Number(query.end_byte) >= Number(query.start_byte) &&
          Number(query.end_byte) - Number(query.start_byte) <= 65536))
    );
  if (
    query.limit !== undefined &&
    (!Number.isSafeInteger(query.limit) ||
      Number(query.limit) < 1 ||
      Number(query.limit) > 20)
  )
    return false;
  if (query.cursor !== undefined && !StructureId.test(String(query.cursor)))
    return false;
  if (kind === 'task') return StructureId.test(String(query.project_id));
  if (kind === 'node')
    return (
      StructureId.test(String(query.source_id)) &&
      (query.parent_id === undefined ||
        StructureHash.test(String(query.parent_id)))
    );
  return (
    StructureHash.test(String(query.node_id)) &&
    (query.direction === undefined ||
      ['in', 'out', 'both'].includes(String(query.direction))) &&
    (query.kind === undefined ||
      validStructureSchema('RelationKind', query.kind))
  );
}

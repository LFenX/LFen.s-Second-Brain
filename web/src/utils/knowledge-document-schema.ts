import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import Contract from '../../../docs/knowledge-document-structure-v1.openapi.json';
import { encodeWebValue, webSha256 } from './knowledge-web';

export const DocumentId = /^[a-f0-9]{32}$/;
export const DocumentHash = /^[a-f0-9]{64}$/;
export const DocumentKey = /^[A-Za-z0-9_-]{1,128}$/;
const Encoder = new TextEncoder();
export const documentBytes = (text: string) => Encoder.encode(text).length;
export const documentJsonBytes = (value: unknown) =>
  documentBytes(JSON.stringify(value));
export const documentSha = (text: string) => webSha256(Encoder.encode(text));
export const documentDigest = (value: object) =>
  documentSha(encodeWebValue(value));
export const documentScalar = (value: string) =>
  new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
    Encoder.encode(value),
  ) === value;
const EmptySha =
  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
type Schema = Record<string, any>;
const Definitions = Contract.components.schemas as Record<string, Schema>;
const SchemaId = 'knowledge-document-0.1.0';
const Validator = new Ajv({ strict: false, allErrors: false });
addFormats(Validator);
Validator.addKeyword({
  keyword: 'x-max-utf8-bytes',
  type: 'string',
  schemaType: 'number',
  validate: (n: number, v: string) => documentBytes(v) <= n,
});
Validator.addKeyword({
  keyword: 'x-max-json-bytes',
  schemaType: 'number',
  validate: (n: number, v: unknown) => documentJsonBytes(v) <= n,
});
Validator.addSchema({ $id: SchemaId, components: Contract.components });
const Compiled = new Map<string, ReturnType<typeof Validator.compile>>();
function portable(schema: Schema, value: unknown): boolean {
  const key = JSON.stringify(schema);
  let check = Compiled.get(key);
  if (!check) {
    check = Validator.compile({ ...schema, components: Contract.components });
    Compiled.set(key, check);
  }
  return !!check(value);
}
function scalarTree(value: unknown): boolean {
  if (typeof value === 'number') return Number.isSafeInteger(value);
  if (typeof value === 'string') return documentScalar(value);
  if (Array.isArray(value)) return value.every(scalarTree);
  return (
    !value ||
    typeof value !== 'object' ||
    Object.entries(value).every(
      ([key, item]) => documentScalar(key) && scalarTree(item),
    )
  );
}
async function predicate(schema: Schema, v: any): Promise<boolean> {
  if (
    schema['x-reading-range'] &&
    (v.end_byte < v.start_byte ||
      (v.end_byte === v.start_byte && v.text_sha256 !== EmptySha))
  )
    return false;
  if (
    schema['x-progress-range'] &&
    v.total_units !== null &&
    v.completed_units > v.total_units
  )
    return false;
  if (
    schema['x-content-query'] &&
    v.end_byte !== undefined &&
    (v.start_byte === undefined ||
      v.end_byte < v.start_byte ||
      v.end_byte - v.start_byte > 65536)
  )
    return false;
  if (
    schema['x-capabilities-ready'] &&
    (new Set(v.profiles.map((p: any) => p.profile_id)).size !== 3 ||
      (v.runtime_ready && v.profiles.some((p: any) => !p.runtime_ready)))
  )
    return false;
  if (schema['x-profile-bindings']) {
    if (
      v.producer.configuration_sha256 !==
        (await documentDigest({
          kind: 'knowledge-document-configuration/0.1.0',
          configuration: v.configuration,
        })) ||
      v.producer_sha256 !==
        (await documentDigest({
          kind: 'knowledge-document-producer/0.1.0',
          producer: v.producer,
        }))
    )
      return false;
  }
  if (schema['x-task-bindings']) {
    const t = v.original_request.target;
    if (
      v.domain_id !== v.original.domain_id ||
      t.source_id !== v.original.source_id ||
      t.source_revision_id !== v.original.source_revision_id ||
      t.original_sha256 !== v.original.sha256 ||
      t.expected_lifecycle_version !== v.source_snapshot.lifecycle_version ||
      v.producer_sha256 !== v.original_request.expected_producer_sha256
    )
      return false;
  }
  if (schema['x-policy-receipt']) {
    if (
      v.idempotency_key !== v.original_request.idempotency_key ||
      v.target.domain_id !== v.domain_id ||
      v.before.document_policy_version !==
        v.original_request.expected_document_policy_version ||
      v.after.enabled !== v.original_request.enabled
    )
      return false;
    if (
      v.outcome === 'unchanged'
        ? v.before.enabled !== v.after.enabled ||
          v.before.document_policy_version !== v.after.document_policy_version
        : v.before.enabled === v.after.enabled ||
          v.after.document_policy_version !==
            v.before.document_policy_version + 1
    )
      return false;
  }
  if (
    schema['x-cancel-receipt'] &&
    (v.idempotency_key !== v.original_request.idempotency_key ||
      (v.outcome === 'unchanged' &&
        (v.before_state !== v.after_state ||
          v.task_revision !== v.original_request.expected_task_revision)) ||
      (v.outcome === 'requested' &&
        v.task_revision !== v.original_request.expected_task_revision + 1))
  )
    return false;
  if (
    schema['x-rejection-bindings'] &&
    (v.idempotency_key !== v.original_request.idempotency_key ||
      (v.operation === 'task' &&
        encodeWebValue(v.target) !==
          encodeWebValue(v.original_request.target)) ||
      (v.operation === 'policy' && v.target.domain_id !== v.domain_id))
  )
    return false;
  if (
    schema['x-head-current'] &&
    v.currentness === 'current' &&
    v.active_source_revision_id !== v.current_source_revision_id
  )
    return false;
  if (schema['x-node-payload']) {
    const r = v.reading_range,
      own = v.own_reading_range;
    if (v.kind === 'document' ? v.parent_ref !== null : v.parent_ref === null)
      return false;
    if (
      v.text_status === 'available'
        ? r.end_byte <= r.start_byte
        : r.end_byte !== r.start_byte
    )
      return false;
    if (own && (own.start_byte < r.start_byte || own.end_byte > r.end_byte))
      return false;
    if (
      v.kind === 'media' &&
      v.attributes.placement === 'inline' &&
      v.attributes.inline_anchor_byte !== r.start_byte
    )
      return false;
  }
  if (schema['x-document-node']) {
    const p = v.payload;
    if (
      v.node_payload_sha256 !==
      (await documentDigest({
        kind: 'knowledge-document-node-payload/0.1.0',
        payload: p,
      }))
    )
      return false;
    if (
      v.document_node_id !==
      (await documentDigest({
        kind: 'knowledge-document-node/0.1.0',
        representation_id: v.representation_id,
        source_revision_id: v.source_revision_id,
        node_payload_sha256: v.node_payload_sha256,
      }))
    )
      return false;
  }
  if (schema['x-representation']) {
    if (
      v.domain_id !== v.original.domain_id ||
      v.producer.profile_id !== v.profile_id ||
      v.producer.format !== v.format ||
      v.producer.configuration_sha256 !==
        (await documentDigest({
          kind: 'knowledge-document-configuration/0.1.0',
          configuration: v.configuration,
        })) ||
      v.producer_sha256 !==
        (await documentDigest({
          kind: 'knowledge-document-producer/0.1.0',
          producer: v.producer,
        }))
    )
      return false;
    const fields = [
      'owner_id',
      'domain_id',
      'original',
      'source_snapshot',
      'foundation_policy_version',
      'document_policy_version',
      'format',
      'profile_id',
      'producer_sha256',
      'quality',
      'reading_format',
      'reading_text_sha256',
      'reading_byte_length',
      'structure_manifest_sha256',
    ];
    const projection = Object.fromEntries(fields.map((key) => [key, v[key]]));
    if (
      v.representation_id !==
      (await documentDigest({
        kind: 'knowledge-document-representation/0.1.0',
        ...projection,
      }))
    )
      return false;
  }
  if (schema['x-node-page']) {
    if (
      new Set(v.items.map((n: any) => n.document_node_id)).size !==
        v.items.length ||
      v.items.some(
        (n: any, i: number) =>
          n.parent_id !== v.parent_id ||
          n.representation_id !== v.representation_id ||
          n.source_id !== v.source_id ||
          n.source_revision_id !== v.source_revision_id ||
          (i > 0 && n.payload.ordinal <= v.items[i - 1].payload.ordinal),
      )
    )
      return false;
  }
  if (
    schema['x-content-window'] &&
    (v.end_byte < v.start_byte ||
      documentBytes(v.text) !== v.end_byte - v.start_byte ||
      (!v.complete &&
        (v.end_byte <= v.start_byte || v.next_start_byte !== v.end_byte)) ||
      v.fragment_sha256 !== (await documentSha(v.text)))
  )
    return false;
  return true;
}
async function relations(schema: Schema, value: any): Promise<boolean> {
  if (schema.$ref)
    return relations(Definitions[schema.$ref.split('/').pop()!], value);
  if (schema === Definitions.OfficeSegment && value.name.includes(':'))
    return false;
  if (!(await predicate(schema, value))) return false;
  for (const part of schema.allOf ?? [])
    if (!(await relations(part, value))) return false;
  for (const union of [schema.oneOf, schema.anyOf]) {
    if (!union) continue;
    let accepted = false;
    for (const part of union)
      if (portable(part, value) && (await relations(part, value))) {
        accepted = true;
        break;
      }
    if (!accepted) return false;
  }
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    for (const [key, part] of Object.entries(schema.properties ?? {}))
      if (
        Object.hasOwn(value, key) &&
        !(await relations(part as Schema, value[key]))
      )
        return false;
  }
  if (Array.isArray(value) && schema.items)
    for (const item of value)
      if (!(await relations(schema.items, item))) return false;
  return true;
}
export async function validDocumentData(
  schema: string,
  value: unknown,
): Promise<boolean> {
  try {
    return (
      !!Definitions[schema] &&
      scalarTree(value) &&
      portable({ $ref: SchemaId + '#/components/schemas/' + schema }, value) &&
      (await relations(Definitions[schema], value))
    );
  } catch {
    return false;
  }
}

// A bounded parser preserves duplicate-key and scalar validation before JSON values lose that information.
export function parseDocumentJson(text: string): unknown {
  if (documentBytes(text) > 524288) throw Error('document_invalid_json');
  let i = 0;
  const white = () => {
    while (/[\x20\t\r\n]/.test(text[i] ?? '\0')) i++;
  };
  const string = () => {
    const start = i++;
    while (i < text.length) {
      const ch = text[i++];
      if (ch === '\\') {
        i++;
        continue;
      }
      if (ch === '"') {
        const v: string = JSON.parse(text.slice(start, i));
        if (!documentScalar(v)) throw Error('document_invalid_json');
        return v;
      }
    }
    throw Error('document_invalid_json');
  };
  function value(depth: number): unknown {
    if (depth > 128) throw Error('document_invalid_json');
    white();
    if (text[i] === '"') return string();
    if (text[i] === '{' || text[i] === '[') {
      const object = text[i++] === '{',
        end = object ? '}' : ']';
      const result: any = object ? {} : [];
      white();
      if (text[i] === end) {
        i++;
        return result;
      }
      while (i < text.length) {
        white();
        if (object) {
          if (text[i] !== '"') throw Error('document_invalid_json');
          const key = string();
          white();
          if (Object.hasOwn(result, key) || text[i++] !== ':')
            throw Error('document_invalid_json');
          Object.defineProperty(result, key, {
            value: value(depth + 1),
            enumerable: true,
            writable: true,
            configurable: true,
          });
        } else result.push(value(depth + 1));
        white();
        if (text[i] === end) {
          i++;
          return result;
        }
        if (text[i++] !== ',') throw Error('document_invalid_json');
      }
      throw Error('document_invalid_json');
    }
    const token =
      /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/.exec(
        text.slice(i),
      )?.[0];
    if (!token) throw Error('document_invalid_json');
    i += token.length;
    const v: unknown = JSON.parse(token);
    if (typeof v === 'number' && !Number.isSafeInteger(v))
      throw Error('document_invalid_json');
    return v;
  }
  const result = value(0);
  white();
  if (i !== text.length) throw Error('document_invalid_json');
  return result;
}
export async function parseDocumentQuery(
  schema: string,
  query: string,
): Promise<Record<string, unknown>> {
  const result: Record<string, unknown> = {};
  const numeric = new Set(['limit', 'start_byte', 'end_byte']);
  for (const pair of query ? query.split('&') : []) {
    const separator = pair.indexOf('=');
    if (separator < 1) throw Error('document_invalid_query');
    const key = decodeURIComponent(
        pair.slice(0, separator).replace(/\+/g, ' '),
      ),
      raw = decodeURIComponent(pair.slice(separator + 1).replace(/\+/g, ' '));
    if (
      !raw ||
      Object.hasOwn(result, key) ||
      (numeric.has(key) && !/^(0|[1-9]\d*)$/.test(raw))
    )
      throw Error('document_invalid_query');
    Object.defineProperty(result, key, {
      value: numeric.has(key) ? Number(raw) : raw,
      enumerable: true,
    });
  }
  if (!(await validDocumentData(schema, result)))
    throw Error('document_invalid_query');
  return result;
}

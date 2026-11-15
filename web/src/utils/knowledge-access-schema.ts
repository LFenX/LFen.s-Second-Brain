import Ajv2020 from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import type { ValidateFunction } from 'ajv';
import Contract from '../../../docs/knowledge-access-20261108-02/access.schema.candidate.json';
import {
  contentCanonical,
  contentDigest,
  parseContentJSON,
} from './knowledge-consumption';
import type {
  AccessInspectData,
  AccessReadData,
  AccessSearchData,
  AccessTarget,
} from '@/interfaces/database/knowledge-access';

const validator = new Ajv2020({
  strict: false,
  allErrors: false,
  validateFormats: true,
});
addFormats(validator);
validator.addSchema(Contract);
const definitions = new Map<string, ValidateFunction>();
export const accessBytes = (text: string) =>
  new TextEncoder().encode(text).length;
export const accessCanonical = contentCanonical;
export const parseAccessJSON = parseContentJSON;

export function accessSchemaValid(name: string, value: unknown): boolean {
  let check = definitions.get(name);
  if (!check) {
    check = validator.compile({ $ref: Contract.$id + '#/$defs/' + name });
    definitions.set(name, check);
  }
  return check(value) === true;
}

export function accessSame(left: unknown, right: unknown): boolean {
  return accessCanonical(left) === accessCanonical(right);
}

export function accessTargetsValid(
  targets: AccessTarget[],
  domain: string,
): boolean {
  const seen = new Set<string>();
  return (
    targets.length > 0 &&
    targets.length <= 32 &&
    targets.every((target) => {
      const source = target.source;
      if (source.domain_id !== domain || seen.has(source.source_id))
        return false;
      seen.add(source.source_id);
      return (
        target.representation.kind !== 'source_revision' ||
        target.representation.id === source.source_revision_id
      );
    })
  );
}

export function accessInspectValid(value: AccessInspectData): boolean {
  const order = value.targets.map(({ target }) =>
    [
      target.source.domain_id,
      target.source.source_id,
      target.source.source_revision_id,
      target.source.source_sha256,
      target.representation.kind,
      target.representation.id,
      target.representation.producer_sha256 ?? '',
    ].join('\u0000'),
  );
  return (
    accessTargetsValid(
      value.targets.map((item) => item.target),
      value.domain_id,
    ) &&
    order.every((item, index) => index === 0 || order[index - 1] < item) &&
    value.source_count === value.targets.length &&
    value.unit_count ===
      value.targets.reduce((sum, item) => sum + item.unit_count, 0) &&
    value.unit_count <= 32768 &&
    Number.isFinite(Date.parse(value.expires_at))
  );
}

export async function accessTextValid(value: {
  text: string;
  byte_length: number;
  citation: AccessReadData['citation'];
}): Promise<boolean> {
  const range = value.citation.range;
  return (
    accessBytes(value.text) === value.byte_length &&
    value.byte_length <= 8192 &&
    range.end_byte - range.start_byte === value.byte_length &&
    accessBytes(value.citation.display_name) <= 256 &&
    accessBytes(value.citation.position_label) <= 512 &&
    (await contentDigest(value.text)) === range.sha256
  );
}

export async function accessSearchValid(
  value: AccessSearchData,
  view: AccessInspectData,
): Promise<boolean> {
  if (
    value.view_id !== view.view_id ||
    value.returned_count !== value.hits.length ||
    value.hits.length > value.requested_limit
  )
    return false;
  const seen = new Set<string>();
  for (const [index, hit] of value.hits.entries()) {
    if (
      hit.rank_ordinal !== index + 1 ||
      hit.evidence_ref.view_id !== view.view_id ||
      seen.has(hit.evidence_ref.evidence_id) ||
      !view.targets.some((entry) =>
        accessSame(entry.target, hit.citation.target),
      ) ||
      !(await accessTextValid(hit))
    )
      return false;
    seen.add(hit.evidence_ref.evidence_id);
  }
  return true;
}

export async function accessReadValid(
  value: AccessReadData,
  view: AccessInspectData,
): Promise<boolean> {
  const locator = value.unit_locator;
  const target = view.targets[locator.target_index];
  if (
    !target ||
    locator.view_id !== view.view_id ||
    value.evidence_ref.view_id !== view.view_id ||
    locator.target_unit_count !== target.unit_count ||
    locator.unit_ordinal >= target.unit_count ||
    !accessSame(target.target, value.citation.target) ||
    locator.next_unit_ordinal !==
      (locator.unit_ordinal + 1 < target.unit_count
        ? locator.unit_ordinal + 1
        : null) ||
    !(await accessTextValid(value)) ||
    accessBytes(JSON.stringify(value.typed_context)) > 65536
  )
    return false;
  const outer = value.citation.range;
  const bytes = new TextEncoder().encode(value.text);
  for (const member of value.typed_context.members) {
    if (
      member.range.start_byte < outer.start_byte ||
      member.range.end_byte < member.range.start_byte ||
      member.range.end_byte > outer.end_byte ||
      (member.text_status === 'empty' &&
        member.range.start_byte !== member.range.end_byte)
    )
      return false;
    try {
      const text = new TextDecoder('utf-8', { fatal: true }).decode(
        bytes.slice(
          member.range.start_byte - outer.start_byte,
          member.range.end_byte - outer.start_byte,
        ),
      );
      if ((await contentDigest(text)) !== member.range.sha256) return false;
    } catch {
      return false;
    }
  }
  return true;
}

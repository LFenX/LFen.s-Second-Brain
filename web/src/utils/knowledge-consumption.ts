import Ajv2020 from 'ajv/dist/2020';
import dayjs from 'dayjs';
import type { ValidateFunction } from 'ajv';
import type {
  ContentAddress,
  ContentLocator,
  ContentNode,
  ContentNodeRef,
  ContentNodePage,
  ContentSourceRef,
  ContentParsedRepresentationPage,
  ContentRepresentation,
  ContentTarget,
  ContentWindow,
  ContentPolicy,
  ContentParsedSource,
  ContentParsedTask,
} from '@/interfaces/database/knowledge-consumption';
import type {
  ContentParsedTaskRequest,
  ContentPolicyRequest,
  ContentScope,
} from '@/interfaces/request/knowledge-consumption';
import { KnowledgeContentVersion } from '@/interfaces/database/knowledge-consumption';
import type { KnowledgeSource } from '@/interfaces/database/knowledge-foundation';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import type {
  DocumentHead,
  DocumentRepresentation,
} from '@/interfaces/database/knowledge-document';
import type { StructureView } from '@/interfaces/database/knowledge-structure';
import { lifecycleDigest } from '@/utils/knowledge-lifecycle';
import Contract from '../../../docs/knowledge-core-content-v0.2.parsed.candidate.schema.json';

const Id = /^[a-f0-9]{32}$/,
  Hash = /^[a-f0-9]{64}$/;
export const ContentLimits = {
  request: 32768,
  response: 1048576,
  read: 65536,
  page: 100,
  original: 2097152,
  unit: 8192,
} as const;
export const contentBytes = (text: string) =>
  new TextEncoder().encode(text).length;
export const contentCanonical = (value: unknown): string =>
  Array.isArray(value)
    ? '[' + value.map(contentCanonical).join(',') + ']'
    : value !== null && typeof value === 'object'
      ? '{' +
        Object.keys(value)
          .sort()
          .map(
            (key) =>
              JSON.stringify(key) +
              ':' +
              contentCanonical((value as Record<string, unknown>)[key]),
          )
          .join(',') +
        '}'
      : JSON.stringify(value);
export const contentNodeKey = (ref: ContentNodeRef) => contentCanonical(ref);
export const contentTargetKey = (target: ContentTarget) =>
  contentCanonical(target);
export function contentSameTarget(left: ContentTarget, right: ContentTarget) {
  return contentTargetKey(left) === contentTargetKey(right);
}
export function contentFixedTarget(
  domain: string,
  target: ContentTarget,
): boolean {
  const s = target.source_ref,
    r = target.representation_ref;
  if (
    !Id.test(domain) ||
    s.domain_id !== domain ||
    !Id.test(s.source_id) ||
    !Id.test(s.source_revision_id) ||
    !Hash.test(s.source_sha256)
  )
    return false;
  return r.kind === 'source_revision'
    ? r.id === s.source_revision_id && r.producer_sha256 === null
    : (r.kind === 'structure_view' ||
        r.kind === 'document_representation' ||
        r.kind === 'parsed_representation') &&
        r.id !== null &&
        Hash.test(r.id) &&
        r.producer_sha256 !== null &&
        Hash.test(r.producer_sha256);
}
export function contentBoundNode(target: ContentTarget, ref: ContentNodeRef) {
  const kind = {
    source_revision: 'c1_node',
    structure_view: 'r1_node',
    document_representation: 'r2_node',
    parsed_representation: 'parsed_node',
    parser_result: '',
  }[target.representation_ref.kind];
  return (
    contentFixedTarget(target.source_ref.domain_id, target) &&
    ref.kind === kind &&
    ref.id !== null &&
    Hash.test(ref.id) &&
    ref.item_index === null
  );
}
export function contentSourceTarget(source: KnowledgeSource): ContentTarget {
  return {
    source_ref: {
      domain_id: source.domain_id,
      source_id: source.source_id,
      source_revision_id: source.source_revision_id,
      source_sha256: source.sha256,
    },
    representation_ref: {
      kind: 'source_revision',
      id: source.source_revision_id,
      producer_sha256: null,
    },
  };
}
export function contentStructureTarget(
  view: StructureView,
  source: string,
): ContentTarget {
  const member = view.members.find((item) => item.source_id === source);
  if (!member) throw Error('content_source_not_in_fixed_view');
  const target: ContentTarget = {
    source_ref: {
      domain_id: view.domain_id,
      source_id: member.source_id,
      source_revision_id: member.source_revision_id,
      source_sha256: member.sha256,
    },
    representation_ref: {
      kind: 'structure_view',
      id: view.view_id,
      producer_sha256: view.producer_sha256,
    },
  };
  if (!contentFixedTarget(view.domain_id, target))
    throw Error('content_invalid_target');
  return target;
}
export function contentDocumentTarget(
  original: Pick<
    OriginalRevision,
    'domain_id' | 'source_id' | 'source_revision_id' | 'sha256'
  >,
  head: Pick<
    DocumentHead,
    | 'domain_id'
    | 'source_id'
    | 'active_source_revision_id'
    | 'active_representation_id'
  >,
  representation: Pick<
    DocumentRepresentation,
    'original' | 'domain_id' | 'representation_id' | 'producer_sha256'
  >,
): ContentTarget {
  const fixed = representation.original;
  if (
    head.domain_id !== original.domain_id ||
    head.source_id !== original.source_id ||
    head.active_source_revision_id !== original.source_revision_id ||
    head.active_representation_id !== representation.representation_id ||
    representation.domain_id !== original.domain_id ||
    fixed.domain_id !== original.domain_id ||
    fixed.source_id !== original.source_id ||
    fixed.source_revision_id !== original.source_revision_id ||
    fixed.sha256 !== original.sha256
  )
    throw Error('content_original_head_mismatch');
  const target: ContentTarget = {
    source_ref: {
      domain_id: fixed.domain_id,
      source_id: fixed.source_id,
      source_revision_id: fixed.source_revision_id,
      source_sha256: fixed.sha256,
    },
    representation_ref: {
      kind: 'document_representation',
      id: representation.representation_id,
      producer_sha256: representation.producer_sha256,
    },
  };
  if (!contentFixedTarget(original.domain_id, target))
    throw Error('content_invalid_target');
  return target;
}
export function contentOriginalLocator(locator: ContentLocator) {
  if (locator.kind === 'source_utf8') return locator.source_utf8;
  if (locator.kind === 'parser_item' || locator.kind === 'parsed_item')
    return { item_index: locator.item_index };
  return locator.document?.scheme === 'derived_passage_v1'
    ? locator.document.parent_locator
    : locator.document;
}

const Validators = new Map<string, ValidateFunction>();
const Validator = new Ajv2020({
  strict: false,
  allErrors: false,
  validateFormats: true,
  strictNumbers: true,
});
export function contentTimestamp(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value))
    return false;
  const stamp = dayjs(value);
  const expected = value.includes('.')
    ? value.replace(
        /\.(\d+)Z$/,
        (_match, fraction: string) =>
          '.' + (fraction + '000').slice(0, 3) + 'Z',
      )
    : value.replace(/Z$/, '.000Z');
  return stamp.isValid() && stamp.toISOString() === expected;
}
Validator.addFormat('date-time', contentTimestamp);
const scalar = (value: string) =>
  new TextDecoder('utf-8', { fatal: true }).decode(
    new TextEncoder().encode(value),
  ) === value;
Validator.addKeyword({
  keyword: 'x-max-utf8-bytes',
  type: 'string',
  schemaType: 'number',
  validate: (limit: number, value: string) => contentBytes(value) <= limit,
});
Validator.addKeyword({
  keyword: 'x-unicode-scalar',
  type: 'string',
  schemaType: 'boolean',
  validate: (enabled: boolean, value: string) => !enabled || scalar(value),
});
Validator.addKeyword({
  keyword: 'x-xml-local-name',
  type: 'string',
  schemaType: 'boolean',
  validate: (enabled: boolean, value: string) =>
    !enabled || /^[_\p{L}][_\p{L}\p{N}\p{M}.\-\u00b7]*$/u.test(value),
});
Validator.addSchema(Contract);
export function contentSchemaValid(name: string, value: unknown): boolean {
  if (!Object.hasOwn(Contract.$defs, name)) return false;
  let validate = Validators.get(name);
  if (!validate) {
    validate = Validator.compile({ $ref: Contract.$id + '#/$defs/' + name });
    Validators.set(name, validate);
  }
  return !!validate(value);
}
export function contentVersion(value: unknown) {
  return value === KnowledgeContentVersion;
}
export async function checkContentNode(
  target: ContentTarget,
  node: ContentNode,
) {
  if (
    !contentBoundNode(target, node.ref) ||
    (node.parent_ref &&
      (!contentBoundNode(target, node.parent_ref) ||
        contentNodeKey(node.parent_ref) === contentNodeKey(node.ref)))
  )
    throw Error('content_node_binding');
  if (!contentParsedPosition(node)) throw Error('content_parsed_position');
  const parser = node.attributes.parser;
  if (
    parser?.positions?.some((row) =>
      row?.some((value) => !Number.isFinite(value)),
    )
  )
    throw Error('content_parser_position');
  if (parser?.table) {
    if (parser.text_assembly !== 'row_major_uncovered_cells_tab_lf_v1')
      throw Error('content_table_assembly');
    const tableText = contentTableText(parser.table);
    if (
      node.text.range &&
      contentBytes(tableText) !==
        node.text.range.end_byte - node.text.range.start_byte
    )
      throw Error('content_table_range');
    if (
      node.text.range?.sha256 &&
      (await contentDigest(tableText)) !== node.text.range.sha256
    )
      throw Error('content_table_digest');
  }
  const text = node.text,
    range = text.range,
    own = text.own_range;
  const space =
    target.representation_ref.kind === 'document_representation' ||
    target.representation_ref.kind === 'parsed_representation'
      ? 'derived_utf8'
      : 'source_utf8';
  if (
    text.coordinate_space !== space ||
    (range && range.end_byte < range.start_byte) ||
    (own &&
      (!range ||
        own.start_byte < range.start_byte ||
        own.end_byte > range.end_byte ||
        own.end_byte < own.start_byte))
  )
    throw Error('content_node_range');
  if (
    (text.status === 'available' &&
      (!range || range.start_byte === range.end_byte)) ||
    (text.status === 'empty' && (!range || range.start_byte !== range.end_byte))
  )
    throw Error('content_text_state');
  if (node.locator.kind === 'source_utf8') {
    const source = node.locator.source_utf8;
    if (
      !source ||
      !range ||
      source.start_byte !== range.start_byte ||
      source.end_byte !== range.end_byte ||
      source.end_line < source.start_line
    )
      throw Error('content_original_position');
  }
  if (node.locator.kind === 'document') {
    const locator = node.locator.document,
      original = node.original_position;
    if (!locator || !original) throw Error('content_original_position');
    if (
      locator.scheme === 'pdf_page_v1' &&
      (original.kind !== 'pdf_page' ||
        original.page_number !== locator.page_number)
    )
      throw Error('content_original_position');
    const attributes = node.attributes.document;
    if (
      attributes?.page &&
      (original.kind !== 'pdf_page' ||
        attributes.page.page_number !== original.page_number)
    )
      throw Error('content_original_position');
    if (
      attributes?.slide &&
      (original.kind !== 'pptx_slide' ||
        attributes.slide.slide_number !== original.slide_number)
    )
      throw Error('content_original_position');
  }
  if (
    node.attachment_ref &&
    node.attachment_ref.domain_id !== target.source_ref.domain_id
  )
    throw Error('content_attachment_binding');
  if (text.value !== null) {
    if (
      !range ||
      contentBytes(text.value) !== range.end_byte - range.start_byte ||
      (range.sha256 !== null &&
        (await contentDigest(text.value)) !== range.sha256)
    )
      throw Error('content_text_integrity');
    if (own) {
      const bytes = new TextEncoder().encode(text.value),
        a = own.start_byte - range.start_byte,
        b = own.end_byte - range.start_byte;
      const part = new TextDecoder('utf-8', { fatal: true }).decode(
        bytes.subarray(a, b),
      );
      if (own.sha256 !== null && (await contentDigest(part)) !== own.sha256)
        throw Error('content_text_integrity');
    }
  }
}
export function checkContentRepresentation(
  target: ContentTarget,
  representation: ContentRepresentation,
) {
  if (
    representation.binding !== 'bound' ||
    !representation.source_ref ||
    !contentSameTarget(target, {
      source_ref: representation.source_ref,
      representation_ref: representation.representation_ref,
    }) ||
    (representation.root_ref !== null &&
      !contentBoundNode(target, representation.root_ref))
  )
    throw Error('content_representation_binding');
}

export function contentPageItems(pages: ContentNodePage[]): ContentNode[] {
  const nodes = new Set<string>(),
    cursors = new Set<string>();
  const items: ContentNode[] = [];
  for (const page of pages) {
    if (page.next_cursor !== null) {
      if (cursors.has(page.next_cursor)) throw Error('content_repeated_cursor');
      cursors.add(page.next_cursor);
    }
    for (const node of page.items) {
      const key = contentNodeKey(node.ref);
      if (nodes.has(key)) throw Error('content_repeated_node');
      nodes.add(key);
      items.push(node);
    }
  }
  return items;
}

// Bounded strict JSON, including duplicate keys and scalar Unicode. The existing document parser has a smaller bound.
const ContentWireSizes = new WeakMap<object, number>();
export const contentWireBytes = (value: object) => ContentWireSizes.get(value);
export function parseContentJSON(text: string, lexicalScores = false): unknown {
  if (contentBytes(text) > ContentLimits.response)
    throw Error('content_invalid_json');
  let at = 0;
  const whitespace = () => {
    while (/[\x20\t\r\n]/.test(text[at] ?? '\0')) at++;
  };
  const string = () => {
    const start = at++;
    while (at < text.length) {
      const ch = text[at++];
      if (ch === '\\') {
        at++;
        continue;
      }
      if (ch === '"') {
        const result: unknown = JSON.parse(text.slice(start, at));
        if (
          typeof result !== 'string' ||
          new TextDecoder('utf-8', { fatal: true }).decode(
            new TextEncoder().encode(result),
          ) !== result
        )
          throw Error('content_invalid_json');
        return result;
      }
    }
    throw Error('content_invalid_json');
  };
  function exactInteger(token: string) {
    const match = /^-?(\d+)(?:\.(\d+))?(?:e([+-]?\d+))?$/i.exec(token);
    if (!match) return false;
    const digits = (match[1] + (match[2] ?? '')).replace(/^0+/, '');
    if (!digits) return true;
    const scale = Number(match[3] ?? 0) - (match[2]?.length ?? 0);
    return (
      scale >= 0 ||
      (scale >= -digits.length && digits.endsWith('0'.repeat(-scale)))
    );
  }
  function value(depth: number, names: string[] = []): unknown {
    if (depth > 128) throw Error('content_invalid_json');
    whitespace();
    if (text[at] === '"') return string();
    if (text[at] === '{') {
      const start = at++;
      const result: Record<string, unknown> = Object.create(null);
      const complete = () => {
        if (names.join('/') === 'data/context')
          ContentWireSizes.set(result, contentBytes(text.slice(start, at)));
        return result;
      };
      whitespace();
      if (text[at] === '}') {
        at++;
        return complete();
      }
      while (at < text.length) {
        whitespace();
        if (text[at] !== '"') throw Error('content_invalid_json');
        const key = string();
        whitespace();
        if (Object.hasOwn(result, key) || text[at++] !== ':')
          throw Error('content_invalid_json');
        result[key] = value(depth + 1, [...names, key]);
        whitespace();
        if (text[at] === '}') {
          at++;
          return complete();
        }
        if (text[at++] !== ',') throw Error('content_invalid_json');
      }
    }
    if (text[at] === '[') {
      at++;
      const result: unknown[] = [];
      whitespace();
      if (text[at] === ']') {
        at++;
        return result;
      }
      while (at < text.length) {
        result.push(value(depth + 1, names));
        whitespace();
        if (text[at] === ']') {
          at++;
          return result;
        }
        if (text[at++] !== ',') throw Error('content_invalid_json');
      }
    }
    const token =
      /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/.exec(
        text.slice(at),
      )?.[0];
    if (!token) throw Error('content_invalid_json');
    at += token.length;
    const result: unknown = JSON.parse(token);
    if (
      typeof result === 'number' &&
      (!Number.isFinite(result) ||
        (!names.includes('positions') &&
          !(lexicalScores && names.join('/') === 'data/hits/score') &&
          (!exactInteger(token) || !Number.isSafeInteger(result))))
    )
      throw Error('content_invalid_json');
    return result;
  }
  const result = value(0);
  whitespace();
  if (at !== text.length) throw Error('content_invalid_json');
  return result;
}
export async function contentDigest(text: string) {
  return [
    ...new Uint8Array(
      await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)),
    ),
  ]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}
export async function checkContentWindow(
  address: ContentAddress,
  node: ContentNode,
  start: number,
  end: number | null,
  window: ContentWindow,
) {
  const range = node.text.range;
  if (
    !range ||
    !contentBoundNode(address.target, node.ref) ||
    contentNodeKey(node.ref) !== contentNodeKey(address.node_ref) ||
    !contentSameTarget(address.target, window.address.target) ||
    contentNodeKey(address.node_ref) !==
      contentNodeKey(window.address.node_ref) ||
    contentCanonical(node.locator) !== contentCanonical(window.locator) ||
    window.coordinate_space !== node.text.coordinate_space ||
    window.text_status !== node.text.status ||
    window.text_reason !== node.text.reason ||
    (window.text_status === 'available' && window.range.end_byte === start) ||
    window.range.start_byte !== start ||
    window.range.end_byte < start ||
    window.range.end_byte > range.end_byte ||
    start < range.start_byte ||
    (end !== null && window.range.end_byte !== end) ||
    window.range.end_byte - start > ContentLimits.read ||
    contentBytes(window.text) !== window.range.end_byte - start ||
    window.complete !== (window.range.end_byte === range.end_byte) ||
    window.next_start_byte !== (window.complete ? null : window.range.end_byte)
  )
    throw Error('content_read_integrity');
  const sha = await contentDigest(window.text);
  if (
    sha !== window.range.sha256 ||
    (start === range.start_byte &&
      window.complete &&
      range.sha256 !== null &&
      sha !== range.sha256)
  )
    throw Error('content_read_integrity');
  return window;
}
export function contentFixedLink(address: ContentAddress): string {
  const s = address.target.source_ref,
    r = address.target.representation_ref;
  if (!contentBoundNode(address.target, address.node_ref))
    throw Error('content_invalid_fixed_link');
  return (
    '/dataset/knowledge-catalogue/' +
    s.domain_id +
    '?' +
    new URLSearchParams({
      content: '1',
      content_source: s.source_id,
      content_revision: s.source_revision_id,
      content_sha256: s.source_sha256,
      content_kind: r.kind,
      content_representation: r.id!,
      content_producer: r.producer_sha256 ?? '',
      content_node_kind: address.node_ref.kind,
      content_node: address.node_ref.id!,
    })
  );
}
export function contentRoute(
  domain: string,
  params: URLSearchParams,
): ContentAddress | null {
  const names = [
    'content',
    'content_source',
    'content_revision',
    'content_sha256',
    'content_kind',
    'content_representation',
    'content_producer',
    'content_node_kind',
    'content_node',
  ];
  if (
    [...params.keys()].some(
      (name) => name.startsWith('content_') && !names.includes(name),
    )
  )
    throw Error('content_invalid_fixed_link');
  if (!names.some((name) => params.has(name))) return null;
  if (!names.slice(1).some((name) => params.has(name))) {
    if (params.getAll('content').length !== 1 || params.get('content') !== '1')
      throw Error('content_invalid_fixed_link');
    return null;
  }
  if (
    names.some((name) => params.getAll(name).length !== 1) ||
    params.get('content') !== '1'
  )
    throw Error('content_invalid_fixed_link');
  const kind = params.get('content_kind'),
    nodeKind = params.get('content_node_kind');
  if (
    ![
      'source_revision',
      'structure_view',
      'document_representation',
      'parsed_representation',
    ].includes(kind ?? '') ||
    !['c1_node', 'r1_node', 'r2_node', 'parsed_node'].includes(nodeKind ?? '')
  )
    throw Error('content_invalid_fixed_link');
  const target: ContentTarget = {
    source_ref: {
      domain_id: domain,
      source_id: params.get('content_source')!,
      source_revision_id: params.get('content_revision')!,
      source_sha256: params.get('content_sha256')!,
    },
    representation_ref: {
      kind: kind as ContentTarget['representation_ref']['kind'],
      id: params.get('content_representation'),
      producer_sha256: params.get('content_producer') || null,
    },
  };
  const address: ContentAddress = {
    target,
    node_ref: {
      kind: nodeKind as ContentNodeRef['kind'],
      id: params.get('content_node'),
      item_index: null,
    },
  };
  if (!contentBoundNode(target, address.node_ref))
    throw Error('content_invalid_fixed_link');
  return address;
}

// Discovery never follows a head. Every item must bind the complete selected revision.
export function contentParsedTargets(
  source: ContentSourceRef,
  pages: ContentParsedRepresentationPage[],
): ContentTarget[] {
  const ids = new Set<string>(),
    cursors = new Set<string>(),
    targets: ContentTarget[] = [];
  for (const page of pages) {
    if (
      contentCanonical(page.source_ref) !== contentCanonical(source) ||
      page.items.length > 20
    )
      throw Error('content_parsed_source_mismatch');
    if (page.next_cursor !== null) {
      if (!page.next_cursor || cursors.has(page.next_cursor))
        throw Error('content_repeated_cursor');
      cursors.add(page.next_cursor);
    }
    for (const target of page.items) {
      if (
        !contentFixedTarget(source.domain_id, target) ||
        contentCanonical(target.source_ref) !== contentCanonical(source) ||
        target.representation_ref.kind !== 'parsed_representation'
      )
        throw Error('content_parsed_source_mismatch');
      const id = target.representation_ref.id!;
      if (ids.has(id)) throw Error('content_repeated_representation');
      ids.add(id);
      targets.push(target);
    }
  }
  return targets;
}
export function contentParsedPosition(node: ContentNode): boolean {
  if (node.ref.kind !== 'parsed_node') return true;
  if (node.ordinal === null || node.text.coordinate_space !== 'derived_utf8')
    return false;
  if (node.kind === 'document' || node.kind === 'passage')
    return (
      node.locator.kind === 'document' &&
      node.original_position?.kind === 'document_only' &&
      node.attributes.document !== null &&
      node.attributes.parser === null
    );
  return (
    node.locator.kind === 'parsed_item' &&
    node.locator.item_index !== null &&
    Number.isSafeInteger(node.locator.item_index) &&
    node.locator.item_index >= 0 &&
    node.locator.document === null &&
    node.locator.source_utf8 === null &&
    node.original_position === null &&
    node.coordinate_precision === 'item_only' &&
    node.attributes.parser !== null &&
    node.attributes.document === null
  );
}
export function contentTableText(
  table: import('@/interfaces/database/knowledge-consumption').ContentTableData,
): string {
  const rows = table.grid.map((row) => row ?? []),
    covered = new Map<string, true>();
  if (
    !Number.isSafeInteger(table.header_rows) ||
    table.header_rows < 0 ||
    table.header_rows > rows.length
  )
    throw Error('content_invalid_table');
  rows.forEach((row, r) => {
    row.forEach((cell, c) => {
      const key = r + ':' + c,
        height = Math.max(1, cell.row_span ?? 0),
        width = Math.max(1, cell.col_span ?? 0);
      if (
        !Number.isSafeInteger(cell.row_span ?? 0) ||
        !Number.isSafeInteger(cell.col_span ?? 0) ||
        (cell.row_span ?? 0) < 0 ||
        (cell.col_span ?? 0) < 0
      )
        throw Error('content_invalid_table');
      if (cell.covered) {
        if (cell.text !== '' || !covered.has(key))
          throw Error('content_invalid_table');
        return;
      }
      if (
        covered.has(key) ||
        height > rows.length - r ||
        width > row.length - c
      )
        throw Error('content_invalid_table');
      for (let rr = r; rr < r + height; rr++) {
        if (c + width > rows[rr].length) throw Error('content_invalid_table');
        for (let cc = c; cc < c + width; cc++) {
          if (rr === r && cc === c) continue;
          const spanKey = rr + ':' + cc;
          if (covered.has(spanKey)) throw Error('content_invalid_table');
          covered.set(spanKey, true);
        }
      }
    });
  });
  return rows
    .map((row) =>
      row
        .filter((cell) => !cell.covered)
        .map((cell) => cell.text)
        .join('\t'),
    )
    .join('\n');
}

const ParsedExtensions: Record<string, string> = {
  doc: 'doc',
  ppt: 'ppt',
  xls: 'xls',
  xlsx: 'xlsx',
  csv: 'csv',
  html: 'html',
  htm: 'html',
  md: 'md',
  markdown: 'md',
  mdx: 'md',
  epub: 'epub',
  json: 'json',
  jsonl: 'json',
  ldjson: 'json',
  eml: 'email',
  msg: 'email',
  txt: 'txt',
  py: 'txt',
  js: 'txt',
  java: 'txt',
  c: 'txt',
  cpp: 'txt',
  h: 'txt',
  php: 'txt',
  go: 'txt',
  ts: 'txt',
  sh: 'txt',
  cs: 'txt',
  kt: 'txt',
  sql: 'txt',
};
export function contentParsedFileFormat(filename: string): string | null {
  if (
    !scalar(filename) ||
    !filename ||
    contentBytes(filename) > 255 ||
    ['/', '\\', '\0', '\r', '\n'].some((character) =>
      filename.includes(character),
    ) ||
    /^\.env/i.test(filename)
  )
    return null;
  const extension = /\.([^.]+)$/.exec(filename)?.[1].toLowerCase();
  return extension ? (ParsedExtensions[extension] ?? null) : null;
}
export function contentEnableParseRequest(
  policy: ContentPolicy,
  key: string,
): ContentPolicyRequest {
  if (!contentSchemaValid('ContentPolicy', policy))
    throw Error('content_invalid_policy');
  const request: ContentPolicyRequest = {
    expected_policy_version: policy.foundation_policy_version,
    expected_content_policy_version: policy.content_policy_version,
    parse_enabled: true,
    lexical_enabled: policy.lexical_enabled,
    confirmed: true,
    idempotency_key: key,
  };
  if (!contentSchemaValid('ContentPolicyRequest', request))
    throw Error('content_invalid_policy');
  return request;
}
export function contentParsedTaskRequest(
  source: ContentParsedSource,
  policy: ContentPolicy,
  key: string,
): ContentParsedTaskRequest {
  if (
    !contentSchemaValid('ParsedSourceState', source) ||
    !contentSchemaValid('ContentPolicy', policy) ||
    source.source_ref.domain_id !== policy.domain_id ||
    source.state !== 'active' ||
    !source.profile?.runtime_ready ||
    !policy.parse_enabled ||
    source.foundation_policy_version !== policy.foundation_policy_version
  )
    throw Error('content_parse_not_ready');
  const request: ContentParsedTaskRequest = {
    source_ref: source.source_ref,
    expected_lifecycle_version: source.lifecycle_version,
    expected_policy_version: policy.foundation_policy_version,
    expected_content_policy_version: policy.content_policy_version,
    expected_producer_sha256: source.profile.producer_sha256,
    max_unit_bytes: ContentLimits.unit,
    confirmed: true,
    idempotency_key: key,
  };
  if (!contentSchemaValid('ParsedTaskRequest', request))
    throw Error('content_parse_not_ready');
  return request;
}
export const contentTaskTerminal = (task: ContentParsedTask) =>
  ['succeeded', 'failed', 'cancelled'].includes(task.state);
export async function checkContentTask(
  scope: ContentScope,
  task: ContentParsedTask,
  expected: {
    taskId?: string;
    request?: ContentParsedTaskRequest;
    sourceId?: string;
    previous?: ContentParsedTask;
  } = {},
) {
  const request = task.original_request;
  if (
    !contentSchemaValid('ParsedTask', task) ||
    task.owner_id !== scope.owner ||
    task.domain_id !== scope.domain ||
    request.source_ref.domain_id !== scope.domain ||
    task.producer_sha256 !== request.expected_producer_sha256 ||
    (expected.taskId && task.task_id !== expected.taskId) ||
    (expected.sourceId && request.source_ref.source_id !== expected.sourceId) ||
    (expected.request &&
      contentCanonical(request) !== contentCanonical(expected.request)) ||
    task.request_sha256 !==
      (await lifecycleDigest({
        kind: 'parsed-task/0.2.0',
        owner_id: scope.owner,
        domain_id: scope.domain,
        request: { ...request, source_ref: { ...request.source_ref } },
      })) ||
    dayjs(task.updated_at).isBefore(dayjs(task.created_at)) ||
    dayjs(task.deadline_at).isBefore(dayjs(task.created_at)) ||
    (task.completed_at !== null &&
      dayjs(task.completed_at).isBefore(dayjs(task.created_at)))
  )
    throw Error('content_task_binding');
  if (
    task.result &&
    (!contentFixedTarget(scope.domain, task.result) ||
      task.result.representation_ref.kind !== 'parsed_representation' ||
      contentCanonical(task.result.source_ref) !==
        contentCanonical(request.source_ref) ||
      task.result.representation_ref.producer_sha256 !== task.producer_sha256)
  )
    throw Error('content_task_result_binding');
  const old = expected.previous;
  if (
    old &&
    (old.task_id !== task.task_id ||
      old.request_sha256 !== task.request_sha256 ||
      old.created_at !== task.created_at ||
      old.deadline_at !== task.deadline_at ||
      task.revision < old.revision ||
      task.attempt_count < old.attempt_count ||
      dayjs(task.updated_at).isBefore(dayjs(old.updated_at)) ||
      ((task.revision === old.revision || contentTaskTerminal(old)) &&
        contentCanonical(task) !== contentCanonical(old)))
  )
    throw Error('content_task_revision');
}

export function contentParsedWorkflowRoute(params: URLSearchParams) {
  const source = params.get('parsed_source') ?? '',
    task = params.get('parsed_task') ?? '';
  if (
    ['parsed', 'parsed_source', 'parsed_task'].some(
      (name) => params.getAll(name).length > 1,
    ) ||
    (params.has('parsed') && params.get('parsed') !== '1') ||
    ((source || task) && params.get('parsed') !== '1') ||
    [...params.keys()].some(
      (name) =>
        (name.startsWith('parsed_') &&
          !['parsed_source', 'parsed_task'].includes(name)) ||
        ((source || task) && name.startsWith('content_')),
    ) ||
    (source && !Id.test(source)) ||
    (task && (!Id.test(task) || !source))
  )
    throw Error('content_invalid_parsed_route');
  return { source, task };
}

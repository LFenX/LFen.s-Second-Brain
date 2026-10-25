import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import dayjs from 'dayjs';
import Contract from '../../../docs/knowledge-batch-import-v1.openapi.json';
import {
  compareDirectoryPaths,
  validDirectoryPath,
} from './knowledge-directory-schema';

const Validator = new Ajv({ strict: false });
addFormats(Validator);
Validator.addFormat('knowledge-relative-file', {
  type: 'string',
  validate: validDirectoryPath,
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
export const BatchTerminalStatuses = [
  'succeeded',
  'partial_failed',
  'failed',
  'cancelled',
];
export function batchTerminal(status: string) {
  return BatchTerminalStatuses.includes(status);
}
function ordered(paths: string[]) {
  return paths.every(
    (p, i) => !i || compareDirectoryPaths(paths[i - 1], p) < 0,
  );
}
function unique(values: string[]) {
  return new Set(values).size === values.length;
}
function validCounts(v: Record<string, number>) {
  return (
    ['pending', 'running', 'succeeded', 'failed', 'cancelled'].reduce(
      (n, k) => n + v[k],
      0,
    ) === v.total
  );
}
function validPeriod(start: string, end: string) {
  const ttl = dayjs(end).valueOf() - dayjs(start).valueOf();
  return ttl > 0 && ttl <= 600000;
}
export function validBatchData(
  schema: string,
  data: unknown,
  httpStatus?: number,
): boolean {
  if (!Schemas[schema]?.(data)) return false;
  const name = schema.endsWith('Envelope') ? schema.slice(0, -8) : schema;
  const v = (
    schema.endsWith('Envelope') ? (data as { data: unknown }).data : data
  ) as Record<string, any>;
  if (name === 'Counts') return validCounts(v);
  if (name === 'PreviewRequest') {
    return ordered(
      v.selected_items.map((i: { relative_path: string }) => i.relative_path),
    );
  }
  if (name === 'PreviewItem' || name === 'BatchPreview') {
    const items = name === 'PreviewItem' ? [v] : v.items;
    if (
      !items.every((i: Record<string, unknown>) =>
        i.action === 'add'
          ? i.source_id === null && i.expected_revision_id === null
          : i.source_id !== null && i.expected_revision_id !== null,
      )
    )
      return false;
    if (name === 'BatchPreview')
      return (
        validPeriod(v.created_at, v.expires_at) &&
        v.byte_length ===
          v.items.reduce(
            (n: number, i: { byte_length: number }) => n + i.byte_length,
            0,
          ) &&
        ordered(
          v.items.map((i: { relative_path: string }) => i.relative_path),
        ) &&
        unique(v.items.map((i: { item_id: string }) => i.item_id)) &&
        v.items.every((i: { ordinal: number }, n: number) => i.ordinal === n)
      );
  }
  if (name === 'Task') {
    const c = v.counts,
      terminal = batchTerminal(v.status);
    if (
      !validCounts(c) ||
      c.total !== v.confirmed_item_ids.length ||
      terminal !== (v.terminal_at !== null) ||
      (terminal && c.pending + c.running !== 0) ||
      (!terminal && c.pending + c.running === 0)
    )
      return false;
    if (
      dayjs(v.execution_deadline).valueOf() - dayjs(v.created_at).valueOf() !==
        1800000 ||
      dayjs(v.updated_at).isBefore(dayjs(v.created_at)) ||
      (terminal && dayjs(v.terminal_at).isBefore(dayjs(v.created_at)))
    )
      return false;
    if (v.status === 'succeeded')
      return c.succeeded === c.total && !v.cancel_requested;
    if (v.status === 'partial_failed')
      return (
        c.succeeded > 0 &&
        c.failed > 0 &&
        c.succeeded + c.failed === c.total &&
        !v.cancel_requested
      );
    if (v.status === 'failed')
      return c.failed === c.total && !v.cancel_requested;
    if (v.status === 'cancelled')
      return v.cancel_requested && c.succeeded < c.total;
    if (v.status === 'cancelling') return v.cancel_requested;
    return !v.cancel_requested;
  }
  if (name === 'TaskItem') {
    const states: Record<string, [string, string]> = {
      pending: ['pending', 'awaiting_execution'],
      running: ['pending', 'running'],
      succeeded: ['registered', 'saved'],
      cancelled: ['cancelled', 'cancelled'],
    };
    if (v.status === 'failed')
      return (
        v.catalogue_status === 'failed' &&
        [
          'source_changed',
          'revision_changed',
          'root_changed',
          'policy_changed',
          'root_unready',
          'permission_revoked',
          'save_failed',
          'execution_expired',
        ].includes(v.code)
      );
    return (
      states[v.status]?.[0] === v.catalogue_status &&
      states[v.status]?.[1] === v.code
    );
  }
  if (name === 'TaskPage')
    return (
      validPeriod(v.captured_at, v.expires_at) &&
      v.items.length <= v.total_tasks &&
      unique(v.items.map((t: { task_id: string }) => t.task_id)) &&
      v.items.every(
        (t: Record<string, unknown>) =>
          t.domain_id === v.domain_id &&
          t.registration_id === v.registration_id &&
          validBatchData('Task', t),
      )
    );
  if (name === 'TaskItemsPage')
    return (
      validCounts(v.counts) &&
      v.items.length <= v.counts.total &&
      v.items.every(
        (i: Record<string, any>, n: number) =>
          validBatchData('TaskItem', i) &&
          (!n || v.items[n - 1].ordinal < i.ordinal),
      ) &&
      unique(v.items.map((i: { item_id: string }) => i.item_id))
    );
  if (name === 'ManifestPreviewPage' || name === 'ManifestVersionPage') {
    if (
      !ordered(
        v.items.map(
          (i: { registration_id: string; relative_path: string }) =>
            i.registration_id + '/' + i.relative_path,
        ),
      ) ||
      !unique(v.items.map((i: { source_id: string }) => i.source_id)) ||
      v.items.length > v.total_entries
    )
      return false;
    if (name === 'ManifestPreviewPage')
      return (
        validPeriod(v.created_at, v.expires_at) &&
        v.added + v.replaced + v.carried === v.total_entries
      );
  }
  if (name === 'ManifestHistoryPage')
    return (
      validPeriod(v.captured_at, v.expires_at) &&
      v.items.length <= v.total_manifests &&
      unique(v.items.map((i: { manifest_id: string }) => i.manifest_id)) &&
      v.items.every(
        (i: Record<string, unknown>) =>
          i.domain_id === v.domain_id && i.project_id === v.project_id,
      )
    );
  if (name === 'ContentWindow') {
    const bytes = new TextEncoder().encode(v.text),
      end = v.offset + v.byte_length;
    return (
      bytes.length === v.byte_length &&
      new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
        bytes,
      ) === v.text &&
      v.offset <= v.total_bytes &&
      end <= v.total_bytes &&
      (v.offset >= v.total_bytes || v.byte_length > 0) &&
      v.next_offset === (end < v.total_bytes ? end : null)
    );
  }
  if (name === 'Error' && httpStatus !== undefined) {
    const mappings = Contract['x-error-mapping'] as Record<
      string,
      { code: string; reasons: (string | null)[] }[]
    >;
    return !!mappings[String(httpStatus)]?.some(
      (i) => i.code === v.code && i.reasons.includes(v.reason),
    );
  }
  return true;
}

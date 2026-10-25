import { createHash } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-batch-import-v1.examples.json';
import type {
  BatchPreview,
  BatchTask,
  BatchTaskItem,
  BatchManifestEntry,
  BatchManifestPreviewPage,
  BatchManifestReceipt,
} from '@/interfaces/database/knowledge-batch';
import type {
  BatchPreviewRequest,
  BatchAcceptRequest,
  BatchManifestPreviewRequest,
} from '@/interfaces/request/knowledge-batch';
import {
  Registration as DirectoryRegistration,
  Root as DirectoryRoot,
  Capabilities as DirectoryCapabilities,
  respond as directoryRespond,
  reject as directoryReject,
  makeObservation as directoryObservation,
  sealObservation,
  FoundationCapabilities,
  foundationEnvelope,
  page,
} from './directory-fixture';
import { ImportCapabilities, c2Envelope } from './fixture';
import {
  Domain as FoundationDomain,
  Source as FoundationSource,
  Root as FoundationNode,
} from '../../knowledge-catalogue/__tests__/fixture';
export { FoundationCapabilities, foundationEnvelope, page };
export const Id = (n: number) => n.toString(16).padStart(32, '0');
export function copy<T>(id: string): T {
  const value = Examples.schema_cases.find((c) => c.id === id);
  if (!value) throw Error('Missing frozen fixture ' + id);
  return JSON.parse(JSON.stringify(value.data));
}
export const Raw = Examples.raw_byte_vectors;
export const Text = Buffer.from(Raw[0].hex, 'hex').toString('utf8');
export const sha = (text: string) =>
  createHash('sha256').update(text, 'utf8').digest('hex');
export const digest = (tokens: unknown[]) =>
  sha(
    tokens
      .map((t) => {
        const s = t === null ? '' : String(t);
        return Buffer.byteLength(s, 'utf8') + ':' + s;
      })
      .join(''),
  );
export const FrozenPreview = copy<BatchPreview>('fixed-preview');
export const DomainId = FrozenPreview.domain_id;
export const Domain = { ...FoundationDomain, domain_id: DomainId };
export const Domain2 = {
  ...Domain,
  domain_id: 'd'.repeat(32),
  name: 'Second synthetic batch domain',
};
export const ProjectId = FrozenPreview.items[0].classification.group_id!;
export const Project = {
  id: ProjectId,
  domain_id: DomainId,
  parent_id: null,
  kind: 'project' as const,
  name: 'Synthetic project',
  component_side: 'frontend' as const,
};
export const Registration = {
  ...DirectoryRegistration,
  domain_id: DomainId,
  registration_id: FrozenPreview.registration_id,
  root_id: FrozenPreview.root_id,
  default_classification: FrozenPreview.items[0].classification,
};
export const Root = {
  ...DirectoryRoot,
  root_id: Registration.root_id,
  root_version: 2,
};
export const Capabilities = copy('capability-not-live');
export function times() {
  const time = Date.now();
  return {
    created_at: new Date(time).toISOString(),
    expires_at: new Date(time + 600000).toISOString(),
  };
}
export function makeObservation(
  paths = ['web/卡片.tsx', 'web/空.ts'],
  n = 4,
  text = Text,
) {
  const value = directoryObservation(paths, n, Registration);
  value.root_version = 2;
  for (const item of value.items)
    if (item.eligible) {
      const content = item.relative_path.includes('空') ? '' : text;
      item.sha256 = sha(content);
      item.byte_length = Buffer.byteLength(content);
    }
  return sealObservation(value);
}
export function batchTokens(v: BatchPreview, user = Id(8)) {
  return [
    'lfen.batch.preview.v0.4',
    v.preview_id,
    v.domain_id,
    user,
    v.registration_id,
    v.observation_id,
    v.observation_sha256,
    v.root_id,
    v.root_version,
    v.policy_version,
    v.created_at,
    v.expires_at,
    v.items.length,
    ...v.items.flatMap((i) => [
      i.item_id,
      i.ordinal,
      i.relative_path,
      i.source_id,
      i.expected_revision_id,
      i.sha256,
      i.byte_length,
      i.classification.group_id,
      i.classification.component_side,
      i.classification.language,
      i.action,
    ]),
  ];
}
export function manifestTokens(v: BatchManifestPreviewPage) {
  return [
    'lfen.project.manifest.preview.v0.4',
    v.preview_id,
    v.domain_id,
    v.project_id,
    v.task_id,
    v.task_manifest_sha256,
    v.expected_manifest_id,
    v.coverage,
    v.captured_atomically,
    v.created_at,
    v.expires_at,
    v.items.length,
    v.added,
    v.replaced,
    v.carried,
    ...v.items.flatMap((i) => [
      i.registration_id,
      i.relative_path,
      i.source_id,
      i.source_revision_id,
      i.root_node_id,
      i.sha256,
      i.byte_length,
      i.classification.group_id,
      i.classification.component_side,
      i.classification.language,
      i.origin_task_id,
    ]),
  ];
}
export function makePreview(
  observation = makeObservation(),
  request?: BatchPreviewRequest,
  n = 1,
): BatchPreview {
  const value = copy<BatchPreview>('fixed-preview'),
    selected =
      request?.selected_items ||
      observation.items
        .filter((i) => i.eligible)
        .map((i) => ({
          relative_path: i.relative_path,
          classification: observation.default_classification,
        }));
  Object.assign(value, times(), {
    preview_id: Id(n),
    domain_id: observation.domain_id,
    registration_id: observation.registration_id,
    root_id: observation.root_id,
    root_version: observation.root_version,
    policy_version: observation.policy_version,
    observation_id: observation.observation_id,
    observation_sha256: observation.observation_sha256,
    expires_at: observation.expires_at,
  });
  value.items = selected.map((s, ordinal) => {
    const before = observation.items.find(
      (i) => i.relative_path === s.relative_path,
    )!;
    return {
      item_id: Id(100 + n * 32 + ordinal),
      ordinal,
      relative_path: s.relative_path,
      classification: s.classification,
      sha256: before.sha256!,
      byte_length: before.byte_length!,
      source_id: before.source_id,
      expected_revision_id: before.expected_revision_id,
      action: before.source_id ? 'update' : 'add',
    };
  });
  value.byte_length = value.items.reduce((n, i) => n + i.byte_length, 0);
  value.manifest_sha256 = digest(batchTokens(value));
  return value;
}
export function acceptRequest(
  preview = makePreview(),
  key = 'synthetic-batch-001',
  parent: string | null = null,
): BatchAcceptRequest {
  return {
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: key,
    confirmed_item_ids: preview.items.map((i) => i.item_id),
    retry_of_task_id: parent,
  };
}
export function makeTask(
  preview = makePreview(),
  status: BatchTask['status'] = 'queued',
  request = acceptRequest(preview),
  n = 9,
): BatchTask {
  const value = copy<BatchTask>('queued-not-success'),
    now = Date.now(),
    total = preview.items.length;
  Object.assign(value, {
    ...request,
    task_id: Id(n),
    domain_id: preview.domain_id,
    registration_id: preview.registration_id,
    status,
    revision: status === 'queued' ? 1 : 5,
    cancel_requested: status === 'cancelled' || status === 'cancelling',
    created_at: new Date(now).toISOString(),
    updated_at: new Date(now).toISOString(),
    execution_deadline: new Date(now + 1800000).toISOString(),
    terminal_at: ['queued', 'running', 'cancelling'].includes(status)
      ? null
      : new Date(now).toISOString(),
  });
  value.counts = {
    total,
    pending: 0,
    running: 0,
    succeeded: 0,
    failed: 0,
    cancelled: 0,
  };
  if (status === 'queued') value.counts.pending = total;
  if (status === 'running' || status === 'cancelling') {
    value.counts.running = 1;
    value.counts.pending = total - 1;
  }
  if (status === 'succeeded') value.counts.succeeded = total;
  if (status === 'failed') value.counts.failed = total;
  if (status === 'partial_failed') {
    value.counts.succeeded = 1;
    value.counts.failed = total - 1;
  }
  if (status === 'cancelled') {
    value.counts.succeeded = total > 1 ? 1 : 0;
    value.counts.cancelled = total - value.counts.succeeded;
  }
  return value;
}
export function makeResults(
  task: BatchTask,
  preview: BatchPreview,
): BatchTaskItem[] {
  const sequence = [
    'succeeded',
    'failed',
    'cancelled',
    'running',
    'pending',
  ] as const;
  const statuses = sequence.flatMap((s) => Array(task.counts[s]).fill(s));
  return preview.items.map((i, n) => {
    const value = copy<BatchTaskItem>('fixed-success'),
      status = statuses[n],
      source = sha(preview.registration_id + '/' + i.relative_path).slice(
        0,
        32,
      );
    Object.assign(value, {
      item_id: i.item_id,
      ordinal: n,
      relative_path: i.relative_path,
      classification: i.classification,
      sha256: i.sha256,
      byte_length: i.byte_length,
      status,
      save_status:
        status === 'succeeded'
          ? i.action === 'add'
            ? 'created'
            : i.action === 'update'
              ? 'updated'
              : 'unchanged'
          : null,
      source_id: status === 'succeeded' ? i.source_id || source : null,
      source_revision_id:
        status === 'succeeded'
          ? sha(
              source + '/' + i.sha256 + '/' + JSON.stringify(i.classification),
            ).slice(0, 32)
          : null,
      root_node_id:
        status === 'succeeded'
          ? sha(
              'node/' +
                source +
                '/' +
                i.sha256 +
                '/' +
                JSON.stringify(i.classification),
            )
          : null,
      catalogue_status:
        status === 'succeeded'
          ? 'registered'
          : status === 'failed'
            ? 'failed'
            : status === 'cancelled'
              ? 'cancelled'
              : 'pending',
      code:
        status === 'succeeded'
          ? 'saved'
          : status === 'failed'
            ? 'source_changed'
            : status === 'cancelled'
              ? 'cancelled'
              : status === 'running'
                ? 'running'
                : 'awaiting_execution',
    });
    return value;
  });
}
export function makeManifest(
  task: BatchTask,
  results: BatchTaskItem[],
  request: BatchManifestPreviewRequest,
  base: BatchManifestEntry[] = [],
  n = 30,
): BatchManifestPreviewPage {
  const value = copy<BatchManifestPreviewPage>('project-preview'),
    entries = new Map(base.map((i) => [i.source_id, i]));
  let added = 0,
    replaced = 0;
  for (const i of results) {
    if (entries.has(i.source_id!)) replaced += 1;
    else added += 1;
    entries.set(i.source_id!, {
      registration_id: task.registration_id,
      relative_path: i.relative_path,
      source_id: i.source_id!,
      source_revision_id: i.source_revision_id!,
      root_node_id: i.root_node_id!,
      sha256: i.sha256,
      byte_length: i.byte_length,
      classification: i.classification,
      origin_task_id: task.task_id,
    });
  }
  Object.assign(value, times(), {
    preview_id: Id(n),
    domain_id: task.domain_id,
    project_id: request.project_id,
    task_id: task.task_id,
    task_manifest_sha256: task.manifest_sha256,
    expected_manifest_id: request.expected_manifest_id,
    added,
    replaced,
    carried: base.length - replaced,
  });
  value.items = [...entries.values()].sort((a, b) =>
    Buffer.compare(
      Buffer.from(a.registration_id + '/' + a.relative_path),
      Buffer.from(b.registration_id + '/' + b.relative_path),
    ),
  );
  value.total_entries = value.items.length;
  value.manifest_sha256 = digest(manifestTokens(value));
  return value;
}
export function makeReceipt(
  preview: BatchManifestPreviewPage,
  key = 'synthetic-publication-001',
  n = 32,
): BatchManifestReceipt {
  const value = copy<BatchManifestReceipt>('publication-receipt');
  Object.assign(value, {
    domain_id: preview.domain_id,
    project_id: preview.project_id,
    manifest_id: Id(n),
    preview_id: preview.preview_id,
    task_id: preview.task_id,
    manifest_sha256: preview.manifest_sha256,
    idempotency_key: key,
    expected_manifest_id: preview.expected_manifest_id,
    total_entries: preview.total_entries,
    published_at: new Date().toISOString(),
  });
  return value;
}
export function content(
  preview: BatchPreview,
  itemId: string,
  offset = 0,
  length = 65536,
  text = Text,
) {
  const item = preview.items.find((i) => i.item_id === itemId)!;
  const bytes = Buffer.from(item.byte_length === 0 ? '' : text, 'utf8'),
    end = Math.min(bytes.length, offset + length),
    decoded = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      bytes.subarray(offset, end),
    );
  return {
    preview_id: preview.preview_id,
    item_id: itemId,
    relative_path: item.relative_path,
    sha256: item.sha256,
    total_bytes: bytes.length,
    offset,
    byte_length: end - offset,
    text: decoded,
    next_offset: end < bytes.length ? end : null,
  };
}
export const envelope = (data: unknown) => ({
  code: 0,
  message: 'Synthetic only',
  data,
  contract_version: '1.0.0',
  source_contract_version: '0.4.0',
  request_id: Id(90),
});
export const respond = (data: unknown, status = 200) => ({
  status,
  data: envelope(data),
});
export const reject = (
  status: number,
  code: string,
  reason: string | null = null,
) => ({
  response: {
    status,
    data: {
      code,
      message: 'Synthetic failure',
      reason,
      contract_version: '1.0.0',
      source_contract_version: '0.4.0',
      request_id: Id(90),
    },
  },
});
export function c1Projection(item: BatchTaskItem) {
  const source = {
    ...FoundationSource,
    domain_id: DomainId,
    source_id: item.source_id!,
    source_revision_id: item.source_revision_id!,
    root_node_id: item.root_node_id!,
    relative_path: item.relative_path,
    group_id: item.classification.group_id,
    component_side: item.classification.component_side,
    language: item.classification.language,
    sha256: item.sha256,
    byte_length: item.byte_length,
    line_count: item.byte_length ? 3 : 0,
  };
  const root = {
    ...FoundationNode,
    domain_id: DomainId,
    node_id: item.root_node_id!,
    source_revision_id: item.source_revision_id!,
    end_byte: item.byte_length,
    end_line: item.byte_length ? 3 : 1,
  };
  return { source, root };
}

export function createBatchBackend() {
  const records = new Map<
      string,
      { task: BatchTask; preview: BatchPreview; results: BatchTaskItem[] }
    >(),
    keys = new Map<string, string>(),
    previews = new Map<string, BatchPreview>();
  const manifestPreviews = new Map<string, BatchManifestPreviewPage>(),
    versions = new Map<
      string,
      { receipt: BatchManifestReceipt; items: BatchManifestEntry[] }
    >(),
    publicationKeys = new Map<string, BatchManifestReceipt>();
  const catalogue = new Map<string, BatchTaskItem>(),
    currentSources = new Map<string, BatchTaskItem>(),
    texts = new Map([
      [sha(Text), Text],
      [sha(''), ''],
    ]);
  let currentRegistration = { ...Registration },
    observation = makeObservation(),
    taskSnapshot: string[] = [],
    historySnapshot: string[] = [];
  let taskStamp = times(),
    historyStamp = times(),
    sequence = 1;
  const state: {
    bodyText: string;
    rootReady: boolean;
    rootRevoked: boolean;
    domainDenied: boolean;
    versionMismatch: boolean;
    acceptMode: string;
    publicationMode: string;
    taskStatus: BatchTask['status'];
    taskFailures: number;
    byKeyInvisible: boolean;
    resultChangedOnNext: boolean;
    badWindow: boolean;
    head: string | null;
    lastTaskId: string;
    lastPreview: BatchPreview | null;
    lastManifest: BatchManifestPreviewPage | null;
    taskListCount: number;
    registrationReads: number;
    registrationPosts: number;
    observationPosts: number;
    acceptRequests: BatchAcceptRequest[];
    prepareRequests: BatchPreviewRequest[];
    publishRequests: import('@/interfaces/request/knowledge-batch').BatchManifestPublicationRequest[];
    cancelRequests: string[];
    records: typeof records;
    versions: typeof versions;
    publicationKeys: typeof publicationKeys;
    finish(status: BatchTask['status'], id?: string): BatchTask;
    seedTasks(count: number): void;
    seedProject(entries: number, historyCount?: number): void;
    handler(
      config: Record<string, any>,
    ): Promise<{ status: number; data: unknown }>;
  } = {
    bodyText: Text,
    rootReady: true,
    rootRevoked: false,
    domainDenied: false,
    versionMismatch: false,
    acceptMode: 'success',
    publicationMode: 'success',
    taskStatus: 'queued' as BatchTask['status'],
    taskFailures: 0,
    byKeyInvisible: false,
    resultChangedOnNext: false,
    badWindow: false,
    head: null as string | null,
    lastTaskId: '',
    lastPreview: null as BatchPreview | null,
    lastManifest: null as BatchManifestPreviewPage | null,
    taskListCount: 0,
    registrationReads: 0,
    registrationPosts: 0,
    observationPosts: 0,
    acceptRequests: [] as BatchAcceptRequest[],
    prepareRequests: [] as BatchPreviewRequest[],
    publishRequests:
      [] as import('@/interfaces/request/knowledge-batch').BatchManifestPublicationRequest[],
    cancelRequests: [] as string[],
    records,
    versions,
    publicationKeys,
    finish(
      status: BatchTask['status'],
      id: string = state.lastTaskId,
    ): BatchTask {
      const record = records.get(id);
      if (!record) throw Error('Unknown synthetic task');
      const next = makeTask(
        record.preview,
        status,
        {
          preview_id: record.task.preview_id,
          manifest_sha256: record.task.manifest_sha256,
          idempotency_key: record.task.idempotency_key,
          confirmed_item_ids: record.task.confirmed_item_ids,
          retry_of_task_id: record.task.retry_of_task_id,
        },
        Number.parseInt(id, 16),
      );
      Object.assign(next, {
        created_at: record.task.created_at,
        execution_deadline: record.task.execution_deadline,
        revision: record.task.revision + 1,
      });
      record.task = next;
      record.results = makeResults(next, record.preview);
      for (const i of record.results)
        if (i.status === 'succeeded') {
          catalogue.set(i.root_node_id!, i);
          currentSources.set(i.relative_path, i);
        }
      return next;
    },
    seedTasks(count: number) {
      for (let n = 0; n < count; n += 1) {
        const p = makePreview(makeObservation(), undefined, 100 + n),
          r = acceptRequest(p, 'listed-' + n),
          t = makeTask(p, 'queued', r, 1000 + n);
        records.set(t.task_id, {
          task: t,
          preview: p,
          results: makeResults(t, p),
        });
      }
    },
    seedProject(entries: number, historyCount = 1) {
      const p = makePreview(),
        t = makeTask(p, 'succeeded'),
        rows = makeResults(t, p),
        m = makeManifest(t, rows, {
          project_id: ProjectId,
          task_id: t.task_id,
          expected_manifest_id: null,
        });
      const all = Array.from({ length: entries }, (_, n) => ({
        ...m.items[0],
        registration_id: Id(80),
        relative_path: 'base/file-' + String(n).padStart(4, '0') + '.ts',
        source_id: Id(10000 + n),
        source_revision_id: Id(12000 + n),
        root_node_id: sha('base-node-' + n),
      }));
      for (let n = 0; n < historyCount; n += 1) {
        const r = {
          ...makeReceipt(m, 'seed-' + n, 2000 + n),
          total_entries: entries,
        };
        versions.set(r.manifest_id, { receipt: r, items: all });
        state.head = r.manifest_id;
      }
      for (const i of all)
        catalogue.set(i.root_node_id, {
          ...rows[0],
          ...i,
          status: 'succeeded',
          save_status: 'created',
          catalogue_status: 'registered',
          code: 'saved',
        });
    },
    async handler(
      config: Record<string, any>,
    ): Promise<{ status: number; data: unknown }> {
      const url = config.url as string,
        body =
          typeof config.data === 'string'
            ? JSON.parse(config.data)
            : config.data,
        params = config.params || {};
      if (url === '/api/v1/knowledge/batch-import-contract')
        return state.versionMismatch
          ? {
              ...respond(Capabilities),
              data: {
                ...envelope(Capabilities),
                source_contract_version: '0.3.0',
              },
            }
          : respond(Capabilities);
      if (url === '/api/v1/knowledge/directory-import-contract')
        return directoryRespond(DirectoryCapabilities);
      if (url === '/api/v1/knowledge/import-contract')
        return { status: 200, data: c2Envelope(ImportCapabilities) };
      if (url === '/api/v1/knowledge/contract')
        return {
          status: 200,
          data: foundationEnvelope(FoundationCapabilities),
        };
      if (url === '/api/v1/knowledge/domains')
        return {
          status: 200,
          data: foundationEnvelope(page([Domain, Domain2])),
        };
      if (url.endsWith('/knowledge/domain')) {
        if (state.domainDenied)
          throw {
            response: {
              status: 401,
              data: { code: 401, message: 'Synthetic login required' },
            },
          };
        return {
          status: 200,
          data: foundationEnvelope(
            url.includes(Domain2.domain_id) ? Domain2 : Domain,
          ),
        };
      }
      if (url.endsWith('/knowledge/groups'))
        return {
          status: 200,
          data: foundationEnvelope(page(params.parent_id ? [] : [Project])),
        };
      if (url.endsWith('/directory-roots'))
        return directoryRespond({
          domain_id: DomainId,
          items: [
            {
              ...Root,
              ready: state.rootReady,
              reason: state.rootReady ? null : 'unavailable',
            },
          ],
        });
      if (
        url.endsWith('/directory-registrations') &&
        config.method === 'post'
      ) {
        state.registrationPosts += 1;
        currentRegistration = { ...Registration, ...body };
        return directoryRespond(currentRegistration, 201);
      }
      if (
        url.endsWith('/directory-registrations/' + Registration.registration_id)
      ) {
        state.registrationReads += 1;
        if (state.rootRevoked)
          throw directoryReject(404, 'knowledge_directory_not_found');
        return directoryRespond(currentRegistration);
      }
      if (url.endsWith('/observations') && config.method === 'post') {
        state.observationPosts += 1;
        if (state.rootRevoked)
          throw directoryReject(404, 'knowledge_directory_not_found');
        if (!state.rootReady)
          throw directoryReject(
            503,
            'knowledge_directory_unavailable',
            'root_unready',
          );
        observation = makeObservation(
          body.selected_paths,
          40 + state.observationPosts,
          state.bodyText,
        );
        observation.default_classification =
          currentRegistration.default_classification;
        for (const i of observation.items)
          if (i.eligible) {
            const old = currentSources.get(i.relative_path);
            if (old) {
              i.source_id = old.source_id;
              i.expected_revision_id = old.source_revision_id;
              i.change_kind = old.sha256 === i.sha256 ? 'unchanged' : 'update';
            }
          }
        texts.set(sha(state.bodyText), state.bodyText);
        sealObservation(observation);
        return directoryRespond(observation, 201);
      }
      if (
        url.includes('/directory-registrations/') &&
        url.includes('/batch-') &&
        state.rootRevoked
      )
        throw reject(404, 'knowledge_batch_not_found');
      if (url.endsWith('/batch-previews') && config.method === 'post') {
        state.prepareRequests.push(body);
        const value = makePreview(observation, body, sequence++);
        state.lastPreview = value;
        for (const i of value.items) {
          const old = currentSources.get(i.relative_path);
          if (
            old &&
            old.sha256 === i.sha256 &&
            JSON.stringify(old.classification) ===
              JSON.stringify(i.classification)
          )
            i.action = 'unchanged';
        }
        value.manifest_sha256 = digest(batchTokens(value));
        previews.set(value.preview_id, value);
        return respond(value, 201);
      }
      if (url.includes('/batch-previews/') && url.endsWith('/content')) {
        const pieces = url.split('/'),
          preview = previews.get(pieces.at(-4)!)!,
          item = pieces.at(-2)!;
        if (preview.consumed_task_id)
          throw reject(409, 'knowledge_batch_conflict', 'preview_consumed');
        try {
          const result = content(
            preview,
            item,
            params.offset,
            params.length,
            texts.get(preview.items.find((i) => i.item_id === item)!.sha256) ||
              '',
          );
          return respond(
            state.badWindow ? { ...result, item_id: Id(999) } : result,
          );
        } catch {
          throw reject(416, 'knowledge_batch_range');
        }
      }
      if (url.includes('/batch-previews/'))
        return respond(previews.get(url.split('/').at(-1)!));
      if (url.endsWith('/batch-tasks') && config.method === 'post') {
        state.acceptRequests.push(body);
        const existing = keys.get(body.idempotency_key);
        if (existing) return respond(records.get(existing)!.task);
        if (state.acceptMode === 'timeout-before')
          throw Error('Synthetic lost acceptance before commit');
        if (state.acceptMode === 'expired')
          throw reject(410, 'knowledge_batch_expired');
        const preview = previews.get(body.preview_id)!;
        const task = makeTask(
            preview,
            state.taskStatus,
            body,
            500 + records.size,
          ),
          results = makeResults(task, preview);
        keys.set(body.idempotency_key, task.task_id);
        records.set(task.task_id, { task, preview, results });
        state.lastTaskId = task.task_id;
        preview.consumed_task_id = task.task_id;
        for (const i of results)
          if (i.status === 'succeeded') {
            catalogue.set(i.root_node_id!, i);
            currentSources.set(i.relative_path, i);
          }
        if (state.acceptMode === 'timeout-after')
          throw Error('Synthetic lost acceptance after commit');
        return respond(task, 202);
      }
      if (url.includes('/batch-tasks/by-key/')) {
        const id = keys.get(url.split('/').at(-1)!);
        if (!id || state.byKeyInvisible)
          throw reject(404, 'knowledge_batch_not_found');
        return respond(records.get(id)!.task);
      }
      if (url.endsWith('/cancellation')) {
        state.cancelRequests.push(body.idempotency_key);
        const id = url.split('/').at(-2)!,
          record = records.get(id)!;
        if (
          !['succeeded', 'failed', 'partial_failed', 'cancelled'].includes(
            record.task.status,
          )
        )
          state.finish('cancelled', id);
        return respond(record.task);
      }
      if (url.endsWith('/batch-tasks') && config.method === 'get') {
        if (!params.cursor) {
          if (state.taskListCount > records.size)
            state.seedTasks(state.taskListCount - records.size);
          taskSnapshot = [...records.keys()];
          taskStamp = times();
        }
        const start = params.cursor
          ? Number(String(params.cursor).split('_')[1])
          : 0;
        return respond({
          domain_id: DomainId,
          registration_id: Registration.registration_id,
          list_snapshot_id: Id(900),
          total_tasks: taskSnapshot.length,
          captured_at: taskStamp.created_at,
          expires_at: taskStamp.expires_at,
          items: taskSnapshot
            .slice(start, start + 20)
            .map((id) => records.get(id)!.task),
          next_cursor:
            start + 20 < taskSnapshot.length ? 'tasks_' + (start + 20) : null,
        });
      }
      if (url.includes('/batch-tasks/') && url.endsWith('/items')) {
        const id = url.split('/').at(-2)!,
          record = records.get(id)!;
        if (params.cursor && state.resultChangedOnNext) {
          state.resultChangedOnNext = false;
          record.task = { ...record.task, revision: record.task.revision + 1 };
          throw reject(409, 'knowledge_batch_conflict', 'result_changed');
        }
        const start = params.cursor
          ? Number(String(params.cursor).split('_')[1])
          : 0;
        return respond({
          task_id: id,
          registration_id: Registration.registration_id,
          result_revision: record.task.revision,
          status: record.task.status,
          counts: record.task.counts,
          items: record.results.slice(start, start + 10),
          next_cursor:
            start + 10 < record.results.length ? 'items_' + (start + 10) : null,
        });
      }
      if (url.includes('/batch-tasks/')) {
        if (state.taskFailures > 0) {
          state.taskFailures -= 1;
          throw reject(503, 'knowledge_batch_unavailable', 'result_unknown');
        }
        const record = records.get(url.split('/').at(-1)!);
        if (!record) throw reject(404, 'knowledge_batch_not_found');
        return respond(record.task);
      }
      if (url.endsWith('/manifest-head'))
        return respond({
          domain_id: DomainId,
          project_id: ProjectId,
          manifest_id: state.head,
        });
      if (
        url.endsWith('/project-manifest-previews') &&
        config.method === 'post'
      ) {
        const record = records.get(body.task_id);
        if (!record || record.task.status !== 'succeeded')
          throw reject(409, 'knowledge_batch_conflict', 'task_not_succeeded');
        if (record.results.some((i) => i.classification.group_id !== ProjectId))
          throw reject(
            409,
            'knowledge_batch_conflict',
            'project_membership_changed',
          );
        if (body.expected_manifest_id !== state.head)
          throw reject(409, 'knowledge_batch_conflict', 'project_head_changed');
        const base = state.head ? versions.get(state.head)!.items : [],
          value = makeManifest(
            record.task,
            record.results,
            body,
            base,
            700 + manifestPreviews.size,
          );
        manifestPreviews.set(value.preview_id, value);
        state.lastManifest = value;
        return respond(
          {
            ...value,
            items: value.items.slice(0, 50),
            next_cursor: value.items.length > 50 ? 'manifest_50' : null,
          },
          201,
        );
      }
      if (url.includes('/project-manifest-previews/')) {
        const value = manifestPreviews.get(url.split('/').at(-1)!)!,
          start = params.cursor
            ? Number(String(params.cursor).split('_')[1])
            : 0;
        return respond({
          ...value,
          items: value.items.slice(start, start + 50),
          next_cursor:
            start + 50 < value.items.length ? 'manifest_' + (start + 50) : null,
        });
      }
      if (url.endsWith('/project-manifest-publications')) {
        state.publishRequests.push(body);
        const existing = publicationKeys.get(body.idempotency_key);
        if (existing) return respond(existing);
        if (state.publicationMode === 'timeout-before')
          throw Error('Synthetic lost publication before commit');
        const value = manifestPreviews.get(body.preview_id)!;
        if (state.publicationMode === 'cas') {
          const foreign = {
            ...makeReceipt(value, 'foreign-publication', 777),
            manifest_id: Id(777),
          };
          versions.set(foreign.manifest_id, {
            receipt: foreign,
            items: value.items,
          });
          state.head = foreign.manifest_id;
          throw reject(409, 'knowledge_batch_conflict', 'project_head_changed');
        }
        if (value.expected_manifest_id !== state.head)
          throw reject(409, 'knowledge_batch_conflict', 'project_head_changed');
        const receipt = makeReceipt(
          value,
          body.idempotency_key,
          800 + versions.size,
        );
        state.head = receipt.manifest_id;
        value.consumed_manifest_id = receipt.manifest_id;
        versions.set(receipt.manifest_id, { receipt, items: value.items });
        publicationKeys.set(body.idempotency_key, receipt);
        if (state.publicationMode === 'timeout-after')
          throw Error('Synthetic lost publication after commit');
        return respond(receipt, 201);
      }
      if (url.includes('/manifest-publications/by-key/')) {
        const value = publicationKeys.get(url.split('/').at(-1)!);
        if (!value) throw reject(404, 'knowledge_batch_not_found');
        return respond(value);
      }
      if (url.endsWith('/manifests')) {
        if (!params.cursor) {
          historySnapshot = [...versions.keys()];
          historyStamp = times();
        }
        const start = params.cursor
          ? Number(String(params.cursor).split('_')[1])
          : 0;
        return respond({
          domain_id: DomainId,
          project_id: ProjectId,
          list_snapshot_id: Id(901),
          total_manifests: historySnapshot.length,
          captured_at: historyStamp.created_at,
          expires_at: historyStamp.expires_at,
          items: historySnapshot
            .slice(start, start + 20)
            .map((id) => versions.get(id)!.receipt),
          next_cursor:
            start + 20 < historySnapshot.length
              ? 'history_' + (start + 20)
              : null,
        });
      }
      if (url.includes('/manifests/')) {
        const value = versions.get(url.split('/').at(-1)!);
        if (!value) throw reject(404, 'knowledge_batch_not_found');
        const { idempotency_key: _key, ...metadata } = value.receipt;
        void _key;
        const start = params.cursor
          ? Number(String(params.cursor).split('_')[1])
          : 0;
        return respond({
          ...metadata,
          items: value.items.slice(start, start + 50),
          next_cursor:
            start + 50 < value.items.length ? 'version_' + (start + 50) : null,
        });
      }
      if (url.endsWith('/knowledge/sources'))
        return {
          status: 200,
          data: foundationEnvelope(
            page(
              [...currentSources.values()].map((i) => c1Projection(i).source),
            ),
          ),
        };
      if (url.includes('/knowledge/sources/')) {
        const item = [...catalogue.values()].find(
          (i) => i.source_revision_id === url.split('/').at(-1),
        );
        if (!item) throw Error('Missing synthetic C1 revision');
        return {
          status: 200,
          data: foundationEnvelope(c1Projection(item).source),
        };
      }
      if (url.includes('/knowledge/nodes/') && url.endsWith('/content')) {
        const item = catalogue.get(url.split('/').at(-2)!)!,
          { source, root } = c1Projection(item),
          text = texts.get(item.sha256) || '';
        return {
          status: 200,
          data: foundationEnvelope({
            node_id: root.node_id,
            source_id: source.source_id,
            source_revision_id: source.source_revision_id,
            source_sha256: source.sha256,
            start_byte: 0,
            end_byte: item.byte_length,
            encoding: 'utf-8',
            text,
          }),
        };
      }
      if (url.includes('/knowledge/nodes/'))
        return {
          status: 200,
          data: foundationEnvelope(
            c1Projection(catalogue.get(url.split('/').at(-1)!)!).root,
          ),
        };
      if (url.endsWith('/knowledge/nodes'))
        return { status: 200, data: foundationEnvelope(page([])) };
      throw Error('Unexpected synthetic request ' + url);
    },
  };
  return state;
}

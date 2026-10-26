import type { AxiosResponse } from 'axios';
import type {
  DocumentCapabilities,
  DocumentPolicy,
  DocumentTask,
  DocumentHead,
  DocumentRepresentation,
  DocumentNode,
  DocumentNodePage,
  DocumentContentWindow,
  DocumentPolicyReceipt,
  DocumentCancelReceipt,
  DocumentProfile,
  DocumentOperationRecord,
} from '@/interfaces/database/knowledge-document';
import type {
  DocumentTaskRequest,
  DocumentPolicyChangeRequest,
  DocumentCancelRequest,
  DocumentOperationQuery,
  DocumentNodeQuery,
  DocumentContentQuery,
} from '@/interfaces/request/knowledge-document';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import { registerNextServer } from '@/utils/register-server';
import {
  DocumentId,
  DocumentHash,
  documentJsonBytes,
  validDocumentData,
  parseDocumentJson,
} from '@/utils/knowledge-document-schema';
import {
  checkDocumentOutcome,
  checkDocumentNode,
  checkDocumentContent,
  sameDocumentOriginal,
  sameDocumentValue,
  type DocumentScope,
  type DocumentSourceScope,
  type DocumentViewScope,
  type DocumentOperation,
} from '@/utils/knowledge-document';
import foundation, {
  KnowledgeRequestError,
} from './knowledge-foundation-service';
import originals from './knowledge-original-service';
import lifecycle from './knowledge-lifecycle-service';

export class DocumentRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId = '',
    public outcomeKnown = false,
    public rejectionId: string | null = null,
  ) {
    super(status, code, requestId);
  }
}
export type DocumentResponse<T> = { data: T; httpStatus: number };
export type DocumentPrepared = {
  request: Omit<DocumentTaskRequest, 'confirmed' | 'idempotency_key'>;
  original: OriginalRevision;
  snapshot: {
    lifecycle_version: number;
    relative_path: string;
    classification: OriginalRevision['classification'];
  };
  profile: DocumentProfile;
};
const Server = registerNextServer({
  read: { url: (c: { url: string }) => c.url, method: 'get' },
  write: { url: (c: { url: string }) => c.url, method: 'post' },
});
const Headers = {
  'X-Knowledge-Contract-Version': '1.0.0',
  'X-Knowledge-Document-Structure-Contract-Version': '0.1.0',
};
const invalid = (status = 502) =>
  new DocumentRequestError(
    status,
    status === 400 ? 'invalid_request' : 'document_integrity',
  );
function id(value: string, hash = false) {
  if (!(hash ? DocumentHash : DocumentId).test(value)) throw invalid(400);
  return value;
}
function scope(s: DocumentScope) {
  id(s.owner);
  id(s.domain);
}
const domainPath = (s: DocumentScope) => {
  scope(s);
  return `/datasets/${id(s.domain)}/knowledge`;
};
const representationPath = (s: DocumentViewScope) => {
  id(s.source);
  id(s.revision);
  id(s.sha, true);
  return (
    domainPath(s) + '/document-representations/' + id(s.representation, true)
  );
};
function header(r: AxiosResponse, name: string) {
  return String(
    Object.entries(r.headers ?? {}).find(
      ([k]) => k.toLowerCase() === name.toLowerCase(),
    )?.[1] ?? '',
  );
}
function headersValid(r: AxiosResponse) {
  return (
    Object.entries(Headers).every(([k, v]) => header(r, k) === v) &&
    header(r, 'Cache-Control') === 'no-store' &&
    DocumentId.test(header(r, 'X-Request-ID')) &&
    header(r, 'X-Request-ID') === r.data?.request_id
  );
}
async function failure(error: unknown): Promise<never> {
  const issue = error as { code?: string; response?: AxiosResponse };
  if (issue.code === 'ERR_CANCELED') throw error;
  const r = issue.response;
  if (!r) throw new DocumentRequestError(0, 'document_result_unknown');
  if (
    !headersValid(r) ||
    ![400, 401, 404, 409, 412, 429, 503].includes(r.status) ||
    !(await validDocumentData('Error' + r.status, r.data))
  )
    throw invalid();
  throw new DocumentRequestError(
    r.status,
    r.data.code,
    r.data.request_id,
    r.data.outcome_known,
    r.data.rejection_id,
  );
}
async function call<T>(
  path: string,
  schema: string,
  options: {
    body?: unknown;
    requestSchema?: string;
    params?: object;
    querySchema?: string;
    signal?: AbortSignal;
    bindings?: Record<string, unknown>;
  } = {},
): Promise<DocumentResponse<T>> {
  if (
    options.requestSchema &&
    !(await validDocumentData(options.requestSchema, options.body))
  )
    throw invalid(400);
  if (
    options.querySchema &&
    !(await validDocumentData(options.querySchema, options.params))
  )
    throw invalid(400);
  if (options.body !== undefined && documentJsonBytes(options.body) > 32768)
    throw invalid(400);
  let response: AxiosResponse;
  try {
    response = await Server[options.body === undefined ? 'read' : 'write'](
      {
        url: '/api/v1' + path,
        params: options.params,
        data:
          options.body === undefined ? undefined : JSON.stringify(options.body),
        signal: options.signal,
        timeout: 25000,
        skipGlobalErrorNotification: true,
        transformResponse: [parseDocumentJson],
        headers: {
          ...Headers,
          ...(options.body === undefined
            ? {}
            : { 'Content-Type': 'application/json' }),
        },
      },
      true,
    );
  } catch (error) {
    return failure(error);
  }
  if (
    response.data?.contract_version !== '1.0.0' ||
    response.data?.document_structure_contract_version !== '0.1.0'
  )
    throw new DocumentRequestError(412, 'contract_mismatch');
  if (
    !(options.body === undefined ? [200] : [200, 201]).includes(
      response.status,
    ) ||
    !headersValid(response) ||
    !(await validDocumentData(schema, response.data))
  )
    throw invalid();
  for (const [k, v] of Object.entries(options.bindings ?? {}))
    if (response.data.data[k] !== v) throw invalid();
  return { data: response.data.data as T, httpStatus: response.status };
}
async function operation<T extends DocumentOperationRecord>(
  s: DocumentScope,
  response: DocumentResponse<T>,
  op?: DocumentOperation,
) {
  try {
    await checkDocumentOutcome(s, response.data, op);
  } catch {
    throw invalid();
  }
  return response;
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<DocumentCapabilities>(
      '/knowledge/document-structure-contract',
      'CapabilitiesResponse',
      { signal },
    ),
  policy: (s: DocumentScope, signal?: AbortSignal) =>
    call<DocumentPolicy>(
      domainPath(s) + '/document-structure-policy',
      'DocumentPolicyResponse',
      { signal, bindings: { domain_id: s.domain } },
    ),
  async changePolicy(
    s: DocumentScope,
    body: DocumentPolicyChangeRequest,
    signal?: AbortSignal,
  ) {
    return operation(
      s,
      await call<DocumentPolicyReceipt>(
        domainPath(s) + '/document-structure-policy/changes',
        'PolicyReceiptResponse',
        { body, requestSchema: 'PolicyChangeRequest', signal },
      ),
      { kind: 'policy', scope: s, body },
    );
  },
  async createTask(
    s: DocumentSourceScope,
    body: DocumentTaskRequest,
    signal?: AbortSignal,
  ) {
    id(s.source);
    if (body.target.source_id !== s.source) throw invalid(400);
    return operation(
      s,
      await call<DocumentTask>(
        domainPath(s) + '/document-tasks',
        'TaskResponse',
        { body, requestSchema: 'TaskRequest', signal },
      ),
      { kind: 'task', scope: s, body },
    );
  },
  async byKey(
    s: DocumentScope,
    params: DocumentOperationQuery,
    signal?: AbortSignal,
  ) {
    const result = await operation(
      s,
      await call<DocumentOperationRecord>(
        domainPath(s) + '/document-operations/by-key',
        'OperationRecordResponse',
        { params, querySchema: 'OperationQuery', signal },
      ),
    );
    const v = result.data,
      kind =
        v.record_kind === 'task'
          ? 'task'
          : v.record_kind === 'policy_receipt'
            ? 'policy'
            : v.record_kind === 'cancel_receipt'
              ? 'cancel'
              : v.operation;
    if (
      kind !== params.kind ||
      v.original_request.idempotency_key !== params.idempotency_key
    )
      throw invalid();
    return result;
  },
  async task(s: DocumentScope, task: string, signal?: AbortSignal) {
    return operation(
      s,
      await call<DocumentTask>(
        domainPath(s) + '/document-tasks/' + id(task),
        'TaskResponse',
        { signal, bindings: { task_id: task } },
      ),
    );
  },
  async cancel(
    s: DocumentScope & { task: string },
    body: DocumentCancelRequest,
    signal?: AbortSignal,
  ) {
    return operation(
      s,
      await call<DocumentCancelReceipt>(
        domainPath(s) + '/document-tasks/' + id(s.task) + '/cancel',
        'CancelReceiptResponse',
        { body, requestSchema: 'CancelRequest', signal },
      ),
      { kind: 'cancel', scope: s, body },
    );
  },
  head: (s: DocumentSourceScope, signal?: AbortSignal) =>
    call<DocumentHead>(
      domainPath(s) + '/sources/' + id(s.source) + '/document-head',
      'DocumentHeadResponse',
      { signal, bindings: { domain_id: s.domain, source_id: s.source } },
    ),
  async representation(s: DocumentViewScope, signal?: AbortSignal) {
    const result = await call<DocumentRepresentation>(
      representationPath(s),
      'RepresentationResponse',
      {
        signal,
        bindings: {
          owner_id: s.owner,
          domain_id: s.domain,
          representation_id: s.representation,
        },
      },
    );
    const ref = result.data.original;
    if (
      ref.domain_id !== s.domain ||
      ref.source_id !== s.source ||
      ref.source_revision_id !== s.revision ||
      ref.sha256 !== s.sha
    )
      throw invalid();
    return result;
  },
  async nodes(
    s: DocumentViewScope,
    rep: DocumentRepresentation,
    parent: DocumentNode,
    params: DocumentNodeQuery,
    signal?: AbortSignal,
  ) {
    await checkDocumentNode(rep, parent);
    if (
      params.parent_id !== parent.document_node_id ||
      rep.representation_id !== s.representation
    )
      throw invalid(400);
    const result = await call<DocumentNodePage>(
      representationPath(s) + '/nodes',
      'NodePageResponse',
      {
        params,
        querySchema: 'NodeQuery',
        signal,
        bindings: {
          representation_id: s.representation,
          source_id: s.source,
          source_revision_id: s.revision,
          parent_id: params.parent_id,
        },
      },
    );
    if (result.data.items.length > (params.limit ?? 20)) throw invalid();
    for (const node of result.data.items) {
      await checkDocumentNode(rep, node);
      if (
        !sameDocumentValue(node.payload.parent_ref, {
          kind: parent.payload.kind,
          locator: parent.payload.locator,
        }) ||
        node.payload.reading_range.start_byte <
          parent.payload.reading_range.start_byte ||
        node.payload.reading_range.end_byte >
          parent.payload.reading_range.end_byte
      )
        throw invalid();
    }
    return result;
  },
  async node(
    s: DocumentViewScope,
    rep: DocumentRepresentation,
    node: string,
    signal?: AbortSignal,
  ) {
    const result = await call<DocumentNode>(
      representationPath(s) + '/nodes/' + id(node, true),
      'DocumentNodeResponse',
      { signal, bindings: { document_node_id: node } },
    );
    await checkDocumentNode(rep, result.data);
    return result;
  },
  async content(
    s: DocumentViewScope,
    rep: DocumentRepresentation,
    node: DocumentNode,
    params: DocumentContentQuery = {},
    signal?: AbortSignal,
  ) {
    await checkDocumentNode(rep, node);
    const result = await call<DocumentContentWindow>(
      representationPath(s) +
        '/nodes/' +
        id(node.document_node_id, true) +
        '/content',
      'ContentWindowResponse',
      { params, querySchema: 'ContentQuery', signal },
    );
    await checkDocumentContent(rep, node, result.data, params);
    return result;
  },
  originals: (s: DocumentScope, cursor?: string, signal?: AbortSignal) => {
    scope(s);
    return originals.list(s.domain, undefined, cursor, signal);
  },
  original: (s: DocumentScope, revision: string, signal?: AbortSignal) => {
    scope(s);
    return originals.revision(s.domain, id(revision), signal);
  },
  async fixedOriginal(
    s: DocumentViewScope,
    rep: DocumentRepresentation,
    signal?: AbortSignal,
  ) {
    const result = await service.original(s, s.revision, signal);
    if (
      !sameDocumentOriginal(rep.original, result.data) ||
      result.data.source_id !== s.source ||
      result.data.sha256 !== s.sha
    )
      throw invalid();
    return result;
  },
  async download(
    s: DocumentViewScope,
    rep: DocumentRepresentation,
    signal?: AbortSignal,
  ) {
    const original = (await service.fixedOriginal(s, rep, signal)).data;
    if (
      !sameDocumentOriginal(rep.original, original) ||
      original.sha256 !== s.sha ||
      original.source_id !== s.source
    )
      throw invalid();
    return { original, blob: await originals.download(original, signal) };
  },
  async prepare(
    s: DocumentSourceScope,
    selected: OriginalRevision,
    signal?: AbortSignal,
  ): Promise<DocumentPrepared> {
    scope(s);
    id(s.source);
    if (
      selected.domain_id !== s.domain ||
      selected.source_id !== s.source ||
      selected.byte_length > 8388608 ||
      !selected.is_current
    )
      throw invalid(400);
    const [domain, fixed, current, policy, caps, head] = await Promise.all([
      foundation.domain(s.domain, signal),
      service.original(s, selected.source_revision_id, signal),
      lifecycle.detail(s, signal),
      service.policy(s, signal),
      service.contract(signal),
      service.head(s, signal),
    ]);
    if (
      domain.owner_id !== s.owner ||
      domain.domain_id !== s.domain ||
      domain.policy.external_models ||
      domain.policy.automatic_ingestion ||
      domain.policy.allowed_model_uses.length ||
      !sameDocumentValue(domain.policy.index_modes, ['catalogue'])
    )
      throw new DocumentRequestError(401, 'unauthorized');
    const now = current.data.current,
      original = fixed.data;
    if (
      current.data.owner_id !== s.owner ||
      current.data.source_id !== s.source ||
      current.data.domain_id !== s.domain ||
      current.data.representation_kind !== 'original_only' ||
      now.state !== 'active' ||
      !original.is_current ||
      !sameDocumentValue(original, selected) ||
      now.source_revision_id !== original.source_revision_id ||
      now.sha256 !== original.sha256 ||
      now.byte_length !== original.byte_length ||
      now.relative_path !== original.relative_path ||
      !sameDocumentValue(now.classification, original.classification)
    )
      throw new DocumentRequestError(409, 'source_revision_changed');
    if (!policy.data.enabled)
      throw new DocumentRequestError(409, 'document_policy_disabled');
    if (
      policy.data.foundation_policy_version !== domain.policy.version ||
      head.data.foundation_policy_version !== domain.policy.version ||
      head.data.document_policy_version !==
        policy.data.document_policy_version ||
      head.data.current_source_revision_id !== original.source_revision_id ||
      head.data.current_lifecycle_version !== now.lifecycle_version
    )
      throw new DocumentRequestError(409, 'document_policy_changed');
    const profile = caps.data.profiles.find(
      (p) => p.media_type === original.media_type,
    );
    if (!caps.data.runtime_ready || !profile?.runtime_ready)
      throw new DocumentRequestError(503, 'unavailable');
    return {
      request: {
        target: {
          kind: 'original_document',
          source_id: s.source,
          source_revision_id: original.source_revision_id,
          original_sha256: original.sha256,
          expected_lifecycle_version: now.lifecycle_version,
        },
        expected_policy_version: domain.policy.version,
        expected_document_policy_version: policy.data.document_policy_version,
        expected_active_representation_id: head.data.active_representation_id,
        profile_id: profile.profile_id,
        expected_producer_sha256: profile.producer_sha256,
      },
      original,
      snapshot: {
        lifecycle_version: now.lifecycle_version,
        relative_path: now.relative_path,
        classification: now.classification,
      },
      profile,
    };
  },
};
export default service;

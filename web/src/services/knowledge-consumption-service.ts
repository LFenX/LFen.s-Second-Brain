import type { AxiosResponse } from 'axios';
import type {
  ContentAddress,
  ContentEnvelope,
  ContentError,
  ContentOperation,
  ContentResults,
  ContentTarget,
  ContentNode,
  ContentParsedTask,
} from '@/interfaces/database/knowledge-consumption';
import { KnowledgeContentVersion } from '@/interfaces/database/knowledge-consumption';
import type {
  ContentNodeQuery,
  ContentParsedRepresentationQuery,
  ContentReadRequest,
  ContentRequests,
  ContentScope,
  ContentPolicyRequest,
  ContentParsedUploadRequest,
  ContentParsedSnapshotRequest,
  ContentParsedTaskRequest,
  ContentParsedCancelRequest,
} from '@/interfaces/request/knowledge-consumption';
import { registerNextServer } from '@/utils/register-server';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  checkContentNode,
  checkContentRepresentation,
  checkContentWindow,
  contentBoundNode,
  contentBytes,
  contentCanonical,
  contentFixedTarget,
  contentSameTarget,
  contentSchemaValid,
  ContentLimits,
  contentNodeKey,
  contentParsedTargets,
  parseContentJSON,
  checkContentTask,
  contentParsedFileFormat,
} from '@/utils/knowledge-consumption';

const Server = registerNextServer({
  get: { url: (config: { url: string }) => config.url, method: 'get' },
  post: { url: (config: { url: string }) => config.url, method: 'post' },
});
const Definitions = {
  capabilities: ['get', 'CapabilitiesResponse', '', 'capabilities'],
  representation: [
    'post',
    'RepresentationResponse',
    'HTTPRepresentationRequest',
    'representation',
  ],
  'parsed-representations': [
    'post',
    'ParsedRepresentationsResponse',
    'HTTPParsedRepresentationsRequest',
    'parsed-representations',
  ],
  node: ['post', 'NodeResponse', 'HTTPNodeRequest', 'node'],
  nodes: ['post', 'NodesResponse', 'HTTPNodesRequest', 'nodes'],
  read: ['post', 'ReadResponse', 'HTTPReadRequest', 'read'],
  'content-policy': ['get', 'ContentPolicyResponse', '', 'policy'],
  'content-policy-change': [
    'post',
    'ContentPolicyChangeResponse',
    'HTTPContentPolicyRequest',
    'policy',
  ],
  'parsed-source-upload': [
    'post',
    'ParsedUploadResponse',
    'HTTPParsedUploadRequest',
    'parsed-source/upload',
  ],
  'parsed-source-snapshot': [
    'post',
    'ParsedSnapshotResponse',
    'HTTPParsedSnapshotRequest',
    'parsed-source/snapshot',
  ],
  'parsed-source': [
    'post',
    'ParsedSourceResponse',
    'HTTPParsedSourceRequest',
    'parsed-source',
  ],
  'parsed-task-create': [
    'post',
    'ParsedTaskCreateResponse',
    'HTTPParsedTaskCreateRequest',
    'parsed-task',
  ],
  'parsed-task-get': [
    'post',
    'ParsedTaskGetResponse',
    'HTTPParsedTaskGetRequest',
    'parsed-task/get',
  ],
  'parsed-task-cancel': [
    'post',
    'ParsedTaskCancelResponse',
    'HTTPParsedTaskCancelRequest',
    'parsed-task/cancel',
  ],
} as const;
const Status = {
  invalid_request: 400,
  unauthenticated: 401,
  not_found: 404,
  content_integrity: 409,
  contract_mismatch: 412,
  capacity_exceeded: 413,
  unsupported: 415,
  unavailable: 503,
  foundation_policy_changed: 409,
  content_policy_changed: 409,
  content_policy_disabled: 409,
  source_changed: 409,
  source_archived: 409,
  lifecycle_changed: 409,
  producer_changed: 409,
  idempotency_conflict: 409,
  task_changed: 409,
  policy_changed: 409,
  unsupported_source: 415,
  original_integrity: 409,
  original_unavailable: 503,
} as const;
export class ContentRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId = '',
    public outcomeKnown: boolean | null = null,
    public rejectionId: string | null = null,
  ) {
    super(status, code, requestId);
    this.name = 'ContentRequestError';
  }
}
const invalid = (status = 502) =>
  new ContentRequestError(
    status,
    status === 400 ? 'invalid_request' : 'content_invalid_response',
    '',
    status === 400 ? true : null,
  );
function scopeValid(scope: ContentScope, target?: ContentTarget) {
  if (
    !/^[a-f0-9]{32}$/.test(scope.owner) ||
    !/^[a-f0-9]{32}$/.test(scope.domain) ||
    (target && !contentFixedTarget(scope.domain, target))
  )
    throw invalid(400);
}
async function call<O extends ContentOperation>(
  operation: O,
  scope: ContentScope,
  body: ContentRequests[O],
  signal?: AbortSignal,
  file?: File,
): Promise<ContentResults[O]> {
  scopeValid(scope);
  const [method, responseSchema, requestSchema, route] = Definitions[operation];
  if (
    requestSchema &&
    (!contentSchemaValid(requestSchema, body) ||
      contentBytes(JSON.stringify(body)) > ContentLimits.request)
  )
    throw invalid(400);
  let form: FormData | undefined;
  if (operation === 'parsed-source-upload') {
    if (
      !file ||
      !contentParsedFileFormat(file.name) ||
      file.size < 0 ||
      file.size > ContentLimits.original
    )
      throw invalid(400);
    form = new FormData();
    form.append('request', JSON.stringify(body));
    form.append('file', file, file.name);
  } else if (file) throw invalid(400);
  let response: AxiosResponse<unknown>;
  try {
    response = await Server[method](
      {
        url: '/api/v1/datasets/' + scope.domain + '/knowledge/content/' + route,
        ...(body === undefined
          ? {}
          : {
              data: form ?? JSON.stringify(body),
              ...(form
                ? {}
                : { headers: { 'Content-Type': 'application/json' } }),
            }),
        signal,
        responseType: 'arraybuffer',
        timeout: 25000,
        skipGlobalErrorNotification: true,
        transformResponse: [
          (raw: unknown) => {
            try {
              if (!(raw instanceof ArrayBuffer) && !ArrayBuffer.isView(raw))
                throw invalid();
              if (raw.byteLength > ContentLimits.response) throw invalid();
              return parseContentJSON(
                new TextDecoder('utf-8', { fatal: true }).decode(raw),
              );
            } catch {
              throw invalid();
            }
          },
        ],
      },
      true,
    );
  } catch (error) {
    if (error instanceof KnowledgeRequestError) throw error;
    const failure = error as {
      response?: AxiosResponse<unknown>;
      code?: string;
    };
    if (failure.code === 'ERR_CANCELED') throw error;
    if (!failure.response) throw new ContentRequestError(0, 'unavailable');
    response = failure.response;
  }
  if (
    String(response.headers['x-knowledge-content-version']) !==
      KnowledgeContentVersion ||
    String(response.headers['content-type'])
      .split(';')[0]
      .trim()
      .toLowerCase() !== 'application/json' ||
    !contentSchemaValid(responseSchema, response.data)
  )
    throw invalid();
  const envelope = response.data as ContentEnvelope<O> | ContentError;
  if (
    String(response.headers['x-request-id']) !== envelope.request_id ||
    envelope.operation !== operation
  )
    throw invalid();
  if ('code' in envelope) {
    if (envelope.data !== null) throw invalid();
    const expectedStatus =
      envelope.code === 'capacity_exceeded' &&
      envelope.outcome_known !== undefined &&
      operation === 'parsed-task-create'
        ? 429
        : Status[envelope.code];
    if (expectedStatus !== response.status) throw invalid();
    throw new ContentRequestError(
      response.status,
      envelope.code,
      envelope.request_id,
      envelope.outcome_known ?? null,
      envelope.rejection_id ?? null,
    );
  }
  if (
    response.status !== 200 &&
    !(operation === 'parsed-task-create' && response.status === 201)
  )
    throw invalid();
  return envelope.data;
}
const service = {
  capabilities: (scope: ContentScope, signal?: AbortSignal) =>
    call('capabilities', scope, undefined, signal),
  async policy(scope: ContentScope, signal?: AbortSignal) {
    const data = await call('content-policy', scope, undefined, signal);
    if (data.domain_id !== scope.domain) throw invalid();
    return data;
  },
  async changePolicy(scope: ContentScope, request: ContentPolicyRequest) {
    const data = await call('content-policy-change', scope, {
      contract_version: KnowledgeContentVersion,
      request,
    });
    if (
      data.domain_id !== scope.domain ||
      data.foundation_policy_version !== request.expected_policy_version ||
      data.parse_enabled !== request.parse_enabled ||
      data.lexical_enabled !== request.lexical_enabled ||
      ![
        request.expected_content_policy_version,
        request.expected_content_policy_version + 1,
      ].includes(data.content_policy_version)
    )
      throw invalid();
    return data;
  },
  async uploadParsedSource(
    scope: ContentScope,
    request: ContentParsedUploadRequest,
    file: File,
  ) {
    const data = await call(
      'parsed-source-upload',
      scope,
      { contract_version: KnowledgeContentVersion, request },
      undefined,
      file,
    );
    if (data.original_filename !== file.name || data.byte_length !== file.size)
      throw invalid();
    return data;
  },
  async snapshotParsedSource(
    scope: ContentScope,
    request: ContentParsedSnapshotRequest,
  ) {
    const data = await call('parsed-source-snapshot', scope, {
      contract_version: KnowledgeContentVersion,
      request,
    });
    if (
      data.source_ref.domain_id !== scope.domain ||
      data.source_ref.source_id !== request.source_id
    )
      throw invalid();
    return data;
  },
  async parsedSource(
    scope: ContentScope,
    sourceId: string,
    signal?: AbortSignal,
  ) {
    const data = await call(
      'parsed-source',
      scope,
      { contract_version: KnowledgeContentVersion, source_id: sourceId },
      signal,
    );
    if (
      data.source_ref.domain_id !== scope.domain ||
      data.source_ref.source_id !== sourceId
    )
      throw invalid();
    return data;
  },
  async createParsedTask(
    scope: ContentScope,
    request: ContentParsedTaskRequest,
  ) {
    if (request.source_ref.domain_id !== scope.domain) throw invalid(400);
    const data = await call('parsed-task-create', scope, {
      contract_version: KnowledgeContentVersion,
      request,
    });
    try {
      await checkContentTask(scope, data, { request });
    } catch {
      throw invalid();
    }
    return data;
  },
  async parsedTask(
    scope: ContentScope,
    taskId: string,
    sourceId: string,
    signal?: AbortSignal,
  ) {
    const data = await call(
      'parsed-task-get',
      scope,
      { contract_version: KnowledgeContentVersion, task_id: taskId },
      signal,
    );
    try {
      await checkContentTask(scope, data, { taskId, sourceId });
    } catch {
      throw invalid();
    }
    return data;
  },
  async cancelParsedTask(
    scope: ContentScope,
    task: ContentParsedTask,
    request: ContentParsedCancelRequest,
  ) {
    if (
      task.owner_id !== scope.owner ||
      task.domain_id !== scope.domain ||
      request.expected_task_revision !== task.revision
    )
      throw invalid(400);
    const data = await call('parsed-task-cancel', scope, {
      contract_version: KnowledgeContentVersion,
      task_id: task.task_id,
      request,
    });
    try {
      await checkContentTask(scope, data, {
        taskId: task.task_id,
        sourceId: task.original_request.source_ref.source_id,
        previous: task,
      });
    } catch {
      throw invalid();
    }
    return data;
  },
  async parsedRepresentations(
    scope: ContentScope,
    query: ContentParsedRepresentationQuery,
    signal?: AbortSignal,
  ) {
    if (
      query.source_ref.domain_id !== scope.domain ||
      query.limit < 1 ||
      query.limit > 20
    )
      throw invalid(400);
    const data = await call(
      'parsed-representations',
      scope,
      { contract_version: KnowledgeContentVersion, query },
      signal,
    );
    try {
      if (
        data.items.length > query.limit ||
        (data.next_cursor !== null && data.next_cursor === query.cursor)
      )
        throw invalid();
      contentParsedTargets(query.source_ref, [data]);
    } catch {
      throw invalid();
    }
    return data;
  },
  async representation(
    scope: ContentScope,
    target: ContentTarget,
    signal?: AbortSignal,
  ) {
    scopeValid(scope, target);
    const data = await call(
      'representation',
      scope,
      { contract_version: KnowledgeContentVersion, target },
      signal,
    );
    try {
      checkContentRepresentation(target, data);
    } catch {
      throw invalid();
    }
    return data;
  },
  async node(
    scope: ContentScope,
    address: ContentAddress,
    signal?: AbortSignal,
  ) {
    scopeValid(scope, address.target);
    if (!contentBoundNode(address.target, address.node_ref)) throw invalid(400);
    const data = await call(
      'node',
      scope,
      { contract_version: KnowledgeContentVersion, address },
      signal,
    );
    try {
      if (
        !contentSameTarget(address.target, data.target) ||
        contentNodeKey(address.node_ref) !== contentNodeKey(data.node.ref)
      )
        throw invalid();
      await checkContentNode(data.target, data.node);
    } catch {
      throw invalid();
    }
    return data;
  },
  async nodes(
    scope: ContentScope,
    query: ContentNodeQuery,
    signal?: AbortSignal,
  ) {
    scopeValid(scope, query.target);
    if (
      query.parent_ref !== null &&
      !contentBoundNode(query.target, query.parent_ref)
    )
      throw invalid(400);
    const data = await call(
      'nodes',
      scope,
      { contract_version: KnowledgeContentVersion, query },
      signal,
    );
    try {
      if (
        !contentSameTarget(query.target, data.target) ||
        contentCanonical(data.parent_ref) !==
          contentCanonical(query.parent_ref) ||
        data.items.length > query.limit ||
        (data.next_cursor !== null && data.next_cursor === query.cursor)
      )
        throw invalid();
      const seen = new Set<string>();
      for (const node of data.items) {
        const key = contentNodeKey(node.ref);
        if (seen.has(key)) throw invalid();
        seen.add(key);
        await checkContentNode(data.target, node);
        if (
          query.parent_ref &&
          contentCanonical(node.parent_ref) !==
            contentCanonical(query.parent_ref)
        )
          throw invalid();
      }
    } catch {
      throw invalid();
    }
    return data;
  },
  async read(
    scope: ContentScope,
    read: ContentReadRequest,
    node: ContentNode,
    signal?: AbortSignal,
  ) {
    scopeValid(scope, read.address.target);
    if (
      !node.text.range ||
      !contentBoundNode(read.address.target, read.address.node_ref) ||
      (read.end_byte !== null &&
        (read.start_byte === null ||
          read.end_byte < read.start_byte ||
          read.end_byte - read.start_byte > ContentLimits.read))
    )
      throw invalid(400);
    const data = await call(
      'read',
      scope,
      { contract_version: KnowledgeContentVersion, read },
      signal,
    );
    try {
      return await checkContentWindow(
        read.address,
        node,
        read.start_byte ?? node.text.range.start_byte,
        read.end_byte,
        data,
      );
    } catch {
      throw invalid();
    }
  },
};
export default service;

import type { AxiosResponse } from 'axios';
import type {
  StructureCapabilities,
  StructurePolicy,
  StructurePolicyReceipt,
  StructureTask,
  StructureTaskOutcome,
  StructureCommandReceipt,
  StructureCancelReceipt,
  StructureTaskPage,
  StructureView,
  StructureHead,
  StructureNode,
  StructureNodePage,
  StructureContent,
  StructureRelationPage,
  StructureAlias,
  StructureMember,
} from '@/interfaces/database/knowledge-structure';
import type {
  StructureTaskRequest,
  StructurePolicyChangeRequest,
  StructureCancelRequest,
  StructureTaskQuery,
  StructureNodeQuery,
  StructureRelationQuery,
  StructureContentQuery,
} from '@/interfaces/request/knowledge-structure';
import type { BatchManifestEntry } from '@/interfaces/database/knowledge-batch';
import { registerNextServer } from '@/utils/register-server';
import {
  StructureId,
  StructureHash,
  StructureKey,
  structureJsonBytes,
  validStructureData,
  validStructureQuery,
  validStructureAliases,
  structureParser,
} from '@/utils/knowledge-structure-schema';
import {
  checkStructureOutcome,
  checkStructureView,
  checkStructureNode,
  checkStructureContent,
  structureDigest,
  structureRelationProjection,
  type StructureScope,
  type StructureProjectScope,
  type StructureViewScope,
  type StructureOperation,
} from '@/utils/knowledge-structure';
import foundation, {
  KnowledgeRequestError,
} from './knowledge-foundation-service';
import batch from './knowledge-batch-service';
import lifecycle from './knowledge-lifecycle-service';
import { sameDirectoryClassification } from '@/utils/knowledge-directory';

export class StructureRequestError extends KnowledgeRequestError {
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
export type StructureResponse<T> = { data: T; httpStatus: number };
export type StructurePrepared = {
  request: Omit<StructureTaskRequest, 'confirmed' | 'idempotency_key'>;
  entries: BatchManifestEntry[];
  policy: StructurePolicy;
  head: StructureHead;
};
const Server = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  write: { url: (config: { url: string }) => config.url, method: 'post' },
});
const Headers = {
  'X-Knowledge-Contract-Version': '1.0.0',
  'X-Knowledge-Structure-Contract-Version': '0.1.0',
};
const Invalid = (status = 502) =>
  new StructureRequestError(
    status,
    status === 400 ? 'invalid_request' : 'structure_invalid_response',
  );
function id(value: string, hash = false): string {
  if (!(hash ? StructureHash : StructureId).test(value)) throw Invalid(400);
  return value;
}
function checkScope(scope: StructureScope) {
  id(scope.owner);
  id(scope.domain);
}
const domainPath = (domain: string) => '/datasets/' + id(domain) + '/knowledge';
const taskPath = (scope: StructureScope, task: string) =>
  domainPath(scope.domain) + '/structure-tasks/' + id(task);
const viewPath = (scope: StructureViewScope) =>
  domainPath(scope.domain) + '/structure-views/' + id(scope.view, true);
function header(response: AxiosResponse, name: string): string {
  return String(
    Object.entries(response.headers ?? {}).find(
      ([key]) => key.toLowerCase() === name.toLowerCase(),
    )?.[1] ?? '',
  );
}
function validHeaders(response: AxiosResponse): boolean {
  return (
    Object.entries(Headers).every(
      ([key, value]) => header(response, key) === value,
    ) &&
    header(response, 'Cache-Control') === 'no-store' &&
    StructureId.test(header(response, 'X-Request-ID')) &&
    header(response, 'X-Request-ID') === response.data?.request_id
  );
}
async function failed(error: unknown): Promise<never> {
  const issue = error as { code?: string; response?: AxiosResponse };
  if (issue.code === 'ERR_CANCELED') throw error;
  const response = issue.response;
  if (!response) throw new StructureRequestError(0, 'structure_result_unknown');
  if (response.status === 401)
    throw new StructureRequestError(401, 'knowledge_unauthorized');
  if (
    !validHeaders(response) ||
    structureJsonBytes(response.data) > 262144 ||
    !validStructureData('Error', response.data)
  )
    throw Invalid();
  const codes: Record<string, number> = {
    invalid_request: 400,
    not_found: 404,
    contract_mismatch: 412,
    capacity_exceeded: 429,
    source_unavailable: 503,
    lifecycle_integrity: 409,
    structure_integrity: 409,
    unavailable: 503,
  };
  if ((codes[response.data.code] ?? 409) !== response.status) throw Invalid();
  throw new StructureRequestError(
    response.status,
    response.data.code,
    response.data.request_id,
    response.data.outcome_known,
    response.data.rejection_id,
  );
}
async function call<T>(
  path: string,
  schema: string,
  options: {
    params?: object;
    body?: unknown;
    requestSchema?: string;
    signal?: AbortSignal;
    scope?: Record<string, unknown>;
  } = {},
): Promise<StructureResponse<T>> {
  if (
    options.requestSchema &&
    !validStructureData(options.requestSchema, options.body)
  )
    throw Invalid(400);
  if (options.body !== undefined && structureJsonBytes(options.body) > 32768)
    throw new StructureRequestError(413, 'structure_budget_exceeded');
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
    return failed(error);
  }
  if (
    response.data?.contract_version !== '1.0.0' ||
    response.data?.structure_contract_version !== '0.1.0'
  )
    throw new StructureRequestError(412, 'contract_mismatch');
  if (
    !(options.body === undefined ? [200] : [200, 201]).includes(
      response.status,
    ) ||
    !validHeaders(response) ||
    !validStructureData(schema, response.data) ||
    structureJsonBytes(response.data) >
      (schema === 'ContentResponse' ? 524288 : 262144)
  )
    throw Invalid();
  for (const [key, value] of Object.entries(options.scope ?? {}))
    if (response.data.data[key] !== value) throw Invalid();
  return { data: response.data.data as T, httpStatus: response.status };
}
async function outcome<
  T extends
    | StructureTask
    | StructureTaskOutcome
    | StructurePolicyReceipt
    | StructureCommandReceipt
    | StructureCancelReceipt,
>(
  result: StructureResponse<T>,
  scope: StructureScope & { task?: string; project?: string },
  operation?: StructureOperation,
): Promise<StructureResponse<T>> {
  try {
    await checkStructureOutcome(scope, result.data, operation);
  } catch {
    throw Invalid();
  }
  return result;
}
function query(
  kind: 'task' | 'node' | 'relation' | 'content',
  value: Record<string, unknown>,
) {
  if (!validStructureQuery(kind, value)) throw Invalid(400);
  return value;
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<StructureCapabilities>(
      '/knowledge/structure-contract',
      'CapabilitiesResponse',
      { signal },
    ),
  policy(scope: StructureScope, signal?: AbortSignal) {
    checkScope(scope);
    return call<StructurePolicy>(
      domainPath(scope.domain) + '/structure-policy',
      'PolicyResponse',
      { signal, scope: { domain_id: scope.domain } },
    );
  },
  async changePolicy(
    scope: StructureScope,
    body: StructurePolicyChangeRequest,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    return outcome(
      await call<StructurePolicyReceipt>(
        domainPath(scope.domain) + '/structure-policy/changes',
        'PolicyReceiptResponse',
        { body, requestSchema: 'PolicyChangeRequest', signal },
      ),
      scope,
      { kind: 'policy', scope, body },
    );
  },
  async commandByKey(
    scope: StructureScope,
    kind: 'policy' | 'cancel',
    key: string,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    if (!['policy', 'cancel'].includes(kind) || !StructureKey.test(key))
      throw Invalid(400);
    const result = await outcome(
      await call<StructureCommandReceipt>(
        domainPath(scope.domain) + '/structure-commands/by-key',
        'CommandReceiptResponse',
        { params: { kind, idempotency_key: key }, signal },
      ),
      scope,
    );
    if (
      result.data.original_request.idempotency_key !== key ||
      ('operation' in result.data
        ? result.data.operation
        : result.data.kind) !== kind
    )
      throw Invalid();
    return result;
  },
  async createTask(
    scope: StructureProjectScope,
    body: StructureTaskRequest,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    id(scope.project);
    if (body.project_id !== scope.project) throw Invalid(400);
    return outcome(
      await call<StructureTask>(
        domainPath(scope.domain) + '/structure-tasks',
        'TaskResponse',
        { body, requestSchema: 'TaskRequest', signal },
      ),
      scope,
      { kind: 'task', scope, body },
    );
  },
  async tasks(
    scope: StructureProjectScope,
    params: StructureTaskQuery,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    if (params.project_id !== scope.project) throw Invalid(400);
    const result = await call<StructureTaskPage>(
      domainPath(scope.domain) + '/structure-tasks',
      'TaskPageResponse',
      {
        params: query('task', params),
        signal,
        scope: { domain_id: scope.domain, project_id: scope.project },
      },
    );
    for (const task of result.data.items)
      await outcome({ data: task, httpStatus: 200 }, scope);
    if (result.data.items.length > (params.limit ?? 20)) throw Invalid();
    return result;
  },
  async taskByKey(scope: StructureScope, key: string, signal?: AbortSignal) {
    checkScope(scope);
    if (!StructureKey.test(key)) throw Invalid(400);
    const result = await outcome(
      await call<StructureTaskOutcome>(
        domainPath(scope.domain) + '/structure-tasks/by-key',
        'TaskOutcomeResponse',
        { params: { idempotency_key: key }, signal },
      ),
      scope,
    );
    if (result.data.original_request.idempotency_key !== key) throw Invalid();
    return result;
  },
  async task(scope: StructureScope, task: string, signal?: AbortSignal) {
    checkScope(scope);
    return outcome(
      await call<StructureTask>(taskPath(scope, task), 'TaskResponse', {
        signal,
      }),
      { ...scope, task },
    );
  },
  async cancel(
    scope: StructureScope & { task: string },
    body: StructureCancelRequest,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    return outcome(
      await call<StructureCancelReceipt>(
        taskPath(scope, scope.task) + '/cancel',
        'CancelReceiptResponse',
        { body, requestSchema: 'CancelRequest', signal },
      ),
      scope,
      { kind: 'cancel', scope, body },
    );
  },
  head(scope: StructureProjectScope, signal?: AbortSignal) {
    checkScope(scope);
    return call<StructureHead>(
      domainPath(scope.domain) +
        '/projects/' +
        id(scope.project) +
        '/structure-head',
      'HeadResponse',
      { signal, scope: { domain_id: scope.domain, project_id: scope.project } },
    );
  },
  async view(scope: StructureViewScope, signal?: AbortSignal) {
    checkScope(scope);
    const result = await call<StructureView>(viewPath(scope), 'ViewResponse', {
      signal,
      scope: {
        view_id: scope.view,
        domain_id: scope.domain,
        owner_id: scope.owner,
      },
    });
    try {
      await checkStructureView(scope, result.data);
    } catch {
      throw Invalid();
    }
    return result;
  },
  async nodes(
    scope: StructureViewScope,
    params: StructureNodeQuery,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    const result = await call<StructureNodePage>(
      viewPath(scope) + '/nodes',
      'NodePageResponse',
      {
        params: query('node', params),
        signal,
        scope: {
          view_id: scope.view,
          source_id: params.source_id,
          parent_id: params.parent_id ?? null,
        },
      },
    );
    for (const node of result.data.items) {
      if (
        node.source_id !== params.source_id ||
        node.parent_id !== (params.parent_id ?? null)
      )
        throw Invalid();
      try {
        await checkStructureNode(scope, node);
      } catch {
        throw Invalid();
      }
    }
    if (result.data.items.length > (params.limit ?? 20)) throw Invalid();
    return result;
  },
  async node(scope: StructureViewScope, node: string, signal?: AbortSignal) {
    checkScope(scope);
    const result = await call<StructureNode>(
      viewPath(scope) + '/nodes/' + id(node, true),
      'NodeResponse',
      { signal, scope: { view_id: scope.view, structure_node_id: node } },
    );
    try {
      await checkStructureNode(scope, result.data);
    } catch {
      throw Invalid();
    }
    return result;
  },
  async content(
    scope: StructureViewScope,
    node: StructureNode,
    sourceSha: string,
    params: StructureContentQuery = {},
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    if (node.view_id !== scope.view || !StructureHash.test(sourceSha))
      throw Invalid(400);
    const result = await call<StructureContent>(
      viewPath(scope) +
        '/nodes/' +
        id(node.structure_node_id, true) +
        '/content',
      'ContentResponse',
      { params: query('content', params), signal },
    );
    try {
      await checkStructureContent(node, sourceSha, result.data);
      if (
        result.data.start_byte !== (params.start_byte ?? node.start_byte) ||
        (params.end_byte !== undefined &&
          result.data.end_byte !== params.end_byte)
      )
        throw Error('Range mismatch');
    } catch {
      throw Invalid();
    }
    return result;
  },
  async relations(
    scope: StructureViewScope,
    params: StructureRelationQuery,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    const result = await call<StructureRelationPage>(
      viewPath(scope) + '/relations',
      'RelationPageResponse',
      {
        params: query('relation', params),
        signal,
        scope: {
          view_id: scope.view,
          node_id: params.node_id,
          direction: params.direction ?? 'both',
          kind: params.kind ?? null,
        },
      },
    );
    for (const relation of result.data.items) {
      if (
        relation.view_id !== scope.view ||
        (params.kind && relation.kind !== params.kind) ||
        (params.direction === 'in'
          ? relation.to_node_id !== params.node_id
          : params.direction === 'out'
            ? relation.from_node_id !== params.node_id
            : relation.from_node_id !== params.node_id &&
              relation.to_node_id !== params.node_id) ||
        (await structureDigest(structureRelationProjection(relation))) !==
          relation.relation_id
      )
        throw Invalid();
    }
    if (result.data.items.length > (params.limit ?? 20)) throw Invalid();
    return result;
  },
  manifestHead(scope: StructureProjectScope, signal?: AbortSignal) {
    checkScope(scope);
    id(scope.project);
    return batch.head(scope.domain, scope.project, signal);
  },
  manifest(
    scope: StructureProjectScope,
    manifest: string,
    cursor?: string,
    signal?: AbortSignal,
  ) {
    checkScope(scope);
    id(scope.project);
    id(manifest);
    return batch.manifest(
      scope.domain,
      scope.project,
      manifest,
      cursor,
      signal,
    );
  },
  async prepare(
    scope: StructureProjectScope,
    manifest: string,
    entries: BatchManifestEntry[],
    aliases: StructureAlias[],
    signal?: AbortSignal,
  ): Promise<StructurePrepared> {
    checkScope(scope);
    id(scope.project);
    id(manifest);
    if (
      entries.length < 1 ||
      entries.length > 32 ||
      new Set(entries.map((item) => item.source_id)).size !== entries.length ||
      entries.some(
        (item) =>
          item.byte_length > 2097152 || !structureParser(item.relative_path),
      ) ||
      entries.reduce((sum, item) => sum + item.byte_length, 0) > 16777216 ||
      !validStructureAliases(aliases)
    )
      throw Invalid(400);
    const [domain, currentManifest, policy, head] = await Promise.all([
      foundation.domain(scope.domain, signal),
      batch.head(scope.domain, scope.project, signal),
      service.policy(scope, signal),
      service.head(scope, signal),
    ]);
    if (domain.owner_id !== scope.owner || domain.domain_id !== scope.domain)
      throw new StructureRequestError(401, 'knowledge_unauthorized');
    if (currentManifest.data.manifest_id !== manifest)
      throw new StructureRequestError(409, 'manifest_changed');
    if (!policy.data.enabled)
      throw new StructureRequestError(409, 'policy_disabled');
    if (policy.data.foundation_policy_version !== domain.policy.version)
      throw new StructureRequestError(409, 'policy_changed');
    const members: StructureMember[] = [];
    for (const item of entries) {
      const [detail, source] = await Promise.all([
        lifecycle.detail({ ...scope, source: item.source_id }, signal),
        foundation.source(scope.domain, item.source_revision_id, signal),
      ]);
      const current = detail.data.current;
      if (
        detail.data.owner_id !== scope.owner ||
        detail.data.domain_id !== scope.domain ||
        detail.data.source_id !== item.source_id ||
        detail.data.representation_kind !== 'text' ||
        current.state !== 'active' ||
        current.source_revision_id !== item.source_revision_id ||
        current.relative_path !== item.relative_path ||
        current.sha256 !== item.sha256 ||
        current.byte_length !== item.byte_length ||
        !sameDirectoryClassification(
          current.classification,
          item.classification,
        )
      )
        throw new StructureRequestError(409, 'revision_changed');
      if (
        source.domain_id !== scope.domain ||
        source.source_id !== item.source_id ||
        source.source_revision_id !== item.source_revision_id ||
        source.root_node_id !== item.root_node_id ||
        source.sha256 !== item.sha256 ||
        source.byte_length !== item.byte_length ||
        source.relative_path !== item.relative_path ||
        !sameDirectoryClassification(
          {
            group_id: source.group_id,
            component_side: source.component_side,
            language: source.language,
          },
          item.classification,
        )
      )
        throw new StructureRequestError(409, 'structure_integrity');
      members.push({
        source_id: item.source_id,
        source_revision_id: item.source_revision_id,
        lifecycle_version: current.lifecycle_version,
        parser: structureParser(item.relative_path)!,
      });
    }
    return {
      request: {
        project_id: scope.project,
        manifest_id: manifest,
        expected_active_view_id: head.data.view_id,
        expected_policy_version: policy.data.foundation_policy_version,
        expected_structure_policy_version: policy.data.structure_policy_version,
        members,
        aliases: aliases.map((item) => ({ ...item })),
      },
      entries: entries.map((item) => ({ ...item })),
      policy: policy.data,
      head: head.data,
    };
  },
};
export default service;

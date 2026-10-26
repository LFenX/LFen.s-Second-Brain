import type { AxiosResponse } from 'axios';
import type {
  LifecycleCapabilities,
  LifecycleDetail,
  LifecyclePage,
  LifecycleReceipt,
  LifecycleTargetCheck,
} from '@/interfaces/database/knowledge-lifecycle';
import type {
  LifecycleAction,
  LifecycleCommand,
  LifecycleListQuery,
  LifecycleMoveCommand,
  LifecycleReclassifyCommand,
  LifecycleStateCommand,
  LifecycleTargetQuery,
} from '@/interfaces/request/knowledge-lifecycle';
import { registerNextServer } from '@/utils/register-server';
import {
  LifecycleId,
  LifecycleKey,
  validLifecycleData,
  validLifecycleQuery,
  validLifecycleCommand,
  lifecycleCommandSchema,
  lifecycleJsonBytes,
  lifecyclePath,
} from '@/utils/knowledge-lifecycle-schema';
import {
  checkLifecycleReceipt,
  lifecycleRequestDigest,
  type LifecycleScope,
} from '@/utils/knowledge-lifecycle';
import { KnowledgeRequestError } from './knowledge-foundation-service';
import foundation from './knowledge-foundation-service';
import sources from './knowledge-sources-service';
import original from './knowledge-original-service';
import { knowledgeReceiptLink } from '@/utils/knowledge-source-link';

export class LifecycleRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId = '',
    public outcomeKnown = false,
  ) {
    super(status, code, requestId);
  }
}
export type LifecycleResponse<T> = { data: T; httpStatus: number };
const Server = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  write: { url: (config: { url: string }) => config.url, method: 'post' },
});
const Headers = {
  'X-Knowledge-Contract-Version': '1.0.0',
  'X-Knowledge-Source-Contract-Version': '0.7.0',
};
const Invalid = (status = 502) =>
  new LifecycleRequestError(
    status,
    status === 400 ? 'knowledge_invalid_request' : 'lifecycle_invalid_response',
  );
function id(value: string) {
  if (!LifecycleId.test(value)) throw Invalid(400);
  return value;
}
const domainPath = (domain: string) => '/datasets/' + id(domain) + '/knowledge';
const sourcePath = (scope: LifecycleScope) =>
  domainPath(scope.domain) + '/source-lifecycles/' + id(scope.source);
function header(response: AxiosResponse, name: string) {
  return String(response.headers?.[name.toLowerCase()] ?? '');
}
async function failed(error: unknown): Promise<never> {
  const issue = error as { code?: string; response?: AxiosResponse };
  if (issue.code === 'ERR_CANCELED') throw error;
  const response = issue.response;
  if (!response) throw new LifecycleRequestError(0, 'lifecycle_result_unknown');
  if (response.status === 401)
    throw new LifecycleRequestError(401, 'knowledge_unauthorized');
  if (
    !response.data ||
    lifecycleJsonBytes(response.data) > 32768 ||
    !validLifecycleData('Error', response.data, response.status)
  )
    throw Invalid();
  throw new LifecycleRequestError(
    response.status,
    response.data.code,
    response.data.request_id,
    response.data.outcome_known,
  );
}
async function call<T>(
  method: 'read' | 'write',
  path: string,
  schema: string,
  scope: Record<string, unknown> = {},
  options: {
    params?: object;
    body?: unknown;
    requestSchema?: string;
    signal?: AbortSignal;
    created?: boolean;
  } = {},
): Promise<LifecycleResponse<T>> {
  if (
    options.requestSchema &&
    !validLifecycleData(options.requestSchema, options.body)
  )
    throw Invalid(400);
  if (options.body !== undefined && lifecycleJsonBytes(options.body) > 32768)
    throw new LifecycleRequestError(413, 'lifecycle_budget_exceeded', '', true);
  let response: AxiosResponse;
  try {
    response = await Server[method](
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
          ...(options.body !== undefined
            ? { 'Content-Type': 'application/json' }
            : {}),
        },
      },
      true,
    );
  } catch (error) {
    return failed(error);
  }
  const envelope = response.data;
  if (
    envelope?.contract_version !== '1.0.0' ||
    envelope?.source_contract_version !== '0.7.0'
  )
    throw new LifecycleRequestError(
      409,
      'knowledge_source_contract_version',
      '',
      true,
    );
  if (
    ![200, ...(options.created ? [201] : [])].includes(response.status) ||
    !validLifecycleData(schema, envelope) ||
    lifecycleJsonBytes(envelope) > 32768 ||
    header(response, 'Cache-Control') !== 'no-store'
  )
    throw Invalid();
  for (const [key, value] of Object.entries(scope))
    if (envelope.data[key] !== value) throw Invalid();
  return { data: envelope.data, httpStatus: response.status };
}
async function receiptResult(
  result: LifecycleResponse<LifecycleReceipt>,
  scope: LifecycleScope,
  action?: LifecycleAction,
  request?: LifecycleCommand,
) {
  try {
    checkLifecycleReceipt(result.data, scope, action, request);
    if (
      (await lifecycleRequestDigest(
        scope,
        result.data.action,
        result.data.request,
      )) !== result.data.request_sha256
    )
      throw Error('Command digest mismatch');
  } catch {
    throw Invalid();
  }
  return result;
}
const ActionPaths = {
  reclassify: 'reclassifications',
  move: 'moves',
  archive: 'archives',
  restore: 'restores',
};
async function command(
  scope: LifecycleScope,
  action: LifecycleAction,
  body: LifecycleCommand,
  signal?: AbortSignal,
) {
  id(scope.owner);
  if (!validLifecycleCommand(action, body)) throw Invalid(400);
  return receiptResult(
    await call<LifecycleReceipt>(
      'write',
      sourcePath(scope) + '/' + ActionPaths[action],
      'ReceiptResponse',
      {},
      {
        body,
        requestSchema: lifecycleCommandSchema(action),
        signal,
        created: true,
      },
    ),
    scope,
    action,
    body,
  );
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<LifecycleCapabilities>(
      'read',
      '/knowledge/source-lifecycle-contract',
      'CapabilitiesResponse',
      {},
      { signal },
    ),
  async list(
    owner: string,
    domain: string,
    query: LifecycleListQuery = {},
    signal?: AbortSignal,
  ) {
    id(owner);
    if (!validLifecycleQuery(query)) throw Invalid(400);
    const result = await call<LifecyclePage>(
      'read',
      domainPath(domain) + '/source-lifecycles',
      'PageResponse',
      {},
      { params: query, signal },
    );
    const seen = new Set<string>();
    let last = '';
    for (const item of result.data.items) {
      if (
        item.owner_id !== owner ||
        item.domain_id !== domain ||
        item.current.state !== (query.state || 'active') ||
        (query.group_scope === 'ungrouped' &&
          item.current.classification.group_id !== null) ||
        (query.group_scope === 'exact' &&
          item.current.classification.group_id !== query.group_id) ||
        (query.component_side !== undefined &&
          item.current.classification.component_side !==
            query.component_side) ||
        (query.language !== undefined &&
          item.current.classification.language !== query.language) ||
        seen.has(item.source_id) ||
        (last && last >= item.source_id)
      )
        throw Invalid();
      seen.add(item.source_id);
      last = item.source_id;
    }
    if (
      result.data.items.length > (query.limit ?? 20) ||
      result.data.next_cursor === query.cursor ||
      (result.data.next_cursor && result.data.items.length === 0)
    )
      throw Invalid();
    return result;
  },
  detail: (scope: LifecycleScope, signal?: AbortSignal) =>
    call<LifecycleDetail>(
      'read',
      sourcePath(scope),
      'DetailResponse',
      {
        owner_id: id(scope.owner),
        domain_id: scope.domain,
        source_id: scope.source,
      },
      { signal },
    ),
  async target(
    scope: LifecycleScope,
    query: LifecycleTargetQuery,
    signal?: AbortSignal,
  ) {
    if (
      !lifecyclePath(query.relative_path) ||
      !validLifecycleCommand('move', {
        ...query,
        idempotency_key: 'check',
        confirmed: true,
      })
    )
      throw Invalid(400);
    const result = await call<LifecycleTargetCheck>(
      'read',
      sourcePath(scope) + '/move-target',
      'TargetCheckResponse',
      {
        domain_id: scope.domain,
        source_id: scope.source,
        relative_path: query.relative_path,
        current_source_revision_id: query.expected_source_revision_id,
        lifecycle_version: query.expected_lifecycle_version,
        policy_version: query.expected_policy_version,
      },
      { params: query, signal },
    );
    return result;
  },
  reclassify: (
    scope: LifecycleScope,
    body: LifecycleReclassifyCommand,
    signal?: AbortSignal,
  ) => command(scope, 'reclassify', body, signal),
  move: (
    scope: LifecycleScope,
    body: LifecycleMoveCommand,
    signal?: AbortSignal,
  ) => command(scope, 'move', body, signal),
  archive: (
    scope: LifecycleScope,
    body: LifecycleStateCommand,
    signal?: AbortSignal,
  ) => command(scope, 'archive', body, signal),
  restore: (
    scope: LifecycleScope,
    body: LifecycleStateCommand,
    signal?: AbortSignal,
  ) => command(scope, 'restore', body, signal),
  async byKey(
    scope: LifecycleScope,
    action: LifecycleAction,
    request: LifecycleCommand,
    signal?: AbortSignal,
  ) {
    if (
      !validLifecycleCommand(action, request) ||
      !LifecycleKey.test(request.idempotency_key)
    )
      throw Invalid(400);
    return receiptResult(
      await call<LifecycleReceipt>(
        'read',
        domainPath(scope.domain) + '/lifecycle-commands/by-key',
        'ReceiptResponse',
        {},
        {
          params: { action, idempotency_key: request.idempotency_key },
          signal,
        },
      ),
      scope,
      action,
      request,
    );
  },
  async receipt(
    scope: LifecycleScope,
    commandId: string,
    signal?: AbortSignal,
  ) {
    return receiptResult(
      await call<LifecycleReceipt>(
        'read',
        domainPath(scope.domain) + '/lifecycle-commands/' + id(commandId),
        'ReceiptResponse',
        { command_id: commandId },
        { signal },
      ),
      scope,
    );
  },
  async movePaths(detail: LifecycleDetail, signal?: AbortSignal) {
    if (!detail.registration_id || !detail.actions.move.supported)
      throw Invalid(400);
    const reg =
      detail.source_kind === 'text_file'
        ? await sources.registration(
            detail.domain_id,
            detail.registration_id,
            signal,
          )
        : detail.source_kind === 'original_document'
          ? (
              await original.registration(
                detail.domain_id,
                detail.registration_id,
                detail.owner_id,
                signal,
              )
            ).data
          : null;
    if (!reg || reg.registration_id !== detail.registration_id) throw Invalid();
    return reg.include_paths;
  },
  async fixedLink(detail: LifecycleDetail, signal?: AbortSignal) {
    const current = detail.current;
    if (detail.representation_kind === 'text') {
      const source = await foundation.source(
        detail.domain_id,
        current.source_revision_id,
        signal,
      );
      if (
        source.source_id !== detail.source_id ||
        source.sha256 !== current.sha256 ||
        source.native_file_id !== detail.native_file_id
      )
        throw Invalid();
      return knowledgeReceiptLink(detail.domain_id, source);
    }
    const params: Record<string, string> =
      detail.representation_kind === 'original_only'
        ? {
            mode: 'original',
            original_revision: current.source_revision_id,
            original_source: detail.source_id,
            original_sha: current.sha256,
          }
        : {
            mode: 'web',
            web_revision: current.source_revision_id,
            web_source: detail.source_id,
            web_input_sha: current.source_input_sha256,
          };
    return (
      '/dataset/knowledge-import/' +
      detail.domain_id +
      '?' +
      new URLSearchParams(params)
    );
  },
};
export default service;

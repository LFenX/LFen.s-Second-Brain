import type { AxiosResponse } from 'axios';
import { registerNextServer } from '@/utils/register-server';
import { KnowledgeAccessVersion } from '@/interfaces/database/knowledge-access';
import type {
  AccessError,
  AccessInspectData,
  AccessResults,
} from '@/interfaces/database/knowledge-access';
import type {
  AccessOperation,
  AccessRequests,
} from '@/interfaces/request/knowledge-access';
import {
  accessBytes,
  accessCanonical,
  accessInspectValid,
  accessReadValid,
  accessSame,
  accessSchemaValid,
  accessSearchValid,
  accessTargetsValid,
  parseAccessJSON,
} from '@/utils/knowledge-access-schema';

const Server = registerNextServer({
  post: { url: (config: { url: string }) => config.url, method: 'post' },
});
const Operations = {
  setPolicy: [
    'admin/domains/policy/set',
    'SetPolicyRequest',
    'SetPolicyResponse',
  ],
  setModule: ['admin/modules/set', 'SetModuleRequest', 'SetModuleResponse'],
  getModuleAdmin: [
    'admin/modules/get',
    'GetModuleAdminRequest',
    'GetModuleAdminResponse',
  ],
  listModules: ['modules/list', 'ListModulesRequest', 'ListModulesResponse'],
  createGrant: [
    'admin/grants/create',
    'CreateGrantRequest',
    'CreateGrantResponse',
  ],
  revokeGrant: [
    'admin/grants/revoke',
    'RevokeGrantRequest',
    'RevokeGrantResponse',
  ],
  inspect: ['inspect', 'InspectRequest', 'InspectResponse'],
  search: ['search', 'SearchRequest', 'SearchResponse'],
  read: ['read', 'ReadRequest', 'ReadResponse'],
} as const;
const ErrorStatus: Record<AccessError['code'], number> = {
  invalid_request: 400,
  unauthenticated: 401,
  action_denied: 403,
  not_found: 404,
  version_conflict: 409,
  scope_changed: 409,
  view_stale: 409,
  index_stale: 409,
  index_unindexed: 409,
  scope_not_supported: 409,
  idempotency_conflict: 409,
  integrity_error: 409,
  contract_mismatch: 412,
  capacity_exceeded: 413,
  rate_limited: 429,
  unavailable: 503,
};

export class AccessRequestError extends Error {
  constructor(
    public status: number,
    public code: string,
    public outcomeKnown: boolean | null = null,
    public retrySameKey = false,
    public requestId = '',
  ) {
    super(code);
    this.name = 'AccessRequestError';
  }
}
const invalid = (local = false) =>
  new AccessRequestError(
    local ? 400 : 502,
    local ? 'invalid_request' : 'invalid_response',
    local ? true : null,
    !local,
  );

export function accessRequestValid<O extends AccessOperation>(
  operation: O,
  body: AccessRequests[O],
): boolean {
  try {
    parseAccessJSON(JSON.stringify(body));
  } catch {
    return false;
  }
  if (
    !accessSchemaValid(Operations[operation][1], body) ||
    accessBytes(JSON.stringify(body)) > 32768
  )
    return false;
  if (operation === 'setModule') {
    const value = body as AccessRequests['setModule'];
    return (
      (value.module_id === null
        ? value.expected_version === 0
        : value.expected_version >= 1) &&
      accessBytes(value.name) <= 128 &&
      accessTargetsValid(value.scope.targets, value.domain_id)
    );
  }
  if (operation === 'createGrant') {
    const value = body as AccessRequests['createGrant'];
    return value.subject.kind === 'user' && value.delivery === 'web_only';
  }
  if (operation === 'inspect')
    return (body as AccessRequests['inspect']).delivery === 'web_only';
  if (operation === 'search')
    return accessBytes((body as AccessRequests['search']).query) <= 4096;
  return true;
}

async function call<O extends AccessOperation>(
  operation: O,
  body: AccessRequests[O],
  signal?: AbortSignal,
): Promise<AccessResults[O]> {
  if (!accessRequestValid(operation, body)) throw invalid(true);
  let response: AxiosResponse<unknown>;
  try {
    response = await Server.post(
      {
        url: '/api/v1/knowledge/access/' + Operations[operation][0],
        data: JSON.stringify(body),
        signal,
        timeout: 15000,
        headers: { 'Content-Type': 'application/json' },
        skipGlobalErrorNotification: true,
        responseType: 'arraybuffer',
        transformResponse: [
          (raw: unknown) => {
            if (!(raw instanceof ArrayBuffer) && !ArrayBuffer.isView(raw))
              throw invalid();
            if (raw.byteLength > 1048576) throw invalid();
            try {
              return parseAccessJSON(
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
  } catch (issue) {
    if (issue instanceof AccessRequestError) throw issue;
    const failure = issue as {
      response?: AxiosResponse<unknown>;
      code?: string;
    };
    if (failure.code === 'ERR_CANCELED') throw issue;
    if (!failure.response)
      throw new AccessRequestError(0, 'unavailable', null, true);
    response = failure.response;
  }
  if (
    String(response.headers['x-knowledge-access-version']) !==
      KnowledgeAccessVersion ||
    String(response.headers['content-type'])
      .split(';')[0]
      .trim()
      .toLowerCase() !== 'application/json' ||
    !String(response.headers['cache-control'])
      .toLowerCase()
      .split(',')
      .map((item) => item.trim())
      .includes('no-store') ||
    !accessSchemaValid(Operations[operation][2], response.data)
  )
    throw invalid();
  const envelope = response.data as
    | {
        operation: AccessOperation;
        request_id: string;
        data: AccessResults[O];
      }
    | AccessError;
  if (
    envelope.operation !== operation ||
    String(response.headers['x-request-id']) !== envelope.request_id
  )
    throw invalid();
  if ('code' in envelope) {
    if (
      ErrorStatus[envelope.code] !== response.status ||
      envelope.data !== null
    )
      throw invalid();
    throw new AccessRequestError(
      response.status,
      envelope.code,
      envelope.outcome_known,
      envelope.retry_same_key,
      envelope.request_id,
    );
  }
  if (
    response.status !== 200 &&
    !(operation === 'createGrant' && response.status === 201)
  )
    throw invalid();
  return envelope.data as AccessResults[O];
}

const service = {
  async listModules(body: AccessRequests['listModules'], signal?: AbortSignal) {
    const value = await call('listModules', body, signal);
    if (
      value.owned_domain_policies.some(
        (policy) => policy.version === 0 && policy.enabled,
      )
    )
      throw invalid();
    if (
      body.domain_id !== null &&
      (value.items.some((item) => item.domain_id !== body.domain_id) ||
        value.owned_domain_policies.some(
          (policy) => policy.domain_id !== body.domain_id,
        ))
    )
      throw invalid();
    return value;
  },
  async getModuleAdmin(
    body: AccessRequests['getModuleAdmin'],
    signal?: AbortSignal,
  ) {
    const value = await call('getModuleAdmin', body, signal);
    if (
      value.domain_id !== body.domain_id ||
      value.module_id !== body.module_id ||
      value.policy.domain_id !== body.domain_id ||
      !accessTargetsValid(value.scope.targets, body.domain_id) ||
      (value.policy.version === 0 && value.policy.enabled) ||
      value.grants.some(
        (grant) =>
          grant.domain_id !== body.domain_id ||
          grant.module_id !== body.module_id ||
          grant.module_version !== value.version,
      )
    )
      throw invalid();
    return value;
  },
  async setPolicy(body: AccessRequests['setPolicy']) {
    const value = await call('setPolicy', body);
    if (
      value.domain_id !== body.domain_id ||
      value.enabled !== body.enabled ||
      ![body.expected_version, body.expected_version + 1].includes(
        value.version,
      )
    )
      throw invalid();
    return value;
  },
  async setModule(body: AccessRequests['setModule']) {
    const value = await call('setModule', body);
    const sorted = (targets: typeof body.scope.targets) =>
      targets.map((target) => accessCanonical(target)).sort();
    if (
      value.domain_id !== body.domain_id ||
      (body.module_id !== null && value.module_id !== body.module_id) ||
      value.name !== body.name ||
      value.enabled !== body.enabled ||
      ![body.expected_version, body.expected_version + 1].includes(
        value.version,
      ) ||
      !accessSame(value.scope.publication, body.scope.publication) ||
      !accessSame(sorted(value.scope.targets), sorted(body.scope.targets))
    )
      throw invalid();
    return value;
  },
  async createGrant(body: AccessRequests['createGrant']) {
    const value = await call('createGrant', body);
    if (
      value.domain_id !== body.domain_id ||
      value.module_id !== body.module_id ||
      value.module_version !== body.expected_module_version ||
      !accessSame(value.subject, body.subject) ||
      value.mode !== body.mode ||
      value.delivery !== body.delivery ||
      Date.parse(value.expires_at) !== Date.parse(body.expires_at)
    )
      throw invalid();
    return value;
  },
  async revokeGrant(body: AccessRequests['revokeGrant']) {
    const value = await call('revokeGrant', body);
    if (value.id !== body.grant_id) throw invalid();
    return value;
  },
  async inspect(body: AccessRequests['inspect'], signal?: AbortSignal) {
    const value = await call('inspect', body, signal);
    if (
      value.domain_id !== body.domain_id ||
      value.module_id !== body.module_id ||
      value.mode !== body.mode ||
      value.delivery !== body.delivery ||
      !accessInspectValid(value) ||
      (body.source_ids !== null &&
        !accessSame(
          [...body.source_ids].sort(),
          value.targets.map((entry) => entry.target.source.source_id).sort(),
        ))
    )
      throw invalid();
    return value;
  },
  async search(
    body: AccessRequests['search'],
    view: AccessInspectData,
    signal?: AbortSignal,
  ) {
    if (view.mode !== 'read_search' || body.view_id !== view.view_id)
      throw invalid(true);
    const value = await call('search', body, signal);
    if (
      value.query !== body.query ||
      value.requested_limit !== body.limit ||
      !(await accessSearchValid(value, view))
    )
      throw invalid();
    return value;
  },
  async read(
    body: AccessRequests['read'],
    view: AccessInspectData,
    signal?: AbortSignal,
  ) {
    const selector = body.selector;
    if (
      (selector.kind === 'unit' &&
        (selector.view_id !== view.view_id ||
          !view.targets[selector.target_index] ||
          selector.unit_ordinal >=
            view.targets[selector.target_index].unit_count)) ||
      (selector.kind === 'evidence' &&
        selector.evidence_ref.view_id !== view.view_id)
    )
      throw invalid(true);
    const value = await call('read', body, signal);
    if (
      !(await accessReadValid(value, view)) ||
      (selector.kind === 'unit' &&
        (value.unit_locator.target_index !== selector.target_index ||
          value.unit_locator.unit_ordinal !== selector.unit_ordinal)) ||
      (selector.kind === 'evidence' &&
        !accessSame(value.evidence_ref, selector.evidence_ref))
    )
      throw invalid();
    return value;
  },
};
export default service;

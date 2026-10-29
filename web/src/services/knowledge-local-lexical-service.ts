import type { AxiosResponse } from 'axios';
import type {
  LocalLexicalData,
  LocalLexicalError,
  LocalLexicalOperation,
} from '@/interfaces/database/knowledge-local-lexical';
import type {
  LocalLexicalBodies,
  LocalLexicalCreateBody,
  LocalLexicalInspectBody,
  LocalLexicalPolicyBody,
  LocalLexicalReadBody,
  LocalLexicalScope,
  LocalLexicalSearchBody,
} from '@/interfaces/request/knowledge-local-lexical';
import { registerNextServer } from '@/utils/register-server';
import {
  checkLocalLexicalRequest,
  checkLocalLexicalResponse,
  LocalLexicalOperations,
  validLocalLexicalSchema,
  LocalLexicalVersion,
} from '@/utils/knowledge-local-lexical';
import { parseContentJSON, ContentLimits } from '@/utils/knowledge-consumption';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';

export class LocalLexicalRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId = '',
    public outcomeKnown = false,
    public retrySameKey = false,
  ) {
    super(status, code, requestId);
  }
}
const Server = registerNextServer({
  get: { url: (config: { url: string }) => config.url, method: 'get' },
  post: { url: (config: { url: string }) => config.url, method: 'post' },
});
const ContentOperations = new Set<LocalLexicalOperation>([
  'inspectIndex',
  'createIndex',
  'getIndexJob',
  'search',
  'read',
]);
const Status: Record<string, number[]> = {
  invalid_request: [400],
  unauthenticated: [401],
  not_found: [404],
  contract_mismatch: [412],
  policy_disabled: [409],
  policy_changed: [409],
  profile_changed: [409],
  source_changed: [409],
  idempotency_conflict: [409],
  index_unindexed: [409],
  index_building: [409],
  index_coverage_gap: [409],
  index_stale: [409],
  index_failed: [409],
  coverage_gap: [409],
  unit_oversize: [413],
  unsupported_representation: [415],
  source_unavailable: [409],
  integrity_error: [409],
  capacity_exceeded: [413, 429],
  empty_content: [409],
  unavailable: [503],
};
const invalid = () =>
  new LocalLexicalRequestError(
    502,
    'local_lexical_invalid_response',
    '',
    false,
    true,
  );
async function call<O extends LocalLexicalOperation>(
  operation: O,
  scope: LocalLexicalScope,
  body: LocalLexicalBodies[O],
  signal?: AbortSignal,
): Promise<LocalLexicalData[O]> {
  try {
    checkLocalLexicalRequest(operation, scope, body);
  } catch {
    throw new LocalLexicalRequestError(400, 'invalid_request', '', true, false);
  }
  const [, method, suffix] = LocalLexicalOperations[operation],
    content = ContentOperations.has(operation);
  let response: AxiosResponse;
  try {
    response = await Server[method](
      {
        url: '/api/v1/datasets/' + scope.domain + '/knowledge' + suffix,
        ...(body === undefined
          ? {}
          : {
              data: JSON.stringify(body),
              headers: { 'Content-Type': 'application/json' },
            }),
        signal,
        timeout: 25000,
        responseType: 'arraybuffer',
        skipGlobalErrorNotification: true,
        transformResponse: [
          (raw: unknown) => {
            try {
              if (
                (!(raw instanceof ArrayBuffer) && !ArrayBuffer.isView(raw)) ||
                raw.byteLength > ContentLimits.response
              )
                throw invalid();
              return parseContentJSON(
                new TextDecoder('utf-8', { fatal: true }).decode(raw),
                content,
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
    if (error instanceof LocalLexicalRequestError) throw error;
    const failure = error as { response?: AxiosResponse; code?: string };
    if (failure.code === 'ERR_CANCELED') throw error;
    if (!failure.response)
      throw new LocalLexicalRequestError(
        0,
        'local_lexical_result_unknown',
        '',
        false,
        true,
      );
    response = failure.response;
  }
  if (
    content &&
    (String(response.headers['x-knowledge-content-version']) !==
      LocalLexicalVersion ||
      String(response.headers['x-knowledge-content-lexical-protocol']) !==
        'content_lexical/0.2.0' ||
      String(response.headers['cache-control']) !== 'no-store')
  )
    throw invalid();
  if (
    String(response.headers['content-type'])
      .split(';')[0]
      .trim()
      .toLowerCase() !== 'application/json'
  )
    throw invalid();
  const raw = response.data as
    | LocalLexicalError
    | { operation: string; request_id: string; data: unknown };
  if (
    !raw ||
    typeof raw !== 'object' ||
    raw.operation !== operation ||
    (content && String(response.headers['x-request-id']) !== raw.request_id)
  )
    throw invalid();
  if ('code' in raw) {
    if (
      !validLocalLexicalSchema(content ? 'LexicalError' : 'ErrorResponse', raw)
    )
      throw invalid();
    if (content && !Status[raw.code]?.includes(response.status))
      throw invalid();
    if (!content && [401, 403].includes(response.status))
      throw new LocalLexicalRequestError(response.status, 'unauthenticated');
    throw new LocalLexicalRequestError(
      response.status,
      raw.code,
      raw.request_id,
      raw.outcome_known,
      raw.retry_same_key,
    );
  }
  if (response.status !== (content && operation === 'createIndex' ? 202 : 200))
    throw invalid();
  try {
    return await checkLocalLexicalResponse(operation, raw, scope, body);
  } catch {
    throw invalid();
  }
}
const service = {
  policy: (scope: LocalLexicalScope, signal?: AbortSignal) =>
    call('getLocalPolicy', scope, undefined, signal),
  changePolicy: (
    scope: LocalLexicalScope,
    body: LocalLexicalPolicyBody,
    signal?: AbortSignal,
  ) => call('changeLocalPolicy', scope, body, signal),
  readiness: (
    scope: LocalLexicalScope,
    body: LocalLexicalInspectBody,
    signal?: AbortSignal,
  ) => call('inspectIndex', scope, body, signal),
  createJob: (
    scope: LocalLexicalScope,
    body: LocalLexicalCreateBody,
    signal?: AbortSignal,
  ) => call('createIndex', scope, body, signal),
  job: (scope: LocalLexicalScope, job: string, signal?: AbortSignal) =>
    call(
      'getIndexJob',
      scope,
      { contract_version: LocalLexicalVersion, job_id: job },
      signal,
    ),
  search: (
    scope: LocalLexicalScope,
    body: LocalLexicalSearchBody,
    signal?: AbortSignal,
  ) => call('search', scope, body, signal),
  read: (
    scope: LocalLexicalScope,
    body: LocalLexicalReadBody,
    signal?: AbortSignal,
  ) => call('read', scope, body, signal),
};
export default service;

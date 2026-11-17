import Ajv2020 from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import type { AxiosResponse } from 'axios';
import type { ValidateFunction } from 'ajv';
import Contract from '../../../docs/knowledge-answer-20261117-02/answer.schema.candidate.json';
import LexicalContract from '../../../docs/knowledge-core-content-lexical-v0.2.candidate.schema.json';
import { registerNextServer } from '@/utils/register-server';
import {
  contentBytes,
  contentCanonical,
  parseContentJSON,
} from '@/utils/knowledge-consumption';
import {
  checkLocalLexicalSelection,
  checkLocalLexicalEvidence,
} from '@/utils/knowledge-local-lexical';
import { webSha256 } from '@/utils/knowledge-web';
import { KnowledgeRequestError } from './knowledge-foundation-service';
import type { LocalLexicalScope } from '@/interfaces/request/knowledge-local-lexical';
import {
  KnowledgeAnswerVersion,
  type AnswerData,
  type AnswerOperation,
  type AnswerPolicy,
} from '@/interfaces/database/knowledge-answer';
import type { AnswerRequests } from '@/interfaces/request/knowledge-answer';

export class AnswerRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId = '',
    public outcomeKnown = false,
  ) {
    super(status, code, requestId);
  }
}
const Compiler = new Ajv2020({
  strict: false,
  allErrors: false,
  inlineRefs: false,
  validateFormats: true,
  strictNumbers: true,
});
addFormats(Compiler);
Compiler.addKeyword({
  keyword: 'x-max-utf8-bytes',
  type: 'string',
  schemaType: 'number',
  validate: (limit: number, value: string) => contentBytes(value) <= limit,
});
Compiler.addSchema(LexicalContract);
Compiler.addSchema(Contract);
const Validators = new Map<string, ValidateFunction>();
function matches(name: string, value: unknown) {
  let validate = Validators.get(name);
  if (!validate) {
    validate = Compiler.compile({ $ref: Contract.$id + '#/$defs/' + name });
    Validators.set(name, validate);
  }
  return validate(value);
}
const Operations = {
  inspect: ['inspect', 'InspectRequest', 'InspectResponse'],
  getPolicy: ['policy/get', 'PolicyGetRequest', 'PolicyGetResponse'],
  setPolicy: ['policy/set', 'PolicySetRequest', 'PolicySetResponse'],
  answer: ['run', 'AnswerRequest', 'AnswerResponse'],
} as const;
const Server = registerNextServer({
  post: { url: (config: { url: string }) => config.url, method: 'post' },
});
const invalid = () => new AnswerRequestError(502, 'invalid_response');
const Id = /^[a-f0-9]{32}$/;

async function call<O extends AnswerOperation>(
  operation: O,
  scope: LocalLexicalScope,
  body: AnswerRequests[O],
  signal?: AbortSignal,
  policy?: AnswerPolicy,
): Promise<AnswerData[O]> {
  if (
    !Id.test(scope.domain) ||
    !Id.test(scope.owner) ||
    !matches(Operations[operation][1], body)
  )
    throw new AnswerRequestError(400, 'invalid_request');
  if ('scope' in body && body.scope)
    checkLocalLexicalSelection(body.scope.selection, scope.domain);
  let response: AxiosResponse;
  try {
    response = await Server.post(
      {
        url: `/api/v1/datasets/${scope.domain}/knowledge/answer/${Operations[operation][0]}`,
        data: JSON.stringify(body),
        signal,
        timeout: 80000,
        responseType: 'arraybuffer',
        skipGlobalErrorNotification: true,
        headers: {
          'Content-Type': 'application/json',
          'X-Knowledge-Contract-Version': '1.0.0',
          'X-Knowledge-Answer-Contract-Version': KnowledgeAnswerVersion,
        },
        transformResponse: [
          (raw: unknown) => {
            if (
              (!(raw instanceof ArrayBuffer) && !ArrayBuffer.isView(raw)) ||
              raw.byteLength > 1048576
            )
              throw invalid();
            return parseContentJSON(
              new TextDecoder('utf-8', { fatal: true }).decode(raw),
              true,
            );
          },
        ],
      },
      true,
    );
  } catch (error) {
    if (error instanceof AnswerRequestError) throw error;
    const failure = error as { response?: AxiosResponse; code?: string };
    if (failure.code === 'ERR_CANCELED') throw error;
    if (!failure.response)
      throw new AnswerRequestError(0, 'provider_outcome_unknown');
    response = failure.response;
  }
  const raw = response.data;
  if (
    String(response.headers['x-knowledge-contract-version']) !== '1.0.0' ||
    String(response.headers['x-knowledge-answer-contract-version']) !==
      KnowledgeAnswerVersion ||
    response.headers['cache-control'] !== 'no-store' ||
    String(response.headers['content-type']).split(';')[0].trim() !==
      'application/json' ||
    !matches(Operations[operation][2], raw) ||
    raw.operation !== operation ||
    raw.request_id !== response.headers['x-request-id']
  )
    throw invalid();
  if (raw.code) {
    if (!matches('Error', raw) || response.status < 400) throw invalid();
    throw new AnswerRequestError(
      response.status,
      raw.code,
      raw.request_id,
      raw.outcome_known,
    );
  }
  const result = raw.data as AnswerData[O];
  if (
    response.status !==
    (operation === 'answer' && 'state' in result && result.state === 'running'
      ? 202
      : 200)
  )
    throw invalid();
  if (operation === 'getPolicy' || operation === 'setPolicy') {
    const p = result as AnswerPolicy;
    if (
      p.domain_id !== scope.domain ||
      (p.enabled && p.consent_actor !== scope.owner)
    )
      throw invalid();
    if (p.scope) checkLocalLexicalSelection(p.scope.selection, scope.domain);
  }
  if (operation === 'inspect') {
    const inspected = result as AnswerData['inspect'],
      requested = body as AnswerRequests['inspect'];
    checkLocalLexicalSelection(inspected.scope.selection, scope.domain);
    const targets = (value: typeof inspected.scope.selection) =>
      [...value.targets].map(contentCanonical).sort();
    if (
      inspected.model_ref.model_id !== requested.model_id ||
      contentCanonical(inspected.scope.index_ref) !==
        contentCanonical(requested.scope.index_ref) ||
      contentCanonical(targets(inspected.scope.selection)) !==
        contentCanonical(targets(requested.scope.selection))
    )
      throw invalid();
  }
  if (operation === 'answer') {
    const answer = result as AnswerData['answer'];
    if (
      !policy?.enabled ||
      !policy.scope ||
      answer.policy_version !== policy.version ||
      answer.scope_sha256 !== policy.scope_sha256 ||
      contentCanonical(answer.model_ref) !== contentCanonical(policy.model_ref)
    )
      throw invalid();
    if (answer.state === 'completed') {
      for (const cite of answer.citations) {
        checkLocalLexicalEvidence(
          cite.evidence_ref,
          scope.domain,
          policy.scope.selection,
        );
        if (
          contentCanonical(cite.evidence_ref.index_ref) !==
            contentCanonical(policy.scope.index_ref) ||
          cite.byte_length !== contentBytes(cite.text) ||
          cite.evidence_ref.range.end_byte -
            cite.evidence_ref.range.start_byte !==
            cite.byte_length ||
          cite.evidence_ref.range.sha256 !== cite.sha256 ||
          (await webSha256(new TextEncoder().encode(cite.text))) !== cite.sha256
        )
          throw invalid();
      }
    }
  }
  return result;
}
export default {
  policy: (scope: LocalLexicalScope, signal?: AbortSignal) =>
    call(
      'getPolicy',
      scope,
      { contract_version: KnowledgeAnswerVersion },
      signal,
    ),
  inspect: (
    scope: LocalLexicalScope,
    body: AnswerRequests['inspect'],
    signal?: AbortSignal,
  ) => call('inspect', scope, body, signal),
  setPolicy: (
    scope: LocalLexicalScope,
    body: AnswerRequests['setPolicy'],
    signal?: AbortSignal,
  ) => call('setPolicy', scope, body, signal),
  answer: (
    scope: LocalLexicalScope,
    body: AnswerRequests['answer'],
    policy: AnswerPolicy,
    signal?: AbortSignal,
  ) => call('answer', scope, body, signal, policy),
};

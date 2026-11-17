import request from '@/utils/next-request';
import { createHash, webcrypto } from 'node:crypto';
import service from '../knowledge-answer-service';
import { KnowledgeAnswerVersion } from '@/interfaces/database/knowledge-answer';
type AnswerPolicy =
  import('@/interfaces/database/knowledge-answer').AnswerPolicy;

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock;
Object.defineProperty(globalThis, 'crypto', {
  configurable: true,
  value: webcrypto,
});
const Scope = { domain: '1'.repeat(32), owner: 'a'.repeat(32) };
const Disabled: AnswerPolicy = {
  domain_id: Scope.domain,
  version: 0,
  enabled: false,
  scope: null,
  scope_sha256: null,
  model_ref: null,
  consent_actor: null,
  consent_time: null,
};
const RequestId = 'f'.repeat(32);
function response(
  config: any,
  value: unknown,
  status = 200,
  headers: Record<string, string> = {},
) {
  const bytes = new TextEncoder().encode(
    typeof value === 'string' ? value : JSON.stringify(value),
  );
  return {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
      'x-knowledge-contract-version': '1.0.0',
      'x-knowledge-answer-contract-version': KnowledgeAnswerVersion,
      'x-request-id': RequestId,
      ...headers,
    },
    data: config.transformResponse[0](bytes),
  };
}
const envelope = (data: unknown) => ({
  operation: 'getPolicy',
  contract_version: KnowledgeAnswerVersion,
  request_id: RequestId,
  data,
});
beforeEach(() => Http.mockReset());

it('uses only the native versioned policy endpoint with no model call, retry or credential fields', async () => {
  Http.mockImplementation(async (config) =>
    response(config, envelope(Disabled)),
  );
  await expect(service.policy(Scope)).resolves.toEqual(Disabled);
  const config = Http.mock.calls[0][0];
  expect(config.url).toBe(
    `/api/v1/datasets/${Scope.domain}/knowledge/answer/policy/get`,
  );
  expect(JSON.parse(config.data)).toEqual({
    contract_version: KnowledgeAnswerVersion,
  });
  expect(config.headers['X-Knowledge-Answer-Contract-Version']).toBe(
    KnowledgeAnswerVersion,
  );
  expect(Http).toHaveBeenCalledTimes(1);
});

it.each([
  ['wrong domain', { ...Disabled, domain_id: '2'.repeat(32) }, {}],
  ['unknown field', { ...Disabled, api_key: 'PRIVATE-FIELD' }, {}],
  [
    'wrong version',
    Disabled,
    { 'x-knowledge-answer-contract-version': 'other' },
  ],
  ['missing no-store', Disabled, { 'cache-control': 'public' }],
  ['wrong request ID', Disabled, { 'x-request-id': 'e'.repeat(32) }],
] as const)(
  'rejects %s before exposing data',
  async (_label, data, headers) => {
    Http.mockImplementation(async (config) =>
      response(config, envelope(data), 200, headers),
    );
    await expect(service.policy(Scope)).rejects.toMatchObject({
      code: 'invalid_response',
    });
  },
);

it('rejects duplicated JSON and native error text without surfacing the raw body', async () => {
  Http.mockImplementation(async (config) =>
    response(
      config,
      JSON.stringify(envelope(Disabled)).replace(
        '"version":0',
        '"version":0,"version":99',
      ),
    ),
  );
  await expect(service.policy(Scope)).rejects.toBeDefined();
  Http.mockImplementation(async () => {
    throw {
      response: {
        status: 502,
        headers: { 'content-type': 'text/html' },
        data: 'PRIVATE-NATIVE-ERROR',
      },
    };
  });
  await expect(service.policy(Scope)).rejects.toMatchObject({
    message: 'invalid_response',
  });
});

it('treats a transport timeout as uncertain and never repeats the request automatically', async () => {
  Http.mockRejectedValue({ code: 'ECONNABORTED' });
  await expect(service.policy(Scope)).rejects.toMatchObject({
    code: 'provider_outcome_unknown',
    outcomeKnown: false,
  });
  expect(Http).toHaveBeenCalledTimes(1);
});

it('rejects an enabling request without fixed scope or consent before sending HTTP', async () => {
  await expect(
    service.setPolicy(Scope, {
      contract_version: KnowledgeAnswerVersion,
      expected_version: 0,
      enabled: true,
      scope: null,
      model_ref: null,
      confirmed: true,
      external_send_confirmed: false,
      idempotency_key: 'enable',
    }),
  ).rejects.toMatchObject({ code: 'invalid_request' });
  expect(Http).not.toHaveBeenCalled();
});

it.each([
  'valid',
  'changed-text',
  'foreign-source',
  'wrong-model',
  'native-usage-fields',
] as const)('checks fixed citation bytes and bindings: %s', async (change) => {
  const text = 'SYNTHETIC ORION quota 47',
    sha = createHash('sha256').update(text).digest('hex');
  const target = {
    source_ref: {
      domain_id: Scope.domain,
      source_id: 'e'.repeat(32),
      source_revision_id: 'd'.repeat(32),
      source_sha256: sha,
    },
    representation_ref: {
      kind: 'source_revision' as const,
      id: 'd'.repeat(32),
      producer_sha256: null,
    },
  };
  const index = { index_id: '1'.repeat(64), manifest_sha256: '2'.repeat(64) };
  const permit: AnswerPolicy = {
    ...Disabled,
    version: 1,
    enabled: true,
    scope: {
      index_ref: index,
      selection: { kind: 'fixed_representations', targets: [target] },
    },
    scope_sha256: '3'.repeat(64),
    model_ref: {
      model_id: 'b'.repeat(32),
      configuration_sha256: 'c'.repeat(64),
    },
    consent_actor: Scope.owner,
    consent_time: '2026-11-17T00:00:00Z',
  };
  const evidence = {
    evidence_version: '0.2.0-candidate',
    index_ref: index,
    index_row_id: '5'.repeat(64),
    address: {
      target: JSON.parse(JSON.stringify(target)),
      node_ref: { kind: 'c1_node', id: '4'.repeat(64), item_index: null },
    },
    coordinate_space: 'source_utf8',
    reading_text_sha256: sha,
    range: { start_byte: 0, end_byte: text.length, sha256: sha },
    locator: {
      kind: 'source_utf8',
      source_utf8: {
        start_byte: 0,
        end_byte: text.length,
        start_line: 1,
        end_line: 1,
      },
      document: null,
      item_index: null,
    },
    original_position: null,
    coordinate_precision: 'source_bytes',
    assembly: 'c1_root_subrange_exact_bytes',
  };
  const citation = {
    id: 'E1',
    evidence_ref: evidence,
    text,
    byte_length: text.length,
    sha256: sha,
  };
  const result = {
    answer_id: '6'.repeat(32),
    state: 'completed',
    policy_version: 1,
    scope_sha256: permit.scope_sha256,
    model_ref: { ...permit.model_ref },
    answer: '47',
    insufficient_evidence: false,
    citations: [citation],
    usage:
      change === 'native-usage-fields'
        ? { prompt_tokens: 150, completion_tokens: 12, total_tokens: 162 }
        : { input_tokens: 150, output_tokens: 12, total_tokens: 162 },
    provider_calls: 1,
    replayed: false,
  };
  if (change === 'changed-text') citation.text = 'PRIVATE-MUTATED-CITATION';
  if (change === 'foreign-source')
    evidence.address.target.source_ref.source_id = '7'.repeat(32);
  if (change === 'wrong-model') result.model_ref.model_id = '8'.repeat(32);
  Http.mockImplementation(async (config) =>
    response(config, { ...envelope(result), operation: 'answer' }),
  );
  const reading = service.answer(
    Scope,
    {
      contract_version: KnowledgeAnswerVersion,
      expected_policy_version: 1,
      question: 'ORION quota?',
      retrieval_query: null,
      idempotency_key: 'one-answer',
    },
    permit,
  );
  if (change === 'valid') await expect(reading).resolves.toEqual(result);
  else await expect(reading).rejects.toBeDefined();
  expect(Http).toHaveBeenCalledTimes(1);
});

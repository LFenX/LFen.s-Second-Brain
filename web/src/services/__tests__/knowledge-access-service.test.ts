import { webcrypto as NodeWebCrypto } from 'node:crypto';

type AccessInspectData =
  import('@/interfaces/database/knowledge-access').AccessInspectData;
import service, { AccessRequestError } from '../knowledge-access-service';
import { registerNextServer } from '@/utils/register-server';
import { KnowledgeAccessVersion } from '@/interfaces/database/knowledge-access';
import {
  accessSchemaValid,
  accessTargetsValid,
  parseAccessJSON,
} from '@/utils/knowledge-access-schema';
import { accessPublicationChoice } from '@/utils/knowledge-access-navigation';
import {
  Domain,
  ModuleId,
  Tenant,
  User,
  admin,
  evidence,
  example,
  grant,
  searchResult,
  view,
} from './knowledge-access-fixtures';

jest.mock('@/utils/register-server', () => {
  const post = jest.fn();
  return { registerNextServer: jest.fn(() => ({ post })) };
});
const post = (
  jest.mocked(registerNextServer).mock.results[0].value as { post: jest.Mock }
).post;
function response(
  operation: string,
  data: unknown,
  status = 200,
  extra: Record<string, unknown> = {},
) {
  return {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
      'x-knowledge-access-version': KnowledgeAccessVersion,
      'x-request-id': '0'.repeat(32),
    } as Record<string, string>,
    data: {
      operation,
      contract_version: KnowledgeAccessVersion,
      request_id: '0'.repeat(32),
      data,
      ...extra,
    },
  };
}
const inspectBody = () => ({
  contract_version: KnowledgeAccessVersion,
  domain_id: Domain,
  module_id: ModuleId,
  mode: 'read' as const,
  delivery: 'web_only' as const,
  source_ids: null,
});
let cryptoGetterSpy: jest.SpyInstance<Crypto, []> | undefined;

beforeEach(() => {
  post.mockReset();
  cryptoGetterSpy = jest
    .spyOn(globalThis, 'crypto', 'get')
    .mockReturnValue(NodeWebCrypto as unknown as Crypto);
});

afterEach(() => {
  cryptoGetterSpy?.mockRestore();
  cryptoGetterSpy = undefined;
});

it('posts only the new native read selector, with strict byte decoding and no owner fallback', async () => {
  post.mockResolvedValue(response('read', evidence()));
  await expect(service.read(example('EX04'), view())).resolves.toEqual(
    evidence(),
  );
  expect(post).toHaveBeenCalledTimes(1);
  const [config, native] = post.mock.calls[0];
  expect(native).toBe(true);
  expect(config.url).toBe('/api/v1/knowledge/access/read');
  expect(JSON.parse(config.data)).toEqual(example('EX04'));
  expect(config.responseType).toBe('arraybuffer');
  expect(config.skipGlobalErrorNotification).toBe(true);
  const decode = config.transformResponse[0];
  expect(() => decode(new TextEncoder().encode('{"a":1,"a":2}'))).toThrow();
  expect(() => decode(new Uint8Array([0xc0, 0xaf]))).toThrow();
  expect(() => decode(new ArrayBuffer(1048577))).toThrow();
  expect(() =>
    decode(new TextEncoder().encode('{"n":9007199254740992}')),
  ).toThrow();
  expect(() => decode(new TextEncoder().encode('{"s":"\\ud800"}'))).toThrow();
});

it.each([
  'extra field',
  'wrong version',
  'wrong request id',
  'cacheable',
  'wrong media type',
  'wrong operation',
  'bad target',
  'bad total',
  'bad order',
])('refuses malformed inspect output: %s', async (problem) => {
  const result = response('inspect', view());
  const data = result.data.data as AccessInspectData;
  if (problem === 'extra field')
    Object.assign(data, { publication: 'PRIVATE-B-MARKER' });
  if (problem === 'wrong version')
    result.headers['x-knowledge-access-version'] = '0.2.0-candidate';
  if (problem === 'wrong request id')
    result.headers['x-request-id'] = '1'.repeat(32);
  if (problem === 'cacheable') result.headers['cache-control'] = 'public';
  if (problem === 'wrong media type')
    result.headers['content-type'] = 'text/plain';
  if (problem === 'wrong operation') result.data.operation = 'read';
  if (problem === 'bad target')
    data.targets[0].target.source.domain_id = '1'.repeat(32);
  if (problem === 'bad total') data.unit_count += 1;
  if (problem === 'bad order') {
    const second = JSON.parse(JSON.stringify(data.targets[0]));
    second.target.source.source_id = '0'.repeat(32);
    data.targets.push(second);
    data.source_count = 2;
    data.unit_count = 2;
  }
  post.mockResolvedValue(result);
  await expect(service.inspect(inspectBody())).rejects.toMatchObject({
    code: 'invalid_response',
  });
});

it.each([
  'text SHA',
  'byte length',
  'locator',
  'foreign citation',
  'raw typed payload',
  'empty member',
  'member slice',
])('rejects an invalid complete read: %s', async (problem) => {
  const data = evidence();
  if (problem === 'text SHA') data.text += 'B_ONLY_NOT_AUTHORIZED';
  if (problem === 'byte length') data.byte_length += 1;
  if (problem === 'locator') data.unit_locator.next_unit_ordinal = 1;
  if (problem === 'foreign citation')
    data.citation.target.source.source_id = '1'.repeat(32);
  if (problem === 'raw typed payload')
    Object.assign(data.typed_context.members[0], {
      Attributes: { secret: 'PRIVATE-B' },
    });
  if (problem === 'empty member')
    data.typed_context.members[1].range.sha256 = '0'.repeat(64);
  if (problem === 'member slice')
    data.typed_context.members[0].range.sha256 = '0'.repeat(64);
  post.mockResolvedValue(response('read', data));
  await expect(service.read(example('EX04'), view())).rejects.toMatchObject({
    code: 'invalid_response',
  });
});

it('binds search output to the same view, ordinal, query, limit and exact evidence reread', async () => {
  const catalogue = view();
  catalogue.mode = 'read_search';
  const body = {
    contract_version: KnowledgeAccessVersion,
    view_id: catalogue.view_id,
    query: '合成A',
    limit: 20,
  };
  post.mockResolvedValue(response('search', searchResult(catalogue)));
  await expect(service.search(body, catalogue)).resolves.toMatchObject({
    returned_count: 1,
  });
  const wrong = searchResult(catalogue);
  wrong.hits[0].rank_ordinal = 2;
  post.mockResolvedValue(response('search', wrong));
  await expect(service.search(body, catalogue)).rejects.toMatchObject({
    code: 'invalid_response',
  });
  const selector = {
    kind: 'evidence' as const,
    evidence_ref: evidence().evidence_ref,
  };
  post.mockResolvedValue(response('read', evidence()));
  await expect(
    service.read(
      { contract_version: KnowledgeAccessVersion, selector },
      catalogue,
    ),
  ).resolves.toEqual(evidence());
});

it('read-only search and out-of-catalogue selectors never dispatch', async () => {
  await expect(
    service.search(
      {
        contract_version: KnowledgeAccessVersion,
        view_id: view().view_id,
        query: 'A',
        limit: 20,
      },
      view(),
    ),
  ).rejects.toMatchObject({ status: 400, outcomeKnown: true });
  const first = example('EX04');
  first.selector.target_index = 1;
  await expect(service.read(first, view())).rejects.toMatchObject({
    status: 400,
  });
  expect(post).not.toHaveBeenCalled();
});

it('keeps unknown commit and known CAS conflict distinct, including status validation', async () => {
  const body = {
    contract_version: KnowledgeAccessVersion,
    domain_id: Domain,
    expected_version: 0,
    enabled: true,
    confirmed: true as const,
    idempotency_key: 'synthetic-policy-key-0001',
  };
  post.mockRejectedValue(new Error('lost response'));
  await expect(service.setPolicy(body)).rejects.toMatchObject({
    outcomeKnown: null,
    retrySameKey: true,
  });
  const error = response('setPolicy', null, 409, {
    code: 'version_conflict',
    message: 'Knowledge access operation could not be completed',
    outcome_known: true,
    retry_same_key: false,
  });
  post.mockRejectedValue({ response: error });
  await expect(service.setPolicy(body)).rejects.toMatchObject({
    status: 409,
    outcomeKnown: true,
    retrySameKey: false,
  });
  error.status = 400;
  await expect(service.setPolicy(body)).rejects.toBeInstanceOf(
    AccessRequestError,
  );
});

it('accepts all nine S1 operations, including a 200 create-grant receipt replay after expiry', async () => {
  const module = admin();
  const setModule = { ...example('EX01'), name: module.name };
  const createGrant = {
    ...example('EX10'),
    subject: { kind: 'user' as const, id: User },
    delivery: 'web_only' as const,
  };
  const cases: [string, unknown, () => Promise<unknown>][] = [
    [
      'setPolicy',
      { domain_id: Domain, version: 1, enabled: true },
      () =>
        service.setPolicy({
          contract_version: KnowledgeAccessVersion,
          domain_id: Domain,
          expected_version: 0,
          enabled: true,
          confirmed: true,
          idempotency_key: 'synthetic-policy-key-0001',
        }),
    ],
    ['setModule', module, () => service.setModule(setModule)],
    [
      'getModuleAdmin',
      module,
      () =>
        service.getModuleAdmin({
          contract_version: KnowledgeAccessVersion,
          domain_id: Domain,
          module_id: ModuleId,
        }),
    ],
    [
      'listModules',
      {
        items: [],
        owned_domain_policies: [
          { domain_id: Domain, version: 0, enabled: false },
        ],
        pagination: 'none',
      },
      () =>
        service.listModules({
          contract_version: KnowledgeAccessVersion,
          tenant_id: Tenant,
          domain_id: Domain,
        }),
    ],
    ['createGrant', grant(), () => service.createGrant(createGrant)],
    [
      'revokeGrant',
      { id: grant().grant_id, revoked_at: '2030-01-01T00:00:01Z' },
      () =>
        service.revokeGrant({
          contract_version: KnowledgeAccessVersion,
          domain_id: Domain,
          grant_id: grant().grant_id,
          confirmed: true,
          idempotency_key: 'synthetic-revoke-key-0001',
        }),
    ],
    ['inspect', view(), () => service.inspect(inspectBody())],
    ['read', evidence(), () => service.read(example('EX04'), view())],
    [
      'search',
      searchResult(),
      () =>
        service.search(
          {
            contract_version: KnowledgeAccessVersion,
            view_id: view().view_id,
            query: '合成A',
            limit: 20,
          },
          { ...view(), mode: 'read_search' },
        ),
    ],
  ];
  for (const [operation, data, invoke] of cases) {
    post.mockResolvedValue(response(operation, data));
    const clock =
      operation === 'createGrant'
        ? jest
            .spyOn(Date, 'now')
            .mockReturnValue(Date.parse('2031-01-01T00:00:00Z'))
        : null;
    try {
      if (operation === 'createGrant')
        expect(Date.parse(createGrant.expires_at)).toBeLessThan(Date.now());
      await invoke();
    } finally {
      clock?.mockRestore();
    }
  }
  expect(post.mock.calls.map(([config]) => config.url)).toHaveLength(9);
  expect(
    post.mock.calls.every(([config]) =>
      config.url.startsWith('/api/v1/knowledge/access/'),
    ),
  ).toBe(true);
});

it('applies UTF-8 request byte limits and refuses lone Unicode surrogates before dispatch', async () => {
  await expect(
    service.search(
      {
        contract_version: KnowledgeAccessVersion,
        view_id: view().view_id,
        query: '合'.repeat(2000),
        limit: 20,
      },
      { ...view(), mode: 'read_search' },
    ),
  ).rejects.toMatchObject({ status: 400, outcomeKnown: true });
  const module = example('EX01');
  module.name = '\ud800';
  await expect(service.setModule(module)).rejects.toMatchObject({
    status: 400,
    outcomeKnown: true,
  });
  expect(post).not.toHaveBeenCalled();
});

it('converts an existing whole source revision to the approved exact access target without latest inheritance', () => {
  const scope = admin().scope;
  const source = scope.targets[0].source;
  const choice = accessPublicationChoice(
    Domain,
    {
      kind: 'fixed_representations',
      targets: [
        {
          source_ref: source,
          representation_ref: {
            kind: 'source_revision',
            id: null,
            producer_sha256: null,
          },
        },
      ],
    },
    {
      index_id: scope.publication.id,
      manifest_sha256: scope.publication.manifest_sha256,
    },
    { [source.source_id]: 'A.txt' },
  );
  expect(choice?.scope.targets[0].representation).toEqual({
    kind: 'source_revision',
    id: source.source_revision_id,
    producer_sha256: null,
  });
  expect(accessSchemaValid('ModuleScope', choice?.scope)).toBe(true);
  expect(accessTargetsValid(choice!.scope.targets, Domain)).toBe(true);
});

it('accepts multiple distinct search hits through the actual strict byte decoder', async () => {
  const catalogue = view();
  catalogue.mode = 'read_search';
  const result = searchResult(catalogue);
  const second = JSON.parse(JSON.stringify(result.hits[0]));
  second.evidence_ref.evidence_id = 'f'.repeat(64);
  second.rank_ordinal = 2;
  result.hits.push(second);
  result.returned_count = 2;
  post.mockImplementation(async (config) => {
    const reply = response('search', result);
    reply.data = config.transformResponse[0](
      new TextEncoder().encode(JSON.stringify(reply.data)),
    );
    return reply;
  });
  await expect(
    service.search(
      {
        contract_version: KnowledgeAccessVersion,
        view_id: catalogue.view_id,
        query: result.query,
        limit: result.requested_limit,
      },
      catalogue,
    ),
  ).resolves.toMatchObject({ returned_count: 2, hits: result.hits });
});

it('rejects duplicate decoded search hits as an invalid response', async () => {
  const catalogue = view();
  catalogue.mode = 'read_search';
  const result = searchResult(catalogue);
  result.hits.push(JSON.parse(JSON.stringify(result.hits[0])));
  result.returned_count = 2;
  post.mockImplementation(async (config) => {
    const reply = response('search', result);
    reply.data = config.transformResponse[0](
      new TextEncoder().encode(JSON.stringify(reply.data)),
    );
    return reply;
  });
  await expect(
    service.search(
      {
        contract_version: KnowledgeAccessVersion,
        view_id: catalogue.view_id,
        query: result.query,
        limit: result.requested_limit,
      },
      catalogue,
    ),
  ).rejects.toMatchObject({ code: 'invalid_response', status: 502 });
});

it('decodes special property names as inert JSON data and still rejects duplicates', () => {
  const value = parseAccessJSON(
    '{"__proto__":{"polluted":true},"constructor":{"prototype":{"polluted":true}},"valueOf":"inert"}',
  ) as Record<string, unknown>;
  expect(Object.hasOwn(value, '__proto__')).toBe(true);
  expect(value.__proto__).toEqual({ polluted: true });
  expect(({} as { polluted?: boolean }).polluted).toBeUndefined();
  expect(value.valueOf).toBe('inert');
  expect(() =>
    parseAccessJSON('{"__proto__":1,"\\u005f_proto__":2}'),
  ).toThrow();
});

const departmentRequest = () => ({
  contract_version: KnowledgeAccessVersion,
  tenant_id: Tenant,
  department_id: null,
  expected_version: 0,
  name: 'Synthetic department',
  enabled: true,
  user_ids: [User],
  confirmed: true as const,
  idempotency_key: 'synthetic-department-create-001',
});
const departmentData = () => ({
  department_id: 'a'.repeat(32),
  tenant_id: Tenant,
  name: 'Synthetic department',
  version: 1,
  enabled: true,
  member_count: 1,
  user_ids: [User],
});
it('dispatches the approved tenant-scoped department mutation and preserves its original receipt', async () => {
  const body = departmentRequest(),
    value = departmentData();
  post.mockResolvedValue(response('setDepartment', value));
  await expect(service.setDepartment(body)).resolves.toEqual(value);
  expect(post.mock.calls[0][0].url).toBe(
    '/api/v1/knowledge/access/admin/departments/set',
  );
  expect(JSON.parse(post.mock.calls[0][0].data)).toEqual(body);
});
it('accepts multiple department catalog entries through the actual strict byte decoder', async () => {
  const data = {
    items: [
      { ...departmentData(), member_count: null, user_ids: null },
      {
        ...departmentData(),
        department_id: 'b'.repeat(32),
        name: 'Synthetic department B',
        member_count: null,
        user_ids: null,
      },
    ],
    pagination: 'none',
  };
  post.mockImplementationOnce(({ transformResponse }: any) => {
    const reply = response('listDepartments', data);
    return Promise.resolve({
      ...reply,
      data: transformResponse[0](
        new TextEncoder().encode(JSON.stringify(reply.data)),
      ),
    });
  });
  await expect(
    service.listDepartments({
      contract_version: KnowledgeAccessVersion,
      tenant_id: Tenant,
    }),
  ).resolves.toEqual(data);
});
it.each([
  'foreign tenant',
  'wrong count',
  'partial null',
  'duplicate id',
  'disabled public catalog',
])('rejects department metadata with %s', async (problem) => {
  const item = departmentData(),
    items: any[] = [item];
  if (problem === 'foreign tenant') item.tenant_id = 'c'.repeat(32);
  if (problem === 'wrong count') item.member_count = 2;
  if (problem === 'partial null') (item as any).user_ids = null;
  if (problem === 'duplicate id')
    items.push({ ...item, name: 'Different name' });
  if (problem === 'disabled public catalog')
    Object.assign(item, { enabled: false, member_count: null, user_ids: null });
  post.mockResolvedValue(
    response('listDepartments', { items, pagination: 'none' }),
  );
  await expect(
    service.listDepartments({
      contract_version: KnowledgeAccessVersion,
      tenant_id: Tenant,
    }),
  ).rejects.toMatchObject({ status: 502 });
});
it('rejects invalid department creation/CAS and duplicate roster before transport', async () => {
  const body = departmentRequest();
  await expect(
    service.setDepartment({ ...body, expected_version: 1 }),
  ).rejects.toMatchObject({ status: 400 });
  await expect(
    service.setDepartment({ ...body, user_ids: [User, User] }),
  ).rejects.toMatchObject({ status: 400 });
  expect(post).not.toHaveBeenCalled();
});
it('creates a fixed department web grant with its explicitly selected delivery', async () => {
  const item = {
    ...grant(),
    subject: { kind: 'department' as const, id: 'a'.repeat(32) },
  };
  const body = {
    ...example('EX10'),
    subject: item.subject,
    domain_id: item.domain_id,
    module_id: item.module_id,
    expected_module_version: item.module_version,
    mode: item.mode,
    delivery: 'web_only' as const,
    expires_at: item.expires_at,
  };
  post.mockResolvedValue(response('createGrant', item, 201));
  await expect(service.createGrant(body)).resolves.toEqual(item);
  expect(JSON.parse(post.mock.calls[0][0].data).subject).toEqual(item.subject);
});

function credentialMetadata() {
  return {
    credential_id: '6'.repeat(32),
    domain_id: Domain,
    module_id: ModuleId,
    module_version: 1,
    mode: 'read' as const,
    client_label: 'Synthetic SDK',
    issued_at: '2030-01-01T00:00:00Z',
    expires_at: '2030-01-01T01:00:00Z',
    revoked_at: null,
    last_used_at: null,
  };
}
function credentialRequest() {
  return {
    contract_version: KnowledgeAccessVersion,
    view_id: view().view_id,
    mode: 'read' as const,
    ttl_seconds: null,
    client_label: 'Synthetic SDK',
    confirmed: true as const,
    idempotency_key: 'Synthetic_Credential_Request01',
  };
}
const syntheticCredentialToken = () =>
  'kac1_' + '6'.repeat(32) + '.' + 'A'.repeat(43);

it('allows explicitly selected MCP grant delivery and a separate native MCP inspect', async () => {
  const item = { ...grant(), delivery: 'web_and_mcp' as const };
  post.mockResolvedValueOnce(response('createGrant', item, 201));
  await service.createGrant({
    ...example('EX10'),
    subject: item.subject,
    mode: item.mode,
    expires_at: item.expires_at,
    delivery: item.delivery,
  });
  post.mockResolvedValueOnce(
    response('inspect', { ...view(), delivery: 'web_and_mcp' }),
  );
  await service.inspect({ ...inspectBody(), delivery: 'web_and_mcp' });
  expect(
    post.mock.calls.map((call) => JSON.parse(call[0].data).delivery),
  ).toEqual(['web_and_mcp', 'web_and_mcp']);
});
it('accepts first-response credentials and safe replay without recovering a token', async () => {
  const first = {
    credential: credentialMetadata(),
    token: syntheticCredentialToken(),
    delivery: 'issued_once',
  };
  post.mockResolvedValueOnce(response('createCredential', first, 201));
  await expect(service.createCredential(credentialRequest())).resolves.toEqual(
    first,
  );
  const replay = {
    ...first,
    token: null,
    delivery: 'unavailable_after_first_response',
  };
  post.mockResolvedValueOnce(response('createCredential', replay));
  await expect(service.createCredential(credentialRequest())).resolves.toEqual(
    replay,
  );
  expect(JSON.parse(post.mock.calls[0][0].data)).toEqual(
    JSON.parse(post.mock.calls[1][0].data),
  );
  expect(post.mock.calls[0][0].url).toBe(
    '/api/v1/knowledge/access/credentials/create',
  );
});
it.each([
  'token replay',
  'null first token',
  'wrong token ID',
  'overlong lifetime',
  'wrong label',
  'private metadata',
])('rejects unsafe credential output: %s', async (problem) => {
  const data: any = {
    credential: credentialMetadata(),
    token: syntheticCredentialToken(),
    delivery: 'issued_once',
  };
  let status = 201;
  if (problem === 'token replay') status = 200;
  if (problem === 'null first token') {
    data.token = null;
    data.delivery = 'unavailable_after_first_response';
  }
  if (problem === 'wrong token ID')
    data.token = 'kac1_' + '7'.repeat(32) + '.' + 'A'.repeat(43);
  if (problem === 'overlong lifetime')
    data.credential.expires_at = '2030-01-01T09:00:00Z';
  if (problem === 'wrong label') data.credential.client_label = 'Other client';
  if (problem === 'private metadata')
    data.credential.verifier_sha256 = 'Synthetic_Private_Verifier';
  post.mockResolvedValueOnce(response('createCredential', data, status));
  await expect(
    service.createCredential(credentialRequest()),
  ).rejects.toMatchObject({ status: 502, retrySameKey: true });
});
it('lists only own safe domain-bound metadata and revokes one exact credential', async () => {
  const item = credentialMetadata();
  post.mockResolvedValueOnce(
    response('listCredentials', { items: [item], pagination: 'none' }),
  );
  await expect(
    service.listCredentials({
      contract_version: KnowledgeAccessVersion,
      domain_id: Domain,
      idempotency_key: null,
    }),
  ).resolves.toEqual({ items: [item], pagination: 'none' });
  post.mockResolvedValueOnce(
    response('revokeCredential', {
      id: item.credential_id,
      revoked_at: '2030-01-01T00:30:00Z',
    }),
  );
  await service.revokeCredential({
    contract_version: KnowledgeAccessVersion,
    credential_id: item.credential_id,
    confirmed: true,
    idempotency_key: 'Synthetic_Credential_Revoke01',
  });
  expect(post.mock.calls[1][0].url).toBe(
    '/api/v1/knowledge/access/credentials/revoke',
  );
  expect(JSON.parse(post.mock.calls[1][0].data)).not.toHaveProperty(
    'domain_id',
  );
});
it.each(['foreign domain', 'duplicate ID', 'secret token'])(
  'rejects a credential metadata list containing %s',
  async (problem) => {
    const item: any = credentialMetadata(),
      items = [item];
    if (problem === 'foreign domain') item.domain_id = '8'.repeat(32);
    if (problem === 'duplicate ID')
      items.push({ ...item, client_label: 'Other label' });
    if (problem === 'secret token') item.token = syntheticCredentialToken();
    post.mockResolvedValueOnce(
      response('listCredentials', { items, pagination: 'none' }),
    );
    await expect(
      service.listCredentials({
        contract_version: KnowledgeAccessVersion,
        domain_id: Domain,
        idempotency_key: null,
      }),
    ).rejects.toMatchObject({ status: 502 });
  },
);
it('rejects invalid lifetime, consent and UTF8 client label length before mint transport', async () => {
  for (const patch of [
    { ttl_seconds: 59 },
    { ttl_seconds: 28801 },
    { confirmed: false },
    { client_label: '合'.repeat(43) },
  ])
    await expect(
      service.createCredential({ ...credentialRequest(), ...patch } as any),
    ).rejects.toMatchObject({ status: 400 });
  expect(post).not.toHaveBeenCalled();
});

import request from '@/utils/next-request';
import service, { LifecycleRequestError } from '../knowledge-lifecycle-service';
import {
  detail,
  receipt,
  command,
  Scope,
  Owner,
  Domain,
  Capabilities,
  response,
  failure,
} from '@/pages/dataset/knowledge-catalogue/__tests__/lifecycle-fixture';
type LifecycleTargetCheck =
  import('@/interfaces/database/knowledge-lifecycle').LifecycleTargetCheck;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Http = request as unknown as jest.Mock;
beforeEach(() => Http.mockReset());
it.each([
  'contract',
  'list',
  'detail',
  'target',
  'reclassify',
  'move',
  'archive',
  'restore',
  'byKey',
  'receipt',
])('frozen HTTP operation %s', async (name) => {
  const value = detail(),
    input = command(
      value,
      name === 'move'
        ? 'move'
        : name === 'reclassify'
          ? 'reclassify'
          : 'archive',
    ),
    result = await receipt(
      value,
      name === 'move'
        ? 'move'
        : name === 'reclassify'
          ? 'reclassify'
          : 'archive',
      input,
    ),
    base = '/api/v1/datasets/' + Domain + '/knowledge';
  const target: LifecycleTargetCheck = {
    domain_id: Domain,
    source_id: Scope.source,
    registration_id: value.registration_id,
    current_source_revision_id: input.expected_source_revision_id,
    lifecycle_version: input.expected_lifecycle_version,
    policy_version: input.expected_policy_version,
    relative_path: 'src/B.tsx',
    allowed: true,
    reason: null,
    reserved: false,
  };
  let work: () => Promise<unknown>,
    path: string,
    body: unknown = result,
    post = false;
  switch (name) {
    case 'contract':
      work = () => service.contract();
      path = '/api/v1/knowledge/source-lifecycle-contract';
      body = Capabilities;
      break;
    case 'list':
      work = () => service.list(Owner, Domain);
      path = base + '/source-lifecycles';
      body = { items: [value], next_cursor: null };
      break;
    case 'detail':
      work = () => service.detail(Scope);
      path = base + '/source-lifecycles/' + Scope.source;
      body = value;
      break;
    case 'target':
      work = () =>
        service.target(Scope, {
          relative_path: target.relative_path,
          expected_source_revision_id: input.expected_source_revision_id,
          expected_lifecycle_version: input.expected_lifecycle_version,
          expected_policy_version: input.expected_policy_version,
        });
      path = base + '/source-lifecycles/' + Scope.source + '/move-target';
      body = target;
      break;
    case 'byKey':
      work = () => service.byKey(Scope, 'archive', input);
      path = base + '/lifecycle-commands/by-key';
      break;
    case 'receipt':
      work = () => service.receipt(Scope, result.command_id);
      path = base + '/lifecycle-commands/' + result.command_id;
      break;
    default:
      post = true;
      path =
        base +
        '/source-lifecycles/' +
        Scope.source +
        '/' +
        (
          {
            reclassify: 'reclassifications',
            move: 'moves',
            archive: 'archives',
            restore: 'restores',
          } as Record<string, string>
        )[name];
      if (name === 'reclassify' && 'classification' in input)
        work = () => service.reclassify(Scope, input);
      else if (name === 'move' && 'relative_path' in input)
        work = () => service.move(Scope, input);
      else if (name === 'archive') work = () => service.archive(Scope, input);
      else {
        const restore = command(value, 'restore');
        body = await receipt(value, 'restore', restore);
        work = () => service.restore(Scope, restore);
      }
      break;
  }
  Http.mockResolvedValue(response(body, post ? 201 : 200));
  await work();
  const config = Http.mock.calls[0][0];
  expect(config.url).toBe(path);
  expect(config.method).toBe(post ? 'post' : 'get');
  expect(config.headers['X-Knowledge-Contract-Version']).toBe('1.0.0');
  expect(config.headers['X-Knowledge-Source-Contract-Version']).toBe('0.7.0');
  expect(config.timeout).toBe(25000);
  if (post)
    expect(JSON.parse(config.data)).toMatchObject({
      confirmed: true,
      expected_lifecycle_version: 5,
      expected_policy_version: 1,
    });
});
it('preserves empty language versus omission and exact group scope, page limit and original cursor', async () => {
  const value = detail();
  value.current.classification.language = '';
  value.current.classification.group_id = '3'.repeat(32);
  Http.mockResolvedValue(response({ items: [value], next_cursor: null }));
  const query = {
    state: 'active' as const,
    group_scope: 'exact' as const,
    group_id: '3'.repeat(32),
    language: '',
    limit: 1,
    cursor: '4'.repeat(32),
  };
  await service.list(Owner, Domain, query);
  expect(Http.mock.calls[0][0].params).toEqual(query);
  Http.mockResolvedValue(response({ items: [], next_cursor: null }));
  await service.list(Owner, Domain);
  expect(Http.mock.calls[1][0].params).not.toHaveProperty('language');
  await expect(
    service.list(Owner, Domain, {
      group_scope: 'all',
      group_id: '3'.repeat(32),
    }),
  ).rejects.toMatchObject({ status: 400 });
});
it('rejects crossed owner/domain/source, contradictory results, unknown fields and unsafe integer versions', async () => {
  const value = detail();
  for (const bad of [
    { ...value, owner_id: '3'.repeat(32) },
    { ...value, domain_id: '3'.repeat(32) },
    { ...value, source_id: '3'.repeat(32) },
    { ...value, extra: true },
    {
      ...value,
      current: {
        ...value.current,
        lifecycle_version: Number.MAX_SAFE_INTEGER + 1,
      },
    },
  ]) {
    Http.mockResolvedValue(response(bad));
    await expect(service.detail(Scope)).rejects.toBeInstanceOf(
      LifecycleRequestError,
    );
  }
  const saved = await receipt();
  Http.mockResolvedValue(
    response({ ...saved, request_sha256: '0'.repeat(64) }),
  );
  await expect(
    service.byKey(Scope, 'archive', saved.request),
  ).rejects.toThrow();
  Http.mockResolvedValue(
    response({ ...saved, after: { ...saved.after, lifecycle_version: 99 } }),
  );
  await expect(service.receipt(Scope, saved.command_id)).rejects.toThrow();
});
it('rejects JSON over 32KiB, wrong versions, unexpected status and missing no-store', async () => {
  for (const bad of [
    { ...response(detail()), status: 201 },
    { ...response(detail()), headers: {} },
    {
      ...response(detail()),
      data: { ...response(detail()).data, source_contract_version: '0.6.0' },
    },
    {
      ...response(detail()),
      data: { ...response(detail()).data, message: '中'.repeat(12000) },
    },
  ]) {
    Http.mockResolvedValue(bad);
    await expect(service.detail(Scope)).rejects.toThrow();
  }
});
it('distinguishes known errors from unknown timeouts and validates HTTP/error mapping', async () => {
  Http.mockRejectedValue(failure(409, 'revision_changed', true));
  await expect(service.detail(Scope)).rejects.toMatchObject({
    status: 409,
    outcomeKnown: true,
  });
  Http.mockRejectedValue(
    Object.assign(Error('timeout'), { code: 'ECONNABORTED' }),
  );
  await expect(service.detail(Scope)).rejects.toMatchObject({
    status: 0,
    outcomeKnown: false,
  });
  Http.mockRejectedValue(failure(503, 'revision_changed', true));
  await expect(service.detail(Scope)).rejects.toMatchObject({
    code: 'lifecycle_invalid_response',
  });
});
it('checks every page item and rejects wrong filtering, duplicate source order and nonadvancing cursors', async () => {
  const value = detail();
  for (const data of [
    { items: [{ ...value, owner_id: '3'.repeat(32) }], next_cursor: null },
    { items: [value, value], next_cursor: null },
    { items: [value], next_cursor: '3'.repeat(32) },
  ]) {
    Http.mockResolvedValue(response(data));
    await expect(
      service.list(Owner, Domain, { cursor: '3'.repeat(32) }),
    ).rejects.toThrow();
  }
  Http.mockResolvedValue(response({ items: [value], next_cursor: null }));
  await expect(
    service.list(Owner, Domain, { language: 'go' }),
  ).rejects.toThrow();
});
it('creates family-specific fixed links using the original digest and Web source input digest', async () => {
  const value = detail(),
    original = { ...value, representation_kind: 'original_only' as const };
  const link = await service.fixedLink(original);
  expect(link).toContain('original_sha=' + value.current.sha256);
  const web = await service.fixedLink({
    ...value,
    representation_kind: 'web_capture',
  });
  expect(web).toContain('web_input_sha=' + value.current.source_input_sha256);
  expect(web).not.toContain('web_input_sha=' + value.current.sha256);
});

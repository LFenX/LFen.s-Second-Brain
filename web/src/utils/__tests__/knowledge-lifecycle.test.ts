import Examples from '../../../../docs/knowledge-source-lifecycle-v1.examples.json';
import Contract from '../../../../docs/knowledge-source-lifecycle-v1.openapi.json';
import { encodeWebValue } from '../knowledge-web';
import {
  validLifecycleStructure,
  validLifecycleData,
  validLifecycleQuery,
  lifecycleLanguage,
  lifecyclePath,
  LifecycleKey,
} from '../knowledge-lifecycle-schema';
import {
  lifecycleDigest,
  lifecycleRequestDigest,
  lifecycleStorageKey,
  readLifecycleRecovery,
  saveLifecycleRecovery,
  checkLifecycleReceipt,
  equalLifecycle,
} from '../knowledge-lifecycle';
import {
  detail,
  receipt,
  recovery,
  Scope,
  Owner,
  Domain,
  command,
} from '@/pages/dataset/knowledge-catalogue/__tests__/lifecycle-fixture';
type Query =
  import('@/interfaces/request/knowledge-lifecycle').LifecycleListQuery;
beforeEach(() => localStorage.clear());
it.each(Examples.structure_cases)(
  'frozen schema: $name',
  ({ schema, value, valid }) =>
    expect(validLifecycleStructure(schema, value)).toBe(valid),
);
it.each(Examples.digest_vectors)(
  'frozen typed-LP: $name',
  async ({ value, sha256, encoded_hex }) => {
    expect(await lifecycleDigest(value)).toBe(sha256);
    expect(Buffer.from(encodeWebValue(value)).toString('hex')).toBe(
      encoded_hex,
    );
  },
);
it.each(Examples.query_cases)('frozen filters: $name', ({ query, valid }) =>
  expect(validLifecycleQuery(query as Query)).toBe(valid),
);
it.each(Examples.path_cases)(
  'exact path bytes: $name',
  ({ left, right, left_utf8, right_utf8, equal }) => {
    expect(Buffer.from(left).toString('hex')).toBe(left_utf8);
    expect(Buffer.from(right).toString('hex')).toBe(right_utf8);
    expect(left === right).toBe(equal);
    expect(lifecyclePath(left)).toBe(true);
    expect(lifecyclePath(right)).toBe(true);
  },
);
it.each(Object.entries(Contract['x-error-mapping']))(
  'frozen error mapping: %s',
  (code, rule) => {
    const value = {
      code,
      message: '',
      contract_version: '1.0.0',
      source_contract_version: '0.7.0',
      request_id: '1'.repeat(32),
      data: null,
      outcome_known: rule[1],
    };
    expect(validLifecycleData('Error', value, Number(rule[0]))).toBe(true);
    expect(
      validLifecycleData(
        'Error',
        { ...value, outcome_known: !rule[1] },
        Number(rule[0]),
      ),
    ).toBe(false);
  },
);
it('distinguishes empty language, omitted language and all group modes without normalizing input', () => {
  expect(validLifecycleQuery({ language: '' })).toBe(true);
  expect(validLifecycleQuery({})).toBe(true);
  expect(lifecycleLanguage(' go')).toBe(false);
  expect(lifecycleLanguage('\u0085go')).toBe(false);
  expect(lifecycleLanguage('go\u3000')).toBe(false);
  expect(lifecycleLanguage('\ud800')).toBe(false);
  for (const path of ['../a', '/a', 'a//b', 'a\\b', 'a\0b', 'a:b'])
    expect(lifecyclePath(path)).toBe(false);
  expect(LifecycleKey.test('A'.repeat(128))).toBe(true);
  expect(LifecycleKey.test('a:b')).toBe(false);
});
it('binds action, all expected values, target and owner into the digest, while excluding the original key', async () => {
  const value = detail(),
    request = command(value),
    sha = await lifecycleRequestDigest(Scope, 'archive', request);
  expect(
    await lifecycleRequestDigest(Scope, 'archive', {
      ...request,
      idempotency_key: 'different',
    }),
  ).toBe(sha);
  for (const changed of [
    { ...request, expected_lifecycle_version: 6 },
    { ...request, expected_policy_version: 2 },
    { ...request, expected_source_revision_id: '1'.repeat(32) },
  ])
    expect(await lifecycleRequestDigest(Scope, 'archive', changed)).not.toBe(
      sha,
    );
  expect(
    await lifecycleRequestDigest(
      { ...Scope, owner: '9'.repeat(32) },
      'archive',
      request,
    ),
  ).not.toBe(sha);
  expect(await lifecycleRequestDigest(Scope, 'restore', request)).not.toBe(sha);
});
it('validates a fixed result without replacing it with the current state; no-op keeps generation and body', async () => {
  const value = detail(),
    result = await receipt(value);
  expect(() =>
    checkLifecycleReceipt(result, Scope, 'archive', result.request),
  ).not.toThrow();
  expect(() =>
    checkLifecycleReceipt(
      { ...result, after: { ...result.after, lifecycle_version: 99 } },
      Scope,
    ),
  ).toThrow();
  expect(() =>
    checkLifecycleReceipt(
      { ...result, after: { ...result.after, sha256: '9'.repeat(64) } },
      Scope,
    ),
  ).toThrow();
  const noop = await receipt(value, 'restore');
  expect(noop.result).toBe('unchanged');
  expect(equalLifecycle(noop.before, noop.after)).toBe(true);
  expect(() => checkLifecycleReceipt(noop, Scope)).not.toThrow();
  expect(() =>
    checkLifecycleReceipt(
      {
        ...noop,
        result: 'applied',
        after: {
          ...noop.after,
          lifecycle_version: noop.before.lifecycle_version + 1,
        },
      },
      Scope,
    ),
  ).toThrow();
});
it('stores a complete bounded original and refuses unknown fields, conflicting same-domain requests and corrupt slots', async () => {
  const original = await recovery();
  await saveLifecycleRecovery(original);
  expect(readLifecycleRecovery(Owner, Domain)).toEqual(original);
  await expect(
    saveLifecycleRecovery({
      ...original,
      request: { ...original.request, idempotency_key: 'other' },
    }),
  ).rejects.toThrow();
  await expect(
    saveLifecycleRecovery({ ...original, body: 'forbidden' } as never),
  ).rejects.toThrow();
  localStorage.setItem(lifecycleStorageKey(Owner, Domain), 'broken');
  expect(() => readLifecycleRecovery(Owner, Domain)).toThrow();
});
it('allows eight unresolved domains, never evicts them, and only replaces a domain after a fixed receipt is saved', async () => {
  const original = await recovery();
  for (let n = 1; n <= 8; n++) {
    const domain = n.toString(16).padStart(32, '0');
    await saveLifecycleRecovery({ ...original, domain_id: domain });
  }
  await expect(saveLifecycleRecovery(original)).rejects.toThrow();
  expect(localStorage.length).toBe(8);
  const first = { ...original, domain_id: '1'.padStart(32, '0') };
  await saveLifecycleRecovery({ ...first, command_id: '2'.repeat(32) });
  await saveLifecycleRecovery(original);
  expect(localStorage.length).toBe(9);
  await saveLifecycleRecovery({ ...original, command_id: '2'.repeat(32) });
  await saveLifecycleRecovery({
    ...original,
    request: { ...original.request, idempotency_key: 'new' },
  });
});
it('serializes competing storage writers so only one same-domain original survives, and blocks before a failing storage write', async () => {
  const original = await recovery(),
    other = {
      ...original,
      request: { ...original.request, idempotency_key: 'second' },
    };
  const results = await Promise.allSettled([
    saveLifecycleRecovery(original),
    saveLifecycleRecovery(other),
  ]);
  expect(results.filter((item) => item.status === 'fulfilled')).toHaveLength(1);
  expect(readLifecycleRecovery(Owner, Domain)?.request.idempotency_key).toBe(
    original.request.idempotency_key,
  );
  const write = jest
    .spyOn(Storage.prototype, 'setItem')
    .mockImplementation(() => {
      throw Error('Quota');
    });
  await expect(
    saveLifecycleRecovery({ ...original, command_id: '7'.repeat(32) }),
  ).rejects.toThrow();
  write.mockRestore();
});
it('refuses oversized serialized slots without mutating stored records', async () => {
  const key = lifecycleStorageKey(Owner, Domain);
  localStorage.setItem(key, ' '.repeat(8193));
  expect(() => readLifecycleRecovery(Owner, Domain)).toThrow();
  expect(localStorage.getItem(key)).toHaveLength(8193);
});

import { createHash, webcrypto } from 'node:crypto';
import Examples from '../../../docs/knowledge-directory-import-v1.examples.json';
import {
  validDirectoryData,
  validDirectoryPath,
  compareDirectoryPaths,
} from './knowledge-directory-schema';
import {
  directoryStorageKey,
  saveDirectoryPending,
  loadDirectoryPending,
  sameDirectoryClassification,
  directoryObservationMatches,
  directoryPreviewMatches,
  directoryReceiptMatches,
  directoryWindowMatches,
  appendDirectoryPage,
} from './knowledge-directory';
import {
  copy,
  Registration,
  makeObservation,
  makePreview,
  makeReceipt,
  makeListing,
  windowFor,
  Text,
  Hash,
  ByteLength,
  digest,
} from '@/pages/dataset/knowledge-import/__tests__/directory-fixture';

beforeAll(() =>
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  }),
);
beforeEach(() => sessionStorage.clear());

describe('frozen 0.3.0 examples', () => {
  it.each(Examples.schema_cases)('$id', ({ schema, data, valid }) => {
    expect(validDirectoryData(schema, data)).toBe(valid);
  });
  it.each(Examples.semantic_cases)('$id', (item) => {
    let valid = validDirectoryData(
      item.schema,
      item.data,
      'http_status' in item ? item.http_status : undefined,
    );
    if (
      item.schema === 'Receipt' &&
      'expected_request' in item &&
      item.expected_request
    ) {
      const expected = item.expected_request;
      valid =
        valid &&
        directoryReceiptMatches(
          item.data as import('@/interfaces/database/knowledge-directory').DirectoryReceipt,
          {
            registrationId: expected.registration_id,
            request: {
              preview_id: expected.preview_id,
              manifest_sha256: expected.manifest_sha256,
              idempotency_key: 'synthetic-semantic',
            },
          },
        );
    }
    expect(valid).toBe(item.semantic_valid);
  });
  it.each(['observation', 'preview'] as const)(
    'checks the independent %s LP digest vector',
    (name) => {
      expect(digest(Examples.digest_vectors[name].tokens)).toBe(
        Examples.digest_vectors[name].sha256,
      );
    },
  );
  it('checks the exact BOM, CRLF, Chinese and emoji content vector', () => {
    expect(Buffer.byteLength(Text)).toBe(ByteLength);
    expect(createHash('sha256').update(Text, 'utf8').digest('hex')).toBe(Hash);
    expect(Text.startsWith('\uFEFF')).toBe(true);
    expect(Text).toContain('\r\n');
    expect(Text).toContain('🧠');
  });
});

it.each([
  '../a.ts',
  '/a.ts',
  'C:/a.ts',
  'a\\b.ts',
  'a//b',
  './a',
  'a/../b',
  'a/*',
  '.env.local',
  '.git/config',
  'node_modules/a.ts',
  'vendor/a.go',
  'build/a.ts',
  'a/NUL.txt',
  'a/COM1',
  'a/key.pem',
  'a ',
  'a.',
  'a/\0b',
  '\ud800',
])('rejects unsafe or excluded path %p', (path) => {
  expect(validDirectoryPath(path)).toBe(false);
});
it('counts UTF-8 bytes and path depth without changing case or Unicode', () => {
  expect(validDirectoryPath('中'.repeat(169) + '.tsx')).toBe(true);
  expect(validDirectoryPath('中'.repeat(170) + '.tsx')).toBe(false);
  expect(validDirectoryPath(Array(32).fill('a').join('/'))).toBe(true);
  expect(validDirectoryPath(Array(33).fill('a').join('/'))).toBe(false);
  expect(validDirectoryPath('.')).toBe(false);
  expect(validDirectoryPath('.', true)).toBe(true);
  expect(
    ['中.ts', 'A.ts', 'a.ts', '🧠.ts'].sort(compareDirectoryPaths),
  ).toEqual(['A.ts', 'a.ts', '中.ts', '🧠.ts']);
  expect(
    validDirectoryData('ListingRequest', {
      relative_directory: '.',
      name_prefix: '中'.repeat(43),
    }),
  ).toBe(false);
});
it('matches explicit classification fields regardless of JSON property order', () => {
  expect(
    sameDirectoryClassification(Registration.default_classification, {
      language: 'tsx',
      component_side: 'frontend',
      group_id: null,
    }),
  ).toBe(true);
  expect(
    sameDirectoryClassification(Registration.default_classification, {
      language: '',
      component_side: 'frontend',
      group_id: null,
    }),
  ).toBe(false);
});
it('binds observation, one-file preview, fixed content and receipt to the original context', () => {
  const observation = makeObservation(),
    preview = makePreview(observation),
    receipt = makeReceipt(preview);
  const input = {
    observation_id: observation.observation_id,
    observation_sha256: observation.observation_sha256,
    relative_path: 'web/card.tsx',
    classification: observation.default_classification,
  };
  const pending = {
    registrationId: preview.registration_id,
    request: {
      preview_id: preview.preview_id,
      manifest_sha256: preview.manifest_sha256,
      idempotency_key: 'synthetic-original',
    },
  };
  expect(
    directoryObservationMatches(
      observation,
      [...observation.selected_paths].reverse(),
    ),
  ).toBe(true);
  expect(directoryObservationMatches(observation, ['web/card.tsx'])).toBe(
    false,
  );
  expect(directoryPreviewMatches(preview, input, observation)).toBe(true);
  expect(
    directoryPreviewMatches(
      { ...preview, policy_version: 2 },
      input,
      observation,
    ),
  ).toBe(false);
  expect(
    directoryPreviewMatches(
      preview,
      { ...input, classification: { ...input.classification, language: 'go' } },
      observation,
    ),
  ).toBe(false);
  expect(
    directoryPreviewMatches(preview, input, {
      ...observation,
      complete: false,
    }),
  ).toBe(false);
  expect(directoryReceiptMatches(receipt, pending, preview)).toBe(true);
  expect(
    directoryReceiptMatches(
      { ...receipt, manifest_sha256: 'f'.repeat(64) },
      pending,
      preview,
    ),
  ).toBe(false);
  expect(directoryWindowMatches(windowFor(preview), preview, 0)).toBe(true);
  expect(
    directoryWindowMatches(
      { ...windowFor(preview), sha256: 'e'.repeat(64) },
      preview,
      0,
    ),
  ).toBe(false);
});
it('merges only pages of one immutable listing, preserving exact order and count', () => {
  const full = makeListing(51),
    first = { ...full, items: full.items.slice(0, 50), next_cursor: 'page-2' },
    next = { ...full, items: full.items.slice(50), next_cursor: null };
  expect(
    appendDirectoryPage([first], next).flatMap((value) => value.items),
  ).toEqual(full.items);
  expect(() =>
    appendDirectoryPage([first], { ...next, root_version: 2 }),
  ).toThrow();
  expect(() =>
    appendDirectoryPage([first], { ...next, items: [first.items[0]] }),
  ).toThrow();
  expect(() => appendDirectoryPage([first], { ...next, items: [] })).toThrow();
  expect(() =>
    appendDirectoryPage([], { ...first, next_cursor: null }),
  ).toThrow();
});
it('isolates recovery by domain and native user, falls back to token scope and stores only original identifiers', async () => {
  const key = await directoryStorageKey(
    Registration.domain_id,
    'synthetic-A',
    'owner',
  );
  const preview = makePreview(),
    pending = {
      registrationId: Registration.registration_id,
      request: {
        preview_id: preview.preview_id,
        manifest_sha256: preview.manifest_sha256,
        idempotency_key: 'synthetic-original',
      },
    };
  saveDirectoryPending(key, pending);
  expect(loadDirectoryPending(key)).toEqual(pending);
  expect(sessionStorage.getItem(key)).not.toMatch(
    /root_id|classification|text|synthetic-A|Authorization/,
  );
  expect(
    await directoryStorageKey(Registration.domain_id, 'synthetic-B', 'owner'),
  ).toBe(key);
  expect(
    await directoryStorageKey(Registration.domain_id, 'synthetic-B'),
  ).not.toBe(await directoryStorageKey(Registration.domain_id, 'synthetic-A'));
  for (const other of [
    await directoryStorageKey('d'.repeat(32), 'synthetic-A', 'owner'),
    await directoryStorageKey(Registration.domain_id, 'synthetic-A', 'other'),
  ]) {
    expect(other).not.toBe(key);
    expect(loadDirectoryPending(other)).toBeNull();
  }
  sessionStorage.setItem(
    key,
    JSON.stringify({ ...pending, text: 'must not persist' }),
  );
  expect(loadDirectoryPending(key)).toBeNull();
  expect(sessionStorage.getItem(key)).toBeNull();
  sessionStorage.setItem(key, '{invalid');
  expect(loadDirectoryPending(key)).toBeNull();
  expect(() =>
    saveDirectoryPending(key, { ...pending, registrationId: 'bad' }),
  ).toThrow();
});
it('rejects overlapping registration scope and expired snapshot intervals', () => {
  const request = copy<
    import('@/interfaces/request/knowledge-directory').DirectoryRegistrationRequest
  >('valid-RegistrationRequest');
  expect(
    validDirectoryData('RegistrationRequest', {
      ...request,
      scope_directories: ['web', 'web/src'],
    }),
  ).toBe(false);
  expect(
    validDirectoryData('RegistrationRequest', {
      ...request,
      scope_directories: ['.', 'web'],
    }),
  ).toBe(false);
  const preview = makePreview();
  expect(
    validDirectoryData('Preview', {
      ...preview,
      expires_at: preview.captured_at,
    }),
  ).toBe(false);
  expect(
    validDirectoryData('Preview', {
      ...preview,
      expires_at: new Date(
        Date.parse(preview.captured_at) + 600001,
      ).toISOString(),
    }),
  ).toBe(false);
});

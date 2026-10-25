import { webcrypto, createHash } from 'node:crypto';
import { TextEncoder, TextDecoder } from 'node:util';
import {
  readKnowledgeTextFile,
  knowledgeTextDigest,
  checkKnowledgePreview,
  checkKnowledgeReceipt,
  knowledgeImportStorageKey,
  saveKnowledgeConfirmation,
  loadKnowledgeConfirmation,
} from './knowledge-import';
import {
  knowledgeReceiptLink,
  knowledgeFixedTarget,
} from './knowledge-source-link';
import {
  PreviewRequest,
  makePreview,
  makeReceipt,
} from '@/pages/dataset/knowledge-import/__tests__/fixture';

beforeAll(() => {
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  });
  Object.assign(globalThis, { TextEncoder, TextDecoder });
});
beforeEach(() => sessionStorage.clear());

it.each(['', '\uFEFF// 合成\r\n你好 🧠\r\n', '中'.repeat(23347) + '\r\n'])(
  'preserves exact synthetic UTF-8 bytes, BOM, CRLF, emoji and empty content (%#)',
  async (text) => {
    const bytes = new TextEncoder().encode(text);
    const result = await readKnowledgeTextFile({
      size: bytes.length,
      arrayBuffer: async () => bytes.buffer,
    });
    expect(result.text).toBe(text);
    expect(result.byteLength).toBe(bytes.length);
    expect(result.sha256).toBe(
      createHash('sha256').update(bytes).digest('hex'),
    );
    expect(new TextEncoder().encode(result.text)).toEqual(bytes);
  },
);

it('rejects invalid UTF-8, NUL, unpaired surrogates and oversized original bytes without conversion', async () => {
  await expect(
    readKnowledgeTextFile({
      size: 1,
      arrayBuffer: async () => new Uint8Array([255]).buffer,
    }),
  ).rejects.toMatchObject({ status: 415 });
  await expect(knowledgeTextDigest('synthetic\0')).rejects.toMatchObject({
    status: 415,
  });
  await expect(knowledgeTextDigest('\ud800')).rejects.toMatchObject({
    status: 415,
  });
  await expect(knowledgeTextDigest('中'.repeat(699051))).rejects.toMatchObject({
    status: 413,
  });
  await expect(knowledgeTextDigest('a'.repeat(2097152))).resolves.toMatchObject(
    { byteLength: 2097152 },
  );
});

it('checks preview and receipt identities and classifications without generating source identities', async () => {
  const preview = makePreview();
  const digest = await knowledgeTextDigest(PreviewRequest.text);
  expect(() =>
    checkKnowledgePreview(preview, PreviewRequest, digest),
  ).not.toThrow();
  expect(() =>
    checkKnowledgePreview(
      preview,
      { ...PreviewRequest, component_side: 'cli' },
      digest,
    ),
  ).toThrow();
  expect(() =>
    checkKnowledgeReceipt(preview, makeReceipt(preview)),
  ).not.toThrow();
  expect(() =>
    checkKnowledgeReceipt(preview, {
      ...makeReceipt(preview),
      preview_id: 'f'.repeat(32),
    }),
  ).toThrow();
  const receipt = makeReceipt(preview);
  if (receipt.status !== 'succeeded') throw Error('fixture');
  const link = knowledgeReceiptLink(preview.domain_id, receipt.items[0]);
  expect(
    knowledgeFixedTarget(new URL(link, 'http://localhost').searchParams),
  ).toEqual({
    sourceId: receipt.items[0].source_id,
    revisionId: receipt.items[0].source_revision_id,
    rootNodeId: receipt.items[0].root_node_id,
    sha256: receipt.items[0].sha256,
  });
  expect(() =>
    knowledgeFixedTarget(new URLSearchParams('revision=bad')),
  ).toThrow();
});

it('persists only original confirmation identifiers and isolates domains and native sessions', async () => {
  const key = await knowledgeImportStorageKey(
    'a'.repeat(32),
    'synthetic-login-A',
  );
  expect(key).toBe(
    await knowledgeImportStorageKey('a'.repeat(32), 'synthetic-login-A'),
  );
  expect(key).not.toContain('synthetic-login-A');
  expect(
    await knowledgeImportStorageKey(
      'a'.repeat(32),
      'renewed-session',
      'native-owner',
    ),
  ).toBe(
    await knowledgeImportStorageKey(
      'a'.repeat(32),
      'previous-session',
      'native-owner',
    ),
  );
  expect(
    await knowledgeImportStorageKey(
      'a'.repeat(32),
      'renewed-session',
      'another-owner',
    ),
  ).not.toBe(
    await knowledgeImportStorageKey(
      'a'.repeat(32),
      'previous-session',
      'native-owner',
    ),
  );
  const value = {
    registrationId: '7'.repeat(32),
    request: {
      preview_id: '8'.repeat(32),
      manifest_sha256: 'a'.repeat(64),
      idempotency_key: 'original-key',
    },
  };
  saveKnowledgeConfirmation(key, value);
  expect(loadKnowledgeConfirmation(key)).toEqual(value);
  expect(
    loadKnowledgeConfirmation(
      await knowledgeImportStorageKey('a'.repeat(32), 'synthetic-login-B'),
    ),
  ).toBeNull();
  expect(
    loadKnowledgeConfirmation(
      await knowledgeImportStorageKey('b'.repeat(32), 'synthetic-login-A'),
    ),
  ).toBeNull();
  expect(JSON.parse(sessionStorage.getItem(key)!)).toEqual(value);
  expect(sessionStorage.getItem(key)).not.toMatch(
    /authorization|text|source_id|password/,
  );
});

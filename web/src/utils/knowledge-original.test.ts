import Examples from '../../../docs/knowledge-original-import-v1.examples.json';
import {
  originalDigest,
  latestOriginalUpload,
  originalScope,
  originalStorageKey,
  saveOriginalRecovery,
  loadOriginalRecovery,
  checkOriginalReceipt,
  verifyOriginalBlob,
} from './knowledge-original';
import {
  validOriginalData,
  validOriginalStructure,
} from './knowledge-original-schema';
import {
  DomainId,
  OwnerId,
  RegistrationId,
  Bytes,
  blob,
  makeUpload,
  makeRevision,
  makeReceipt,
  createRequest,
} from '@/pages/dataset/knowledge-import/__tests__/original-fixture';

beforeEach(() => localStorage.clear());
it.each(
  Examples.structure_cases.map(
    (entry) => [entry.name, entry.schema, entry.value, entry.valid] as const,
  ),
)('frozen structure: %s', (_name, schema, value, expected) => {
  expect(validOriginalStructure(schema, value)).toBe(expected);
});
it('rebuilds the frozen registration, upload and preview length-prefixed digests from independent vectors', async () => {
  for (const vector of Examples.digest_vectors) {
    expect(await originalDigest('registration', vector.registration)).toBe(
      vector.registration_digest.sha256,
    );
    expect(await originalDigest('request', vector.request)).toBe(
      vector.request_digest.sha256,
    );
    expect(await originalDigest('preview', vector.preview)).toBe(
      vector.preview_digest.sha256,
    );
  }
});
it('validates frozen schemas, exact names, families, paths and ready byte invariants', async () => {
  expect(validOriginalData('CreateUpload', createRequest())).toBe(true);
  for (const change of [
    { original_filename: '../bad.pdf' },
    { relative_path: '.env/local.pdf' },
    { relative_path: 'docs/a.docx' },
    { declared_media_type: 'application/pdf;charset=utf-8' },
    { declared_byte_length: 67108865 },
  ])
    expect(
      validOriginalData('CreateUpload', { ...createRequest(), ...change }),
    ).toBe(false);
  const ready = await makeUpload();
  expect(validOriginalData('Upload', ready)).toBe(true);
  expect(validOriginalData('Upload', { ...ready, received_bytes: 1 })).toBe(
    false,
  );
  expect(
    validOriginalData('Upload', { ...ready, source_id: 'e'.repeat(32) }),
  ).toBe(false);
  expect(
    validOriginalData('OriginalRevision', {
      ...makeRevision(ready),
      root_node_id: 'f'.repeat(64),
    }),
  ).toBe(false);
  expect(
    validOriginalData(
      'Error',
      {
        code: 'upload_busy',
        message: '',
        data: null,
        contract_version: '1.0.0',
        source_contract_version: '0.5.0',
        request_id: '1'.repeat(32),
      },
      503,
    ),
  ).toBe(false);
});
it('discards late lower revisions and rejects conflicting equal revisions while allowing progress to restart', async () => {
  const latest = await makeUpload('receiving', {
    revision: 4,
    received_bytes: 4,
  });
  expect(
    latestOriginalUpload(latest, { ...latest, revision: 3, received_bytes: 1 }),
  ).toBe(latest);
  expect(() =>
    latestOriginalUpload(latest, { ...latest, received_bytes: 3 }),
  ).toThrow();
  expect(
    latestOriginalUpload(latest, { ...latest, revision: 5, received_bytes: 0 }),
  ).toHaveProperty('received_bytes', 0);
  expect(() =>
    latestOriginalUpload(latest, {
      ...latest,
      request_sha256: 'a'.repeat(64),
      revision: 3,
    }),
  ).toThrow();
  const ready = await makeUpload('ready', { revision: 6 });
  expect(() =>
    latestOriginalUpload(ready, { ...ready, revision: 7, state: 'receiving' }),
  ).toThrow();
  expect(() =>
    latestOriginalUpload(
      { ...ready, state: 'cancelled' },
      { ...ready, revision: 7 },
    ),
  ).toThrow();
});
it('saves two bounded scope-checked slots and retains the complete original request tuples', () => {
  const key = originalStorageKey(DomainId, OwnerId),
    request = createRequest();
  const pending = {
    ...originalScope(DomainId, OwnerId),
    kind: 'upload' as const,
    registration_id: RegistrationId,
    request,
    request_sha256: 'a'.repeat(64),
    upload_id: null,
  };
  saveOriginalRecovery(key, 'create', pending);
  saveOriginalRecovery(key, 'confirm', {
    ...originalScope(DomainId, OwnerId),
    registration_id: RegistrationId,
    request: {
      upload_id: 'd'.repeat(32),
      manifest_sha256: 'b'.repeat(64),
      idempotency_key: 'original:confirm',
      confirmed: true,
    },
    import_task_id: null,
  });
  expect(localStorage.length).toBe(2);
  expect(loadOriginalRecovery(key, 'create', DomainId, OwnerId)).toEqual(
    pending,
  );
  expect(() =>
    loadOriginalRecovery(key, 'create', DomainId, '0'.repeat(32)),
  ).toThrow();
  expect(() =>
    saveOriginalRecovery(key, 'create', {
      ...pending,
      token: 'not-allowed',
    } as typeof pending),
  ).toThrow();
  expect(localStorage.getItem(key + ':create')).not.toMatch(
    /Authorization|blob:|arrayBuffer/,
  );
});
it('does not silently erase corrupt recovery metadata or submit from it', () => {
  const key = originalStorageKey(DomainId, OwnerId);
  localStorage.setItem(key + ':create', '{broken');
  expect(() =>
    loadOriginalRecovery(key, 'create', DomainId, OwnerId),
  ).toThrow();
  expect(localStorage.getItem(key + ':create')).toBe('{broken');
});
it('matches every confirmation identity field and checks complete raw Blob hashes without decoding', async () => {
  const upload = await makeUpload(),
    receipt = makeReceipt(upload);
  const request = {
    upload_id: upload.upload_id,
    manifest_sha256: upload.manifest_sha256!,
    idempotency_key: receipt.idempotency_key,
    confirmed: true as const,
  };
  expect(
    checkOriginalReceipt(receipt, DomainId, RegistrationId, request, upload),
  ).toBe(receipt);
  for (const name of [
    'upload_id',
    'idempotency_key',
    'manifest_sha256',
    'registration_id',
    'domain_id',
  ])
    expect(() =>
      checkOriginalReceipt(
        { ...receipt, [name]: 'bad' },
        DomainId,
        RegistrationId,
        request,
        upload,
      ),
    ).toThrow();
  const source = makeRevision(upload);
  await expect(verifyOriginalBlob(blob(), source)).resolves.toHaveProperty(
    'size',
    Bytes.length,
  );
  await expect(
    verifyOriginalBlob(blob(new Uint8Array(Bytes.length)), source),
  ).rejects.toThrow();
});

import Examples from '../../../../docs/knowledge-web-import-v1.examples.json';
import {
  validWebStructure,
  validWebData,
  webOriginAllowed,
} from '../knowledge-web-schema';
import {
  encodeWebValue,
  webDigest,
  webDigestEncoding,
  webSha256,
  latestWebResource,
  webCaptureAction,
  newWebSlot,
  writeWebSlot,
  readWebSlot,
  webStorageKey,
  checkWebPreview,
  checkWebReceipt,
  webSourceProjection,
  webStoredJsonBytes,
} from '../knowledge-web';
import {
  webReadingDocument,
  appendWebWindow,
  webWindowRange,
  verifiedWebText,
  webReferenceHref,
  webByteWindow,
} from '../knowledge-web-reading';
import {
  fixture,
  Scope,
  Raw,
  blob,
  Reading,
  DomainId,
  OwnerId,
} from '@/pages/dataset/knowledge-import/__tests__/web-fixture';
type DigestKind = import('../knowledge-web').WebDigestKind;
type WebWindow = import('@/interfaces/database/knowledge-web').WebWindow;

it.each(Examples.structure_cases)(
  'frozen structure: $name',
  ({ schema, value, valid }) =>
    expect(validWebStructure(schema, value)).toBe(valid),
);
it.each(Examples.digest_vectors)(
  'frozen digest and bytes: $name',
  async ({ kind, input, sha256, encoded_hex }) => {
    expect(await webDigest(kind as DigestKind, input)).toBe(sha256);
    expect(
      Buffer.from(webDigestEncoding(kind as DigestKind, input)).toString('hex'),
    ).toBe(encoded_hex);
  },
);
it.each(Examples.typed_encoding_vectors)(
  'typed-LP: $name',
  async ({ input, encoded_hex, sha256 }) => {
    const encoded = encodeWebValue(input);
    expect(Buffer.from(encoded).toString('hex')).toBe(encoded_hex);
    expect(await webSha256(new TextEncoder().encode(encoded))).toBe(sha256);
  },
);
it.each(Examples.fingerprint_vectors)(
  'fingerprint: $name',
  async ({
    kind,
    original,
    changed,
    expected_equal,
    original_sha256,
    changed_sha256,
  }) => {
    const before = await webDigest(kind as DigestKind, original),
      after = await webDigest(kind as DigestKind, changed);
    expect(before).toBe(original_sha256);
    expect(after).toBe(changed_sha256);
    expect(before === after).toBe(expected_equal);
  },
);
for (const item of Examples.semantic_cases.filter(
  (value) => value.kind === 'capture_action',
))
  it('capture rule: ' + item.name, () => {
    const input = item.input as {
      mode: string;
      state: string;
      action: string;
      has_attempt?: boolean;
    };
    expect(
      webCaptureAction(
        input.mode,
        input.state,
        input.action,
        input.has_attempt,
      ),
    ).toBe(item.expected);
  });
for (const item of Examples.semantic_cases.filter(
  (value) => value.kind === 'window',
))
  it('window rule: ' + item.name, () => {
    const input = item.input as { text: string; start: number; end: number };
    expect(webByteWindow(input.text, input.start, input.end) !== null).toBe(
      item.expected,
    );
  });
for (const item of Examples.semantic_cases.filter(
  (value) => value.kind === 'origin',
))
  it('origin rule: ' + item.name, () => {
    const input = item.input as { url: string; origins: string[] };
    expect(webOriginAllowed(input.url, input.origins)).toBe(item.expected);
  });
it('rejects noninteger, undefined and malformed Unicode typed values without normalization', () => {
  for (const value of [
    NaN,
    Infinity,
    1.5,
    undefined,
    '\ud800',
    { a: undefined },
  ])
    expect(() => encodeWebValue(value)).toThrow();
  expect(encodeWebValue(null)).not.toBe(encodeWebValue(''));
  expect(encodeWebValue('é')).not.toBe(encodeWebValue('é'));
});
it('accepts runtime_ready dynamically and applies semantic checks beyond the frozen structural examples', async () => {
  const f = await fixture();
  for (const ready of [true, false])
    expect(
      validWebData('Capabilities', { ...f.capabilities, runtime_ready: ready }),
    ).toBe(true);
  expect(validWebData('Capture', f.capture)).toBe(true);
  expect(validWebData('Entry', f.entry)).toBe(true);
  expect(validWebData('Preview', f.preview)).toBe(true);
  expect(validWebData('WebRevision', f.revision)).toBe(true);
  expect(validWebData('Capture', { ...f.capture, received_bytes: 0 })).toBe(
    false,
  );
});
it('isolates a strict original slot by owner and domain and retains complete bounded requests', async () => {
  const f = await fixture(),
    key = webStorageKey(DomainId, OwnerId),
    slot = newWebSlot(OwnerId, DomainId, Scope.registration, f.captureRequest);
  writeWebSlot(key, slot);
  expect(readWebSlot(key, DomainId, OwnerId)).toEqual(slot);
  expect(() => readWebSlot(key, DomainId, '8'.repeat(32))).toThrow();
  expect(() =>
    writeWebSlot(key, { ...slot, body: 'forbidden' } as never),
  ).toThrow();
  expect(() =>
    writeWebSlot(key, { ...slot, capture_create_key: 'different' }),
  ).toThrow();
  localStorage.removeItem(key);
});
it('discards lower revisions and rejects contradictions at an equal revision', () => {
  const current = { revision: 3, state: 'ready' };
  expect(latestWebResource(current, { revision: 2, state: 'fetching' })).toBe(
    current,
  );
  expect(() =>
    latestWebResource(current, { revision: 3, state: 'expired' }),
  ).toThrow();
  expect(
    latestWebResource(current, { revision: 4, state: 'expired' }).state,
  ).toBe('expired');
});
it('reprojects a changed classification and verifies immutable preview and receipt including metadata and reference budgets', async () => {
  const f = await fixture();
  await checkWebPreview(
    f.preview,
    f.previewRequest,
    [f.previewItem],
    [f.entry],
    { [f.entry.entry_key]: f.references },
  );
  await checkWebReceipt(
    f.receipt,
    f.confirm,
    {
      ...f.preview,
      state: 'committed',
      import_task_id: f.receipt.import_task_id,
    },
    [f.receiptItem],
    [f.previewItem],
  );
  const changed = {
    ...f.previewRequest.selection[0].classification,
    language: 'go',
  };
  const input = await webDigest(
    'source_input',
    webSourceProjection(f.entry, changed),
  );
  expect(input).not.toBe(f.entry.source_input_sha256);
  const request = {
      ...f.previewRequest,
      selection: [
        { ...f.previewRequest.selection[0], classification: changed },
      ],
    },
    item = {
      ...f.previewItem,
      classification: changed,
      source_input_sha256: input,
    },
    preview = {
      ...f.preview,
      request_sha256: await webDigest('selection_request', request),
      selection_sha256: await webDigest('selection', request),
    };
  preview.manifest_sha256 = await webDigest('preview_manifest', {
    ...preview,
    items: [item],
  });
  await expect(
    checkWebPreview(preview, request, [item], [f.entry], {
      [f.entry.entry_key]: f.references,
    }),
  ).resolves.toBeUndefined();
  await expect(
    checkWebPreview(
      { ...preview, selected_byte_length: preview.selected_byte_length - 1 },
      request,
      [item],
      [f.entry],
      { [f.entry.entry_key]: f.references },
    ),
  ).rejects.toThrow();
});
it('verifies complete representations including empty bodies before producing an isolated document', async () => {
  const f = await fixture();
  expect(
    await verifiedWebText(
      blob(new TextEncoder().encode(Reading)),
      f.entry.representations[1],
    ),
  ).toBe(Reading);
  await expect(
    verifiedWebText(blob(Raw), f.entry.representations[1]),
  ).rejects.toThrow();
  const empty = {
    ...f.entry.representations[1],
    byte_length: 0,
    sha256: await webSha256(new Uint8Array()),
  };
  expect(await verifiedWebText(blob(new Uint8Array()), empty)).toBe('');
  const doc = webReadingDocument(
    '<script>fetch("https://bad.test")</script><img src="https://bad.test"><iframe src="https://bad.test"></iframe><a href="https://bad.test">open</a><pre>&lt;x&gt;</pre>',
  );
  expect(doc.indexOf('Content-Security-Policy')).toBeLessThan(
    doc.indexOf('<body>'),
  );
  expect(doc).not.toMatch(/<script|<img|<iframe|href=|src=/);
  expect(doc).toContain("connect-src 'none'");
  expect(
    webReferenceHref({
      ...f.references[0],
      resolved_url: 'javascript:alert(1)',
    }),
  ).toBeNull();
});
it('keeps UTF-8 windows continuous, pinned and escaped; permits an empty representation', async () => {
  const f = await fixture(),
    text = '中'.repeat(2000),
    range = webWindowRange(text, 0);
  expect(range.end).toBe(4095);
  const descriptor = { ...f.entry.representations[0], byte_length: 6000 },
    value = {
      ...Examples.samples.window,
      representation: descriptor,
      start_byte: 0,
      end_byte: 4095,
      next_start_byte: 4095,
      text: '中'.repeat(1365),
    } as WebWindow;
  expect(appendWebWindow([], value, descriptor)).toHaveLength(1);
  expect(() => appendWebWindow([value], value, descriptor)).toThrow();
  expect(() =>
    appendWebWindow(
      [],
      { ...value, representation: { ...descriptor, sha256: '0'.repeat(64) } },
      descriptor,
    ),
  ).toThrow();
  const empty = { ...descriptor, byte_length: 0 };
  expect(
    appendWebWindow(
      [],
      {
        ...value,
        representation: empty,
        end_byte: 0,
        next_start_byte: null,
        text: '',
      },
      empty,
    ),
  ).toHaveLength(1);
  expect(webStoredJsonBytes('<')).toBe(8);
});
it('accepts an unchanged receipt that reuses a revision from an older capture, while retaining its head and source identity', async () => {
  const f = await fixture(),
    item = {
      ...f.previewItem,
      action: 'unchanged' as const,
      source_id: f.revision.source_id,
      expected_revision_id: f.revision.source_revision_id,
    },
    preview = {
      ...f.preview,
      state: 'committed' as const,
      import_task_id: f.receipt.import_task_id,
    },
    result = { ...f.receipt, saved_count: 0, unchanged_count: 1 },
    saved = { ...f.receiptItem, action: 'unchanged' as const };
  const oldRevision = { ...f.revision, capture_id: '8'.repeat(32) };
  expect(oldRevision.capture_id).not.toBe(result.capture_id);
  await expect(
    checkWebReceipt(result, f.confirm, preview, [saved], [item]),
  ).resolves.toBeUndefined();
  await expect(
    checkWebReceipt(
      result,
      f.confirm,
      preview,
      [{ ...saved, source_revision_id: '7'.repeat(32) }],
      [item],
    ),
  ).rejects.toThrow();
});

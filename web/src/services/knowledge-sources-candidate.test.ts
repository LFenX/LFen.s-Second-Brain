import Ajv from 'ajv/dist/2020';
import Contract from '../../../docs/knowledge-sources-v1.openapi.json';
import Example from '../../../docs/knowledge-sources-v1.example.json';
import Trace from '../../../docs/stage2a-c2-http.json';
import request from '@/utils/next-request';
import service from './knowledge-foundation-service';
import {
  Content,
  Source,
  Root,
  envelope,
} from '@/pages/dataset/knowledge-catalogue/__tests__/fixture';

// Consumer preparation for C2 0.2.0 at 657ebd7. No C2 client, network or UI wiring.
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
const Http = request as unknown as jest.Mock;
const Validator = new Ajv({ strict: false, validateFormats: false });
const Schemas = Object.fromEntries(
  Object.keys(Contract.components.schemas).map((name) => [
    name,
    Validator.compile({
      components: Contract.components,
      $ref: `#/components/schemas/${name}`,
    }),
  ]),
);
const RegistrationId = '7'.repeat(32);
const PreviewId = '8'.repeat(32);
const TaskId = '9'.repeat(32);
const Manifest = 'a'.repeat(64);
const PreviewItem = {
  item_key: Source.relative_path,
  relative_path: Source.relative_path,
  group_id: Source.group_id,
  component_side: Source.component_side,
  language: Source.language,
  action: 'update',
  source_id: Source.source_id,
  expected_revision_id: Source.source_revision_id,
  sha256: Source.sha256,
  byte_length: Source.byte_length,
};
const Preview = {
  preview_id: PreviewId,
  registration_id: RegistrationId,
  domain_id: Source.domain_id,
  policy_version: 1,
  manifest_sha256: Manifest,
  expires_at: '2026-10-25T01:10:00Z',
  consumed_task_id: null,
  items: [PreviewItem],
};
const SavedItem = {
  item_key: Source.relative_path,
  relative_path: Source.relative_path,
  group_id: Source.group_id,
  component_side: Source.component_side,
  language: Source.language,
  save_status: 'saved',
  catalogue_status: 'ready',
  structure_status: 'deferred',
  full_text_status: 'disabled',
  vector_status: 'disabled',
  source_id: Source.source_id,
  source_revision_id: Source.source_revision_id,
  root_node_id: Source.root_node_id,
  sha256: Source.sha256,
  byte_length: Source.byte_length,
  code: 'ok',
};
const Receipt = {
  import_task_id: TaskId,
  registration_id: RegistrationId,
  domain_id: Source.domain_id,
  preview_id: PreviewId,
  manifest_sha256: Manifest,
  status: 'succeeded',
  model_calls: 0,
  created_at: '2026-10-25T01:01:00Z',
  items: [SavedItem],
};
const FailedItem = {
  ...SavedItem,
  save_status: 'failed',
  catalogue_status: 'failed',
  source_id: null,
  source_revision_id: null,
  root_node_id: null,
  code: 'knowledge_import_publish_failed',
};
const FailedReceipt = { ...Receipt, status: 'failed', items: [FailedItem] };
const c2Envelope = (data: unknown) => ({
  ...envelope(data),
  source_contract_version: '0.2.0',
});

beforeEach(() => Http.mockReset());

it("compiles all 13 candidate schemas and accepts A's three request examples", () => {
  expect(Contract.info.version).toBe('0.2.0');
  expect(Object.keys(Schemas)).toHaveLength(13);
  expect(Schemas.RegistrationRequest(Example.registration_request)).toBe(true);
  expect(Schemas.PreviewRequest(Example.preview_request)).toBe(true);
  expect(Schemas.CommitRequest(Example.commit_request)).toBe(true);
});

it('requires the candidate header on all seven operations without replacing C1', () => {
  expect(Object.keys(Contract.paths)).toHaveLength(7);
  for (const operations of Object.values(Contract.paths)) {
    for (const operation of Object.values(operations)) {
      expect(operation.parameters).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            name: 'X-Knowledge-Source-Contract-Version',
            required: true,
            schema: { type: 'string', enum: ['0.2.0'] },
          }),
          expect.objectContaining({
            name: 'X-Knowledge-Contract-Version',
            schema: { type: 'string', enum: ['1.0.0'] },
          }),
        ]),
      );
    }
  }
});

it.each(['frontend', 'backend', 'shared', 'cli', 'non_code'])(
  'accepts explicit %s classification, a nullable group and an empty language',
  (componentSide) => {
    expect(
      Schemas.PreviewRequest({
        ...Example.preview_request,
        component_side: componentSide,
        group_id: null,
        language: '',
      }),
    ).toBe(true);
  },
);

it('rejects missing explicit classification and client-controlled identities or processing', () => {
  for (const field of ['group_id', 'component_side', 'language']) {
    const input: Record<string, unknown> = { ...Example.preview_request };
    delete input[field];
    expect(Schemas.PreviewRequest(input)).toBe(false);
  }
  for (const extra of [
    { source_id: Source.source_id },
    { owner_id: 'synthetic-owner' },
    { external_models: true },
    { vector_status: 'ready' },
  ]) {
    expect(
      Schemas.PreviewRequest({ ...Example.preview_request, ...extra }),
    ).toBe(false);
  }
});

it('accepts one private preview with nullable first-import identities and a consumed task', () => {
  expect(Schemas.Preview(Preview)).toBe(true);
  expect(
    Schemas.Preview({
      ...Preview,
      items: [
        {
          ...PreviewItem,
          action: 'add',
          source_id: null,
          expected_revision_id: null,
        },
      ],
    }),
  ).toBe(true);
  expect(Schemas.Preview({ ...Preview, consumed_task_id: TaskId })).toBe(true);
  expect(Schemas.Preview({ ...Preview, text: 'synthetic private body' })).toBe(
    false,
  );
  expect(
    Schemas.Preview({ ...Preview, items: [PreviewItem, PreviewItem] }),
  ).toBe(false);
});

it('accepts a failed receipt with a success envelope on HTTP 201', () => {
  const response = { status: 201, data: c2Envelope(FailedReceipt) };
  expect(Schemas.ReceiptResponse(response.data)).toBe(true);
  expect(response.data.code).toBe(0);
  expect(response.data.data.status).toBe('failed');
  expect(response.data.data.items[0].source_id).toBeNull();
  expect(Schemas.ReceiptResponse(c2Envelope(Receipt))).toBe(true);
});

it('rejects failed receipts claiming published identities and success receipts claiming indexes', () => {
  expect(
    Schemas.Receipt({
      ...FailedReceipt,
      items: [{ ...FailedItem, source_id: Source.source_id }],
    }),
  ).toBe(false);
  expect(
    Schemas.Receipt({
      ...Receipt,
      items: [{ ...SavedItem, vector_status: 'ready' }],
    }),
  ).toBe(false);
  expect(Schemas.Receipt({ ...Receipt, status: 'running' })).toBe(false);
  expect(Schemas.Receipt({ ...Receipt, model_calls: 1 })).toBe(false);
  expect(
    Schemas.Receipt({
      ...Receipt,
      items: [{ ...SavedItem, save_status: 'unchanged' }],
    }),
  ).toBe(true);
});

it('supports both native 401 and versioned errors, without inventing precise 409 reasons', () => {
  expect(
    Schemas.NativeUnauthorized({ code: 401, message: 'synthetic', data: null }),
  ).toBe(true);
  expect(
    Schemas.Error({
      ...c2Envelope(null),
      code: 'knowledge_conflict',
      message:
        'Preview expired, consumed or no longer matches the current policy/manifest',
    }),
  ).toBe(true);
});

it('matches recorded C2 trace methods and statuses to the candidate, without claiming response-body replay', () => {
  const routes = Object.entries(Contract.paths).map(([path, operations]) => ({
    pattern: new RegExp(`^${path.replace(/\{[^}]+\}/g, '[^/]+')}$`),
    operations,
  }));
  let matched = 0;
  for (const entry of Trace) {
    const route = routes.find(({ pattern }) => pattern.test(entry.path));
    if (!route) continue;
    const operation = Object.entries(route.operations).find(
      ([method]) => method === entry.method.toLowerCase(),
    )?.[1];
    expect(operation).toBeDefined();
    expect(operation?.responses).toHaveProperty(String(entry.status));
    matched += 1;
  }
  expect(matched).toBeGreaterThan(0);
});

it('passes receipt identities directly to the existing C1 service with mocked transport', async () => {
  expect(Schemas.Receipt(Receipt)).toBe(true);
  const item = Receipt.items[0];
  Http.mockResolvedValueOnce({ status: 200, data: envelope(Source) });
  Http.mockResolvedValueOnce({ status: 200, data: envelope(Root) });
  Http.mockResolvedValueOnce({ status: 200, data: envelope(Content) });
  const source = await service.source(
    Receipt.domain_id,
    item.source_revision_id,
  );
  const node = await service.node(Receipt.domain_id, item.root_node_id);
  const content = await service.content(Receipt.domain_id, item.root_node_id, {
    start_byte: 0,
    end_byte: item.byte_length,
  });
  expect(source.source_id).toBe(item.source_id);
  expect(source.root_node_id).toBe(item.root_node_id);
  expect(node.source_revision_id).toBe(item.source_revision_id);
  expect(content.source_id).toBe(item.source_id);
  expect(content.source_revision_id).toBe(item.source_revision_id);
  expect(content.source_sha256).toBe(item.sha256);
  expect(Http.mock.calls.map(([config]) => config.url)).toEqual([
    `/api/v1/datasets/${Receipt.domain_id}/knowledge/sources/${item.source_revision_id}`,
    `/api/v1/datasets/${Receipt.domain_id}/knowledge/nodes/${item.root_node_id}`,
    `/api/v1/datasets/${Receipt.domain_id}/knowledge/nodes/${item.root_node_id}/content`,
  ]);
  for (const [config] of Http.mock.calls) {
    expect(config.headers).toEqual({ 'X-Knowledge-Contract-Version': '1.0.0' });
    expect(config.method).toBe('get');
  }
});

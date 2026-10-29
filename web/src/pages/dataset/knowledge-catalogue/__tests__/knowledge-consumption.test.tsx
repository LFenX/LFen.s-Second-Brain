import {
  KnowledgeArchitecture,
  KnowledgeCapabilityMatrix,
} from '@/pages/dataset/knowledge-catalogue/knowledge-capabilities';
import Zh from '@/locales/zh';
import En from '@/locales/en';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {
  MemoryRouter,
  useSearchParams,
  useLocation,
  useNavigate,
} from 'react-router';
import {
  QueryClient,
  QueryClientProvider,
  notifyManager,
} from '@tanstack/react-query';
import { webcrypto } from 'node:crypto';
import { TextDecoder, TextEncoder } from 'node:util';
import request from '@/utils/next-request';
import { getAuthorization } from '@/utils/authorization-util';
import service from '@/services/knowledge-consumption-service';
type ContentAddress =
  import('@/interfaces/database/knowledge-consumption').ContentAddress;
type ContentCapabilities =
  import('@/interfaces/database/knowledge-consumption').ContentCapabilities;
type ContentNode =
  import('@/interfaces/database/knowledge-consumption').ContentNode;
type ContentRepresentation =
  import('@/interfaces/database/knowledge-consumption').ContentRepresentation;
type ContentTarget =
  import('@/interfaces/database/knowledge-consumption').ContentTarget;
type ContentWindow =
  import('@/interfaces/database/knowledge-consumption').ContentWindow;
type KnowledgeSource =
  import('@/interfaces/database/knowledge-foundation').KnowledgeSource;
import { KnowledgeContentVersion } from '@/interfaces/database/knowledge-consumption';
import {
  checkContentWindow,
  contentBoundNode,
  contentDigest,
  contentFixedLink,
  contentFixedTarget,
  contentRoute,
  contentSchemaValid,
  contentSourceTarget,
  parseContentJSON,
  contentDocumentTarget,
  contentPageItems,
  contentParsedTargets,
  contentParsedPosition,
  contentTableText,
  contentEnableParseRequest,
  contentParsedTaskRequest,
  contentParsedFileFormat,
  contentParsedWorkflowRoute,
  checkContentTask,
  checkContentNode,
  contentWireBytes,
} from '@/utils/knowledge-consumption';
import {
  scopedKnowledgeContent,
  useContentParsedUpload,
} from '@/hooks/use-knowledge-consumption';
import { lifecycleDigest } from '@/utils/knowledge-lifecycle';
type ContentPolicy =
  import('@/interfaces/database/knowledge-consumption').ContentPolicy;
type ContentParsedSource =
  import('@/interfaces/database/knowledge-consumption').ContentParsedSource;
type ContentParsedTask =
  import('@/interfaces/database/knowledge-consumption').ContentParsedTask;
type ContentParsedTaskRequest =
  import('@/interfaces/request/knowledge-consumption').ContentParsedTaskRequest;
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import KnowledgeRepresentation from '@/pages/dataset/knowledge-catalogue/knowledge-representation';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'offline-synthetic-session'),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/ui/button', () => ({
  Button: ({
    variant: _variant,
    size: _size,
    ...props
  }: Record<string, unknown>) => {
    void _variant;
    void _size;
    return <button {...props} />;
  },
}));
jest.mock('@/pages/dataset/knowledge-import/batch-manifest', () => ({
  ProjectPicker: () => null,
}));
jest.mock('@/pages/dataset/knowledge-catalogue/document-prepare', () => ({
  DocumentOriginalList: () => null,
}));
jest.mock('@/hooks/use-knowledge-structure', () => ({
  useStructureHead: jest.fn(),
  useStructureView: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-document', () => ({
  useDocumentHead: jest.fn(),
  useDocumentRepresentation: jest.fn(),
}));
jest.mock('../local-lexical', () => ({
  __esModule: true,
  default: ({
    initialChoice,
    domain,
    owner,
  }: {
    initialChoice: unknown;
    domain: string;
    owner: string;
  }) => (
    <output data-testid="parsed-index-entry">
      {JSON.stringify({ initialChoice, domain, owner })}
    </output>
  ),
}));
const Http = request as unknown as jest.Mock;
const Auth = getAuthorization as jest.Mock;
const Domain = 'a'.repeat(32),
  Owner = 'b'.repeat(32),
  Revision = 'c'.repeat(32),
  NodeId = 'd'.repeat(64),
  SourceId = 'e'.repeat(32),
  RequestId = 'f'.repeat(32);
const Scope = { owner: Owner, domain: Domain },
  Text = '合成\r\nT';
let Sha = '';
let Target: ContentTarget,
  Address: ContentAddress,
  Node: ContentNode,
  Representation: ContentRepresentation,
  Window: ContentWindow,
  Source: KnowledgeSource;
const Capabilities: ContentCapabilities = {
  projection_version: KnowledgeContentVersion,
  owner_only: true,
  parser_registrations: [
    { file_type: 'pdf', base_result: 'structured' },
    { file_type: 'visual', base_result: 'image_payload' },
    { file_type: 'aural', base_result: 'configuration_only' },
  ],
  fixed_representation_kinds: [
    'source_revision',
    'structure_view',
    'document_representation',
    'parsed_representation',
  ],
  coordinates: ['source_utf8', 'derived_utf8', 'item_utf8'],
  parser_registration_is_extraction_proof: false,
  unbound_parser_items_publishable: false,
  max_content_bytes: 65536,
  max_page_items: 100,
  max_parsed_representations_page_items: 20,
  parsed_profiles: [
    {
      profile_id: 'knowledge-parsed-local/1',
      format: 'txt',
      producer_sha256: '2'.repeat(64),
      runtime_ready: true,
    },
  ],
  parsed_publication_runtime_ready: true,
  parsed_policy_default_enabled: false,
  max_parsed_original_bytes: 2097152,
};
function response(
  operation: string,
  data: unknown,
  status = 200,
  code?: string,
) {
  const body = {
    operation,
    contract_version: KnowledgeContentVersion,
    request_id: RequestId,
    data,
    ...(code
      ? { code, message: 'Knowledge content request could not be completed' }
      : {}),
  };
  return {
    status,
    data: parseContentJSON(JSON.stringify(body)),
    headers: {
      'x-knowledge-content-version': KnowledgeContentVersion,
      'x-request-id': RequestId,
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
    },
  };
}
beforeAll(async () => {
  notifyManager.setNotifyFunction((callback) => {
    act(callback);
  });
  Object.assign(globalThis, { TextEncoder, TextDecoder });
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  });
  Sha = await contentDigest(Text);
});
afterAll(() => {
  notifyManager.setNotifyFunction((callback) => {
    callback();
  });
});
beforeEach(() => {
  Auth.mockReturnValue('offline-synthetic-session');
  Target = {
    source_ref: {
      domain_id: Domain,
      source_id: SourceId,
      source_revision_id: Revision,
      source_sha256: Sha,
    },
    representation_ref: {
      kind: 'source_revision',
      id: Revision,
      producer_sha256: null,
    },
  };
  Address = {
    target: Target,
    node_ref: { kind: 'c1_node', id: NodeId, item_index: null },
  };
  Node = {
    ref: Address.node_ref,
    parent_ref: null,
    kind: 'file',
    name: 'synthetic.txt',
    name_truncated: false,
    ordinal: null,
    locator: {
      kind: 'source_utf8',
      source_utf8: { start_byte: 0, end_byte: 9, start_line: 1, end_line: 2 },
      document: null,
      item_index: null,
    },
    original_position: null,
    coordinate_precision: 'source_bytes',
    text: {
      coordinate_space: 'source_utf8',
      range: { start_byte: 0, end_byte: 9, sha256: null },
      own_range: null,
      status: 'available',
      reason: null,
      value: null,
    },
    attributes: {
      source: {
        logical_id: NodeId,
        logical_key: '/',
        structure_version: 'source-v1',
      },
      structure: null,
      document: null,
      parser: null,
    },
    attachment_ref: null,
  };
  Representation = {
    projection_version: KnowledgeContentVersion,
    binding: 'bound',
    source_ref: Target.source_ref,
    representation_ref: Target.representation_ref,
    root_ref: Address.node_ref,
    state: 'ready',
    reason: null,
    coordinate_space: 'source_utf8',
    node_count: null,
  };
  Window = {
    projection_version: KnowledgeContentVersion,
    address: Address,
    locator: Node.locator,
    coordinate_space: 'source_utf8',
    range: { start_byte: 0, end_byte: 9, sha256: Sha },
    text_status: 'available',
    text_reason: null,
    text: Text,
    complete: true,
    next_start_byte: null,
  };
  Source = {
    domain_id: Domain,
    source_id: SourceId,
    source_revision_id: Revision,
    native_file_id: '1'.repeat(32),
    native_commit_id: '2'.repeat(32),
    group_id: null,
    relative_path: 'synthetic.txt',
    component_side: 'non_code',
    language: 'text',
    sha256: Sha,
    byte_length: 9,
    line_count: 2,
    is_current: false,
    root_node_id: NodeId,
  };
  Http.mockReset().mockImplementation(
    async (config: { url: string; data?: string }) => {
      const operation = config.url.split('/').at(-1)!;
      if (operation === 'capabilities')
        return response(operation, Capabilities);
      if (operation === 'representation')
        return response(operation, Representation);
      if (operation === 'node')
        return response(operation, {
          projection_version: KnowledgeContentVersion,
          target: Target,
          node: Node,
        });
      if (operation === 'nodes') {
        const body = parseContentJSON(config.data!) as {
          query: { parent_ref: ContentNode['ref'] | null };
        };
        return response(operation, {
          projection_version: KnowledgeContentVersion,
          target: Target,
          parent_ref: body.query.parent_ref,
          items: body.query.parent_ref ? [] : [Node],
          next_cursor: null,
        });
      }
      if (operation === 'read') return response(operation, Window);
      throw Error('unexpected_offline_request');
    },
  );
});

it('uses explicit nullable keys and strict fixed namespaces', () => {
  expect(contentSchemaValid('Target', Target)).toBe(true);
  expect(contentSchemaValid('Address', Address)).toBe(true);
  expect(
    contentBoundNode(Target, { ...Address.node_ref, kind: 'r2_node' }),
  ).toBe(false);
  expect(
    contentSchemaValid('Address', {
      target: Target,
      node_ref: { kind: 'c1_node', id: NodeId },
    }),
  ).toBe(false);
  expect(
    contentFixedTarget(Domain, {
      ...Target,
      representation_ref: {
        kind: 'structure_view',
        id: NodeId,
        producer_sha256: null,
      },
    }),
  ).toBe(false);
});
it.each([
  '{"a":1,"a":2}',
  '{"a":1,"\\u0061":2}',
  '{"a":"\\ud800"}',
  '{"limit":1.0000000000000000000001}',
  '{"limit":1e999}',
  '{"n":9007199254740993}',
])('rejects unsafe raw JSON %s', (raw) =>
  expect(() => parseContentJSON(raw)).toThrow(),
);
it('accepts exact exponent integers and finite parser positions', () => {
  expect((parseContentJSON('{"limit":2e1}') as { limit: number }).limit).toBe(
    20,
  );
  expect(
    (
      parseContentJSON('{"positions":[[1.25,2.5]]}') as {
        positions: number[][];
      }
    ).positions[0][0],
  ).toBe(1.25);
});
it('keeps fixed URLs independent of current head and rejects duplicate query keys', () => {
  const link = contentFixedLink(Address),
    params = new URLSearchParams(link.split('?')[1]);
  expect(contentRoute(Domain, params)).toEqual(Address);
  params.append('content_node', NodeId);
  expect(() => contentRoute(Domain, params)).toThrow();
  expect(
    contentRoute(Domain, new URLSearchParams({ content: '1' })),
  ).toBeNull();
});
it('calls the five actual HTTP shapes with no query parameters or missing null keys', async () => {
  await service.capabilities(Scope);
  await service.representation(Scope, Target);
  await service.node(Scope, Address);
  await service.nodes(Scope, {
    target: Target,
    parent_ref: null,
    cursor: '',
    limit: 20,
  });
  await service.read(
    Scope,
    { address: Address, start_byte: 0, end_byte: null },
    Node,
  );
  expect(Http.mock.calls).toHaveLength(5);
  const bodies = Http.mock.calls
    .slice(1)
    .map(([config]) => parseContentJSON(config.data));
  expect(
    bodies.map((body) => Object.keys(body as Record<string, unknown>).sort()),
  ).toEqual([
    ['contract_version', 'target'],
    ['address', 'contract_version'],
    ['contract_version', 'query'],
    ['contract_version', 'read'],
  ]);
  expect((bodies[3] as { read: { end_byte: null } }).read.end_byte).toBeNull();
});
it('rejects mismatched identity even when the response schema is valid', async () => {
  Http.mockImplementation(async () =>
    response('representation', {
      ...Representation,
      source_ref: { ...Target.source_ref, source_id: '9'.repeat(32) },
    }),
  );
  await expect(service.representation(Scope, Target)).rejects.toMatchObject({
    status: 502,
  });
});
it('keeps native refusal distinct from empty results', async () => {
  Http.mockImplementation(async () => {
    throw { response: response('representation', null, 404, 'not_found') };
  });
  await expect(service.representation(Scope, Target)).rejects.toMatchObject({
    status: 404,
    code: 'not_found',
  });
});
it('verifies original UTF-8/CRLF and rejects wrong coordinates, digest and completeness', async () => {
  expect((await checkContentWindow(Address, Node, 0, null, Window)).text).toBe(
    Text,
  );
  await expect(
    checkContentWindow(Address, Node, 0, null, {
      ...Window,
      coordinate_space: 'derived_utf8',
    }),
  ).rejects.toThrow();
  await expect(
    checkContentWindow(Address, Node, 0, null, {
      ...Window,
      text: Text.replace('T', 'X'),
    }),
  ).rejects.toThrow();
  await expect(
    checkContentWindow(Address, Node, 0, null, {
      ...Window,
      complete: false,
      next_start_byte: 9,
    }),
  ).rejects.toThrow();
});
it('does not treat parser registration as runtime or extraction proof', async () => {
  const data = await service.capabilities(Scope);
  expect(data.parser_registration_is_extraction_proof).toBe(false);
  expect(data.unbound_parser_items_publishable).toBe(false);
  expect(
    data.parser_registrations.find((entry) => entry.file_type === 'aural')
      ?.base_result,
  ).toBe('configuration_only');
});
it('reads a selected source through unified metadata/node/read and clears text on refresh', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[
          '/dataset/knowledge-catalogue/' + Domain + '?content=1',
        ]}
      >
        <KnowledgeRepresentation scope={Scope} source={Source} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByText('knowledgeConsumption.read'));
  expect(
    await screen.findByText(Text, { normalizer: (value) => value }),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByText('knowledgeCatalogue.refresh'));
  await waitFor(() =>
    expect(
      screen.queryByText(Text, { normalizer: (value) => value }),
    ).not.toBeInTheDocument(),
  );
  expect(Http.mock.calls.some(([config]) => config.url.endsWith('/read'))).toBe(
    true,
  );
  expect(contentSourceTarget(Source)).toEqual(Target);
  view.unmount();
  client.clear();
});

function documentFixture() {
  Target = {
    source_ref: { ...Target.source_ref, source_sha256: '0'.repeat(64) },
    representation_ref: {
      kind: 'document_representation',
      id: '1'.repeat(64),
      producer_sha256: '2'.repeat(64),
    },
  };
  Address = {
    target: Target,
    node_ref: { kind: 'r2_node', id: NodeId, item_index: null },
  };
  Node = {
    ...Node,
    ref: Address.node_ref,
    kind: 'page',
    ordinal: 0,
    locator: {
      kind: 'document',
      source_utf8: null,
      document: { scheme: 'pdf_page_v1', page_number: 1 },
      item_index: null,
    },
    original_position: { kind: 'pdf_page', page_number: 1 },
    coordinate_precision: 'physical_page',
    text: {
      ...Node.text,
      coordinate_space: 'derived_utf8',
      range: { start_byte: 0, end_byte: 9, sha256: Sha },
    },
    attributes: {
      source: null,
      structure: null,
      parser: null,
      document: {
        empty: null,
        page: { page_number: 1, page_content_classification: 'unknown' },
        slide: null,
        heading: null,
        table: null,
        row: null,
        cell: null,
        list: null,
        list_item: null,
        media: null,
        passage: null,
        unsupported_block: null,
      },
    },
  };
  Representation = {
    ...Representation,
    source_ref: Target.source_ref,
    representation_ref: Target.representation_ref,
    root_ref: Address.node_ref,
    coordinate_space: 'derived_utf8',
    node_count: 1,
  };
  Window = {
    ...Window,
    address: Address,
    locator: Node.locator,
    coordinate_space: 'derived_utf8',
  };
}
it('reads R1 using the actual fixed view and producer without changing source bytes', async () => {
  Target = {
    ...Target,
    representation_ref: {
      kind: 'structure_view',
      id: '1'.repeat(64),
      producer_sha256: '2'.repeat(64),
    },
  };
  Address = {
    target: Target,
    node_ref: { kind: 'r1_node', id: NodeId, item_index: null },
  };
  Node = {
    ...Node,
    ref: Address.node_ref,
    attributes: {
      source: null,
      document: null,
      parser: null,
      structure: {
        logical_node_id: NodeId,
        logical_key: '/',
        identity_stability: 'stable',
        parser: 'text',
        classification: {
          group_id: null,
          component_side: 'non_code',
          language: 'text',
        },
      },
    },
  };
  Window = { ...Window, address: Address };
  expect((await service.node(Scope, Address)).node.ref.kind).toBe('r1_node');
  expect(
    (
      await service.read(
        Scope,
        { address: Address, start_byte: 0, end_byte: null },
        Node,
      )
    ).text,
  ).toBe(Text);
});
it('reads R2 derived bytes independently of the binary original SHA and keeps the physical page', async () => {
  documentFixture();
  expect((await service.representation(Scope, Target)).coordinate_space).toBe(
    'derived_utf8',
  );
  expect((await service.node(Scope, Address)).node.original_position).toEqual({
    kind: 'pdf_page',
    page_number: 1,
  });
  expect(
    (
      await service.read(
        Scope,
        { address: Address, start_byte: 0, end_byte: null },
        Node,
      )
    ).range.sha256,
  ).toBe(Sha);
  expect(Target.source_ref.source_sha256).not.toBe(Sha);
  Node = { ...Node, original_position: { kind: 'pdf_page', page_number: 2 } };
  await expect(service.node(Scope, Address)).rejects.toMatchObject({
    status: 502,
  });
});
it('refuses an R2 head from another source revision and never substitutes its producer', () => {
  documentFixture();
  const original = {
    domain_id: Domain,
    source_id: SourceId,
    source_revision_id: Revision,
    sha256: Target.source_ref.source_sha256,
  };
  const head = {
    domain_id: Domain,
    source_id: SourceId,
    active_source_revision_id: Revision,
    active_representation_id: Target.representation_ref.id,
  };
  const representation = {
    domain_id: Domain,
    representation_id: Target.representation_ref.id!,
    producer_sha256: Target.representation_ref.producer_sha256!,
    original: {
      ...original,
      registration_id: '1'.repeat(32),
      byte_length: 50,
      media_type: 'application/pdf' as const,
      original_filename: 'synthetic.pdf',
      representation_kind: 'original_only' as const,
      root_node_id: null,
      text_available: false as const,
    },
  };
  expect(contentDocumentTarget(original, head, representation)).toEqual(Target);
  expect(() =>
    contentDocumentTarget(
      original,
      { ...head, active_source_revision_id: '9'.repeat(32) },
      representation,
    ),
  ).toThrow('content_original_head_mismatch');
  expect(() =>
    contentDocumentTarget(original, head, {
      ...representation,
      producer_sha256: '',
    }),
  ).toThrow('content_invalid_target');
});
it.each(['x-knowledge-content-version', 'x-request-id', 'content-type'])(
  'refuses inconsistent %s headers',
  async (header) => {
    const value = response('representation', Representation);
    Http.mockResolvedValue({
      ...value,
      headers: { ...value.headers, [header]: 'invalid' },
    });
    await expect(service.representation(Scope, Target)).rejects.toMatchObject({
      status: 502,
    });
  },
);
it.each([
  ['invalid_request', 400],
  ['unauthenticated', 401],
  ['not_found', 404],
  ['content_integrity', 409],
  ['contract_mismatch', 412],
  ['capacity_exceeded', 413],
  ['unsupported', 415],
  ['unavailable', 503],
] as const)(
  'keeps %s native status and refuses wrong HTTP mapping',
  async (code, status) => {
    Http.mockResolvedValue(response('representation', null, status, code));
    await expect(service.representation(Scope, Target)).rejects.toMatchObject({
      status,
      code,
    });
    Http.mockResolvedValue(response('representation', null, 200, code));
    await expect(service.representation(Scope, Target)).rejects.toMatchObject({
      status: 502,
    });
  },
);
it('rejects malformed raw transport data through the production response transform', async () => {
  Http.mockImplementation(async (config) => {
    config.transformResponse[0](
      new TextEncoder().encode('{"data":null,"data":{}}'),
    );
  });
  await expect(service.capabilities(Scope)).rejects.toMatchObject({
    status: 502,
  });
});
it('decodes actual transport bytes with fatal UTF-8 and the response byte limit', async () => {
  Http.mockImplementation(async (config) => {
    expect(config.responseType).toBe('arraybuffer');
    const value = response('capabilities', Capabilities);
    return {
      ...value,
      data: config.transformResponse[0](
        new TextEncoder().encode(JSON.stringify(value.data)),
      ),
    };
  });
  expect((await service.capabilities(Scope)).projection_version).toBe(
    KnowledgeContentVersion,
  );
  for (const bytes of [new Uint8Array([0xff]), new Uint8Array(1048577)]) {
    Http.mockImplementation(async (config) => {
      config.transformResponse[0](bytes);
    });
    await expect(service.capabilities(Scope)).rejects.toMatchObject({
      status: 502,
    });
  }
});
it('rejects nonprogressing windows and repeated pagination nodes or cursors', async () => {
  const sha = await contentDigest('');
  await expect(
    checkContentWindow(Address, Node, 0, null, {
      ...Window,
      text: '',
      range: { start_byte: 0, end_byte: 0, sha256: sha },
      complete: false,
      next_start_byte: 0,
    }),
  ).rejects.toThrow();
  const page = {
    projection_version: KnowledgeContentVersion,
    target: Target,
    parent_ref: null,
    items: [Node],
    next_cursor: 'opaque-cursor',
  };
  expect(() =>
    contentPageItems([page, { ...page, next_cursor: null }]),
  ).toThrow('content_repeated_node');
  expect(() =>
    contentPageItems([
      { ...page, items: [] },
      { ...page, items: [] },
    ]),
  ).toThrow('content_repeated_cursor');
});
it('rejects a pending success after the native session changes', async () => {
  const session = getKnowledgeSessionScope();
  let finish: (value: string) => void = () => {
    throw Error('uninitialized');
  };
  const pending = scopedKnowledgeContent(
    session,
    () =>
      new Promise<string>((resolve) => {
        finish = resolve;
      }),
  );
  Auth.mockReturnValue('different-offline-session');
  finish('old-owner-result');
  await expect(pending).rejects.toMatchObject({ status: 401 });
});
it('clears the old reading when the source or revision selection changes', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const tree = (source: KnowledgeSource | null) => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['?content=1']}>
        <KnowledgeRepresentation scope={Scope} source={source} />
      </MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(tree(Source));
  fireEvent.click(await screen.findByText('knowledgeConsumption.read'));
  expect(
    await screen.findByText(Text, { normalizer: (value) => value }),
  ).toBeInTheDocument();
  await act(async () => {
    view.rerender(tree({ ...Source, source_revision_id: '9'.repeat(32) }));
  });
  expect(
    screen.queryByText(Text, { normalizer: (value) => value }),
  ).not.toBeInTheDocument();
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  view.rerender(tree(null));
  expect(
    screen.queryByText('knowledgeConsumption.read'),
  ).not.toBeInTheDocument();
  view.unmount();
  client.clear();
});
function ChangeFixedNode({ address }: { address: ContentAddress }) {
  const [, setParams] = useSearchParams();
  const open = () =>
    setParams(new URLSearchParams(contentFixedLink(address).split('?')[1]));
  return <button onClick={open}>next-fixed-node</button>;
}
it('opens an R2 fixed URL without following a current head and resets when its node changes', async () => {
  documentFixture();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const next: ContentAddress = {
    ...Address,
    node_ref: { ...Address.node_ref, id: '9'.repeat(64) },
  };
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[contentFixedLink(Address)]}>
        <KnowledgeRepresentation scope={Scope} source={null} />
        <ChangeFixedNode address={next} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByText('knowledgeConsumption.read'));
  expect(
    await screen.findByText(Text, { normalizer: (value) => value }),
  ).toBeInTheDocument();
  Node = { ...Node, ref: next.node_ref, name: 'next-page' };
  Address = next;
  Window = { ...Window, address: next };
  fireEvent.click(screen.getByText('next-fixed-node'));
  expect(
    screen.queryByText(Text, { normalizer: (value) => value }),
  ).not.toBeInTheDocument();
  expect(
    await screen.findByText('next-page · knowledgeConsumption.kind_page'),
  ).toBeInTheDocument();
  expect(
    Http.mock.calls.every(([config]) =>
      config.url.includes('/knowledge/content/'),
    ),
  ).toBe(true);
  view.unmount();
  client.clear();
});

describe('architecture and capability visibility', () => {
  const translate =
    (words: Record<string, string>) =>
    (key: string, params?: Record<string, unknown>) =>
      (words[key.split('.').at(-1)!] ?? key).replace(
        /\{\{(\w+)\}\}/g,
        (match, name: string) =>
          params?.[name] === undefined ? match : String(params[name]),
      );
  it.each([
    ['zh', Zh.translation.knowledgeConsumption],
    ['en', En.translation.knowledgeConsumption],
  ] as const)(
    'shows all seven architecture stages and four separate states in %s',
    (_language, dictionary) => {
      const words = dictionary as unknown as Record<string, string>;
      const spy = jest
        .spyOn(require('react-i18next'), 'useTranslation')
        .mockReturnValue({ t: translate(words) });
      try {
        render(
          <KnowledgeArchitecture
            capabilities={Capabilities}
            lexicalEnabled={false}
          />,
        );
        expect(
          screen.getByRole('list', { name: words.archFlow }).children,
        ).toHaveLength(7);
        for (const stage of [
          'source',
          'revision',
          'representation',
          'node',
          'chunk',
          'index',
          'evidence',
        ])
          expect(
            screen.getAllByText(words['archStage_' + stage], { exact: false })
              .length,
          ).toBeGreaterThan(0);
        for (const kind of ['save', 'parse', 'index', 'search'])
          expect(
            screen.getByText(words['archIndependent_' + kind]),
          ).toBeInTheDocument();
        expect(screen.getByText(words.archCoordinates)).toBeInTheDocument();
        expect(screen.getByText(words.archIncomplete)).toBeInTheDocument();
        expect(screen.getByText(words.archIndexDisabled)).toBeInTheDocument();
        expect(
          screen.getByText(words.archTitle).closest('details'),
        ).toHaveAttribute('open');
      } finally {
        spy.mockRestore();
      }
    },
  );
  const sample: ContentCapabilities = {
    ...Capabilities,
    parser_registrations: [
      ...Capabilities.parser_registrations,
      { file_type: 'email', base_result: 'structured' },
      { file_type: 'md', base_result: 'structured' },
      { file_type: 'pptx', base_result: 'structured' },
      { file_type: 'csv', base_result: 'structured' },
    ],
  };
  it.each([
    ['zh', Zh.translation.knowledgeConsumption],
    ['en', En.translation.knowledgeConsumption],
  ] as const)(
    'shows families, public extension mappings and every independent state in %s',
    (_language, dictionary) => {
      const words = dictionary as unknown as Record<string, string>,
        t = translate(words);
      const spy = jest
        .spyOn(require('react-i18next'), 'useTranslation')
        .mockReturnValue({ t });
      try {
        render(
          <KnowledgeCapabilityMatrix
            capabilities={sample}
            lexicalEnabled={true}
          />,
        );
        const table = screen.getByRole('table', { name: words.matrixTitle });
        expect(within(table).getAllByRole('columnheader')).toHaveLength(7);
        expect(within(table).getAllByRole('row')).toHaveLength(8);
        expect(
          screen.getByText(
            t('knowledgeConsumption.registrationCount', {
              count: 7,
              structures: words.registrationPending,
            }),
          ),
        ).toBeInTheDocument();
        expect(screen.getByText(words.family_email)).toBeInTheDocument();
        expect(screen.getByText('.eml .msg')).toBeInTheDocument();
        expect(screen.getByText(words.family_visual)).toBeInTheDocument();
        expect(screen.getByText(words.family_aural)).toBeInTheDocument();
        expect(
          screen.getByText(words.output_configuration),
        ).toBeInTheDocument();
        expect(screen.getByText(words.note_imageIsNotOCR)).toBeInTheDocument();
        expect(
          screen.getByText(words.note_audioIsNotTranscript),
        ).toBeInTheDocument();
        expect(
          screen.getAllByText(words.noEvidenceInCapabilities),
        ).toHaveLength(7);
        expect(screen.getAllByText(words.matrixIndexEnabled)).toHaveLength(7);
        expect(screen.getByText(words.runtimeSources)).toBeInTheDocument();
      } finally {
        spy.mockRestore();
      }
    },
  );
  it('keeps local slide table flattening distinct from typed table output', () => {
    const words = Zh.translation.knowledgeConsumption as unknown as Record<
      string,
      string
    >;
    const spy = jest
      .spyOn(require('react-i18next'), 'useTranslation')
      .mockReturnValue({ t: translate(words) });
    try {
      render(<KnowledgeCapabilityMatrix capabilities={sample} />);
      const row = screen.getByText(words.family_pptx).closest('tr')!;
      expect(
        within(row).queryByText(words.output_table),
      ).not.toBeInTheDocument();
      expect(
        within(row).getByText(words.note_slidesFlattenTables),
      ).toBeInTheDocument();
      expect(
        within(row).getByText(words.matrixIndexUnknown),
      ).toBeInTheDocument();
    } finally {
      spy.mockRestore();
    }
  });
  it('does not infer outputs or extensions for an unreviewed producer identifier', () => {
    const words = En.translation.knowledgeConsumption as unknown as Record<
      string,
      string
    >;
    const spy = jest
      .spyOn(require('react-i18next'), 'useTranslation')
      .mockReturnValue({ t: translate(words) });
    try {
      render(
        <KnowledgeCapabilityMatrix
          capabilities={{
            ...Capabilities,
            parser_registrations: [
              { file_type: 'unreviewed', base_result: 'structured' },
            ],
          }}
        />,
      );
      expect(screen.getAllByText(words.mappingUnknown)).toHaveLength(2);
      expect(screen.queryByText(words.output_table)).not.toBeInTheDocument();
      expect(
        screen.getByText(words.noEvidenceInCapabilities),
      ).toBeInTheDocument();
    } finally {
      spy.mockRestore();
    }
  });
});

function parsedTargetFixture(): ContentTarget {
  return {
    source_ref: Target.source_ref,
    representation_ref: {
      kind: 'parsed_representation',
      id: '1'.repeat(64),
      producer_sha256: '2'.repeat(64),
    },
  };
}
function parsedPolicyFixture(enabled = true): ContentPolicy {
  return {
    domain_id: Domain,
    foundation_policy_version: 1,
    content_policy_version: enabled ? 2 : 1,
    parse_enabled: enabled,
    lexical_enabled: false,
  };
}
function parsedSourceFixture(): ContentParsedSource {
  return {
    source_ref: Target.source_ref,
    native_file_id: '4'.repeat(32),
    representation_kind: 'original_only',
    original_filename: 'synthetic.txt',
    relative_path: 'synthetic.txt',
    classification: {
      group_id: null,
      component_side: 'non_code',
      language: '',
    },
    byte_length: 9,
    lifecycle_version: 1,
    state: 'active',
    foundation_policy_version: 1,
    profile: Capabilities.parsed_profiles[0],
  };
}
async function parsedTaskFixture(
  state: ContentParsedTask['state'] = 'queued',
  request = contentParsedTaskRequest(
    parsedSourceFixture(),
    parsedPolicyFixture(),
    'fixture-key',
  ),
): Promise<ContentParsedTask> {
  const terminal = ['succeeded', 'failed', 'cancelled'].includes(state);
  return {
    task_id: '3'.repeat(32),
    owner_id: Owner,
    domain_id: Domain,
    original_request: request,
    request_sha256: await lifecycleDigest({
      kind: 'parsed-task/0.2.0',
      owner_id: Owner,
      domain_id: Domain,
      request: { ...request, source_ref: { ...request.source_ref } },
    }),
    producer_sha256: request.expected_producer_sha256,
    revision: state === 'queued' ? 1 : 2,
    attempt_count: state === 'queued' ? 0 : 1,
    state,
    phase: terminal
      ? 'done'
      : state === 'cancel_requested'
        ? 'cleaning'
        : state === 'running'
          ? 'extracting'
          : 'queued',
    result: state === 'succeeded' ? parsedTargetFixture() : null,
    failure_code: state === 'failed' ? 'parse_failed' : null,
    created_at: '2026-10-29T01:00:00Z',
    updated_at: '2026-10-29T01:00:01Z',
    deadline_at: '2026-10-29T01:02:00Z',
    completed_at: terminal ? '2026-10-29T01:00:01Z' : null,
  };
}
function parsedReaderFixture() {
  Target = parsedTargetFixture();
  Address = {
    target: Target,
    node_ref: { kind: 'parsed_node', id: NodeId, item_index: null },
  };
  Node = {
    ...Node,
    ref: Address.node_ref,
    ordinal: 0,
    kind: 'document',
    locator: {
      kind: 'document',
      source_utf8: null,
      document: { scheme: 'document_v1' },
      item_index: null,
    },
    original_position: { kind: 'document_only' },
    coordinate_precision: 'document',
    text: {
      ...Node.text,
      coordinate_space: 'derived_utf8',
      range: { ...Node.text.range!, sha256: Sha },
    },
    attributes: {
      source: null,
      structure: null,
      parser: null,
      document: {
        empty: {},
        page: null,
        slide: null,
        heading: null,
        table: null,
        row: null,
        cell: null,
        list: null,
        list_item: null,
        media: null,
        passage: null,
        unsupported_block: null,
      },
    },
  };
  Representation = {
    ...Representation,
    representation_ref: Target.representation_ref,
    root_ref: Address.node_ref,
    coordinate_space: 'derived_utf8',
    node_count: 1,
  };
  Window = {
    ...Window,
    address: Address,
    locator: Node.locator,
    coordinate_space: 'derived_utf8',
  };
}
function ParsedLocationProof() {
  const location = useLocation();
  return (
    <output data-testid="parsed-location">
      {JSON.stringify({ search: location.search, state: location.state })}
    </output>
  );
}
function renderParsed(
  entry: string | { search: string; state: unknown } = '?content=1&parsed=1',
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <ParsedLocationProof />
        <KnowledgeRepresentation scope={Scope} source={Source} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, client };
}
function renderParsedHistory(
  entries: Array<string | { search: string; state: unknown }>,
) {
  function HistoryNavigation() {
    const navigate = useNavigate();
    const previous = () => navigate(-1),
      next = () => navigate(1);
    return (
      <>
        <button onClick={previous}>previous parsed history</button>
        <button onClick={next}>next parsed history</button>
      </>
    );
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
        <HistoryNavigation />
        <ParsedLocationProof />
        <KnowledgeRepresentation scope={Scope} source={Source} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, client };
}
function installParsedHTTP(
  options: {
    enabled?: boolean;
    snapshot?: boolean;
    task?: ContentParsedTask;
    create?: (
      taskInput: ContentParsedTaskRequest,
      count: number,
    ) => Promise<ContentParsedTask>;
    uploadFailure?: boolean;
  } = {},
) {
  const original = Http.getMockImplementation()!;
  const state = {
    policy: parsedPolicyFixture(options.enabled ?? true),
    snapshot: options.snapshot ?? true,
    task: options.task,
    counts: {} as Record<string, number>,
    createBodies: [] as string[],
  };
  Http.mockImplementation(
    async (config: {
      url: string;
      method: string;
      data?: string | FormData;
    }) => {
      const route = config.url.split('/knowledge/content/')[1];
      state.counts[route] = (state.counts[route] ?? 0) + 1;
      if (route === 'policy') {
        if (config.method === 'post') {
          const body = parseContentJSON(config.data as string) as {
            request: import('@/interfaces/request/knowledge-consumption').ContentPolicyRequest;
          };
          expect(body.request.lexical_enabled).toBe(
            state.policy.lexical_enabled,
          );
          state.policy = {
            ...state.policy,
            content_policy_version: state.policy.content_policy_version + 1,
            parse_enabled: body.request.parse_enabled,
          };
          return response('content-policy-change', state.policy);
        }
        return response('content-policy', state.policy);
      }
      if (route === 'parsed-source/upload') {
        if (options.uploadFailure) throw Error('offline response lost');
        const file = (config.data as FormData).get('file') as File;
        return response('parsed-source-upload', {
          source_id: SourceId,
          original_filename: file.name,
          byte_length: file.size,
        });
      }
      if (route === 'parsed-source/snapshot') {
        state.snapshot = true;
        return response('parsed-source-snapshot', {
          source_ref: Target.source_ref,
          representation_kind: 'original_only',
          source_kind: 'parsed_file',
        });
      }
      if (route === 'parsed-source')
        return state.snapshot
          ? response('parsed-source', parsedSourceFixture())
          : response('parsed-source', null, 404, 'not_found');
      if (route === 'parsed-representations')
        return response('parsed-representations', {
          projection_version: KnowledgeContentVersion,
          source_ref: Target.source_ref,
          items: [],
          next_cursor: null,
        });
      if (route === 'parsed-task') {
        state.createBodies.push(config.data as string);
        const body = parseContentJSON(config.data as string) as {
          request: ContentParsedTaskRequest;
        };
        state.task = options.create
          ? await options.create(body.request, state.createBodies.length)
          : await parsedTaskFixture('queued', body.request);
        return response('parsed-task-create', state.task, 201);
      }
      if (route === 'parsed-task/get')
        return response(
          'parsed-task-get',
          state.task ?? (await parsedTaskFixture()),
        );
      if (route === 'parsed-task/cancel') {
        state.task = {
          ...state.task!,
          revision: state.task!.revision + 1,
          state: 'cancel_requested',
          phase: 'cleaning',
        };
        return response('parsed-task-cancel', state.task);
      }
      return original(config);
    },
  );
  return state;
}

describe('complete parsed candidate', () => {
  it('accepts fractional lexical scores only at the explicit wire path and retains exact context wire bytes', () => {
    const raw = '{"data":{"hits":[{"score":0.25}],"context":{"node":null}}}';
    expect(() => parseContentJSON(raw)).toThrow();
    const value = parseContentJSON(raw, true) as {
      data: { hits: Array<{ score: number }>; context: object };
    };
    expect(value.data.hits[0].score).toBe(0.25);
    expect(contentWireBytes(value.data.context)).toBe(
      new TextEncoder().encode('{"node":null}').length,
    );
    expect(() =>
      parseContentJSON('{"data":{"hits":[{"limit":0.25}]}}', true),
    ).toThrow();
    expect(() =>
      parseContentJSON('{"data":{"hits":[{"score":1e999}]}}', true),
    ).toThrow();
  });
  it('binds fixed parsed namespaces and complete discovery pages without promoting temporary items', () => {
    const target = parsedTargetFixture(),
      ref = { kind: 'parsed_node' as const, id: NodeId, item_index: null };
    expect(contentBoundNode(target, ref)).toBe(true);
    expect(
      contentBoundNode(target, {
        kind: 'parser_item',
        id: null,
        item_index: 0,
      }),
    ).toBe(false);
    expect(
      contentRoute(
        Domain,
        new URLSearchParams(
          contentFixedLink({ target, node_ref: ref }).split('?')[1],
        ),
      ),
    ).toEqual({ target, node_ref: ref });
    const page = {
      projection_version: KnowledgeContentVersion,
      source_ref: target.source_ref,
      items: [target],
      next_cursor: null,
    };
    expect(contentParsedTargets(target.source_ref, [page])).toEqual([target]);
    expect(() =>
      contentParsedTargets(
        { ...target.source_ref, source_revision_id: '9'.repeat(32) },
        [page],
      ),
    ).toThrow();
    expect(() => contentParsedTargets(target.source_ref, [page, page])).toThrow(
      'content_repeated_representation',
    );
  });
  it('preserves nullable positions and public ragged table geometry without fabricating original pages', async () => {
    const target = parsedTargetFixture(),
      table = {
        grid: [
          [
            { text: 'A', col_span: 2 },
            { text: '', covered: true },
          ],
          null,
          [{ text: '' }, { text: '合成' }],
        ],
        header_rows: 1,
      };
    const text = contentTableText(table),
      sha = await contentDigest(text);
    expect(text).toBe('A\n\n\t合成');
    const node: ContentNode = {
      ...Node,
      ref: { kind: 'parsed_node', id: NodeId, item_index: null },
      parent_ref: { kind: 'parsed_node', id: '8'.repeat(64), item_index: null },
      ordinal: 1,
      kind: 'table',
      locator: {
        kind: 'parsed_item',
        source_utf8: null,
        document: null,
        item_index: 0,
      },
      original_position: null,
      coordinate_precision: 'item_only',
      text: {
        coordinate_space: 'derived_utf8',
        range: {
          start_byte: 0,
          end_byte: new TextEncoder().encode(text).length,
          sha256: sha,
        },
        own_range: null,
        status: 'available',
        reason: null,
        value: null,
      },
      attributes: {
        source: null,
        document: null,
        structure: null,
        parser: {
          item_type: 'table',
          ck_type: 'table',
          table,
          sheet: 'Sheet1',
          positions: [null, [1.25, 2.5]],
          alt_text: null,
          payload_present: false,
          text_assembly: 'row_major_uncovered_cells_tab_lf_v1',
        },
      },
    };
    expect(contentParsedPosition(node)).toBe(true);
    await expect(checkContentNode(target, node)).resolves.toBeUndefined();
    expect(
      contentParsedPosition({
        ...node,
        original_position: { kind: 'pdf_page', page_number: 1 },
      }),
    ).toBe(false);
    node.attributes.parser!.positions = [[Infinity]];
    await expect(checkContentNode(target, node)).rejects.toThrow();
    expect(() =>
      contentTableText({
        grid: [[{ text: 'covered', covered: true }]],
        header_rows: 0,
      }),
    ).toThrow();
    expect(() =>
      contentTableText({
        grid: [[{ text: 'span', row_span: 2 }]],
        header_rows: 0,
      }),
    ).toThrow();
  });
  it('derives explicit parse permission and request pins from actual source and policy', () => {
    const policy = { ...parsedPolicyFixture(false), lexical_enabled: true };
    expect(contentEnableParseRequest(policy, 'same-request')).toMatchObject({
      parse_enabled: true,
      lexical_enabled: true,
      expected_policy_version: 1,
      expected_content_policy_version: 1,
      confirmed: true,
    });
    const request = contentParsedTaskRequest(
      parsedSourceFixture(),
      parsedPolicyFixture(),
      'same-task',
    );
    expect(request.max_unit_bytes).toBe(8192);
    expect(request.expected_producer_sha256).toBe(
      parsedSourceFixture().profile!.producer_sha256,
    );
    expect(() =>
      contentParsedTaskRequest(
        { ...parsedSourceFixture(), state: 'archived' },
        parsedPolicyFixture(),
        'x',
      ),
    ).toThrow();
    expect(() =>
      contentParsedTaskRequest(
        parsedSourceFixture(),
        parsedPolicyFixture(false),
        'x',
      ),
    ).toThrow();
    for (const filename of [
      '.env.txt',
      '../a.txt',
      'a.pdf',
      'a.docx',
      'a.pptx',
      'a.png',
      'a.mp3',
      'a\ud800.txt',
    ])
      expect(contentParsedFileFormat(filename)).toBeNull();
    expect(contentParsedFileFormat('合成.JSONL')).toBe('json');
  });
  it('validates task owner, request digest, fixed result binding and monotonic revisions', async () => {
    const task = await parsedTaskFixture('succeeded');
    await expect(checkContentTask(Scope, task)).resolves.toBeUndefined();
    await expect(
      checkContentTask(Scope, { ...task, owner_id: '9'.repeat(32) }),
    ).rejects.toThrow();
    await expect(
      checkContentTask(Scope, { ...task, request_sha256: '9'.repeat(64) }),
    ).rejects.toThrow();
    await expect(
      checkContentTask(Scope, {
        ...task,
        result: {
          ...task.result!,
          source_ref: {
            ...task.result!.source_ref,
            source_revision_id: '9'.repeat(32),
          },
        },
      }),
    ).rejects.toThrow();
    await expect(
      checkContentTask(
        Scope,
        { ...task, revision: task.revision + 1 },
        { previous: task },
      ),
    ).rejects.toThrow();
    expect(
      contentSchemaValid('ParsedTask', {
        ...task,
        completed_at: '2026-02-31T01:00:01Z',
      }),
    ).toBe(false);
  });
  it('uses one exact multipart request and one original file through the existing service transport', async () => {
    const file = new File([Text], 'synthetic.txt', { type: 'text/plain' });
    Http.mockResolvedValue(
      response('parsed-source-upload', {
        source_id: SourceId,
        original_filename: file.name,
        byte_length: file.size,
      }),
    );
    await service.uploadParsedSource(
      Scope,
      {
        expected_policy_version: 1,
        expected_content_policy_version: 2,
        confirmed: true,
      },
      file,
    );
    const [config] = Http.mock.calls[0];
    expect(config.url.endsWith('/knowledge/content/parsed-source/upload')).toBe(
      true,
    );
    expect(config.data).toBeInstanceOf(FormData);
    expect([...config.data.keys()]).toEqual(['request', 'file']);
    expect(config.headers?.['Content-Type']).toBeUndefined();
    expect(JSON.parse(config.data.get('request'))).toEqual({
      contract_version: KnowledgeContentVersion,
      request: {
        expected_policy_version: 1,
        expected_content_policy_version: 2,
        confirmed: true,
      },
    });
    expect(config.data.get('file').size).toBe(file.size);
    Http.mockClear();
    await expect(
      service.uploadParsedSource(
        Scope,
        {
          expected_policy_version: 1,
          expected_content_policy_version: 2,
          confirmed: true,
        },
        new File(['x'], 'bad.pdf'),
      ),
    ).rejects.toMatchObject({ status: 400 });
    expect(Http).not.toHaveBeenCalled();
  });
  it('accepts native task 201 and replay 200 while preserving durable 429 rejections', async () => {
    const request = contentParsedTaskRequest(
        parsedSourceFixture(),
        parsedPolicyFixture(),
        'test-key',
      ),
      task = await parsedTaskFixture('queued', request);
    for (const status of [201, 200]) {
      Http.mockResolvedValue(response('parsed-task-create', task, status));
      expect((await service.createParsedTask(Scope, request)).task_id).toBe(
        task.task_id,
      );
    }
    const rejected = response(
      'parsed-task-create',
      null,
      429,
      'capacity_exceeded',
    );
    Object.assign(rejected.data as object, {
      outcome_known: true,
      rejection_id: '5'.repeat(64),
    });
    Http.mockResolvedValue(rejected);
    await expect(
      service.createParsedTask(Scope, request),
    ).rejects.toMatchObject({
      status: 429,
      code: 'capacity_exceeded',
      outcomeKnown: true,
      rejectionId: '5'.repeat(64),
    });
    Http.mockResolvedValue({ ...rejected, status: 413 });
    await expect(
      service.createParsedTask(Scope, request),
    ).rejects.toMatchObject({ status: 502 });
  });
  it('rejects duplicate, malformed and mixed fixed/manual recovery routes', () => {
    const params = new URLSearchParams({
      content: '1',
      parsed: '1',
      parsed_source: SourceId,
      parsed_task: '3'.repeat(32),
    });
    expect(contentParsedWorkflowRoute(params)).toEqual({
      source: SourceId,
      task: '3'.repeat(32),
    });
    params.append('parsed_task', '4'.repeat(32));
    expect(() => contentParsedWorkflowRoute(params)).toThrow();
    expect(() =>
      contentParsedWorkflowRoute(
        new URLSearchParams({ parsed: '1', parsed_task: '3'.repeat(32) }),
      ),
    ).toThrow();
    expect(() =>
      contentParsedWorkflowRoute(
        new URLSearchParams({
          parsed: '1',
          parsed_source: SourceId,
          content_node: NodeId,
        }),
      ),
    ).toThrow();
  });
  it('keeps save, snapshot and manual parse separate, then automatically reads the exact successful target', async () => {
    parsedReaderFixture();
    const state = installParsedHTTP({
        enabled: false,
        snapshot: false,
        create: (request) => parsedTaskFixture('succeeded', request),
      }),
      view = renderParsed();
    await screen.findByText('knowledgeConsumption.manualDisabled');
    expect(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualSave' }),
    ).toBeDisabled();
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualEnable' }),
    );
    await screen.findByText('knowledgeConsumption.manualEnabled');
    fireEvent.change(screen.getByLabelText('knowledgeConsumption.manualFile'), {
      target: {
        files: [new File([Text], 'synthetic.txt', { type: 'text/plain' })],
      },
    });
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualSave' }),
    );
    await screen.findByText(/knowledgeConsumption.manualSaved/);
    expect(state.counts['parsed-source/upload']).toBe(1);
    expect(state.counts['parsed-source/snapshot'] ?? 0).toBe(0);
    expect(state.counts['parsed-task'] ?? 0).toBe(0);
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualSnapshot',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', {
        name: 'knowledgeConsumption.manualSnapshot',
      }),
    );
    await screen.findByText(/knowledgeConsumption.manualSnapshotted/);
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualStart',
        }),
      ).toBeEnabled(),
    );
    expect(state.counts['parsed-task'] ?? 0).toBe(0);
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualStart' }),
    );
    await screen.findByText('knowledgeConsumption.manualResult');
    await waitFor(() =>
      expect(
        [...view.container.querySelectorAll('pre')].some(
          (element) => element.textContent === Text,
        ),
      ).toBe(true),
    );
    const read = Http.mock.calls.find(([config]) =>
      config.url.endsWith('/content/read'),
    )![0];
    expect(JSON.parse(read.data).read.address.target).toEqual(
      state.task!.result,
    );
    fireEvent.click(
      screen.getByRole('button', {
        name: 'knowledgeLocalLexical.addFixedRepresentation',
      }),
    );
    const indexEntry = JSON.parse(
      (await screen.findByTestId('parsed-index-entry')).textContent!,
    );
    expect(indexEntry).toMatchObject({
      domain: Domain,
      owner: Owner,
      initialChoice: { target: state.task!.result, currentness: 'unknown' },
    });
    expect(state.counts['parsed-task']).toBe(1);
    expect(state.counts['parsed-source/upload']).toBe(1);
    view.unmount();
    view.client.clear();
  });
  it('recovers a fixed task, keeps cancellation pending until server terminal state, and never reads a result early', async () => {
    const running = await parsedTaskFixture('running'),
      state = installParsedHTTP({ task: running }),
      view = renderParsed(
        '?content=1&parsed=1&parsed_source=' +
          SourceId +
          '&parsed_task=' +
          running.task_id,
      );
    await screen.findByText(/knowledgeConsumption.manualState_running/);
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualCancel' }),
    );
    await screen.findByText(
      /knowledgeConsumption.manualState_cancel_requested/,
    );
    expect(
      screen.queryByText('knowledgeConsumption.manualState_cancelled'),
    ).not.toBeInTheDocument();
    expect(state.counts.read ?? 0).toBe(0);
    state.task = {
      ...state.task!,
      revision: state.task!.revision + 1,
      state: 'cancelled',
      phase: 'done',
      completed_at: '2026-10-29T01:00:01Z',
    };
    fireEvent.click(
      screen.getByRole('button', {
        name: 'knowledgeConsumption.manualRefresh',
      }),
    );
    await screen.findByText(/knowledgeConsumption.manualState_cancelled/);
    expect(state.counts['parsed-task/cancel']).toBe(1);
    view.unmount();
    view.client.clear();
  });
  it('retains an ambiguous upload in page history and blocks automatic or repeated upload', async () => {
    const state = installParsedHTTP({ snapshot: false, uploadFailure: true }),
      view = renderParsed();
    await screen.findByText('knowledgeConsumption.manualEnabled');
    fireEvent.change(screen.getByLabelText('knowledgeConsumption.manualFile'), {
      target: { files: [new File([Text], 'synthetic.txt')] },
    });
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualSave' }),
    );
    await screen.findByText('knowledgeConsumption.manualUploadUnknown');
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'knowledgeConsumption.manualSave' }),
      ).toBeDisabled(),
    );
    const entry = JSON.parse(
      screen.getByTestId('parsed-location').textContent!,
    );
    view.unmount();
    view.client.clear();
    const reopened = renderParsed(entry);
    await screen.findByText('knowledgeConsumption.manualUploadUnknown');
    expect(
      screen.getByLabelText('knowledgeConsumption.manualFile'),
    ).toBeDisabled();
    expect(state.counts['parsed-source/upload']).toBe(1);
    reopened.unmount();
    reopened.client.clear();
  });
  it('restores an unconfirmed task request and retries the same body and idempotency key', async () => {
    const state = installParsedHTTP({
        create: async (request, count) => {
          if (count === 1) throw Error('offline response lost');
          return parsedTaskFixture('queued', request);
        },
      }),
      view = renderParsed('?content=1&parsed=1&parsed_source=' + SourceId);
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualStart',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualStart' }),
    );
    await screen.findByText('knowledgeConsumption.manualTaskUnknown');
    const entry = JSON.parse(
      screen.getByTestId('parsed-location').textContent!,
    );
    view.unmount();
    view.client.clear();
    const reopened = renderParsed(entry);
    await screen.findByText('knowledgeConsumption.manualTaskUnknown');
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualRetryTask',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', {
        name: 'knowledgeConsumption.manualRetryTask',
      }),
    );
    await screen.findByText(/knowledgeConsumption.manualState_queued/);
    expect(state.createBodies).toHaveLength(2);
    expect(state.createBodies[1]).toBe(state.createBodies[0]);
    reopened.unmount();
    reopened.client.clear();
  });
  it('releases a durably rejected cancellation and uses the refreshed task revision for a new explicit cancellation', async () => {
    const state = installParsedHTTP({
        task: await parsedTaskFixture('running'),
      }),
      normal = Http.getMockImplementation()!,
      bodies: Array<{
        task_id: string;
        request: { expected_task_revision: number; idempotency_key: string };
      }> = [];
    Http.mockImplementation(async (config) => {
      if (config.url.endsWith('/parsed-task/cancel')) {
        bodies.push(JSON.parse(config.data));
        if (bodies.length === 1) {
          state.task = {
            ...state.task!,
            revision: 3,
            updated_at: '2026-10-29T01:00:02Z',
          };
          const rejected = response(
            'parsed-task-cancel',
            null,
            409,
            'task_changed',
          );
          Object.assign(rejected.data as object, {
            outcome_known: true,
            rejection_id: '6'.repeat(64),
          });
          return rejected;
        }
      }
      return normal(config);
    });
    const view = renderParsed(
      '?content=1&parsed=1&parsed_source=' +
        SourceId +
        '&parsed_task=' +
        '3'.repeat(32),
    );
    await screen.findByText(/knowledgeConsumption.manualState_running/);
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualCancel' }),
    );
    await screen.findByText(/task_changed/);
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualCancel',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualCancel' }),
    );
    await screen.findByText(
      /knowledgeConsumption.manualState_cancel_requested/,
    );
    expect(bodies.map((body) => body.request.expected_task_revision)).toEqual([
      2, 3,
    ]);
    expect(bodies[1].request.idempotency_key).not.toBe(
      bodies[0].request.idempotency_key,
    );
    view.unmount();
    view.client.clear();
  });
  it('clears a previous fixed read immediately when browser history selects another pending task', async () => {
    parsedReaderFixture();
    installParsedHTTP({ task: await parsedTaskFixture('succeeded') });
    const normal = Http.getMockImplementation()!;
    let finish!: (value: unknown) => void;
    Http.mockImplementation((config) =>
      config.url.endsWith('/parsed-task/get') &&
      JSON.parse(config.data).task_id === '4'.repeat(32)
        ? new Promise((resolve) => {
            finish = resolve;
          })
        : normal(config),
    );
    function SwitchTask() {
      const [, setParams] = useSearchParams();
      const change = () =>
        setParams({
          content: '1',
          parsed: '1',
          parsed_source: SourceId,
          parsed_task: '4'.repeat(32),
        });
      return <button onClick={change}>switch manual task</button>;
    }
    const client = new QueryClient(),
      view = render(
        <QueryClientProvider client={client}>
          <MemoryRouter
            initialEntries={[
              '?content=1&parsed=1&parsed_source=' +
                SourceId +
                '&parsed_task=' +
                '3'.repeat(32),
            ]}
          >
            <SwitchTask />
            <KnowledgeRepresentation scope={Scope} source={Source} />
          </MemoryRouter>
        </QueryClientProvider>,
      );
    await waitFor(() =>
      expect(
        [...view.container.querySelectorAll('pre')].some(
          (element) => element.textContent === Text,
        ),
      ).toBe(true),
    );
    fireEvent.click(screen.getByRole('button', { name: 'switch manual task' }));
    await waitFor(() =>
      expect(
        [...view.container.querySelectorAll('pre')].some(
          (element) => element.textContent === Text,
        ),
      ).toBe(false),
    );
    const next = await parsedTaskFixture(
      'queued',
      contentParsedTaskRequest(
        parsedSourceFixture(),
        parsedPolicyFixture(),
        'second-task',
      ),
    );
    await act(async () => {
      finish(response('parsed-task-get', { ...next, task_id: '4'.repeat(32) }));
    });
    await screen.findByText(/knowledgeConsumption.manualState_queued/);
    view.unmount();
    client.clear();
  });
  it('drops mutation completion after the consuming scope unmounts', async () => {
    let finish!: (value: unknown) => void,
      send!: (file: File) => Promise<unknown>;
    Http.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    function ActionProbe() {
      const action = useContentParsedUpload(Scope);
      send = (file) =>
        action.mutateAsync({
          file,
          request: {
            expected_policy_version: 1,
            expected_content_policy_version: 2,
            confirmed: true,
          },
        });
      return null;
    }
    const client = new QueryClient(),
      view = render(
        <QueryClientProvider client={client}>
          <ActionProbe />
        </QueryClientProvider>,
      );
    let pending!: Promise<unknown>;
    await act(async () => {
      pending = send(new File([Text], 'synthetic.txt'));
    });
    view.unmount();
    finish(
      response('parsed-source-upload', {
        source_id: SourceId,
        original_filename: 'synthetic.txt',
        byte_length: 9,
      }),
    );
    await expect(pending).rejects.toMatchObject({
      code: 'content_scope_changed',
    });
    client.clear();
  });
  it('keeps an unknown cancellation key for task A retries, then history task B gets its own exact task and key', async () => {
    const taskA = await parsedTaskFixture('running');
    let taskB = {
      ...(await parsedTaskFixture('running')),
      task_id: '4'.repeat(32),
    };
    installParsedHTTP({ task: taskA });
    const normal = Http.getMockImplementation()!,
      bodies: Array<{
        task_id: string;
        request: { idempotency_key: string; expected_task_revision: number };
      }> = [];
    Http.mockImplementation(async (config) => {
      if (config.url.endsWith('/parsed-task/get'))
        return response(
          'parsed-task-get',
          JSON.parse(config.data).task_id === taskA.task_id ? taskA : taskB,
        );
      if (config.url.endsWith('/parsed-task/cancel')) {
        const body = JSON.parse(config.data);
        bodies.push(body);
        if (body.task_id === taskA.task_id) throw Error('task A response lost');
        taskB = {
          ...taskB,
          revision: 3,
          state: 'cancel_requested',
          phase: 'cleaning',
        };
        return response('parsed-task-cancel', taskB);
      }
      return normal(config);
    });
    const route = (task: string) =>
      '?content=1&parsed=1&parsed_source=' + SourceId + '&parsed_task=' + task;
    const view = renderParsedHistory([
      route(taskB.task_id),
      route(taskA.task_id),
    ]);
    await screen.findByText(/knowledgeConsumption.manualState_running/);
    for (let attempt = 1; attempt <= 2; attempt++) {
      await waitFor(() =>
        expect(
          screen.getByRole('button', {
            name: /^knowledgeConsumption.manual(?:Retry)?Cancel$/,
          }),
        ).toBeEnabled(),
      );
      fireEvent.click(
        screen.getByRole('button', {
          name: /^knowledgeConsumption.manual(?:Retry)?Cancel$/,
        }),
      );
      await waitFor(() => expect(bodies).toHaveLength(attempt));
      await waitFor(() =>
        expect(
          screen.getByRole('button', {
            name: /^knowledgeConsumption.manual(?:Retry)?Cancel$/,
          }),
        ).toBeEnabled(),
      );
    }
    expect(bodies[1]).toEqual(bodies[0]);
    fireEvent.click(
      screen.getByRole('button', { name: 'previous parsed history' }),
    );
    await waitFor(() =>
      expect(screen.getByTestId('parsed-location')).toHaveTextContent(
        taskB.task_id,
      ),
    );
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualCancel',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualCancel' }),
    );
    await screen.findByText(
      /knowledgeConsumption.manualState_cancel_requested/,
    );
    expect(bodies.map((body) => body.task_id)).toEqual([
      taskA.task_id,
      taskA.task_id,
      taskB.task_id,
    ]);
    expect(bodies[2].request.idempotency_key).not.toBe(
      bodies[0].request.idempotency_key,
    );
    view.unmount();
    view.client.clear();
  });
  it.each(['success', 'known_rejection'] as const)(
    'ignores a late create %s after internal source history changes, while preserving the original pending request for a later retry',
    async (outcome) => {
      const sourceA = parsedSourceFixture(),
        sourceB = {
          ...sourceA,
          source_ref: { ...sourceA.source_ref, source_id: '8'.repeat(32) },
        },
        taskB = {
          ...(await parsedTaskFixture(
            'running',
            contentParsedTaskRequest(sourceB, parsedPolicyFixture(), 'task-b'),
          )),
          task_id: '4'.repeat(32),
        };
      const state = installParsedHTTP();
      const normal = Http.getMockImplementation()!;
      let finish!: (value: unknown) => void,
        submitted!: ContentParsedTaskRequest;
      Http.mockImplementation((config) => {
        if (config.url.endsWith('/parsed-source'))
          return response(
            'parsed-source',
            JSON.parse(config.data).source_id === sourceA.source_ref.source_id
              ? sourceA
              : sourceB,
          );
        if (config.url.endsWith('/parsed-task/get'))
          return response('parsed-task-get', taskB);
        if (config.url.endsWith('/parsed-task')) {
          submitted = JSON.parse(config.data).request;
          state.createBodies.push(config.data);
          return new Promise((resolve) => {
            finish = resolve;
          });
        }
        return normal(config);
      });
      const routeB =
        '?content=1&parsed=1&parsed_source=' +
        sourceB.source_ref.source_id +
        '&parsed_task=' +
        taskB.task_id;
      const view = renderParsedHistory([
        routeB,
        '?content=1&parsed=1&parsed_source=' + SourceId,
      ]);
      await waitFor(() =>
        expect(
          screen.getByRole('button', {
            name: 'knowledgeConsumption.manualStart',
          }),
        ).toBeEnabled(),
      );
      fireEvent.click(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualStart',
        }),
      );
      await waitFor(() => expect(state.createBodies).toHaveLength(1));
      const pendingEntry = JSON.parse(
        screen.getByTestId('parsed-location').textContent!,
      );
      expect(pendingEntry.state.knowledgeParsed.pendingTask).toEqual(submitted);
      // start saves the pending request in a new entry for the same source.
      fireEvent.click(
        screen.getByRole('button', { name: 'previous parsed history' }),
      );
      fireEvent.click(
        screen.getByRole('button', { name: 'previous parsed history' }),
      );
      await waitFor(() =>
        expect(screen.getByTestId('parsed-location')).toHaveTextContent(
          sourceB.source_ref.source_id,
        ),
      );
      await screen.findByText(/knowledgeConsumption.manualState_running/);
      await act(async () => {
        if (outcome === 'success')
          finish(
            response(
              'parsed-task-create',
              await parsedTaskFixture('queued', submitted),
              201,
            ),
          );
        else {
          const rejected = response(
            'parsed-task-create',
            null,
            409,
            'content_policy_changed',
          );
          Object.assign(rejected.data as object, {
            outcome_known: true,
            rejection_id: '6'.repeat(64),
          });
          finish(rejected);
        }
      });
      const current = JSON.parse(
        screen.getByTestId('parsed-location').textContent!,
      );
      expect(new URLSearchParams(current.search).get('parsed_source')).toBe(
        sourceB.source_ref.source_id,
      );
      expect(new URLSearchParams(current.search).get('parsed_task')).toBe(
        taskB.task_id,
      );
      expect(
        screen.queryByText(/content_policy_changed/),
      ).not.toBeInTheDocument();
      view.unmount();
      view.client.clear();
      installParsedHTTP({
        create: (request) => parsedTaskFixture('queued', request),
      });
      if (outcome === 'known_rejection') {
        const replay = Http.getMockImplementation()!;
        Http.mockImplementation((config) => {
          if (!config.url.endsWith('/parsed-task')) return replay(config);
          const rejected = response(
            'parsed-task-create',
            null,
            409,
            'content_policy_changed',
          );
          Object.assign(rejected.data as object, {
            outcome_known: true,
            rejection_id: '6'.repeat(64),
          });
          return rejected;
        });
      }
      const reopened = renderParsed(pendingEntry);
      await waitFor(() =>
        expect(
          screen.getByRole('button', {
            name: 'knowledgeConsumption.manualRetryTask',
          }),
        ).toBeEnabled(),
      );
      fireEvent.click(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualRetryTask',
        }),
      );
      if (outcome === 'known_rejection') {
        await screen.findByText(/content_policy_changed/);
        expect(
          screen.queryByText('knowledgeConsumption.manualTaskUnknown'),
        ).not.toBeInTheDocument();
      } else await screen.findByText(/knowledgeConsumption.manualState_queued/);
      const newRequest = Http.mock.calls
        .filter(([config]) => config.url.endsWith('/parsed-task'))
        .at(-1)![0];
      expect(newRequest.data).toBe(state.createBodies[0]);
      reopened.unmount();
      reopened.client.clear();
    },
  );
  it('drops a late upload result after history selects another source; the old entry retains its unknown receipt state', async () => {
    installParsedHTTP();
    const normal = Http.getMockImplementation()!;
    let finish!: (value: unknown) => void;
    Http.mockImplementation((config) =>
      config.url.endsWith('/parsed-source/upload')
        ? new Promise((resolve) => {
            finish = resolve;
          })
        : normal(config),
    );
    const view = renderParsedHistory([
      '?content=1&parsed=1&parsed_source=' + SourceId,
      '?content=1&parsed=1',
    ]);
    await screen.findByText('knowledgeConsumption.manualEnabled');
    fireEvent.change(screen.getByLabelText('knowledgeConsumption.manualFile'), {
      target: { files: [new File([Text], 'synthetic.txt')] },
    });
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualSave' }),
    );
    await waitFor(() => expect(finish).toBeDefined());
    const pendingEntry = JSON.parse(
      screen.getByTestId('parsed-location').textContent!,
    );
    expect(pendingEntry.state.knowledgeParsed.uploadUnconfirmed).toBe(true);
    fireEvent.click(
      screen.getByRole('button', { name: 'previous parsed history' }),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'previous parsed history' }),
    );
    await waitFor(() =>
      expect(screen.getByTestId('parsed-location')).toHaveTextContent(SourceId),
    );
    await act(async () => {
      finish(
        response('parsed-source-upload', {
          source_id: '9'.repeat(32),
          original_filename: 'synthetic.txt',
          byte_length: 9,
        }),
      );
    });
    expect(
      new URLSearchParams(
        JSON.parse(screen.getByTestId('parsed-location').textContent!).search,
      ).get('parsed_source'),
    ).toBe(SourceId);
    view.unmount();
    view.client.clear();
    const reopened = renderParsed(pendingEntry);
    await screen.findByText('knowledgeConsumption.manualUploadUnknown');
    expect(
      screen.getByLabelText('knowledgeConsumption.manualFile'),
    ).toBeDisabled();
    reopened.unmount();
    reopened.client.clear();
  });
  it.each(['succeeded', 'failed', 'cancelled'] as const)(
    'creates and automatically reads task B in one explicit action from terminal task A (%s)',
    async (terminal) => {
      parsedReaderFixture();
      const taskA = await parsedTaskFixture(terminal),
        taskBId = '4'.repeat(32);
      const state = installParsedHTTP({
        task: taskA,
        create: async (request) => ({
          ...(await parsedTaskFixture('succeeded', request)),
          task_id: taskBId,
        }),
      });
      const view = renderParsed(
        '?content=1&parsed=1&parsed_source=' +
          SourceId +
          '&parsed_task=' +
          taskA.task_id,
      );
      await screen.findByText('knowledgeConsumption.manualState_' + terminal, {
        exact: false,
      });
      await waitFor(() =>
        expect(
          screen.getByRole('button', {
            name: 'knowledgeConsumption.manualStart',
          }),
        ).toBeEnabled(),
      );
      fireEvent.click(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualStart',
        }),
      );
      await waitFor(() => expect(state.createBodies).toHaveLength(1));
      await waitFor(() =>
        expect(
          new URLSearchParams(
            JSON.parse(screen.getByTestId('parsed-location').textContent!)
              .search,
          ).get('parsed_task'),
        ).toBe(taskBId),
      );
      await screen.findByText('knowledgeConsumption.manualResult');
      await waitFor(() =>
        expect(
          [...view.container.querySelectorAll('pre')].some(
            (element) => element.textContent === Text,
          ),
        ).toBe(true),
      );
      const reads = Http.mock.calls.filter(([config]) =>
        config.url.endsWith('/content/read'),
      );
      expect(JSON.parse(reads.at(-1)![0].data).read.address.target).toEqual(
        state.task!.result,
      );
      expect(state.createBodies).toHaveLength(1);
      expect(
        screen.queryByText('knowledgeConsumption.manualTaskUnknown'),
      ).not.toBeInTheDocument();
      view.unmount();
      view.client.clear();
    },
  );
  it('retains a new unknown task request on terminal task A history and retries its exact key without reopening A result', async () => {
    parsedReaderFixture();
    const taskA = await parsedTaskFixture('succeeded');
    const state = installParsedHTTP({
      task: taskA,
      create: async (request, count) => {
        if (count === 1) throw Error('new task response lost');
        return {
          ...(await parsedTaskFixture('succeeded', request)),
          task_id: '4'.repeat(32),
        };
      },
    });
    const view = renderParsed(
      '?content=1&parsed=1&parsed_source=' +
        SourceId +
        '&parsed_task=' +
        taskA.task_id,
    );
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualStart',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeConsumption.manualStart' }),
    );
    await screen.findByText('knowledgeConsumption.manualTaskUnknown');
    const entry = JSON.parse(
      screen.getByTestId('parsed-location').textContent!,
    );
    expect(new URLSearchParams(entry.search).get('parsed_task')).toBe(
      taskA.task_id,
    );
    const original = state.createBodies[0];
    view.unmount();
    view.client.clear();
    const reopened = renderParsed(entry);
    await screen.findByText('knowledgeConsumption.manualTaskUnknown');
    expect(
      screen.queryByText('knowledgeConsumption.manualResult'),
    ).not.toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByRole('button', {
          name: 'knowledgeConsumption.manualRetryTask',
        }),
      ).toBeEnabled(),
    );
    fireEvent.click(
      screen.getByRole('button', {
        name: 'knowledgeConsumption.manualRetryTask',
      }),
    );
    await waitFor(() =>
      expect(
        new URLSearchParams(
          JSON.parse(screen.getByTestId('parsed-location').textContent!).search,
        ).get('parsed_task'),
      ).toBe('4'.repeat(32)),
    );
    await screen.findByText('knowledgeConsumption.manualResult');
    expect(state.createBodies).toHaveLength(2);
    expect(state.createBodies[1]).toBe(original);
    reopened.unmount();
    reopened.client.clear();
  });
});

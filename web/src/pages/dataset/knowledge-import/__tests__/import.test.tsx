import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import {
  QueryClient,
  QueryClientProvider,
  focusManager,
} from '@tanstack/react-query';
import { webcrypto } from 'node:crypto';
import request from '@/utils/next-request';
import { getAuthorization } from '@/utils/authorization-util';
import KnowledgeImport from '..';
import KnowledgeCatalogue from '../../knowledge-catalogue';
import {
  Domain,
  DomainId,
  Source,
  SourceText,
  Root,
  Capabilities,
  envelope,
  page,
} from '../../knowledge-catalogue/__tests__/fixture';
import {
  ImportCapabilities,
  Registration,
  makePreview,
  makeReceipt,
  c2Envelope,
} from './fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { getUserInfoObject: () => ({ id: 'owner' }) },
  getAuthorization: jest.fn(() => 'synthetic-native-session'),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
const Http = request as unknown as jest.Mock;
let Preview = makePreview();
let StoredReceipt:
  | import('@/interfaces/database/knowledge-sources').KnowledgeImportReceipt
  | null;
let Text: string;
let Mode:
  | 'success'
  | 'failed-first'
  | 'timeout-before'
  | 'timeout-after'
  | 'unchanged'
  | 'conflict'
  | 'deny' = 'success';
let CapabilityStatus = 'Frozen';
let PreviewCount = 0;
let Imports: unknown[];
let CurrentRegistration = Registration;
let NewCatalogueRevision = false;
let ConflictScenario = 'expired';
let DeniedStatus = 404;

function respond(data: unknown, c2 = false, status = 200) {
  return {
    status,
    data: JSON.parse(JSON.stringify(c2 ? c2Envelope(data) : envelope(data))),
  };
}

beforeAll(() => {
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  });
  Object.defineProperty(File.prototype, 'arrayBuffer', {
    configurable: true,
    value: function (this: File) {
      return new Promise<ArrayBuffer>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result as ArrayBuffer);
        reader.onerror = reject;
        reader.readAsArrayBuffer(this);
      });
    },
  });
  focusManager.setFocused(false);
});
afterAll(() => focusManager.setFocused(undefined));
beforeEach(() => {
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-native-session');
  sessionStorage.clear();
  Mode = 'success';
  CapabilityStatus = 'Frozen';
  PreviewCount = 0;
  StoredReceipt = null;
  Text = SourceText;
  Imports = [];
  CurrentRegistration = Registration;
  NewCatalogueRevision = false;
  ConflictScenario = 'expired';
  DeniedStatus = 404;
  Http.mockReset().mockImplementation(async (config) => {
    config = {
      ...config,
      data:
        typeof config.data === 'string' ? JSON.parse(config.data) : config.data,
    };
    const url: string = config.url;
    if (url === '/api/v1/knowledge/import-contract')
      return respond({ ...ImportCapabilities, status: CapabilityStatus }, true);
    if (url === '/api/v1/knowledge/contract') return respond(Capabilities);
    if (url.endsWith('/knowledge/domain')) return respond(Domain);
    if (url === '/api/v1/knowledge/domains') return respond(page([Domain]));
    if (url.endsWith('/knowledge/groups')) return respond(page([]));
    if (url.endsWith('/knowledge/registrations')) {
      CurrentRegistration = { ...Registration, ...config.data };
      return respond(CurrentRegistration, true, 201);
    }
    if (url.endsWith(`/registrations/${Registration.registration_id}`))
      return respond(CurrentRegistration, true);
    if (url.endsWith('/previews') && config.method === 'post') {
      if (Mode === 'deny')
        throw {
          response: {
            status: DeniedStatus,
            data: { code: 'knowledge_not_found' },
          },
        };
      Text = config.data.text;
      PreviewCount += 1;
      Preview = makePreview(
        config.data,
        (PreviewCount === 1 ? '8' : 'b').repeat(32),
      );
      if (
        Mode === 'unchanged' ||
        (Mode === 'conflict' && ConflictScenario === 'revision-changed')
      ) {
        Preview.items[0].action = Mode === 'unchanged' ? 'unchanged' : 'update';
        Preview.items[0].source_id = Source.source_id;
        Preview.items[0].expected_revision_id = Source.source_revision_id;
      }
      if (Mode === 'conflict' && ConflictScenario === 'expired')
        Preview.expires_at = '2020-01-01T00:00:00Z';
      return respond(Preview, true, 201);
    }
    if (/\/previews\/[a-f0-9]+$/.test(url)) return respond(Preview, true);
    if (url.endsWith('/imports') && config.method === 'post') {
      Imports.push(JSON.parse(JSON.stringify(config.data)));
      if (Mode === 'conflict')
        throw {
          response: {
            status: 409,
            data: {
              code: 'knowledge_conflict',
              message: `Unstable ${ConflictScenario} text`,
            },
          },
        };
      if (Mode === 'timeout-before' && Imports.length === 1)
        throw { code: 'ECONNABORTED' };
      StoredReceipt = makeReceipt(
        Preview,
        Mode === 'failed-first' && Imports.length === 1,
      );
      Preview.consumed_task_id = StoredReceipt.import_task_id;
      if (Mode === 'timeout-after' && Imports.length === 1)
        throw { code: 'ECONNABORTED' };
      return respond(StoredReceipt, true, 201);
    }
    if (/\/imports\/[a-f0-9]+$/.test(url)) return respond(StoredReceipt, true);
    const original = {
      ...Source,
      relative_path: Preview?.items[0].relative_path || Source.relative_path,
      group_id: Preview?.items[0].group_id ?? null,
      component_side: Preview?.items[0].component_side || Source.component_side,
      language: Preview?.items[0].language || '',
      sha256: Preview?.items[0].sha256 || Source.sha256,
      byte_length: Preview?.items[0].byte_length ?? Source.byte_length,
      is_current: !NewCatalogueRevision,
    };
    if (url.endsWith('/knowledge/sources'))
      return respond(
        page([
          {
            ...original,
            source_revision_id: NewCatalogueRevision
              ? 'f'.repeat(32)
              : original.source_revision_id,
          },
        ]),
      );
    if (url.endsWith(`/knowledge/sources/${Source.source_revision_id}`))
      return respond(original);
    if (url.endsWith('/knowledge/nodes')) return respond(page([]));
    if (url.endsWith(`/knowledge/nodes/${Root.node_id}`))
      return respond({ ...Root, end_byte: original.byte_length });
    if (url.endsWith(`/knowledge/nodes/${Root.node_id}/content`))
      return respond({
        node_id: Root.node_id,
        source_id: Source.source_id,
        source_revision_id: Source.source_revision_id,
        source_sha256: original.sha256,
        start_byte: config.params.start_byte,
        end_byte: Math.min(config.params.end_byte, original.byte_length),
        text: Buffer.from(Text)
          .subarray(config.params.start_byte, config.params.end_byte)
          .toString('utf8'),
        encoding: 'utf-8',
      });
    throw Error(`Unexpected synthetic request ${config.method} ${url}`);
  });
});

function renderImport() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/dataset/knowledge-import/${DomainId}`]}>
        <Routes>
          <Route
            path="/dataset/knowledge-import/:id"
            element={<KnowledgeImport />}
          />
          <Route
            path="/dataset/knowledge-catalogue/:id"
            element={<KnowledgeCatalogue />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function registerSource() {
  const keyInput = await screen.findByLabelText(
    'knowledgeImport.registrationKey',
  );
  // HTML pattern uses the Unicode-sets v flag in modern browsers, unlike Ajv.
  expect(
    new RegExp(keyInput.getAttribute('pattern')!, 'v').test('synthetic-ui'),
  ).toBe(true);
  fireEvent.change(keyInput, { target: { value: 'synthetic-ui' } });
  fireEvent.change(screen.getByLabelText('knowledgeImport.registrationName'), {
    target: { value: 'Synthetic private source' },
  });
  fireEvent.change(screen.getByLabelText('knowledgeImport.includePaths'), {
    target: { value: Source.relative_path },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.register' }),
  );
  await screen.findByRole('button', { name: 'knowledgeImport.loadSample' });
}
async function previewSample() {
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.loadSample' }),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.preview' }),
  );
  await screen.findByRole('button', { name: 'knowledgeImport.confirm' });
}
function explicitlyConfirm() {
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  );
}

it('registers, previews and waits for explicit consent, then displays saved/catalogued/disabled states separately', async () => {
  renderImport();
  await registerSource();
  expect(Imports).toHaveLength(0);
  for (const side of ['frontend', 'backend', 'shared', 'cli', 'non_code'])
    expect(
      screen.getByRole('option', { name: `knowledgeCatalogue.${side}` }),
    ).toBeInTheDocument();
  await previewSample();
  expect(Imports).toHaveLength(0);
  expect(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  ).toBeDisabled();
  explicitlyConfirm();
  await screen.findByRole('heading', { name: 'knowledgeImport.succeeded' });
  expect(Imports).toHaveLength(1);
  expect(screen.getByText('knowledgeImport.catalogued')).toBeInTheDocument();
  expect(screen.getAllByText('knowledgeImport.disabled')).toHaveLength(2);
  expect(screen.getByText(/HTTP 201/)).toBeInTheDocument();
  expect(
    screen.getByRole('link', { name: 'knowledgeImport.readRevision' }),
  ).toHaveAttribute(
    'href',
    expect.stringContaining(`revision=${Source.source_revision_id}`),
  );
});

it('invalidates the preview and consent when text or classification changes', async () => {
  renderImport();
  await registerSource();
  await previewSample();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.change(screen.getByLabelText('knowledgeImport.localText'), {
    target: { value: 'changed synthetic\n' },
  });
  expect(
    screen.queryByRole('button', { name: 'knowledgeImport.confirm' }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.preview' }),
  );
  await screen.findByRole('checkbox');
  expect(screen.getByRole('checkbox')).not.toBeChecked();
  fireEvent.change(screen.getByLabelText('knowledgeImport.componentSide'), {
    target: { value: 'cli' },
  });
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  expect(Imports).toHaveLength(0);
});

it('treats HTTP 201 failed as a failed import, then requires a new preview, new key and new consent', async () => {
  Mode = 'failed-first';
  renderImport();
  await registerSource();
  await previewSample();
  explicitlyConfirm();
  await screen.findByRole('heading', { name: 'knowledgeImport.failed' });
  expect(
    screen.queryByRole('link', { name: 'knowledgeImport.readRevision' }),
  ).not.toBeInTheDocument();
  expect(screen.getByText(/HTTP 201/)).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.retryPreview' }),
  );
  await screen.findByRole('button', { name: 'knowledgeImport.loadSample' });
  expect(Imports).toHaveLength(1);
  await previewSample();
  expect(screen.getByRole('checkbox')).not.toBeChecked();
  explicitlyConfirm();
  await screen.findByRole('heading', { name: 'knowledgeImport.succeeded' });
  expect(PreviewCount).toBe(2);
  expect((Imports[0] as { idempotency_key: string }).idempotency_key).not.toBe(
    (Imports[1] as { idempotency_key: string }).idempotency_key,
  );
});

it('retains the original key through timeout and remount and replays only on a user result check', async () => {
  Mode = 'timeout-before';
  const first = renderImport();
  await registerSource();
  await previewSample();
  explicitlyConfirm();
  await screen.findByText('knowledgeImport.pending');
  expect(Imports).toHaveLength(1);
  expect(
    screen.queryByRole('button', { name: 'knowledgeImport.retryPreview' }),
  ).not.toBeInTheDocument();
  first.unmount();
  (getAuthorization as jest.Mock).mockReturnValue('synthetic-renewed-session');
  renderImport();
  const check = await screen.findByRole('button', {
    name: 'knowledgeImport.checkResult',
  });
  expect(Imports).toHaveLength(1);
  fireEvent.click(check);
  await screen.findByRole('heading', { name: 'knowledgeImport.succeeded' });
  expect(Imports).toHaveLength(2);
  expect(Imports[1]).toEqual(Imports[0]);
});

it('recovers a consumed preview by reading its receipt without a replacement POST', async () => {
  Mode = 'timeout-after';
  renderImport();
  await registerSource();
  await previewSample();
  explicitlyConfirm();
  await screen.findByText('knowledgeImport.pending');
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.checkResult' }),
  );
  await screen.findByRole('heading', { name: 'knowledgeImport.succeeded' });
  expect(Imports).toHaveLength(1);
  expect(screen.getByText(/HTTP 200/)).toBeInTheDocument();
});

it.each(['expired', 'revision-changed'])(
  'uses generic %s conflict handling and verifies consumption before allowing a fresh preview',
  async (scenario) => {
    Mode = 'conflict';
    ConflictScenario = scenario;
    renderImport();
    await registerSource();
    await previewSample();
    explicitlyConfirm();
    await screen.findByText('knowledgeImport.conflict');
    expect(
      screen.queryByRole('button', { name: 'knowledgeImport.retryPreview' }),
    ).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeImport.checkResult' }),
    );
    await screen.findByRole('button', { name: 'knowledgeImport.retryPreview' });
    expect(Imports).toHaveLength(2);
    expect(Imports[1]).toEqual(Imports[0]);
    expect(
      screen.queryByText(`Unstable ${scenario} text`),
    ).not.toBeInTheDocument();
  },
);

it.each([401, 404])(
  'hides all private names and original text after HTTP %i',
  async (status) => {
    Mode = 'deny';
    DeniedStatus = status;
    renderImport();
    await registerSource();
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeImport.loadSample' }),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeImport.preview' }),
    );
    await screen.findByText('knowledgeImport.notVisible');
    expect(
      screen.queryByText('Synthetic private source', { exact: false }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(Domain.name, { exact: false }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText('knowledgeImport.localText'),
    ).not.toBeInTheDocument();
    expect(Imports).toHaveLength(0);
  },
  10000,
);

it('shows unchanged as the existing revision and keeps indexes disabled', async () => {
  Mode = 'unchanged';
  renderImport();
  await registerSource();
  await previewSample();
  explicitlyConfirm();
  await screen.findByRole('heading', { name: 'knowledgeImport.succeeded' });
  expect(screen.getByText('knowledgeImport.unchangedHint')).toBeInTheDocument();
  expect(screen.getAllByText('knowledgeImport.disabled')).toHaveLength(2);
  expect(
    screen.getByRole('link', { name: 'knowledgeImport.readRevision' }),
  ).toHaveAttribute(
    'href',
    expect.stringContaining(`revision=${Source.source_revision_id}`),
  );
});

it('stops mutations when the backend still declares ReviewCandidate', async () => {
  CapabilityStatus = 'ReviewCandidate';
  renderImport();
  await screen.findByText('knowledgeImport.versionMismatch');
  expect(
    screen.queryByLabelText('knowledgeImport.registrationKey'),
  ).not.toBeInTheDocument();
  expect(
    Http.mock.calls.filter(([config]) => config.method === 'post'),
  ).toHaveLength(0);
});

it('reads the receipt’s fixed revision after catalogue refresh even when the current revision changes', async () => {
  renderImport();
  await registerSource();
  await previewSample();
  explicitlyConfirm();
  fireEvent.click(
    await screen.findByRole('link', { name: 'knowledgeImport.readRevision' }),
  );
  await screen.findByTestId('knowledge-original');
  const original = screen.getByTestId('knowledge-original').textContent;
  NewCatalogueRevision = true;
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeCatalogue.refresh' }),
  );
  await waitFor(() =>
    expect(screen.getByTestId('knowledge-original').textContent).toBe(original),
  );
  expect(
    Http.mock.calls
      .filter(([config]) => /\/knowledge\/sources\/[^/]+$/.test(config.url))
      .every(([config]) => config.url.endsWith(Source.source_revision_id)),
  ).toBe(true);
  expect(
    screen.getByText('knowledgeImport.pinnedHint', { exact: false }),
  ).toBeInTheDocument();
});

it('accepts an empty UTF-8 file and rejects malformed bytes without converting them', async () => {
  renderImport();
  await registerSource();
  fireEvent.change(screen.getByLabelText('knowledgeImport.file'), {
    target: { files: [new File([new Uint8Array([255])], 'synthetic.txt')] },
  });
  await screen.findByText('knowledgeImport.unsupported');
  expect(PreviewCount).toBe(0);
  fireEvent.change(screen.getByLabelText('knowledgeImport.file'), {
    target: { files: [new File([], 'empty.md')] },
  });
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'knowledgeImport.preview' }),
    ).not.toBeDisabled(),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.preview' }),
  );
  await screen.findByRole('checkbox');
  expect(Preview.items[0].byte_length).toBe(0);
});

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import {
  QueryClient,
  QueryClientProvider,
  focusManager,
} from '@tanstack/react-query';
import { webcrypto, createHash } from 'node:crypto';
import request from '@/utils/next-request';
import { getAuthorization } from '@/utils/authorization-util';
import KnowledgeImport from '..';
import KnowledgeCatalogue from '../../knowledge-catalogue';
import {
  Registration,
  Root,
  Capabilities,
  Domain,
  Domain2,
  DomainId,
  Text,
  makeObservation,
  makePreview,
  makeReceipt,
  makeListing,
  windowFor,
  respond,
  reject,
  FoundationCapabilities,
  foundationEnvelope,
  page,
  c1Projection,
  sealObservation,
} from './directory-fixture';
import { ImportCapabilities, c2Envelope } from './fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { getUserInfoObject: () => ({ id: 'owner' }) },
  getAuthorization: jest.fn(() => 'synthetic-directory-session'),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
const Http = request as unknown as jest.Mock;
let Mode = 'success',
  Deny = false,
  RegFailure = 0,
  RootReady = true,
  CapabilityStatus = 'Frozen',
  ListingCount = 2;
let CurrentRegistration: import('@/interfaces/database/knowledge-directory').DirectoryRegistration,
  Observation: import('@/interfaces/database/knowledge-directory').DirectoryObservation,
  Preview: import('@/interfaces/database/knowledge-directory').DirectoryPreview,
  Receipt:
    | import('@/interfaces/database/knowledge-directory').DirectoryReceipt
    | null;
let Posts: import('@/interfaces/request/knowledge-directory').DirectoryCommitRequest[],
  RegistrationReads: number,
  ObservationCount: number,
  RegistrationPosts: number;
let Snapshot: import('@/interfaces/database/knowledge-directory').DirectoryListing,
  CurrentRevision = false;
let BodyText = Text;
beforeAll(() => {
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  });
  focusManager.setFocused(false);
});
afterAll(() => focusManager.setFocused(undefined));
beforeEach(() => {
  (getAuthorization as jest.Mock).mockReturnValue(
    'synthetic-directory-session',
  );
  sessionStorage.clear();
  Mode = 'success';
  Deny = false;
  RegFailure = 0;
  RootReady = true;
  CapabilityStatus = 'Frozen';
  ListingCount = 2;
  BodyText = Text;
  CurrentRegistration = { ...Registration };
  Observation = makeObservation();
  Preview = makePreview(Observation);
  Receipt = null;
  Posts = [];
  RegistrationReads = 0;
  ObservationCount = 0;
  RegistrationPosts = 0;
  Snapshot = makeListing();
  CurrentRevision = false;
  Http.mockReset().mockImplementation(async (config) => {
    const url: string = config.url,
      body =
        typeof config.data === 'string' ? JSON.parse(config.data) : config.data;
    if (url === '/api/v1/knowledge/directory-import-contract')
      return respond({ ...Capabilities, status: CapabilityStatus });
    if (url === '/api/v1/knowledge/import-contract')
      return { status: 200, data: c2Envelope(ImportCapabilities) };
    if (url === '/api/v1/knowledge/contract')
      return { status: 200, data: foundationEnvelope(FoundationCapabilities) };
    if (url === '/api/v1/knowledge/domains')
      return { status: 200, data: foundationEnvelope(page([Domain, Domain2])) };
    if (url.endsWith('/knowledge/domain'))
      return {
        status: 200,
        data: foundationEnvelope(
          url.includes(Domain2.domain_id) ? Domain2 : Domain,
        ),
      };
    if (url.endsWith('/knowledge/groups'))
      return { status: 200, data: foundationEnvelope(page([])) };
    if (url.endsWith('/directory-roots'))
      return respond({
        domain_id: url.includes(Domain2.domain_id)
          ? Domain2.domain_id
          : DomainId,
        items: [
          {
            ...Root,
            ready: RootReady,
            reason: RootReady ? null : 'unavailable',
          },
        ],
      });
    if (url.endsWith('/directory-registrations')) {
      RegistrationPosts += 1;
      CurrentRegistration = {
        ...Registration,
        ...body,
        domain_id: url.split('/')[4],
      };
      return respond(CurrentRegistration, 201);
    }
    if (
      url.endsWith('/directory-registrations/' + Registration.registration_id)
    ) {
      RegistrationReads += 1;
      if (RegFailure === 409)
        return {
          status: 200,
          data: {
            ...respond(CurrentRegistration).data,
            source_contract_version: '0.2.0',
          },
        };
      if (RegFailure)
        throw reject(
          RegFailure,
          RegFailure === 401
            ? 'knowledge_directory_invalid'
            : 'knowledge_directory_not_found',
        );
      return respond(CurrentRegistration);
    }
    if (url.endsWith('/listings')) {
      Snapshot = makeListing(ListingCount);
      return respond(
        {
          ...Snapshot,
          items: Snapshot.items.slice(0, 50),
          next_cursor: Snapshot.items.length > 50 ? 'page-2' : null,
        },
        201,
      );
    }
    if (url.includes('/listings/'))
      return respond({
        ...Snapshot,
        items: Snapshot.items.slice(50),
        next_cursor: null,
      });
    if (url.endsWith('/observations')) {
      ObservationCount += 1;
      Observation = makeObservation(
        body.selected_paths,
        20 + ObservationCount,
        CurrentRegistration,
      );
      for (const item of Observation.items)
        if (item.eligible)
          Object.assign(item, {
            byte_length: Buffer.byteLength(BodyText, 'utf8'),
            sha256: createHash('sha256').update(BodyText).digest('hex'),
          });
      sealObservation(Observation);
      return respond(Observation, 201);
    }
    if (url.endsWith('/previews')) {
      Preview = makePreview(Observation, body, 40 + ObservationCount);
      return respond(Preview, 201);
    }
    if (url.endsWith('/content') && url.includes('/previews/')) {
      try {
        return respond(
          windowFor(
            Preview,
            config.params.offset,
            config.params.length,
            BodyText,
          ),
        );
      } catch {
        throw reject(416, 'knowledge_directory_range');
      }
    }
    if (url.endsWith('/previews/' + Preview.preview_id))
      return respond({
        ...Preview,
        consumed_task_id: Receipt?.import_task_id || null,
      });
    if (url.endsWith('/imports')) {
      Posts.push(body);
      if (Mode === 'expired') throw reject(410, 'knowledge_directory_expired');
      if (Mode === 'timeout-before')
        throw new Error('Synthetic transport loss');
      Receipt = makeReceipt(Preview, Mode === 'failed');
      if (Mode === 'timeout-after')
        throw new Error('Synthetic response lost after commit');
      return respond(Receipt, 201);
    }
    if (url.includes('/imports/by-key/')) {
      if (Deny || !Receipt) throw reject(404, 'knowledge_directory_not_found');
      return respond(Receipt);
    }
    if (url.includes('/imports/')) return respond(Receipt);
    if (url.endsWith('/knowledge/sources')) {
      const projection = c1Projection(Receipt || makeReceipt(Preview));
      return {
        status: 200,
        data: foundationEnvelope(
          page([
            {
              ...projection.source,
              source_revision_id: CurrentRevision
                ? 'f'.repeat(32)
                : projection.source.source_revision_id,
            },
          ]),
        ),
      };
    }
    if (url.includes('/knowledge/sources/'))
      return {
        status: 200,
        data: foundationEnvelope(
          c1Projection(Receipt || makeReceipt(Preview)).source,
        ),
      };
    if (url.includes('/knowledge/nodes/') && url.endsWith('/content')) {
      const { source, root } = c1Projection(Receipt || makeReceipt(Preview));
      return {
        status: 200,
        data: foundationEnvelope({
          node_id: root.node_id,
          source_id: source.source_id,
          source_revision_id: source.source_revision_id,
          source_sha256: source.sha256,
          start_byte: 0,
          end_byte: source.byte_length,
          encoding: 'utf-8',
          text: Text,
        }),
      };
    }
    if (url.includes('/knowledge/nodes/'))
      return {
        status: 200,
        data: foundationEnvelope(
          c1Projection(Receipt || makeReceipt(Preview)).root,
        ),
      };
    if (url.endsWith('/knowledge/nodes'))
      return { status: 200, data: foundationEnvelope(page([])) };
    throw new Error('Unexpected mocked request ' + url);
  });
});
function LocationProbe() {
  const location = useLocation();
  return (
    <output data-testid="location">
      {location.pathname + location.search}
    </output>
  );
}
function mount(
  path = '/dataset/knowledge-import/' + DomainId + '?mode=directory',
) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <LocationProbe />
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
async function register() {
  await screen.findByLabelText('knowledgeDirectory.root');
  fireEvent.change(screen.getByLabelText('knowledgeDirectory.root'), {
    target: { value: Root.root_id },
  });
  fireEvent.change(screen.getByLabelText('knowledgeImport.registrationKey'), {
    target: { value: Registration.registration_key },
  });
  fireEvent.change(screen.getByLabelText('knowledgeImport.registrationName'), {
    target: { value: Registration.name },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.register' }),
  );
  await screen.findByLabelText('knowledgeDirectory.exactSelection');
}
async function preview(paths = 'web/card.tsx') {
  fireEvent.change(screen.getByLabelText('knowledgeDirectory.exactSelection'), {
    target: { value: paths },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.observe' }),
  );
  const choice = await screen.findByLabelText('knowledgeDirectory.oneFile');
  fireEvent.change(choice, { target: { value: 'web/card.tsx' } });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.preview' }),
  );
  await screen.findByTestId('directory-preview-text');
}
async function save() {
  fireEvent.click(screen.getByLabelText('knowledgeDirectory.consent'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  );
}
async function recoveredReceipt(failed = false) {
  const view = mount();
  await register();
  await preview();
  Mode = 'timeout-after';
  await save();
  await screen.findByText('knowledgeDirectory.pending');
  view.unmount();
  if (failed) Receipt = makeReceipt(Preview, true);
  const next = mount();
  await screen.findByRole('button', { name: 'knowledgeImport.checkResult' });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.checkResult' }),
  );
  await screen.findByText(
    failed ? 'knowledgeImport.failed' : 'knowledgeImport.succeeded',
  );
  return next;
}
it('UI-01 restores the fixed registration after reload and a successful GET receipt before observing again', async () => {
  await recoveredReceipt();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await waitFor(() => expect(RegistrationReads).toBe(1));
  expect(
    await screen.findByLabelText('knowledgeDirectory.exactSelection'),
  ).toBeVisible();
  expect(
    screen.queryByLabelText('knowledgeImport.registrationKey'),
  ).not.toBeInTheDocument();
  expect(RegistrationPosts).toBe(1);
  expect(Posts).toHaveLength(1);
  expect(sessionStorage.length).toBe(0);
});
it('UI-01 restores fixed context after reload and a failed receipt', async () => {
  await recoveredReceipt(true);
  expect(
    screen.queryByRole('link', { name: 'knowledgeImport.readRevision' }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await waitFor(() => expect(RegistrationReads).toBe(1));
  expect(
    await screen.findByLabelText('knowledgeDirectory.exactSelection'),
  ).toBeVisible();
  expect(ObservationCount).toBe(1);
  expect(Posts).toHaveLength(1);
});
it('UI-01 retains original metadata and hides private content if registration context cannot be authorized', async () => {
  await recoveredReceipt();
  const stored = sessionStorage.getItem(sessionStorage.key(0)!);
  RegFailure = 404;
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await screen.findByText('knowledgeDirectory.errors.notVisible');
  expect(sessionStorage.getItem(sessionStorage.key(0)!)).toBe(stored);
  expect(screen.queryByText(Registration.name)).not.toBeInTheDocument();
  expect(Posts).toHaveLength(1);
  expect(RegistrationPosts).toBe(1);
});
it('UI-02 uses a directory-specific registered prompt after previous saved sources', async () => {
  mount();
  await register();
  expect(screen.getByText(/knowledgeDirectory.registered/)).toBeVisible();
  expect(
    screen.queryByText(/knowledgeImport.registered/),
  ).not.toBeInTheDocument();
});

it.each([401, 409])(
  'UI-01 retains metadata and hides the receipt when restoring registration fails with %s',
  async (status) => {
    await recoveredReceipt();
    const stored = sessionStorage.getItem(sessionStorage.key(0)!);
    RegFailure = status;
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
    );
    await screen.findByText(
      status === 401
        ? 'knowledgeDirectory.errors.notVisible'
        : 'knowledgeDirectory.errors.versionMismatch',
    );
    expect(sessionStorage.getItem(sessionStorage.key(0)!)).toBe(stored);
    expect(
      screen.queryByRole('link', { name: 'knowledgeImport.readRevision' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText('knowledgeDirectory.exactSelection'),
    ).not.toBeInTheDocument();
    expect(Posts).toHaveLength(1);
    expect(RegistrationPosts).toBe(1);
  },
);
it('keeps inactive mode requests lazy and preserves directory mode when changing domain', async () => {
  mount('/dataset/knowledge-import/' + DomainId);
  await screen.findByLabelText('knowledgeImport.registrationKey');
  expect(
    Http.mock.calls.some(([config]) => config.url.includes('/directory-')),
  ).toBe(false);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.title' }),
  );
  await screen.findByLabelText('knowledgeDirectory.root');
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.chooseDomain' }),
  );
  fireEvent.click(await screen.findByRole('link', { name: Domain2.name }));
  expect(await screen.findByTestId('location')).toHaveTextContent(
    '/' + Domain2.domain_id + '?mode=directory',
  );
  await screen.findByLabelText('knowledgeDirectory.root');
  expect(screen.queryByText(Registration.name)).not.toBeInTheDocument();
});
it('browses 50 plus 1 fixed metadata entries without automatically selecting or observing them', async () => {
  ListingCount = 51;
  mount();
  await register();
  fireEvent.change(
    screen.getByLabelText('knowledgeDirectory.relativeDirectory'),
    { target: { value: 'web' } },
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.browse' }),
  );
  await screen.findByLabelText('web/file-049.ts');
  expect(screen.queryByLabelText('web/file-050.ts')).not.toBeInTheDocument();
  expect(
    screen.getByLabelText('knowledgeDirectory.exactSelection'),
  ).toHaveValue('');
  expect(ObservationCount).toBe(0);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeCatalogue.more' }),
  );
  await screen.findByLabelText('web/file-050.ts');
  expect(
    Http.mock.calls.filter(
      ([config]) =>
        config.url.includes('/listings/') && config.method === 'get',
    ),
  ).toHaveLength(1);
  fireEvent.click(screen.getByLabelText('web/card.tsx'));
  expect(
    screen.getByLabelText('knowledgeDirectory.exactSelection'),
  ).toHaveValue('web/card.tsx');
  expect(ObservationCount).toBe(0);
});
it('accepts exactly 32 selected paths, blocks 33 and does not add newly browsed files', async () => {
  mount();
  await register();
  const paths = Array.from(
    { length: 32 },
    (_, i) => 'web/file-' + String(i).padStart(2, '0') + '.ts',
  );
  fireEvent.change(screen.getByLabelText('knowledgeDirectory.exactSelection'), {
    target: { value: [...paths, 'web/extra.ts'].join('\n') },
  });
  expect(
    screen.getByRole('button', { name: 'knowledgeDirectory.observe' }),
  ).toBeDisabled();
  expect(ObservationCount).toBe(0);
  fireEvent.change(screen.getByLabelText('knowledgeDirectory.exactSelection'), {
    target: { value: paths.join('\n') },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.observe' }),
  );
  await screen.findByText('knowledgeDirectory.observationComplete');
  expect(Observation.selected_paths).toEqual(paths);
  expect(ObservationCount).toBe(1);
  expect(Posts).toHaveLength(0);
});
it('allows a complete observation containing an ineligible PDF but previews only one eligible text file', async () => {
  mount();
  await register();
  await preview('web/card.tsx\nweb/readme.pdf');
  expect(Observation.complete).toBe(true);
  expect(
    screen.queryByRole('option', { name: 'web/readme.pdf' }),
  ).not.toBeInTheDocument();
  expect(Preview.items).toHaveLength(1);
  expect(screen.getByTestId('directory-preview-text').textContent).toBe(Text);
  expect(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  ).toBeDisabled();
  expect(Posts).toHaveLength(0);
});
it('blocks all previews from an incomplete exact observation containing a missing selected path', async () => {
  mount();
  await register();
  fireEvent.change(screen.getByLabelText('knowledgeDirectory.exactSelection'), {
    target: { value: 'web/card.tsx\nweb/missing.ts' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.observe' }),
  );
  await screen.findByText('knowledgeDirectory.observationIncomplete');
  expect(
    screen.queryByLabelText('knowledgeDirectory.oneFile'),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: 'knowledgeImport.preview' }),
  ).not.toBeInTheDocument();
  expect(Posts).toHaveLength(0);
});
it('invalidates staged text and consent after classification or selection changes', async () => {
  mount();
  await register();
  await preview();
  fireEvent.click(screen.getByLabelText('knowledgeDirectory.consent'));
  expect(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  ).toBeEnabled();
  fireEvent.change(screen.getByLabelText('knowledgeImport.language'), {
    target: { value: 'typescript' },
  });
  expect(
    screen.queryByTestId('directory-preview-text'),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.preview' }),
  );
  await screen.findByTestId('directory-preview-text');
  expect(screen.getByLabelText('knowledgeDirectory.consent')).not.toBeChecked();
  expect(Preview.items[0].classification.language).toBe('typescript');
  fireEvent.change(screen.getByLabelText('knowledgeDirectory.exactSelection'), {
    target: { value: 'web/new.ts' },
  });
  expect(
    screen.queryByTestId('directory-preview-text'),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByLabelText('knowledgeDirectory.oneFile'),
  ).not.toBeInTheDocument();
  expect(Posts).toHaveLength(0);
});
it('displays a synchronous 201 failed receipt without reading links and requires a new observation and consent', async () => {
  mount();
  await register();
  await preview();
  Mode = 'failed';
  await save();
  await screen.findByText('knowledgeImport.failed');
  expect(screen.getByText(/HTTP 201/)).toBeVisible();
  expect(
    screen.queryByRole('link', { name: 'knowledgeImport.readRevision' }),
  ).not.toBeInTheDocument();
  expect(Receipt!.items[0]).toMatchObject({
    source_id: null,
    source_revision_id: null,
    root_node_id: null,
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await screen.findByLabelText('knowledgeDirectory.exactSelection');
  expect(RegistrationReads).toBe(1);
  expect(Posts).toHaveLength(1);
  Mode = 'success';
  await preview();
  expect(ObservationCount).toBe(2);
  expect(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  ).toBeDisabled();
});
it('recovers a lost committed response by GET with the root offline and without mounting the root picker', async () => {
  const view = mount();
  await register();
  await preview();
  Mode = 'timeout-after';
  await save();
  await screen.findByText('knowledgeDirectory.pending');
  const original = Posts[0];
  view.unmount();
  RootReady = false;
  Http.mockClear();
  mount();
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeImport.checkResult' }),
  );
  await screen.findByText('knowledgeImport.succeeded');
  expect(Posts).toEqual([original]);
  expect(RegistrationReads).toBe(0);
  expect(
    Http.mock.calls.some(([config]) => config.url.endsWith('/directory-roots')),
  ).toBe(false);
  expect(
    screen.queryByLabelText('knowledgeDirectory.root'),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await screen.findByLabelText('knowledgeDirectory.exactSelection');
  expect(RegistrationReads).toBe(1);
  expect(RegistrationPosts).toBe(1);
  expect(
    Http.mock.calls.some(([config]) => config.url.endsWith('/directory-roots')),
  ).toBe(false);
});
it('stops on indistinguishable by-key 404, retains the original metadata, then reauthorizes and recovers by GET', async () => {
  const view = mount();
  await register();
  await preview();
  Mode = 'timeout-after';
  await save();
  await screen.findByText('knowledgeDirectory.pending');
  const stored = sessionStorage.getItem(sessionStorage.key(0)!);
  view.unmount();
  Deny = true;
  mount();
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeImport.checkResult' }),
  );
  await screen.findByText('knowledgeDirectory.errors.notVisible');
  expect(
    screen.queryByTestId('directory-preview-text'),
  ).not.toBeInTheDocument();
  expect(screen.queryByText(Registration.name)).not.toBeInTheDocument();
  expect(sessionStorage.getItem(sessionStorage.key(0)!)).toBe(stored);
  expect(Posts).toHaveLength(1);
  Deny = false;
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.reload' }),
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeImport.checkResult' }),
  );
  await screen.findByText('knowledgeImport.succeeded');
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await screen.findByLabelText('knowledgeDirectory.exactSelection');
  expect(RegistrationReads).toBe(1);
  expect(RegistrationPosts).toBe(1);
  expect(Posts).toHaveLength(1);
});
it('resends only on a separate explicit action, retaining exactly the original key, manifest and preview', async () => {
  const view = mount();
  await register();
  await preview();
  Mode = 'timeout-before';
  await save();
  await screen.findByText('knowledgeDirectory.pending');
  const original = { ...Posts[0] };
  view.unmount();
  mount();
  await screen.findByRole('button', { name: 'knowledgeImport.checkResult' });
  expect(Posts).toHaveLength(1);
  expect(
    screen.getByRole('button', { name: 'knowledgeDirectory.resend' }),
  ).toBeDisabled();
  fireEvent.click(screen.getByLabelText('knowledgeDirectory.resendConsent'));
  Mode = 'success';
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.resend' }),
  );
  await screen.findByText('knowledgeImport.succeeded');
  expect(Posts).toEqual([original, original]);
});
it('restores fixed registration after reload and expiration of the explicitly resent original request', async () => {
  const view = mount();
  await register();
  await preview();
  Mode = 'timeout-before';
  await save();
  await screen.findByText('knowledgeDirectory.pending');
  const original = { ...Posts[0] };
  view.unmount();
  mount();
  await screen.findByRole('button', { name: 'knowledgeImport.checkResult' });
  Mode = 'expired';
  fireEvent.click(screen.getByLabelText('knowledgeDirectory.resendConsent'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.resend' }),
  );
  await screen.findByText('knowledgeDirectory.errors.expired');
  expect(sessionStorage.length).toBe(1);
  expect(Posts).toEqual([original, original]);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDirectory.newObservation' }),
  );
  await screen.findByLabelText('knowledgeDirectory.exactSelection');
  expect(RegistrationReads).toBe(1);
  expect(RegistrationPosts).toBe(1);
  expect(ObservationCount).toBe(1);
  expect(sessionStorage.length).toBe(0);
  expect(Posts).toEqual([original, original]);
});
it('keeps a pending original request isolated when switching to another domain', async () => {
  mount();
  await register();
  await preview();
  Mode = 'timeout-before';
  await save();
  await screen.findByText('knowledgeDirectory.pending');
  const stored = sessionStorage.getItem(sessionStorage.key(0)!);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeImport.chooseDomain' }),
  );
  fireEvent.click(await screen.findByRole('link', { name: Domain2.name }));
  await screen.findByLabelText('knowledgeDirectory.root');
  expect(
    screen.queryByText('knowledgeDirectory.pending'),
  ).not.toBeInTheDocument();
  expect(sessionStorage.getItem(sessionStorage.key(0)!)).toBe(stored);
  expect(Posts).toHaveLength(1);
});
it('links the saved receipt to its fixed C1 revision and reads it through existing C1 endpoints only', async () => {
  mount();
  await register();
  await preview();
  await save();
  await screen.findByText('knowledgeImport.succeeded');
  const link = screen.getByRole('link', {
    name: 'knowledgeImport.readRevision',
  });
  const href = link.getAttribute('href')!;
  expect(href).toContain(Receipt!.items[0].source_revision_id!);
  expect(href).toContain(Receipt!.items[0].root_node_id!);
  Http.mockClear();
  CurrentRevision = true;
  fireEvent.click(link);
  const original = await screen.findByRole('region', {
    name: 'knowledgeCatalogue.original',
  });
  expect(within(original).getByTestId('knowledge-original').textContent).toBe(
    Text,
  );
  expect(
    Http.mock.calls.filter(([config]) => config.url.endsWith('/content')),
  ).toHaveLength(1);
  for (const [config] of Http.mock.calls) {
    expect(config.method).toBe('get');
    expect(
      config.headers['X-Knowledge-Source-Contract-Version'],
    ).toBeUndefined();
  }
});

it('reads a 65546-byte synthetic original in UTF-8-safe fixed windows with next and previous GET operations', async () => {
  BodyText = '\uFEFF' + 'a'.repeat(65532) + '🧠\r\n尾xx';
  expect(Buffer.byteLength(BodyText)).toBe(65546);
  mount();
  await register();
  await preview();
  expect(screen.getByTestId('directory-preview-text').textContent).toBe(
    BodyText.slice(0, 65533),
  );
  expect(
    screen.getByRole('button', { name: 'knowledgeCatalogue.previous' }),
  ).toBeDisabled();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeCatalogue.next' }),
  );
  await waitFor(() =>
    expect(screen.getByTestId('directory-preview-text').textContent).toBe(
      '🧠\r\n尾xx',
    ),
  );
  expect(
    screen.getByRole('button', { name: 'knowledgeCatalogue.next' }),
  ).toBeDisabled();
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeCatalogue.previous' }),
  );
  await waitFor(() =>
    expect(screen.getByTestId('directory-preview-text').textContent).toBe(
      BodyText.slice(0, 65533),
    ),
  );
  const reads = Http.mock.calls.filter(
    ([config]) =>
      config.url.includes('/previews/') && config.url.endsWith('/content'),
  );
  expect(reads.map(([config]) => config.params)).toEqual([
    { offset: 0, length: 65536 },
    { offset: 0, length: 65535 },
    { offset: 65535, length: 11 },
    { offset: 0, length: 65536 },
    { offset: 0, length: 65535 },
  ]);
  expect(reads.every(([config]) => config.method === 'get')).toBe(true);
  expect(Posts).toHaveLength(0);
});
it('previews an empty original without requesting a zero-length content range', async () => {
  BodyText = '';
  mount();
  await register();
  await preview();
  expect(screen.getByTestId('directory-preview-text').textContent).toBe('');
  const read = Http.mock.calls.find(
    ([config]) =>
      config.url.includes('/previews/') && config.url.endsWith('/content'),
  )![0];
  expect(read.params).toEqual({ offset: 0, length: 1 });
  expect(
    screen.getByRole('button', { name: 'knowledgeImport.confirm' }),
  ).toBeDisabled();
});
it.each([
  [413, 'knowledge_directory_too_large', null, 'tooLarge'],
  [415, 'knowledge_directory_unsupported', null, 'unsupported'],
  [429, 'knowledge_directory_capacity', null, 'capacity'],
  [503, 'knowledge_directory_unavailable', 'result_unknown', 'unavailable'],
])(
  'stops observation on HTTP %s without previewing or saving',
  async (status, code, reason, label) => {
    mount();
    await register();
    fireEvent.change(
      screen.getByLabelText('knowledgeDirectory.exactSelection'),
      { target: { value: 'web/card.tsx' } },
    );
    Http.mockRejectedValueOnce(
      reject(status as number, code as string, reason as string | null),
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'knowledgeDirectory.observe' }),
    );
    await screen.findByText('knowledgeDirectory.errors.' + label);
    expect(
      screen.queryByTestId('directory-preview-text'),
    ).not.toBeInTheDocument();
    expect(Posts).toHaveLength(0);
  },
);

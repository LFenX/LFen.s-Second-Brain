import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import request from '@/utils/next-request';
import OriginalImport from '../original';
import { Domain } from '../../knowledge-catalogue/__tests__/fixture';
import {
  DomainId,
  OwnerId,
  RegistrationId,
  Capabilities,
  Bytes,
  file,
  makeRegistration,
  makeUpload,
  makeReceipt,
  makeRevision,
  respond,
  binaryResponse,
  failure,
} from './original-fixture';
type OriginalUpload =
  import('@/interfaces/database/knowledge-original').OriginalUpload;
type OriginalReceipt =
  import('@/interfaces/database/knowledge-original').OriginalReceipt;
type OriginalRegistration =
  import('@/interfaces/database/knowledge-original').OriginalRegistration;
import {
  originalRegistrationRequest,
  originalStorageKey,
} from '@/utils/knowledge-original';
import { originalDigest } from '@/utils/knowledge-original';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: {
    getUserInfoObject: () => ({ name: 'Native login profile has no ID' }),
  },
  getAuthorization: () => 'synthetic-original-session',
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/ui/button', () => ({
  Button: ({ variant: _variant, ...props }: Record<string, unknown>) => {
    void _variant;
    return <button {...props} />;
  },
}));
jest.mock('@/components/file-uploader', () => ({
  FileUploader: (props: {
    onValueChange: (files: File[]) => void;
    maxSize: number;
    maxFileCount: number;
    multiple: boolean;
    showFolderTab: boolean;
    disabled?: boolean;
  }) => {
    const change = (event: React.ChangeEvent<HTMLInputElement>) =>
      props.onValueChange(Array.from(event.target.files || []));
    return (
      <input
        aria-label="synthetic-file"
        type="file"
        disabled={props.disabled}
        multiple={props.multiple}
        onChange={change}
        data-max-size={props.maxSize}
        data-max-count={props.maxFileCount}
        data-folder={String(props.showFolderTab)}
      />
    );
  },
}));
const Http = request as unknown as jest.Mock;
let reg: OriginalRegistration,
  upload: OriginalUpload | null,
  receipt: OriginalReceipt | null;
let loseCreate: boolean, loseConfirm: boolean, loseRegistration: boolean;
const NativeDomain = { ...Domain, domain_id: DomainId, owner_id: OwnerId };
function mount(search = '?mode=original') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={['/dataset/knowledge-import/' + DomainId + search]}
      >
        <Routes>
          <Route
            path="/dataset/knowledge-import/:id"
            element={<OriginalImport domain={DomainId} />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(async () => {
  localStorage.clear();
  reg = await makeRegistration();
  upload = null;
  receipt = null;
  loseCreate = false;
  loseConfirm = false;
  loseRegistration = false;
  Http.mockReset().mockImplementation(async (config) => {
    const url: string = config.url;
    const input = config.data ? JSON.parse(config.data) : undefined;
    if (url.endsWith('/original-import-contract')) return respond(Capabilities);
    if (url.endsWith('/domain')) return respond(NativeDomain);
    if (url.endsWith('/groups')) return respond({ items: [], next_cursor: '' });
    if (url.endsWith('/original-registrations') && config.method === 'post') {
      reg = await makeRegistration(input);
      if (loseRegistration) {
        loseRegistration = false;
        throw new Error('Synthetic lost registration response');
      }
      return respond(reg, 201);
    }
    if (
      url.endsWith('/original-registrations/' + RegistrationId) ||
      url.endsWith('/original-registrations/by-key')
    )
      return respond(reg);
    if (url.endsWith('/target'))
      return respond({
        domain_id: DomainId,
        registration_id: RegistrationId,
        relative_path: config.params.relative_path,
        source: null,
      });
    if (url.endsWith('/uploads') && config.method === 'post') {
      upload = await makeUpload('awaiting_bytes', {}, input);
      if (loseCreate) {
        loseCreate = false;
        throw new Error('Synthetic lost creation response');
      }
      return respond(upload, 201);
    }
    if (url.endsWith('/content') && config.method === 'put' && upload) {
      expect(config.transformRequest[0]()).toBeInstanceOf(File);
      upload = await makeUpload(
        'ready',
        {
          created_at: upload.created_at,
          updated_at: upload.updated_at,
          expires_at: upload.expires_at,
        },
        upload,
      );
      return respond(upload);
    }
    if (
      (url.endsWith('/uploads/by-key') ||
        url.endsWith('/uploads/' + upload?.upload_id)) &&
      upload
    ) {
      if (config.method === 'delete')
        upload = {
          ...upload,
          state: 'cancelled',
          revision: upload.revision + 1,
        };
      return respond(upload);
    }
    if (url.endsWith('/imports') && config.method === 'post' && upload) {
      receipt = makeReceipt(upload, input);
      if (loseConfirm) {
        loseConfirm = false;
        throw new Error('Synthetic lost confirmation response');
      }
      return respond(receipt, 201);
    }
    if (url.endsWith('/imports/by-key') && receipt) return respond(receipt);
    if (url.endsWith('/original-revisions'))
      return respond({
        domain_id: DomainId,
        items: receipt ? [receipt.source] : [],
        next_cursor: null,
      });
    if (
      url.endsWith('/original-revisions/' + receipt?.source.source_revision_id)
    )
      return respond(receipt!.source);
    if (url.endsWith('/content') && receipt)
      return binaryResponse(receipt.source);
    throw Error('Unexpected synthetic original request: ' + url);
  });
});
async function openRegistration() {
  const input = await screen.findByLabelText(
    'knowledgeOriginal.registrationId',
  );
  fireEvent.change(input, { target: { value: RegistrationId } });
  fireEvent.click(screen.getByText('knowledgeOriginal.openRegistration'));
  await screen.findByRole('form', { name: 'knowledgeOriginal.prepare' });
}
async function prepare() {
  fireEvent.change(screen.getByLabelText('synthetic-file'), {
    target: { files: [file()] },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.prepare' }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.confirmation' });
}
it('requires a single explicit upload and an initially unchecked confirmation before saving a fixed original', async () => {
  mount();
  await openRegistration();
  const chooser = screen.getByLabelText('synthetic-file');
  expect(chooser).toHaveAttribute('data-max-size', '67108864');
  expect(chooser).toHaveAttribute('data-max-count', '1');
  expect(chooser).toHaveAttribute('data-folder', 'false');
  fireEvent.change(chooser, { target: { files: [file()] } });
  expect(Http.mock.calls.some(([config]) => config.method === 'put')).toBe(
    false,
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.prepare' }),
  );
  const panel = await screen.findByRole('region', {
    name: 'knowledgeOriginal.confirmation',
  });
  expect(within(panel).getByRole('checkbox')).not.toBeChecked();
  expect(
    within(panel).getByText('knowledgeOriginal.readyHint'),
  ).toBeInTheDocument();
  expect(within(panel).getByRole('button')).toBeDisabled();
  expect(receipt).toBeNull();
  fireEvent.click(within(panel).getByRole('checkbox'));
  fireEvent.click(within(panel).getByRole('button'));
  await screen.findByRole('region', { name: 'knowledgeOriginal.receipt' });
  expect(
    screen.getByRole('link', { name: 'knowledgeOriginal.fixedDetails' }),
  ).toHaveAttribute(
    'href',
    expect.stringContaining(
      'original_revision=' + receipt!.source.source_revision_id,
    ),
  );
  expect(
    Http.mock.calls.some(([config]) =>
      /\/nodes\/|\/sources\//.test(config.url),
    ),
  ).toBe(false);
  expect(
    Http.mock.calls.filter(
      ([config]) => config.url.endsWith('/imports') && config.method === 'post',
    ),
  ).toHaveLength(1);
});
it('recovers a lost creation by the original key after remount and does not automatically resend bytes', async () => {
  loseCreate = true;
  const view = mount();
  await openRegistration();
  fireEvent.change(screen.getByLabelText('synthetic-file'), {
    target: { files: [file()] },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.prepare' }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.recovery' });
  const raw = localStorage.getItem(
    originalStorageKey(DomainId, OwnerId) + ':create',
  )!;
  expect(raw).toContain('original.pdf');
  expect(raw).not.toContain('include_paths');
  expect(raw).not.toContain('synthetic-original-session');
  const original = JSON.parse(raw).request;
  view.unmount();
  mount();
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeOriginal.checkResult',
    }),
  );
  await screen.findByRole('button', { name: 'knowledgeOriginal.retransmit' });
  expect(
    Http.mock.calls.filter(([config]) => config.method === 'put'),
  ).toHaveLength(0);
  expect(
    Http.mock.calls.filter(
      ([config]) => config.url.endsWith('/uploads') && config.method === 'post',
    ),
  ).toHaveLength(1);
  expect(
    JSON.parse(
      localStorage.getItem(originalStorageKey(DomainId, OwnerId) + ':create')!,
    ).request,
  ).toEqual(original);
  fireEvent.change(screen.getByLabelText('synthetic-file'), {
    target: { files: [file()] },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.retransmit' }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.confirmation' });
  expect(receipt).toBeNull();
});
it('recovers a committed confirmation through GET only and preserves its original tuple across remount', async () => {
  loseConfirm = true;
  const view = mount();
  await openRegistration();
  await prepare();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.confirm' }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.recovery' });
  const before = JSON.parse(
    localStorage.getItem(originalStorageKey(DomainId, OwnerId) + ':confirm')!,
  );
  view.unmount();
  mount();
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeOriginal.checkResult',
    }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.receipt' });
  expect(
    JSON.parse(
      localStorage.getItem(originalStorageKey(DomainId, OwnerId) + ':confirm')!,
    ).request,
  ).toEqual(before.request);
  expect(
    Http.mock.calls.filter(
      ([config]) => config.method === 'post' && config.url.endsWith('/imports'),
    ),
  ).toHaveLength(1);
});
it('keeps only a registration key and canonical configuration digest before POST and recovers by GET', async () => {
  loseRegistration = true;
  const view = mount();
  fireEvent.change(await screen.findByLabelText('knowledgeOriginal.name'), {
    target: { value: 'Synthetic registration' },
  });
  fireEvent.change(screen.getByLabelText('knowledgeOriginal.paths'), {
    target: { value: 'docs/合成.pdf' },
  });
  fireEvent.click(
    screen.getByRole('button', {
      name: 'knowledgeOriginal.createRegistration',
    }),
  );
  await waitFor(() => expect(loseRegistration).toBe(false));
  const stored = JSON.parse(
    localStorage.getItem(originalStorageKey(DomainId, OwnerId) + ':create')!,
  );
  expect(stored.kind).toBe('registration');
  expect(stored.request).toBeUndefined();
  expect(stored.include_paths).toBeUndefined();
  expect(stored.config_sha256).toBe(
    await originalDigest('registration', {
      ...originalRegistrationRequest(reg),
      domain_id: DomainId,
      owner_id: OwnerId,
    }),
  );
  view.unmount();
  mount();
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'knowledgeOriginal.checkResult',
    }),
  );
  await screen.findByRole('form', { name: 'knowledgeOriginal.prepare' });
  expect(
    Http.mock.calls.filter(
      ([config]) =>
        config.method === 'post' &&
        config.url.endsWith('/original-registrations'),
    ),
  ).toHaveLength(1);
});
it('does not automatically re-post or change keys when confirmation lookup is 404', async () => {
  loseConfirm = true;
  mount();
  await openRegistration();
  await prepare();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.confirm' }),
  );
  await screen.findByRole('region', { name: 'knowledgeOriginal.recovery' });
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/imports/by-key')
      ? Promise.reject(failure(404, 'knowledge_not_found'))
      : original(config),
  );
  const recovery = screen.getByRole('region', {
    name: 'knowledgeOriginal.recovery',
  });
  const check = within(recovery).getByRole('button', {
    name: 'knowledgeOriginal.checkResult',
  });
  await waitFor(() => expect(check).not.toBeDisabled());
  fireEvent.click(check);
  const resend = await screen.findByRole('button', {
    name: 'knowledgeOriginal.resend',
  });
  expect(screen.queryByText('original.pdf')).not.toBeInTheDocument();
  expect(
    Http.mock.calls.filter(
      ([config]) => config.method === 'post' && config.url.endsWith('/imports'),
    ),
  ).toHaveLength(1);
  await waitFor(() => expect(resend).not.toBeDisabled());
  fireEvent.click(resend);
  await screen.findByRole('region', { name: 'knowledgeOriginal.receipt' });
  const posts = Http.mock.calls.filter(
    ([config]) => config.method === 'post' && config.url.endsWith('/imports'),
  );
  expect(posts[0][0].data).toBe(posts[1][0].data);
});
it('downloads the fixed revision only after validation and releases its object URL', async () => {
  const ready = await makeUpload();
  receipt = makeReceipt(ready);
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: jest.fn(() => 'blob:verified'),
  });
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: jest.fn(),
  });
  const click = jest
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => {});
  const source = makeRevision(ready, { is_current: false });
  receipt.source = source;
  mount(
    '?mode=original&original_revision=' +
      source.source_revision_id +
      '&original_source=' +
      source.source_id +
      '&original_sha=' +
      source.sha256,
  );
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeOriginal.download' }),
  );
  await waitFor(() => expect(URL.createObjectURL).toHaveBeenCalledTimes(1));
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:verified');
  expect(
    Http.mock.calls.filter(([config]) => config.url.endsWith('/content'))[0][0]
      .url,
  ).toContain(source.source_revision_id);
  expect(source.byte_length).toBe(Bytes.length);
  click.mockRestore();
});

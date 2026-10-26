import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import request from '@/utils/next-request';
import { useWebFlow } from '../use-web-flow';
import WebImport from '../web';
import WebPreviewPanel from '../web-preview';
import WebReading from '../web-reading';
import { Domain } from '../../knowledge-catalogue/__tests__/fixture';
import {
  fixture,
  file,
  Scope,
  DomainId,
  OwnerId,
  CaptureId,
  PreviewId,
  ReceiptId,
  response,
  binaryResponse,
  Raw,
  Reading,
  Encoder,
  failure,
} from './web-fixture';
import {
  webCaptureProjection,
  webDigest,
  webSourceProjection,
  webStorageKey,
  webSha256,
} from '@/utils/knowledge-web';
type ReactNode = import('react').ReactNode;
type Fixture = import('./web-fixture').WebFixture;
type Flow = import('../use-web-flow').WebFlow;
type Capture = import('@/interfaces/database/knowledge-web').WebCapture;
type Preview = import('@/interfaces/database/knowledge-web').WebPreview;
type Receipt = import('@/interfaces/database/knowledge-web').WebReceipt;
type PreviewItem = import('@/interfaces/database/knowledge-web').WebPreviewItem;
type ReceiptItem = import('@/interfaces/database/knowledge-web').WebReceiptItem;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { getUserInfoObject: () => ({ name: 'Native profile without ID' }) },
  getAuthorization: () => 'synthetic-web-session',
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
const Http = request as unknown as jest.Mock;
let f: Fixture,
  cap: Capture | null,
  observed: boolean,
  preview: Preview | null,
  previewItem: PreviewItem,
  receipt: Receipt | null,
  receiptItem: ReceiptItem,
  lose: Set<string>,
  missingCapture: boolean;
const Page = (items: unknown[]) => ({ items, next_cursor: null });
function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    client,
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    ),
  };
}
beforeEach(async () => {
  localStorage.clear();
  f = await fixture();
  cap = null;
  observed = false;
  preview = null;
  receipt = null;
  lose = new Set();
  missingCapture = false;
  previewItem = f.previewItem;
  receiptItem = f.receiptItem;
  Http.mockReset().mockImplementation(async (config) => {
    const path: string = config.url,
      method = config.method,
      input = config.data ? JSON.parse(config.data) : undefined;
    const reply = (name: string, value: unknown, status = 200) => {
      if (lose.delete(name))
        throw Object.assign(
          Error('Synthetic lost ' + name),
          name === 'acquire' ? { code: 'ECONNABORTED' } : {},
        );
      return response(value, status);
    };
    if (path.endsWith('/web-import-contract')) return response(f.capabilities);
    if (path.endsWith('/domain'))
      return response({ ...Domain, domain_id: DomainId, owner_id: OwnerId });
    if (path.endsWith('/groups'))
      return response({ items: [], next_cursor: '' });
    if (path.endsWith('/web-registrations') && method === 'post') {
      f.registration = {
        ...f.registration,
        ...input,
        config_sha256: await webDigest('registration', input),
        scope_sha256: await webDigest('network_scope', input.network_scope),
      };
      return reply('registration', f.registration, 201);
    }
    if (
      path.endsWith('/web-registrations/' + Scope.registration) ||
      path.endsWith('/web-registrations/by-key')
    )
      return response(f.registration);
    if (path.endsWith('/captures') && method === 'post') {
      const local = input.acquisition_mode === 'local_snapshot';
      cap = {
        ...f.capture,
        idempotency_key: input.idempotency_key,
        registration_config_sha256: input.registration_config_sha256,
        request_sha256: await webDigest(
          'capture_request',
          webCaptureProjection(Scope.registration, input),
        ),
        state: local ? 'awaiting_bytes' : 'awaiting_fetch',
        acquisition_mode: input.acquisition_mode,
        revision: 1,
        original_filename: local ? input.original_filename : null,
        declared_byte_length: local ? input.declared_byte_length : null,
        declared_media_type: local ? input.declared_media_type : null,
        expected_sha256: local ? input.expected_sha256 : null,
        received_bytes: 0,
        raw_sha256: null,
        raw_byte_length: null,
        media_type: null,
        attempt_started_at: null,
        attempt_deadline_at: null,
        observation_id: null,
        provenance: {
          ...f.capture.provenance,
          kind: local ? 'user_supplied' : 'server_http',
          final_url: local ? input.declared_final_url : null,
          claimed_captured_at: local ? input.claimed_captured_at : null,
          server_received_at: null,
        },
      };
      if (missingCapture) {
        cap = null;
        throw Error('Synthetic no response before creation');
      }
      return reply('capture', cap, 201);
    }
    if (path.endsWith('/captures/by-key')) {
      if (!cap) throw failure(404, 'knowledge_not_found');
      return response(cap);
    }
    if (path.endsWith('/captures/' + CaptureId)) {
      if (!cap) throw failure(404, 'knowledge_not_found');
      if (method === 'delete')
        cap = { ...cap, state: 'cancelled', revision: cap.revision + 1 };
      return response(cap);
    }
    if (path.endsWith('/content') && method === 'put') {
      if (!cap) throw Error('Missing capture');
      expect(config.transformRequest[0]()).toBeInstanceOf(File);
      cap = {
        ...cap,
        state: 'ready',
        revision: cap.revision + 1,
        raw_sha256: f.capture.raw_sha256,
        raw_byte_length: Raw.length,
        received_bytes: Raw.length,
        media_type: 'application/rss+xml',
        provenance: {
          ...cap.provenance,
          server_received_at: f.capture.created_at,
        },
      };
      return reply('upload', cap);
    }
    if (path.endsWith('/acquire')) {
      if (!cap) throw Error('Missing capture');
      cap = {
        ...cap,
        state: 'ready',
        revision: cap.revision + 1,
        raw_sha256: f.capture.raw_sha256,
        raw_byte_length: Raw.length,
        received_bytes: Raw.length,
        media_type: 'application/rss+xml',
        attempt_started_at: f.capture.created_at,
        attempt_deadline_at: new Date(
          Date.parse(f.capture.created_at) + 15000,
        ).toISOString(),
        provenance: {
          ...cap.provenance,
          final_url: f.registration.origin_url,
          fetch_started_at: f.capture.created_at,
          fetch_completed_at: f.capture.created_at,
          upstream_http_status: 200,
        },
      };
      return reply('acquire', cap);
    }
    if (path.endsWith('/observation')) {
      if (method === 'post') {
        observed = true;
        return reply('observe', f.observation, 201);
      }
      if (!observed) throw failure(404, 'knowledge_not_found');
      return response(f.observation);
    }
    if (path.endsWith('/observation/entries'))
      return response(Page([f.summary]));
    if (path.endsWith('/observation/entries/' + f.entry.entry_key))
      return response(f.entry);
    if (path.endsWith('/references')) return response(Page(f.references));
    if (path.endsWith('/target'))
      return response({
        domain_id: DomainId,
        registration_id: Scope.registration,
        entry_key: f.entry.entry_key,
        source_id: null,
        current_revision_id: null,
        current_input_sha256: null,
        current_classification: null,
      });
    if (path.endsWith('/previews') && method === 'post') {
      previewItem = {
        ...f.previewItem,
        classification: input.selection[0].classification,
        expected_revision_id: input.selection[0].expected_revision_id,
        source_input_sha256: await webDigest(
          'source_input',
          webSourceProjection(f.entry, input.selection[0].classification),
        ),
      };
      preview = {
        ...f.preview,
        idempotency_key: input.idempotency_key,
        observation_sha256: input.observation_sha256,
        request_sha256: await webDigest('selection_request', input),
        selection_sha256: await webDigest('selection', input),
        registration_config_sha256: f.registration.config_sha256,
        scope_sha256: f.registration.scope_sha256,
      };
      preview.manifest_sha256 = await webDigest('preview_manifest', {
        ...preview,
        items: [previewItem],
      });
      return reply('preview', preview, 201);
    }
    if (
      path.endsWith('/previews/by-key') ||
      path.endsWith('/previews/' + PreviewId)
    ) {
      if (!preview) throw failure(404, 'knowledge_not_found');
      if (method === 'delete')
        preview = {
          ...preview,
          state: 'cancelled',
          revision: preview.revision + 1,
        };
      return response(preview);
    }
    if (path.endsWith('/previews/' + PreviewId + '/items'))
      return response(Page([previewItem]));
    if (path.endsWith('/imports') && method === 'post') {
      if (!preview) throw Error('Missing preview');
      receipt = {
        ...f.receipt,
        idempotency_key: input.idempotency_key,
        manifest_sha256: input.manifest_sha256,
        request_sha256: await webDigest('confirmation', input),
        preview_request_sha256: preview.request_sha256,
        selection_sha256: preview.selection_sha256,
      };
      receiptItem = {
        ...f.receiptItem,
        classification: previewItem.classification,
        source_input_sha256: previewItem.source_input_sha256,
      };
      preview = {
        ...preview,
        state: 'committed',
        revision: preview.revision + 1,
        import_task_id: ReceiptId,
      };
      return reply('confirm', receipt, 201);
    }
    if (
      path.endsWith('/imports/by-key') ||
      path.endsWith('/imports/' + ReceiptId)
    ) {
      if (!receipt) throw failure(404, 'knowledge_not_found');
      return response(receipt);
    }
    if (path.endsWith('/imports/' + ReceiptId + '/items'))
      return response(Page([receiptItem]));
    if (path.endsWith('/safe_reading/content'))
      return binaryResponse(
        Encoder.encode(Reading),
        f.entry.representations[1].sha256,
      );
    if (path.endsWith('/web-revisions')) return response(Page([f.revision]));
    throw Error('Unhandled synthetic path ' + method + ' ' + path);
  });
});
async function prepare(result: { current: Flow }) {
  await act(async () => {
    await result.current.openRegistration(Scope.registration);
  });
  await act(async () => {
    await result.current.createCapture(f.captureRequest, file());
  });
  await act(async () => {
    await result.current.upload();
  });
  await act(async () => {
    await result.current.observe();
  });
}
async function previewFlow(result: { current: Flow }) {
  await act(async () => {
    await result.current.createPreview([
      {
        entry_key: f.entry.entry_key,
        classification: f.registration.default_classification,
        expected_revision_id: null,
      },
    ]);
  });
}
it('completes registration context, local upload, observation, exact preview and atomic receipt with model calls zero', async () => {
  const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
    wrapper: wrapper().wrapper,
  });
  await prepare(result);
  expect(result.current.entries).toHaveLength(1);
  await previewFlow(result);
  expect(result.current.error).toBeNull();
  expect(result.current.previewItems).toHaveLength(1);
  await act(async () => {
    await result.current.confirm([f.entry.entry_key], true);
  });
  expect(result.current.error).toBeNull();
  expect(result.current.receipt?.model_calls).toBe(0);
  expect(result.current.receiptItems).toHaveLength(1);
  expect(result.current.slot?.confirmation_request).not.toBeNull();
});
it.each(['capture', 'preview', 'confirm'])(
  'recovers a lost %s result with original metadata after a reload, without another POST',
  async (stage) => {
    const first = renderHook(() => useWebFlow(DomainId, OwnerId), {
      wrapper: wrapper().wrapper,
    });
    if (stage === 'capture') {
      lose.add(stage);
      await act(async () => {
        await first.result.current.openRegistration(Scope.registration);
      });
      await act(async () => {
        await first.result.current.createCapture(f.captureRequest, file());
      });
    } else {
      await prepare(first.result);
      if (stage === 'preview') lose.add(stage);
      await previewFlow(first.result);
      if (stage === 'confirm') {
        lose.add(stage);
        await act(async () => {
          await first.result.current.confirm([f.entry.entry_key], true);
        });
      }
    }
    expect(first.result.current.unknown).toBe(stage);
    const before = Http.mock.calls.filter(
      ([config]) => config.method === 'post',
    ).length;
    first.unmount();
    if (stage === 'confirm' && cap)
      cap = { ...cap, state: 'cancelled', revision: cap.revision + 1 };
    const next = renderHook(() => useWebFlow(DomainId, OwnerId), {
      wrapper: wrapper().wrapper,
    });
    await waitFor(() => expect(next.result.current.slot).not.toBeNull());
    await act(async () => {
      await next.result.current.recover();
    });
    expect(next.result.current.error).toBeNull();
    expect(next.result.current.unknown).toBeNull();
    expect(
      Http.mock.calls.filter(([config]) => config.method === 'post'),
    ).toHaveLength(before);
    if (stage === 'confirm')
      expect(next.result.current.receiptItems).toHaveLength(1);
  },
);
it('retains an unknown404 and only resends the identical original capture key and body on explicit action', async () => {
  missingCapture = true;
  const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
    wrapper: wrapper().wrapper,
  });
  await act(async () => {
    await result.current.openRegistration(Scope.registration);
  });
  await act(async () => {
    await result.current.createCapture(f.captureRequest, file());
  });
  const key = result.current.slot!.capture_create_key;
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.canResend).toBe(true);
  expect(result.current.slot?.capture_create_key).toBe(key);
  expect(result.current.unknown).toBe('capture');
  missingCapture = false;
  await act(async () => {
    await result.current.resend();
  });
  const posts = Http.mock.calls.filter(
    ([config]) => config.url.endsWith('/captures') && config.method === 'post',
  );
  expect(posts).toHaveLength(2);
  expect(posts[0][0].data).toBe(posts[1][0].data);
});
it('persists registration metadata before POST and recovers the original registration key', async () => {
  lose.add('registration');
  const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
    wrapper: wrapper().wrapper,
  });
  await act(async () => {
    await result.current.createRegistration(f.registrationRequest);
  });
  expect(result.current.registrationPending?.request).toEqual(
    f.registrationRequest,
  );
  expect(
    localStorage.getItem(webStorageKey(DomainId, OwnerId) + ':registration'),
  ).not.toBeNull();
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.registration?.registration_id).toBe(Scope.registration);
  expect(result.current.registrationPending).toBeNull();
});
it('never auto-acquires or retries an explicit fetch after a lost response; GET recovers the single attempt', async () => {
  const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
    wrapper: wrapper().wrapper,
  });
  await act(async () => {
    await result.current.openRegistration(Scope.registration);
  });
  await act(async () => {
    await result.current.createCapture({
      registration_config_sha256: f.registration.config_sha256,
      idempotency_key: 'single-fetch',
      acquisition_mode: 'explicit_fetch',
    });
  });
  expect(
    Http.mock.calls.some(([config]) => config.url.endsWith('/acquire')),
  ).toBe(false);
  lose.add('acquire');
  await act(async () => {
    await result.current.acquire(true);
  });
  await act(async () => {
    await result.current.acquire(true);
  });
  expect(
    Http.mock.calls.filter(([config]) => config.url.endsWith('/acquire')),
  ).toHaveLength(1);
  expect(
    Http.mock.calls.find(([config]) => config.url.endsWith('/acquire'))![0]
      .timeout,
  ).toBe(25000);
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.capture?.state).toBe('ready');
  expect(
    Http.mock.calls.filter(([config]) => config.url.endsWith('/acquire')),
  ).toHaveLength(1);
});
it('requires a fresh per-item acknowledgement and final consent for every preview mount', async () => {
  const confirm = jest.fn(),
    flow = {
      preview: f.preview,
      previewItems: [f.previewItem],
      busy: false,
      unknown: null,
      slot: null,
      confirm,
      changeSelection: jest.fn(),
      recover: jest.fn(),
    } as unknown as Flow;
  const first = render(<WebPreviewPanel flow={flow} />);
  const button = screen.getByText('knowledgeWeb.confirm');
  expect(button).toBeDisabled();
  const boxes = screen.getAllByRole('checkbox');
  fireEvent.click(boxes[0]);
  expect(button).toBeDisabled();
  fireEvent.click(boxes[1]);
  expect(button).toBeEnabled();
  fireEvent.click(button);
  expect(confirm).toHaveBeenCalledWith([f.entry.entry_key], true);
  first.unmount();
  render(<WebPreviewPanel flow={flow} />);
  expect(screen.getByText('knowledgeWeb.confirm')).toBeDisabled();
  expect(screen.getAllByRole('checkbox')[0]).not.toBeChecked();
});
it('renders verified safe HTML only inside an empty sandbox, with parent references separate and no resource requests', async () => {
  const failed = jest.fn();
  render(
    <WebReading
      target={{ scope: Scope, entry: f.entry }}
      onScopeFailure={failed}
    />,
  );
  expect(document.querySelector('iframe')).toBeNull();
  fireEvent.click(screen.getByText('knowledgeWeb.readFull'));
  await waitFor(() => expect(document.querySelector('iframe')).not.toBeNull());
  const frame = document.querySelector('iframe')!;
  expect(frame.getAttribute('sandbox')).toBe('');
  expect(frame.srcdoc).toContain("img-src 'none'");
  expect(frame.srcdoc).toContain('Content-Security-Policy');
  expect(frame.srcdoc).not.toContain('allow-same-origin');
  expect(document.querySelectorAll('img,script')).toHaveLength(0);
  expect(Http.mock.calls).toHaveLength(1);
  fireEvent.click(screen.getByText('knowledgeWeb.references'));
  await waitFor(() =>
    expect(screen.getByRole('link')).toHaveAttribute(
      'rel',
      'noopener noreferrer',
    ),
  );
  expect(failed).not.toHaveBeenCalled();
});
it('uses the authorized domain owner even when the native display profile has no ID, and performs no eager web revision requests', async () => {
  render(<WebImport domain={DomainId} />, { wrapper: wrapper().wrapper });
  await screen.findByRole('form', { name: 'knowledgeWeb.registration' });
  expect(screen.getByText('knowledgeWeb.openRegistration')).toBeVisible();
  expect(
    Http.mock.calls.some(([config]) => config.url.endsWith('/web-revisions')),
  ).toBe(false);
});
it('walks the four visible stages and requires selection plus two separate acknowledgements before saving', async () => {
  render(<WebImport domain={DomainId} />, { wrapper: wrapper().wrapper });
  fireEvent.change(
    await screen.findByLabelText('knowledgeWeb.registrationId'),
    { target: { value: Scope.registration } },
  );
  fireEvent.click(screen.getByText('knowledgeWeb.openRegistration'));
  const captureForm = await screen.findByRole('form', {
    name: 'knowledgeWeb.capture',
  });
  fireEvent.change(
    within(captureForm).getByLabelText('knowledgeWeb.snapshotFile'),
    { target: { files: [file()] } },
  );
  const prepareButton = within(captureForm).getByText(
    'knowledgeWeb.prepareCapture',
  );
  await waitFor(() => expect(prepareButton).toBeEnabled());
  fireEvent.submit(captureForm);
  const upload = await screen.findByText('knowledgeWeb.upload');
  await waitFor(() => expect(upload).toBeEnabled());
  fireEvent.click(upload);
  const observe = await screen.findByText('knowledgeWeb.observe');
  await waitFor(() => expect(observe).toBeEnabled());
  fireEvent.click(observe);
  const observedPanel = await screen.findByRole('region', {
    name: 'knowledgeWeb.observation',
  });
  const entryBox = await within(observedPanel).findByRole('checkbox', {
    name: '合成代码',
  });
  await waitFor(() => expect(entryBox).toBeEnabled());
  fireEvent.click(entryBox);
  fireEvent.click(
    within(observedPanel).getByText('knowledgeWeb.createPreview'),
  );
  const previewPanel = await screen.findByRole('region', {
      name: 'knowledgeWeb.preview',
    }),
    save = within(previewPanel).getByText('knowledgeWeb.confirm');
  expect(save).toBeDisabled();
  const boxes = within(previewPanel).getAllByRole('checkbox');
  await waitFor(() => expect(boxes[0]).toBeEnabled());
  fireEvent.click(boxes[0]);
  expect(save).toBeDisabled();
  fireEvent.click(boxes[1]);
  fireEvent.click(save);
  await screen.findByRole('region', { name: 'knowledgeWeb.receipt' });
  expect(screen.getByText('knowledgeImport.readRevision')).toHaveAttribute(
    'href',
    expect.stringContaining('web_input_sha='),
  );
});
it.each(['upload', 'observe'])(
  'GET-recovers a lost %s without resending bytes or normalizing again',
  async (stage) => {
    const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
      wrapper: wrapper().wrapper,
    });
    await act(async () => {
      await result.current.openRegistration(Scope.registration);
    });
    await act(async () => {
      await result.current.createCapture(f.captureRequest, file());
    });
    if (stage === 'upload') lose.add(stage);
    await act(async () => {
      await result.current.upload();
    });
    if (stage === 'observe') {
      lose.add(stage);
      await act(async () => {
        await result.current.observe();
      });
    }
    expect(result.current.unknown).toBe(stage);
    const count = Http.mock.calls.filter(([config]) =>
      ['post', 'put'].includes(config.method),
    ).length;
    await act(async () => {
      await result.current.recover();
    });
    expect(result.current.error).toBeNull();
    expect(result.current.unknown).toBeNull();
    expect(
      Http.mock.calls.filter(([config]) =>
        ['post', 'put'].includes(config.method),
      ),
    ).toHaveLength(count);
  },
);
it('lets the user revise only after a determinate rejection and another original-key GET proves no publication', async () => {
  const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
    wrapper: wrapper().wrapper,
  });
  await prepare(result);
  const original = Http.getMockImplementation()!,
    issue = failure(409, 'revision_changed');
  issue.response.data.failure.outcome_known = true;
  Http.mockImplementation((config) =>
    config.url.endsWith('/previews') && config.method === 'post'
      ? Promise.reject(issue)
      : original(config),
  );
  await previewFlow(result);
  const previousKey = result.current.slot!.preview_create_key;
  expect(result.current.canRevise).toBe(true);
  await act(async () => {
    await result.current.recover();
  });
  expect(result.current.unknown).toBe('preview');
  await act(async () => {
    await result.current.revisePreparation();
  });
  expect(result.current.slot?.preview_create_request).toBeNull();
  expect(result.current.unknown).toBeNull();
  Http.mockImplementation(original);
  await previewFlow(result);
  expect(result.current.error).toBeNull();
  expect(result.current.slot?.preview_create_key).not.toBe(previousKey);
});
it('clears visible private state on native404 while retaining the owner-scoped recovery record', async () => {
  const { result } = renderHook(() => useWebFlow(DomainId, OwnerId), {
    wrapper: wrapper().wrapper,
  });
  await prepare(result);
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/captures/' + CaptureId)
      ? Promise.reject(failure(404, 'knowledge_not_found'))
      : original(config),
  );
  await act(async () => {
    await result.current.recheckCapture();
  });
  expect(result.current.blocked).toBe(true);
  expect(result.current.registration).toBeNull();
  expect(result.current.capture).toBeNull();
  expect(result.current.entries).toEqual([]);
  expect(localStorage.getItem(webStorageKey(DomainId, OwnerId))).not.toBeNull();
});
it('shows an empty verified safe body and refuses a body with the wrong digest before mounting an iframe', async () => {
  const empty = {
    ...f.entry,
    representations: [
      f.entry.representations[0],
      {
        ...f.entry.representations[1],
        byte_length: 0,
        sha256: await webSha256(new Uint8Array()),
      },
    ],
  };
  Http.mockResolvedValue(
    binaryResponse(new Uint8Array(), empty.representations[1].sha256),
  );
  const first = render(
    <WebReading
      target={{ scope: Scope, entry: empty }}
      onScopeFailure={jest.fn()}
    />,
  );
  fireEvent.click(screen.getByText('knowledgeWeb.readFull'));
  await screen.findByText('knowledgeWeb.emptyBody');
  expect(document.querySelector('iframe')).not.toBeNull();
  first.unmount();
  Http.mockResolvedValue(
    binaryResponse(Raw, f.entry.representations[1].sha256),
  );
  render(
    <WebReading
      target={{ scope: Scope, entry: f.entry }}
      onScopeFailure={jest.fn()}
    />,
  );
  fireEvent.click(screen.getByText('knowledgeWeb.readFull'));
  await screen.findByRole('alert');
  expect(document.querySelector('iframe')).toBeNull();
});

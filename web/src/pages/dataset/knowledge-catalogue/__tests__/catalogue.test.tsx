import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query';
import request from '@/utils/next-request';
import KnowledgeCatalogue from '..';
import SourceContent from '../content';
import {
  Capabilities,
  Child,
  Content,
  Domain,
  DomainId,
  Group,
  Root,
  Source,
  SourceText,
  envelope,
  page,
} from './fixture';
import { getAuthorization } from '@/utils/authorization-util';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: jest.fn(() => 'native-local-test-token'),
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
const Authorization = getAuthorization as unknown as jest.Mock;

function mount(element = <KnowledgeCatalogue />) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const tree = () => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/catalogue/${DomainId}`]}>
        <Routes>
          <Route path="/catalogue/:id" element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(tree());
  return { ...view, rerenderSession: () => view.rerender(tree()) };
}

beforeEach(() => {
  Authorization.mockReturnValue('native-local-test-token');
  Http.mockReset().mockImplementation(async (config) => {
    const url = config.url as string;
    let data: unknown;
    if (url.endsWith('/contract')) data = Capabilities;
    else if (url.endsWith('/domain')) data = Domain;
    else if (url.endsWith('/groups'))
      data = page(config.params.parent_id ? [] : [Group]);
    else if (url.endsWith('/sources')) data = page([Source]);
    else if (url.endsWith('/nodes'))
      data = page(config.params.parent_id === Root.node_id ? [Child] : []);
    else if (url.endsWith(`/${Root.node_id}/content`)) data = Content;
    else if (url.endsWith(`/${Child.node_id}/content`))
      data = {
        ...Content,
        node_id: Child.node_id,
        start_byte: 8,
        end_byte: 24,
        text: 'const value = 1;',
      };
    else if (url.endsWith(`/${Root.node_id}`)) data = Root;
    else if (url.endsWith(`/${Child.node_id}`)) data = Child;
    else throw new Error(`Unexpected request: ${url}`);
    return { status: 200, data: envelope(data) };
  });
});

it('browses domain → project → source → child node → original, preserving CRLF', async () => {
  mount();
  expect(
    await screen.findByText(
      'Synthetic private domain · knowledgeCatalogue.title',
    ),
  ).toBeInTheDocument();
  fireEvent.click(await screen.findByText('Synthetic project'));
  await waitFor(() =>
    expect(
      Http.mock.calls.some(
        ([c]) => c.url.endsWith('/sources') && c.params.group_id === Group.id,
      ),
    ).toBe(true),
  );
  fireEvent.click(await screen.findByText(Source.relative_path));
  const original = await screen.findByTestId('knowledge-original');
  expect(original.textContent).toBe(SourceText);
  expect(screen.getAllByText(Source.source_revision_id).length).toBeGreaterThan(
    0,
  );
  expect(
    screen.getByText(
      'knowledgeCatalogue.frontend · knowledgeCatalogue.file · source-v1',
    ),
  ).toBeInTheDocument();
  fireEvent.click(await screen.findByText('value'));
  await waitFor(() =>
    expect(screen.getByTestId('knowledge-original').textContent).toBe(
      'const value = 1;',
    ),
  );
  fireEvent.click(screen.getByText('knowledgeCatalogue.parentNode'));
  await waitFor(() =>
    expect(screen.getByTestId('knowledge-original').textContent).toBe(
      SourceText,
    ),
  );
  expect(
    Http.mock.calls.every(
      ([c]) => c.method === 'get' && !/parse|retrieval|chat|models/.test(c.url),
    ),
  ).toBe(true);
});

it('shows loading without fetching child views before the contract completes', () => {
  Http.mockImplementation(() => new Promise(() => {}));
  mount();
  expect(screen.getByRole('status')).toHaveTextContent(
    'knowledgeCatalogue.loading',
  );
  expect(Http).toHaveBeenCalledTimes(1);
});

it.each([403, 404, 409, 500])(
  'handles %i without showing source names or original content',
  async (status) => {
    Http.mockRejectedValue({
      response: { status, data: { code: 'knowledge_not_found' } },
    });
    mount();
    expect(await screen.findByRole('alert')).toHaveTextContent(
      status === 409
        ? 'knowledgeCatalogue.conflict'
        : status === 500
          ? 'knowledgeCatalogue.failed'
          : 'knowledgeCatalogue.notVisible',
    );
    expect(screen.queryByText(Source.relative_path)).not.toBeInTheDocument();
    expect(screen.queryByTestId('knowledge-original')).not.toBeInTheDocument();
  },
);

it('allows explicit retry after a network failure', async () => {
  Http.mockRejectedValueOnce(new Error('offline'));
  mount();
  fireEvent.click(await screen.findByText('knowledgeCatalogue.retry'));
  expect(await screen.findByText('Synthetic project')).toBeInTheDocument();
});

it('shows empty group and source states', async () => {
  const normal = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    /\/(groups|sources)$/.test(config.url)
      ? Promise.resolve({ status: 200, data: envelope(page([])) })
      : normal(config),
  );
  mount();
  expect(
    await screen.findByText('knowledgeCatalogue.emptyGroups'),
  ).toBeInTheDocument();
  expect(
    await screen.findByText('knowledgeCatalogue.emptySources'),
  ).toBeInTheDocument();
});

it('navigates nested modules while keeping source and node requests in their exact scope', async () => {
  const module = {
    ...Group,
    id: '9'.repeat(32),
    parent_id: Group.id,
    kind: 'module',
    name: 'Synthetic module',
    component_side: 'backend',
  };
  const normal = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/groups') && config.params.parent_id === Group.id
      ? Promise.resolve({ status: 200, data: envelope(page([module])) })
      : normal(config),
  );
  mount();
  fireEvent.click(await screen.findByText(Group.name));
  fireEvent.click(await screen.findByText(module.name));
  await waitFor(() =>
    expect(
      Http.mock.calls.some(
        ([c]) => c.url.endsWith('/sources') && c.params.group_id === module.id,
      ),
    ).toBe(true),
  );
  fireEvent.click(await screen.findByText(Source.relative_path));
  expect((await screen.findByTestId('knowledge-original')).textContent).toBe(
    SourceText,
  );
  expect(
    Http.mock.calls.some(
      ([c]) =>
        c.url.endsWith('/nodes') &&
        c.params.source_revision_id === Source.source_revision_id &&
        c.params.parent_id === Root.node_id,
    ),
  ).toBe(true);
  expect(
    screen.getByText(
      'knowledgeCatalogue.frontend · knowledgeCatalogue.file · source-v1',
    ),
  ).toBeInTheDocument();
});

it('renders a zero-byte source without issuing a next-window read', async () => {
  const sha =
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
  const node = { ...Root, end_byte: 0, start_line: 1, end_line: 1 };
  Http.mockResolvedValue({
    status: 200,
    data: envelope({
      ...Content,
      source_sha256: sha,
      start_byte: 0,
      end_byte: 0,
      text: '',
    }),
  });
  mount(
    <SourceContent
      domainId={DomainId}
      node={node}
      source={{ ...Source, byte_length: 0, sha256: sha }}
      onRefresh={jest.fn()}
    />,
  );
  expect((await screen.findByTestId('knowledge-original')).textContent).toBe(
    '',
  );
  expect(screen.getByText('knowledgeCatalogue.next')).toBeDisabled();
  expect(screen.getByText('knowledgeCatalogue.previous')).toBeDisabled();
  expect(Http).toHaveBeenCalledTimes(1);
});

it('paginates using the server cursor without dropping the original group filter', async () => {
  const normal = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/groups')
      ? Promise.resolve({
          status: 200,
          data: envelope(
            config.params.cursor ? page([]) : page([Group], 'opaque+/='),
          ),
        })
      : normal(config),
  );
  mount();
  fireEvent.click(await screen.findByText('knowledgeCatalogue.more'));
  await waitFor(() =>
    expect(Http.mock.calls.some(([c]) => c.params.cursor === 'opaque+/=')).toBe(
      true,
    ),
  );
  expect(screen.getByText('Synthetic project')).toBeInTheDocument();
});

it('renders source HTML as inert text', async () => {
  const text = '<img src=x onerror=alert(1) />';
  const length = new TextEncoder().encode(text).length;
  const node = { ...Root, end_byte: length };
  Http.mockResolvedValue({
    status: 200,
    data: envelope({ ...Content, end_byte: length, text }),
  });
  const view = mount(
    <SourceContent
      domainId={DomainId}
      node={node}
      source={Source}
      onRefresh={jest.fn()}
    />,
  );
  expect((await screen.findByTestId('knowledge-original')).textContent).toBe(
    text,
  );
  expect(view.container.querySelector('img')).toBeNull();
});

it('does not display content whose revision or byte length differs from the selected source', async () => {
  Http.mockResolvedValue({
    status: 200,
    data: envelope({ ...Content, source_revision_id: 'f'.repeat(32) }),
  });
  mount(
    <SourceContent
      domainId={DomainId}
      node={Root}
      source={Source}
      onRefresh={jest.fn()}
    />,
  );
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'knowledgeCatalogue.failed',
  );
  expect(screen.queryByTestId('knowledge-original')).not.toBeInTheDocument();
});

it('pages a large Chinese source using UTF-8 byte boundaries and preserves all bytes', async () => {
  const text = '中'.repeat(23000) + '\r\nend';
  const bytes = new TextEncoder().encode(text);
  const node = { ...Root, end_byte: bytes.length, end_line: 2 };
  const windows: number[][] = [];
  Http.mockImplementation(async (config) => {
    const { start_byte: start, end_byte: end } = config.params;
    windows.push([start, end]);
    let slice: string;
    try {
      slice = new TextDecoder('utf-8', { fatal: true }).decode(
        bytes.slice(start, end),
      );
    } catch {
      throw {
        response: { status: 400, data: { code: 'knowledge_invalid_request' } },
      };
    }
    return {
      status: 200,
      data: envelope({
        ...Content,
        start_byte: start,
        end_byte: end,
        text: slice,
      }),
    };
  });
  mount(
    <SourceContent
      domainId={DomainId}
      node={node}
      source={Source}
      onRefresh={jest.fn()}
    />,
  );
  const first = (await screen.findByTestId('knowledge-original')).textContent!;
  expect(new TextEncoder().encode(first).length).toBe(65535);
  fireEvent.click(screen.getByText('knowledgeCatalogue.next'));
  await waitFor(() =>
    expect(screen.getByTestId('knowledge-original').textContent).not.toBe(
      first,
    ),
  );
  const second = screen.getByTestId('knowledge-original').textContent!;
  expect(first + second).toBe(text);
  expect(screen.getByText('knowledgeCatalogue.next')).toBeDisabled();
  fireEvent.click(screen.getByText('knowledgeCatalogue.previous'));
  await waitFor(() =>
    expect(screen.getByTestId('knowledge-original').textContent).toBe(first),
  );
  expect(windows.every(([start, end]) => end - start <= 65536)).toBe(true);
  expect(windows.some(([start]) => start === 65535)).toBe(true);
});

it('hides a previously rendered original when a focus refetch denies access', async () => {
  mount();
  fireEvent.click(await screen.findByText(Source.relative_path));
  await screen.findByTestId('knowledge-original');
  Http.mockRejectedValue({
    response: { status: 404, data: { code: 'knowledge_not_found' } },
  });
  focusManager.setFocused(false);
  focusManager.setFocused(true);
  await waitFor(() =>
    expect(screen.queryByTestId('knowledge-original')).not.toBeInTheDocument(),
  );
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'knowledgeCatalogue.notVisible',
  );
});

it('hides the entire selected scope when a later source page denies access', async () => {
  const normal = Http.getMockImplementation()!;
  Http.mockImplementation((config) => {
    if (!config.url.endsWith('/sources')) return normal(config);
    if (config.params.cursor)
      return Promise.reject({
        response: { status: 404, data: { code: 'knowledge_not_found' } },
      });
    return Promise.resolve({
      status: 200,
      data: envelope(page([Source], 'private-next')),
    });
  });
  mount();
  fireEvent.click(await screen.findByText(Source.relative_path));
  await screen.findByTestId('knowledge-original');
  fireEvent.click(screen.getByText('knowledgeCatalogue.more'));
  await waitFor(() =>
    expect(screen.queryByTestId('knowledge-original')).not.toBeInTheDocument(),
  );
  expect(screen.queryByText(Source.relative_path)).not.toBeInTheDocument();
  expect(screen.getByRole('alert')).toHaveTextContent(
    'knowledgeCatalogue.notVisible',
  );
  let authorize: (value: unknown) => void = () => {};
  Http.mockImplementation((config) =>
    config.url.endsWith('/domain')
      ? new Promise((resolve) => {
          authorize = resolve;
        })
      : normal(config),
  );
  fireEvent.click(screen.getByText('knowledgeCatalogue.retry'));
  expect(
    screen.queryByText('Synthetic private domain · knowledgeCatalogue.title'),
  ).not.toBeInTheDocument();
  authorize({ status: 200, data: envelope(Domain) });
  await screen.findByText(Source.relative_path);
  expect(screen.queryByTestId('knowledge-original')).not.toBeInTheDocument();
});

it('does not reuse a previous login session in the same mounted query client', async () => {
  const view = mount();
  fireEvent.click(await screen.findByText(Source.relative_path));
  await screen.findByTestId('knowledge-original');
  Authorization.mockReturnValue('another-local-test-session');
  Http.mockRejectedValue({
    response: { status: 404, data: { code: 'knowledge_not_found' } },
  });
  view.rerenderSession();
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'knowledgeCatalogue.notVisible',
  );
  expect(screen.queryByTestId('knowledge-original')).not.toBeInTheDocument();
  expect(screen.queryByText(Source.relative_path)).not.toBeInTheDocument();
});

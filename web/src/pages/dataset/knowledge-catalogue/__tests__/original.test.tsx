import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import request from '@/utils/next-request';
import KnowledgeCatalogue from '..';
import { DomainId, Domain, Group, Capabilities } from './fixture';
import {
  Capabilities as OriginalCapabilities,
  makeUpload,
  makeRevision,
  respond,
  failure,
} from '../../knowledge-import/__tests__/original-fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'synthetic-catalogue-session',
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
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/catalogue/' + DomainId]}>
        <Routes>
          <Route path="/catalogue/:id" element={<KnowledgeCatalogue />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  Http.mockReset().mockImplementation(async (config) => {
    if (config.url.endsWith('/original-import-contract'))
      return respond(OriginalCapabilities);
    if (config.url.endsWith('/contract')) return respond(Capabilities);
    if (config.url.endsWith('/domain')) return respond(Domain);
    if (config.url.endsWith('/groups'))
      return respond({
        items: config.params.parent_id ? [] : [Group],
        next_cursor: '',
      });
    if (config.url.endsWith('/sources'))
      return respond({ items: [], next_cursor: '' });
    if (config.url.endsWith('/original-revisions'))
      return respond({ domain_id: DomainId, items: [], next_cursor: null });
    throw Error('Unexpected synthetic catalogue request');
  });
});
it('keeps C1 as the default and lazily mounts original cards within the existing selected group', async () => {
  const source = makeRevision(
    await makeUpload('ready', {
      domain_id: DomainId,
      classification: {
        group_id: Group.id,
        component_side: 'non_code',
        language: '',
      },
    }),
  );
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/original-revisions')
      ? Promise.resolve(
          respond({ domain_id: DomainId, items: [source], next_cursor: null }),
        )
      : original(config),
  );
  mount();
  fireEvent.click(await screen.findByText(Group.name));
  await waitFor(() =>
    expect(
      Http.mock.calls.some(([config]) => config.params?.parent_id === Group.id),
    ).toBe(true),
  );
  expect(
    Http.mock.calls.some(([config]) => config.url.includes('/original-')),
  ).toBe(false);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.originals' }),
  );
  const card = await screen.findByRole('link', { name: /original\.pdf/ });
  expect(card).toHaveAttribute(
    'href',
    expect.stringContaining('original_revision=' + source.source_revision_id),
  );
  expect(
    Http.mock.calls.find(([config]) =>
      config.url.endsWith('/original-revisions'),
    )![0].params.group_id,
  ).toBe(Group.id);
  expect(
    Http.mock.calls.some(([config]) =>
      /\/nodes\/|\/sources\//.test(config.url),
    ),
  ).toBe(false);
});
it('preserves the group on cursor reads and refreshes the current list from its first page', async () => {
  const first = makeRevision(
    await makeUpload('ready', { domain_id: DomainId }),
    { source_revision_id: '1'.repeat(32) },
  );
  const second = {
    ...first,
    source_revision_id: '2'.repeat(32),
    original_filename: 'second.pdf',
  };
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/original-revisions')
      ? Promise.resolve(
          respond({
            domain_id: DomainId,
            items: [config.params.cursor ? second : first],
            next_cursor: config.params.cursor ? null : 'cursor1',
          }),
        )
      : original(config),
  );
  mount();
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeOriginal.originals' }),
  );
  await screen.findByRole('link', { name: /original\.pdf/ });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeCatalogue.more' }),
  );
  await screen.findByRole('link', { name: /second\.pdf/ });
  const calls = Http.mock.calls.filter(([config]) =>
    config.url.endsWith('/original-revisions'),
  );
  expect(calls[1][0].params.cursor).toBe('cursor1');
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeOriginal.refresh' }),
  );
  await waitFor(() =>
    expect(
      Http.mock.calls.filter(([config]) =>
        config.url.endsWith('/original-revisions'),
      ),
    ).toHaveLength(3),
  );
  expect(
    Http.mock.calls.filter(([config]) =>
      config.url.endsWith('/original-revisions'),
    )[2][0].params.cursor,
  ).toBeUndefined();
});
it('hides original cards and private domain names after the original list loses access', async () => {
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/original-revisions')
      ? Promise.reject(failure(404, 'knowledge_not_found'))
      : original(config),
  );
  mount();
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeOriginal.originals' }),
  );
  await screen.findByText('knowledgeCatalogue.notVisible');
  expect(screen.queryByText(Group.name)).not.toBeInTheDocument();
  expect(
    screen.queryByRole('link', { name: /original\.pdf/ }),
  ).not.toBeInTheDocument();
});

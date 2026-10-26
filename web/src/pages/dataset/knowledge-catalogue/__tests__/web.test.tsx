import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import request from '@/utils/next-request';
import KnowledgeCatalogue from '..';
import { DomainId, Domain, Group, Capabilities } from './fixture';
import {
  fixture,
  OwnerId,
  response,
  failure,
} from '../../knowledge-import/__tests__/web-fixture';
import { webDigest, webSourceProjection } from '@/utils/knowledge-web';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'synthetic-web-catalogue-session',
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
beforeEach(async () => {
  const f = await fixture(),
    revision = {
      ...f.revision,
      domain_id: DomainId,
      classification: { ...f.revision.classification, group_id: Group.id },
    };
  revision.source_input_sha256 = await webDigest(
    'source_input',
    webSourceProjection(revision, revision.classification),
  );
  Http.mockReset().mockImplementation(async (config) => {
    if (config.url.endsWith('/contract')) return response(Capabilities);
    if (config.url.endsWith('/domain'))
      return response({ ...Domain, owner_id: OwnerId });
    if (config.url.endsWith('/groups'))
      return response({
        items: config.params.parent_id ? [] : [Group],
        next_cursor: '',
      });
    if (config.url.endsWith('/sources'))
      return response({ items: [], next_cursor: '' });
    if (config.url.endsWith('/web-revisions'))
      return response({ items: [revision], next_cursor: null });
    throw Error('Unhandled synthetic catalogue path');
  });
});
it('keeps C1 text as default and lazily reads Web cards with the selected group and a fixed revision link', async () => {
  mount();
  fireEvent.click(await screen.findByText(Group.name));
  await waitFor(() =>
    expect(
      Http.mock.calls.some(([config]) => config.params?.parent_id === Group.id),
    ).toBe(true),
  );
  expect(Http.mock.calls.some(([config]) => config.url.includes('/web-'))).toBe(
    false,
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeWeb.revisions' }),
  );
  const card = await screen.findByRole('link', { name: '合成代码' });
  expect(card).toHaveAttribute('href', expect.stringContaining('mode=web'));
  expect(card).toHaveAttribute(
    'href',
    expect.stringContaining('web_revision='),
  );
  expect(
    Http.mock.calls.find(([config]) =>
      config.url.endsWith('/web-revisions'),
    )![0].params.group_id,
  ).toBe(Group.id);
  expect(
    Http.mock.calls.some(([config]) =>
      /\/nodes\/|\/sources\//.test(config.url),
    ),
  ).toBe(false);
});
it('clears private names and cards when Web list access is lost', async () => {
  const original = Http.getMockImplementation()!;
  Http.mockImplementation((config) =>
    config.url.endsWith('/web-revisions')
      ? Promise.reject(failure(404, 'knowledge_not_found'))
      : original(config),
  );
  mount();
  fireEvent.click(
    await screen.findByRole('button', { name: 'knowledgeWeb.revisions' }),
  );
  await screen.findByText('knowledgeCatalogue.notVisible');
  expect(screen.queryByText(Group.name)).not.toBeInTheDocument();
  expect(screen.queryByText(Domain.name)).not.toBeInTheDocument();
});

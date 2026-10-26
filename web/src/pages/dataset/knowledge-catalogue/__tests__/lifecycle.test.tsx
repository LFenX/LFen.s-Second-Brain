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
import Catalogue from '..';
import LifecycleBrowser, { LifecycleSourceLink } from '../lifecycle';
import {
  createLifecycleBackend,
  Scope,
  Owner,
  Domain,
  Source,
  copy,
} from './lifecycle-fixture';
type Detail =
  import('@/interfaces/database/knowledge-lifecycle').LifecycleDetail;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'synthetic-lifecycle-session',
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
let backend: import('./lifecycle-fixture').LifecycleBackend;
function mount(catalogue = false, selected = true, group?: string) {
  const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    }),
    path =
      '/dataset/knowledge-catalogue/' +
      Domain +
      (selected ? '?lifecycle_source=' + Source : '');
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            path="/dataset/knowledge-catalogue/:id"
            element={
              catalogue ? (
                <Catalogue />
              ) : (
                <LifecycleBrowser domain={Domain} owner={Owner} group={group} />
              )
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  localStorage.clear();
  backend = createLifecycleBackend();
  Http.mockReset().mockImplementation(backend.http);
});
async function actions() {
  return screen.findByRole('region', { name: 'knowledgeLifecycle.actions' });
}
async function selectAction(name: string) {
  const panel = await actions();
  fireEvent.change(within(panel).getByLabelText('knowledgeLifecycle.action'), {
    target: { value: name },
  });
  return panel;
}
async function confirm(panel: HTMLElement) {
  const box = within(panel).getByRole('checkbox');
  await waitFor(() => expect(box).toBeEnabled());
  fireEvent.click(box);
  const button = within(panel).getByRole('button', {
    name: 'knowledgeLifecycle.confirm',
  });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
}
it('keeps C1 as default and mounts lifecycle capabilities/list/detail only after the explicit entry', async () => {
  mount(true, false);
  const entry = await screen.findByRole('button', {
    name: 'knowledgeLifecycle.title',
  });
  expect(
    backend.calls.some((config) => config.url.includes('source-lifecycle')),
  ).toBe(false);
  fireEvent.click(entry);
  const link = await screen.findByRole('link', { name: 'src/A.tsx' });
  fireEvent.click(link);
  await actions();
  expect(
    backend.calls.some((config) =>
      config.url.endsWith('/source-lifecycles/' + Source),
    ),
  ).toBe(true);
});
it('archives and restores with fresh consent, showing the fixed receipt separately from current details', async () => {
  mount();
  let panel = await selectAction('archive');
  expect(
    within(panel).getByRole('button', { name: 'knowledgeLifecycle.confirm' }),
  ).toBeDisabled();
  await confirm(panel);
  await screen.findByRole('region', { name: 'knowledgeLifecycle.receipt' });
  await waitFor(() => expect(backend.current.current.state).toBe('archived'));
  await waitFor(() =>
    expect(
      screen.getByRole('region', { name: 'knowledgeLifecycle.currentDetails' }),
    ).toHaveTextContent('knowledgeLifecycle.archived'),
  );
  panel = await actions();
  expect(within(panel).getByLabelText('knowledgeLifecycle.action')).toHaveValue(
    'restore',
  );
  expect(within(panel).getByRole('checkbox')).not.toBeChecked();
  await confirm(panel);
  await waitFor(() => expect(backend.current.current.state).toBe('active'));
  expect(backend.posts.map((item) => item.action)).toEqual([
    'archive',
    'restore',
  ]);
});
it('uses complete classification and resets acknowledgement on target change and manual refresh', async () => {
  mount();
  let panel = await actions(),
    box = within(panel).getByRole('checkbox');
  fireEvent.click(box);
  expect(box).toBeChecked();
  fireEvent.change(within(panel).getByLabelText('knowledgeImport.language'), {
    target: { value: 'go' },
  });
  expect(box).not.toBeChecked();
  fireEvent.click(box);
  fireEvent.click(
    within(panel).getByRole('button', { name: 'knowledgeLifecycle.refresh' }),
  );
  await waitFor(() =>
    expect(
      within(
        screen.getByRole('region', { name: 'knowledgeLifecycle.actions' }),
      ).getByRole('checkbox'),
    ).not.toBeChecked(),
  );
  panel = await actions();
  fireEvent.change(within(panel).getByLabelText('knowledgeImport.language'), {
    target: { value: 'go' },
  });
  await confirm(panel);
  await waitFor(() => expect(backend.posts).toHaveLength(1));
  expect(backend.posts[0].request).toMatchObject({
    classification: {
      group_id: null,
      component_side: 'frontend',
      language: 'go',
    },
    expected_lifecycle_version: 5,
    expected_policy_version: 1,
  });
});
it('only offers exact registered move paths and requires a separate non-reserving target check', async () => {
  mount();
  const panel = await selectAction('move');
  const select = await within(panel).findByLabelText(
    'knowledgeLifecycle.targetPath',
  );
  await waitFor(() =>
    expect(within(select).getAllByRole('option')).toHaveLength(3),
  );
  fireEvent.change(select, { target: { value: 'src/B.tsx' } });
  expect(within(panel).getByRole('checkbox')).toBeDisabled();
  fireEvent.click(within(panel).getByText('knowledgeLifecycle.checkTarget'));
  await within(panel).findByText(/knowledgeLifecycle.targetAllowed/);
  expect(backend.posts).toHaveLength(0);
  await confirm(panel);
  await waitFor(() =>
    expect(backend.current.current.relative_path).toBe('src/B.tsx'),
  );
  expect(backend.posts[0].request).toMatchObject({
    relative_path: 'src/B.tsx',
  });
});
it.each([
  'direct_C1_nullable_registration',
  'controlled_directory_no_move',
  'web_page_no_move',
])('disables unsupported movement for %s', async (name) => {
  backend.current = copy<Detail>(name);
  mount();
  const panel = await selectAction('move');
  expect(within(panel).getByRole('checkbox')).toBeDisabled();
  expect(
    within(panel).queryByText('knowledgeLifecycle.checkTarget'),
  ).not.toBeInTheDocument();
  expect(
    within(panel).getByText('knowledgeLifecycle.reason.action_unsupported'),
  ).toBeVisible();
});
it('preserves group scope and distinguishes empty versus omitted language while paginating and filtering', async () => {
  mount(false, false, '3'.repeat(32));
  const form = await screen.findByRole('form', {
    name: 'knowledgeLifecycle.filters',
  });
  fireEvent.change(
    within(form).getByLabelText('knowledgeLifecycle.groupScope'),
    { target: { value: 'ungrouped' } },
  );
  fireEvent.change(
    within(form).getByLabelText('knowledgeLifecycle.languageFilter'),
    { target: { value: 'empty' } },
  );
  fireEvent.submit(form);
  await waitFor(() =>
    expect(
      backend.calls
        .filter((config) => config.url.endsWith('/source-lifecycles'))
        .at(-1)?.params,
    ).toMatchObject({ group_scope: 'ungrouped', language: '', limit: 20 }),
  );
  fireEvent.change(
    within(form).getByLabelText('knowledgeLifecycle.languageFilter'),
    { target: { value: 'all' } },
  );
  fireEvent.submit(form);
  await waitFor(() =>
    expect(
      backend.calls
        .filter((config) => config.url.endsWith('/source-lifecycles'))
        .at(-1)?.params,
    ).not.toHaveProperty('language'),
  );
});
it('creates current fixed C1 and Web links without treating the decoded digest as the original capture digest', async () => {
  mount();
  fireEvent.click(await screen.findByText('knowledgeLifecycle.prepareRead'));
  await screen.findByRole('link', { name: 'knowledgeImport.readRevision' });
  expect(
    screen.getByRole('link', { name: 'knowledgeImport.readRevision' }),
  ).toHaveAttribute('href', expect.stringContaining('node=' + '6'.repeat(64)));
});
it('clears the catalogue private view if lifecycle detail loses current authorization', async () => {
  backend.denied = true;
  mount(true);
  await screen.findByText('knowledgeCatalogue.notVisible');
  expect(
    screen.queryByRole('region', { name: 'knowledgeLifecycle.actions' }),
  ).not.toBeInTheDocument();
});
it('provides a stable source entry beside existing fixed reading without issuing lifecycle requests', () => {
  render(
    <MemoryRouter>
      <LifecycleSourceLink domain={Domain} source={Scope.source} />
    </MemoryRouter>,
  );
  expect(screen.getByRole('link')).toHaveAttribute(
    'href',
    '/dataset/knowledge-catalogue/' + Domain + '?lifecycle_source=' + Source,
  );
  expect(Http).not.toHaveBeenCalled();
});

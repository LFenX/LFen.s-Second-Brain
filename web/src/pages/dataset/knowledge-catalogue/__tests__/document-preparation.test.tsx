import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  render as renderView,
  screen,
  waitFor,
  fireEvent,
} from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { DocumentPolicyPanel } from '../document-policy';
import { DocumentPreparation } from '../document-prepare';
import {
  useDocumentPolicy,
  useDocumentHead,
} from '@/hooks/use-knowledge-document';
import { Scope, Rep, Caps, Policy, clone } from './document-fixture';
function render(ui: import('react').ReactElement) {
  const client = new QueryClient();
  return renderView(ui, {
    wrapper: ({ children }: { children: import('react').ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-document', () => {
  const actual = jest.requireActual('@/hooks/use-knowledge-document');
  return {
    ...actual,
    useDocumentPolicy: jest.fn(),
    useDocumentHead: jest.fn(),
  };
});
const Original = {
  ...Rep.original,
  relative_path: Rep.source_snapshot.relative_path,
  classification: Rep.source_snapshot.classification,
  is_current: true,
  created_at: Rep.published_at,
  parse_status: 'not_parsed',
  structure_status: 'not_built',
  full_text_status: 'not_built',
  vector_status: 'not_built',
} as import('@/interfaces/database/knowledge-original').OriginalRevision;
const Prepared = {
  original: Original,
  snapshot: Rep.source_snapshot,
  profile: Caps.profiles.find((p) => p.format === 'docx')!,
  request: {
    target: {
      kind: 'original_document',
      source_id: Original.source_id,
      source_revision_id: Original.source_revision_id,
      original_sha256: Original.sha256,
      expected_lifecycle_version: Rep.source_snapshot.lifecycle_version,
    },
    expected_policy_version: Policy.foundation_policy_version,
    expected_document_policy_version: Policy.document_policy_version,
    expected_active_representation_id: null,
    profile_id: 'docx-ooxml-blocks/1',
    expected_producer_sha256: Rep.producer_sha256,
  },
};
beforeEach(() => {
  jest.clearAllMocks();
  (useDocumentPolicy as jest.Mock).mockReturnValue({
    data: { ...Policy, enabled: false },
    isPending: false,
    isError: false,
  });
  (useDocumentHead as jest.Mock).mockReturnValue({
    data: {
      currentness: 'none',
      reasons: [],
      active_representation_id: null,
      latest_task_id: null,
    },
    isError: false,
  });
});
it('requires new policy acknowledgement after either version changes', () => {
  const flow = { ready: true, busy: false, submit: jest.fn() } as any;
  const { rerender } = render(
    <DocumentPolicyPanel scope={Scope} flow={flow} />,
  );
  fireEvent.click(screen.getByRole('checkbox'));
  expect(
    screen.getByRole('button', { name: 'knowledgeDocument.enable' }),
  ).toBeEnabled();
  (useDocumentPolicy as jest.Mock).mockReturnValue({
    data: {
      ...Policy,
      enabled: false,
      document_policy_version: Policy.document_policy_version + 1,
    },
    isPending: false,
    isError: false,
  });
  rerender(<DocumentPolicyPanel scope={Scope} flow={flow} />);
  expect(screen.getByRole('checkbox')).not.toBeChecked();
  expect(
    screen.getByRole('button', { name: 'knowledgeDocument.enable' }),
  ).toBeDisabled();
  expect(flow.submit).not.toHaveBeenCalled();
});
it('rechecks the original tuple and stops without POST when head changes after acknowledgement', async () => {
  const fresh = clone(Prepared);
  fresh.request.expected_active_representation_id = 'f'.repeat(64) as any;
  const flow = {
    ready: true,
    busy: false,
    preparationEpoch: 0,
    prepare: jest
      .fn()
      .mockResolvedValueOnce(Prepared)
      .mockResolvedValueOnce(fresh),
    submit: jest.fn(),
  } as any;
  render(
    <MemoryRouter>
      <DocumentPreparation
        scope={Scope}
        original={Original}
        flow={flow}
        onTask={jest.fn()}
      />
    </MemoryRouter>,
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDocument.prepare' }),
  );
  await screen.findByRole('checkbox');
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDocument.start' }),
  );
  await screen.findByText('document_preparation_changed');
  expect(flow.submit).not.toHaveBeenCalled();
  expect(screen.queryByRole('checkbox')).toBeNull();
});
it('persists and submits only after fresh preparation still matches the approved snapshot', async () => {
  const flow = {
      ready: true,
      busy: false,
      preparationEpoch: 0,
      prepare: jest.fn().mockResolvedValue(Prepared),
      submit: jest
        .fn()
        .mockResolvedValue({ record_kind: 'task', task_id: 'a'.repeat(32) }),
    } as any,
    onTask = jest.fn();
  render(
    <MemoryRouter>
      <DocumentPreparation
        scope={Scope}
        original={Original}
        flow={flow}
        onTask={onTask}
      />
    </MemoryRouter>,
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDocument.prepare' }),
  );
  await screen.findByRole('checkbox');
  expect(flow.submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDocument.start' }),
  );
  await waitFor(() => expect(flow.submit).toHaveBeenCalledTimes(1));
  expect(flow.prepare).toHaveBeenCalledTimes(2);
  expect(flow.submit.mock.calls[0][0].body).toMatchObject({
    ...Prepared.request,
    confirmed: true,
    idempotency_key: expect.stringMatching(/^[A-Za-z0-9_-]{1,128}$/),
  });
});
it('shows all stale reasons without pretending archive revoked access', () => {
  (useDocumentHead as jest.Mock).mockReturnValue({
    data: {
      currentness: 'stale',
      reasons: [
        'source_changed',
        'lifecycle_changed',
        'classification_changed',
        'position_changed',
        'source_archived',
        'policy_changed',
        'producer_changed',
      ],
      active_representation_id: Rep.representation_id,
      active_source_revision_id: Original.source_revision_id,
      latest_task_id: null,
    },
    isError: false,
  });
  const flow = { ready: true, busy: false, preparationEpoch: 0 } as any;
  render(
    <MemoryRouter>
      <DocumentPreparation
        scope={Scope}
        original={Original}
        flow={flow}
        onTask={jest.fn()}
      />
    </MemoryRouter>,
  );
  for (const reason of [
    'source_changed',
    'lifecycle_changed',
    'classification_changed',
    'position_changed',
    'source_archived',
    'policy_changed',
    'producer_changed',
  ])
    expect(
      screen.getByText('knowledgeDocument.reason_' + reason),
    ).toBeInTheDocument();
  expect(
    screen.getByRole('link', { name: 'knowledgeDocument.open' }),
  ).toHaveAttribute('href', expect.stringContaining(Rep.representation_id));
});

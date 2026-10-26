import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as hooks from '@/hooks/use-knowledge-structure';
import { useStructureFlow } from '../use-structure-flow';
import StructureBrowser from '../structure';
import { StructurePolicyPanel } from '../structure-policy';
import { StructurePrepare, parseStructureAliases } from '../structure-prepare';
import { StructureFixedView, StructureHeadPanel } from '../structure-view';
import { StructureRelations } from '../structure-relations';
import { StructureContent } from '../structure-content';
import { StructureTaskTracker, StructureRecovery } from '../structure-tasks';
import {
  Scope,
  ProjectScope,
  ViewScope,
  Task,
  View,
  Node,
  Content,
  Policy,
  Capabilities,
  Head,
  Relation,
  Entry,
  TaskOperation,
  CancelOperation,
  PolicyReceipt,
  rejection,
  sample,
  manifestEntry,
  running,
} from './structure-fixture';
import {
  saveStructureSlot,
  structureSlotKey,
} from '@/utils/knowledge-structure';
type ReactNode = import('react').ReactNode;
type Flow = import('../use-structure-flow').StructureFlow;
type Prepared =
  import('@/services/knowledge-structure-service').StructurePrepared;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/theme-provider', () => ({
  useTheme: () => ({ theme: 'light' }),
}));
jest.mock('@monaco-editor/react', () => ({
  __esModule: true,
  loader: { config: jest.fn() },
  default: ({ value }: { value: string }) => (
    <pre data-testid="structure-code">{value}</pre>
  ),
}));
jest.mock('@/components/ui/resizable', () => ({
  ResizablePanelGroup: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  ResizablePanel: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  ResizableHandle: () => null,
}));
jest.mock('../use-structure-flow', () => ({ useStructureFlow: jest.fn() }));
jest.mock('../../knowledge-import/batch-manifest', () => ({
  ProjectPicker: ({ onChoose }: { onChoose: (id: string) => void }) => {
    const choose = () => onChoose('be0206919af323472454bb60a4b1ec58');
    return <button onClick={choose}>choose-synthetic-project</button>;
  },
}));
jest.mock('@/hooks/use-knowledge-structure', () => {
  const actual = jest.requireActual('@/hooks/use-knowledge-structure');
  return {
    ...actual,
    useStructureContract: jest.fn(),
    useStructurePolicy: jest.fn(),
    useStructureManifestHead: jest.fn(),
    useStructureManifest: jest.fn(),
    useStructureHead: jest.fn(),
    useStructureTasks: jest.fn(),
    useStructureTask: jest.fn(),
    useStructureView: jest.fn(),
    useStructureNodes: jest.fn(),
    useStructureNode: jest.fn(),
    useStructureRelations: jest.fn(),
    useStructureContent: jest.fn(),
  };
});
function mount(children: ReactNode) {
  const client = new QueryClient();
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>,
  );
}
const result = (data: unknown) => ({
  data,
  isError: false,
  isPending: false,
  isSuccess: true,
  refetch: jest.fn(),
  hasNextPage: false,
  isFetchingNextPage: false,
  fetchNextPage: jest.fn(),
});
let flow: Flow;
beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  flow = {
    slots: [],
    outcomes: {},
    rejections: {},
    ready: true,
    preparationEpoch: 0,
    invalidatePreparation: jest.fn(),
    busy: false,
    error: null,
    submit: jest.fn(),
    recover: jest.fn(),
    resend: jest.fn(),
    abandon: jest.fn(),
    reconcile: jest.fn().mockResolvedValue(undefined),
    prepare: jest.fn(),
    refresh: jest.fn(),
  } as unknown as Flow;
  (useStructureFlow as jest.Mock).mockReturnValue(flow);
  (hooks.useStructureContract as jest.Mock).mockReturnValue(
    result(Capabilities),
  );
  (hooks.useStructurePolicy as jest.Mock).mockReturnValue(result(Policy));
  (hooks.useStructureManifestHead as jest.Mock).mockReturnValue(
    result({ manifest_id: Task.original_request.manifest_id }),
  );
  (hooks.useStructureManifest as jest.Mock).mockReturnValue(
    result({ pages: [{ items: [manifestEntry()] }] }),
  );
  (hooks.useStructureHead as jest.Mock).mockReturnValue(result(Head));
  (hooks.useStructureTasks as jest.Mock).mockReturnValue(
    result({ pages: [{ items: [] }] }),
  );
  (hooks.useStructureTask as jest.Mock).mockReturnValue({
    ...result(running()),
    waiting: true,
    resume: jest.fn(),
  });
  (hooks.useStructureView as jest.Mock).mockReturnValue(result(View));
  (hooks.useStructureNodes as jest.Mock).mockReturnValue(
    result({ pages: [{ items: [] }] }),
  );
  (hooks.useStructureNode as jest.Mock).mockReturnValue(result(Node));
  (hooks.useStructureRelations as jest.Mock).mockReturnValue(
    result({ pages: [{ items: [Relation] }] }),
  );
  (hooks.useStructureContent as jest.Mock).mockReturnValue(result(Content));
});
it('mounts project manifest, task list and head only after explicit project selection; fixed view stays lazy', async () => {
  mount(<StructureBrowser owner={Scope.owner} domain={Scope.domain} />);
  expect(hooks.useStructureManifestHead).not.toHaveBeenCalled();
  expect(hooks.useStructureTasks).not.toHaveBeenCalled();
  expect(hooks.useStructureView).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('choose-synthetic-project'));
  expect(
    await screen.findByText('knowledgeStructure.noManifest').catch(() => null),
  ).toBeNull();
  expect(hooks.useStructureManifestHead).toHaveBeenCalledWith(ProjectScope);
  expect(hooks.useStructureTasks).toHaveBeenCalledWith(ProjectScope);
  expect(hooks.useStructureView).not.toHaveBeenCalled();
});
it('requires explicit policy confirmation and separately rereads the current policy', async () => {
  (flow.submit as jest.Mock).mockResolvedValue(PolicyReceipt);
  mount(<StructurePolicyPanel scope={Scope} flow={flow} />);
  const button = screen.getByRole('button', {
    name: 'knowledgeStructure.disable',
  });
  expect(button).toBeDisabled();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(button);
  await waitFor(() => expect(flow.submit).toHaveBeenCalled());
  expect(flow.submit).toHaveBeenCalledWith(
    expect.objectContaining({
      kind: 'policy',
      body: expect.objectContaining({
        enabled: false,
        expected_policy_version: Policy.foundation_policy_version,
        expected_structure_policy_version: Policy.structure_policy_version,
        confirmed: true,
      }),
    }),
  );
  expect(screen.getByRole('checkbox')).not.toBeChecked();
  expect(
    (hooks.useStructurePolicy as jest.Mock).mock.results[0].value.refetch,
  ).toHaveBeenCalled();
});
it('checks exact selection, preserves aliases, resets acknowledgement on inputs and never accepts a late stale preparation', async () => {
  const prepared: Prepared = {
    request: {
      ...Task.original_request,
      members: [Task.original_request.members[2]],
    },
    entries: [manifestEntry()],
    policy: Policy,
    head: Head,
  };
  (flow.prepare as jest.Mock).mockResolvedValue(prepared);
  (flow.submit as jest.Mock).mockResolvedValue(Task);
  const onTask = jest.fn();
  mount(<StructurePrepare scope={ProjectScope} flow={flow} onTask={onTask} />);
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.change(screen.getByLabelText('knowledgeStructure.aliases'), {
    target: { value: '@/=frontend/' },
  });
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeStructure.prepare' }),
  );
  await screen.findByText('knowledgeStructure.preparedHint');
  expect(flow.prepare).toHaveBeenCalledWith(
    ProjectScope,
    Task.original_request.manifest_id,
    [manifestEntry()],
    [{ prefix: '@/', target: 'frontend/' }],
  );
  const submit = screen.getByRole('button', {
    name: 'knowledgeStructure.createTask',
  });
  expect(submit).toBeDisabled();
  fireEvent.click(screen.getByLabelText('knowledgeStructure.confirmTask'));
  expect(submit).toBeEnabled();
  fireEvent.change(screen.getByLabelText('knowledgeStructure.aliases'), {
    target: { value: '@/=' },
  });
  expect(
    screen.queryByText('knowledgeStructure.preparedHint'),
  ).not.toBeInTheDocument();
  expect(flow.submit).not.toHaveBeenCalled();
  let finish!: (value: Prepared) => void;
  (flow.prepare as jest.Mock).mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeStructure.prepare' }),
  );
  fireEvent.change(screen.getByLabelText('knowledgeStructure.aliases'), {
    target: { value: '@/=new/' },
  });
  await act(async () => {
    finish(prepared);
  });
  expect(
    screen.queryByText('knowledgeStructure.preparedHint'),
  ).not.toBeInTheDocument();
  expect(parseStructureAliases('@/=frontend/')).toEqual([
    { prefix: '@/', target: 'frontend/' },
  ]);
  expect(() => parseStructureAliases('@/=frontend')).toThrow();
});
it('displays global stale reasons even when affected sources are empty and never switches fixed history automatically', () => {
  (hooks.useStructureHead as jest.Mock).mockReturnValue(
    result({ ...sample<object>('head-stale'), affected_sources: [] }),
  );
  const onView = jest.fn();
  mount(
    <StructureHeadPanel
      scope={ProjectScope}
      fixed={'f'.repeat(64)}
      onView={onView}
    />,
  );
  expect(
    screen.getByText(/knowledgeStructure.staleReasons/),
  ).toBeInTheDocument();
  expect(
    screen.getByText('knowledgeStructure.fixedDifferent'),
  ).toBeInTheDocument();
  expect(onView).not.toHaveBeenCalled();
});
it('renders escaped HTML and literal image URLs through the read-only local code viewer', () => {
  (hooks.useStructureContent as jest.Mock).mockReturnValue(
    result({
      ...Content,
      text: '<img src="https://external.invalid/x.png">\n![x](https://external.invalid/y.png)',
    }),
  );
  mount(<StructureContent scope={ViewScope} node={Node} member={Entry} />);
  expect(screen.getByTestId('structure-code')).toHaveTextContent(
    '<img src="https://external.invalid/x.png">',
  );
  expect(document.querySelector('img')).toBeNull();
  expect(hooks.useStructureContent).toHaveBeenCalledWith(
    ViewScope,
    Node,
    Entry.sha256,
    {},
  );
});
it('retains unresolved references, evidence and reasons without a guessed target action', () => {
  (hooks.useStructureRelations as jest.Mock).mockReturnValue(
    result({ pages: [{ items: [sample('unresolved-dynamic')] }] }),
  );
  const onNode = jest.fn();
  mount(
    <StructureRelations
      scope={ViewScope}
      view={View}
      node={Node.structure_node_id}
      onNode={onNode}
    />,
  );
  expect(screen.getByText(/dynamic_import/)).toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: 'knowledgeStructure.openTarget' }),
  ).not.toBeInTheDocument();
  expect(screen.getByText(/knowledgeStructure.evidence/)).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('knowledgeStructure.direction'), {
    target: { value: 'out' },
  });
  expect(hooks.useStructureRelations).toHaveBeenLastCalledWith(ViewScope, {
    node_id: Node.structure_node_id,
    direction: 'out',
    limit: 20,
  });
});
it('follows a resolved cross-file target with its member, tree and fixed origin', async () => {
  const target = View.members.find(
    (member) => member.source_id !== Entry.source_id,
  )!;
  const members = [Entry, target];
  const roots = members.map((member) => ({
    ...Node,
    structure_node_id: member.root_structure_node_id,
    source_id: member.source_id,
    source_revision_id: member.source_revision_id,
    source_node_id: member.source_node_id,
    parser: member.parser,
    classification: member.classification,
    name: member.relative_path,
    kind: 'file' as const,
    identity_stability: 'file' as const,
    parent_id: null,
    start_byte: 0,
    end_byte: member.byte_length,
  }));
  (hooks.useStructureView as jest.Mock).mockReturnValue(
    result({ ...View, members, source_count: members.length }),
  );
  (hooks.useStructureNode as jest.Mock).mockImplementation((_, id) =>
    result(roots.find((node) => node.structure_node_id === id)),
  );
  (hooks.useStructureRelations as jest.Mock).mockReturnValue(
    result({
      pages: [
        {
          items: [
            {
              ...Relation,
              from_node_id: Entry.root_structure_node_id,
              to_node_id: target.root_structure_node_id,
              resolution: 'resolved',
              evidence: {
                ...Relation.evidence,
                source_id: Entry.source_id,
                source_revision_id: Entry.source_revision_id,
                start_byte: 0,
                end_byte: Math.min(8, Entry.byte_length),
              },
            },
          ],
        },
      ],
    }),
  );
  mount(<StructureFixedView scope={ViewScope} />);
  const initialLabel = `${Entry.relative_path} · ${Entry.classification.component_side} / ${Entry.classification.language}`;
  const targetLabel = `${target.relative_path} · ${target.classification.component_side} / ${target.classification.language}`;
  expect(screen.getByRole('button', { name: initialLabel })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeStructure.openTarget' }),
  );
  await waitFor(() => {
    expect(screen.getByRole('button', { name: targetLabel })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });
  expect(screen.getByRole('button', { name: initialLabel })).toHaveAttribute(
    'aria-pressed',
    'false',
  );
  expect(hooks.useStructureNodes).toHaveBeenLastCalledWith(ViewScope, {
    source_id: target.source_id,
    limit: 20,
  });
  expect(
    screen.getByRole('heading', {
      name: `${target.relative_path} · knowledgeStructure.node_file`,
    }),
  ).toBeInTheDocument();
});
it('uses task revision for cancellation and keeps cancel_requested in the tracking view', async () => {
  (flow.submit as jest.Mock).mockResolvedValue({});
  mount(
    <StructureTaskTracker
      scope={Scope}
      taskId={Task.task_id}
      flow={flow}
      onView={jest.fn()}
    />,
  );
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeStructure.cancel' }),
  );
  await waitFor(() =>
    expect(flow.submit).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'cancel',
        scope: { ...Scope, task: Task.task_id },
        body: expect.objectContaining({
          expected_task_revision: Task.revision,
        }),
      }),
    ),
  );
  expect(screen.getByRole('checkbox')).not.toBeChecked();
  expect(flow.reconcile).toHaveBeenCalled();
});
it('offers abandonment only for a verified fixed GET rejection and keeps task recovery separate', async () => {
  const task = await saveStructureSlot(localStorage, TaskOperation),
    cancel = await saveStructureSlot(localStorage, CancelOperation),
    proof = await rejection(CancelOperation);
  flow.slots = [task, cancel];
  flow.outcomes = { [structureSlotKey(cancel)]: proof };
  const view = mount(<StructureRecovery flow={flow} onTask={jest.fn()} />);
  expect(
    screen.queryByRole('button', { name: 'knowledgeStructure.abandon' }),
  ).not.toBeInTheDocument();
  flow.rejections = { [structureSlotKey(cancel)]: true };
  view.rerender(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <StructureRecovery flow={flow} onTask={jest.fn()} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeStructure.abandon' }),
  );
  expect(flow.abandon).toHaveBeenCalledWith(cancel);
  expect(flow.slots).toHaveLength(2);
});
it('advances content only with the returned next_start_byte, never by guessed text length', () => {
  (hooks.useStructureContent as jest.Mock).mockReturnValue(
    result({ ...Content, complete: false, next_start_byte: 100 }),
  );
  mount(<StructureContent scope={ViewScope} node={Node} member={Entry} />);
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeStructure.nextWindow' }),
  );
  expect(hooks.useStructureContent).toHaveBeenLastCalledWith(
    ViewScope,
    Node,
    Entry.sha256,
    { start_byte: 100 },
  );
});
it('keeps all writes disabled when the advertised runtime is not ready', () => {
  (hooks.useStructureContract as jest.Mock).mockReturnValue(
    result({ ...Capabilities, runtime_ready: false }),
  );
  mount(<StructureBrowser owner={Scope.owner} domain={Scope.domain} />);
  expect(
    screen.getByText('knowledgeStructure.runtimeNotReady'),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText('knowledgeStructure.confirmDisable'));
  expect(
    screen.getByRole('button', { name: 'knowledgeStructure.disable' }),
  ).toBeDisabled();
});

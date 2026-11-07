import { useState } from 'react';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import request from '@/utils/next-request';
import { ClassificationFields } from '../fields';
import {
  OriginalRegistrationForm,
  OriginalFileFields,
} from '../original-fields';
import { useOriginalFlow } from '../use-original-flow';
import originalService, {
  OriginalRequestError,
} from '@/services/knowledge-original-service';
import {
  originalScope,
  originalStorageKey,
  saveOriginalRecovery,
} from '@/utils/knowledge-original';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'synthetic-group-ui',
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/file-uploader', () => ({
  FileUploader: () => <div />,
}));
const Http = request as unknown as jest.Mock;
const Id = 'a'.repeat(32),
  Owner = 'b'.repeat(32),
  GroupId = 'c'.repeat(32);
const Domain = {
  domain_id: Id,
  name: 'Synthetic domain',
  category: 'project',
  owner_id: Owner,
  contract_version: '1.0.0',
  policy: {
    version: 1,
    access: 'owner_only',
    index_modes: ['catalogue'],
    allowed_model_uses: [],
    external_models: false,
    automatic_ingestion: false,
  },
};
const Group = {
  id: GroupId,
  domain_id: Id,
  parent_id: null,
  kind: 'project',
  name: 'Same name',
  component_side: 'shared',
};
const respond = (data: unknown, status = 200) => ({
  status,
  data: { code: 0, data, contract_version: '1.0.0', request_id: 'synthetic' },
});
const Changed = jest.fn(),
  Submitted = jest.fn();
let Groups: unknown[];
function transport(config: { method: string; url: string }) {
  if (config.method === 'post') {
    Groups = [Group];
    return respond(Group, 201);
  }
  if (config.url.endsWith('/domain')) return respond(Domain);
  return respond({ items: Groups, next_cursor: '' });
}
function Harness({ domain = Id }: { domain?: string }) {
  const [value, setValue] = useState({
    group_id: null as string | null,
    component_side: 'backend' as const,
    language: 'go',
  });
  const change = (next: typeof value) => {
    Changed(next);
    setValue(next);
  };
  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    Submitted();
  };
  return (
    <form onSubmit={submit}>
      <ClassificationFields
        domainId={domain}
        value={value}
        onChange={change as never}
      />
      <button type="submit">source-submit</button>
      <output>{JSON.stringify(value)}</output>
    </form>
  );
}
function mount(node = <Harness />) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      {node}
    </QueryClientProvider>,
  );
}
async function openCreator() {
  fireEvent.click(screen.getByText('knowledgeImport.group:', { exact: false }));
  fireEvent.click(
    await screen.findByRole('button', { name: 'groupCreate.title' }),
  );
  return screen.findByLabelText('groupCreate.name');
}
beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    configurable: true,
    value: () => '00000000-0000-4000-8000-000000000001',
  });
  Groups = [];
  Changed.mockReset();
  Submitted.mockReset();
  Http.mockReset().mockImplementation(transport);
});

it('keeps creation outside form ownership and selects a verified root group only on explicit action', async () => {
  mount();
  const input = (await openCreator()) as HTMLInputElement;
  expect(input.form).toBeNull();
  expect(fireEvent.keyDown(input, { key: 'Enter' })).toBe(false);
  fireEvent.keyDown(input, { key: 'Enter', isComposing: true });
  expect(Submitted).not.toHaveBeenCalled();
  expect(document.querySelectorAll('form')).toHaveLength(1);
  fireEvent.change(input, { target: { value: 'Same name' } });
  fireEvent.click(screen.getByRole('button', { name: 'groupCreate.create' }));
  await screen.findByText('groupCreate.created: Same name');
  expect(Changed).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'groupCreate.select' }));
  await waitFor(() =>
    expect(Changed).toHaveBeenCalledWith({
      group_id: GroupId,
      component_side: 'backend',
      language: 'go',
    }),
  );
  expect(Http.mock.calls.find(([c]) => c.method === 'post')[0].data).toEqual({
    parent_id: null,
    kind: 'project',
    name: 'Same name',
    component_side: 'shared',
  });
  expect(Submitted).not.toHaveBeenCalled();
});

it('retains unknown state, blocks repeated creation/scope switching and distinguishes same-name recovery IDs', async () => {
  const other = { ...Group, id: 'd'.repeat(32) };
  Groups = [Group, other];
  Http.mockImplementation((config) =>
    config.method === 'post'
      ? Promise.reject(new Error('synthetic lost reply'))
      : transport(config),
  );
  mount();
  fireEvent.change(await openCreator(), { target: { value: 'Same name' } });
  const create = screen.getByRole('button', { name: 'groupCreate.create' });
  fireEvent.click(create);
  fireEvent.click(create);
  await screen.findByText('groupCreate.unknown');
  expect(create).toBeDisabled();
  expect(
    screen.getByRole('button', { name: 'knowledgeImport.noGroup' }),
  ).toBeDisabled();
  const details = screen.getAllByText(other.id)[0].closest('li')!;
  fireEvent.click(
    within(details).getByRole('button', { name: 'groupCreate.checkChoice' }),
  );
  await screen.findByText('groupCreate.found: Same name');
  expect(
    screen.queryByText('groupCreate.created: Same name'),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'groupCreate.select' }));
  await waitFor(() =>
    expect(Changed).toHaveBeenCalledWith({
      group_id: other.id,
      component_side: 'backend',
      language: 'go',
    }),
  );
  expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(1);
});

it.each([
  'busy',
  'cancelling',
  'unknown',
  'confirmation',
  'upload',
  'blocked',
  'recoveryChanged',
  'lockingUnavailable',
  'corruptRecovery',
  'registration',
])(
  'passes original registration %s state into the actual shared classification owner',
  async (state) => {
    const flow = {
      busy: state === 'busy',
      cancelling: state === 'cancelling',
      createPending:
        state === 'registration'
          ? { kind: 'registration', registration_key: 'synthetic-key' }
          : state === 'upload'
            ? { kind: 'upload' }
            : null,
      confirmPending: state === 'confirmation' ? {} : null,
      unknown: state === 'unknown',
      blocked: state === 'blocked',
      recoveryChanged: state === 'recoveryChanged',
      lockingUnavailable: state === 'lockingUnavailable',
      corruptRecovery: state === 'corruptRecovery',
      canResend: false,
      register: jest.fn(),
      fail: jest.fn(),
    };
    mount(<OriginalRegistrationForm domain={Id} flow={flow as never} />);
    fireEvent.click(
      screen.getByText('knowledgeImport.group:', { exact: false }),
    );
    expect(
      await screen.findByRole('button', { name: 'groupCreate.title' }),
    ).toBeDisabled();
    const existing = screen.getByRole('button', {
      name: 'knowledgeImport.noGroup',
    });
    if (state === 'busy' || state === 'cancelling')
      expect(existing).toBeDisabled();
    else expect(existing).toBeEnabled();
    expect(Http.mock.calls.filter(([c]) => c.method === 'post')).toHaveLength(
      0,
    );
  },
);

it('does not disable existing-group preparation for an already-restored registration descriptor', async () => {
  const registration = {
    registration_id: 'e'.repeat(32),
    name: 'Restored synthetic',
    include_paths: ['web/synthetic.ts'],
    default_classification: {
      group_id: null,
      component_side: 'backend',
      language: 'go',
    },
  };
  const flow = {
    registration,
    createPending: { kind: 'registration', registration_key: 'synthetic-key' },
    busy: false,
    cancelling: false,
    unknown: false,
    blocked: false,
    recoveryChanged: false,
    corruptRecovery: false,
    prepare: jest.fn(),
  };
  mount(<OriginalFileFields domain={Id} flow={flow as never} />);
  fireEvent.click(screen.getByText('knowledgeImport.group:', { exact: false }));
  expect(
    await screen.findByRole('button', { name: 'knowledgeImport.noGroup' }),
  ).toBeEnabled();
  expect(
    screen.getByRole('button', { name: 'groupCreate.title' }),
  ).toBeEnabled();
});

it('shows a real group refusal and allows correcting the unsent draft without selecting a group', async () => {
  Http.mockImplementation((config) =>
    config.method === 'post'
      ? Promise.reject({
          response: { status: 409, data: { code: 'knowledge_group_rejected' } },
        })
      : transport(config),
  );
  mount();
  fireEvent.change(await openCreator(), { target: { value: 'Same name' } });
  fireEvent.click(screen.getByRole('button', { name: 'groupCreate.create' }));
  await screen.findByText('knowledgeDomain.rejected');
  expect(screen.getByLabelText('groupCreate.name')).toBeEnabled();
  expect(Changed).not.toHaveBeenCalled();
});

it('keeps existing groups selectable but blocks new groups in the actual C02 by-key-404 retry state', async () => {
  const key = originalStorageKey(Id, Owner);
  saveOriginalRecovery(key, 'create', {
    ...originalScope(Id, Owner),
    kind: 'registration',
    registration_key: 'synthetic-group-boundary',
    config_sha256: 'f'.repeat(64),
    registration_id: null,
  });
  const snapshot = localStorage.getItem(key + ':create');
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: (_name: string, callback: () => Promise<unknown>) =>
        Promise.resolve().then(callback),
    },
  });
  const byKey = jest
    .spyOn(originalService, 'registrationByKey')
    .mockRejectedValue(new OriginalRequestError(404, 'knowledge_not_found'));
  const register = jest.spyOn(originalService, 'register');
  Groups = [Group];
  function RecoveredForm() {
    const flow = useOriginalFlow(Id, Owner, key);
    const handleRecover = () => {
      void flow.recover();
    };
    return (
      <>
        <button type="button" onClick={handleRecover}>
          recover-synthetic-boundary
        </button>
        <output data-testid="actual-c02-state">
          {JSON.stringify({
            busy: flow.busy,
            unknown: flow.unknown,
            canResend: flow.canResend,
            blocked: flow.blocked,
            pending: flow.createPending?.kind,
          })}
        </output>
        <OriginalRegistrationForm domain={Id} flow={flow} />
      </>
    );
  }
  mount(<RecoveredForm />);
  fireEvent.click(
    screen.getByRole('button', { name: 'recover-synthetic-boundary' }),
  );
  await waitFor(() =>
    expect(screen.getByTestId('actual-c02-state')).toHaveTextContent(
      '"canResend":true',
    ),
  );
  expect(screen.getByTestId('actual-c02-state')).toHaveTextContent(
    '"busy":false',
  );
  expect(screen.getByTestId('actual-c02-state')).toHaveTextContent(
    '"blocked":false',
  );
  expect(screen.getByTestId('actual-c02-state')).toHaveTextContent(
    '"pending":"registration"',
  );
  fireEvent.click(screen.getByText('knowledgeImport.group:', { exact: false }));
  expect(
    screen.getByRole('button', { name: 'groupCreate.title' }),
  ).toBeDisabled();
  const existing = await screen.findByRole('button', { name: Group.name });
  expect(existing).toBeEnabled();
  fireEvent.click(existing);
  expect(
    screen.getByText('knowledgeImport.group:', { exact: false }),
  ).toHaveTextContent(Group.name);
  expect(localStorage.getItem(key + ':create')).toBe(snapshot);
  expect(register).not.toHaveBeenCalled();
  expect(
    Http.mock.calls.filter(([config]) => config.method === 'post'),
  ).toHaveLength(0);
  byKey.mockRestore();
  register.mockRestore();
});

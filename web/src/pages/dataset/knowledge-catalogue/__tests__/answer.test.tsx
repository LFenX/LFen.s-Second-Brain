import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { webcrypto } from 'node:crypto';
import KnowledgeAnswer from '../answer';
import { useKnowledgeAnswer } from '@/hooks/use-knowledge-answer';
type AnswerPolicy =
  import('@/interfaces/database/knowledge-answer').AnswerPolicy;

jest.mock('@/hooks/use-knowledge-answer', () => ({
  useKnowledgeAnswer: jest.fn(),
}));
jest.mock('@/hooks/use-llm-request', () => ({
  useFetchDefaultModels: () => ({
    data: [{ model_id: 'b'.repeat(32), model_type: 'chat', enable: true }],
  }),
}));
jest.mock('@/hooks/use-knowledge-foundation', () => ({
  useKnowledgeSource: () => ({ data: { relative_path: 'synthetic.txt' } }),
}));
jest.mock('@/hooks/use-knowledge-local-lexical', () => ({
  useLocalLexicalReadiness: () => ({
    data: {
      readiness: {
        kind: 'ready',
        publication: 'published',
        index_ref: {
          index_id: '1'.repeat(64),
          manifest_sha256: '2'.repeat(64),
        },
      },
    },
    refetch: jest.fn(),
  }),
}));
jest.mock('../local-lexical-selection', () => ({
  LocalLexicalPicker: ({ onAdd }: any) => {
    const add = () =>
      onAdd({
        path: 'synthetic.txt',
        target: {
          source_ref: {
            domain_id: '1'.repeat(32),
            source_id: 'e'.repeat(32),
            source_revision_id: 'd'.repeat(32),
            source_sha256: '3'.repeat(64),
          },
          representation_ref: {
            kind: 'source_revision',
            id: 'd'.repeat(32),
            producer_sha256: null,
          },
        },
      });
    return <button onClick={add}>选择合成资料</button>;
  },
}));
jest.mock('@/components/ui/button', () => ({
  Button: ({ variant: _variant, ...props }: any) => {
    void _variant;
    return <button {...props} />;
  },
}));
jest.mock('@/components/ui/input', () => ({
  Input: (props: any) => <input {...props} />,
}));
const Scope = { domain: '1'.repeat(32), owner: 'a'.repeat(32) };
Object.defineProperty(globalThis, 'crypto', {
  configurable: true,
  value: webcrypto,
});
const Disabled: AnswerPolicy = {
  domain_id: Scope.domain,
  version: 0,
  enabled: false,
  scope: null,
  scope_sha256: null,
  model_ref: null,
  consent_actor: null,
  consent_time: null,
};
const Target = {
  source_ref: {
    domain_id: Scope.domain,
    source_id: 'e'.repeat(32),
    source_revision_id: 'd'.repeat(32),
    source_sha256: '3'.repeat(64),
  },
  representation_ref: {
    kind: 'source_revision' as const,
    id: 'd'.repeat(32),
    producer_sha256: null,
  },
};
const Enabled: AnswerPolicy = {
  ...Disabled,
  version: 1,
  enabled: true,
  scope: {
    index_ref: { index_id: '1'.repeat(64), manifest_sha256: '2'.repeat(64) },
    selection: { kind: 'fixed_representations', targets: [Target] },
  },
  scope_sha256: '3'.repeat(64),
  model_ref: { model_id: 'b'.repeat(32), configuration_sha256: 'c'.repeat(64) },
  consent_actor: Scope.owner,
  consent_time: '2026-11-17T00:00:00Z',
};
function flow(permit = Disabled) {
  return {
    policy: {
      data: permit,
      refetch: jest.fn().mockResolvedValue({ data: permit }),
    },
    inspect: {
      mutateAsync: jest.fn().mockResolvedValue({
        policy_version: 0,
        scope: Enabled.scope,
        scope_sha256: Enabled.scope_sha256,
        model_ref: Enabled.model_ref,
        model_display_name: 'deepseek-flash',
        destination_origin: 'https://api.deepseek.com',
      }),
    },
    setPolicy: { mutateAsync: jest.fn(), isPending: false },
    ask: { mutateAsync: jest.fn() },
  };
}
function mount() {
  return render(
    <MemoryRouter>
      <KnowledgeAnswer {...Scope} />
    </MemoryRouter>,
  );
}
beforeEach(() => jest.clearAllMocks());

it('stays off until a fixed snapshot is inspected and the owner explicitly checks consent', async () => {
  const actions = flow();
  jest.mocked(useKnowledgeAnswer).mockReturnValue(actions as any);
  mount();
  expect(
    screen.getByText('当前已关闭，不会发送问题或资料给模型。'),
  ).toBeInTheDocument();
  expect(actions.ask.mutateAsync).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('选择合成资料'));
  fireEvent.click(screen.getByText('预览资料外发范围'));
  const enable = await screen.findByText('确认并开启此资料范围的问答');
  expect(enable).toBeDisabled();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(enable);
  await waitFor(() =>
    expect(actions.setPolicy.mutateAsync).toHaveBeenCalledTimes(1),
  );
  expect(actions.setPolicy.mutateAsync.mock.calls[0][0]).toMatchObject({
    enabled: true,
    confirmed: true,
    external_send_confirmed: true,
    scope: Enabled.scope,
    model_ref: Enabled.model_ref,
  });
  expect(actions.ask.mutateAsync).not.toHaveBeenCalled();
});

it('queries an uncertain original request using the same key without silently resubmitting', async () => {
  const actions = flow(Enabled);
  actions.ask.mutateAsync
    .mockRejectedValueOnce(Error('timeout'))
    .mockResolvedValueOnce({ state: 'running' });
  jest.mocked(useKnowledgeAnswer).mockReturnValue(actions as any);
  mount();
  fireEvent.change(screen.getByLabelText('你的问题'), {
    target: { value: 'ORION quota?' },
  });
  fireEvent.click(screen.getByText('提问'));
  const poll = await screen.findByText('查询本次结果（不重复调用模型）');
  const first = actions.ask.mutateAsync.mock.calls[0][0].body;
  expect(actions.ask.mutateAsync).toHaveBeenCalledTimes(1);
  fireEvent.click(poll);
  await waitFor(() => expect(actions.ask.mutateAsync).toHaveBeenCalledTimes(2));
  expect(actions.ask.mutateAsync.mock.calls[1][0].body).toEqual(first);
});

it('discards an in-flight answer immediately when the owner closes the policy', async () => {
  let finish!: (value: unknown) => void;
  const actions = flow(Enabled);
  actions.ask.mutateAsync.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  jest.mocked(useKnowledgeAnswer).mockReturnValue(actions as any);
  mount();
  fireEvent.change(screen.getByLabelText('你的问题'), {
    target: { value: 'ORION quota?' },
  });
  fireEvent.click(screen.getByText('提问'));
  await waitFor(() => expect(actions.ask.mutateAsync).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByText('关闭模型问答'));
  await act(async () =>
    finish({
      state: 'completed',
      answer: 'PRIVATE-LATE-ANSWER',
      citations: [],
    }),
  );
  expect(screen.queryByText('PRIVATE-LATE-ANSWER')).not.toBeInTheDocument();
  expect(actions.setPolicy.mutateAsync.mock.calls[0][0]).toMatchObject({
    enabled: false,
    scope: null,
    model_ref: null,
    external_send_confirmed: false,
  });
});

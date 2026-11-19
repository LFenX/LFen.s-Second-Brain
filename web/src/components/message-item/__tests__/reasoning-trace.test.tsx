import { IReference } from '@/interfaces/database/chat';
import { fireEvent, render, screen } from '@testing-library/react';
import {
  parseAnswerMetrics,
  splitAssistantContent,
  summarizeCitations,
} from '../reasoning-chain';
import { ReasoningTrace } from '../reasoning-trace';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) =>
      options ? `${key}${JSON.stringify(options)}` : key,
  }),
}));

// react-markdown is ESM-only; the reasoning text is checked as plain text.
jest.mock('../../markdown-content', () => ({
  __esModule: true,
  default: ({ content }: { content: string }) => <div>{content}</div>,
}));

const reference: IReference = {
  chunks: [
    {
      id: 'c0',
      content: null,
      document_id: 'docA',
      document_name: 'Alpha.pdf',
      dataset_id: 'kb',
      image_id: '',
      similarity: 0.8,
      vector_similarity: 0.7,
      term_similarity: 0.9,
      positions: [],
    },
  ],
  doc_aggs: [{ doc_id: 'docA', doc_name: 'Alpha.pdf', count: 1 }],
  total: 1,
};

const traceOf = (
  content: string,
  live: boolean,
  options: { prompt?: string; hasKnowledge?: boolean } = {},
) => {
  const parts = splitAssistantContent(content);
  return (
    <ReasoningTrace
      live={live}
      parts={parts}
      metrics={parseAnswerMetrics(options.prompt)}
      citations={summarizeCitations(
        `${parts.thinking}\n${parts.answer}`,
        live ? undefined : reference,
      )}
      reference={reference}
      hasKnowledge={options.hasKnowledge}
    />
  );
};

describe('ReasoningTrace', () => {
  it('shows every planned step once, in one card, before anything streams', () => {
    render(traceOf('', true, { hasKnowledge: true }));

    expect(screen.queryByRole('button')).toBeNull();
    expect(screen.getAllByText('chat.reasoningTrace.title')).toHaveLength(1);
    expect(screen.getByText('chat.reasoningTrace.stage.retrieve')).toBeTruthy();
    expect(
      screen.getByText('chat.reasoningTrace.describe.retrieve.active'),
    ).toBeTruthy();
    expect(screen.getByText('chat.reasoningTrace.stage.compose')).toBeTruthy();
    expect(
      screen.getByText('chat.reasoningTrace.describe.compose.todo'),
    ).toBeTruthy();
    expect(screen.getByText('chat.reasoningTrace.stage.verify')).toBeTruthy();
  });

  it('streams the reasoning text under its step', () => {
    render(traceOf('<think>weighing the evidence', true));

    expect(
      screen.getByText('chat.reasoningTrace.describe.reason.active'),
    ).toBeTruthy();
    expect(screen.getByText('weighing the evidence')).toBeTruthy();
  });

  it('times reasoning that closes in the same update that ends the stream', () => {
    const { rerender } = render(traceOf('', true));
    rerender(traceOf('<think>plan', true));
    rerender(traceOf('<think>plan</think>Answer', false));

    const pill = screen.getByRole('button', { expanded: false });
    expect(pill.textContent).toMatch(
      /chat\.reasoningTrace\.summaryReason\{"value":"\d+ms"\}/,
    );
  });

  it('folds a finished answer into a summary that opens to the steps', () => {
    render(
      traceOf('<think>plan</think>Answer [ID:0]', false, {
        prompt:
          '## Time elapsed:\n  - Total: 2400ms\n  - Retrieval: 300ms\n  - Generate answer: 2000ms\n  - Generated tokens(approximately): 120\n  - Token speed: 50/s',
      }),
    );

    const pill = screen.getByRole('button', { expanded: false });
    expect(pill.textContent).toContain(
      'chat.reasoningTrace.summaryRetrieved{"count":1}',
    );
    expect(pill.textContent).toContain(
      'chat.reasoningTrace.summaryCited{"count":1}',
    );
    expect(pill.textContent).toContain('2.4s');
    expect(screen.queryByText('chat.reasoningTrace.stage.retrieve')).toBeNull();

    fireEvent.click(pill);

    expect(
      screen.getByText(
        'chat.reasoningTrace.describe.retrieve.found{"chunks":1,"documents":1}',
      ),
    ).toBeTruthy();
    expect(screen.getByText('chat.reasoningTrace.stage.reason')).toBeTruthy();
    expect(
      screen.getByText(
        'chat.reasoningTrace.describe.compose.tokens{"tokens":120,"speed":50}',
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        'chat.reasoningTrace.describe.verify.done{"cited":1,"documents":1}',
      ),
    ).toBeTruthy();
    expect(screen.getByText('chat.reasoningTrace.phase.generate')).toBeTruthy();
  });
});

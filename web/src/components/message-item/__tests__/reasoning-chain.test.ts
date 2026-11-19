import { IReference } from '@/interfaces/database/chat';
import {
  buildTraceStages,
  parseAnswerMetrics,
  parseResearchSteps,
  splitAssistantContent,
  summarizeCitations,
} from '../reasoning-chain';

const chunk = (id: string, documentId: string, similarity?: number) =>
  ({
    id,
    content: null,
    document_id: documentId,
    document_name: `${documentId}.pdf`,
    dataset_id: 'kb',
    image_id: '',
    similarity,
    vector_similarity: 0,
    term_similarity: 0,
    positions: [],
  }) as any;

describe('splitAssistantContent', () => {
  it('separates research, reasoning and answer', () => {
    const parts = splitAssistantContent(
      '<retrieving>Searching by `a`...</retrieving><think>weigh evidence</think>\n\nThe answer [ID:0].',
    );
    expect(parts).toEqual({
      research: 'Searching by `a`...',
      researchOpen: false,
      thinking: 'weigh evidence',
      thinkingOpen: false,
      answer: 'The answer [ID:0].',
    });
  });

  it('marks a block that is still streaming as open', () => {
    const parts = splitAssistantContent('<think>still going');
    expect(parts.thinking).toBe('still going');
    expect(parts.thinkingOpen).toBe(true);
    expect(parts.answer).toBe('');
  });

  it('drops empty reasoning and stray closing tags', () => {
    const parts = splitAssistantContent('<think>  </think></think>Hi');
    expect(parts.thinking).toBe('');
    expect(parts.answer).toBe('Hi');
  });
});

describe('parseResearchSteps', () => {
  const log =
    'Searching by `q1`...Retrieval 5 results in 12.5ms' +
    'Checking the sufficiency for retrieved information.' +
    "Next step is to search for the following questions:</br> - Who? it's me</br> - Why?" +
    'Searching by `who`...Retrieval 0 results in 3.0ms' +
    "Yes, the retrieved information is sufficient for 'Who? it's me'.";

  it('reads every sentence the deep researcher emits', () => {
    expect(parseResearchSteps(log)).toEqual([
      { kind: 'search', query: 'q1' },
      { kind: 'retrieved', count: 5, ms: 12.5 },
      { kind: 'check' },
      { kind: 'expand', questions: ["Who? it's me", 'Why?'] },
      { kind: 'search', query: 'who' },
      { kind: 'retrieved', count: 0, ms: 3 },
      { kind: 'sufficient', question: "Who? it's me" },
    ]);
  });

  it('holds back a sentence still arriving in an open block', () => {
    expect(
      parseResearchSteps('Searching by `q1`...Searching by `q', true),
    ).toEqual([{ kind: 'search', query: 'q1' }]);
    expect(parseResearchSteps('Unknown progress')).toEqual([
      { kind: 'note', text: 'Unknown progress' },
    ]);
  });
});

describe('parseAnswerMetrics', () => {
  const prompt = [
    'System prompt mentioning - Retrieval: 999ms that must be ignored',
    '### Query:  ',
    'question  ',
    '## Time elapsed:  ',
    '  - Total: 3420.5ms  ',
    '  - Check LLM: 2.0ms  ',
    '  - Check Langfuse tracer: 1.0ms  ',
    '  - Bind models: 7.0ms  ',
    '  - Query refinement(LLM): 410.0ms  ',
    '  - Retrieval: 300.2ms  ',
    '  - Generate answer: 2600.0ms  ',
    '  ',
    '  - Generated tokens(approximately): 512  ',
    '  - Token speed: 149/s',
  ].join('\n');

  it('reads the phase timings and token stats', () => {
    expect(parseAnswerMetrics(prompt)).toEqual({
      totalMs: 3420.5,
      prepareMs: 10,
      refineMs: 410,
      retrievalMs: 300.2,
      generateMs: 2600,
      tokens: 512,
      tokensPerSecond: 149,
    });
  });

  it('returns nothing without a timing block', () => {
    expect(parseAnswerMetrics('### Query:\nq')).toBeUndefined();
    expect(parseAnswerMetrics(undefined)).toBeUndefined();
  });
});

describe('summarizeCitations', () => {
  const reference: IReference = {
    chunks: [
      chunk('c0', 'docA', 0.42),
      chunk('c1', 'docB', 0.81),
      chunk('c2', 'docA', 0.77),
      chunk('c3', 'docC'),
    ],
    doc_aggs: [
      { doc_id: 'docA', doc_name: 'Alpha.pdf', count: 2, url: '' },
      { doc_id: 'docB', doc_name: 'Beta.md', count: 1 },
    ],
    total: 4,
  };

  it('groups cited chunks by document in reading order', () => {
    const summary = summarizeCitations(
      'B first [ID:1], then A [ID:2] and A again [ID:0][ID:2], missing [ID:9].',
      reference,
    );
    expect(summary.sources).toEqual([
      {
        documentId: 'docB',
        documentName: 'Beta.md',
        url: undefined,
        chunkIndexes: [1],
        bestSimilarity: 0.81,
      },
      {
        documentId: 'docA',
        documentName: 'Alpha.pdf',
        url: '',
        chunkIndexes: [2, 0],
        bestSimilarity: 0.77,
      },
    ]);
    expect(summary.citedChunkCount).toBe(3);
    expect(summary.retrievedChunkCount).toBe(4);
    expect(summary.retrievedDocumentCount).toBe(3);
    expect(summary.quoteEnabled).toBe(true);
  });

  it('knows when citations are turned off', () => {
    const summary = summarizeCitations('text', {
      chunks: reference.chunks,
    } as IReference);
    expect(summary.quoteEnabled).toBe(false);
    expect(summary.sources).toEqual([]);
  });
});

describe('buildTraceStages', () => {
  const noCitations = summarizeCitations('', undefined);

  it('lays out the planned stages from the start of a live answer', () => {
    const live = (content: string, hasKnowledge = true) =>
      buildTraceStages({
        live: true,
        parts: splitAssistantContent(content),
        citations: noCitations,
        hasKnowledge,
      });

    expect(live('')).toEqual([
      { key: 'retrieve', state: 'active' },
      { key: 'compose', state: 'todo' },
      { key: 'verify', state: 'todo' },
    ]);
    expect(live('<think>hmm')).toEqual([
      { key: 'retrieve', state: 'done' },
      { key: 'reason', state: 'active' },
      { key: 'compose', state: 'todo' },
      { key: 'verify', state: 'todo' },
    ]);
    expect(live('<think>hmm</think>The answer')).toEqual([
      { key: 'retrieve', state: 'done' },
      { key: 'reason', state: 'done' },
      { key: 'compose', state: 'active' },
      { key: 'verify', state: 'todo' },
    ]);
    expect(live('<retrieving>Searching by `a`...')).toEqual([
      { key: 'retrieve', state: 'active' },
      { key: 'compose', state: 'todo' },
      { key: 'verify', state: 'todo' },
    ]);
  });

  it('plans no retrieval for a chat without a knowledge base', () => {
    expect(
      buildTraceStages({
        live: true,
        parts: splitAssistantContent(''),
        citations: noCitations,
      }),
    ).toEqual([{ key: 'compose', state: 'active' }]);
  });

  it('closes a finished answer with retrieval and citation checks', () => {
    const reference: IReference = {
      chunks: [chunk('c0', 'docA', 0.5)],
      doc_aggs: [{ doc_id: 'docA', doc_name: 'A', count: 1 }],
      total: 1,
    };
    const metrics = parseAnswerMetrics(
      '## Time elapsed:\n  - Total: 10ms\n  - Query refinement(LLM): 2ms\n  - Retrieval: 3ms\n  - Generate answer: 5ms',
    );

    expect(
      buildTraceStages({
        live: false,
        parts: splitAssistantContent('Answer [ID:0]'),
        metrics,
        citations: summarizeCitations('Answer [ID:0]', reference),
      }),
    ).toEqual([
      { key: 'refine', state: 'done' },
      { key: 'retrieve', state: 'done' },
      { key: 'compose', state: 'done' },
      { key: 'verify', state: 'done' },
    ]);

    expect(
      buildTraceStages({
        live: false,
        parts: splitAssistantContent('Answer without markers'),
        metrics,
        citations: summarizeCitations('Answer without markers', reference),
      }).at(-1),
    ).toEqual({ key: 'verify', state: 'warn' });
  });

  it('flags a knowledge base search that found nothing', () => {
    const metrics = parseAnswerMetrics(
      '## Time elapsed:\n  - Total: 10ms\n  - Retrieval: 3ms',
    );
    expect(
      buildTraceStages({
        live: false,
        parts: splitAssistantContent('No idea.'),
        metrics,
        citations: summarizeCitations('No idea.', {
          chunks: [],
          doc_aggs: [],
          total: 0,
        }),
      }),
    ).toEqual([
      { key: 'retrieve', state: 'warn' },
      { key: 'compose', state: 'done' },
    ]);
  });
});

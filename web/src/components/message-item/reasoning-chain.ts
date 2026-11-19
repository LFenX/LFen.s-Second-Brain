import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { buildCitationDisplayMap, CitationKey } from '@/utils/citation-utils';

/** An assistant message split into what it did on the way and what it answered. */
export interface AssistantContentParts {
  /** Model reasoning streamed inside <think>…</think>. */
  thinking: string;
  /** True while a <think> block has opened and not yet closed. */
  thinkingOpen: boolean;
  /** Deep-research progress streamed inside <retrieving>…</retrieving>. */
  research: string;
  /** True while a <retrieving> block has opened and not yet closed. */
  researchOpen: boolean;
  /** The answer shown to the reader. */
  answer: string;
}

const takeTagBlocks = (text: string, tag: string) => {
  const open = `<${tag}>`;
  const close = `</${tag}>`;
  const inner: string[] = [];
  let rest = '';
  let isOpen = false;
  let cursor = 0;

  while (cursor < text.length) {
    const start = text.indexOf(open, cursor);
    if (start < 0) {
      rest += text.slice(cursor);
      break;
    }
    rest += text.slice(cursor, start);
    const end = text.indexOf(close, start + open.length);
    if (end < 0) {
      // Still streaming: everything after the opener belongs to the block.
      inner.push(text.slice(start + open.length));
      isOpen = true;
      break;
    }
    inner.push(text.slice(start + open.length, end));
    cursor = end + close.length;
  }

  return {
    inner: inner
      .map((part) => part.trim())
      .filter(Boolean)
      .join('\n\n'),
    isOpen,
    rest: rest.split(close).join(''),
  };
};

export function splitAssistantContent(content = ''): AssistantContentParts {
  const research = takeTagBlocks(content, 'retrieving');
  const thinking = takeTagBlocks(research.rest, 'think');
  return {
    thinking: thinking.inner,
    thinkingOpen: thinking.isOpen,
    research: research.inner,
    researchOpen: research.isOpen,
    answer: thinking.rest.trim(),
  };
}

export type ResearchStep =
  | { kind: 'search'; query: string }
  | { kind: 'retrieved'; count: number; ms: number }
  | { kind: 'check' }
  | { kind: 'sufficient'; question: string }
  | { kind: 'expand'; questions: string[] }
  | { kind: 'note'; text: string };

// The deep researcher (internal/service/deep_researcher.go) streams these
// sentences back to back with no separator, so free-text captures stop where
// the next known sentence begins.
const NextResearchSentence =
  '(?=Searching by `|Retrieval \\d+ results in|Checking the sufficiency for retrieved information|Yes, the retrieved information is sufficient for|Next step is to search for the following questions:|$)';
const ResearchSentenceReg = new RegExp(
  [
    'Searching by `([\\s\\S]*?)`\\.\\.\\.',
    'Retrieval (\\d+) results in ([\\d.]+)ms',
    'Checking the sufficiency for retrieved information\\.',
    `Yes, the retrieved information is sufficient for '([\\s\\S]*?)'\\.${NextResearchSentence}`,
    `Next step is to search for the following questions:([\\s\\S]*?)${NextResearchSentence}`,
  ].join('|'),
  'g',
);

const pushNote = (steps: ResearchStep[], text: string) => {
  const note = text.replace(/<\/?br\s*\/?>/gi, ' ').trim();
  if (note) steps.push({ kind: 'note', text: note });
};

/** Parses deep-research progress into ordered steps. */
export function parseResearchSteps(text = '', open = false): ResearchStep[] {
  const steps: ResearchStep[] = [];
  let cursor = 0;

  for (const match of text.matchAll(ResearchSentenceReg)) {
    pushNote(steps, text.slice(cursor, match.index));
    cursor = (match.index ?? 0) + match[0].length;

    if (match[1] !== undefined) {
      steps.push({ kind: 'search', query: match[1].trim() });
    } else if (match[2] !== undefined) {
      steps.push({
        kind: 'retrieved',
        count: Number(match[2]),
        ms: Number(match[3]),
      });
    } else if (match[4] !== undefined) {
      steps.push({ kind: 'sufficient', question: match[4].trim() });
    } else if (match[5] !== undefined) {
      const questions = match[5]
        .split(/<\/?br\s*\/?>/i)
        .map((line) => line.replace(/^\s*-\s*/, '').trim())
        .filter(Boolean);
      steps.push({ kind: 'expand', questions });
    } else {
      steps.push({ kind: 'check' });
    }
  }

  // A trailing fragment of an open block is a sentence still arriving.
  if (!open) pushNote(steps, text.slice(cursor));
  return steps;
}

export interface AnswerMetrics {
  totalMs: number;
  /** Model check, tracer check and model binding before the pipeline runs. */
  prepareMs: number;
  refineMs: number;
  retrievalMs: number;
  generateMs: number;
  tokens?: number;
  tokensPerSecond?: number;
}

const PhaseLineReg = /-\s*([A-Za-z][A-Za-z ()]*?):\s*([\d.]+)\s*ms/g;

/**
 * Reads the `## Time elapsed:` block the Go pipeline appends to the final
 * answer's prompt (internal/common/timer.go Markdown).
 */
export function parseAnswerMetrics(prompt?: string): AnswerMetrics | undefined {
  const start = prompt?.indexOf('## Time elapsed:') ?? -1;
  if (!prompt || start < 0) return undefined;
  const section = prompt.slice(start);

  const phases = new Map<string, number>();
  for (const match of section.matchAll(PhaseLineReg)) {
    phases.set(match[1].trim().toLowerCase(), Number(match[2]));
  }
  const totalMs = phases.get('total');
  if (totalMs === undefined || Number.isNaN(totalMs)) return undefined;

  const phase = (name: string) => phases.get(name) ?? 0;
  const tokens = section.match(/Generated tokens\(approximately\):\s*(\d+)/);
  const speed = section.match(/Token speed:\s*(\d+)\s*\/s/);

  return {
    totalMs,
    prepareMs:
      phase('check llm') +
      phase('check langfuse tracer') +
      phase('bind models'),
    refineMs: phase('query refinement(llm)'),
    retrievalMs: phase('retrieval'),
    generateMs: phase('generate answer'),
    tokens: tokens ? Number(tokens[1]) : undefined,
    tokensPerSecond: speed ? Number(speed[1]) : undefined,
  };
}

export interface CitedSource {
  documentId: string;
  documentName: string;
  url?: string;
  /** 0-based indexes into reference.chunks, in the order the text cites them. */
  chunkIndexes: number[];
  bestSimilarity?: number;
}

export interface CitationSummary {
  sources: CitedSource[];
  citedChunkCount: number;
  retrievedChunkCount: number;
  retrievedDocumentCount: number;
  /** False when the chat has citations turned off (no doc_aggs shipped). */
  quoteEnabled: boolean;
}

const chunkIndexOf = (key: CitationKey, chunks: IReferenceChunk[]) =>
  typeof key === 'number' ? key : chunks.findIndex((chunk) => chunk.id === key);

const similarityOf = (chunk: IReferenceChunk) =>
  typeof chunk.similarity === 'number' && Number.isFinite(chunk.similarity)
    ? chunk.similarity
    : undefined;

/** Groups the chunks cited in `text` by document, in order of first citation. */
export function summarizeCitations(
  text: string,
  reference?: IReference,
): CitationSummary {
  const chunks = Array.isArray(reference?.chunks) ? reference.chunks : [];
  const documents = Array.isArray(reference?.doc_aggs)
    ? reference.doc_aggs
    : [];
  const sources = new Map<string, CitedSource>();
  let citedChunkCount = 0;

  for (const key of buildCitationDisplayMap(text).keys()) {
    const index = chunkIndexOf(key, chunks);
    const chunk = chunks[index];
    if (!chunk?.document_id) continue;
    citedChunkCount += 1;

    let source = sources.get(chunk.document_id);
    if (!source) {
      const document = documents.find((x) => x?.doc_id === chunk.document_id);
      source = {
        documentId: chunk.document_id,
        documentName: document?.doc_name || chunk.document_name || '',
        url: document?.url,
        chunkIndexes: [],
      };
      sources.set(chunk.document_id, source);
    }
    source.chunkIndexes.push(index);
    const similarity = similarityOf(chunk);
    if (
      similarity !== undefined &&
      (source.bestSimilarity === undefined ||
        similarity > source.bestSimilarity)
    ) {
      source.bestSimilarity = similarity;
    }
  }

  return {
    sources: [...sources.values()],
    citedChunkCount,
    retrievedChunkCount: chunks.length,
    retrievedDocumentCount: new Set(
      chunks.map((chunk) => chunk.document_id).filter(Boolean),
    ).size,
    quoteEnabled: Array.isArray(reference?.doc_aggs),
  };
}

export type TraceStageKey =
  | 'refine'
  | 'retrieve'
  | 'reason'
  | 'compose'
  | 'verify';
export type TraceStageState = 'todo' | 'active' | 'done' | 'warn';
export interface TraceStage {
  key: TraceStageKey;
  state: TraceStageState;
}

/**
 * The chain of work behind an answer, in pipeline order.
 *
 * While the answer streams, the stages the pipeline will run are laid out
 * from the start: retrieval when the chat has a knowledge base, then
 * composing, then the citation check. The stage under way is the one the
 * stream points at; later ones wait. Once the answer is final, only stages
 * with evidence in the payload remain, so the chain never claims work that
 * did not happen.
 */
export function buildTraceStages({
  live,
  parts,
  metrics,
  citations,
  hasKnowledge = false,
}: {
  live: boolean;
  parts: AssistantContentParts;
  metrics?: AnswerMetrics;
  citations: CitationSummary;
  hasKnowledge?: boolean;
}): TraceStage[] {
  return live
    ? buildLiveStages(parts, hasKnowledge)
    : buildFinalStages(parts, metrics, citations);
}

function buildLiveStages(
  parts: AssistantContentParts,
  hasKnowledge: boolean,
): TraceStage[] {
  const reasoning = parts.thinkingOpen || Boolean(parts.thinking);
  const writing =
    Boolean(parts.answer) ||
    (!parts.thinkingOpen &&
      !parts.researchOpen &&
      Boolean(parts.thinking || parts.research));
  const retrieving =
    parts.researchOpen || (!reasoning && !writing && !parts.research);
  const stages: TraceStage[] = [];

  if (hasKnowledge || parts.research) {
    stages.push({ key: 'retrieve', state: retrieving ? 'active' : 'done' });
  }
  if (reasoning) {
    stages.push({
      key: 'reason',
      state: parts.thinkingOpen ? 'active' : 'done',
    });
  }
  stages.push({
    key: 'compose',
    state: writing ? 'active' : retrieving && !hasKnowledge ? 'active' : 'todo',
  });
  if (hasKnowledge) {
    stages.push({ key: 'verify', state: 'todo' });
  }
  return stages;
}

function buildFinalStages(
  parts: AssistantContentParts,
  metrics: AnswerMetrics | undefined,
  citations: CitationSummary,
): TraceStage[] {
  const stages: TraceStage[] = [];

  if ((metrics?.refineMs ?? 0) > 0) {
    stages.push({ key: 'refine', state: 'done' });
  }

  const retrievalMs = metrics?.retrievalMs ?? 0;
  if (parts.research || citations.retrievedChunkCount > 0 || retrievalMs > 0) {
    stages.push({
      key: 'retrieve',
      state:
        citations.retrievedChunkCount === 0 && retrievalMs > 0
          ? 'warn'
          : 'done',
    });
  }

  if (parts.thinking) {
    stages.push({ key: 'reason', state: 'done' });
  }

  if (parts.answer) {
    stages.push({ key: 'compose', state: 'done' });
  }

  if (citations.quoteEnabled && citations.retrievedChunkCount > 0) {
    stages.push({
      key: 'verify',
      state: citations.citedChunkCount > 0 ? 'done' : 'warn',
    });
  }

  return stages;
}

export const formatDuration = (ms: number) =>
  ms < 1000 ? `${Math.max(0, Math.round(ms))}ms` : `${(ms / 1000).toFixed(1)}s`;

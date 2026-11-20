/**
 * Module-level state of 波西米亚. The conversation, the setup and the stream
 * live here rather than in the panel, so an answer keeps arriving while the
 * person follows one of its links to another page and the panel remounts.
 */
import { IReference } from '@/interfaces/database/chat';
import { IDataset } from '@/interfaces/database/dataset';
import { IAddedModel } from '@/interfaces/database/llm';
import {
  BohemiaRequestError,
  listAddedModels,
  listDefaultModels,
  listMyDatasets,
  requestBohemiaCompletion,
} from '@/services/bohemia-assistant-service';
import {
  parseCompletionEventStream,
  readJsonSafely,
} from '@/services/chat-completion-stream';
import { mergeAnswerChunk } from '@/utils/chat';
import { v4 as uuid } from 'uuid';
import { create } from 'zustand';
import { answerForHistory } from './actions';
import { BohemiaReady, prepareBohemia } from './bootstrap';
import {
  BohemiaStorageKeys,
  HistoryTurns,
  KeptTurns,
  SettableModelType,
} from './constants';
import { NamedPages, datasetPagePath } from './pages';
import { buildSituation } from './situation';

export type ActionRun = {
  state: 'running' | 'done' | 'failed' | 'declined';
  message?: string;
  href?: string;
};

export type Turn = {
  id: string;
  question: string;
  answer: string;
  reference?: IReference;
  error?: string;
  at: number;
  ms?: number;
  /** The state of each action card, by its index under the answer. */
  runs: Record<number, ActionRun>;
};

export type Setup =
  | { state: 'idle' }
  | { state: 'preparing'; note: string }
  | BohemiaReadySetup
  | { state: 'needs-models'; missing: SettableModelType[] }
  | { state: 'failed'; message: string };
export type BohemiaReadySetup = { state: 'ready' } & Omit<BohemiaReady, 'kind'>;

type BohemiaState = {
  open: boolean;
  setup: Setup;
  turns: Turn[];
  busy: boolean;
  /** The person's datasets as last read, for names on cards and the picker. */
  datasets: IDataset[];
  /** The added models as last read, for model names on cards. */
  models: IAddedModel[];
  setOpen(open: boolean): void;
  toggle(): void;
  prepare(force?: boolean): Promise<void>;
  send(question: string, page: string): Promise<void>;
  stop(): void;
  clear(): void;
  setRun(turnId: string, index: number, run: ActionRun): void;
  setConnected(ids: string[]): void;
  setDatasets(datasets: IDataset[]): void;
};

const readOpen = () => {
  try {
    return sessionStorage.getItem(BohemiaStorageKeys.open) === 'open';
  } catch {
    return false;
  }
};

const readTurns = (): Turn[] => {
  try {
    const value = JSON.parse(
      sessionStorage.getItem(BohemiaStorageKeys.turns) || '[]',
    );
    return Array.isArray(value)
      ? value
          .filter(
            (item) =>
              item &&
              typeof item.question === 'string' &&
              typeof item.answer === 'string',
          )
          .map((item) => ({
            ...item,
            runs: Object.fromEntries(
              Object.entries(item.runs ?? {}).map(([index, run]) => [
                index,
                // A card still running when the tab closed cannot finish.
                (run as ActionRun).state === 'running'
                  ? {
                      state: 'failed',
                      message: '页面刷新时还没有结果，请到对应页面确认。',
                    }
                  : run,
              ]),
            ),
          }))
          .slice(-KeptTurns)
      : [];
  } catch {
    return [];
  }
};

const keep = (key: string, value: string) => {
  try {
    sessionStorage.setItem(key, value);
  } catch {
    /* Not kept; it still works for this visit. */
  }
};

let controller: AbortController | null = null;

/** A reference with its chunks as an array, whatever shape it arrived in. */
export function normalizeReference(value: unknown): IReference | undefined {
  if (!value || typeof value !== 'object') return;
  const raw = value as { chunks?: unknown; doc_aggs?: unknown; total?: number };
  const list = <T>(item: unknown): T[] =>
    Array.isArray(item)
      ? item
      : item && typeof item === 'object'
        ? Object.values(item)
        : [];
  const chunks = list<IReference['chunks'][number]>(raw.chunks);
  if (!chunks.length) return;
  return {
    chunks,
    doc_aggs: list(raw.doc_aggs),
    total: raw.total ?? chunks.length,
  };
}

export const useBohemiaStore = create<BohemiaState>((set, get) => {
  const patchTurn = (id: string, change: (turn: Turn) => Partial<Turn>) =>
    set((state) => ({
      turns: state.turns.map((turn) =>
        turn.id === id ? { ...turn, ...change(turn) } : turn,
      ),
    }));

  return {
    open: readOpen(),
    setup: { state: 'idle' },
    turns: readTurns(),
    busy: false,
    datasets: [],
    models: [],

    setOpen(open) {
      keep(BohemiaStorageKeys.open, open ? 'open' : '');
      set({ open });
    },
    toggle() {
      get().setOpen(!get().open);
    },

    async prepare(force = false) {
      const current = get().setup.state;
      if (!force && (current === 'ready' || current === 'preparing')) return;
      set({ setup: { state: 'preparing', note: '正在准备…' } });
      try {
        const result = await prepareBohemia((note) =>
          set((state) =>
            state.setup.state === 'preparing'
              ? { setup: { state: 'preparing', note } }
              : {},
          ),
        );
        if (result.kind === 'needs-models') {
          set({ setup: { state: 'needs-models', missing: result.missing } });
          return;
        }
        set({
          setup: {
            state: 'ready',
            chatId: result.chatId,
            guideDatasetId: result.guideDatasetId,
            guideEmbedding: result.guideEmbedding,
            datasetIds: result.datasetIds,
          },
        });
      } catch (error) {
        set({
          setup: {
            state: 'failed',
            message:
              error instanceof Error ? error.message : '准备波西米亚失败',
          },
        });
      }
    },

    async send(text, page) {
      const question = text.trim();
      if (!question || get().busy) return;
      if (get().setup.state !== 'ready') await get().prepare();
      const setup = get().setup;
      if (setup.state !== 'ready') return;

      const earlier = get()
        .turns.filter((turn) => turn.answer && !turn.error)
        .slice(-HistoryTurns)
        .flatMap((turn) => [
          { role: 'user' as const, content: turn.question },
          {
            role: 'assistant' as const,
            content: answerForHistory(turn.answer),
          },
        ]);
      const turn: Turn = {
        id: uuid(),
        question,
        answer: '',
        at: Date.now(),
        runs: {},
      };
      set((state) => ({
        turns: [...state.turns, turn].slice(-KeptTurns),
        busy: true,
      }));
      const started = Date.now();
      const own = new AbortController();
      controller = own;
      let answer = '';
      let reference: IReference | undefined;
      let lastFlush = 0;
      const flush = () => {
        patchTurn(turn.id, () => ({
          answer,
          ...(reference ? { reference } : {}),
        }));
        lastFlush = Date.now();
      };
      try {
        const [datasets, defaults, models] = await Promise.all([
          listMyDatasets(),
          listDefaultModels(),
          listAddedModels(),
        ]);
        set({ datasets, models });
        const situation = buildSituation({
          page,
          datasets,
          guideDatasetId: setup.guideDatasetId,
          guideEmbedding: setup.guideEmbedding,
          connected: setup.datasetIds,
          defaults,
          models,
          pages: NamedPages,
          datasetPath: datasetPagePath,
        });
        const response = await requestBohemiaCompletion(
          {
            chatId: setup.chatId,
            messages: [...earlier, { role: 'user', content: question }],
            situation,
          },
          own.signal,
        );
        const errorBody = readJsonSafely(response.clone());
        for await (const chunk of parseCompletionEventStream(response)) {
          answer = mergeAnswerChunk(answer, chunk);
          reference = normalizeReference(chunk.reference) ?? reference;
          if (chunk.final || Date.now() - lastFlush > 60) flush();
        }
        flush();
        const body = await errorBody;
        if (
          body !== undefined &&
          (response.status !== 200 || body.code !== 0)
        ) {
          throw new BohemiaRequestError(
            body.message || `波西米亚暂时不可用（HTTP ${response.status}）`,
          );
        }
        if (answer.includes('**ERROR**')) {
          throw new BohemiaRequestError(
            answer.replace(/\*\*ERROR\*\*:?\s*/, '').trim(),
          );
        }
        if (!answer.trim()) throw new BohemiaRequestError('没有收到回答');
      } catch (error) {
        const aborted = own.signal.aborted;
        patchTurn(turn.id, () => ({
          answer: answer.includes('**ERROR**') ? '' : answer,
          error: aborted
            ? answer
              ? undefined
              : '已停止'
            : error instanceof Error
              ? error.message
              : '连接波西米亚失败',
        }));
      } finally {
        if (controller === own) controller = null;
        patchTurn(turn.id, () => ({ ms: Date.now() - started }));
        set({ busy: false });
      }
    },

    stop() {
      controller?.abort();
    },

    clear() {
      controller?.abort();
      set({ turns: [] });
    },

    setRun(turnId, index, run) {
      patchTurn(turnId, (turn) => ({ runs: { ...turn.runs, [index]: run } }));
    },

    setConnected(ids) {
      set((state) =>
        state.setup.state === 'ready'
          ? { setup: { ...state.setup, datasetIds: ids } }
          : {},
      );
    },

    setDatasets(datasets) {
      set({ datasets });
    },
  };
});

useBohemiaStore.subscribe((state, previous) => {
  if (state.turns !== previous.turns) {
    keep(
      BohemiaStorageKeys.turns,
      JSON.stringify(state.turns.filter((turn) => turn.answer || turn.error)),
    );
  }
});

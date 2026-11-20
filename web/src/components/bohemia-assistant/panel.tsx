import { BrandMark } from '@/components/brand-mark';
import { ChatApiAction } from '@/hooks/use-chat-request';
import { KnowledgeApiAction } from '@/hooks/use-knowledge-request';
import { LlmKeys } from '@/hooks/use-llm-request';
import { IDataset } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  BohemiaRequestError,
  listMyDatasets,
  patchChat,
} from '@/services/bohemia-assistant-service';
import { useQueryClient } from '@tanstack/react-query';
import {
  Check,
  CircleAlert,
  Copy,
  Library,
  Maximize2,
  Minimize2,
  Plus,
  RefreshCw,
  Send,
  Settings2,
  Square,
  X,
} from 'lucide-react';
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  type UIEvent,
} from 'react';
import { useLocation, useNavigate } from 'react-router';
import { ActionCard } from './action-cards';
import { AssistantAction, splitAnswer } from './actions';
import { Answer, SourcePreview, closePreview } from './answer';
import { BohemiaName, BohemiaSubtitle, ModelTypeNames } from './constants';
import styles from './index.module.less';
import { describeAction, offeredActions, runAction } from './operations';
import {
  NamedPages,
  NavigablePrefixes,
  datasetPagePath,
  describePage,
} from './pages';
import { EligibilityWords, datasetEligibility } from './situation';
import { BohemiaReadySetup, Turn, useBohemiaStore } from './store';
import { Edges, usePanelFrame } from './use-panel-frame';

type Starter = {
  kind: string;
  text: string;
  tone: 'sky' | 'mint' | 'yellow' | 'rose';
};

/** Questions the guide answers, plus two that ask it to do something. */
const Starters: Starter[] = [
  { kind: '知识库', text: '怎么添加一个知识库？', tone: 'sky' },
  { kind: '代码库', text: '怎么把一个代码仓库加入知识库？', tone: 'mint' },
  {
    kind: 'MCP',
    text: '怎么在 Claude Code 里调用知识库的 MCP 工具？',
    tone: 'yellow',
  },
  { kind: '团队', text: '怎么创建团队、邀请同事？', tone: 'rose' },
  { kind: '聊天', text: '聊天功能怎么用？', tone: 'sky' },
  { kind: '记忆', text: '记忆功能是做什么的？', tone: 'mint' },
  { kind: '模型', text: '怎么配置模型？', tone: 'yellow' },
  { kind: '动手', text: '帮我把自己的知识库也接到你这里', tone: 'rose' },
];
const StarterCount = 4;

const clock = (at: number) =>
  at
    ? new Date(at).toLocaleTimeString('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    : '';
const seconds = (ms: number) =>
  ms < 10_000 ? `${(ms / 1000).toFixed(1)} 秒` : `${Math.round(ms / 1000)} 秒`;

/** After an action changed something, the pages showing it read it again. */
function useRefreshAfterAction() {
  const queryClient = useQueryClient();
  return useCallback(
    (action: AssistantAction) => {
      if (action.type === 'create_dataset') {
        queryClient.invalidateQueries({
          queryKey: [KnowledgeApiAction.FetchKnowledgeList],
        });
        queryClient.invalidateQueries({
          queryKey: [KnowledgeApiAction.FetchKnowledgeListByPage],
        });
      }
      if (action.type === 'connect_datasets') {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchChatList],
        });
      }
      if (action.type === 'set_default_model') {
        queryClient.invalidateQueries({ queryKey: LlmKeys.defaultModels() });
      }
    },
    [queryClient],
  );
}

function SetupView({
  onSettings,
  onRetry,
}: {
  onSettings: () => void;
  onRetry: () => void;
}) {
  const setup = useBohemiaStore((state) => state.setup);
  if (setup.state === 'needs-models') {
    return (
      <div className={styles.hello}>
        <span className={styles.helloMark}>
          <BrandMark size={48} />
        </span>
        <h2>先给{BohemiaName}接上模型</h2>
        <p>
          我回答问题要用默认的 LLM，查使用说明要用默认的 Embedding
          模型。到“用户设置 →
          模型提供商”添加模型并在“设置默认模型”里选好，回来就能用。
        </p>
        <ol className={styles.setupList}>
          {(['chat', 'embedding'] as const).map((type) => (
            <li
              key={type}
              data-done={!setup.missing.includes(type) || undefined}
            >
              <b>默认 {ModelTypeNames[type]}</b>
              <span>
                {setup.missing.includes(type) ? '还没有设置' : '已设置'}
              </span>
            </li>
          ))}
        </ol>
        <button type="button" className={styles.resume} onClick={onSettings}>
          <Settings2 size={16} />
          <span className={styles.resumeText}>
            <b>去设置模型</b>
          </span>
          <small>用户设置 → 模型提供商</small>
        </button>
        <button type="button" className={styles.reroll} onClick={onRetry}>
          <RefreshCw size={13} />
          设置好了，重新检查
        </button>
      </div>
    );
  }
  if (setup.state === 'failed') {
    return (
      <div className={styles.hello}>
        <span className={styles.helloMark}>
          <BrandMark size={48} />
        </span>
        <h2>{BohemiaName}还没准备好</h2>
        <div className={styles.error} role="alert">
          <CircleAlert size={16} />
          <span>{setup.message}</span>
          <button type="button" onClick={onRetry}>
            重试
          </button>
        </div>
      </div>
    );
  }
  const note = setup.state === 'preparing' ? setup.note : '正在准备…';
  return (
    <div className={styles.hello}>
      <span className={styles.helloMark}>
        <BrandMark size={56} thinking />
      </span>
      <h2>嗨，我是{BohemiaName}</h2>
      <p className={styles.shimmer}>{note}</p>
      <p className={styles.note}>
        第一次打开时，我会在你的账号下建一个“平台使用说明”知识库并解析，大约需要半分钟。
      </p>
    </div>
  );
}

function Hello({ onAsk }: { onAsk: (text: string) => void }) {
  const [round, setRound] = useState(0);
  const starters = useMemo(() => {
    const start = (round * StarterCount) % Starters.length;
    return [...Starters, ...Starters].slice(start, start + StarterCount);
  }, [round]);
  const handleReroll = useCallback(() => setRound((value) => value + 1), []);
  return (
    <div className={styles.hello}>
      <span className={styles.helloMark}>
        <BrandMark size={56} />
      </span>
      <h2>嗨，我是{BohemiaName}</h2>
      <p>
        问我这个平台怎么用：知识库、代码库、MCP、团队、聊天、记忆和模型。我也能带你去对应页面、帮你连接知识库、新建知识库或换默认模型，改设置前会先问你。
      </p>
      <div className={styles.starters} aria-label="可以这样问">
        {starters.map((item) => (
          <StarterButton key={item.text} starter={item} onAsk={onAsk} />
        ))}
      </div>
      <button type="button" className={styles.reroll} onClick={handleReroll}>
        <RefreshCw size={13} />
        换一批
      </button>
    </div>
  );
}

function StarterButton({
  starter,
  onAsk,
}: {
  starter: Starter;
  onAsk: (text: string) => void;
}) {
  const handleClick = useCallback(
    () => onAsk(starter.text),
    [onAsk, starter.text],
  );
  return (
    <button
      type="button"
      className={styles[`starter-${starter.tone}`]}
      onClick={handleClick}
    >
      <span className={styles.starterKind}>{starter.kind}</span>
      <span className={styles.starterText}>{starter.text}</span>
    </button>
  );
}

/** The datasets 波西米亚 answers from, switched on and off by the person. */
function DatasetPicker({
  setup,
  onClose,
}: {
  setup: BohemiaReadySetup;
  onClose: () => void;
}) {
  const datasets = useBohemiaStore((state) => state.datasets);
  const setDatasets = useBohemiaStore((state) => state.setDatasets);
  const setConnected = useBohemiaStore((state) => state.setConnected);
  const refresh = useRefreshAfterAction();
  const [problem, setProblem] = useState('');
  const [saving, setSaving] = useState('');
  useEffect(() => {
    let alive = true;
    listMyDatasets()
      .then((list) => alive && setDatasets(list))
      .catch(
        (error) =>
          alive &&
          setProblem(error instanceof Error ? error.message : '读取失败'),
      );
    return () => {
      alive = false;
    };
  }, [setDatasets]);
  const own = datasets.filter((dataset) => dataset.id !== setup.guideDatasetId);
  const handleToggle = useCallback(
    async (dataset: IDataset) => {
      const on = setup.datasetIds.includes(dataset.id);
      const next = on
        ? setup.datasetIds.filter((id) => id !== dataset.id)
        : [...setup.datasetIds, dataset.id];
      if (!next.includes(setup.guideDatasetId))
        next.unshift(setup.guideDatasetId);
      setSaving(dataset.id);
      setProblem('');
      try {
        const chat = await patchChat(setup.chatId, { dataset_ids: next });
        setConnected(chat.dataset_ids ?? next);
        refresh({ type: 'connect_datasets', add: [], remove: [] });
      } catch (error) {
        setProblem(
          error instanceof BohemiaRequestError
            ? error.message
            : '没有保存，请再试一次',
        );
      } finally {
        setSaving('');
      }
    },
    [refresh, setConnected, setup],
  );
  return (
    <section className={styles.picker} aria-label="连接的知识库">
      <div className={styles.label}>
        <span>我会检索的知识库</span>
        <button type="button" className={styles.labelButton} onClick={onClose}>
          完成
        </button>
      </div>
      <div className={cn(styles.pickRow, styles.pickFixed)}>
        <span className={styles.pickCheck} data-on>
          <Check size={12} />
        </span>
        <span className={styles.pickName}>平台使用说明</span>
        <small>自带，一直连接</small>
      </div>
      {own.map((dataset) => (
        <PickRow
          key={dataset.id}
          dataset={dataset}
          setup={setup}
          saving={saving === dataset.id}
          onToggle={handleToggle}
        />
      ))}
      {!own.length && <p className={styles.note}>你还没有自己的知识库。</p>}
      {problem && (
        <p className={styles.askProblem}>
          <CircleAlert size={13} />
          {problem}
        </p>
      )}
    </section>
  );
}

function PickRow({
  dataset,
  setup,
  saving,
  onToggle,
}: {
  dataset: IDataset;
  setup: BohemiaReadySetup;
  saving: boolean;
  onToggle: (dataset: IDataset) => void;
}) {
  const state = datasetEligibility(
    dataset,
    setup.datasetIds,
    setup.guideEmbedding,
  );
  const disabled = saving || state === 'empty' || state === 'embedding';
  const handleClick = useCallback(() => onToggle(dataset), [dataset, onToggle]);
  return (
    <button
      type="button"
      className={styles.pickRow}
      disabled={disabled}
      onClick={handleClick}
      aria-pressed={state === 'connected'}
    >
      <span
        className={styles.pickCheck}
        data-on={state === 'connected' || undefined}
      >
        {state === 'connected' && <Check size={12} />}
      </span>
      <span className={styles.pickName}>{dataset.name}</span>
      <small>{saving ? '正在保存…' : EligibilityWords[state]}</small>
    </button>
  );
}

type TurnViewProps = {
  turn: Turn;
  live: boolean;
  last: boolean;
  setup: BohemiaReadySetup;
  datasets: IDataset[];
  copied: boolean;
  pause: boolean;
  goTo: (path: string) => void;
  onRetry: (turn: Turn) => void;
  onCopy: (turn: Turn) => void;
};

const pageName = (path: string) =>
  NamedPages.find((page) => page.path === path)?.name;

function TurnView({
  turn,
  live,
  last,
  setup,
  datasets,
  copied,
  pause,
  goTo,
  onRetry,
  onCopy,
}: TurnViewProps) {
  const setRun = useBohemiaStore((state) => state.setRun);
  const setConnected = useBohemiaStore((state) => state.setConnected);
  const busy = useBohemiaStore((state) => state.busy);
  const refresh = useRefreshAfterAction();
  const { text, actions } = useMemo(() => {
    const split = splitAnswer(turn.answer, NavigablePrefixes, live);
    return {
      text: split.text,
      actions: offeredActions(split.actions, setup.guideDatasetId),
    };
  }, [turn.answer, live, setup.guideDatasetId]);
  const datasetName = useCallback(
    (id: string) =>
      id === setup.guideDatasetId
        ? '平台使用说明'
        : datasets.find((dataset) => dataset.id === id)?.name,
    [datasets, setup.guideDatasetId],
  );
  const handleRetry = useCallback(() => onRetry(turn), [onRetry, turn]);
  const handleCopy = useCallback(() => onCopy(turn), [onCopy, turn]);
  const confirm = useCallback(
    async (index: number, action: AssistantAction) => {
      if (action.type === 'navigate') return;
      setRun(turn.id, index, { state: 'running' });
      try {
        const outcome = await runAction(
          action,
          {
            chatId: setup.chatId,
            guideDatasetId: setup.guideDatasetId,
            guideEmbedding: setup.guideEmbedding,
            connected: setup.datasetIds,
          },
          datasetPagePath,
        );
        if (outcome.connected) setConnected(outcome.connected);
        refresh(action);
        setRun(turn.id, index, {
          state: 'done',
          message: outcome.message,
          href: outcome.href,
        });
      } catch (error) {
        setRun(turn.id, index, {
          state: 'failed',
          message:
            error instanceof Error ? error.message : '没有成功，请再试一次',
        });
      }
    },
    [refresh, setConnected, setRun, setup, turn.id],
  );
  return (
    <section className={styles.turn} aria-busy={live}>
      {pause && clock(turn.at) && (
        <div className={styles.when}>
          <span>{clock(turn.at)}</span>
        </div>
      )}
      <div className={styles.question}>
        <span>{turn.question}</span>
      </div>
      <div className={styles.reply}>
        {live && !text && (
          <div className={styles.trace}>
            <BrandMark size={14} thinking />
            <span className={styles.shimmer}>在查使用说明、想怎么回答…</span>
          </div>
        )}
        <Answer
          text={text}
          reference={turn.reference}
          guideDatasetId={setup.guideDatasetId}
          datasetName={datasetName}
          navigable={NavigablePrefixes}
          goTo={goTo}
          writing={live}
        />
        {!live && actions.length > 0 && (
          <div className={styles.actions}>
            {actions.map((action, index) => (
              <ActionSlot
                key={index}
                index={index}
                action={action}
                turn={turn}
                datasets={datasets}
                goTo={goTo}
                onConfirm={confirm}
              />
            ))}
          </div>
        )}
        {turn.error && (
          <div className={styles.error} role="alert">
            <CircleAlert size={16} />
            <span>{turn.error}</span>
            {last && !busy && turn.error !== '已停止' && (
              <button type="button" onClick={handleRetry}>
                重试
              </button>
            )}
          </div>
        )}
        {!live && text && !turn.error && (
          <div className={styles.acts}>
            <button
              type="button"
              className={styles.iconButton}
              title={copied ? '已复制' : '复制'}
              aria-label="复制回答"
              onClick={handleCopy}
            >
              {copied ? <Check size={15} /> : <Copy size={15} />}
            </button>
            {last && !busy && (
              <button
                type="button"
                className={styles.iconButton}
                title="重新回答"
                aria-label="重新回答"
                onClick={handleRetry}
              >
                <RefreshCw size={15} />
              </button>
            )}
            {turn.ms ? (
              <span className={styles.actsTime}>{seconds(turn.ms)}</span>
            ) : null}
          </div>
        )}
      </div>
    </section>
  );
}

function ActionSlot({
  index,
  action,
  turn,
  datasets,
  goTo,
  onConfirm,
}: {
  index: number;
  action: AssistantAction;
  turn: Turn;
  datasets: IDataset[];
  goTo: (path: string) => void;
  onConfirm: (index: number, action: AssistantAction) => void;
}) {
  const setRun = useBohemiaStore((state) => state.setRun);
  const models = useBohemiaStore((state) => state.models);
  const copy = useMemo(
    () => describeAction(action, datasets, pageName, models),
    [action, datasets, models],
  );
  const handleConfirm = useCallback(
    () => onConfirm(index, action),
    [action, index, onConfirm],
  );
  const handleDecline = useCallback(
    () => setRun(turn.id, index, { state: 'declined' }),
    [index, setRun, turn.id],
  );
  return (
    <ActionCard
      action={action}
      copy={copy}
      run={turn.runs[index]}
      onNavigate={goTo}
      onConfirm={handleConfirm}
      onDecline={handleDecline}
    />
  );
}

export default function BohemiaPanel() {
  const open = useBohemiaStore((state) => state.open);
  const setup = useBohemiaStore((state) => state.setup);
  const turns = useBohemiaStore((state) => state.turns);
  const busy = useBohemiaStore((state) => state.busy);
  const datasets = useBohemiaStore((state) => state.datasets);
  const { setOpen, prepare, send, stop, clear } = useBohemiaStore.getState();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const frame = usePanelFrame();
  const [input, setInput] = useState('');
  const [scrolled, setScrolled] = useState(false);
  const [picking, setPicking] = useState(false);
  const [copied, setCopied] = useState('');
  const list = useRef<HTMLDivElement>(null);
  const flow = useRef<HTMLDivElement>(null);
  const field = useRef<HTMLTextAreaElement>(null);
  const stick = useRef(true);

  useEffect(() => {
    if (open) void prepare();
  }, [open, prepare]);

  const close = useCallback(() => {
    closePreview();
    setOpen(false);
  }, [setOpen]);
  const goTo = useCallback(
    (path: string) => {
      closePreview();
      navigate(path);
    },
    [navigate],
  );
  const openDataset = useCallback(
    (id: string) => goTo(datasetPagePath(id)),
    [goTo],
  );
  const ask = useCallback(
    (text: string) => {
      stick.current = true;
      void send(text, describePage(pathname, document.title));
    },
    [pathname, send],
  );
  const handleRetry = useCallback(
    (turn: Turn) => {
      useBohemiaStore.setState((state) => ({
        turns: state.turns.filter((item) => item.id !== turn.id),
      }));
      ask(turn.question);
    },
    [ask],
  );
  const handleCopy = useCallback((turn: Turn) => {
    const { text } = splitAnswer(turn.answer, NavigablePrefixes);
    void navigator.clipboard
      ?.writeText(
        text
          .replace(/\[ID:\s*\d+\]/g, '')
          .replace(/<think>[\s\S]*?<\/think>/g, ''),
      )
      .then(() => {
        setCopied(turn.id);
        setTimeout(() => setCopied(''), 1400);
      });
  }, []);

  // Follows the answer down as it grows, unless the reader scrolled up to read.
  useLayoutEffect(() => {
    const element = list.current;
    const inner = flow.current;
    if (!element || !inner) return;
    const follow = () => {
      if (stick.current) element.scrollTop = element.scrollHeight;
    };
    follow();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(follow);
    observer.observe(inner);
    return () => observer.disconnect();
  }, [open]);
  // The picker sits above the conversation; opening it scrolls up to it.
  useLayoutEffect(() => {
    const element = list.current;
    if (!picking || !element) return;
    stick.current = false;
    element.scrollTop = 0;
  }, [picking]);
  useLayoutEffect(() => {
    const element = field.current;
    if (!element) return;
    element.style.height = 'auto';
    element.style.height = `${Math.min(132, element.scrollHeight)}px`;
  }, [input]);
  useEffect(() => {
    if (!open) return;
    const handleKey = (event: KeyboardEvent) => {
      if (
        event.key === 'Escape' &&
        !busy &&
        !document.querySelector('[role="dialog"][data-state="open"]')
      ) {
        close();
      }
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [busy, close, open]);

  const handleScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const element = event.currentTarget;
    stick.current =
      element.scrollHeight - element.scrollTop - element.clientHeight < 48;
    setScrolled(element.scrollTop > 4);
  }, []);
  const submit = useCallback(() => {
    const text = input.trim();
    if (!text || busy || setup.state !== 'ready') return;
    setInput('');
    ask(text);
  }, [ask, busy, input, setup.state]);
  const handleSubmit = useCallback(
    (event: FormEvent) => {
      event.preventDefault();
      submit();
    },
    [submit],
  );
  const handleInput = useCallback(
    (event: ChangeEvent<HTMLTextAreaElement>) => setInput(event.target.value),
    [],
  );
  const handleKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
      if (
        event.key === 'Enter' &&
        !event.shiftKey &&
        !event.nativeEvent.isComposing
      ) {
        event.preventDefault();
        submit();
      }
    },
    [submit],
  );
  const handleHeadDown = useCallback(
    (event: ReactPointerEvent) => {
      if (!(event.target as HTMLElement).closest('button, a'))
        frame.begin(event, 'move');
    },
    [frame],
  );
  const handleHeadDouble = useCallback(
    (event: ReactMouseEvent) => {
      if (!(event.target as HTMLElement).closest('button, a'))
        frame.toggleExpanded();
    },
    [frame],
  );
  const handleSettings = useCallback(
    () => goTo(`${Routes.UserSetting}${Routes.Model}`),
    [goTo],
  );
  const handleRetrySetup = useCallback(() => void prepare(true), [prepare]);
  const togglePicking = useCallback(() => setPicking((value) => !value), []);
  const closePicking = useCallback(() => setPicking(false), []);
  const handleNew = useCallback(() => {
    clear();
    setPicking(false);
  }, [clear]);

  if (!open) return null;
  const ready = setup.state === 'ready' ? setup : null;
  const extra = ready
    ? ready.datasetIds.filter((id) => id !== ready.guideDatasetId).length
    : 0;
  const subtitle = ready
    ? `${BohemiaSubtitle} · 检索使用说明${extra ? ` + ${extra} 个知识库` : ''}`
    : setup.state === 'needs-models'
      ? `${BohemiaSubtitle} · 还没有接上模型`
      : `${BohemiaSubtitle} · 正在准备`;
  const placed = frame.frame;
  return (
    <aside
      className={cn(
        styles.panel,
        placed && styles.placed,
        frame.moving && styles.moving,
      )}
      aria-label={BohemiaName}
      style={
        placed
          ? {
              left: placed.x,
              top: placed.y,
              width: placed.w,
              height: placed.h,
              right: 'auto',
              bottom: 'auto',
            }
          : undefined
      }
    >
      <div className={styles.box} data-scrolled={scrolled || undefined}>
        <div
          className={cn(
            styles.head,
            busy && styles.busy,
            frame.desk && styles.handle,
          )}
          onPointerDown={handleHeadDown}
          onDoubleClick={handleHeadDouble}
        >
          <BrandMark size={30} thinking={busy || setup.state === 'preparing'} />
          <div className={styles.headText}>
            <b>{BohemiaName}</b>
            <span className={styles.headSub}>{subtitle}</span>
          </div>
          <div className={styles.headActions}>
            {ready && (
              <button
                type="button"
                className={cn(
                  styles.iconButton,
                  picking && styles.iconButtonOn,
                )}
                title="选择我会检索的知识库"
                aria-label="选择我会检索的知识库"
                aria-pressed={picking}
                onClick={togglePicking}
              >
                <Library size={17} />
              </button>
            )}
            {turns.length > 0 && (
              <button
                type="button"
                className={styles.iconButton}
                title="新对话"
                aria-label="新对话"
                onClick={handleNew}
              >
                <Plus size={17} />
              </button>
            )}
            {frame.desk && (
              <button
                type="button"
                className={styles.iconButton}
                title={frame.expanded ? '还原大小' : '放大'}
                aria-label={frame.expanded ? '还原大小' : '放大'}
                onClick={frame.toggleExpanded}
              >
                {frame.expanded ? (
                  <Minimize2 size={16} />
                ) : (
                  <Maximize2 size={16} />
                )}
              </button>
            )}
            <button
              type="button"
              className={styles.iconButton}
              title="关闭（Esc）"
              aria-label={`关闭${BohemiaName}`}
              onClick={close}
            >
              <X size={17} />
            </button>
          </div>
          <i className={styles.headLine} aria-hidden="true" />
        </div>
        <div className={styles.scroll} ref={list} onScroll={handleScroll}>
          <div className={styles.flow} ref={flow}>
            {ready && picking && (
              <DatasetPicker setup={ready} onClose={closePicking} />
            )}
            {!ready ? (
              <SetupView
                onSettings={handleSettings}
                onRetry={handleRetrySetup}
              />
            ) : !turns.length ? (
              <Hello onAsk={ask} />
            ) : (
              turns.map((turn, index) => {
                const last = index === turns.length - 1;
                const before = turns[index - 1];
                return (
                  <TurnView
                    key={turn.id}
                    turn={turn}
                    live={busy && last}
                    last={last}
                    setup={ready}
                    datasets={datasets}
                    copied={copied === turn.id}
                    pause={!before || turn.at - before.at > 30 * 60 * 1000}
                    goTo={goTo}
                    onRetry={handleRetry}
                    onCopy={handleCopy}
                  />
                );
              })
            )}
          </div>
        </div>
        <div className={styles.bottom}>
          <form className={styles.compose} onSubmit={handleSubmit}>
            <textarea
              ref={field}
              rows={1}
              value={input}
              maxLength={4000}
              disabled={!ready}
              placeholder={
                ready
                  ? turns.length
                    ? '接着问…'
                    : '问问平台怎么用，或者让我帮你设置…'
                  : setup.state === 'needs-models'
                    ? '先接上模型'
                    : '正在准备…'
              }
              aria-label={`问${BohemiaName}`}
              onChange={handleInput}
              onKeyDown={handleKeyDown}
            />
            {busy && !input.trim() ? (
              <button
                type="button"
                className={cn(styles.send, styles.sendStop)}
                aria-label="停止"
                title="停止"
                onClick={stop}
              >
                <Square size={13} />
              </button>
            ) : (
              <button
                type="submit"
                className={styles.send}
                aria-label="发送"
                title="发送（Enter，Shift+Enter 换行）"
                disabled={!ready || busy || !input.trim()}
              >
                <Send size={16} />
              </button>
            )}
          </form>
        </div>
      </div>
      {frame.desk &&
        Edges.map((edge) => (
          <EdgeHandle key={edge} edge={edge} frame={frame} />
        ))}
      <SourcePreview openDataset={openDataset} />
    </aside>
  );
}

function EdgeHandle({
  edge,
  frame,
}: {
  edge: (typeof Edges)[number];
  frame: ReturnType<typeof usePanelFrame>;
}) {
  const handleDown = useCallback(
    (event: ReactPointerEvent) => frame.begin(event, edge),
    [edge, frame],
  );
  return (
    <span
      className={cn(styles.edge, styles[`edge-${edge}`])}
      aria-hidden="true"
      onPointerDown={handleDown}
      onDoubleClick={frame.reset}
    />
  );
}

import { BrandMark } from '@/components/brand-mark';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import {
  AlertTriangle,
  BadgeCheck,
  Check,
  ChevronDown,
  CircleCheck,
  CornerDownRight,
  Dot,
  GitBranch,
  Lightbulb,
  PenLine,
  Scale,
  Search,
  Telescope,
  TextSearch,
} from 'lucide-react';
import {
  Fragment,
  ReactNode,
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import MarkdownContent from '../markdown-content';
import {
  AnswerMetrics,
  AssistantContentParts,
  CitationSummary,
  ResearchStep,
  TraceStage,
  TraceStageKey,
  buildTraceStages,
  formatDuration,
  parseResearchSteps,
} from './reasoning-chain';
import styles from './reasoning-trace.module.less';

type ClickDocumentButton = (documentId: string, chunk: IReferenceChunk) => void;

interface ReasoningTraceProps {
  live: boolean;
  parts: AssistantContentParts;
  metrics?: AnswerMetrics;
  citations: CitationSummary;
  reference: IReference;
  /** The chat searches a knowledge base, so retrieval and citing are planned. */
  hasKnowledge?: boolean;
  clickDocumentButton?: ClickDocumentButton;
}

/**
 * Measures how long the model reasoned, but only when this view saw the
 * reasoning begin: a view mounted mid-stream would under-count.
 */
function useReasoningDuration(live: boolean, parts: AssistantContentParts) {
  const sawStart = useRef(!parts.thinking && !parts.thinkingOpen);
  const startedAt = useRef<number | null>(null);
  const [duration, setDuration] = useState<number>();

  useEffect(() => {
    if (!sawStart.current || duration !== undefined) return;
    if (startedAt.current === null) {
      if (live && (parts.thinkingOpen || parts.thinking)) {
        startedAt.current = Date.now();
      }
      return;
    }
    // Reasoning can close in the same update that ends the stream.
    if (!parts.thinkingOpen || !live) {
      setDuration(Date.now() - startedAt.current);
    }
  }, [duration, live, parts.thinking, parts.thinkingOpen]);

  return duration;
}

/** Seconds since this answer started streaming, when this view saw it start. */
function useLiveElapsed(live: boolean, sawStart: boolean) {
  const [startedAt] = useState(() => Date.now());
  const [now, setNow] = useState(startedAt);

  useEffect(() => {
    if (!live || !sawStart) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [live, sawStart]);

  return live && sawStart ? now - startedAt : undefined;
}

const StageIcon: Record<TraceStageKey, ReactNode> = {
  refine: <TextSearch />,
  retrieve: <Search />,
  reason: <Lightbulb />,
  compose: <PenLine />,
  verify: <BadgeCheck />,
};

function ResearchSteps({
  steps,
  live,
}: {
  steps: ResearchStep[];
  live: boolean;
}) {
  const { t } = useTranslation();

  return (
    <ol className={styles.research}>
      {steps.map((step, index) => {
        const latest = live && index === steps.length - 1;
        let icon: ReactNode = <Dot />;
        let body: ReactNode = null;
        let metric: string | null = null;

        if (step.kind === 'search') {
          icon = <Search />;
          body = <q>{step.query}</q>;
        } else if (step.kind === 'retrieved') {
          icon = <CornerDownRight />;
          body = t('chat.reasoningTrace.research.retrieved', {
            count: step.count,
          });
          metric = formatDuration(step.ms);
        } else if (step.kind === 'check') {
          icon = <Scale />;
          body = t('chat.reasoningTrace.research.check');
        } else if (step.kind === 'sufficient') {
          icon = <CircleCheck />;
          body = (
            <>
              {t('chat.reasoningTrace.research.sufficient')}{' '}
              <q>{step.question}</q>
            </>
          );
        } else if (step.kind === 'expand') {
          icon = <GitBranch />;
          body = (
            <>
              {t('chat.reasoningTrace.research.expand')}
              <ul>
                {step.questions.map((question) => (
                  <li key={question}>{question}</li>
                ))}
              </ul>
            </>
          );
        } else {
          body = step.text;
        }

        return (
          <li key={index} className={styles.researchStep}>
            {icon}
            <span className={cn('min-w-0', latest && styles.shimmer)}>
              {body}
            </span>
            {metric && <span className={styles.researchMetric}>{metric}</span>}
          </li>
        );
      })}
    </ol>
  );
}

function ReasoningText({
  text,
  streaming,
  reference,
  clickDocumentButton,
}: {
  text: string;
  streaming: boolean;
  reference: IReference;
  clickDocumentButton?: ClickDocumentButton;
}) {
  const boxRef = useRef<HTMLDivElement>(null);

  // Follow the newest reasoning while it streams.
  useLayoutEffect(() => {
    if (streaming && boxRef.current) {
      boxRef.current.scrollTop = boxRef.current.scrollHeight;
    }
  }, [streaming, text]);

  return (
    <div ref={boxRef} className={styles.reasoningBox}>
      <MarkdownContent
        content={text}
        loading={streaming}
        reference={reference}
        clickDocumentButton={clickDocumentButton}
      />
    </div>
  );
}

function TimeBreakdown({ metrics }: { metrics: AnswerMetrics }) {
  const { t } = useTranslation();
  const known =
    metrics.prepareMs +
    metrics.refineMs +
    metrics.retrievalMs +
    metrics.generateMs;
  const segments = [
    { key: 'prepare', ms: metrics.prepareMs },
    { key: 'refine', ms: metrics.refineMs },
    { key: 'retrieval', ms: metrics.retrievalMs },
    { key: 'generate', ms: metrics.generateMs },
    // Citation matching, speech and bookkeeping after the answer.
    { key: 'other', ms: metrics.totalMs - known },
  ].filter((segment) => segment.ms >= 1);

  if (!segments.length) return null;

  return (
    <div className={styles.breakdown}>
      <div className={styles.bar}>
        {segments.map((segment) => (
          <span
            key={segment.key}
            className={cn(styles.segment, styles[segment.key])}
            style={{ flexGrow: segment.ms }}
          />
        ))}
      </div>
      <div className={styles.legend}>
        {segments.map((segment) => (
          <span key={segment.key}>
            <i className={cn(styles.swatch, styles[segment.key])} />
            {t(`chat.reasoningTrace.phase.${segment.key}`)}
            <b>{formatDuration(segment.ms)}</b>
          </span>
        ))}
      </div>
    </div>
  );
}

/**
 * The chain of work behind an answer: retrieve, reason, compose, verify.
 * While the answer streams it is one card with every planned step, the step
 * under way animated; afterwards it folds into a single summary line that
 * opens to the same steps with what each one found.
 */
function ReasoningTraceInner({
  live,
  parts,
  metrics,
  citations,
  reference,
  hasKnowledge = false,
  clickDocumentButton,
}: ReasoningTraceProps) {
  const { t } = useTranslation();
  const sawStart = useRef(
    !parts.answer && !parts.thinking && !parts.research,
  ).current;
  const reasoningMs = useReasoningDuration(live, parts);
  const elapsedMs = useLiveElapsed(live, sawStart);
  const [open, setOpen] = useState(false);

  const stages = useMemo(
    () => buildTraceStages({ live, parts, metrics, citations, hasKnowledge }),
    [citations, hasKnowledge, live, metrics, parts],
  );
  const researchOpen = live && parts.researchOpen;
  const researchSteps = useMemo(
    () => parseResearchSteps(parts.research, researchOpen),
    [parts.research, researchOpen],
  );
  const researchHits = researchSteps.reduce(
    (sum, step) => sum + (step.kind === 'retrieved' ? step.count : 0),
    0,
  );

  const handleToggle = useCallback(() => setOpen((value) => !value), []);

  const reasoningLabel =
    reasoningMs !== undefined
      ? formatDuration(reasoningMs)
      : parts.thinking
        ? t('chat.reasoningTrace.reasonChars', {
            count: parts.thinking.length,
          })
        : '';

  const describe = (
    stage: TraceStage,
  ): { text: string; metric?: string; detail?: ReactNode } => {
    const key = `chat.reasoningTrace.describe.${stage.key}`;
    switch (stage.key) {
      case 'refine':
        return {
          text: t(`${key}.done`),
          metric: formatDuration(metrics?.refineMs ?? 0),
        };
      case 'retrieve': {
        const detail = researchSteps.length > 0 && (
          <ResearchSteps steps={researchSteps} live={researchOpen} />
        );
        const metric =
          (metrics?.retrievalMs ?? 0) > 0
            ? formatDuration(metrics!.retrievalMs)
            : undefined;
        if (stage.state === 'active') {
          return {
            text: t(parts.research ? `${key}.deepActive` : `${key}.active`),
            detail,
          };
        }
        if (stage.state === 'warn') {
          return { text: t(`${key}.empty`), metric, detail };
        }
        if (citations.retrievedChunkCount > 0) {
          return {
            text: t(`${key}.found`, {
              chunks: citations.retrievedChunkCount,
              documents: citations.retrievedDocumentCount,
            }),
            metric,
            detail,
          };
        }
        return {
          text: researchSteps.length
            ? t(`${key}.deepDone`, { count: researchHits })
            : t(`${key}.done`),
          metric,
          detail,
        };
      }
      case 'reason':
        return {
          text: t(stage.state === 'active' ? `${key}.active` : `${key}.done`),
          metric: reasoningLabel || undefined,
          detail: parts.thinking ? (
            <ReasoningText
              text={parts.thinking}
              streaming={live && parts.thinkingOpen}
              reference={reference}
              clickDocumentButton={clickDocumentButton}
            />
          ) : undefined,
        };
      case 'compose':
        if (stage.state === 'done' && metrics?.tokens !== undefined) {
          return {
            text: t(`${key}.tokens`, {
              tokens: metrics.tokens,
              speed: metrics.tokensPerSecond ?? 0,
            }),
          };
        }
        return { text: t(`${key}.${stage.state}`) };
      case 'verify':
        if (stage.state === 'done') {
          return {
            text: t(`${key}.done`, {
              cited: citations.citedChunkCount,
              documents: citations.sources.length,
            }),
          };
        }
        return { text: t(`${key}.${stage.state}`) };
    }
  };

  const steps = (
    <ol className={styles.stages}>
      {stages.map((stage, index) => {
        const { text, metric, detail } = describe(stage);
        return (
          <li
            key={stage.key}
            className={cn(styles.stage, styles[stage.state])}
            style={{ animationDelay: `${index * 50}ms` }}
          >
            <span className={styles.node}>
              {stage.state === 'warn' ? (
                <AlertTriangle />
              ) : stage.state === 'done' ? (
                <Check />
              ) : stage.key === 'retrieve' && parts.research ? (
                <Telescope />
              ) : (
                StageIcon[stage.key]
              )}
            </span>
            <div className="min-w-0">
              <div className={styles.stageHead}>
                <span className={styles.stageTitle}>
                  {t(`chat.reasoningTrace.stage.${stage.key}`)}
                </span>
                <span
                  className={cn(
                    styles.stageText,
                    stage.state === 'active' && styles.shimmer,
                  )}
                >
                  {text}
                </span>
                {metric && <span className={styles.stageMetric}>{metric}</span>}
              </div>
              {detail}
            </div>
          </li>
        );
      })}
    </ol>
  );

  if (live) {
    return (
      <section className={cn(styles.trace, styles.live)}>
        <div className={styles.card}>
          <div className={styles.cardHead}>
            <BrandMark size={18} thinking />
            <span className={styles.cardTitle}>
              {t('chat.reasoningTrace.title')}
            </span>
            {elapsedMs !== undefined && (
              <span className={styles.elapsed}>
                {Math.floor(elapsedMs / 1000)}s
              </span>
            )}
          </div>
          {steps}
        </div>
      </section>
    );
  }

  const summary = stages
    .map((stage) => {
      if (stage.key === 'retrieve') {
        return stage.state === 'warn'
          ? t('chat.reasoningTrace.summaryNoHit')
          : citations.retrievedChunkCount > 0
            ? t('chat.reasoningTrace.summaryRetrieved', {
                count: citations.retrievedChunkCount,
              })
            : '';
      }
      if (stage.key === 'reason' && reasoningLabel) {
        return t('chat.reasoningTrace.summaryReason', {
          value: reasoningLabel,
        });
      }
      if (stage.key === 'verify' && stage.state === 'done') {
        return t('chat.reasoningTrace.summaryCited', {
          count: citations.citedChunkCount,
        });
      }
      return '';
    })
    .concat(metrics ? formatDuration(metrics.totalMs) : '')
    .filter(Boolean);

  return (
    <section className={styles.trace}>
      <button
        type="button"
        className={styles.pill}
        onClick={handleToggle}
        aria-expanded={open}
      >
        <BrandMark size={16} />
        <span className={styles.pillTitle}>
          {t('chat.reasoningTrace.title')}
        </span>
        {summary.map((piece, index) => (
          <Fragment key={index}>
            <i className={styles.pillDot} aria-hidden="true">
              ·
            </i>
            <span className={styles.pillPiece}>{piece}</span>
          </Fragment>
        ))}
        <ChevronDown
          className={cn(styles.chevron, open && styles.chevronOpen)}
        />
      </button>
      {open && (
        <div className={styles.card}>
          {steps}
          {metrics && <TimeBreakdown metrics={metrics} />}
        </div>
      )}
    </section>
  );
}

export const ReasoningTrace = memo(ReasoningTraceInner);

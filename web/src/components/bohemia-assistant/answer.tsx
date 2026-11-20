import { BrandMark } from '@/components/brand-mark';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import {
  ArrowUpRight,
  ChevronDown,
  ChevronUp,
  Database,
  FileText,
} from 'lucide-react';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ComponentProps,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { isNavigablePath } from './actions';
import {
  CiteHref,
  CitedSource,
  answerMarkdown,
  citedSources,
  excerptOf,
} from './citations';
import styles from './index.module.less';

/*
 * How an answer points at its sources, as in 留白's 波西米亚: a numbered chip in
 * the text, a card for each source under it, and one floating preview shared
 * by the whole panel that slides from chip to chip. Sources are numbered by
 * document, in the order a reader meets them.
 */

/* ---------- the one floating preview ---------- */

type Preview = {
  source: CitedSource;
  chunk?: IReferenceChunk;
  rect: DOMRect;
} | null;
let preview: Preview = null;
const listeners = new Set<() => void>();
const setPreview = (next: Preview) => {
  preview = next;
  listeners.forEach((listener) => listener());
};
let showTimer: ReturnType<typeof setTimeout> | undefined;
let hideTimer: ReturnType<typeof setTimeout> | undefined;

function previewSoon(
  source: CitedSource,
  element: HTMLElement,
  chunk?: IReferenceChunk,
) {
  clearTimeout(hideTimer);
  clearTimeout(showTimer);
  const show = () =>
    setPreview({ source, chunk, rect: element.getBoundingClientRect() });
  if (preview) show();
  else showTimer = setTimeout(show, 110);
}
function previewLater() {
  clearTimeout(showTimer);
  clearTimeout(hideTimer);
  hideTimer = setTimeout(() => setPreview(null), 170);
}
const previewKeep = () => clearTimeout(hideTimer);
export function closePreview() {
  clearTimeout(showTimer);
  clearTimeout(hideTimer);
  if (preview) setPreview(null);
}
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
const usePreview = () => useSyncExternalStore(subscribe, () => preview);

/** Opens a source's document in the viewer, in a new tab. */
export function openSource(source: CitedSource) {
  closePreview();
  window.open(
    `/document/${source.documentId}?ext=${source.extension}&resource=document`,
    '_blank',
    'noopener',
  );
}

function SourceThumb({
  source,
  size = 40,
}: {
  source: CitedSource;
  size?: number;
}) {
  return (
    <span
      className={cn(styles.thumb, source.guide && styles.thumbGuide)}
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      {source.guide ? (
        <BrandMark size={Math.round(size * 0.62)} />
      ) : (
        <FileText size={Math.round(size * 0.44)} />
      )}
    </span>
  );
}

const PreviewWidth = 328;

export function SourcePreview({
  openDataset,
}: {
  openDataset: (datasetId: string) => void;
}) {
  const open = usePreview();
  const box = useRef<HTMLDivElement>(null);
  const [place, setPlace] = useState<{
    top: number;
    left: number;
    below: boolean;
  } | null>(null);
  useLayoutEffect(() => {
    if (!open || !box.current) {
      setPlace(null);
      return;
    }
    const { rect } = open;
    const height = box.current.offsetHeight;
    const width = Math.min(PreviewWidth, innerWidth - 24);
    const below =
      rect.top - height - 10 < 8 && rect.bottom + height + 10 < innerHeight;
    const top = below ? rect.bottom + 8 : Math.max(8, rect.top - height - 8);
    const left = Math.min(
      Math.max(12, rect.left + rect.width / 2 - width / 2),
      innerWidth - width - 12,
    );
    setPlace({ top, left, below });
  }, [open]);
  // Scrolling or a tap elsewhere puts it away; Escape too, without closing the panel.
  useEffect(() => {
    if (!open) return;
    const handleAway = (event: Event) => {
      if (!box.current?.contains(event.target as Node)) closePreview();
    };
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        closePreview();
      }
    };
    document.addEventListener('scroll', closePreview, true);
    document.addEventListener('pointerdown', handleAway, true);
    document.addEventListener('keydown', handleKey, true);
    return () => {
      document.removeEventListener('scroll', closePreview, true);
      document.removeEventListener('pointerdown', handleAway, true);
      document.removeEventListener('keydown', handleKey, true);
    };
  }, [open]);
  const handleOpen = useCallback(() => open && openSource(open.source), [open]);
  const handleDataset = useCallback(() => {
    if (!open) return;
    closePreview();
    openDataset(open.source.datasetId);
  }, [openDataset, open]);
  if (!open) return null;
  const { source } = open;
  const excerpt = excerptOf(open.chunk ?? source.chunks[0]);
  return createPortal(
    <div
      ref={box}
      className={cn(
        styles.preview,
        place?.below && styles.previewBelow,
        place && styles.previewOn,
      )}
      role="dialog"
      aria-label={`出处 ${source.n}`}
      style={{
        top: place?.top ?? -9999,
        left: place?.left ?? -9999,
        width: Math.min(PreviewWidth, innerWidth - 24),
      }}
      onMouseEnter={previewKeep}
      onMouseLeave={previewLater}
    >
      <div className={styles.previewBody}>
        <div className={styles.previewKind}>
          <SourceThumb source={source} size={30} />
          <i className={styles.num}>{source.n}</i>
          <span>{source.place}</span>
        </div>
        <button
          type="button"
          className={styles.previewTitle}
          onClick={handleOpen}
        >
          {source.title}
        </button>
        {excerpt && <p className={styles.previewExcerpt}>{excerpt}</p>}
      </div>
      <div className={styles.previewActions}>
        <button type="button" onClick={handleOpen}>
          <FileText size={14} />
          打开原文
        </button>
        {!source.guide && (
          <button type="button" onClick={handleDataset}>
            <Database size={14} />
            去知识库
          </button>
        )}
      </div>
    </div>,
    document.body,
  );
}

/* ---------- chips and cards share one highlight ---------- */

type HotContext = { hot: string | null; setHot: (id: string | null) => void };
const Hot = createContext<HotContext>({ hot: null, setHot: () => {} });

let lastPointer = 'mouse';

function RefChip({
  source,
  chunk,
}: {
  source: CitedSource;
  chunk?: IReferenceChunk;
}) {
  const { hot, setHot } = useContext(Hot);
  const handlePointerDown = useCallback((event: ReactPointerEvent) => {
    lastPointer = event.pointerType;
  }, []);
  const handleEnter = useCallback(
    (event: ReactMouseEvent<HTMLElement>) => {
      setHot(source.documentId);
      previewSoon(source, event.currentTarget, chunk);
    },
    [chunk, setHot, source],
  );
  const handleLeave = useCallback(() => {
    setHot(null);
    previewLater();
  }, [setHot]);
  const handleClick = useCallback(
    (event: ReactMouseEvent<HTMLElement>) => {
      // On a phone the first tap previews; the second opens.
      if (
        lastPointer === 'touch' &&
        preview?.source.documentId !== source.documentId
      ) {
        previewSoon(source, event.currentTarget, chunk);
        return;
      }
      openSource(source);
    },
    [chunk, source],
  );
  return (
    <button
      type="button"
      className={cn(styles.ref, hot === source.documentId && styles.hot)}
      aria-label={`出处 ${source.n}：${source.title}`}
      onPointerDown={handlePointerDown}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onClick={handleClick}
    >
      {source.n}
    </button>
  );
}

function SourceCard({ source }: { source: CitedSource }) {
  const { hot, setHot } = useContext(Hot);
  const handleEnter = useCallback(
    (event: ReactMouseEvent<HTMLElement>) => {
      setHot(source.documentId);
      previewSoon(source, event.currentTarget);
    },
    [setHot, source],
  );
  const handleLeave = useCallback(() => {
    setHot(null);
    previewLater();
  }, [setHot]);
  const handleOpen = useCallback(() => openSource(source), [source]);
  return (
    <div
      className={cn(styles.card, hot === source.documentId && styles.hot)}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
    >
      <button type="button" className={styles.cardMain} onClick={handleOpen}>
        <SourceThumb source={source} />
        <span className={styles.cardBody}>
          <span className={styles.cardTitle}>{source.title}</span>
          <span className={styles.cardMeta}>
            <i className={styles.num}>{source.n}</i>
            <span className={styles.cardMetaText}>
              {source.place}
              {source.chunks.length > 1
                ? ` · 引用 ${source.chunks.length} 段`
                : ''}
            </span>
          </span>
        </span>
      </button>
      <button
        type="button"
        className={styles.cardOpen}
        onClick={handleOpen}
        aria-label="打开原文"
        title="打开原文"
      >
        <ArrowUpRight size={15} />
      </button>
    </div>
  );
}

/** A page inside the app reads as a small chip, so it is clearly a place to go. */
function InLink({
  to,
  goTo,
  children,
}: {
  to: string;
  goTo: (path: string) => void;
  children?: ReactNode;
}) {
  const handleClick = useCallback(() => goTo(to), [goTo, to]);
  return (
    <button type="button" className={styles.inLink} onClick={handleClick}>
      {children}
    </button>
  );
}

const Keep = 3;

/** Sources under the answer: all when few, the first few and a fold when many. */
function Sources({ sources }: { sources: CitedSource[] }) {
  const [open, setOpen] = useState(false);
  const toggle = useCallback(() => setOpen((value) => !value), []);
  const folds = sources.length > Keep + 1;
  const shown = folds && !open ? sources.slice(0, Keep) : sources;
  const rest = sources.slice(Keep);
  const guides = sources.filter((source) => source.guide).length;
  const mix =
    guides && guides < sources.length
      ? `使用说明 ${guides} · 知识库 ${sources.length - guides}`
      : '';
  return (
    <section className={styles.sources} aria-label="出处">
      <div className={styles.label}>
        <span>出处</span>
        <i>{sources.length}</i>
        {mix && <span className={styles.labelMix}>{mix}</span>}
        {folds && (
          <button
            type="button"
            className={styles.labelButton}
            onClick={toggle}
            aria-expanded={open}
          >
            {open ? '收起' : '全部'}
          </button>
        )}
      </div>
      <div className={styles.sourceList}>
        {shown.map((source) => (
          <SourceCard key={source.documentId} source={source} />
        ))}
      </div>
      {folds && (
        <button
          type="button"
          className={cn(styles.sourceMore, open && styles.sourceMoreOpen)}
          onClick={toggle}
          aria-expanded={open}
        >
          {open ? (
            <>
              <ChevronUp size={14} />
              <span>收起，只看前 {Keep} 条</span>
            </>
          ) : (
            <>
              <span className={styles.stack} aria-hidden="true">
                {rest.slice(0, 5).map((source) => (
                  <SourceThumb
                    key={source.documentId}
                    source={source}
                    size={22}
                  />
                ))}
              </span>
              <span>还有 {rest.length} 条出处</span>
              <ChevronDown size={14} />
            </>
          )}
        </button>
      )}
    </section>
  );
}

type AnswerProps = {
  text: string;
  reference?: IReference;
  guideDatasetId: string;
  datasetName: (id: string) => string | undefined;
  navigable: string[];
  goTo: (path: string) => void;
  writing: boolean;
};

/** An answer: Markdown with citation chips, then its sources once it is complete. */
export function Answer({
  text,
  reference,
  guideDatasetId,
  datasetName,
  navigable,
  goTo,
  writing,
}: AnswerProps) {
  const [hot, setHot] = useState<string | null>(null);
  const { sources, byChunk } = useMemo(
    () => citedSources(text, reference, guideDatasetId, datasetName),
    [text, reference, guideDatasetId, datasetName],
  );
  const markdown = useMemo(
    () => answerMarkdown(text, byChunk),
    [text, byChunk],
  );
  const hotValue = useMemo(() => ({ hot, setHot }), [hot]);
  const components = useMemo(
    () => ({
      a: ({ href, children }: ComponentProps<'a'>) => {
        if (href?.startsWith(CiteHref)) {
          const index = Number(href.slice(CiteHref.length));
          const source = byChunk.get(index);
          return source ? (
            <RefChip source={source} chunk={reference?.chunks[index]} />
          ) : null;
        }
        if (href && isNavigablePath(href, navigable)) {
          return (
            <InLink to={href} goTo={goTo}>
              {children}
            </InLink>
          );
        }
        if (href && /^https?:\/\//i.test(href)) {
          return (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          );
        }
        return <span>{children}</span>;
      },
      table: ({ children }: ComponentProps<'table'>) => (
        <div className={styles.tableWrap}>
          <table>{children}</table>
        </div>
      ),
      img: () => null,
    }),
    [byChunk, goTo, navigable, reference],
  );
  if (!markdown) return null;
  return (
    <Hot.Provider value={hotValue}>
      <div className={styles.text}>
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          skipHtml
          components={components}
        >
          {markdown}
        </ReactMarkdown>
        {writing && <i className={styles.caret} aria-hidden="true" />}
      </div>
      {!writing && sources.length > 0 && <Sources sources={sources} />}
    </Hot.Provider>
  );
}

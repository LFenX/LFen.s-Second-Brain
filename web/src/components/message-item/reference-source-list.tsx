import FileIcon from '@/components/file-icon';
import {
  ClickDocumentButton,
  citationOpenMode,
  openCitationSource,
} from '@/components/markdown-content/citation-actions';
import { formatScore } from '@/components/markdown-content/citation-chip';
import { useCitationHighlight } from '@/components/markdown-content/citation-highlight';
import { IReference } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import { getExtension } from '@/utils/document-util';
import { ArrowUpRight, ChevronDown } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CitedSource } from './reasoning-chain';

const VisibleSourceCount = 4;

function SourceCard({
  source,
  reference,
  clickDocumentButton,
}: {
  source: CitedSource;
  reference: IReference;
  clickDocumentButton?: ClickDocumentButton;
}) {
  const { t } = useTranslation();
  const { activeDocumentId, setActiveDocumentId } = useCitationHighlight();
  const active = activeDocumentId === source.documentId;
  const firstChunk = reference.chunks[source.chunkIndexes[0]];
  const fileExtension = getExtension(source.documentName);
  const openable =
    !!firstChunk &&
    !!citationOpenMode(
      { fileExtension, documentUrl: source.url },
      clickDocumentButton,
    );

  const handleMouseEnter = useCallback(() => {
    setActiveDocumentId(source.documentId);
  }, [setActiveDocumentId, source.documentId]);
  const handleMouseLeave = useCallback(() => {
    setActiveDocumentId(null);
  }, [setActiveDocumentId]);
  const handleOpen = useCallback(() => {
    if (!firstChunk) return;
    openCitationSource(
      {
        documentId: source.documentId,
        chunk: firstChunk,
        fileExtension,
        documentUrl: source.url,
      },
      clickDocumentButton,
    );
  }, [clickDocumentButton, fileExtension, firstChunk, source]);

  return (
    <button
      type="button"
      className={cn(
        'group/source flex min-w-0 items-center gap-3 rounded-xl border border-border-button bg-bg-base px-3 py-2.5 text-left',
        'transition-[border-color,box-shadow] hover:border-accent-primary/50',
        active &&
          'border-accent-primary shadow-[0_0_0_3px_rgb(var(--accent-primary)/0.12)]',
        !openable && 'cursor-default',
      )}
      aria-disabled={!openable}
      onClick={handleOpen}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      <span className="grid size-9 flex-none place-items-center overflow-hidden rounded-lg bg-bg-card">
        <FileIcon
          id={source.documentId}
          name={source.documentName}
          width={20}
        />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span
          className="truncate text-[13px] font-medium leading-snug text-text-primary"
          title={source.documentName}
        >
          {source.documentName}
        </span>
        <span className="flex min-w-0 items-center gap-1.5 text-[11px] leading-none text-text-secondary">
          {source.chunkIndexes.slice(0, 5).map((index) => (
            <span
              key={index}
              className={cn(
                'inline-flex h-4 min-w-4 items-center justify-center rounded px-1 font-semibold tabular-nums',
                active
                  ? 'bg-accent-primary text-bg-base'
                  : 'bg-accent-primary/10 text-accent-primary',
              )}
            >
              {index + 1}
            </span>
          ))}
          {source.chunkIndexes.length > 5 && <span>…</span>}
          <span className="truncate">
            {t('chat.citation.chunkCount', {
              count: source.chunkIndexes.length,
            })}
            {source.bestSimilarity !== undefined &&
              ` · ${t('chat.citation.relevance')} ${formatScore(source.bestSimilarity)}`}
          </span>
        </span>
      </span>
      {openable && (
        <ArrowUpRight className="size-4 flex-none text-text-disabled transition-colors group-hover/source:text-text-primary" />
      )}
    </button>
  );
}

/**
 * The documents an answer cites, one card per document in the order the text
 * first cites them. Hovering a card lights its chips in the text.
 */
export function ReferenceSourceList({
  sources,
  reference,
  clickDocumentButton,
}: {
  sources: CitedSource[];
  reference: IReference;
  clickDocumentButton?: ClickDocumentButton;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const folds = sources.length > VisibleSourceCount + 1;
  const shown =
    folds && !expanded ? sources.slice(0, VisibleSourceCount) : sources;

  const handleToggle = useCallback(() => setExpanded((value) => !value), []);

  if (!sources.length) return null;

  return (
    <section
      className="@container flex flex-col gap-2"
      aria-label={t('chat.citation.sources')}
    >
      <div className="flex items-center gap-2 text-xs font-medium text-text-secondary">
        <span>{t('chat.citation.sources')}</span>
        <span className="tabular-nums text-text-disabled">
          {sources.length}
        </span>
        <span className="h-px flex-1 bg-border-button" />
        {folds && (
          <button
            type="button"
            className="rounded-full px-2 py-0.5 font-normal text-text-secondary transition-colors hover:bg-bg-card hover:text-text-primary"
            onClick={handleToggle}
            aria-expanded={expanded}
          >
            {expanded
              ? t('chat.citation.collapse')
              : t('chat.citation.showAll')}
          </button>
        )}
      </div>
      <div className="grid grid-cols-1 gap-2 @lg:grid-cols-2">
        {shown.map((source) => (
          <SourceCard
            key={source.documentId}
            source={source}
            reference={reference}
            clickDocumentButton={clickDocumentButton}
          />
        ))}
      </div>
      {folds && !expanded && (
        <button
          type="button"
          className="flex h-9 items-center justify-center gap-1.5 rounded-xl border border-dashed border-border-button text-xs text-text-secondary transition-colors hover:bg-bg-card hover:text-text-primary"
          onClick={handleToggle}
          aria-expanded={expanded}
        >
          {t('chat.citation.more', {
            count: sources.length - VisibleSourceCount,
          })}
          <ChevronDown className="size-3.5" />
        </button>
      )}
    </section>
  );
}

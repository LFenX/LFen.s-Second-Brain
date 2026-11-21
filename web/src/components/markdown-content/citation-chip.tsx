import { Portal } from '@radix-ui/react-hover-card';
import Image, { AuthenticatedImg } from '@/components/image';
import SvgIcon, { hasSvgIcon } from '@/components/svg-icon';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { cn } from '@/lib/utils';
import { ArrowUpRight, FileText, LocateFixed } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useCitationHighlight } from './citation-highlight';
import styles from './index.module.less';

const isScore = (value?: number | null): value is number =>
  typeof value === 'number' && Number.isFinite(value);

export const formatScore = (value: number) => value.toFixed(2);

interface CitationChipProps {
  /** The number the reader sees, 1-based. */
  number: number;
  documentId?: string;
  /** The marker arrived before its sources; it cannot be previewed yet. */
  pending?: boolean;
  onOpen?: () => void;
  preview?: ReactNode;
}

/**
 * A citation as a small numbered chip. Hovering previews the chunk and lights
 * the document's source card; a click opens the source.
 */
export function CitationChip({
  number,
  documentId,
  pending = false,
  onOpen,
  preview,
}: CitationChipProps) {
  const { t } = useTranslation();
  const { activeDocumentId, setActiveDocumentId } = useCitationHighlight();
  const active = !!documentId && activeDocumentId === documentId;

  const handleMouseEnter = useCallback(() => {
    if (documentId) setActiveDocumentId(documentId);
  }, [documentId, setActiveDocumentId]);
  const handleMouseLeave = useCallback(() => {
    setActiveDocumentId(null);
  }, [setActiveDocumentId]);

  const chip = (
    <button
      type="button"
      className={cn(
        'relative -top-px mx-0.5 inline-flex h-[18px] min-w-[18px] cursor-pointer items-center justify-center rounded-md px-[5px] align-middle',
        'text-[11px] font-semibold leading-none tabular-nums transition-colors',
        'bg-accent-primary/10 text-accent-primary hover:bg-accent-primary hover:text-bg-base',
        active && 'bg-accent-primary text-bg-base',
        pending &&
          'cursor-default animate-pulse bg-bg-card text-text-disabled hover:bg-bg-card hover:text-text-disabled',
      )}
      aria-label={t('chat.citation.label', { number })}
      disabled={pending}
      onClick={onOpen}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      <bdi>{number}</bdi>
    </button>
  );

  if (!preview) return chip;

  return (
    <HoverCard openDelay={120} closeDelay={100}>
      <HoverCardTrigger asChild>{chip}</HoverCardTrigger>
      <Portal>
        <HoverCardContent
          side="top"
          className="w-[360px] max-w-[calc(100vw-32px)] overflow-hidden whitespace-normal rounded-xl border-border-button p-0 shadow-lg"
        >
          {preview}
        </HoverCardContent>
      </Portal>
    </HoverCard>
  );
}

function RelevanceMeter({
  similarity,
  vectorSimilarity,
  termSimilarity,
}: {
  similarity: number;
  vectorSimilarity?: number | null;
  termSimilarity?: number | null;
}) {
  const { t } = useTranslation();
  const width = Math.min(1, Math.max(0, similarity)) * 100;
  const showBreakdown =
    isScore(vectorSimilarity) &&
    isScore(termSimilarity) &&
    (vectorSimilarity > 0 || termSimilarity > 0);

  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-text-secondary">
      <span>{t('chat.citation.relevance')}</span>
      <span className="relative h-1 w-14 overflow-hidden rounded-full bg-bg-card">
        <span
          className="absolute inset-y-0 left-0 rounded-full bg-accent-primary"
          style={{ width: `${width}%` }}
        />
      </span>
      <span className="font-medium tabular-nums text-text-primary">
        {formatScore(similarity)}
      </span>
      {showBreakdown && (
        <span className="tabular-nums text-text-disabled">
          {t('chat.citation.semantic')} {formatScore(vectorSimilarity)} ·{' '}
          {t('chat.citation.keyword')} {formatScore(termSimilarity)}
        </span>
      )}
    </div>
  );
}

interface CitationPreviewProps {
  number: number;
  documentId?: string;
  documentName?: string;
  fileThumbnail?: string;
  fileExtension?: string;
  imageId?: string;
  /** Sanitized chunk HTML. */
  contentHtml: string;
  similarity?: number | null;
  vectorSimilarity?: number | null;
  termSimilarity?: number | null;
  openMode?: 'locate' | 'open';
  onOpen?: () => void;
}

/** The floating card behind a citation chip: which document, how relevant, what it says. */
export function CitationPreview({
  number,
  documentId,
  documentName,
  fileThumbnail,
  fileExtension,
  imageId,
  contentHtml,
  similarity,
  vectorSimilarity,
  termSimilarity,
  openMode,
  onOpen,
}: CitationPreviewProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col">
      {imageId && (
        <div className="flex justify-center border-b border-border-button bg-bg-card">
          <Image
            id={imageId}
            documentId={documentId}
            className="max-h-40 w-auto max-w-full object-contain"
          />
        </div>
      )}
      <div className="flex flex-col gap-2 px-4 pb-3 pt-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="inline-flex h-[18px] min-w-[18px] flex-none items-center justify-center rounded-md bg-accent-primary px-[5px] text-[11px] font-semibold leading-none tabular-nums text-bg-base">
            {number}
          </span>
          {documentId &&
            (fileThumbnail ? (
              <AuthenticatedImg
                src={fileThumbnail}
                alt=""
                className="size-4 flex-none rounded-sm object-cover"
              />
            ) : hasSvgIcon(`file-icon/${fileExtension}`) ? (
              <SvgIcon
                name={`file-icon/${fileExtension}`}
                width={16}
                className="flex-none"
              />
            ) : (
              <FileText className="size-4 flex-none text-text-secondary" />
            ))}
          <span
            className="min-w-0 truncate text-[13px] font-medium text-text-primary"
            title={documentName}
          >
            {documentName}
          </span>
        </div>
        {isScore(similarity) && (
          <RelevanceMeter
            similarity={similarity}
            vectorSimilarity={vectorSimilarity}
            termSimilarity={termSimilarity}
          />
        )}
        <div
          className={styles.citationExcerpt}
          dir="auto"
          dangerouslySetInnerHTML={{ __html: contentHtml }}
        />
      </div>
      {openMode && onOpen && (
        <div className="px-3 pb-3">
          <button
            type="button"
            className="inline-flex h-7 items-center gap-1.5 rounded-full border border-border-button px-3 text-xs text-text-primary transition-colors hover:border-accent-primary hover:text-accent-primary"
            onClick={onOpen}
          >
            {openMode === 'locate' ? (
              <LocateFixed className="size-3.5" />
            ) : (
              <ArrowUpRight className="size-3.5" />
            )}
            {openMode === 'locate'
              ? t('chat.citation.locate')
              : t('chat.citation.open')}
          </button>
        </div>
      )}
    </div>
  );
}

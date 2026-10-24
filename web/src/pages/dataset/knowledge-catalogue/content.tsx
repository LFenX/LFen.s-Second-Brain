import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useKnowledgeContent } from '@/hooks/use-knowledge-foundation';
import type {
  KnowledgeNode,
  KnowledgeSource,
} from '@/interfaces/database/knowledge-foundation';
import { CatalogueFailure, CatalogueLoading } from './state';

export default function SourceContent({
  domainId,
  node,
  source,
  onRefresh,
}: {
  domainId: string;
  node: KnowledgeNode;
  source: KnowledgeSource;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  const [offsets, setOffsets] = useState([node.start_byte]);
  const start = offsets[offsets.length - 1];
  const query = useKnowledgeContent(
    domainId,
    node.node_id,
    start,
    Math.min(node.end_byte, start + 65536),
  );
  const handleNext = () => {
    if (query.data && query.data.end_byte > start)
      setOffsets([...offsets, query.data.end_byte]);
  };
  const handlePrevious = () => setOffsets(offsets.slice(0, -1));
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={onRefresh} />;
  if (!query.data || query.isPending) return <CatalogueLoading />;
  const content = query.data;
  if (
    content.node_id !== node.node_id ||
    content.source_revision_id !== source.source_revision_id ||
    content.source_id !== source.source_id ||
    content.source_sha256 !== source.sha256 ||
    content.start_byte !== start ||
    content.end_byte < start ||
    content.end_byte > Math.min(node.end_byte, start + 65536) ||
    new TextEncoder().encode(content.text).length !==
      content.end_byte - start ||
    (start < node.end_byte && content.end_byte === start)
  ) {
    return (
      <CatalogueFailure
        error={new Error('source identity mismatch')}
        onRetry={onRefresh}
      />
    );
  }
  return (
    <section
      aria-label={t('knowledgeCatalogue.original')}
      className="min-w-0 space-y-3"
    >
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs text-text-secondary">
        <dt>{t('knowledgeCatalogue.path')}</dt>
        <dd className="break-all">{source.relative_path}</dd>
        <dt>{t('knowledgeCatalogue.revision')}</dt>
        <dd className="break-all">{content.source_revision_id}</dd>
        <dt>SHA-256</dt>
        <dd className="break-all">{content.source_sha256}</dd>
        <dt>{t('knowledgeCatalogue.range')}</dt>
        <dd>
          [{content.start_byte}, {content.end_byte}) · UTF-8
        </dd>
        <dt>{t('knowledgeCatalogue.lines')}</dt>
        <dd>
          {node.start_line}–{node.end_line} ({t('knowledgeCatalogue.nodeRange')}
          )
        </dd>
      </dl>
      <pre
        data-testid="knowledge-original"
        className="max-h-[60vh] overflow-auto whitespace-pre rounded-lg border border-border-button bg-bg-base p-4 font-mono text-sm text-text-primary"
      >
        <code>{content.text}</code>
      </pre>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="outline"
          disabled={offsets.length === 1 || query.isFetching}
          onClick={handlePrevious}
        >
          {t('knowledgeCatalogue.previous')}
        </Button>
        <Button
          variant="outline"
          disabled={content.end_byte >= node.end_byte || query.isFetching}
          onClick={handleNext}
        >
          {t('knowledgeCatalogue.next')}
        </Button>
        <span className="text-xs text-text-secondary">
          {t('knowledgeCatalogue.windowHint')}
        </span>
      </div>
    </section>
  );
}

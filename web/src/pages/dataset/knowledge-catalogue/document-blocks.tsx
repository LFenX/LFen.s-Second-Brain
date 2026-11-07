import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useDocumentChildren } from '@/hooks/use-knowledge-document';
import type {
  DocumentNode,
  DocumentRepresentation,
} from '@/interfaces/database/knowledge-document';
import type { DocumentViewScope } from '@/utils/knowledge-document';
import { DocumentText, DocumentMedia } from './document-content';
import { CatalogueLoading, LoadMore } from './state';
import { DocumentFailure } from './document-state';
export function DocumentBlockView({
  scope,
  representation,
  node,
  selected = false,
  depth = 0,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node: DocumentNode;
  selected?: boolean;
  depth?: number;
}) {
  const { t } = useTranslation(),
    kind = node.payload.kind;
  const [expanded, setExpanded] = useState(
      selected ||
        ['paragraph', 'text_block', 'row', 'cell', 'list_item'].includes(kind),
    ),
    [aggregate, setAggregate] = useState(false);
  const expand = () => setExpanded((value) => !value),
    toggleAggregate = () => setAggregate((value) => !value);
  if (depth > 16)
    return <DocumentFailure error={Error('document_depth_exceeded')} />;
  if (kind === 'media') return <DocumentMedia node={node} />;
  if (kind === 'unsupported_block')
    return (
      <div role="note" className="rounded border border-border-button p-3">
        {t('knowledgeDocument.unsupported')}:{' '}
        {node.payload.attributes.native_type}
      </div>
    );
  if (kind === 'passage' && !selected) return null;
  const role =
    kind === 'table'
      ? 'table'
      : kind === 'row'
        ? 'row'
        : kind === 'cell'
          ? 'cell'
          : undefined;
  const className =
    kind === 'row'
      ? 'min-w-0 space-y-2 border-b border-border-button p-2'
      : kind === 'cell'
        ? 'min-w-48 flex-1 border border-border-button p-2'
        : 'min-w-0 space-y-2 rounded border border-border-button p-3';
  return (
    <div role={role} className={className}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">
          {t('knowledgeDocument.kind_' + kind)}
          {node.payload.name ? ' · ' + node.payload.name : ''}
        </span>
        {node.payload.name_truncated && (
          <span>{t('knowledgeDocument.nameTruncated')}</span>
        )}
        <Button variant="outline" size="sm" onClick={expand}>
          {t(
            expanded
              ? 'knowledgeDocument.collapse'
              : 'knowledgeDocument.expand',
          )}
        </Button>
        {selected &&
          node.payload.own_reading_range &&
          node.payload.own_reading_range.end_byte <
            node.payload.reading_range.end_byte && (
            <Button variant="outline" size="sm" onClick={toggleAggregate}>
              {t(
                aggregate
                  ? 'knowledgeDocument.ownText'
                  : 'knowledgeDocument.aggregateText',
              )}
            </Button>
          )}
      </div>
      <DocumentPosition node={node} />
      <DocumentAttributes node={node} />
      {expanded &&
        (aggregate ? (
          <DocumentText
            scope={scope}
            representation={representation}
            node={node}
            aggregate
          />
        ) : (
          <DocumentBlockChildren
            scope={scope}
            representation={representation}
            node={node}
            depth={depth}
          />
        ))}
      <details>
        <summary className="text-xs text-text-secondary">
          {t('knowledgeDocument.evidence')}
        </summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(node.payload, null, 2)}
        </pre>
      </details>
    </div>
  );
}
function DocumentBlockChildren({
  scope,
  representation,
  node,
  depth,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node: DocumentNode;
  depth: number;
}) {
  const { t } = useTranslation(),
    query = useDocumentChildren(scope, representation, node),
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending) return <CatalogueLoading />;
  const nodes = query.data.pages.flatMap((page) => page.items),
    inline = nodes.filter(
      (n) =>
        n.payload.kind === 'media' &&
        n.payload.attributes.placement === 'inline',
    );
  const children = nodes.filter(
    (n) =>
      n.payload.kind !== 'passage' &&
      !(
        n.payload.kind === 'media' &&
        n.payload.attributes.placement === 'inline'
      ),
  );
  const childBlocks = children.map((child) => (
    <DocumentBlockView
      key={child.document_node_id}
      scope={scope}
      representation={representation}
      node={child}
      depth={depth + 1}
    />
  ));
  return (
    <>
      <DocumentText
        scope={scope}
        representation={representation}
        node={node}
        media={inline}
      />
      {node.payload.kind === 'row' ? (
        <div className="flex min-w-0 items-stretch gap-2 overflow-x-auto">
          {childBlocks}
        </div>
      ) : (
        childBlocks
      )}
      {!children.length &&
        !node.payload.own_reading_range &&
        !inline.length && (
          <p className="text-sm text-text-secondary">
            {t('knowledgeDocument.text_' + node.payload.text_status)}
          </p>
        )}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={more}
      />
    </>
  );
}
export function DocumentPosition({ node }: { node: DocumentNode }) {
  const { t } = useTranslation(),
    position = node.payload.original_position;
  return (
    <p className="text-xs text-text-secondary">
      {t('knowledgeDocument.derivedCoordinates')}
      {position.kind === 'pdf_page'
        ? ' · ' + t('knowledgeDocument.page', { number: position.page_number })
        : ''}
      {position.kind === 'pptx_slide'
        ? ' · ' +
          t('knowledgeDocument.slide', { number: position.slide_number })
        : ''}
      {node.payload.kind === 'slide' && node.payload.attributes.hidden
        ? ' · ' + t('knowledgeDocument.hiddenSlide')
        : ''}
    </p>
  );
}
function DocumentAttributes({ node }: { node: DocumentNode }) {
  const { t } = useTranslation(),
    p = node.payload;
  if (p.kind === 'list')
    return (
      <p className="text-xs text-text-secondary">
        {t(
          p.attributes.ordered === true
            ? 'knowledgeDocument.orderedList'
            : p.attributes.ordered === false
              ? 'knowledgeDocument.unorderedList'
              : 'knowledgeDocument.unknownList',
        )}{' '}
        ·{' '}
        {t('knowledgeDocument.listLevel', {
          level: p.attributes.source_level ?? t('knowledgeDocument.unknown'),
        })}
        {p.attributes.start_number !== null
          ? ' · ' +
            t('knowledgeDocument.listStart', {
              number: p.attributes.start_number,
            })
          : ''}
      </p>
    );
  if (p.kind === 'list_item')
    return (
      <p className="text-xs text-text-secondary">
        {t('knowledgeDocument.itemPosition', {
          index: p.attributes.item_index + 1,
        })}
      </p>
    );
  if (p.kind === 'table')
    return (
      <p className="text-xs text-text-secondary">
        {t('knowledgeDocument.tableSource', {
          rows: p.attributes.row_count,
          cells: p.attributes.max_cells_per_row,
        })}
      </p>
    );
  return null;
}

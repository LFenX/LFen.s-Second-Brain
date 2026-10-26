import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useDocumentChildren } from '@/hooks/use-knowledge-document';
import type {
  DocumentNode,
  DocumentRepresentation,
} from '@/interfaces/database/knowledge-document';
import type { DocumentViewScope } from '@/utils/knowledge-document';
import { CatalogueLoading, LoadMore } from './state';
import { DocumentFailure } from './document-state';
export function DocumentTree({
  scope,
  representation,
  node,
  selected,
  onSelect,
  depth = 0,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node: DocumentNode;
  selected: string;
  onSelect: (node: string) => void;
  depth?: number;
}) {
  const { t } = useTranslation(),
    [open, setOpen] = useState(false),
    choose = () => onSelect(node.document_node_id),
    toggle = () => setOpen((value) => !value);
  if (depth > 16)
    return <DocumentFailure error={Error('document_depth_exceeded')} />;
  return (
    <li className="space-y-2">
      <div className="flex gap-2">
        <Button
          className="h-auto max-w-full whitespace-normal break-all text-left"
          variant={selected === node.document_node_id ? 'default' : 'outline'}
          onClick={choose}
        >
          {t('knowledgeDocument.kind_' + node.payload.kind)} ·{' '}
          {node.payload.name || t('knowledgeDocument.unnamed')}
        </Button>
        {!['media', 'unsupported_block', 'passage'].includes(
          node.payload.kind,
        ) && (
          <Button
            variant="outline"
            size="sm"
            aria-expanded={open}
            onClick={toggle}
          >
            {t(
              open ? 'knowledgeDocument.collapse' : 'knowledgeDocument.expand',
            )}
          </Button>
        )}
      </div>
      {open && (
        <DocumentTreeChildren
          scope={scope}
          representation={representation}
          parent={node}
          selected={selected}
          onSelect={onSelect}
          depth={depth + 1}
        />
      )}
    </li>
  );
}
function DocumentTreeChildren({
  scope,
  representation,
  parent,
  selected,
  onSelect,
  depth,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  parent: DocumentNode;
  selected: string;
  onSelect: (node: string) => void;
  depth: number;
}) {
  const query = useDocumentChildren(scope, representation, parent),
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending) return <CatalogueLoading />;
  return (
    <div className="space-y-2 border-l border-border-button pl-3">
      <ul className="space-y-2">
        {query.data.pages
          .flatMap((page) => page.items)
          .map((node) => (
            <DocumentTree
              key={node.document_node_id}
              scope={scope}
              representation={representation}
              node={node}
              selected={selected}
              onSelect={onSelect}
              depth={depth}
            />
          ))}
      </ul>
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={more}
      />
    </div>
  );
}

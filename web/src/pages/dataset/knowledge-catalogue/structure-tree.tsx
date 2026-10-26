import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { TreeView as Tree, type TreeDataItem } from '@/components/ui/tree-view';
import { useStructureNodes } from '@/hooks/use-knowledge-structure';
import type { StructureViewScope } from '@/utils/knowledge-structure';
import { CatalogueLoading, LoadMore } from './state';
import { StructureFailure } from './structure-state';

export function StructureTree({
  scope,
  source,
  onNode,
}: {
  scope: StructureViewScope;
  source: string;
  onNode: (node: string) => void;
}) {
  const { t } = useTranslation(),
    [path, setPath] = useState<{ id: string; name: string }[]>([]),
    query = useStructureNodes(scope, {
      source_id: source,
      ...(path.length ? { parent_id: path[path.length - 1].id } : {}),
      limit: 20,
    });
  const parent = () => setPath((value) => value.slice(0, -1)),
    reload = () => {
      void query.refetch();
    },
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  const data: TreeDataItem[] = query.data.pages
    .flatMap((page) => page.items)
    .map((node) => ({
      id: node.structure_node_id,
      name: node.name,
      entityType: t('knowledgeStructure.node_' + node.kind),
      hasChildren: node.kind !== 'passage' && path.length < 16,
      onClick: () => onNode(node.structure_node_id),
      onExpand: () =>
        setPath((value) => [
          ...value,
          { id: node.structure_node_id, name: node.name },
        ]),
    }));
  return (
    <section className="space-y-2">
      <h4 className="font-semibold">{t('knowledgeStructure.derivedTree')}</h4>
      <p className="break-all text-xs">
        {path.map((node) => node.name).join(' / ')}
      </p>
      {path.length > 0 && (
        <Button type="button" variant="outline" onClick={parent}>
          {t('knowledgeCatalogue.parentNode')}
        </Button>
      )}
      <Tree
        key={path.at(-1)?.id ?? source}
        data={data}
        expandOnRowClick={false}
      />
      {data.length === 0 && <p>{t('knowledgeCatalogue.empty')}</p>}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetchingNextPage}
        onMore={more}
      />
    </section>
  );
}

import { useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useStructureRelations } from '@/hooks/use-knowledge-structure';
import type {
  StructureRelation,
  StructureRelationKind,
  StructureView,
} from '@/interfaces/database/knowledge-structure';
import type { StructureViewScope } from '@/utils/knowledge-structure';
import { CatalogueLoading, LoadMore } from './state';
import { StructureFailure } from './structure-state';

const Kinds: StructureRelationKind[] = [
  'contains',
  'imports_file',
  'exports_symbol',
  'references_type',
  'calls_local',
];
function RelationRow({
  scope,
  relation,
  onNode,
}: {
  scope: StructureViewScope;
  relation: StructureRelation;
  onNode: (node: string) => void;
}) {
  const { t } = useTranslation(),
    target = relation.to_node_id,
    open = () => {
      if (
        relation.resolution === 'resolved' &&
        target &&
        relation.view_id === scope.view
      )
        onNode(target);
    },
    origin = () => onNode(relation.from_node_id);
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p>
        {relation.kind} · {t('knowledgeStructure.' + relation.resolution)} ·{' '}
        {relation.reason}
      </p>
      <pre className="whitespace-pre-wrap break-all text-xs">
        {relation.reference}
      </pre>
      {relation.reference_truncated && (
        <p>{t('knowledgeStructure.referenceTruncated')}</p>
      )}
      <p className="break-all text-xs">
        {t('knowledgeStructure.evidence')}: {relation.evidence.source_id} /{' '}
        {relation.evidence.source_revision_id} [{relation.evidence.start_byte},
        {relation.evidence.end_byte}) · {relation.evidence.fragment_sha256}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" onClick={origin}>
          {t('knowledgeStructure.openFrom')}
        </Button>
        {relation.resolution === 'resolved' &&
          target &&
          relation.view_id === scope.view && (
            <Button type="button" variant="outline" onClick={open}>
              {t('knowledgeStructure.openTarget')}
            </Button>
          )}
      </div>
    </li>
  );
}
export function StructureRelations({
  scope,
  view,
  node,
  onNode,
}: {
  scope: StructureViewScope;
  view: StructureView;
  node: string;
  onNode: (node: string) => void;
}) {
  const { t } = useTranslation(),
    [direction, setDirection] = useState<'in' | 'out' | 'both'>('both'),
    [kind, setKind] = useState<StructureRelationKind | ''>(''),
    query = useStructureRelations(scope, {
      node_id: node,
      direction,
      ...(kind ? { kind } : {}),
      limit: 20,
    });
  const directionChanged = (event: ChangeEvent<HTMLSelectElement>) =>
      setDirection(event.target.value as typeof direction),
    kindChanged = (event: ChangeEvent<HTMLSelectElement>) =>
      setKind(event.target.value as typeof kind),
    reload = () => {
      void query.refetch();
    },
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  const items = query.data.pages.flatMap((page) => page.items),
    valid = items.every((relation) =>
      view.members.some(
        (member) =>
          member.source_id === relation.evidence.source_id &&
          member.source_revision_id === relation.evidence.source_revision_id &&
          relation.evidence.end_byte <= member.byte_length,
      ),
    );
  if (!valid)
    return (
      <StructureFailure error={Error('Structure evidence source mismatch')} />
    );
  return (
    <section className="space-y-3">
      <h4 className="font-semibold">{t('knowledgeStructure.oneHop')}</h4>
      <p className="text-xs text-text-secondary">
        {t('knowledgeStructure.relationsHint')}
      </p>
      <div className="flex flex-wrap gap-2">
        <label>
          {t('knowledgeStructure.direction')}{' '}
          <select
            aria-label={t('knowledgeStructure.direction')}
            className="rounded border border-border-button bg-bg-base p-2"
            value={direction}
            onChange={directionChanged}
          >
            {['both', 'in', 'out'].map((value) => (
              <option key={value} value={value}>
                {t('knowledgeStructure.direction_' + value)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t('knowledgeStructure.relationKind')}{' '}
          <select
            aria-label={t('knowledgeStructure.relationKind')}
            className="rounded border border-border-button bg-bg-base p-2"
            value={kind}
            onChange={kindChanged}
          >
            <option value="">{t('knowledgeStructure.allKinds')}</option>
            {Kinds.map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
        </label>
      </div>
      <ul className="space-y-2">
        {items.map((relation) => (
          <RelationRow
            key={relation.relation_id}
            scope={scope}
            relation={relation}
            onNode={onNode}
          />
        ))}
      </ul>
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetchingNextPage}
        onMore={more}
      />
    </section>
  );
}

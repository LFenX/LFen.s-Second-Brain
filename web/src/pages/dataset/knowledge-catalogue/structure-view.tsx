import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import {
  useStructureHead,
  useStructureNode,
  useStructureView,
} from '@/hooks/use-knowledge-structure';
import type {
  StructureView,
  StructureViewMember,
} from '@/interfaces/database/knowledge-structure';
import type {
  StructureProjectScope,
  StructureViewScope,
} from '@/utils/knowledge-structure';
import { sameDirectoryClassification } from '@/utils/knowledge-directory';
import { knowledgeReceiptLink } from '@/utils/knowledge-source-link';
import { CatalogueLoading } from './state';
import { StructureFailure } from './structure-state';
import { StructureTree } from './structure-tree';
import { StructureContent } from './structure-content';
import { StructureRelations } from './structure-relations';

export function StructureHeadPanel({
  scope,
  fixed,
  onView,
  onRefresh,
}: {
  scope: StructureProjectScope;
  fixed: string | null;
  onView: (view: string) => void;
  onRefresh?: () => void;
}) {
  const { t } = useTranslation(),
    query = useStructureHead(scope),
    reload = () => {
      onRefresh?.();
      void query.refetch();
    },
    open = () => {
      if (query.data?.view_id) onView(query.data.view_id);
    };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  const head = query.data;
  return (
    <section className="space-y-2 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeStructure.currentHead')}</h3>
      <p>{t('knowledgeStructure.currentness_' + head.currentness)}</p>
      <p className="break-all text-xs">{head.view_id ?? '∅'}</p>
      {fixed && fixed !== head.view_id && (
        <p>{t('knowledgeStructure.fixedDifferent')}</p>
      )}
      {head.reasons.length > 0 && (
        <p role="status">
          {t('knowledgeStructure.staleReasons')}:{' '}
          {head.reasons
            .map((reason) => t('knowledgeStructure.reason_' + reason))
            .join(' / ')}
        </p>
      )}
      <ul>
        {head.affected_sources.map((source) => (
          <li key={source.source_id} className="break-all text-xs">
            {source.source_id}:{' '}
            {source.reasons
              .map((reason) => t('knowledgeStructure.reason_' + reason))
              .join(' / ')}
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" onClick={reload}>
          {t('knowledgeCatalogue.refresh')}
        </Button>
        {head.view_id && (
          <Button type="button" variant="outline" onClick={open}>
            {t('knowledgeStructure.openHead')}
          </Button>
        )}
      </div>
    </section>
  );
}
function MemberRow({
  member,
  onChoose,
  selected,
}: {
  member: StructureViewMember;
  onChoose: (member: StructureViewMember) => void;
  selected: boolean;
}) {
  const choose = () => onChoose(member);
  return (
    <li>
      <Button
        type="button"
        variant="outline"
        className="h-auto w-full justify-start whitespace-normal text-left"
        aria-pressed={selected}
        onClick={choose}
      >
        <span className="break-all">
          {member.relative_path} · {member.classification.component_side} /{' '}
          {member.classification.language}
        </span>
      </Button>
    </li>
  );
}
function StructureNodeDetail({
  scope,
  view,
  nodeId,
  onNode,
  onMember,
}: {
  scope: StructureViewScope;
  view: StructureView;
  nodeId: string;
  onNode: (node: string) => void;
  onMember: (member: StructureViewMember) => void;
}) {
  const { t } = useTranslation(),
    query = useStructureNode(scope, nodeId),
    reload = () => {
      void query.refetch();
    },
    node = query.data,
    member = view.members.find((value) => value.source_id === node?.source_id),
    valid =
      !query.isError &&
      node &&
      member &&
      node.source_revision_id === member.source_revision_id &&
      node.source_node_id === member.source_node_id &&
      node.parser === member.parser &&
      node.producer_sha256 === view.producer_sha256 &&
      node.end_byte <= member.byte_length &&
      sameDirectoryClassification(node.classification, member.classification);
  useEffect(() => {
    if (valid && member) onMember(member);
  }, [valid, member, onMember]);
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!node) return <CatalogueLoading />;
  if (!valid || !member)
    return (
      <StructureFailure error={Error('Structure member identity mismatch')} />
    );
  return (
    <div className="space-y-5">
      <p className="break-all text-sm font-medium">{member.relative_path}</p>
      <h3 className="break-all font-semibold">
        {node.name} · {t('knowledgeStructure.node_' + node.kind)}
      </h3>
      <p className="break-all text-xs text-text-secondary">
        {node.structure_node_id} · {node.identity_stability} · {node.start_line}
        –{node.end_line}
      </p>
      <Link
        className="text-sm underline"
        to={knowledgeReceiptLink(scope.domain, {
          source_id: member.source_id,
          source_revision_id: member.source_revision_id,
          root_node_id: member.source_node_id,
          sha256: member.sha256,
        })}
      >
        {t('knowledgeStructure.sourceBacklink')}
      </Link>
      <StructureContent
        key={node.structure_node_id}
        scope={scope}
        node={node}
        member={member}
      />
      <StructureRelations
        key={node.structure_node_id + ':relations'}
        scope={scope}
        view={view}
        node={nodeId}
        onNode={onNode}
      />
    </div>
  );
}
function FixedViewBody({
  scope,
  view,
}: {
  scope: StructureViewScope;
  view: StructureView;
}) {
  const { t } = useTranslation(),
    [member, setMember] = useState(view.members[0]),
    [node, setNode] = useState(view.members[0].root_structure_node_id);
  const choose = (value: StructureViewMember) => {
    setMember(value);
    setNode(value.root_structure_node_id);
  };
  const followMember = useCallback((value: StructureViewMember) => {
    setMember((current) =>
      current.source_id === value.source_id ? current : value,
    );
  }, []);
  return (
    <>
      <p className="break-all text-xs">
        {t('knowledgeStructure.fixedView')}: {view.view_id} ·{' '}
        {view.source_count} / {view.node_count} / {view.relation_count}
      </p>
      <p className="text-xs text-text-secondary">
        {t('knowledgeStructure.fixedViewHint')}
      </p>
      <ResizablePanelGroup
        direction="horizontal"
        className="min-h-[520px] rounded border border-border-button"
      >
        <ResizablePanel defaultSize={35} minSize={20}>
          <aside className="h-full space-y-4 overflow-auto p-3">
            <h4 className="font-semibold">{t('knowledgeStructure.members')}</h4>
            <ul className="space-y-2">
              {view.members.map((value) => (
                <MemberRow
                  key={value.source_id}
                  member={value}
                  onChoose={choose}
                  selected={value.source_id === member.source_id}
                />
              ))}
            </ul>
            <p className="break-all text-xs">{member.relative_path}</p>
            <StructureTree
              key={member.source_id}
              scope={scope}
              source={member.source_id}
              onNode={setNode}
            />
          </aside>
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize={65} minSize={35}>
          <div className="h-full overflow-auto p-3">
            <StructureNodeDetail
              key={node}
              scope={scope}
              view={view}
              nodeId={node}
              onNode={setNode}
              onMember={followMember}
            />
          </div>
        </ResizablePanel>
      </ResizablePanelGroup>
    </>
  );
}
export function StructureFixedView({ scope }: { scope: StructureViewScope }) {
  const query = useStructureView(scope),
    reload = () => {
      void query.refetch();
    };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  return (
    <section className="space-y-3">
      <FixedViewBody key={scope.view} scope={scope} view={query.data} />
    </section>
  );
}

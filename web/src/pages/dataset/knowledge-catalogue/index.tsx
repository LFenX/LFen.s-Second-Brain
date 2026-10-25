import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams, useSearchParams } from 'react-router';
import { Button } from '@/components/ui/button';
import {
  useKnowledgeContract,
  useKnowledgeDomain,
  useKnowledgeGroups,
  useKnowledgeNode,
  useKnowledgeNodes,
  useKnowledgeSources,
  useKnowledgeSource,
} from '@/hooks/use-knowledge-foundation';
import type {
  KnowledgeGroup,
  KnowledgeNode,
  KnowledgeSource,
} from '@/interfaces/database/knowledge-foundation';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  knowledgeFixedTarget,
  type KnowledgeFixedTarget,
} from '@/utils/knowledge-source-link';
import SourceContent from './content';
import {
  CatalogueEmpty,
  CatalogueFailure,
  CatalogueLoading,
  CatalogueScopeFailure,
  LoadMore,
} from './state';

function GroupRow({
  group,
  onSelect,
}: {
  group: KnowledgeGroup;
  onSelect: (group: KnowledgeGroup) => void;
}) {
  const { t } = useTranslation();
  const handleSelect = () => onSelect(group);
  return (
    <li>
      <button
        type="button"
        className="w-full rounded-lg border border-border-button p-3 text-left hover:bg-bg-card"
        onClick={handleSelect}
      >
        <span className="block break-all font-medium">{group.name}</span>
        <span className="text-xs text-text-secondary">
          {t(`knowledgeCatalogue.${group.kind}`)} ·{' '}
          {t(`knowledgeCatalogue.${group.component_side}`)}
        </span>
      </button>
    </li>
  );
}

function SourceRow({
  source,
  onSelect,
}: {
  source: KnowledgeSource;
  onSelect: (source: KnowledgeSource) => void;
}) {
  const { t } = useTranslation();
  const handleSelect = () => onSelect(source);
  return (
    <li>
      <button
        type="button"
        className="w-full rounded-lg border border-border-button p-3 text-left hover:bg-bg-card"
        onClick={handleSelect}
      >
        <span className="block break-all font-medium">
          {source.relative_path}
        </span>
        <span className="text-xs text-text-secondary">
          {t(`knowledgeCatalogue.${source.component_side}`)} ·{' '}
          {source.language || t('knowledgeCatalogue.unknownLanguage')} ·{' '}
          {source.byte_length} B
        </span>
        <span className="block break-all text-xs text-text-secondary">
          {t('knowledgeCatalogue.revision')}: {source.source_revision_id}
        </span>
      </button>
    </li>
  );
}

function NodeRow({
  node,
  onSelect,
}: {
  node: KnowledgeNode;
  onSelect: (nodeId: string) => void;
}) {
  const { t } = useTranslation();
  const handleSelect = () => onSelect(node.node_id);
  return (
    <li>
      <button
        type="button"
        className="w-full rounded-lg border border-border-button p-3 text-left hover:bg-bg-card"
        onClick={handleSelect}
      >
        <span className="block break-all font-medium">{node.name}</span>
        <span className="text-xs text-text-secondary">
          {t(`knowledgeCatalogue.${node.kind}`)} · {node.start_line}–
          {node.end_line} · [{node.start_byte}, {node.end_byte})
        </span>
      </button>
    </li>
  );
}

function Groups({
  domainId,
  path,
  onSelect,
  onRefresh,
}: {
  domainId: string;
  path: KnowledgeGroup[];
  onSelect: (group: KnowledgeGroup) => void;
  onRefresh: () => void;
}) {
  const query = useKnowledgeGroups(domainId, path.at(-1)?.id);
  const handleMore = () => {
    void query.fetchNextPage();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={onRefresh} />;
  if (query.isPending) return <CatalogueLoading />;
  const items = query.data.pages.flatMap((page) => page.items);
  return (
    <>
      <ul className="space-y-2">
        {items.map((group) => (
          <GroupRow key={group.id} group={group} onSelect={onSelect} />
        ))}
      </ul>
      {!items.length && <CatalogueEmpty label="emptyGroups" />}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={handleMore}
      />
    </>
  );
}

function Sources({
  domainId,
  groupId,
  onSelect,
  onRefresh,
}: {
  domainId: string;
  groupId?: string;
  onSelect: (source: KnowledgeSource) => void;
  onRefresh: () => void;
}) {
  const query = useKnowledgeSources(domainId, groupId);
  const handleMore = () => {
    void query.fetchNextPage();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={onRefresh} />;
  if (query.isPending) return <CatalogueLoading />;
  // No group filter means the whole domain according to C1, not just root files.
  const items = query.data.pages.flatMap((page) => page.items);
  return (
    <>
      <ul className="space-y-2">
        {items.map((source) => (
          <SourceRow
            key={source.source_revision_id}
            source={source}
            onSelect={onSelect}
          />
        ))}
      </ul>
      {!items.length && <CatalogueEmpty label="emptySources" />}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={handleMore}
      />
    </>
  );
}

function Children({
  domainId,
  source,
  node,
  onSelect,
  onRefresh,
}: {
  domainId: string;
  source: KnowledgeSource;
  node: KnowledgeNode;
  onSelect: (nodeId: string) => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  const query = useKnowledgeNodes(
    domainId,
    source.source_revision_id,
    node.node_id,
  );
  const handleMore = () => {
    void query.fetchNextPage();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={onRefresh} />;
  if (query.isPending) return <CatalogueLoading />;
  const items = query.data.pages.flatMap((page) => page.items);
  return (
    <section
      aria-label={t('knowledgeCatalogue.children')}
      className="space-y-2"
    >
      <h3 className="font-medium">{t('knowledgeCatalogue.children')}</h3>
      <ul className="space-y-2">
        {items.map((child) => (
          <NodeRow key={child.node_id} node={child} onSelect={onSelect} />
        ))}
      </ul>
      {!items.length && <CatalogueEmpty label="emptyNodes" />}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={handleMore}
      />
    </section>
  );
}

function NodeDetail({
  domainId,
  source,
  nodeId,
  onSelect,
  onParent,
  onRefresh,
}: {
  domainId: string;
  source: KnowledgeSource;
  nodeId: string;
  onSelect: (nodeId: string) => void;
  onParent: () => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  const query = useKnowledgeNode(domainId, nodeId);
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={onRefresh} />;
  if (query.isPending) return <CatalogueLoading />;
  const node = query.data;
  if (
    node.node_id !== nodeId ||
    node.domain_id !== domainId ||
    node.source_revision_id !== source.source_revision_id
  )
    return (
      <CatalogueFailure
        error={new Error('node identity mismatch')}
        onRetry={onRefresh}
      />
    );
  return (
    <div className="space-y-5">
      <header className="space-y-2">
        <h2 className="break-all text-xl font-semibold">{node.name}</h2>
        <p className="text-xs text-text-secondary">
          {t(`knowledgeCatalogue.${source.component_side}`)} ·{' '}
          {t(`knowledgeCatalogue.${node.kind}`)} · {node.structure_version}
        </p>
        {node.parent_id && (
          <Button variant="outline" onClick={onParent}>
            {t('knowledgeCatalogue.parentNode')}
          </Button>
        )}
      </header>
      <SourceContent
        key={node.node_id}
        domainId={domainId}
        source={source}
        node={node}
        onRefresh={onRefresh}
      />
      <Children
        domainId={domainId}
        source={source}
        node={node}
        onSelect={onSelect}
        onRefresh={onRefresh}
      />
    </div>
  );
}

function NodeBrowser({
  domainId,
  source,
  onRefresh,
}: {
  domainId: string;
  source: KnowledgeSource;
  onRefresh: () => void;
}) {
  const [nodePath, setNodePath] = useState([source.root_node_id]);
  const handleNodeSelect = (nodeId: string) =>
    setNodePath([...nodePath, nodeId]);
  const handleParent = () => setNodePath(nodePath.slice(0, -1));
  return (
    <NodeDetail
      domainId={domainId}
      source={source}
      nodeId={nodePath[nodePath.length - 1]}
      onSelect={handleNodeSelect}
      onParent={handleParent}
      onRefresh={onRefresh}
    />
  );
}

function FixedRevisionBrowser({
  domainId,
  target,
  onRefresh,
}: {
  domainId: string;
  target: KnowledgeFixedTarget;
  onRefresh: () => void;
}) {
  const query = useKnowledgeSource(domainId, target.revisionId);
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={onRefresh} />;
  if (query.isPending) return <CatalogueLoading />;
  const source = query.data;
  if (
    source.domain_id !== domainId ||
    source.source_id !== target.sourceId ||
    source.source_revision_id !== target.revisionId ||
    source.root_node_id !== target.rootNodeId ||
    source.sha256 !== target.sha256
  )
    return (
      <CatalogueFailure
        error={new KnowledgeRequestError(409, 'knowledge_source_integrity')}
        onRetry={onRefresh}
      />
    );
  return (
    <NodeBrowser
      key={source.source_revision_id}
      domainId={domainId}
      source={source}
      onRefresh={onRefresh}
    />
  );
}

function DomainCatalogue({
  domainId,
  target,
}: {
  domainId: string;
  target: KnowledgeFixedTarget | null;
}) {
  const { t } = useTranslation();
  const domain = useKnowledgeDomain(domainId);
  const [path, setPath] = useState<KnowledgeGroup[]>([]);
  const [source, setSource] = useState<KnowledgeSource | null>(null);
  const [generation, setGeneration] = useState(0);
  const [scopeFailure, setScopeFailure] =
    useState<KnowledgeRequestError | null>(null);
  const handleRefresh = () => {
    setSource(null);
    setPath([]);
    setGeneration(generation + 1);
    void domain.refetch().then((result) => {
      if (!result.isError) setScopeFailure(null);
    });
  };
  const handleGroupSelect = (group: KnowledgeGroup) => {
    setPath([...path, group]);
    setSource(null);
  };
  const handleGroupParent = () => {
    setPath(path.slice(0, -1));
    setSource(null);
  };
  const handleDomainRoot = () => {
    setPath([]);
    setSource(null);
  };
  if (scopeFailure)
    return <CatalogueFailure error={scopeFailure} onRetry={handleRefresh} />;
  if (domain.isError)
    return <CatalogueFailure error={domain.error} onRetry={handleRefresh} />;
  if (domain.isPending) return <CatalogueLoading />;
  const policy = domain.data.policy;
  if (
    domain.data.domain_id !== domainId ||
    domain.data.contract_version !== '1.0.0' ||
    policy.access !== 'owner_only' ||
    policy.external_models !== false ||
    policy.automatic_ingestion !== false ||
    policy.allowed_model_uses.length ||
    policy.index_modes.length !== 1 ||
    policy.index_modes[0] !== 'catalogue'
  ) {
    return (
      <CatalogueFailure
        error={new Error('unsupported policy')}
        onRetry={handleRefresh}
      />
    );
  }
  return (
    <CatalogueScopeFailure.Provider value={setScopeFailure}>
      <header className="space-y-3 border-b border-border-button pb-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="break-all text-2xl font-semibold">
            {domain.data.name} · {t('knowledgeCatalogue.title')}
          </h1>
          <Button variant="outline" onClick={handleRefresh}>
            {t('knowledgeCatalogue.refresh')}
          </Button>
          <Link
            className="text-sm underline"
            to={`/dataset/knowledge-import/${domainId}`}
          >
            {t('knowledgeImport.title')}
          </Link>
        </div>
        <p className="text-sm text-text-secondary">
          {t('knowledgeCatalogue.policy')}
        </p>
        <p className="text-xs text-text-secondary">
          {t('knowledgeCatalogue.policyVersion')}: {policy.version} ·{' '}
          {domain.data.category}
        </p>
      </header>
      {target && (
        <p className="pt-3 text-sm text-text-secondary">
          {t('knowledgeImport.pinnedHint')}{' '}
          <Link
            className="underline"
            to={`/dataset/knowledge-catalogue/${domainId}`}
          >
            {t('knowledgeImport.backCatalogue')}
          </Link>
        </p>
      )}
      <div
        key={generation}
        className="grid min-h-0 flex-1 grid-cols-1 gap-6 overflow-auto pt-4 xl:grid-cols-[minmax(16rem,1fr)_minmax(0,2fr)]"
      >
        <nav
          aria-label={t('knowledgeCatalogue.structure')}
          className="min-w-0 space-y-4 overflow-auto"
        >
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" onClick={handleDomainRoot}>
              {t('knowledgeCatalogue.allSources')}
            </Button>
            {!!path.length && (
              <Button variant="outline" onClick={handleGroupParent}>
                {t('knowledgeCatalogue.parentGroup')}
              </Button>
            )}
          </div>
          <p className="break-all text-sm">
            {[domain.data.name, ...path.map((group) => group.name)].join(' / ')}
          </p>
          <h2 className="font-semibold">{t('knowledgeCatalogue.groups')}</h2>
          <Groups
            domainId={domainId}
            path={path}
            onSelect={handleGroupSelect}
            onRefresh={handleRefresh}
          />
          <h2 className="font-semibold">{t('knowledgeCatalogue.sources')}</h2>
          <Sources
            domainId={domainId}
            groupId={path.at(-1)?.id}
            onSelect={setSource}
            onRefresh={handleRefresh}
          />
        </nav>
        <main
          aria-label={t('knowledgeCatalogue.original')}
          className="min-w-0 overflow-auto"
        >
          {target ? (
            <FixedRevisionBrowser
              domainId={domainId}
              target={target}
              onRefresh={handleRefresh}
            />
          ) : source ? (
            <NodeBrowser
              key={source.source_revision_id}
              domainId={domainId}
              source={source}
              onRefresh={handleRefresh}
            />
          ) : (
            <CatalogueEmpty label="selectSource" />
          )}
        </main>
      </div>
    </CatalogueScopeFailure.Provider>
  );
}

export default function KnowledgeCatalogue() {
  const { id = '' } = useParams();
  const [params, setParams] = useSearchParams();
  const { t } = useTranslation();
  const contract = useKnowledgeContract();
  const handleRetry = () => {
    void contract.refetch();
  };
  const handleClearTarget = () => setParams({});
  let target: KnowledgeFixedTarget | null;
  try {
    target = knowledgeFixedTarget(params);
  } catch (error) {
    return <CatalogueFailure error={error} onRetry={handleClearTarget} />;
  }
  if (contract.isError)
    return <CatalogueFailure error={contract.error} onRetry={handleRetry} />;
  if (contract.isPending) return <CatalogueLoading />;
  const capability = contract.data;
  if (
    capability.version !== '1.0.0' ||
    capability.model_calls !== false ||
    capability.ingestion !== false ||
    !capability.contracts.includes('C1') ||
    !capability.contracts.includes('C3') ||
    capability.limits.content_bytes !== 65536
  ) {
    return (
      <CatalogueFailure
        error={new Error('unsupported contract')}
        onRetry={handleRetry}
      />
    );
  }
  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col p-6 text-text-primary">
      <Link
        className="mb-4 text-sm text-text-secondary underline"
        to="/datasets"
      >
        {t('knowledgeCatalogue.datasets')}
      </Link>
      <DomainCatalogue
        key={`${id}:${params.toString()}`}
        domainId={id}
        target={target}
      />
    </section>
  );
}

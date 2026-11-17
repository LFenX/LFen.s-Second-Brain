import { lazy, Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams, useSearchParams } from 'react-router';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
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
import OriginalRevisions from '../knowledge-import/original-revisions';
import WebRevisions from '../knowledge-import/web-revisions';
import LifecycleBrowser, { LifecycleSourceLink } from './lifecycle';
import {
  CatalogueEmpty,
  CatalogueFailure,
  CatalogueLoading,
  CatalogueScopeFailure,
  LoadMore,
} from './state';

const StructureBrowser = lazy(() => import('./structure'));
const DocumentBrowser = lazy(() => import('./document-browser'));
const LocalLexicalBrowser = lazy(() => import('./local-lexical'));
const KnowledgeAnswer = lazy(() => import('./answer'));
const KnowledgeCapabilities = lazy(() => import('./knowledge-capabilities'));
const KnowledgeRepresentation = lazy(
  () => import('./knowledge-representation'),
);

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
  const [params, setParams] = useSearchParams();
  const [path, setPath] = useState<KnowledgeGroup[]>([]);
  const [source, setSource] = useState<KnowledgeSource | null>(null);
  const [explainContent, setExplainContent] = useState(false);
  const [navigationOpen, setNavigationOpen] = useState(false);
  const toggleContentExplanation = () => setExplainContent((value) => !value);
  const [sourceView, setSourceView] = useState<
    | 'text'
    | 'original'
    | 'web'
    | 'lifecycle'
    | 'structure'
    | 'document'
    | 'answer'
    | 'local-lexical'
    | 'content'
  >(
    params.get('knowledge_answer') === '1'
      ? 'answer'
      : params.has('content') ||
          [...params.keys()].some((key) => key.startsWith('content_'))
        ? 'content'
        : params.has('document_structure') ||
            [...params.keys()].some((key) => key.startsWith('document_'))
          ? 'document'
          : params.get('structure') === '1'
            ? 'structure'
            : params.has('lifecycle_source') ||
                params.get('lifecycle_view') === '1'
              ? 'lifecycle'
              : 'text',
  );
  const showAnswer = () => {
    setSourceView('answer');
    setParams({ knowledge_answer: '1' });
  };
  const showContent = () => {
    setSourceView('content');
    setParams({ content: '1' });
  };
  const handleContentSourceSelect = (item: KnowledgeSource) => {
    setParams({ content: '1' });
    setSource(item);
  };
  const clearContentLocation = () => {
    if (
      params.has('knowledge_answer') ||
      params.has('content') ||
      [...params.keys()].some((key) => key.startsWith('content_'))
    )
      setParams({});
  };
  const showText = () => {
    clearContentLocation();
    setSourceView('text');
  };
  const showOriginals = () => {
    clearContentLocation();
    setSourceView('original');
  };
  const showWeb = () => {
    clearContentLocation();
    setSourceView('web');
  };
  const showLocalLexical = () => {
    clearContentLocation();
    setSourceView('local-lexical');
  };
  const showLifecycle = () => {
    setSourceView('lifecycle');
    setParams({ lifecycle_view: '1' });
  };
  const showStructure = () => {
    setSourceView('structure');
    setParams({ structure: '1' });
  };
  const showDocument = () => {
    setSourceView('document');
    setParams({ document_structure: '1' });
  };
  const lifecycleSource = params.get('lifecycle_source');
  useEffect(() => {
    if (lifecycleSource) setSourceView('lifecycle');
  }, [lifecycleSource]);
  useEffect(() => {
    if (
      params.has('content') ||
      [...params.keys()].some((key) => key.startsWith('content_'))
    )
      setSourceView('content');
    else if (
      params.has('document_structure') ||
      [...params.keys()].some((key) => key.startsWith('document_'))
    )
      setSourceView('document');
    else if (params.get('knowledge_answer') === '1') setSourceView('answer');
    else if (params.get('structure') === '1') setSourceView('structure');
  }, [params]);
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
    if (sourceView === 'content') setParams({ content: '1' });
    setSource(null);
  };
  const handleGroupParent = () => {
    setPath(path.slice(0, -1));
    if (sourceView === 'content') setParams({ content: '1' });
    setSource(null);
  };
  const handleDomainRoot = () => {
    if (sourceView === 'content') setParams({ content: '1' });
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
      <header className="shrink-0 space-y-3 border-b border-border-button pb-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="break-all text-lg font-semibold sm:text-2xl">
            {domain.data.name} · {t('knowledgeCatalogue.title')}
          </h1>
          <Button onClick={showAnswer} aria-pressed={sourceView === 'answer'}>
            知识库问答
          </Button>
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
      <div className="flex min-h-0 flex-col gap-3 pt-3">
        <Button
          type="button"
          variant="outline"
          aria-expanded={explainContent}
          className="shrink-0 self-start"
          onClick={toggleContentExplanation}
        >
          {t('knowledgeConsumption.support')}
        </Button>
        {explainContent && (
          <div className="min-h-0 overflow-y-auto">
            <Suspense fallback={<CatalogueLoading />}>
              <KnowledgeCapabilities
                scope={{ owner: domain.data.owner_id, domain: domainId }}
              />
            </Suspense>
          </div>
        )}
      </div>
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
      <Collapsible
        key={generation}
        open={navigationOpen}
        onOpenChange={setNavigationOpen}
        className="flex min-h-0 flex-1 flex-col gap-3 pt-4 xl:grid xl:grid-cols-[minmax(16rem,1fr)_minmax(0,2fr)] xl:gap-6"
      >
        <CollapsibleTrigger asChild>
          <Button variant="outline" className="self-start xl:hidden">
            {t(
              navigationOpen
                ? 'knowledgeDocument.collapse'
                : 'knowledgeDocument.expand',
            )}{' '}
            · {t('knowledgeCatalogue.structure')}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent forceMount asChild>
          <nav
            aria-label={t('knowledgeCatalogue.structure')}
            className="min-h-0 max-h-[40dvh] min-w-0 space-y-4 overflow-auto data-[state=closed]:hidden xl:max-h-none xl:data-[state=closed]:block"
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
              {[domain.data.name, ...path.map((group) => group.name)].join(
                ' / ',
              )}
            </p>
            <h2 className="font-semibold">{t('knowledgeCatalogue.groups')}</h2>
            <Groups
              domainId={domainId}
              path={path}
              onSelect={handleGroupSelect}
              onRefresh={handleRefresh}
            />
            <h2 className="font-semibold">{t('knowledgeCatalogue.sources')}</h2>
            <div className="flex flex-wrap gap-2">
              <Button
                variant={sourceView === 'content' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'content'}
                onClick={showContent}
              >
                {t('knowledgeConsumption.content')}
              </Button>
              <Button
                variant={sourceView === 'text' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'text'}
                onClick={showText}
              >
                {t('knowledgeOriginal.textSources')}
              </Button>
              <Button
                variant={sourceView === 'original' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'original'}
                onClick={showOriginals}
              >
                {t('knowledgeOriginal.originals')}
              </Button>
              <Button
                variant={sourceView === 'web' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'web'}
                onClick={showWeb}
              >
                {t('knowledgeWeb.revisions')}
              </Button>
              <Button
                variant={sourceView === 'lifecycle' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'lifecycle'}
                onClick={showLifecycle}
              >
                {t('knowledgeLifecycle.title')}
              </Button>
              <Button
                variant={sourceView === 'structure' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'structure'}
                onClick={showStructure}
              >
                {t('knowledgeStructure.title')}
              </Button>
              <Button
                variant={sourceView === 'document' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'document'}
                onClick={showDocument}
              >
                {t('knowledgeDocument.title')}
              </Button>
              <Button
                variant={sourceView === 'local-lexical' ? 'default' : 'outline'}
                aria-pressed={sourceView === 'local-lexical'}
                onClick={showLocalLexical}
              >
                {t('knowledgeLocalLexical.title')}
              </Button>
            </div>
            {sourceView === 'content' ? (
              <>
                <p className="text-sm text-text-secondary">
                  {t('knowledgeConsumption.chooseSourceHint')}
                </p>
                <Sources
                  domainId={domainId}
                  groupId={path.at(-1)?.id}
                  onSelect={handleContentSourceSelect}
                  onRefresh={handleRefresh}
                />
              </>
            ) : sourceView === 'local-lexical' ? (
              <p className="text-sm text-text-secondary">
                {t('knowledgeLocalLexical.fixedHint')}
              </p>
            ) : sourceView === 'document' ? (
              <p className="text-sm text-text-secondary">
                {t('knowledgeDocument.chooseOriginal')}
              </p>
            ) : sourceView === 'structure' ? (
              <p className="text-sm text-text-secondary">
                {t('knowledgeStructure.chooseProject')}
              </p>
            ) : sourceView === 'lifecycle' ? (
              <p className="text-sm text-text-secondary">
                {t('knowledgeLifecycle.selectSource')}
              </p>
            ) : sourceView === 'web' ? (
              <WebRevisions
                domain={domainId}
                owner={domain.data.owner_id}
                group={path.at(-1)?.id}
              />
            ) : sourceView === 'original' ? (
              <OriginalRevisions domain={domainId} group={path.at(-1)?.id} />
            ) : (
              <Sources
                domainId={domainId}
                groupId={path.at(-1)?.id}
                onSelect={setSource}
                onRefresh={handleRefresh}
              />
            )}
          </nav>
        </CollapsibleContent>
        <main
          aria-label={t('knowledgeCatalogue.original')}
          className="min-h-0 min-w-0 flex-1 overflow-auto"
        >
          {sourceView === 'answer' ? (
            <Suspense fallback={<CatalogueLoading />}>
              <KnowledgeAnswer
                key={domainId + ':' + domain.data.owner_id}
                domain={domainId}
                owner={domain.data.owner_id}
                group={path.at(-1)?.id}
              />
            </Suspense>
          ) : sourceView === 'content' ? (
            <Suspense fallback={<CatalogueLoading />}>
              <KnowledgeRepresentation
                key={
                  domainId +
                  ':' +
                  domain.data.owner_id +
                  ':' +
                  (path.at(-1)?.id ?? '')
                }
                scope={{ domain: domainId, owner: domain.data.owner_id }}
                source={source}
              />
            </Suspense>
          ) : sourceView === 'local-lexical' ? (
            <Suspense fallback={<CatalogueLoading />}>
              <LocalLexicalBrowser
                domain={domainId}
                owner={domain.data.owner_id}
                group={path.at(-1)?.id}
              />
            </Suspense>
          ) : sourceView === 'document' ? (
            <Suspense fallback={<CatalogueLoading />}>
              <DocumentBrowser domain={domainId} owner={domain.data.owner_id} />
            </Suspense>
          ) : sourceView === 'structure' ? (
            <Suspense fallback={<CatalogueLoading />}>
              <StructureBrowser
                domain={domainId}
                owner={domain.data.owner_id}
              />
            </Suspense>
          ) : sourceView === 'lifecycle' ? (
            <LifecycleBrowser
              domain={domainId}
              owner={domain.data.owner_id}
              group={path.at(-1)?.id}
            />
          ) : target ? (
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
          {sourceView !== 'content' &&
            sourceView !== 'local-lexical' &&
            sourceView !== 'lifecycle' &&
            sourceView !== 'structure' &&
            sourceView !== 'document' &&
            (target || source) && (
              <LifecycleSourceLink
                domain={domainId}
                source={target?.sourceId || source!.source_id}
              />
            )}
        </main>
      </Collapsible>
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
    <section className="flex h-full min-h-0 min-w-0 flex-col p-2 text-text-primary sm:p-6">
      <Link
        className="mb-2 shrink-0 text-sm text-text-secondary underline sm:mb-4"
        to="/datasets"
      >
        {t('knowledgeCatalogue.datasets')}
      </Link>
      <DomainCatalogue
        key={`${id}:${target ? [target.sourceId, target.revisionId, target.rootNodeId, target.sha256].join(':') : 'browse'}`}
        domainId={id}
        target={target}
      />
    </section>
  );
}

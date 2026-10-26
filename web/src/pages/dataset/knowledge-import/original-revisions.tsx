import { useEffect, useState, useContext, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Button } from '@/components/ui/button';
import {
  useOriginalContract,
  useOriginalList,
  useOriginalRevision,
  useOriginalActions,
  originalScopeFailure,
} from '@/hooks/use-knowledge-original';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import service from '@/services/knowledge-original-service';
import { originalRevisionLink } from '@/utils/knowledge-original';
import { OriginalId, OriginalSha } from '@/utils/knowledge-original-schema';
import {
  CatalogueLoading,
  CatalogueScopeFailure,
  LoadMore,
} from '../knowledge-catalogue/state';
import { OriginalFailure } from './original-fields';
import { LifecycleSourceLink } from '../knowledge-catalogue/lifecycle';

export function OriginalRevisionFacts({
  source,
}: {
  source: OriginalRevision;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-sm">
        <dt>{t('knowledgeOriginal.originalName')}</dt>
        <dd className="break-all">{source.original_filename}</dd>
        <dt>{t('knowledgeOriginal.path')}</dt>
        <dd className="break-all">{source.relative_path}</dd>
        <dt>{t('knowledgeOriginal.revision')}</dt>
        <dd className="break-all">{source.source_revision_id}</dd>
        <dt>{t('knowledgeOriginal.bytes')}</dt>
        <dd>{source.byte_length} B</dd>
        <dt>{t('knowledgeOriginal.mediaType')}</dt>
        <dd className="break-all">{source.media_type}</dd>
        <dt>SHA-256</dt>
        <dd className="break-all">{source.sha256}</dd>
        <dt>{t('knowledgeOriginal.group')}</dt>
        <dd className="break-all">
          {source.classification.group_id || t('knowledgeImport.noGroup')}
        </dd>
        <dt>{t('knowledgeOriginal.classification')}</dt>
        <dd>
          {t('knowledgeCatalogue.' + source.classification.component_side)} ·{' '}
          {source.classification.language ||
            t('knowledgeCatalogue.unknownLanguage')}
        </dd>
        <dt>{t('knowledgeOriginal.current')}</dt>
        <dd>
          {t(
            source.is_current
              ? 'knowledgeOriginal.yes'
              : 'knowledgeOriginal.no',
          )}
        </dd>
      </dl>
      <p className="text-sm text-text-secondary">
        {t('knowledgeOriginal.savedOnly')}
      </p>
    </div>
  );
}
function OriginalCards({ domain, group }: { domain: string; group?: string }) {
  const { t } = useTranslation(),
    query = useOriginalList(domain, group),
    onScopeFailure = useContext(CatalogueScopeFailure);
  useEffect(() => {
    if (query.error && originalScopeFailure(query.error))
      onScopeFailure?.(
        query.error as import('@/services/knowledge-foundation-service').KnowledgeRequestError,
      );
  }, [query.error, onScopeFailure]);
  const retry = () => {
    void query.restart();
  };
  const more = () => {
    void query.fetchNextPage();
  };
  if (query.error)
    return <OriginalFailure error={query.error} onRetry={retry} />;
  if (!query.data) return <CatalogueLoading />;
  const items = query.data.pages.flatMap((page) => page.items);
  return (
    <section
      className="space-y-3"
      aria-label={t('knowledgeOriginal.originals')}
    >
      <p className="text-xs text-text-secondary">
        {t('knowledgeOriginal.listHint')}
      </p>
      <Button variant="outline" onClick={retry} disabled={query.isFetching}>
        {t('knowledgeOriginal.refresh')}
      </Button>
      {!items.length && <p>{t('knowledgeOriginal.empty')}</p>}
      <ul className="space-y-2">
        {items.map((source) => (
          <li key={source.source_revision_id}>
            <Link
              className="block space-y-1 rounded-lg border border-border-button p-3 hover:bg-bg-card"
              to={originalRevisionLink(source)}
            >
              <span className="block break-all font-medium">
                {source.original_filename}
              </span>
              <span className="block break-all text-sm">
                {source.relative_path}
              </span>
              <span className="block text-xs text-text-secondary">
                {source.byte_length} B · {source.media_type}
              </span>
              <span className="block break-all text-xs text-text-secondary">
                {source.source_revision_id} · {t('knowledgeOriginal.savedOnly')}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={more}
      />
    </section>
  );
}
export default function OriginalRevisions({
  domain,
  group,
}: {
  domain: string;
  group?: string;
}) {
  const { t } = useTranslation(),
    contract = useOriginalContract();
  const retry = () => {
    void contract.refetch();
  };
  if (contract.error)
    return <OriginalFailure error={contract.error} onRetry={retry} />;
  if (!contract.data) return <CatalogueLoading />;
  if (!contract.data.runtime_ready)
    return <p>{t('knowledgeOriginal.notReady')}</p>;
  return (
    <OriginalCards key={domain + ':' + group} domain={domain} group={group} />
  );
}
export function OriginalRevisionDetail({
  domain,
  revision,
  sourceId,
  sha,
}: {
  domain: string;
  revision: string;
  sourceId: string;
  sha: string;
}) {
  const { t } = useTranslation(),
    query = useOriginalRevision(domain, revision),
    actions = useOriginalActions(domain);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [hidden, setHidden] = useState(false);
  const Alive = useRef(true),
    Controller = useRef<AbortController | null>(null);
  useEffect(() => {
    Alive.current = true;
    return () => {
      Alive.current = false;
      Controller.current?.abort();
    };
  }, []);
  useEffect(() => {
    if (query.error && originalScopeFailure(query.error)) setHidden(true);
  }, [query.error]);
  const retry = () => {
    setError(null);
    void query.refetch().then((result) => {
      if (!result.isError) setHidden(false);
    });
  };
  const download = async () => {
    if (!query.data || busy) return;
    setBusy(true);
    setError(null);
    const controller = new AbortController();
    Controller.current = controller;
    try {
      const blob = await actions.run(() =>
        service.download(query.data!, controller.signal),
      );
      if (!Alive.current) return;
      const url = URL.createObjectURL(blob);
      try {
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = query.data.original_filename;
        anchor.click();
      } finally {
        URL.revokeObjectURL(url);
      }
    } catch (issue) {
      if (Alive.current) {
        setError(issue);
        if (originalScopeFailure(issue)) setHidden(true);
      }
    } finally {
      if (Controller.current === controller) Controller.current = null;
      if (Alive.current) setBusy(false);
    }
  };
  if (query.error || hidden)
    return <OriginalFailure error={query.error || error} onRetry={retry} />;
  if (!query.data) return <CatalogueLoading />;
  const source = query.data;
  if (
    !OriginalId.test(sourceId) ||
    !OriginalSha.test(sha) ||
    source.source_id !== sourceId ||
    source.sha256 !== sha ||
    source.source_revision_id !== revision
  )
    return (
      <OriginalFailure error={new Error('Fixed original identity changed')} />
    );
  return (
    <section
      className="space-y-4"
      aria-label={t('knowledgeOriginal.fixedDetails')}
    >
      <h2 className="text-xl font-semibold">
        {t('knowledgeOriginal.fixedDetails')}
      </h2>
      <OriginalRevisionFacts source={source} />
      <LifecycleSourceLink domain={domain} source={source.source_id} />
      {!!error && <OriginalFailure error={error} />}
      <p className="text-sm text-text-secondary">
        {t('knowledgeOriginal.blobHint')}
      </p>
      <Button onClick={download} disabled={busy || query.isFetching}>
        {t('knowledgeOriginal.download')}
      </Button>
    </section>
  );
}

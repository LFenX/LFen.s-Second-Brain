import {
  useContext,
  useEffect,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { Link, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  useLifecycleContract,
  useLifecycleDetail,
  useLifecycleList,
  useLifecycleActions,
  lifecycleScopeFailure,
} from '@/hooks/use-knowledge-lifecycle';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import service from '@/services/knowledge-lifecycle-service';
import type { LifecycleDetail } from '@/interfaces/database/knowledge-lifecycle';
import type { LifecycleListQuery } from '@/interfaces/request/knowledge-lifecycle';
import {
  lifecycleLink,
  readLifecycleRecovery,
  type LifecycleScope,
} from '@/utils/knowledge-lifecycle';
import { LifecycleId } from '@/utils/knowledge-lifecycle-schema';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { CatalogueLoading, CatalogueScopeFailure, LoadMore } from './state';
import LifecycleActions, { LifecycleFailure } from './lifecycle-actions';
import LifecycleReceiptView from './lifecycle-receipt';
import { useLifecycleFlow } from './use-lifecycle-flow';

function LifecycleFacts({
  detail,
  onScopeFailure,
}: {
  detail: LifecycleDetail;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    [link, setLink] = useState(''),
    [error, setError] = useState<unknown>(null),
    [busy, setBusy] = useState(false),
    action = useLifecycleActions({
      owner: detail.owner_id,
      domain: detail.domain_id,
      source: detail.source_id,
    });
  const read = async () => {
    setBusy(true);
    setError(null);
    try {
      setLink(await action.run(() => service.fixedLink(detail)));
    } catch (issue) {
      setError(issue);
      if (lifecycleScopeFailure(issue)) onScopeFailure(issue);
    } finally {
      setBusy(false);
    }
  };
  return (
    <section
      aria-label={t('knowledgeLifecycle.currentDetails')}
      className="space-y-2"
    >
      <h2 className="text-lg font-semibold">
        {t('knowledgeLifecycle.currentDetails')} ·{' '}
        {t('knowledgeLifecycle.' + detail.current.state)}
      </h2>
      <p className="break-all text-sm">{detail.current.relative_path}</p>
      <p className="break-all text-xs">
        {t('knowledgeLifecycle.source')}: {detail.source_id}
      </p>
      <p className="break-all text-xs">
        {t('knowledgeLifecycle.revision')}: {detail.current.source_revision_id}{' '}
        · {t('knowledgeLifecycle.generation')}:{' '}
        {detail.current.lifecycle_version}
      </p>
      <p className="text-sm">
        {detail.current.classification.group_id || t('knowledgeImport.noGroup')}{' '}
        ·{' '}
        {t(
          'knowledgeCatalogue.' + detail.current.classification.component_side,
        )}{' '}
        ·{' '}
        {detail.current.classification.language ||
          t('knowledgeCatalogue.unknownLanguage')}
      </p>
      <p className="break-all text-xs">
        {t(
          detail.representation_kind === 'web_capture'
            ? 'knowledgeLifecycle.decodedSha'
            : 'knowledgeLifecycle.contentSha',
        )}
        : {detail.current.sha256} · {detail.current.byte_length} B
      </p>
      <Button variant="outline" onClick={read} disabled={busy}>
        {t('knowledgeLifecycle.prepareRead')}
      </Button>
      {link && (
        <Link className="ml-3 underline" to={link}>
          {t('knowledgeImport.readRevision')}
        </Link>
      )}
      {!!error && <LifecycleFailure error={error} />}
    </section>
  );
}
function LifecycleSource({ scope }: { scope: LifecycleScope }) {
  const { t } = useTranslation(),
    query = useLifecycleDetail(scope),
    scopeHandler = useContext(CatalogueScopeFailure),
    [hidden, setHidden] = useState<unknown>(null);
  const onScopeFailure = (issue: unknown) => {
    setHidden(issue);
    if (issue instanceof KnowledgeRequestError) scopeHandler?.(issue);
  };
  const flow = useLifecycleFlow(scope, onScopeFailure);
  useEffect(() => {
    if (query.error && lifecycleScopeFailure(query.error)) {
      setHidden(query.error);
      if (query.error instanceof KnowledgeRequestError)
        scopeHandler?.(query.error);
    }
  }, [query.error, scopeHandler]);
  const retry = () => {
      void query.refetch();
    },
    recover = () => {
      void flow.recover();
    },
    resend = () => {
      void flow.resend();
    },
    abandon = () => {
      void flow.abandon();
    };
  if (query.error || hidden || flow.blocked)
    return <LifecycleFailure error={query.error || hidden || flow.error} />;
  if (!query.data) return <CatalogueLoading />;
  const detail = query.data;
  return (
    <div className="space-y-4">
      {!!flow.error && <LifecycleFailure error={flow.error} />}
      <LifecycleFacts
        key={
          detail.current.source_revision_id +
          ':' +
          detail.current.lifecycle_version
        }
        detail={detail}
        onScopeFailure={onScopeFailure}
      />
      {flow.corrupt ? (
        <p role="alert">{t('knowledgeLifecycle.corruptRecovery')}</p>
      ) : flow.elsewhere ? (
        <p>
          {t('knowledgeLifecycle.pendingElsewhere')}{' '}
          <Link
            className="underline"
            to={lifecycleLink(scope.domain, flow.pending!.source_id)}
          >
            {t('knowledgeLifecycle.openPending')}
          </Link>
        </p>
      ) : (
        <>
          {flow.pending?.source_id === scope.source && (
            <section
              aria-label={t('knowledgeLifecycle.recovery')}
              className="space-y-2 rounded-lg border border-border-button p-3"
            >
              <p>
                {t(
                  flow.unresolved
                    ? 'knowledgeLifecycle.unknown'
                    : 'knowledgeLifecycle.savedOriginal',
                )}
              </p>
              <p className="break-all text-xs">
                {t('knowledgeLifecycle.' + flow.pending.action)} ·{' '}
                {flow.pending.request.idempotency_key}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button onClick={recover} disabled={flow.busy}>
                  {t('knowledgeLifecycle.checkOriginal')}
                </Button>
                {flow.canResend && (
                  <Button
                    variant="outline"
                    onClick={resend}
                    disabled={flow.busy}
                  >
                    {t('knowledgeLifecycle.resend')}
                  </Button>
                )}
                {flow.canAbandon && (
                  <Button
                    variant="outline"
                    onClick={abandon}
                    disabled={flow.busy}
                  >
                    {t('knowledgeLifecycle.abandon')}
                  </Button>
                )}
              </div>
            </section>
          )}
          {flow.receipt && <LifecycleReceiptView receipt={flow.receipt} />}
          <LifecycleActions
            key={[
              detail.current.source_revision_id,
              detail.current.lifecycle_version,
              detail.policy_version,
              flow.consentEpoch,
              query.dataUpdatedAt,
            ].join(':')}
            detail={detail}
            flow={flow}
            onRefresh={retry}
            onScopeFailure={onScopeFailure}
          />
        </>
      )}
    </div>
  );
}
function LifecycleList({
  owner,
  domain,
  group,
}: {
  owner: string;
  domain: string;
  group?: string;
}) {
  const { t } = useTranslation(),
    [params] = useSearchParams();
  const [state, setState] = useState<'active' | 'archived'>('active'),
    [groupScope, setGroupScope] = useState<'all' | 'ungrouped' | 'exact'>(
      group ? 'exact' : 'all',
    ),
    [side, setSide] = useState(''),
    [languageMode, setLanguageMode] = useState<'all' | 'empty' | 'exact'>(
      'all',
    ),
    [language, setLanguage] = useState(''),
    [filters, setFilters] = useState<LifecycleListQuery>({
      state: 'active',
      group_scope: group ? 'exact' : 'all',
      ...(group ? { group_id: group } : {}),
    });
  const query = useLifecycleList(owner, domain, filters),
    scopeHandler = useContext(CatalogueScopeFailure);
  useEffect(() => {
    if (
      query.error &&
      lifecycleScopeFailure(query.error) &&
      query.error instanceof KnowledgeRequestError
    )
      scopeHandler?.(query.error);
  }, [query.error, scopeHandler]);
  const changeState = (event: ChangeEvent<HTMLSelectElement>) =>
      setState(event.target.value as typeof state),
    changeGroup = (event: ChangeEvent<HTMLSelectElement>) =>
      setGroupScope(event.target.value as typeof groupScope),
    changeSide = (event: ChangeEvent<HTMLSelectElement>) =>
      setSide(event.target.value),
    changeMode = (event: ChangeEvent<HTMLSelectElement>) =>
      setLanguageMode(event.target.value as typeof languageMode),
    changeLanguage = (event: ChangeEvent<HTMLInputElement>) =>
      setLanguage(event.target.value),
    apply = (event: FormEvent) => {
      event.preventDefault();
      setFilters({
        state,
        group_scope: groupScope,
        ...(groupScope === 'exact' ? { group_id: group } : {}),
        ...(side
          ? { component_side: side as LifecycleListQuery['component_side'] }
          : {}),
        ...(languageMode === 'all'
          ? {}
          : { language: languageMode === 'empty' ? '' : language }),
        limit: 20,
      });
    },
    more = () => {
      void query.fetchNextPage();
    },
    retry = () => {
      void query.refetch();
    };
  const selected = params.get('lifecycle_source');
  return (
    <div className="space-y-5">
      <form
        onSubmit={apply}
        className="space-y-3 rounded-lg border border-border-button p-3"
        aria-label={t('knowledgeLifecycle.filters')}
      >
        <label className="block">
          {t('knowledgeLifecycle.state')}
          <select
            className="block w-full rounded border border-border-button bg-bg-base p-2"
            value={state}
            onChange={changeState}
          >
            <option value="active">{t('knowledgeLifecycle.active')}</option>
            <option value="archived">{t('knowledgeLifecycle.archived')}</option>
          </select>
        </label>
        <label className="block">
          {t('knowledgeLifecycle.groupScope')}
          <select
            className="block w-full rounded border border-border-button bg-bg-base p-2"
            value={groupScope}
            onChange={changeGroup}
          >
            <option value="all">{t('knowledgeLifecycle.all')}</option>
            <option value="ungrouped">{t('knowledgeImport.noGroup')}</option>
            <option value="exact" disabled={!group}>
              {t('knowledgeLifecycle.exactGroup')}
            </option>
          </select>
        </label>
        <label className="block">
          {t('knowledgeImport.componentSide')}
          <select
            className="block w-full rounded border border-border-button bg-bg-base p-2"
            value={side}
            onChange={changeSide}
          >
            <option value="">{t('knowledgeLifecycle.all')}</option>
            {['frontend', 'backend', 'shared', 'cli', 'non_code'].map(
              (value) => (
                <option key={value} value={value}>
                  {t('knowledgeCatalogue.' + value)}
                </option>
              ),
            )}
          </select>
        </label>
        <label className="block">
          {t('knowledgeLifecycle.languageFilter')}
          <select
            className="block w-full rounded border border-border-button bg-bg-base p-2"
            value={languageMode}
            onChange={changeMode}
          >
            <option value="all">{t('knowledgeLifecycle.all')}</option>
            <option value="empty">
              {t('knowledgeCatalogue.unknownLanguage')}
            </option>
            <option value="exact">
              {t('knowledgeLifecycle.exactLanguage')}
            </option>
          </select>
        </label>
        {languageMode === 'exact' && (
          <Input
            aria-label={t('knowledgeLifecycle.language')}
            value={language}
            onChange={changeLanguage}
            maxLength={32}
          />
        )}
        <Button type="submit">{t('knowledgeLifecycle.applyFilters')}</Button>
      </form>
      {query.error ? (
        <LifecycleFailure error={query.error} onRetry={retry} />
      ) : query.isPending ? (
        <CatalogueLoading />
      ) : (
        <>
          <ul className="space-y-2">
            {query.data.pages
              .flatMap((page) => page.items)
              .map((item) => (
                <li
                  key={item.source_id}
                  className="rounded-lg border border-border-button p-3"
                >
                  <Link
                    to={lifecycleLink(domain, item.source_id)}
                    className="break-all underline"
                  >
                    {item.current.relative_path}
                  </Link>
                  <p className="text-xs">
                    {t('knowledgeLifecycle.' + item.current.state)} ·{' '}
                    {item.current.lifecycle_version} ·{' '}
                    {item.representation_kind}
                  </p>
                </li>
              ))}
          </ul>
          <LoadMore
            more={query.hasNextPage}
            loading={query.isFetching}
            onMore={more}
          />
        </>
      )}
      {selected ? (
        LifecycleId.test(selected) ? (
          <LifecycleSource
            key={
              getKnowledgeSessionScope() +
              ':' +
              owner +
              ':' +
              domain +
              ':' +
              selected
            }
            scope={{ owner, domain, source: selected }}
          />
        ) : (
          <LifecycleFailure error={Error('Invalid source ID')} />
        )
      ) : (
        <p>{t('knowledgeLifecycle.selectSource')}</p>
      )}
    </div>
  );
}
export function LifecycleSourceLink({
  domain,
  source,
}: {
  domain: string;
  source: string;
}) {
  const { t } = useTranslation();
  return (
    <Link
      className="my-3 inline-block text-sm underline"
      to={lifecycleLink(domain, source)}
    >
      {t('knowledgeLifecycle.openSource')}
    </Link>
  );
}
export default function LifecycleBrowser({
  domain,
  owner,
  group,
}: {
  domain: string;
  owner: string;
  group?: string;
}) {
  const { t } = useTranslation(),
    query = useLifecycleContract(),
    retry = () => {
      void query.refetch();
    },
    [recovery] = useState(() => {
      try {
        return {
          slot: readLifecycleRecovery(owner, domain),
          error: null as unknown,
        };
      } catch (error) {
        return { slot: null, error };
      }
    });
  const scopeHandler = useContext(CatalogueScopeFailure);
  useEffect(() => {
    if (
      query.error &&
      lifecycleScopeFailure(query.error) &&
      query.error instanceof KnowledgeRequestError
    )
      scopeHandler?.(query.error);
  }, [query.error, scopeHandler]);
  if (query.error)
    return <LifecycleFailure error={query.error} onRetry={retry} />;
  if (!query.data) return <CatalogueLoading />;
  if (!query.data.runtime_ready)
    return <p>{t('knowledgeLifecycle.notReady')}</p>;
  if (recovery.error)
    return <p role="alert">{t('knowledgeLifecycle.corruptRecovery')}</p>;
  return (
    <section aria-label={t('knowledgeLifecycle.title')} className="space-y-3">
      <h2 className="text-xl font-semibold">{t('knowledgeLifecycle.title')}</h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeLifecycle.scope')}
      </p>
      {recovery.slot?.command_id === null && (
        <p>
          {t('knowledgeLifecycle.unknown')}{' '}
          <LifecycleSourceLink
            domain={domain}
            source={recovery.slot.source_id}
          />
        </p>
      )}
      <LifecycleList
        key={owner + ':' + domain + ':' + (group || '')}
        owner={owner}
        domain={domain}
        group={group}
      />
    </section>
  );
}

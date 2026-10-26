import {
  useState,
  useEffect,
  useContext,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  useWebRevisions,
  useWebRevision,
  webScopeFailure,
} from '@/hooks/use-knowledge-web';
import { WebId, WebSha } from '@/utils/knowledge-web-schema';
import { webRevisionLink } from '@/utils/knowledge-web';
import {
  CatalogueLoading,
  CatalogueScopeFailure,
  LoadMore,
} from '../knowledge-catalogue/state';
import WebReading from './web-reading';
import { WebFailure } from './web-fields';

export default function WebRevisions({
  domain,
  owner,
  group,
}: {
  domain: string;
  owner: string;
  group?: string;
}) {
  const { t } = useTranslation(),
    [filter, setFilter] = useState(''),
    [registration, setRegistration] = useState<string | undefined>();
  const query = useWebRevisions(domain, owner, group, registration);
  const scopeFailure = useContext(CatalogueScopeFailure);
  useEffect(() => {
    if (query.error && webScopeFailure(query.error))
      scopeFailure?.(
        query.error as import('@/services/knowledge-foundation-service').KnowledgeRequestError,
      );
  }, [query.error, scopeFailure]);
  const change = (event: ChangeEvent<HTMLInputElement>) =>
      setFilter(event.target.value),
    submit = (event: FormEvent) => {
      event.preventDefault();
      setRegistration(filter || undefined);
    },
    more = () => {
      void query.fetchNextPage();
    },
    retry = () => {
      void query.refetch();
    };
  return (
    <section aria-label={t('knowledgeWeb.revisions')} className="space-y-3">
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.notIndexed')}
      </p>
      <form onSubmit={submit} className="space-y-2">
        <label>
          {t('knowledgeWeb.registrationFilter')}
          <Input value={filter} onChange={change} pattern="[a-f0-9]{32}" />
        </label>
        <Button type="submit" variant="outline">
          {t('knowledgeWeb.filter')}
        </Button>
      </form>
      {query.error ? (
        <WebFailure error={query.error} onRetry={retry} />
      ) : query.isPending ? (
        <CatalogueLoading />
      ) : (
        <>
          <ul className="space-y-2">
            {query.data.pages
              .flatMap((page) => page.items)
              .map((item) => (
                <li
                  key={item.source_revision_id}
                  className="rounded-lg border border-border-button p-3"
                >
                  <Link
                    className="break-all underline"
                    to={webRevisionLink(domain, item)}
                  >
                    {item.metadata.title || item.relative_path}
                  </Link>
                  <p className="text-xs text-text-secondary">
                    {t(
                      'knowledgeWeb.completeness.' +
                        item.metadata.body_completeness,
                    )}{' '}
                    ·{' '}
                    {item.is_current
                      ? t('knowledgeWeb.current')
                      : t('knowledgeWeb.historical')}
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
    </section>
  );
}
export function WebRevisionDetail({
  domain,
  owner,
  revision,
  source,
  inputSha,
  onScopeFailure,
}: {
  domain: string;
  owner: string;
  revision: string;
  source: string;
  inputSha: string;
  onScopeFailure: (error: unknown) => void;
}) {
  const query = useWebRevision(domain, owner, revision),
    { t } = useTranslation();
  useEffect(() => {
    if (query.error && webScopeFailure(query.error))
      onScopeFailure(query.error);
  }, [query.error, onScopeFailure]);
  const retry = () => {
    void query.refetch();
  };
  if (!WebId.test(revision) || !WebId.test(source) || !WebSha.test(inputSha))
    return <WebFailure error={Error('Invalid fixed target')} />;
  if (query.error) return <WebFailure error={query.error} onRetry={retry} />;
  if (!query.data) return <CatalogueLoading />;
  if (
    query.data.source_id !== source ||
    query.data.source_input_sha256 !== inputSha
  )
    return <WebFailure error={Error('Fixed revision mismatch')} />;
  return (
    <div className="space-y-3">
      <h2>{t('knowledgeWeb.fixedRevision')}</h2>
      <p className="break-all text-xs">{query.data.source_revision_id}</p>
      <WebReading
        key={revision + ':' + inputSha}
        target={{ revision: query.data }}
        onScopeFailure={onScopeFailure}
      />
    </div>
  );
}

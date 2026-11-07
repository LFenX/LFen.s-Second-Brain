import {
  useLayoutEffect,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  getKnowledgeSessionScope,
  useKnowledgeDomains,
} from '@/hooks/use-knowledge-foundation';
import {
  useKnowledgeDomainEntry,
  useKnowledgeEntryDatasets,
} from '@/hooks/use-knowledge-foundation-entry';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import type { KnowledgeDomain } from '@/interfaces/database/knowledge-foundation';
import type { IDataset } from '@/interfaces/database/dataset';

type DomainFlow = ReturnType<typeof useKnowledgeDomainEntry>;
const Categories: KnowledgeDomain['category'][] = [
  'family',
  'personal',
  'project',
  'experience',
  'resource',
  'custom',
];

function ExistingDatasetChoice({
  dataset,
  disabled,
  onSelect,
}: {
  dataset: IDataset;
  disabled: boolean;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  const handleSelect = () => onSelect(dataset.id);
  return (
    <li className="space-y-1 rounded-md border border-border-button p-2">
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        onClick={handleSelect}
      >
        {dataset.name} · {dataset.id.slice(-8)}
      </Button>
      <details className="text-xs text-text-secondary">
        <summary>{t('knowledgeDomain.identity')}</summary>
        <code>{dataset.id}</code>
      </details>
    </li>
  );
}

function ExistingDatasets({
  ownerId,
  flow,
  category,
}: {
  ownerId: string;
  flow: DomainFlow;
  category: KnowledgeDomain['category'];
}) {
  const { t } = useTranslation();
  const query = useKnowledgeEntryDatasets(ownerId);
  const handleRefresh = () => {
    void query.refetch();
  };
  const handleMore = () => {
    void query.fetchNextPage();
  };
  const handleSelect = (id: string) => {
    void flow.registerExisting(id, category);
  };
  const choices =
    query.data?.pages
      .flatMap((page) => page.items)
      .filter(
        (dataset) =>
          dataset.created_by === ownerId &&
          dataset.permission === 'me' &&
          dataset.document_count === 0 &&
          dataset.chunk_count === 0 &&
          dataset.token_num === 0,
      ) || [];
  return (
    <section
      className="space-y-2"
      aria-label={t('knowledgeDomain.existingDataset')}
    >
      <h3>{t('knowledgeDomain.existingDataset')}</h3>
      <p className="text-sm text-text-secondary">
        {t('knowledgeDomain.existingHint')}
      </p>
      {query.isError && <p role="alert">{t('knowledgeDomain.readFailure')}</p>}
      {query.isPending && <p role="status">{t('knowledgeDomain.loading')}</p>}
      <ul className="space-y-2">
        {choices.map((dataset) => (
          <ExistingDatasetChoice
            key={dataset.id}
            dataset={dataset}
            disabled={flow.busy || !!flow.attempt?.datasetId || !!flow.receipt}
            onSelect={handleSelect}
          />
        ))}
      </ul>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={flow.busy || query.isFetching}
          onClick={handleRefresh}
        >
          {t('knowledgeDomain.refresh')}
        </Button>
        {query.hasNextPage && (
          <Button
            type="button"
            variant="outline"
            disabled={flow.busy || query.isFetching}
            onClick={handleMore}
          >
            {t('knowledgeDomain.more')}
          </Button>
        )}
      </div>
    </section>
  );
}

function RegisteredDomains({ flow }: { flow: DomainFlow }) {
  const { t } = useTranslation();
  const query = useKnowledgeDomains();
  const handleChoose = (event: ChangeEvent<HTMLSelectElement>) => {
    if (event.target.value) void flow.selectDomain(event.target.value);
  };
  const handleMore = () => {
    void query.fetchNextPage();
  };
  const handleRefresh = () => {
    void query.refetch();
  };
  return (
    <section className="space-y-2">
      <label className="block space-y-1">
        <span>{t('knowledgeDomain.select')}</span>
        <select
          value={flow.receipt?.domain_id || ''}
          onChange={handleChoose}
          disabled={flow.isLocked() || query.isPending}
          className="block w-full rounded-md border border-border-button bg-bg-base p-2"
        >
          <option value="">{t('knowledgeDomain.select')}</option>
          {query.data?.pages
            .flatMap((page) => page.items)
            .map((domain) => (
              <option key={domain.domain_id} value={domain.domain_id}>
                {domain.name} · {domain.domain_id.slice(-8)}
              </option>
            ))}
        </select>
      </label>
      {query.isError && <p role="alert">{t('knowledgeDomain.readFailure')}</p>}
      <Button
        type="button"
        variant="outline"
        disabled={flow.busy || query.isFetching}
        onClick={handleRefresh}
      >
        {t('knowledgeDomain.refresh')}
      </Button>
      {query.hasNextPage && (
        <Button
          type="button"
          variant="outline"
          disabled={flow.busy || query.isFetching}
          onClick={handleMore}
        >
          {t('knowledgeDomain.more')}
        </Button>
      )}
    </section>
  );
}

function DomainDialog({
  open,
  onOpenChange,
  refreshNative,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  refreshNative: () => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const { data: user, loading: userLoading } = useFetchUserInfo();
  const ownerId = user?.id || '';
  const flow = useKnowledgeDomainEntry(ownerId, refreshNative);
  const [name, setName] = useState('');
  const [category, setCategory] =
    useState<KnowledgeDomain['category']>('project');
  const epoch = getKnowledgeSessionScope();
  useLayoutEffect(() => {
    setName('');
    setCategory('project');
  }, [ownerId, epoch]);
  const changeOpen = (next: boolean) => {
    if (next || !flow.isLocked()) {
      if (!next && flow.receipt) {
        void flow.clearCompleted();
        setName('');
        setCategory('project');
      }
      onOpenChange(next);
    }
  };
  const handleName = (event: ChangeEvent<HTMLInputElement>) =>
    setName(event.target.value);
  const handleCategory = (event: ChangeEvent<HTMLSelectElement>) =>
    setCategory(event.target.value as KnowledgeDomain['category']);
  const handleCreate = (event: FormEvent) => {
    event.preventDefault();
    void flow.create(name, category);
  };
  const handleCheck = () => {
    void flow.check();
  };
  const handleRetry = () => {
    void flow.retryRegistration();
  };
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="max-h-[90dvh] max-w-xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('knowledgeDomain.entry')}</DialogTitle>
          <DialogDescription>
            {t('knowledgeDomain.policyHint')}
          </DialogDescription>
        </DialogHeader>
        {!ownerId || userLoading ? (
          <p role="status">{t('knowledgeDomain.identityPending')}</p>
        ) : (
          <>
            <RegisteredDomains flow={flow} />
            {!flow.receipt && (
              <>
                <form onSubmit={handleCreate} className="space-y-3">
                  <fieldset disabled={flow.locked} className="space-y-3">
                    <label className="block space-y-1">
                      <span>{t('knowledgeDomain.name')}</span>
                      <Input required value={name} onChange={handleName} />
                    </label>
                    <label className="block space-y-1">
                      <span>{t('knowledgeDomain.category')}</span>
                      <select
                        value={category}
                        onChange={handleCategory}
                        className="block w-full rounded-md border border-border-button bg-bg-base p-2"
                      >
                        {Categories.map((value) => (
                          <option key={value} value={value}>
                            {t(`knowledgeDomain.category_${value}`)}
                          </option>
                        ))}
                      </select>
                    </label>
                    <p className="text-xs text-text-secondary">
                      {t('knowledgeDomain.nameHint')}
                    </p>
                    <Button type="submit">{t('knowledgeDomain.create')}</Button>
                  </fieldset>
                </form>
                <ExistingDatasets
                  ownerId={ownerId}
                  flow={flow}
                  category={category}
                />
              </>
            )}
            {flow.busy && <p role="status">{t('knowledgeDomain.working')}</p>}
            {flow.attempt && !flow.receipt && (
              <section className="space-y-2">
                <p>
                  {t(
                    flow.attempt.datasetId
                      ? 'knowledgeDomain.nativeCreated'
                      : 'knowledgeDomain.nativeUnknown',
                  )}
                </p>
                {flow.attempt.datasetId && (
                  <details className="text-xs text-text-secondary">
                    <summary>{t('knowledgeDomain.identity')}</summary>
                    <code>{flow.attempt.datasetId}</code>
                  </details>
                )}
                <Button
                  type="button"
                  variant="outline"
                  disabled={flow.busy}
                  onClick={handleCheck}
                >
                  {t('knowledgeDomain.check')}
                </Button>
                {flow.canRetry && (
                  <Button
                    type="button"
                    disabled={flow.busy}
                    onClick={handleRetry}
                  >
                    {t('knowledgeDomain.retryRegistration')}
                  </Button>
                )}
              </section>
            )}
            {flow.issue && (
              <div role="alert" className="space-y-1">
                <p>
                  {t(
                    flow.issue.kind === 'unknown'
                      ? 'knowledgeDomain.unknown'
                      : 'knowledgeDomain.rejected',
                  )}
                </p>
                <details className="text-xs text-text-secondary">
                  <summary>{t('knowledgeDomain.details')}</summary>
                  <p>
                    {flow.issue.code} {flow.issue.requestId}
                  </p>
                </details>
              </div>
            )}
            {flow.receipt && (
              <section
                className="space-y-2"
                aria-label={t('knowledgeDomain.ready')}
              >
                <p role="status">
                  {t('knowledgeDomain.ready')}: {flow.receipt.name}
                </p>
                <nav className="flex flex-wrap gap-3">
                  <Link
                    to={`/dataset/knowledge-import/${flow.receipt.domain_id}?mode=original`}
                  >
                    {t('knowledgeDomain.original')}
                  </Link>
                  <Link
                    to={`/dataset/knowledge-import/${flow.receipt.domain_id}?mode=directory`}
                  >
                    {t('knowledgeDomain.directory')}
                  </Link>
                  <Link
                    to={`/dataset/knowledge-catalogue/${flow.receipt.domain_id}`}
                  >
                    {t('knowledgeDomain.catalogue')}
                  </Link>
                </nav>
              </section>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

export function KnowledgeDomainEntry({
  refreshNative,
}: {
  refreshNative: () => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const [visited, setVisited] = useState(false);
  const [open, setOpen] = useState(false);
  const handleOpen = () => {
    setVisited(true);
    setOpen(true);
  };
  return (
    <>
      <Button type="button" variant="outline" onClick={handleOpen}>
        {t('knowledgeDomain.entry')}
      </Button>
      {visited && (
        <DomainDialog
          open={open}
          onOpenChange={setOpen}
          refreshNative={refreshNative}
        />
      )}
    </>
  );
}

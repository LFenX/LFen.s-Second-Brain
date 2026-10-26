import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useKnowledgeGroups } from '@/hooks/use-knowledge-foundation';
import {
  useBatchHead,
  useBatchHistory,
  useBatchManifest,
  batchScopeFailure,
} from '@/hooks/use-knowledge-batch';
import type { KnowledgeGroup } from '@/interfaces/database/knowledge-foundation';
import type {
  BatchManifestEntry,
  BatchManifestHistoryPage,
  BatchManifestVersionPage,
} from '@/interfaces/database/knowledge-batch';
import {
  appendBatchHistoryPage,
  appendBatchManifestPage,
} from '@/utils/knowledge-batch';
import { knowledgeReceiptLink } from '@/utils/knowledge-source-link';
import { CatalogueLoading, LoadMore } from '../knowledge-catalogue/state';
import { BatchFailure } from './batch-fields';
import { useBatchManifestFlow, type BatchLocation } from './use-batch-flow';

function ProjectChoice({
  group,
  onChoose,
  onBrowse,
}: {
  group: KnowledgeGroup;
  onChoose: (id: string) => void;
  onBrowse: (group: KnowledgeGroup) => void;
}) {
  const { t } = useTranslation(),
    choose = () => onChoose(group.id),
    browse = () => onBrowse(group);
  return (
    <li className="flex flex-wrap gap-2">
      {group.kind === 'project' && (
        <Button type="button" variant="outline" onClick={choose}>
          {group.name}
        </Button>
      )}
      <Button type="button" variant="ghost" onClick={browse}>
        {group.name} · {t('knowledgeImport.childGroups')}
      </Button>
    </li>
  );
}
export function ProjectPicker({
  domain,
  onChoose,
}: {
  domain: string;
  onChoose: (id: string) => void;
}) {
  const { t } = useTranslation(),
    [path, setPath] = useState<KnowledgeGroup[]>([]),
    query = useKnowledgeGroups(domain, path.at(-1)?.id);
  const browse = (group: KnowledgeGroup) =>
      setPath((current) => [...current, group]),
    parent = () => setPath((current) => current.slice(0, -1));
  const more = () => {
      void query.fetchNextPage();
    },
    reload = () => {
      void query.refetch();
    };
  return (
    <details className="rounded-md border border-border-button p-3">
      <summary>{t('knowledgeBatch.chooseProject')}</summary>
      <p>
        {path.map((g) => g.name).join(' / ') || t('knowledgeImport.rootGroups')}
      </p>
      {!!path.length && (
        <Button type="button" variant="ghost" onClick={parent}>
          {t('knowledgeCatalogue.parentGroup')}
        </Button>
      )}
      {query.error ? (
        <BatchFailure error={query.error} onReload={reload} />
      ) : !query.data ? (
        <CatalogueLoading />
      ) : (
        <>
          <ul className="space-y-2">
            {query.data.pages
              .flatMap((p) => p.items)
              .map((g) => (
                <ProjectChoice
                  key={g.id}
                  group={g}
                  onChoose={onChoose}
                  onBrowse={browse}
                />
              ))}
          </ul>
          <LoadMore
            more={query.hasNextPage}
            loading={query.isFetchingNextPage}
            onMore={more}
          />
        </>
      )}
    </details>
  );
}
export function ManifestEntryRow({
  domain,
  item,
}: {
  domain: string;
  item: BatchManifestEntry;
}) {
  const { t } = useTranslation();
  return (
    <li className="space-y-1 rounded-md border border-border-button p-3">
      <p className="break-all">
        {item.relative_path} · {item.registration_id}
      </p>
      <p>
        {t('knowledgeCatalogue.' + item.classification.component_side)} ·{' '}
        {item.classification.language} · {item.byte_length} B
      </p>
      <p className="break-all">
        {t('knowledgeCatalogue.revision')}: {item.source_revision_id}
      </p>
      <Link className="underline" to={knowledgeReceiptLink(domain, item)}>
        {t('knowledgeImport.readRevision')}
      </Link>
    </li>
  );
}
function FixedManifest({
  domain,
  project,
  id,
  onFailure,
}: {
  domain: string;
  project: string;
  id: string;
  onFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    query = useBatchManifest(domain, project, id);
  useEffect(() => {
    if (query.error && batchScopeFailure(query.error)) onFailure(query.error);
  }, [query.error, onFailure]);
  const more = () => {
      void query.fetchNextPage();
    },
    reload = () => {
      void query.restart();
    };
  if (query.error)
    return <BatchFailure error={query.error} onReload={reload} />;
  if (!query.data) return <CatalogueLoading />;
  let pages: BatchManifestVersionPage[] = [];
  try {
    for (const page of query.data.pages)
      pages = appendBatchManifestPage(pages, page);
  } catch (error) {
    return <BatchFailure error={error} onReload={reload} />;
  }
  return (
    <section
      className="space-y-3"
      aria-label={t('knowledgeBatch.fixedManifest')}
    >
      <h3 className="break-all">
        {t('knowledgeBatch.fixedManifest')}: {id}
      </h3>
      <p>
        {t('knowledgeBatch.explicitCoverage')} ·{' '}
        {pages.flatMap((p) => p.items).length}/{pages[0].total_entries}
      </p>
      <ul className="space-y-2">
        {pages
          .flatMap((p) => p.items)
          .map((i) => (
            <ManifestEntryRow key={i.source_id} domain={domain} item={i} />
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
function HistoryRow({
  id,
  published,
  onSelect,
}: {
  id: string;
  published: string;
  onSelect: (id: string) => void;
}) {
  const select = () => onSelect(id);
  return (
    <li>
      <Button
        variant="outline"
        onClick={select}
        className="h-auto whitespace-normal break-all text-left"
      >
        {id} · {dayjs(published).format('YYYY-MM-DD HH:mm:ss')}
      </Button>
    </li>
  );
}
function ProjectRead({
  domain,
  project,
  manifestId,
  onSelect,
  onFailure,
}: {
  domain: string;
  project: string;
  manifestId: string;
  onSelect: (id: string) => void;
  onFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    head = useBatchHead(domain, project),
    history = useBatchHistory(domain, project);
  useEffect(() => {
    const error = head.error || history.error;
    if (error && batchScopeFailure(error)) onFailure(error);
  }, [head.error, history.error, onFailure]);
  const more = () => {
      void history.fetchNextPage();
    },
    reload = () => {
      void head.refetch();
      void history.restart();
    };
  if (head.error || history.error)
    return (
      <BatchFailure error={head.error || history.error} onReload={reload} />
    );
  if (!head.data || !history.data) return <CatalogueLoading />;
  let pages: BatchManifestHistoryPage[] = [];
  try {
    for (const page of history.data.pages)
      pages = appendBatchHistoryPage(pages, page);
  } catch (error) {
    return <BatchFailure error={error} onReload={reload} />;
  }
  const fixed = manifestId || head.data.manifest_id || '';
  return (
    <div className="space-y-4">
      <p className="break-all">
        {t('knowledgeBatch.currentHead')}:{' '}
        {head.data.manifest_id || t('knowledgeBatch.noManifest')}
      </p>
      <Button variant="outline" onClick={reload}>
        {t('knowledgeCatalogue.refresh')}
      </Button>
      <section aria-label={t('knowledgeBatch.history')} className="space-y-2">
        <h3>{t('knowledgeBatch.history')}</h3>
        <p>
          {pages.flatMap((p) => p.items).length}/{pages[0].total_manifests}
        </p>
        <ul className="space-y-2">
          {pages
            .flatMap((p) => p.items)
            .map((i) => (
              <HistoryRow
                key={i.manifest_id}
                id={i.manifest_id}
                published={i.published_at}
                onSelect={onSelect}
              />
            ))}
        </ul>
        <LoadMore
          more={history.hasNextPage}
          loading={history.isFetchingNextPage}
          onMore={more}
        />
      </section>
      {fixed && (
        <FixedManifest
          key={project + ':' + fixed}
          domain={domain}
          project={project}
          id={fixed}
          onFailure={onFailure}
        />
      )}
    </div>
  );
}
export default function BatchManifest({
  domain,
  storageKey,
  project,
  manifestId,
  initialTask,
  onLocation,
  onAuthorizationFailure,
}: {
  domain: string;
  storageKey: string;
  project: string;
  manifestId: string;
  initialTask: { registration: string; task: string } | null;
  onLocation: (value: BatchLocation) => void;
  onAuthorizationFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    flow = useBatchManifestFlow(
      domain,
      storageKey,
      onLocation,
      onAuthorizationFailure,
    );
  const [projectInput, setProjectInput] = useState(
      project || flow.pending?.request.project_id || '',
    ),
    [reg, setReg] = useState(initialTask?.registration || ''),
    [task, setTask] = useState(initialTask?.task || '');
  const [publicationConsent, setPublicationConsent] = useState(false),
    [resendConsent, setResendConsent] = useState(false),
    [readFailure, setReadFailure] = useState<unknown>(null),
    [time, setTime] = useState(dayjs());
  const currentProject = project || flow.pending?.request.project_id || '',
    first = flow.pages[0],
    initialRegistration = initialTask?.registration,
    initialTaskId = initialTask?.task,
    previewId = first?.preview_id;
  useEffect(() => {
    setProjectInput(project || flow.pending?.request.project_id || '');
    setReadFailure(null);
  }, [project, flow.pending?.request.project_id]);
  useEffect(() => {
    if (initialRegistration && initialTaskId) {
      setReg(initialRegistration);
      setTask(initialTaskId);
    }
  }, [initialRegistration, initialTaskId]);
  useEffect(() => {
    setPublicationConsent(false);
  }, [first?.preview_id]);
  useEffect(() => {
    if (!previewId) return;
    const timer = globalThis.setInterval(() => setTime(dayjs()), 1000);
    return () => globalThis.clearInterval(timer);
  }, [previewId]);
  const chooseProject = (id: string) => {
    flow.invalidate();
    setReadFailure(null);
    onLocation({ project: id, manifest: null, batch_tab: 'projects' });
  };
  const selectManifest = (id: string) =>
    onLocation({
      project: currentProject,
      manifest: id,
      batch_tab: 'projects',
    });
  const handleProject = (e: ChangeEvent<HTMLInputElement>) =>
      setProjectInput(e.target.value),
    handleReg = (e: ChangeEvent<HTMLInputElement>) => {
      flow.invalidate();
      setReg(e.target.value);
    },
    handleTask = (e: ChangeEvent<HTMLInputElement>) => {
      flow.invalidate();
      setTask(e.target.value);
    };
  const submitProject = (e: FormEvent) => {
    e.preventDefault();
    chooseProject(projectInput);
  };
  const prepare = (e: FormEvent) => {
    e.preventDefault();
    setPublicationConsent(false);
    void flow.prepare(currentProject, reg, task);
  };
  const approve = (e: ChangeEvent<HTMLInputElement>) =>
      setPublicationConsent(e.target.checked),
    approveResend = (e: ChangeEvent<HTMLInputElement>) =>
      setResendConsent(e.target.checked);
  const publish = () => {
    if (publicationConsent) {
      setPublicationConsent(false);
      void flow.publish();
    }
  };
  const recover = () => {
      setResendConsent(false);
      void flow.recover();
    },
    resend = () => {
      if (resendConsent) {
        setResendConsent(false);
        void flow.resend();
      }
    };
  const more = () => {
      void flow.more();
    },
    reauthorize = () => flow.reauthorize(),
    readAgain = () => setReadFailure(null);
  const canPrepare =
    (!flow.pending || !!flow.receipt || flow.canPrepare) && !flow.blocked;
  const expired = !!first && !dayjs(first.expires_at).isAfter(time),
    complete =
      !!first &&
      !flow.pages.at(-1)?.next_cursor &&
      flow.pages.flatMap((p) => p.items).length === first.total_entries;
  return (
    <section className="space-y-5" aria-label={t('knowledgeBatch.projects')}>
      <h2 className="text-lg font-semibold">{t('knowledgeBatch.projects')}</h2>
      <p>{t('knowledgeBatch.explicitCoverage')}</p>
      <form onSubmit={submitProject} className="space-y-2">
        <label className="block">
          {t('knowledgeBatch.projectId')}
          <Input
            required
            pattern="[a-f0-9]{32}"
            value={projectInput}
            onChange={handleProject}
          />
        </label>
        <Button type="submit" variant="outline">
          {t('knowledgeBatch.openProject')}
        </Button>
      </form>
      <ProjectPicker domain={domain} onChoose={chooseProject} />
      {!!flow.error && (
        <BatchFailure
          error={flow.error}
          onReload={flow.blocked ? reauthorize : undefined}
        />
      )}
      {flow.pending && !flow.receipt && (
        <section
          className="space-y-2"
          aria-label={t('knowledgeBatch.publicationRecovery')}
        >
          <p role="status">{t('knowledgeBatch.publicationPending')}</p>
          <p>{t('knowledgeBatch.recoveryHint')}</p>
          <p className="break-all">
            {t('knowledgeBatch.originalBase')}:{' '}
            {flow.pending.expected_manifest_id === null
              ? 'null'
              : flow.pending.expected_manifest_id}
          </p>
          <Button onClick={recover} disabled={flow.busy}>
            {t('knowledgeBatch.checkPublication')}
          </Button>
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={resendConsent}
              disabled={flow.busy}
              onChange={approveResend}
            />
            {t('knowledgeDirectory.resendConsent')}
          </label>
          <Button
            onClick={resend}
            variant="outline"
            disabled={flow.busy || !resendConsent || flow.blocked}
          >
            {t('knowledgeBatch.resendPublication')}
          </Button>
        </section>
      )}
      {flow.receipt && (
        <section
          className="space-y-2"
          aria-label={t('knowledgeBatch.publicationReceipt')}
        >
          <p role="status">{t('knowledgeBatch.published')}</p>
          <p className="break-all">{flow.receipt.manifest_id}</p>
          <p>
            {flow.receipt.total_entries} ·{' '}
            {t('knowledgeBatch.explicitCoverage')}
          </p>
        </section>
      )}
      {currentProject && (
        <form onSubmit={prepare} className="space-y-2">
          <p>{t('knowledgeBatch.successfulTaskOnly')}</p>
          <label className="block">
            {t('knowledgeBatch.registrationId')}
            <Input
              required
              pattern="[a-f0-9]{32}"
              value={reg}
              onChange={handleReg}
              disabled={!canPrepare || flow.busy}
            />
          </label>
          <label className="block">
            {t('knowledgeBatch.taskId')}
            <Input
              required
              pattern="[a-f0-9]{32}"
              value={task}
              onChange={handleTask}
              disabled={!canPrepare || flow.busy}
            />
          </label>
          <Button type="submit" disabled={!canPrepare || flow.busy}>
            {t('knowledgeBatch.prepareProject')}
          </Button>
        </form>
      )}
      {first && (
        <section
          className="space-y-3 rounded-md border border-border-button bg-bg-card p-4"
          aria-label={t('knowledgeBatch.manifestPreview')}
        >
          <h3>{t('knowledgeBatch.manifestPreview')}</h3>
          <p>
            {t('knowledgeBatch.originalBase')}:{' '}
            {first.expected_manifest_id === null
              ? 'null'
              : first.expected_manifest_id}
          </p>
          <p>
            {t('knowledgeBatch.manifestCounts', {
              added: first.added,
              replaced: first.replaced,
              carried: first.carried,
              total: first.total_entries,
            })}
          </p>
          <p>
            {flow.pages.flatMap((p) => p.items).length}/{first.total_entries} ·{' '}
            {t('knowledgeBatch.explicitCoverage')}
          </p>
          <ul className="space-y-2">
            {flow.pages
              .flatMap((p) => p.items)
              .map((i) => (
                <ManifestEntryRow key={i.source_id} domain={domain} item={i} />
              ))}
          </ul>
          <LoadMore
            more={!!flow.pages.at(-1)?.next_cursor}
            loading={flow.busy}
            onMore={more}
          />
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={publicationConsent}
              onChange={approve}
              disabled={flow.busy || !complete || expired}
            />
            {t('knowledgeBatch.publicationConsent')}
          </label>
          {!complete && <p>{t('knowledgeBatch.loadFullManifest')}</p>}
          {expired && <p role="alert">{t('knowledgeBatch.errors.expired')}</p>}
          <Button
            onClick={publish}
            disabled={flow.busy || !publicationConsent || !complete || expired}
          >
            {t('knowledgeBatch.publish')}
          </Button>
        </section>
      )}
      {currentProject &&
        (readFailure ? (
          <BatchFailure error={readFailure} onReload={readAgain} />
        ) : (
          <ProjectRead
            key={currentProject}
            domain={domain}
            project={currentProject}
            manifestId={manifestId}
            onSelect={selectManifest}
            onFailure={setReadFailure}
          />
        ))}
    </section>
  );
}

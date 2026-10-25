import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  useBatchActions,
  useBatchTask,
  useBatchTasks,
  useBatchItems,
  batchScopeFailure,
} from '@/hooks/use-knowledge-batch';
import service, { BatchRequestError } from '@/services/knowledge-batch-service';
import type {
  BatchTask,
  BatchTaskItem,
  BatchTaskItemsPage,
  BatchTaskPage,
} from '@/interfaces/database/knowledge-batch';
import {
  appendBatchResultPage,
  appendBatchTaskPage,
  batchSaved,
} from '@/utils/knowledge-batch';
import { batchTerminal } from '@/utils/knowledge-batch-schema';
import { knowledgeReceiptLink } from '@/utils/knowledge-source-link';
import { CatalogueLoading, LoadMore } from '../knowledge-catalogue/state';
import { BatchFailure } from './batch-fields';
import type { useBatchFlow } from './use-batch-flow';

export function BatchResultRow({
  domain,
  item,
}: {
  domain: string;
  item: BatchTaskItem;
}) {
  const { t } = useTranslation();
  return (
    <li className="space-y-1 rounded-md border border-border-button p-3">
      <p className="break-all">
        {item.relative_path} · {t('knowledgeBatch.itemStates.' + item.status)}
      </p>
      <p>
        {t('knowledgeCatalogue.' + item.classification.component_side)} ·{' '}
        {item.classification.language} · {item.byte_length} B
      </p>
      <p>
        {t('knowledgeBatch.reasons.' + item.code)} ·{' '}
        {t('knowledgeBatch.catalogueOnly')}
      </p>
      {batchSaved(item) && (
        <Link className="underline" to={knowledgeReceiptLink(domain, item)}>
          {t('knowledgeImport.readRevision')}
        </Link>
      )}
    </li>
  );
}
function TaskRows({
  domain,
  reg,
  task,
  onFailure,
}: {
  domain: string;
  reg: string;
  task: BatchTask;
  onFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    query = useBatchItems(domain, reg, task),
    handled = useRef<unknown>(null);
  const resultError = query.error,
    restartResults = query.restart;
  useEffect(() => {
    if (resultError && handled.current !== resultError) {
      handled.current = resultError;
      if (
        resultError instanceof BatchRequestError &&
        resultError.reason === 'result_changed'
      )
        void restartResults();
      else if (batchScopeFailure(resultError)) onFailure(resultError);
    }
  }, [resultError, restartResults, onFailure]);
  const more = () => {
      void query.fetchNextPage();
    },
    restart = () => {
      void query.restart();
    };
  if (query.error)
    return <BatchFailure error={query.error} onReload={restart} />;
  if (!query.data) return <CatalogueLoading />;
  let pages: BatchTaskItemsPage[] = [];
  try {
    for (const page of query.data.pages)
      pages = appendBatchResultPage(pages, page, task);
  } catch (error) {
    return <BatchFailure error={error} onReload={restart} />;
  }
  return (
    <section className="space-y-2" aria-label={t('knowledgeBatch.results')}>
      <p>
        {t('knowledgeBatch.resultRevision')}: {pages[0].result_revision} ·{' '}
        {pages.flatMap((p) => p.items).length}/{pages[0].counts.total}
      </p>
      <ul className="space-y-2">
        {pages
          .flatMap((p) => p.items)
          .map((item) => (
            <BatchResultRow key={item.item_id} domain={domain} item={item} />
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
function TaskDetail({
  domain,
  reg,
  id,
  flow,
  onProject,
}: {
  domain: string;
  reg: string;
  id: string;
  flow: ReturnType<typeof useBatchFlow>;
  onProject: (reg: string, task: string) => void;
}) {
  const { t } = useTranslation(),
    query = useBatchTask(domain, reg, id),
    actions = useBatchActions(domain);
  const [cancelConsent, setCancelConsent] = useState(false),
    [cancelling, setCancelling] = useState(false),
    [cancelError, setCancelError] = useState<unknown>(null);
  const CancelKey = useRef<string | null>(null);
  const fail = flow.fail,
    rememberTask = flow.rememberTask;
  useEffect(() => {
    if (query.error && batchScopeFailure(query.error)) fail(query.error);
  }, [query.error, fail]);
  useEffect(() => {
    if (query.data) rememberTask(query.data);
  }, [query.data, rememberTask]);
  const recheck = () => {
    void query.recheck();
  };
  const handleConsent = (e: ChangeEvent<HTMLInputElement>) =>
    setCancelConsent(e.target.checked);
  const handleCancel = async () => {
    if (
      !query.data ||
      !cancelConsent ||
      cancelling ||
      query.data.cancel_requested ||
      batchTerminal(query.data.status)
    )
      return;
    setCancelling(true);
    setCancelError(null);
    CancelKey.current ||= crypto.randomUUID();
    try {
      const result = await actions.run(() =>
        service.cancel(domain, reg, id, {
          idempotency_key: CancelKey.current!,
        }),
      );
      query.adopt(result.data);
      setCancelConsent(false);
    } catch (error) {
      setCancelError(error);
      if (batchScopeFailure(error)) flow.fail(error);
    } finally {
      setCancelling(false);
    }
  };
  const retryUnfinished = () => {
      if (query.data) void flow.retry(query.data, false);
    },
    retryFull = () => {
      if (query.data) void flow.retry(query.data, true);
    };
  const project = () => {
    if (query.data) onProject(reg, query.data.task_id);
  };
  if (query.error && batchScopeFailure(query.error))
    return <BatchFailure error={query.error} onReload={recheck} />;
  if (!query.data)
    return query.error ? (
      <BatchFailure error={query.error} onReload={recheck} />
    ) : (
      <CatalogueLoading />
    );
  const task = query.data,
    c = task.counts,
    mayAct = !query.error && !cancelling && !flow.busy;
  return (
    <section className="space-y-4" aria-label={t('knowledgeBatch.taskDetails')}>
      <h2 className="text-lg font-semibold">
        {t('knowledgeBatch.taskStates.' + task.status)}
      </h2>
      <p className="break-all">
        {t('knowledgeBatch.taskId')}: {task.task_id} ·{' '}
        {t('knowledgeBatch.resultRevision')}: {task.revision}
      </p>
      <p>
        {t('knowledgeBatch.progress', {
          done: c.succeeded + c.failed + c.cancelled,
          total: c.total,
        })}
      </p>
      <p>
        {t('knowledgeBatch.counts', {
          pending: c.pending,
          running: c.running,
          succeeded: c.succeeded,
          failed: c.failed,
          cancelled: c.cancelled,
        })}
      </p>
      <p>{t('knowledgeBatch.catalogueOnly')}</p>
      {!!query.error && <BatchFailure error={query.error} onReload={recheck} />}
      {query.pollingStopped && (
        <p role="status">{t('knowledgeBatch.manualCheck')}</p>
      )}
      <Button variant="outline" onClick={recheck} disabled={query.isFetching}>
        {t('knowledgeImport.checkResult')}
      </Button>
      {!!cancelError && <BatchFailure error={cancelError} />}
      {!batchTerminal(task.status) && (
        <div className="space-y-2">
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={cancelConsent}
              onChange={handleConsent}
              disabled={!mayAct || task.cancel_requested}
            />
            {t('knowledgeBatch.cancelConsent')}
          </label>
          <Button
            variant="outline"
            onClick={handleCancel}
            disabled={!mayAct || !cancelConsent || task.cancel_requested}
          >
            {t('knowledgeBatch.cancel')}
          </Button>
        </div>
      )}
      {['partial_failed', 'failed', 'cancelled'].includes(task.status) && (
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            disabled={!mayAct}
            onClick={retryUnfinished}
          >
            {t('knowledgeBatch.retryUnfinished')}
          </Button>
          <Button variant="outline" disabled={!mayAct} onClick={retryFull}>
            {t('knowledgeBatch.retryFull')}
          </Button>
        </div>
      )}
      {task.status === 'succeeded' && (
        <Button onClick={project} disabled={!mayAct}>
          {t('knowledgeBatch.prepareProject')}
        </Button>
      )}
      <TaskRows
        key={task.task_id + ':' + task.revision}
        domain={domain}
        reg={reg}
        task={task}
        onFailure={flow.fail}
      />
    </section>
  );
}
function TaskRow({
  task,
  onSelect,
}: {
  task: BatchTask;
  onSelect: (task: BatchTask) => void;
}) {
  const { t } = useTranslation(),
    select = () => onSelect(task);
  return (
    <li>
      <Button
        variant="outline"
        onClick={select}
        className="h-auto w-full justify-start whitespace-normal break-all text-left"
      >
        {task.task_id} · {t('knowledgeBatch.taskStates.' + task.status)}
      </Button>
    </li>
  );
}
function TaskList({
  domain,
  reg,
  onSelect,
  onFailure,
}: {
  domain: string;
  reg: string;
  onSelect: (task: BatchTask) => void;
  onFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    query = useBatchTasks(domain, reg);
  useEffect(() => {
    if (query.error && batchScopeFailure(query.error)) onFailure(query.error);
  }, [query.error, onFailure]);
  const more = () => {
      void query.fetchNextPage();
    },
    restart = () => {
      void query.restart();
    };
  if (query.error)
    return <BatchFailure error={query.error} onReload={restart} />;
  if (!query.data) return <CatalogueLoading />;
  let pages: BatchTaskPage[] = [];
  try {
    for (const page of query.data.pages)
      pages = appendBatchTaskPage(pages, page);
  } catch (error) {
    return <BatchFailure error={error} onReload={restart} />;
  }
  return (
    <section className="space-y-2" aria-label={t('knowledgeBatch.tasks')}>
      <p>
        {t('knowledgeBatch.listSnapshot')}: {pages[0].list_snapshot_id} ·{' '}
        {pages.flatMap((p) => p.items).length}/{pages[0].total_tasks}
      </p>
      <p>{t('knowledgeBatch.listStatesHint')}</p>
      <Button variant="outline" onClick={restart}>
        {t('knowledgeCatalogue.refresh')}
      </Button>
      <ul className="space-y-2">
        {pages
          .flatMap((p) => p.items)
          .map((task) => (
            <TaskRow key={task.task_id} task={task} onSelect={onSelect} />
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
export default function BatchTasks({
  domain,
  reg,
  taskId,
  flow,
  onSelect,
  onProject,
}: {
  domain: string;
  reg: string;
  taskId: string;
  flow: ReturnType<typeof useBatchFlow>;
  onSelect: (task: BatchTask) => void;
  onProject: (reg: string, task: string) => void;
}) {
  const { t } = useTranslation(),
    [resendConsent, setResendConsent] = useState(false);
  const resendToggle = (e: ChangeEvent<HTMLInputElement>) =>
    setResendConsent(e.target.checked);
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
  const reauthorize = () => flow.reauthorize(),
    startNew = () => {
      void flow.startNew();
    };
  const [lookup, setLookup] = useState(taskId),
    handleLookup = (e: ChangeEvent<HTMLInputElement>) =>
      setLookup(e.target.value);
  const submitLookup = (e: FormEvent) => {
    e.preventDefault();
    onSelect({ task_id: lookup, registration_id: reg } as BatchTask);
  };
  if (flow.blocked)
    return <BatchFailure error={flow.error} onReload={reauthorize} />;
  return (
    <div className="space-y-5">
      {flow.pending && (
        <section
          className="space-y-2"
          aria-label={t('knowledgeBatch.recovery')}
        >
          <p role="status">
            {t(
              flow.pending.task_id
                ? 'knowledgeBatch.accepted'
                : 'knowledgeBatch.pending',
            )}
          </p>
          <p>{t('knowledgeBatch.recoveryHint')}</p>
          <Button onClick={recover} disabled={flow.busy}>
            {t('knowledgeBatch.checkOriginal')}
          </Button>
          {!flow.pending.task_id && (
            <>
              <label className="flex items-start gap-2">
                <input
                  type="checkbox"
                  checked={resendConsent}
                  onChange={resendToggle}
                  disabled={flow.busy}
                />
                {t('knowledgeDirectory.resendConsent')}
              </label>
              <Button
                onClick={resend}
                variant="outline"
                disabled={flow.busy || !resendConsent}
              >
                {t('knowledgeDirectory.resend')}
              </Button>
            </>
          )}
          {(flow.pending.task_id || flow.canRestart) && (
            <Button variant="outline" disabled={flow.busy} onClick={startNew}>
              {t('knowledgeBatch.newBatch')}
            </Button>
          )}
        </section>
      )}
      {reg ? (
        <>
          <form className="space-y-2" onSubmit={submitLookup}>
            <label className="block">
              {t('knowledgeBatch.taskId')}
              <Input
                required
                pattern="[a-f0-9]{32}"
                value={lookup}
                onChange={handleLookup}
              />
            </label>
            <Button type="submit">{t('knowledgeBatch.openTask')}</Button>
          </form>
          {taskId && (
            <TaskDetail
              key={reg + ':' + taskId}
              domain={domain}
              reg={reg}
              id={taskId}
              flow={flow}
              onProject={onProject}
            />
          )}
          <TaskList
            key={reg}
            domain={domain}
            reg={reg}
            onSelect={onSelect}
            onFailure={flow.fail}
          />
        </>
      ) : (
        <p>{t('knowledgeBatch.chooseRegistration')}</p>
      )}
    </div>
  );
}

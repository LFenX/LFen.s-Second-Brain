import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import {
  useStructureTask,
  useStructureTasks,
} from '@/hooks/use-knowledge-structure';
import type { StructureTask } from '@/interfaces/database/knowledge-structure';
import {
  structureRejected,
  structureSlotKey,
  structureTerminal,
  type StructureProjectScope,
  type StructureScope,
  type StructureRecoverySlot,
} from '@/utils/knowledge-structure';
import type { StructureFlow } from './use-structure-flow';
import { CatalogueLoading, LoadMore } from './state';
import { StructureFailure } from './structure-state';

function RecoveryRow({
  slot,
  flow,
  onTask,
}: {
  slot: StructureRecoverySlot;
  flow: StructureFlow;
  onTask: (task: StructureTask) => void;
}) {
  const { t } = useTranslation(),
    [ack, setAck] = useState(false),
    outcome = flow.outcomes[structureSlotKey(slot)];
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const recover = async () => {
    setAck(false);
    try {
      const result = await flow.recover(slot);
      if (!structureRejected(result) && 'state' in result) onTask(result);
    } catch {
      /* The flow retains the original slot and renders the failure. */
    }
  };
  const resend = async () => {
    if (!ack) return;
    setAck(false);
    try {
      const result = await flow.resend(slot);
      if (!structureRejected(result) && 'state' in result) onTask(result);
    } catch {
      /* The flow retains the original slot. */
    }
  };
  const abandon = async () => {
    setAck(false);
    try {
      await flow.abandon(slot);
    } catch {
      /* The flow renders the failure. */
    }
  };
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p>
        {t('knowledgeStructure.recoveryKind', {
          kind: t(
            'knowledgeStructure.' +
              (slot.kind === 'policy'
                ? 'policyLabel'
                : slot.kind === 'cancel'
                  ? 'cancelLabel'
                  : 'task'),
          ),
        })}
      </p>
      <p className="break-all text-xs">
        {slot.target} · {slot.key} · {slot.request_sha256}
      </p>
      <p>{t('knowledgeStructure.unknownHint')}</p>
      {outcome && structureRejected(outcome) && (
        <p role="status">
          {t('knowledgeStructure.fixedRejection')}: {outcome.reason} ·{' '}
          {outcome.rejection_id}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={flow.busy}
          onClick={recover}
        >
          {t('knowledgeStructure.recover')}
        </Button>
        {outcome &&
          structureRejected(outcome) &&
          flow.rejections[structureSlotKey(slot)] && (
            <Button
              type="button"
              variant="outline"
              disabled={flow.busy}
              onClick={abandon}
            >
              {t('knowledgeStructure.abandon')}
            </Button>
          )}
      </div>
      {!outcome || !structureRejected(outcome) ? (
        <>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={ack} onChange={acknowledge} />
            {t('knowledgeStructure.confirmResend')}
          </label>
          <Button
            type="button"
            variant="outline"
            disabled={!ack || flow.busy || !flow.ready}
            onClick={resend}
          >
            {t('knowledgeStructure.resend')}
          </Button>
        </>
      ) : null}
    </li>
  );
}
export function StructureRecovery({
  flow,
  onTask,
}: {
  flow: StructureFlow;
  onTask: (task: StructureTask) => void;
}) {
  const { t } = useTranslation(),
    receipts = Object.values(flow.outcomes)
      .filter((value) => !structureRejected(value) && !('state' in value))
      .slice(-5);
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeStructure.recovery')}</h3>
      <p className="text-sm text-text-secondary">
        {t('knowledgeStructure.storageRules')}
      </p>
      {!flow.ready && (
        <p role="status">{t('knowledgeStructure.storageBlocked')}</p>
      )}
      {!!flow.error && <StructureFailure error={flow.error} />}
      <ul className="space-y-2">
        {flow.slots.map((slot) => (
          <RecoveryRow
            key={structureSlotKey(slot)}
            slot={slot}
            flow={flow}
            onTask={onTask}
          />
        ))}
      </ul>
      {receipts.map((receipt) => (
        <div
          key={'command_id' in receipt ? receipt.command_id : ''}
          className="space-y-1 text-xs"
        >
          <p>{t('knowledgeStructure.fixedReceipt')}</p>
          <pre className="overflow-auto whitespace-pre-wrap break-all">
            {JSON.stringify(receipt, null, 2)}
          </pre>
        </div>
      ))}
    </section>
  );
}
function TaskRow({
  task,
  onChoose,
}: {
  task: StructureTask;
  onChoose: (task: StructureTask) => void;
}) {
  const { t } = useTranslation(),
    choose = () => onChoose(task);
  return (
    <li>
      <Button
        type="button"
        variant="outline"
        className="h-auto max-w-full whitespace-normal text-left"
        onClick={choose}
      >
        <span className="break-all">
          {dayjs(task.created_at).format('YYYY-MM-DD HH:mm:ss')} ·{' '}
          {t('knowledgeStructure.state_' + task.state)} · {task.task_id}
        </span>
      </Button>
    </li>
  );
}
export function StructureTaskList({
  scope,
  onTask,
}: {
  scope: StructureProjectScope;
  onTask: (task: StructureTask) => void;
}) {
  const { t } = useTranslation(),
    query = useStructureTasks(scope);
  const reload = () => {
      void query.refetch();
    },
    more = () => {
      void query.fetchNextPage();
    };
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeStructure.tasks')}</h3>
      <Button type="button" variant="outline" onClick={reload}>
        {t('knowledgeCatalogue.refresh')}
      </Button>
      {query.isError ? (
        <StructureFailure error={query.error} onRetry={reload} />
      ) : !query.data ? (
        <CatalogueLoading />
      ) : (
        <>
          <ul className="space-y-2">
            {query.data.pages
              .flatMap((page) => page.items)
              .map((task) => (
                <TaskRow key={task.task_id} task={task} onChoose={onTask} />
              ))}
          </ul>
          <LoadMore
            more={query.hasNextPage}
            loading={query.isFetchingNextPage}
            onMore={more}
          />
        </>
      )}
    </section>
  );
}
export function StructureTaskTracker({
  scope,
  taskId,
  flow,
  onView,
}: {
  scope: StructureScope;
  taskId: string;
  flow: StructureFlow;
  onView: (view: string) => void;
}) {
  const { t } = useTranslation(),
    query = useStructureTask(scope, taskId),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  const reconciledRevision = useRef('');
  useEffect(() => {
    const task = query.data;
    if (!task) return;
    const revision = task.task_id + ':' + task.revision;
    if (reconciledRevision.current === revision) return;
    reconciledRevision.current = revision;
    setAck(false);
    void flow.reconcile(task).catch(setError);
  }, [flow, query.data]);
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const reload = () => {
    setAck(false);
    query.resume();
    void query.refetch();
  };
  const resume = () => {
    setAck(false);
    query.resume();
  };
  const cancel = async () => {
    if (!query.data || !ack || !flow.ready || flow.busy) return;
    const task = query.data;
    setAck(false);
    setError(null);
    try {
      await flow.submit({
        kind: 'cancel',
        scope: { ...scope, task: task.task_id },
        body: {
          expected_task_revision: task.revision,
          confirmed: true,
          idempotency_key: crypto.randomUUID(),
        },
      });
    } catch (issue) {
      setError(issue);
    }
  };
  const open = () => {
    if (query.data?.state === 'succeeded' && query.data.result_view_id)
      onView(query.data.result_view_id);
  };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  const task = query.data;
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="break-all font-semibold">
        {t('knowledgeStructure.task')} · {task.task_id}
      </h3>
      <p role="status">
        {t('knowledgeStructure.state_' + task.state)} · revision {task.revision}{' '}
        · {task.finished_members}/{task.total_members}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeStructure.atomicHint')}
      </p>
      {task.failure_code && <p>{task.failure_code}</p>}
      <ul className="space-y-2 text-xs">
        {task.members.map((member) => (
          <li key={member.source_id} className="break-all">
            {member.source_id} ·{' '}
            {t('knowledgeStructure.member_' + member.state)} ·{' '}
            {member.node_count} / {member.relation_count}
            {member.diagnostics.map((diagnostic, index) => (
              <p key={index}>
                {diagnostic.code}: {diagnostic.message} [
                {diagnostic.start_byte ?? '∅'},{diagnostic.end_byte ?? '∅'})
              </p>
            ))}
          </li>
        ))}
      </ul>
      {task.state === 'succeeded' && task.result_view_id && (
        <Button type="button" onClick={open}>
          {t('knowledgeStructure.openResult')}
        </Button>
      )}
      {!structureTerminal(task) && (
        <>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={ack} onChange={acknowledge} />
            {t('knowledgeStructure.confirmCancel', { revision: task.revision })}
          </label>
          <Button
            type="button"
            variant="outline"
            disabled={
              !ack ||
              flow.busy ||
              !flow.ready ||
              task.state === 'cancel_requested'
            }
            onClick={cancel}
          >
            {t('knowledgeStructure.cancel')}
          </Button>
          {!query.waiting && (
            <p role="status">
              {t('knowledgeStructure.waitExpired')}{' '}
              <Button type="button" variant="outline" onClick={resume}>
                {t('knowledgeStructure.continueTracking')}
              </Button>
            </p>
          )}
        </>
      )}
      {!!error && <StructureFailure error={error} />}
    </section>
  );
}

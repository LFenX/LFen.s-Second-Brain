import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { Link } from 'react-router';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useDocumentTask } from '@/hooks/use-knowledge-document';
import {
  documentRejected,
  documentSlotKey,
  documentTerminal,
  documentFixedLink,
  type DocumentScope,
  type DocumentRecoverySlot,
} from '@/utils/knowledge-document';
import type { DocumentFlow } from './use-document-flow';
import { CatalogueLoading } from './state';
import { DocumentFailure } from './document-state';
function RecoveryRow({
  slot,
  flow,
  onTask,
}: {
  slot: DocumentRecoverySlot;
  flow: DocumentFlow;
  onTask: (task: string) => void;
}) {
  const { t } = useTranslation(),
    [ack, setAck] = useState(false),
    outcome = flow.outcomes[documentSlotKey(slot)];
  useEffect(() => setAck(false), [outcome, slot.request_sha256]);
  const checked = (e: ChangeEvent<HTMLInputElement>) =>
    setAck(e.target.checked);
  const accept = (v: Awaited<ReturnType<DocumentFlow['recover']>>) => {
    if (v.record_kind === 'task') onTask(v.task_id);
  };
  const recover = async () => {
    setAck(false);
    try {
      accept(await flow.recover(slot));
    } catch {
      /* The saved request remains available. */
    }
  };
  const resend = async () => {
    if (!ack) return;
    setAck(false);
    try {
      accept(await flow.resend(slot));
    } catch {
      /* The saved request remains available. */
    }
  };
  const abandon = async () => {
    if (!ack) return;
    setAck(false);
    try {
      await flow.abandon(slot);
    } catch {
      /* The flow displays the failure. */
    }
  };
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p>
        {t('knowledgeDocument.recoveryKind', {
          kind: t('knowledgeDocument.' + slot.kind),
        })}
      </p>
      <details>
        <summary>{t('knowledgeDocument.evidence')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {slot.target} · {slot.key} · {slot.request_sha256}
        </pre>
      </details>
      <p>{t('knowledgeDocument.unknownHint')}</p>
      {outcome && documentRejected(outcome) && (
        <p>
          {t('knowledgeDocument.fixedRejection')}: {outcome.reason}
        </p>
      )}
      <Button variant="outline" disabled={flow.busy} onClick={recover}>
        {t('knowledgeDocument.recover')}
      </Button>
      <label className="flex gap-2">
        <input type="checkbox" checked={ack} onChange={checked} />
        {t(
          outcome && documentRejected(outcome)
            ? 'knowledgeDocument.confirmAbandon'
            : 'knowledgeDocument.confirmResend',
        )}
      </label>
      {outcome && documentRejected(outcome) ? (
        <Button
          variant="outline"
          disabled={
            !ack || flow.busy || !flow.rejections[documentSlotKey(slot)]
          }
          onClick={abandon}
        >
          {t('knowledgeDocument.abandon')}
        </Button>
      ) : (
        <Button
          variant="outline"
          disabled={!ack || flow.busy || !flow.ready}
          onClick={resend}
        >
          {t('knowledgeDocument.resend')}
        </Button>
      )}
    </li>
  );
}
export function DocumentRecovery({
  flow,
  onTask,
}: {
  flow: DocumentFlow;
  onTask: (task: string) => void;
}) {
  const { t } = useTranslation(),
    receipts = Object.values(flow.outcomes).filter(
      (v) =>
        v.record_kind === 'policy_receipt' ||
        v.record_kind === 'cancel_receipt',
    );
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeDocument.recovery')}</h3>
      <p className="text-sm text-text-secondary">
        {t('knowledgeDocument.storageRules')}
      </p>
      {!flow.ready && (
        <p role="status">{t('knowledgeDocument.storageBlocked')}</p>
      )}
      {!!flow.error && <DocumentFailure error={flow.error} />}
      <ul className="space-y-3">
        {flow.slots.map((slot) => (
          <RecoveryRow
            key={documentSlotKey(slot)}
            slot={slot}
            flow={flow}
            onTask={onTask}
          />
        ))}
      </ul>
      {receipts.map((r) => (
        <details key={'command_id' in r ? r.command_id : ''}>
          <summary>
            {t('knowledgeDocument.fixedReceipt')}: {r.record_kind}
          </summary>
          <pre className="whitespace-pre-wrap break-all text-xs">
            {JSON.stringify(r, null, 2)}
          </pre>
        </details>
      ))}
    </section>
  );
}
export function DocumentTaskPanel({
  scope,
  task,
  flow,
}: {
  scope: DocumentScope;
  task: string;
  flow: DocumentFlow;
}) {
  const { t } = useTranslation(),
    query = useDocumentTask(scope, task),
    [ack, setAck] = useState(false),
    revision = useRef(0),
    reconcile = flow.reconcile;
  useEffect(() => {
    if (query.data && query.data.revision !== revision.current) {
      revision.current = query.data.revision;
      setAck(false);
      void reconcile(query.data).catch(() => undefined);
    }
  }, [query.data, reconcile]);
  const checked = (e: ChangeEvent<HTMLInputElement>) =>
    setAck(e.target.checked);
  const cancel = async () => {
    if (!ack || !query.data) return;
    const current = query.data;
    setAck(false);
    try {
      await flow.submit({
        kind: 'cancel',
        scope: { ...scope, task },
        body: {
          expected_task_revision: current.revision,
          confirmed: true,
          idempotency_key: crypto.randomUUID(),
        },
      });
      await query.refetch();
    } catch {
      /* The cancel slot remains recoverable. */
    }
  };
  const resume = query.resume;
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending)
    return (
      <>
        <CatalogueLoading />
        {!query.waiting && (
          <Button onClick={resume}>{t('knowledgeDocument.resume')}</Button>
        )}
      </>
    );
  const current = query.data,
    finished = documentTerminal(current);
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeDocument.task')}</h3>
      <p role="status">
        {t('knowledgeDocument.state_' + current.state)} ·{' '}
        {t('knowledgeDocument.phase_' + current.phase)}
      </p>
      <p className="text-sm">
        {current.progress
          ? t('knowledgeDocument.progress', {
              done: current.progress.completed_units,
              total:
                current.progress.total_units ?? t('knowledgeDocument.unknown'),
            })
          : t('knowledgeDocument.unknownProgress')}
      </p>
      <p className="text-xs text-text-secondary">
        {dayjs(current.updated_at).format('YYYY-MM-DD HH:mm:ss')}
      </p>
      {['stopping', 'cleaning'].includes(current.phase) && (
        <p>{t('knowledgeDocument.stoppingHint')}</p>
      )}
      {current.failure_code && <p role="alert">{current.failure_code}</p>}
      {!finished && (
        <>
          <label className="flex gap-2">
            <input type="checkbox" checked={ack} onChange={checked} />
            {t('knowledgeDocument.confirmCancel')}
          </label>
          <Button
            variant="outline"
            disabled={!ack || flow.busy || !flow.ready}
            onClick={cancel}
          >
            {t('knowledgeDocument.cancel')}
          </Button>
          {!query.waiting && (
            <>
              <p>{t('knowledgeDocument.waitExpired')}</p>
              <Button onClick={resume}>{t('knowledgeDocument.resume')}</Button>
            </>
          )}
        </>
      )}
      {current.result_representation_id && (
        <Link
          className="underline"
          to={documentFixedLink({
            ...scope,
            source: current.original.source_id,
            revision: current.original.source_revision_id,
            sha: current.original.sha256,
            representation: current.result_representation_id,
          })}
        >
          {t('knowledgeDocument.open')}
        </Link>
      )}
      <details>
        <summary>{t('knowledgeDocument.evidence')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(current, null, 2)}
        </pre>
      </details>
    </section>
  );
}

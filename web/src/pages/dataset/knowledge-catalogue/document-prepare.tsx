import { useEffect, useState, type ChangeEvent } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  useDocumentHead,
  useDocumentOriginals,
} from '@/hooks/use-knowledge-document';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import type { DocumentPrepared } from '@/services/knowledge-document-service';
import {
  documentFixedLink,
  sameDocumentValue,
  type DocumentScope,
} from '@/utils/knowledge-document';
import { originalRevisionLink } from '@/utils/knowledge-original';
import type { DocumentFlow } from './use-document-flow';
import { CatalogueLoading, LoadMore } from './state';
import { DocumentFailure } from './document-state';

function OriginalRow({
  original,
  onSelect,
}: {
  original: OriginalRevision;
  onSelect: (value: OriginalRevision) => void;
}) {
  const { t } = useTranslation(),
    select = () => onSelect(original);
  return (
    <li>
      <Button
        variant="outline"
        className="h-auto max-w-full whitespace-normal text-left"
        onClick={select}
      >
        {original.original_filename} · {original.byte_length} B
      </Button>
      <p className="break-all text-xs text-text-secondary">
        {original.relative_path}
      </p>
      {!original.is_current && (
        <p>{t('knowledgeDocument.historicalOriginal')}</p>
      )}
    </li>
  );
}
export function DocumentOriginalList({
  scope,
  onSelect,
}: {
  scope: DocumentScope;
  onSelect: (value: OriginalRevision) => void;
}) {
  const { t } = useTranslation(),
    query = useDocumentOriginals(scope),
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending) return <CatalogueLoading />;
  const originals = query.data.pages
    .flatMap((p) => p.items)
    .filter((p) =>
      [
        'application/pdf',
        'application/vnd.openxmlformats-officedocument.presentationml.presentation',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      ].includes(p.media_type),
    );
  return (
    <section className="space-y-3">
      <h3 className="font-semibold">{t('knowledgeDocument.chooseOriginal')}</h3>
      <ul className="space-y-2">
        {originals.map((o) => (
          <OriginalRow
            key={o.source_revision_id}
            original={o}
            onSelect={onSelect}
          />
        ))}
      </ul>
      {!originals.length && <p>{t('knowledgeDocument.noOriginals')}</p>}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={more}
      />
    </section>
  );
}
export function DocumentPreparation({
  scope,
  original,
  flow,
  onTask,
}: {
  scope: DocumentScope;
  original: OriginalRevision;
  flow: DocumentFlow;
  onTask: (task: string) => void;
}) {
  const { t } = useTranslation(),
    sourceScope = { ...scope, source: original.source_id },
    head = useDocumentHead(sourceScope);
  const [prepared, setPrepared] = useState<DocumentPrepared | null>(null),
    [ack, setAck] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  useEffect(() => {
    setPrepared(null);
    setAck(false);
    setError(null);
  }, [original.source_revision_id, flow.preparationEpoch]);
  const checked = (e: ChangeEvent<HTMLInputElement>) =>
    setAck(e.target.checked);
  const prepare = async () => {
    setPrepared(null);
    setAck(false);
    setError(null);
    setBusy(true);
    try {
      setPrepared(await flow.prepare(sourceScope, original));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const submit = async () => {
    if (!prepared || !ack) return;
    setAck(false);
    setBusy(true);
    setError(null);
    try {
      const fresh = await flow.prepare(sourceScope, original);
      if (!sameDocumentValue(fresh, prepared)) {
        setPrepared(null);
        throw Error('document_preparation_changed');
      }
      const result = await flow.submit({
        kind: 'task',
        scope: sourceScope,
        body: {
          ...prepared.request,
          confirmed: true,
          idempotency_key: crypto.randomUUID(),
        },
      });
      if (result.record_kind === 'task') onTask(result.task_id);
    } catch (e) {
      setPrepared(null);
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  if (head.isError) return <DocumentFailure error={head.error} />;
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{original.original_filename}</h3>
      <p className="text-sm">
        {t('knowledgeDocument.originalSaved')} · {original.byte_length} B
      </p>
      <Link className="underline" to={originalRevisionLink(original)}>
        {t('knowledgeDocument.originalLink')}
      </Link>
      <p className="text-sm text-text-secondary">
        {t('knowledgeDocument.processingRules')}
      </p>
      {head.data && (
        <>
          <p role="status">
            {t('knowledgeDocument.currentness_' + head.data.currentness)}
          </p>
          {head.data.reasons.map((reason) => (
            <p key={reason}>{t('knowledgeDocument.reason_' + reason)}</p>
          ))}
          {head.data.active_representation_id &&
            head.data.active_source_revision_id ===
              original.source_revision_id && (
              <Link
                className="underline"
                to={documentFixedLink({
                  ...sourceScope,
                  revision: original.source_revision_id,
                  sha: original.sha256,
                  representation: head.data.active_representation_id,
                })}
              >
                {t('knowledgeDocument.open')}
              </Link>
            )}
          {head.data.latest_task_id && (
            <LatestTask task={head.data.latest_task_id} onTask={onTask} />
          )}
        </>
      )}
      {!!error && <DocumentFailure error={error} />}
      <Button
        variant="outline"
        disabled={
          busy ||
          flow.busy ||
          !flow.ready ||
          original.byte_length > 8388608 ||
          !original.is_current
        }
        onClick={prepare}
      >
        {t('knowledgeDocument.prepare')}
      </Button>
      {prepared && (
        <>
          <p>
            {t('knowledgeDocument.prepared', {
              profile: prepared.profile.format,
            })}
          </p>
          <p className="break-all text-xs">{prepared.snapshot.relative_path}</p>
          <label className="flex gap-2">
            <input type="checkbox" checked={ack} onChange={checked} />
            {t('knowledgeDocument.confirmTask')}
          </label>
          <Button
            disabled={!ack || busy || flow.busy || !flow.ready}
            onClick={submit}
          >
            {t('knowledgeDocument.start')}
          </Button>
          <details>
            <summary>{t('knowledgeDocument.evidence')}</summary>
            <pre className="whitespace-pre-wrap break-all text-xs">
              {JSON.stringify(prepared.request, null, 2)}
            </pre>
          </details>
        </>
      )}
    </section>
  );
}
function LatestTask({
  task,
  onTask,
}: {
  task: string;
  onTask: (task: string) => void;
}) {
  const { t } = useTranslation(),
    open = () => onTask(task);
  return (
    <Button variant="outline" onClick={open}>
      {t('knowledgeDocument.latestTask')}
    </Button>
  );
}

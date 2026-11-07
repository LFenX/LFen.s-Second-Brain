import { useEffect, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import {
  originalUnexpired,
  originalRevisionLink,
} from '@/utils/knowledge-original';
import type { OriginalFlow } from './use-original-flow';
import { OriginalRevisionFacts } from './original-revisions';

export default function OriginalConfirmation({ flow }: { flow: OriginalFlow }) {
  const { t } = useTranslation(),
    upload = flow.upload!;
  const [checked, setChecked] = useState(false),
    [expired, setExpired] = useState(!originalUnexpired(upload));
  useEffect(() => {
    const timer = window.setInterval(
      () => setExpired(!originalUnexpired(upload)),
      1000,
    );
    return () => window.clearInterval(timer);
  }, [upload]);
  const change = (event: ChangeEvent<HTMLInputElement>) =>
    setChecked(event.target.checked);
  const confirm = () => {
    if (checked && !expired) void flow.confirm(true);
  };
  return (
    <section
      className="space-y-3 rounded-lg border border-border-button bg-bg-card p-4"
      aria-label={t('knowledgeOriginal.confirmation')}
    >
      <h2 className="text-lg font-semibold">
        {t('knowledgeOriginal.confirmation')}
      </h2>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-sm">
        <dt>{t('knowledgeOriginal.originalName')}</dt>
        <dd className="break-all">{upload.original_filename}</dd>
        <dt>{t('knowledgeOriginal.path')}</dt>
        <dd className="break-all">{upload.relative_path}</dd>
        <dt>{t('knowledgeOriginal.bytes')}</dt>
        <dd>{upload.byte_length} B</dd>
        <dt>{t('knowledgeOriginal.mediaType')}</dt>
        <dd className="break-all">{upload.media_type}</dd>
        <dt>SHA-256</dt>
        <dd className="break-all">{upload.sha256}</dd>
        <dt>{t('knowledgeOriginal.group')}</dt>
        <dd className="break-all">
          {upload.classification.group_id || t('knowledgeImport.noGroup')}
        </dd>
        <dt>{t('knowledgeOriginal.classification')}</dt>
        <dd>
          {t('knowledgeCatalogue.' + upload.classification.component_side)} ·{' '}
          {upload.classification.language ||
            t('knowledgeCatalogue.unknownLanguage')}
        </dd>
        <dt>{t('knowledgeOriginal.action')}</dt>
        <dd>{t('knowledgeOriginal.' + upload.action)}</dd>
        <dt>{t('knowledgeOriginal.expectedRevision')}</dt>
        <dd className="break-all">
          {upload.expected_revision_id || t('knowledgeOriginal.firstRevision')}
        </dd>
        <dt>{t('knowledgeOriginal.expires')}</dt>
        <dd>{dayjs(upload.expires_at).format('YYYY-MM-DD HH:mm:ss')}</dd>
      </dl>
      <p>{t('knowledgeOriginal.readyHint')}</p>
      {expired && <p role="status">{t('knowledgeOriginal.expired')}</p>}
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={checked}
          onChange={change}
          disabled={flow.busy || expired || flow.unknown}
        />
        <span>{t('knowledgeOriginal.consent')}</span>
      </label>
      <Button
        onClick={confirm}
        disabled={!checked || expired || flow.busy || flow.unknown}
      >
        {t('knowledgeOriginal.confirm')}
      </Button>
    </section>
  );
}
export function OriginalReceiptView({ flow }: { flow: OriginalFlow }) {
  const { t } = useTranslation(),
    receipt = flow.receipt!;
  const another = () => {
    void flow.startAnother();
  };
  const startNew = () => {
    void flow.startNewRegistration();
  };
  return (
    <section className="space-y-4" aria-label={t('knowledgeOriginal.receipt')}>
      <h2 className="text-lg font-semibold">
        {t('knowledgeOriginal.saved')} ·{' '}
        {t('knowledgeOriginal.' + receipt.save_status)}
      </h2>
      <p className="break-all">
        {t('knowledgeOriginal.receiptId')}: {receipt.import_task_id}
      </p>
      <p>
        {t('knowledgeOriginal.modelCalls')}: {receipt.model_calls}
      </p>
      <OriginalRevisionFacts source={receipt.source} />
      <Link className="underline" to={originalRevisionLink(receipt.source)}>
        {t('knowledgeOriginal.fixedDetails')}
      </Link>
      <p className="text-sm text-text-secondary">
        {t('knowledgeOriginal.receiptSnapshot')}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          onClick={another}
          disabled={flow.busy || flow.cancelling}
        >
          {t('knowledgeOriginal.continueRegistration')}
        </Button>
        <Button
          onClick={startNew}
          disabled={flow.busy || flow.cancelling || flow.unknown}
        >
          {t('knowledgeOriginal.startNewRegistration')}
        </Button>
      </div>
    </section>
  );
}

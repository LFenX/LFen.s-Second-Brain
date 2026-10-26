import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import type { LifecycleReceipt } from '@/interfaces/database/knowledge-lifecycle';

export default function LifecycleReceiptView({
  receipt,
}: {
  receipt: LifecycleReceipt;
}) {
  const { t } = useTranslation();
  const formatClass = (value: LifecycleReceipt['before']['classification']) =>
    [
      value.group_id || t('knowledgeImport.noGroup'),
      t('knowledgeCatalogue.' + value.component_side),
      value.language || t('knowledgeCatalogue.unknownLanguage'),
    ].join(' · ');
  return (
    <section
      aria-label={t('knowledgeLifecycle.receipt')}
      className="space-y-3 rounded-lg border border-border-button p-4"
    >
      <h3 className="font-semibold">
        {t('knowledgeLifecycle.receipt')} ·{' '}
        {t('knowledgeLifecycle.' + receipt.result)}
      </h3>
      <p>{t('knowledgeLifecycle.' + receipt.action)}</p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeLifecycle.receiptHint')}
      </p>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-sm">
        <dt>{t('knowledgeLifecycle.commandId')}</dt>
        <dd className="break-all">{receipt.command_id}</dd>
        <dt>{t('knowledgeLifecycle.state')}</dt>
        <dd>
          {t('knowledgeLifecycle.' + receipt.before.state)} →{' '}
          {t('knowledgeLifecycle.' + receipt.after.state)}
        </dd>
        <dt>{t('knowledgeLifecycle.generation')}</dt>
        <dd>
          {receipt.before.lifecycle_version} → {receipt.after.lifecycle_version}
        </dd>
        <dt>{t('knowledgeLifecycle.path')}</dt>
        <dd className="break-all">
          {receipt.before.relative_path} → {receipt.after.relative_path}
        </dd>
        <dt>{t('knowledgeLifecycle.classification')}</dt>
        <dd className="break-all">
          {formatClass(receipt.before.classification)} →{' '}
          {formatClass(receipt.after.classification)}
        </dd>
        <dt>{t('knowledgeLifecycle.revision')}</dt>
        <dd className="break-all">
          {receipt.before.source_revision_id} →{' '}
          {receipt.after.source_revision_id}
        </dd>
        <dt>SHA-256</dt>
        <dd className="break-all">{receipt.request_sha256}</dd>
        <dt>{t('knowledgeLifecycle.completedAt')}</dt>
        <dd>
          {dayjs(receipt.created_at).format('YYYY-MM-DD HH:mm:ss')} ·{' '}
          {t('knowledgeImport.modelCalls')}: {receipt.model_calls}
        </dd>
      </dl>
    </section>
  );
}

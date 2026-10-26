import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Button } from '@/components/ui/button';
import { webRevisionLink } from '@/utils/knowledge-web';
import type { WebFlow } from './use-web-flow';

export default function WebReceiptPanel({ flow }: { flow: WebFlow }) {
  const { t } = useTranslation(),
    receipt = flow.receipt!;
  return (
    <section aria-label={t('knowledgeWeb.receipt')} className="space-y-4">
      <h2 className="font-semibold">4 · {t('knowledgeWeb.receipt')}</h2>
      <p>
        {t('knowledgeWeb.receiptCounts', {
          saved: receipt.saved_count,
          unchanged: receipt.unchanged_count,
        })}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.notIndexed')} · {t('knowledgeImport.modelCalls')}:{' '}
        {receipt.model_calls}
      </p>
      <p className="break-all text-xs">
        {receipt.import_task_id} · {receipt.capture_id}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.unchangedHint')}
      </p>
      <ul className="space-y-3">
        {flow.receiptItems.map((item) => (
          <li
            key={item.entry_key}
            className="space-y-2 rounded-lg border border-border-button p-3"
          >
            <p>
              {t('knowledgeImport.' + item.action)} ·{' '}
              {item.classification.language ||
                t('knowledgeCatalogue.unknownLanguage')}
            </p>
            <p className="break-all text-xs">{item.entry_key}</p>
            <Link
              className="underline"
              to={webRevisionLink(receipt.domain_id, item)}
            >
              {t('knowledgeImport.readRevision')}
            </Link>
          </li>
        ))}
      </ul>
      <Button variant="outline" onClick={flow.startAnother}>
        {t('knowledgeWeb.newCapture')}
      </Button>
    </section>
  );
}

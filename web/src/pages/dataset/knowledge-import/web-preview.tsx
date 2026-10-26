import { useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import type { WebPreviewItem } from '@/interfaces/database/knowledge-web';
import type { WebFlow } from './use-web-flow';

function Acknowledgement({
  item,
  checked,
  busy,
  onChange,
}: {
  item: WebPreviewItem;
  checked: boolean;
  busy: boolean;
  onChange: (key: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const change = (event: ChangeEvent<HTMLInputElement>) =>
    onChange(item.entry_key, event.target.checked);
  return (
    <li className="space-y-2 rounded-lg border border-border-button p-4">
      <h3>
        {item.title || item.entry_key} · {t('knowledgeImport.' + item.action)}
      </h3>
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.completeness.' + item.body_completeness)}
      </p>
      <p className="break-all text-xs">
        {item.classification.group_id || t('knowledgeImport.noGroup')} ·{' '}
        {t('knowledgeCatalogue.' + item.classification.component_side)} ·{' '}
        {item.classification.language ||
          t('knowledgeCatalogue.unknownLanguage')}
      </p>
      <p className="break-all text-xs">
        {t('knowledgeImport.expectedRevision')}:{' '}
        {item.expected_revision_id || t('knowledgeImport.firstImport')}
      </p>
      <p className="break-all text-xs">SHA-256: {item.source_input_sha256}</p>
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={checked}
          disabled={busy}
          onChange={change}
        />
        <span>{t('knowledgeWeb.ack.' + item.acknowledgement)}</span>
      </label>
    </li>
  );
}
export default function WebPreviewPanel({ flow }: { flow: WebFlow }) {
  const { t } = useTranslation(),
    preview = flow.preview!;
  const [acknowledged, setAcknowledged] = useState<string[]>([]),
    [confirmed, setConfirmed] = useState(false);
  const change = (key: string, checked: boolean) => {
      setConfirmed(false);
      setAcknowledged((values) =>
        checked ? [...values, key] : values.filter((value) => value !== key),
      );
    },
    changeConfirmed = (event: ChangeEvent<HTMLInputElement>) =>
      setConfirmed(event.target.checked),
    confirm = () => {
      void flow.confirm(acknowledged, confirmed);
    },
    changeSelection = () => {
      void flow.changeSelection();
    },
    recheck = () => {
      void flow.recover();
    };
  const disabled = flow.busy || !!flow.unknown || preview.state !== 'ready';
  return (
    <section aria-label={t('knowledgeWeb.preview')} className="space-y-4">
      <h2 className="font-semibold">3 · {t('knowledgeWeb.preview')}</h2>
      <p className="text-sm">
        {preview.selected_count} · {preview.selected_byte_length} B ·{' '}
        {t('knowledgeImport.expires')}:{' '}
        {dayjs(preview.expires_at).format('YYYY-MM-DD HH:mm:ss')}
      </p>
      <p className="break-all text-xs">
        {preview.preview_id} · {preview.manifest_sha256}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.previewHint')}
      </p>
      <ul className="space-y-3">
        {flow.previewItems.map((item) => (
          <Acknowledgement
            key={item.entry_key}
            item={item}
            checked={acknowledged.includes(item.entry_key)}
            busy={disabled}
            onChange={change}
          />
        ))}
      </ul>
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={changeConfirmed}
          disabled={
            disabled || acknowledged.length !== flow.previewItems.length
          }
        />
        <span>{t('knowledgeWeb.finalConsent')}</span>
      </label>
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={confirm}
          disabled={
            disabled ||
            !confirmed ||
            acknowledged.length !== preview.selected_count
          }
        >
          {t('knowledgeWeb.confirm')}
        </Button>
        <Button
          variant="outline"
          onClick={changeSelection}
          disabled={
            flow.busy ||
            !!flow.unknown ||
            !!flow.slot?.confirmation_request ||
            preview.state === 'committed'
          }
        >
          {t('knowledgeWeb.changeSelection')}
        </Button>
        <Button variant="outline" onClick={recheck} disabled={flow.busy}>
          {t('knowledgeWeb.recheck')}
        </Button>
      </div>
    </section>
  );
}

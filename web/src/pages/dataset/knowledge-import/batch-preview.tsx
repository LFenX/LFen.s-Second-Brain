import { useEffect, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import type {
  BatchContentWindow,
  BatchPreviewItem,
} from '@/interfaces/database/knowledge-batch';
import type { useBatchFlow } from './use-batch-flow';

function ItemApproval({
  item,
  checked,
  disabled,
  ready,
  onChange,
  onRead,
}: {
  item: BatchPreviewItem;
  checked: boolean;
  disabled: boolean;
  ready: boolean;
  onChange: (id: string, checked: boolean) => void;
  onRead: (id: string) => void;
}) {
  const { t } = useTranslation(),
    toggle = (e: ChangeEvent<HTMLInputElement>) =>
      onChange(item.item_id, e.target.checked),
    read = () => onRead(item.item_id);
  return (
    <li className="space-y-2 rounded-md border border-border-button p-3">
      <p className="break-all">
        {item.relative_path} · {item.action} · {item.byte_length} B
      </p>
      <p>
        {t('knowledgeCatalogue.' + item.classification.component_side)} ·{' '}
        {item.classification.language} ·{' '}
        {item.classification.group_id || t('knowledgeImport.noGroup')}
      </p>
      <p className="break-all">SHA-256 {item.sha256}</p>
      <p className="break-all">
        {t('knowledgeImport.expectedRevision')}:{' '}
        {item.expected_revision_id || t('knowledgeImport.firstImport')}
      </p>
      <Button
        type="button"
        variant="outline"
        onClick={read}
        disabled={disabled}
      >
        {t('knowledgeBatch.readItem')} · {item.relative_path}
      </Button>
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={checked}
          onChange={toggle}
          disabled={disabled || !ready}
        />
        <span>
          {t('knowledgeBatch.approveItem')} · {item.relative_path}
        </span>
      </label>
    </li>
  );
}
export default function BatchPreviewPanel({
  flow,
}: {
  flow: ReturnType<typeof useBatchFlow>;
}) {
  const { t } = useTranslation(),
    preview = flow.preview!;
  const [approved, setApproved] = useState<string[]>([]),
    [readItems, setReadItems] = useState<string[]>([]),
    [itemId, setItemId] = useState(preview.items[0].item_id),
    [window, setWindow] = useState<BatchContentWindow | null>(null),
    [offsets, setOffsets] = useState([0]);
  const [expired, setExpired] = useState(
    !dayjs(preview.expires_at).isAfter(dayjs()),
  );
  useEffect(() => {
    const timer = globalThis.setInterval(
      () => setExpired(!dayjs(preview.expires_at).isAfter(dayjs())),
      1000,
    );
    return () => globalThis.clearInterval(timer);
  }, [preview.expires_at]);
  async function read(id: string, offset = 0) {
    setWindow(null);
    setItemId(id);
    const value = await flow.readContent(id, offset);
    if (value) {
      setWindow(value);
      setReadItems((current) => [...current.filter((item) => item !== id), id]);
    }
  }
  const handleItem = (id: string) => {
    setOffsets([0]);
    void read(id);
  };
  const handleApproval = (id: string, value: boolean) =>
    setApproved((current) =>
      value
        ? [...current.filter((i) => i !== id), id]
        : current.filter((i) => i !== id),
    );
  const handleNext = () => {
    if (window?.next_offset !== null && window?.next_offset !== undefined) {
      setOffsets((current) => [...current, window.next_offset!]);
      void read(itemId, window.next_offset);
    }
  };
  const handlePrevious = () => {
    const next = offsets.slice(0, -1);
    setOffsets(next);
    void read(itemId, next.at(-1) || 0);
  };
  const handleAccept = () => {
    if (approved.length === preview.items.length)
      void flow.accept(
        preview.items
          .filter((i) => approved.includes(i.item_id))
          .map((i) => i.item_id),
      );
  };
  return (
    <section
      aria-label={t('knowledgeBatch.preview')}
      className="space-y-3 rounded-md border border-border-button bg-bg-card p-4"
    >
      <h2 className="text-lg font-semibold">
        {t('knowledgeBatch.preview')} · {preview.items.length}
      </h2>
      <p>{t('knowledgeBatch.catalogueOnly')}</p>
      <p>{t('knowledgeBatch.noScrollRequirement')}</p>
      <p>
        {t('knowledgeImport.expires')}:{' '}
        {dayjs(preview.expires_at).format('YYYY-MM-DD HH:mm:ss')}
      </p>
      {expired && <p role="alert">{t('knowledgeBatch.errors.expired')}</p>}
      <ul className="space-y-3">
        {preview.items.map((item) => (
          <ItemApproval
            key={item.item_id}
            item={item}
            checked={approved.includes(item.item_id)}
            ready={readItems.includes(item.item_id)}
            disabled={flow.busy || expired}
            onChange={handleApproval}
            onRead={handleItem}
          />
        ))}
      </ul>
      {window && (
        <section
          aria-label={t('knowledgeBatch.stagedOriginal')}
          className="space-y-2"
        >
          <p className="break-all">
            {window.relative_path} [{window.offset},{' '}
            {window.offset + window.byte_length}) / {window.total_bytes} B
          </p>
          <pre
            data-testid="batch-preview-text"
            className="max-h-[45vh] overflow-auto whitespace-pre rounded-md border border-border-button p-3"
          >
            {window.text}
          </pre>
          <Button
            variant="outline"
            onClick={handlePrevious}
            disabled={flow.busy || offsets.length === 1}
          >
            {t('knowledgeCatalogue.previous')}
          </Button>
          <Button
            variant="outline"
            onClick={handleNext}
            disabled={flow.busy || window.next_offset === null}
          >
            {t('knowledgeCatalogue.next')}
          </Button>
        </section>
      )}
      <p>
        {t('knowledgeBatch.approvedCount', {
          count: approved.length,
          total: preview.items.length,
        })}
      </p>
      <Button
        onClick={handleAccept}
        disabled={
          flow.busy || expired || approved.length !== preview.items.length
        }
      >
        {t('knowledgeBatch.accept', { count: preview.items.length })}
      </Button>
    </section>
  );
}

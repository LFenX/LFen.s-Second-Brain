import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { BatchRequestError } from '@/services/knowledge-batch-service';
import type { BatchClassification } from '@/interfaces/database/knowledge-batch';
import type { DirectoryObservationItem } from '@/interfaces/database/knowledge-directory';
import {
  compareDirectoryPaths,
  validDirectoryData,
} from '@/utils/knowledge-directory-schema';
import { ClassificationFields } from './fields';
import type { useBatchFlow } from './use-batch-flow';

export function BatchFailure({
  error,
  onReload,
}: {
  error: unknown;
  onReload?: () => void;
}) {
  const { t } = useTranslation(),
    failure = error instanceof KnowledgeRequestError ? error : null;
  const label = failure?.code.includes('version')
    ? 'versionMismatch'
    : failure && [401, 403, 404].includes(failure.status)
      ? 'notVisible'
      : failure?.status === 410
        ? 'expired'
        : failure?.status === 429
          ? 'capacity'
          : failure?.status === 409
            ? 'conflict'
            : failure?.status === 400
              ? 'invalid'
              : failure?.status === 413
                ? 'tooLarge'
                : 'unavailable';
  return (
    <div
      role="alert"
      className="space-y-2 rounded-md border border-border-button p-3"
    >
      <p>{t('knowledgeBatch.errors.' + label)}</p>
      {failure instanceof BatchRequestError && failure.reason && (
        <p>{t('knowledgeBatch.reasons.' + failure.reason)}</p>
      )}
      {failure?.requestId && (
        <p className="break-all text-xs">
          {t('knowledgeCatalogue.requestId')}: {failure.requestId}
        </p>
      )}
      {onReload && (
        <Button variant="outline" onClick={onReload}>
          {t('knowledgeImport.reload')}
        </Button>
      )}
    </div>
  );
}
function SelectionRow({
  domain,
  item,
  value,
  included,
  busy,
  onChange,
  onInclude,
}: {
  domain: string;
  item: DirectoryObservationItem;
  value: BatchClassification;
  included: boolean;
  busy: boolean;
  onChange: (path: string, value: BatchClassification) => void;
  onInclude: (path: string, included: boolean) => void;
}) {
  const { t } = useTranslation();
  const change = (next: BatchClassification) =>
    onChange(item.relative_path, next);
  const toggle = (e: ChangeEvent<HTMLInputElement>) =>
    onInclude(item.relative_path, e.target.checked);
  return (
    <li className="space-y-2 rounded-md border border-border-button p-3">
      <label className="flex items-center gap-2 break-all">
        <input
          type="checkbox"
          checked={included}
          onChange={toggle}
          disabled={busy}
        />
        {item.relative_path}
      </label>
      <p>
        {t('knowledgeDirectory.states.' + item.state)} · {item.change_kind}
        {item.reason
          ? ' · ' + t('knowledgeDirectory.reasons.' + item.reason)
          : ''}
      </p>
      {included && item.eligible && (
        <details>
          <summary>{t('knowledgeBatch.itemClassification')}</summary>
          <fieldset disabled={busy}>
            <ClassificationFields
              domainId={domain}
              value={value}
              onChange={change}
            />
          </fieldset>
        </details>
      )}
    </li>
  );
}
export function BatchSelection({
  domain,
  flow,
}: {
  domain: string;
  flow: ReturnType<typeof useBatchFlow>;
}) {
  const { t } = useTranslation(),
    registration = flow.registration!,
    observation = flow.observation;
  const [selection, setSelection] = useState(
    observation?.selected_paths.join('\n') || '',
  );
  const [classes, setClasses] = useState<Record<string, BatchClassification>>(
    () =>
      Object.fromEntries(
        (flow.retryParent?.items || []).map((i) => [
          i.relative_path,
          i.classification,
        ]),
      ),
  );
  const [included, setIncluded] = useState<Record<string, boolean>>({});
  const paths = selection.split(/\r?\n/).filter(Boolean);
  const chosen =
    observation?.items.filter((i) => included[i.relative_path] !== false) || [];
  const handleSelection = (e: ChangeEvent<HTMLTextAreaElement>) => {
    flow.changeSelection();
    setSelection(e.target.value);
    setIncluded({});
  };
  const handleObserve = (e: FormEvent) => {
    e.preventDefault();
    setIncluded({});
    void flow.observe([...paths].sort(compareDirectoryPaths));
  };
  const handleClassification = (path: string, value: BatchClassification) => {
    flow.invalidate();
    setClasses((current) => ({ ...current, [path]: value }));
  };
  const handleInclude = (path: string, value: boolean) => {
    flow.invalidate();
    setIncluded((current) => ({ ...current, [path]: value }));
  };
  const handlePrepare = (e: FormEvent) => {
    e.preventDefault();
    if (observation)
      void flow.prepare({
        observation_id: observation.observation_id,
        observation_sha256: observation.observation_sha256,
        selected_items: chosen.map((i) => ({
          relative_path: i.relative_path,
          classification:
            classes[i.relative_path] || registration.default_classification,
        })),
      });
  };
  return (
    <section className="space-y-4">
      <p>
        {t('knowledgeDirectory.registered')}: {registration.name}
      </p>
      {flow.retryParent && (
        <p>
          {t('knowledgeBatch.retryParent')}: {flow.retryParent.task.task_id}
        </p>
      )}
      <form onSubmit={handleObserve} className="space-y-2">
        <label className="block">
          {t('knowledgeDirectory.exactSelection')}
          <Textarea
            rows={4}
            disabled={flow.busy}
            value={selection}
            onChange={handleSelection}
          />
        </label>
        <p>{t('knowledgeDirectory.selectedCount', { count: paths.length })}</p>
        <Button
          type="submit"
          disabled={
            flow.busy ||
            !validDirectoryData('ObservationRequest', { selected_paths: paths })
          }
        >
          {t('knowledgeDirectory.observe')}
        </Button>
      </form>
      {observation && (
        <form onSubmit={handlePrepare} className="space-y-3">
          <p role="status">
            {t(
              observation.complete
                ? 'knowledgeDirectory.observationComplete'
                : 'knowledgeDirectory.observationIncomplete',
            )}
          </p>
          <p>{t('knowledgeBatch.explicitSubset')}</p>
          <p>{t('knowledgeBatch.replacementHint')}</p>
          <ul className="space-y-3">
            {observation.items.map((item) => (
              <SelectionRow
                key={item.relative_path}
                domain={domain}
                item={item}
                value={
                  classes[item.relative_path] ||
                  registration.default_classification
                }
                included={included[item.relative_path] !== false}
                busy={flow.busy}
                onChange={handleClassification}
                onInclude={handleInclude}
              />
            ))}
          </ul>
          <Button
            type="submit"
            disabled={
              flow.busy ||
              !observation.complete ||
              !chosen.length ||
              chosen.some((i) => !i.eligible)
            }
          >
            {t('knowledgeBatch.prepare')}
          </Button>
        </form>
      )}
    </section>
  );
}

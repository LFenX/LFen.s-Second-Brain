import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import service from '@/services/knowledge-web-service';
import {
  scopedWeb,
  useWebResource,
  webScopeFailure,
} from '@/hooks/use-knowledge-web';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import type {
  WebEntry,
  WebEntrySummary,
} from '@/interfaces/database/knowledge-web';
import type { WebSelection } from '@/interfaces/request/knowledge-web';
import { ClassificationFields } from './fields';
import WebReading from './web-reading';
import { WebFailure } from './web-fields';
import type { WebFlow } from './use-web-flow';

function EntryChoice({
  entry,
  selection,
  domain,
  disabled,
  onSelect,
  onRead,
  onClassification,
}: {
  entry: WebEntrySummary;
  selection?: WebSelection;
  domain: string;
  disabled: boolean;
  onSelect: (entry: WebEntrySummary, checked: boolean) => void;
  onRead: (entry: WebEntrySummary) => void;
  onClassification: (value: WebSelection) => void;
}) {
  const { t } = useTranslation();
  const select = (event: ChangeEvent<HTMLInputElement>) =>
      onSelect(entry, event.target.checked),
    read = () => onRead(entry),
    classify = (classification: WebSelection['classification']) => {
      if (selection) onClassification({ ...selection, classification });
    };
  return (
    <li className="space-y-2 rounded-lg border border-border-button p-3">
      <label className="flex gap-2">
        <input
          type="checkbox"
          checked={!!selection}
          disabled={disabled || !entry.eligible}
          onChange={select}
        />
        <span>{entry.title || entry.entry_key}</span>
      </label>
      <p className="text-xs text-text-secondary">
        {t('knowledgeWeb.completeness.' + entry.body_completeness)} ·{' '}
        {entry.identity_strength} ·{' '}
        {entry.decoded_byte_length + entry.reading_byte_length} B
        {entry.ineligible_reason ? ' · ' + entry.ineligible_reason : ''}
      </p>
      <Button
        type="button"
        variant="outline"
        onClick={read}
        disabled={disabled}
      >
        {t('knowledgeWeb.inspectEntry')}
      </Button>
      {selection && (
        <fieldset disabled={disabled}>
          <ClassificationFields
            domainId={domain}
            value={selection.classification}
            onChange={classify}
          />
          <p className="break-all text-xs">
            {t('knowledgeImport.expectedRevision')}:{' '}
            {selection.expected_revision_id || t('knowledgeImport.firstImport')}
          </p>
        </fieldset>
      )}
    </li>
  );
}
export default function WebObservationPanel({ flow }: { flow: WebFlow }) {
  const { t } = useTranslation(),
    obs = flow.observation!,
    session = getKnowledgeSessionScope();
  const [selection, setSelection] = useState<Record<string, WebSelection>>({}),
    [visible, setVisible] = useState(20),
    [reading, setReading] = useState<WebEntry | null>(null),
    [error, setError] = useState<unknown>(null),
    [busy, setBusy] = useState(false);
  const query = useWebResource(
    flow.scope,
    'observation',
    obs.capture_id,
    (signal) => service.observation(flow.scope, obs.capture_id, signal),
    flow.scopeFailure,
  );
  const current = useRef(flow);
  current.current = flow;
  const adopt = query.adopt;
  useEffect(() => {
    try {
      adopt(obs);
    } catch (issue) {
      current.current.scopeFailure(issue);
    }
  }, [obs, adopt]);
  useEffect(() => {
    if (query.data && query.data !== current.current.observation)
      void current.current
        .acceptObservation(query.data)
        .catch(current.current.scopeFailure);
  }, [query.data]);
  const choose = (entry: WebEntrySummary, checked: boolean) => {
    setError(null);
    if (checked && Object.keys(selection).length >= 32) {
      setError(Error('Selection limit'));
      return;
    }
    const next = { ...selection };
    if (checked)
      next[entry.entry_key] = {
        entry_key: entry.entry_key,
        classification: flow.registration!.default_classification,
        expected_revision_id: entry.current_revision_id,
      };
    else delete next[entry.entry_key];
    setSelection(next);
  };
  const classify = (value: WebSelection) =>
    setSelection({ ...selection, [value.entry_key]: value });
  const inspect = async (entry: WebEntrySummary) => {
    setBusy(true);
    setError(null);
    try {
      const result = await scopedWeb(session, () =>
        service.entry(flow.scope, obs.capture_id, entry.entry_key),
      );
      setReading(result.data);
    } catch (issue) {
      setReading(null);
      setError(issue);
      if (webScopeFailure(issue)) flow.scopeFailure(issue);
    } finally {
      setBusy(false);
    }
  };
  const refreshTargets = async () => {
    setBusy(true);
    setError(null);
    try {
      const next = { ...selection };
      for (const selected of Object.values(next)) {
        const result = await scopedWeb(session, () =>
          service.target(flow.scope, selected.entry_key),
        );
        next[selected.entry_key] = {
          ...selected,
          expected_revision_id: result.data.current_revision_id,
        };
      }
      setSelection(next);
    } catch (issue) {
      setError(issue);
      if (webScopeFailure(issue)) flow.scopeFailure(issue);
    } finally {
      setBusy(false);
    }
  };
  const createPreview = () => {
      void flow.createPreview(Object.values(selection));
    },
    more = () => setVisible((value) => value + 20),
    recheck = () => {
      void query.recheck().then((result) => {
        if (result.data && !result.error)
          void flow.acceptObservation(result.data).catch(flow.scopeFailure);
      });
    };
  return (
    <section aria-label={t('knowledgeWeb.observation')} className="space-y-4">
      <h2 className="font-semibold">
        2 · {t('knowledgeWeb.observation')} ·{' '}
        {t('knowledgeWeb.state.' + obs.state)}
      </h2>
      <p className="text-sm">
        {t('knowledgeWeb.observationCounts', {
          input: obs.input_entry_count ?? 0,
          entries: obs.entry_count ?? 0,
          eligible: obs.eligible_count ?? 0,
          duplicates: obs.duplicate_count ?? 0,
        })}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.selectionHint')}
      </p>
      {obs.warnings.map((warning, i) => (
        <p key={i} className="text-sm">
          {warning}
        </p>
      ))}
      {(error || query.error) && <WebFailure error={error || query.error} />}
      <Button
        variant="outline"
        onClick={recheck}
        disabled={flow.busy || query.isFetching}
      >
        {t('knowledgeWeb.recheck')}
      </Button>
      {query.pollingStopped && <p>{t('knowledgeWeb.pollingStopped')}</p>}
      {obs.state === 'ready' && (
        <>
          <ul className="space-y-3">
            {flow.entries.slice(0, visible).map((entry) => (
              <EntryChoice
                key={entry.entry_key}
                entry={entry}
                selection={selection[entry.entry_key]}
                domain={flow.scope.domain}
                disabled={flow.busy || busy || !!flow.unknown}
                onSelect={choose}
                onRead={inspect}
                onClassification={classify}
              />
            ))}
          </ul>
          {visible < flow.entries.length && (
            <Button variant="outline" onClick={more}>
              {t('knowledgeCatalogue.more')}
            </Button>
          )}
          <p>
            {t('knowledgeWeb.selectedCount', {
              count: Object.keys(selection).length,
            })}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={refreshTargets}
              disabled={
                flow.busy ||
                busy ||
                !Object.keys(selection).length ||
                !!flow.unknown
              }
            >
              {t('knowledgeWeb.refreshTargets')}
            </Button>
            <Button
              onClick={createPreview}
              disabled={
                flow.busy ||
                busy ||
                !Object.keys(selection).length ||
                !!flow.unknown
              }
            >
              {t('knowledgeWeb.createPreview')}
            </Button>
          </div>
          {reading && (
            <WebReading
              key={reading.entry_key}
              target={{ scope: flow.scope, entry: reading }}
              capture={flow.capture!}
              onScopeFailure={flow.scopeFailure}
            />
          )}
        </>
      )}
    </section>
  );
}

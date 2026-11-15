import {
  useLayoutEffect,
  useState,
  type ChangeEvent,
  type FormEvent,
  type MouseEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AccessFailure } from '@/components/knowledge-access-feedback';
import { useSharedAccessReader } from '@/hooks/use-knowledge-access';
import type {
  AccessActor,
  AccessMode,
  AccessModuleSummary,
  AccessReadData,
} from '@/interfaces/database/knowledge-access';

function TypedSummary({ value }: { value: AccessReadData['typed_context'] }) {
  const { t } = useTranslation();
  return (
    <section
      aria-label={t('knowledgeAccess.structureSummary')}
      className="space-y-1 text-sm text-text-secondary"
    >
      <h3>{t('knowledgeAccess.structureSummary')}</h3>
      {value.members.map((member, index) => (
        <p key={member.node_id + ':' + index}>
          {t('knowledgeAccess.structureItem', { index: index + 1 })} ·{' '}
          {member.kind} ·{' '}
          {t(
            member.text_status === 'empty'
              ? 'knowledgeAccess.emptyMember'
              : 'knowledgeAccess.textMember',
          )}
        </p>
      ))}
      {!value.members_complete && <p>{t('knowledgeAccess.summaryPartial')}</p>}
    </section>
  );
}

export function SharedKnowledgeReader({
  actor,
  module,
}: {
  actor: AccessActor;
  module: AccessModuleSummary;
}) {
  const { t } = useTranslation();
  const reader = useSharedAccessReader(actor, module);
  const [mode, setMode] = useState<AccessMode>('read');
  const [query, setQuery] = useState('');
  const [subset, setSubset] = useState<string[] | null>(null);
  const view = reader.view;
  const inputBoundary = JSON.stringify([
    actor.auth,
    actor.user,
    actor.tenant,
    actor.session,
    module.domain_id,
    module.module_id,
    module.version,
    module.enabled,
    module.modes,
    view?.view_id ?? null,
  ]);
  useLayoutEffect(() => {
    setQuery('');
    setSubset(null);
  }, [inputBoundary]);
  function handleMode(event: ChangeEvent<HTMLSelectElement>) {
    reader.clear();
    setQuery('');
    setSubset(null);
    setMode(event.target.value as AccessMode);
  }
  function handleOpen() {
    setQuery('');
    setSubset(null);
    void reader.inspect(mode);
  }
  function handleClose() {
    reader.clear();
    setQuery('');
    setSubset(null);
  }
  function handleQuery(event: ChangeEvent<HTMLInputElement>) {
    reader.forgetOutput();
    setQuery(event.target.value);
  }
  function handleSearch(event: FormEvent) {
    event.preventDefault();
    void reader.search(query);
  }
  function handleTarget(event: MouseEvent<HTMLButtonElement>) {
    if (!view) return;
    const index = Number(event.currentTarget.dataset.target);
    void reader.read({
      kind: 'unit',
      view_id: view.view_id,
      target_index: index,
      unit_ordinal: 0,
    });
  }
  function handleSubset(event: ChangeEvent<HTMLInputElement>) {
    if (!view) return;
    reader.forgetOutput();
    const source = event.target.value;
    const chosen =
      subset ?? view.targets.map((entry) => entry.target.source.source_id);
    setSubset(
      event.target.checked
        ? [...chosen, source]
        : chosen.filter((id) => id !== source),
    );
  }
  function handleApplySubset() {
    const selected = subset;
    setSubset(null);
    void reader.inspect(mode, selected);
  }
  function handleNext() {
    const locator = reader.evidence?.unit_locator;
    if (!view || !locator || locator.next_unit_ordinal === null) return;
    void reader.read({
      kind: 'unit',
      view_id: view.view_id,
      target_index: locator.target_index,
      unit_ordinal: locator.next_unit_ordinal,
    });
  }
  function handleReread() {
    if (reader.evidence) void reader.reread(reader.evidence.evidence_ref);
  }
  function handleHit(event: MouseEvent<HTMLButtonElement>) {
    const hit = reader.results?.hits[Number(event.currentTarget.dataset.hit)];
    if (hit) void reader.reread(hit.evidence_ref);
  }
  return (
    <section
      className="space-y-4 rounded-lg border border-border-button p-4"
      aria-label={module.name}
    >
      <h2 className="text-lg font-medium">{module.name}</h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.fixedContent')}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <label className="space-x-2">
          <span>{t('knowledgeAccess.mode')}</span>
          <select
            value={mode}
            onChange={handleMode}
            disabled={reader.busy}
            className="rounded border border-border-button bg-bg-base p-2"
          >
            <option value="read">{t('knowledgeAccess.readOnly')}</option>
            {module.modes.includes('read_search') && (
              <option value="read_search">
                {t('knowledgeAccess.readSearch')}
              </option>
            )}
          </select>
        </label>
        <Button onClick={handleOpen} disabled={reader.busy}>
          {t('knowledgeAccess.openCatalogue')}
        </Button>
        {view && (
          <Button variant="outline" onClick={handleClose}>
            {t('knowledgeAccess.closeContent')}
          </Button>
        )}
      </div>
      <AccessFailure error={reader.error} />
      {reader.busy && <p role="status">{t('knowledgeAccess.loading')}</p>}
      {view && (
        <>
          <p>
            {t('knowledgeAccess.catalogueCount', {
              sources: view.source_count,
              units: view.unit_count,
            })}
          </p>
          <div className="space-y-2">
            {view.targets.map((entry, index) => (
              <div
                key={entry.target.source.source_id}
                className="flex flex-wrap items-center gap-3"
              >
                <label className="space-x-2">
                  <input
                    type="checkbox"
                    value={entry.target.source.source_id}
                    checked={
                      subset === null ||
                      subset.includes(entry.target.source.source_id)
                    }
                    onChange={handleSubset}
                    disabled={reader.busy}
                  />
                  <span>
                    {t('knowledgeAccess.contentNumber', {
                      index: index + 1,
                      count: entry.unit_count,
                    })}
                  </span>
                </label>
                <Button
                  variant="outline"
                  data-target={index}
                  disabled={
                    reader.busy || subset !== null || entry.unit_count === 0
                  }
                  onClick={handleTarget}
                >
                  {t('knowledgeAccess.readFirst')}
                </Button>
              </div>
            ))}
          </div>
          {subset !== null && (
            <Button
              disabled={reader.busy || subset.length === 0}
              onClick={handleApplySubset}
            >
              {t('knowledgeAccess.applySubset')}
            </Button>
          )}
          {view.mode === 'read_search' && subset === null && (
            <form
              className="flex gap-2"
              onSubmit={handleSearch}
              aria-label={t('knowledgeAccess.search')}
            >
              <Input
                aria-label={t('knowledgeAccess.query')}
                required
                value={query}
                onChange={handleQuery}
                maxLength={4096}
                disabled={reader.busy}
              />
              <Button type="submit" disabled={reader.busy || !query.trim()}>
                {t('knowledgeAccess.search')}
              </Button>
            </form>
          )}
          {reader.results && (
            <section
              aria-label={t('knowledgeAccess.searchResults')}
              className="space-y-3"
            >
              <p>
                {t('knowledgeAccess.hitCount', {
                  count: reader.results.returned_count,
                })}
              </p>
              {reader.results.hits.map((hit, index) => (
                <article
                  key={hit.evidence_ref.evidence_id}
                  className="space-y-2 rounded border border-border-button p-3"
                >
                  <p>
                    {hit.citation.display_name} · {hit.citation.position_label}
                  </p>
                  <pre className="whitespace-pre-wrap break-words font-sans">
                    {hit.text}
                  </pre>
                  <Button
                    variant="outline"
                    data-hit={index}
                    onClick={handleHit}
                    disabled={reader.busy}
                  >
                    {t('knowledgeAccess.readCitation')}
                  </Button>
                </article>
              ))}
            </section>
          )}
          {reader.evidence && (
            <article
              aria-label={t('knowledgeAccess.evidence')}
              className="space-y-3 rounded border border-border-button p-4"
            >
              <h3>{reader.evidence.citation.display_name}</h3>
              <p className="text-sm text-text-secondary">
                {reader.evidence.citation.position_label}
              </p>
              <pre className="whitespace-pre-wrap break-words font-sans">
                {reader.evidence.text}
              </pre>
              <TypedSummary value={reader.evidence.typed_context} />
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  onClick={handleReread}
                  disabled={reader.busy}
                >
                  {t('knowledgeAccess.reread')}
                </Button>
                {reader.evidence.unit_locator.next_unit_ordinal !== null && (
                  <Button
                    variant="outline"
                    onClick={handleNext}
                    disabled={reader.busy}
                  >
                    {t('knowledgeAccess.nextUnit')}
                  </Button>
                )}
              </div>
            </article>
          )}
        </>
      )}
    </section>
  );
}

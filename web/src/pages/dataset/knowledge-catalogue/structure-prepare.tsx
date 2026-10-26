import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  useStructureManifestHead,
  useStructureManifest,
} from '@/hooks/use-knowledge-structure';
import type { BatchManifestEntry } from '@/interfaces/database/knowledge-batch';
import type {
  StructureAlias,
  StructureTask,
} from '@/interfaces/database/knowledge-structure';
import type { StructurePrepared } from '@/services/knowledge-structure-service';
import {
  structureParser,
  validStructureAliases,
} from '@/utils/knowledge-structure-schema';
import {
  structureRejected,
  type StructureProjectScope,
} from '@/utils/knowledge-structure';
import type { StructureFlow } from './use-structure-flow';
import { CatalogueLoading, LoadMore } from './state';
import { StructureFailure } from './structure-state';

export function parseStructureAliases(text: string): StructureAlias[] {
  const aliases =
    text === ''
      ? []
      : text.split('\n').map((line) => {
          const equals = line.indexOf('=');
          if (equals < 1) throw Error('invalid_alias');
          return {
            prefix: line.slice(0, equals),
            target: line.slice(equals + 1),
          };
        });
  if (!validStructureAliases(aliases)) throw Error('invalid_alias');
  return aliases;
}
function StructureMemberChoice({
  item,
  selected,
  onToggle,
}: {
  item: BatchManifestEntry;
  selected: boolean;
  onToggle: (item: BatchManifestEntry) => void;
}) {
  const { t } = useTranslation(),
    toggle = () => onToggle(item),
    parser = structureParser(item.relative_path);
  return (
    <li className="space-y-1 rounded border border-border-button p-2">
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={selected}
          disabled={!parser || item.byte_length > 2097152}
          onChange={toggle}
        />
        <span className="break-all">{item.relative_path}</span>
      </label>
      <p className="break-all text-xs text-text-secondary">
        {parser ?? t('knowledgeStructure.unsupported')} · {item.byte_length} B ·{' '}
        {item.classification.component_side} / {item.classification.language}
      </p>
      <p className="break-all text-xs text-text-secondary">
        {item.source_revision_id} · {item.sha256}
      </p>
    </li>
  );
}
function FixedStructureSelection({
  scope,
  manifest,
  flow,
  onTask,
}: {
  scope: StructureProjectScope;
  manifest: string;
  flow: StructureFlow;
  onTask: (task: StructureTask) => void;
}) {
  const { t } = useTranslation(),
    query = useStructureManifest(scope, manifest),
    [selected, setSelected] = useState<BatchManifestEntry[]>([]),
    [aliasText, setAliasText] = useState(''),
    [prepared, setPrepared] = useState<StructurePrepared | null>(null),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null),
    [preparing, setPreparing] = useState(false),
    preparationRevision = useRef(0);
  useEffect(() => {
    preparationRevision.current++;
    setPrepared(null);
    setAck(false);
    setError(null);
  }, [flow.preparationEpoch]);
  const clear = () => {
    preparationRevision.current++;
    setPrepared(null);
    setAck(false);
    setError(null);
  };
  const toggle = (item: BatchManifestEntry) => {
    clear();
    setSelected((items) =>
      items.some((value) => value.source_id === item.source_id)
        ? items.filter((value) => value.source_id !== item.source_id)
        : [...items, item],
    );
  };
  const aliasesChanged = (event: ChangeEvent<HTMLTextAreaElement>) => {
    clear();
    setAliasText(event.target.value);
  };
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const more = () => {
    void query.fetchNextPage();
  };
  const retry = () => {
    clear();
    void query.refetch();
  };
  const prepare = async (event: FormEvent) => {
    event.preventDefault();
    clear();
    const revision = preparationRevision.current;
    setPreparing(true);
    try {
      const result = await flow.prepare(
        scope,
        manifest,
        selected,
        parseStructureAliases(aliasText),
      );
      if (revision === preparationRevision.current) setPrepared(result);
    } catch (issue) {
      if (revision === preparationRevision.current) setError(issue);
    } finally {
      setPreparing(false);
    }
  };
  const submit = async () => {
    if (!ack || !prepared || !flow.ready || flow.busy) return;
    const request = prepared.request;
    setAck(false);
    setPrepared(null);
    try {
      const result = await flow.submit({
        kind: 'task',
        scope,
        body: {
          ...request,
          confirmed: true,
          idempotency_key: crypto.randomUUID(),
        },
      });
      if (!structureRejected(result) && 'state' in result) onTask(result);
    } catch (issue) {
      setError(issue);
    }
  };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={retry} />;
  if (!query.data) return <CatalogueLoading />;
  const bytes = selected.reduce((sum, item) => sum + item.byte_length, 0);
  return (
    <section className="space-y-3">
      <p className="break-all text-xs">
        {t('knowledgeStructure.fixedManifest')}: {manifest}
      </p>
      <form onSubmit={prepare} className="space-y-3">
        <p>{t('knowledgeStructure.selectionLimits')}</p>
        <ul className="max-h-80 space-y-2 overflow-auto">
          {query.data.pages
            .flatMap((page) => page.items)
            .map((item) => (
              <StructureMemberChoice
                key={item.source_id}
                item={item}
                selected={selected.some(
                  (value) => value.source_id === item.source_id,
                )}
                onToggle={toggle}
              />
            ))}
        </ul>
        <LoadMore
          more={query.hasNextPage}
          loading={query.isFetchingNextPage}
          onMore={more}
        />
        <p>
          {t('knowledgeStructure.selected', { count: selected.length, bytes })}
        </p>
        <label className="block space-y-1">
          <span>{t('knowledgeStructure.aliases')}</span>
          <textarea
            aria-label={t('knowledgeStructure.aliases')}
            className="min-h-20 w-full rounded border border-border-button bg-bg-base p-2"
            value={aliasText}
            onChange={aliasesChanged}
          />
        </label>
        <p className="text-xs text-text-secondary">
          {t('knowledgeStructure.aliasRules')}
        </p>
        <Button
          type="submit"
          disabled={
            preparing ||
            flow.busy ||
            !flow.ready ||
            selected.length < 1 ||
            selected.length > 32 ||
            bytes > 16777216
          }
        >
          {t('knowledgeStructure.prepare')}
        </Button>
      </form>
      {prepared && (
        <div className="space-y-3 rounded border border-border-button p-3">
          <p>{t('knowledgeStructure.preparedHint')}</p>
          <p className="break-all text-xs">
            {t('knowledgeStructure.expectedView')}:{' '}
            {prepared.request.expected_active_view_id ?? '∅'} ·{' '}
            {t('knowledgeStructure.versions', {
              foundation: prepared.request.expected_policy_version,
              structure: prepared.request.expected_structure_policy_version,
            })}
          </p>
          <ul className="space-y-1">
            {prepared.request.members.map((member) => (
              <li key={member.source_id} className="break-all text-xs">
                {
                  prepared.entries.find(
                    (item) => item.source_id === member.source_id,
                  )?.relative_path
                }{' '}
                · {member.parser} · {t('knowledgeStructure.generation')}:{' '}
                {member.lifecycle_version} · {member.source_revision_id}
              </li>
            ))}
          </ul>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={ack} onChange={acknowledge} />
            {t('knowledgeStructure.confirmTask')}
          </label>
          <Button
            type="button"
            disabled={!ack || flow.busy || !flow.ready}
            onClick={submit}
          >
            {t('knowledgeStructure.createTask')}
          </Button>
        </div>
      )}
      {!!error && <StructureFailure error={error} />}
    </section>
  );
}
export function StructurePrepare({
  scope,
  flow,
  onTask,
}: {
  scope: StructureProjectScope;
  flow: StructureFlow;
  onTask: (task: StructureTask) => void;
}) {
  const { t } = useTranslation(),
    query = useStructureManifestHead(scope),
    [generation, setGeneration] = useState(0);
  const refresh = () => {
    setGeneration((value) => value + 1);
    void query.refetch();
  };
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeStructure.prepare')}</h3>
      <Button type="button" variant="outline" onClick={refresh}>
        {t('knowledgeStructure.refreshPreparation')}
      </Button>
      {query.isError ? (
        <StructureFailure error={query.error} onRetry={refresh} />
      ) : !query.data ? (
        <CatalogueLoading />
      ) : query.data.manifest_id ? (
        <FixedStructureSelection
          key={query.data.manifest_id + ':' + generation}
          scope={scope}
          manifest={query.data.manifest_id}
          flow={flow}
          onTask={onTask}
        />
      ) : (
        <p>{t('knowledgeStructure.noManifest')}</p>
      )}
    </section>
  );
}

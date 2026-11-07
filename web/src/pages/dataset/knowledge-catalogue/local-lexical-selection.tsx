import {
  useContext,
  useEffect,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  useKnowledgeSource,
  useKnowledgeSources,
} from '@/hooks/use-knowledge-foundation';
import { LocalLexicalKeys } from '@/hooks/use-knowledge-local-lexical';
import type { KnowledgeSource } from '@/interfaces/database/knowledge-foundation';
import type {
  ContentTarget,
  ContentNode,
} from '@/interfaces/database/knowledge-consumption';
import type { ContentScope } from '@/interfaces/request/knowledge-consumption';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import {
  useStructureHead,
  useStructureView,
} from '@/hooks/use-knowledge-structure';
import {
  useDocumentHead,
  useDocumentRepresentation,
} from '@/hooks/use-knowledge-document';
import { useContentScopeError } from '@/hooks/use-knowledge-consumption';
import {
  contentStructureTarget,
  contentDocumentTarget,
} from '@/utils/knowledge-consumption';
import { ProjectPicker } from '../knowledge-import/batch-manifest';
import { DocumentOriginalList } from './document-prepare';
import type { LocalLexicalTarget } from '@/interfaces/database/knowledge-local-lexical';
import type { LocalLexicalScope } from '@/interfaces/request/knowledge-local-lexical';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { LocalLexicalId } from '@/utils/knowledge-local-lexical';
import {
  CatalogueFailure,
  CatalogueLoading,
  CatalogueScopeFailure,
  LoadMore,
} from './state';

export type LocalLexicalChoice = {
  target: LocalLexicalTarget;
  path: string;
  currentness: 'current' | 'historical' | 'unknown';
};
export function LocalLexicalFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation(),
    client = useQueryClient(),
    onScopeFailure = useContext(CatalogueScopeFailure);
  useEffect(() => {
    if (
      error instanceof KnowledgeRequestError &&
      [401, 403].includes(error.status)
    ) {
      client.removeQueries({ queryKey: LocalLexicalKeys.root() });
      onScopeFailure?.(error);
    }
  }, [client, error, onScopeFailure]);
  const code =
    error instanceof KnowledgeRequestError
      ? error.code
      : 'local_lexical_invalid_data';
  return (
    <div
      role="alert"
      className="space-y-2 rounded border border-border-button p-3 text-text-secondary"
    >
      <p>{t('knowledgeLocalLexical.failed')}</p>
      <p>{t('knowledgeLocalLexical.error_' + code, { defaultValue: code })}</p>
      {error instanceof KnowledgeRequestError && error.requestId && (
        <p className="break-all text-xs">{error.requestId}</p>
      )}
      {onRetry && (
        <Button type="button" variant="outline" onClick={onRetry}>
          {t('knowledgeCatalogue.retry')}
        </Button>
      )}
    </div>
  );
}
function C1Choice({
  source,
  domain,
  onAdd,
  disabled,
}: {
  source: KnowledgeSource;
  domain: string;
  onAdd: (choice: LocalLexicalChoice) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const add = () =>
    onAdd({
      path: source.relative_path,
      currentness: source.is_current ? 'current' : 'historical',
      target: {
        source_ref: {
          domain_id: source.domain_id,
          source_id: source.source_id,
          source_revision_id: source.source_revision_id,
          source_sha256: source.sha256,
        },
        representation_ref: {
          kind: 'source_revision',
          id: source.source_revision_id,
          producer_sha256: null,
        },
      },
    });
  const valid = source.domain_id === domain;
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p className="break-all">{source.relative_path}</p>
      <p className="break-all text-xs">
        {source.source_revision_id} ·{' '}
        {t(
          'knowledgeLocalLexical.currentness_' +
            (source.is_current ? 'current' : 'historical'),
        )}{' '}
        · {source.byte_length} B
      </p>
      {!valid && <p>{t('knowledgeLocalLexical.error_unit_oversize')}</p>}
      <Button
        type="button"
        variant="outline"
        disabled={disabled || !valid}
        onClick={add}
      >
        {t('knowledgeLocalLexical.addC1')}
      </Button>
    </li>
  );
}
function CurrentSources({ scope, group, onAdd, disabled }: PickerProps) {
  const query = useKnowledgeSources(scope.domain, group);
  const more = () => {
      void query.fetchNextPage();
    },
    reload = () => {
      void query.refetch();
    };
  if (query.isError)
    return <LocalLexicalFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  return (
    <>
      <ul className="space-y-2">
        {query.data.pages
          .flatMap((page) => page.items)
          .map((source) => (
            <C1Choice
              key={source.source_revision_id}
              source={source}
              domain={scope.domain}
              onAdd={onAdd}
              disabled={disabled}
            />
          ))}
      </ul>
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={more}
      />
    </>
  );
}
function FixedRevision({
  scope,
  revision,
  onAdd,
  disabled,
}: PickerProps & { revision: string }) {
  const query = useKnowledgeSource(scope.domain, revision),
    reload = () => {
      void query.refetch();
    };
  if (query.isError)
    return <LocalLexicalFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  if (
    query.data.source_revision_id !== revision ||
    query.data.domain_id !== scope.domain
  )
    return <LocalLexicalFailure error={Error('identity mismatch')} />;
  return (
    <ul>
      <C1Choice
        source={query.data}
        domain={scope.domain}
        onAdd={onAdd}
        disabled={disabled}
      />
    </ul>
  );
}
type PickerProps = {
  scope: LocalLexicalScope;
  group?: string;
  onAdd: (choice: LocalLexicalChoice) => void;
  disabled: boolean;
};
export function LocalLexicalPicker(props: PickerProps) {
  const { t } = useTranslation(),
    [mode, setMode] = useState<
      'current' | 'revision' | 'structure' | 'document'
    >('current'),
    [revisionInput, setRevisionInput] = useState(''),
    [revision, setRevision] = useState('');
  const current = () => setMode('current'),
    history = () => setMode('revision'),
    structure = () => setMode('structure'),
    document = () => setMode('document'),
    editRevision = (event: ChangeEvent<HTMLInputElement>) =>
      setRevisionInput(event.target.value),
    clear = () => undefined,
    choose = (value: { target: ContentTarget; label: string }) => {
      if (!props.disabled)
        props.onAdd({
          target: value.target,
          path: value.label,
          currentness: 'unknown',
        });
    };
  const lookupRevision = (event: FormEvent) => {
    event.preventDefault();
    if (LocalLexicalId.test(revisionInput)) setRevision(revisionInput);
  };
  return (
    <section
      className="space-y-3"
      aria-label={t('knowledgeLocalLexical.chooseSources')}
    >
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={current}
          aria-pressed={mode === 'current'}
        >
          {t('knowledgeLocalLexical.currentSources')}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={history}
          aria-pressed={mode === 'revision'}
        >
          {t('knowledgeLocalLexical.fixedRevision')}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={structure}
          aria-pressed={mode === 'structure'}
        >
          {t('knowledgeLocalLexical.structureView')}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={document}
          aria-pressed={mode === 'document'}
        >
          {t('knowledgeLocalLexical.documentView')}
        </Button>
      </div>
      {mode === 'current' && <CurrentSources {...props} />}
      {mode === 'revision' && (
        <>
          <form onSubmit={lookupRevision} className="flex flex-wrap gap-2">
            <Input
              aria-label={t('knowledgeLocalLexical.revisionId')}
              value={revisionInput}
              onChange={editRevision}
              maxLength={32}
            />
            <Button
              type="submit"
              variant="outline"
              disabled={!LocalLexicalId.test(revisionInput)}
            >
              {t('knowledgeLocalLexical.openRevision')}
            </Button>
          </form>
          {revision && (
            <FixedRevision key={revision} {...props} revision={revision} />
          )}
        </>
      )}
      <fieldset disabled={props.disabled}>
        {mode === 'structure' && (
          <StructureChoice
            scope={props.scope}
            onChoose={choose}
            onClear={clear}
          />
        )}
        {mode === 'document' && (
          <OriginalChoice
            scope={props.scope}
            onChoose={choose}
            onClear={clear}
          />
        )}
      </fieldset>
      <p className="text-xs text-text-secondary">
        {t('knowledgeLocalLexical.parsedSelectionHint')}
      </p>
    </section>
  );
}

type Selection = { label: string; target: ContentTarget; autoRead?: boolean };
export function StructureChoice({
  scope,
  onChoose,
  onClear,
  readRefresh = 0,
}: {
  scope: ContentScope;
  onChoose: (value: Selection) => void;
  onClear: () => void;
  readRefresh?: number;
}) {
  const [project, setProject] = useState(''),
    [view, setView] = useState('');
  const choose = (id: string) => {
    onClear();
    setProject(id);
    setView('');
  };
  return (
    <div className="space-y-3">
      <ProjectPicker
        key={readRefresh}
        domain={scope.domain}
        onChoose={choose}
      />
      {project && (
        <StructureHeadChoice
          key={project + ':' + readRefresh}
          scope={scope}
          project={project}
          onView={setView}
        />
      )}
      {view && (
        <StructureMembers
          key={view}
          scope={scope}
          view={view}
          onChoose={onChoose}
        />
      )}
    </div>
  );
}
function StructureHeadChoice({
  scope,
  project,
  onView,
}: {
  scope: ContentScope;
  project: string;
  onView: (view: string) => void;
}) {
  const { t } = useTranslation(),
    query = useStructureHead({ ...scope, project });
  useContentScopeError(query.error);
  const reload = () => {
      void query.refetch();
    },
    open = () => {
      if (query.data?.view_id) onView(query.data.view_id);
    };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  return query.data.view_id ? (
    <Button type="button" variant="outline" onClick={open}>
      {t('knowledgeConsumption.fixCurrentView')}
    </Button>
  ) : (
    <p>{t('knowledgeConsumption.noStructure')}</p>
  );
}
function StructureMembers({
  scope,
  view,
  onChoose,
}: {
  scope: ContentScope;
  view: string;
  onChoose: (value: Selection) => void;
}) {
  const query = useStructureView({ ...scope, view });
  useContentScopeError(query.error);
  const reload = () => {
    void query.refetch();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  return (
    <ul className="space-y-2">
      {query.data.members.map((member) => (
        <ChoiceRow
          key={member.source_id}
          label={member.relative_path}
          onChoose={onChoose}
          target={contentStructureTarget(query.data!, member.source_id)}
        />
      ))}
    </ul>
  );
}
export function ChoiceRow({
  label,
  target,
  onChoose,
}: Selection & { onChoose: (value: Selection) => void }) {
  const choose = () => onChoose({ label, target });
  return (
    <li>
      <Button
        type="button"
        variant="outline"
        className="h-auto whitespace-normal text-left"
        onClick={choose}
      >
        {label}
      </Button>
    </li>
  );
}
export function OriginalChoice({
  scope,
  onChoose,
  onClear,
  readRefresh = 0,
}: {
  scope: ContentScope;
  onChoose: (value: Selection) => void;
  onClear: () => void;
  readRefresh?: number;
}) {
  const [original, setOriginal] = useState<OriginalRevision | null>(null);
  const choose = (value: OriginalRevision) => {
    onClear();
    setOriginal(value);
  };
  return (
    <div className="space-y-3">
      <DocumentOriginalList key={readRefresh} scope={scope} onSelect={choose} />
      {original && (
        <OriginalHeadChoice
          key={original.source_revision_id + ':' + readRefresh}
          scope={scope}
          original={original}
          onChoose={onChoose}
        />
      )}
    </div>
  );
}
function OriginalHeadChoice({
  scope,
  original,
  onChoose,
}: {
  scope: ContentScope;
  original: OriginalRevision;
  onChoose: (value: Selection) => void;
}) {
  const { t } = useTranslation(),
    query = useDocumentHead({ ...scope, source: original.source_id });
  useContentScopeError(query.error);
  const reload = () => {
    void query.refetch();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  if (!query.data.active_representation_id)
    return <p>{t('knowledgeConsumption.noStructure')}</p>;
  if (query.data.active_source_revision_id !== original.source_revision_id)
    return (
      <p role="status">{t('knowledgeConsumption.originalHeadDifferent')}</p>
    );
  return (
    <OriginalMetadataChoice
      scope={scope}
      original={original}
      head={query.data}
      representation={query.data.active_representation_id}
      onChoose={onChoose}
    />
  );
}
function OriginalMetadataChoice({
  scope,
  original,
  head,
  representation,
  onChoose,
}: {
  scope: ContentScope;
  original: OriginalRevision;
  head: import('@/interfaces/database/knowledge-document').DocumentHead;
  representation: string;
  onChoose: (value: Selection) => void;
}) {
  const query = useDocumentRepresentation({
    ...scope,
    source: original.source_id,
    revision: original.source_revision_id,
    sha: original.sha256,
    representation,
  });
  useContentScopeError(query.error);
  const reload = () => {
    void query.refetch();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  let target: ContentTarget;
  try {
    target = contentDocumentTarget(original, head, query.data);
  } catch (error) {
    return <CatalogueFailure error={error} onRetry={reload} />;
  }
  return (
    <ul>
      <ChoiceRow
        label={original.original_filename}
        target={target}
        onChoose={onChoose}
      />
    </ul>
  );
}

export function ContentAttributesView({ node }: { node: ContentNode }) {
  const { t } = useTranslation(),
    attributes = node.attributes.document;
  if (node.attributes.parser) return <ParserAttributesView node={node} />;
  if (node.kind === 'media')
    return <p>{t('knowledgeConsumption.mediaNotOcr')}</p>;
  if (attributes?.table)
    return (
      <p>
        {t('knowledgeConsumption.tableShape', {
          rows: attributes.table.row_count,
          cells: attributes.table.max_cells_per_row,
        })}
      </p>
    );
  if (attributes?.cell)
    return (
      <p>
        {t('knowledgeConsumption.cellPosition', {
          row: attributes.cell.row_index,
          cell: attributes.cell.cell_index,
        })}
      </p>
    );
  if (attributes?.list)
    return (
      <p>
        {t(
          attributes.list.ordered
            ? 'knowledgeDocument.orderedList'
            : 'knowledgeDocument.unorderedList',
        )}
      </p>
    );
  return null;
}

function ParserAttributesView({ node }: { node: ContentNode }) {
  const { t } = useTranslation(),
    parser = node.attributes.parser!;
  return (
    <section className="space-y-2">
      <p>{t('knowledgeConsumption.parserMetadataWarning')}</p>
      {parser.sheet && (
        <p>
          {t('knowledgeConsumption.sheet')}: {parser.sheet}
        </p>
      )}
      {parser.item_type === 'image' && (
        <p>{t('knowledgeConsumption.mediaNotOcr')}</p>
      )}
      {parser.alt_text !== null && (
        <p>
          {t('knowledgeConsumption.altText')}: {parser.alt_text}
        </p>
      )}
      {parser.table && <ParserTablePreview table={parser.table} />}
      <details>
        <summary>{t('knowledgeConsumption.producerMetadata')}</summary>
        <p>
          {t(
            parser.positions === null
              ? 'knowledgeConsumption.positionsUnknown'
              : 'knowledgeConsumption.positionsProducerOnly',
          )}
        </p>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(
            {
              item_type: parser.item_type,
              ck_type: parser.ck_type,
              positions: parser.positions,
              payload_present: parser.payload_present,
              text_assembly: parser.text_assembly,
            },
            null,
            2,
          )}
        </pre>
      </details>
    </section>
  );
}
function ParserTablePreview({
  table,
}: {
  table: import('@/interfaces/database/knowledge-consumption').ContentTableData;
}) {
  const { t } = useTranslation(),
    rows = table.grid.slice(0, 20),
    clipped =
      table.grid.length > 20 ||
      table.grid.some((row) => (row?.length ?? 0) > 12);
  return (
    <details>
      <summary>{t('knowledgeConsumption.parsedTable')}</summary>
      {table.caption && <p>{table.caption}</p>}
      <p>{t('knowledgeConsumption.tableGridMeaning')}</p>
      <div className="overflow-auto">
        <table className="w-full text-left text-sm">
          <tbody>
            {rows.map((row, r) => (
              <tr key={r}>
                {!row?.length && (
                  <td className="border border-border-button p-2">
                    {t('knowledgeConsumption.emptyTableRow')}
                  </td>
                )}
                {(row ?? []).slice(0, 12).map((cell, c) => (
                  <td
                    key={c}
                    className="whitespace-pre-wrap border border-border-button p-2"
                  >
                    {cell.covered
                      ? t('knowledgeConsumption.coveredCell')
                      : cell.text}
                    {(cell.col_span ?? 1) > 1 || (cell.row_span ?? 1) > 1 ? (
                      <small className="block">
                        {t('knowledgeConsumption.cellSpan', {
                          rows: Math.max(1, cell.row_span ?? 1),
                          columns: Math.max(1, cell.col_span ?? 1),
                        })}
                      </small>
                    ) : null}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {clipped && <p>{t('knowledgeConsumption.tablePreviewClipped')}</p>}
    </details>
  );
}

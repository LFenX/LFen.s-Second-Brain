import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
} from 'react';
import { useLocation, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  useContentBoundary,
  useContentNode,
  useContentNodes,
  useContentRead,
  useContentRepresentation,
  useContentScopeError,
  useContentParsedRepresentations,
  useContentCapabilities,
  useContentPolicy,
  useContentPolicyChange,
  useContentParsedSource,
  useContentParsedUpload,
  useContentParsedSnapshot,
  useContentParsedTask,
  useContentParsedTaskCreate,
  useContentParsedTaskCancel,
} from '@/hooks/use-knowledge-consumption';
import { CatalogueFailure, CatalogueLoading, LoadMore } from './state';
import type { KnowledgeSource } from '@/interfaces/database/knowledge-foundation';
import type {
  ContentAddress,
  ContentNode,
  ContentNodeRef,
  ContentTarget,
  ContentWindow,
  ContentParsedSource,
  ContentParsedUploadReceipt,
} from '@/interfaces/database/knowledge-consumption';
import type {
  ContentScope,
  ContentParsedTaskRequest,
} from '@/interfaces/request/knowledge-consumption';
import { ContentRequestError } from '@/services/knowledge-consumption-service';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  contentBoundNode,
  contentCanonical,
  contentFixedLink,
  contentNodeKey,
  contentOriginalLocator,
  contentPageItems,
  contentParsedTargets,
  contentRoute,
  contentSourceTarget,
  contentTargetKey,
  contentEnableParseRequest,
  contentParsedFileFormat,
  contentParsedTaskRequest,
  contentParsedWorkflowRoute,
  contentSchemaValid,
  contentTaskTerminal,
} from '@/utils/knowledge-consumption';
import {
  StructureChoice,
  OriginalChoice,
  ContentAttributesView,
  ChoiceRow,
  type LocalLexicalChoice,
} from './local-lexical-selection';
import LocalLexicalBrowser from './local-lexical';

type Selection = { label: string; target: ContentTarget; autoRead?: boolean };
function parsedChoiceKey(params: URLSearchParams): string {
  try {
    const route = contentParsedWorkflowRoute(params);
    return route.source + ':' + route.task;
  } catch {
    return 'invalid:' + params.toString();
  }
}
export default function KnowledgeRepresentation({
  scope,
  source,
}: {
  scope: ContentScope;
  source: KnowledgeSource | null;
}) {
  const session = useContentBoundary(scope);
  const sourceKey = source ? contentTargetKey(contentSourceTarget(source)) : '';
  return (
    <ContentSelection
      key={session + ':' + scope.owner + ':' + scope.domain + ':' + sourceKey}
      scope={scope}
      source={source}
      session={session}
    />
  );
}
function ContentSelection({
  scope,
  source,
  session,
}: {
  scope: ContentScope;
  source: KnowledgeSource | null;
  session: number;
}) {
  const { t } = useTranslation(),
    [params, setParams] = useSearchParams();
  const [selection, setSelection] = useState<Selection | null>(null),
    [mode, setMode] = useState<'text' | 'structure' | 'document' | 'parsed'>(
      () => (params.get('parsed') === '1' ? 'parsed' : 'text'),
    ),
    [generation, setGeneration] = useState(0),
    [indexChoice, setIndexChoice] = useState<LocalLexicalChoice | null>(null);
  let fixed: ContentAddress | null = null,
    error: unknown = null;
  try {
    fixed = contentRoute(scope.domain, params);
  } catch (issue) {
    error = issue;
  }
  const choose = (value: Selection) => {
    setIndexChoice(null);
    setSelection(value);
    setParams({ content: '1' });
    setGeneration((n) => n + 1);
  };
  const clear = () => {
    setIndexChoice(null);
    setSelection(null);
    setParams({ content: '1' });
    setGeneration((n) => n + 1);
  };
  const parsed = () => {
    clear();
    setParams({ content: '1', parsed: '1' });
    setMode('parsed');
  };
  const chooseParsed = useCallback((value: Selection) => {
    setIndexChoice(null);
    setSelection(value);
    setGeneration((n) => n + 1);
  }, []);
  const clearParsed = useCallback(() => {
    setIndexChoice(null);
    setSelection(null);
    setGeneration((n) => n + 1);
  }, []);
  const text = () => {
      clear();
      setMode('text');
    },
    structure = () => {
      clear();
      setMode('structure');
    },
    document = () => {
      clear();
      setMode('document');
    },
    reset = clear,
    refresh = () => setGeneration((n) => n + 1);
  if (error) return <CatalogueFailure error={error} onRetry={reset} />;
  const textSelection: Selection | null =
    mode === 'text' && source && source.domain_id === scope.domain
      ? { label: source.relative_path, target: contentSourceTarget(source) }
      : null;
  const selected = mode === 'text' ? textSelection : selection;
  const target = fixed?.target ?? selected?.target;
  const addToIndex = () => {
    if (target)
      setIndexChoice({
        target,
        path: selected?.label || target.source_ref.source_id,
        currentness: 'unknown',
      });
  };
  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold">
        {t('knowledgeConsumption.content')}
      </h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeConsumption.fixedSelection')}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" onClick={text}>
          {t('knowledgeConsumption.chooseText')}
        </Button>
        <Button type="button" variant="outline" onClick={parsed}>
          {t('knowledgeConsumption.chooseParsed')}
        </Button>
        <Button type="button" variant="outline" onClick={structure}>
          {t('knowledgeConsumption.chooseStructure')}
        </Button>
        <Button type="button" variant="outline" onClick={document}>
          {t('knowledgeConsumption.chooseDocument')}
        </Button>
        <Button type="button" variant="outline" onClick={refresh}>
          {t('knowledgeCatalogue.refresh')}
        </Button>
      </div>
      {mode === 'text' && (
        <p>
          {source?.relative_path ?? t('knowledgeConsumption.chooseSourceHint')}
        </p>
      )}
      {mode === 'parsed' && (
        <ParsedChoice
          key={parsedChoiceKey(params)}
          scope={scope}
          source={source}
          onChoose={chooseParsed}
          onClear={clearParsed}
        />
      )}
      {mode === 'structure' && (
        <StructureChoice scope={scope} onChoose={choose} onClear={clear} />
      )}
      {mode === 'document' && (
        <OriginalChoice scope={scope} onChoose={choose} onClear={clear} />
      )}
      {target ? (
        <ResolvedContent
          key={
            session +
            ':' +
            scope.owner +
            ':' +
            scope.domain +
            ':' +
            contentTargetKey(target) +
            ':' +
            generation
          }
          scope={scope}
          target={target}
          selected={fixed?.node_ref ?? null}
          label={fixed ? '' : (selected?.label ?? '')}
          autoRead={!fixed && selected?.autoRead === true}
        />
      ) : (
        <p>{t('knowledgeConsumption.chooseSourceHint')}</p>
      )}
      {target && (
        <Button type="button" variant="outline" onClick={addToIndex}>
          {t('knowledgeLocalLexical.addFixedRepresentation')}
        </Button>
      )}
      {indexChoice && (
        <LocalLexicalBrowser
          key={contentTargetKey(indexChoice.target)}
          domain={scope.domain}
          owner={scope.owner}
          initialChoice={indexChoice}
        />
      )}
    </section>
  );
}
function ResolvedContent({
  scope,
  target,
  selected,
  label,
  autoRead = false,
}: {
  scope: ContentScope;
  target: ContentTarget;
  selected: ContentNodeRef | null;
  label: string;
  autoRead?: boolean;
}) {
  const { t } = useTranslation(),
    query = useContentRepresentation(scope, target);
  useContentScopeError(query.error);
  const reload = () => {
    void query.refetch();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (query.isFetching || !query.data) return <CatalogueLoading />;
  const representation = query.data;
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="break-all font-semibold">
        {label || t('knowledgeConsumption.fixedContent')}
      </h3>
      <p>{t('knowledgeConsumption.representation_' + representation.state)}</p>
      {representation.reason && (
        <p>{t('knowledgeConsumption.reason_' + representation.reason)}</p>
      )}
      <details>
        <summary>{t('knowledgeConsumption.fixedIdentity')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(target, null, 2)}
        </pre>
      </details>
      {representation.root_ref ? (
        <ContentTree
          key={
            contentTargetKey(target) +
            ':' +
            contentNodeKey(selected ?? representation.root_ref)
          }
          scope={scope}
          target={target}
          root={representation.root_ref}
          selected={selected ?? representation.root_ref}
          autoRead={autoRead}
        />
      ) : (
        <p>{t('knowledgeConsumption.noTextRepresentation')}</p>
      )}
    </section>
  );
}
function ContentTree({
  scope,
  target,
  root,
  selected,
  autoRead = false,
}: {
  scope: ContentScope;
  target: ContentTarget;
  root: ContentNodeRef;
  selected: ContentNodeRef;
  autoRead?: boolean;
}) {
  const { t } = useTranslation(),
    [path, setPath] = useState<ContentNodeRef[]>([root]),
    [node, setNode] = useState(selected);
  const parent = () => setPath((items) => items.slice(0, -1)),
    reset = () => {
      setPath([root]);
      setNode(root);
    },
    descend = (ref: ContentNodeRef) => {
      setPath((items) => [...items, ref]);
      setNode(ref);
    };
  return (
    <div className="grid min-w-0 grid-cols-1 gap-4 2xl:grid-cols-[minmax(12rem,1fr)_minmax(0,2fr)]">
      <nav className="min-w-0 space-y-2">
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={reset}>
            {t('knowledgeConsumption.root')}
          </Button>
          {path.length > 1 && (
            <Button variant="outline" onClick={parent}>
              {t('knowledgeCatalogue.parentNode')}
            </Button>
          )}
        </div>
        <ContentChildren
          key={contentNodeKey(path.at(-1)!)}
          scope={scope}
          target={target}
          parent={path.at(-1)!}
          onSelect={setNode}
          onDescend={descend}
        />
      </nav>
      <main className="min-w-0">
        <ContentNodeView
          key={contentNodeKey(node)}
          scope={scope}
          address={{ target, node_ref: node }}
          autoRead={autoRead && contentNodeKey(node) === contentNodeKey(root)}
        />
      </main>
    </div>
  );
}
function ContentChildren({
  scope,
  target,
  parent,
  onSelect,
  onDescend,
}: {
  scope: ContentScope;
  target: ContentTarget;
  parent: ContentNodeRef;
  onSelect: (ref: ContentNodeRef) => void;
  onDescend: (ref: ContentNodeRef) => void;
}) {
  const { t } = useTranslation(),
    query = useContentNodes(scope, target, parent);
  useContentScopeError(query.error);
  const reload = () => {
      void query.refetch();
    },
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  let items: ContentNode[];
  try {
    items = contentPageItems(query.data.pages);
  } catch (error) {
    return <CatalogueFailure error={error} onRetry={reload} />;
  }
  return (
    <>
      <ul className="space-y-2">
        {items.map((node) => (
          <ContentNodeChoice
            key={contentNodeKey(node.ref)}
            node={node}
            onSelect={onSelect}
            onDescend={onDescend}
          />
        ))}
      </ul>
      {!items.length && <p>{t('knowledgeConsumption.noChildren')}</p>}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetchingNextPage}
        onMore={more}
      />
    </>
  );
}
function ContentNodeChoice({
  node,
  onSelect,
  onDescend,
}: {
  node: ContentNode;
  onSelect: (ref: ContentNodeRef) => void;
  onDescend: (ref: ContentNodeRef) => void;
}) {
  const { t } = useTranslation(),
    choose = () => onSelect(node.ref),
    descend = () => onDescend(node.ref);
  return (
    <li className="space-y-1">
      <Button
        variant="outline"
        className="h-auto w-full justify-start whitespace-normal text-left"
        onClick={choose}
      >
        {node.name || t('knowledgeConsumption.unnamed')} ·{' '}
        {t('knowledgeConsumption.kind_' + node.kind, {
          defaultValue: node.kind,
        })}
      </Button>
      <Button variant="outline" size="sm" onClick={descend}>
        {t('knowledgeConsumption.children')}
      </Button>
    </li>
  );
}
function ContentNodeView({
  scope,
  address,
  autoRead = false,
}: {
  scope: ContentScope;
  address: ContentAddress;
  autoRead?: boolean;
}) {
  const { t } = useTranslation(),
    query = useContentNode(scope, address),
    [reading, setReading] = useState(autoRead);
  useContentScopeError(query.error);
  const reload = () => {
      setReading(false);
      void query.refetch();
    },
    open = () => setReading(true);
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (query.isFetching || !query.data) return <CatalogueLoading />;
  const node = query.data.node,
    range = node.text.range,
    readable =
      contentBoundNode(address.target, node.ref) &&
      node.text.status === 'available' &&
      range !== null;
  return (
    <section className="space-y-3">
      <h3 className="break-all font-semibold">
        {node.name || t('knowledgeConsumption.unnamed')} ·{' '}
        {t('knowledgeConsumption.kind_' + node.kind, {
          defaultValue: node.kind,
        })}
      </h3>
      {node.name_truncated && (
        <p className="text-xs text-text-secondary">
          {t('knowledgeConsumption.nameTruncated')}
        </p>
      )}
      <p>{t('knowledgeConsumption.text_' + node.text.status)}</p>
      {node.text.reason && (
        <p>{t('knowledgeConsumption.reason_' + node.text.reason)}</p>
      )}
      <p>
        {t('knowledgeConsumption.originalLocator')}:{' '}
        <span className="break-all">{originalLabel(node, t)}</span>
      </p>
      <p>
        {t('knowledgeConsumption.readingCoordinates')}:{' '}
        {t('knowledgeConsumption.space_' + node.text.coordinate_space)}{' '}
        {range
          ? '[' + range.start_byte + ', ' + range.end_byte + ')'
          : t('knowledgeConsumption.noReadingRange')}
      </p>
      <p className="text-xs text-text-secondary">
        {t('knowledgeConsumption.coordinateWarning')}
      </p>
      <ContentAttributesView node={node} />
      <a className="text-sm underline" href={contentFixedLink(address)}>
        {t('knowledgeConsumption.fixedLink')}
      </a>
      <Button
        type="button"
        variant="outline"
        disabled={!readable}
        onClick={open}
      >
        {t('knowledgeConsumption.read')}
      </Button>
      {reading && readable && range && (
        <ContentReading
          key={contentCanonical(address)}
          scope={scope}
          address={address}
          node={node}
        />
      )}
      <details>
        <summary>{t('knowledgeConsumption.fixedIdentity')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify({ address, node }, null, 2)}
        </pre>
      </details>
    </section>
  );
}
function originalLabel(
  node: ContentNode,
  t: (key: string, args?: Record<string, unknown>) => string,
) {
  if (node.original_position?.kind === 'pptx_slide')
    return t('knowledgeConsumption.originalSlide', {
      number: node.original_position.slide_number,
    });
  if (node.original_position?.kind === 'pdf_page')
    return t('knowledgeConsumption.originalPage', {
      number: node.original_position.page_number,
    });
  const locator = node.locator;
  if (locator.kind === 'parsed_item')
    return t('knowledgeConsumption.parsedItemPosition', {
      number: locator.item_index,
    });
  if (locator.kind === 'source_utf8' && locator.source_utf8)
    return t('knowledgeConsumption.originalLines', {
      start: locator.source_utf8.start_line,
      end: locator.source_utf8.end_line,
    });
  const original = contentOriginalLocator(locator);
  if (
    original &&
    'scheme' in original &&
    original.scheme === 'office_package_v1'
  )
    return (
      original.part +
      ' / ' +
      original.segments
        .map((segment) => segment.name + '[' + segment.index + ']')
        .join(' / ')
    );
  return t('knowledgeConsumption.documentOnly');
}
function ContentReading({
  scope,
  address,
  node,
}: {
  scope: ContentScope;
  address: ContentAddress;
  node: ContentNode;
}) {
  const { t } = useTranslation(),
    [start, setStart] = useState(node.text.range!.start_byte),
    query = useContentRead(scope, address, node, start);
  useContentScopeError(query.error);
  const reload = () => {
      void query.refetch();
    },
    more = () => {
      if (
        query.data?.next_start_byte !== null &&
        query.data?.next_start_byte !== undefined
      )
        setStart(query.data.next_start_byte);
    };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (query.isFetching || !query.data) return <CatalogueLoading />;
  const window: ContentWindow = query.data;
  return (
    <section className="space-y-2 rounded border border-border-button p-3">
      <p>
        {t('knowledgeConsumption.verifiedWindow')} [{window.range.start_byte},{' '}
        {window.range.end_byte})
      </p>
      <pre className="whitespace-pre-wrap break-all">{window.text}</pre>
      {window.next_start_byte !== null && (
        <Button variant="outline" onClick={more}>
          {t('knowledgeConsumption.moreText')}
        </Button>
      )}
    </section>
  );
}

type ParsedHistory = {
  owner: string;
  domain: string;
  receipt?: ContentParsedUploadReceipt;
  pendingTask?: ContentParsedTaskRequest;
  uploadUnconfirmed?: boolean;
};
function ParsedChoice({
  scope,
  source,
  onChoose,
  onClear,
}: {
  scope: ContentScope;
  source: KnowledgeSource | null;
  onChoose: (value: Selection) => void;
  onClear: () => void;
}) {
  const { t } = useTranslation(),
    [params, setParams] = useSearchParams(),
    location = useLocation();
  let route = { source: '', task: '' },
    routeError: unknown = null;
  try {
    route = contentParsedWorkflowRoute(params);
  } catch (error) {
    routeError = error;
  }
  const raw = (location.state as { knowledgeParsed?: ParsedHistory } | null)
    ?.knowledgeParsed;
  const saved =
    raw?.owner === scope.owner && raw.domain === scope.domain ? raw : undefined;
  const receipt =
    saved?.receipt &&
    saved.receipt.source_id === route.source &&
    contentSchemaValid('ParsedUploadReceipt', saved.receipt)
      ? saved.receipt
      : undefined;
  const pendingTask =
    saved?.pendingTask &&
    saved.pendingTask.source_ref.domain_id === scope.domain &&
    saved.pendingTask.source_ref.source_id === route.source &&
    contentSchemaValid('ParsedTaskRequest', saved.pendingTask)
      ? saved.pendingTask
      : undefined;
  const [file, setFile] = useState<File | null>(null),
    [problem, setProblem] = useState<unknown>(null),
    [busy, setBusy] = useState('');
  const resultShown = useRef(''),
    cancelRequest = useRef<
      | Parameters<
          ReturnType<typeof useContentParsedTaskCancel>['mutateAsync']
        >[0]
      | null
    >(null);
  const capabilities = useContentCapabilities(scope),
    policy = useContentPolicy(scope),
    current = useContentParsedSource(scope, route.source),
    task = useContentParsedTask(scope, route.task, route.source);
  const policyChange = useContentPolicyChange(scope),
    upload = useContentParsedUpload(scope),
    snapshot = useContentParsedSnapshot(scope),
    create = useContentParsedTaskCreate(scope),
    cancel = useContentParsedTaskCancel(scope);
  useContentScopeError(policy.error ?? capabilities.error);
  const privateError = [current.error, task.error, problem].find(
    (error) => error instanceof KnowledgeRequestError && error.status === 401,
  );
  useContentScopeError(privateError);
  useEffect(() => {
    resultShown.current = '';
    onClear();
  }, [route.source, route.task, onClear]);
  useEffect(() => {
    if (pendingTask) resultShown.current = '';
    if (
      !pendingTask &&
      !task.isError &&
      task.data?.state === 'succeeded' &&
      task.data.result
    ) {
      const key = task.data.task_id + ':' + contentTargetKey(task.data.result);
      if (resultShown.current !== key) {
        resultShown.current = key;
        onChoose({
          label: t('knowledgeConsumption.manualResult'),
          target: task.data.result,
          autoRead: true,
        });
      }
    }
    if (
      task.data &&
      (contentTaskTerminal(task.data) || task.data.state === 'cancel_requested')
    )
      cancelRequest.current = null;
  }, [task.data, task.isError, pendingTask, onChoose, t]);
  const writeRoute = (
    sourceId: string,
    taskId: string,
    patch: Partial<ParsedHistory> = {},
  ) => {
    const history: ParsedHistory = {
      owner: scope.owner,
      domain: scope.domain,
      receipt,
      pendingTask,
      uploadUnconfirmed: saved?.uploadUnconfirmed === true,
      ...patch,
    };
    setParams(
      {
        content: '1',
        parsed: '1',
        ...(sourceId ? { parsed_source: sourceId } : {}),
        ...(taskId ? { parsed_task: taskId } : {}),
      },
      { state: { knowledgeParsed: history } },
    );
  };
  const chooseExisting = () => {
    if (source) {
      onClear();
      setProblem(null);
      writeRoute(source.source_id, '', {
        receipt: undefined,
        pendingTask: undefined,
        uploadUnconfirmed: false,
      });
    }
  };
  const chooseFile = (event: ChangeEvent<HTMLInputElement>) => {
    setFile(event.target.files?.[0] ?? null);
    setProblem(null);
  };
  const enable = async () => {
    if (!policy.data || busy) return;
    setBusy('policy');
    setProblem(null);
    try {
      await policyChange.mutateAsync(
        contentEnableParseRequest(policy.data, crypto.randomUUID()),
      );
      await policy.refetch();
    } catch (error) {
      if (
        error instanceof KnowledgeRequestError &&
        error.code === 'content_scope_changed'
      )
        return;
      setProblem(error);
      await policy.refetch();
    } finally {
      setBusy('');
    }
  };
  const save = async () => {
    if (
      !file ||
      !policy.data?.parse_enabled ||
      busy ||
      saved?.uploadUnconfirmed
    )
      return;
    onClear();
    setBusy('upload');
    setProblem(null);
    writeRoute('', '', {
      uploadUnconfirmed: true,
      receipt: undefined,
      pendingTask: undefined,
    });
    try {
      const value = await upload.mutateAsync({
        file,
        request: {
          expected_policy_version: policy.data.foundation_policy_version,
          expected_content_policy_version: policy.data.content_policy_version,
          confirmed: true,
        },
      });
      writeRoute(value.source_id, '', {
        receipt: value,
        uploadUnconfirmed: false,
        pendingTask: undefined,
      });
      setFile(null);
    } catch (error) {
      if (
        error instanceof KnowledgeRequestError &&
        error.code === 'content_scope_changed'
      )
        return;
      setProblem(error);
    } finally {
      setBusy('');
    }
  };
  const fixOriginal = async () => {
    if (!receipt || !policy.data || busy || current.data) return;
    setBusy('snapshot');
    setProblem(null);
    try {
      await snapshot.mutateAsync({
        source_id: receipt.source_id,
        expected_revision_id: '',
        expected_policy_version: policy.data.foundation_policy_version,
        relative_path: receipt.original_filename,
        classification: {
          group_id: null,
          component_side: 'non_code',
          language: '',
        },
      });
      await current.refetch();
    } catch (error) {
      if (
        error instanceof KnowledgeRequestError &&
        error.code === 'content_scope_changed'
      )
        return;
      setProblem(error);
      await current.refetch();
    } finally {
      setBusy('');
    }
  };
  const start = async () => {
    if (!current.data || !policy.data || busy) return;
    setBusy('task');
    setProblem(null);
    onClear();
    try {
      const request =
        pendingTask ??
        contentParsedTaskRequest(
          current.data,
          policy.data,
          crypto.randomUUID(),
        );
      writeRoute(route.source, route.task, { pendingTask: request });
      const value = await create.mutateAsync(request);
      writeRoute(route.source, value.task_id, { pendingTask: undefined });
    } catch (error) {
      if (
        error instanceof KnowledgeRequestError &&
        error.code === 'content_scope_changed'
      )
        return;
      setProblem(error);
      if (error instanceof ContentRequestError && error.outcomeKnown === true)
        writeRoute(route.source, route.task, { pendingTask: undefined });
    } finally {
      setBusy('');
    }
  };
  const stop = async () => {
    if (!task.data || contentTaskTerminal(task.data) || busy) return;
    setBusy('cancel');
    setProblem(null);
    try {
      const input = cancelRequest.current ?? {
        task: task.data,
        request: {
          expected_task_revision: task.data.revision,
          idempotency_key: crypto.randomUUID(),
        },
      };
      cancelRequest.current = input;
      await cancel.mutateAsync(input);
      await task.refetch();
    } catch (error) {
      if (
        error instanceof KnowledgeRequestError &&
        error.code === 'content_scope_changed'
      )
        return;
      setProblem(error);
      if (error instanceof ContentRequestError && error.outcomeKnown === true)
        cancelRequest.current = null;
      await task.refetch();
    } finally {
      setBusy('');
    }
  };
  const refresh = async () => {
    setProblem(null);
    await policy.refetch();
    if (route.source) await current.refetch();
    if (route.task) await task.refetch();
  };
  const resetRoute = () =>
    writeRoute('', '', {
      receipt: undefined,
      pendingTask: undefined,
      uploadUnconfirmed: false,
    });
  if (routeError)
    return <CatalogueFailure error={routeError} onRetry={resetRoute} />;
  if (policy.isError || capabilities.isError)
    return (
      <CatalogueFailure
        error={policy.error ?? capabilities.error}
        onRetry={refresh}
      />
    );
  if (!policy.data || !capabilities.data) return <CatalogueLoading />;
  const meta = current.data,
    cap = capabilities.data,
    active = task.data && !contentTaskTerminal(task.data);
  const format = file ? contentParsedFileFormat(file.name) : null;
  const fileValid =
    !!file &&
    file.size <= cap.max_parsed_original_bytes &&
    !!format &&
    cap.parsed_profiles.some((profile) => profile.format === format);
  const canParse =
    !!meta &&
    !current.isFetching &&
    !policy.isFetching &&
    meta.state === 'active' &&
    meta.profile?.runtime_ready &&
    cap.parsed_publication_runtime_ready &&
    policy.data.parse_enabled &&
    meta.foundation_policy_version === policy.data.foundation_policy_version;
  return (
    <section
      className="space-y-4 rounded border border-border-button p-4"
      aria-label={t('knowledgeConsumption.manualFlow')}
    >
      <h3 className="font-semibold">{t('knowledgeConsumption.manualFlow')}</h3>
      <p>{t('knowledgeConsumption.manualSeparation')}</p>
      <p role="status">
        {t(
          policy.data.parse_enabled
            ? 'knowledgeConsumption.manualEnabled'
            : 'knowledgeConsumption.manualDisabled',
        )}
      </p>
      {!policy.data.parse_enabled && (
        <Button
          type="button"
          variant="outline"
          disabled={!!busy}
          onClick={enable}
        >
          {t('knowledgeConsumption.manualEnable')}
        </Button>
      )}
      <p className="text-sm text-text-secondary">
        {t('knowledgeConsumption.manualFormats')}:{' '}
        {cap.parsed_profiles.map((profile) => profile.format).join('、')}
      </p>
      {!route.source && (
        <div className="space-y-2">
          <label className="block">
            {t('knowledgeConsumption.manualFile')}
            <input
              key={route.source}
              type="file"
              onChange={chooseFile}
              disabled={!!busy || saved?.uploadUnconfirmed === true}
              className="block w-full text-sm"
            />
          </label>
          {file && !fileValid && (
            <p role="alert">{t('knowledgeConsumption.manualInvalidFile')}</p>
          )}
          <Button
            type="button"
            variant="outline"
            disabled={
              !fileValid ||
              !policy.data.parse_enabled ||
              !!busy ||
              saved?.uploadUnconfirmed === true
            }
            onClick={save}
          >
            {t(
              busy === 'upload'
                ? 'knowledgeConsumption.manualSaving'
                : 'knowledgeConsumption.manualSave',
            )}
          </Button>
          {source && (
            <Button
              type="button"
              variant="outline"
              disabled={!!busy || saved?.uploadUnconfirmed === true}
              onClick={chooseExisting}
            >
              {t('knowledgeConsumption.manualUseSelected')}:{' '}
              {source.relative_path}
            </Button>
          )}
        </div>
      )}
      {saved?.uploadUnconfirmed === true && (
        <p role="alert">{t('knowledgeConsumption.manualUploadUnknown')}</p>
      )}
      {receipt && !meta && (
        <section className="space-y-2">
          <p role="status">
            {t('knowledgeConsumption.manualSaved')}: {receipt.original_filename}{' '}
            · {receipt.byte_length} {t('knowledgeConsumption.bytes')}
          </p>
          <Button
            type="button"
            variant="outline"
            disabled={!!busy || current.isFetching}
            onClick={fixOriginal}
          >
            {t('knowledgeConsumption.manualSnapshot')}
          </Button>
        </section>
      )}
      {route.source && current.isFetching && <CatalogueLoading />}
      {current.isError &&
        !(
          current.error instanceof KnowledgeRequestError &&
          current.error.status === 404 &&
          receipt
        ) && <CatalogueFailure error={current.error} onRetry={refresh} />}
      {meta && (
        <section className="space-y-2">
          <p role="status">
            {t('knowledgeConsumption.manualSnapshotted')}:{' '}
            {meta.original_filename} · {meta.byte_length}{' '}
            {t('knowledgeConsumption.bytes')}
          </p>
          <p>
            {t('knowledgeConsumption.manualSourceState')}: {meta.state};{' '}
            {t('knowledgeConsumption.manualParser')}:{' '}
            {meta.profile?.format ??
              t('knowledgeConsumption.manualUnsupported')}
          </p>
          <details>
            <summary>{t('knowledgeConsumption.fixedIdentity')}</summary>
            <pre className="whitespace-pre-wrap break-all text-xs">
              {JSON.stringify(
                {
                  source_ref: meta.source_ref,
                  lifecycle_version: meta.lifecycle_version,
                  profile: meta.profile,
                },
                null,
                2,
              )}
            </pre>
          </details>
          {!canParse && <p>{t('knowledgeConsumption.manualNotReady')}</p>}
          <Button
            type="button"
            variant="outline"
            disabled={!!busy || !!active || (!pendingTask && !canParse)}
            onClick={start}
          >
            {t(
              pendingTask
                ? 'knowledgeConsumption.manualRetryTask'
                : 'knowledgeConsumption.manualStart',
            )}
          </Button>
          <PublishedParsedChoices
            key={contentCanonical(meta.source_ref)}
            scope={scope}
            source={meta}
            onChoose={onChoose}
          />
        </section>
      )}
      {pendingTask && (
        <p role="alert">{t('knowledgeConsumption.manualTaskUnknown')}</p>
      )}
      {route.task && task.isFetching && !task.data && <CatalogueLoading />}
      {task.isError && (
        <CatalogueFailure error={task.error} onRetry={refresh} />
      )}
      {task.data && (
        <section
          className="space-y-2"
          aria-label={t('knowledgeConsumption.manualTask')}
        >
          <p role="status">
            {t('knowledgeConsumption.manualState_' + task.data.state)} ·{' '}
            {t('knowledgeConsumption.manualPhase_' + task.data.phase)}
          </p>
          {task.data.failure_code && (
            <p role="alert">
              {t('knowledgeConsumption.manualFailure')}:{' '}
              {task.data.failure_code}
            </p>
          )}
          {task.data.state === 'cancel_requested' && (
            <p>{t('knowledgeConsumption.manualCancelling')}</p>
          )}
          {active && task.data.state !== 'cancel_requested' && (
            <Button
              type="button"
              variant="outline"
              disabled={!!busy}
              onClick={stop}
            >
              {t(
                cancelRequest.current
                  ? 'knowledgeConsumption.manualRetryCancel'
                  : 'knowledgeConsumption.manualCancel',
              )}
            </Button>
          )}
        </section>
      )}
      {!!problem && (
        <div role="alert">
          <p>
            {t('knowledgeConsumption.manualProblem')}:{' '}
            {problem instanceof KnowledgeRequestError
              ? problem.code
              : t('knowledgeConsumption.manualUnexpected')}
          </p>
          {problem instanceof ContentRequestError &&
            problem.outcomeKnown !== true && (
              <p>{t('knowledgeConsumption.manualUnconfirmed')}</p>
            )}
        </div>
      )}
      <Button
        type="button"
        variant="outline"
        disabled={!!busy}
        onClick={refresh}
      >
        {t('knowledgeConsumption.manualRefresh')}
      </Button>
    </section>
  );
}
function PublishedParsedChoices({
  scope,
  source,
  onChoose,
}: {
  scope: ContentScope;
  source: ContentParsedSource;
  onChoose: (value: Selection) => void;
}) {
  const { t } = useTranslation(),
    ref = source.source_ref,
    query = useContentParsedRepresentations(scope, ref);
  useContentScopeError(query.error);
  const reload = () => {
      void query.refetch();
    },
    more = () => {
      void query.fetchNextPage();
    };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  let targets: ContentTarget[];
  try {
    targets = contentParsedTargets(ref, query.data.pages);
  } catch (error) {
    return <CatalogueFailure error={error} onRetry={reload} />;
  }
  return (
    <section className="space-y-2">
      <h3>
        {source.relative_path} · {t('knowledgeConsumption.chooseParsed')}
      </h3>
      <p className="text-sm text-text-secondary">
        {t('knowledgeConsumption.parsedOrder')}
      </p>
      <ul className="space-y-2">
        {targets.map((target, index) => (
          <ChoiceRow
            key={contentTargetKey(target)}
            label={
              source.relative_path +
              ' · ' +
              t('knowledgeConsumption.parsedEntry', { number: index + 1 })
            }
            target={target}
            onChoose={onChoose}
          />
        ))}
      </ul>
      {!targets.length && (
        <p role="status">{t('knowledgeConsumption.noParsedRepresentation')}</p>
      )}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetchingNextPage}
        onMore={more}
      />
    </section>
  );
}

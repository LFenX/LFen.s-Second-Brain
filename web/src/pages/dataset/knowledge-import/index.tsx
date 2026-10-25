import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams, useSearchParams } from 'react-router';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import {
  useKnowledgeDomain,
  useKnowledgeDomains,
  getKnowledgeSessionScope,
} from '@/hooks/use-knowledge-foundation';
import { useKnowledgeImportContract } from '@/hooks/use-knowledge-sources';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import authorizationStorage, {
  getAuthorization,
} from '@/utils/authorization-util';
import { knowledgeImportStorageKey } from '@/utils/knowledge-import';
import { knowledgeReceiptLink } from '@/utils/knowledge-source-link';
import type {
  KnowledgeImportPreview,
  KnowledgeImportReceipt,
} from '@/interfaces/database/knowledge-sources';
import type { KnowledgeSourceResult } from '@/services/knowledge-sources-service';
import {
  CatalogueLoading,
  CatalogueScopeFailure,
  LoadMore,
} from '../knowledge-catalogue/state';
import { RegistrationForm, SourceForm } from './fields';
import { useImportFlow } from './use-import-flow';
import DirectoryImport from './directory';

export function ImportFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  const failure = error instanceof KnowledgeRequestError ? error : null;
  const key =
    failure &&
    [
      'knowledge_source_contract_version',
      'knowledge_contract_version',
    ].includes(failure.code)
      ? 'versionMismatch'
      : failure && [401, 403, 404].includes(failure.status)
        ? 'notVisible'
        : failure?.status === 409
          ? 'conflict'
          : failure?.status === 413
            ? 'tooLarge'
            : failure?.status === 415
              ? 'unsupported'
              : failure?.status === 400
                ? 'invalidInput'
                : 'unavailable';
  return (
    <div
      role="alert"
      className="space-y-3 rounded-lg border border-border-button p-4 text-text-secondary"
    >
      <p>{t(`knowledgeImport.${key}`)}</p>
      {failure?.requestId && (
        <p className="break-all text-xs">
          {t('knowledgeCatalogue.requestId')}: {failure.requestId}
        </p>
      )}
      {onRetry && (
        <Button variant="outline" onClick={onRetry}>
          {t('knowledgeImport.reload')}
        </Button>
      )}
    </div>
  );
}

function DomainPicker({
  onSelect,
  directory,
}: {
  onSelect: () => void;
  directory: boolean;
}) {
  const { t } = useTranslation();
  const query = useKnowledgeDomains();
  const handleRetry = () => {
    void query.refetch();
  };
  const handleMore = () => {
    void query.fetchNextPage();
  };
  if (query.isError)
    return <ImportFailure error={query.error} onRetry={handleRetry} />;
  if (query.isPending) return <CatalogueLoading />;
  return (
    <nav aria-label={t('knowledgeImport.chooseDomain')} className="space-y-2">
      {query.data.pages
        .flatMap((page) => page.items)
        .map((domain) => (
          <Link
            key={domain.domain_id}
            className="block rounded-lg border border-border-button p-3"
            to={`/dataset/knowledge-import/${domain.domain_id}${directory ? '?mode=directory' : ''}`}
            onClick={onSelect}
          >
            {domain.name}
          </Link>
        ))}
      <LoadMore
        more={query.hasNextPage}
        loading={query.isFetching}
        onMore={handleMore}
      />
    </nav>
  );
}

function PreviewConfirmation({
  preview,
  busy,
  onConfirm,
}: {
  preview: KnowledgeImportPreview;
  busy: boolean;
  onConfirm: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [checked, setChecked] = useState(false);
  const item = preview.items[0];
  const handleChecked = (event: React.ChangeEvent<HTMLInputElement>) =>
    setChecked(event.target.checked);
  const handleConfirm = () => {
    if (checked) void onConfirm();
  };
  return (
    <section
      aria-label={t('knowledgeImport.confirmation')}
      className="space-y-4 rounded-lg border border-border-button bg-bg-card p-4"
    >
      <h2 className="text-lg font-semibold">
        {t('knowledgeImport.confirmation')}
      </h2>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-sm">
        <dt>{t('knowledgeImport.relativePath')}</dt>
        <dd className="break-all">{item.relative_path}</dd>
        <dt>{t('knowledgeImport.action')}</dt>
        <dd>{t(`knowledgeImport.${item.action}`)}</dd>
        <dt>{t('knowledgeImport.componentSide')}</dt>
        <dd>
          {t(`knowledgeCatalogue.${item.component_side}`)} ·{' '}
          {item.language || t('knowledgeCatalogue.unknownLanguage')}
        </dd>
        <dt>{t('knowledgeImport.group')}</dt>
        <dd className="break-all">
          {item.group_id || t('knowledgeImport.noGroup')}
        </dd>
        <dt>{t('knowledgeImport.bytes')}</dt>
        <dd>{item.byte_length} B</dd>
        <dt>SHA-256</dt>
        <dd className="break-all">{item.sha256}</dd>
        <dt>{t('knowledgeImport.expectedRevision')}</dt>
        <dd className="break-all">
          {item.expected_revision_id || t('knowledgeImport.firstImport')}
        </dd>
        <dt>{t('knowledgeImport.expires')}</dt>
        <dd>{dayjs(preview.expires_at).format('YYYY-MM-DD HH:mm:ss')}</dd>
      </dl>
      <p className="text-sm text-text-secondary">
        {t('knowledgeImport.previewHint')}
      </p>
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={checked}
          onChange={handleChecked}
          disabled={busy}
          className="mt-1"
        />
        <span>{t('knowledgeImport.consent')}</span>
      </label>
      <Button disabled={!checked || busy} onClick={handleConfirm}>
        {t('knowledgeImport.confirm')}
      </Button>
    </section>
  );
}

function ReceiptView({
  result,
  onAnother,
}: {
  result: KnowledgeSourceResult<KnowledgeImportReceipt>;
  onAnother: () => void;
}) {
  const { t } = useTranslation();
  const value = result.data;
  const item = value.items[0];
  return (
    <section aria-label={t('knowledgeImport.receipt')} className="space-y-4">
      <h2 className="text-lg font-semibold">
        {t(
          value.status === 'succeeded'
            ? 'knowledgeImport.succeeded'
            : 'knowledgeImport.failed',
        )}
      </h2>
      <p className="text-sm text-text-secondary">
        HTTP {result.httpStatus} · {t('knowledgeImport.receiptCreated')}
      </p>
      <p className="break-all">
        {item.relative_path} · {t(`knowledgeCatalogue.${item.component_side}`)}{' '}
        · {item.language || t('knowledgeCatalogue.unknownLanguage')}
      </p>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-sm">
        <dt>{t('knowledgeImport.saveStatus')}</dt>
        <dd>{t(`knowledgeImport.${item.save_status}`)}</dd>
        <dt>{t('knowledgeImport.catalogue')}</dt>
        <dd>
          {t(
            item.catalogue_status === 'ready'
              ? 'knowledgeImport.catalogued'
              : 'knowledgeImport.notCatalogued',
          )}
        </dd>
        <dt>{t('knowledgeImport.structure')}</dt>
        <dd>{t('knowledgeImport.deferred')}</dd>
        <dt>{t('knowledgeImport.fullText')}</dt>
        <dd>{t('knowledgeImport.disabled')}</dd>
        <dt>{t('knowledgeImport.vector')}</dt>
        <dd>{t('knowledgeImport.disabled')}</dd>
        <dt>{t('knowledgeImport.modelCalls')}</dt>
        <dd>{value.model_calls}</dd>
        <dt>{t('knowledgeImport.receiptId')}</dt>
        <dd className="break-all">{value.import_task_id}</dd>
        <dt>{t('knowledgeCatalogue.revision')}</dt>
        <dd className="break-all">{item.source_revision_id || '—'}</dd>
      </dl>
      {item.save_status === 'unchanged' && (
        <p className="text-sm text-text-secondary">
          {t('knowledgeImport.unchangedHint')}
        </p>
      )}
      <div className="flex flex-wrap gap-3">
        {value.status === 'succeeded' && (
          <Link
            className="rounded-md border border-border-button px-4 py-2 underline"
            to={knowledgeReceiptLink(value.domain_id, value.items[0])}
          >
            {t('knowledgeImport.readRevision')}
          </Link>
        )}
        <Button variant="outline" onClick={onAnother}>
          {t(
            value.status === 'failed'
              ? 'knowledgeImport.retryPreview'
              : 'knowledgeImport.anotherFile',
          )}
        </Button>
      </div>
      {value.status === 'failed' && (
        <p className="text-sm text-text-secondary">
          {t('knowledgeImport.failedHint')}
        </p>
      )}
    </section>
  );
}

function ImportWizard({
  domainId,
  storageKey,
  onReload,
  onScopeFailure,
}: {
  domainId: string;
  storageKey: string;
  onReload: () => void;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation();
  const flow = useImportFlow(domainId, storageKey, onScopeFailure);
  const [fileError, setFileError] = useState<unknown>(null);
  const handleChange = () => {
    setFileError(null);
    flow.invalidatePreview();
  };
  const handleFileError = (error: unknown) => {
    setFileError(error);
  };
  const handleRecover = () => {
    void flow.recover();
  };
  if (flow.scopeFailure)
    return <ImportFailure error={flow.scopeFailure} onRetry={onReload} />;
  const step =
    flow.phase === 'registration'
      ? 0
      : flow.phase === 'file'
        ? 1
        : flow.phase === 'receipt'
          ? 3
          : 2;
  return (
    <CatalogueScopeFailure.Provider value={flow.hideScope}>
      <ol
        aria-label={t('knowledgeImport.steps')}
        className="mb-6 grid gap-2 sm:grid-cols-4"
      >
        {['registration', 'privatePreview', 'confirmation', 'receipt'].map(
          (label, index) => (
            <li
              key={label}
              aria-current={step === index ? 'step' : undefined}
              className={`rounded-lg border border-border-button p-3 ${step === index ? 'bg-bg-card font-semibold' : 'text-text-secondary'}`}
            >
              {index + 1}. {t(`knowledgeImport.${label}`)}
            </li>
          ),
        )}
      </ol>
      {!!flow.error && <ImportFailure error={flow.error} />}
      {!!fileError && <ImportFailure error={fileError} />}
      {flow.phase === 'registration' && (
        <RegistrationForm
          domainId={domainId}
          busy={flow.busy}
          attempt={flow.registrationAttempt}
          onSubmit={flow.register}
        />
      )}
      {flow.registration && ['file', 'preview'].includes(flow.phase) && (
        <>
          <p className="mb-4 text-sm text-text-secondary">
            {t('knowledgeImport.registered')}: {flow.registration.name}
          </p>
          <SourceForm
            registration={flow.registration}
            busy={flow.busy}
            onChange={handleChange}
            onSubmit={flow.makePreview}
            onError={handleFileError}
          >
            {flow.preview && flow.phase === 'preview' && (
              <PreviewConfirmation
                key={flow.preview.preview_id}
                preview={flow.preview}
                busy={flow.busy}
                onConfirm={flow.confirm}
              />
            )}
          </SourceForm>
        </>
      )}
      {['submitting', 'unknown', 'conflict'].includes(flow.phase) && (
        <section
          className="space-y-4 py-4"
          aria-label={t('knowledgeImport.recovery')}
        >
          <p role="status">
            {t(
              flow.phase === 'submitting'
                ? 'knowledgeImport.submitting'
                : 'knowledgeImport.pending',
            )}
          </p>
          <p className="text-sm text-text-secondary">
            {t('knowledgeImport.pendingHint')}
          </p>
          <Button disabled={flow.busy} onClick={handleRecover}>
            {t('knowledgeImport.checkResult')}
          </Button>
          {flow.canRepreview && (
            <Button
              variant="outline"
              disabled={flow.busy}
              onClick={flow.startAnother}
            >
              {t('knowledgeImport.retryPreview')}
            </Button>
          )}
        </section>
      )}
      {flow.phase === 'receipt' && flow.receipt && (
        <ReceiptView result={flow.receipt} onAnother={flow.startAnother} />
      )}
    </CatalogueScopeFailure.Provider>
  );
}

function AuthorizedImport({ domainId }: { domainId: string }) {
  const { t } = useTranslation();
  const domain = useKnowledgeDomain(domainId);
  const contract = useKnowledgeImportContract();
  const [key, setKey] = useState('');
  const [keyError, setKeyError] = useState<unknown>(null);
  const [generation, setGeneration] = useState(0);
  const [scopeFailure, setScopeFailure] = useState<unknown>(null);
  const handleScopeFailure = useCallback(
    (error: unknown) => setScopeFailure(error),
    [],
  );
  const authorization = getAuthorization();
  let nativeUserId = '';
  try {
    const id = authorizationStorage?.getUserInfoObject?.()?.id;
    if (typeof id === 'string') nativeUserId = id;
  } catch {
    /* A direct token session can still recover within its own scope. */
  }
  useEffect(() => {
    let active = true;
    void knowledgeImportStorageKey(domainId, authorization, nativeUserId)
      .then((value) => {
        if (active) setKey(value);
      })
      .catch((error) => {
        if (active) setKeyError(error);
      });
    return () => {
      active = false;
    };
  }, [authorization, domainId, nativeUserId]);
  const handleReload = () => {
    void Promise.all([domain.refetch(), contract.refetch()]).then((results) => {
      if (results.every((result) => !result.isError)) {
        setScopeFailure(null);
        setGeneration((value) => value + 1);
      }
    });
  };
  if (domain.isError || contract.isError || keyError || scopeFailure)
    return (
      <ImportFailure
        error={domain.error || contract.error || keyError || scopeFailure}
        onRetry={handleReload}
      />
    );
  if (domain.isPending || contract.isPending || !key)
    return <CatalogueLoading />;
  const value = domain.data;
  if (
    value.domain_id !== domainId ||
    value.contract_version !== '1.0.0' ||
    value.policy.access !== 'owner_only' ||
    value.policy.external_models !== false ||
    value.policy.automatic_ingestion !== false ||
    value.policy.allowed_model_uses.length ||
    value.policy.index_modes.length !== 1 ||
    value.policy.index_modes[0] !== 'catalogue'
  )
    return (
      <ImportFailure
        error={new KnowledgeRequestError(409, 'knowledge_contract_version')}
        onRetry={handleReload}
      />
    );
  return (
    <>
      <h1 className="text-2xl font-semibold">
        {t('knowledgeImport.title')} · {value.name}
      </h1>
      <p className="my-3 text-sm text-text-secondary">
        {t('knowledgeImport.scope')}
      </p>
      <ImportWizard
        key={`${key}:${generation}`}
        domainId={domainId}
        storageKey={key}
        onReload={handleReload}
        onScopeFailure={handleScopeFailure}
      />
    </>
  );
}

export default function KnowledgeImport() {
  const { id = '' } = useParams();
  const [params, setParams] = useSearchParams();
  const directory = params.get('mode') === 'directory';
  const { t } = useTranslation();
  const [chooseDomain, setChooseDomain] = useState(false);
  const handleChooseDomain = () => setChooseDomain(!chooseDomain);
  const handleDomainSelected = () => setChooseDomain(false);
  const handleManual = () => {
    const next = new URLSearchParams(params);
    next.delete('mode');
    setParams(next);
  };
  const handleDirectory = () => {
    const next = new URLSearchParams(params);
    next.set('mode', 'directory');
    setParams(next);
  };
  return (
    <section className="min-w-0 space-y-4 overflow-auto p-6 text-text-primary">
      <div className="flex flex-wrap gap-4">
        <Link
          className="text-sm text-text-secondary underline"
          to={`/dataset/knowledge-catalogue/${id}`}
        >
          {t('knowledgeImport.backCatalogue')}
        </Link>
        <Button variant="outline" onClick={handleChooseDomain}>
          {t('knowledgeImport.chooseDomain')}
        </Button>
      </div>
      <nav
        aria-label={t('knowledgeDirectory.mode')}
        className="flex flex-wrap gap-2"
      >
        <Button
          variant={directory ? 'outline' : 'default'}
          aria-pressed={!directory}
          onClick={handleManual}
        >
          {t('knowledgeDirectory.manual')}
        </Button>
        <Button
          variant={directory ? 'default' : 'outline'}
          aria-pressed={directory}
          onClick={handleDirectory}
        >
          {t('knowledgeDirectory.title')}
        </Button>
      </nav>
      {chooseDomain ? (
        <DomainPicker onSelect={handleDomainSelected} directory={directory} />
      ) : directory ? (
        <DirectoryImport
          key={`${id}:${getKnowledgeSessionScope()}`}
          domain={id}
        />
      ) : (
        <AuthorizedImport
          key={`${id}:${getKnowledgeSessionScope()}`}
          domainId={id}
        />
      )}
    </section>
  );
}

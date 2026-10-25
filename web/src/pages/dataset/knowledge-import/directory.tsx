import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import { useKnowledgeDomain } from '@/hooks/use-knowledge-foundation';
import { useDirectoryContract } from '@/hooks/use-knowledge-directory';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { DirectoryRequestError } from '@/services/knowledge-directory-service';
import authorizationStorage, {
  getAuthorization,
} from '@/utils/authorization-util';
import { directoryStorageKey } from '@/utils/knowledge-directory';
import { knowledgeReceiptLink } from '@/utils/knowledge-source-link';
import {
  CatalogueLoading,
  CatalogueScopeFailure,
} from '../knowledge-catalogue/state';
import {
  DirectoryRegistrationForm,
  DirectorySelection,
} from './directory-fields';
import { useDirectoryFlow } from './use-directory-flow';

function DirectoryFailure({
  error,
  onReload,
}: {
  error: unknown;
  onReload?: () => void;
}) {
  const { t } = useTranslation(),
    failure = error instanceof KnowledgeRequestError ? error : null;
  const key =
    failure?.code === 'knowledge_directory_version_mismatch'
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
                  : failure?.status === 415
                    ? 'unsupported'
                    : 'unavailable';
  return (
    <div
      role="alert"
      className="space-y-2 rounded-md border border-border-button p-4"
    >
      <p>{t('knowledgeDirectory.errors.' + key)}</p>
      {failure instanceof DirectoryRequestError && failure.reason && (
        <p>{t('knowledgeDirectory.reasons.' + failure.reason)}</p>
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
function DirectoryPreviewPanel({
  flow,
}: {
  flow: ReturnType<typeof useDirectoryFlow>;
}) {
  const { t } = useTranslation();
  const [consent, setConsent] = useState(false),
    [offsets, setOffsets] = useState([0]);
  const preview = flow.preview!,
    item = preview.items[0],
    window = flow.window;
  const handleConsent = (e: React.ChangeEvent<HTMLInputElement>) =>
    setConsent(e.target.checked);
  const handleConfirm = () => {
    if (consent) void flow.confirm();
  };
  const handleFirst = () => {
    setOffsets([0]);
    void flow.readWindow(0);
  };
  const handleNext = () => {
    if (window) {
      const next = window.offset + window.byte_length;
      setOffsets([...offsets, next]);
      void flow.readWindow(next);
    }
  };
  const handlePrevious = () => {
    const next = offsets.slice(0, -1);
    setOffsets(next);
    void flow.readWindow(next.at(-1) || 0);
  };
  return (
    <section
      aria-label={t('knowledgeImport.confirmation')}
      className="space-y-3 rounded-md border border-border-button bg-bg-card p-4"
    >
      <h2 className="text-lg font-semibold">
        {t('knowledgeImport.confirmation')}
      </h2>
      <p className="break-all">
        {item.relative_path} · {t('knowledgeImport.' + item.action)}
      </p>
      <p>
        {t('knowledgeCatalogue.' + item.classification.component_side)} ·{' '}
        {item.classification.language} ·{' '}
        {item.classification.group_id || t('knowledgeImport.noGroup')}
      </p>
      <p className="break-all">
        {item.byte_length} B · SHA-256 {item.sha256}
      </p>
      <p className="break-all">
        {t('knowledgeImport.expectedRevision')}:{' '}
        {item.expected_revision_id || t('knowledgeImport.firstImport')}
      </p>
      <p>
        {t('knowledgeImport.expires')}:{' '}
        {dayjs(preview.expires_at).format('YYYY-MM-DD HH:mm:ss')}
      </p>
      <p>{t('knowledgeDirectory.catalogueOnly')}</p>
      {window ? (
        <>
          <p>
            [{window.offset}, {window.offset + window.byte_length}) /{' '}
            {window.total_bytes} B
          </p>
          <pre
            data-testid="directory-preview-text"
            className="max-h-[50vh] overflow-auto whitespace-pre rounded-md border border-border-button p-3"
          >
            {window.text}
          </pre>
        </>
      ) : (
        <Button disabled={flow.busy} onClick={handleFirst}>
          {t('knowledgeDirectory.readPreview')}
        </Button>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={flow.busy || offsets.length === 1}
          onClick={handlePrevious}
        >
          {t('knowledgeCatalogue.previous')}
        </Button>
        <Button
          variant="outline"
          disabled={
            flow.busy ||
            !window ||
            window.offset + window.byte_length >= window.total_bytes
          }
          onClick={handleNext}
        >
          {t('knowledgeCatalogue.next')}
        </Button>
      </div>
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={consent}
          onChange={handleConsent}
          disabled={flow.busy || !window}
        />
        <span>{t('knowledgeDirectory.consent')}</span>
      </label>
      <Button
        disabled={flow.busy || !window || !consent}
        onClick={handleConfirm}
      >
        {t('knowledgeImport.confirm')}
      </Button>
    </section>
  );
}
function DirectoryWizard({
  domain,
  storageKey,
  onReload,
  onScopeFailure,
}: {
  domain: string;
  storageKey: string;
  onReload: () => void;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    flow = useDirectoryFlow(domain, storageKey, onScopeFailure);
  const [resendConsent, setResendConsent] = useState(false);
  const handleRecover = () => {
    setResendConsent(false);
    void flow.recover();
  };
  const handleResend = () => {
    if (resendConsent) {
      setResendConsent(false);
      void flow.resend();
    }
  };
  const handleResendConsent = (e: React.ChangeEvent<HTMLInputElement>) =>
    setResendConsent(e.target.checked);
  if (flow.blocked)
    return <DirectoryFailure error={flow.blocked} onReload={onReload} />;
  const value = flow.receipt?.data;
  return (
    <CatalogueScopeFailure.Provider value={flow.fail}>
      {!!flow.error && <DirectoryFailure error={flow.error} />}
      {flow.phase === 'registration' && (
        <DirectoryRegistrationForm
          domain={domain}
          busy={flow.busy}
          attempt={flow.attempt}
          onSubmit={flow.register}
          onError={flow.fail}
        />
      )}
      {flow.registration && !flow.pending && (
        <DirectorySelection
          domain={domain}
          registration={flow.registration}
          busy={flow.busy}
          observation={flow.observation}
          onObserve={flow.observe}
          onPreview={flow.makePreview}
          onChange={flow.changeSelection}
          onInvalidate={flow.invalidatePreview}
          onError={flow.fail}
        />
      )}
      {flow.preview && !flow.pending && (
        <DirectoryPreviewPanel key={flow.preview.preview_id} flow={flow} />
      )}
      {flow.pending && flow.phase !== 'receipt' && (
        <section
          aria-label={t('knowledgeImport.recovery')}
          className="space-y-3 py-3"
        >
          <p role="status">{t('knowledgeDirectory.pending')}</p>
          <p>{t('knowledgeDirectory.pendingHint')}</p>
          <Button disabled={flow.busy} onClick={handleRecover}>
            {t('knowledgeImport.checkResult')}
          </Button>
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={resendConsent}
              onChange={handleResendConsent}
              disabled={flow.busy}
            />
            {t('knowledgeDirectory.resendConsent')}
          </label>
          <Button
            variant="outline"
            disabled={flow.busy || !resendConsent}
            onClick={handleResend}
          >
            {t('knowledgeDirectory.resend')}
          </Button>
          {flow.canRestart && (
            <Button
              variant="outline"
              disabled={flow.busy}
              onClick={flow.restart}
            >
              {t('knowledgeDirectory.newObservation')}
            </Button>
          )}
        </section>
      )}
      {value && (
        <section
          aria-label={t('knowledgeImport.receipt')}
          className="space-y-3"
        >
          <h2 className="text-lg font-semibold">
            {t('knowledgeImport.' + value.status)}
          </h2>
          <p>
            HTTP {flow.receipt!.httpStatus} ·{' '}
            {t('knowledgeImport.receiptCreated')}
          </p>
          <p className="break-all">
            {value.items[0].relative_path} ·{' '}
            {t(
              'knowledgeCatalogue.' +
                value.items[0].classification.component_side,
            )}{' '}
            · {value.items[0].classification.language}
          </p>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
            <dt>{t('knowledgeImport.saveStatus')}</dt>
            <dd>
              {t('knowledgeDirectory.saved.' + value.items[0].save_status)}
            </dd>
            <dt>{t('knowledgeImport.catalogue')}</dt>
            <dd>
              {t(
                value.status === 'succeeded'
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
            <dt>{t('knowledgeCatalogue.revision')}</dt>
            <dd className="break-all">
              {value.items[0].source_revision_id || '—'}
            </dd>
            <dt>{t('knowledgeImport.receiptId')}</dt>
            <dd className="break-all">{value.import_task_id}</dd>
          </dl>
          {value.status === 'succeeded' && (
            <Link
              className="inline-block underline"
              to={knowledgeReceiptLink(domain, value.items[0])}
            >
              {t('knowledgeImport.readRevision')}
            </Link>
          )}
          <p>
            {t(
              value.status === 'failed'
                ? 'knowledgeDirectory.retryFailed'
                : 'knowledgeDirectory.savedHint',
            )}
          </p>
          <Button variant="outline" onClick={flow.restart}>
            {t('knowledgeDirectory.newObservation')}
          </Button>
        </section>
      )}
    </CatalogueScopeFailure.Provider>
  );
}
function AuthorizedDirectory({
  domain,
  onScopeFailure,
}: {
  domain: string;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    query = useKnowledgeDomain(domain),
    contract = useDirectoryContract();
  const [key, setKey] = useState(''),
    [keyError, setKeyError] = useState<unknown>(null),
    [generation, setGeneration] = useState(0);
  const authorization = getAuthorization();
  let userId = '';
  try {
    userId = authorizationStorage.getUserInfoObject?.()?.id || '';
  } catch {
    /* token-only session remains isolated */
  }
  useEffect(() => {
    let alive = true;
    setKey('');
    void directoryStorageKey(domain, authorization, userId)
      .then((value) => {
        if (alive) setKey(value);
      })
      .catch((error) => {
        if (alive) setKeyError(error);
      });
    return () => {
      alive = false;
    };
  }, [domain, authorization, userId]);
  const reload = () => {
    void Promise.all([query.refetch(), contract.refetch()]).then((results) => {
      if (results.every((result) => !result.isError))
        setGeneration((value) => value + 1);
    });
  };
  if (query.error || contract.error || keyError)
    return (
      <DirectoryFailure
        error={query.error || contract.error || keyError}
        onReload={reload}
      />
    );
  if (query.isPending || contract.isPending || !key)
    return <CatalogueLoading />;
  const value = query.data;
  if (
    value.domain_id !== domain ||
    value.contract_version !== '1.0.0' ||
    value.policy.access !== 'owner_only' ||
    value.policy.external_models ||
    value.policy.automatic_ingestion ||
    value.policy.allowed_model_uses.length ||
    value.policy.index_modes.length !== 1 ||
    value.policy.index_modes[0] !== 'catalogue'
  )
    return (
      <DirectoryFailure
        error={
          new DirectoryRequestError(409, 'knowledge_directory_version_mismatch')
        }
        onReload={reload}
      />
    );
  return (
    <>
      <h1 className="text-2xl font-semibold">
        {t('knowledgeDirectory.title')} · {value.name}
      </h1>
      <p className="text-sm text-text-secondary">
        {t('knowledgeDirectory.catalogueOnly')}
      </p>
      <DirectoryWizard
        key={key + ':' + generation}
        domain={domain}
        storageKey={key}
        onReload={reload}
        onScopeFailure={onScopeFailure}
      />
    </>
  );
}

export default function DirectoryImport({ domain }: { domain: string }) {
  const [failure, setFailure] = useState<unknown>(null);
  const [generation, setGeneration] = useState(0);
  const handleReload = () => {
    setFailure(null);
    setGeneration((value) => value + 1);
  };
  if (failure)
    return <DirectoryFailure error={failure} onReload={handleReload} />;
  return (
    <AuthorizedDirectory
      key={generation}
      domain={domain}
      onScopeFailure={setFailure}
    />
  );
}

import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  useKnowledgeDomain,
  getKnowledgeSessionScope,
} from '@/hooks/use-knowledge-foundation';
import { useOriginalContract } from '@/hooks/use-knowledge-original';
import { originalStorageKey } from '@/utils/knowledge-original';
import { OriginalId } from '@/utils/knowledge-original-schema';
import { CatalogueLoading } from '../knowledge-catalogue/state';
import {
  OriginalFailure,
  OriginalRegistrationForm,
  OriginalFileFields,
} from './original-fields';
import OriginalUploadPanel from './original-upload';
import OriginalConfirmation, {
  OriginalReceiptView,
} from './original-confirmation';
import OriginalRevisions, {
  OriginalRevisionDetail,
} from './original-revisions';
import { useOriginalFlow } from './use-original-flow';

function OriginalWorkspaceFlow({
  domain,
  owner,
  onReload,
}: {
  domain: string;
  owner: string;
  onReload: () => void;
}) {
  const { t } = useTranslation(),
    [params] = useSearchParams();
  const flow = useOriginalFlow(
    domain,
    owner,
    originalStorageKey(domain, owner),
  );
  const [lookup, setLookup] = useState(params.get('registration') || ''),
    [showList, setShowList] = useState(false);
  const lookupChange = (event: ChangeEvent<HTMLInputElement>) =>
    setLookup(event.target.value);
  const open = (event: FormEvent) => {
    event.preventDefault();
    void flow.openRegistration(lookup);
  };
  const recover = () => {
    void flow.recover();
  };
  const resend = () => {
    void flow.resend();
  };
  const another = () => {
    void flow.startAnother();
  };
  const toggleList = () => setShowList((value) => !value);
  const hasPending = !!flow.createPending || !!flow.confirmPending;
  const registrationRecovery =
    flow.createPending?.kind === 'registration' && flow.unknown;
  return (
    <div className="space-y-5">
      <p>{t('knowledgeOriginal.scope')}</p>
      {!!flow.error && <OriginalFailure error={flow.error} />}
      {flow.lockingUnavailable && (
        <p role="status">{t('knowledgeOriginal.lockingUnavailable')}</p>
      )}
      {(flow.recoveryChanged || flow.corruptRecovery) && (
        <Button variant="outline" onClick={onReload}>
          {t('knowledgeOriginal.reloadRecovery')}
        </Button>
      )}
      {flow.corruptRecovery ? (
        <p>{t('knowledgeOriginal.corruptRecovery')}</p>
      ) : (
        <>
          {hasPending &&
            (flow.unknown ||
              flow.blocked ||
              (!flow.registration && !flow.receipt)) && (
              <section
                className="space-y-3"
                aria-label={t('knowledgeOriginal.recovery')}
              >
                <p role="status">{t('knowledgeOriginal.unconfirmed')}</p>
                <Button onClick={recover} disabled={flow.busy}>
                  {t('knowledgeOriginal.checkResult')}
                </Button>
                {flow.canResend && !registrationRecovery && (
                  <Button
                    variant="outline"
                    onClick={resend}
                    disabled={flow.busy}
                  >
                    {t('knowledgeOriginal.resend')}
                  </Button>
                )}
                {flow.canReset && (
                  <Button
                    variant="outline"
                    onClick={another}
                    disabled={flow.busy}
                  >
                    {t('knowledgeOriginal.newPreparation')}
                  </Button>
                )}
              </section>
            )}
          {(!flow.blocked || (registrationRecovery && flow.canResend)) &&
            !flow.receipt &&
            !flow.registration &&
            (!flow.createPending || registrationRecovery) && (
              <>
                {!hasPending && !flow.freshRegistration && (
                  <form
                    onSubmit={open}
                    className="flex flex-wrap items-end gap-2"
                  >
                    <label>
                      <span>{t('knowledgeOriginal.registrationId')}</span>
                      <Input
                        required
                        pattern="[a-f0-9]{32}"
                        value={lookup}
                        onChange={lookupChange}
                        disabled={flow.busy}
                      />
                    </label>
                    <Button type="submit" disabled={flow.busy}>
                      {t('knowledgeOriginal.openRegistration')}
                    </Button>
                  </form>
                )}
                <OriginalRegistrationForm domain={domain} flow={flow} />
              </>
            )}
          {!flow.blocked &&
            flow.registration &&
            !flow.upload &&
            !flow.receipt &&
            !flow.unknown && (
              <OriginalFileFields
                key={flow.registration.registration_id}
                domain={domain}
                flow={flow}
              />
            )}
          {!flow.blocked && flow.upload && !flow.receipt && (
            <>
              <OriginalUploadPanel
                key={flow.upload.upload_id}
                domain={domain}
                flow={flow}
              />
              {flow.upload.state === 'ready' && !flow.confirmPending && (
                <OriginalConfirmation
                  key={
                    flow.upload.upload_id + ':' + flow.upload.manifest_sha256
                  }
                  flow={flow}
                />
              )}
              {['rejected', 'cancelled', 'expired'].includes(
                flow.upload.state,
              ) && (
                <Button
                  variant="outline"
                  onClick={another}
                  disabled={flow.busy}
                >
                  {t('knowledgeOriginal.newPreparation')}
                </Button>
              )}
            </>
          )}
          {!flow.blocked && flow.receipt && <OriginalReceiptView flow={flow} />}
        </>
      )}
      <Button variant="outline" onClick={toggleList} aria-expanded={showList}>
        {t('knowledgeOriginal.originals')}
      </Button>
      {showList && <OriginalRevisions domain={domain} />}
    </div>
  );
}
function OriginalWorkspace({
  domain,
  owner,
}: {
  domain: string;
  owner: string;
}) {
  const [revision, setRevision] = useState(0);
  const reload = () => setRevision((value) => value + 1);
  return (
    <OriginalWorkspaceFlow
      key={revision}
      domain={domain}
      owner={owner}
      onReload={reload}
    />
  );
}
export default function OriginalImport({ domain }: { domain: string }) {
  const { t } = useTranslation(),
    [params] = useSearchParams();
  const query = useKnowledgeDomain(domain),
    contract = useOriginalContract();
  const session = getKnowledgeSessionScope();
  // C1 authorizes this owner-only domain only when its owner is the native actor.
  // The login's local display profile intentionally contains no user ID.
  const owner = query.data?.owner_id || '';
  const retry = () => {
    void query.refetch();
    void contract.refetch();
  };
  if (query.error || contract.error)
    return (
      <OriginalFailure error={query.error || contract.error} onRetry={retry} />
    );
  if (!query.data || !contract.data) return <CatalogueLoading />;
  const policy = query.data.policy;
  if (
    query.data.domain_id !== domain ||
    policy.access !== 'owner_only' ||
    policy.external_models ||
    policy.automatic_ingestion ||
    policy.allowed_model_uses.length ||
    policy.index_modes.length !== 1 ||
    policy.index_modes[0] !== 'catalogue'
  )
    return <OriginalFailure error={new Error('Unsupported domain policy')} />;
  if (!contract.data.runtime_ready)
    return <p>{t('knowledgeOriginal.notReady')}</p>;
  const revision = params.get('original_revision');
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">
        {t('knowledgeOriginal.title')} · {query.data.name}
      </h1>
      {revision ? (
        <OriginalRevisionDetail
          key={session + ':' + revision}
          domain={domain}
          revision={revision}
          sourceId={params.get('original_source') || ''}
          sha={params.get('original_sha') || ''}
        />
      ) : OriginalId.test(owner) ? (
        <OriginalWorkspace
          key={session + ':' + domain + ':' + owner}
          domain={domain}
          owner={owner}
        />
      ) : (
        <p>{t('knowledgeOriginal.loginRequired')}</p>
      )}
    </div>
  );
}

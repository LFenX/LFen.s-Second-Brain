import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  useKnowledgeDomain,
  getKnowledgeSessionScope,
} from '@/hooks/use-knowledge-foundation';
import { useWebContract } from '@/hooks/use-knowledge-web';
import { WebId } from '@/utils/knowledge-web-schema';
import { CatalogueLoading } from '../knowledge-catalogue/state';
import { WebFailure, WebRegistrationForm, WebCaptureForm } from './web-fields';
import WebCapturePanel from './web-capture';
import WebObservationPanel from './web-observation';
import WebPreviewPanel from './web-preview';
import WebReceiptPanel from './web-receipt';
import WebRevisions, { WebRevisionDetail } from './web-revisions';
import { useWebFlow } from './use-web-flow';

function WebWorkspace({ domain, owner }: { domain: string; owner: string }) {
  const { t } = useTranslation(),
    [params] = useSearchParams(),
    flow = useWebFlow(domain, owner),
    [lookup, setLookup] = useState(params.get('registration') || ''),
    [showList, setShowList] = useState(false);
  const changeLookup = (event: ChangeEvent<HTMLInputElement>) =>
      setLookup(event.target.value),
    open = (event: FormEvent) => {
      event.preventDefault();
      void flow.openRegistration(lookup);
    },
    recover = () => {
      void flow.recover();
    },
    resend = () => {
      void flow.resend();
    },
    revise = () => {
      void flow.revisePreparation();
    },
    toggleList = () => setShowList((value) => !value);
  return (
    <div className="space-y-5">
      <p className="text-sm text-text-secondary">{t('knowledgeWeb.scope')}</p>
      {!!flow.error && <WebFailure error={flow.error} />}{' '}
      {flow.corrupt ? (
        <p role="alert">{t('knowledgeWeb.corruptRecovery')}</p>
      ) : (
        !flow.blocked && (
          <>
            {flow.unknown && (
              <section
                aria-label={t('knowledgeWeb.recovery')}
                className="space-y-3 rounded-lg border border-border-button p-4"
              >
                <p role="status">{t('knowledgeWeb.recoveryHint')}</p>
                <Button onClick={recover} disabled={flow.busy}>
                  {t('knowledgeWeb.checkOriginal')}
                </Button>
                {flow.canResend && (
                  <Button
                    variant="outline"
                    onClick={resend}
                    disabled={flow.busy}
                  >
                    {t('knowledgeWeb.resendOriginal')}
                  </Button>
                )}
                {flow.canRevise && (
                  <Button
                    variant="outline"
                    onClick={revise}
                    disabled={flow.busy}
                  >
                    {t('knowledgeWeb.reviseRejected')}
                  </Button>
                )}
              </section>
            )}
            {!flow.registration &&
              !flow.slot &&
              !flow.registrationPending &&
              !flow.unknown && (
                <>
                  <form
                    onSubmit={open}
                    className="flex flex-wrap items-end gap-2"
                  >
                    <label>
                      {t('knowledgeWeb.registrationId')}
                      <Input
                        required
                        pattern="[a-f0-9]{32}"
                        value={lookup}
                        onChange={changeLookup}
                        disabled={flow.busy}
                      />
                    </label>
                    <Button type="submit" disabled={flow.busy}>
                      {t('knowledgeWeb.openRegistration')}
                    </Button>
                  </form>
                  <WebRegistrationForm flow={flow} />
                </>
              )}
            {flow.registration && !flow.slot && !flow.unknown && (
              <WebCaptureForm
                key={flow.registration.registration_id}
                flow={flow}
              />
            )}{' '}
            {flow.capture && !flow.receipt && (
              <WebCapturePanel key={flow.capture.capture_id} flow={flow} />
            )}{' '}
            {flow.observation && !flow.preview && !flow.receipt && (
              <WebObservationPanel
                key={flow.observation.observation_id}
                flow={flow}
              />
            )}{' '}
            {flow.preview && !flow.receipt && (
              <WebPreviewPanel
                key={
                  flow.preview.preview_id + ':' + flow.preview.manifest_sha256
                }
                flow={flow}
              />
            )}{' '}
            {flow.receipt && <WebReceiptPanel flow={flow} />}{' '}
            {!flow.unknown &&
              !flow.receipt &&
              flow.capture &&
              ['cancelled', 'expired', 'rejected', 'interrupted'].includes(
                flow.capture.state,
              ) &&
              !flow.slot?.confirmation_request && (
                <Button
                  variant="outline"
                  disabled={flow.busy}
                  onClick={flow.startAnother}
                >
                  {t('knowledgeWeb.newCapture')}
                </Button>
              )}
          </>
        )
      )}
      {!flow.blocked && (
        <>
          <Button
            variant="outline"
            onClick={toggleList}
            aria-expanded={showList}
          >
            {t('knowledgeWeb.revisions')}
          </Button>
          {showList && <WebRevisions domain={domain} owner={owner} />}
        </>
      )}
    </div>
  );
}
export default function WebImport({ domain }: { domain: string }) {
  const { t } = useTranslation(),
    [params] = useSearchParams(),
    query = useKnowledgeDomain(domain),
    contract = useWebContract(),
    session = getKnowledgeSessionScope(),
    [failure, setFailure] = useState<unknown>(null);
  const retry = () => {
    setFailure(null);
    void query.refetch();
    void contract.refetch();
  };
  if (query.error || contract.error || failure)
    return (
      <WebFailure
        error={query.error || contract.error || failure}
        onRetry={retry}
      />
    );
  if (!query.data || !contract.data) return <CatalogueLoading />;
  const value = query.data,
    policy = value.policy,
    owner = value.owner_id;
  if (
    value.domain_id !== domain ||
    value.contract_version !== '1.0.0' ||
    policy.access !== 'owner_only' ||
    policy.external_models ||
    policy.automatic_ingestion ||
    policy.allowed_model_uses.length ||
    policy.index_modes.length !== 1 ||
    policy.index_modes[0] !== 'catalogue' ||
    !WebId.test(owner)
  )
    return <WebFailure error={Error('Unsupported domain policy')} />;
  if (!contract.data.runtime_ready) return <p>{t('knowledgeWeb.notReady')}</p>;
  const revision = params.get('web_revision');
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">
        {t('knowledgeWeb.title')} · {value.name}
      </h1>
      {revision ? (
        <WebRevisionDetail
          key={session + ':' + revision}
          domain={domain}
          owner={owner}
          revision={revision}
          source={params.get('web_source') || ''}
          inputSha={params.get('web_input_sha') || ''}
          onScopeFailure={setFailure}
        />
      ) : (
        <WebWorkspace
          key={session + ':' + owner + ':' + domain}
          domain={domain}
          owner={owner}
        />
      )}
    </div>
  );
}

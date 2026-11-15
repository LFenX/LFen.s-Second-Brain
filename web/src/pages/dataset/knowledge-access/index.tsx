import { useState, type ChangeEvent } from 'react';
import { useLocation, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  AccessCommandFeedback,
  AccessFailure,
} from '@/components/knowledge-access-feedback';
import {
  useAccessIdentity,
  useAccessModuleAdmin,
  useAccessModules,
} from '@/hooks/use-knowledge-access';
import {
  useAccessCommand,
  type AccessCommandController,
} from '@/hooks/use-knowledge-access-command';
import {
  KnowledgeAccessVersion,
  type AccessActor,
  type AccessPolicy,
} from '@/interfaces/database/knowledge-access';
import {
  KnowledgeAccessPaths,
  type AccessPublicationChoice,
} from '@/utils/knowledge-access-navigation';
import {
  accessSchemaValid,
  accessTargetsValid,
} from '@/utils/knowledge-access-schema';
import { useOwnerTenantId } from '../contexts/knowledge-base-context';
import { AccessModuleEditor } from './module-editor';
import { AccessGrants } from './grants';
import { AccessRevocationWarning } from './revocation-warning';

function SharingPolicy({
  policy,
  command,
}: {
  policy: AccessPolicy;
  command: AccessCommandController;
}) {
  const { t } = useTranslation();
  const [confirmed, setConfirmed] = useState(false);
  function handleConfirm(event: ChangeEvent<HTMLInputElement>) {
    setConfirmed(event.target.checked);
  }
  function handleChange() {
    if (!confirmed || command.locked) return;
    void command.start({
      operation: 'setPolicy',
      body: {
        contract_version: KnowledgeAccessVersion,
        domain_id: policy.domain_id,
        expected_version: policy.version,
        enabled: !policy.enabled,
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
    setConfirmed(false);
  }
  return (
    <section
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t('knowledgeAccess.policy')}
    >
      <h2 className="text-lg font-medium">{t('knowledgeAccess.policy')}</h2>
      <p>
        {t(
          policy.enabled
            ? 'knowledgeAccess.policyOn'
            : 'knowledgeAccess.policyOff',
        )}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.policyHint')}
      </p>
      {policy.enabled && <AccessRevocationWarning />}
      <label className="block space-x-2">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={handleConfirm}
          disabled={command.locked}
        />
        <span>{t('knowledgeAccess.confirmPolicy')}</span>
      </label>
      <Button disabled={command.locked || !confirmed} onClick={handleChange}>
        {t(
          policy.enabled
            ? 'knowledgeAccess.disablePolicy'
            : 'knowledgeAccess.enablePolicy',
        )}
      </Button>
    </section>
  );
}
function ExistingModule({
  actor,
  domain,
  module,
  choice,
  command,
}: {
  actor: AccessActor;
  domain: string;
  module: string;
  choice: AccessPublicationChoice | null;
  command: AccessCommandController;
}) {
  const { t } = useTranslation();
  const query = useAccessModuleAdmin(actor, domain, module);
  if (query.isFetching)
    return <p role="status">{t('knowledgeAccess.loading')}</p>;
  if (query.isError || !query.data)
    return <AccessFailure error={query.error} />;
  return (
    <div className="space-y-4">
      <AccessModuleEditor
        key={'module-editor:' + query.data.module_id + ':' + query.data.version}
        domain={domain}
        module={query.data}
        choice={choice}
        command={command}
      />
      <AccessGrants
        key={'module-grants:' + query.data.module_id + ':' + query.data.version}
        actor={actor}
        module={query.data}
        command={command}
      />
    </div>
  );
}
function OwnerAccessWorkspace({
  actor,
  domain,
  choice,
}: {
  actor: AccessActor;
  domain: string;
  choice: AccessPublicationChoice | null;
}) {
  const { t } = useTranslation();
  const modules = useAccessModules(actor, domain);
  const command = useAccessCommand(actor, domain);
  const [selected, setSelected] = useState('');
  const current =
    !modules.isFetching && !modules.isError ? modules.data : undefined;
  const policy = current?.owned_domain_policies.find(
    (item) => item.domain_id === domain,
  );
  const items =
    current?.items.filter((item) => item.administrative_owner) ?? [];
  const module = items.find((item) => item.module_id === selected);
  function handleSelect(event: ChangeEvent<HTMLSelectElement>) {
    setSelected(event.target.value);
  }
  function handleRefresh() {
    void modules.refetch();
  }
  return (
    <div className="space-y-4">
      <AccessCommandFeedback command={command} />
      <Button
        variant="outline"
        disabled={modules.isFetching || command.busy}
        onClick={handleRefresh}
      >
        {t('knowledgeAccess.refresh')}
      </Button>
      <AccessFailure error={modules.error} />
      {modules.isFetching && (
        <p role="status">{t('knowledgeAccess.loading')}</p>
      )}
      {current && !policy && (
        <p role="alert">{t('knowledgeAccess.ownerOnly')}</p>
      )}
      {policy && (
        <>
          <p className="text-sm text-text-secondary">
            {t('knowledgeAccess.tenantRenewalWarning')}
          </p>
          <SharingPolicy
            key={policy.version + ':' + policy.enabled}
            policy={policy}
            command={command}
          />
          <label className="block space-y-1">
            <span>{t('knowledgeAccess.module')}</span>
            <select
              className="block w-full rounded border border-border-button bg-bg-base p-2"
              value={module ? module.module_id : ''}
              onChange={handleSelect}
              disabled={command.locked}
            >
              <option value="">{t('knowledgeAccess.newModule')}</option>
              {items.map((item) => (
                <option key={item.module_id} value={item.module_id}>
                  {item.name}
                  {item.enabled ? '' : ' · ' + t('knowledgeAccess.disabled')}
                </option>
              ))}
            </select>
          </label>
          {module ? (
            <ExistingModule
              key={module.module_id}
              actor={actor}
              domain={domain}
              module={module.module_id}
              choice={choice}
              command={command}
            />
          ) : (
            <AccessModuleEditor
              key={choice?.scope.publication.id ?? 'empty'}
              domain={domain}
              module={null}
              choice={choice}
              command={command}
            />
          )}
          <Button
            asLink
            to={KnowledgeAccessPaths.catalogue(domain)}
            variant="outline"
          >
            {t('knowledgeAccess.choosePublication')}
          </Button>
        </>
      )}
    </div>
  );
}
export default function KnowledgeAccessPage() {
  const { t } = useTranslation();
  const { id: domain = '' } = useParams();
  const tenant = useOwnerTenantId();
  const identity = useAccessIdentity();
  const location = useLocation();
  const incoming = location.state?.accessPublication as
    | AccessPublicationChoice
    | undefined;
  const choice =
    incoming?.domain === domain &&
    incoming.names &&
    typeof incoming.names === 'object' &&
    !Array.isArray(incoming.names) &&
    Object.values(incoming.names).every((name) => typeof name === 'string') &&
    accessSchemaValid('ModuleScope', incoming.scope) &&
    accessTargetsValid(incoming.scope.targets, domain)
      ? incoming
      : null;
  return (
    <main className="min-h-0 min-w-0 overflow-auto p-6">
      <div className="mx-auto max-w-5xl space-y-5">
        <h1 className="text-2xl font-semibold">
          {t('knowledgeAccess.ownerTitle')}
        </h1>
        <AccessFailure error={identity.error} />
        {identity.isFetching && (
          <p role="status">{t('knowledgeAccess.loading')}</p>
        )}
        {identity.data &&
          !identity.isError &&
          !identity.isFetching &&
          tenant && (
            <OwnerAccessWorkspace
              key={
                identity.data.user +
                ':' +
                tenant +
                ':' +
                domain +
                ':' +
                identity.session
              }
              actor={{
                user: identity.data.user,
                tenant,
                session: identity.session,
                auth: 'native',
              }}
              domain={domain}
              choice={choice}
            />
          )}
      </div>
    </main>
  );
}

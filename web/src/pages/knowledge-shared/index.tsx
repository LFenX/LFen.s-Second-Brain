import { useLayoutEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  Field,
  FieldSelect,
  Panel,
  StatusText,
} from '@/components/form-fields';
import { RefreshCw } from 'lucide-react';
import { AccessFailure } from '@/components/knowledge-access-feedback';
import {
  useAccessIdentity,
  useAccessModules,
} from '@/hooks/use-knowledge-access';
import type { AccessActor } from '@/interfaces/database/knowledge-access';
import type { AccessIdentity } from '@/services/knowledge-access-native-service';
import { SharedKnowledgeReader } from './reader';
import { MintMCPCredential, OwnMCPCredentials } from './temporary-mcp';
import { accessSchemaValid } from '@/utils/knowledge-access-schema';
import notification from '@/utils/notification';

function OwnAccountID({ user }: { user: string }) {
  const { t } = useTranslation();
  const valid = accessSchemaValid('ID', user);
  const copyScope = useRef({ user, active: false });
  useLayoutEffect(() => {
    const scope = { user, active: true };
    copyScope.current = scope;
    return () => {
      scope.active = false;
    };
  }, [user]);
  async function handleCopy() {
    const scope = copyScope.current;
    function isCurrentCopy() {
      return (
        scope.active &&
        copyScope.current === scope &&
        scope.user === user &&
        accessSchemaValid('ID', scope.user)
      );
    }
    if (!valid || !isCurrentCopy()) return;
    try {
      await navigator.clipboard.writeText(user);
      if (isCurrentCopy())
        notification.success({ message: t('knowledgeAccess.accountIDCopied') });
    } catch {
      if (isCurrentCopy())
        notification.error({ message: t('knowledgeAccess.copyIDFailed') });
    }
  }
  return valid ? (
    <Panel
      title={t('knowledgeAccess.ownAccountID')}
      description={t('knowledgeAccess.ownAccountIDHint')}
      aria-label={t('knowledgeAccess.ownAccountID')}
      actions={
        <Button variant="outline" size="sm" onClick={handleCopy}>
          {t('knowledgeAccess.copyAccountID')}
        </Button>
      }
    >
      <code className="block break-all rounded-xl bg-bg-card px-3 py-2 font-mono text-xs text-text-primary">
        {user}
      </code>
    </Panel>
  ) : null;
}

function SharedModules({ actor }: { actor: AccessActor }) {
  const { t } = useTranslation();
  const query = useAccessModules(actor, null);
  const [selected, setSelected] = useState('');
  const items =
    query.isError || query.isFetching
      ? []
      : (query.data?.items ?? []).filter(
          (item) => item.enabled && item.modes.length > 0,
        );
  const module = items.find((item) => item.module_id === selected);
  function handleSelect(event: ChangeEvent<HTMLSelectElement>) {
    setSelected(event.target.value);
  }
  function handleRefresh() {
    setSelected('');
    void query.refetch();
  }
  const domainChoices = [...new Set(items.map((item) => item.domain_id))].map(
    (id) => ({
      id,
      label: items
        .filter((item) => item.domain_id === id)
        .map((item) => item.name)
        .join(' · '),
    }),
  );
  return (
    <div className="space-y-4">
      <AccessFailure error={query.error} />
      {query.isFetching ? (
        <StatusText role="status">{t('knowledgeAccess.loading')}</StatusText>
      ) : (
        <>
          {!query.isError && items.length === 0 && (
            <Panel>
              <StatusText>{t('knowledgeAccess.noSharedModules')}</StatusText>
              <Button
                variant="outline"
                onClick={handleRefresh}
                disabled={query.isFetching}
              >
                {t('knowledgeAccess.refresh')}
              </Button>
            </Panel>
          )}
          {items.length > 0 && (
            <div className="flex items-end gap-2">
              <Field label={t('knowledgeAccess.module')} className="flex-1">
                <FieldSelect
                  value={module ? selected : ''}
                  onChange={handleSelect}
                >
                  <option value="">{t('knowledgeAccess.chooseModule')}</option>
                  {items.map((item) => (
                    <option key={item.module_id} value={item.module_id}>
                      {item.name}
                    </option>
                  ))}
                </FieldSelect>
              </Field>
              <Button
                variant="outline"
                size="icon"
                className="size-9"
                onClick={handleRefresh}
                disabled={query.isFetching}
                aria-label={t('knowledgeAccess.refresh')}
                title={t('knowledgeAccess.refresh')}
              >
                <RefreshCw />
              </Button>
            </div>
          )}
          {module && (
            <MintMCPCredential
              key={'mint:' + module.module_id + ':' + module.version}
              actor={actor}
              module={module}
            />
          )}
          {module && (
            <SharedKnowledgeReader
              key={module.module_id + ':' + module.version}
              actor={actor}
              module={module}
            />
          )}
          {domainChoices.length > 0 && (
            <OwnMCPCredentials
              key={'own-credentials:' + actor.user + ':' + actor.tenant}
              actor={actor}
              domainChoices={domainChoices}
            />
          )}
        </>
      )}
    </div>
  );
}
function SharedTenants({
  identity,
  session,
}: {
  identity: AccessIdentity;
  session: number;
}) {
  const { t } = useTranslation();
  const [tenant, setTenant] = useState(identity.tenants[0]?.id ?? '');
  const current = identity.tenants.find((item) => item.id === tenant);
  function handleTenant(event: ChangeEvent<HTMLSelectElement>) {
    setTenant(event.target.value);
  }
  useLayoutEffect(() => {
    if (!current) setTenant(identity.tenants[0]?.id ?? '');
  }, [current, identity.tenants]);
  return (
    <div className="space-y-4">
      <Field label={t('knowledgeAccess.team')}>
        <FieldSelect value={current?.id ?? ''} onChange={handleTenant}>
          {identity.tenants.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </FieldSelect>
      </Field>
      {current && (
        <SharedModules
          key={identity.user + ':' + current.id + ':' + session}
          actor={{
            user: identity.user,
            tenant: current.id,
            session,
            auth: 'native',
          }}
        />
      )}
    </div>
  );
}
export default function SharedKnowledgePage() {
  const { t } = useTranslation();
  const identity = useAccessIdentity();
  return (
    <main className="size-full min-h-0 min-w-0 overflow-auto px-5 py-8">
      <div className="mx-auto max-w-3xl space-y-5">
        <header className="space-y-1.5">
          <h1 className="text-[26px] font-semibold tracking-tight">
            {t('knowledgeAccess.sharedWithMe')}
          </h1>
          <p className="text-sm leading-relaxed text-text-secondary">
            {t('knowledgeAccess.sharedWithMeHint')}
          </p>
        </header>
        <AccessFailure error={identity.error} />
        {identity.isFetching && (
          <StatusText role="status">{t('knowledgeAccess.loading')}</StatusText>
        )}
        {identity.data && !identity.isError && !identity.isFetching && (
          <>
            <OwnAccountID
              key={'own-account:' + identity.data.user + ':' + identity.session}
              user={identity.data.user}
            />
            <SharedTenants
              key={
                'shared-teams:' + identity.data.user + ':' + identity.session
              }
              identity={identity.data}
              session={identity.session}
            />
          </>
        )}
      </div>
    </main>
  );
}

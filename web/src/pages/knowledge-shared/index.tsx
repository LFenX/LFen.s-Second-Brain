import { useLayoutEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { AccessFailure } from '@/components/knowledge-access-feedback';
import {
  useAccessIdentity,
  useAccessModules,
} from '@/hooks/use-knowledge-access';
import type { AccessActor } from '@/interfaces/database/knowledge-access';
import type { AccessIdentity } from '@/services/knowledge-access-native-service';
import { SharedKnowledgeReader } from './reader';
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
    <section
      className="space-y-2 rounded border border-border-button p-3"
      aria-label={t('knowledgeAccess.ownAccountID')}
    >
      <h2>{t('knowledgeAccess.ownAccountID')}</h2>
      <code className="block break-all">{user}</code>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.ownAccountIDHint')}
      </p>
      <Button variant="outline" onClick={handleCopy}>
        {t('knowledgeAccess.copyAccountID')}
      </Button>
    </section>
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
  return (
    <div className="space-y-4">
      <Button
        variant="outline"
        onClick={handleRefresh}
        disabled={query.isFetching}
      >
        {t('knowledgeAccess.refresh')}
      </Button>
      <AccessFailure error={query.error} />
      {query.isFetching ? (
        <p role="status">{t('knowledgeAccess.loading')}</p>
      ) : (
        <>
          {!query.isError && items.length === 0 && (
            <p>{t('knowledgeAccess.noSharedModules')}</p>
          )}
          {items.length > 0 && (
            <label className="block space-y-1">
              <span>{t('knowledgeAccess.module')}</span>
              <select
                className="block w-full rounded border border-border-button bg-bg-base p-2"
                value={module ? selected : ''}
                onChange={handleSelect}
              >
                <option value="">{t('knowledgeAccess.chooseModule')}</option>
                {items.map((item) => (
                  <option key={item.module_id} value={item.module_id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          {module && (
            <SharedKnowledgeReader
              key={module.module_id + ':' + module.version}
              actor={actor}
              module={module}
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
      <label className="block space-y-1">
        <span>{t('knowledgeAccess.team')}</span>
        <select
          className="rounded border border-border-button bg-bg-base p-2"
          value={current?.id ?? ''}
          onChange={handleTenant}
        >
          {identity.tenants.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </select>
      </label>
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
    <main className="min-h-0 min-w-0 flex-1 overflow-auto p-6">
      <div className="mx-auto max-w-5xl space-y-5">
        <h1 className="text-2xl font-semibold">
          {t('knowledgeAccess.sharedWithMe')}
        </h1>
        <AccessFailure error={identity.error} />
        {identity.isFetching && (
          <p role="status">{t('knowledgeAccess.loading')}</p>
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

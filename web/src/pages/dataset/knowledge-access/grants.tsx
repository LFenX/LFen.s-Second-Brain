import { useState, type ChangeEvent, type FormEvent } from 'react';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AccessRevocationWarning } from './revocation-warning';
import { accessSchemaValid } from '@/utils/knowledge-access-schema';
import type { AccessCommandController } from '@/hooks/use-knowledge-access-command';
import {
  KnowledgeAccessVersion,
  type AccessActor,
  type AccessGrant,
  type AccessMode,
  type AccessModuleAdmin,
} from '@/interfaces/database/knowledge-access';

function RevokeGrant({
  grant,
  command,
  disabled,
}: {
  grant: AccessGrant;
  command: AccessCommandController;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [confirmed, setConfirmed] = useState(false);
  function handleConfirm(event: ChangeEvent<HTMLInputElement>) {
    setConfirmed(event.target.checked);
  }
  function handleRevoke() {
    if (!confirmed || command.locked) return;
    void command.start({
      operation: 'revokeGrant',
      body: {
        contract_version: KnowledgeAccessVersion,
        domain_id: grant.domain_id,
        grant_id: grant.grant_id,
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
    setConfirmed(false);
  }
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p>
        {t('knowledgeAccess.recipientID')}:{' '}
        <code className="break-all">{grant.subject.id}</code>
      </p>
      <p>
        {t(
          grant.mode === 'read'
            ? 'knowledgeAccess.readOnly'
            : 'knowledgeAccess.readSearch',
        )}
      </p>
      <p>
        {t('knowledgeAccess.expires', {
          time: dayjs(grant.expires_at).format('YYYY-MM-DD HH:mm'),
        })}
      </p>
      <AccessRevocationWarning />
      <label className="block space-x-2">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={handleConfirm}
          disabled={disabled || command.locked}
        />
        <span>{t('knowledgeAccess.confirmRevoke')}</span>
      </label>
      <Button
        variant="danger"
        disabled={disabled || command.locked || !confirmed}
        onClick={handleRevoke}
      >
        {t('knowledgeAccess.revokeGrant')}
      </Button>
    </li>
  );
}

function GrantForm({
  module,
  command,
}: {
  module: AccessModuleAdmin;
  command: AccessCommandController;
}) {
  const { t } = useTranslation();
  const [user, setUser] = useState('');
  const [mode, setMode] = useState<AccessMode>('read');
  const [expires, setExpires] = useState(() =>
    dayjs().add(7, 'day').format('YYYY-MM-DDTHH:mm'),
  );
  const [confirmed, setConfirmed] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [invalidUser, setInvalidUser] = useState(false);
  const available = module.enabled && module.policy.enabled;
  const validUser = accessSchemaValid('ID', user);
  function handleUser(event: ChangeEvent<HTMLInputElement>) {
    setUser(event.target.value);
    setConfirmed(false);
    setInvalidUser(false);
  }
  function handleMode(event: ChangeEvent<HTMLSelectElement>) {
    setMode(event.target.value as AccessMode);
    setConfirmed(false);
  }
  function handleExpires(event: ChangeEvent<HTMLInputElement>) {
    setExpires(event.target.value);
    setConfirmed(false);
    setInvalid(false);
  }
  function handleConfirm(event: ChangeEvent<HTMLInputElement>) {
    setConfirmed(event.target.checked);
  }
  function handleCreate(event: FormEvent) {
    event.preventDefault();
    if (!validUser) {
      setInvalidUser(true);
      return;
    }
    const expiry = dayjs(expires),
      now = dayjs();
    if (
      !expiry.isValid() ||
      !expiry.isAfter(now) ||
      expiry.isAfter(now.add(30, 'day'))
    ) {
      setInvalid(true);
      return;
    }
    if (!available || !confirmed || command.locked) return;
    void command.start({
      operation: 'createGrant',
      body: {
        contract_version: KnowledgeAccessVersion,
        domain_id: module.domain_id,
        module_id: module.module_id,
        expected_module_version: module.version,
        subject: { kind: 'user', id: user },
        mode,
        delivery: 'web_only',
        expires_at: expiry.toISOString(),
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
    setConfirmed(false);
  }
  const grants = module.grants.filter(
    (grant) =>
      grant.subject.kind === 'user' &&
      grant.delivery === 'web_only' &&
      grant.revoked_at === null,
  );
  return (
    <section
      className="space-y-4 rounded border border-border-button p-4"
      aria-label={t('knowledgeAccess.personalGrants')}
    >
      <h2 className="text-lg font-medium">
        {t('knowledgeAccess.personalGrants')}
      </h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.grantHint')}
      </p>
      <p className="text-sm text-text-secondary" id="access-recipient-id-help">
        {t('knowledgeAccess.recipientIDHint')}
      </p>
      {!available && <p>{t('knowledgeAccess.enableBeforeGrant')}</p>}
      <form
        onSubmit={handleCreate}
        className="space-y-3"
        aria-label={t('knowledgeAccess.createGrant')}
      >
        <label className="block space-y-1">
          <span>{t('knowledgeAccess.recipientID')}</span>
          <Input
            type="text"
            autoComplete="off"
            spellCheck={false}
            value={user}
            onChange={handleUser}
            disabled={!available || command.locked}
            required
            aria-describedby="access-recipient-id-help"
            aria-invalid={invalidUser}
          />
        </label>
        {invalidUser && (
          <p role="alert">{t('knowledgeAccess.invalidRecipientID')}</p>
        )}
        <label className="block space-y-1">
          <span>{t('knowledgeAccess.mode')}</span>
          <select
            className="rounded border border-border-button bg-bg-base p-2"
            value={mode}
            onChange={handleMode}
            disabled={!available || command.locked}
          >
            <option value="read">{t('knowledgeAccess.readOnly')}</option>
            <option value="read_search">
              {t('knowledgeAccess.readSearch')}
            </option>
          </select>
        </label>
        <label className="block space-y-1">
          <span>{t('knowledgeAccess.expiry')}</span>
          <Input
            type="datetime-local"
            required
            value={expires}
            onChange={handleExpires}
            disabled={!available || command.locked}
          />
        </label>
        {invalid && <p role="alert">{t('knowledgeAccess.invalidExpiry')}</p>}
        <div
          className="space-y-1 rounded border border-border-button p-3"
          role="group"
          aria-label={t('knowledgeAccess.grantReview')}
        >
          <p>
            {t('knowledgeAccess.recipientID')}:{' '}
            <code className="break-all">{user}</code>
          </p>
          <p className="break-all">
            {t('knowledgeAccess.grantScope', {
              domain: module.domain_id,
              module: module.module_id,
              version: module.version,
            })}
          </p>
          <p>
            {t('knowledgeAccess.mode')}:{' '}
            {t(
              mode === 'read'
                ? 'knowledgeAccess.readOnly'
                : 'knowledgeAccess.readSearch',
            )}
          </p>
          <p>{t('knowledgeAccess.webOnly')}</p>
          <p>
            {t('knowledgeAccess.expiry')}: {expires}
          </p>
        </div>
        <label className="block space-x-2">
          <input
            type="checkbox"
            checked={confirmed}
            onChange={handleConfirm}
            disabled={!available || command.locked || !validUser}
          />
          <span>{t('knowledgeAccess.confirmGrant')}</span>
        </label>
        <Button
          type="submit"
          disabled={!available || command.locked || !confirmed || !validUser}
        >
          {t('knowledgeAccess.createGrant')}
        </Button>
      </form>
      <h3>{t('knowledgeAccess.currentGrants')}</h3>
      {grants.length === 0 && <p>{t('knowledgeAccess.noGrants')}</p>}
      <ul className="space-y-3">
        {grants.map((grant) => (
          <RevokeGrant
            key={grant.grant_id}
            grant={grant}
            command={command}
            disabled={false}
          />
        ))}
      </ul>
    </section>
  );
}

export function AccessGrants({
  actor,
  module,
  command,
}: {
  actor: AccessActor;
  module: AccessModuleAdmin;
  command: AccessCommandController;
}) {
  const boundary = [
    actor.user,
    actor.tenant,
    actor.session,
    module.domain_id,
    module.module_id,
    module.version,
    module.enabled,
    module.policy.version,
    module.policy.enabled,
  ].join(':');
  return <GrantForm key={boundary} module={module} command={command} />;
}

import { useState, type ChangeEvent, type FormEvent } from 'react';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import { Field, FieldSelect, CheckRow } from '@/components/form-fields';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AccessRevocationWarning } from './revocation-warning';
import { AccessFailure } from '@/components/knowledge-access-feedback';
import { useAccessDepartments } from '@/hooks/use-knowledge-access';
import { accessSchemaValid } from '@/utils/knowledge-access-schema';
import type { AccessCommandController } from '@/hooks/use-knowledge-access-command';
import {
  KnowledgeAccessVersion,
  type AccessActor,
  type AccessDepartment,
  type AccessDelivery,
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
    <li className="space-y-2 rounded-2xl border-0.5 border-border-button p-3">
      <p>
        {t(
          grant.subject.kind === 'department'
            ? 'knowledgeAccess.department'
            : 'knowledgeAccess.recipientID',
        )}
        : <code className="break-all">{grant.subject.id}</code>
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
      <p>
        {t(
          grant.delivery === 'web_only'
            ? 'knowledgeAccess.webOnly'
            : 'knowledgeAccess.webAndMCP',
        )}
      </p>
      <AccessRevocationWarning />
      <CheckRow
        checked={confirmed}
        onChange={handleConfirm}
        disabled={disabled || command.locked}
      >
        {t('knowledgeAccess.confirmRevoke')}
      </CheckRow>
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
  kind,
  departments,
}: {
  module: AccessModuleAdmin;
  command: AccessCommandController;
  kind: 'user' | 'department';
  departments: AccessDepartment[];
}) {
  const { t } = useTranslation();
  const [user, setUser] = useState('');
  const [mode, setMode] = useState<AccessMode>('read');
  const [delivery, setDelivery] = useState<AccessDelivery>('web_only');
  const [expires, setExpires] = useState(() =>
    dayjs().add(7, 'day').format('YYYY-MM-DDTHH:mm'),
  );
  const [confirmed, setConfirmed] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [invalidUser, setInvalidUser] = useState(false);
  const available = module.enabled && module.policy.enabled;
  const validUser =
    accessSchemaValid('ID', user) &&
    (kind === 'user' ||
      departments.some((item) => item.department_id === user && item.enabled));
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
        subject: { kind, id: user },
        mode,
        delivery,
        expires_at: expiry.toISOString(),
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
    setConfirmed(false);
  }
  const grants = module.grants.filter(
    (grant) => grant.subject.kind === kind && grant.revoked_at === null,
  );
  return (
    <section
      className="space-y-4 rounded-3xl border-0.5 border-border-button bg-bg-component p-5"
      aria-label={t(
        kind === 'department'
          ? 'knowledgeAccess.departmentGrants'
          : 'knowledgeAccess.personalGrants',
      )}
    >
      <h2 className="text-base font-semibold">
        {t(
          kind === 'department'
            ? 'knowledgeAccess.departmentGrants'
            : 'knowledgeAccess.personalGrants',
        )}
      </h2>
      <p className="text-sm text-text-secondary">
        {t(
          kind === 'department'
            ? 'knowledgeAccess.departmentGrantHint'
            : 'knowledgeAccess.grantHint',
        )}
      </p>
      {kind === 'user' && (
        <p
          className="text-sm text-text-secondary"
          id="access-recipient-id-help"
        >
          {t('knowledgeAccess.recipientIDHint')}
        </p>
      )}
      {!available && <p>{t('knowledgeAccess.enableBeforeGrant')}</p>}
      <form
        onSubmit={handleCreate}
        className="space-y-3"
        aria-label={t(
          kind === 'department'
            ? 'knowledgeAccess.createDepartmentGrant'
            : 'knowledgeAccess.createGrant',
        )}
      >
        <label className="block space-y-1">
          <span>
            {t(
              kind === 'department'
                ? 'knowledgeAccess.department'
                : 'knowledgeAccess.recipientID',
            )}
          </span>
          {kind === 'user' ? (
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
          ) : (
            <FieldSelect
              value={user}
              onChange={(event) => {
                setUser(event.target.value);
                setConfirmed(false);
                setInvalidUser(false);
              }}
              disabled={!available || command.locked}
              required
            >
              <option value="">{t('knowledgeAccess.chooseDepartment')}</option>
              {departments.map((item) => (
                <option key={item.department_id} value={item.department_id}>
                  {item.name}
                </option>
              ))}
            </FieldSelect>
          )}
        </label>
        {invalidUser && (
          <p role="alert">{t('knowledgeAccess.invalidRecipientID')}</p>
        )}
        <Field label={t('knowledgeAccess.mode')}>
          <FieldSelect
            value={mode}
            onChange={handleMode}
            disabled={!available || command.locked}
          >
            <option value="read">{t('knowledgeAccess.readOnly')}</option>
            <option value="read_search">
              {t('knowledgeAccess.readSearch')}
            </option>
          </FieldSelect>
        </Field>
        <Field label={t('knowledgeAccess.delivery')}>
          <FieldSelect
            value={delivery}
            disabled={!available || command.locked}
            onChange={(event) => {
              setDelivery(event.target.value as AccessDelivery);
              setConfirmed(false);
            }}
          >
            <option value="web_only">{t('knowledgeAccess.webOnly')}</option>
            <option value="web_and_mcp">
              {t('knowledgeAccess.webAndMCP')}
            </option>
          </FieldSelect>
        </Field>
        <Field label={t('knowledgeAccess.expiry')}>
          <Input
            type="datetime-local"
            required
            value={expires}
            onChange={handleExpires}
            disabled={!available || command.locked}
          />
        </Field>
        {invalid && <p role="alert">{t('knowledgeAccess.invalidExpiry')}</p>}
        <div
          className="space-y-1 rounded-2xl border-0.5 border-border-button p-3"
          role="group"
          aria-label={t('knowledgeAccess.grantReview')}
        >
          <p>
            {t(
              kind === 'department'
                ? 'knowledgeAccess.department'
                : 'knowledgeAccess.recipientID',
            )}
            : <code className="break-all">{user}</code>
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
          <p>
            {t(
              delivery === 'web_only'
                ? 'knowledgeAccess.webOnly'
                : 'knowledgeAccess.webAndMCP',
            )}
          </p>
          <p>
            {t('knowledgeAccess.expiry')}: {expires}
          </p>
        </div>
        <CheckRow
          checked={confirmed}
          onChange={handleConfirm}
          disabled={!available || command.locked || !validUser}
        >
          {t(
            kind === 'department'
              ? 'knowledgeAccess.confirmDepartmentGrant'
              : 'knowledgeAccess.confirmGrant',
          )}
        </CheckRow>
        <Button
          type="submit"
          disabled={!available || command.locked || !confirmed || !validUser}
        >
          {t(
            kind === 'department'
              ? 'knowledgeAccess.createDepartmentGrant'
              : 'knowledgeAccess.createGrant',
          )}
        </Button>
      </form>
      <h3>{t('knowledgeAccess.currentGrants')}</h3>
      {grants.length === 0 && (
        <p>
          {t(
            kind === 'department'
              ? 'knowledgeAccess.noDepartmentGrants'
              : 'knowledgeAccess.noGrants',
          )}
        </p>
      )}
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
  const { t } = useTranslation();
  const [kind, setKind] = useState<'user' | 'department'>('user');
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
  return (
    <div className="space-y-4">
      <Field label={t('knowledgeAccess.recipientKind')}>
        <FieldSelect
          value={kind}
          disabled={command.locked}
          onChange={(event) =>
            setKind(event.target.value as 'user' | 'department')
          }
        >
          <option value="user">{t('knowledgeAccess.individual')}</option>
          <option value="department">{t('knowledgeAccess.department')}</option>
        </FieldSelect>
      </Field>
      {kind === 'user' ? (
        <GrantForm
          key={boundary + ':user'}
          module={module}
          command={command}
          kind="user"
          departments={[]}
        />
      ) : (
        <DepartmentGrantForm
          key={boundary + ':department'}
          actor={actor}
          module={module}
          command={command}
        />
      )}
    </div>
  );
}

function DepartmentGrantForm({
  actor,
  module,
  command,
}: {
  actor: AccessActor;
  module: AccessModuleAdmin;
  command: AccessCommandController;
}) {
  const { t } = useTranslation(),
    query = useAccessDepartments(actor);
  if (query.isFetching)
    return <p role="status">{t('knowledgeAccess.loading')}</p>;
  if (query.isError || !query.data)
    return <AccessFailure error={query.error} />;
  return (
    <GrantForm
      module={module}
      command={command}
      kind="department"
      departments={query.data.items.filter((item) => item.enabled)}
    />
  );
}

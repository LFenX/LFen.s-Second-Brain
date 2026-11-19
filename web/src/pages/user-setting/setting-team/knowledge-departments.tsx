import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Field, FieldSelect, CheckRow } from '@/components/form-fields';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  AccessCommandFeedback,
  AccessFailure,
} from '@/components/knowledge-access-feedback';
import {
  useAccessIdentity,
  useAccessDepartments,
  useAccessDepartmentMembers,
} from '@/hooks/use-knowledge-access';
import {
  useAccessCommand,
  type AccessCommandController,
} from '@/hooks/use-knowledge-access-command';
import {
  KnowledgeAccessVersion,
  type AccessActor,
  type AccessDepartment,
} from '@/interfaces/database/knowledge-access';
import { accessRequestValid } from '@/services/knowledge-access-service';

function DepartmentEditor({
  actor,
  department,
  members,
  command,
}: {
  actor: AccessActor;
  department: AccessDepartment | null;
  members: { id: string; name: string }[];
  command: AccessCommandController;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(department?.name ?? '');
  const [enabled, setEnabled] = useState(department?.enabled ?? true);
  const [selected, setSelected] = useState<string[]>(
    department?.user_ids ?? [],
  );
  const [confirmed, setConfirmed] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const inactive = selected.filter(
    (id) => !members.some((member) => member.id === id),
  );
  function toggle(id: string, checked: boolean) {
    setSelected((current) =>
      checked ? [...current, id] : current.filter((item) => item !== id),
    );
    setConfirmed(false);
    setInvalid(false);
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (command.locked || !confirmed || inactive.length) return;
    const body = {
      contract_version: KnowledgeAccessVersion,
      tenant_id: actor.tenant,
      department_id: department?.department_id ?? null,
      expected_version: department?.version ?? 0,
      name,
      enabled,
      user_ids: selected,
      confirmed: true as const,
      idempotency_key: crypto.randomUUID(),
    };
    if (!accessRequestValid('setDepartment', body)) {
      setInvalid(true);
      return;
    }
    setConfirmed(false);
    await command.start({ operation: 'setDepartment', body });
  }
  return (
    <form
      className="space-y-4"
      aria-label={t('knowledgeAccess.saveDepartment')}
      onSubmit={save}
    >
      <Field label={t('knowledgeAccess.departmentName')}>
        <Input
          value={name}
          required
          disabled={command.locked}
          onChange={(event) => {
            setName(event.target.value);
            setConfirmed(false);
            setInvalid(false);
          }}
        />
      </Field>
      <CheckRow
        checked={enabled}
        disabled={command.locked}
        onChange={(event) => {
          setEnabled(event.target.checked);
          setConfirmed(false);
        }}
      >
        {t('knowledgeAccess.departmentEnabled')}
      </CheckRow>
      <fieldset className="space-y-2">
        <legend>{t('knowledgeAccess.departmentMembers')}</legend>
        <div className="max-h-64 space-y-2 overflow-auto rounded-2xl border-0.5 border-border-button p-3">
          {members.map((member) => (
            <label key={member.id} className="flex items-center gap-2 text-sm">
              <input
                className="size-4 accent-[rgb(var(--accent-primary))]"
                type="checkbox"
                checked={selected.includes(member.id)}
                disabled={
                  command.locked ||
                  (!selected.includes(member.id) && selected.length >= 256)
                }
                onChange={(event) => toggle(member.id, event.target.checked)}
              />
              <span>{member.name || t('knowledgeAccess.currentAccount')}</span>
            </label>
          ))}
          {inactive.map((id) => (
            <label key={id} className="flex items-center gap-2 text-sm">
              <input
                className="size-4 accent-[rgb(var(--accent-primary))]"
                type="checkbox"
                checked
                disabled={command.locked}
                onChange={() => toggle(id, false)}
              />
              <span>
                {t('knowledgeAccess.inactiveDepartmentMember')}: {id}
              </span>
            </label>
          ))}
        </div>
      </fieldset>
      {inactive.length > 0 && (
        <p role="alert">{t('knowledgeAccess.removeInactiveMembers')}</p>
      )}
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.departmentRosterWarning')}
      </p>
      <CheckRow
        checked={confirmed}
        disabled={command.locked || inactive.length > 0}
        onChange={(event) => setConfirmed(event.target.checked)}
      >
        {t('knowledgeAccess.confirmDepartment')}
      </CheckRow>
      {invalid && <p role="alert">{t('knowledgeAccess.invalidInput')}</p>}
      <Button
        type="submit"
        disabled={
          command.locked || !confirmed || inactive.length > 0 || !name.trim()
        }
      >
        {t('knowledgeAccess.saveDepartment')}
      </Button>
    </form>
  );
}

function DepartmentWorkspace({ actor }: { actor: AccessActor }) {
  const { t } = useTranslation(),
    departments = useAccessDepartments(actor),
    members = useAccessDepartmentMembers(actor),
    command = useAccessCommand(actor, 'tenant:' + actor.tenant);
  const [selection, setSelection] = useState('');
  const busy = departments.isFetching || members.isFetching;
  const data =
    !busy && !departments.isError && !members.isError
      ? departments.data
      : undefined;
  const selected =
    data?.items.find((item) => item.department_id === selection) ?? null;
  const ownerRoster = data?.items.every(
    (item) => item.user_ids !== null && item.member_count !== null,
  );
  function refresh() {
    void departments.refetch();
    void members.refetch();
  }
  return (
    <div className="space-y-4">
      <AccessCommandFeedback command={command} />
      <Button
        variant="outline"
        disabled={busy || command.busy}
        onClick={refresh}
      >
        {t('knowledgeAccess.refresh')}
      </Button>
      <AccessFailure error={departments.error || members.error} />
      {busy && <p role="status">{t('knowledgeAccess.loading')}</p>}
      {data &&
        (!ownerRoster ? (
          <p role="alert">{t('knowledgeAccess.departmentOwnerOnly')}</p>
        ) : (
          <>
            <Field label={t('knowledgeAccess.department')}>
              <FieldSelect
                value={selected?.department_id ?? ''}
                disabled={command.locked}
                onChange={(event) => setSelection(event.target.value)}
              >
                <option value="">{t('knowledgeAccess.newDepartment')}</option>
                {data.items.map((item) => (
                  <option key={item.department_id} value={item.department_id}>
                    {item.name}
                    {item.enabled ? '' : ' · ' + t('knowledgeAccess.disabled')}
                  </option>
                ))}
              </FieldSelect>
            </Field>
            {members.data && (
              <DepartmentEditor
                key={
                  (selected?.department_id ?? 'new') +
                  ':' +
                  (selected?.version ?? 0)
                }
                actor={actor}
                department={selected}
                members={members.data}
                command={command}
              />
            )}
          </>
        ))}
    </div>
  );
}

export default function KnowledgeDepartments() {
  const { t } = useTranslation(),
    identity = useAccessIdentity();
  const [selection, setSelection] = useState('');
  const current =
    !identity.isFetching && !identity.isError ? identity.data : undefined;
  const teams = current?.tenants.filter((team) => team.role === 'owner') ?? [];
  const team = teams.find((item) => item.id === selection) ?? teams[0];
  return (
    <section
      className="space-y-4 rounded-3xl border-0.5 border-border-button bg-bg-component p-5"
      aria-label={t('knowledgeAccess.departments')}
    >
      <h2 className="text-base font-semibold">
        {t('knowledgeAccess.departments')}
      </h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.departmentHint')}
      </p>
      <AccessFailure error={identity.error} />
      {identity.isFetching && (
        <p role="status">{t('knowledgeAccess.loading')}</p>
      )}
      {current && !team && <p>{t('knowledgeAccess.departmentOwnerOnly')}</p>}
      {current && team && (
        <>
          <Field label={t('knowledgeAccess.team')}>
            <FieldSelect
              value={team.id}
              onChange={(event) => setSelection(event.target.value)}
            >
              {teams.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </FieldSelect>
          </Field>
          <DepartmentWorkspace
            key={[current.user, team.id, identity.session].join(':')}
            actor={{
              user: current.user,
              tenant: team.id,
              session: identity.session,
              auth: 'native',
            }}
          />
        </>
      )}
    </section>
  );
}

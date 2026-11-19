import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Field, CheckRow } from '@/components/form-fields';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AccessRevocationWarning } from './revocation-warning';
import type { AccessCommandController } from '@/hooks/use-knowledge-access-command';
import {
  KnowledgeAccessVersion,
  type AccessModuleAdmin,
} from '@/interfaces/database/knowledge-access';
import type { AccessPublicationChoice } from '@/utils/knowledge-access-navigation';

export function AccessModuleEditor({
  domain,
  module,
  choice,
  command,
}: {
  domain: string;
  module: AccessModuleAdmin | null;
  choice: AccessPublicationChoice | null;
  command: AccessCommandController;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(module?.name ?? '');
  const [enabled, setEnabled] = useState(module?.enabled ?? false);
  const [useChoice, setUseChoice] = useState(!module);
  const scope = useChoice ? choice?.scope : module?.scope;
  const [selected, setSelected] = useState(
    () => scope?.targets.map((target) => target.source.source_id) ?? [],
  );
  const [confirmed, setConfirmed] = useState(false);
  function handleName(event: ChangeEvent<HTMLInputElement>) {
    setName(event.target.value);
    setConfirmed(false);
  }
  function handleEnabled(event: ChangeEvent<HTMLInputElement>) {
    setEnabled(event.target.checked);
    setConfirmed(false);
  }
  function handleConfirmed(event: ChangeEvent<HTMLInputElement>) {
    setConfirmed(event.target.checked);
  }
  function handleReplacement(event: ChangeEvent<HTMLInputElement>) {
    setUseChoice(event.target.checked);
    setConfirmed(false);
    setSelected(
      (event.target.checked ? choice?.scope : module?.scope)?.targets.map(
        (target) => target.source.source_id,
      ) ?? [],
    );
  }
  function handleTarget(event: ChangeEvent<HTMLInputElement>) {
    const source = event.target.value;
    setSelected((values) =>
      event.target.checked
        ? [...values, source]
        : values.filter((id) => id !== source),
    );
    setConfirmed(false);
  }
  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!scope || !confirmed || command.locked || !selected.length) return;
    void command.start({
      operation: 'setModule',
      body: {
        contract_version: KnowledgeAccessVersion,
        domain_id: domain,
        module_id: module?.module_id ?? null,
        expected_version: module?.version ?? 0,
        name: name.trim(),
        enabled,
        scope: {
          ...scope,
          targets: scope.targets.filter((target) =>
            selected.includes(target.source.source_id),
          ),
        },
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
    setConfirmed(false);
  }
  return (
    <form
      onSubmit={handleSubmit}
      className="space-y-4 rounded-3xl border-0.5 border-border-button bg-bg-component p-5"
      aria-label={t(
        module ? 'knowledgeAccess.editModule' : 'knowledgeAccess.newModule',
      )}
    >
      <h2 className="text-base font-semibold">
        {t(module ? 'knowledgeAccess.editModule' : 'knowledgeAccess.newModule')}
      </h2>
      <Field label={t('knowledgeAccess.moduleName')}>
        <Input
          required
          maxLength={128}
          value={name}
          onChange={handleName}
          disabled={command.locked}
        />
      </Field>
      {module && choice && (
        <CheckRow
          checked={useChoice}
          onChange={handleReplacement}
          disabled={command.locked}
        >
          {t('knowledgeAccess.replaceSelection')}
        </CheckRow>
      )}
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.fixedSelectionHint')}
      </p>
      {scope?.targets.map((target, index) => (
        <div key={target.source.source_id} className="space-y-1">
          <CheckRow
            value={target.source.source_id}
            checked={selected.includes(target.source.source_id)}
            onChange={handleTarget}
            disabled={command.locked}
          >
            {choice?.names[target.source.source_id] ??
              t('knowledgeAccess.selectionNumber', { index: index + 1 })}
          </CheckRow>
          <details className="text-sm text-text-secondary">
            <summary>{t('knowledgeAccess.fixedRevision')}</summary>
            <dl className="space-y-1 break-all">
              <dt>{t('knowledgeAccess.sourceRevision')}</dt>
              <dd>{target.source.source_revision_id}</dd>
              <dt>{t('knowledgeAccess.sourceDigest')}</dt>
              <dd>{target.source.source_sha256}</dd>
              <dt>{t('knowledgeAccess.representation')}</dt>
              <dd>
                {target.representation.kind} · {target.representation.id}
              </dd>
              {target.representation.producer_sha256 && (
                <>
                  <dt>{t('knowledgeAccess.producerDigest')}</dt>
                  <dd>{target.representation.producer_sha256}</dd>
                </>
              )}
            </dl>
          </details>
        </div>
      ))}
      {!scope && <p>{t('knowledgeAccess.choosePublicationFirst')}</p>}
      <CheckRow
        checked={enabled}
        onChange={handleEnabled}
        disabled={command.locked}
      >
        {t('knowledgeAccess.moduleEnabled')}
      </CheckRow>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.moduleChangeWarning')}
      </p>
      {module?.enabled && !enabled && <AccessRevocationWarning />}
      <CheckRow
        checked={confirmed}
        onChange={handleConfirmed}
        disabled={command.locked}
      >
        {t('knowledgeAccess.confirmModule')}
      </CheckRow>
      <Button
        type="submit"
        disabled={
          command.locked ||
          !confirmed ||
          !scope ||
          !selected.length ||
          !name.trim()
        }
      >
        {t('knowledgeAccess.saveModule')}
      </Button>
    </form>
  );
}

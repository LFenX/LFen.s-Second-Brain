import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
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
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t(
        module ? 'knowledgeAccess.editModule' : 'knowledgeAccess.newModule',
      )}
    >
      <h2 className="text-lg font-medium">
        {t(module ? 'knowledgeAccess.editModule' : 'knowledgeAccess.newModule')}
      </h2>
      <label className="block space-y-1">
        <span>{t('knowledgeAccess.moduleName')}</span>
        <Input
          required
          maxLength={128}
          value={name}
          onChange={handleName}
          disabled={command.locked}
        />
      </label>
      {module && choice && (
        <label className="block space-x-2">
          <input
            type="checkbox"
            checked={useChoice}
            onChange={handleReplacement}
            disabled={command.locked}
          />
          <span>{t('knowledgeAccess.replaceSelection')}</span>
        </label>
      )}
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.fixedSelectionHint')}
      </p>
      {scope?.targets.map((target, index) => (
        <div key={target.source.source_id} className="space-y-1">
          <label className="block space-x-2">
            <input
              type="checkbox"
              value={target.source.source_id}
              checked={selected.includes(target.source.source_id)}
              onChange={handleTarget}
              disabled={command.locked}
            />
            <span>
              {choice?.names[target.source.source_id] ??
                t('knowledgeAccess.selectionNumber', { index: index + 1 })}
            </span>
          </label>
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
      <label className="block space-x-2">
        <input
          type="checkbox"
          checked={enabled}
          onChange={handleEnabled}
          disabled={command.locked}
        />
        <span>{t('knowledgeAccess.moduleEnabled')}</span>
      </label>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.moduleChangeWarning')}
      </p>
      {module?.enabled && !enabled && <AccessRevocationWarning />}
      <label className="block space-x-2">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={handleConfirmed}
          disabled={command.locked}
        />
        <span>{t('knowledgeAccess.confirmModule')}</span>
      </label>
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

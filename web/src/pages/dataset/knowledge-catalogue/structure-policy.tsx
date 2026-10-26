import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useStructurePolicy } from '@/hooks/use-knowledge-structure';
import type { StructureScope } from '@/utils/knowledge-structure';
import type { StructureFlow } from './use-structure-flow';
import { CatalogueLoading } from './state';
import { StructureFailure } from './structure-state';

export function StructurePolicyPanel({
  scope,
  flow,
}: {
  scope: StructureScope;
  flow: StructureFlow;
}) {
  const { t } = useTranslation(),
    query = useStructurePolicy(scope),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  useEffect(() => {
    setAck(false);
  }, [query.data?.structure_policy_version, query.data?.enabled]);
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const refresh = () => {
    setAck(false);
    flow.invalidatePreparation();
    void query.refetch();
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!ack || !query.data || flow.busy || !flow.ready) return;
    const policy = query.data;
    setAck(false);
    setError(null);
    try {
      await flow.submit({
        kind: 'policy',
        scope,
        body: {
          expected_policy_version: policy.foundation_policy_version,
          expected_structure_policy_version: policy.structure_policy_version,
          enabled: !policy.enabled,
          confirmed: true,
          idempotency_key: crypto.randomUUID(),
        },
      });
      await query.refetch();
    } catch (issue) {
      setError(issue);
    }
  };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={refresh} />;
  if (!query.data) return <CatalogueLoading />;
  const policy = query.data;
  return (
    <section className="space-y-3 rounded-md border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeStructure.policy')}</h3>
      <p>{t('knowledgeStructure.policyRules')}</p>
      <p>
        {t(
          policy.enabled
            ? 'knowledgeStructure.enabled'
            : 'knowledgeStructure.disabled',
        )}{' '}
        ·{' '}
        {t('knowledgeStructure.versions', {
          foundation: policy.foundation_policy_version,
          structure: policy.structure_policy_version,
        })}
      </p>
      <form onSubmit={submit} className="space-y-3">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={ack} onChange={acknowledge} />
          {t(
            policy.enabled
              ? 'knowledgeStructure.confirmDisable'
              : 'knowledgeStructure.confirmEnable',
          )}
        </label>
        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={!ack || !flow.ready || flow.busy}>
            {t(
              policy.enabled
                ? 'knowledgeStructure.disable'
                : 'knowledgeStructure.enable',
            )}
          </Button>
          <Button type="button" variant="outline" onClick={refresh}>
            {t('knowledgeCatalogue.refresh')}
          </Button>
        </div>
      </form>
      {!!error && <StructureFailure error={error} />}
    </section>
  );
}

import { useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { useDocumentPolicy } from '@/hooks/use-knowledge-document';
import type { DocumentScope } from '@/utils/knowledge-document';
import type { DocumentFlow } from './use-document-flow';
import { CatalogueLoading } from './state';
import { DocumentFailure } from './document-state';
export function DocumentPolicyPanel({
  scope,
  flow,
}: {
  scope: DocumentScope;
  flow: DocumentFlow;
}) {
  const { t } = useTranslation(),
    query = useDocumentPolicy(scope),
    [ackFor, setAckFor] = useState('');
  const snapshot = query.data
    ? `${query.data.foundation_policy_version}:${query.data.document_policy_version}:${query.data.enabled}`
    : '';
  const ack = !!snapshot && snapshot === ackFor;
  const checked = (e: ChangeEvent<HTMLInputElement>) =>
    setAckFor(e.target.checked ? snapshot : '');
  const submit = async () => {
    if (!ack || !query.data) return;
    const p = query.data;
    setAckFor('');
    try {
      await flow.submit({
        kind: 'policy',
        scope,
        body: {
          expected_policy_version: p.foundation_policy_version,
          expected_document_policy_version: p.document_policy_version,
          enabled: !p.enabled,
          confirmed: true,
          idempotency_key: crypto.randomUUID(),
        },
      });
    } catch {
      /* The durable flow retains the original request. */
    }
  };
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending) return <CatalogueLoading />;
  const p = query.data;
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeDocument.policy')}</h3>
      <p>
        {t(
          p.enabled
            ? 'knowledgeDocument.enabled'
            : 'knowledgeDocument.disabled',
        )}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeDocument.policyRules')}
      </p>
      <p className="text-xs">
        {t('knowledgeDocument.versions', {
          foundation: p.foundation_policy_version,
          document: p.document_policy_version,
        })}
      </p>
      <label className="flex gap-2">
        <input type="checkbox" checked={ack} onChange={checked} />
        {t(
          p.enabled
            ? 'knowledgeDocument.confirmDisable'
            : 'knowledgeDocument.confirmEnable',
        )}
      </label>
      <Button disabled={!ack || flow.busy || !flow.ready} onClick={submit}>
        {t(
          p.enabled ? 'knowledgeDocument.disable' : 'knowledgeDocument.enable',
        )}
      </Button>
    </section>
  );
}

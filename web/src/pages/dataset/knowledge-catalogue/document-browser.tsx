import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import {
  useDocumentContract,
  KnowledgeDocumentKeys,
} from '@/hooks/use-knowledge-document';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import { DocumentId, DocumentHash } from '@/utils/knowledge-document-schema';
import type { OriginalRevision } from '@/interfaces/database/knowledge-original';
import type {
  DocumentScope,
  DocumentViewScope,
} from '@/utils/knowledge-document';
import { useDocumentFlow } from './use-document-flow';
import { DocumentPolicyPanel } from './document-policy';
import { DocumentRecovery, DocumentTaskPanel } from './document-tasks';
import { DocumentOriginalList, DocumentPreparation } from './document-prepare';
import { DocumentView } from './document-view';
import { DocumentFailure } from './document-state';
import { CatalogueLoading } from './state';
export function documentRoute(
  params: URLSearchParams,
  scope: DocumentScope,
): { scope: DocumentViewScope; node?: string } | null {
  const keys = [
    'document_source',
    'document_revision',
    'document_original_sha',
    'document_representation',
    'document_node',
  ];
  if (
    params.getAll('document_structure').length > 1 ||
    keys.some((k) => params.getAll(k).length > 1)
  )
    throw Error('document_invalid_fixed_target');
  const hasTarget = keys.some((k) => params.has(k));
  if (!hasTarget) return null;
  const source = params.get('document_source') ?? '',
    revision = params.get('document_revision') ?? '',
    sha = params.get('document_original_sha') ?? '',
    representation = params.get('document_representation') ?? '',
    node = params.get('document_node');
  if (
    params.get('document_structure') !== '1' ||
    !DocumentId.test(source) ||
    !DocumentId.test(revision) ||
    !DocumentHash.test(sha) ||
    !DocumentHash.test(representation) ||
    (node !== null && !DocumentHash.test(node))
  )
    throw Error('document_invalid_fixed_target');
  return {
    scope: { ...scope, source, revision, sha, representation },
    ...(node ? { node } : {}),
  };
}
export default function DocumentBrowser({
  domain,
  owner,
}: {
  domain: string;
  owner: string;
}) {
  const [session] = useState(getKnowledgeSessionScope()),
    [changed, setChanged] = useState(false),
    client = useQueryClient();
  useEffect(() => {
    const changedSession = () => {
      if (session !== getKnowledgeSessionScope()) {
        setChanged(true);
        void client.cancelQueries({ queryKey: KnowledgeDocumentKeys.root() });
        client.removeQueries({ queryKey: KnowledgeDocumentKeys.root() });
      }
    };
    window.addEventListener('storage', changedSession);
    window.addEventListener('focus', changedSession);
    return () => {
      window.removeEventListener('storage', changedSession);
      window.removeEventListener('focus', changedSession);
    };
  }, [client, session]);
  if (changed || session !== getKnowledgeSessionScope())
    return <DocumentFailure error={Error('document_session_changed')} />;
  return (
    <DocumentSession
      key={session + ':' + owner + ':' + domain}
      scope={{ domain, owner }}
    />
  );
}
function DocumentSession({ scope }: { scope: DocumentScope }) {
  const { t } = useTranslation(),
    [params] = useSearchParams(),
    caps = useDocumentContract(),
    flow = useDocumentFlow(scope),
    [selected, setSelected] = useState<OriginalRevision | null>(null),
    [task, setTask] = useState<string | null>(null);
  let target: ReturnType<typeof documentRoute> = null,
    error: unknown = null;
  try {
    target = documentRoute(params, scope);
  } catch (e) {
    error = e;
  }
  if (error) return <DocumentFailure error={error} />;
  if (caps.isError) return <DocumentFailure error={caps.error} />;
  if (caps.isPending) return <CatalogueLoading />;
  return (
    <div className="space-y-5">
      <h2 className="text-xl font-semibold">{t('knowledgeDocument.title')}</h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeDocument.intro')}
      </p>
      {!caps.data.runtime_ready && (
        <p role="status">{t('knowledgeDocument.runtimeNotReady')}</p>
      )}
      <DocumentPolicyPanel scope={scope} flow={flow} />
      <DocumentRecovery flow={flow} onTask={setTask} />
      {task && (
        <DocumentTaskPanel key={task} scope={scope} task={task} flow={flow} />
      )}{' '}
      {target ? (
        <DocumentView
          key={target.scope.representation}
          scope={target.scope}
          node={target.node}
        />
      ) : (
        <>
          <DocumentOriginalList scope={scope} onSelect={setSelected} />
          {selected && (
            <DocumentPreparation
              key={selected.source_revision_id}
              scope={scope}
              original={selected}
              flow={flow}
              onTask={setTask}
            />
          )}
        </>
      )}
    </div>
  );
}

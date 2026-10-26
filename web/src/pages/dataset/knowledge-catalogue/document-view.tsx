import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  useDocumentRepresentation,
  useDocumentNode,
  useDocumentHead,
  useDocumentFixedOriginal,
  useDocumentActions,
} from '@/hooks/use-knowledge-document';
import type {
  DocumentRepresentation,
  DocumentNode,
} from '@/interfaces/database/knowledge-document';
import {
  documentFixedLink,
  type DocumentViewScope,
} from '@/utils/knowledge-document';
import { originalRevisionLink } from '@/utils/knowledge-original';
import { CatalogueLoading } from './state';
import { DocumentFailure } from './document-state';
import { DocumentTree } from './document-tree';
import { DocumentBlockView } from './document-blocks';
export function DocumentView({
  scope,
  node,
}: {
  scope: DocumentViewScope;
  node?: string;
}) {
  const query = useDocumentRepresentation(scope);
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending) return <CatalogueLoading />;
  return (
    <DocumentPublished
      key={scope.representation}
      scope={scope}
      representation={query.data}
      node={node}
    />
  );
}
function DocumentPublished({
  scope,
  representation,
  node,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node?: string;
}) {
  const { t } = useTranslation(),
    root = useDocumentNode(
      scope,
      representation,
      representation.root_document_node_id,
    ),
    head = useDocumentHead(scope),
    [, setParams] = useSearchParams();
  const select = (id: string) =>
    setParams(new URLSearchParams(documentFixedLink(scope, id).split('?')[1]));
  if (root.isError || head.isError)
    return <DocumentFailure error={root.error ?? head.error} />;
  if (root.isPending || head.isPending) return <CatalogueLoading />;
  const stale =
    head.data.currentness !== 'current' ||
    head.data.active_representation_id !== scope.representation;
  return (
    <section className="space-y-4">
      <h3 className="break-all text-xl font-semibold">
        {representation.original.original_filename}
      </h3>
      <p>
        {t(
          stale
            ? 'knowledgeDocument.fixedHistory'
            : 'knowledgeDocument.currentness_current',
        )}
      </p>
      {head.data.reasons.map((reason) => (
        <p key={reason}>{t('knowledgeDocument.reason_' + reason)}</p>
      ))}
      <p className="text-sm text-text-secondary">
        {t('knowledgeDocument.coverage', {
          count: representation.unsupported_block_count,
        })}
      </p>
      {representation.format === 'docx' && (
        <p className="text-sm text-text-secondary">
          {t('knowledgeDocument.noWordPages')}
        </p>
      )}
      <DocumentOriginalDownload scope={scope} representation={representation} />
      <div className="grid min-w-0 grid-cols-1 gap-4 2xl:grid-cols-[minmax(12rem,1fr)_minmax(0,2fr)]">
        <nav
          aria-label={t('knowledgeDocument.tree')}
          className="min-w-0 overflow-auto"
        >
          <ul>
            <DocumentTree
              scope={scope}
              representation={representation}
              node={root.data}
              selected={node ?? root.data.document_node_id}
              onSelect={select}
            />
          </ul>
        </nav>
        <main aria-label={t('knowledgeDocument.reading')} className="min-w-0">
          {!node || node === root.data.document_node_id ? (
            <DocumentSelected
              scope={scope}
              representation={representation}
              node={root.data}
            />
          ) : (
            <DocumentSelectedQuery
              key={node}
              scope={scope}
              representation={representation}
              id={node}
            />
          )}
        </main>
      </div>
      <details>
        <summary>{t('knowledgeDocument.evidence')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(representation, null, 2)}
        </pre>
      </details>
    </section>
  );
}
function DocumentSelectedQuery({
  scope,
  representation,
  id,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  id: string;
}) {
  const query = useDocumentNode(scope, representation, id);
  if (query.isError) return <DocumentFailure error={query.error} />;
  if (query.isPending) return <CatalogueLoading />;
  return (
    <DocumentSelected
      scope={scope}
      representation={representation}
      node={query.data}
    />
  );
}
function DocumentSelected({
  scope,
  representation,
  node,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node: DocumentNode;
}) {
  return (
    <DocumentBlockView
      key={node.document_node_id}
      scope={scope}
      representation={representation}
      node={node}
      selected
    />
  );
}
function DocumentOriginalDownload({
  scope,
  representation,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
}) {
  const { t } = useTranslation(),
    original = useDocumentFixedOriginal(scope, representation),
    actions = useDocumentActions(scope),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  const download = async () => {
    setBusy(true);
    setError(null);
    try {
      const value = await actions.download(scope, representation),
        url = URL.createObjectURL(value.blob),
        anchor = document.createElement('a');
      try {
        anchor.href = url;
        anchor.download = value.original.original_filename;
        anchor.click();
      } finally {
        URL.revokeObjectURL(url);
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  if (original.isError || error)
    return <DocumentFailure error={original.error ?? error} />;
  if (original.isPending) return <CatalogueLoading />;
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Link className="underline" to={originalRevisionLink(original.data)}>
        {t('knowledgeDocument.originalLink')}
      </Link>
      <Button variant="outline" disabled={busy} onClick={download}>
        {t('knowledgeDocument.download')}
      </Button>
    </div>
  );
}

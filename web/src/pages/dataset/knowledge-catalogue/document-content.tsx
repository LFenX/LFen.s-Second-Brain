import { Fragment, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  useDocumentContent,
  scopedDocument,
} from '@/hooks/use-knowledge-document';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import type {
  DocumentNode,
  DocumentRepresentation,
} from '@/interfaces/database/knowledge-document';
import type { DocumentContentQuery } from '@/interfaces/request/knowledge-document';
import { documentBytes } from '@/utils/knowledge-document-schema';
import {
  appendDocumentContent,
  documentSlice,
  type DocumentViewScope,
} from '@/utils/knowledge-document';
import { CatalogueLoading } from './state';
import { DocumentFailure } from './document-state';
export function DocumentMedia({ node }: { node: DocumentNode }) {
  const { t } = useTranslation();
  if (node.payload.kind !== 'media') return null;
  const a = node.payload.attributes;
  return (
    <span className="mx-1 inline-block rounded border border-border-button p-1 text-xs text-text-secondary">
      {t('knowledgeDocument.media')} ·{' '}
      {a.media_type ?? t('knowledgeDocument.unknown')} · {a.byte_length ?? '?'}{' '}
      B · {t('knowledgeDocument.notOcr')}
      {a.origin === 'external_reference' &&
        ' · ' + t('knowledgeDocument.externalNotLoaded')}
    </span>
  );
}
export function DocumentAnchoredText({
  text,
  start,
  media,
}: {
  text: string;
  start: number;
  media: DocumentNode[];
}) {
  const anchors = media
    .filter(
      (n) =>
        n.payload.kind === 'media' &&
        n.payload.attributes.placement === 'inline' &&
        n.payload.attributes.inline_anchor_byte !== null &&
        n.payload.attributes.inline_anchor_byte >= start &&
        n.payload.attributes.inline_anchor_byte <= start + documentBytes(text),
    )
    .sort((a, b) => a.payload.ordinal - b.payload.ordinal);
  let at = 0;
  const parts = anchors.map((node) => {
    if (node.payload.kind !== 'media') return null;
    const next = node.payload.attributes.inline_anchor_byte! - start;
    if (next < at) throw Error('document_integrity');
    const before = documentSlice(text, at, next);
    at = next;
    return (
      <Fragment key={node.document_node_id}>
        {before}
        <DocumentMedia node={node} />
      </Fragment>
    );
  });
  return (
    <div className="whitespace-pre-wrap break-words">
      {parts}
      {documentSlice(text, at, documentBytes(text))}
    </div>
  );
}
export function DocumentText({
  scope,
  representation,
  node,
  aggregate = false,
  media = [],
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node: DocumentNode;
  aggregate?: boolean;
  media?: DocumentNode[];
}) {
  const { t } = useTranslation(),
    range = aggregate
      ? node.payload.reading_range
      : node.payload.own_reading_range;
  if (!range) return null;
  if (range.start_byte === range.end_byte)
    return (
      <p className="text-sm text-text-secondary">
        {t(
          aggregate
            ? 'knowledgeDocument.text_' + node.payload.text_status
            : 'knowledgeDocument.emptyOwn',
        )}
      </p>
    );
  return (
    <DocumentTextWindows
      key={node.document_node_id + ':' + aggregate}
      scope={scope}
      representation={representation}
      node={node}
      range={range}
      media={media}
    />
  );
}
function DocumentTextWindows({
  scope,
  representation,
  node,
  range,
  media,
}: {
  scope: DocumentViewScope;
  representation: DocumentRepresentation;
  node: DocumentNode;
  range: { start_byte: number; end_byte: number; text_sha256: string };
  media: DocumentNode[];
}) {
  const { t } = useTranslation(),
    [start, setStart] = useState(range.start_byte),
    [text, setText] = useState(''),
    [end, setEnd] = useState(range.start_byte),
    [error, setError] = useState<unknown>(null);
  const params: DocumentContentQuery =
    range.end_byte - start <= 65536
      ? { start_byte: start, end_byte: range.end_byte }
      : { start_byte: start };
  const query = useDocumentContent(scope, representation, node, params);
  const accumulated = useRef(''),
    session = getKnowledgeSessionScope();
  const {
    start_byte: rangeStart,
    end_byte: rangeEnd,
    text_sha256: rangeSha,
  } = range;
  useEffect(() => {
    let active = true;
    if (query.data)
      void scopedDocument(session, () =>
        appendDocumentContent(accumulated.current, query.data!, {
          start_byte: rangeStart,
          end_byte: rangeEnd,
          text_sha256: rangeSha,
        }),
      ).then(
        (value) => {
          if (active) {
            accumulated.current = value;
            setText(value);
            setEnd(query.data!.end_byte);
          }
        },
        (issue) => {
          if (active) setError(issue);
        },
      );
    return () => {
      active = false;
    };
  }, [query.data, rangeStart, rangeEnd, rangeSha, session]);
  const more = () => setStart(end);
  if (query.isError || error)
    return <DocumentFailure error={query.error ?? error} />;
  return (
    <div className="space-y-2">
      {!!text && (
        <DocumentAnchoredText
          text={text}
          start={range.start_byte}
          media={media}
        />
      )}{' '}
      {query.isFetching && <CatalogueLoading />}
      {end < range.end_byte && !query.isFetching && (
        <Button variant="outline" onClick={more}>
          {t('knowledgeDocument.moreText')}
        </Button>
      )}
      <p className="text-xs text-text-secondary">
        {end === range.end_byte
          ? t('knowledgeDocument.textVerified')
          : t('knowledgeDocument.fragmentVerified')}
      </p>
    </div>
  );
}

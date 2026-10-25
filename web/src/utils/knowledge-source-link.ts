import type { KnowledgeSavedItem } from '@/interfaces/database/knowledge-sources';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';

export interface KnowledgeFixedTarget {
  sourceId: string;
  revisionId: string;
  rootNodeId: string;
  sha256: string;
}

export function knowledgeReceiptLink(
  domainId: string,
  item: Pick<
    KnowledgeSavedItem,
    'source_id' | 'source_revision_id' | 'root_node_id' | 'sha256'
  >,
) {
  const params = new URLSearchParams({
    source: item.source_id,
    revision: item.source_revision_id,
    node: item.root_node_id,
    sha256: item.sha256,
  });
  return `/dataset/knowledge-catalogue/${encodeURIComponent(domainId)}?${params}`;
}

export function knowledgeFixedTarget(
  params: URLSearchParams,
): KnowledgeFixedTarget | null {
  const names = ['source', 'revision', 'node', 'sha256'];
  if (!names.some((name) => params.has(name))) return null;
  const values = names.map((name) => params.get(name) || '');
  if (
    names.some((name) => params.getAll(name).length !== 1) ||
    values.some(
      (value, index) =>
        !new RegExp(`^[a-f0-9]{${index < 2 ? 32 : 64}}$`).test(value),
    )
  )
    throw new KnowledgeRequestError(409, 'knowledge_invalid_response');
  return {
    sourceId: values[0],
    revisionId: values[1],
    rootNodeId: values[2],
    sha256: values[3],
  };
}

import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import type {
  KnowledgeGroup,
  KnowledgeNode,
  KnowledgeSource,
  KnowledgePage,
} from '@/interfaces/database/knowledge-foundation';
import service from '@/services/knowledge-foundation-service';
import { getAuthorization } from '@/utils/authorization-util';

let SessionToken = '';
let SessionVersion = 0;
function sessionScope() {
  const token = getAuthorization() || '';
  if (token !== SessionToken) {
    SessionToken = token;
    SessionVersion += 1;
  }
  return SessionVersion;
}

export const KnowledgeKeys = {
  contract: (session: number) => ['knowledge', session, 'contract'] as const,
  domain: (session: number, domainId: string) =>
    ['knowledge', session, domainId, 'domain'] as const,
  groups: (session: number, domainId: string, parentId?: string) =>
    ['knowledge', session, domainId, 'groups', parentId] as const,
  sources: (session: number, domainId: string, groupId?: string) =>
    ['knowledge', session, domainId, 'sources', groupId] as const,
  nodes: (
    session: number,
    domainId: string,
    revisionId: string,
    parentId?: string,
  ) => ['knowledge', session, domainId, 'nodes', revisionId, parentId] as const,
  node: (session: number, domainId: string, nodeId: string) =>
    ['knowledge', session, domainId, 'node', nodeId] as const,
  content: (
    session: number,
    domainId: string,
    nodeId: string,
    start: number,
    end: number,
  ) => ['knowledge', session, domainId, 'content', nodeId, start, end] as const,
};

const QueryOptions = {
  retry: false,
  staleTime: 0,
  gcTime: 0,
  refetchOnWindowFocus: true,
} as const;

export function useKnowledgeContract() {
  return useQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.contract(sessionScope()),
    queryFn: ({ signal }) => service.contract(signal),
  });
}

export function useKnowledgeDomain(domainId: string) {
  return useQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.domain(sessionScope(), domainId),
    queryFn: ({ signal }) => service.domain(domainId, signal),
  });
}

function nextCursor<T>(page: KnowledgePage<T>) {
  return page.next_cursor || undefined;
}

export function useKnowledgeGroups(domainId: string, parentId?: string) {
  return useInfiniteQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.groups(sessionScope(), domainId, parentId),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: nextCursor<KnowledgeGroup>,
    queryFn: ({ pageParam, signal }) =>
      service.groups(
        domainId,
        { parent_id: parentId, cursor: pageParam },
        signal,
      ),
  });
}

export function useKnowledgeSources(domainId: string, groupId?: string) {
  return useInfiniteQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.sources(sessionScope(), domainId, groupId),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: nextCursor<KnowledgeSource>,
    queryFn: ({ pageParam, signal }) =>
      service.sources(
        domainId,
        { group_id: groupId, cursor: pageParam },
        signal,
      ),
  });
}

export function useKnowledgeNodes(
  domainId: string,
  revisionId: string,
  parentId?: string,
) {
  return useInfiniteQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.nodes(
      sessionScope(),
      domainId,
      revisionId,
      parentId,
    ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: nextCursor<KnowledgeNode>,
    queryFn: ({ pageParam, signal }) =>
      service.nodes(
        domainId,
        {
          source_revision_id: revisionId,
          parent_id: parentId,
          cursor: pageParam,
        },
        signal,
      ),
  });
}

export function useKnowledgeNode(domainId: string, nodeId: string) {
  return useQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.node(sessionScope(), domainId, nodeId),
    queryFn: ({ signal }) => service.node(domainId, nodeId, signal),
  });
}

export function useKnowledgeContent(
  domainId: string,
  nodeId: string,
  start: number,
  end: number,
) {
  return useQuery({
    ...QueryOptions,
    queryKey: KnowledgeKeys.content(
      sessionScope(),
      domainId,
      nodeId,
      start,
      end,
    ),
    queryFn: ({ signal }) =>
      service.content(
        domainId,
        nodeId,
        { start_byte: start, end_byte: end },
        signal,
      ),
  });
}

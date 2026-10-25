import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import service, {
  DirectoryRequestError,
  type DirectoryResponse,
} from '@/services/knowledge-directory-service';
import type { DirectoryReceipt } from '@/interfaces/database/knowledge-directory';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from './use-knowledge-foundation';

export const KnowledgeDirectoryKeys = {
  contract: (session: number) =>
    ['knowledge-directory', session, 'contract'] as const,
  scope: (session: number, domain: string) =>
    ['knowledge-directory', session, domain] as const,
  roots: (session: number, domain: string) =>
    ['knowledge-directory', session, domain, 'roots'] as const,
  action: (session: number, domain: string) =>
    ['knowledge-directory', session, domain, 'action'] as const,
};
const Options = {
  retry: false,
  gcTime: 0,
  staleTime: 0,
  refetchOnWindowFocus: false,
} as const;
export function useDirectoryContract() {
  return useQuery({
    ...Options,
    queryKey: KnowledgeDirectoryKeys.contract(getKnowledgeSessionScope()),
    queryFn: async ({ signal }) => (await service.contract(signal)).data,
  });
}
export function useDirectoryRoots(domain: string) {
  return useQuery({
    ...Options,
    queryKey: KnowledgeDirectoryKeys.roots(getKnowledgeSessionScope(), domain),
    queryFn: async ({ signal }) => (await service.roots(domain, signal)).data,
  });
}
export function useDirectoryActions(domain: string) {
  const session = getKnowledgeSessionScope(),
    client = useQueryClient();
  const mutation = useMutation({
    retry: false,
    gcTime: 0,
    mutationKey: KnowledgeDirectoryKeys.action(session, domain),
    mutationFn: async (work: () => Promise<unknown>) => {
      if (getKnowledgeSessionScope() !== session)
        throw new DirectoryRequestError(401, 'knowledge_unauthorized');
      const value = await work();
      if (getKnowledgeSessionScope() !== session)
        throw new DirectoryRequestError(401, 'knowledge_unauthorized');
      return value;
    },
  });
  async function run<T>(work: () => Promise<DirectoryResponse<T>>) {
    const result = (await mutation.mutateAsync(work)) as DirectoryResponse<T>;
    if ((result.data as Partial<DirectoryReceipt>).status === 'succeeded')
      void client.invalidateQueries({
        queryKey: KnowledgeKeys.domainScope(session, domain),
      });
    return result;
  }
  return { run, reset: mutation.reset };
}

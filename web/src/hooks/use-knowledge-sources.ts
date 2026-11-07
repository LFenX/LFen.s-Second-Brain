import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type {
  KnowledgeCommitRequest,
  KnowledgePreviewRequest,
  KnowledgeRegistrationRequest,
} from '@/interfaces/request/knowledge-sources';
import service from '@/services/knowledge-sources-service';
import type { KnowledgeImportReceipt } from '@/interfaces/database/knowledge-sources';
import { KnowledgeConfigurationKeys } from './knowledge-configuration-keys';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  getKnowledgeSessionScope,
  KnowledgeKeys,
} from './use-knowledge-foundation';

export const KnowledgeImportKeys = {
  contract: (session: number) =>
    ['knowledge-import', session, 'contract'] as const,
  scope: (session: number, domainId: string) =>
    ['knowledge-import', session, domainId] as const,
  action: (session: number, domainId: string, operation: string) =>
    ['knowledge-import', session, domainId, operation] as const,
};

export function useKnowledgeImportContract() {
  return useQuery({
    queryKey: KnowledgeImportKeys.contract(getKnowledgeSessionScope()),
    queryFn: ({ signal }) => service.contract(signal),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: true,
  });
}

export function useKnowledgeImportActions(domainId: string) {
  const session = getKnowledgeSessionScope();
  const client = useQueryClient();
  async function guarded<T>(work: () => Promise<T>) {
    const value = await work();
    if (getKnowledgeSessionScope() !== session)
      throw new KnowledgeRequestError(401, 'knowledge_unauthorized');
    return value;
  }
  const options = { retry: false, gcTime: 0 } as const;
  const refreshImportedSource = (receipt: KnowledgeImportReceipt) => {
    if (
      getKnowledgeSessionScope() !== session ||
      receipt.domain_id !== domainId ||
      receipt.status !== 'succeeded'
    )
      return;
    void client.invalidateQueries({
      queryKey: KnowledgeKeys.domainScope(session, domainId),
    });
    void client.invalidateQueries({
      queryKey: KnowledgeConfigurationKeys.detail(domainId),
      exact: true,
    });
  };
  const register = useMutation({
    ...options,
    mutationKey: KnowledgeImportKeys.action(session, domainId, 'register'),
    mutationFn: (request: KnowledgeRegistrationRequest) =>
      guarded(() => service.register(domainId, request)),
  });
  const preview = useMutation({
    ...options,
    mutationKey: KnowledgeImportKeys.action(session, domainId, 'preview'),
    mutationFn: ({
      registrationId,
      request,
    }: {
      registrationId: string;
      request: KnowledgePreviewRequest;
    }) => guarded(() => service.preview(domainId, registrationId, request)),
  });
  const confirm = useMutation({
    ...options,
    mutationKey: KnowledgeImportKeys.action(session, domainId, 'confirm'),
    mutationFn: ({
      registrationId,
      request,
    }: {
      registrationId: string;
      request: KnowledgeCommitRequest;
    }) => guarded(() => service.confirm(domainId, registrationId, request)),
    onSuccess: (result) => refreshImportedSource(result.data),
  });
  const readPreview = useMutation({
    ...options,
    mutationKey: KnowledgeImportKeys.action(session, domainId, 'read-preview'),
    mutationFn: ({
      registrationId,
      previewId,
    }: {
      registrationId: string;
      previewId: string;
    }) =>
      guarded(() => service.readPreview(domainId, registrationId, previewId)),
  });
  const receipt = useMutation({
    ...options,
    mutationKey: KnowledgeImportKeys.action(session, domainId, 'receipt'),
    mutationFn: ({
      registrationId,
      taskId,
    }: {
      registrationId: string;
      taskId: string;
    }) => guarded(() => service.receipt(domainId, registrationId, taskId)),
    onSuccess: refreshImportedSource,
  });
  const registration = useMutation({
    ...options,
    mutationKey: KnowledgeImportKeys.action(session, domainId, 'registration'),
    mutationFn: (id: string) =>
      guarded(() => service.registration(domainId, id)),
  });
  return { register, preview, confirm, readPreview, receipt, registration };
}

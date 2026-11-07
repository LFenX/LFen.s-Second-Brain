export const KnowledgeConfigurationKeys = {
  all: () => ['fetchKnowledgeDetail'] as const,
  detail: (knowledgeBaseId: string | null | undefined) =>
    [...KnowledgeConfigurationKeys.all(), knowledgeBaseId] as const,
};

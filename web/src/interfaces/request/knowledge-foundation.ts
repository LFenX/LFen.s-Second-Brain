import type {
  ComponentSide,
  KnowledgeDomain,
} from '@/interfaces/database/knowledge-foundation';

export interface RegisterKnowledgeDomainRequest {
  category?: KnowledgeDomain['category'];
}

export interface CreateKnowledgeGroupRequest {
  parent_id?: string | null;
  kind: 'project' | 'topic' | 'module';
  name: string;
  component_side?: ComponentSide;
}

export interface KnowledgeListParams {
  cursor?: string;
  limit?: number;
}

export interface KnowledgeContentParams {
  start_byte?: number;
  end_byte?: number;
}

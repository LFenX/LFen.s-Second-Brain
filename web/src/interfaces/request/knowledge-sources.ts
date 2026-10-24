import type { ComponentSide } from '../database/knowledge-foundation';

// DTOs consume docs/knowledge-sources-v1.openapi.json (frozen C2 0.2.0).
export interface KnowledgeRegistrationRequest {
  registration_key: string;
  name: string;
  source_kind: 'text_file';
  input_mode: 'inline_text';
  include_paths: string[];
  group_id: string | null;
  component_side: ComponentSide;
  language: string;
}

export interface KnowledgePreviewRequest {
  relative_path: string;
  text: string;
  group_id: string | null;
  component_side: ComponentSide;
  language: string;
}

export interface KnowledgeCommitRequest {
  preview_id: string;
  manifest_sha256: string;
  idempotency_key: string;
}

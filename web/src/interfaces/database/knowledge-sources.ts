import type { ComponentSide } from './knowledge-foundation';
import type { KnowledgeRegistrationRequest } from '../request/knowledge-sources';

export interface KnowledgeRegistration extends KnowledgeRegistrationRequest {
  registration_id: string;
  domain_id: string;
  created_at: string;
}

export interface KnowledgeImportClassification {
  item_key: string;
  relative_path: string;
  group_id: string | null;
  component_side: ComponentSide;
  language: string;
  sha256: string;
  byte_length: number;
}

export interface KnowledgePreviewItem extends KnowledgeImportClassification {
  action: 'add' | 'update' | 'unchanged';
  source_id: string | null;
  expected_revision_id: string | null;
}

export interface KnowledgeImportPreview {
  preview_id: string;
  registration_id: string;
  domain_id: string;
  policy_version: number;
  manifest_sha256: string;
  expires_at: string;
  consumed_task_id: string | null;
  items: [KnowledgePreviewItem];
}

interface KnowledgeImportItem extends KnowledgeImportClassification {
  structure_status: 'deferred';
  full_text_status: 'disabled';
  vector_status: 'disabled';
}

export interface KnowledgeSavedItem extends KnowledgeImportItem {
  save_status: 'saved' | 'unchanged';
  catalogue_status: 'ready';
  source_id: string;
  source_revision_id: string;
  root_node_id: string;
  code: 'ok';
}

export interface KnowledgeFailedItem extends KnowledgeImportItem {
  save_status: 'failed';
  catalogue_status: 'failed';
  source_id: null;
  source_revision_id: null;
  root_node_id: null;
  code: 'knowledge_import_publish_failed';
}

interface KnowledgeReceiptBase {
  import_task_id: string;
  registration_id: string;
  domain_id: string;
  preview_id: string;
  manifest_sha256: string;
  model_calls: 0;
  created_at: string;
}

export type KnowledgeImportReceipt = KnowledgeReceiptBase &
  (
    | { status: 'succeeded'; items: [KnowledgeSavedItem] }
    | { status: 'failed'; items: [KnowledgeFailedItem] }
  );

export interface KnowledgeImportCapabilities {
  version: '0.2.0';
  status: 'Frozen';
  foundation_contract_version: '1.0.0';
  schema: 'docs/knowledge-sources-v1.openapi.json';
  source_kinds: ['text_file'];
  input_modes: ['inline_text'];
  max_preview_items: 1;
  max_include_paths: 16;
  source_bytes: 2097152;
  request_bytes: 16777216;
  preview_ttl_seconds: 600;
  task_execution: 'synchronous_single_item';
  processing: {
    mode: 'catalogue_only';
    structure: 'deferred';
    full_text: 'disabled';
    vector: 'disabled';
    external_models: false;
    automatic_ingestion: false;
    allowed_model_uses: [];
    model_calls: 0;
  };
}

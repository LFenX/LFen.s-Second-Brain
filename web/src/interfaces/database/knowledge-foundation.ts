export type ComponentSide =
  | 'frontend'
  | 'backend'
  | 'shared'
  | 'cli'
  | 'non_code';

export interface KnowledgePolicy {
  version: 1;
  access: 'owner_only';
  index_modes: ['catalogue'];
  allowed_model_uses: [];
  external_models: false;
  automatic_ingestion: false;
}

export interface KnowledgeDomain {
  domain_id: string;
  name: string;
  category:
    | 'family'
    | 'personal'
    | 'project'
    | 'experience'
    | 'resource'
    | 'custom';
  owner_id: string;
  contract_version: '1.0.0';
  policy: KnowledgePolicy;
}

export interface KnowledgeGroup {
  id: string;
  domain_id: string;
  parent_id: string | null;
  kind: 'project' | 'topic' | 'module';
  name: string;
  component_side: ComponentSide;
}

export interface KnowledgeSource {
  source_revision_id: string;
  domain_id: string;
  source_id: string;
  native_file_id: string;
  native_commit_id: string;
  group_id: string | null;
  relative_path: string;
  component_side: ComponentSide;
  language: string;
  sha256: string;
  byte_length: number;
  line_count: number;
  is_current: boolean;
  root_node_id: string;
}

export interface KnowledgeNode {
  node_id: string;
  logical_node_id: string;
  domain_id: string;
  source_revision_id: string;
  parent_id: string | null;
  kind: 'file' | 'section' | 'symbol' | 'passage';
  logical_key: string;
  structure_version: 'source-v1' | 'manual-v1';
  name: string;
  start_byte: number;
  end_byte: number;
  start_line: number;
  end_line: number;
}

export interface KnowledgeContent {
  node_id: string;
  source_id: string;
  source_revision_id: string;
  source_sha256: string;
  start_byte: number;
  end_byte: number;
  text: string;
  encoding: 'utf-8';
}

export interface KnowledgePage<T> {
  items: T[];
  next_cursor: string;
}

export interface KnowledgeCapabilities {
  version: '1.0.0';
  contracts: string[];
  model_calls: false;
  ingestion: false;
  limits: {
    content_bytes: number;
    source_bytes: number;
    page_size: number;
    depth: number;
  };
}

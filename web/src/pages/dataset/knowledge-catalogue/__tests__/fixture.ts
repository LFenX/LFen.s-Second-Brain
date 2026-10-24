import Example from '../../../../../../docs/knowledge-foundation-v1.example.json';
import type {
  KnowledgeDomain,
  KnowledgeNode,
  KnowledgeSource,
  KnowledgeGroup,
  KnowledgeContent,
} from '@/interfaces/database/knowledge-foundation';

export const DomainId = Example.identity_inputs.domain_id;
export const SourceText = Example.source_text;
export const Root: KnowledgeNode = {
  node_id: Example.expected.node_id,
  logical_node_id: Example.expected.logical_node_id,
  domain_id: DomainId,
  source_revision_id: Example.identity_inputs.source_revision_id,
  parent_id: null,
  kind: 'file',
  logical_key: '.',
  structure_version: 'source-v1',
  name: 'demo.tsx',
  start_byte: 0,
  end_byte: Example.expected.byte_length,
  start_line: 1,
  end_line: 2,
};
export const Source: KnowledgeSource = {
  source_revision_id: Root.source_revision_id,
  domain_id: DomainId,
  source_id: Example.identity_inputs.source_id,
  native_file_id: 'a'.repeat(32),
  native_commit_id: 'b'.repeat(32),
  group_id: '4'.repeat(32),
  relative_path: 'web/demo.tsx',
  component_side: 'frontend',
  language: 'tsx',
  sha256: Example.expected.source_sha256,
  byte_length: Example.expected.byte_length,
  line_count: Example.expected.line_count,
  is_current: true,
  root_node_id: Root.node_id,
};
export const Domain: KnowledgeDomain = {
  domain_id: DomainId,
  name: 'Synthetic private domain',
  category: 'project',
  owner_id: 'owner',
  contract_version: '1.0.0',
  policy: {
    version: 1,
    access: 'owner_only',
    index_modes: ['catalogue'],
    allowed_model_uses: [],
    external_models: false,
    automatic_ingestion: false,
  },
};
export const Group: KnowledgeGroup = {
  id: '4'.repeat(32),
  domain_id: DomainId,
  parent_id: null,
  kind: 'project',
  name: 'Synthetic project',
  component_side: 'backend',
};
export const Child: KnowledgeNode = {
  ...Root,
  node_id: 'c'.repeat(64),
  logical_node_id: 'd'.repeat(64),
  kind: 'symbol',
  structure_version: 'manual-v1',
  parent_id: Root.node_id,
  name: 'value',
  logical_key: 'value',
  start_byte: 8,
  end_byte: 24,
  start_line: 2,
  end_line: 2,
};
export const Content: KnowledgeContent = {
  node_id: Root.node_id,
  source_id: Source.source_id,
  source_revision_id: Source.source_revision_id,
  source_sha256: Source.sha256,
  start_byte: 0,
  end_byte: Root.end_byte,
  encoding: 'utf-8',
  text: SourceText,
};
export const Capabilities = {
  version: '1.0.0',
  contracts: ['C1', 'C3'],
  schema: 'foundation',
  identity: 'opaque-source-and-structure-versioned',
  capabilities: [],
  limits: {
    source_bytes: 2097152,
    content_bytes: 65536,
    page_size: 200,
    depth: 16,
  },
  model_calls: false,
  ingestion: false,
};
export const envelope = (data: unknown) => ({
  code: 0,
  message: '',
  data,
  contract_version: '1.0.0',
  request_id: 'e'.repeat(32),
});
export const page = (items: unknown[], next_cursor = '') => ({
  items,
  next_cursor,
});

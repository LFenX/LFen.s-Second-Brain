import { createHash } from 'node:crypto';
import type {
  KnowledgeImportCapabilities,
  KnowledgeImportPreview,
  KnowledgeImportReceipt,
  KnowledgeRegistration,
} from '@/interfaces/database/knowledge-sources';
import type {
  KnowledgePreviewRequest,
  KnowledgeRegistrationRequest,
} from '@/interfaces/request/knowledge-sources';
import {
  DomainId,
  Source,
  SourceText,
} from '../../knowledge-catalogue/__tests__/fixture';

export const RegistrationRequest: KnowledgeRegistrationRequest = {
  registration_key: 'synthetic-ui',
  name: 'Synthetic source',
  source_kind: 'text_file',
  input_mode: 'inline_text',
  include_paths: [Source.relative_path],
  group_id: null,
  component_side: 'shared',
  language: '',
};
export const Registration: KnowledgeRegistration = {
  ...RegistrationRequest,
  domain_id: DomainId,
  registration_id: '7'.repeat(32),
  created_at: '2026-10-25T00:00:00Z',
};
export const PreviewRequest: KnowledgePreviewRequest = {
  relative_path: Source.relative_path,
  text: SourceText,
  group_id: null,
  component_side: 'shared',
  language: '',
};
export const ImportCapabilities: KnowledgeImportCapabilities = {
  version: '0.2.0',
  status: 'Frozen',
  foundation_contract_version: '1.0.0',
  schema: 'docs/knowledge-sources-v1.openapi.json',
  source_kinds: ['text_file'],
  input_modes: ['inline_text'],
  max_preview_items: 1,
  max_include_paths: 16,
  source_bytes: 2097152,
  request_bytes: 16777216,
  preview_ttl_seconds: 600,
  task_execution: 'synchronous_single_item',
  processing: {
    mode: 'catalogue_only',
    structure: 'deferred',
    full_text: 'disabled',
    vector: 'disabled',
    external_models: false,
    automatic_ingestion: false,
    allowed_model_uses: [],
    model_calls: 0,
  },
};
export function makePreview(
  request = PreviewRequest,
  id = '8'.repeat(32),
): KnowledgeImportPreview {
  return {
    preview_id: id,
    registration_id: Registration.registration_id,
    domain_id: DomainId,
    policy_version: 1,
    manifest_sha256: 'a'.repeat(64),
    expires_at: '2030-01-01T00:10:00Z',
    consumed_task_id: null,
    items: [
      {
        item_key: request.relative_path,
        relative_path: request.relative_path,
        group_id: request.group_id,
        component_side: request.component_side,
        language: request.language,
        action: 'add',
        source_id: null,
        expected_revision_id: null,
        sha256: createHash('sha256').update(request.text).digest('hex'),
        byte_length: Buffer.byteLength(request.text),
      },
    ],
  };
}
export function makeReceipt(
  preview = makePreview(),
  failed = false,
): KnowledgeImportReceipt {
  const common = {
    import_task_id: '9'.repeat(32),
    registration_id: preview.registration_id,
    domain_id: preview.domain_id,
    preview_id: preview.preview_id,
    manifest_sha256: preview.manifest_sha256,
    model_calls: 0 as const,
    created_at: '2026-10-25T00:01:00Z',
  };
  const candidate = preview.items[0];
  const item = {
    item_key: candidate.item_key,
    relative_path: candidate.relative_path,
    group_id: candidate.group_id,
    component_side: candidate.component_side,
    language: candidate.language,
    sha256: candidate.sha256,
    byte_length: candidate.byte_length,
    structure_status: 'deferred' as const,
    full_text_status: 'disabled' as const,
    vector_status: 'disabled' as const,
  };
  return failed
    ? {
        ...common,
        status: 'failed',
        items: [
          {
            ...item,
            save_status: 'failed',
            catalogue_status: 'failed',
            source_id: null,
            source_revision_id: null,
            root_node_id: null,
            code: 'knowledge_import_publish_failed',
          },
        ],
      }
    : {
        ...common,
        status: 'succeeded',
        items: [
          {
            ...item,
            save_status:
              candidate.action === 'unchanged' ? 'unchanged' : 'saved',
            catalogue_status: 'ready',
            source_id: Source.source_id,
            source_revision_id:
              candidate.expected_revision_id || Source.source_revision_id,
            root_node_id: Source.root_node_id,
            code: 'ok',
          },
        ],
      };
}
export const c2Envelope = (data: unknown) => ({
  code: 0,
  message: '',
  data,
  contract_version: '1.0.0',
  source_contract_version: '0.2.0',
  request_id: 'e'.repeat(32),
});

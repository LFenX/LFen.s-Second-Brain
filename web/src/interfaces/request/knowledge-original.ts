import type { OriginalClassification } from '@/interfaces/database/knowledge-original';

export type OriginalRegistrationRequest = {
  registration_key: string;
  name: string;
  source_kind: 'original_document';
  input_mode: 'original_upload';
  include_paths: string[];
  default_classification: OriginalClassification;
};
export type CreateOriginalUpload = {
  original_filename: string;
  relative_path: string;
  classification: OriginalClassification;
  declared_byte_length: number;
  declared_media_type: string;
  expected_sha256: string | null;
  expected_revision_id: string | null;
  idempotency_key: string;
};
export type ConfirmOriginalImport = {
  upload_id: string;
  manifest_sha256: string;
  idempotency_key: string;
  confirmed: true;
};

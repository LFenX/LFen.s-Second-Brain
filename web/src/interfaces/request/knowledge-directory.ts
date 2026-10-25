import type { DirectoryClassification } from '../database/knowledge-directory';

export type DirectoryRegistrationRequest = {
  registration_key: string;
  name: string;
  source_kind: 'controlled_directory';
  input_mode: 'server_read';
  root_id: string;
  scope_directories: string[];
  default_classification: DirectoryClassification;
};

export type DirectoryListingRequest = {
  relative_directory: string;
  name_prefix: string;
};

export type DirectoryObservationRequest = {
  selected_paths: string[];
};

export type DirectoryPreviewRequest = {
  observation_id: string;
  observation_sha256: string;
  relative_path: string;
  classification: DirectoryClassification;
};

export type DirectoryCommitRequest = {
  preview_id: string;
  manifest_sha256: string;
  idempotency_key: string;
};

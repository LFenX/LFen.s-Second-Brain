import type { BatchClassification } from '../database/knowledge-batch';

export type BatchSelectionItem = {
  relative_path: string;
  classification: BatchClassification;
};

export type BatchPreviewRequest = {
  observation_id: string;
  observation_sha256: string;
  selected_items: Array<BatchSelectionItem>;
};

export type BatchAcceptRequest = {
  preview_id: string;
  manifest_sha256: string;
  idempotency_key: string;
  confirmed_item_ids: Array<string>;
  retry_of_task_id: string | null;
};

export type BatchCancellationRequest = {
  idempotency_key: string;
};

export type BatchManifestPreviewRequest = {
  project_id: string;
  task_id: string;
  expected_manifest_id: string | null;
};

export type BatchManifestPublicationRequest = {
  project_id: string;
  preview_id: string;
  manifest_sha256: string;
  idempotency_key: string;
};

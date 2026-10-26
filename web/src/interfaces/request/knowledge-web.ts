import type {
  WebClassification,
  WebNetworkScope,
} from '@/interfaces/database/knowledge-web';

export type WebCreateRegistration = {
  registration_key: string;
  name: string;
  source_kind: 'web_page' | 'web_feed';
  origin_url: string;
  producer: 'direct' | 'rsshub' | 'other_feed' | 'browser_export';
  default_classification: WebClassification;
  network_scope: WebNetworkScope;
};

export type WebCreateLocalCapture = {
  registration_config_sha256: string;
  idempotency_key: string;
  acquisition_mode: 'local_snapshot';
  original_filename: string;
  declared_byte_length: number;
  declared_media_type:
    | 'text/html'
    | 'application/rss+xml'
    | 'application/atom+xml'
    | 'application/xml'
    | 'text/xml';
  expected_sha256: string | null;
  declared_final_url: string | null;
  claimed_captured_at: string | null;
};

export type WebCreateFetchCapture = {
  registration_config_sha256: string;
  idempotency_key: string;
  acquisition_mode: 'explicit_fetch';
};

export type WebCreateCapture = WebCreateLocalCapture | WebCreateFetchCapture;

export type WebAcquire = {
  scope_sha256: string;
  expected_capture_revision: number;
  confirmed: true;
};

export type WebObserve = {
  normalizer_version: 'web-normalizer/0.6.0';
};

export type WebSelection = {
  entry_key: string;
  classification: WebClassification;
  expected_revision_id: string | null;
};

export type WebCreatePreview = {
  capture_id: string;
  observation_id: string;
  observation_sha256: string;
  idempotency_key: string;
  selection: WebSelection[];
};

export type WebConfirm = {
  preview_id: string;
  manifest_sha256: string;
  idempotency_key: string;
  confirmed: true;
  acknowledged_entry_keys: string[];
};

import { createHash } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-directory-import-v1.examples.json';
import type {
  DirectoryRegistration,
  DirectoryCapabilities,
  DirectoryRoot,
  DirectoryListing,
  DirectoryObservation,
  DirectoryPreview,
  DirectoryReceipt,
  DirectoryContentWindow,
} from '@/interfaces/database/knowledge-directory';
import {
  Domain as FoundationDomain,
  Source as FoundationSource,
  Root as FoundationRoot,
  Capabilities as FoundationCapabilities,
  envelope as foundationEnvelope,
  page,
} from '../../knowledge-catalogue/__tests__/fixture';
import type {
  DirectoryRegistrationRequest as RegistrationRequestType,
  DirectoryPreviewRequest as PreviewRequestType,
} from '@/interfaces/request/knowledge-directory';

export { FoundationCapabilities, foundationEnvelope, page };
export function copy<T>(name: string): T {
  const item = Examples.schema_cases.find((value) => value.id === name);
  if (!item) throw new Error('Unknown frozen example ' + name);
  return JSON.parse(JSON.stringify(item.data)) as T;
}
export const Registration = copy<DirectoryRegistration>('valid-Registration');
export const RegistrationRequest = copy<RegistrationRequestType>(
  'valid-RegistrationRequest',
);
export const Root = copy<DirectoryRoot>('valid-Root');
export const Capabilities = copy<DirectoryCapabilities>('valid-Capabilities');
export const DomainId = Registration.domain_id;
export const Domain = { ...FoundationDomain, domain_id: DomainId };
export const Domain2 = {
  ...Domain,
  domain_id: 'd'.repeat(32),
  name: 'Second synthetic domain',
};
export const Text = Examples.digest_vectors.content.utf8_text;
export const Hash = Examples.digest_vectors.content.sha256;
export const ByteLength = Examples.digest_vectors.content.byte_length;
export const Id = (n: number) => n.toString(16).padStart(32, '0');
export const stamp = () => {
  const captured = Date.now();
  return {
    captured_at: new Date(captured).toISOString(),
    expires_at: new Date(captured + 600000).toISOString(),
  };
};
export function digest(tokens: unknown[]) {
  return createHash('sha256')
    .update(
      tokens
        .map((token) => {
          const text = token === null ? '' : String(token);
          return Buffer.byteLength(text, 'utf8') + ':' + text;
        })
        .join(''),
      'utf8',
    )
    .digest('hex');
}
export function sealObservation(value: DirectoryObservation) {
  value.observation_sha256 = digest([
    'C2-DIR-OBS-1',
    value.observation_id,
    value.domain_id,
    value.registration_id,
    value.root_id,
    value.root_version,
    value.policy_version,
    value.default_classification.group_id,
    value.default_classification.component_side,
    value.default_classification.language,
    value.captured_at,
    value.expires_at,
    value.complete,
    value.selected_paths.length,
    ...value.selected_paths,
    value.items.length,
    ...value.items.flatMap((item) => [
      item.relative_path,
      item.state,
      item.eligible,
      item.reason,
      item.byte_length,
      item.sha256,
      item.source_id,
      item.expected_revision_id,
      item.change_kind,
    ]),
  ]);
  return value;
}
export function makeObservation(
  paths = ['web/card.tsx', 'web/readme.pdf'],
  n = 5,
  reg = Registration,
): DirectoryObservation {
  const value = copy<DirectoryObservation>('valid-Observation');
  Object.assign(value, stamp(), {
    observation_id: Id(n),
    domain_id: reg.domain_id,
    registration_id: reg.registration_id,
    root_id: reg.root_id,
    default_classification: reg.default_classification,
    selected_paths: [...paths].sort(),
  });
  value.items = value.selected_paths.map((path) => {
    const item = copy<DirectoryObservation['items'][number]>(
      'valid-ObservationItem',
    );
    item.relative_path = path;
    if (path.endsWith('.pdf'))
      Object.assign(item, {
        state: 'ineligible',
        eligible: false,
        reason: 'unsupported_format',
        byte_length: 1024,
        sha256: null,
        source_id: null,
        expected_revision_id: null,
        change_kind: 'blocked',
      });
    if (path.includes('missing'))
      Object.assign(item, {
        state: 'missing',
        eligible: false,
        reason: 'missing',
        byte_length: null,
        sha256: null,
        source_id: null,
        expected_revision_id: null,
        change_kind: 'unobserved',
      });
    return item;
  });
  value.complete = value.items.every((item) =>
    ['verified', 'ineligible'].includes(item.state),
  );
  return sealObservation(value);
}
export function makePreview(
  observation = makeObservation(),
  request?: PreviewRequestType,
  n = 6,
): DirectoryPreview {
  const value = copy<DirectoryPreview>('valid-Preview');
  Object.assign(value, stamp(), {
    preview_id: Id(n),
    domain_id: observation.domain_id,
    registration_id: observation.registration_id,
    root_id: observation.root_id,
    root_version: observation.root_version,
    policy_version: observation.policy_version,
    observation_id: observation.observation_id,
    observation_sha256: observation.observation_sha256,
    expires_at: observation.expires_at,
  });
  const observed = observation.items.find(
    (item) =>
      item.relative_path ===
      (request?.relative_path || observation.selected_paths[0]),
  )!;
  Object.assign(value.items[0], {
    relative_path: observed.relative_path,
    classification:
      request?.classification || observation.default_classification,
    source_id: observed.source_id,
    expected_revision_id: observed.expected_revision_id,
    sha256: observed.sha256,
    byte_length: observed.byte_length,
    action: observed.change_kind,
  });
  value.manifest_sha256 = digest([
    'C2-DIR-PREVIEW-1',
    value.preview_id,
    value.domain_id,
    value.registration_id,
    value.root_id,
    value.root_version,
    value.policy_version,
    value.observation_id,
    value.observation_sha256,
    value.captured_at,
    value.expires_at,
    value.items[0].relative_path,
    value.items[0].classification.group_id,
    value.items[0].classification.component_side,
    value.items[0].classification.language,
    value.items[0].action,
    value.items[0].source_id,
    value.items[0].expected_revision_id,
    value.items[0].sha256,
    value.items[0].byte_length,
  ]);
  return value;
}
export function makeReceipt(
  preview = makePreview(),
  failed = false,
): DirectoryReceipt {
  const value = copy<DirectoryReceipt>(
    failed ? 'valid-failed-receipt' : 'valid-Receipt',
  );
  Object.assign(value, {
    domain_id: preview.domain_id,
    registration_id: preview.registration_id,
    preview_id: preview.preview_id,
    observation_id: preview.observation_id,
    manifest_sha256: preview.manifest_sha256,
    created_at: new Date().toISOString(),
  });
  Object.assign(value.items[0], {
    relative_path: preview.items[0].relative_path,
    classification: preview.items[0].classification,
    sha256: preview.items[0].sha256,
    byte_length: preview.items[0].byte_length,
  });
  if (value.status === 'succeeded')
    value.items[0].save_status =
      preview.items[0].action === 'add'
        ? 'created'
        : preview.items[0].action === 'update'
          ? 'updated'
          : 'unchanged';
  return value;
}
export function makeListing(count = 2): DirectoryListing {
  const value = copy<DirectoryListing>('valid-Listing');
  Object.assign(value, stamp());
  if (count > 2)
    value.items = Array.from({ length: count }, (_, i) => ({
      relative_path:
        i === 0
          ? 'web/card.tsx'
          : 'web/file-' + String(i).padStart(3, '0') + '.ts',
      entry_kind: 'file' as const,
      byte_length: 57,
      selectable: true,
      reason: null,
    }));
  value.total_entries = value.items.length;
  return value;
}
export function windowFor(
  preview = makePreview(),
  offset = 0,
  length = 65536,
  text = Text,
): DirectoryContentWindow {
  const bytes = Buffer.from(text, 'utf8'),
    end = Math.min(bytes.length, offset + length);
  const decoded = new TextDecoder('utf-8', {
    fatal: true,
    ignoreBOM: true,
  }).decode(bytes.subarray(offset, end));
  return {
    preview_id: preview.preview_id,
    offset,
    byte_length: end - offset,
    total_bytes: bytes.length,
    sha256: preview.items[0].sha256,
    text: decoded,
  };
}
export const directoryEnvelope = (data: unknown) => ({
  code: 0,
  message: 'Synthetic fixture only',
  data,
  contract_version: '1.0.0',
  source_contract_version: '0.3.0',
  request_id: Id(12),
});
export const respond = (data: unknown, status = 200) => ({
  status,
  data: directoryEnvelope(data),
});
export function reject(
  status: number,
  code: string,
  reason: string | null = null,
) {
  return {
    response: {
      status,
      data: {
        ...directoryEnvelope(null),
        code,
        reason,
        message: 'Synthetic failure',
      },
    },
  };
}
export function c1Projection(receipt = makeReceipt()) {
  const item = receipt.items[0];
  const source = {
    ...FoundationSource,
    domain_id: receipt.domain_id,
    source_id: item.source_id!,
    source_revision_id: item.source_revision_id!,
    root_node_id: item.root_node_id!,
    relative_path: item.relative_path,
    group_id: item.classification.group_id,
    component_side: item.classification.component_side,
    language: item.classification.language,
    sha256: item.sha256,
    byte_length: item.byte_length,
    line_count: 3,
  };
  const root = {
    ...FoundationRoot,
    domain_id: receipt.domain_id,
    node_id: item.root_node_id!,
    source_revision_id: item.source_revision_id!,
    end_byte: item.byte_length,
    end_line: 3,
  };
  return { source, root };
}

import type {
  DirectoryListing,
  DirectoryObservation,
  DirectoryPreview,
  DirectoryReceipt,
  DirectoryContentWindow,
} from '@/interfaces/database/knowledge-directory';
import dayjs from 'dayjs';
import type {
  DirectoryCommitRequest,
  DirectoryPreviewRequest,
} from '@/interfaces/request/knowledge-directory';
import { knowledgeImportStorageKey } from './knowledge-import';
import {
  compareDirectoryPaths,
  validDirectoryData,
} from './knowledge-directory-schema';

export interface DirectoryPending {
  registrationId: string;
  request: DirectoryCommitRequest;
}
export function sameDirectoryClassification(
  left: { group_id: string | null; component_side: string; language: string },
  right: { group_id: string | null; component_side: string; language: string },
) {
  return (
    left.group_id === right.group_id &&
    left.component_side === right.component_side &&
    left.language === right.language
  );
}
export function directoryStorageKey(
  domainId: string,
  authorization: string,
  userId?: string,
) {
  return knowledgeImportStorageKey(domainId, authorization, userId).then(
    (key) => 'directory:0.3.0:' + key,
  );
}
export function loadDirectoryPending(key: string): DirectoryPending | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(key) || 'null');
    if (!value) return null;
    if (
      Object.keys(value).sort().join(',') !== 'registrationId,request' ||
      !/^[a-f0-9]{32}$/.test(value.registrationId) ||
      !validDirectoryData('CommitRequest', value.request)
    )
      throw new Error('Invalid recovery metadata');
    return value;
  } catch {
    try {
      sessionStorage.removeItem(key);
    } catch {
      /* unavailable storage never permits a first POST */
    }
    return null;
  }
}
export function saveDirectoryPending(key: string, value: DirectoryPending) {
  if (
    !validDirectoryData('CommitRequest', value.request) ||
    !/^[a-f0-9]{32}$/.test(value.registrationId)
  )
    throw new Error('Invalid recovery metadata');
  sessionStorage.setItem(
    key,
    JSON.stringify({
      registrationId: value.registrationId,
      request: value.request,
    }),
  );
}
export function directoryObservationMatches(
  value: DirectoryObservation,
  selected: string[],
) {
  return (
    validDirectoryData('Observation', value) &&
    JSON.stringify(value.selected_paths) ===
      JSON.stringify([...selected].sort(compareDirectoryPaths))
  );
}
export function directoryPreviewMatches(
  value: DirectoryPreview,
  request: DirectoryPreviewRequest,
  observation: DirectoryObservation,
) {
  const item = value.items[0];
  const observed = observation.items.find(
    (entry) => entry.relative_path === request.relative_path,
  );
  return (
    !!observed &&
    observation.complete &&
    observed.eligible &&
    value.observation_id === observation.observation_id &&
    value.observation_sha256 === observation.observation_sha256 &&
    value.root_id === observation.root_id &&
    value.root_version === observation.root_version &&
    value.policy_version === observation.policy_version &&
    value.items[0].relative_path === request.relative_path &&
    sameDirectoryClassification(item.classification, request.classification) &&
    item.sha256 === observed.sha256 &&
    item.byte_length === observed.byte_length &&
    item.source_id === observed.source_id &&
    item.expected_revision_id === observed.expected_revision_id &&
    !dayjs(value.expires_at).isAfter(dayjs(observation.expires_at))
  );
}
export function directoryReceiptMatches(
  value: DirectoryReceipt,
  pending: DirectoryPending,
  preview?: DirectoryPreview | null,
) {
  if (
    value.registration_id !== pending.registrationId ||
    value.preview_id !== pending.request.preview_id ||
    value.manifest_sha256 !== pending.request.manifest_sha256
  )
    return false;
  if (!preview) return true;
  const before = preview.items[0],
    after = value.items[0];
  return (
    before.relative_path === after.relative_path &&
    before.sha256 === after.sha256 &&
    before.byte_length === after.byte_length &&
    sameDirectoryClassification(before.classification, after.classification) &&
    (value.status !== 'succeeded' ||
      before.source_id === null ||
      before.source_id === after.source_id) &&
    (value.status !== 'succeeded' ||
      before.action !== 'unchanged' ||
      before.expected_revision_id === after.source_revision_id)
  );
}
export function directoryWindowMatches(
  value: DirectoryContentWindow,
  preview: DirectoryPreview,
  offset: number,
) {
  return (
    value.preview_id === preview.preview_id &&
    value.offset === offset &&
    value.sha256 === preview.items[0].sha256 &&
    value.total_bytes === preview.items[0].byte_length &&
    (offset >= value.total_bytes || value.byte_length > 0)
  );
}
export function appendDirectoryPage(
  pages: DirectoryListing[],
  next: DirectoryListing,
) {
  const first = pages[0];
  if (first) {
    for (const key of [
      'listing_id',
      'registration_id',
      'domain_id',
      'root_id',
      'root_version',
      'relative_directory',
      'name_prefix',
      'captured_at',
      'expires_at',
      'complete',
      'reason',
      'total_entries',
    ] as const)
      if (first[key] !== next[key]) throw new Error('Listing changed');
  }
  const all = [...pages, next],
    paths = all.flatMap((page) => page.items.map((item) => item.relative_path));
  if (
    paths.some(
      (p, i) => i > 0 && compareDirectoryPaths(paths[i - 1], p) >= 0,
    ) ||
    paths.length > next.total_entries ||
    (!next.next_cursor && paths.length !== next.total_entries)
  )
    throw new Error('Invalid fixed listing page');
  return all;
}

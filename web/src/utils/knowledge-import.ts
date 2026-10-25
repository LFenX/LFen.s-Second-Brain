import type {
  KnowledgeImportPreview,
  KnowledgeImportReceipt,
} from '@/interfaces/database/knowledge-sources';
import type {
  KnowledgeCommitRequest,
  KnowledgePreviewRequest,
} from '@/interfaces/request/knowledge-sources';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import { validKnowledgeSourceData } from './knowledge-sources-schema';

export async function knowledgeTextDigest(text: string) {
  const bytes = new TextEncoder().encode(text);
  if (bytes.length > 2097152)
    throw new KnowledgeRequestError(413, 'knowledge_source_too_large');
  if (
    text.includes('\0') ||
    new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes) !==
      text
  )
    throw new KnowledgeRequestError(415, 'knowledge_source_unsupported');
  const hash = await crypto.subtle.digest('SHA-256', bytes);
  return {
    byteLength: bytes.length,
    sha256: Array.from(new Uint8Array(hash), (byte) =>
      byte.toString(16).padStart(2, '0'),
    ).join(''),
  };
}

export async function readKnowledgeTextFile(
  file: Pick<File, 'size' | 'arrayBuffer'>,
) {
  if (file.size > 2097152)
    throw new KnowledgeRequestError(413, 'knowledge_source_too_large');
  const bytes = await file.arrayBuffer();
  let text: string;
  try {
    text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      bytes,
    );
  } catch {
    throw new KnowledgeRequestError(415, 'knowledge_source_unsupported');
  }
  return { text, ...(await knowledgeTextDigest(text)) };
}

export function checkKnowledgePreview(
  preview: KnowledgeImportPreview,
  request: KnowledgePreviewRequest,
  digest: { sha256: string; byteLength: number },
) {
  const item = preview.items[0];
  if (
    item.item_key !== request.relative_path ||
    item.relative_path !== request.relative_path ||
    item.group_id !== request.group_id ||
    item.component_side !== request.component_side ||
    item.language !== request.language ||
    item.sha256 !== digest.sha256 ||
    item.byte_length !== digest.byteLength
  )
    throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
}

export function checkKnowledgeReceipt(
  preview: KnowledgeImportPreview,
  receipt: KnowledgeImportReceipt,
) {
  const expected = preview.items[0];
  const actual = receipt.items[0];
  if (
    receipt.domain_id !== preview.domain_id ||
    receipt.registration_id !== preview.registration_id ||
    receipt.preview_id !== preview.preview_id ||
    receipt.manifest_sha256 !== preview.manifest_sha256 ||
    actual.item_key !== expected.item_key ||
    actual.relative_path !== expected.relative_path ||
    actual.group_id !== expected.group_id ||
    actual.component_side !== expected.component_side ||
    actual.language !== expected.language ||
    actual.sha256 !== expected.sha256 ||
    actual.byte_length !== expected.byte_length ||
    (receipt.status === 'succeeded' &&
      expected.source_id !== null &&
      actual.source_id !== expected.source_id) ||
    (receipt.status === 'succeeded' &&
      expected.action === 'unchanged' &&
      actual.source_revision_id !== expected.expected_revision_id)
  )
    throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
}

export interface KnowledgePendingConfirmation {
  registrationId: string;
  request: KnowledgeCommitRequest;
}

// Only recovery identifiers are stored. Prefer the native user so re-login can
// recover the original request. Auth-only sessions stay scoped to that session.
// This local cache scope is never an API/source identity. No original or token.
export async function knowledgeImportStorageKey(
  domainId: string,
  authorization: string,
  nativeUserId?: string,
) {
  const hash = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(
      nativeUserId ? `user:${nativeUserId}` : `session:${authorization}`,
    ),
  );
  const scope = Array.from(new Uint8Array(hash), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('');
  return `lfen-c2:${scope}:${domainId}`;
}

export function loadKnowledgeConfirmation(
  key: string,
): KnowledgePendingConfirmation | null {
  try {
    const raw = sessionStorage.getItem(key);
    if (!raw) return null;
    const value = JSON.parse(raw);
    if (
      !/^[a-f0-9]{32}$/.test(value.registrationId) ||
      !validKnowledgeSourceData('CommitRequest', value.request)
    )
      throw new Error('invalid recovery');
    return { registrationId: value.registrationId, request: value.request };
  } catch {
    try {
      sessionStorage.removeItem(key);
    } catch {
      /* Storage can be unavailable; the first POST still requires a successful save. */
    }
    return null;
  }
}

export function saveKnowledgeConfirmation(
  key: string,
  value: KnowledgePendingConfirmation,
) {
  sessionStorage.setItem(key, JSON.stringify(value));
}

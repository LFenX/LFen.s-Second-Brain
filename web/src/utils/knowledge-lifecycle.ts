import type {
  LifecycleDetail,
  LifecycleReceipt,
} from '@/interfaces/database/knowledge-lifecycle';
import type {
  LifecycleAction,
  LifecycleCommand,
  LifecycleStateCommand,
} from '@/interfaces/request/knowledge-lifecycle';
import { encodeWebValue, webSha256 } from './knowledge-web';
import {
  LifecycleId,
  LifecycleSha,
  LifecycleActions,
  lifecycleJsonBytes,
  validLifecycleCommand,
} from './knowledge-lifecycle-schema';

export type LifecycleScope = { owner: string; domain: string; source: string };
export type LifecycleRecovery = {
  slot_version: 1;
  session_scope: number;
  owner_id: string;
  domain_id: string;
  source_id: string;
  action: LifecycleAction;
  request: LifecycleCommand;
  request_sha256: string;
  command_id: string | null;
};
export function lifecycleProjection(
  scope: LifecycleScope,
  action: LifecycleAction,
  request: LifecycleCommand,
) {
  return {
    kind: 'knowledge-lifecycle-command/0.7.0',
    owner_id: scope.owner,
    domain_id: scope.domain,
    source_id: scope.source,
    action,
    expected_source_revision_id: request.expected_source_revision_id,
    expected_lifecycle_version: request.expected_lifecycle_version,
    expected_policy_version: request.expected_policy_version,
    confirmed: request.confirmed,
    target:
      action === 'reclassify' && 'classification' in request
        ? request.classification
        : action === 'move' && 'relative_path' in request
          ? { relative_path: request.relative_path }
          : null,
  };
}
export function lifecycleDigest(value: unknown) {
  return webSha256(new TextEncoder().encode(encodeWebValue(value)));
}
export function lifecycleRequestDigest(
  scope: LifecycleScope,
  action: LifecycleAction,
  request: LifecycleCommand,
) {
  return lifecycleDigest(lifecycleProjection(scope, action, request));
}
export function equalLifecycle(a: unknown, b: unknown) {
  return encodeWebValue(a) === encodeWebValue(b);
}
export function lifecycleExpected(
  detail: LifecycleDetail,
): Omit<LifecycleStateCommand, 'confirmed' | 'idempotency_key'> {
  return {
    expected_source_revision_id: detail.current.source_revision_id,
    expected_lifecycle_version: detail.current.lifecycle_version,
    expected_policy_version: detail.policy_version,
  };
}
export function lifecycleNewKey() {
  return crypto.randomUUID();
}
export function checkLifecycleReceipt(
  receipt: LifecycleReceipt,
  scope: LifecycleScope,
  action?: LifecycleAction,
  request?: LifecycleCommand,
) {
  if (
    receipt.owner_id !== scope.owner ||
    receipt.domain_id !== scope.domain ||
    receipt.source_id !== scope.source ||
    (action !== undefined && receipt.action !== action) ||
    (request && !equalLifecycle(receipt.request, request))
  )
    throw Error('Lifecycle receipt scope mismatch');
  const before = receipt.before,
    after = receipt.after,
    original = receipt.request;
  const changed =
    receipt.action === 'reclassify' && 'classification' in original
      ? !equalLifecycle(before.classification, original.classification)
      : receipt.action === 'move' && 'relative_path' in original
        ? before.relative_path !== original.relative_path
        : before.state !==
          (receipt.action === 'archive' ? 'archived' : 'active');
  if (receipt.result !== (changed ? 'applied' : 'unchanged'))
    throw Error('Lifecycle no-op result mismatch');
  if (
    before.source_revision_id !== original.expected_source_revision_id ||
    before.lifecycle_version !== original.expected_lifecycle_version ||
    receipt.policy_version !== original.expected_policy_version
  )
    throw Error('Lifecycle receipt expectation mismatch');
  if (
    receipt.result === 'unchanged'
      ? !equalLifecycle(before, after)
      : after.lifecycle_version !== before.lifecycle_version + 1 ||
        after.lifecycle_version > Number.MAX_SAFE_INTEGER
  )
    throw Error('Lifecycle receipt generation mismatch');
  if (
    before.sha256 !== after.sha256 ||
    before.byte_length !== after.byte_length
  )
    throw Error('Lifecycle command changed content');
  if (receipt.action === 'reclassify' && 'classification' in original) {
    if (
      before.state !== 'active' ||
      !equalLifecycle(after.classification, original.classification) ||
      before.state !== after.state ||
      before.relative_path !== after.relative_path
    )
      throw Error('Invalid classification result');
  }
  if (receipt.action === 'move' && 'relative_path' in original) {
    if (
      before.state !== 'active' ||
      after.relative_path !== original.relative_path ||
      before.state !== after.state ||
      !equalLifecycle(before.classification, after.classification)
    )
      throw Error('Invalid move result');
  }
  if (receipt.action === 'archive' || receipt.action === 'restore') {
    if (
      after.state !== (receipt.action === 'archive' ? 'archived' : 'active') ||
      before.source_revision_id !== after.source_revision_id ||
      before.source_input_sha256 !== after.source_input_sha256 ||
      before.relative_path !== after.relative_path ||
      !equalLifecycle(before.classification, after.classification)
    )
      throw Error('Invalid state result');
  }
}
export function lifecycleStorageKey(owner: string, domain: string) {
  if (!LifecycleId.test(owner) || !LifecycleId.test(domain))
    throw Error('Invalid lifecycle scope');
  return 'knowledge-lifecycle:1:' + owner + ':' + domain;
}
function validRecovery(value: unknown): value is LifecycleRecovery {
  if (!value || typeof value !== 'object') return false;
  const record = value as LifecycleRecovery;
  return (
    Object.keys(record).sort().join(',') ===
      'action,command_id,domain_id,owner_id,request,request_sha256,session_scope,slot_version,source_id' &&
    record.slot_version === 1 &&
    Number.isSafeInteger(record.session_scope) &&
    record.session_scope >= 0 &&
    LifecycleId.test(record.owner_id) &&
    LifecycleId.test(record.domain_id) &&
    LifecycleId.test(record.source_id) &&
    LifecycleActions.includes(record.action) &&
    validLifecycleCommand(record.action, record.request) &&
    LifecycleSha.test(record.request_sha256) &&
    (record.command_id === null || LifecycleId.test(record.command_id))
  );
}
export function readLifecycleRecovery(
  owner: string,
  domain: string,
): LifecycleRecovery | null {
  const raw = localStorage.getItem(lifecycleStorageKey(owner, domain));
  if (raw === null) return null;
  if (new TextEncoder().encode(raw).length > 8192)
    throw Error('Recovery exceeds 8 KiB');
  const value = JSON.parse(raw);
  if (
    !validRecovery(value) ||
    value.owner_id !== owner ||
    value.domain_id !== domain
  )
    throw Error('Invalid lifecycle recovery');
  return value;
}
function persistLifecycleRecovery(value: LifecycleRecovery) {
  if (!validRecovery(value) || lifecycleJsonBytes(value) > 8192)
    throw Error('Invalid lifecycle recovery');
  const key = lifecycleStorageKey(value.owner_id, value.domain_id),
    old = readLifecycleRecovery(value.owner_id, value.domain_id);
  if (
    old &&
    old.command_id === null &&
    (!equalLifecycle(old.request, value.request) ||
      old.source_id !== value.source_id ||
      old.action !== value.action ||
      old.request_sha256 !== value.request_sha256)
  )
    throw Error('Unresolved request already exists');
  let unresolved = 0;
  for (let i = 0; i < localStorage.length; i++) {
    const item = localStorage.key(i)!;
    if (
      !item.startsWith('knowledge-lifecycle:1:' + value.owner_id + ':') ||
      item === key
    )
      continue;
    const domain = item.slice(item.lastIndexOf(':') + 1);
    const record = readLifecycleRecovery(value.owner_id, domain);
    if (record?.command_id === null) unresolved++;
  }
  if (value.command_id === null && unresolved >= 8)
    throw Error('Eight unresolved domains already exist');
  localStorage.setItem(key, JSON.stringify(value));
}
const StorageWriters = new Map<string, Promise<void>>();
async function serializeRecoveryWrite<T>(
  owner: string,
  work: () => Promise<T> | T,
): Promise<T> {
  const key = 'knowledge-lifecycle-storage:' + owner,
    previous = StorageWriters.get(key) || Promise.resolve();
  let finish!: () => void;
  const next = new Promise<void>((resolve) => {
    finish = resolve;
  });
  StorageWriters.set(key, next);
  await previous;
  try {
    return navigator.locks
      ? await navigator.locks.request(key, async () => work())
      : await work();
  } finally {
    finish();
    if (StorageWriters.get(key) === next) StorageWriters.delete(key);
  }
}
export function saveLifecycleRecovery(value: LifecycleRecovery) {
  return serializeRecoveryWrite(value.owner_id, () =>
    persistLifecycleRecovery(value),
  );
}
export function removeLifecycleRecovery(owner: string, domain: string) {
  localStorage.removeItem(lifecycleStorageKey(owner, domain));
}
export function lifecycleLink(domain: string, source: string) {
  return (
    '/dataset/knowledge-catalogue/' +
    domain +
    '?' +
    new URLSearchParams({ lifecycle_source: source })
  );
}

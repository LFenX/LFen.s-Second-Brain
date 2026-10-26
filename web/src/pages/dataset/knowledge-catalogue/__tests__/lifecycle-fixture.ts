import { webcrypto, randomUUID } from 'node:crypto';
import Examples from '../../../../../../docs/knowledge-source-lifecycle-v1.examples.json';
import {
  Domain as FoundationDomain,
  Capabilities as FoundationCapabilities,
} from './fixture';
import type {
  LifecycleCapabilities,
  LifecycleDetail,
  LifecycleReceipt,
} from '@/interfaces/database/knowledge-lifecycle';
import type {
  LifecycleAction,
  LifecycleCommand,
} from '@/interfaces/request/knowledge-lifecycle';
import {
  lifecycleExpected,
  lifecycleRequestDigest,
  type LifecycleRecovery,
} from '@/utils/knowledge-lifecycle';
Object.defineProperty(globalThis.crypto, 'subtle', {
  configurable: true,
  value: webcrypto.subtle,
});
Object.defineProperty(globalThis.crypto, 'randomUUID', {
  configurable: true,
  value: randomUUID,
});
export const Owner = 'a'.repeat(32),
  Domain = 'b'.repeat(32),
  Source = 'd'.repeat(32),
  Scope = { owner: Owner, domain: Domain, source: Source };
export function copy<T>(name: string): T {
  const item = Examples.structure_cases.find((item) => item.name === name);
  if (!item) throw Error('Missing lifecycle fixture ' + name);
  return JSON.parse(JSON.stringify(item.value)) as T;
}
export const Capabilities: LifecycleCapabilities = {
  ...copy<LifecycleCapabilities>('candidate_runtime_not_ready'),
  runtime_ready: true,
};
export function detail(patch: Partial<LifecycleDetail> = {}): LifecycleDetail {
  return { ...copy<LifecycleDetail>('manual_text_detail'), ...patch };
}
export function command(
  value = detail(),
  action: LifecycleAction = 'archive',
): LifecycleCommand {
  return {
    ...lifecycleExpected(value),
    idempotency_key: 'original_001',
    confirmed: true,
    ...(action === 'reclassify'
      ? { classification: { ...value.current.classification, language: 'go' } }
      : action === 'move'
        ? { relative_path: 'src/B.tsx' }
        : {}),
  };
}
export async function receipt(
  value = detail(),
  action: LifecycleAction = 'archive',
  request = command(value, action),
): Promise<LifecycleReceipt> {
  const after = JSON.parse(JSON.stringify(value.current));
  if (action === 'archive' || action === 'restore')
    after.state = action === 'archive' ? 'archived' : 'active';
  if (action === 'move' && 'relative_path' in request) {
    after.relative_path = request.relative_path;
    after.source_revision_id = '9'.repeat(32);
    after.source_input_sha256 = '9'.repeat(64);
  }
  if (action === 'reclassify' && 'classification' in request) {
    after.classification = request.classification;
    after.source_revision_id = '9'.repeat(32);
    after.source_input_sha256 = '9'.repeat(64);
  }
  const changed = JSON.stringify(after) !== JSON.stringify(value.current);
  after.lifecycle_version = value.current.lifecycle_version + (changed ? 1 : 0);
  return {
    command_id: '7'.repeat(32),
    owner_id: value.owner_id,
    domain_id: value.domain_id,
    source_id: value.source_id,
    action,
    idempotency_key: request.idempotency_key,
    request_sha256: await lifecycleRequestDigest(
      {
        owner: value.owner_id,
        domain: value.domain_id,
        source: value.source_id,
      },
      action,
      request,
    ),
    request,
    result: changed ? 'applied' : 'unchanged',
    before: JSON.parse(JSON.stringify(value.current)),
    after,
    policy_version: value.policy_version,
    created_at: '2026-10-26T03:00:00Z',
    model_calls: 0,
  };
}
export async function recovery(
  value = detail(),
  action: LifecycleAction = 'archive',
  request = command(value, action),
): Promise<LifecycleRecovery> {
  return {
    slot_version: 1,
    session_scope: 1,
    owner_id: value.owner_id,
    domain_id: value.domain_id,
    source_id: value.source_id,
    action,
    request,
    request_sha256: await lifecycleRequestDigest(
      {
        owner: value.owner_id,
        domain: value.domain_id,
        source: value.source_id,
      },
      action,
      request,
    ),
    command_id: null,
  };
}
export function response(data: unknown, status = 200) {
  return {
    status,
    data: {
      code: 0,
      message: '',
      data,
      contract_version: '1.0.0',
      source_contract_version: '0.7.0',
      request_id: '8'.repeat(32),
    },
    headers: { 'cache-control': 'no-store' },
  };
}
export function failure(
  status = 503,
  code = 'lifecycle_result_unknown',
  outcome = false,
) {
  return {
    response: {
      status,
      data: {
        code,
        message: 'synthetic failure',
        data: null,
        contract_version: '1.0.0',
        source_contract_version: '0.7.0',
        request_id: '8'.repeat(32),
        outcome_known: outcome,
      },
    },
  };
}
export function createLifecycleBackend() {
  const records = new Map<string, LifecycleReceipt>();
  const backend = {
    current: detail(),
    records,
    posts: [] as { action: LifecycleAction; request: LifecycleCommand }[],
    calls: [] as Record<string, any>[],
    losePost: false,
    missingResults: false,
    denied: false,
    conflict: false,
    targetOccupied: false,
    async http(config: Record<string, any>): Promise<any> {
      backend.calls.push(config);
      const url = config.url as string,
        params = config.params || {};
      if (url.endsWith('/knowledge/contract'))
        return response(FoundationCapabilities);
      if (url.endsWith('/domain'))
        return response({
          ...FoundationDomain,
          domain_id: Domain,
          owner_id: Owner,
        });
      if (url.endsWith('/groups'))
        return response({ items: [], next_cursor: '' });
      const current = backend.current;
      const native = {
        source_revision_id: current.current.source_revision_id,
        domain_id: Domain,
        source_id: Source,
        native_file_id: current.native_file_id,
        native_commit_id: current.current.source_revision_id,
        group_id: current.current.classification.group_id,
        relative_path: current.current.relative_path,
        component_side: current.current.classification.component_side,
        language: current.current.classification.language,
        sha256: current.current.sha256,
        byte_length: current.current.byte_length,
        line_count: 1,
        is_current: true,
        root_node_id: '6'.repeat(64),
      };
      if (url.endsWith('/sources'))
        return response({ items: [native], next_cursor: '' });
      if (url.endsWith('/sources/' + current.current.source_revision_id))
        return response(native);
      if (url.endsWith('/registrations/' + current.registration_id)) {
        const result = response({
          registration_key: 'synthetic',
          name: 'Synthetic registration',
          source_kind: 'text_file',
          input_mode: 'inline_text',
          include_paths: ['src/A.tsx', 'src/B.tsx'],
          group_id: null,
          component_side: 'frontend',
          language: 'typescript',
          registration_id: current.registration_id,
          domain_id: Domain,
          created_at: '2026-10-26T03:00:00Z',
        });
        result.data.source_contract_version = '0.2.0';
        return result;
      }
      if (backend.denied) throw failure(404, 'knowledge_not_found');
      if (url.endsWith('/source-lifecycle-contract'))
        return response(Capabilities);
      if (url.endsWith('/source-lifecycles')) {
        const match =
          (params.state || 'active') === current.current.state &&
          (params.group_scope !== 'ungrouped' ||
            current.current.classification.group_id === null) &&
          (params.group_scope !== 'exact' ||
            current.current.classification.group_id === params.group_id) &&
          (params.language === undefined ||
            params.language === current.current.classification.language) &&
          (params.component_side === undefined ||
            params.component_side ===
              current.current.classification.component_side);
        return response({ items: match ? [current] : [], next_cursor: null });
      }
      if (url.endsWith('/source-lifecycles/' + Source))
        return response(current);
      if (url.endsWith('/move-target'))
        return response({
          domain_id: Domain,
          source_id: Source,
          registration_id: current.registration_id,
          current_source_revision_id: params.expected_source_revision_id,
          lifecycle_version: params.expected_lifecycle_version,
          policy_version: params.expected_policy_version,
          relative_path: params.relative_path,
          allowed: !backend.targetOccupied,
          reason: backend.targetOccupied ? 'target_occupied' : null,
          reserved: false,
        });
      if (url.endsWith('/lifecycle-commands/by-key')) {
        const found = records.get(params.action + ':' + params.idempotency_key);
        if (!found || backend.missingResults)
          throw failure(404, 'knowledge_not_found');
        return response(found);
      }
      if (url.includes('/lifecycle-commands/')) {
        const found = [...records.values()].find((record) =>
          url.endsWith('/' + record.command_id),
        );
        if (!found || backend.missingResults)
          throw failure(404, 'knowledge_not_found');
        return response(found);
      }
      const suffix = url.slice(url.lastIndexOf('/') + 1),
        action = (
          {
            reclassifications: 'reclassify',
            moves: 'move',
            archives: 'archive',
            restores: 'restore',
          } as Record<string, LifecycleAction>
        )[suffix];
      if (action && config.method === 'post') {
        const request = JSON.parse(config.data) as LifecycleCommand;
        backend.posts.push({ action, request });
        const key = action + ':' + request.idempotency_key,
          prior = records.get(key);
        if (prior) {
          if (JSON.stringify(prior.request) !== JSON.stringify(request))
            throw failure(409, 'idempotency_conflict', true);
          return response(prior);
        }
        if (
          backend.conflict ||
          request.expected_source_revision_id !==
            current.current.source_revision_id ||
          request.expected_lifecycle_version !==
            current.current.lifecycle_version
        )
          throw failure(409, 'revision_changed', true);
        const saved = await receipt(current, action, request);
        saved.command_id = (records.size + 10).toString(16).padStart(32, '0');
        records.set(key, saved);
        backend.current = {
          ...current,
          current: saved.after,
          actions: {
            ...current.actions,
            reclassify: {
              ...current.actions.reclassify,
              allowed: saved.after.state === 'active',
              reason: saved.after.state === 'active' ? null : 'source_archived',
            },
            move: {
              ...current.actions.move,
              allowed:
                current.actions.move.supported &&
                saved.after.state === 'active',
              reason: !current.actions.move.supported
                ? 'action_unsupported'
                : saved.after.state === 'active'
                  ? null
                  : 'source_archived',
            },
          },
        };
        if (backend.losePost) {
          backend.losePost = false;
          throw Object.assign(Error('Synthetic timeout'), {
            code: 'ECONNABORTED',
          });
        }
        return response(saved, 201);
      }
      throw Error('Unhandled synthetic lifecycle path ' + url);
    },
  };
  return backend;
}
export type LifecycleBackend = ReturnType<typeof createLifecycleBackend>;

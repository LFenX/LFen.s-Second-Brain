import type { AxiosResponse } from 'axios';
import type {
  BatchCapabilities,
  BatchPreview,
  BatchContentWindow,
  BatchTask,
  BatchTaskPage,
  BatchTaskItemsPage,
  BatchManifestPreviewPage,
  BatchManifestReceipt,
  BatchManifestHead,
  BatchManifestHistoryPage,
  BatchManifestVersionPage,
} from '@/interfaces/database/knowledge-batch';
import type {
  BatchPreviewRequest,
  BatchAcceptRequest,
  BatchCancellationRequest,
  BatchManifestPreviewRequest,
  BatchManifestPublicationRequest,
} from '@/interfaces/request/knowledge-batch';
import { registerNextServer } from '@/utils/register-server';
import { validBatchData } from '@/utils/knowledge-batch-schema';
import {
  batchTaskMatches,
  batchReceiptMatches,
  type ManifestPending,
} from '@/utils/knowledge-batch';
import { KnowledgeRequestError } from './knowledge-foundation-service';

export class BatchRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId?: string,
    public readonly reason: string | null = null,
  ) {
    super(status, code, requestId);
  }
}
export type BatchResponse<T> = { data: T; httpStatus: number };
const Server = registerNextServer({
  read: { url: (c: { url: string }) => c.url, method: 'get' },
  write: { url: (c: { url: string }) => c.url, method: 'post' },
});
const Invalid = (status = 502) =>
  new BatchRequestError(
    status,
    status === 400
      ? 'knowledge_batch_invalid'
      : 'knowledge_batch_invalid_response',
  );
function id(value: string) {
  if (!/^[a-f0-9]{32}$/.test(value)) throw Invalid(400);
  return value;
}
const domainPath = (domain: string) => '/datasets/' + id(domain) + '/knowledge';
const regPath = (domain: string, reg: string) =>
  domainPath(domain) + '/directory-registrations/' + id(reg);
function cursor(value?: string) {
  if (value !== undefined && !/^[A-Za-z0-9_-]{1,1024}$/.test(value))
    throw Invalid(400);
  return value ? { cursor: value } : undefined;
}
async function call<T>(
  path: string,
  schema: string,
  scope: Record<string, string | null> = {},
  body?: { schema: string; data: unknown; statuses: number[] },
  params?: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<BatchResponse<T>> {
  if (body && !validBatchData(body.schema, body.data)) throw Invalid(400);
  const serialized = body ? JSON.stringify(body.data) : undefined;
  if (serialized && new TextEncoder().encode(serialized).length > 32768)
    throw new BatchRequestError(413, 'knowledge_batch_too_large');
  let response: AxiosResponse;
  try {
    response = await Server[body ? 'write' : 'read'](
      {
        url: '/api/v1' + path,
        params,
        signal,
        data: serialized,
        timeout: 45000,
        skipGlobalErrorNotification: true,
        headers: {
          'X-Knowledge-Contract-Version': '1.0.0',
          'X-Knowledge-Source-Contract-Version': '0.4.0',
          ...(body ? { 'Content-Type': 'application/json' } : {}),
        },
      },
      true,
    );
  } catch (error) {
    const failure = error as { response?: AxiosResponse };
    if (!failure.response)
      throw new BatchRequestError(
        0,
        'knowledge_batch_unavailable',
        undefined,
        'result_unknown',
      );
    const { status, data } = failure.response;
    if (status === 401)
      throw new BatchRequestError(401, 'knowledge_unauthorized');
    if (!validBatchData('Error', data, status)) throw Invalid();
    throw new BatchRequestError(
      status,
      data.code,
      data.request_id,
      data.reason,
    );
  }
  const envelope = response.data;
  if (
    envelope?.contract_version !== '1.0.0' ||
    envelope?.source_contract_version !== '0.4.0'
  )
    throw new BatchRequestError(409, 'knowledge_batch_version_mismatch');
  if (
    !(body?.statuses || [200]).includes(response.status) ||
    !validBatchData(schema, envelope) ||
    new TextEncoder().encode(JSON.stringify(envelope)).length >
      (schema === 'ContentWindowEnvelope' ? 1048576 : 262144)
  )
    throw Invalid();
  for (const [key, value] of Object.entries(scope))
    if (envelope.data[key] !== value) throw Invalid();
  return { data: envelope.data as T, httpStatus: response.status };
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<BatchCapabilities>(
      '/knowledge/batch-import-contract',
      'CapabilitiesEnvelope',
      {},
      undefined,
      undefined,
      signal,
    ),
  prepare: (domain: string, reg: string, request: BatchPreviewRequest) =>
    call<BatchPreview>(
      regPath(domain, reg) + '/batch-previews',
      'BatchPreviewEnvelope',
      {
        domain_id: domain,
        registration_id: reg,
        observation_id: request.observation_id,
        observation_sha256: request.observation_sha256,
      },
      { schema: 'PreviewRequest', data: request, statuses: [201] },
    ),
  preview: (domain: string, reg: string, preview: string) =>
    call<BatchPreview>(
      regPath(domain, reg) + '/batch-previews/' + id(preview),
      'BatchPreviewEnvelope',
      { domain_id: domain, registration_id: reg, preview_id: preview },
    ),
  content: async (
    domain: string,
    reg: string,
    preview: string,
    item: string,
    offset: number,
    length: number,
  ) => {
    if (
      !Number.isInteger(offset) ||
      offset < 0 ||
      offset > 2097152 ||
      !Number.isInteger(length) ||
      length < 1 ||
      length > 65536
    )
      throw Invalid(400);
    for (let shrink = 0; shrink < 4; shrink += 1) {
      try {
        return await call<BatchContentWindow>(
          regPath(domain, reg) +
            '/batch-previews/' +
            id(preview) +
            '/items/' +
            id(item) +
            '/content',
          'ContentWindowEnvelope',
          { preview_id: preview, item_id: item },
          undefined,
          { offset, length: length - shrink },
        );
      } catch (error) {
        if (
          !(error instanceof BatchRequestError) ||
          error.status !== 416 ||
          length - shrink <= 1 ||
          shrink === 3
        )
          throw error;
      }
    }
    throw Invalid();
  },
  accept: async (domain: string, reg: string, request: BatchAcceptRequest) => {
    const result = await call<BatchTask>(
      regPath(domain, reg) + '/batch-tasks',
      'TaskEnvelope',
      { domain_id: domain, registration_id: reg },
      { schema: 'AcceptRequest', data: request, statuses: [200, 202] },
    );
    if (!batchTaskMatches(result.data, request)) throw Invalid();
    return result;
  },
  tasks: (domain: string, reg: string, next?: string, signal?: AbortSignal) =>
    call<BatchTaskPage>(
      regPath(domain, reg) + '/batch-tasks',
      'TaskPageEnvelope',
      { domain_id: domain, registration_id: reg },
      undefined,
      cursor(next),
      signal,
    ),
  task: (domain: string, reg: string, task: string, signal?: AbortSignal) =>
    call<BatchTask>(
      regPath(domain, reg) + '/batch-tasks/' + id(task),
      'TaskEnvelope',
      { domain_id: domain, registration_id: reg, task_id: task },
      undefined,
      undefined,
      signal,
    ),
  items: (
    domain: string,
    reg: string,
    task: string,
    next?: string,
    signal?: AbortSignal,
  ) =>
    call<BatchTaskItemsPage>(
      regPath(domain, reg) + '/batch-tasks/' + id(task) + '/items',
      'TaskItemsPageEnvelope',
      { registration_id: reg, task_id: task },
      undefined,
      cursor(next),
      signal,
    ),
  byKey: async (domain: string, reg: string, request: BatchAcceptRequest) => {
    if (!validBatchData('AcceptRequest', request)) throw Invalid(400);
    const result = await call<BatchTask>(
      regPath(domain, reg) +
        '/batch-tasks/by-key/' +
        encodeURIComponent(request.idempotency_key),
      'TaskEnvelope',
      { domain_id: domain, registration_id: reg },
    );
    if (!batchTaskMatches(result.data, request)) throw Invalid();
    return result;
  },
  cancel: (
    domain: string,
    reg: string,
    task: string,
    request: BatchCancellationRequest,
  ) =>
    call<BatchTask>(
      regPath(domain, reg) + '/batch-tasks/' + id(task) + '/cancellation',
      'TaskEnvelope',
      { domain_id: domain, registration_id: reg, task_id: task },
      { schema: 'CancellationRequest', data: request, statuses: [200] },
    ),
  prepareManifest: (domain: string, request: BatchManifestPreviewRequest) =>
    call<BatchManifestPreviewPage>(
      domainPath(domain) + '/project-manifest-previews',
      'ManifestPreviewPageEnvelope',
      {
        domain_id: domain,
        project_id: request.project_id,
        task_id: request.task_id,
        expected_manifest_id: request.expected_manifest_id,
      },
      { schema: 'ManifestPreviewRequest', data: request, statuses: [201] },
    ),
  manifestPreview: (domain: string, preview: string, next?: string) =>
    call<BatchManifestPreviewPage>(
      domainPath(domain) + '/project-manifest-previews/' + id(preview),
      'ManifestPreviewPageEnvelope',
      { domain_id: domain, preview_id: preview },
      undefined,
      cursor(next),
    ),
  publish: (domain: string, request: BatchManifestPublicationRequest) =>
    call<BatchManifestReceipt>(
      domainPath(domain) + '/project-manifest-publications',
      'ManifestReceiptEnvelope',
      {
        domain_id: domain,
        project_id: request.project_id,
        preview_id: request.preview_id,
        manifest_sha256: request.manifest_sha256,
      },
      {
        schema: 'ManifestPublicationRequest',
        data: request,
        statuses: [200, 201],
      },
    ),
  publicationByKey: async (domain: string, pending: ManifestPending) => {
    if (
      pending.domain_id !== domain ||
      !validBatchData('ManifestPublicationRequest', pending.request)
    )
      throw Invalid(400);
    const request = pending.request,
      result = await call<BatchManifestReceipt>(
        domainPath(domain) +
          '/projects/' +
          id(request.project_id) +
          '/manifest-publications/by-key/' +
          encodeURIComponent(request.idempotency_key),
        'ManifestReceiptEnvelope',
        { domain_id: domain, project_id: request.project_id },
      );
    if (!batchReceiptMatches(result.data, pending)) throw Invalid();
    return result;
  },
  head: (domain: string, project: string, signal?: AbortSignal) =>
    call<BatchManifestHead>(
      domainPath(domain) + '/projects/' + id(project) + '/manifest-head',
      'ManifestHeadEnvelope',
      { domain_id: domain, project_id: project },
      undefined,
      undefined,
      signal,
    ),
  history: (
    domain: string,
    project: string,
    next?: string,
    signal?: AbortSignal,
  ) =>
    call<BatchManifestHistoryPage>(
      domainPath(domain) + '/projects/' + id(project) + '/manifests',
      'ManifestHistoryPageEnvelope',
      { domain_id: domain, project_id: project },
      undefined,
      cursor(next),
      signal,
    ),
  manifest: (
    domain: string,
    project: string,
    manifest: string,
    next?: string,
    signal?: AbortSignal,
  ) =>
    call<BatchManifestVersionPage>(
      domainPath(domain) +
        '/projects/' +
        id(project) +
        '/manifests/' +
        id(manifest),
      'ManifestVersionPageEnvelope',
      { domain_id: domain, project_id: project, manifest_id: manifest },
      undefined,
      cursor(next),
      signal,
    ),
};
export default service;

import type { AxiosResponse, AxiosProgressEvent } from 'axios';
import type * as Data from '@/interfaces/database/knowledge-web';
import type * as Request from '@/interfaces/request/knowledge-web';
import { registerNextServer } from '@/utils/register-server';
import {
  WebId,
  WebKey,
  WebSha,
  validWebData,
  webJsonBytes,
} from '@/utils/knowledge-web-schema';
import {
  webDigest,
  webCaptureProjection,
  checkWebEntry,
  webSha256,
  equalWebValue,
} from '@/utils/knowledge-web';
import { KnowledgeRequestError } from './knowledge-foundation-service';

export class WebRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId = '',
    public failure: Data.WebFailure | null = null,
  ) {
    super(status, code, requestId);
  }
}
export type WebScope = { domain: string; owner: string; registration: string };
export type WebResponse<T> = { data: T; httpStatus: number };
export type WebReadingTarget =
  | { scope: WebScope; entry: Data.WebEntry }
  | { revision: Data.WebRevision };
const Server = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  write: { url: (config: { url: string }) => config.url, method: 'post' },
  put: { url: (config: { url: string }) => config.url, method: 'put' },
  cancel: { url: (config: { url: string }) => config.url, method: 'delete' },
});
const Headers = {
  'X-Knowledge-Contract-Version': '1.0.0',
  'X-Knowledge-Source-Contract-Version': '0.6.0',
};
const Invalid = (status = 502) =>
  new WebRequestError(
    status,
    status === 400 ? 'knowledge_invalid_request' : 'web_invalid_response',
  );
function id(value: string) {
  if (!WebId.test(value)) throw Invalid(400);
  return value;
}
function key(value: string) {
  if (!WebKey.test(value)) throw Invalid(400);
  return value;
}
function sha(value: string) {
  if (!WebSha.test(value)) throw Invalid(400);
  return value;
}
const domainPath = (domain: string) => '/datasets/' + id(domain) + '/knowledge';
const registrationPath = (scope: WebScope) =>
  domainPath(scope.domain) + '/web-registrations/' + id(scope.registration);
const capturePath = (scope: WebScope, capture: string) =>
  registrationPath(scope) + '/captures/' + id(capture);
const entryPath = (scope: WebScope, capture: string, entry: string) =>
  capturePath(scope, capture) + '/observation/entries/' + sha(entry);
const revisionPath = (domain: string, revision: string) =>
  domainPath(domain) + '/web-revisions/' + id(revision);
function header(response: AxiosResponse, name: string) {
  return String(response.headers?.[name.toLowerCase()] ?? '');
}
async function failure(error: unknown): Promise<never> {
  const issue = error as { code?: string; response?: AxiosResponse };
  if (issue.code === 'ERR_CANCELED') throw error;
  const response = issue.response;
  if (!response) throw new WebRequestError(0, 'web_unavailable');
  if (response.status === 401)
    throw new WebRequestError(401, 'knowledge_unauthorized');
  let value = response.data;
  if (value instanceof Blob) {
    if (
      value.size > 32768 ||
      header(response, 'Content-Type').split(';')[0] !== 'application/json'
    )
      throw Invalid();
    try {
      value = JSON.parse(await value.text());
    } catch {
      throw Invalid();
    }
  }
  if (
    !value ||
    webJsonBytes(value) > 32768 ||
    !validWebData('Error', value, response.status)
  )
    throw Invalid();
  throw new WebRequestError(
    response.status,
    value.code,
    value.request_id,
    value.failure,
  );
}
async function call<T>(
  method: 'read' | 'write' | 'put' | 'cancel',
  path: string,
  schema: string,
  scope: Record<string, unknown> = {},
  options: {
    body?: unknown;
    requestSchema?: string;
    params?: Record<string, unknown>;
    signal?: AbortSignal;
    file?: Blob;
    onProgress?: (event: AxiosProgressEvent) => void;
    created?: boolean;
  } = {},
): Promise<WebResponse<T>> {
  if (
    options.requestSchema &&
    !validWebData(options.requestSchema, options.body)
  )
    throw Invalid(400);
  if (options.body !== undefined && webJsonBytes(options.body) > 32768)
    throw new WebRequestError(413, 'web_too_large');
  let response: AxiosResponse;
  try {
    response = await Server[method](
      {
        url: '/api/v1' + path,
        params: options.params,
        data:
          options.body === undefined ? undefined : JSON.stringify(options.body),
        signal: options.signal,
        timeout: 25000,
        skipGlobalErrorNotification: true,
        headers: {
          ...Headers,
          ...(options.file
            ? { 'Content-Type': 'application/octet-stream' }
            : options.body !== undefined
              ? { 'Content-Type': 'application/json' }
              : {}),
        },
        ...(options.file
          ? {
              transformRequest: [() => options.file],
              onUploadProgress: options.onProgress,
            }
          : {}),
      },
      true,
    );
  } catch (error) {
    return failure(error);
  }
  const envelope = response.data;
  if (
    envelope?.contract_version !== '1.0.0' ||
    envelope?.source_contract_version !== '0.6.0'
  )
    throw new WebRequestError(409, 'knowledge_source_contract_version');
  if (
    ![200, ...(options.created ? [201] : [])].includes(response.status) ||
    webJsonBytes(envelope) > 32768 ||
    !validWebData(schema, envelope) ||
    header(response, 'Cache-Control') !== 'no-store'
  )
    throw Invalid();
  for (const [name, value] of Object.entries(scope))
    if (envelope.data[name] !== value) throw Invalid();
  const value = envelope.data;
  if (
    schema === 'RegistrationEnvelope' &&
    ((await webDigest('registration', value)) !== value.config_sha256 ||
      (await webDigest('network_scope', value.network_scope)) !==
        value.scope_sha256)
  )
    throw Invalid();
  if (
    schema === 'CaptureEnvelope' &&
    (await webDigest(
      'capture_request',
      webCaptureProjection(value.registration_id, value),
    )) !== value.request_sha256
  )
    throw Invalid();
  if (schema === 'EntryEnvelope' || schema === 'WebRevisionEnvelope')
    try {
      await checkWebEntry(
        value,
        schema === 'WebRevisionEnvelope' ? value.classification : undefined,
      );
    } catch {
      throw Invalid();
    }
  return { data: value as T, httpStatus: response.status };
}
function context(scope: WebScope) {
  id(scope.owner);
  return {
    domain_id: id(scope.domain),
    registration_id: id(scope.registration),
  };
}
function owned(scope: WebScope) {
  return { ...context(scope), owner_id: scope.owner };
}
async function registrationResult(
  result: WebResponse<Data.WebRegistration>,
  request?: Request.WebCreateRegistration,
) {
  if (
    request &&
    (result.data.registration_key !== request.registration_key ||
      result.data.config_sha256 !== (await webDigest('registration', request)))
  )
    throw Invalid();
  return result;
}
async function captureResult(
  result: WebResponse<Data.WebCapture>,
  scope: WebScope,
  request: Request.WebCreateCapture,
) {
  if (
    result.data.idempotency_key !== request.idempotency_key ||
    result.data.request_sha256 !==
      (await webDigest(
        'capture_request',
        webCaptureProjection(scope.registration, request),
      ))
  )
    throw Invalid();
  return result;
}
async function previewResult(
  result: WebResponse<Data.WebPreview>,
  request: Request.WebCreatePreview,
) {
  if (
    result.data.idempotency_key !== request.idempotency_key ||
    result.data.request_sha256 !==
      (await webDigest('selection_request', request)) ||
    result.data.selection_sha256 !== (await webDigest('selection', request)) ||
    result.data.capture_id !== request.capture_id ||
    result.data.observation_id !== request.observation_id ||
    result.data.observation_sha256 !== request.observation_sha256
  )
    throw Invalid();
  return result;
}
async function receiptResult(
  result: WebResponse<Data.WebReceipt>,
  request: Request.WebConfirm,
) {
  if (
    result.data.idempotency_key !== request.idempotency_key ||
    result.data.preview_id !== request.preview_id ||
    result.data.manifest_sha256 !== request.manifest_sha256 ||
    result.data.request_sha256 !== (await webDigest('confirmation', request))
  )
    throw Invalid();
  return result;
}
function pageParams(cursor?: string | null) {
  return cursor ? { cursor: id(cursor) } : {};
}
export function webRepresentation(
  target: WebReadingTarget,
  kind: Data.WebRepresentation['kind'],
) {
  const data = 'entry' in target ? target.entry : target.revision;
  const value = data.representations.find((item) => item.kind === kind);
  if (!value) throw Invalid(400);
  return value;
}
function representationPath(
  target: WebReadingTarget,
  kind: Data.WebRepresentation['kind'],
) {
  if (kind !== 'safe_reading' && kind !== 'decoded_content') throw Invalid(400);
  return (
    ('entry' in target
      ? entryPath(target.scope, target.entry.capture_id, target.entry.entry_key)
      : revisionPath(
          target.revision.domain_id,
          target.revision.source_revision_id,
        )) +
    '/representations/' +
    kind
  );
}
async function binary(
  path: string,
  expectedSha: string,
  expectedLength?: number,
  signal?: AbortSignal,
) {
  sha(expectedSha);
  let response: AxiosResponse<Blob>;
  try {
    response = await Server.read(
      {
        url: '/api/v1' + path,
        responseType: 'blob',
        timeout: 25000,
        signal,
        skipGlobalErrorNotification: true,
        headers: Headers,
      },
      true,
    );
  } catch (error) {
    return failure(error);
  }
  if (
    header(response, 'X-Knowledge-Contract-Version') !== '1.0.0' ||
    header(response, 'X-Knowledge-Source-Contract-Version') !== '0.6.0'
  )
    throw new WebRequestError(409, 'knowledge_source_contract_version');
  const length = header(response, 'Content-Length');
  if (
    response.status !== 200 ||
    !(response.data instanceof Blob) ||
    !/^\d+$/.test(length) ||
    Number(length) > 2097152 ||
    response.data.size !== Number(length) ||
    (expectedLength !== undefined && response.data.size !== expectedLength) ||
    header(response, 'Content-Type').split(';')[0] !==
      'application/octet-stream' ||
    header(response, 'X-Content-SHA256') !== expectedSha ||
    header(response, 'Cache-Control') !== 'no-store' ||
    header(response, 'X-Content-Type-Options') !== 'nosniff' ||
    !/^attachment(?:;|$)/.test(header(response, 'Content-Disposition')) ||
    !header(response, 'Content-Security-Policy').includes("default-src 'none'")
  )
    throw Invalid();
  if ((await webSha256(await response.data.arrayBuffer())) !== expectedSha)
    throw new WebRequestError(502, 'web_capture_integrity');
  return response.data;
}
async function windowResult(
  target: WebReadingTarget,
  kind: Data.WebRepresentation['kind'],
  start: number,
  end: number,
  signal?: AbortSignal,
) {
  const descriptor = webRepresentation(target, kind),
    data = 'entry' in target ? target.entry : target.revision;
  if (
    !Number.isSafeInteger(start) ||
    !Number.isSafeInteger(end) ||
    start < 0 ||
    end < start ||
    end > descriptor.byte_length ||
    end - start > 4096
  )
    throw Invalid(400);
  const result = await call<Data.WebWindow>(
    'read',
    representationPath(target, kind) + '/window',
    'WindowEnvelope',
    {
      domain_id:
        'entry' in target ? target.scope.domain : target.revision.domain_id,
      capture_id: data.capture_id,
      observation_id: data.observation_id,
      entry_key: data.entry_key,
      source_revision_id:
        'entry' in target ? null : target.revision.source_revision_id,
    },
    { params: { start_byte: start, end_byte: end }, signal },
  );
  if (
    !equalWebValue(result.data.representation, descriptor) ||
    result.data.start_byte !== start ||
    result.data.end_byte !== end
  )
    throw Invalid();
  return result;
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<Data.WebCapabilities>(
      'read',
      '/knowledge/web-import-contract',
      'CapabilitiesEnvelope',
      {},
      { signal },
    ),
  async createRegistration(
    scope: Pick<WebScope, 'domain' | 'owner'>,
    body: Request.WebCreateRegistration,
    signal?: AbortSignal,
  ) {
    return registrationResult(
      await call<Data.WebRegistration>(
        'write',
        domainPath(scope.domain) + '/web-registrations',
        'RegistrationEnvelope',
        { domain_id: scope.domain, owner_id: id(scope.owner) },
        { body, requestSchema: 'CreateRegistration', created: true, signal },
      ),
      body,
    );
  },
  async registrationByKey(
    scope: Pick<WebScope, 'domain' | 'owner'>,
    request: Request.WebCreateRegistration,
    signal?: AbortSignal,
  ) {
    return registrationResult(
      await call<Data.WebRegistration>(
        'read',
        domainPath(scope.domain) + '/web-registrations/by-key',
        'RegistrationEnvelope',
        { domain_id: scope.domain, owner_id: id(scope.owner) },
        { params: { registration_key: key(request.registration_key) }, signal },
      ),
      request,
    );
  },
  registration: (scope: WebScope, signal?: AbortSignal) =>
    call<Data.WebRegistration>(
      'read',
      registrationPath(scope),
      'RegistrationEnvelope',
      owned(scope),
      { signal },
    ),
  target: (scope: WebScope, entry: string, signal?: AbortSignal) =>
    call<Data.WebTarget>(
      'read',
      registrationPath(scope) + '/target',
      'TargetEnvelope',
      { ...context(scope), entry_key: sha(entry) },
      { params: { entry_key: entry }, signal },
    ),
  async createCapture(
    scope: WebScope,
    body: Request.WebCreateCapture,
    signal?: AbortSignal,
  ) {
    return captureResult(
      await call<Data.WebCapture>(
        'write',
        registrationPath(scope) + '/captures',
        'CaptureEnvelope',
        owned(scope),
        { body, requestSchema: 'CreateCapture', created: true, signal },
      ),
      scope,
      body,
    );
  },
  async captureByKey(
    scope: WebScope,
    request: Request.WebCreateCapture,
    signal?: AbortSignal,
  ) {
    return captureResult(
      await call<Data.WebCapture>(
        'read',
        registrationPath(scope) + '/captures/by-key',
        'CaptureEnvelope',
        owned(scope),
        { params: { idempotency_key: key(request.idempotency_key) }, signal },
      ),
      scope,
      request,
    );
  },
  capture: (scope: WebScope, capture: string, signal?: AbortSignal) =>
    call<Data.WebCapture>(
      'read',
      capturePath(scope, capture),
      'CaptureEnvelope',
      { ...owned(scope), capture_id: capture },
      { signal },
    ),
  cancelCapture: (scope: WebScope, capture: string) =>
    call<Data.WebCapture>(
      'cancel',
      capturePath(scope, capture),
      'CaptureEnvelope',
      { ...owned(scope), capture_id: capture },
    ),
  upload: (
    scope: WebScope,
    capture: string,
    file: Blob,
    signal?: AbortSignal,
    onProgress?: (event: AxiosProgressEvent) => void,
  ) =>
    call<Data.WebCapture>(
      'put',
      capturePath(scope, capture) + '/content',
      'CaptureEnvelope',
      { ...owned(scope), capture_id: capture },
      { file, signal, onProgress },
    ),
  captureContent: (
    scope: WebScope,
    capture: Data.WebCapture,
    signal?: AbortSignal,
  ) =>
    binary(
      capturePath(scope, capture.capture_id) + '/content',
      capture.raw_sha256 || '',
      capture.raw_byte_length ?? undefined,
      signal,
    ),
  acquire: (
    scope: WebScope,
    capture: string,
    body: Request.WebAcquire,
    signal?: AbortSignal,
  ) =>
    call<Data.WebCapture>(
      'write',
      capturePath(scope, capture) + '/acquire',
      'CaptureEnvelope',
      { ...owned(scope), capture_id: capture },
      { body, requestSchema: 'Acquire', signal },
    ),
  observe: (
    scope: WebScope,
    capture: string,
    body: Request.WebObserve,
    signal?: AbortSignal,
  ) =>
    call<Data.WebObservation>(
      'write',
      capturePath(scope, capture) + '/observation',
      'ObservationEnvelope',
      { ...context(scope), capture_id: capture },
      { body, requestSchema: 'Observe', created: true, signal },
    ),
  observation: (scope: WebScope, capture: string, signal?: AbortSignal) =>
    call<Data.WebObservation>(
      'read',
      capturePath(scope, capture) + '/observation',
      'ObservationEnvelope',
      { ...context(scope), capture_id: capture },
      { signal },
    ),
  entries: (
    scope: WebScope,
    capture: string,
    cursor?: string | null,
    signal?: AbortSignal,
  ) =>
    call<Data.WebEntryPage>(
      'read',
      capturePath(scope, capture) + '/observation/entries',
      'EntryPageEnvelope',
      {},
      { params: pageParams(cursor), signal },
    ),
  entry: (
    scope: WebScope,
    capture: string,
    entry: string,
    signal?: AbortSignal,
  ) =>
    call<Data.WebEntry>(
      'read',
      entryPath(scope, capture, entry),
      'EntryEnvelope',
      { capture_id: capture, entry_key: entry },
      { signal },
    ),
  entryReferences: (
    scope: WebScope,
    capture: string,
    entry: string,
    cursor?: string | null,
    signal?: AbortSignal,
  ) =>
    call<Data.WebReferencePage>(
      'read',
      entryPath(scope, capture, entry) + '/references',
      'ReferencePageEnvelope',
      {},
      { params: pageParams(cursor), signal },
    ),
  representation: (
    target: WebReadingTarget,
    kind: Data.WebRepresentation['kind'],
    signal?: AbortSignal,
  ) => {
    const descriptor = webRepresentation(target, kind);
    return binary(
      representationPath(target, kind) + '/content',
      descriptor.sha256,
      descriptor.byte_length,
      signal,
    );
  },
  window: windowResult,
  async createPreview(
    scope: WebScope,
    body: Request.WebCreatePreview,
    signal?: AbortSignal,
  ) {
    return previewResult(
      await call<Data.WebPreview>(
        'write',
        registrationPath(scope) + '/previews',
        'PreviewEnvelope',
        owned(scope),
        { body, requestSchema: 'CreatePreview', created: true, signal },
      ),
      body,
    );
  },
  async previewByKey(
    scope: WebScope,
    request: Request.WebCreatePreview,
    signal?: AbortSignal,
  ) {
    return previewResult(
      await call<Data.WebPreview>(
        'read',
        registrationPath(scope) + '/previews/by-key',
        'PreviewEnvelope',
        owned(scope),
        { params: { idempotency_key: key(request.idempotency_key) }, signal },
      ),
      request,
    );
  },
  preview: (scope: WebScope, preview: string, signal?: AbortSignal) =>
    call<Data.WebPreview>(
      'read',
      registrationPath(scope) + '/previews/' + id(preview),
      'PreviewEnvelope',
      { ...owned(scope), preview_id: preview },
      { signal },
    ),
  cancelPreview: (scope: WebScope, preview: string) =>
    call<Data.WebPreview>(
      'cancel',
      registrationPath(scope) + '/previews/' + id(preview),
      'PreviewEnvelope',
      { ...owned(scope), preview_id: preview },
    ),
  previewItems: (
    scope: WebScope,
    preview: string,
    cursor?: string | null,
    signal?: AbortSignal,
  ) =>
    call<Data.WebPreviewItemPage>(
      'read',
      registrationPath(scope) + '/previews/' + id(preview) + '/items',
      'PreviewItemPageEnvelope',
      {},
      { params: pageParams(cursor), signal },
    ),
  async confirm(
    scope: WebScope,
    body: Request.WebConfirm,
    signal?: AbortSignal,
  ) {
    return receiptResult(
      await call<Data.WebReceipt>(
        'write',
        registrationPath(scope) + '/imports',
        'ReceiptEnvelope',
        owned(scope),
        { body, requestSchema: 'Confirm', created: true, signal },
      ),
      body,
    );
  },
  async receiptByKey(
    scope: WebScope,
    request: Request.WebConfirm,
    signal?: AbortSignal,
  ) {
    return receiptResult(
      await call<Data.WebReceipt>(
        'read',
        registrationPath(scope) + '/imports/by-key',
        'ReceiptEnvelope',
        owned(scope),
        { params: { idempotency_key: key(request.idempotency_key) }, signal },
      ),
      request,
    );
  },
  receipt: (scope: WebScope, receipt: string, signal?: AbortSignal) =>
    call<Data.WebReceipt>(
      'read',
      registrationPath(scope) + '/imports/' + id(receipt),
      'ReceiptEnvelope',
      { ...owned(scope), import_task_id: receipt },
      { signal },
    ),
  receiptItems: (
    scope: WebScope,
    receipt: string,
    cursor?: string | null,
    signal?: AbortSignal,
  ) =>
    call<Data.WebReceiptItemPage>(
      'read',
      registrationPath(scope) + '/imports/' + id(receipt) + '/items',
      'ReceiptItemPageEnvelope',
      {},
      { params: pageParams(cursor), signal },
    ),
  revisions: (
    domain: string,
    options: {
      group_id?: string;
      registration_id?: string;
      cursor?: string | null;
    } = {},
    signal?: AbortSignal,
  ) =>
    call<Data.WebRevisionPage>(
      'read',
      domainPath(domain) + '/web-revisions',
      'RevisionPageEnvelope',
      {},
      {
        params: {
          ...pageParams(options.cursor),
          ...(options.group_id ? { group_id: id(options.group_id) } : {}),
          ...(options.registration_id
            ? { registration_id: id(options.registration_id) }
            : {}),
        },
        signal,
      },
    ),
  revision: (domain: string, revision: string, signal?: AbortSignal) =>
    call<Data.WebRevision>(
      'read',
      revisionPath(domain, revision),
      'WebRevisionEnvelope',
      { domain_id: domain, source_revision_id: revision },
      { signal },
    ),
  revisionReferences: (
    domain: string,
    revision: string,
    cursor?: string | null,
    signal?: AbortSignal,
  ) =>
    call<Data.WebReferencePage>(
      'read',
      revisionPath(domain, revision) + '/references',
      'ReferencePageEnvelope',
      {},
      { params: pageParams(cursor), signal },
    ),
  revisionCapture: (revision: Data.WebRevision, signal?: AbortSignal) =>
    binary(
      revisionPath(revision.domain_id, revision.source_revision_id) +
        '/capture/content',
      revision.capture_sha256,
      undefined,
      signal,
    ),
};
export default service;

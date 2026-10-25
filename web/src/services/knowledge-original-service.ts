import type { AxiosResponse, AxiosProgressEvent } from 'axios';
import type {
  OriginalCapabilities,
  OriginalRegistration,
  OriginalUpload,
  OriginalReceipt,
  OriginalTarget,
  OriginalRevision,
  OriginalPage,
} from '@/interfaces/database/knowledge-original';
import type {
  OriginalRegistrationRequest,
  CreateOriginalUpload,
  ConfirmOriginalImport,
} from '@/interfaces/request/knowledge-original';
import { registerNextServer } from '@/utils/register-server';
import {
  OriginalId,
  OriginalKey,
  validOriginalData,
} from '@/utils/knowledge-original-schema';
import {
  originalDigest,
  checkOriginalUpload,
  checkOriginalReceipt,
  originalRegistrationRequest,
  verifyOriginalBlob,
  type OriginalCreatePending,
  type OriginalConfirmPending,
} from '@/utils/knowledge-original';
import { KnowledgeRequestError } from './knowledge-foundation-service';

export class OriginalRequestError extends KnowledgeRequestError {}
export type OriginalResponse<T> = { data: T; httpStatus: number };
const Server = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  write: { url: (config: { url: string }) => config.url, method: 'post' },
  put: { url: (config: { url: string }) => config.url, method: 'put' },
  cancel: { url: (config: { url: string }) => config.url, method: 'delete' },
  head: { url: (config: { url: string }) => config.url, method: 'head' },
});
const Headers = {
  'X-Knowledge-Contract-Version': '1.0.0',
  'X-Knowledge-Source-Contract-Version': '0.5.0',
};
const Invalid = (status = 502) =>
  new OriginalRequestError(
    status,
    status === 400 ? 'knowledge_invalid_request' : 'original_invalid_response',
  );
function id(value: string) {
  if (!OriginalId.test(value)) throw Invalid(400);
  return value;
}
function key(value: string) {
  if (!OriginalKey.test(value)) throw Invalid(400);
  return value;
}
const domainPath = (domain: string) => '/datasets/' + id(domain) + '/knowledge';
const regPath = (domain: string, registration: string) =>
  domainPath(domain) + '/original-registrations/' + id(registration);
function header(response: AxiosResponse, name: string) {
  return String(response.headers?.[name.toLowerCase()] ?? '');
}
async function failure(error: unknown, head = false): Promise<never> {
  const issue = error as { code?: string; response?: AxiosResponse };
  if (issue.code === 'ERR_CANCELED') throw error;
  const response = issue.response;
  if (!response) throw new OriginalRequestError(0, 'original_unavailable');
  if (response.status === 401)
    throw new OriginalRequestError(401, 'knowledge_unauthorized');
  if (head) {
    const code = header(response, 'X-Knowledge-Error-Code');
    if (
      !validOriginalData(
        'Error',
        {
          code,
          message: '',
          data: null,
          contract_version: '1.0.0',
          source_contract_version: '0.5.0',
          request_id: header(response, 'X-Request-ID'),
        },
        response.status,
      )
    )
      throw Invalid();
    throw new OriginalRequestError(
      response.status,
      code,
      header(response, 'X-Request-ID'),
    );
  }
  let data = response.data;
  if (data instanceof Blob) {
    if (
      data.size > 32768 ||
      header(response, 'Content-Type').split(';')[0] !== 'application/json'
    )
      throw Invalid();
    try {
      data = JSON.parse(await data.text());
    } catch {
      throw Invalid();
    }
  }
  if (
    !data ||
    new TextEncoder().encode(JSON.stringify(data)).length > 32768 ||
    !validOriginalData('Error', data, response.status)
  )
    throw Invalid();
  throw new OriginalRequestError(response.status, data.code, data.request_id);
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
    statuses?: number[];
    signal?: AbortSignal;
    file?: Blob;
    onUploadProgress?: (event: AxiosProgressEvent) => void;
  } = {},
): Promise<OriginalResponse<T>> {
  if (
    options.requestSchema &&
    !validOriginalData(options.requestSchema, options.body)
  )
    throw Invalid(400);
  const data =
    options.body === undefined ? undefined : JSON.stringify(options.body);
  if (data && new TextEncoder().encode(data).length > 32768)
    throw new OriginalRequestError(413, 'upload_too_large');
  let response: AxiosResponse;
  try {
    response = await Server[method](
      {
        url: '/api/v1' + path,
        params: options.params,
        data,
        signal: options.signal,
        timeout: options.file ? 300000 : 45000,
        skipGlobalErrorNotification: true,
        headers: {
          ...Headers,
          ...(options.file
            ? { 'Content-Type': 'application/octet-stream' }
            : data
              ? { 'Content-Type': 'application/json' }
              : {}),
        },
        ...(options.file
          ? {
              transformRequest: [() => options.file],
              onUploadProgress: options.onUploadProgress,
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
    envelope?.source_contract_version !== '0.5.0'
  )
    throw new OriginalRequestError(409, 'knowledge_source_contract_version');
  if (
    !(options.statuses || [200]).includes(response.status) ||
    !validOriginalData(schema, envelope) ||
    new TextEncoder().encode(JSON.stringify(envelope)).length > 262144
  )
    throw Invalid();
  for (const [name, value] of Object.entries(scope))
    if (envelope.data[name] !== value) throw Invalid();
  if (schema === 'UploadEnvelope') {
    const upload = envelope.data as OriginalUpload;
    if (
      (await originalDigest('request', upload)) !== upload.request_sha256 ||
      (['ready', 'committed'].includes(upload.state) &&
        (await originalDigest('preview', upload)) !== upload.manifest_sha256)
    )
      throw Invalid();
  }
  return { data: envelope.data as T, httpStatus: response.status };
}
async function registrationResult(
  result: OriginalResponse<OriginalRegistration>,
  owner: string,
  expected?: string,
) {
  const digest = await originalDigest('registration', {
    ...originalRegistrationRequest(result.data),
    domain_id: result.data.domain_id,
    owner_id: id(owner),
  });
  if (digest !== result.data.config_sha256 || (expected && expected !== digest))
    throw Invalid();
  return result;
}
async function binary(
  source: OriginalRevision,
  head: boolean,
  signal?: AbortSignal,
  range?: string,
  ifRange?: string,
): Promise<AxiosResponse<Blob>> {
  if (
    !validOriginalData('OriginalRevision', source) ||
    (range && range.length > 128) ||
    (ifRange && ifRange.length > 256)
  )
    throw Invalid(400);
  let response: AxiosResponse<Blob>;
  try {
    response = await Server[head ? 'head' : 'read'](
      {
        url:
          '/api/v1' +
          domainPath(source.domain_id) +
          '/original-revisions/' +
          id(source.source_revision_id) +
          '/content',
        signal,
        responseType: 'blob',
        timeout: 300000,
        skipGlobalErrorNotification: true,
        headers: {
          ...Headers,
          ...(range ? { Range: range } : {}),
          ...(ifRange ? { 'If-Range': ifRange } : {}),
        },
      },
      true,
    );
  } catch (error) {
    return failure(error, head);
  }
  const etag = '"sha256-' + source.sha256 + '"';
  const length = header(response, 'Content-Length');
  if (
    !/^[1-9][0-9]*$/.test(length) ||
    header(response, 'Content-Type') !== source.media_type ||
    header(response, 'ETag') !== etag ||
    header(response, 'X-Knowledge-Contract-Version') !== '1.0.0' ||
    header(response, 'X-Knowledge-Source-Contract-Version') !== '0.5.0' ||
    header(response, 'Cache-Control') !== 'no-store' ||
    header(response, 'Accept-Ranges') !== 'bytes' ||
    header(response, 'X-Content-Type-Options') !== 'nosniff' ||
    !header(response, 'Content-Disposition').startsWith('attachment;')
  )
    throw Invalid();
  if (head || response.status === 200) {
    if (
      response.status !== 200 ||
      (!head && range && (!ifRange || ifRange === etag)) ||
      Number(length) !== source.byte_length ||
      header(response, 'Content-Range')
    )
      throw Invalid();
  } else {
    if (response.status !== 206 || !range || (ifRange && ifRange !== etag))
      throw Invalid();
    const match = /^bytes ([0-9]+)-([0-9]+)\/([0-9]+)$/.exec(
      header(response, 'Content-Range'),
    );
    if (
      !match ||
      Number(match[3]) !== source.byte_length ||
      Number(match[1]) > Number(match[2]) ||
      Number(match[2]) >= source.byte_length ||
      Number(match[2]) - Number(match[1]) + 1 !== Number(length)
    )
      throw Invalid();
    const wanted = /^bytes=(\d*)-(\d*)$/.exec(range);
    if (!wanted || (!wanted[1] && !wanted[2])) throw Invalid();
    const start = wanted[1]
      ? Number(wanted[1])
      : Math.max(0, source.byte_length - Number(wanted[2]));
    const end =
      wanted[1] && wanted[2]
        ? Math.min(source.byte_length - 1, Number(wanted[2]))
        : source.byte_length - 1;
    if (Number(match[1]) !== start || Number(match[2]) !== end) throw Invalid();
  }
  if (
    !head &&
    (!(response.data instanceof Blob) || response.data.size !== Number(length))
  )
    throw Invalid();
  return response;
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<OriginalCapabilities>(
      'read',
      '/knowledge/original-import-contract',
      'CapabilitiesEnvelope',
      {},
      { signal },
    ),
  register: async (
    domain: string,
    owner: string,
    request: OriginalRegistrationRequest,
  ) =>
    registrationResult(
      await call<OriginalRegistration>(
        'write',
        domainPath(domain) + '/original-registrations',
        'RegistrationEnvelope',
        { domain_id: domain, registration_key: request.registration_key },
        {
          body: request,
          requestSchema: 'CreateRegistration',
          statuses: [200, 201],
        },
      ),
      owner,
      await originalDigest('registration', {
        ...request,
        domain_id: domain,
        owner_id: owner,
      }),
    ),
  registration: async (
    domain: string,
    reg: string,
    owner: string,
    signal?: AbortSignal,
  ) =>
    registrationResult(
      await call<OriginalRegistration>(
        'read',
        regPath(domain, reg),
        'RegistrationEnvelope',
        { domain_id: domain, registration_id: reg },
        { signal },
      ),
      owner,
    ),
  registrationByKey: async (
    domain: string,
    owner: string,
    registrationKey: string,
    digest: string,
  ) =>
    registrationResult(
      await call<OriginalRegistration>(
        'read',
        domainPath(domain) + '/original-registrations/by-key',
        'RegistrationEnvelope',
        { domain_id: domain, registration_key: registrationKey },
        { params: { registration_key: key(registrationKey) } },
      ),
      owner,
      digest,
    ),
  target: (
    domain: string,
    reg: string,
    relative: string,
    signal?: AbortSignal,
  ) =>
    call<OriginalTarget>(
      'read',
      regPath(domain, reg) + '/target',
      'TargetEnvelope',
      { domain_id: domain, registration_id: reg, relative_path: relative },
      { params: { relative_path: relative }, signal },
    ),
  create: async (
    domain: string,
    reg: string,
    owner: string,
    request: CreateOriginalUpload,
  ) => {
    const result = await call<OriginalUpload>(
      'write',
      regPath(domain, reg) + '/uploads',
      'UploadEnvelope',
      { domain_id: domain, registration_id: reg, owner_id: owner },
      { body: request, requestSchema: 'CreateUpload', statuses: [200, 201] },
    );
    await checkOriginalUpload(result.data, request, domain, owner, reg);
    return result;
  },
  upload: (domain: string, reg: string, upload: string, signal?: AbortSignal) =>
    call<OriginalUpload>(
      'read',
      regPath(domain, reg) + '/uploads/' + id(upload),
      'UploadEnvelope',
      { domain_id: domain, registration_id: reg, upload_id: upload },
      { signal },
    ),
  uploadByKey: async (
    pending: Extract<OriginalCreatePending, { kind: 'upload' }>,
  ) => {
    const result = await call<OriginalUpload>(
      'read',
      regPath(pending.domain_id, pending.registration_id) + '/uploads/by-key',
      'UploadEnvelope',
      {
        domain_id: pending.domain_id,
        owner_id: pending.owner_id,
        registration_id: pending.registration_id,
        request_sha256: pending.request_sha256,
        idempotency_key: pending.request.idempotency_key,
      },
      { params: { idempotency_key: key(pending.request.idempotency_key) } },
    );
    if (pending.upload_id && result.data.upload_id !== pending.upload_id)
      throw Invalid();
    await checkOriginalUpload(
      result.data,
      pending.request,
      pending.domain_id,
      pending.owner_id,
      pending.registration_id,
    );
    return result;
  },
  put: (
    domain: string,
    reg: string,
    upload: string,
    file: Blob,
    onUploadProgress?: (event: AxiosProgressEvent) => void,
    signal?: AbortSignal,
  ) =>
    call<OriginalUpload>(
      'put',
      regPath(domain, reg) + '/uploads/' + id(upload) + '/content',
      'UploadEnvelope',
      { domain_id: domain, registration_id: reg, upload_id: upload },
      { file, onUploadProgress, signal },
    ),
  cancel: (domain: string, reg: string, upload: string) =>
    call<OriginalUpload>(
      'cancel',
      regPath(domain, reg) + '/uploads/' + id(upload),
      'UploadEnvelope',
      { domain_id: domain, registration_id: reg, upload_id: upload },
    ),
  confirm: (domain: string, reg: string, request: ConfirmOriginalImport) =>
    call<OriginalReceipt>(
      'write',
      regPath(domain, reg) + '/imports',
      'ReceiptEnvelope',
      {
        domain_id: domain,
        registration_id: reg,
        upload_id: request.upload_id,
        manifest_sha256: request.manifest_sha256,
        idempotency_key: request.idempotency_key,
      },
      { body: request, requestSchema: 'Confirm', statuses: [200, 201] },
    ),
  receipt: (domain: string, reg: string, task: string, signal?: AbortSignal) =>
    call<OriginalReceipt>(
      'read',
      regPath(domain, reg) + '/imports/' + id(task),
      'ReceiptEnvelope',
      { domain_id: domain, registration_id: reg, import_task_id: task },
      { signal },
    ),
  receiptByKey: async (pending: OriginalConfirmPending) => {
    const result = await call<OriginalReceipt>(
      'read',
      regPath(pending.domain_id, pending.registration_id) + '/imports/by-key',
      'ReceiptEnvelope',
      {
        domain_id: pending.domain_id,
        registration_id: pending.registration_id,
      },
      { params: { idempotency_key: key(pending.request.idempotency_key) } },
    );
    checkOriginalReceipt(
      result.data,
      pending.domain_id,
      pending.registration_id,
      pending.request,
    );
    if (
      pending.import_task_id &&
      result.data.import_task_id !== pending.import_task_id
    )
      throw Invalid();
    return result;
  },
  list: (
    domain: string,
    group?: string,
    cursor?: string,
    signal?: AbortSignal,
  ) => {
    if (cursor !== undefined && (!cursor || cursor.length > 4096))
      throw Invalid(400);
    return call<OriginalPage>(
      'read',
      domainPath(domain) + '/original-revisions',
      'OriginalPageEnvelope',
      { domain_id: domain },
      {
        params: {
          ...(group ? { group_id: id(group) } : {}),
          ...(cursor ? { cursor } : {}),
        },
        signal,
      },
    );
  },
  revision: (domain: string, revision: string, signal?: AbortSignal) =>
    call<OriginalRevision>(
      'read',
      domainPath(domain) + '/original-revisions/' + id(revision),
      'OriginalRevisionEnvelope',
      { domain_id: domain, source_revision_id: revision },
      { signal },
    ),
  head: (
    source: OriginalRevision,
    signal?: AbortSignal,
    range?: string,
    ifRange?: string,
  ) => binary(source, true, signal, range, ifRange),
  content: (
    source: OriginalRevision,
    signal?: AbortSignal,
    range?: string,
    ifRange?: string,
  ) => binary(source, false, signal, range, ifRange),
  download: async (source: OriginalRevision, signal?: AbortSignal) => {
    const response = await binary(source, false, signal);
    if (response.status !== 200) throw Invalid();
    try {
      return await verifyOriginalBlob(response.data, source);
    } catch {
      throw new OriginalRequestError(409, 'original_integrity');
    }
  },
};
export default service;

import type { AxiosResponse } from 'axios';
import type {
  DirectoryCapabilities,
  DirectoryRoots,
  DirectoryRegistration,
  DirectoryListing,
  DirectoryObservation,
  DirectoryPreview,
  DirectoryContentWindow,
  DirectoryReceipt,
  DirectoryError,
} from '@/interfaces/database/knowledge-directory';
import type {
  DirectoryRegistrationRequest,
  DirectoryListingRequest,
  DirectoryObservationRequest,
  DirectoryPreviewRequest,
  DirectoryCommitRequest,
} from '@/interfaces/request/knowledge-directory';
import { registerNextServer } from '@/utils/register-server';
import {
  validDirectoryData,
  compareDirectoryPaths,
} from '@/utils/knowledge-directory-schema';
import { sameDirectoryClassification } from '@/utils/knowledge-directory';
import { KnowledgeRequestError } from './knowledge-foundation-service';

export const DirectoryContractVersion = '0.3.0';
export class DirectoryRequestError extends KnowledgeRequestError {
  constructor(
    status: number,
    code: string,
    requestId?: string,
    public readonly reason: DirectoryError['reason'] = null,
  ) {
    super(status, code, requestId);
  }
}
export interface DirectoryResponse<T> {
  data: T;
  httpStatus: number;
}
const Server = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  write: { url: (config: { url: string }) => config.url, method: 'post' },
});
function invalid(status = 502) {
  return new DirectoryRequestError(
    status,
    status === 400
      ? 'knowledge_directory_invalid'
      : 'knowledge_directory_invalid_response',
  );
}
function id(value: string) {
  if (!/^[a-f0-9]{32}$/.test(value)) throw invalid(400);
  return value;
}
const domainPath = (domain: string) => '/datasets/' + id(domain) + '/knowledge';
const registrationPath = (domain: string, reg?: string) =>
  domainPath(domain) + '/directory-registrations' + (reg ? '/' + id(reg) : '');
async function call<T>(
  path: string,
  schema: string,
  scope: Record<string, string> = {},
  body?: { schema: string; data: unknown },
  params?: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<DirectoryResponse<T>> {
  const encoder = new TextEncoder();
  if (body && !validDirectoryData(body.schema, body.data)) throw invalid(400);
  if (body && encoder.encode(JSON.stringify(body.data)).length > 32768)
    throw new DirectoryRequestError(413, 'knowledge_directory_too_large');
  let response: AxiosResponse;
  try {
    response = await Server[body ? 'write' : 'read'](
      {
        url: '/api/v1' + path,
        signal,
        params,
        data: body ? JSON.stringify(body.data) : undefined,
        timeout: 45000,
        skipGlobalErrorNotification: true,
        headers: {
          'X-Knowledge-Contract-Version': '1.0.0',
          'X-Knowledge-Source-Contract-Version': DirectoryContractVersion,
          ...(body ? { 'Content-Type': 'application/json' } : {}),
        },
      },
      true,
    );
  } catch (error) {
    const failure = error as { response?: AxiosResponse };
    if (!failure.response)
      throw new DirectoryRequestError(
        0,
        'knowledge_directory_unavailable',
        undefined,
        'result_unknown',
      );
    const status = failure.response.status,
      value = failure.response.data;
    if (status === 401)
      throw new DirectoryRequestError(401, 'knowledge_unauthorized');
    if (!validDirectoryData('Error', value, status)) throw invalid();
    throw new DirectoryRequestError(
      status,
      value.code,
      value.request_id,
      value.reason,
    );
  }
  const value = response.data;
  if (
    value?.contract_version !== '1.0.0' ||
    value?.source_contract_version !== DirectoryContractVersion
  )
    throw new DirectoryRequestError(
      409,
      'knowledge_directory_version_mismatch',
    );
  const statuses = body
    ? ['RegistrationResponse', 'ReceiptResponse'].includes(schema)
      ? [200, 201]
      : [201]
    : [200];
  if (
    !statuses.includes(response.status) ||
    !validDirectoryData(schema, value) ||
    encoder.encode(JSON.stringify(value)).length >
      (schema === 'ContentWindowResponse' ? 1048576 : 262144)
  )
    throw invalid();
  for (const [key, expected] of Object.entries(scope))
    if (value.data[key] !== expected) throw invalid();
  return { data: value.data as T, httpStatus: response.status };
}
const service = {
  contract: (signal?: AbortSignal) =>
    call<DirectoryCapabilities>(
      '/knowledge/directory-import-contract',
      'CapabilitiesResponse',
      {},
      undefined,
      undefined,
      signal,
    ),
  roots: (domain: string, signal?: AbortSignal) =>
    call<DirectoryRoots>(
      domainPath(domain) + '/directory-roots',
      'RootsResponse',
      { domain_id: domain },
      undefined,
      undefined,
      signal,
    ),
  register: async (domain: string, request: DirectoryRegistrationRequest) => {
    const result = await call<DirectoryRegistration>(
      registrationPath(domain),
      'RegistrationResponse',
      { domain_id: domain },
      { schema: 'RegistrationRequest', data: request },
    );
    const { data } = result;
    if (
      data.registration_key !== request.registration_key ||
      data.name !== request.name ||
      data.root_id !== request.root_id ||
      !sameDirectoryClassification(
        data.default_classification,
        request.default_classification,
      ) ||
      JSON.stringify(
        [...data.scope_directories].sort(compareDirectoryPaths),
      ) !==
        JSON.stringify(
          [...request.scope_directories].sort(compareDirectoryPaths),
        )
    )
      throw invalid();
    return result;
  },
  registration: (domain: string, reg: string) =>
    call<DirectoryRegistration>(
      registrationPath(domain, reg),
      'RegistrationResponse',
      { domain_id: domain, registration_id: reg },
    ),
  createListing: (
    domain: string,
    reg: string,
    request: DirectoryListingRequest,
  ) =>
    call<DirectoryListing>(
      registrationPath(domain, reg) + '/listings',
      'ListingResponse',
      { domain_id: domain, registration_id: reg },
      { schema: 'ListingRequest', data: request },
    ),
  listing: (domain: string, reg: string, listing: string, cursor?: string) => {
    if (cursor !== undefined && (!cursor || cursor.length > 256))
      throw invalid(400);
    return call<DirectoryListing>(
      registrationPath(domain, reg) + '/listings/' + id(listing),
      'ListingResponse',
      { domain_id: domain, registration_id: reg, listing_id: listing },
      undefined,
      cursor ? { cursor } : undefined,
    );
  },
  observe: (
    domain: string,
    reg: string,
    request: DirectoryObservationRequest,
  ) =>
    call<DirectoryObservation>(
      registrationPath(domain, reg) + '/observations',
      'ObservationResponse',
      { domain_id: domain, registration_id: reg },
      { schema: 'ObservationRequest', data: request },
    ),
  observation: (domain: string, reg: string, observation: string) =>
    call<DirectoryObservation>(
      registrationPath(domain, reg) + '/observations/' + id(observation),
      'ObservationResponse',
      { domain_id: domain, registration_id: reg, observation_id: observation },
    ),
  preview: (domain: string, reg: string, request: DirectoryPreviewRequest) =>
    call<DirectoryPreview>(
      registrationPath(domain, reg) + '/previews',
      'PreviewResponse',
      {
        domain_id: domain,
        registration_id: reg,
        observation_id: request.observation_id,
        observation_sha256: request.observation_sha256,
      },
      { schema: 'PreviewRequest', data: request },
    ),
  readPreview: (domain: string, reg: string, preview: string) =>
    call<DirectoryPreview>(
      registrationPath(domain, reg) + '/previews/' + id(preview),
      'PreviewResponse',
      { domain_id: domain, registration_id: reg, preview_id: preview },
    ),
  content: async (
    domain: string,
    reg: string,
    preview: string,
    offset: number,
    length: number,
  ) => {
    if (
      !Number.isSafeInteger(offset) ||
      offset < 0 ||
      !Number.isInteger(length) ||
      length < 1 ||
      length > 65536
    )
      throw invalid(400);
    for (let shrink = 0; shrink < 4; shrink += 1) {
      try {
        return await call<DirectoryContentWindow>(
          registrationPath(domain, reg) +
            '/previews/' +
            id(preview) +
            '/content',
          'ContentWindowResponse',
          { preview_id: preview },
          undefined,
          { offset, length: length - shrink },
        );
      } catch (error) {
        if (
          !(error instanceof DirectoryRequestError) ||
          error.status !== 416 ||
          length - shrink <= 1 ||
          shrink === 3
        )
          throw error;
      }
    }
    throw invalid();
  },
  confirm: (domain: string, reg: string, request: DirectoryCommitRequest) =>
    call<DirectoryReceipt>(
      registrationPath(domain, reg) + '/imports',
      'ReceiptResponse',
      {
        domain_id: domain,
        registration_id: reg,
        preview_id: request.preview_id,
        manifest_sha256: request.manifest_sha256,
      },
      { schema: 'CommitRequest', data: request },
    ),
  receipt: (domain: string, reg: string, task: string) =>
    call<DirectoryReceipt>(
      registrationPath(domain, reg) + '/imports/' + id(task),
      'ReceiptResponse',
      { domain_id: domain, registration_id: reg, import_task_id: task },
    ),
  byKey: (domain: string, reg: string, request: DirectoryCommitRequest) => {
    if (!validDirectoryData('CommitRequest', request)) throw invalid(400);
    return call<DirectoryReceipt>(
      registrationPath(domain, reg) +
        '/imports/by-key/' +
        encodeURIComponent(request.idempotency_key),
      'ReceiptResponse',
      {
        domain_id: domain,
        registration_id: reg,
        preview_id: request.preview_id,
        manifest_sha256: request.manifest_sha256,
      },
    );
  },
};
export default service;

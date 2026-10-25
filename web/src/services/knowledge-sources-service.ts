import type { AxiosResponse } from 'axios';
import type {
  KnowledgeImportCapabilities,
  KnowledgeImportPreview,
  KnowledgeImportReceipt,
  KnowledgeRegistration,
} from '@/interfaces/database/knowledge-sources';
import type {
  KnowledgeCommitRequest,
  KnowledgePreviewRequest,
  KnowledgeRegistrationRequest,
} from '@/interfaces/request/knowledge-sources';
import { registerNextServer } from '@/utils/register-server';
import { validKnowledgeSourceData } from '@/utils/knowledge-sources-schema';
import { KnowledgeRequestError } from './knowledge-foundation-service';

export const KnowledgeSourceContractVersion = '0.2.0';
const Server = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  write: { url: (config: { url: string }) => config.url, method: 'post' },
});

export interface KnowledgeSourceResult<T> {
  data: T;
  httpStatus: number;
}

async function call<T>(
  path: string,
  responseSchema: string,
  signal?: AbortSignal,
  body?: { data: unknown; schema: string; maxBytes: number },
): Promise<KnowledgeSourceResult<T>> {
  if (body) {
    if (!validKnowledgeSourceData(body.schema, body.data))
      throw new KnowledgeRequestError(400, 'knowledge_invalid_request');
    if (
      new TextEncoder().encode(JSON.stringify(body.data)).length > body.maxBytes
    )
      throw new KnowledgeRequestError(413, 'knowledge_body_too_large');
  }
  let response: AxiosResponse;
  try {
    response = await Server[body ? 'write' : 'read'](
      {
        url: `/api/v1${path}`,
        // Freeze the validated wire keys before the shared snake-case interceptor.
        // In particular, manifest_sha256 must not become manifest_sha_256.
        data: body ? JSON.stringify(body.data) : undefined,
        signal,
        timeout: 30000,
        headers: {
          ...(body ? { 'Content-Type': 'application/json' } : {}),
          'X-Knowledge-Contract-Version': '1.0.0',
          'X-Knowledge-Source-Contract-Version': KnowledgeSourceContractVersion,
        },
        skipGlobalErrorNotification: true,
      },
      true,
    );
  } catch (error) {
    const failure = error as { response?: AxiosResponse; code?: string };
    throw new KnowledgeRequestError(
      failure.response?.status || 0,
      String(failure.response?.data?.code || 'knowledge_unavailable'),
      failure.response?.data?.request_id,
    );
  }
  const value = response.data;
  if (value?.code !== 0) {
    const code = Number(value?.code);
    throw new KnowledgeRequestError(
      [401, 403, 404, 409].includes(code) ? code : response.status,
      String(value?.code || 'knowledge_invalid_response'),
      value?.request_id,
    );
  }
  if (
    value.contract_version !== '1.0.0' ||
    value.source_contract_version !== KnowledgeSourceContractVersion ||
    (responseSchema === 'CapabilitiesResponse' &&
      (value.data?.version !== KnowledgeSourceContractVersion ||
        value.data?.status !== 'Frozen'))
  ) {
    throw new KnowledgeRequestError(
      409,
      'knowledge_source_contract_version',
      value.request_id,
    );
  }
  const statuses = body
    ? responseSchema === 'PreviewResponse'
      ? [201]
      : [200, 201]
    : [200];
  if (
    !statuses.includes(response.status) ||
    !validKnowledgeSourceData(responseSchema, value)
  )
    throw new KnowledgeRequestError(
      502,
      'knowledge_invalid_response',
      value.request_id,
    );
  return { data: value.data as T, httpStatus: response.status };
}

const registrationPath = (domainId: string, registrationId?: string) =>
  `/datasets/${encodeURIComponent(domainId)}/knowledge/registrations${
    registrationId ? `/${encodeURIComponent(registrationId)}` : ''
  }`;

function inDomain<T extends { domain_id: string }>(
  value: T,
  domainId: string,
): T {
  if (value.domain_id !== domainId)
    throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
  return value;
}

function inRegistration<
  T extends { domain_id: string; registration_id: string },
>(value: T, domainId: string, registrationId: string): T {
  inDomain(value, domainId);
  if (value.registration_id !== registrationId)
    throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
  return value;
}

const knowledgeSourcesService = {
  contract: async (signal?: AbortSignal) =>
    (
      await call<KnowledgeImportCapabilities>(
        '/knowledge/import-contract',
        'CapabilitiesResponse',
        signal,
      )
    ).data,
  register: async (domainId: string, request: KnowledgeRegistrationRequest) => {
    const result = await call<KnowledgeRegistration>(
      registrationPath(domainId),
      'RegistrationResponse',
      undefined,
      {
        data: request,
        schema: 'RegistrationRequest',
        maxBytes: 16384,
      },
    );
    inDomain(result.data, domainId);
    return result;
  },
  registration: async (
    domainId: string,
    registrationId: string,
    signal?: AbortSignal,
  ) =>
    inRegistration(
      (
        await call<KnowledgeRegistration>(
          registrationPath(domainId, registrationId),
          'RegistrationResponse',
          signal,
        )
      ).data,
      domainId,
      registrationId,
    ),
  preview: async (
    domainId: string,
    registrationId: string,
    request: KnowledgePreviewRequest,
  ) => {
    if (new TextEncoder().encode(request.text).length > 2097152)
      throw new KnowledgeRequestError(413, 'knowledge_source_too_large');
    const result = await call<KnowledgeImportPreview>(
      `${registrationPath(domainId, registrationId)}/previews`,
      'PreviewResponse',
      undefined,
      {
        data: request,
        schema: 'PreviewRequest',
        maxBytes: 16777216,
      },
    );
    inRegistration(result.data, domainId, registrationId);
    return result;
  },
  readPreview: async (
    domainId: string,
    registrationId: string,
    previewId: string,
    signal?: AbortSignal,
  ) => {
    const value = inRegistration(
      (
        await call<KnowledgeImportPreview>(
          `${registrationPath(domainId, registrationId)}/previews/${encodeURIComponent(previewId)}`,
          'PreviewResponse',
          signal,
        )
      ).data,
      domainId,
      registrationId,
    );
    if (value.preview_id !== previewId)
      throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
    return value;
  },
  confirm: async (
    domainId: string,
    registrationId: string,
    request: KnowledgeCommitRequest,
  ) => {
    const result = await call<KnowledgeImportReceipt>(
      `${registrationPath(domainId, registrationId)}/imports`,
      'ReceiptResponse',
      undefined,
      {
        data: request,
        schema: 'CommitRequest',
        maxBytes: 16384,
      },
    );
    inRegistration(result.data, domainId, registrationId);
    if (
      result.data.preview_id !== request.preview_id ||
      result.data.manifest_sha256 !== request.manifest_sha256
    )
      throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
    return result;
  },
  receipt: async (
    domainId: string,
    registrationId: string,
    taskId: string,
    signal?: AbortSignal,
  ) => {
    const value = inRegistration(
      (
        await call<KnowledgeImportReceipt>(
          `${registrationPath(domainId, registrationId)}/imports/${encodeURIComponent(taskId)}`,
          'ReceiptResponse',
          signal,
        )
      ).data,
      domainId,
      registrationId,
    );
    if (value.import_task_id !== taskId)
      throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
    return value;
  },
};

export default knowledgeSourcesService;

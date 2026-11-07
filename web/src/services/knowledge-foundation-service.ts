import type { AxiosResponse } from 'axios';
import type { IDataset } from '@/interfaces/database/dataset';
import type {
  KnowledgeCapabilities,
  KnowledgeContent,
  KnowledgeDomain,
  KnowledgeGroup,
  KnowledgeNode,
  KnowledgePage,
  KnowledgeSource,
} from '@/interfaces/database/knowledge-foundation';
import type {
  CreateKnowledgeGroupRequest,
  RegisterKnowledgeDomainRequest,
  KnowledgeContentParams,
  KnowledgeListParams,
} from '@/interfaces/request/knowledge-foundation';
import { registerNextServer } from '@/utils/register-server';
import { validKnowledgeData } from '@/utils/knowledge-foundation-schema';

export const KnowledgeContractVersion = '1.0.0';

export class KnowledgeRequestError extends Error {
  constructor(
    public status: number,
    public code: string,
    public requestId = '',
  ) {
    super(code);
    this.name = 'KnowledgeRequestError';
  }
}

const KnowledgeServer = registerNextServer({
  read: { url: (config: { url: string }) => config.url, method: 'get' },
  post: { url: (config: { url: string }) => config.url, method: 'post' },
  put: { url: (config: { url: string }) => config.url, method: 'put' },
});

async function read<T>(
  path: string,
  params: object = {},
  signal?: AbortSignal,
  schema = '',
): Promise<T> {
  let response: AxiosResponse;
  try {
    response = await KnowledgeServer.read(
      {
        url: `/api/v1${path}`,
        params,
        signal,
        headers: { 'X-Knowledge-Contract-Version': KnowledgeContractVersion },
        skipGlobalErrorNotification: true,
      },
      true,
    );
  } catch (error) {
    const failure = error as { response?: AxiosResponse; code?: string };
    if (failure.code === 'ERR_CANCELED') throw error;
    throw new KnowledgeRequestError(
      failure.response?.status || 0,
      failure.response?.data?.code || 'knowledge_unavailable',
      failure.response?.data?.request_id,
    );
  }
  const envelope = response.data;
  if (envelope?.code !== 0) {
    const code = envelope?.code;
    const status = [401, 403, 404, 409].includes(Number(code))
      ? Number(code)
      : response.status;
    throw new KnowledgeRequestError(
      status,
      String(code || 'knowledge_invalid_response'),
      envelope?.request_id,
    );
  }
  if (envelope?.contract_version !== KnowledgeContractVersion) {
    throw new KnowledgeRequestError(
      409,
      'knowledge_contract_version',
      envelope?.request_id,
    );
  }
  if (response.status !== 200) {
    throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
  }
  if (envelope.data == null) {
    throw new KnowledgeRequestError(
      502,
      'knowledge_invalid_response',
      envelope?.request_id,
    );
  }
  if (!validKnowledgeData(schema, envelope.data)) {
    throw new KnowledgeRequestError(
      502,
      'knowledge_invalid_response',
      envelope?.request_id,
    );
  }
  return envelope.data as T;
}

const domainPath = (domainId: string) =>
  `/datasets/${encodeURIComponent(domainId)}/knowledge`;

async function write<T>(
  path: string,
  method: 'post' | 'put',
  data: object,
  status: number,
  schema = '',
): Promise<T> {
  let response: AxiosResponse;
  try {
    response = await KnowledgeServer[method](
      {
        url: `/api/v1${path}`,
        data,
        headers: schema
          ? { 'X-Knowledge-Contract-Version': KnowledgeContractVersion }
          : {},
        skipGlobalErrorNotification: true,
      },
      true,
    );
  } catch (error) {
    const failure = error as { response?: AxiosResponse };
    throw new KnowledgeRequestError(
      failure.response?.status || 0,
      String(failure.response?.data?.code || 'knowledge_unavailable'),
      failure.response?.data?.request_id,
    );
  }
  const envelope = response.data;
  if (envelope?.code !== 0) {
    const code = Number(envelope?.code);
    const rejected = [101, 102].includes(code)
      ? 400
      : [400, 401, 403, 404, 409, 413, 415, 422].includes(code)
        ? code
        : response.status;
    throw new KnowledgeRequestError(
      rejected,
      String(envelope?.code || 'knowledge_invalid_response'),
      envelope?.request_id,
    );
  }
  if (
    response.status !== status ||
    envelope.data == null ||
    (schema &&
      (envelope.contract_version !== KnowledgeContractVersion ||
        !validKnowledgeData(schema, envelope.data)))
  ) {
    throw new KnowledgeRequestError(
      502,
      'knowledge_invalid_response',
      envelope?.request_id,
    );
  }
  return envelope.data as T;
}

const knowledgeFoundationService = {
  datasets: async (ownerId: string, page: number, signal?: AbortSignal) => {
    const response = await KnowledgeServer.read(
      {
        url: '/api/v1/datasets',
        params: { owner_ids: ownerId, page, page_size: 20 },
        signal,
        skipGlobalErrorNotification: true,
      },
      true,
    );
    const envelope = response.data;
    if (
      response.status !== 200 ||
      envelope?.code !== 0 ||
      !Array.isArray(envelope.data) ||
      !Number.isSafeInteger(envelope.total_datasets) ||
      envelope.total_datasets < 0 ||
      !envelope.data.every(
        (item: IDataset) =>
          item &&
          /^[a-f0-9]{32}$/.test(item.id) &&
          typeof item.name === 'string' &&
          typeof item.created_by === 'string' &&
          typeof item.permission === 'string' &&
          [item.document_count, item.chunk_count, item.token_num].every(
            Number.isFinite,
          ),
      )
    ) {
      throw new KnowledgeRequestError(502, 'knowledge_invalid_response');
    }
    return {
      items: envelope.data as IDataset[],
      total: envelope.total_datasets as number,
    };
  },
  createDataset: (name: string) =>
    write<unknown>(
      '/datasets',
      'post',
      { name, permission: 'me', parse_type: 1, parser_id: 'general' },
      200,
    ),
  registerDomain: (
    domainId: string,
    request: RegisterKnowledgeDomainRequest,
  ) => {
    if (!validKnowledgeData('RegisterDomain', request)) {
      throw new KnowledgeRequestError(400, 'knowledge_invalid_request');
    }
    return write<KnowledgeDomain>(
      `${domainPath(domainId)}/domain`,
      'put',
      request,
      200,
      'Domain',
    );
  },
  createGroup: (domainId: string, request: CreateKnowledgeGroupRequest) => {
    if (!validKnowledgeData('CreateGroup', request)) {
      throw new KnowledgeRequestError(400, 'knowledge_invalid_request');
    }
    return write<KnowledgeGroup>(
      `${domainPath(domainId)}/groups`,
      'post',
      request,
      201,
      'Group',
    );
  },
  contract: (signal?: AbortSignal) =>
    read<KnowledgeCapabilities>(
      '/knowledge/contract',
      {},
      signal,
      'Capabilities',
    ),
  domains: (params: KnowledgeListParams, signal?: AbortSignal) =>
    read<KnowledgePage<KnowledgeDomain>>(
      '/knowledge/domains',
      params,
      signal,
      'DomainPage',
    ),
  domain: (domainId: string, signal?: AbortSignal) =>
    read<KnowledgeDomain>(
      `${domainPath(domainId)}/domain`,
      {},
      signal,
      'Domain',
    ),
  groups: (
    domainId: string,
    params: KnowledgeListParams & { parent_id?: string },
    signal?: AbortSignal,
  ) =>
    read<KnowledgePage<KnowledgeGroup>>(
      `${domainPath(domainId)}/groups`,
      params,
      signal,
      'GroupPage',
    ),
  sources: (
    domainId: string,
    params: KnowledgeListParams & { group_id?: string },
    signal?: AbortSignal,
  ) =>
    read<KnowledgePage<KnowledgeSource>>(
      `${domainPath(domainId)}/sources`,
      params,
      signal,
      'SourcePage',
    ),
  source: (domainId: string, revisionId: string, signal?: AbortSignal) =>
    read<KnowledgeSource>(
      `${domainPath(domainId)}/sources/${encodeURIComponent(revisionId)}`,
      {},
      signal,
      'Source',
    ),
  nodes: (
    domainId: string,
    params: KnowledgeListParams & {
      source_revision_id: string;
      parent_id?: string;
    },
    signal?: AbortSignal,
  ) =>
    read<KnowledgePage<KnowledgeNode>>(
      `${domainPath(domainId)}/nodes`,
      params,
      signal,
      'NodePage',
    ),
  node: (domainId: string, nodeId: string, signal?: AbortSignal) =>
    read<KnowledgeNode>(
      `${domainPath(domainId)}/nodes/${encodeURIComponent(nodeId)}`,
      {},
      signal,
      'Node',
    ),
  content: async (
    domainId: string,
    nodeId: string,
    params: KnowledgeContentParams,
    signal?: AbortSignal,
  ) => {
    // A 64KiB window may end inside a UTF-8 character. Only the server knows
    // the source bytes; shrink its end by at most three bytes, never JS indices.
    for (let trim = 0; trim < 4; trim += 1) {
      try {
        const window = {
          ...params,
          end_byte:
            params.end_byte === undefined ? undefined : params.end_byte - trim,
        };
        return await read<KnowledgeContent>(
          `${domainPath(domainId)}/nodes/${encodeURIComponent(nodeId)}/content`,
          window,
          signal,
          'Content',
        );
      } catch (error) {
        if (
          !(error instanceof KnowledgeRequestError) ||
          error.status !== 400 ||
          error.code !== 'knowledge_invalid_request' ||
          params.end_byte === undefined ||
          params.start_byte === undefined ||
          params.end_byte - trim <= params.start_byte ||
          trim === 3
        )
          throw error;
      }
    }
    throw new KnowledgeRequestError(400, 'knowledge_invalid_request');
  },
};

export default knowledgeFoundationService;

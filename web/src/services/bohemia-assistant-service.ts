/**
 * Network calls for 波西米亚, the in-app guide. Everything goes through the
 * existing dataset, document, chat and model services, so the assistant has
 * exactly the signed-in person's permissions. The answer stream uses the same
 * `/chat/completions` endpoint as the chat page; it goes through `fetch` for
 * the reason given in chat-completion-stream.ts.
 */
import { Authorization } from '@/constants/authorization';
import { IDialog } from '@/interfaces/database/chat';
import { IDataset } from '@/interfaces/database/dataset';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IAddedModel, IDefaultModel } from '@/interfaces/database/llm';
import { ISetDefaultModelRequestBody } from '@/interfaces/request/llm';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { registerNextServer } from '@/utils/register-server';
import { buildDocumentIngestPayload } from './document-ingest-adapter';
import kbService, {
  deleteDocument,
  listDataset,
  listDocument,
  uploadDocument,
} from './knowledge-service';
import llmService from './llm-service';
import chatService from './next-chat-service';
import userService from './user-service';

type Envelope<T> = { code?: number; data?: T; message?: string };

export class BohemiaRequestError extends Error {}

function unwrap<T>(envelope: Envelope<T> | undefined, fallback: string): T {
  if (!envelope || envelope.code !== 0) {
    throw new BohemiaRequestError(envelope?.message || fallback);
  }
  return envelope.data as T;
}

/** Errors are shown on the assistant's own cards, not as global toasts. */
const quiet = { skipGlobalErrorNotification: true };

export async function fetchCurrentUserId(): Promise<string> {
  const { data } = await userService.userInfo();
  return unwrap<{ id: string }>(data, '读取当前账号失败').id;
}

export async function listMyDatasets(keywords?: string): Promise<IDataset[]> {
  const { data } = await listDataset({
    page: 1,
    page_size: 100,
    ...(keywords ? { keywords } : {}),
  });
  return unwrap<IDataset[]>(data, '读取知识库列表失败') ?? [];
}

const datasetService = registerNextServer({
  createDataset: { url: api.createKb, method: 'post' },
  updateDataset: {
    url: ({ datasetId }: { datasetId: string }) => api.updateKb(datasetId),
    method: 'put',
  },
} as const);

/**
 * Creates an empty dataset with the general chunker. The create endpoint
 * takes no description, so one is written by a follow-up update.
 */
export async function createDataset({
  description,
  ...params
}: {
  name: string;
  description?: string;
  embedding_model: string;
}): Promise<Pick<IDataset, 'id' | 'name' | 'embedding_model'>> {
  const { data } = await datasetService.createDataset(
    { data: { ...params, parse_type: 1, parser_id: 'general' }, ...quiet },
    true,
  );
  const created = unwrap<Pick<IDataset, 'id' | 'name' | 'embedding_model'>>(
    data,
    '新建知识库失败',
  );
  if (description) {
    const { data: updated } = await datasetService.updateDataset(
      { datasetId: created.id, data: { description }, ...quiet },
      true,
    );
    unwrap(updated, '写入知识库说明失败');
  }
  return created;
}

export async function listDatasetDocuments(
  datasetId: string,
): Promise<IDocumentInfo[]> {
  const { data } = await listDocument({
    id: datasetId,
    page: 1,
    page_size: 100,
  });
  return unwrap<{ docs: IDocumentInfo[] }>(data, '读取文件列表失败').docs ?? [];
}

export async function uploadDatasetDocuments(
  datasetId: string,
  files: File[],
): Promise<IDocumentInfo[]> {
  const formData = new FormData();
  files.forEach((file) => formData.append('file', file));
  return unwrap<IDocumentInfo[]>(
    await uploadDocument(datasetId, formData),
    '上传使用说明失败',
  );
}

export async function removeDatasetDocuments(datasetId: string, ids: string[]) {
  if (!ids.length) return;
  const { data } = await deleteDocument(datasetId, ids);
  unwrap(data, '替换旧版使用说明失败');
}

export async function parseDocuments(documentIds: string[]) {
  if (!documentIds.length) return;
  const { data } = await kbService.documentIngest(
    buildDocumentIngestPayload({ documentIds, run: 1 }),
  );
  unwrap(data, '解析使用说明失败');
}

export async function listMyChats(keywords: string): Promise<IDialog[]> {
  const { data } = await chatService.listChats(
    { params: { keywords, page: 1, page_size: 50 }, ...quiet },
    true,
  );
  return unwrap<{ chats: IDialog[] }>(data, '读取聊天列表失败').chats ?? [];
}

export async function createChat(params: Record<string, unknown>) {
  const { data } = await chatService.createChat(params);
  return unwrap<IDialog>(data, '创建助手聊天失败');
}

export async function patchChat(
  chatId: string,
  params: Record<string, unknown>,
) {
  const { data } = await chatService.patchChat(
    { url: api.patchChat(chatId), data: params, ...quiet },
    true,
  );
  return unwrap<IDialog>(data, '更新助手设置失败');
}

export async function listDefaultModels(): Promise<IDefaultModel[]> {
  const { data } = await llmService.listDefaultModels({ ...quiet }, true);
  return (
    unwrap<{ models: IDefaultModel[] }>(data, '读取默认模型失败').models ?? []
  );
}

export async function listAddedModels(): Promise<IAddedModel[]> {
  const { data } = await llmService.listAllAddedModels({ ...quiet }, true);
  return unwrap<IAddedModel[]>(data, '读取已配置模型失败') ?? [];
}

export async function setDefaultModel(params: ISetDefaultModelRequestBody) {
  const { data } = await llmService.setDefaultModel(
    { data: params, ...quiet },
    true,
  );
  unwrap(data, '设置默认模型失败');
}

export type BohemiaCompletionRequest = {
  chatId: string;
  messages: Array<{ role: 'user' | 'assistant'; content: string }>;
  situation: string;
};

/** Streams one answer from 波西米亚's chat without storing a session. */
export function requestBohemiaCompletion(
  { chatId, messages, situation }: BohemiaCompletionRequest,
  signal: AbortSignal,
) {
  return fetch(api.completionUrl, {
    method: 'POST',
    headers: {
      [Authorization]: getAuthorization(),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      chat_id: chatId,
      messages,
      store_history_messages: false,
      stream: true,
      situation,
    }),
    signal,
  });
}

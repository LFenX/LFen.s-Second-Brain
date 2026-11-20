import { IngestionTaskStatus } from '@/constants/knowledge';
import { IDialog } from '@/interfaces/database/chat';
import { IDataset } from '@/interfaces/database/dataset';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IAddedModel, IDefaultModel } from '@/interfaces/database/llm';
import {
  BohemiaRequestError,
  createChat,
  createDataset,
  fetchCurrentUserId,
  listDatasetDocuments,
  listDefaultModels,
  listMyChats,
  listMyDatasets,
  parseDocuments,
  patchChat,
  removeDatasetDocuments,
  uploadDatasetDocuments,
} from '@/services/bohemia-assistant-service';
import { buildModelValue, getRealModelName } from '@/utils/llm-util';
import {
  BohemiaChatName,
  BohemiaGuideDatasetDescription,
  BohemiaGuideDatasetName,
  GuideParsePollMs,
  GuideParseTimeoutMs,
  SettableModelType,
} from './constants';
import { GuideDocument, GuideDocuments } from './guide-documents';
import { BohemiaPromptParameters, BohemiaSystemPrompt } from './prompt';

/*
 * Getting 波西米亚 ready in this account: its guide dataset holds the current
 * guide files, parsed by the normal pipeline, and its chat answers from that
 * dataset with the current prompt. Both are ordinary objects found again by
 * name; the chat's prompt_config remembers which guide version each file is,
 * so a changed guide replaces only the files that changed.
 */

/** Which version of each guide file and of the prompt the chat was set up with. */
export type GuideManifest = { prompt: string; docs: Record<string, string> };

export type BohemiaReady = {
  kind: 'ready';
  chatId: string;
  guideDatasetId: string;
  guideEmbedding: string;
  datasetIds: string[];
};
export type BohemiaNeedsModels = {
  kind: 'needs-models';
  missing: SettableModelType[];
};
export type PrepareResult = BohemiaReady | BohemiaNeedsModels;

/** FNV-1a, as 8 hex digits: enough to tell one version of a text from another. */
export function hashText(text: string) {
  let hash = 0x811c9dc5;
  for (let index = 0; index < text.length; index++) {
    hash ^= text.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
}

const docHash = (doc: GuideDocument) =>
  hashText(`${doc.fileName}\n${doc.content}`);

export function guideManifest(
  documents: GuideDocument[] = GuideDocuments,
): GuideManifest {
  return {
    prompt: hashText(BohemiaSystemPrompt),
    docs: Object.fromEntries(
      documents.map((doc) => [doc.fileName, docHash(doc)]),
    ),
  };
}

const byteLength = (text: string) => new TextEncoder().encode(text).length;

export type GuideSyncPlan = {
  upload: GuideDocument[];
  remove: string[];
  /** Files already current that still need parsing (never started or failed). */
  parse: string[];
};

/**
 * What to change in the guide dataset. A file is current when the manifest
 * says this exact version was uploaded; without a manifest (the chat is
 * missing) a file of the same name and size counts as current.
 */
export function planGuideSync(
  existing: IDocumentInfo[],
  manifest: GuideManifest | undefined,
  documents: GuideDocument[] = GuideDocuments,
): GuideSyncPlan {
  const plan: GuideSyncPlan = { upload: [], remove: [], parse: [] };
  const wanted = new Set(documents.map((doc) => doc.fileName));
  for (const doc of documents) {
    const copies = existing.filter((item) => item.name === doc.fileName);
    const [first, ...extra] = copies;
    plan.remove.push(...extra.map((item) => item.id));
    const current =
      !!first &&
      (manifest
        ? manifest.docs[doc.fileName] === docHash(doc)
        : first.size === byteLength(doc.content));
    if (!current) {
      if (first) plan.remove.push(first.id);
      plan.upload.push(doc);
      continue;
    }
    const status = first.ingestion_status;
    if (
      !first.chunk_count &&
      (!status ||
        status === IngestionTaskStatus.UNSTART ||
        status === IngestionTaskStatus.FAILED ||
        status === IngestionTaskStatus.STOPPED)
    ) {
      plan.parse.push(first.id);
    }
  }
  // Files an older guide had and this one no longer does.
  for (const name of Object.keys(manifest?.docs ?? {})) {
    if (wanted.has(name)) continue;
    plan.remove.push(
      ...existing.filter((item) => item.name === name).map((item) => item.id),
    );
  }
  return plan;
}

export const defaultModelValue = (model?: IDefaultModel) =>
  model && model.enable
    ? model.model_id ||
      buildModelValue({
        model_name: model.model_name,
        model_instance: model.model_instance,
        model_provider: model.model_provider,
      })
    : '';

export const defaultModelOf = (
  defaults: IDefaultModel[],
  type: SettableModelType,
) => defaultModelValue(defaults.find((model) => model.model_type === type));

/** The value a default-model picker stores for an added model. */
export const addedModelValue = (model: IAddedModel) =>
  model.model_id ||
  buildModelValue({
    model_name: getRealModelName(model.name),
    model_instance: model.instance_name,
    model_provider: model.provider_name,
  });

const owned = <
  T extends { name: string; tenant_id: string; create_time: number },
>(
  items: T[],
  name: string,
  userId: string,
) =>
  items
    .filter((item) => item.name === name && item.tenant_id === userId)
    .sort((a, b) => (a.create_time ?? 0) - (b.create_time ?? 0))[0];

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitForGuide(datasetId: string, report: (note: string) => void) {
  const names = new Set(GuideDocuments.map((doc) => doc.fileName));
  const started = Date.now();
  for (;;) {
    const docs = (await listDatasetDocuments(datasetId)).filter((doc) =>
      names.has(doc.name),
    );
    const failed = docs.find(
      (doc) => doc.ingestion_status === IngestionTaskStatus.FAILED,
    );
    if (failed) {
      throw new BohemiaRequestError(
        `使用说明「${failed.name}」解析失败：${failed.progress_msg || '请到知识库页查看日志'}`,
      );
    }
    const done = docs.filter((doc) => doc.chunk_count > 0).length;
    if (done >= names.size) return;
    if (Date.now() - started > GuideParseTimeoutMs) {
      throw new BohemiaRequestError(
        '使用说明还没有解析完，请稍后再打开波西米亚，或到知识库页查看解析进度。',
      );
    }
    report(`正在解析使用说明（${done}/${names.size}）…`);
    await sleep(GuideParsePollMs);
  }
}

const promptConfig = (manifest: GuideManifest) => ({
  system: BohemiaSystemPrompt,
  parameters: BohemiaPromptParameters,
  prologue: '',
  empty_response: '',
  quote: true,
  keyword: false,
  refine_multiturn: false,
  tts: false,
  bohemia_guide: manifest,
});

const sameManifest = (left: unknown, right: GuideManifest) =>
  JSON.stringify(left ?? null) === JSON.stringify(right);

const PrepareLock = 'bohemia-assistant-prepare';
let preparing: Promise<PrepareResult> | null = null;

/**
 * Gets 波西米亚 ready; concurrent callers in this page share one run, and the
 * browser lock keeps other tabs from building a second guide or chat while
 * the first parse is still running.
 */
export function prepareBohemia(report: (note: string) => void) {
  if (!preparing) {
    const run = () => runPrepare(report);
    const locked = async (): Promise<PrepareResult> =>
      navigator.locks ? await navigator.locks.request(PrepareLock, run) : run();
    preparing = locked().finally(() => {
      preparing = null;
    });
  }
  return preparing;
}

async function runPrepare(
  report: (note: string) => void,
): Promise<PrepareResult> {
  report('正在检查模型…');
  const defaults = await listDefaultModels();
  const chatModel = defaultModelOf(defaults, 'chat');
  const embedding = defaultModelOf(defaults, 'embedding');
  const missing: SettableModelType[] = [];
  if (!chatModel) missing.push('chat');
  if (!embedding) missing.push('embedding');
  if (missing.length) return { kind: 'needs-models', missing };

  const userId = await fetchCurrentUserId();
  report('正在准备使用说明…');
  let dataset: Pick<IDataset, 'id' | 'embedding_model'> | undefined = owned(
    await listMyDatasets(BohemiaGuideDatasetName),
    BohemiaGuideDatasetName,
    userId,
  );
  if (!dataset) {
    const created = await createDataset({
      name: BohemiaGuideDatasetName,
      description: BohemiaGuideDatasetDescription,
      embedding_model: embedding,
    });
    // The default may be an opaque model id; the dataset names its model the
    // way other datasets do, which is what embedding comparisons need.
    dataset = {
      id: created.id,
      embedding_model: created.embedding_model || embedding,
    };
  }
  const findChat = async (): Promise<IDialog | undefined> =>
    owned(await listMyChats(BohemiaChatName), BohemiaChatName, userId);
  const chat = await findChat();
  const previous = (chat?.prompt_config as { bohemia_guide?: GuideManifest })
    ?.bohemia_guide;
  const plan = planGuideSync(await listDatasetDocuments(dataset.id), previous);
  await removeDatasetDocuments(dataset.id, plan.remove);
  const uploaded = plan.upload.length
    ? await uploadDatasetDocuments(
        dataset.id,
        plan.upload.map(
          (doc) =>
            new File([doc.content], doc.fileName, { type: 'text/markdown' }),
        ),
      )
    : [];
  await parseDocuments([...uploaded.map((doc) => doc.id), ...plan.parse]);
  await waitForGuide(dataset.id, report);

  report('正在准备助手…');
  const manifest = guideManifest();
  let ready: IDialog;
  // Parsing takes a while; a chat made meanwhile elsewhere is used, not doubled.
  const current = chat ?? (await findChat());
  if (!current) {
    ready = await createChat({
      name: BohemiaChatName,
      description: '平台使用助手：回答这个平台怎么用，也能帮你做几件设置。',
      dataset_ids: [dataset.id],
      prompt_config: promptConfig(manifest),
    });
  } else {
    const ids = current.dataset_ids ?? [];
    const patch: Record<string, unknown> = {};
    if (
      current.prompt_config?.system !== BohemiaSystemPrompt ||
      !sameManifest(
        (current.prompt_config as { bohemia_guide?: GuideManifest })
          ?.bohemia_guide,
        manifest,
      )
    ) {
      patch.prompt_config = promptConfig(manifest);
    }
    if (!ids.includes(dataset.id)) patch.dataset_ids = [dataset.id, ...ids];
    ready = Object.keys(patch).length
      ? await patchChat(current.id, patch)
      : current;
  }
  return {
    kind: 'ready',
    chatId: ready.id,
    guideDatasetId: dataset.id,
    guideEmbedding: dataset.embedding_model,
    datasetIds: ready.dataset_ids ?? [dataset.id],
  };
}

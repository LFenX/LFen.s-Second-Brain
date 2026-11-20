import { IDataset } from '@/interfaces/database/dataset';
import { IAddedModel } from '@/interfaces/database/llm';
import {
  BohemiaRequestError,
  createDataset,
  listAddedModels,
  listDefaultModels,
  listMyDatasets,
  patchChat,
  setDefaultModel,
} from '@/services/bohemia-assistant-service';
import { parseModelValue } from '@/utils/llm-util';
import {
  AssistantAction,
  ConnectDatasetsAction,
  CreateDatasetAction,
  SetDefaultModelAction,
} from './actions';
import { defaultModelOf } from './bootstrap';
import { ModelTypeNames } from './constants';
import {
  EligibilityWords,
  datasetEligibility,
  modelDisplayName,
  modelsOfType,
} from './situation';

/*
 * Carrying out an action the person confirmed. Each one checks again against
 * what the account holds right now, then calls the same API the page would.
 */

export type ActionContext = {
  chatId: string;
  guideDatasetId: string;
  guideEmbedding: string;
  connected: string[];
};

export type ActionOutcome = {
  message: string;
  href?: string;
  /** The datasets the assistant answers from after this action. */
  connected?: string[];
};

export type ActionCopy = {
  title: string;
  subject?: string;
  consequence?: string;
  /** Navigation runs on a click; everything else is confirmed first. */
  confirm: boolean;
};

const names = (ids: string[], datasets: IDataset[]) =>
  ids
    .map((id) => datasets.find((dataset) => dataset.id === id)?.name ?? id)
    .join('、');

/**
 * The actions worth a card. The guide is always connected, so a request to
 * add or remove it is dropped, and a connection change left empty disappears.
 */
export function offeredActions(
  actions: AssistantAction[],
  guideDatasetId: string,
) {
  return actions.flatMap<AssistantAction>((action) => {
    if (action.type !== 'connect_datasets') return [action];
    const add = action.add.filter((id) => id !== guideDatasetId);
    const remove = action.remove.filter((id) => id !== guideDatasetId);
    return add.length || remove.length ? [{ ...action, add, remove }] : [];
  });
}

/** What a card says before anything runs. */
export function describeAction(
  action: AssistantAction,
  datasets: IDataset[],
  pageName: (path: string) => string | undefined,
  models: IAddedModel[] = [],
): ActionCopy {
  switch (action.type) {
    case 'navigate':
      return {
        title: action.label || pageName(action.to) || '打开页面',
        subject: pageName(action.to),
        confirm: false,
      };
    case 'connect_datasets': {
      const added = names(action.add, datasets);
      const removed = names(action.remove, datasets);
      const both = action.add.length > 0 && action.remove.length > 0;
      return {
        title: both
          ? '调整我连接的知识库'
          : action.remove.length
            ? '断开知识库'
            : '连接知识库',
        // The title already says which way a one-sided change goes.
        subject: both ? `连接 ${added}；断开 ${removed}` : added || removed,
        consequence: '之后我回答时会检索连接的知识库，平台使用说明一直保留。',
        confirm: true,
      };
    }
    case 'create_dataset':
      return {
        title: '新建知识库',
        subject: action.name,
        consequence:
          '用默认 Embedding 模型和通用切片新建一个空知识库，之后可以上传文件。',
        confirm: true,
      };
    case 'set_default_model':
      return {
        title: `设置默认 ${ModelTypeNames[action.model_type]}`,
        subject: modelDisplayName(action.model_id, models),
        consequence:
          action.model_type === 'embedding'
            ? '之后新建的知识库默认用它；已有的知识库不会换模型。'
            : '之后新建的聊天和知识库默认用它；已有的设置不变。',
        confirm: true,
      };
  }
}

async function connectDatasets(
  action: ConnectDatasetsAction,
  context: ActionContext,
): Promise<ActionOutcome> {
  const datasets = await listMyDatasets();
  // The guide always stays; asking to add it or remove it changes nothing.
  const add = action.add.filter((id) => id !== context.guideDatasetId);
  const remove = action.remove.filter(
    (id) => id !== context.guideDatasetId && context.connected.includes(id),
  );
  if (!add.length && !remove.length) {
    throw new BohemiaRequestError('没有需要改动的知识库。');
  }
  for (const id of add) {
    const dataset = datasets.find((item) => item.id === id);
    if (!dataset)
      throw new BohemiaRequestError(`找不到知识库 ${id}，可能已被删除。`);
    const state = datasetEligibility(
      dataset,
      context.connected,
      context.guideEmbedding,
    );
    if (state === 'empty' || state === 'embedding') {
      throw new BohemiaRequestError(
        `「${dataset.name}」${EligibilityWords[state]}。`,
      );
    }
  }
  const kept = context.connected.filter(
    (id) => id !== context.guideDatasetId && !remove.includes(id),
  );
  const next = [
    context.guideDatasetId,
    ...kept,
    ...add.filter((id) => !kept.includes(id)),
  ];
  const chat = await patchChat(context.chatId, { dataset_ids: next });
  const parts = [
    add.length ? `已连接 ${names(add, datasets)}` : '',
    remove.length ? `已断开 ${names(remove, datasets)}` : '',
  ].filter(Boolean);
  return { message: parts.join('；'), connected: chat.dataset_ids ?? next };
}

async function createEmptyDataset(
  action: CreateDatasetAction,
  datasetPath: (id: string) => string,
): Promise<ActionOutcome> {
  const embedding = defaultModelOf(await listDefaultModels(), 'embedding');
  if (!embedding) {
    throw new BohemiaRequestError(
      '还没有默认 Embedding 模型，先到“模型提供商”里设置。',
    );
  }
  const created = await createDataset({
    name: action.name,
    embedding_model: embedding,
  });
  return {
    message: `已新建知识库「${created.name || action.name}」`,
    href: datasetPath(created.id),
  };
}

async function changeDefaultModel(
  action: SetDefaultModelAction,
): Promise<ActionOutcome> {
  const models = await listAddedModels();
  const name = modelDisplayName(action.model_id, models);
  if (!modelsOfType(models, action.model_type).includes(action.model_id)) {
    throw new BohemiaRequestError(
      `「${name}」不在已配置的 ${ModelTypeNames[action.model_type]} 模型里。`,
    );
  }
  const parsed = parseModelValue(action.model_id);
  await setDefaultModel(
    parsed
      ? { ...parsed, model_type: action.model_type }
      : { model_id: action.model_id, model_type: action.model_type },
  );
  return {
    message: `默认 ${ModelTypeNames[action.model_type]} 已换成 ${name}`,
  };
}

/** Runs a confirmed action that changes something. */
export function runAction(
  action: Exclude<AssistantAction, { type: 'navigate' }>,
  context: ActionContext,
  datasetPath: (id: string) => string,
): Promise<ActionOutcome> {
  switch (action.type) {
    case 'connect_datasets':
      return connectDatasets(action, context);
    case 'create_dataset':
      return createEmptyDataset(action, datasetPath);
    case 'set_default_model':
      return changeDefaultModel(action);
  }
}

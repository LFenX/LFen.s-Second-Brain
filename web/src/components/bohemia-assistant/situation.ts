import { IDataset } from '@/interfaces/database/dataset';
import { IAddedModel, IDefaultModel } from '@/interfaces/database/llm';
import { getRealModelName, parseModelValue } from '@/utils/llm-util';
import dayjs from 'dayjs';
import { addedModelValue, defaultModelOf } from './bootstrap';
import {
  ModelTypeNames,
  SettableModelType,
  SettableModelTypes,
} from './constants';

/** The added-model types each default can be chosen from, as the picker shows them. */
export const PickerModelTypes: Record<SettableModelType, string[]> = {
  chat: ['chat', 'vision'],
  embedding: ['embedding'],
  rerank: ['rerank'],
};

const MaxListedDatasets = 30;
const MaxListedModels = 20;

/** Two embedding ids name the same model when their base names agree. */
export const sameEmbedding = (left: string, right: string) =>
  !!left &&
  !!right &&
  left.split('@')[0].trim().toLowerCase() ===
    right.split('@')[0].trim().toLowerCase();

export type DatasetEligibility = 'connected' | 'ok' | 'empty' | 'embedding';

export function datasetEligibility(
  dataset: IDataset,
  connected: string[],
  guideEmbedding: string,
): DatasetEligibility {
  if (connected.includes(dataset.id)) return 'connected';
  if (!dataset.chunk_count) return 'empty';
  if (guideEmbedding && !sameEmbedding(dataset.embedding_model, guideEmbedding))
    return 'embedding';
  return 'ok';
}

export const EligibilityWords: Record<DatasetEligibility, string> = {
  connected: '已连接',
  ok: '可以连接',
  empty: '不能连接：还没有解析出分块',
  embedding: '不能连接：Embedding 模型和使用说明不同',
};

export type ModelChoice = { value: string; name: string };

/**
 * The added models a default can be set to: the value the picker stores,
 * which may be an opaque id, and the name a person knows the model by.
 */
export function modelChoicesOfType(
  models: IAddedModel[],
  type: SettableModelType,
): ModelChoice[] {
  const types = PickerModelTypes[type];
  const seen = new Set<string>();
  return models
    .filter((model) => model.model_type?.some((item) => types.includes(item)))
    .map((model) => ({
      value: addedModelValue(model),
      name: getRealModelName(model.name),
    }))
    .filter(({ value }) => !seen.has(value) && !!seen.add(value));
}

export const modelsOfType = (models: IAddedModel[], type: SettableModelType) =>
  modelChoicesOfType(models, type).map((choice) => choice.value);

/** A model value as a person reads it: the model's own name. */
export function modelDisplayName(value: string, models: IAddedModel[] = []) {
  const model = models.find((item) => addedModelValue(item) === value);
  return getRealModelName(
    model?.name ?? parseModelValue(value)?.model_name ?? value,
  );
}

const defaultModelName = (
  defaults: IDefaultModel[],
  type: SettableModelType,
) => {
  const model = defaults.find((item) => item.model_type === type);
  return model?.enable && model.model_name
    ? getRealModelName(model.model_name)
    : '';
};

export type SituationInput = {
  page: string;
  datasets: IDataset[];
  guideDatasetId: string;
  guideEmbedding: string;
  connected: string[];
  defaults: IDefaultModel[];
  models: IAddedModel[];
  pages: Array<{ path: string; name: string }>;
  datasetPath: (id: string) => string;
  now?: Date;
};

/** What changes per question, written for the prompt's `{situation}`. */
export function buildSituation({
  page,
  datasets,
  guideDatasetId,
  guideEmbedding,
  connected,
  defaults,
  models,
  pages,
  datasetPath,
  now = new Date(),
}: SituationInput) {
  const lines: string[] = [];
  lines.push(`- 对方正在看：${page}`);
  lines.push(`- 现在是 ${dayjs(now).format('YYYY-MM-DD HH:mm')}（本地时间）`);

  const own = datasets.filter((dataset) => dataset.id !== guideDatasetId);
  const linked = own.filter((dataset) => connected.includes(dataset.id));
  lines.push(
    `- 你已连接的知识库：平台使用说明（自带，一直连接）${linked
      .map((dataset) => `；${dataset.name}（ID ${dataset.id}）`)
      .join('')}`,
  );
  if (own.length) {
    lines.push('- 对方的知识库（名称｜ID｜文件数｜页面路径｜状态）：');
    own.slice(0, MaxListedDatasets).forEach((dataset) => {
      const state =
        EligibilityWords[
          datasetEligibility(dataset, connected, guideEmbedding)
        ];
      lines.push(
        `  ${dataset.name}｜${dataset.id}｜${dataset.document_count ?? 0} 个文件｜${datasetPath(dataset.id)}｜${state}`,
      );
    });
    if (own.length > MaxListedDatasets) {
      lines.push(`  ……还有 ${own.length - MaxListedDatasets} 个没有列出`);
    }
  } else {
    lines.push('- 对方还没有自己的知识库。');
  }

  lines.push(
    `- 可以去的页面：${pages.map((item) => `${item.name} ${item.path}`).join('；')}`,
  );

  lines.push(
    `- 默认模型：${SettableModelTypes.map(
      (type) =>
        `${ModelTypeNames[type]} ${
          defaultModelName(defaults, type) ||
          (defaultModelOf(defaults, type) ? '已设置' : '未设置')
        }`,
    ).join('；')}`,
  );
  const choices = SettableModelTypes.map((type) => {
    const all = modelChoicesOfType(models, type);
    const listed = all
      .slice(0, MaxListedModels)
      .map(({ value, name }) => `${name}（${value}）`)
      .join('、');
    return `${ModelTypeNames[type]}：${listed || '没有'}${all.length > MaxListedModels ? ' 等' : ''}`;
  });
  lines.push(
    `- 已配置、可设为默认的模型，写作“名称（model_id）”，动作里填括号里的 model_id：${choices.join('；')}`,
  );
  return lines.join('\n');
}

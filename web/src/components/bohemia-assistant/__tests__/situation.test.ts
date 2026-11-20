import { IDataset } from '@/interfaces/database/dataset';
import { IAddedModel } from '@/interfaces/database/llm';
import {
  buildSituation,
  datasetEligibility,
  modelsOfType,
  sameEmbedding,
} from '../situation';

jest.mock('@/services/bohemia-assistant-service', () => ({}));

const Embedding = 'qwen3.7-text-embedding@models@Tongyi-Qianwen';

const dataset = (patch: Partial<IDataset>) =>
  ({
    id: 'kb1',
    name: '星湖科技',
    chunk_count: 4,
    document_count: 2,
    embedding_model: Embedding,
    ...patch,
  }) as IDataset;

const model = (patch: Partial<IAddedModel>): IAddedModel => ({
  model_id: '',
  model_type: ['chat'],
  name: 'deepseek-flash',
  provider_id: 'p',
  provider_name: 'DeepSeek',
  instance_id: 'i',
  instance_name: 'SecondBrain',
  ...patch,
});

describe('datasetEligibility', () => {
  it('says whether a dataset can join the guide', () => {
    expect(datasetEligibility(dataset({}), [], Embedding)).toBe('ok');
    expect(datasetEligibility(dataset({}), ['kb1'], Embedding)).toBe(
      'connected',
    );
    expect(datasetEligibility(dataset({ chunk_count: 0 }), [], Embedding)).toBe(
      'empty',
    );
    expect(
      datasetEligibility(
        dataset({ embedding_model: 'bge-m3@x@BAAI' }),
        [],
        Embedding,
      ),
    ).toBe('embedding');
  });

  it('compares embeddings by their base name', () => {
    expect(sameEmbedding('Qwen3.7-text-embedding@a@b', Embedding)).toBe(true);
    expect(sameEmbedding('', Embedding)).toBe(false);
  });
});

describe('modelsOfType', () => {
  it('lists the values a default can be set to, chat including vision models', () => {
    const models = [
      model({}),
      model({
        name: 'qwen-vl',
        model_type: ['vision'],
        provider_name: 'Tongyi-Qianwen',
      }),
      model({ name: 'rerank', model_type: ['rerank'], model_id: 'rr-1' }),
      model({}),
    ];
    expect(modelsOfType(models, 'chat')).toEqual([
      'deepseek-flash@SecondBrain@DeepSeek',
      'qwen-vl@SecondBrain@Tongyi-Qianwen',
    ]);
    expect(modelsOfType(models, 'rerank')).toEqual(['rr-1']);
    expect(modelsOfType(models, 'embedding')).toEqual([]);
  });
});

describe('buildSituation', () => {
  it('writes the page, datasets, pages and models for the prompt', () => {
    const text = buildSituation({
      page: '知识库列表（/datasets）',
      datasets: [
        dataset({ id: 'guide', name: '波西米亚 · 平台使用说明' }),
        dataset({}),
        dataset({ id: 'kb2', name: '空库', chunk_count: 0, document_count: 0 }),
      ],
      guideDatasetId: 'guide',
      guideEmbedding: Embedding,
      connected: ['guide', 'kb1'],
      defaults: [],
      models: [model({})],
      pages: [{ path: '/datasets', name: '知识库列表' }],
      datasetPath: (id) => `/dataset/${id}`,
      now: new Date(2026, 10, 19, 23, 5),
    });
    expect(text).toContain('对方正在看：知识库列表（/datasets）');
    expect(text).toContain('2026-11-19 23:05');
    expect(text).toContain(
      '平台使用说明（自带，一直连接）；星湖科技（ID kb1）',
    );
    expect(text).toContain('星湖科技｜kb1｜2 个文件｜/dataset/kb1｜已连接');
    expect(text).toContain(
      '空库｜kb2｜0 个文件｜/dataset/kb2｜不能连接：还没有解析出分块',
    );
    expect(text).not.toContain('波西米亚 · 平台使用说明｜');
    expect(text).toContain('知识库列表 /datasets');
    expect(text).toContain('LLM 未设置');
    expect(text).toContain(
      'LLM：deepseek-flash（deepseek-flash@SecondBrain@DeepSeek）',
    );
  });

  it('names models stored under opaque ids', () => {
    const text = buildSituation({
      page: '首页（/）',
      datasets: [],
      guideDatasetId: 'guide',
      guideEmbedding: Embedding,
      connected: ['guide'],
      defaults: [
        {
          model_type: 'chat',
          enable: true,
          model_id: 'cf7c',
          model_name: 'deepseek-flash',
          model_instance: 'SecondBrain',
          model_provider: 'DeepSeek',
        },
      ],
      models: [
        model({ model_id: 'cf7c' }),
        model({ model_id: '0cb5', name: 'deepseek-v4-flash' }),
      ],
      pages: [],
      datasetPath: (id) => `/dataset/${id}`,
    });
    expect(text).toContain('LLM deepseek-flash；');
    expect(text).toContain(
      'LLM：deepseek-flash（cf7c）、deepseek-v4-flash（0cb5）',
    );
  });
});

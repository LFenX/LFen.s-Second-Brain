import { IDataset } from '@/interfaces/database/dataset';
import * as service from '@/services/bohemia-assistant-service';
import { describeAction, offeredActions, runAction } from '../operations';
import { modelDisplayName } from '../situation';

jest.mock('@/services/bohemia-assistant-service', () => {
  class BohemiaRequestError extends Error {}
  return {
    BohemiaRequestError,
    createDataset: jest.fn(),
    listAddedModels: jest.fn(),
    listDefaultModels: jest.fn(),
    listMyDatasets: jest.fn(),
    patchChat: jest.fn(),
    setDefaultModel: jest.fn(),
  };
});

const mocked = service as jest.Mocked<typeof service>;
const Embedding = 'qwen3.7-text-embedding@models@Tongyi-Qianwen';
const Context = {
  chatId: 'chat1',
  guideDatasetId: 'guide',
  guideEmbedding: Embedding,
  connected: ['guide', 'kb1'],
};
const datasetPath = (id: string) => `/dataset/${id}`;

const dataset = (patch: Partial<IDataset>) =>
  ({ chunk_count: 3, embedding_model: Embedding, ...patch }) as IDataset;

beforeEach(() => jest.resetAllMocks());

describe('runAction', () => {
  it('connects and disconnects datasets while keeping the guide first', async () => {
    mocked.listMyDatasets.mockResolvedValue([
      dataset({ id: 'kb1', name: '旧库' }),
      dataset({ id: 'kb2', name: '星湖科技' }),
    ]);
    mocked.patchChat.mockResolvedValue({
      dataset_ids: ['guide', 'kb2'],
    } as never);
    const outcome = await runAction(
      { type: 'connect_datasets', add: ['kb2'], remove: ['kb1', 'guide'] },
      Context,
      datasetPath,
    );
    expect(mocked.patchChat).toHaveBeenCalledWith('chat1', {
      dataset_ids: ['guide', 'kb2'],
    });
    expect(outcome).toEqual({
      message: '已连接 星湖科技；已断开 旧库',
      connected: ['guide', 'kb2'],
    });
  });

  it('refuses a dataset that cannot join before calling the API', async () => {
    mocked.listMyDatasets.mockResolvedValue([
      dataset({ id: 'kb3', name: '空库', chunk_count: 0 }),
    ]);
    await expect(
      runAction(
        { type: 'connect_datasets', add: ['kb3'], remove: [] },
        Context,
        datasetPath,
      ),
    ).rejects.toThrow('「空库」不能连接：还没有解析出分块。');
    expect(mocked.patchChat).not.toHaveBeenCalled();
  });

  it('creates a dataset with the default embedding and links to it', async () => {
    mocked.listDefaultModels.mockResolvedValue([
      {
        model_type: 'embedding',
        enable: true,
        model_id: 'emb-1',
        model_name: '',
        model_instance: '',
        model_provider: '',
      },
    ]);
    mocked.createDataset.mockResolvedValue({
      id: 'new1',
      name: '读书笔记',
      embedding_model: Embedding,
    });
    const outcome = await runAction(
      { type: 'create_dataset', name: '读书笔记' },
      Context,
      datasetPath,
    );
    expect(mocked.createDataset).toHaveBeenCalledWith({
      name: '读书笔记',
      embedding_model: 'emb-1',
    });
    expect(outcome).toEqual({
      message: '已新建知识库「读书笔记」',
      href: '/dataset/new1',
    });
  });

  it('sets only a model that is already configured', async () => {
    mocked.listAddedModels.mockResolvedValue([
      {
        model_id: '',
        model_type: ['chat'],
        name: 'deepseek-flash',
        provider_id: 'p',
        provider_name: 'DeepSeek',
        instance_id: 'i',
        instance_name: 'SecondBrain',
      },
    ]);
    await expect(
      runAction(
        {
          type: 'set_default_model',
          model_type: 'chat',
          model_id: 'gpt-x@a@OpenAI',
        },
        Context,
        datasetPath,
      ),
    ).rejects.toThrow('不在已配置的 LLM 模型里');
    expect(mocked.setDefaultModel).not.toHaveBeenCalled();

    const outcome = await runAction(
      {
        type: 'set_default_model',
        model_type: 'chat',
        model_id: 'deepseek-flash@SecondBrain@DeepSeek',
      },
      Context,
      datasetPath,
    );
    expect(mocked.setDefaultModel).toHaveBeenCalledWith({
      model_name: 'deepseek-flash',
      model_instance: 'SecondBrain',
      model_provider: 'DeepSeek',
      model_type: 'chat',
    });
    expect(outcome.message).toBe('默认 LLM 已换成 deepseek-flash');
  });

  it('names a model stored under an opaque id by its own name', async () => {
    mocked.listAddedModels.mockResolvedValue([
      {
        model_id: 'cf7c3756',
        model_type: ['chat'],
        name: 'deepseek-v4-flash',
        provider_id: 'p',
        provider_name: 'DeepSeek',
        instance_id: 'i',
        instance_name: 'SecondBrain',
      },
    ]);
    const outcome = await runAction(
      { type: 'set_default_model', model_type: 'chat', model_id: 'cf7c3756' },
      Context,
      datasetPath,
    );
    expect(mocked.setDefaultModel).toHaveBeenCalledWith({
      model_id: 'cf7c3756',
      model_type: 'chat',
    });
    expect(outcome.message).toBe('默认 LLM 已换成 deepseek-v4-flash');
  });
});

describe('describeAction', () => {
  it('names what a card will do, and only navigation skips confirmation', () => {
    const datasets = [dataset({ id: 'kb2', name: '星湖科技' })];
    const pageName = (path: string) =>
      path === '/datasets' ? '知识库列表' : undefined;
    expect(
      describeAction({ type: 'navigate', to: '/datasets' }, datasets, pageName),
    ).toEqual({
      title: '知识库列表',
      subject: '知识库列表',
      confirm: false,
    });
    expect(
      describeAction(
        { type: 'connect_datasets', add: ['kb2'], remove: [] },
        datasets,
        pageName,
      ),
    ).toMatchObject({
      title: '连接知识库',
      subject: '星湖科技',
      confirm: true,
    });
    expect(
      describeAction(
        { type: 'connect_datasets', add: ['kb2'], remove: ['kb9'] },
        datasets,
        pageName,
      ),
    ).toMatchObject({
      title: '调整我连接的知识库',
      subject: '连接 星湖科技；断开 kb9',
    });
    expect(modelDisplayName('qwen__v2@models@Tongyi-Qianwen')).toBe('qwen');
    expect(
      describeAction(
        { type: 'set_default_model', model_type: 'rerank', model_id: 'rr-1' },
        datasets,
        pageName,
        [
          {
            model_id: 'rr-1',
            model_type: ['rerank'],
            name: 'qwen3.7-text-rerank',
            provider_id: 'p',
            provider_name: 'Tongyi-Qianwen',
            instance_id: 'i',
            instance_name: 'models',
          },
        ],
      ),
    ).toMatchObject({
      title: '设置默认 Rerank',
      subject: 'qwen3.7-text-rerank',
    });
  });
});

describe('offeredActions', () => {
  it('drops the always-connected guide from connection changes', () => {
    expect(
      offeredActions(
        [
          { type: 'connect_datasets', add: ['guide'], remove: [] },
          {
            type: 'connect_datasets',
            add: ['kb2', 'guide'],
            remove: ['guide'],
          },
          { type: 'create_dataset', name: '读书笔记' },
        ],
        'guide',
      ),
    ).toEqual([
      { type: 'connect_datasets', add: ['kb2'], remove: [] },
      { type: 'create_dataset', name: '读书笔记' },
    ]);
  });
});

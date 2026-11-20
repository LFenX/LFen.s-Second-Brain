import { IngestionTaskStatus } from '@/constants/knowledge';
import { IDocumentInfo } from '@/interfaces/database/document';
import {
  addedModelValue,
  defaultModelOf,
  guideManifest,
  hashText,
  planGuideSync,
} from '../bootstrap';
import { GuideDocument } from '../guide-documents';

jest.mock('@/services/bohemia-assistant-service', () => ({}));

const Docs: GuideDocument[] = [
  { fileName: 'a.md', content: '# A\n知识库' },
  { fileName: 'b.md', content: '# B\n聊天' },
];

const doc = (patch: Partial<IDocumentInfo>) =>
  ({
    id: patch.name,
    name: 'a.md',
    size: 0,
    chunk_count: 1,
    ingestion_status: IngestionTaskStatus.COMPLETED,
    ...patch,
  }) as IDocumentInfo;

const bytes = (text: string) => new TextEncoder().encode(text).length;

describe('hashText', () => {
  it('is stable and tells versions apart', () => {
    expect(hashText('知识库')).toBe(hashText('知识库'));
    expect(hashText('知识库')).not.toBe(hashText('知识库。'));
    expect(hashText('')).toMatch(/^[0-9a-f]{8}$/);
  });
});

describe('planGuideSync', () => {
  it('uploads every file into an empty guide dataset', () => {
    const plan = planGuideSync([], undefined, Docs);
    expect(plan.upload.map((item) => item.fileName)).toEqual(['a.md', 'b.md']);
    expect(plan.remove).toEqual([]);
    expect(plan.parse).toEqual([]);
  });

  it('changes nothing when the manifest matches', () => {
    const manifest = guideManifest(Docs);
    const plan = planGuideSync(
      [doc({ name: 'a.md', id: 'd-a' }), doc({ name: 'b.md', id: 'd-b' })],
      manifest,
      Docs,
    );
    expect(plan).toEqual({ upload: [], remove: [], parse: [] });
  });

  it('replaces only the file that changed and drops duplicates', () => {
    const manifest = guideManifest(Docs);
    const changed = [Docs[0], { ...Docs[1], content: '# B\n聊天（新）' }];
    const plan = planGuideSync(
      [
        doc({ name: 'a.md', id: 'd-a' }),
        doc({ name: 'a.md', id: 'd-a2' }),
        doc({ name: 'b.md', id: 'd-b' }),
      ],
      manifest,
      changed,
    );
    expect(plan.upload.map((item) => item.fileName)).toEqual(['b.md']);
    expect(plan.remove.sort()).toEqual(['d-a2', 'd-b']);
  });

  it('removes a file an older guide had and this one does not', () => {
    const manifest = guideManifest([
      ...Docs,
      { fileName: 'old.md', content: 'x' },
    ]);
    const plan = planGuideSync(
      [
        doc({ name: 'a.md', id: 'd-a' }),
        doc({ name: 'b.md', id: 'd-b' }),
        doc({ name: 'old.md', id: 'd-old' }),
        doc({ name: 'mine.md', id: 'd-mine' }),
      ],
      manifest,
      Docs,
    );
    expect(plan.remove).toEqual(['d-old']);
    expect(plan.upload).toEqual([]);
  });

  it('matches by size without a manifest and re-parses a failed file', () => {
    const plan = planGuideSync(
      [
        doc({
          name: 'a.md',
          id: 'd-a',
          size: bytes(Docs[0].content),
          chunk_count: 0,
          ingestion_status: IngestionTaskStatus.FAILED,
        }),
        doc({ name: 'b.md', id: 'd-b', size: 1 }),
      ],
      undefined,
      Docs,
    );
    expect(plan.parse).toEqual(['d-a']);
    expect(plan.upload.map((item) => item.fileName)).toEqual(['b.md']);
    expect(plan.remove).toEqual(['d-b']);
  });
});

describe('model values', () => {
  it('reads defaults and added models the way the pickers store them', () => {
    const defaults = [
      {
        model_type: 'chat',
        enable: true,
        model_id: '',
        model_name: 'deepseek-flash',
        model_instance: 'SecondBrain',
        model_provider: 'DeepSeek',
      },
      {
        model_type: 'embedding',
        enable: false,
        model_id: 'x',
        model_name: '',
        model_instance: '',
        model_provider: '',
      },
    ];
    expect(defaultModelOf(defaults, 'chat')).toBe(
      'deepseek-flash@SecondBrain@DeepSeek',
    );
    expect(defaultModelOf(defaults, 'embedding')).toBe('');
    expect(
      addedModelValue({
        model_id: '',
        model_type: ['chat'],
        name: 'qwen__v2',
        provider_id: 'p',
        provider_name: 'Tongyi-Qianwen',
        instance_id: 'i',
        instance_name: 'models',
      }),
    ).toBe('qwen@models@Tongyi-Qianwen');
  });
});

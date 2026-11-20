import { IReference } from '@/interfaces/database/chat';
import { fireEvent, render, screen } from '@testing-library/react';
import { Answer } from '../answer';
import { answerMarkdown, citedSources, excerptOf } from '../citations';

// react-markdown is ESM-only and jest does not transform node_modules. This
// stand-in renders the text as one paragraph and hands every Markdown link to
// the `a` component, which is all the citation chips depend on.
jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children, components }: any) => {
    const ReactLib = jest.requireActual('react');
    const text = String(children);
    const parts: unknown[] = [];
    let last = 0;
    for (const match of text.matchAll(/\[([^\]]+)\]\(([^)]+)\)/g)) {
      parts.push(text.slice(last, match.index));
      parts.push(
        ReactLib.createElement(
          components.a,
          { key: match.index, href: match[2] },
          match[1],
        ),
      );
      last = (match.index ?? 0) + match[0].length;
    }
    parts.push(text.slice(last));
    return ReactLib.createElement('p', null, ...parts);
  },
}));
jest.mock('remark-gfm', () => jest.fn());

const chunk = (
  id: string,
  document: string,
  dataset: string,
  content: string,
) =>
  ({
    id,
    document_id: document,
    document_name: `${document}.md`,
    dataset_id: dataset,
    content,
  }) as unknown as IReference['chunks'][number];

const Reference: IReference = {
  chunks: [
    chunk('c0', '添加知识库', 'guide', '## 新建知识库\n1. 点顶栏的“知识库”'),
    chunk('c1', '添加知识库', 'guide', '上传文件并解析'),
    chunk('c2', '星湖科技周报', 'kb1', '<b>样件</b> 放在 4 号柜'),
  ],
  doc_aggs: [],
  total: 3,
};

const Text = '先创建知识库[ID:0]。样件在 4 号柜[ID:2]，上传后解析[ID:1]。';
const datasetName = (id: string) => (id === 'kb1' ? '星湖科技' : undefined);

describe('citedSources', () => {
  it('numbers sources by document in reading order', () => {
    const { sources, byChunk } = citedSources(
      Text,
      Reference,
      'guide',
      datasetName,
    );
    expect(
      sources.map((source) => [source.n, source.title, source.place]),
    ).toEqual([
      [1, '添加知识库', '平台使用说明'],
      [2, '星湖科技周报', '知识库 · 星湖科技'],
    ]);
    expect(sources[0].chunks).toHaveLength(2);
    expect(byChunk.get(1)?.n).toBe(1);
    expect(answerMarkdown(`<think>x</think>${Text}[ID:9]`, byChunk)).toBe(
      '先创建知识库[1](#bohemia-cite-0)。样件在 4 号柜[2](#bohemia-cite-2)，上传后解析[1](#bohemia-cite-1)。',
    );
  });

  it('shows side-by-side citations of one document as one chip', () => {
    const text = '先建库[ID:1] [ID:0][ID:2]。';
    const { byChunk } = citedSources(text, Reference, 'guide', datasetName);
    expect(answerMarkdown(text, byChunk)).toBe(
      '先建库[1](#bohemia-cite-1)[2](#bohemia-cite-2)。',
    );
  });

  it('cites nothing without a reference', () => {
    expect(citedSources(Text, undefined, 'guide', datasetName).sources).toEqual(
      [],
    );
  });

  it('reads a passage as plain words', () => {
    expect(excerptOf(Reference.chunks[0])).toBe(
      '新建知识库 1. 点顶栏的“知识库”',
    );
    expect(excerptOf(Reference.chunks[2])).toBe('样件 放在 4 号柜');
  });
});

describe('Answer', () => {
  it('shows citation chips and the sources under a finished answer', () => {
    const goTo = jest.fn();
    render(
      <Answer
        text={`${Text}到[模型设置](/user-setting/model)看看。`}
        reference={Reference}
        guideDatasetId="guide"
        datasetName={datasetName}
        navigable={['/user-setting']}
        goTo={goTo}
        writing={false}
      />,
    );
    const chips = screen.getAllByRole('button', { name: /^出处 \d/ });
    expect(chips.map((chip) => chip.textContent)).toEqual(['1', '2', '1']);
    expect(screen.getByRole('region', { name: '出处' })).toHaveTextContent(
      '使用说明 1 · 知识库 1',
    );
    expect(screen.getByText('星湖科技周报')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '模型设置' }));
    expect(goTo).toHaveBeenCalledWith('/user-setting/model');
  });

  it('holds the sources back while the answer is being written', () => {
    render(
      <Answer
        text={Text}
        reference={Reference}
        guideDatasetId="guide"
        datasetName={datasetName}
        navigable={[]}
        goTo={jest.fn()}
        writing
      />,
    );
    expect(
      screen.queryByRole('region', { name: '出处' }),
    ).not.toBeInTheDocument();
  });
});

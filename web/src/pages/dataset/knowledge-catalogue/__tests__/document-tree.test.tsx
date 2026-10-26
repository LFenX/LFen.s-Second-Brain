import { fireEvent, render, screen } from '@testing-library/react';
import { createInstance } from 'i18next';
import { I18nextProvider } from 'react-i18next';
import Zh from '@/locales/zh';
import En from '@/locales/en';
import { DocumentTree } from '../document-tree';
import { useDocumentChildren } from '@/hooks/use-knowledge-document';
import { ViewScope, Rep, clone } from './document-fixture';
import Examples from '../../../../../../docs/knowledge-document-structure-v1.examples.json';

type TreeNode = import('@/interfaces/database/knowledge-document').DocumentNode;
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-document', () => {
  const actual = jest.requireActual('@/hooks/use-knowledge-document');
  return { ...actual, useDocumentChildren: jest.fn() };
});
const Pages = Examples.fixtures.trees.pdf.nodes.filter(
  (node) => node.payload.kind === 'page',
) as TreeNode[];
const Slides = Examples.fixtures.trees.pptx.nodes.filter(
  (node) => node.payload.kind === 'slide',
) as TreeNode[];
function renderNodes(nodes: TreeNode[], language = 'zh', onSelect = jest.fn()) {
  const i18n = createInstance();
  void i18n.init({
    lng: language,
    resources: { zh: Zh, en: En },
    initImmediate: false,
    interpolation: { escapeValue: false },
  });
  return {
    onSelect,
    ...render(
      <I18nextProvider i18n={i18n}>
        <ul>
          {nodes.map((node) => (
            <DocumentTree
              key={node.document_node_id}
              scope={ViewScope}
              representation={Rep}
              node={node}
              selected=""
              onSelect={onSelect}
            />
          ))}
        </ul>
      </I18nextProvider>,
    ),
  };
}
beforeEach(() => jest.clearAllMocks());
it('distinguishes every unnamed physical PDF page without changing identity or loading children', () => {
  const before = clone(Pages);
  const { onSelect } = renderNodes(Pages);
  for (let number = 1; number <= 3; number++) {
    const button = screen.getByRole('button', { name: `物理页 ${number}` });
    fireEvent.click(button);
    expect(onSelect).toHaveBeenLastCalledWith(
      Pages[number - 1].document_node_id,
    );
  }
  expect(Pages).toEqual(before);
  expect(useDocumentChildren).not.toHaveBeenCalled();
});
it('distinguishes unnamed slides by their native physical position', () => {
  renderNodes(Slides);
  expect(screen.getByRole('button', { name: '幻灯片 1' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '幻灯片 2' })).toBeInTheDocument();
});
it.each([
  { node: Pages[0], title: '物理页 1 · 原有页名' },
  { node: Slides[0], title: '幻灯片 1 · 原有页名' },
])('preserves existing source names in $title', ({ node, title }) => {
  const named = clone(node);
  named.payload.name = '原有页名';
  renderNodes([named]);
  expect(screen.getByRole('button', { name: title })).toBeInTheDocument();
  expect(named.document_node_id).toBe(node.document_node_id);
});
it('keeps DOCX source headings and does not invent physical pages or slides', () => {
  const node = Examples.fixtures.trees.docx.nodes.find(
    (node) => node.payload.kind === 'heading',
  ) as TreeNode;
  renderNodes([node]);
  expect(
    screen.getByRole('button', { name: `标题 · ${node.payload.name}` }),
  ).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /物理页|幻灯片/ })).toBeNull();
});
it('uses the existing English physical position translations', () => {
  renderNodes([Pages[0], Slides[1]], 'en');
  expect(
    screen.getByRole('button', { name: 'Physical page 1' }),
  ).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Slide 2' })).toBeInTheDocument();
});

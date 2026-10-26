import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import {
  useDocumentContent,
  useDocumentChildren,
} from '@/hooks/use-knowledge-document';
import { DocumentText, DocumentAnchoredText } from '../document-content';
import { DocumentBlockView, DocumentPosition } from '../document-blocks';
import { documentRoute } from '../document-browser';
import { Scope, ViewScope, Rep, Nodes, clone } from './document-fixture';
import Examples from '../../../../../../docs/knowledge-document-structure-v1.examples.json';
import { documentSlice } from '@/utils/knowledge-document';
import { documentSha } from '@/utils/knowledge-document-schema';
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-document', () => {
  const actual = jest.requireActual('@/hooks/use-knowledge-document');
  return {
    ...actual,
    useDocumentContent: jest.fn(),
    useDocumentChildren: jest.fn(),
  };
});
beforeEach(() => jest.clearAllMocks());
it('stops heading own reading at own_end even when aggregate complete=false', async () => {
  const payload = Examples.positive.find(
      (v) => v.name === 'heading_own_range_excludes_section_body',
    )!.value as any,
    node = { ...Nodes[2], payload },
    own = payload.own_reading_range,
    text = '标题';
  expect(await documentSha(text)).toBe(own.text_sha256);
  const data = {
    start_byte: own.start_byte,
    end_byte: own.end_byte,
    text,
    complete: false,
    next_start_byte: own.end_byte,
  };
  (useDocumentContent as jest.Mock).mockReturnValue({
    data,
    isFetching: false,
    isError: false,
  });
  render(<DocumentText scope={ViewScope} representation={Rep} node={node} />);
  await screen.findByText('标题');
  expect(
    screen.queryByRole('button', { name: 'knowledgeDocument.moreText' }),
  ).toBeNull();
  expect((useDocumentContent as jest.Mock).mock.calls[0][3]).toEqual({
    start_byte: own.start_byte,
    end_byte: own.end_byte,
  });
  expect(screen.queryByText(/Body/)).toBeNull();
});
it('does not request a zero length own range inside a nonempty aggregate', () => {
  const node = clone(Nodes[2]) as any;
  node.payload.own_reading_range = {
    start_byte: 0,
    end_byte: 0,
    text_sha256:
      'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
  };
  render(<DocumentText scope={ViewScope} representation={Rep} node={node} />);
  expect(useDocumentContent).not.toHaveBeenCalled();
});
it('escapes HTML and puts zero-width inline media between exact UTF8 segments', () => {
  const media = clone(Nodes.find((n) => n.payload.kind === 'media')!) as any;
  media.payload.attributes.inline_anchor_byte = 3;
  media.payload.attributes.placement = 'inline';
  const { container } = render(
    <DocumentAnchoredText
      text={'a—b<img src="https://invalid.example/x">'}
      start={2}
      media={[media]}
    />,
  );
  expect(container.querySelector('img')).toBeNull();
  expect(container.querySelector('svg')).toBeNull();
  expect(container.textContent).toContain('a');
  expect(container.textContent).toContain('—b<img');
  expect(
    container.textContent!.indexOf('knowledgeDocument.media'),
  ).toBeLessThan(container.textContent!.indexOf('—b<img'));
});
it('shows explicit unsupported without fetching or executing content', () => {
  const node = Nodes.find((n) => n.payload.kind === 'unsupported_block')!;
  render(
    <DocumentBlockView
      scope={ViewScope}
      representation={Rep}
      node={node}
      selected
    />,
  );
  expect(screen.getByRole('note').textContent).toContain(
    'knowledgeDocument.unsupported',
  );
  expect(useDocumentContent).not.toHaveBeenCalled();
  expect(useDocumentChildren).not.toHaveBeenCalled();
});
it('renders source table rows, empty cells and exact cell text without parent aggregation', async () => {
  const cache = new Map();
  (useDocumentChildren as jest.Mock).mockImplementation((_s, _r, parent) => ({
    data: {
      pages: [
        {
          items: Nodes.filter((n) => n.parent_id === parent.document_node_id),
          next_cursor: null,
        },
      ],
    },
    isPending: false,
    isError: false,
    hasNextPage: false,
    isFetching: false,
  }));
  for (const node of Nodes) {
    const own = node.payload.own_reading_range;
    if (!own) continue;
    const text = documentSlice(
      Examples.fixtures.trees.docx.reading_text,
      own.start_byte,
      own.end_byte,
    );
    cache.set(node.document_node_id, {
      data: {
        text,
        start_byte: own.start_byte,
        end_byte: own.end_byte,
        complete: true,
        next_start_byte: null,
      },
      isFetching: false,
      isError: false,
    });
  }
  (useDocumentContent as jest.Mock).mockImplementation((_s, _r, n) =>
    cache.get(n.document_node_id),
  );
  render(
    <DocumentBlockView
      scope={ViewScope}
      representation={Rep}
      node={Nodes.find((n) => n.payload.kind === 'table')!}
      selected
    />,
  );
  expect(screen.getAllByRole('row')).toHaveLength(2);
  expect(screen.getAllByRole('cell')).toHaveLength(4);
  await waitFor(() =>
    expect(
      screen.getAllByText('knowledgeDocument.textVerified').length,
    ).toBeGreaterThan(0),
  );
  expect(
    screen.getAllByText('knowledgeDocument.emptyOwn').length,
  ).toBeGreaterThan(0);
});
it('keeps nonselected sections collapsed without preloading their descendants', () => {
  render(
    <DocumentBlockView
      scope={ViewScope}
      representation={Rep}
      node={Nodes[1]}
    />,
  );
  expect(useDocumentChildren).not.toHaveBeenCalled();
  expect(useDocumentContent).not.toHaveBeenCalled();
});
it('provides explicit aggregate reading and never renders both copies', async () => {
  const node = clone(Nodes[2]) as any;
  node.payload = Examples.positive.find(
    (v) => v.name === 'heading_own_range_excludes_section_body',
  )!.value;
  const range = node.payload.reading_range;
  (useDocumentChildren as jest.Mock).mockReturnValue({
    data: { pages: [{ items: [] }] },
    isPending: false,
    isError: false,
    hasNextPage: false,
    isFetching: false,
  });
  const ownData = {
    data: {
      start_byte: 0,
      end_byte: 6,
      text: '标题',
      complete: false,
      next_start_byte: 6,
    },
    isFetching: false,
    isError: false,
  };
  const fullText = '标题\na—b';
  expect(await documentSha(fullText)).toBe(range.text_sha256);
  const allData = {
    data: {
      start_byte: range.start_byte,
      end_byte: range.end_byte,
      text: fullText,
      complete: true,
      next_start_byte: null,
    },
    isFetching: false,
    isError: false,
  };
  (useDocumentContent as jest.Mock).mockImplementation((_s, _r, _n, q) =>
    q.end_byte === range.end_byte ? allData : ownData,
  );
  render(
    <DocumentBlockView
      scope={ViewScope}
      representation={Rep}
      node={node}
      selected
    />,
  );
  await screen.findByText('标题');
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeDocument.aggregateText' }),
  );
  await screen.findByText('标题 a—b', { exact: false });
  expect(screen.queryByText(/^标题$/)).toBeNull();
});
it('does not invent Word physical pages', () => {
  const { container } = render(<DocumentPosition node={Nodes[2]} />);
  expect(container.textContent).not.toContain('knowledgeDocument.page');
});
it('requires all fixed identity parameters and rejects duplicates', () => {
  const p = new URLSearchParams({
    document_structure: '1',
    document_source: ViewScope.source,
    document_revision: ViewScope.revision,
    document_original_sha: ViewScope.sha,
    document_representation: ViewScope.representation,
  });
  expect(documentRoute(p, Scope)?.scope).toEqual(ViewScope);
  p.delete('document_revision');
  expect(() => documentRoute(p, Scope)).toThrow();
  p.set('document_revision', ViewScope.revision);
  p.append('document_node', 'a'.repeat(64));
  p.append('document_node', 'b'.repeat(64));
  expect(() => documentRoute(p, Scope)).toThrow();
  expect(
    documentRoute(new URLSearchParams({ document_structure: '1' }), Scope),
  ).toBeNull();
});

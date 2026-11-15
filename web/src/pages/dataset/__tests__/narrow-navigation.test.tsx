import { useEffect, useState } from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import Zh from '@/locales/zh';
import request from '@/utils/next-request';
import DatasetWrapper from '../index';
import KnowledgeCatalogue from '../knowledge-catalogue';

type IDataset = import('@/interfaces/database/dataset').IDataset;
type KnowledgeSource =
  import('@/interfaces/database/knowledge-foundation').KnowledgeSource;
type ChangeEvent<T> = import('react').ChangeEvent<T>;

const MockDomain = 'a'.repeat(32);
const MockOwner = 'b'.repeat(32);
const MockDataset = {
  id: MockDomain,
  tenant_id: MockOwner,
  name: '窄屏合成知识库',
  document_count: 4,
  create_time: 0,
  size: 1024,
} as IDataset;
const MockSource = {
  domain_id: MockDomain,
  source_id: 'c'.repeat(32),
  source_revision_id: 'd'.repeat(32),
  root_node_id: 'e'.repeat(64),
  relative_path: 'synthetic-note.txt',
  component_side: 'non_code',
  language: null,
  byte_length: 143,
  sha256: 'f'.repeat(64),
} as KnowledgeSource;
const mockRefetchDomain = jest.fn();
const mockRefetchContract = jest.fn();
const mockConsumerMount = jest.fn();
const mockConsumerUnmount = jest.fn();
const mockSourceListMount = jest.fn();
const mockSourceListUnmount = jest.fn();
const MockFixedParams = new URLSearchParams({
  content: '1',
  content_source: MockSource.source_id,
  content_revision: MockSource.source_revision_id,
  content_sha256: MockSource.sha256,
  content_kind: 'parsed_representation',
  content_representation: '1'.repeat(64),
  content_producer: '2'.repeat(64),
  content_node_kind: 'parsed_node',
  content_node: '3'.repeat(64),
});
const MockFixedPath = `/dataset/knowledge-catalogue/${MockDomain}?${MockFixedParams}`;

function mockTranslate(key: string) {
  const value = key.split('.').reduce<unknown>((part, name) => {
    return part && typeof part === 'object'
      ? (part as Record<string, unknown>)[name]
      : undefined;
  }, Zh.translation);
  return typeof value === 'string' ? value : key;
}

// The stateful leaf makes a parent remount observable without exercising a
// business workflow. The layout and Radix disclosure components remain real.
function MockWorkflowProbe({ source }: { source?: KnowledgeSource }) {
  const [file, setFile] = useState<File | null>(null);
  const [choices, setChoices] = useState<string[]>([]);
  const [index, setIndex] = useState('');
  const [receipt, setReceipt] = useState('');
  const [waiting, setWaiting] = useState(false);
  useEffect(() => {
    mockConsumerMount();
    return () => {
      mockConsumerUnmount();
    };
  }, []);
  const chooseFile = (event: ChangeEvent<HTMLInputElement>) => {
    setFile(event.target.files?.[0] ?? null);
  };
  const chooseFour = () => {
    setChoices([
      'source_revision',
      'structure_view',
      'document_representation',
      'parsed_representation',
    ]);
    setIndex('synthetic-index:synthetic-manifest');
  };
  const updateReceipt = (event: ChangeEvent<HTMLTextAreaElement>) => {
    setReceipt(event.target.value);
  };
  const beginWait = () => setWaiting(true);
  return (
    <section aria-label="消费状态探针">
      <h2>消费状态探针</h2>
      <input type="file" aria-label="待上传原件" onChange={chooseFile} />
      <output aria-label="保留的文件">{file?.name}</output>
      <button type="button" onClick={chooseFour}>
        固定四来源
      </button>
      <output aria-label="固定选择">{choices.join(',')}</output>
      <output aria-label="索引和清单">{index}</output>
      <textarea
        aria-label="操作回执"
        value={receipt}
        onChange={updateReceipt}
      />
      <button type="button" onClick={beginWait}>
        保留等待状态
      </button>
      <output aria-label="任务状态">{waiting ? 'waiting' : 'idle'}</output>
      <output aria-label="选中来源">{source?.source_id ?? ''}</output>
    </section>
  );
}

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: mockTranslate }),
}));
jest.mock('react', () => ({
  ...jest.requireActual('react'),
  lazy: () => MockWorkflowProbe,
}));
// Routes also creates the global browser router at import time. This test
// exercises the actual route constants inside its own real MemoryRouter.
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  createBrowserRouter: jest.fn(),
}));
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchKnowledgeBaseConfiguration: () => ({
    data: MockDataset,
    loading: false,
  }),
}));
jest.mock('@/pages/agent/context', () => ({
  OwnerTenantIdContext: jest.requireActual('react').createContext(''),
}));
jest.mock('@/components/ragflow-avatar', () => ({
  RAGFlowAvatar: () => null,
}));
jest.mock('@/services/knowledge-foundation-service', () => ({
  KnowledgeRequestError: class extends Error {},
}));
jest.mock('@/hooks/use-knowledge-foundation', () => ({
  useKnowledgeContract: () => ({
    data: {
      version: '1.0.0',
      model_calls: false,
      ingestion: false,
      contracts: ['C1', 'C3'],
      limits: { content_bytes: 65536 },
    },
    refetch: mockRefetchContract,
  }),
  useKnowledgeDomain: () => ({
    data: {
      domain_id: MockDomain,
      owner_id: MockOwner,
      contract_version: '1.0.0',
      name: MockDataset.name,
      category: 'synthetic',
      policy: {
        access: 'owner_only',
        external_models: false,
        automatic_ingestion: false,
        allowed_model_uses: [],
        index_modes: ['catalogue'],
        version: 1,
      },
    },
    refetch: mockRefetchDomain,
  }),
  useKnowledgeGroups: () => ({
    data: { pages: [{ items: [] }] },
    hasNextPage: false,
  }),
  useKnowledgeSources: function useMockSources() {
    jest.requireActual('react').useEffect(() => {
      mockSourceListMount();
      return () => {
        mockSourceListUnmount();
      };
    }, []);
    return {
      data: { pages: [{ items: [MockSource] }] },
      hasNextPage: false,
    };
  },
}));
jest.mock('../knowledge-catalogue/content', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('../knowledge-import/original-revisions', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('../knowledge-import/web-revisions', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('../knowledge-catalogue/lifecycle', () => ({
  __esModule: true,
  default: () => null,
  LifecycleSourceLink: () => null,
}));

function LocationProbe() {
  const location = useLocation();
  return (
    <output aria-label="当前位置">{location.pathname + location.search}</output>
  );
}

async function renderCatalogue(entry = MockFixedPath) {
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/dataset" element={<DatasetWrapper />}>
          <Route
            path="knowledge-catalogue/:id"
            element={<KnowledgeCatalogue />}
          />
          <Route path="files/:id" element={<p>文件列表目的页</p>} />
          <Route path="knowledge-access/:id" element={<p>共享授权目的页</p>} />
        </Route>
      </Routes>
      <LocationProbe />
    </MemoryRouter>,
  );
  await screen.findByRole('heading', { name: '消费状态探针' });
}

function toggleBoth() {
  fireEvent.click(screen.getByRole('button', { name: /知识库:.*展开/ }));
  fireEvent.click(screen.getByRole('button', { name: '展开 · 知识结构' }));
  fireEvent.click(screen.getByRole('button', { name: /知识库:.*收起/ }));
  fireEvent.click(screen.getByRole('button', { name: '收起 · 知识结构' }));
}

beforeEach(() => {
  jest.clearAllMocks();
});

test('both navigation disclosures have focusable native controls and stable controlled panels', async () => {
  await renderCatalogue();
  const dataset = screen.getByRole('button', { name: /知识库:.*展开/ });
  const catalogue = screen.getByRole('button', { name: '展开 · 知识结构' });
  for (const trigger of [dataset, catalogue]) {
    expect(trigger).toHaveAttribute('type', 'button');
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    const panel = document.getElementById(
      trigger.getAttribute('aria-controls')!,
    );
    expect(panel).not.toBeNull();
    trigger.focus();
    expect(trigger).toHaveFocus();
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    expect(
      document.getElementById(trigger.getAttribute('aria-controls')!),
    ).toBe(panel);
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(trigger).toHaveFocus();
  }
});

test('opening and closing both navigations preserves fixed URL, four choices, file, wait and receipt state', async () => {
  await renderCatalogue();
  const consumer = screen.getByRole('region', { name: '消费状态探针' });
  const reader = screen.getByRole('main', { name: '固定修订原文' });
  const file = new File(['synthetic'], 'pending.txt', { type: 'text/plain' });
  const input = screen.getByLabelText('待上传原件') as HTMLInputElement;
  fireEvent.change(input, { target: { files: [file] } });
  fireEvent.click(screen.getByRole('button', { name: '固定四来源' }));
  fireEvent.change(screen.getByLabelText('操作回执'), {
    target: { value: 'fixed receipt retained' },
  });
  fireEvent.click(screen.getByRole('button', { name: '保留等待状态' }));
  toggleBoth();
  toggleBoth();
  expect(screen.getByRole('region', { name: '消费状态探针' })).toBe(consumer);
  expect(screen.getByRole('main', { name: '固定修订原文' })).toBe(reader);
  expect(screen.getByLabelText('待上传原件')).toBe(input);
  expect(input.files?.[0]).toBe(file);
  expect(screen.getByLabelText('保留的文件')).toHaveTextContent('pending.txt');
  expect(screen.getByLabelText('固定选择')).toHaveTextContent(
    'source_revision,structure_view,document_representation,parsed_representation',
  );
  expect(screen.getByLabelText('索引和清单')).toHaveTextContent(
    'synthetic-index:synthetic-manifest',
  );
  expect(screen.getByLabelText('操作回执')).toHaveValue(
    'fixed receipt retained',
  );
  expect(screen.getByLabelText('任务状态')).toHaveTextContent('waiting');
  expect(screen.getByLabelText('当前位置')).toHaveTextContent(MockFixedPath);
  expect(mockConsumerMount).toHaveBeenCalledTimes(1);
  expect(mockConsumerUnmount).not.toHaveBeenCalled();
  expect(mockRefetchDomain).not.toHaveBeenCalled();
  expect(mockRefetchContract).not.toHaveBeenCalled();
  expect(request).not.toHaveBeenCalled();
});

test('catalogue navigation stays mounted and has a single source list across disclosure changes', async () => {
  await renderCatalogue();
  const nav = screen.getByRole('navigation', { name: '知识结构' });
  const source = within(nav).getByRole('button', {
    name: /synthetic-note.txt/,
  });
  toggleBoth();
  expect(screen.getByRole('navigation', { name: '知识结构' })).toBe(nav);
  expect(within(nav).getByRole('button', { name: /synthetic-note.txt/ })).toBe(
    source,
  );
  expect(screen.getAllByRole('navigation', { name: '知识结构' })).toHaveLength(
    1,
  );
  expect(mockSourceListMount).toHaveBeenCalledTimes(1);
  expect(mockSourceListUnmount).not.toHaveBeenCalled();
});

test('expanded dataset navigation keeps all normal links and routes to the selected dataset page', async () => {
  await renderCatalogue();
  fireEvent.click(screen.getByRole('button', { name: /知识库:.*展开/ }));
  const navigation = screen.getByRole('navigation', { name: '知识库' });
  expect(within(navigation).getAllByRole('link')).toHaveLength(7);
  const ownerAccess = within(navigation).getByRole('link', {
    name: mockTranslate('knowledgeAccess.ownerTitle'),
  });
  expect(ownerAccess).toHaveAttribute(
    'href',
    `/dataset/knowledge-access/${MockDomain}`,
  );
  fireEvent.click(ownerAccess);
  expect(await screen.findByText('共享授权目的页')).toBeInTheDocument();
  expect(screen.getByLabelText('当前位置')).toHaveTextContent(
    `/dataset/knowledge-access/${MockDomain}`,
  );
  const files = within(
    screen.getByRole('navigation', { name: '知识库' }),
  ).getByRole('link', { name: '文件列表' });
  expect(files).toHaveAttribute('href', `/dataset/files/${MockDomain}`);
  fireEvent.click(files);
  expect(await screen.findByText('文件列表目的页')).toBeInTheDocument();
  expect(screen.getByLabelText('当前位置')).toHaveTextContent(
    `/dataset/files/${MockDomain}`,
  );
  expect(request).not.toHaveBeenCalled();
});

test('an explicit catalogue source selection still reaches the consumer and survives collapse', async () => {
  await renderCatalogue();
  fireEvent.click(screen.getByRole('button', { name: '展开 · 知识结构' }));
  const navigation = screen.getByRole('navigation', { name: '知识结构' });
  fireEvent.click(
    within(navigation).getByRole('button', { name: /synthetic-note.txt/ }),
  );
  expect(screen.getByLabelText('选中来源')).toHaveTextContent(
    MockSource.source_id,
  );
  fireEvent.click(screen.getByRole('button', { name: '收起 · 知识结构' }));
  fireEvent.click(screen.getByRole('button', { name: '展开 · 知识结构' }));
  expect(screen.getByLabelText('选中来源')).toHaveTextContent(
    MockSource.source_id,
  );
  expect(screen.getByLabelText('当前位置')).toHaveTextContent(
    `/dataset/knowledge-catalogue/${MockDomain}?content=1`,
  );
  expect(mockConsumerMount).toHaveBeenCalledTimes(1);
  expect(mockConsumerUnmount).not.toHaveBeenCalled();
  expect(request).not.toHaveBeenCalled();
});

test('native document selection keeps its complete fixed node URL and mounted reader on navigation toggles', async () => {
  const params = new URLSearchParams({
    document_structure: '1',
    document_source: '1'.repeat(32),
    document_revision: '2'.repeat(32),
    document_original_sha: '3'.repeat(64),
    document_representation: '4'.repeat(64),
    document_node: '5'.repeat(64),
  });
  const entry = `/dataset/knowledge-catalogue/${MockDomain}?${params}`;
  await renderCatalogue(entry);
  const consumer = screen.getByRole('region', { name: '消费状态探针' });
  toggleBoth();
  expect(screen.getByLabelText('当前位置')).toHaveTextContent(entry);
  expect(screen.getByRole('region', { name: '消费状态探针' })).toBe(consumer);
  expect(mockConsumerMount).toHaveBeenCalledTimes(1);
  expect(mockConsumerUnmount).not.toHaveBeenCalled();
  expect(mockRefetchDomain).not.toHaveBeenCalled();
  expect(request).not.toHaveBeenCalled();
});

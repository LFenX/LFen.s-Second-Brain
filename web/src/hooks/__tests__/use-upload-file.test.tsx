import fileManagerService from '@/services/file-manager-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { useUploadFile } from '../use-file-request';

jest.mock('@/services/file-manager-service', () => ({
  __esModule: true,
  default: { uploadFile: jest.fn() },
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn() },
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('@/utils/file-util', () => ({ downloadFileFromBlob: jest.fn() }));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('react-router', () => ({
  useSearchParams: () => [new URLSearchParams()],
}));
jest.mock('@/hooks/logic-hooks', () => ({}));
jest.mock('@/hooks/route-hook', () => ({
  useSetPaginationParams: () => ({ setPaginationParams: jest.fn() }),
}));

it('uploads plain files and folder entries with base filenames and separate relative paths', async () => {
  const service = jest.mocked(fileManagerService.uploadFile);
  service.mockResolvedValue({ data: { code: 0 } });
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const plain = new File(['plain'], 'plain.txt');
  Object.defineProperty(plain, 'webkitRelativePath', { value: '' });
  const nested = new File(['nested'], 'nested.txt');
  const { result } = renderHook(() => useUploadFile(), { wrapper });
  await act(async () => {
    await expect(
      result.current.uploadFile({
        parentId: 'root',
        fileList: [plain, { file: nested, path: 'folder/nested.txt' }],
      }),
    ).resolves.toEqual({ code: 0 });
  });
  const payload: FormData = service.mock.calls[0][0];
  expect(payload.get('parent_id')).toBe('root');
  expect(payload.getAll('path')).toEqual(['', 'folder/nested.txt']);
  expect(
    payload
      .getAll('file')
      .map((value) => (typeof value === 'string' ? value : value.name)),
  ).toEqual(['plain.txt', 'nested.txt']);
  expect(plain.name).toBe('plain.txt');
  queryClient.clear();
});

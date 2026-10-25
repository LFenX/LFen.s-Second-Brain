import { AxiosError } from 'axios';
import request from '@/utils/next-request';
import service from './knowledge-original-service';
import {
  DomainId,
  OwnerId,
  RegistrationId,
  UploadId,
  Bytes,
  blob,
  file,
  makeUpload,
  createRequest,
  confirmRequest,
  makeReceipt,
  respond,
} from '@/pages/dataset/knowledge-import/__tests__/original-fixture';
import { getAuthorization, redirectToLogin } from '@/utils/authorization-util';

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));
jest.mock('@/utils/notification', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  default: { removeAll: jest.fn() },
  getAuthorization: jest.fn(() => 'synthetic-original-wire-token'),
  redirectToLogin: jest.fn(),
}));
it('preserves enhanced File and Blob identity and raw bytes through real Axios interceptors and local transforms', async () => {
  const upload = await makeUpload(),
    progress = jest.fn(),
    signal = new AbortController().signal;
  const data = file();
  Object.defineProperty(data, 'preview', {
    value: 'blob:synthetic',
    enumerable: true,
  });
  const second = blob();
  const adapter = jest.fn(async (config) => ({
    ...respond(upload),
    headers: {},
    statusText: 'Synthetic',
    config,
  }));
  const previous = request.defaults.adapter;
  request.defaults.adapter = adapter;
  try {
    await service.put(
      DomainId,
      RegistrationId,
      UploadId,
      data,
      progress,
      signal,
    );
    await service.put(DomainId, RegistrationId, UploadId, second);
    expect(adapter.mock.calls[0][0].data).toBe(data);
    expect(adapter.mock.calls[1][0].data).toBe(second);
    const buffer = await new Promise<ArrayBuffer>((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.readAsArrayBuffer(adapter.mock.calls[0][0].data);
    });
    expect(new Uint8Array(buffer)).toEqual(Bytes);
    const config = adapter.mock.calls[0][0];
    expect(config.headers.get('Content-Type')).toBe('application/octet-stream');
    expect(config.headers.get('Authorization')).toBe(
      'synthetic-original-wire-token',
    );
    expect(config.headers.get('X-Knowledge-Source-Contract-Version')).toBe(
      '0.5.0',
    );
    expect(config.signal).toBe(signal);
    expect(config.onUploadProgress).toBe(progress);
  } finally {
    request.defaults.adapter = previous;
  }
});
it('preserves original JSON SHA fields through real interceptors and rejects cancellation before the adapter', async () => {
  const upload = await makeUpload(),
    input = createRequest(),
    confirm = confirmRequest(upload);
  const adapter = jest.fn(async (config) => ({
    ...respond(
      config.url.endsWith('/imports') ? makeReceipt(upload, confirm) : upload,
    ),
    headers: {},
    statusText: 'Synthetic',
    config,
  }));
  const previous = request.defaults.adapter;
  request.defaults.adapter = adapter;
  try {
    await service.create(DomainId, RegistrationId, OwnerId, input);
    await service.confirm(DomainId, RegistrationId, confirm);
    expect(
      adapter.mock.calls.map(([config]) => JSON.parse(config.data)),
    ).toEqual([input, confirm]);
    const controller = new AbortController();
    controller.abort();
    await expect(
      service.put(
        DomainId,
        RegistrationId,
        UploadId,
        file(),
        undefined,
        controller.signal,
      ),
    ).rejects.toMatchObject({ code: 'ERR_CANCELED' });
    expect(adapter).toHaveBeenCalledTimes(2);
  } finally {
    request.defaults.adapter = previous;
  }
});
it('retains the original HTTP 401 cleanup and redirect with the same Axios instance', async () => {
  const previous = request.defaults.adapter;
  request.defaults.adapter = async (config) => {
    throw new AxiosError(
      'Unauthorized',
      'ERR_BAD_REQUEST',
      config,
      {},
      {
        status: 401,
        data: {},
        headers: {},
        statusText: 'Unauthorized',
        config,
      },
    );
  };
  try {
    await expect(service.contract()).rejects.toMatchObject({ status: 401 });
    expect(getAuthorization).toHaveBeenCalled();
    expect(redirectToLogin).toHaveBeenCalled();
  } finally {
    request.defaults.adapter = previous;
  }
});

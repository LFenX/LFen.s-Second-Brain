import request from '@/utils/next-request';
import service, { KnowledgeRequestError } from './knowledge-foundation-service';
import {
  Content,
  Domain,
  Root,
  envelope,
  page,
} from '@/pages/dataset/knowledge-catalogue/__tests__/fixture';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
const Http = request as unknown as jest.Mock;

beforeEach(() => Http.mockReset());

it('uses C1 paths and headers without changing native auth or adding model parameters', async () => {
  Http.mockResolvedValue({ status: 200, data: envelope(Domain) });
  expect(await service.domain(Domain.domain_id)).toEqual(Domain);
  expect(Http.mock.calls[0][0]).toMatchObject({
    url: `/api/v1/datasets/${Domain.domain_id}/knowledge/domain`,
    method: 'get',
    headers: { 'X-Knowledge-Contract-Version': '1.0.0' },
  });
  expect(Http.mock.calls[0][0].headers.Authorization).toBeUndefined();
  expect(Http.mock.calls[0][0].data).toBeUndefined();
});

it('preserves the opaque cursor and exact parent scope', async () => {
  Http.mockResolvedValue({ status: 200, data: envelope(page([], 'next')) });
  await service.nodes(Domain.domain_id, {
    source_revision_id: Root.source_revision_id,
    parent_id: Root.node_id,
    cursor: 'opaque+/=',
  });
  expect(Http.mock.calls[0][0].params).toEqual({
    source_revision_id: Root.source_revision_id,
    parent_id: Root.node_id,
    cursor: 'opaque+/=',
  });
});

it.each([401, 403, 404, 409, 500])(
  'propagates HTTP %i as a typed failure',
  async (status) => {
    Http.mockRejectedValue({
      response: {
        status,
        data: { code: 'knowledge_not_found', request_id: 'request' },
      },
    });
    await expect(service.domain(Domain.domain_id)).rejects.toMatchObject({
      status,
      requestId: 'request',
    });
  },
);

it('rejects native HTTP 200 with a nonzero application code', async () => {
  Http.mockResolvedValue({ status: 200, data: { code: 403, data: null } });
  await expect(service.domain(Domain.domain_id)).rejects.toMatchObject({
    status: 403,
  });
  Http.mockResolvedValue({
    status: 200,
    data: { ...envelope(Domain), code: 7 },
  });
  await expect(service.domain(Domain.domain_id)).rejects.toBeInstanceOf(
    KnowledgeRequestError,
  );
});

it('rejects mismatched contracts and malformed or unexpectedly permissive policy', async () => {
  Http.mockResolvedValueOnce({
    status: 200,
    data: { ...envelope(Domain), contract_version: '2.0.0' },
  });
  await expect(service.domain(Domain.domain_id)).rejects.toMatchObject({
    status: 409,
  });
  Http.mockResolvedValueOnce({
    status: 200,
    data: envelope({
      ...Domain,
      policy: { ...Domain.policy, external_models: true },
    }),
  });
  await expect(service.domain(Domain.domain_id)).rejects.toMatchObject({
    status: 502,
  });
  Http.mockResolvedValueOnce({ status: 200, data: envelope({}) });
  await expect(
    service.node(Domain.domain_id, Root.node_id),
  ).rejects.toMatchObject({ status: 502 });
});

it('adjusts a server-rejected UTF-8 end boundary by bytes only', async () => {
  Http.mockRejectedValueOnce({
    response: { status: 400, data: { code: 'knowledge_invalid_request' } },
  });
  Http.mockResolvedValueOnce({ status: 200, data: envelope(Content) });
  await service.content(Domain.domain_id, Root.node_id, {
    start_byte: 0,
    end_byte: 26,
  });
  expect(Http.mock.calls.map(([config]) => config.params.end_byte)).toEqual([
    26, 25,
  ]);
});

it('does not retry denial, revision conflicts, network errors or cancellation', async () => {
  for (const status of [403, 404, 409, 500]) {
    Http.mockReset().mockRejectedValue({
      response: { status, data: { code: 'knowledge_invalid_request' } },
    });
    await expect(
      service.content(Domain.domain_id, Root.node_id, {
        start_byte: 0,
        end_byte: 26,
      }),
    ).rejects.toMatchObject({ status });
    expect(Http).toHaveBeenCalledTimes(1);
  }
  Http.mockReset().mockRejectedValue({ code: 'ERR_CANCELED' });
  await expect(service.domain(Domain.domain_id)).rejects.toMatchObject({
    code: 'ERR_CANCELED',
  });
  expect(Http).toHaveBeenCalledTimes(1);
});

it('bounds UTF-8 probes to four total attempts', async () => {
  Http.mockRejectedValue({
    response: { status: 400, data: { code: 'knowledge_invalid_request' } },
  });
  await expect(
    service.content(Domain.domain_id, Root.node_id, {
      start_byte: 0,
      end_byte: 65536,
    }),
  ).rejects.toMatchObject({ status: 400 });
  expect(Http).toHaveBeenCalledTimes(4);
});

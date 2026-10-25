import request from '@/utils/next-request';
import service from './knowledge-batch-service';
import { manifestPending } from '@/utils/knowledge-batch';
import {
  Registration,
  DomainId,
  ProjectId,
  Capabilities,
  makeObservation,
  makePreview,
  acceptRequest,
  makeTask,
  makeResults,
  makeManifest,
  makeReceipt,
  content,
  respond,
  reject,
  times,
  Id,
} from '@/pages/dataset/knowledge-import/__tests__/batch-fixture';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: jest.fn() }));
const Http = request as unknown as jest.Mock;
const Reg = Registration.registration_id,
  Base = '/api/v1/datasets/' + DomainId + '/knowledge',
  R = Base + '/directory-registrations/' + Reg;
beforeEach(() => Http.mockReset());
it('consumes all seventeen frozen operations with correct version headers, JSON keys and HTTP statuses', async () => {
  const observation = makeObservation(),
    preview = makePreview(observation),
    accept = acceptRequest(preview),
    task = makeTask(preview),
    done = makeTask(preview, 'succeeded'),
    rows = makeResults(done, preview);
  const manifestInput = {
      project_id: ProjectId,
      task_id: done.task_id,
      expected_manifest_id: null,
    },
    manifest = makeManifest(done, rows, manifestInput),
    receipt = makeReceipt(manifest);
  const publish = {
      project_id: ProjectId,
      preview_id: manifest.preview_id,
      manifest_sha256: manifest.manifest_sha256,
      idempotency_key: receipt.idempotency_key,
    },
    pending = manifestPending(DomainId, publish, null, done.task_id);
  const stamp = times(),
    taskPage = {
      domain_id: DomainId,
      registration_id: Reg,
      list_snapshot_id: Id(80),
      total_tasks: 1,
      captured_at: stamp.created_at,
      expires_at: stamp.expires_at,
      items: [task],
      next_cursor: null,
    };
  const items = {
    task_id: task.task_id,
    registration_id: Reg,
    result_revision: task.revision,
    status: task.status,
    counts: task.counts,
    items: makeResults(task, preview),
    next_cursor: null,
  };
  const history = {
    domain_id: DomainId,
    project_id: ProjectId,
    list_snapshot_id: Id(81),
    total_manifests: 1,
    captured_at: stamp.created_at,
    expires_at: stamp.expires_at,
    items: [receipt],
    next_cursor: null,
  };
  const { idempotency_key: _key, ...version } = receipt;
  void _key;
  const input = {
    observation_id: observation.observation_id,
    observation_sha256: observation.observation_sha256,
    selected_items: preview.items.map((i) => ({
      relative_path: i.relative_path,
      classification: i.classification,
    })),
  };
  const cases = [
    [
      () => service.contract(),
      Capabilities,
      200,
      '/api/v1/knowledge/batch-import-contract',
      'get',
    ],
    [
      () => service.prepare(DomainId, Reg, input),
      preview,
      201,
      R + '/batch-previews',
      'post',
    ],
    [
      () => service.preview(DomainId, Reg, preview.preview_id),
      preview,
      200,
      R + '/batch-previews/' + preview.preview_id,
      'get',
    ],
    [
      () =>
        service.content(
          DomainId,
          Reg,
          preview.preview_id,
          preview.items[0].item_id,
          0,
          65536,
        ),
      content(preview, preview.items[0].item_id),
      200,
      R +
        '/batch-previews/' +
        preview.preview_id +
        '/items/' +
        preview.items[0].item_id +
        '/content',
      'get',
    ],
    [
      () => service.accept(DomainId, Reg, accept),
      task,
      202,
      R + '/batch-tasks',
      'post',
    ],
    [
      () => service.tasks(DomainId, Reg),
      taskPage,
      200,
      R + '/batch-tasks',
      'get',
    ],
    [
      () => service.task(DomainId, Reg, task.task_id),
      task,
      200,
      R + '/batch-tasks/' + task.task_id,
      'get',
    ],
    [
      () => service.items(DomainId, Reg, task.task_id),
      items,
      200,
      R + '/batch-tasks/' + task.task_id + '/items',
      'get',
    ],
    [
      () => service.byKey(DomainId, Reg, accept),
      task,
      200,
      R + '/batch-tasks/by-key/' + accept.idempotency_key,
      'get',
    ],
    [
      () =>
        service.cancel(DomainId, Reg, task.task_id, {
          idempotency_key: 'cancel-original',
        }),
      makeTask(preview, 'cancelling'),
      200,
      R + '/batch-tasks/' + task.task_id + '/cancellation',
      'post',
    ],
    [
      () => service.prepareManifest(DomainId, manifestInput),
      manifest,
      201,
      Base + '/project-manifest-previews',
      'post',
    ],
    [
      () => service.manifestPreview(DomainId, manifest.preview_id),
      manifest,
      200,
      Base + '/project-manifest-previews/' + manifest.preview_id,
      'get',
    ],
    [
      () => service.publish(DomainId, publish),
      receipt,
      201,
      Base + '/project-manifest-publications',
      'post',
    ],
    [
      () => service.publicationByKey(DomainId, pending),
      receipt,
      200,
      Base +
        '/projects/' +
        ProjectId +
        '/manifest-publications/by-key/' +
        publish.idempotency_key,
      'get',
    ],
    [
      () => service.head(DomainId, ProjectId),
      { domain_id: DomainId, project_id: ProjectId, manifest_id: null },
      200,
      Base + '/projects/' + ProjectId + '/manifest-head',
      'get',
    ],
    [
      () => service.history(DomainId, ProjectId),
      history,
      200,
      Base + '/projects/' + ProjectId + '/manifests',
      'get',
    ],
    [
      () => service.manifest(DomainId, ProjectId, receipt.manifest_id),
      { ...version, items: manifest.items, next_cursor: null },
      200,
      Base + '/projects/' + ProjectId + '/manifests/' + receipt.manifest_id,
      'get',
    ],
  ] as const;
  expect(Object.keys(service)).toHaveLength(17);
  for (const [call, data, status, url, method] of cases) {
    Http.mockResolvedValueOnce(respond(data, status));
    const result = await call();
    expect(result.httpStatus).toBe(status);
    const wire = Http.mock.calls.at(-1)![0];
    expect(wire.url).toBe(url);
    expect(wire.method).toBe(method);
    expect(wire.headers).toEqual({
      'X-Knowledge-Contract-Version': '1.0.0',
      'X-Knowledge-Source-Contract-Version': '0.4.0',
      ...(method === 'post' ? { 'Content-Type': 'application/json' } : {}),
    });
    if (method === 'post') expect(typeof wire.data).toBe('string');
    else expect(wire.data).toBeUndefined();
  }
});
it('keeps acceptance and publication replay as HTTP200 with exact original request identity', async () => {
  const preview = makePreview(),
    request = acceptRequest(preview),
    task = makeTask(preview);
  Http.mockResolvedValueOnce(respond(task));
  expect((await service.accept(DomainId, Reg, request)).httpStatus).toBe(200);
  Http.mockResolvedValueOnce(
    respond({
      ...task,
      confirmed_item_ids: [...task.confirmed_item_ids].reverse(),
    }),
  );
  await expect(service.byKey(DomainId, Reg, request)).rejects.toMatchObject({
    status: 502,
  });
  Http.mockResolvedValueOnce(respond({ ...task, retry_of_task_id: Id(99) }));
  await expect(service.byKey(DomainId, Reg, request)).rejects.toMatchObject({
    status: 502,
  });
});
it.each([
  [400, 'knowledge_batch_invalid', null],
  [404, 'knowledge_batch_not_found', null],
  [409, 'knowledge_batch_version_mismatch', null],
  [409, 'knowledge_batch_conflict', 'result_changed'],
  [409, 'knowledge_batch_conflict', 'project_head_changed'],
  [409, 'knowledge_batch_conflict', 'retry_parent_invalid'],
  [410, 'knowledge_batch_expired', null],
  [413, 'knowledge_batch_too_large', null],
  [416, 'knowledge_batch_range', null],
  [429, 'knowledge_batch_capacity', null],
  [503, 'knowledge_batch_unavailable', 'root_unready'],
  [503, 'knowledge_batch_unavailable', 'result_unknown'],
])('retains exact HTTP/code/reason %s/%s/%s', async (status, code, reason) => {
  Http.mockRejectedValueOnce(
    reject(status as number, code as string, reason as string | null),
  );
  await expect(service.task(DomainId, Reg, Id(9))).rejects.toMatchObject({
    status,
    code,
    reason,
  });
  expect(Http).toHaveBeenCalledTimes(1);
});
it('stops wrong versions, lifecycle, model policies, HTTP status, scopes and malformed error combinations', async () => {
  Http.mockResolvedValueOnce({
    ...respond(Capabilities),
    data: { ...respond(Capabilities).data, source_contract_version: '0.3.0' },
  });
  await expect(service.contract()).rejects.toMatchObject({
    status: 409,
    code: 'knowledge_batch_version_mismatch',
  });
  for (const changed of [
    { status: 'ReviewCandidate', implemented: false },
    { model_calls: 1 },
    { external_models: true },
  ]) {
    Http.mockResolvedValueOnce(
      respond({ ...(Capabilities as object), ...changed }),
    );
    await expect(service.contract()).rejects.toMatchObject({ status: 502 });
  }
  Http.mockResolvedValueOnce(respond(makeTask(), 201));
  await expect(service.task(DomainId, Reg, Id(9))).rejects.toMatchObject({
    status: 502,
  });
  Http.mockResolvedValueOnce(respond({ ...makeTask(), domain_id: Id(99) }));
  await expect(service.task(DomainId, Reg, Id(9))).rejects.toMatchObject({
    status: 502,
  });
  Http.mockRejectedValueOnce(
    reject(404, 'knowledge_batch_conflict', 'source_changed'),
  );
  await expect(service.contract()).rejects.toMatchObject({ status: 502 });
  Http.mockRejectedValueOnce(
    reject(410, 'knowledge_batch_expired', 'result_unknown'),
  );
  await expect(service.contract()).rejects.toMatchObject({ status: 502 });
});
it('consumes native unauthorized and transport unknown without interpreting 404 as non-submission', async () => {
  Http.mockRejectedValueOnce({
    response: { status: 401, data: { code: 401, message: 'Synthetic login' } },
  });
  await expect(service.contract()).rejects.toMatchObject({ status: 401 });
  Http.mockRejectedValueOnce(new Error('Synthetic lost response'));
  await expect(service.contract()).rejects.toMatchObject({
    status: 0,
    reason: 'result_unknown',
  });
});
it('uses bounded GET-only UTF-8 boundary retries and enforces input limits before transport', async () => {
  const preview = makePreview(),
    item = preview.items[0].item_id;
  Http.mockRejectedValueOnce(reject(416, 'knowledge_batch_range'))
    .mockRejectedValueOnce(reject(416, 'knowledge_batch_range'))
    .mockRejectedValueOnce(reject(416, 'knowledge_batch_range'))
    .mockResolvedValueOnce(respond(content(preview, item, 0, 3)));
  await service.content(DomainId, Reg, preview.preview_id, item, 0, 6);
  expect(Http.mock.calls.map(([c]) => c.params.length)).toEqual([6, 5, 4, 3]);
  expect(Http.mock.calls.every(([c]) => c.method === 'get')).toBe(true);
  Http.mockReset().mockRejectedValue(reject(416, 'knowledge_batch_range'));
  await expect(
    service.content(DomainId, Reg, preview.preview_id, item, 0, 65536),
  ).rejects.toMatchObject({ status: 416 });
  expect(Http).toHaveBeenCalledTimes(4);
  Http.mockReset();
  await expect(
    service.content(DomainId, Reg, preview.preview_id, item, 0, 65537),
  ).rejects.toMatchObject({ status: 400 });
  await expect(
    service.prepare(DomainId, Reg, {
      observation_id: Id(4),
      observation_sha256: 'a'.repeat(64),
      selected_items: [],
    }),
  ).rejects.toMatchObject({ status: 400 });
  const selected_items = Array.from({ length: 32 }, (_, i) => ({
    relative_path: '"'.repeat(510) + String(i).padStart(2, '0'),
    classification: preview.items[0].classification,
  }));
  await expect(
    service.prepare(DomainId, Reg, {
      observation_id: Id(4),
      observation_sha256: 'a'.repeat(64),
      selected_items,
    }),
  ).rejects.toMatchObject({ status: 413 });
  expect(Http).not.toHaveBeenCalled();
});
it('checks original publication task and explicit base after lost response, without fetching a new head or posting', async () => {
  const preview = makePreview(),
    task = makeTask(preview, 'succeeded'),
    manifest = makeManifest(task, makeResults(task, preview), {
      project_id: ProjectId,
      task_id: task.task_id,
      expected_manifest_id: null,
    }),
    receipt = makeReceipt(manifest);
  const pending = manifestPending(
    DomainId,
    {
      project_id: ProjectId,
      preview_id: manifest.preview_id,
      manifest_sha256: manifest.manifest_sha256,
      idempotency_key: receipt.idempotency_key,
    },
    null,
    task.task_id,
  );
  Http.mockResolvedValueOnce(respond(receipt));
  await expect(
    service.publicationByKey(DomainId, pending),
  ).resolves.toMatchObject({ data: receipt });
  Http.mockResolvedValueOnce(
    respond({ ...receipt, expected_manifest_id: Id(88) }),
  );
  await expect(
    service.publicationByKey(DomainId, pending),
  ).rejects.toMatchObject({ status: 502 });
  Http.mockResolvedValueOnce(respond({ ...receipt, task_id: Id(87) }));
  await expect(
    service.publicationByKey(DomainId, pending),
  ).rejects.toMatchObject({ status: 502 });
  expect(
    Http.mock.calls.every(
      ([c]) => c.method === 'get' && !c.url.endsWith('/manifest-head'),
    ),
  ).toBe(true);
});

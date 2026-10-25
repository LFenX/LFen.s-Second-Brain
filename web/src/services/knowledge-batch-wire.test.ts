import request from '@/utils/next-request';
import service from './knowledge-batch-service';
import {
  Registration,
  DomainId,
  ProjectId,
  makeObservation,
  makePreview,
  makeTask,
  makeResults,
  makeManifest,
  makeReceipt,
  acceptRequest,
  respond,
} from '@/pages/dataset/knowledge-import/__tests__/batch-fixture';
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
it('preserves batch and publication JSON through real Axios interceptors with isolated version headers', async () => {
  const observation = makeObservation(),
    preview = makePreview(observation),
    input = {
      observation_id: observation.observation_id,
      observation_sha256: observation.observation_sha256,
      selected_items: preview.items.map((i) => ({
        relative_path: i.relative_path,
        classification: i.classification,
      })),
    },
    accept = acceptRequest(preview);
  const task = makeTask(preview, 'succeeded', accept),
    manifest = makeManifest(task, makeResults(task, preview), {
      project_id: ProjectId,
      task_id: task.task_id,
      expected_manifest_id: null,
    }),
    receipt = makeReceipt(manifest);
  const publish = {
    project_id: ProjectId,
    preview_id: manifest.preview_id,
    manifest_sha256: manifest.manifest_sha256,
    idempotency_key: receipt.idempotency_key,
  };
  const adapter = jest.fn(async (config) => ({
    ...respond(
      config.url.endsWith('/batch-previews')
        ? preview
        : config.url.endsWith('/batch-tasks')
          ? task
          : receipt,
      config.url.endsWith('/batch-tasks') ? 202 : 201,
    ),
    config,
    statusText: 'Synthetic',
    headers: {},
  }));
  const original = request.defaults.adapter;
  request.defaults.adapter = adapter;
  localStorage.setItem('Authorization', 'synthetic-batch-wire-token');
  try {
    await service.prepare(DomainId, Registration.registration_id, input);
    await service.accept(DomainId, Registration.registration_id, accept);
    await service.publish(DomainId, publish);
    expect(adapter).toHaveBeenCalledTimes(3);
    expect(adapter.mock.calls.map(([c]) => JSON.parse(c.data))).toEqual([
      input,
      accept,
      publish,
    ]);
    for (const [c] of adapter.mock.calls) {
      expect(c.headers.get('Content-Type')).toBe('application/json');
      expect(c.headers.get('X-Knowledge-Contract-Version')).toBe('1.0.0');
      expect(c.headers.get('X-Knowledge-Source-Contract-Version')).toBe(
        '0.4.0',
      );
      expect(c.headers.get('Authorization')).toBe('synthetic-batch-wire-token');
    }
  } finally {
    request.defaults.adapter = original;
    localStorage.removeItem('Authorization');
  }
});

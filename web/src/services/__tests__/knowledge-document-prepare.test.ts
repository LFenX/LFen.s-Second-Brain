import service from '../knowledge-document-service';
import foundation from '../knowledge-foundation-service';
import originals from '../knowledge-original-service';
import lifecycle from '../knowledge-lifecycle-service';
import {
  Scope,
  Rep,
  Caps,
  Policy,
  clone,
} from '@/pages/dataset/knowledge-catalogue/__tests__/document-fixture';
jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
const Original = {
  ...Rep.original,
  relative_path: Rep.source_snapshot.relative_path,
  classification: Rep.source_snapshot.classification,
  is_current: true,
  created_at: Rep.published_at,
  parse_status: 'not_parsed',
  structure_status: 'not_built',
  full_text_status: 'not_built',
  vector_status: 'not_built',
} as import('@/interfaces/database/knowledge-original').OriginalRevision;
const Source = { ...Scope, source: Original.source_id };
function setup() {
  const domain = {
    owner_id: Scope.owner,
    domain_id: Scope.domain,
    policy: {
      version: Policy.foundation_policy_version,
      external_models: false,
      automatic_ingestion: false,
      allowed_model_uses: [],
      index_modes: ['catalogue'],
    },
  };
  const current = {
    source_revision_id: Original.source_revision_id,
    sha256: Original.sha256,
    byte_length: Original.byte_length,
    relative_path: Original.relative_path,
    classification: Original.classification,
    state: 'active',
    lifecycle_version: Rep.source_snapshot.lifecycle_version,
  };
  const detail = {
    owner_id: Scope.owner,
    domain_id: Scope.domain,
    source_id: Original.source_id,
    representation_kind: 'original_only',
    current,
  };
  const readyCaps = clone(Caps);
  readyCaps.runtime_ready = true;
  readyCaps.profiles.forEach((profile) => {
    profile.runtime_ready = true;
  });
  const head = {
    domain_id: Scope.domain,
    source_id: Original.source_id,
    currentness: 'none',
    active_representation_id: null,
    active_source_revision_id: null,
    current_source_revision_id: Original.source_revision_id,
    current_lifecycle_version: current.lifecycle_version,
    foundation_policy_version: Policy.foundation_policy_version,
    document_policy_version: Policy.document_policy_version,
    latest_task_id: null,
    reasons: [],
    checked_at: Rep.published_at,
  };
  jest.spyOn(foundation, 'domain').mockResolvedValue(domain as any);
  jest
    .spyOn(originals, 'revision')
    .mockResolvedValue({ data: clone(Original), httpStatus: 200 });
  jest
    .spyOn(lifecycle, 'detail')
    .mockResolvedValue({ data: detail as any, httpStatus: 200 });
  jest
    .spyOn(service, 'policy')
    .mockResolvedValue({ data: { ...Policy, enabled: true }, httpStatus: 200 });
  jest
    .spyOn(service, 'contract')
    .mockResolvedValue({ data: readyCaps, httpStatus: 200 });
  jest
    .spyOn(service, 'head')
    .mockResolvedValue({ data: head as any, httpStatus: 200 });
  return { domain, detail, head };
}
afterEach(() => jest.restoreAllMocks());
it('prepares one exact active fixed original with independent document policy and actual producer', async () => {
  const { detail } = setup();
  const result = await service.prepare(Source, Original);
  expect(result.request.target).toEqual({
    kind: 'original_document',
    source_id: Original.source_id,
    source_revision_id: Original.source_revision_id,
    original_sha256: Original.sha256,
    expected_lifecycle_version: detail.current.lifecycle_version,
  });
  expect(result.request.profile_id).toBe('docx-ooxml-blocks/1');
  expect(result.request.expected_producer_sha256).toBe(
    Caps.profiles.find((p) => p.format === 'docx')!.producer_sha256,
  );
  expect(result.request).not.toHaveProperty('project_id');
  expect(result.request).not.toHaveProperty('confirmed');
  expect(lifecycle.detail).toHaveBeenCalledWith(Source, undefined);
  expect(originals.revision).toHaveBeenCalledWith(
    Scope.domain,
    Original.source_revision_id,
    undefined,
  );
});
it.each([
  'state',
  'source_revision_id',
  'sha256',
  'byte_length',
  'relative_path',
  'classification',
] as const)('stops preparation when active %s differs', async (field) => {
  const { detail } = setup();
  (detail.current as any)[field] =
    field === 'state'
      ? 'archived'
      : field === 'byte_length'
        ? Original.byte_length + 1
        : field === 'classification'
          ? { ...Original.classification, language: 'changed' }
          : 'f'.repeat(32);
  await expect(service.prepare(Source, Original)).rejects.toMatchObject({
    status: 409,
  });
});
it('rejects stale selected .5 metadata rather than silently replacing it', async () => {
  setup();
  (originals.revision as jest.Mock).mockResolvedValue({
    data: { ...Original, original_filename: 'other.docx' },
  });
  await expect(service.prepare(Source, Original)).rejects.toMatchObject({
    status: 409,
  });
});
it('does not require code permission to be enabled', async () => {
  const { domain } = setup();
  (domain.policy as any).structure_enabled = false;
  expect(
    (await service.prepare(Source, Original)).request
      .expected_document_policy_version,
  ).toBe(Policy.document_policy_version);
});
it('blocks when independent document permission is disabled', async () => {
  setup();
  (service.policy as jest.Mock).mockResolvedValue({
    data: { ...Policy, enabled: false },
  });
  await expect(service.prepare(Source, Original)).rejects.toMatchObject({
    code: 'document_policy_disabled',
  });
});
it.each([
  'foundation_policy_version',
  'document_policy_version',
  'current_lifecycle_version',
  'current_source_revision_id',
] as const)('rejects changed head %s', async (field) => {
  const { head } = setup();
  (head as any)[field] =
    typeof head[field] === 'number' ? Number(head[field]) + 1 : 'f'.repeat(32);
  await expect(service.prepare(Source, Original)).rejects.toMatchObject({
    status: 409,
  });
});
it('rejects an unready actual profile instead of guessing a producer', async () => {
  setup();
  const caps = clone(Caps);
  caps.profiles.find((p) => p.format === 'docx')!.runtime_ready = false;
  (service.contract as jest.Mock).mockResolvedValue({ data: caps });
  await expect(service.prepare(Source, Original)).rejects.toMatchObject({
    status: 503,
  });
});
it('rejects a foreign owner before preparing', async () => {
  const { domain } = setup();
  domain.owner_id = 'f'.repeat(32);
  await expect(service.prepare(Source, Original)).rejects.toMatchObject({
    status: 401,
  });
});
it('preserves8MiB processing and the existing64MiB original save boundary', async () => {
  setup();
  await expect(
    service.prepare(Source, { ...Original, byte_length: 8388609 }),
  ).rejects.toMatchObject({ status: 400 });
  expect(originals.revision).not.toHaveBeenCalled();
});
it('requires current original and never automatically changes revision', async () => {
  setup();
  await expect(
    service.prepare(Source, { ...Original, is_current: false }),
  ).rejects.toMatchObject({ status: 400 });
  expect(originals.revision).not.toHaveBeenCalled();
});

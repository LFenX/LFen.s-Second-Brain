import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import service, {
  BatchRequestError,
  type BatchResponse,
} from '@/services/knowledge-batch-service';
import directoryService from '@/services/knowledge-directory-service';
import {
  useBatchActions,
  batchScopeFailure,
  KnowledgeBatchKeys,
} from '@/hooks/use-knowledge-batch';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import type {
  DirectoryRegistration,
  DirectoryObservation,
} from '@/interfaces/database/knowledge-directory';
import type { DirectoryRegistrationRequest } from '@/interfaces/request/knowledge-directory';
import type {
  BatchPreview,
  BatchTask,
  BatchTaskItem,
  BatchManifestEntry,
  BatchManifestPreviewPage,
  BatchManifestReceipt,
} from '@/interfaces/database/knowledge-batch';
import type { BatchPreviewRequest } from '@/interfaces/request/knowledge-batch';
import {
  batchPending,
  manifestPending,
  loadBatchRecovery,
  saveBatchRecovery,
  clearBatchRecovery,
  batchTaskMatches,
  batchPreviewMatches,
  batchWindowMatches,
  batchReceiptMatches,
  appendBatchResultPage,
  appendBatchManifestPage,
  batchRetryPaths,
  batchProjectMatches,
  type BatchPending,
  type ManifestPending,
} from '@/utils/knowledge-batch';
import {
  directoryObservationMatches,
  sameDirectoryClassification,
} from '@/utils/knowledge-directory';

export type BatchLocation = {
  registration?: string | null;
  task?: string | null;
  project?: string | null;
  manifest?: string | null;
  batch_tab?: 'prepare' | 'tasks' | 'projects';
};
type Request = <T>(
  work: () => Promise<BatchResponse<T>>,
) => Promise<BatchResponse<T>>;
const invalid = () =>
  new BatchRequestError(502, 'knowledge_batch_invalid_response');
export async function readBatchResults(
  domain: string,
  task: BatchTask,
  request: Request,
) {
  let pages: import('@/interfaces/database/knowledge-batch').BatchTaskItemsPage[] =
    [];
  const seen = new Set<string>();
  let cursor: string | undefined;
  for (let n = 0; n < 4; n += 1) {
    const result = await request(() =>
      service.items(domain, task.registration_id, task.task_id, cursor),
    );
    if (
      result.data.result_revision !== task.revision ||
      result.data.status !== task.status
    )
      throw invalid();
    pages = appendBatchResultPage(pages, result.data, task);
    if (!result.data.next_cursor) return pages.flatMap((p) => p.items);
    if (seen.has(result.data.next_cursor)) throw invalid();
    seen.add(result.data.next_cursor);
    cursor = result.data.next_cursor;
  }
  throw invalid();
}
async function readBaseManifest(
  domain: string,
  project: string,
  manifest: string,
  request: Request,
) {
  let pages: import('@/interfaces/database/knowledge-batch').BatchManifestVersionPage[] =
    [];
  const seen = new Set<string>();
  let cursor: string | undefined;
  for (let n = 0; n < 21; n += 1) {
    const result = await request(() =>
      service.manifest(domain, project, manifest, cursor),
    );
    pages = appendBatchManifestPage(pages, result.data);
    if (!result.data.next_cursor) return pages.flatMap((p) => p.items);
    if (seen.has(result.data.next_cursor)) throw invalid();
    seen.add(result.data.next_cursor);
    cursor = result.data.next_cursor;
  }
  throw invalid();
}
function useBatchWork(domain: string, onFailure?: (error: unknown) => void) {
  const actions = useBatchActions(domain),
    Actions = useRef(actions),
    Alive = useRef(true),
    Busy = useRef(false);
  Actions.current = actions;
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [blocked, setBlocked] = useState(false);
  useEffect(() => {
    Alive.current = true;
    return () => {
      Alive.current = false;
    };
  }, []);
  function fail(value: unknown) {
    if (!Alive.current) return;
    setError(value);
    if (batchScopeFailure(value)) {
      setBlocked(true);
      onFailure?.(value);
    }
  }
  async function request<T>(work: () => Promise<BatchResponse<T>>) {
    if (!Alive.current) throw Error('Inactive view');
    const value = await Actions.current.run(work);
    if (!Alive.current) throw Error('Inactive view');
    return value;
  }
  async function run<T>(work: () => Promise<T>): Promise<T | undefined> {
    if (Busy.current) return undefined;
    Busy.current = true;
    setBusy(true);
    setError(null);
    try {
      return await work();
    } catch (value) {
      fail(value);
      return undefined;
    } finally {
      Busy.current = false;
      if (Alive.current) setBusy(false);
    }
  }
  function reauthorize() {
    setBlocked(false);
    setError(null);
    Actions.current.reset();
  }
  return {
    busy,
    error,
    blocked,
    request,
    run,
    fail,
    reauthorize,
    clearError: () => setError(null),
  };
}
export function useBatchFlow(
  domain: string,
  key: string,
  initialRegistration: string,
  onLocation: (value: BatchLocation) => void,
  onAuthorizationFailure: (error: unknown) => void,
) {
  const client = useQueryClient();
  const [registration, setRegistration] =
      useState<DirectoryRegistration | null>(null),
    [observation, setObservation] = useState<DirectoryObservation | null>(null),
    [preview, setPreview] = useState<BatchPreview | null>(null);
  const [pending, setPending] = useState<BatchPending | null>(() =>
      loadBatchRecovery(key, 'task', domain),
    ),
    [task, setTask] = useState<BatchTask | null>(null),
    [canRestart, setCanRestart] = useState(false);
  const [attempt, setAttempt] = useState<DirectoryRegistrationRequest | null>(
      null,
    ),
    [retryParent, setRetryParent] = useState<{
      task: BatchTask;
      items: BatchTaskItem[];
    } | null>(null);
  const work = useBatchWork(domain, hidePrivate);
  function hidePrivate(error: unknown) {
    setRegistration(null);
    setObservation(null);
    setPreview(null);
    setTask(null);
    setAttempt(null);
    setRetryParent(null);
    client.removeQueries({
      queryKey: KnowledgeBatchKeys.registration(
        getKnowledgeSessionScope(),
        domain,
        pending?.registration_id || initialRegistration,
      ),
    });
    if (
      error instanceof BatchRequestError &&
      (error.status === 401 ||
        error.code === 'knowledge_batch_version_mismatch')
    )
      onAuthorizationFailure(error);
  }
  function requireResolved() {
    if (pending && !pending.task_id && !canRestart)
      throw new BatchRequestError(
        0,
        'knowledge_batch_unavailable',
        undefined,
        'result_unknown',
      );
  }
  async function restore(reg: string) {
    const result = await work.request(() =>
      directoryService.registration(domain, reg),
    );
    if (
      registration?.registration_id === reg &&
      (registration.root_id !== result.data.root_id ||
        registration.registration_key !== result.data.registration_key ||
        !sameDirectoryClassification(
          registration.default_classification,
          result.data.default_classification,
        ) ||
        JSON.stringify([...registration.scope_directories].sort()) !==
          JSON.stringify([...result.data.scope_directories].sort()))
    )
      throw invalid();
    return result.data;
  }
  function acceptTask(value: BatchTask, original: BatchPending) {
    if (
      !batchTaskMatches(value, original.request) ||
      value.domain_id !== domain ||
      value.registration_id !== original.registration_id ||
      (original.task_id !== null && value.task_id !== original.task_id)
    )
      throw invalid();
    const updated = { ...original, task_id: value.task_id };
    saveBatchRecovery(key, 'task', updated);
    void client.invalidateQueries({
      queryKey: KnowledgeBatchKeys.tasks(
        getKnowledgeSessionScope(),
        domain,
        original.registration_id,
      ),
    });
    setPending(updated);
    setTask(value);
    setPreview(null);
    setObservation(null);
    setCanRestart(false);
    onLocation({
      registration: value.registration_id,
      task: value.task_id,
      batch_tab: 'tasks',
    });
  }
  async function postOriginal(original: BatchPending) {
    try {
      acceptTask(
        (
          await work.request(() =>
            service.accept(domain, original.registration_id, original.request),
          )
        ).data,
        original,
      );
    } catch (error) {
      if (
        error instanceof BatchRequestError &&
        (error.status === 410 ||
          (error.status === 409 &&
            [
              'source_changed',
              'revision_changed',
              'root_changed',
              'policy_changed',
              'observation_mismatch',
              'observation_incomplete',
              'ineligible',
              'retry_parent_invalid',
            ].includes(error.reason || '')))
      )
        setCanRestart(true);
      throw error;
    }
  }
  const register = (request: DirectoryRegistrationRequest) =>
    work.run(async () => {
      requireResolved();
      setAttempt(request);
      const result = await work.request(() =>
        directoryService.register(domain, request),
      );
      setRegistration(result.data);
      setAttempt(null);
      setObservation(null);
      setPreview(null);
      onLocation({
        registration: result.data.registration_id,
        task: null,
        batch_tab: 'prepare',
      });
    });
  const openRegistration = (reg: string) =>
    work.run(async () => {
      requireResolved();
      const value = await restore(reg);
      setRegistration(value);
      setObservation(null);
      setPreview(null);
      setRetryParent(null);
      onLocation({ registration: reg, task: null, batch_tab: 'prepare' });
      return true;
    });
  const startNew = () =>
    work.run(async () => {
      requireResolved();
      const reg =
        registration?.registration_id ||
        pending?.registration_id ||
        initialRegistration;
      if (!reg) return false;
      const value = await restore(reg);
      clearBatchRecovery(key, 'task');
      setPending(null);
      setTask(null);
      setRegistration(value);
      setObservation(null);
      setPreview(null);
      setRetryParent(null);
      setCanRestart(false);
      onLocation({ registration: reg, task: null, batch_tab: 'prepare' });
      return true;
    });
  const observe = (paths: string[]) =>
    work.run(async () => {
      if (!registration || pending) return;
      if (retryParent) {
        const all = retryParent.items.map((i) => i.relative_path),
          unfinished = retryParent.items
            .filter((i) => i.status !== 'succeeded')
            .map((i) => i.relative_path);
        const complete =
          paths.length === all.length && paths.every((p) => all.includes(p));
        if (
          !paths.length ||
          (!complete && !paths.every((p) => unfinished.includes(p)))
        )
          throw new BatchRequestError(
            409,
            'knowledge_batch_conflict',
            undefined,
            'retry_parent_invalid',
          );
      }
      setObservation(null);
      setPreview(null);
      const result = await work.request(() =>
        directoryService.observe(domain, registration.registration_id, {
          selected_paths: paths,
        }),
      );
      if (
        !directoryObservationMatches(result.data, paths) ||
        result.data.root_id !== registration.root_id
      )
        throw invalid();
      setObservation(result.data);
    });
  const prepare = (request: BatchPreviewRequest) =>
    work.run(async () => {
      if (!registration || !observation || pending) return;
      setPreview(null);
      const result = await work.request(() =>
        service.prepare(domain, registration.registration_id, request),
      );
      if (!batchPreviewMatches(result.data, request, observation))
        throw invalid();
      setPreview(result.data);
    });
  const readContent = (itemId: string, offset: number) =>
    work.run(async () => {
      if (!registration || !preview || pending) return;
      const item = preview.items.find((i) => i.item_id === itemId);
      if (!item) throw invalid();
      const result = await work.request(() =>
        service.content(
          domain,
          registration.registration_id,
          preview.preview_id,
          itemId,
          offset,
          Math.max(1, Math.min(65536, item.byte_length - offset)),
        ),
      );
      if (!batchWindowMatches(result.data, preview, itemId, offset))
        throw invalid();
      return result.data;
    });
  const accept = (confirmed: string[]) =>
    work.run(async () => {
      if (!preview || !registration || pending) return;
      if (!dayjs(preview.expires_at).isAfter(dayjs()))
        throw new BatchRequestError(410, 'knowledge_batch_expired');
      if (
        JSON.stringify(confirmed) !==
        JSON.stringify(preview.items.map((i) => i.item_id))
      )
        throw new BatchRequestError(
          409,
          'knowledge_batch_conflict',
          undefined,
          'confirmation_mismatch',
        );
      const original = batchPending(domain, registration.registration_id, {
        preview_id: preview.preview_id,
        manifest_sha256: preview.manifest_sha256,
        idempotency_key: crypto.randomUUID(),
        confirmed_item_ids: confirmed,
        retry_of_task_id: retryParent?.task.task_id || null,
      });
      saveBatchRecovery(key, 'task', original);
      setPending(original);
      setPreview(null);
      setObservation(null);
      onLocation({
        registration: original.registration_id,
        task: null,
        batch_tab: 'tasks',
      });
      await postOriginal(original);
    });
  const recover = () =>
    work.run(async () => {
      if (pending)
        acceptTask(
          (
            await work.request(() =>
              service.byKey(domain, pending.registration_id, pending.request),
            )
          ).data,
          pending,
        );
    });
  const resend = () =>
    work.run(async () => {
      if (pending) {
        setCanRestart(false);
        await postOriginal(pending);
      }
    });
  function rememberTask(value: BatchTask) {
    setTask(value);
    if (
      pending &&
      batchTaskMatches(value, pending.request) &&
      pending.registration_id === value.registration_id &&
      value.domain_id === domain &&
      pending.task_id === null
    ) {
      const updated = { ...pending, task_id: value.task_id };
      saveBatchRecovery(key, 'task', updated);
      setPending(updated);
    }
  }
  const retry = (parent: BatchTask, full: boolean) =>
    work.run(async () => {
      requireResolved();
      if (parent.domain_id !== domain) throw invalid();
      const rows = await readBatchResults(domain, parent, work.request),
        paths = batchRetryPaths(parent, rows, full),
        reg = await restore(parent.registration_id);
      const result = await work.request(() =>
        directoryService.observe(domain, reg.registration_id, {
          selected_paths: paths,
        }),
      );
      if (
        !directoryObservationMatches(result.data, paths) ||
        result.data.root_id !== reg.root_id
      )
        throw invalid();
      clearBatchRecovery(key, 'task');
      setPending(null);
      setTask(null);
      setPreview(null);
      setRegistration(reg);
      setObservation(result.data);
      setRetryParent({ task: parent, items: rows });
      setCanRestart(false);
      onLocation({
        registration: reg.registration_id,
        task: null,
        batch_tab: 'prepare',
      });
    });
  function invalidate() {
    if (!pending) setPreview(null);
  }
  function changeSelection() {
    if (!pending) {
      setPreview(null);
      setObservation(null);
    }
  }
  return {
    ...work,
    registration,
    observation,
    preview,
    pending,
    task,
    attempt,
    retryParent,
    canRestart,
    register,
    openRegistration,
    startNew,
    observe,
    prepare,
    readContent,
    accept,
    recover,
    resend,
    retry,
    rememberTask,
    invalidate,
    changeSelection,
  };
}
export function useBatchManifestFlow(
  domain: string,
  key: string,
  onLocation: (value: BatchLocation) => void,
  onAuthorizationFailure: (error: unknown) => void,
) {
  const client = useQueryClient();
  const [pending, setPending] = useState<ManifestPending | null>(() =>
      loadBatchRecovery(key, 'publication', domain),
    ),
    [receipt, setReceipt] = useState<BatchManifestReceipt | null>(null);
  const [pages, setPages] = useState<BatchManifestPreviewPage[]>([]),
    [canPrepare, setCanPrepare] = useState(false);
  const [context, setContext] = useState<{
    task: BatchTask;
    results: BatchTaskItem[];
    base: BatchManifestEntry[];
  } | null>(null);
  const work = useBatchWork(domain, hidePrivate);
  function hidePrivate(error: unknown) {
    setPages([]);
    setContext(null);
    setReceipt(null);
    if (
      error instanceof BatchRequestError &&
      (error.status === 401 ||
        error.code === 'knowledge_batch_version_mismatch')
    )
      onAuthorizationFailure(error);
  }
  const prepare = (project: string, reg: string, taskId: string) =>
    work.run(async () => {
      if (pending && !receipt && !canPrepare)
        throw new BatchRequestError(
          0,
          'knowledge_batch_unavailable',
          undefined,
          'result_unknown',
        );
      const task = (await work.request(() => service.task(domain, reg, taskId)))
        .data;
      if (task.status !== 'succeeded')
        throw new BatchRequestError(
          409,
          'knowledge_batch_conflict',
          undefined,
          'task_not_succeeded',
        );
      const results = await readBatchResults(domain, task, work.request),
        head = (await work.request(() => service.head(domain, project))).data;
      const base = head.manifest_id
        ? await readBaseManifest(
            domain,
            project,
            head.manifest_id,
            work.request,
          )
        : [];
      const result = await work.request(() =>
        service.prepareManifest(domain, {
          project_id: project,
          task_id: taskId,
          expected_manifest_id: head.manifest_id,
        }),
      );
      if (result.data.task_manifest_sha256 !== task.manifest_sha256)
        throw invalid();
      const next = appendBatchManifestPage([], result.data);
      if (pending) clearBatchRecovery(key, 'publication');
      setPending(null);
      setReceipt(null);
      setPages(next);
      setContext({ task, results, base });
      setCanPrepare(false);
    });
  const more = () =>
    work.run(async () => {
      const first = pages[0],
        last = pages.at(-1);
      if (!first || !last?.next_cursor) return;
      const result = await work.request(() =>
        service.manifestPreview(domain, first.preview_id, last.next_cursor!),
      );
      setPages(appendBatchManifestPage(pages, result.data));
    });
  function acceptReceipt(
    value: BatchManifestReceipt,
    original: ManifestPending,
  ) {
    if (!batchReceiptMatches(value, original)) throw invalid();
    setReceipt(value);
    setPages([]);
    setContext(null);
    setCanPrepare(false);
    void client.invalidateQueries({
      queryKey: KnowledgeBatchKeys.head(
        getKnowledgeSessionScope(),
        domain,
        value.project_id,
      ),
    });
    void client.invalidateQueries({
      queryKey: KnowledgeBatchKeys.history(
        getKnowledgeSessionScope(),
        domain,
        value.project_id,
      ),
    });
    onLocation({
      project: value.project_id,
      manifest: value.manifest_id,
      batch_tab: 'projects',
    });
  }
  async function postOriginal(original: ManifestPending) {
    try {
      acceptReceipt(
        (await work.request(() => service.publish(domain, original.request)))
          .data,
        original,
      );
    } catch (error) {
      if (
        error instanceof BatchRequestError &&
        (error.status === 410 ||
          (error.status === 409 &&
            [
              'project_head_changed',
              'project_membership_changed',
              'task_not_succeeded',
            ].includes(error.reason || '')))
      )
        setCanPrepare(true);
      throw error;
    }
  }
  const publish = () =>
    work.run(async () => {
      const first = pages[0];
      if (!first || !context || pending) return;
      if (!dayjs(first.expires_at).isAfter(dayjs()))
        throw new BatchRequestError(410, 'knowledge_batch_expired');
      if (
        !batchProjectMatches(
          first,
          pages.flatMap((p) => p.items),
          context.task,
          context.results,
          context.base,
        )
      )
        throw invalid();
      const original = manifestPending(
        domain,
        {
          project_id: first.project_id,
          preview_id: first.preview_id,
          manifest_sha256: first.manifest_sha256,
          idempotency_key: crypto.randomUUID(),
        },
        first.expected_manifest_id,
        first.task_id,
      );
      saveBatchRecovery(key, 'publication', original);
      setPending(original);
      setPages([]);
      setContext(null);
      await postOriginal(original);
    });
  const recover = () =>
    work.run(async () => {
      if (pending)
        acceptReceipt(
          (await work.request(() => service.publicationByKey(domain, pending)))
            .data,
          pending,
        );
    });
  const resend = () =>
    work.run(async () => {
      if (pending) {
        setCanPrepare(false);
        await postOriginal(pending);
      }
    });
  function invalidate() {
    if (!pending) {
      setPages([]);
      setContext(null);
    }
  }
  return {
    ...work,
    pending,
    receipt,
    pages,
    canPrepare,
    prepare,
    more,
    publish,
    recover,
    resend,
    invalidate,
  };
}

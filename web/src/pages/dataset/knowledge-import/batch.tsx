import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import authorizationStorage, {
  getAuthorization,
} from '@/utils/authorization-util';
import {
  useKnowledgeDomain,
  getKnowledgeSessionScope,
} from '@/hooks/use-knowledge-foundation';
import { useBatchContract } from '@/hooks/use-knowledge-batch';
import { batchStorageKey } from '@/utils/knowledge-batch';
import { BatchRequestError } from '@/services/knowledge-batch-service';
import type { BatchTask } from '@/interfaces/database/knowledge-batch';
import {
  CatalogueLoading,
  CatalogueScopeFailure,
} from '../knowledge-catalogue/state';
import { DirectoryRegistrationForm } from './directory-fields';
import { BatchFailure, BatchSelection } from './batch-fields';
import BatchPreviewPanel from './batch-preview';
import BatchTasks from './batch-tasks';
import BatchManifest from './batch-manifest';
import { useBatchFlow, type BatchLocation } from './use-batch-flow';

function BatchWorkspace({
  domain,
  storageKey,
  onAuthorizationFailure,
}: {
  domain: string;
  storageKey: string;
  onAuthorizationFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    [params, setParams] = useSearchParams();
  const registrationParam = params.get('registration') || '',
    taskParam = params.get('task') || '',
    project = params.get('project') || '',
    manifest = params.get('manifest') || '';
  function location(values: BatchLocation) {
    const next = new URLSearchParams(params);
    next.set('mode', 'batch');
    for (const [name, value] of Object.entries(values)) {
      if (value === null || value === '') next.delete(name);
      else if (value !== undefined) next.set(name, value);
    }
    setParams(next);
  }
  const flow = useBatchFlow(
    domain,
    storageKey,
    registrationParam,
    location,
    onAuthorizationFailure,
  );
  const tab =
    params.get('batch_tab') ||
    (project || manifest
      ? 'projects'
      : taskParam || flow.pending
        ? 'tasks'
        : 'prepare');
  const reg =
    registrationParam ||
    flow.pending?.registration_id ||
    flow.registration?.registration_id ||
    '';
  const task = taskParam || flow.pending?.task_id || '';
  const [lookup, setLookup] = useState(reg),
    [newRegistration, setNewRegistration] = useState(!reg);
  const choosePrepare = () => location({ batch_tab: 'prepare' }),
    chooseTasks = () => location({ batch_tab: 'tasks' }),
    chooseProjects = () => location({ batch_tab: 'projects' });
  const handleLookup = (e: ChangeEvent<HTMLInputElement>) =>
    setLookup(e.target.value);
  const openRegistration = (e: FormEvent) => {
    e.preventDefault();
    void flow.openRegistration(lookup);
  };
  const showNew = () => setNewRegistration((current) => !current);
  const startNew = () => {
    void flow.startNew();
  };
  const selectTask = (value: BatchTask) =>
    location({
      registration: value.registration_id,
      task: value.task_id,
      batch_tab: 'tasks',
    });
  const prepareProject = (registration: string, taskId: string) =>
    location({ registration, task: taskId, batch_tab: 'projects' });
  useEffect(() => setLookup(reg), [reg]);
  const taskReference = reg && task ? { registration: reg, task } : null;
  return (
    <div className="space-y-4">
      <nav
        className="flex flex-wrap gap-2"
        aria-label={t('knowledgeBatch.workspace')}
      >
        <Button
          variant={tab === 'prepare' ? 'default' : 'outline'}
          onClick={choosePrepare}
        >
          {t('knowledgeBatch.selection')}
        </Button>
        <Button
          variant={tab === 'tasks' ? 'default' : 'outline'}
          onClick={chooseTasks}
        >
          {t('knowledgeBatch.tasks')}
        </Button>
        <Button
          variant={tab === 'projects' ? 'default' : 'outline'}
          onClick={chooseProjects}
        >
          {t('knowledgeBatch.projects')}
        </Button>
      </nav>
      {tab !== 'projects' && !!flow.error && !flow.blocked && (
        <BatchFailure error={flow.error} />
      )}
      {tab === 'prepare' && (
        <CatalogueScopeFailure.Provider value={flow.fail}>
          {flow.blocked ? (
            <BatchFailure error={flow.error} onReload={flow.reauthorize} />
          ) : flow.pending ? (
            <div className="space-y-2">
              <p>{t('knowledgeBatch.resolveOriginalFirst')}</p>
              <Button onClick={chooseTasks}>{t('knowledgeBatch.tasks')}</Button>
              {(flow.pending.task_id || flow.canRestart) && (
                <Button onClick={startNew} disabled={flow.busy}>
                  {t('knowledgeBatch.newBatch')}
                </Button>
              )}
            </div>
          ) : (
            <>
              {!flow.registration && (
                <>
                  <form onSubmit={openRegistration} className="space-y-2">
                    <label className="block">
                      {t('knowledgeBatch.registrationId')}
                      <Input
                        required
                        pattern="[a-f0-9]{32}"
                        value={lookup}
                        onChange={handleLookup}
                        disabled={flow.busy}
                      />
                    </label>
                    <Button type="submit" disabled={flow.busy}>
                      {t('knowledgeBatch.openRegistration')}
                    </Button>
                  </form>
                  <Button variant="outline" onClick={showNew}>
                    {t('knowledgeBatch.newRegistration')}
                  </Button>
                  {newRegistration && (
                    <DirectoryRegistrationForm
                      domain={domain}
                      busy={flow.busy}
                      attempt={flow.attempt}
                      onSubmit={flow.register}
                      onError={flow.fail}
                    />
                  )}
                </>
              )}
              {flow.registration && (
                <BatchSelection
                  key={
                    flow.registration.registration_id +
                    ':' +
                    (flow.retryParent?.task.task_id || '')
                  }
                  domain={domain}
                  flow={flow}
                />
              )}
              {flow.preview && (
                <BatchPreviewPanel key={flow.preview.preview_id} flow={flow} />
              )}
            </>
          )}
        </CatalogueScopeFailure.Provider>
      )}
      {tab === 'tasks' && (
        <BatchTasks
          domain={domain}
          reg={reg}
          taskId={task}
          flow={flow}
          onSelect={selectTask}
          onProject={prepareProject}
        />
      )}
      {tab === 'projects' && (
        <BatchManifest
          domain={domain}
          storageKey={storageKey}
          project={project}
          manifestId={manifest}
          initialTask={taskReference}
          onLocation={location}
          onAuthorizationFailure={onAuthorizationFailure}
        />
      )}
    </div>
  );
}
function AuthorizedBatch({
  domain,
  onFailure,
}: {
  domain: string;
  onFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    query = useKnowledgeDomain(domain),
    contract = useBatchContract();
  const [key, setKey] = useState(''),
    [keyError, setKeyError] = useState<unknown>(null);
  const authorization = getAuthorization();
  let userId = '';
  try {
    userId = authorizationStorage.getUserInfoObject?.()?.id || '';
  } catch {
    /* token-only scope */
  }
  useEffect(() => {
    let active = true;
    setKey('');
    void batchStorageKey(domain, authorization, userId)
      .then((value) => {
        if (active) setKey(value);
      })
      .catch((error) => {
        if (active) setKeyError(error);
      });
    return () => {
      active = false;
    };
  }, [domain, authorization, userId]);
  const reload = () => {
    void query.refetch();
    void contract.refetch();
  };
  if (query.error || contract.error || keyError)
    return (
      <BatchFailure
        error={query.error || contract.error || keyError}
        onReload={reload}
      />
    );
  if (!query.data || !contract.data || !key) return <CatalogueLoading />;
  const value = query.data;
  if (
    value.domain_id !== domain ||
    value.contract_version !== '1.0.0' ||
    value.policy.access !== 'owner_only' ||
    value.policy.external_models ||
    value.policy.automatic_ingestion ||
    value.policy.allowed_model_uses.length ||
    value.policy.index_modes.length !== 1 ||
    value.policy.index_modes[0] !== 'catalogue'
  )
    return (
      <BatchFailure
        error={new BatchRequestError(409, 'knowledge_batch_version_mismatch')}
      />
    );
  return (
    <>
      <h1 className="text-2xl font-semibold">
        {t('knowledgeBatch.title')} · {value.name}
      </h1>
      <p>{t('knowledgeBatch.catalogueOnly')}</p>
      <BatchWorkspace
        key={key}
        domain={domain}
        storageKey={key}
        onAuthorizationFailure={onFailure}
      />
    </>
  );
}
export default function BatchImport({ domain }: { domain: string }) {
  const [error, setError] = useState<unknown>(null),
    [generation, setGeneration] = useState(0);
  const reload = () => {
    setError(null);
    setGeneration((current) => current + 1);
  };
  return error ? (
    <BatchFailure error={error} onReload={reload} />
  ) : (
    <AuthorizedBatch
      key={domain + ':' + getKnowledgeSessionScope() + ':' + generation}
      domain={domain}
      onFailure={setError}
    />
  );
}

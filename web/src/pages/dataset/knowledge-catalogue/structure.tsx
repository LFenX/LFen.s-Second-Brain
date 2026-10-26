import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import { useStructureContract } from '@/hooks/use-knowledge-structure';
import { StructureId, StructureHash } from '@/utils/knowledge-structure-schema';
import {
  structureRejected,
  type StructureScope,
} from '@/utils/knowledge-structure';
import type { StructureTask } from '@/interfaces/database/knowledge-structure';
import { ProjectPicker } from '../knowledge-import/batch-manifest';
import { useStructureFlow } from './use-structure-flow';
import { StructurePolicyPanel } from './structure-policy';
import { StructurePrepare } from './structure-prepare';
import {
  StructureRecovery,
  StructureTaskList,
  StructureTaskTracker,
} from './structure-tasks';
import { StructureHeadPanel, StructureFixedView } from './structure-view';
import { CatalogueLoading } from './state';
import { StructureFailure } from './structure-state';

function StructureWorkspace({
  scope,
  runtimeReady,
}: {
  scope: StructureScope;
  runtimeReady: boolean;
}) {
  const { t } = useTranslation(),
    [params, setParams] = useSearchParams(),
    project = params.get('structure_project') ?? '',
    fixedView = params.get('structure_view_id') ?? '',
    [task, setTask] = useState<string | null>(null),
    storedFlow = useStructureFlow(scope),
    flow = runtimeReady ? storedFlow : { ...storedFlow, ready: false };
  const choose = (value: string) => {
    setTask(null);
    const next = new URLSearchParams({
      structure: '1',
      structure_project: value,
    });
    if (fixedView) next.set('structure_view_id', fixedView);
    setParams(next);
  };
  const open = (value: string) => {
    const next = new URLSearchParams({
      structure: '1',
      structure_view_id: value,
    });
    if (project) next.set('structure_project', project);
    setParams(next);
  };
  const track = (value: StructureTask) => setTask(value.task_id);
  if (
    (project && !StructureId.test(project)) ||
    (fixedView && !StructureHash.test(fixedView)) ||
    params.getAll('structure_project').length > 1 ||
    params.getAll('structure_view_id').length > 1
  )
    return <StructureFailure error={Error('Invalid structure target')} />;
  const recovered = Object.values(flow.outcomes)
      .filter((value) => !structureRejected(value) && 'state' in value)
      .at(-1) as StructureTask | undefined,
    taskId = task ?? recovered?.task_id;
  return (
    <div className="space-y-5">
      <h2 className="text-xl font-semibold">{t('knowledgeStructure.title')}</h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeStructure.intro')}
      </p>
      {!runtimeReady && (
        <p role="status">{t('knowledgeStructure.runtimeNotReady')}</p>
      )}
      <StructurePolicyPanel scope={scope} flow={flow} />
      <StructureRecovery flow={flow} onTask={track} />
      <ProjectPicker domain={scope.domain} onChoose={choose} />
      {project && (
        <>
          <p className="break-all text-xs">
            {t('knowledgeStructure.project')}: {project}
          </p>
          <StructureHeadPanel
            scope={{ ...scope, project }}
            fixed={fixedView || null}
            onView={open}
            onRefresh={flow.invalidatePreparation}
          />
          <StructurePrepare
            key={project}
            scope={{ ...scope, project }}
            flow={flow}
            onTask={track}
          />
          <StructureTaskList
            key={project + ':tasks'}
            scope={{ ...scope, project }}
            onTask={track}
          />
        </>
      )}
      {taskId && (
        <StructureTaskTracker
          key={taskId}
          scope={scope}
          taskId={taskId}
          flow={flow}
          onView={open}
        />
      )}
      {fixedView && (
        <StructureFixedView
          key={fixedView}
          scope={{ ...scope, view: fixedView }}
        />
      )}
    </div>
  );
}
export default function StructureBrowser({
  owner,
  domain,
}: {
  owner: string;
  domain: string;
}) {
  const query = useStructureContract(),
    reload = () => {
      void query.refetch();
    };
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  return (
    <StructureWorkspace
      key={getKnowledgeSessionScope() + ':' + owner + ':' + domain}
      scope={{ owner, domain }}
      runtimeReady={query.data.runtime_ready}
    />
  );
}

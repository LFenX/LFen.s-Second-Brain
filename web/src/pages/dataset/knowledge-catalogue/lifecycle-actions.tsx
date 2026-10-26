import { useEffect, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { ClassificationFields } from '../knowledge-import/fields';
import {
  useLifecycleMovePaths,
  scopedLifecycle,
  lifecycleScopeFailure,
} from '@/hooks/use-knowledge-lifecycle';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import service, {
  LifecycleRequestError,
} from '@/services/knowledge-lifecycle-service';
import type {
  LifecycleClassification,
  LifecycleDetail,
  LifecycleTargetCheck,
} from '@/interfaces/database/knowledge-lifecycle';
import type { LifecycleAction } from '@/interfaces/request/knowledge-lifecycle';
import { lifecycleExpected } from '@/utils/knowledge-lifecycle';
import type { LifecycleFlow } from './use-lifecycle-flow';

export function LifecycleFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation(),
    issue = error instanceof LifecycleRequestError ? error : null;
  return (
    <div
      role="alert"
      className="space-y-2 rounded-lg border border-border-button p-3 text-text-secondary"
    >
      <p>
        {t(
          issue && [401, 404].includes(issue.status)
            ? 'knowledgeLifecycle.notVisible'
            : issue?.code === 'revision_changed'
              ? 'knowledgeLifecycle.conflict'
              : 'knowledgeLifecycle.failure',
        )}
      </p>
      {issue && (
        <p className="break-all text-xs">
          {issue.code}
          {issue.requestId ? ' · ' + issue.requestId : ''}
        </p>
      )}
      {onRetry && (
        <Button variant="outline" onClick={onRetry}>
          {t('knowledgeImport.reload')}
        </Button>
      )}
    </div>
  );
}
function MoveFields({
  detail,
  value,
  onChange,
  disabled,
  onScopeFailure,
}: {
  detail: LifecycleDetail;
  value: string;
  onChange: (path: string) => void;
  disabled: boolean;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    query = useLifecycleMovePaths(detail),
    change = (event: ChangeEvent<HTMLSelectElement>) =>
      onChange(event.target.value);
  useEffect(() => {
    if (query.error && lifecycleScopeFailure(query.error))
      onScopeFailure(query.error);
  }, [query.error, onScopeFailure]);
  if (query.error) return <LifecycleFailure error={query.error} />;
  return (
    <label className="block">
      {t('knowledgeLifecycle.targetPath')}
      <select
        className="block w-full rounded border border-border-button bg-bg-base p-2"
        value={value}
        onChange={change}
        disabled={disabled || !query.data}
      >
        <option value="">{t('knowledgeLifecycle.chooseTarget')}</option>
        {query.data?.map((path) => (
          <option key={path} value={path}>
            {path}
          </option>
        ))}
      </select>
    </label>
  );
}
export default function LifecycleActions({
  detail,
  flow,
  onRefresh,
  onScopeFailure,
}: {
  detail: LifecycleDetail;
  flow: LifecycleFlow;
  onRefresh: () => void;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    session = getKnowledgeSessionScope(),
    [action, setAction] = useState<LifecycleAction>(
      detail.current.state === 'active' ? 'reclassify' : 'restore',
    ),
    [classification, setClassification] = useState<LifecycleClassification>(
      detail.current.classification,
    ),
    [path, setPath] = useState(''),
    [checked, setChecked] = useState(false),
    [target, setTarget] = useState<LifecycleTargetCheck | null>(null),
    [checking, setChecking] = useState(false),
    [error, setError] = useState<unknown>(null);
  const changeAction = (event: ChangeEvent<HTMLSelectElement>) => {
      setAction(event.target.value as LifecycleAction);
      setChecked(false);
      setTarget(null);
      setPath('');
      setClassification(detail.current.classification);
      setError(null);
    },
    changeClass = (value: LifecycleClassification) => {
      setClassification(value);
      setChecked(false);
    },
    changePath = (value: string) => {
      setPath(value);
      setChecked(false);
      setTarget(null);
    },
    changeChecked = (event: ChangeEvent<HTMLInputElement>) =>
      setChecked(event.target.checked);
  const checkTarget = async () => {
    setChecked(false);
    setChecking(true);
    setError(null);
    try {
      const result = await scopedLifecycle(session, () =>
        service.target(
          {
            owner: detail.owner_id,
            domain: detail.domain_id,
            source: detail.source_id,
          },
          { ...lifecycleExpected(detail), relative_path: path },
        ),
      );
      setTarget(result.data);
    } catch (issue) {
      setTarget(null);
      setError(issue);
      if (lifecycleScopeFailure(issue)) onScopeFailure(issue);
    } finally {
      setChecking(false);
    }
  };
  const submit = () => {
    if (!checked) return;
    const selected =
      action === 'reclassify'
        ? { classification }
        : action === 'move'
          ? { relative_path: path }
          : {};
    setChecked(false);
    void flow.submit(detail, action, selected, true);
  };
  const allowed = detail.actions[action],
    disabled = flow.busy || flow.unresolved || flow.corrupt || checking,
    moveReady =
      action !== 'move' || (!!target?.allowed && target.relative_path === path);
  const targetState =
    action === 'archive'
      ? 'archived'
      : action === 'restore'
        ? 'active'
        : detail.current.state;
  const formatClass = (value: LifecycleClassification) =>
    [
      value.group_id || t('knowledgeImport.noGroup'),
      t('knowledgeCatalogue.' + value.component_side),
      value.language || t('knowledgeCatalogue.unknownLanguage'),
    ].join(' · ');
  return (
    <section
      aria-label={t('knowledgeLifecycle.actions')}
      className="space-y-3 rounded-lg border border-border-button bg-bg-card p-4"
    >
      <h3 className="font-semibold">{t('knowledgeLifecycle.actions')}</h3>
      <label>
        {t('knowledgeLifecycle.action')}
        <select
          className="block w-full rounded border border-border-button bg-bg-base p-2"
          value={action}
          onChange={changeAction}
          disabled={disabled}
        >
          <option value="reclassify">
            {t('knowledgeLifecycle.reclassify')}
          </option>
          <option value="move">{t('knowledgeLifecycle.move')}</option>
          {detail.current.state === 'active' ? (
            <option value="archive">{t('knowledgeLifecycle.archive')}</option>
          ) : (
            <option value="restore">{t('knowledgeLifecycle.restore')}</option>
          )}
        </select>
      </label>
      {!allowed.allowed && (
        <p role="status" className="text-sm text-text-secondary">
          {t(
            'knowledgeLifecycle.reason.' +
              (allowed.reason || 'action_unsupported'),
          )}
        </p>
      )}
      <fieldset disabled={disabled || !allowed.allowed}>
        {action === 'reclassify' && (
          <ClassificationFields
            domainId={detail.domain_id}
            value={classification}
            onChange={changeClass}
          />
        )}{' '}
        {action === 'move' && allowed.supported && (
          <div className="space-y-2">
            <MoveFields
              detail={detail}
              value={path}
              onChange={changePath}
              disabled={disabled}
              onScopeFailure={onScopeFailure}
            />
            <Button
              type="button"
              variant="outline"
              disabled={!path || disabled}
              onClick={checkTarget}
            >
              {t('knowledgeLifecycle.checkTarget')}
            </Button>
            {target && (
              <p role="status">
                {t(
                  target.allowed
                    ? 'knowledgeLifecycle.targetAllowed'
                    : 'knowledgeLifecycle.reason.' + target.reason,
                )}{' '}
                · {t('knowledgeLifecycle.notReserved')}
              </p>
            )}
          </div>
        )}
      </fieldset>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-sm">
        <dt>{t('knowledgeLifecycle.expected')}</dt>
        <dd className="break-all">
          {detail.current.source_revision_id} ·{' '}
          {detail.current.lifecycle_version} · {detail.policy_version}
        </dd>
        <dt>{t('knowledgeLifecycle.path')}</dt>
        <dd className="break-all">
          {detail.current.relative_path}
          {action === 'move' ? ' → ' + (path || '—') : ''}
        </dd>
        <dt>{t('knowledgeLifecycle.classification')}</dt>
        <dd className="break-all">
          {formatClass(detail.current.classification)}
          {action === 'reclassify' ? ' → ' + formatClass(classification) : ''}
        </dd>
        <dt>{t('knowledgeLifecycle.state')}</dt>
        <dd>
          {t('knowledgeLifecycle.' + detail.current.state)} →{' '}
          {t('knowledgeLifecycle.' + targetState)}
        </dd>
      </dl>
      <p className="text-sm text-text-secondary">
        {t(
          action === 'archive'
            ? 'knowledgeLifecycle.archiveHint'
            : action === 'restore'
              ? 'knowledgeLifecycle.restoreHint'
              : 'knowledgeLifecycle.metadataHint',
        )}
      </p>
      {!!error && <LifecycleFailure error={error} onRetry={onRefresh} />}
      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          checked={checked}
          onChange={changeChecked}
          disabled={disabled || !allowed.allowed || !moveReady}
        />
        <span>{t('knowledgeLifecycle.consent')}</span>
      </label>
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={submit}
          disabled={disabled || !allowed.allowed || !checked || !moveReady}
        >
          {t('knowledgeLifecycle.confirm')}
        </Button>
        <Button
          variant="outline"
          onClick={onRefresh}
          disabled={flow.busy || checking}
        >
          {t('knowledgeLifecycle.refresh')}
        </Button>
      </div>
    </section>
  );
}

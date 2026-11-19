import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { AccessRequestError } from '@/services/knowledge-access-service';
import type { AccessCommandController } from '@/hooks/use-knowledge-access-command';

export function AccessFailure({ error }: { error: unknown }) {
  const { t } = useTranslation();
  if (!error) return null;
  const code = error instanceof AccessRequestError ? error.code : 'unavailable';
  const label = ['unauthenticated', 'action_denied', 'not_found'].includes(code)
    ? 'notVisible'
    : [
          'version_conflict',
          'scope_changed',
          'view_stale',
          'idempotency_conflict',
        ].includes(code)
      ? 'changed'
      : code === 'index_stale' || code === 'index_unindexed'
        ? 'indexNotReady'
        : code === 'scope_not_supported'
          ? 'unsupportedScope'
          : code === 'invalid_request'
            ? 'invalidInput'
            : 'unavailable';
  return (
    <div
      role="alert"
      className="space-y-1 rounded-xl border-0.5 border-state-warning/40 bg-state-warning/5 px-3 py-2.5 text-sm"
    >
      <p className="text-text-primary">{t('knowledgeAccess.' + label)}</p>
      <p className="font-mono text-xs text-text-secondary">
        {t('knowledgeAccess.errorCode', { code })}
      </p>
    </div>
  );
}

export function AccessCommandFeedback({
  command,
}: {
  command: AccessCommandController;
}) {
  const { t } = useTranslation();
  function handleRecover() {
    void command.recover();
  }
  function handleReview() {
    void command.reviewLatest();
  }
  return (
    <div className="space-y-2" aria-live="polite">
      {command.pending && (
        <div className="space-y-2 rounded border border-border-button p-3">
          <p>
            {t(
              command.busy
                ? 'knowledgeAccess.saving'
                : 'knowledgeAccess.unknown',
            )}
          </p>
          <p className="text-sm text-text-secondary">
            {t('knowledgeAccess.operation.' + command.pending.operation)}
          </p>
          <Button
            variant="outline"
            disabled={command.busy}
            onClick={handleRecover}
          >
            {t('knowledgeAccess.checkOriginal')}
          </Button>
        </div>
      )}
      {command.storageBlocked && (
        <p role="alert">{t('knowledgeAccess.storageBlocked')}</p>
      )}
      {!command.pending && !command.storageBlocked && (
        <AccessFailure error={command.error} />
      )}
      {command.needsReview && (
        <Button
          variant="outline"
          disabled={command.busy}
          onClick={handleReview}
        >
          {t('knowledgeAccess.reviewLatest')}
        </Button>
      )}
      {command.completed && <p>{t('knowledgeAccess.saved')}</p>}
    </div>
  );
}

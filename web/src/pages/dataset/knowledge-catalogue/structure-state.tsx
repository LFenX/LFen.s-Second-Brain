import { useContext, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  useStructurePrivateError,
  structureScopeFailure,
} from '@/hooks/use-knowledge-structure';
import { CatalogueScopeFailure } from './state';

export function StructureFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation(),
    onScopeFailure = useContext(CatalogueScopeFailure);
  useStructurePrivateError(error);
  useEffect(() => {
    if (structureScopeFailure(error) && error instanceof KnowledgeRequestError)
      onScopeFailure?.(error);
  }, [error, onScopeFailure]);
  const code =
    error instanceof KnowledgeRequestError
      ? error.code
      : error instanceof Error
        ? error.message
        : 'structure_result_unknown';
  return (
    <div
      role="alert"
      className="space-y-2 rounded-md border border-border-button p-3 text-text-secondary"
    >
      <p>{t('knowledgeStructure.failed')}</p>
      <p className="break-all text-xs">{code}</p>
      {error instanceof KnowledgeRequestError && error.requestId && (
        <p className="break-all text-xs">{error.requestId}</p>
      )}
      {onRetry && (
        <Button type="button" variant="outline" onClick={onRetry}>
          {t('knowledgeCatalogue.retry')}
        </Button>
      )}
    </div>
  );
}

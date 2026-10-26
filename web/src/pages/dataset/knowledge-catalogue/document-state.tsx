import { useContext, useLayoutEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import {
  useDocumentPrivateError,
  documentScopeFailure,
} from '@/hooks/use-knowledge-document';
import { CatalogueScopeFailure } from './state';
export function DocumentFailure({ error }: { error: unknown }) {
  const { t } = useTranslation(),
    fail = useContext(CatalogueScopeFailure);
  useDocumentPrivateError(error);
  useLayoutEffect(() => {
    if (documentScopeFailure(error) && error instanceof KnowledgeRequestError)
      fail?.(error);
  }, [error, fail]);
  return (
    <div role="alert" className="rounded border border-border-button p-3">
      <p>{t('knowledgeDocument.failed')}</p>
      <p className="break-all text-xs text-text-secondary">
        {error instanceof Error
          ? error.message
          : t('knowledgeDocument.unknown')}
      </p>
    </div>
  );
}

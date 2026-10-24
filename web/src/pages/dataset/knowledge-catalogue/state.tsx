import { createContext, useContext, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';

export const CatalogueScopeFailure = createContext<
  ((error: KnowledgeRequestError) => void) | null
>(null);

export function CatalogueFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const onScopeFailure = useContext(CatalogueScopeFailure);
  const status = error instanceof KnowledgeRequestError ? error.status : 0;
  useEffect(() => {
    if (
      onScopeFailure &&
      error instanceof KnowledgeRequestError &&
      [401, 403, 404, 409].includes(error.status)
    ) {
      onScopeFailure(error);
    }
  }, [error, onScopeFailure]);
  const reason =
    status === 401 || status === 403 || status === 404
      ? 'notVisible'
      : status === 409
        ? 'conflict'
        : 'failed';
  return (
    <div
      role="alert"
      className="space-y-3 rounded-lg border border-border-button p-4 text-text-secondary"
    >
      <p>{t(`knowledgeCatalogue.${reason}`)}</p>
      {error instanceof KnowledgeRequestError && error.requestId && (
        <p className="break-all text-xs">
          {t('knowledgeCatalogue.requestId')}: {error.requestId}
        </p>
      )}
      <Button type="button" variant="outline" onClick={onRetry}>
        {t('knowledgeCatalogue.retry')}
      </Button>
    </div>
  );
}

export function CatalogueLoading() {
  const { t } = useTranslation();
  return (
    <p role="status" className="p-4 text-text-secondary">
      {t('knowledgeCatalogue.loading')}
    </p>
  );
}

export function CatalogueEmpty({ label = 'empty' }: { label?: string }) {
  const { t } = useTranslation();
  return (
    <p role="status" className="p-4 text-text-secondary">
      {t(`knowledgeCatalogue.${label}`)}
    </p>
  );
}

export function LoadMore({
  more,
  loading,
  onMore,
}: {
  more: boolean;
  loading: boolean;
  onMore: () => void;
}) {
  const { t } = useTranslation();
  return (
    more && (
      <Button
        type="button"
        variant="outline"
        disabled={loading}
        onClick={onMore}
      >
        {t('knowledgeCatalogue.more')}
      </Button>
    )
  );
}

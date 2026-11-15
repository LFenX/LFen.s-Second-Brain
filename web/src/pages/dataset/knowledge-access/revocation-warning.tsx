import { useTranslation } from 'react-i18next';

export function AccessRevocationWarning() {
  const { t } = useTranslation();
  return (
    <p role="note" className="text-sm text-text-secondary">
      {t('knowledgeAccess.revocationBoundary')}
    </p>
  );
}

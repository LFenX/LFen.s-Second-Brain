import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import type {
  LocalLexicalIndexRef,
  LocalLexicalSelection,
} from '@/interfaces/database/knowledge-local-lexical';
import {
  accessPublicationChoice,
  KnowledgeAccessPaths,
} from '@/utils/knowledge-access-navigation';
import {
  accessSchemaValid,
  accessTargetsValid,
} from '@/utils/knowledge-access-schema';

export function AccessPublicationLink({
  domain,
  selection,
  publication,
  names,
}: {
  domain: string;
  selection: LocalLexicalSelection;
  publication: LocalLexicalIndexRef;
  names: Record<string, string>;
}) {
  const { t } = useTranslation();
  const choice = accessPublicationChoice(domain, selection, publication, names);
  if (
    !accessSchemaValid('ModuleScope', choice.scope) ||
    !accessTargetsValid(choice.scope.targets, domain)
  )
    return null;
  return (
    <Button
      asLink
      variant="outline"
      to={KnowledgeAccessPaths.owner(domain)}
      state={{ accessPublication: choice }}
    >
      {t('knowledgeAccess.manageSelection')}
    </Button>
  );
}

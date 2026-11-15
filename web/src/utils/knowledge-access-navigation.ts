import type {
  AccessModuleScope,
  AccessTarget,
} from '@/interfaces/database/knowledge-access';
import type {
  LocalLexicalSelection,
  LocalLexicalIndexRef,
} from '@/interfaces/database/knowledge-local-lexical';

export const KnowledgeAccessPaths = {
  shared: '/shared-knowledge',
  ownerPattern: '/dataset/knowledge-access/:id',
  ownerKey: '/knowledge-access',
  owner: (domain: string) => '/dataset/knowledge-access/' + domain,
  catalogue: (domain: string) =>
    '/dataset/knowledge-catalogue/' + domain + '?content=1',
} as const;
export interface AccessPublicationChoice {
  domain: string;
  scope: AccessModuleScope;
  names: Record<string, string>;
}
export function accessPublicationChoice(
  domain: string,
  selection: LocalLexicalSelection,
  publication: LocalLexicalIndexRef,
  names: Record<string, string>,
): AccessPublicationChoice {
  const targets: AccessTarget[] = selection.targets.map((target) => ({
    source: { ...target.source_ref },
    representation:
      target.representation_ref.kind === 'source_revision'
        ? {
            kind: 'source_revision',
            id: target.source_ref.source_revision_id,
            producer_sha256: null,
          }
        : { ...target.representation_ref },
  })) as AccessTarget[];
  const choice: AccessPublicationChoice = {
    domain,
    scope: {
      kind: 'fixed_published_targets',
      publication: {
        id: publication.index_id,
        manifest_sha256: publication.manifest_sha256,
      },
      targets,
    },
    names: { ...names },
  };
  return choice;
}

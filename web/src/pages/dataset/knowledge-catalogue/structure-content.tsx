import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import Editor, { loader } from '@monaco-editor/react';
import { useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { useStructureContent } from '@/hooks/use-knowledge-structure';
import type {
  StructureNode,
  StructureViewMember,
} from '@/interfaces/database/knowledge-structure';
import type { StructureViewScope } from '@/utils/knowledge-structure';
import { structureBytePosition } from '@/utils/knowledge-structure';
import { CatalogueLoading } from './state';
import { StructureFailure } from './structure-state';

loader.config({ paths: { vs: '/vs' } });
export function StructureContent({
  scope,
  node,
  member,
}: {
  scope: StructureViewScope;
  node: StructureNode;
  member: StructureViewMember;
}) {
  const { t } = useTranslation(),
    { theme } = useTheme(),
    [starts, setStarts] = useState<(number | undefined)[]>([undefined]),
    query = useStructureContent(
      scope,
      node,
      member.sha256,
      starts.at(-1) === undefined ? {} : { start_byte: starts.at(-1) },
    );
  const reload = () => {
      void query.refetch();
    },
    next = () => {
      if (
        query.data?.next_start_byte !== null &&
        query.data?.next_start_byte !== undefined
      )
        setStarts((value) => [...value, query.data!.next_start_byte!]);
    },
    previous = () => setStarts((value) => value.slice(0, -1));
  if (query.isError)
    return <StructureFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  const content = query.data,
    last = structureBytePosition(
      content.text,
      new TextEncoder().encode(content.text).length,
    );
  return (
    <section className="space-y-2">
      <h4 className="font-semibold">{t('knowledgeStructure.original')}</h4>
      <p className="break-all text-xs">
        {t('knowledgeStructure.byteRange', {
          start: content.start_byte,
          end: content.end_byte,
        })}{' '}
        ·{' '}
        {t('knowledgeStructure.localCoordinates', {
          lines: last.lineNumber,
          column: last.column,
        })}
      </p>
      <p className="text-xs text-text-secondary">
        {t('knowledgeStructure.coordinateHint')}
      </p>
      <Editor
        height="360px"
        path={
          'knowledge-structure://' +
          scope.domain +
          '/' +
          scope.view +
          '/' +
          node.source_revision_id +
          '/' +
          node.structure_node_id +
          '/' +
          content.start_byte
        }
        value={content.text}
        language={node.parser === 'tsx' ? 'typescript' : node.parser}
        theme={theme === 'dark' ? 'vs-dark' : 'vs'}
        options={{
          readOnly: true,
          domReadOnly: true,
          automaticLayout: true,
          minimap: { enabled: false },
          links: false,
          contextmenu: false,
          wordWrap: 'on',
          scrollBeyondLastLine: false,
          unicodeHighlight: { ambiguousCharacters: false },
          renderValidationDecorations: 'off',
          quickSuggestions: false,
        }}
      />
      <div className="flex gap-2">
        {starts.length > 1 && (
          <Button type="button" variant="outline" onClick={previous}>
            {t('knowledgeStructure.previousWindow')}
          </Button>
        )}
        {!content.complete && (
          <Button type="button" variant="outline" onClick={next}>
            {t('knowledgeStructure.nextWindow')}
          </Button>
        )}
      </div>
    </section>
  );
}

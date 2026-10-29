import { useTranslation } from 'react-i18next';
import {
  useContentCapabilities,
  useContentScopeError,
} from '@/hooks/use-knowledge-consumption';
import { useOriginalContract } from '@/hooks/use-knowledge-original';
import { useStructureContract } from '@/hooks/use-knowledge-structure';
import { useDocumentContract } from '@/hooks/use-knowledge-document';
import { useLocalLexicalPolicy } from '@/hooks/use-knowledge-local-lexical';
import type { ContentCapabilities } from '@/interfaces/database/knowledge-consumption';
import type { OriginalCapabilities } from '@/interfaces/database/knowledge-original';
import type { StructureCapabilities } from '@/interfaces/database/knowledge-structure';
import type { DocumentCapabilities } from '@/interfaces/database/knowledge-document';
import type { ContentScope } from '@/interfaces/request/knowledge-consumption';
import {
  CatalogueFailure,
  CatalogueLoading,
} from '@/pages/dataset/knowledge-catalogue/state';

type FormatFacts = {
  family: string;
  extensions: string[];
  outputs: string[];
  note?: string;
};
// Fixed classification and output facts; runtime results still come from the owning APIs.
const ParserFormats: Record<string, FormatFacts> = {
  pdf: {
    family: 'pdf',
    extensions: ['pdf'],
    outputs: ['text', 'table', 'image'],
  },
  doc: {
    family: 'doc',
    extensions: ['doc'],
    outputs: ['text', 'markdown'],
    note: 'markdownNormalization',
  },
  docx: {
    family: 'docx',
    extensions: ['docx'],
    outputs: ['text', 'heading', 'table', 'image', 'markdown'],
    note: 'markdownNormalization',
  },
  ppt: {
    family: 'ppt',
    extensions: ['ppt'],
    outputs: ['text', 'image'],
    note: 'slidesFlattenTables',
  },
  pptx: {
    family: 'pptx',
    extensions: ['pptx'],
    outputs: ['text', 'image'],
    note: 'slidesFlattenTables',
  },
  xls: {
    family: 'xls',
    extensions: ['xls'],
    outputs: ['table', 'image'],
    note: 'xlsImageBranch',
  },
  xlsx: { family: 'xlsx', extensions: ['xlsx'], outputs: ['table', 'image'] },
  csv: { family: 'csv', extensions: ['csv'], outputs: ['table'] },
  html: {
    family: 'html',
    extensions: ['html', 'htm'],
    outputs: ['text', 'heading', 'list', 'code', 'table', 'image'],
  },
  md: {
    family: 'md',
    extensions: ['md', 'markdown', 'mdx'],
    outputs: ['text', 'heading', 'list', 'code', 'table', 'image'],
  },
  txt: {
    family: 'txt',
    extensions: [
      'txt',
      'py',
      'js',
      'java',
      'c',
      'cpp',
      'h',
      'php',
      'go',
      'ts',
      'sh',
      'cs',
      'kt',
      'sql',
    ],
    outputs: ['text'],
    note: 'plainCodeText',
  },
  epub: { family: 'epub', extensions: ['epub'], outputs: ['text'] },
  json: {
    family: 'json',
    extensions: ['json', 'jsonl', 'ldjson'],
    outputs: ['text'],
  },
  email: {
    family: 'email',
    extensions: ['eml', 'msg'],
    outputs: ['text'],
    note: 'emailBody',
  },
  visual: {
    family: 'visual',
    extensions: [
      'png',
      'jpg',
      'jpeg',
      'gif',
      'bmp',
      'tiff',
      'tif',
      'webp',
      'svg',
      'ico',
      'avif',
      'heic',
      'apng',
    ],
    outputs: ['image'],
    note: 'imageIsNotOCR',
  },
  aural: {
    family: 'aural',
    extensions: [
      'da',
      'wave',
      'wav',
      'mp3',
      'aac',
      'flac',
      'ogg',
      'aiff',
      'au',
      'midi',
      'wma',
      'ape',
      'alac',
      'wv',
      'opus',
    ],
    outputs: ['configuration'],
    note: 'audioIsNotTranscript',
  },
};
const CodeFormats: Record<string, FormatFacts> = {
  typescript: { family: 'typescript', extensions: ['ts'], outputs: ['code'] },
  tsx: { family: 'tsx', extensions: ['tsx'], outputs: ['code'] },
  go: { family: 'go', extensions: ['go'], outputs: ['code'] },
  markdown: {
    family: 'md',
    extensions: ['md', 'markdown'],
    outputs: ['text', 'heading'],
  },
};
type MatrixProps = {
  capabilities: ContentCapabilities;
  original?: OriginalCapabilities;
  structure?: StructureCapabilities;
  document?: DocumentCapabilities;
  lexicalEnabled?: boolean;
};

export default function KnowledgeCapabilities({
  scope,
}: {
  scope: ContentScope;
}) {
  const { t } = useTranslation(),
    query = useContentCapabilities(scope),
    original = useOriginalContract(),
    structure = useStructureContract(),
    document = useDocumentContract(),
    lexical = useLocalLexicalPolicy(scope);
  useContentScopeError(query.error);
  const reload = () => {
    void query.refetch();
    void original.refetch();
    void structure.refetch();
    void document.refetch();
    void lexical.refetch();
  };
  if (query.isError)
    return <CatalogueFailure error={query.error} onRetry={reload} />;
  if (!query.data) return <CatalogueLoading />;
  return (
    <section
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t('knowledgeConsumption.support')}
    >
      <h2 className="font-semibold">{t('knowledgeConsumption.support')}</h2>
      <KnowledgeArchitecture
        capabilities={query.data}
        lexicalEnabled={lexical.data?.enabled}
      />
      <p className="text-sm">
        {t('knowledgeConsumption.currentReading', {
          bytes: query.data.max_content_bytes,
          count: query.data.max_page_items,
        })}
      </p>
      <KnowledgeCapabilityMatrix
        capabilities={query.data}
        original={original.data}
        structure={structure.data}
        document={document.data}
        lexicalEnabled={lexical.data?.enabled}
      />
      <p className="text-sm text-text-secondary">
        {t('knowledgeConsumption.noQualityClaim')}
      </p>
    </section>
  );
}

export function KnowledgeCapabilityMatrix({
  capabilities,
  original,
  structure,
  document,
  lexicalEnabled,
}: MatrixProps) {
  const { t } = useTranslation();
  const registrations = capabilities.parser_registrations.map((entry) => ({
    ...entry,
    code: false,
  }));
  const code = (structure?.parser_ids ?? [])
    .filter((id) => !registrations.some((entry) => entry.file_type === id))
    .map((id) => ({
      file_type: id,
      base_result: 'structured' as const,
      code: true,
    }));
  return (
    <div className="space-y-3">
      <h3 className="font-semibold">{t('knowledgeConsumption.matrixTitle')}</h3>
      <p>
        {t('knowledgeConsumption.registrationCount', {
          count: registrations.length,
          structures: structure
            ? code.length
            : t('knowledgeConsumption.registrationPending'),
        })}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeConsumption.outputBoundaries')}
      </p>
      <div className="overflow-auto">
        <table
          className="w-full text-left text-sm"
          aria-label={t('knowledgeConsumption.matrixTitle')}
        >
          <thead>
            <tr>
              {[
                'familyExtensions',
                'outputKinds',
                'saveState',
                'parseState',
                'indexState',
                'searchState',
                'verification',
              ].map((key) => (
                <th key={key} scope="col" className="p-2">
                  {t('knowledgeConsumption.' + key)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {[...registrations, ...code].map((entry) => {
              const facts = (entry.code ? CodeFormats : ParserFormats)[
                entry.file_type
              ];
              const originalFormat =
                original?.supported_formats.some(
                  (format) => format === entry.file_type,
                ) ?? false;
              const profile = document?.profiles.find(
                (item) => item.format === entry.file_type,
              );
              const codeParser =
                structure?.parser_ids.some((id) => id === entry.file_type) ??
                entry.code;
              const ready = codeParser
                ? structure?.runtime_ready
                : profile
                  ? document?.runtime_ready && profile.runtime_ready
                  : undefined;
              return (
                <tr
                  key={entry.file_type}
                  className="border-t border-border-button"
                >
                  <th scope="row" className="p-2">
                    <p>
                      {facts
                        ? t('knowledgeConsumption.family_' + facts.family)
                        : entry.file_type}
                    </p>
                    <p className="text-xs font-normal text-text-secondary">
                      {entry.code
                        ? t('knowledgeConsumption.structureRegistration')
                        : t('knowledgeConsumption.producerRegistration')}{' '}
                      · {entry.file_type}
                    </p>
                    <p className="break-words text-xs font-normal">
                      {facts
                        ? facts.extensions
                            .map((extension) => '.' + extension)
                            .join(' ')
                        : t('knowledgeConsumption.mappingUnknown')}
                    </p>
                  </th>
                  <td className="p-2">
                    <ul>
                      {facts ? (
                        facts.outputs.map((kind) => (
                          <li key={kind}>
                            {t('knowledgeConsumption.output_' + kind)}
                          </li>
                        ))
                      ) : (
                        <li>{t('knowledgeConsumption.mappingUnknown')}</li>
                      )}
                    </ul>
                    {facts?.note && (
                      <p className="text-xs text-text-secondary">
                        {t('knowledgeConsumption.note_' + facts.note)}
                      </p>
                    )}
                  </td>
                  <td className="p-2">
                    {t(
                      entry.code
                        ? 'knowledgeConsumption.savedTextRequired'
                        : originalFormat
                          ? original?.runtime_ready
                            ? 'knowledgeConsumption.originalImportAvailable'
                            : 'knowledgeConsumption.runtimeNotReady'
                          : 'knowledgeConsumption.saveNotReported',
                    )}
                  </td>
                  <td className="p-2">
                    <p>
                      {t('knowledgeConsumption.result_' + entry.base_result)}
                    </p>
                    <p className="text-xs text-text-secondary">
                      {t(
                        ready === undefined
                          ? 'knowledgeConsumption.runtimeUnknown'
                          : ready
                            ? 'knowledgeConsumption.runtimeReadyConditional'
                            : 'knowledgeConsumption.runtimeNotReady',
                      )}
                    </p>
                    {!codeParser && !profile && (
                      <p className="text-xs">
                        {t('knowledgeConsumption.fixedPathPending')}
                      </p>
                    )}
                  </td>
                  <td className="p-2">
                    {t(
                      lexicalEnabled === undefined
                        ? 'knowledgeConsumption.matrixIndexUnknown'
                        : lexicalEnabled
                          ? 'knowledgeConsumption.matrixIndexEnabled'
                          : 'knowledgeConsumption.matrixIndexDisabled',
                    )}
                  </td>
                  <td className="p-2">
                    {t('knowledgeConsumption.retrievalNeedsSelection')}
                  </td>
                  <td className="p-2">
                    {t('knowledgeConsumption.noEvidenceInCapabilities')}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <details className="text-sm text-text-secondary">
        <summary>{t('knowledgeConsumption.capabilitySources')}</summary>
        <p>{t('knowledgeConsumption.classificationSource')}</p>
        <code>internal/utility/file.go · GetFileType</code>
        <p>{t('knowledgeConsumption.producerSource')}</p>
        <code>
          internal/parser/parser/catalog.go · producer / NormalizeParseResult
        </code>
        <p>{t('knowledgeConsumption.structureSource')}</p>
        <code>
          internal/service/dataset/knowledge_structure.go ·
          structureParserMatches
        </code>
        <p>{t('knowledgeConsumption.runtimeSources')}</p>
      </details>
    </div>
  );
}

const ArchitectureStages = [
  'source',
  'revision',
  'representation',
  'node',
  'chunk',
  'index',
  'evidence',
] as const;
export function KnowledgeArchitecture({
  capabilities,
  lexicalEnabled,
}: {
  capabilities: ContentCapabilities;
  lexicalEnabled: boolean | undefined;
}) {
  const { t } = useTranslation();
  const state = (stage: (typeof ArchitectureStages)[number]) =>
    stage === 'index'
      ? t(
          lexicalEnabled === undefined
            ? 'knowledgeConsumption.archIndexUnknown'
            : lexicalEnabled
              ? 'knowledgeConsumption.archIndexEnabled'
              : 'knowledgeConsumption.archIndexDisabled',
        )
      : t('knowledgeConsumption.archState_' + stage);
  return (
    <details className="rounded border border-border-button p-3" open>
      <summary className="cursor-pointer font-semibold">
        {t('knowledgeConsumption.archTitle')}
      </summary>
      <div className="space-y-3 pt-3">
        <ol
          className="flex flex-wrap items-center gap-2"
          aria-label={t('knowledgeConsumption.archFlow')}
        >
          {ArchitectureStages.map((stage, index) => (
            <li
              key={stage}
              className="rounded border border-border-button px-2 py-1"
            >
              <span>
                {index + 1}. {t('knowledgeConsumption.archStage_' + stage)}
              </span>
              {index < ArchitectureStages.length - 1 && (
                <span className="ml-2" aria-hidden="true">
                  →
                </span>
              )}
            </li>
          ))}
        </ol>
        <p className="text-sm">{t('knowledgeConsumption.archAuthority')}</p>
        <p className="text-sm">{t('knowledgeConsumption.archIdentity')}</p>
        <div className="overflow-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr>
                <th scope="col" className="p-2">
                  {t('knowledgeConsumption.archObject')}
                </th>
                <th scope="col" className="p-2">
                  {t('knowledgeConsumption.archFields')}
                </th>
                <th scope="col" className="p-2">
                  {t('knowledgeConsumption.archEvidence')}
                </th>
              </tr>
            </thead>
            <tbody>
              {ArchitectureStages.map((stage) => (
                <tr key={stage} className="border-t border-border-button">
                  <th scope="row" className="p-2">
                    {t('knowledgeConsumption.archStage_' + stage)}
                  </th>
                  <td className="p-2">
                    {t('knowledgeConsumption.archFields_' + stage)}
                    {stage === 'representation' && (
                      <ul>
                        {capabilities.fixed_representation_kinds.map((kind) => (
                          <li key={kind}>
                            {t('knowledgeConsumption.representation_' + kind)}
                          </li>
                        ))}
                      </ul>
                    )}
                  </td>
                  <td className="p-2">{state(stage)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <dl className="grid grid-cols-1 gap-2 md:grid-cols-2">
          {['save', 'parse', 'index', 'search'].map((kind) => (
            <div key={kind} className="rounded border border-border-button p-2">
              <dt className="font-semibold">
                {t('knowledgeConsumption.archIndependent_' + kind)}
              </dt>
              <dd>
                {t('knowledgeConsumption.archIndependentMeaning_' + kind)}
              </dd>
            </div>
          ))}
        </dl>
        <p className="text-sm text-text-secondary">
          {t('knowledgeConsumption.archCoordinates')}
        </p>
        <p className="text-sm text-text-secondary">
          {t('knowledgeConsumption.archIncomplete')}
        </p>
      </div>
    </details>
  );
}

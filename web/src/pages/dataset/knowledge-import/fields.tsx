import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import Contract from '../../../../../docs/knowledge-sources-v1.openapi.json';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useKnowledgeGroups } from '@/hooks/use-knowledge-foundation';
import type {
  ComponentSide,
  KnowledgeGroup,
} from '@/interfaces/database/knowledge-foundation';
import type { KnowledgeRegistration } from '@/interfaces/database/knowledge-sources';
import type {
  KnowledgePreviewRequest,
  KnowledgeRegistrationRequest,
} from '@/interfaces/request/knowledge-sources';
import { readKnowledgeTextFile } from '@/utils/knowledge-import';
import {
  CatalogueFailure,
  CatalogueLoading,
  LoadMore,
} from '../knowledge-catalogue/state';

type Classification = Pick<
  KnowledgePreviewRequest,
  'group_id' | 'component_side' | 'language'
>;
const ComponentSides = Contract.components.schemas.PreviewRequest.properties
  .component_side.enum as ComponentSide[];

function GroupChoice({
  group,
  onSelect,
  onBrowse,
}: {
  group: KnowledgeGroup;
  onSelect: (group: KnowledgeGroup) => void;
  onBrowse: (group: KnowledgeGroup) => void;
}) {
  const { t } = useTranslation();
  const handleSelect = () => onSelect(group);
  const handleBrowse = () => onBrowse(group);
  return (
    <li className="flex min-w-0 flex-wrap gap-2">
      <Button type="button" variant="outline" onClick={handleSelect}>
        {group.name}
      </Button>
      <Button type="button" variant="ghost" onClick={handleBrowse}>
        {t('knowledgeImport.childGroups')}
      </Button>
    </li>
  );
}

function GroupPicker({
  domainId,
  value,
  onChange,
}: {
  domainId: string;
  value: string | null;
  onChange: (value: string | null) => void;
}) {
  const { t } = useTranslation();
  const [path, setPath] = useState<KnowledgeGroup[]>([]);
  const [selectedName, setSelectedName] = useState('');
  const query = useKnowledgeGroups(domainId, path.at(-1)?.id);
  const handleSelect = (group: KnowledgeGroup) => {
    setSelectedName(group.name);
    onChange(group.id);
  };
  const handleBrowse = (group: KnowledgeGroup) => setPath([...path, group]);
  const handleParent = () => setPath(path.slice(0, -1));
  const handleNone = () => {
    onChange(null);
    setSelectedName('');
  };
  const handleMore = () => {
    void query.fetchNextPage();
  };
  const handleRetry = () => {
    void query.refetch();
  };
  return (
    <details className="rounded-lg border border-border-button p-3">
      <summary className="cursor-pointer">
        {t('knowledgeImport.group')}:{' '}
        {value
          ? selectedName || t('knowledgeImport.selectedGroup')
          : t('knowledgeImport.noGroup')}
      </summary>
      <div className="mt-3 space-y-2">
        <p className="text-xs text-text-secondary">
          {path.map((group) => group.name).join(' / ') ||
            t('knowledgeImport.rootGroups')}
        </p>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={handleNone}>
            {t('knowledgeImport.noGroup')}
          </Button>
          {!!path.length && (
            <Button type="button" variant="ghost" onClick={handleParent}>
              {t('knowledgeCatalogue.parentGroup')}
            </Button>
          )}
        </div>
        {query.isError ? (
          <CatalogueFailure error={query.error} onRetry={handleRetry} />
        ) : query.isPending ? (
          <CatalogueLoading />
        ) : (
          <>
            <ul className="space-y-2">
              {query.data.pages
                .flatMap((page) => page.items)
                .map((group) => (
                  <GroupChoice
                    key={group.id}
                    group={group}
                    onSelect={handleSelect}
                    onBrowse={handleBrowse}
                  />
                ))}
            </ul>
            <LoadMore
              more={query.hasNextPage}
              loading={query.isFetching}
              onMore={handleMore}
            />
          </>
        )}
      </div>
    </details>
  );
}

export function ClassificationFields({
  domainId,
  value,
  onChange,
}: {
  domainId: string;
  value: Classification;
  onChange: (value: Classification) => void;
}) {
  const { t } = useTranslation();
  const handleGroup = (group_id: string | null) =>
    onChange({ ...value, group_id });
  const handleSide = (event: ChangeEvent<HTMLSelectElement>) =>
    onChange({ ...value, component_side: event.target.value as ComponentSide });
  const handleLanguage = (event: ChangeEvent<HTMLInputElement>) =>
    onChange({ ...value, language: event.target.value });
  return (
    <div className="space-y-3">
      <GroupPicker
        domainId={domainId}
        value={value.group_id}
        onChange={handleGroup}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          {' '}
          <span>{t('knowledgeImport.componentSide')}</span>
          <select
            className="block w-full rounded-md border border-border-button bg-bg-base p-2"
            value={value.component_side}
            onChange={handleSide}
          >
            {ComponentSides.map((side) => (
              <option key={side} value={side}>
                {t(`knowledgeCatalogue.${side}`)}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-1">
          <span>{t('knowledgeImport.language')}</span>
          <Input
            maxLength={32}
            value={value.language}
            onChange={handleLanguage}
          />
        </label>
      </div>
    </div>
  );
}

export function RegistrationForm({
  domainId,
  busy,
  attempt,
  onSubmit,
}: {
  domainId: string;
  busy: boolean;
  attempt: KnowledgeRegistrationRequest | null;
  onSubmit: (request: KnowledgeRegistrationRequest) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [paths, setPaths] = useState('web/synthetic.tsx');
  const [classification, setClassification] = useState<Classification>({
    group_id: null,
    component_side: 'shared',
    language: '',
  });
  const handleKey = (event: ChangeEvent<HTMLInputElement>) =>
    setKey(event.target.value);
  const handleName = (event: ChangeEvent<HTMLInputElement>) =>
    setName(event.target.value);
  const handlePaths = (event: ChangeEvent<HTMLTextAreaElement>) =>
    setPaths(event.target.value);
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    void onSubmit({
      registration_key: key,
      name,
      source_kind: 'text_file',
      input_mode: 'inline_text',
      include_paths: paths.split(/\r?\n/).filter((path) => path !== ''),
      ...classification,
    });
  };
  const handleRecover = () => {
    if (attempt) void onSubmit(attempt);
  };
  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <p className="text-sm text-text-secondary">
        {t('knowledgeImport.registrationHint')}
      </p>
      <fieldset disabled={busy || !!attempt} className="space-y-4">
        <label className="block space-y-1">
          <span>{t('knowledgeImport.registrationKey')}</span>
          <Input
            required
            pattern={'[A-Za-z0-9_\\-]{1,64}'}
            maxLength={64}
            value={key}
            onChange={handleKey}
          />
        </label>
        <label className="block space-y-1">
          <span>{t('knowledgeImport.registrationName')}</span>
          <Input required maxLength={128} value={name} onChange={handleName} />
        </label>
        <label className="block space-y-1">
          <span>{t('knowledgeImport.includePaths')}</span>
          <Textarea required rows={4} value={paths} onChange={handlePaths} />
        </label>
        <ClassificationFields
          domainId={domainId}
          value={classification}
          onChange={setClassification}
        />
      </fieldset>
      {attempt ? (
        <Button type="button" disabled={busy} onClick={handleRecover}>
          {t('knowledgeImport.checkRegistration')}
        </Button>
      ) : (
        <Button type="submit" disabled={busy}>
          {t('knowledgeImport.register')}
        </Button>
      )}
    </form>
  );
}

export function SourceForm({
  registration,
  busy,
  onChange,
  onSubmit,
  onError,
  children,
}: {
  registration: KnowledgeRegistration;
  busy: boolean;
  onChange: () => void;
  onSubmit: (request: KnowledgePreviewRequest) => Promise<void>;
  onError: (error: unknown) => void;
  children?: React.ReactNode;
}) {
  const { t } = useTranslation();
  const [path, setPath] = useState(registration.include_paths[0]);
  const [text, setText] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [reading, setReading] = useState(false);
  const [classification, setClassification] = useState<Classification>({
    group_id: registration.group_id,
    component_side: registration.component_side,
    language: registration.language,
  });
  const handlePath = (event: ChangeEvent<HTMLSelectElement>) => {
    onChange();
    setPath(event.target.value);
  };
  const handleText = (event: ChangeEvent<HTMLTextAreaElement>) => {
    onChange();
    setText(event.target.value);
  };
  const handleClassification = (value: Classification) => {
    onChange();
    setClassification(value);
  };
  const handleFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    onChange();
    setLoaded(false);
    setText('');
    setReading(true);
    try {
      const result = await readKnowledgeTextFile(file);
      setText(result.text);
      setLoaded(true);
    } catch (error) {
      onError(error);
    } finally {
      setReading(false);
    }
  };
  const handleSample = () => {
    onChange();
    setText('// 合成\r\nexport const value = "你好 🧠";\r\n');
    setLoaded(true);
  };
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (loaded) void onSubmit({ relative_path: path, text, ...classification });
  };
  return (
    <div className="space-y-5">
      <form onSubmit={handleSubmit} className="space-y-4">
        <p className="text-sm text-text-secondary">
          {t('knowledgeImport.sourceHint')}
        </p>
        <fieldset disabled={busy || reading} className="space-y-4">
          <label className="block space-y-1">
            <span>{t('knowledgeImport.relativePath')}</span>
            <select
              className="block w-full rounded-md border border-border-button bg-bg-base p-2"
              value={path}
              onChange={handlePath}
            >
              {registration.include_paths.map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
          </label>
          <label className="block space-y-1">
            <span>{t('knowledgeImport.file')}</span>
            <Input type="file" onChange={handleFile} />
          </label>
          <Button type="button" variant="outline" onClick={handleSample}>
            {t('knowledgeImport.loadSample')}
          </Button>
          <label className="block space-y-1">
            <span>{t('knowledgeImport.localText')}</span>
            <Textarea
              rows={8}
              disabled={!loaded}
              value={text}
              onChange={handleText}
              className="font-mono"
            />
          </label>
          {loaded && (
            <p className="text-xs text-text-secondary">
              {new TextEncoder().encode(text).length} B · UTF-8
            </p>
          )}
          <ClassificationFields
            domainId={registration.domain_id}
            value={classification}
            onChange={handleClassification}
          />
          <Button type="submit" disabled={!loaded}>
            {t('knowledgeImport.preview')}
          </Button>
        </fieldset>
      </form>
      {children}
    </div>
  );
}

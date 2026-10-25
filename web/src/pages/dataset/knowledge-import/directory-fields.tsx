import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
  useDirectoryRoots,
  useDirectoryActions,
} from '@/hooks/use-knowledge-directory';
import service from '@/services/knowledge-directory-service';
import type {
  DirectoryClassification,
  DirectoryEntry,
  DirectoryListing,
  DirectoryObservation,
  DirectoryRegistration,
} from '@/interfaces/database/knowledge-directory';
import type {
  DirectoryRegistrationRequest,
  DirectoryPreviewRequest,
} from '@/interfaces/request/knowledge-directory';
import { appendDirectoryPage } from '@/utils/knowledge-directory';
import { compareDirectoryPaths } from '@/utils/knowledge-directory-schema';
import { CatalogueLoading, LoadMore } from '../knowledge-catalogue/state';
import { ClassificationFields } from './fields';

export function DirectoryRegistrationForm({
  domain,
  busy,
  attempt,
  onSubmit,
  onError,
}: {
  domain: string;
  busy: boolean;
  attempt: DirectoryRegistrationRequest | null;
  onSubmit: (request: DirectoryRegistrationRequest) => Promise<void>;
  onError: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    roots = useDirectoryRoots(domain);
  const [key, setKey] = useState(''),
    [name, setName] = useState(''),
    [root, setRoot] = useState(''),
    [scope, setScope] = useState('.');
  const [classification, setClassification] = useState<DirectoryClassification>(
    { group_id: null, component_side: 'shared', language: '' },
  );
  useEffect(() => {
    if (roots.error) onError(roots.error);
  }, [roots.error, onError]);
  const handleKey = (e: ChangeEvent<HTMLInputElement>) =>
    setKey(e.target.value);
  const handleName = (e: ChangeEvent<HTMLInputElement>) =>
    setName(e.target.value);
  const handleRoot = (e: ChangeEvent<HTMLSelectElement>) =>
    setRoot(e.target.value);
  const handleScope = (e: ChangeEvent<HTMLTextAreaElement>) =>
    setScope(e.target.value);
  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    void onSubmit({
      registration_key: key,
      name,
      root_id: root,
      scope_directories: scope.split(/\r?\n/).filter((p) => p !== ''),
      source_kind: 'controlled_directory',
      input_mode: 'server_read',
      default_classification: classification,
    });
  };
  const handleRecover = () => {
    if (attempt) void onSubmit(attempt);
  };
  if (roots.isPending) return <CatalogueLoading />;
  if (!roots.data) return null;
  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <p className="text-sm text-text-secondary">
        {t('knowledgeDirectory.platform')}
      </p>
      {!roots.data.items.length && <p>{t('knowledgeDirectory.noRoots')}</p>}
      <fieldset disabled={busy || !!attempt} className="space-y-3">
        <label className="block">
          {t('knowledgeDirectory.root')}
          <select
            className="block w-full rounded-md border border-border-button bg-bg-base p-2"
            required
            value={root}
            onChange={handleRoot}
          >
            <option value="">{t('knowledgeDirectory.chooseRoot')}</option>
            {roots.data.items.map((item) => (
              <option
                key={item.root_id}
                value={item.root_id}
                disabled={!item.ready}
              >
                {item.display_name} · {item.runtime_platform}
                {item.ready ? '' : ' · ' + t('knowledgeDirectory.notReady')}
              </option>
            ))}
          </select>
        </label>
        <label className="block">
          {t('knowledgeImport.registrationKey')}
          <Input
            required
            value={key}
            pattern="[A-Za-z0-9_-]{1,64}"
            maxLength={64}
            onChange={handleKey}
          />
        </label>
        <label className="block">
          {t('knowledgeImport.registrationName')}
          <Input required value={name} maxLength={128} onChange={handleName} />
        </label>
        <label className="block">
          {t('knowledgeDirectory.scope')}
          <Textarea required rows={3} value={scope} onChange={handleScope} />
        </label>
        <ClassificationFields
          domainId={domain}
          value={classification}
          onChange={setClassification}
        />
      </fieldset>
      {attempt ? (
        <Button type="button" disabled={busy} onClick={handleRecover}>
          {t('knowledgeDirectory.retryRegistration')}
        </Button>
      ) : (
        <Button disabled={busy || !root} type="submit">
          {t('knowledgeImport.register')}
        </Button>
      )}
    </form>
  );
}

function DirectoryEntryRow({
  entry,
  selected,
  busy,
  onToggle,
  onBrowse,
}: {
  entry: DirectoryEntry;
  selected: boolean;
  busy: boolean;
  onToggle: (path: string) => void;
  onBrowse: (path: string) => void;
}) {
  const { t } = useTranslation();
  const toggle = () => onToggle(entry.relative_path),
    browse = () => onBrowse(entry.relative_path);
  return (
    <li className="flex min-w-0 flex-wrap items-center gap-3 rounded-md border border-border-button p-2">
      {entry.entry_kind === 'directory' ? (
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={browse}
        >
          {entry.relative_path}/
        </Button>
      ) : (
        <label className="flex min-w-0 items-center gap-2">
          <input
            type="checkbox"
            aria-label={entry.relative_path}
            checked={selected}
            onChange={toggle}
            disabled={!entry.selectable || busy}
          />
          <span className="break-all">{entry.relative_path}</span>
        </label>
      )}
      <span className="text-xs text-text-secondary">
        {entry.byte_length ?? '—'} B ·{' '}
        {entry.reason
          ? t('knowledgeDirectory.reasons.' + entry.reason)
          : t('knowledgeDirectory.metadataOnly')}
      </span>
    </li>
  );
}
export function DirectorySelection({
  domain,
  registration,
  busy,
  observation,
  onObserve,
  onPreview,
  onChange,
  onInvalidate,
  onError,
}: {
  domain: string;
  registration: DirectoryRegistration;
  busy: boolean;
  observation: DirectoryObservation | null;
  onObserve: (paths: string[]) => Promise<void>;
  onPreview: (request: DirectoryPreviewRequest) => Promise<void>;
  onChange: () => void;
  onInvalidate: () => void;
  onError: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    actions = useDirectoryActions(domain);
  const [directory, setDirectory] = useState(registration.scope_directories[0]),
    [prefix, setPrefix] = useState(''),
    [selection, setSelection] = useState('');
  const [pages, setPages] = useState<DirectoryListing[]>([]),
    [listingBusy, setListingBusy] = useState(false);
  const [path, setPath] = useState(''),
    [classification, setClassification] = useState<DirectoryClassification>(
      registration.default_classification,
    );
  const selected = selection.split(/\r?\n/).filter((p) => p !== '');
  const allBusy = busy || listingBusy;
  async function browse(value: string) {
    if (allBusy) return;
    setListingBusy(true);
    setPages([]);
    try {
      const result = await actions.run(() =>
        service.createListing(domain, registration.registration_id, {
          relative_directory: value,
          name_prefix: prefix,
        }),
      );
      if (
        result.data.root_id !== registration.root_id ||
        result.data.relative_directory !== value ||
        result.data.name_prefix !== prefix
      )
        throw new Error('Invalid listing scope');
      setPages(appendDirectoryPage([], result.data));
    } catch (error) {
      onError(error);
    } finally {
      setListingBusy(false);
    }
  }
  const handleBrowse = (e: FormEvent) => {
    e.preventDefault();
    void browse(directory);
  };
  const handleDirectory = (e: ChangeEvent<HTMLInputElement>) => {
    setDirectory(e.target.value);
    setPages([]);
  };
  const handlePrefix = (e: ChangeEvent<HTMLInputElement>) => {
    setPrefix(e.target.value);
    setPages([]);
  };
  const handleChild = (value: string) => {
    setDirectory(value);
    void browse(value);
  };
  const handleMore = async () => {
    const first = pages[0],
      last = pages.at(-1);
    if (!first || !last?.next_cursor || allBusy) return;
    setListingBusy(true);
    try {
      const result = await actions.run(() =>
        service.listing(
          domain,
          registration.registration_id,
          first.listing_id,
          last.next_cursor!,
        ),
      );
      setPages(appendDirectoryPage(pages, result.data));
    } catch (error) {
      onError(error);
    } finally {
      setListingBusy(false);
    }
  };
  const handleSelection = (e: ChangeEvent<HTMLTextAreaElement>) => {
    onChange();
    setPath('');
    setSelection(e.target.value);
  };
  const handleToggle = (value: string) => {
    onChange();
    setPath('');
    setSelection(
      (selected.includes(value)
        ? selected.filter((p) => p !== value)
        : [...selected, value]
      ).join('\n'),
    );
  };
  const handleObserve = (e: FormEvent) => {
    e.preventDefault();
    setPath('');
    void onObserve([...selected].sort(compareDirectoryPaths));
  };
  const handlePath = (e: ChangeEvent<HTMLSelectElement>) => {
    onInvalidate();
    setPath(e.target.value);
  };
  const handleClassification = (value: DirectoryClassification) => {
    onInvalidate();
    setClassification(value);
  };
  const handlePreview = (e: FormEvent) => {
    e.preventDefault();
    if (observation && path)
      void onPreview({
        observation_id: observation.observation_id,
        observation_sha256: observation.observation_sha256,
        relative_path: path,
        classification,
      });
  };
  return (
    <section className="space-y-5">
      <p className="break-all">
        {t('knowledgeDirectory.registered')}: {registration.name}
      </p>
      <form onSubmit={handleBrowse} className="space-y-2">
        <fieldset disabled={allBusy} className="space-y-2">
          <label className="block">
            {t('knowledgeDirectory.relativeDirectory')}
            <Input value={directory} onChange={handleDirectory} />
          </label>
          <label className="block">
            {t('knowledgeDirectory.prefix')}
            <Input value={prefix} maxLength={128} onChange={handlePrefix} />
          </label>
          <Button type="submit">{t('knowledgeDirectory.browse')}</Button>
        </fieldset>
      </form>
      {!!pages.length && (
        <div className="space-y-2">
          <p>
            {t(
              pages[0].complete
                ? 'knowledgeDirectory.listComplete'
                : 'knowledgeDirectory.listIncomplete',
            )}{' '}
            · {pages.flatMap((p) => p.items).length}/{pages[0].total_entries}
          </p>
          {pages[0].reason && (
            <p>{t('knowledgeDirectory.reasons.' + pages[0].reason)}</p>
          )}
          <ul className="space-y-2">
            {pages
              .flatMap((page) => page.items)
              .map((entry) => (
                <DirectoryEntryRow
                  key={entry.relative_path}
                  entry={entry}
                  selected={selected.includes(entry.relative_path)}
                  busy={allBusy}
                  onToggle={handleToggle}
                  onBrowse={handleChild}
                />
              ))}
          </ul>
          <LoadMore
            more={!!pages.at(-1)?.next_cursor}
            loading={allBusy}
            onMore={handleMore}
          />
        </div>
      )}
      <form onSubmit={handleObserve} className="space-y-2">
        <label className="block">
          {t('knowledgeDirectory.exactSelection')}
          <Textarea
            rows={4}
            value={selection}
            onChange={handleSelection}
            disabled={allBusy}
          />
        </label>
        <p>
          {t('knowledgeDirectory.selectedCount', { count: selected.length })}
        </p>
        <Button
          type="submit"
          disabled={allBusy || !selected.length || selected.length > 32}
        >
          {t('knowledgeDirectory.observe')}
        </Button>
      </form>
      {observation && (
        <div className="space-y-3">
          <p role="status">
            {t(
              observation.complete
                ? 'knowledgeDirectory.observationComplete'
                : 'knowledgeDirectory.observationIncomplete',
            )}
          </p>
          <ul>
            {observation.items.map((item) => (
              <li key={item.relative_path} className="break-all">
                {item.relative_path} ·{' '}
                {t('knowledgeDirectory.states.' + item.state)} ·{' '}
                {item.change_kind}
                {item.reason
                  ? ' · ' + t('knowledgeDirectory.reasons.' + item.reason)
                  : ''}
              </li>
            ))}
          </ul>
          {observation.complete && (
            <form onSubmit={handlePreview} className="space-y-3">
              <fieldset disabled={busy} className="space-y-3">
                <label className="block">
                  {t('knowledgeDirectory.oneFile')}
                  <select
                    className="block w-full rounded-md border border-border-button bg-bg-base p-2"
                    value={path}
                    onChange={handlePath}
                    required
                  >
                    <option value="">
                      {t('knowledgeDirectory.chooseFile')}
                    </option>
                    {observation.items
                      .filter((item) => item.eligible)
                      .map((item) => (
                        <option
                          key={item.relative_path}
                          value={item.relative_path}
                        >
                          {item.relative_path}
                        </option>
                      ))}
                  </select>
                </label>
                <ClassificationFields
                  domainId={domain}
                  value={classification}
                  onChange={handleClassification}
                />
                <Button type="submit" disabled={!path}>
                  {t('knowledgeImport.preview')}
                </Button>
              </fieldset>
            </form>
          )}
        </div>
      )}
    </section>
  );
}

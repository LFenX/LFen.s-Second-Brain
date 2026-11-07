import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { FileUploader } from '@/components/file-uploader';
import { KnowledgeRequestError } from '@/services/knowledge-foundation-service';
import type { OriginalClassification } from '@/interfaces/database/knowledge-original';
import type { OriginalRegistrationRequest } from '@/interfaces/request/knowledge-original';
import { compareDirectoryPaths } from '@/utils/knowledge-directory-schema';
import {
  OriginalMaxBytes,
  validOriginalData,
} from '@/utils/knowledge-original-schema';
import { ClassificationFields } from './fields';
import type { OriginalFlow } from './use-original-flow';

export const OriginalAccept = {
  'application/octet-stream': [
    '.pdf',
    '.pptx',
    '.docx',
    '.txt',
    '.md',
    '.markdown',
    '.ts',
    '.tsx',
    '.js',
    '.jsx',
    '.go',
    '.css',
    '.scss',
    '.json',
    '.yaml',
    '.yml',
    '.html',
    '.htm',
    '.sql',
    '.sh',
    '.c',
    '.cpp',
    '.h',
    '.hpp',
    '.java',
    '.rs',
    '.py',
  ],
};
const InitialClassification: OriginalClassification = {
  group_id: null,
  component_side: 'non_code',
  language: '',
};
export function OriginalFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  const value = error instanceof KnowledgeRequestError ? error : null;
  const label =
    value && [401, 403, 404].includes(value.status)
      ? 'notVisible'
      : value?.status === 409
        ? 'conflict'
        : value?.status === 413
          ? 'tooLarge'
          : value?.status === 415
            ? 'unsupported'
            : value?.status === 429
              ? 'slotsBusy'
              : value?.status === 410
                ? 'expired'
                : value?.status === 400
                  ? 'invalidInput'
                  : 'unconfirmed';
  return (
    <div
      role="alert"
      className="space-y-2 rounded-lg border border-border-button p-3"
    >
      <p>{t('knowledgeOriginal.' + label)}</p>
      {value && (
        <p className="break-all text-xs text-text-secondary">
          {value.code}
          {value.requestId ? ' · ' + value.requestId : ''}
        </p>
      )}
      {onRetry && (
        <Button variant="outline" onClick={onRetry}>
          {t('knowledgeOriginal.checkResult')}
        </Button>
      )}
    </div>
  );
}
function groupCreationDisabled(flow: OriginalFlow) {
  return (
    flow.unknown ||
    !!flow.confirmPending ||
    (!!flow.createPending &&
      (flow.createPending.kind === 'upload' || !flow.registration)) ||
    flow.blocked ||
    flow.recoveryChanged ||
    flow.lockingUnavailable ||
    flow.corruptRecovery
  );
}

export function OriginalRegistrationForm({
  domain,
  flow,
}: {
  domain: string;
  flow: OriginalFlow;
}) {
  const { t } = useTranslation();
  const [registrationKey, setRegistrationKey] = useState(
    flow.createPending?.kind === 'registration'
      ? flow.createPending.registration_key
      : crypto.randomUUID(),
  );
  const [name, setName] = useState(''),
    [paths, setPaths] = useState('');
  const [classification, setClassification] = useState(InitialClassification);
  const handleKey = (event: ChangeEvent<HTMLInputElement>) =>
    setRegistrationKey(event.target.value);
  const handleName = (event: ChangeEvent<HTMLInputElement>) =>
    setName(event.target.value);
  const handlePaths = (event: ChangeEvent<HTMLTextAreaElement>) =>
    setPaths(event.target.value);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const input: OriginalRegistrationRequest = {
      registration_key: registrationKey,
      name,
      source_kind: 'original_document',
      input_mode: 'original_upload',
      include_paths: paths.split('\n').sort(compareDirectoryPaths),
      default_classification: classification,
    };
    if (!validOriginalData('CreateRegistration', input)) {
      flow.fail(new Error('Invalid registration'));
      return;
    }
    void flow.register(input);
  };
  return (
    <form
      className="space-y-3"
      onSubmit={submit}
      aria-label={t('knowledgeOriginal.registration')}
    >
      <label className="block space-y-1">
        <span>{t('knowledgeOriginal.registrationKey')}</span>
        <Input
          required
          maxLength={64}
          value={registrationKey}
          onChange={handleKey}
          disabled={flow.busy || !!flow.createPending}
        />
      </label>
      <label className="block space-y-1">
        <span>{t('knowledgeOriginal.name')}</span>
        <Input
          required
          maxLength={128}
          value={name}
          onChange={handleName}
          disabled={flow.busy}
        />
      </label>
      <label className="block space-y-1">
        <span>{t('knowledgeOriginal.paths')}</span>
        <Textarea
          required
          value={paths}
          onChange={handlePaths}
          disabled={flow.busy}
        />
      </label>
      <p className="text-sm text-text-secondary">
        {t('knowledgeOriginal.pathsHint')}
      </p>
      <ClassificationFields
        domainId={domain}
        value={classification}
        onChange={setClassification}
        disabled={flow.busy || flow.cancelling}
        createDisabled={groupCreationDisabled(flow)}
      />
      <Button
        type="submit"
        disabled={
          flow.busy || (!!flow.createPending && flow.unknown && !flow.canResend)
        }
      >
        {t(
          flow.createPending
            ? 'knowledgeOriginal.resendRegistration'
            : 'knowledgeOriginal.createRegistration',
        )}
      </Button>
    </form>
  );
}
export function OriginalFileFields({
  domain,
  flow,
}: {
  domain: string;
  flow: OriginalFlow;
}) {
  const { t } = useTranslation();
  const reg = flow.registration!;
  const [files, setFiles] = useState<File[]>([]),
    [path, setPath] = useState(reg.include_paths[0]);
  const [classification, setClassification] = useState(
    reg.default_classification,
  );
  const handlePath = (event: ChangeEvent<HTMLSelectElement>) =>
    setPath(event.target.value);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (files[0]) void flow.prepare(files[0], path, classification);
  };
  return (
    <form
      onSubmit={submit}
      className="space-y-4"
      aria-label={t('knowledgeOriginal.prepare')}
    >
      <p className="break-all">
        {reg.name} · {reg.registration_id}
      </p>
      <FileUploader
        value={files}
        onValueChange={setFiles}
        maxSize={OriginalMaxBytes}
        maxFileCount={1}
        multiple={false}
        showFolderTab={false}
        accept={OriginalAccept}
        disabled={flow.busy}
        title={t('knowledgeOriginal.chooseFile')}
        description={t('knowledgeOriginal.fileHint')}
      />
      <p className="text-sm text-text-secondary">
        {t('knowledgeOriginal.originalName')}: {files[0]?.name || '—'}
      </p>
      <label className="block space-y-1">
        <span>{t('knowledgeOriginal.path')}</span>
        <select
          className="block w-full rounded-md border border-border-button bg-bg-base p-2"
          value={path}
          onChange={handlePath}
          disabled={flow.busy}
        >
          {reg.include_paths.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </select>
      </label>
      <ClassificationFields
        domainId={domain}
        value={classification}
        onChange={setClassification}
        disabled={flow.busy || flow.cancelling}
        createDisabled={groupCreationDisabled(flow)}
      />
      <Button type="submit" disabled={flow.busy || !files.length}>
        {t('knowledgeOriginal.prepare')}
      </Button>
    </form>
  );
}

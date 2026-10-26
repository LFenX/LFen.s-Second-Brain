import { useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { WebRequestError } from '@/services/knowledge-web-service';
import type { WebClassification } from '@/interfaces/database/knowledge-web';
import type {
  WebCreateRegistration,
  WebCreateLocalCapture,
} from '@/interfaces/request/knowledge-web';
import { webNewKey, webSha256 } from '@/utils/knowledge-web';
import { compareWebBytes, validWebData } from '@/utils/knowledge-web-schema';
import { ClassificationFields } from './fields';
import type { WebFlow } from './use-web-flow';

export function WebFailure({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation(),
    issue = error instanceof WebRequestError ? error : null;
  return (
    <div
      role="alert"
      className="space-y-2 rounded-lg border border-border-button p-4 text-text-secondary"
    >
      <p>
        {t(
          issue?.status === 401 || issue?.status === 404
            ? 'knowledgeImport.notVisible'
            : issue?.status === 409
              ? 'knowledgeImport.conflict'
              : issue?.status === 400
                ? 'knowledgeImport.invalidInput'
                : 'knowledgeWeb.failure',
        )}
      </p>
      {issue && (
        <p className="break-all text-xs">
          {issue.code}
          {issue.failure?.upstream_http_status
            ? ' · HTTP ' + issue.failure.upstream_http_status
            : ''}
          {issue.requestId ? ' · ' + issue.requestId : ''}
        </p>
      )}
      {onRetry && (
        <Button variant="outline" onClick={onRetry}>
          {t('knowledgeImport.reload')}
        </Button>
      )}
    </div>
  );
}
export function WebRegistrationForm({ flow }: { flow: WebFlow }) {
  const { t } = useTranslation();
  const [name, setName] = useState(''),
    [url, setUrl] = useState(''),
    [kind, setKind] =
      useState<WebCreateRegistration['source_kind']>('web_page'),
    [producer, setProducer] =
      useState<WebCreateRegistration['producer']>('direct'),
    [enabled, setEnabled] = useState(false),
    [origins, setOrigins] = useState(''),
    [registrationKey, setRegistrationKey] = useState(webNewKey),
    [classification, setClassification] = useState<WebClassification>({
      group_id: null,
      component_side: 'non_code',
      language: '',
    }),
    [error, setError] = useState<unknown>(null);
  const changeName = (event: ChangeEvent<HTMLInputElement>) =>
      setName(event.target.value),
    changeUrl = (event: ChangeEvent<HTMLInputElement>) =>
      setUrl(event.target.value),
    changeKind = (event: ChangeEvent<HTMLSelectElement>) =>
      setKind(event.target.value as typeof kind),
    changeProducer = (event: ChangeEvent<HTMLSelectElement>) =>
      setProducer(event.target.value as typeof producer),
    changeEnabled = (event: ChangeEvent<HTMLInputElement>) =>
      setEnabled(event.target.checked),
    changeOrigins = (event: ChangeEvent<HTMLTextAreaElement>) =>
      setOrigins(event.target.value),
    changeKey = (event: ChangeEvent<HTMLInputElement>) =>
      setRegistrationKey(event.target.value);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    const body: WebCreateRegistration = {
      registration_key: registrationKey,
      name,
      source_kind: kind,
      origin_url: url,
      producer,
      default_classification: classification,
      network_scope: enabled
        ? {
            enabled: true,
            allowed_origins: origins
              .split(/\r?\n/)
              .filter(Boolean)
              .sort(compareWebBytes),
          }
        : { enabled: false, allowed_origins: [] },
    };
    if (!validWebData('CreateRegistration', body)) {
      setError(new WebRequestError(400, 'knowledge_invalid_request'));
      return;
    }
    void flow.createRegistration(body);
  };
  return (
    <form
      onSubmit={submit}
      className="space-y-4 rounded-lg border border-border-button bg-bg-card p-4"
      aria-label={t('knowledgeWeb.registration')}
    >
      <h2 className="font-semibold">1 · {t('knowledgeWeb.registration')}</h2>
      <label className="block">
        {t('knowledgeImport.registrationName')}
        <Input required maxLength={128} value={name} onChange={changeName} />
      </label>
      <label className="block">
        {t('knowledgeWeb.originUrl')}
        <Input
          required
          type="url"
          value={url}
          onChange={changeUrl}
          maxLength={2048}
        />
      </label>
      <label className="block">
        {t('knowledgeWeb.sourceKind')}
        <select
          className="w-full rounded border border-border-button bg-bg-base p-2"
          value={kind}
          onChange={changeKind}
        >
          <option value="web_page">{t('knowledgeWeb.webPage')}</option>
          <option value="web_feed">{t('knowledgeWeb.webFeed')}</option>
        </select>
      </label>
      <label className="block">
        {t('knowledgeWeb.producer')}
        <select
          className="w-full rounded border border-border-button bg-bg-base p-2"
          value={producer}
          onChange={changeProducer}
        >
          <option value="direct">{t('knowledgeWeb.direct')}</option>
          <option value="rsshub">RSSHub</option>
          <option value="other_feed">{t('knowledgeWeb.otherFeed')}</option>
          <option value="browser_export">
            {t('knowledgeWeb.browserExport')}
          </option>
        </select>
      </label>
      <ClassificationFields
        domainId={flow.scope.domain}
        value={classification}
        onChange={setClassification}
      />
      <label className="flex items-start gap-2">
        <input type="checkbox" checked={enabled} onChange={changeEnabled} />
        <span>{t('knowledgeWeb.enableNetwork')}</span>
      </label>
      {enabled && (
        <label className="block">
          {t('knowledgeWeb.origins')}
          <Textarea
            required
            value={origins}
            onChange={changeOrigins}
            placeholder="https://example.test"
          />
        </label>
      )}
      <details>
        <summary>{t('knowledgeWeb.registrationKey')}</summary>
        <Input
          aria-label={t('knowledgeWeb.registrationKey')}
          required
          pattern="[A-Za-z0-9._:\-]{1,128}"
          value={registrationKey}
          onChange={changeKey}
        />
      </details>
      {!!error && <WebFailure error={error} />}
      <Button type="submit" disabled={flow.busy}>
        {t('knowledgeWeb.createRegistration')}
      </Button>
    </form>
  );
}
export function WebCaptureForm({ flow }: { flow: WebFlow }) {
  const { t } = useTranslation(),
    registration = flow.registration!;
  const [mode, setMode] = useState<'local_snapshot' | 'explicit_fetch'>(
      'local_snapshot',
    ),
    [file, setFile] = useState<File | null>(null),
    [media, setMedia] = useState<WebCreateLocalCapture['declared_media_type']>(
      registration.source_kind === 'web_page'
        ? 'text/html'
        : 'application/rss+xml',
    ),
    [finalUrl, setFinalUrl] = useState(''),
    [claimedAt, setClaimedAt] = useState(''),
    [error, setError] = useState<unknown>(null),
    [preparing, setPreparing] = useState(false);
  const changeMode = (event: ChangeEvent<HTMLSelectElement>) =>
      setMode(event.target.value as typeof mode),
    changeFile = (event: ChangeEvent<HTMLInputElement>) =>
      setFile(event.target.files?.[0] || null),
    changeMedia = (event: ChangeEvent<HTMLSelectElement>) =>
      setMedia(event.target.value as typeof media),
    changeUrl = (event: ChangeEvent<HTMLInputElement>) =>
      setFinalUrl(event.target.value),
    changeAt = (event: ChangeEvent<HTMLInputElement>) =>
      setClaimedAt(event.target.value);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    setPreparing(true);
    try {
      if (mode === 'explicit_fetch') {
        await flow.createCapture({
          registration_config_sha256: registration.config_sha256,
          idempotency_key: webNewKey(),
          acquisition_mode: mode,
        });
        return;
      }
      if (!file || file.size < 1 || file.size > 2097152)
        throw new WebRequestError(413, 'web_too_large');
      const bytes = await file.arrayBuffer();
      const text = new TextDecoder('utf-8', {
        fatal: true,
        ignoreBOM: true,
      }).decode(bytes);
      if (text.includes('\0'))
        throw new WebRequestError(415, 'web_encoding_unsupported');
      const body: WebCreateLocalCapture = {
        registration_config_sha256: registration.config_sha256,
        idempotency_key: webNewKey(),
        acquisition_mode: mode,
        original_filename: file.name,
        declared_byte_length: file.size,
        declared_media_type: media,
        expected_sha256: await webSha256(bytes),
        declared_final_url: finalUrl || null,
        claimed_captured_at: claimedAt || null,
      };
      if (!validWebData('CreateLocalCapture', body))
        throw new WebRequestError(400, 'knowledge_invalid_request');
      await flow.createCapture(body, file);
    } catch (issue) {
      setError(issue);
    } finally {
      setPreparing(false);
    }
  };
  return (
    <form
      onSubmit={submit}
      className="space-y-4 rounded-lg border border-border-button p-4"
      aria-label={t('knowledgeWeb.capture')}
    >
      <h2 className="font-semibold">{t('knowledgeWeb.capture')}</h2>
      <p>
        {registration.name} · {registration.origin_url}
      </p>
      <p className="break-all text-xs">
        {t('knowledgeWeb.registrationId')}: {registration.registration_id}
      </p>
      <label className="block">
        {t('knowledgeWeb.acquisitionMode')}
        <select
          className="w-full rounded border border-border-button bg-bg-base p-2"
          value={mode}
          onChange={changeMode}
        >
          <option value="local_snapshot">
            {t('knowledgeWeb.localSnapshot')}
          </option>
          {registration.network_scope.enabled && (
            <option value="explicit_fetch">
              {t('knowledgeWeb.explicitFetch')}
            </option>
          )}
        </select>
      </label>
      {mode === 'local_snapshot' ? (
        <>
          <label className="block">
            {t('knowledgeWeb.snapshotFile')}
            <input
              type="file"
              required
              className="block w-full rounded-md border border-border-button bg-bg-input px-3 py-2 text-sm text-text-primary"
              accept=".html,.htm,.xml,.rss,.atom,text/html,application/rss+xml,application/atom+xml,application/xml,text/xml"
              onChange={changeFile}
            />
          </label>
          <label className="block">
            {t('knowledgeWeb.mediaType')}
            <select
              className="w-full rounded border border-border-button bg-bg-base p-2"
              value={media}
              onChange={changeMedia}
            >
              {[
                'text/html',
                'application/rss+xml',
                'application/atom+xml',
                'application/xml',
                'text/xml',
              ].map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
          </label>
          <label className="block">
            {t('knowledgeWeb.declaredUrl')}
            <Input
              type="url"
              maxLength={2048}
              value={finalUrl}
              onChange={changeUrl}
            />
          </label>
          <label className="block">
            {t('knowledgeWeb.claimedAt')}
            <Input
              value={claimedAt}
              onChange={changeAt}
              placeholder="2026-10-26T00:00:00Z"
            />
          </label>
          <p className="text-sm text-text-secondary">
            {t('knowledgeWeb.localProvenance')}
          </p>
        </>
      ) : (
        <p className="text-sm text-text-secondary">
          {t('knowledgeWeb.fetchPreparation')}
        </p>
      )}
      {!!error && <WebFailure error={error} />}
      <Button type="submit" disabled={flow.busy || preparing}>
        {t('knowledgeWeb.prepareCapture')}
      </Button>
    </form>
  );
}

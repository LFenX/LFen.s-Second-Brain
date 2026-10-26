import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import service from '@/services/knowledge-web-service';
import { useWebResource } from '@/hooks/use-knowledge-web';
import { WebFailure } from './web-fields';
import type { WebFlow } from './use-web-flow';

export default function WebCapturePanel({ flow }: { flow: WebFlow }) {
  const { t } = useTranslation(),
    capture = flow.capture!;
  const [consent, setConsent] = useState(false),
    [file, setFile] = useState<File | null>(null);
  const query = useWebResource(
    flow.scope,
    'capture',
    capture.capture_id,
    (signal) => service.capture(flow.scope, capture.capture_id, signal),
    flow.scopeFailure,
  );
  const current = useRef(flow);
  current.current = flow;
  const adopt = query.adopt;
  useEffect(() => {
    try {
      adopt(capture);
    } catch (issue) {
      current.current.scopeFailure(issue);
    }
  }, [capture, adopt]);
  useEffect(() => {
    if (query.data)
      try {
        current.current.acceptCapture(query.data);
      } catch (issue) {
        current.current.scopeFailure(issue);
      }
  }, [query.data]);
  const changeConsent = (event: ChangeEvent<HTMLInputElement>) =>
      setConsent(event.target.checked),
    changeFile = (event: ChangeEvent<HTMLInputElement>) =>
      setFile(event.target.files?.[0] || null);
  const upload = () => {
      void flow.upload(file || undefined);
    },
    acquire = () => {
      setConsent(false);
      void flow.acquire(consent);
    },
    observe = () => {
      void flow.observe();
    },
    cancel = () => {
      void flow.cancelCapture();
    },
    recheck = () => {
      void query.recheck().then((result) => {
        if (result.data && !result.error) flow.acceptCapture(result.data, true);
      });
    };
  const mayUpload =
      capture.acquisition_mode === 'local_snapshot' &&
      ['awaiting_bytes', 'interrupted'].includes(capture.state),
    mayAcquire =
      capture.acquisition_mode === 'explicit_fetch' &&
      capture.state === 'awaiting_fetch' &&
      capture.attempt_started_at === null;
  return (
    <section
      aria-label={t('knowledgeWeb.captureState')}
      className="space-y-3 rounded-lg border border-border-button p-4"
    >
      <h2 className="font-semibold">
        {t('knowledgeWeb.captureState')} ·{' '}
        {t('knowledgeWeb.state.' + capture.state)}
      </h2>
      <p className="break-all text-xs">{capture.capture_id}</p>
      <p className="text-sm">
        {t('knowledgeWeb.captureExpires')}:{' '}
        {dayjs(capture.expires_at).format('YYYY-MM-DD HH:mm:ss')}
      </p>
      <p className="text-sm">
        {t(
          capture.provenance.kind === 'user_supplied'
            ? 'knowledgeWeb.localProvenance'
            : 'knowledgeWeb.serverProvenance',
        )}
      </p>
      <p className="break-all text-sm">
        {capture.provenance.requested_url}
        {capture.provenance.final_url
          ? ' → ' + capture.provenance.final_url
          : ''}
      </p>
      {capture.raw_sha256 && (
        <p className="break-all text-xs">
          SHA-256: {capture.raw_sha256} · {capture.raw_byte_length} B
        </p>
      )}
      {capture.failure && (
        <p className="text-sm text-text-secondary">
          {capture.failure.machine_code}
          {capture.failure.upstream_http_status
            ? ' · HTTP ' + capture.failure.upstream_http_status
            : ''}
        </p>
      )}
      {query.error && <WebFailure error={query.error} />}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          onClick={recheck}
          disabled={flow.busy || query.isFetching}
        >
          {t('knowledgeWeb.recheck')}
        </Button>
        {!flow.unknown && !['expired', 'cancelled'].includes(capture.state) && (
          <Button variant="outline" onClick={cancel} disabled={flow.busy}>
            {t('knowledgeWeb.cancelCapture')}
          </Button>
        )}
      </div>
      {query.pollingStopped && (
        <p role="status">{t('knowledgeWeb.pollingStopped')}</p>
      )}
      {mayUpload && !flow.unknown && (
        <div className="space-y-2">
          <label className="block">
            {t('knowledgeWeb.reselectFile')}
            <input
              type="file"
              className="block w-full rounded-md border border-border-button bg-bg-input px-3 py-2 text-sm text-text-primary"
              onChange={changeFile}
            />
          </label>
          <Button onClick={upload} disabled={flow.busy}>
            {t('knowledgeWeb.upload')}
          </Button>
          <p className="text-sm">
            {flow.progress} B · {t('knowledgeWeb.uploadHint')}
          </p>
        </div>
      )}
      {mayAcquire && !flow.unknown && (
        <div className="space-y-3">
          <p className="break-all text-sm">
            {flow.registration?.network_scope.allowed_origins.join(' · ')}
          </p>
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={consent}
              onChange={changeConsent}
              disabled={flow.busy}
            />
            <span>{t('knowledgeWeb.acquireConsent')}</span>
          </label>
          <Button onClick={acquire} disabled={!consent || flow.busy}>
            {t('knowledgeWeb.acquire')}
          </Button>
        </div>
      )}
      {capture.state === 'ready' && !flow.observation && !flow.unknown && (
        <Button onClick={observe} disabled={flow.busy}>
          {t('knowledgeWeb.observe')}
        </Button>
      )}
    </section>
  );
}

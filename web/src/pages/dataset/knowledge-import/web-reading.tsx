import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import service, {
  type WebReadingTarget,
  webRepresentation,
  WebRequestError,
} from '@/services/knowledge-web-service';
import {
  scopedWeb,
  collectWebPages,
  webScopeFailure,
} from '@/hooks/use-knowledge-web';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import type {
  WebCapture,
  WebRepresentation,
  WebReference,
  WebWindow,
} from '@/interfaces/database/knowledge-web';
import {
  verifiedWebText,
  webReadingDocument,
  webReferenceHref,
  appendWebWindow,
  webWindowRange,
  webByteWindow,
} from '@/utils/knowledge-web-reading';
import { webDigest } from '@/utils/knowledge-web';
import { WebFailure } from './web-fields';

function ReferenceItem({ reference }: { reference: WebReference }) {
  const href = webReferenceHref(reference);
  return (
    <li className="space-y-1 border-b border-border-button py-2 text-sm">
      <p>
        {reference.relation} · {reference.label}
      </p>
      <p className="break-all text-text-secondary">{reference.raw_href}</p>
      {href ? (
        <a
          className="break-all underline"
          href={href}
          target="_blank"
          rel="noopener noreferrer"
        >
          {href}
        </a>
      ) : (
        <p className="break-all">{reference.resolved_url || '—'}</p>
      )}
    </li>
  );
}
export default function WebReading({
  target,
  capture,
  onScopeFailure,
}: {
  target: WebReadingTarget;
  capture?: WebCapture;
  onScopeFailure: (error: unknown) => void;
}) {
  const { t } = useTranslation(),
    session = getKnowledgeSessionScope(),
    data = 'entry' in target ? target.entry : target.revision;
  const [kind, setKind] = useState<WebRepresentation['kind']>('safe_reading'),
    [text, setText] = useState<string | null>(null),
    [document, setDocument] = useState<string | null>(null),
    [references, setReferences] = useState<WebReference[] | null>(null),
    [windows, setWindows] = useState<WebWindow[]>([]),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null);
  const live = useRef(true),
    controller = useRef<AbortController | null>(null),
    generation = useRef(0);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
      controller.current?.abort();
    };
  }, []);
  async function run(work: (signal: AbortSignal) => Promise<void>) {
    if (busy) return;
    const current = ++generation.current,
      abort = new AbortController();
    controller.current = abort;
    setBusy(true);
    setError(null);
    try {
      await scopedWeb(session, () => work(abort.signal));
    } catch (issue) {
      if (live.current && current === generation.current) {
        setText(null);
        setDocument(null);
        setWindows([]);
        setReferences(null);
        setError(issue);
        if (webScopeFailure(issue)) onScopeFailure(issue);
      }
    } finally {
      if (live.current && current === generation.current) setBusy(false);
    }
  }
  function checkCurrent() {
    if (!live.current || session !== getKnowledgeSessionScope())
      throw new WebRequestError(401, 'knowledge_unauthorized');
  }
  const changeKind = (event: ChangeEvent<HTMLSelectElement>) => {
    controller.current?.abort();
    generation.current++;
    setKind(event.target.value as typeof kind);
    setText(null);
    setDocument(null);
    setWindows([]);
    setBusy(false);
  };
  async function readText(signal: AbortSignal) {
    const blob = await service.representation(target, kind, signal);
    const value = await verifiedWebText(blob, webRepresentation(target, kind));
    checkCurrent();
    return value;
  }
  const read = () => {
    void run(async (signal) => {
      const value = await readText(signal);
      setText(value);
      setDocument(kind === 'safe_reading' ? webReadingDocument(value) : null);
      setWindows([]);
    });
  };
  const readWindow = () => {
    void run(async (signal) => {
      const value = text === null ? await readText(signal) : text,
        descriptor = webRepresentation(target, kind),
        start = windows.at(-1)?.next_start_byte ?? 0,
        range = webWindowRange(value, start),
        result = (
          await service.window(target, kind, range.start, range.end, signal)
        ).data;
      checkCurrent();
      if (
        webByteWindow(value, result.start_byte, result.end_byte) !== result.text
      )
        throw Error('Window text does not match verified representation');
      setText(value);
      setDocument(null);
      setWindows(appendWebWindow(windows, result, descriptor));
    });
  };
  const readReferences = () => {
    void run(async () => {
      const values = await collectWebPages(
        (cursor) =>
          'entry' in target
            ? service.entryReferences(
                target.scope,
                data.capture_id,
                data.entry_key,
                cursor,
              )
            : service.revisionReferences(
                target.revision.domain_id,
                target.revision.source_revision_id,
                cursor,
              ),
        (item) => item.reference_id,
        256,
      );
      if (
        values.length !== data.reference_count ||
        (await webDigest('references', { references: values })) !==
          data.references_sha256
      )
        throw Error('Reference integrity mismatch');
      for (const item of values)
        if ((await webDigest('reference', item)) !== item.reference_id)
          throw Error('Reference identity mismatch');
      checkCurrent();
      setReferences(values);
    });
  };
  const download = () => {
    void run(async (signal) => {
      const blob =
        'revision' in target
          ? await service.revisionCapture(target.revision, signal)
          : capture
            ? await service.captureContent(target.scope, capture, signal)
            : null;
      if (!blob) throw Error('Capture metadata required');
      checkCurrent();
      const url = URL.createObjectURL(blob),
        anchor = window.document.createElement('a');
      anchor.href = url;
      anchor.download = 'web-capture-' + data.capture_id + '.bin';
      anchor.click();
      requestAnimationFrame(() => URL.revokeObjectURL(url));
    });
  };
  return (
    <section
      aria-label={t('knowledgeWeb.reading')}
      className="space-y-3 rounded-lg border border-border-button bg-bg-card p-4"
    >
      <h3 className="font-semibold">{data.metadata.title || data.entry_key}</h3>
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.completeness.' + data.metadata.body_completeness)} ·{' '}
        {t('knowledgeWeb.notIndexed')}
      </p>
      <p className="break-all text-xs">
        {data.entry_key} · {data.capture_id}
      </p>
      <label className="block">
        {t('knowledgeWeb.representation')}
        <select
          className="rounded border border-border-button bg-bg-base p-2"
          value={kind}
          onChange={changeKind}
          disabled={busy}
        >
          <option value="safe_reading">{t('knowledgeWeb.safeReading')}</option>
          <option value="decoded_content">
            {t('knowledgeWeb.decodedContent')}
          </option>
        </select>
      </label>
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={read} disabled={busy}>
          {t('knowledgeWeb.readFull')}
        </Button>
        <Button
          variant="outline"
          onClick={readWindow}
          disabled={busy || windows.at(-1)?.next_start_byte === null}
        >
          {t('knowledgeWeb.readWindow')}
        </Button>
        <Button variant="outline" onClick={readReferences} disabled={busy}>
          {t('knowledgeWeb.references')}
        </Button>
        {('revision' in target || capture) && (
          <Button variant="outline" onClick={download} disabled={busy}>
            {t('knowledgeWeb.downloadRaw')}
          </Button>
        )}
      </div>
      {!!error && <WebFailure error={error} />}
      <p className="text-sm text-text-secondary">
        {t('knowledgeWeb.readingPolicy')}
      </p>
      {document !== null && (
        <iframe
          title={t('knowledgeWeb.safeReading')}
          sandbox=""
          referrerPolicy="no-referrer"
          srcDoc={document}
          className="h-[36rem] w-full rounded border border-border-button bg-bg-base"
        />
      )}
      {text === '' && document !== null && (
        <p role="status">{t('knowledgeWeb.emptyBody')}</p>
      )}
      {text !== null && !document && !windows.length && (
        <pre className="max-h-[36rem] overflow-auto whitespace-pre-wrap break-words p-3">
          {text || t('knowledgeWeb.emptyBody')}
        </pre>
      )}
      {!!windows.length && (
        <div>
          <p className="text-xs">
            {t('knowledgeWeb.windowText')} · {windows[0].start_byte}–
            {windows.at(-1)!.end_byte} B
          </p>
          <pre className="max-h-[36rem] overflow-auto whitespace-pre-wrap break-words p-3">
            {windows.map((value) => value.text).join('') ||
              t('knowledgeWeb.emptyBody')}
          </pre>
        </div>
      )}
      {references !== null && (
        <div>
          <h4>
            {t('knowledgeWeb.references')} · {references.length}
          </h4>
          <ul>
            {references.map((reference) => (
              <ReferenceItem
                key={reference.reference_id}
                reference={reference}
              />
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

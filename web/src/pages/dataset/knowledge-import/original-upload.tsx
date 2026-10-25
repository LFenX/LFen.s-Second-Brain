import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { FileUploader } from '@/components/file-uploader';
import { useOriginalUpload } from '@/hooks/use-knowledge-original';
import { OriginalMaxBytes } from '@/utils/knowledge-original-schema';
import { OriginalAccept, OriginalFailure } from './original-fields';
import type { OriginalFlow } from './use-original-flow';

export default function OriginalUploadPanel({
  domain,
  flow,
}: {
  domain: string;
  flow: OriginalFlow;
}) {
  const { t } = useTranslation(),
    upload = flow.upload!;
  const query = useOriginalUpload(
    domain,
    upload.registration_id,
    upload.upload_id,
    flow.fail,
  );
  const { data, adopt } = query;
  const { rememberUpload, fail } = flow;
  const [files, setFiles] = useState<File[]>([]);
  useEffect(() => {
    if (data) {
      try {
        rememberUpload(data);
      } catch (error) {
        fail(error);
      }
    }
  }, [data, rememberUpload, fail]);
  useEffect(() => {
    adopt(upload);
  }, [upload, adopt]);
  const check = () => {
    void query.recheck();
  };
  const retransmit = () => {
    if (files[0]) void flow.retransmit(files[0]);
  };
  const cancel = () => {
    void flow.cancel();
  };
  const retryable = ['awaiting_bytes', 'interrupted'].includes(upload.state);
  const terminal = ['rejected', 'cancelled', 'expired', 'committed'].includes(
    upload.state,
  );
  return (
    <section
      className="space-y-3 rounded-lg border border-border-button p-4"
      aria-label={t('knowledgeOriginal.uploadState')}
    >
      <p role="status">{t('knowledgeOriginal.state.' + upload.state)}</p>
      <p className="break-all text-sm">
        {upload.original_filename} · {upload.relative_path}
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeOriginal.sent')}: {Math.floor(flow.progress)}% ·{' '}
        {t('knowledgeOriginal.received')}: {upload.received_bytes} /{' '}
        {upload.declared_byte_length} B · revision {upload.revision}
      </p>
      <progress
        className="w-full"
        max={100}
        value={flow.progress}
        aria-label={t('knowledgeOriginal.sent')}
      />
      {flow.progress >= 100 &&
        upload.state !== 'ready' &&
        upload.state !== 'committed' && (
          <p>{t('knowledgeOriginal.waitReady')}</p>
        )}
      {query.error && <OriginalFailure error={query.error} />}
      {query.pollingStopped && (
        <p role="status">{t('knowledgeOriginal.pollingStopped')}</p>
      )}
      <Button
        variant="outline"
        onClick={check}
        disabled={query.isFetching || flow.busy}
      >
        {t('knowledgeOriginal.checkResult')}
      </Button>
      {!terminal && (
        <Button variant="outline" onClick={cancel} disabled={flow.cancelling}>
          {t('knowledgeOriginal.cancel')}
        </Button>
      )}
      {retryable && !flow.busy && (
        <div className="space-y-3">
          <p>{t('knowledgeOriginal.retryHint')}</p>
          <FileUploader
            value={files}
            onValueChange={setFiles}
            maxSize={OriginalMaxBytes}
            maxFileCount={1}
            multiple={false}
            showFolderTab={false}
            accept={OriginalAccept}
            title={t('knowledgeOriginal.chooseFile')}
            description={t('knowledgeOriginal.fileHint')}
          />
          <Button onClick={retransmit} disabled={!files[0] || flow.busy}>
            {t('knowledgeOriginal.retransmit')}
          </Button>
        </div>
      )}
    </section>
  );
}

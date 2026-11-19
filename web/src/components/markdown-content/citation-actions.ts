import { IReferenceChunk } from '@/interfaces/database/chat';
import { supportsSourceLocate } from '@/utils/source-locate';

export interface CitationSource {
  documentId: string;
  chunk: IReferenceChunk;
  fileExtension: string;
  documentUrl?: string;
}

export type ClickDocumentButton = (
  documentId: string,
  chunk: IReferenceChunk,
) => void;

/** How a cited source can be opened, or undefined when it cannot. */
export const citationOpenMode = (
  {
    fileExtension,
    documentUrl,
  }: Pick<CitationSource, 'fileExtension' | 'documentUrl'>,
  clickDocumentButton?: ClickDocumentButton,
): 'locate' | 'open' | undefined => {
  if (supportsSourceLocate(fileExtension) && clickDocumentButton) {
    return 'locate';
  }
  return documentUrl ? 'open' : undefined;
};

/**
 * Opens a cited source: the in-app locate drawer for formats that support it,
 * otherwise the web page a web document came from, or the document view.
 */
export function openCitationSource(
  source: CitationSource,
  clickDocumentButton?: ClickDocumentButton,
) {
  const mode = citationOpenMode(source, clickDocumentButton);
  if (mode === 'locate') {
    clickDocumentButton?.(source.documentId, source.chunk);
    return;
  }
  if (mode !== 'open' || !source.documentUrl) return;
  // Web page documents carry their own url; opening the internal /document
  // route for them lands on an empty page.
  if (/^https?:\/\//i.test(source.documentUrl)) {
    window.open(source.documentUrl, '_blank', 'noopener,noreferrer');
    return;
  }
  window.open(
    `/document/${source.documentId}?ext=${source.fileExtension}&resource=document`,
    '_blank',
  );
}

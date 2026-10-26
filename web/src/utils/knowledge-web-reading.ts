import DOMPurify from 'dompurify';
import type {
  WebRepresentation,
  WebWindow,
  WebReference,
} from '@/interfaces/database/knowledge-web';
import { equalWebValue, webSha256 } from './knowledge-web';
import { validWebData, webHttpUrl } from './knowledge-web-schema';

export const WebReadingCsp =
  "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; media-src 'none'; font-src 'none'; object-src 'none'; frame-src 'none'; child-src 'none'; base-uri 'none'; form-action 'none'";
export async function verifiedWebText(
  blob: Blob,
  descriptor: WebRepresentation,
) {
  const bytes = await blob.arrayBuffer();
  if (
    bytes.byteLength !== descriptor.byte_length ||
    (await webSha256(bytes)) !== descriptor.sha256
  )
    throw Error('Representation integrity mismatch');
  return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
    bytes,
  );
}
export function webReadingDocument(text: string) {
  const fragment = DOMPurify.sanitize(text, {
    ALLOWED_TAGS: [
      'article',
      'section',
      'div',
      'p',
      'br',
      'hr',
      'h1',
      'h2',
      'h3',
      'h4',
      'h5',
      'h6',
      'ul',
      'ol',
      'li',
      'dl',
      'dt',
      'dd',
      'blockquote',
      'pre',
      'code',
      'strong',
      'b',
      'em',
      'i',
      'u',
      's',
      'del',
      'table',
      'thead',
      'tbody',
      'tfoot',
      'tr',
      'th',
      'td',
      'caption',
      'span',
      'a',
    ],
    ALLOWED_ATTR: ['colspan', 'rowspan'],
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    RETURN_DOM_FRAGMENT: true,
  });
  for (const node of fragment.querySelectorAll('[colspan], [rowspan]')) {
    for (const attribute of ['colspan', 'rowspan']) {
      const value = node.getAttribute(attribute);
      if (
        value !== null &&
        (!['TD', 'TH'].includes(node.tagName) ||
          !/^(?:[1-9][0-9]?|100)$/.test(value))
      )
        node.removeAttribute(attribute);
    }
  }
  const container = document.createElement('div');
  container.appendChild(fragment);
  return (
    '<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="' +
    WebReadingCsp +
    '"><meta charset="utf-8"><style>body{font:16px/1.65 system-ui;margin:20px;overflow-wrap:anywhere}pre{white-space:pre-wrap}table{border-collapse:collapse;max-width:100%}td,th{border:1px solid;padding:6px}</style></head><body>' +
    container.innerHTML +
    '</body></html>'
  );
}
export function webReferenceHref(reference: WebReference) {
  return reference.open_allowed &&
    reference.resolved_url &&
    webHttpUrl(reference.resolved_url)
    ? reference.resolved_url
    : null;
}
export function webWindowRange(text: string, start: number) {
  const bytes = new TextEncoder().encode(text);
  let end = Math.min(start + 4096, bytes.length);
  while (end < bytes.length && (bytes[end] & 0xc0) === 0x80) end--;
  if (webByteWindow(text, start, end) === null)
    throw Error('Invalid UTF-8 window');
  return { start, end };
}
export function webByteWindow(
  text: string,
  start: number,
  end: number,
): string | null {
  const bytes = new TextEncoder().encode(text);
  if (
    !Number.isSafeInteger(start) ||
    !Number.isSafeInteger(end) ||
    start < 0 ||
    end < start ||
    end > bytes.length ||
    end - start > 4096 ||
    (start === end && bytes.length !== 0)
  )
    return null;
  try {
    return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(
      bytes.slice(start, end),
    );
  } catch {
    return null;
  }
}
export function appendWebWindow(
  previous: WebWindow[],
  next: WebWindow,
  descriptor: WebRepresentation,
) {
  const last = previous.at(-1);
  if (
    !validWebData('Window', next) ||
    !equalWebValue(next.representation, descriptor) ||
    next.start_byte !== (last ? last.next_start_byte : 0) ||
    last?.next_start_byte === null ||
    (last &&
      [
        'domain_id',
        'capture_id',
        'observation_id',
        'entry_key',
        'source_revision_id',
      ].some(
        (key) =>
          (last as unknown as Record<string, unknown>)[key] !==
          (next as unknown as Record<string, unknown>)[key],
      ))
  )
    throw Error('Discontinuous representation window');
  return [...previous, next];
}

import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { getExtension } from '@/utils/document-util';

/*
 * The sources an answer cites. The chat pipeline marks a citation as [ID:n],
 * n being the passage's index in the reference; 波西米亚 numbers sources by
 * document, in the order a reader meets them, as 留白 does.
 */

export type CitedSource = {
  n: number;
  documentId: string;
  datasetId: string;
  title: string;
  /** Where it lives, in words: the guide or one of the person's datasets. */
  place: string;
  guide: boolean;
  extension: string;
  /** The passages cited from it, in the order they were cited. */
  chunks: IReferenceChunk[];
};

export const CitePattern = /\[ID:\s*(\d+)\]/g;
export const CiteHref = '#bohemia-cite-';

const titleOf = (name: string) => name.replace(/\.(md|markdown|txt)$/i, '');

/** The sources an answer cites, numbered by first appearance. */
export function citedSources(
  text: string,
  reference: IReference | undefined,
  guideDatasetId: string,
  datasetName: (id: string) => string | undefined,
) {
  const sources: CitedSource[] = [];
  const byChunk = new Map<number, CitedSource>();
  if (!reference?.chunks?.length) return { sources, byChunk };
  for (const match of text.matchAll(CitePattern)) {
    const index = Number(match[1]);
    const chunk = reference.chunks[index];
    if (!chunk || byChunk.has(index)) continue;
    let source = sources.find((item) => item.documentId === chunk.document_id);
    if (!source) {
      const guide = chunk.dataset_id === guideDatasetId;
      source = {
        n: sources.length + 1,
        documentId: chunk.document_id,
        datasetId: chunk.dataset_id,
        title: titleOf(chunk.document_name || '未命名文件'),
        place: guide
          ? '平台使用说明'
          : `知识库 · ${datasetName(chunk.dataset_id) ?? '未知'}`,
        guide,
        extension: getExtension(chunk.document_name || ''),
        chunks: [],
      };
      sources.push(source);
    }
    source.chunks.push(chunk);
    byChunk.set(index, source);
  }
  return { sources, byChunk };
}

/** Citations written side by side, e.g. "[ID:2][ID:0]". */
const CiteRunPattern = /\[ID:\s*\d+\](?:\s*\[ID:\s*\d+\])*/g;

/**
 * The words of an answer ready for Markdown: no reasoning, citations as
 * links. Side-by-side citations of one document become a single chip.
 */
export function answerMarkdown(
  text: string,
  byChunk: Map<number, CitedSource>,
) {
  return text
    .replace(/<think>[\s\S]*?<\/think>/g, '')
    .replace(/<think>[\s\S]*$/, '')
    .replace(CiteRunPattern, (run) => {
      const shown = new Set<number>();
      return [...run.matchAll(CitePattern)]
        .map((match) => {
          const source = byChunk.get(Number(match[1]));
          if (!source || shown.has(source.n)) return '';
          shown.add(source.n);
          return `[${source.n}](${CiteHref}${match[1]})`;
        })
        .join('');
    })
    .trim();
}

/** A passage as plain words for a preview. */
export const excerptOf = (chunk?: IReferenceChunk) =>
  String((chunk as { content?: unknown } | undefined)?.content ?? '')
    .replace(/<[^>]+>/g, ' ')
    .replace(/^#+\s*/gm, '')
    .replace(/[*_`>|]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 260);

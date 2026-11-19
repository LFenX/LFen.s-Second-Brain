/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  MarkdownRemarkPlugins,
  MarkdownRemarkPluginsLite,
} from '@/constants/markdown-remark-plugins';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { citationMarkerReg } from '@/utils/citation-utils';
import { getExtension } from '@/utils/document-util';
import { getDirAttribute } from '@/utils/text-direction';
import DOMPurify from 'dompurify';
import { memo, useCallback, useEffect, useMemo } from 'react';
import Markdown from 'react-markdown';
import SyntaxHighlighter from 'react-syntax-highlighter';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import { RehypeSanitizeAssistantMarkdown } from '@/constants/markdown-rehype-plugins';
import { visitParents } from 'unist-util-visit-parents';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import { useLoadingPause } from '@/hooks/use-loading-pause';
import {
  decodeAngleBracketEntities,
  escapeUnmatchedAngleBrackets,
  parseCitationIndex,
  preprocessLaTeX,
  replaceTextByOldReg,
  unescapeAngleBrackets,
} from '@/utils/chat';
import classNames from 'classnames';
import { omit } from 'lodash';
import reactStringReplace from 'react-string-replace';
import { LoadingDots } from '../loading-dots';
import { citationOpenMode, openCitationSource } from './citation-actions';
import { CitationChip, CitationPreview } from './citation-chip';
import styles from './index.module.less';
import { sanitizeHtmlWithImagesAsText } from '@/utils/dom-util';
import { SafeImg } from '@/components/safe-img';

const getChunkIndex = (match: string) => parseCitationIndex(match);
const ReferenceMarkerReg = /(\[(?:ID:)?[0-9٠-٩۰-۹]+\])/g;

// Wraps every text node so citation markers can be replaced by React elements.
// Defined at module scope: react-markdown rebuilds its whole processor whenever
// a plugin's identity changes, and while an answer streams this component
// re-renders many times a second.
const rehypeWrapReference = () => {
  return function wrapTextTransform(tree: any) {
    visitParents(tree, 'text', (node, ancestors) => {
      const latestAncestor = ancestors.at(-1);
      if (
        latestAncestor.tagName !== 'custom-typography' &&
        latestAncestor.tagName !== 'code'
      ) {
        node.type = 'element';
        node.tagName = 'custom-typography';
        node.properties = {};
        node.children = [{ type: 'text', value: node.value }];
      }
    });
  };
};

const MarkdownRehypePlugins = [
  rehypeRaw,
  RehypeSanitizeAssistantMarkdown,
  rehypeWrapReference,
  rehypeKatex,
];

const MarkdownParagraph = ({ children, ...props }: any) => (
  <p {...omit(props, 'node')}>{children}</p>
);

const MarkdownCode = (props: any) => {
  const { children, className, ...rest } = props;
  const restProps = omit(rest, 'node');
  const match = /language-(\w+)/.exec(className || '');
  // Code content is literal for react-markdown, so the &lt;/&gt; entities
  // restored by unescapeAngleBrackets survive parsing; decode them back.
  const codeText = decodeAngleBracketEntities(String(children));
  return match ? (
    <SyntaxHighlighter
      {...restProps}
      PreTag="div"
      language={match[1]}
      wrapLongLines
    >
      {codeText.replace(/\n$/, '')}
    </SyntaxHighlighter>
  ) : (
    <code {...restProps} className={classNames(className, 'text-wrap')}>
      {codeText}
    </code>
  );
};

// TODO: The display of the table is inconsistent with the display previously placed in the MessageItem.
const MarkdownContent = ({
  reference,
  clickDocumentButton,
  content,
  loading,
  disableMath = false,
}: {
  content: string;
  loading: boolean;
  reference: IReference;
  clickDocumentButton?: (documentId: string, chunk: IReferenceChunk) => void;
  /**
   * When true, disables LaTeX math rendering (remark-math + rehype-katex).
   * Use this for user-generated content where `$` should be treated as literal text.
   */
  disableMath?: boolean;
}) => {
  const { setDocumentIds, data: fileThumbnails } =
    useFetchDocumentThumbnailsByIds();
  const contentWithCursor = useMemo(() => {
    // Escape standalone < and > outside matched <...> tags
    // so DOMPurify doesn't strip them as HTML.
    const safeContent = escapeUnmatchedAngleBrackets(content);

    const text = DOMPurify.sanitize(safeContent, {
      ADD_TAGS: ['section', 'details', 'summary'],
      ADD_ATTR: ['class'],
    });

    return unescapeAngleBrackets(preprocessLaTeX(replaceTextByOldReg(text)));
  }, [content]);

  const documentIds = useMemo(() => {
    const docAggs = reference?.doc_aggs;
    return Array.isArray(docAggs) ? docAggs.map((x) => x.doc_id) : [];
  }, [reference?.doc_aggs]);

  // Skipping the empty case matters: this component is mounted once per message,
  // and setting a fresh empty array would re-render (and re-parse the markdown
  // of) every message that carries no reference at all.
  useEffect(() => {
    if (documentIds.length === 0) return;
    setDocumentIds(documentIds);
  }, [documentIds, setDocumentIds]);

  const getReferenceInfo = useCallback(
    (chunkIndex: number) => {
      const chunks = reference?.chunks ?? [];
      const chunkItem = chunks[chunkIndex];
      const document = reference?.doc_aggs?.find(
        (x) => x?.doc_id === chunkItem?.document_id,
      );
      const documentId = document?.doc_id;
      const documentUrl = document?.url;
      const fileThumbnail = documentId ? fileThumbnails[documentId] : '';
      const fileExtension = documentId ? getExtension(document?.doc_name) : '';

      return {
        documentUrl,
        fileThumbnail,
        fileExtension,
        chunkItem,
        documentId,
        document,
      };
    },
    [fileThumbnails, reference],
  );

  const renderReference = useCallback(
    (text: string) => {
      const replacedText = reactStringReplace(
        text,
        ReferenceMarkerReg,
        (match, i) => {
          const chunkIndex = getChunkIndex(match);
          if (typeof chunkIndex !== 'number') {
            return match;
          }
          const hasReference = !!reference?.chunks?.[chunkIndex];
          // Explicit markers can render while their sources are still arriving.
          if (!hasReference && !(loading && match.startsWith('[ID:'))) {
            return match;
          }
          if (!hasReference) {
            return <CitationChip key={i} number={chunkIndex + 1} pending />;
          }

          const {
            documentUrl,
            fileThumbnail,
            fileExtension,
            chunkItem,
            documentId,
            document,
          } = getReferenceInfo(chunkIndex);
          const source = documentId
            ? {
                documentId,
                chunk: chunkItem,
                fileExtension,
                documentUrl,
              }
            : undefined;
          const openMode = source
            ? citationOpenMode(source, clickDocumentButton)
            : undefined;
          const handleOpen = openMode
            ? () => openCitationSource(source!, clickDocumentButton)
            : undefined;

          return (
            <CitationChip
              key={i}
              number={chunkIndex + 1}
              documentId={chunkItem.document_id}
              onOpen={handleOpen}
              preview={
                <CitationPreview
                  number={chunkIndex + 1}
                  documentId={documentId}
                  documentName={document?.doc_name ?? chunkItem.document_name}
                  fileThumbnail={fileThumbnail}
                  fileExtension={fileExtension}
                  imageId={chunkItem.image_id}
                  contentHtml={sanitizeHtmlWithImagesAsText(
                    chunkItem.content ?? '',
                  )}
                  similarity={chunkItem.similarity}
                  vectorSimilarity={chunkItem.vector_similarity}
                  termSimilarity={chunkItem.term_similarity}
                  openMode={openMode}
                  onOpen={handleOpen}
                />
              }
            />
          );
        },
      );

      return replacedText;
    },
    [clickDocumentButton, getReferenceInfo, loading, reference?.chunks],
  );

  const dir = getDirAttribute(content.replace(citationMarkerReg, ''));
  const showLoadingDots = useLoadingPause(loading, content);

  const markdownComponents = useMemo(
    () =>
      ({
        p: MarkdownParagraph,
        'custom-typography': ({ children }: { children: string }) =>
          renderReference(children),
        img: SafeImg,
        code: MarkdownCode,
      }) as any,
    [renderReference],
  );

  return (
    <div dir={dir} className={styles.markdownContentWrapper}>
      <Markdown
        rehypePlugins={
          disableMath
            ? MarkdownRehypePlugins.filter((p) => p !== rehypeKatex)
            : MarkdownRehypePlugins
        }
        remarkPlugins={
          disableMath ? MarkdownRemarkPluginsLite : MarkdownRemarkPlugins
        }
        components={markdownComponents}
      >
        {contentWithCursor}
      </Markdown>
      {showLoadingDots && (
        <LoadingDots className="ml-1 inline-block text-text-secondary" />
      )}
    </div>
  );
};

export default memo(MarkdownContent);

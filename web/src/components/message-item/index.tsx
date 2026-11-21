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

import { AssistantAvatar } from '@/components/anime-avatar';
import { MessageType } from '@/constants/chat';
import { IRegenerateMessage, IRemoveMessageById } from '@/hooks/logic-hooks';
import {
  IMessage,
  IReference,
  IReferenceChunk,
  UploadResponseDataType,
} from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import { memo, useCallback, useMemo } from 'react';
import { DocumentDownloadButton } from '../document-download-button';
import MarkdownContent from '../markdown-content';
import { CitationHighlightProvider } from '../markdown-content/citation-highlight';
import { ReferenceImageList } from '../next-message-item/reference-image-list';
import { UploadedMessageFiles } from '../next-message-item/uploaded-message-files';
import { RAGFlowAvatar } from '../ragflow-avatar';
import { AssistantGroupButton, UserGroupButton } from './group-button';
import styles from './index.module.less';
import {
  parseAnswerMetrics,
  splitAssistantContent,
  summarizeCitations,
} from './reasoning-chain';
import { ReasoningTrace } from './reasoning-trace';
import { ReferenceSourceList } from './reference-source-list';
import { TurnUsage } from './turn-usage';

interface IProps extends Partial<IRemoveMessageById>, IRegenerateMessage {
  item: IMessage;
  reference: IReference;
  loading?: boolean;
  sendLoading?: boolean;
  deleteDisabled?: boolean;
  visibleAvatar?: boolean;
  nickname?: string;
  avatar?: string;
  /** The chat app's own icon, shown beside its answers. */
  assistantAvatar?: string;
  /** The chat searches a knowledge base, so the chain plans retrieval. */
  hasKnowledge?: boolean;
  clickDocumentButton?: (documentId: string, chunk: IReferenceChunk) => void;
  index: number;
  showLikeButton?: boolean;
  showLoudspeaker?: boolean;
}

const MessageItem = ({
  item,
  reference,
  loading = false,
  avatar,
  assistantAvatar,
  sendLoading = false,
  deleteDisabled = false,
  hasKnowledge = false,
  clickDocumentButton,
  index,
  removeMessageById,
  regenerateMessage,
  showLikeButton = true,
  showLoudspeaker = true,
  visibleAvatar = true,
  nickname,
}: IProps) => {
  const isAssistant = item.role === MessageType.Assistant;
  const messageContent = item.content;

  const uploadedFiles = useMemo(() => item?.files ?? [], [item?.files]);
  const documentDownloadInfos = useMemo(
    () => item.downloads ?? [],
    [item.downloads],
  );

  // An assistant message carries its research and reasoning inline; they are
  // shown in the thought process, and only the answer is set as text.
  const parts = useMemo(
    () => (isAssistant ? splitAssistantContent(messageContent) : undefined),
    [isAssistant, messageContent],
  );
  const answerContent = parts ? parts.answer : messageContent;
  const citationText = parts
    ? [parts.thinking, parts.answer].filter(Boolean).join('\n')
    : '';
  const citations = useMemo(
    () => summarizeCitations(citationText, reference),
    [citationText, reference],
  );
  const metrics = useMemo(
    () => (isAssistant ? parseAnswerMetrics(item.prompt) : undefined),
    [isAssistant, item.prompt],
  );
  const showTrace =
    !!parts &&
    (loading ||
      !!parts.thinking ||
      !!parts.research ||
      citations.retrievedChunkCount > 0 ||
      !!metrics);

  const handleRegenerateMessage = useCallback(() => {
    regenerateMessage?.(item);
  }, [regenerateMessage, item]);

  const downloads = documentDownloadInfos.length > 0 && (
    <div className="flex flex-col gap-3">
      {documentDownloadInfos.map((downloadInfo, at) => (
        <DocumentDownloadButton
          key={`${downloadInfo.filename}-${at}`}
          downloadInfo={downloadInfo}
        />
      ))}
    </div>
  );

  if (!isAssistant) {
    return (
      <div className={cn(styles.userTurn, 'group')}>
        <div className={styles.userColumn}>
          {messageContent && (
            <div className={styles.question}>
              <MarkdownContent
                loading={false}
                content={messageContent}
                reference={reference}
                disableMath
              />
            </div>
          )}
          {Array.isArray(uploadedFiles) && uploadedFiles.length > 0 && (
            <UploadedMessageFiles
              files={uploadedFiles as UploadResponseDataType[]}
            />
          )}
          {downloads}
          <UserGroupButton
            content={messageContent}
            messageId={item.id}
            removeMessageById={removeMessageById}
            regenerateMessage={regenerateMessage && handleRegenerateMessage}
            sendLoading={sendLoading}
            deleteDisabled={deleteDisabled}
          />
        </div>
        {visibleAvatar && (
          <RAGFlowAvatar
            className={styles.userAvatar}
            avatar={avatar || undefined}
            name={nickname}
            isPerson
          />
        )}
      </div>
    );
  }

  return (
    <CitationHighlightProvider>
      <div className={cn(styles.assistantTurn, 'group')}>
        {visibleAvatar && (
          <span className={styles.assistantMark}>
            <AssistantAvatar
              src={assistantAvatar}
              thinking={loading}
              className="size-[30px]"
            />
          </span>
        )}
        <div className={styles.assistantColumn}>
          {showTrace && (
            <ReasoningTrace
              live={loading}
              parts={parts}
              metrics={metrics}
              citations={citations}
              reference={reference}
              hasKnowledge={hasKnowledge}
              clickDocumentButton={clickDocumentButton}
            />
          )}
          {answerContent && (
            <div className={styles.answer}>
              <MarkdownContent
                loading={loading}
                content={answerContent}
                reference={reference}
                clickDocumentButton={clickDocumentButton}
              />
            </div>
          )}
          <TurnUsage usage={item.usage} elapsedMs={item.elapsed_ms} />
          <ReferenceImageList
            referenceChunks={reference.chunks}
            messageContent={citationText}
          />
          {!loading && (
            <ReferenceSourceList
              sources={citations.sources}
              reference={reference}
              clickDocumentButton={clickDocumentButton}
            />
          )}
          {downloads}
          {index !== 0 && !loading && (
            <AssistantGroupButton
              messageId={item.id}
              content={answerContent}
              prompt={item.prompt}
              showLikeButton={showLikeButton}
              audioBinary={item.audio_binary}
              showLoudspeaker={showLoudspeaker}
            />
          )}
        </div>
      </div>
    </CitationHighlightProvider>
  );
};

export default memo(MessageItem);

import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import PdfSheet from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { VirtualMessageList } from '@/components/virtual-message-list';
import { MessageType } from '@/constants/chat';
import { useFetchChat, useGetChatSearchParams } from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { IClientConversation } from '@/interfaces/database/chat';
import { useEffect } from 'react';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../../hooks/use-button-disabled';
import { useCreateConversationBeforeUploadDocument } from '../../hooks/use-create-conversation';
import { useSendMessage } from '../../hooks/use-send-chat-message';
import { useMessageReferences } from '../../hooks/use-message-references';
import { EmptyReference } from '../../utils';
import { useShowInternet } from '../use-show-internet';

interface IProps {
  conversation: IClientConversation;
}

export function SingleChatBox({ conversation }: IProps) {
  const {
    value,
    virtualizer,
    messageContainerRef,
    sendLoading,
    messages,
    isUploading,
    handleInputChange,
    handlePressEnter,
    regenerateMessage,
    removeMessageById,
    handleUploadFile,
    removeFile,
    hydrateFromServer,
    stopOutputMessage,
  } = useSendMessage();
  const { data: userInfo } = useFetchUserInfo();
  const { data: currentDialog } = useFetchChat();
  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();
  const { conversationId } = useGetChatSearchParams();
  const disabled = useGetSendButtonDisabled();
  const sendDisabled = useSendButtonDisabled(value);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  const showInternet = useShowInternet();

  const messageReferences = useMessageReferences(messages);

  useEffect(() => {
    // Skip when the conversation prop is stale — its id doesn't match the
    // URL's current conversationId. This happens during a switch (e.g.
    // clicking "+" to create a new session): child effects fire before the
    // parent's clear/load effect, so for one render the prop still holds the
    // previous conversation's messages. Applying them here would leak the old
    // conversation's content into the newly switched (or new) conversation.
    if (conversation?.id && conversation.id !== conversationId) return;

    const messages = conversation?.messages;
    if (Array.isArray(messages)) {
      // hydrateFromServer is a no-op while a stream is in flight, and rejects a
      // server list shorter than the local one, so it can never truncate an
      // answer the server hasn't persisted yet.
      hydrateFromServer(conversationId, messages);
    }
  }, [
    conversation?.messages,
    conversation?.id,
    conversationId,
    hydrateFromServer,
  ]);

  return (
    <section className="flex flex-col h-full gap-2">
      <div
        ref={messageContainerRef}
        className="flex-1 overflow-auto min-h-0 scrollbar-auto px-6 pt-4"
      >
        <VirtualMessageList
          className="mx-auto w-full max-w-[860px]"
          items={messages ?? []}
          virtualizer={virtualizer}
          renderItem={(message, i) => (
            <MessageItem
              loading={
                message.role === MessageType.Assistant &&
                sendLoading &&
                (messages?.length ?? 0) - 1 === i
              }
              nickname={userInfo.nickname}
              avatar={userInfo.avatar}
              hasKnowledge={!!currentDialog.dataset_ids?.length}
              assistantAvatar={currentDialog.icon}
              item={message}
              reference={messageReferences.get(message) ?? EmptyReference}
              clickDocumentButton={clickDocumentButton}
              index={i}
              removeMessageById={removeMessageById}
              deleteDisabled={message.awaitingServerId}
              regenerateMessage={regenerateMessage}
              sendLoading={sendLoading}
            />
          )}
        />
      </div>

      <div className="mx-auto w-full max-w-[908px] px-6 pb-5">
        <NextMessageInput
          disabled={disabled}
          sendDisabled={sendDisabled}
          sendLoading={sendLoading}
          value={value}
          resize="vertical"
          onInputChange={handleInputChange}
          onPressEnter={handlePressEnter}
          conversationId={conversationId}
          createConversationBeforeUploadDocument={
            createConversationBeforeUploadDocument
          }
          stopOutputMessage={stopOutputMessage}
          onUpload={handleUploadFile}
          isUploading={isUploading}
          removeFile={removeFile}
          showReasoning
          showInternet={showInternet}
        />
        {visible && (
          <PdfSheet
            visible={visible}
            hideModal={hideModal}
            documentId={documentId}
            chunk={selectedChunk}
          ></PdfSheet>
        )}
      </div>
    </section>
  );
}

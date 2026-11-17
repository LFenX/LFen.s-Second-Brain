jest.mock('eventsource-parser/stream', () => ({}));
jest.mock('../route-hook', () => ({ useSetPaginationParams: jest.fn() }));
jest.mock('../use-user-setting-request', () => ({ useSaveSetting: jest.fn() }));
jest.mock('../common-hooks', () => ({}));

import { MessageType } from '@/constants/chat';
import type { IClientConversation } from '@/interfaces/database/chat';
import { act, renderHook } from '@testing-library/react';
import { useRemoveMessagesAfterCurrentMessage } from '../logic-hooks';

function conversation(): IClientConversation {
  return {
    id: 'synthetic-conversation',
    chat_id: 'synthetic-chat',
    avatar: '',
    name: 'Synthetic transcript',
    create_date: '',
    create_time: 0,
    update_date: '',
    update_time: 0,
    is_new: true,
    reference: [],
    messages: [
      { id: 'q1', role: MessageType.User, content: 'Question one' },
      {
        id: 'a1',
        role: MessageType.Assistant,
        content: 'Old answer',
        reference: { chunks: [], doc_aggs: [], total: 0 },
        prompt: 'Old prompt',
      },
      { id: 'q2', role: MessageType.User, content: 'Question two' },
    ],
  };
}

it('truncates the current messages field and clears the answer before regeneration', () => {
  const original = conversation();
  let current = original;
  const { result } = renderHook(() =>
    useRemoveMessagesAfterCurrentMessage((update) => {
      current = update(current);
    }),
  );
  act(() => result.current.removeMessagesAfterCurrentMessage('q1'));
  expect(current.messages.map((message) => message.id)).toEqual(['q1', 'a1']);
  expect(current.messages[1]).toMatchObject({
    content: '',
    reference: undefined,
    prompt: undefined,
  });
  expect(original.messages).toHaveLength(3);
  expect(original.messages[1].content).toBe('Old answer');
});

it('keeps the same conversation when the requested message is absent', () => {
  const original = conversation();
  let current = original;
  const { result } = renderHook(() =>
    useRemoveMessagesAfterCurrentMessage((update) => {
      current = update(current);
    }),
  );
  act(() => result.current.removeMessagesAfterCurrentMessage('missing'));
  expect(current).toBe(original);
});

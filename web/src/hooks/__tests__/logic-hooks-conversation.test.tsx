jest.mock('eventsource-parser/stream', () => ({}));
jest.mock('../route-hook', () => ({ useSetPaginationParams: jest.fn() }));
jest.mock('../use-user-setting-request', () => ({ useSaveSetting: jest.fn() }));
jest.mock('../common-hooks', () => ({}));

import { MessageType } from '@/constants/chat';
import type { IMessage } from '@/interfaces/database/chat';
import { act, renderHook } from '@testing-library/react';
import { useSelectDerivedMessages } from '../logic-hooks';

function messages(): IMessage[] {
  return [
    { id: 'q1', role: MessageType.User, content: 'Question one' },
    {
      id: 'a1',
      role: MessageType.Assistant,
      content: 'Old answer',
      reference: { chunks: [], doc_aggs: [], total: 0 },
      prompt: 'Old prompt',
    },
    { id: 'q2', role: MessageType.User, content: 'Question two' },
  ];
}

it('truncates the current messages field and clears the answer before regeneration', () => {
  const original = messages();
  const { result } = renderHook(() => useSelectDerivedMessages());
  act(() => result.current.setDerivedMessages(original));
  act(() => result.current.removeMessagesAfterCurrentMessage('q1'));
  expect(result.current.derivedMessages.map((message) => message.id)).toEqual([
    'q1',
    'a1',
  ]);
  expect(result.current.derivedMessages[1]).toMatchObject({
    content: '',
    reference: undefined,
    prompt: undefined,
  });
  expect(original).toHaveLength(3);
  expect(original[1].content).toBe('Old answer');
});

it('keeps the same conversation when the requested message is absent', () => {
  const original = messages();
  const { result } = renderHook(() => useSelectDerivedMessages());
  act(() => result.current.setDerivedMessages(original));
  act(() => result.current.removeMessagesAfterCurrentMessage('missing'));
  expect(result.current.derivedMessages).toBe(original);
});

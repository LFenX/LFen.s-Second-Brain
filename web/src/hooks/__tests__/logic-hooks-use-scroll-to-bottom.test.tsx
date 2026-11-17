jest.mock('eventsource-parser/stream', () => ({}));
jest.mock('../route-hook', () => ({ useSetPaginationParams: jest.fn() }));
jest.mock('../use-user-setting-request', () => ({ useSaveSetting: jest.fn() }));
jest.mock('../common-hooks', () => ({}));

import { act, fireEvent, renderHook } from '@testing-library/react';
import { useScrollToBottom } from '../logic-hooks';

const InitialMessages: string[] = [];

function createContainer(atBottom = true) {
  const container = document.createElement('div');
  Object.defineProperties(container, {
    clientHeight: { value: 100, configurable: true },
    scrollHeight: { value: 200, configurable: true },
  });
  container.scrollTop = atBottom ? 100 : 0;
  container.scrollTo = jest.fn();
  return { current: container };
}

async function flushFrames() {
  await act(async () => {
    jest.runOnlyPendingTimers();
    await Promise.resolve();
  });
}

describe('useScrollToBottom', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it('recognizes the bottom and disables native scroll anchoring', () => {
    const containerRef = createContainer();
    const { result } = renderHook(() => useScrollToBottom([], containerRef));
    expect(result.current.isAtBottom).toBe(true);
    expect(containerRef.current.style.overflowAnchor).toBe('none');
  });

  it('recognizes a transcript scrolled away from the bottom', () => {
    const containerRef = createContainer(false);
    const { result } = renderHook(() => useScrollToBottom([], containerRef));
    expect(result.current.isAtBottom).toBe(false);
  });

  it('scrolls the actual container when a pinned transcript receives messages', async () => {
    const containerRef = createContainer();
    const { rerender } = renderHook(
      ({ messages }: { messages: string[] }) =>
        useScrollToBottom(messages, containerRef),
      { initialProps: { messages: InitialMessages } },
    );
    await flushFrames();
    jest.mocked(containerRef.current.scrollTo).mockClear();
    rerender({ messages: ['msg1'] });
    await flushFrames();
    expect(containerRef.current.scrollTo).toHaveBeenCalledWith({
      top: 200,
      behavior: 'auto',
    });
  });

  it('does not pull the user down after they scroll up', async () => {
    const containerRef = createContainer();
    const { result, rerender } = renderHook(
      ({ messages }: { messages: string[] }) =>
        useScrollToBottom(messages, containerRef),
      { initialProps: { messages: InitialMessages } },
    );
    await flushFrames();
    jest.mocked(containerRef.current.scrollTo).mockClear();
    act(() => {
      containerRef.current.scrollTop = 0;
      fireEvent.scroll(containerRef.current);
    });
    expect(result.current.isAtBottom).toBe(false);
    rerender({ messages: ['msg1'] });
    await flushFrames();
    expect(containerRef.current.scrollTo).not.toHaveBeenCalled();
  });

  it('stays pinned after content growth but rejects a pending frame after a user scroll', async () => {
    const containerRef = createContainer();
    const { result } = renderHook(() => useScrollToBottom([], containerRef));
    act(() => {
      Object.defineProperty(containerRef.current, 'scrollHeight', {
        value: 400,
        configurable: true,
      });
      fireEvent.scroll(containerRef.current);
    });
    expect(result.current.isAtBottom).toBe(true);
    act(() => {
      containerRef.current.scrollTop = 95;
      fireEvent.scroll(containerRef.current);
    });
    await flushFrames();
    expect(result.current.isAtBottom).toBe(false);
    expect(containerRef.current.scrollTo).not.toHaveBeenCalled();
  });

  it('treats content shorter than the viewport as pinned', () => {
    const containerRef = createContainer(false);
    Object.defineProperty(containerRef.current, 'scrollHeight', { value: 80 });
    const { result } = renderHook(() => useScrollToBottom([], containerRef));
    expect(result.current.isAtBottom).toBe(true);
  });
});

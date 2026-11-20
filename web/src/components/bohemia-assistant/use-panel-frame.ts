import type { PointerEvent as ReactPointerEvent } from 'react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { BohemiaStorageKeys } from './constants';

/*
 * Where 波西米亚's panel sits and how big it is, as in 留白 and lgmon. It opens
 * docked at the bottom right; dragging the header moves it, any edge or corner
 * resizes it, and the expand button (or a double click on the header) makes it
 * tall and wide for long answers. A placed frame is remembered in this browser
 * and always kept inside the window. On a narrow window it is a docked sheet.
 */

export type Frame = { x: number; y: number; w: number; h: number };
export type Edge = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw';
export const Edges: Edge[] = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw'];
type Saved = { custom: Frame | null; expanded: boolean };

const Gap = 12;
const MinWidth = 360;
const MinHeight = 420;
/** Below this width the panel is a sheet and cannot be moved. */
export const DeskWidth = 720;

const valid = (value: unknown): Frame | null => {
  const frame = value as Frame | null;
  return frame &&
    [frame.x, frame.y, frame.w, frame.h].every((n) => Number.isFinite(n))
    ? frame
    : null;
};

function read(): Saved {
  try {
    const value = JSON.parse(
      localStorage.getItem(BohemiaStorageKeys.frame) || 'null',
    );
    if (value && typeof value === 'object') {
      return { custom: valid(value.custom), expanded: !!value.expanded };
    }
  } catch {
    // A private window or a bad value: start docked.
  }
  return { custom: null, expanded: false };
}

function write(saved: Saved) {
  try {
    localStorage.setItem(BohemiaStorageKeys.frame, JSON.stringify(saved));
  } catch {
    // Not kept; it still works for this visit.
  }
}

export function dockedFrame(): Frame {
  const w = Math.min(440, innerWidth - 2 * Gap);
  const h = Math.min(740, innerHeight - 92);
  return { x: innerWidth - w - 20, y: innerHeight - h - 20, w, h };
}

function expandedFrame(): Frame {
  const w = Math.min(960, innerWidth - 48);
  const h = innerHeight - 48;
  return { x: innerWidth - w - 24, y: 24, w, h };
}

/** Kept inside the window and no smaller than the minimum, unless the window is. */
export function clampFrame(frame: Frame): Frame {
  const w = Math.max(
    Math.min(MinWidth, innerWidth - 2 * Gap),
    Math.min(frame.w, innerWidth - 2 * Gap),
  );
  const h = Math.max(
    Math.min(MinHeight, innerHeight - 2 * Gap),
    Math.min(frame.h, innerHeight - 2 * Gap),
  );
  const x = Math.min(Math.max(Gap, frame.x), innerWidth - w - Gap);
  const y = Math.min(Math.max(Gap, frame.y), innerHeight - h - Gap);
  return {
    x: Math.round(x),
    y: Math.round(y),
    w: Math.round(w),
    h: Math.round(h),
  };
}

function resized(start: Frame, mode: Edge, dx: number, dy: number): Frame {
  let { x, y, w, h } = start;
  if (mode.includes('e')) w = start.w + dx;
  if (mode.includes('s')) h = start.h + dy;
  if (mode.includes('w')) {
    w = start.w - dx;
    x = start.x + dx;
  }
  if (mode.includes('n')) {
    h = start.h - dy;
    y = start.y + dy;
  }
  // Shrinking past the minimum from the left or top stops the edge.
  if (w < MinWidth) {
    if (mode.includes('w')) x -= MinWidth - w;
    w = MinWidth;
  }
  if (h < MinHeight) {
    if (mode.includes('n')) y -= MinHeight - h;
    h = MinHeight;
  }
  if (x < Gap) {
    w -= Gap - x;
    x = Gap;
  }
  if (y < Gap) {
    h -= Gap - y;
    y = Gap;
  }
  return clampFrame({
    x,
    y,
    w: Math.min(w, innerWidth - x - Gap),
    h: Math.min(h, innerHeight - y - Gap),
  });
}

export function usePanelFrame() {
  const [saved, setSaved] = useState<Saved>(read);
  const [, setTick] = useState(0);
  const [moving, setMoving] = useState<null | 'move' | Edge>(null);
  const live = useRef(saved);
  useEffect(() => {
    live.current = saved;
  });
  const persist = useCallback((next: Saved) => {
    live.current = next;
    setSaved(next);
    write(next);
  }, []);
  // The window changed size: a docked panel follows; a placed one is kept inside.
  useEffect(() => {
    const handleResize = () => {
      const current = live.current;
      if (current.custom)
        setSaved({ ...current, custom: clampFrame(current.custom) });
      setTick((n) => n + 1);
    };
    addEventListener('resize', handleResize);
    return () => removeEventListener('resize', handleResize);
  }, []);
  const desk = typeof window !== 'undefined' && innerWidth >= DeskWidth;
  const frame = !desk
    ? null
    : saved.custom
      ? clampFrame(saved.custom)
      : saved.expanded
        ? clampFrame(expandedFrame())
        : null;

  /** A move (from the header) or a resize (from an edge), until the pointer is let go. */
  const begin = useCallback(
    (event: ReactPointerEvent, mode: 'move' | Edge) => {
      if (event.button !== 0 || innerWidth < DeskWidth) return;
      const current = live.current;
      const start = current.custom
        ? clampFrame(current.custom)
        : current.expanded
          ? clampFrame(expandedFrame())
          : dockedFrame();
      const from = { x: event.clientX, y: event.clientY };
      const target = event.currentTarget as HTMLElement;
      target.setPointerCapture?.(event.pointerId);
      event.preventDefault();
      setMoving(mode);
      document.documentElement.classList.add('bohemia-dragging');
      let next = start;
      const handleMove = (pointer: PointerEvent) => {
        const dx = pointer.clientX - from.x;
        const dy = pointer.clientY - from.y;
        next =
          mode === 'move'
            ? clampFrame({ ...start, x: start.x + dx, y: start.y + dy })
            : resized(start, mode, dx, dy);
        setSaved((value) => ({ ...value, custom: next }));
      };
      const handleUp = () => {
        target.removeEventListener('pointermove', handleMove);
        target.removeEventListener('pointerup', handleUp);
        target.removeEventListener('pointercancel', handleUp);
        document.documentElement.classList.remove('bohemia-dragging');
        setMoving(null);
        // A click on the header that did not move leaves the panel as it was.
        if (next !== start) persist({ ...live.current, custom: next });
      };
      target.addEventListener('pointermove', handleMove);
      target.addEventListener('pointerup', handleUp);
      target.addEventListener('pointercancel', handleUp);
    },
    [persist],
  );
  const toggleExpanded = useCallback(
    () => persist({ custom: null, expanded: !live.current.expanded }),
    [persist],
  );
  const reset = useCallback(
    () => persist({ custom: null, expanded: false }),
    [persist],
  );
  return {
    frame,
    desk,
    expanded: saved.expanded && !saved.custom,
    moving,
    begin,
    toggleExpanded,
    reset,
  };
}

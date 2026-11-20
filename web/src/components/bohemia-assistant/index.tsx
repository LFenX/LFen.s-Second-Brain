import { BrandMark } from '@/components/brand-mark';
import { cn } from '@/lib/utils';
import { lazy, Suspense, useCallback, useEffect } from 'react';
import { BohemiaName } from './constants';
import styles from './index.module.less';
import { useBohemiaStore } from './store';

/*
 * 波西米亚, the platform guide, as its 第二大脑 分身. The header carries the
 * entry button; the panel loads on first open and stays docked at the bottom
 * right on every page that has the header. Ctrl+J (⌘J) opens and closes it.
 */

const BohemiaPanel = lazy(() => import('./panel'));

export function BohemiaAssistantButton({ compact }: { compact?: boolean }) {
  const open = useBohemiaStore((state) => state.open);
  const busy = useBohemiaStore((state) => state.busy);
  const toggle = useBohemiaStore((state) => state.toggle);
  const handleClick = useCallback(() => toggle(), [toggle]);
  return (
    <button
      type="button"
      className={cn(
        styles.entry,
        open && styles.entryOn,
        compact && styles.entryCompact,
      )}
      aria-pressed={open}
      onClick={handleClick}
      title={`${BohemiaName}：问这个平台怎么用（Ctrl J）`}
      aria-label={BohemiaName}
      data-testid="bohemia-entry"
    >
      <BrandMark size={20} thinking={busy} />
      {!compact && <span>{BohemiaName}</span>}
    </button>
  );
}

export function BohemiaAssistantHost() {
  const open = useBohemiaStore((state) => state.open);
  const toggle = useBohemiaStore((state) => state.toggle);
  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (
        (event.ctrlKey || event.metaKey) &&
        !event.altKey &&
        event.key.toLowerCase() === 'j'
      ) {
        event.preventDefault();
        toggle();
      }
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [toggle]);
  if (!open) return null;
  return (
    <Suspense fallback={null}>
      <BohemiaPanel />
    </Suspense>
  );
}

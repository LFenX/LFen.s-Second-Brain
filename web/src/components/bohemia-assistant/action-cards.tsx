import { cn } from '@/lib/utils';
import {
  ArrowRight,
  Check,
  CircleAlert,
  Compass,
  Database,
  Link2,
  Sparkles,
} from 'lucide-react';
import { useCallback } from 'react';
import { AssistantAction } from './actions';
import { ActionCopy } from './operations';
import styles from './index.module.less';
import { ActionRun } from './store';

/*
 * What an answer offers to do. Navigation is one click. Anything that changes
 * a setting is a card with what it will do and two buttons, as in lgmon; once
 * it ran, the card says what changed and leads to the place it changed.
 */

const ActionIcons: Record<AssistantAction['type'], typeof Compass> = {
  navigate: Compass,
  connect_datasets: Link2,
  create_dataset: Database,
  set_default_model: Sparkles,
};

type ActionCardProps = {
  action: AssistantAction;
  copy: ActionCopy;
  run?: ActionRun;
  onNavigate: (to: string) => void;
  onConfirm: () => void;
  onDecline: () => void;
};

function NavigateCard({
  to,
  copy,
  onNavigate,
}: {
  to: string;
  copy: ActionCopy;
  onNavigate: (to: string) => void;
}) {
  const handleClick = useCallback(() => onNavigate(to), [onNavigate, to]);
  return (
    <button type="button" className={styles.go} onClick={handleClick}>
      <span className={styles.goIcon}>
        <Compass size={14} />
      </span>
      <span className={styles.goText}>
        {copy.title}
        {copy.subject && copy.subject !== copy.title && (
          <small>{copy.subject}</small>
        )}
      </span>
      <ArrowRight size={14} />
    </button>
  );
}

function DoneCard({
  run,
  onNavigate,
}: {
  run: ActionRun;
  onNavigate: (to: string) => void;
}) {
  const href = run.href;
  const handleClick = useCallback(
    () => href && onNavigate(href),
    [href, onNavigate],
  );
  const body = (
    <>
      <span className={styles.changeIcon}>
        <Check size={13} />
      </span>
      <span className={styles.changeText}>{run.message}</span>
      {href && <ArrowRight size={14} />}
    </>
  );
  return href ? (
    <button type="button" className={styles.change} onClick={handleClick}>
      {body}
    </button>
  ) : (
    <div className={styles.change}>{body}</div>
  );
}

export function ActionCard({
  action,
  copy,
  run,
  onNavigate,
  onConfirm,
  onDecline,
}: ActionCardProps) {
  if (action.type === 'navigate') {
    return <NavigateCard to={action.to} copy={copy} onNavigate={onNavigate} />;
  }
  if (run?.state === 'done')
    return <DoneCard run={run} onNavigate={onNavigate} />;
  const Icon = ActionIcons[action.type];
  const settled = run?.state === 'declined';
  const running = run?.state === 'running';
  return (
    <div
      className={cn(styles.ask, settled && styles.askSettled)}
      role="group"
      aria-label={`需要你确认：${copy.title}`}
    >
      <div className={styles.askHead}>
        <span className={styles.askIcon}>
          <Icon size={15} />
        </span>
        <span>
          <b>{copy.title}</b>
          {copy.subject && <q>{copy.subject}</q>}
        </span>
      </div>
      {copy.consequence && <p>{copy.consequence}</p>}
      {run?.state === 'failed' && (
        <p className={styles.askProblem}>
          <CircleAlert size={13} />
          {run.message}
        </p>
      )}
      {settled ? (
        <div className={styles.askDone}>已取消，没有执行</div>
      ) : (
        <div className={styles.askButtons}>
          <button
            type="button"
            className={styles.askNo}
            disabled={running}
            onClick={onDecline}
          >
            取消
          </button>
          <button
            type="button"
            className={styles.askYes}
            disabled={running}
            onClick={onConfirm}
          >
            {running
              ? '正在执行…'
              : run?.state === 'failed'
                ? '重试'
                : `确认${copy.title}`}
          </button>
        </div>
      )}
    </div>
  );
}

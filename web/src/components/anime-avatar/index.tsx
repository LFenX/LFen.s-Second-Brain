import { cn } from '@/lib/utils';
import { AssistantAvatarPreset } from './presets';
import styles from './index.module.less';

/** The assistant's round portrait in a conversation. */
export function AssistantAvatar({
  src,
  thinking = false,
  className,
}: {
  /** The chat app's own icon; falls back to the preset mascot. */
  src?: string;
  /** Shows a slow breathing ring while the assistant is answering. */
  thinking?: boolean;
  className?: string;
}) {
  return (
    <span className={cn(styles.avatar, thinking && styles.thinking, className)}>
      <img
        src={src || AssistantAvatarPreset}
        alt=""
        draggable={false}
        className={styles.image}
      />
    </span>
  );
}

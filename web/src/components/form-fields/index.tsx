import { cn } from '@/lib/utils';
import { Check, ChevronDown, Copy } from 'lucide-react';
import {
  forwardRef,
  useState,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Lightweight, native-backed form pieces for settings-style pages. They keep
 * the browser's own <select>/<input type="checkbox"> semantics (keyboard,
 * screen readers, form events) and only restyle them.
 */

export function Panel({
  title,
  description,
  actions,
  children,
  className,
  ...props
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
} & Omit<React.HTMLAttributes<HTMLElement>, 'title'>) {
  return (
    <section
      className={cn(
        'space-y-4 rounded-3xl border-0.5 border-border-button bg-bg-component p-5',
        className,
      )}
      {...props}
    >
      {(title || actions) && (
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            {title && (
              <h2 className="text-base font-semibold text-text-primary">
                {title}
              </h2>
            )}
            {description && (
              <p className="text-sm leading-relaxed text-text-secondary">
                {description}
              </p>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 items-center gap-2">{actions}</div>
          )}
        </header>
      )}
      {children}
    </section>
  );
}

export function Field({
  label,
  hint,
  children,
  className,
}: {
  label: ReactNode;
  hint?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  // The hint stays outside the <label> so the control's accessible name is
  // the label text alone.
  return (
    <div className={cn('space-y-1.5', className)}>
      <label className="block space-y-1.5">
        <span className="block text-sm font-medium text-text-primary">
          {label}
        </span>
        {children}
      </label>
      {hint && (
        <p className="text-xs leading-relaxed text-text-secondary">{hint}</p>
      )}
    </div>
  );
}

export const FieldSelect = forwardRef<
  HTMLSelectElement,
  SelectHTMLAttributes<HTMLSelectElement>
>(function FieldSelect({ className, children, ...props }, ref) {
  return (
    <span className="relative block">
      <select
        ref={ref}
        className={cn(
          'h-9 w-full appearance-none rounded-lg border-0.5 border-border-button bg-bg-input pe-9 ps-3 text-sm text-text-primary outline-none transition-[background-color,border-color,box-shadow]',
          'hover:border-border-default focus-visible:border-accent-primary/60 focus-visible:bg-bg-component focus-visible:ring-[3px] focus-visible:ring-accent-primary/15',
          'disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDown
        className="pointer-events-none absolute end-3 top-1/2 size-4 -translate-y-1/2 text-text-secondary"
        aria-hidden
      />
    </span>
  );
});

export function CheckRow({
  children,
  className,
  tone = 'plain',
  ...props
}: {
  children: ReactNode;
  className?: string;
  /** "confirm" frames the row as an explicit acknowledgement step. */
  tone?: 'plain' | 'confirm';
} & Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'children'>) {
  return (
    <label
      className={cn(
        'flex cursor-pointer items-start gap-2.5 text-sm text-text-primary',
        tone === 'confirm' &&
          'rounded-xl border-0.5 border-border-button bg-bg-card px-3 py-2.5',
        props.disabled && 'cursor-not-allowed opacity-60',
        className,
      )}
    >
      <input
        type="checkbox"
        className="mt-0.5 size-4 shrink-0 cursor-[inherit] rounded accent-[rgb(var(--accent-primary))]"
        {...props}
      />
      <span className="min-w-0 leading-relaxed">{children}</span>
    </label>
  );
}

export function StatusText({
  children,
  tone = 'muted',
  role,
}: {
  children: ReactNode;
  tone?: 'muted' | 'success' | 'warning';
  role?: 'status' | 'alert';
}) {
  return (
    <p
      role={role}
      className={cn(
        'text-sm leading-relaxed',
        tone === 'muted' && 'text-text-secondary',
        tone === 'success' && 'text-state-success',
        tone === 'warning' && 'text-state-warning',
      )}
    >
      {children}
    </p>
  );
}

/** Monospace value with an inline copy button. */
export function CopyValue({
  value,
  display,
  label,
  className,
}: {
  value: string;
  /** What to show; defaults to the value itself. */
  display?: ReactNode;
  label?: string;
  className?: string;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  }
  return (
    <div
      className={cn(
        'group flex items-start gap-2 rounded-xl border-0.5 border-border-button bg-bg-card px-3 py-2',
        className,
      )}
    >
      <code className="min-w-0 flex-1 whitespace-pre-wrap break-all font-mono text-[12.5px] leading-relaxed text-text-primary">
        {display ?? value}
      </code>
      <button
        type="button"
        onClick={handleCopy}
        className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-text-secondary transition-colors hover:bg-bg-component hover:text-text-primary focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-accent-primary/15"
        aria-label={label ?? t('common.copy')}
        title={label ?? t('common.copy')}
      >
        {copied ? (
          <Check className="size-4 text-state-success" />
        ) : (
          <Copy className="size-4" />
        )}
      </button>
    </div>
  );
}

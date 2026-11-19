import { cn } from '@/lib/utils';
import { CSSProperties, useId } from 'react';
import styles from './index.module.less';
import {
  BubblePagesPath,
  BubblePath,
  CloudPagesPath,
  CloudPath,
  CoreRadius,
} from './paths';

const SparkPath =
  'M0 -10C1.2 -3 3 -1.2 10 0C3 1.2 1.2 3 0 10C-1.2 3 -3 1.2 -10 0C-3 -1.2 -1.2 -3 0 -10Z';
const Sparks = [
  { x: 214, y: 26, scale: 2, color: '#ffc94a' },
  { x: 18, y: 62, scale: 1.5, color: '#ff7ad0' },
  { x: 226, y: 194, scale: 1.7, color: '#4fdcff' },
  { x: 30, y: 214, scale: 1.3, color: '#8be86a' },
];

interface BrandMarkProps {
  size?: number;
  /** While true the two lobes turn about the core and the glow breathes. */
  thinking?: boolean;
  className?: string;
}

/**
 * The animated brand mark. At rest a prismatic sheen crosses it now and then;
 * while thinking the cloud and bubble turn about the orange core. Reduced
 * motion keeps it still.
 */
export function BrandMark({
  size = 20,
  thinking = false,
  className,
}: BrandMarkProps) {
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, '');
  const id = (name: string) => `brand-mark-${uid}-${name}`;

  return (
    <span
      className={cn(
        styles.mark,
        thinking && styles.thinking,
        size < 26 && styles.small,
        className,
      )}
      style={{ '--brand-mark-size': `${size}px` } as CSSProperties}
      aria-hidden="true"
    >
      <svg viewBox="0 0 240 240" width={size} height={size} focusable="false">
        <defs>
          <linearGradient id={id('cloud')} x1="0.1" y1="0" x2="0.8" y2="1">
            <stop offset="0" stopColor="#3b7dff" />
            <stop offset="0.5" stopColor="#0b51d2" />
            <stop offset="1" stopColor="#053aa6" />
          </linearGradient>
          <linearGradient id={id('bubble')} x1="0.2" y1="0" x2="0.7" y2="1">
            <stop offset="0" stopColor="#2fcfdc" />
            <stop offset="0.55" stopColor="#0ea2b9" />
            <stop offset="1" stopColor="#07849e" />
          </linearGradient>
          <radialGradient id={id('core')} cx="0.36" cy="0.32" r="0.8">
            <stop offset="0" stopColor="#ffe08a" />
            <stop offset="0.55" stopColor="#ffa514" />
            <stop offset="1" stopColor="#ff8a00" />
          </radialGradient>
          <linearGradient id={id('sheen')} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0" stopColor="#ff7ad9" stopOpacity="0" />
            <stop offset="0.2" stopColor="#ff7ad9" />
            <stop offset="0.38" stopColor="#ffd36e" />
            <stop offset="0.54" stopColor="#7dffb2" />
            <stop offset="0.7" stopColor="#6ee7ff" />
            <stop offset="0.86" stopColor="#a78bff" />
            <stop offset="1" stopColor="#a78bff" stopOpacity="0" />
          </linearGradient>
          <clipPath id={id('clip')}>
            <path d={CloudPath} />
            <path d={BubblePath} />
          </clipPath>
          <filter id={id('blur')} x="-1" y="-1" width="3" height="3">
            <feGaussianBlur stdDeviation="9" />
          </filter>
        </defs>
        <circle
          className={styles.glow}
          cx="120"
          cy="120"
          r="26"
          fill="#ffb21f"
          filter={`url(#${id('blur')})`}
        />
        <g className={styles.turn}>
          <g className={styles.cloud}>
            <path d={CloudPath} fill={`url(#${id('cloud')})`} />
            <path d={CloudPagesPath} className={styles.page} />
          </g>
          <g className={styles.bubble}>
            <path d={BubblePath} fill={`url(#${id('bubble')})`} />
            <path d={BubblePagesPath} className={styles.page} />
          </g>
          <g clipPath={`url(#${id('clip')})`}>
            <g className={styles.sheen}>
              <rect
                x="0"
                y="-60"
                width="70"
                height="360"
                transform="rotate(24 35 120)"
                fill={`url(#${id('sheen')})`}
              />
            </g>
          </g>
        </g>
        <circle
          className={styles.core}
          cx="120"
          cy="120"
          r={CoreRadius}
          fill={`url(#${id('core')})`}
        />
        <g className={styles.sparks}>
          {Sparks.map((spark, index) => (
            <g
              key={index}
              transform={`translate(${spark.x} ${spark.y}) scale(${spark.scale})`}
            >
              <path
                className={styles.spark}
                style={{ animationDelay: `${index * 1.05}s` }}
                d={SparkPath}
                fill={spark.color}
              />
            </g>
          ))}
        </g>
      </svg>
    </span>
  );
}

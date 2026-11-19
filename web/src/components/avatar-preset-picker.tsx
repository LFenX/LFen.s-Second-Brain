import { cn } from '@/lib/utils';
import { Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  AnimatedAvatarPresets,
  StillAvatarPresets,
} from './anime-avatar/presets';

function PresetGroup({
  title,
  presets,
  value,
  onSelect,
}: {
  title: string;
  presets: string[];
  value?: string;
  onSelect: (src: string) => void;
}) {
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium text-text-secondary">{title}</p>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(44px,1fr))] gap-2">
        {presets.map((src) => (
          <PresetButton
            key={src}
            src={src}
            selected={value === src}
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

function PresetButton({
  src,
  selected,
  onSelect,
}: {
  src: string;
  selected: boolean;
  onSelect: (src: string) => void;
}) {
  function handleClick() {
    onSelect(src);
  }
  return (
    <button
      type="button"
      onClick={handleClick}
      aria-pressed={selected}
      className={cn(
        'relative aspect-square overflow-hidden rounded-full outline-none transition-[box-shadow,transform]',
        'shadow-[0_0_0_0.5px_var(--border-default)] hover:scale-105',
        'focus-visible:ring-[3px] focus-visible:ring-accent-primary/30',
        selected &&
          'shadow-[0_0_0_2px_rgb(var(--accent-primary))] hover:scale-100',
      )}
    >
      <img
        src={src}
        alt=""
        loading="lazy"
        draggable={false}
        className="size-full object-cover"
      />
      {selected && (
        <span className="absolute inset-0 flex items-center justify-center bg-black/25">
          <Check className="size-4 text-white" />
        </span>
      )}
    </button>
  );
}

/** Grid of preset portraits, still and animated. */
export function AvatarPresetPicker({
  value,
  onSelect,
  className,
}: {
  value?: string;
  onSelect: (src: string) => void;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className={cn('max-w-md space-y-3', className)}>
      <PresetGroup
        title={t('setting.avatarPresetsStill')}
        presets={StillAvatarPresets}
        value={value}
        onSelect={onSelect}
      />
      <PresetGroup
        title={t('setting.avatarPresetsAnimated')}
        presets={AnimatedAvatarPresets}
        value={value}
        onSelect={onSelect}
      />
    </div>
  );
}

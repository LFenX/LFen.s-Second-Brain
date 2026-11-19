// Preset avatars from the open-source chat app fiora (github.com/yinxin630/fiora,
// MIT, see public/avatars/fiora/LICENSE). Served from web/public.
const Base = '/avatars/fiora/';

export const StillAvatarPresets = Array.from(
  { length: 15 },
  (_, index) => `${Base}${index}.jpg`,
);

export const AnimatedAvatarPresets = [
  `${Base}wuzeiniang.gif`,
  `${Base}huaji-1.gif`,
  `${Base}huaji-18.gif`,
  `${Base}huaji-33.gif`,
  `${Base}huaji-34.gif`,
  `${Base}huaji-36.gif`,
];

export const AvatarPresets = [...StillAvatarPresets, ...AnimatedAvatarPresets];

/** The assistant speaks as fiora's squid mascot unless the chat has its own icon. */
export const AssistantAvatarPreset = `${Base}wuzeiniang.gif`;

function stringHash(value: string): number {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

/** A stable still preset for someone who has not chosen an avatar. */
export function defaultPersonAvatar(seed?: string): string {
  const key = (seed ?? '').trim().toLowerCase();
  return StillAvatarPresets[stringHash(key) % StillAvatarPresets.length];
}

import { Authorization, Token, UserInfo } from '@/constants/authorization';

let revision = 0;
const listeners = new Set<() => void>();
export const nativeSessionRevision = () => revision;
export function notifyNativeSessionChanged() {
  revision += 1;
  listeners.forEach((listener) => listener());
}
export function subscribeNativeSession(listener: () => void) {
  listeners.add(listener);
  const changed = (event: StorageEvent) => {
    if (
      event.key === null ||
      [Authorization, Token, UserInfo].includes(event.key)
    )
      notifyNativeSessionChanged();
  };
  window.addEventListener('storage', changed);
  return () => {
    listeners.delete(listener);
    window.removeEventListener('storage', changed);
  };
}

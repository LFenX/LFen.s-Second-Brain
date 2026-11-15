import type { AccessActor } from '@/interfaces/database/knowledge-access';

const actor = (value: AccessActor) =>
  [
    'knowledge-access',
    'native',
    value.session,
    value.user,
    value.tenant,
  ] as const;
export const KnowledgeAccessKeys = {
  root: ['knowledge-access'] as const,
  identity: (session: number) =>
    ['knowledge-access', 'identity', session] as const,
  actor,
  modules: (value: AccessActor, domain: string | null) =>
    [...actor(value), 'modules', domain] as const,
  admin: (value: AccessActor, domain: string, module: string) =>
    [...actor(value), 'management', domain, module] as const,
  consumer: (value: AccessActor, module: string) =>
    [...actor(value), 'consumer', module] as const,
  view: (value: AccessActor, module: string, version: number, view: string) =>
    [...actor(value), 'consumer', module, version, view, 'view'] as const,
  search: (
    value: AccessActor,
    module: string,
    version: number,
    view: string,
    query: string,
    limit: number,
  ) =>
    [
      ...actor(value),
      'consumer',
      module,
      version,
      view,
      'search',
      query,
      limit,
    ] as const,
  read: (
    value: AccessActor,
    module: string,
    version: number,
    view: string,
    selector: string,
  ) =>
    [
      ...actor(value),
      'consumer',
      module,
      version,
      view,
      'read',
      selector,
    ] as const,
  isConsumer: (key: readonly unknown[]) =>
    key[0] === 'knowledge-access' && key[5] === 'consumer',
};

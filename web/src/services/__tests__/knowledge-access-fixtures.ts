import Examples from '../../../../docs/knowledge-access-20261108-02/examples.candidate.json';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';
import type {
  AccessActor,
  AccessGrant,
  AccessInspectData,
  AccessModuleAdmin,
  AccessModuleSummary,
  AccessReadData,
  AccessSearchData,
} from '@/interfaces/database/knowledge-access';
import { nativeSessionRevision } from '@/utils/native-session-scope';

export const Domain = '00000000000000000000000000000064';
export const Tenant = '0000000000000000000000000000000a';
export const User = '0000000000000000000000000000000c';
export const ModuleId = '000000000000000000000000000001f5';
export function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value));
}
export function example(id: string): any {
  return clone(Examples.examples.find((item) => item.id === id)!.value);
}
export const actor = (): AccessActor => ({
  user: User,
  tenant: Tenant,
  session: nativeSessionRevision(),
  auth: 'native',
});
export const moduleSummary = (): AccessModuleSummary => ({
  module_id: ModuleId,
  domain_id: Domain,
  name: 'Synthetic shared A',
  version: 1,
  target_count: 1,
  modes: ['read'],
  mcp_delivery_allowed: false,
  enabled: true,
  administrative_owner: false,
});
export function view(): AccessInspectData {
  const value = example('EX03').data as AccessInspectData;
  value.delivery = 'web_only';
  value.expires_at = new Date(Date.now() + 300000).toISOString();
  return value;
}
export const evidence = (): AccessReadData => example('EX06').data;
export function searchResult(catalogue = view()): AccessSearchData {
  const value = example('EX05').data as AccessSearchData;
  value.view_id = catalogue.view_id;
  value.hits.forEach((hit) => {
    hit.evidence_ref.view_id = catalogue.view_id;
  });
  return value;
}
export function grant(): AccessGrant {
  const value = example('EX11').data as AccessGrant;
  value.subject = { kind: 'user', id: User };
  value.delivery = 'web_only';
  return value;
}
export function admin(): AccessModuleAdmin {
  return {
    module_id: ModuleId,
    domain_id: Domain,
    name: 'Synthetic shared A',
    version: 1,
    enabled: true,
    scope: example('EX01').scope,
    grants: [grant()],
    policy: { domain_id: Domain, version: 1, enabled: true },
  };
}
export function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return { client, Wrapper };
}
export function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

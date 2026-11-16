import { useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { clearSharedAccessContent } from './use-knowledge-access';
import type {
  AccessActor,
  AccessCredential,
} from '@/interfaces/database/knowledge-access';
import type { AccessRequests } from '@/interfaces/request/knowledge-access';
import service, {
  AccessRequestError,
  accessRequestValid,
} from '@/services/knowledge-access-service';
import {
  accessBytes,
  accessCanonical,
  accessSchemaValid,
  parseAccessJSON,
} from '@/utils/knowledge-access-schema';
import {
  nativeSessionRevision,
  subscribeNativeSession,
} from '@/utils/native-session-scope';

type CredentialScope = { domain: string; module: string; version: number };
export type CredentialCommand =
  | {
      operation: 'createCredential';
      scope: CredentialScope;
      body: AccessRequests['createCredential'];
    }
  | {
      operation: 'revokeCredential';
      scope: CredentialScope;
      body: AccessRequests['revokeCredential'];
    };
type SafeState = {
  boundary: string;
  pending: CredentialCommand | null;
  busy: boolean;
  blocked: boolean;
  needsReview: boolean;
  error: unknown;
  completed: boolean;
  receipt: AccessCredential | null;
  delivery: 'issued_once' | 'unavailable_after_first_response' | null;
};

export function credentialCommandKey(actor: AccessActor, domain: string) {
  return (
    'knowledge-access-credential-command:' +
    accessCanonical([actor.auth, actor.user, domain])
  );
}
function validCommand(value: CredentialCommand, domain: string): boolean {
  return (
    !!value &&
    Object.keys(value).sort().join(',') === 'body,operation,scope' &&
    !!value.scope &&
    Object.keys(value.scope).sort().join(',') === 'domain,module,version' &&
    value.scope.domain === domain &&
    accessSchemaValid('ID', domain) &&
    accessSchemaValid('ID', value.scope.module) &&
    Number.isSafeInteger(value.scope.version) &&
    value.scope.version >= 1 &&
    ['createCredential', 'revokeCredential'].includes(value.operation) &&
    accessRequestValid(value.operation, value.body)
  );
}
function readPending(key: string, domain: string): CredentialCommand | null {
  const raw = sessionStorage.getItem(key);
  if (raw === null) return null;
  if (accessBytes(raw) > 32768) throw new Error('invalid_credential_command');
  const value = parseAccessJSON(raw) as CredentialCommand;
  if (!validCommand(value, domain))
    throw new Error('invalid_credential_command');
  return value;
}

// Only the request and safe receipt enter hook state or session storage. The
// first token is delivered directly to the mounted component, never QueryClient.
export function useCredentialCommand(
  actor: AccessActor,
  domain: string,
  deliver: (token: string, credential: AccessCredential) => void,
) {
  const client = useQueryClient();
  const session = useSyncExternalStore(
    subscribeNativeSession,
    nativeSessionRevision,
    nativeSessionRevision,
  );
  const key = credentialCommandKey(actor, domain),
    boundary = accessCanonical([key, actor.session, session]);
  const current = useRef({ boundary, mounted: true, busy: false });
  current.current.boundary = boundary;
  const initial = (): SafeState => {
    let pending: CredentialCommand | null = null,
      blocked = false;
    try {
      if (accessSchemaValid('ID', domain)) pending = readPending(key, domain);
    } catch {
      blocked = true;
    }
    return {
      boundary,
      pending,
      busy: false,
      blocked,
      needsReview: false,
      error: null,
      completed: false,
      receipt: null,
      delivery: null,
    };
  };
  const [state, setState] = useState<SafeState>(initial);
  useLayoutEffect(() => {
    current.current.mounted = true;
    current.current.busy = false;
    setState(initial());
    const lifetime = current.current;
    return () => {
      lifetime.mounted = false;
    };
    // The boundary includes native session generation, not a bearer value.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [boundary]);
  const visible = state.boundary === boundary ? state : initial();
  const live = (expected: string) =>
    current.current.mounted &&
    current.current.boundary === expected &&
    nativeSessionRevision() === actor.session;
  async function send(command: CredentialCommand) {
    if (current.current.busy || !live(boundary)) return false;
    const expected = boundary;
    const same = () =>
      accessCanonical(readPending(key, domain)) === accessCanonical(command);
    try {
      if (!same()) throw new Error('credential_command_changed');
    } catch {
      setState({ ...visible, blocked: true });
      return false;
    }
    current.current.busy = true;
    setState({
      ...visible,
      pending: command,
      busy: true,
      completed: false,
      error: null,
    });
    let transient: string | null = null;
    try {
      let receipt: AccessCredential | null = null,
        delivery: SafeState['delivery'] = null;
      if (command.operation === 'createCredential') {
        const value = await service.createCredential(command.body);
        transient = value.token;
        value.token = null;
        receipt = value.credential;
        delivery = value.delivery;
        if (
          receipt.domain_id !== command.scope.domain ||
          receipt.module_id !== command.scope.module ||
          receipt.module_version !== command.scope.version
        )
          throw new AccessRequestError(502, 'invalid_response', null, true);
      } else await service.revokeCredential(command.body);
      if (!live(expected)) return false;
      if (!same()) throw new Error('credential_command_changed');
      sessionStorage.removeItem(key);
      setState({ ...initial(), completed: true, receipt, delivery });
      if (transient !== null && receipt !== null && live(expected))
        deliver(transient, receipt);
      return true;
    } catch (error) {
      if (!live(expected)) return false;
      const known =
        error instanceof AccessRequestError &&
        error.outcomeKnown === true &&
        !error.retrySameKey;
      if (
        error instanceof AccessRequestError &&
        [401, 404, 409].includes(error.status)
      )
        clearSharedAccessContent(client);
      let blocked = false;
      try {
        if (!same()) blocked = true;
        else if (known) sessionStorage.removeItem(key);
      } catch {
        blocked = true;
      }
      setState({
        ...initial(),
        pending: known && !blocked ? null : command,
        busy: false,
        blocked,
        needsReview: known,
        error,
      });
      return false;
    } finally {
      transient = null;
      if (live(expected)) current.current.busy = false;
    }
  }
  async function start(command: CredentialCommand) {
    if (
      visible.pending ||
      visible.busy ||
      visible.blocked ||
      visible.needsReview ||
      !live(boundary)
    )
      return false;
    try {
      const immutable = parseAccessJSON(
        JSON.stringify(command),
      ) as CredentialCommand;
      if (!validCommand(immutable, domain))
        throw new AccessRequestError(400, 'invalid_request', true);
      if (readPending(key, domain) !== null)
        throw new Error('credential_command_changed');
      sessionStorage.setItem(key, JSON.stringify(immutable));
      return await send(immutable);
    } catch (error) {
      if (live(boundary))
        setState({
          ...visible,
          error,
          blocked: !(error instanceof AccessRequestError),
        });
      return false;
    }
  }
  return {
    ...visible,
    start,
    recover: () =>
      visible.pending ? send(visible.pending) : Promise.resolve(false),
    review: () => {
      if (!visible.pending && !visible.blocked && !visible.busy)
        setState(initial());
    },
    locked:
      !!visible.pending ||
      visible.busy ||
      visible.blocked ||
      visible.needsReview,
  };
}

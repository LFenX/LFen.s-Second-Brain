import { useLayoutEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type {
  AccessActor,
  AccessResults,
} from '@/interfaces/database/knowledge-access';
import type {
  AccessRequests,
  AccessWriteOperation,
} from '@/interfaces/request/knowledge-access';
import service, {
  AccessRequestError,
  accessRequestValid,
} from '@/services/knowledge-access-service';
import {
  accessBytes,
  accessCanonical,
  parseAccessJSON,
} from '@/utils/knowledge-access-schema';
import { nativeSessionRevision } from '@/utils/native-session-scope';
import { KnowledgeAccessKeys } from './knowledge-access-keys';
import { clearSharedAccessContent } from './use-knowledge-access';

export type AccessCommand = {
  [O in AccessWriteOperation]: { operation: O; body: AccessRequests[O] };
}[AccessWriteOperation];
type CommandState = {
  scope: string;
  pending: AccessCommand | null;
  busy: boolean;
  error: unknown;
  completed: boolean;
  needsReview: boolean;
  storageBlocked: boolean;
};
const Operations: AccessWriteOperation[] = [
  'setPolicy',
  'setModule',
  'createGrant',
  'revokeGrant',
];
function frozen<T>(value: T): T {
  if (value && typeof value === 'object') {
    Object.values(value).forEach(frozen);
    Object.freeze(value);
  }
  return value;
}
export function accessCommandKey(actor: AccessActor, domain: string) {
  return (
    'knowledge-access-command:' +
    accessCanonical([actor.auth, actor.user, actor.tenant, domain])
  );
}
function savedCommand(key: string, domain: string): AccessCommand | null {
  const raw = sessionStorage.getItem(key);
  if (raw === null) return null;
  if (accessBytes(raw) > 65536) throw new Error('invalid_command_storage');
  const value = parseAccessJSON(raw) as AccessCommand;
  if (
    !value ||
    Object.keys(value).sort().join(',') !== 'body,operation' ||
    !Operations.includes(value.operation) ||
    value.body?.domain_id !== domain ||
    !accessRequestValid(value.operation, value.body)
  )
    throw new Error('invalid_command_storage');
  return frozen(value);
}
async function execute(
  command: AccessCommand,
): Promise<AccessResults[AccessWriteOperation]> {
  switch (command.operation) {
    case 'setPolicy':
      return service.setPolicy(command.body);
    case 'setModule':
      return service.setModule(command.body);
    case 'createGrant':
      return service.createGrant(command.body);
    case 'revokeGrant':
      return service.revokeGrant(command.body);
  }
}

export function useAccessCommand(actor: AccessActor, domain: string) {
  const client = useQueryClient();
  const storageKey = accessCommandKey(actor, domain);
  const boundary = accessCanonical([storageKey, actor.session]);
  const current = useRef({ boundary, mounted: true, busy: false });
  current.current.boundary = boundary;
  const initial = (): CommandState => {
    try {
      return {
        scope: boundary,
        pending: savedCommand(storageKey, domain),
        busy: false,
        error: null,
        completed: false,
        needsReview: false,
        storageBlocked: false,
      };
    } catch {
      return {
        scope: boundary,
        pending: null,
        busy: false,
        error: new Error('command_storage_unavailable'),
        completed: false,
        needsReview: false,
        storageBlocked: true,
      };
    }
  };
  const [state, setState] = useState<CommandState>(initial);
  useLayoutEffect(() => {
    const lifetime = current.current;
    lifetime.mounted = true;
    lifetime.busy = false;
    setState(initial());
    return () => {
      lifetime.mounted = false;
    };
    // The boundary includes identity, tenant, domain and native session generation.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [boundary]);
  const visible = state.scope === boundary ? state : initial();
  const live = (expected: string) =>
    current.current.mounted &&
    current.current.boundary === expected &&
    nativeSessionRevision() === actor.session;
  async function send(command: AccessCommand) {
    if (current.current.busy || !live(boundary)) return false;
    const sameSaved = () =>
      accessCanonical(savedCommand(storageKey, domain)) ===
      accessCanonical(command);
    try {
      if (!sameSaved()) throw new Error('command_storage_changed');
    } catch (error) {
      setState({ ...visible, pending: command, error, storageBlocked: true });
      return false;
    }
    const expected = boundary;
    current.current.busy = true;
    setState({
      ...visible,
      pending: command,
      busy: true,
      error: null,
      completed: false,
    });
    clearSharedAccessContent(client);
    try {
      await execute(command);
      if (!live(expected)) return false;
      if (!sameSaved()) throw new Error('command_storage_changed');
      sessionStorage.removeItem(storageKey);
      clearSharedAccessContent(client);
      await client.invalidateQueries({
        queryKey: KnowledgeAccessKeys.actor(actor),
      });
      if (!live(expected)) return false;
      setState({ ...initial(), completed: true });
      return true;
    } catch (error) {
      if (!live(expected)) return false;
      const known =
        error instanceof AccessRequestError &&
        error.outcomeKnown === true &&
        !error.retrySameKey;
      let storageBlocked = false;
      try {
        storageBlocked = !sameSaved();
      } catch {
        storageBlocked = true;
      }
      if (known) {
        try {
          if (!storageBlocked) sessionStorage.removeItem(storageKey);
        } catch {
          storageBlocked = true;
        }
        clearSharedAccessContent(client);
        await client.invalidateQueries({
          queryKey: KnowledgeAccessKeys.actor(actor),
        });
        if (!live(expected)) return false;
      }
      const pending = known && !storageBlocked ? null : command;
      setState({
        scope: boundary,
        pending,
        busy: false,
        completed: false,
        storageBlocked,
        error,
        needsReview: known,
      });
      return false;
    } finally {
      if (live(expected)) current.current.busy = false;
    }
  }
  async function start(command: AccessCommand) {
    if (
      visible.busy ||
      visible.pending ||
      visible.needsReview ||
      visible.storageBlocked ||
      !live(boundary)
    )
      return false;
    let immutable: AccessCommand;
    try {
      if (savedCommand(storageKey, domain)) {
        setState({ ...visible, storageBlocked: true });
        return false;
      }
      immutable = frozen(
        parseAccessJSON(JSON.stringify(command)) as AccessCommand,
      );
      if (
        immutable.body.domain_id !== domain ||
        !accessRequestValid(immutable.operation, immutable.body)
      )
        throw new AccessRequestError(400, 'invalid_request', true);
      sessionStorage.setItem(storageKey, JSON.stringify(immutable));
    } catch (error) {
      setState({
        ...visible,
        error,
        storageBlocked: !(error instanceof AccessRequestError),
      });
      return false;
    }
    return send(immutable);
  }
  function recover() {
    return visible.pending ? send(visible.pending) : Promise.resolve(false);
  }
  async function reviewLatest() {
    if (visible.pending || visible.storageBlocked || visible.busy) return;
    await client.invalidateQueries({
      queryKey: KnowledgeAccessKeys.actor(actor),
    });
    if (live(boundary)) setState(initial());
  }
  return {
    ...visible,
    start,
    recover,
    reviewLatest,
    locked:
      visible.busy ||
      !!visible.pending ||
      visible.needsReview ||
      visible.storageBlocked,
  };
}

export type AccessCommandController = ReturnType<typeof useAccessCommand>;

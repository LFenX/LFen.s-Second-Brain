import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type FormEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { clearSharedAccessContent } from '@/hooks/use-knowledge-access';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AccessFailure } from '@/components/knowledge-access-feedback';
import { useCredentialCommand } from '@/hooks/use-knowledge-credential-command';
import {
  KnowledgeAccessVersion,
  type AccessActor,
  type AccessCredential,
  type AccessInspectData,
  type AccessMode,
  type AccessModuleSummary,
} from '@/interfaces/database/knowledge-access';
import service, {
  AccessRequestError,
} from '@/services/knowledge-access-service';
import {
  accessCanonical,
  accessSchemaValid,
} from '@/utils/knowledge-access-schema';
import {
  nativeSessionRevision,
  subscribeNativeSession,
} from '@/utils/native-session-scope';

const endpointPath = '/api/v1/knowledge/access/mcp';
export function knowledgeAccessMCPEndpoint(origin: string): string | null {
  try {
    const url = new URL(origin);
    if (
      url.origin !== origin ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      url.pathname !== '/' ||
      (url.protocol !== 'https:' &&
        !(
          url.protocol === 'http:' &&
          ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
        ))
    )
      return null;
    return origin + endpointPath;
  } catch {
    return null;
  }
}

function CredentialFeedback({
  command,
}: {
  command: ReturnType<typeof useCredentialCommand>;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" aria-live="polite">
      {command.pending && (
        <>
          <p>
            {t(
              command.busy
                ? 'knowledgeAccess.saving'
                : 'knowledgeAccess.credentialUnknown',
            )}
          </p>
          <p className="break-all">
            {t('knowledgeAccess.grantScope', {
              domain: command.pending.scope.domain,
              module: command.pending.scope.module,
              version: command.pending.scope.version,
            })}
          </p>
          <Button
            variant="outline"
            disabled={command.busy || command.blocked}
            onClick={() => {
              void command.recover();
            }}
          >
            {t('knowledgeAccess.checkOriginal')}
          </Button>
        </>
      )}
      {command.blocked && (
        <p role="alert">{t('knowledgeAccess.storageBlocked')}</p>
      )}
      {!command.pending && <AccessFailure error={command.error} />}
      {command.needsReview && (
        <Button variant="outline" onClick={command.review}>
          {t('knowledgeAccess.reviewLatest')}
        </Button>
      )}
      {command.completed && <p>{t('knowledgeAccess.saved')}</p>}
    </div>
  );
}

export function MintMCPCredential({
  actor,
  module,
}: {
  actor: AccessActor;
  module: AccessModuleSummary;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const session = useSyncExternalStore(
    subscribeNativeSession,
    nativeSessionRevision,
    nativeSessionRevision,
  );
  const boundary = accessCanonical([
    actor,
    session,
    module.module_id,
    module.version,
    module.modes,
    module.mcp_delivery_allowed,
  ]);
  const [mode, setMode] = useState<AccessMode>('read'),
    [ttl, setTTL] = useState(3600),
    [label, setLabel] = useState(''),
    [confirmed, setConfirmed] = useState(false);
  const [view, setView] = useState<AccessInspectData | null>(null),
    [catalog, setCatalog] = useState<AccessInspectData['targets']>([]),
    [sources, setSources] = useState<string[]>([]);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [revealed, setRevealed] = useState(false),
    [hasToken, setHasToken] = useState(false),
    [copied, setCopied] = useState(false);
  const secret = useRef<{ boundary: string; token: string } | null>(null),
    request = useRef<AbortController | null>(null),
    lifetime = useRef({ boundary, mounted: true, cycle: 0 });
  lifetime.current.boundary = boundary;
  const endpoint = knowledgeAccessMCPEndpoint(window.location.origin);
  function closeDelivery() {
    secret.current = null;
    setHasToken(false);
    setRevealed(false);
    setCopied(false);
  }
  const command = useCredentialCommand(
    actor,
    module.domain_id,
    (token, credential) => {
      if (
        lifetime.current.mounted &&
        lifetime.current.boundary === boundary &&
        nativeSessionRevision() === actor.session &&
        credential.module_id === module.module_id &&
        credential.module_version === module.version
      ) {
        secret.current = { boundary, token };
        setRevealed(false);
        setCopied(false);
        setHasToken(true);
      }
    },
  );
  const ownCommand =
    command.pending === null ||
    (command.pending.scope.module === module.module_id &&
      command.pending.scope.version === module.version);
  const live = (expected: string, cycle: number) =>
    lifetime.current.mounted &&
    lifetime.current.boundary === expected &&
    lifetime.current.cycle === cycle &&
    nativeSessionRevision() === actor.session;
  useLayoutEffect(() => {
    lifetime.current.mounted = true;
    lifetime.current.cycle += 1;
    request.current?.abort();
    closeDelivery();
    setView(null);
    setCatalog([]);
    setSources([]);
    setConfirmed(false);
    setError(null);
    setBusy(false);
    const current = lifetime.current;
    return () => {
      current.mounted = false;
      current.cycle += 1;
      request.current?.abort();
      secret.current = null;
    };
    // Source/view state belongs to this exact native identity and module version.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [boundary]);
  useEffect(() => {
    if (!view) return;
    const clear = () => {
      request.current?.abort();
      setView(null);
      setConfirmed(false);
      closeDelivery();
    };
    const delay = Date.parse(view.expires_at) - Date.now();
    if (delay <= 0) {
      clear();
      return;
    }
    const timer = setTimeout(clear, delay);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view]);
  useEffect(() => {
    if (
      command.error instanceof AccessRequestError &&
      [401, 404, 409].includes(command.error.status)
    ) {
      request.current?.abort();
      setView(null);
      setCatalog([]);
      setSources([]);
      setConfirmed(false);
      closeDelivery();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [command.error]);
  function changeScope() {
    lifetime.current.cycle += 1;
    request.current?.abort();
    setView(null);
    setConfirmed(false);
    setError(null);
    setBusy(false);
    closeDelivery();
  }
  async function prepare() {
    if (busy || command.locked || !module.mcp_delivery_allowed || !endpoint)
      return;
    changeScope();
    const cycle = lifetime.current.cycle,
      expected = boundary,
      controller = new AbortController();
    request.current = controller;
    setBusy(true);
    try {
      const result = await service.inspect(
        {
          contract_version: KnowledgeAccessVersion,
          domain_id: module.domain_id,
          module_id: module.module_id,
          mode,
          delivery: 'web_and_mcp',
          source_ids: catalog.length ? sources : null,
        },
        controller.signal,
      );
      if (!live(expected, cycle)) return;
      if (
        result.module_version !== module.version ||
        Date.parse(result.expires_at) <= Date.now()
      )
        throw new AccessRequestError(409, 'scope_changed', true);
      setView(result);
      if (!catalog.length) {
        setCatalog(result.targets);
        setSources(result.targets.map((item) => item.target.source.source_id));
      }
    } catch (issue) {
      if (live(expected, cycle)) {
        setError(issue);
        setView(null);
        closeDelivery();
        if (
          issue instanceof AccessRequestError &&
          [401, 404, 409].includes(issue.status)
        ) {
          setCatalog([]);
          setSources([]);
          clearSharedAccessContent(client);
        }
      }
    } finally {
      if (live(expected, cycle)) setBusy(false);
    }
  }
  function mint(event: FormEvent) {
    event.preventDefault();
    if (
      !view ||
      !confirmed ||
      command.locked ||
      !endpoint ||
      Date.parse(view.expires_at) <= Date.now()
    )
      return;
    closeDelivery();
    setConfirmed(false);
    void command.start({
      operation: 'createCredential',
      scope: {
        domain: module.domain_id,
        module: module.module_id,
        version: module.version,
      },
      body: {
        contract_version: KnowledgeAccessVersion,
        view_id: view.view_id,
        mode,
        ttl_seconds: ttl,
        client_label: label,
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
  }
  async function copy() {
    const value = secret.current;
    if (
      !value ||
      value.boundary !== boundary ||
      nativeSessionRevision() !== actor.session
    )
      return;
    try {
      await navigator.clipboard.writeText(value.token);
      if (secret.current === value && lifetime.current.mounted) setCopied(true);
    } catch {
      if (secret.current === value)
        setError(new Error('clipboard_unavailable'));
    }
  }
  const currentSecret =
    session === actor.session && secret.current?.boundary === boundary
      ? secret.current
      : null;
  if (!module.mcp_delivery_allowed)
    return <p>{t('knowledgeAccess.noMCPGrant')}</p>;
  return (
    <section
      className="space-y-4 rounded border border-border-button p-4"
      aria-label={t('knowledgeAccess.temporaryMCP')}
    >
      <h2 className="text-lg font-medium">
        {t('knowledgeAccess.temporaryMCP')}
      </h2>
      <p>
        {t('knowledgeAccess.credentialOwner')}:{' '}
        <code className="break-all">{actor.user}</code>
      </p>
      <p>
        {t('knowledgeAccess.mcpEndpoint')}:{' '}
        <code className="break-all">
          {endpoint ?? t('knowledgeAccess.unavailable')}
        </code>
      </p>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.mcpClientHint')}
      </p>
      <label className="block space-y-1">
        <span>{t('knowledgeAccess.mode')}</span>
        <select
          className="rounded border border-border-button bg-bg-base p-2"
          value={mode}
          disabled={command.locked || busy}
          onChange={(event) => {
            changeScope();
            setMode(event.target.value as AccessMode);
            setCatalog([]);
            setSources([]);
          }}
        >
          <option value="read">{t('knowledgeAccess.readOnly')}</option>
          {module.modes.includes('read_search') && (
            <option value="read_search">
              {t('knowledgeAccess.readSearch')}
            </option>
          )}
        </select>
      </label>
      {catalog.length > 0 && (
        <fieldset disabled={command.locked || busy} className="space-y-2">
          <legend>{t('knowledgeAccess.credentialSources')}</legend>
          {catalog.map(({ target }) => (
            <label className="block break-all" key={target.source.source_id}>
              <input
                type="checkbox"
                checked={sources.includes(target.source.source_id)}
                onChange={(event) => {
                  changeScope();
                  setSources((values) =>
                    event.target.checked
                      ? [...values, target.source.source_id]
                      : values.filter((id) => id !== target.source.source_id),
                  );
                }}
              />{' '}
              <code>
                {target.source.source_id} / {target.source.source_revision_id} /{' '}
                {target.source.source_sha256}
              </code>
            </label>
          ))}
        </fieldset>
      )}
      <Button
        variant="outline"
        disabled={
          busy ||
          command.locked ||
          !endpoint ||
          (catalog.length > 0 && sources.length === 0)
        }
        onClick={() => {
          void prepare();
        }}
      >
        {t('knowledgeAccess.prepareMCP')}
      </Button>
      <AccessFailure error={error} />
      {view && (
        <div
          className="space-y-2"
          role="group"
          aria-label={t('knowledgeAccess.credentialReview')}
        >
          <p>
            {t('knowledgeAccess.grantScope', {
              domain: view.domain_id,
              module: view.module_id,
              version: view.module_version,
            })}
          </p>
          <p>
            {t('knowledgeAccess.credentialScopeCount', {
              sources: view.source_count,
              units: view.unit_count,
            })}
          </p>
          <p>{t('knowledgeAccess.expires', { time: view.expires_at })}</p>
        </div>
      )}
      <form onSubmit={mint} className="space-y-3">
        <label className="block space-y-1">
          <span>{t('knowledgeAccess.clientLabel')}</span>
          <Input
            value={label}
            required
            maxLength={128}
            autoComplete="off"
            disabled={command.locked || busy}
            onChange={(event) => {
              closeDelivery();
              setLabel(event.target.value);
              setConfirmed(false);
            }}
          />
        </label>
        <p className="text-sm text-text-secondary">
          {t('knowledgeAccess.clientLabelHint')}
        </p>
        <label className="block space-y-1">
          <span>{t('knowledgeAccess.credentialTTL')}</span>
          <Input
            type="number"
            min={60}
            max={28800}
            step={1}
            value={ttl}
            disabled={command.locked || busy}
            onChange={(event) => {
              closeDelivery();
              setTTL(Number(event.target.value));
              setConfirmed(false);
            }}
          />
        </label>
        <p className="text-sm text-text-secondary">
          {t('knowledgeAccess.credentialTTLHint')}
        </p>
        <label className="block space-x-2">
          <input
            type="checkbox"
            checked={confirmed}
            disabled={!view || command.locked || busy}
            onChange={(event) => setConfirmed(event.target.checked)}
          />
          <span>{t('knowledgeAccess.confirmCredential')}</span>
        </label>
        <Button
          type="submit"
          disabled={
            !view ||
            !confirmed ||
            !label.trim() ||
            command.locked ||
            busy ||
            !ownCommand
          }
        >
          {t('knowledgeAccess.createCredential')}
        </Button>
      </form>
      <CredentialFeedback command={command} />
      {command.receipt && (
        <p>
          <span>{t('knowledgeAccess.credentialID')}: </span>
          <code className="break-all">
            {command.receipt.credential_id}
          </code> —{' '}
          {command.receipt.expires_at}
        </p>
      )}
      {command.delivery !== null && !hasToken && (
        <p role="status">{t('knowledgeAccess.tokenUnavailable')}</p>
      )}
      {hasToken && currentSecret && (
        <div className="space-y-2 rounded border border-border-button p-3">
          <p>{t('knowledgeAccess.tokenOnce')}</p>
          {revealed ? (
            <code className="block break-all" data-testid="one-time-token">
              {currentSecret.token}
            </code>
          ) : (
            <Button variant="outline" onClick={() => setRevealed(true)}>
              {t('knowledgeAccess.showToken')}
            </Button>
          )}
          <Button
            variant="outline"
            onClick={() => {
              void copy();
            }}
          >
            {t('knowledgeAccess.copyToken')}
          </Button>
          <Button variant="outline" onClick={closeDelivery}>
            {t('knowledgeAccess.closeToken')}
          </Button>
          {copied && <p role="status">{t('knowledgeAccess.tokenCopied')}</p>}
        </div>
      )}
    </section>
  );
}

export function OwnMCPCredentials({ actor }: { actor: AccessActor }) {
  const { t } = useTranslation();
  const [domain, setDomain] = useState(''),
    [items, setItems] = useState<AccessCredential[]>([]),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(null),
    [confirmation, setConfirmation] = useState('');
  const current = useRef({ mounted: true, cycle: 0, domain });
  current.current.domain = domain;
  const request = useRef<AbortController | null>(null);
  const command = useCredentialCommand(actor, domain, () => {});
  useLayoutEffect(() => {
    current.current.mounted = true;
    return () => {
      current.current.mounted = false;
      current.current.cycle += 1;
      request.current?.abort();
    };
  }, []);
  function clear() {
    current.current.cycle += 1;
    request.current?.abort();
    setItems([]);
    setError(null);
    setConfirmation('');
    setBusy(false);
  }
  async function list() {
    if (!accessSchemaValid('ID', domain) || busy) return;
    clear();
    const cycle = current.current.cycle,
      expected = domain,
      controller = new AbortController();
    request.current = controller;
    setBusy(true);
    try {
      const value = await service.listCredentials(
        {
          contract_version: KnowledgeAccessVersion,
          domain_id: domain,
          idempotency_key: null,
        },
        controller.signal,
      );
      if (
        current.current.mounted &&
        current.current.cycle === cycle &&
        current.current.domain === expected &&
        nativeSessionRevision() === actor.session
      )
        setItems(value.items);
    } catch (issue) {
      if (current.current.mounted && current.current.cycle === cycle)
        setError(issue);
    } finally {
      if (current.current.mounted && current.current.cycle === cycle)
        setBusy(false);
    }
  }
  async function revoke(item: AccessCredential) {
    if (confirmation !== item.credential_id || command.locked) return;
    clear();
    await command.start({
      operation: 'revokeCredential',
      scope: {
        domain: item.domain_id,
        module: item.module_id,
        version: item.module_version,
      },
      body: {
        contract_version: KnowledgeAccessVersion,
        credential_id: item.credential_id,
        confirmed: true,
        idempotency_key: crypto.randomUUID(),
      },
    });
  }
  return (
    <section
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t('knowledgeAccess.ownCredentials')}
    >
      <h2 className="text-lg font-medium">
        {t('knowledgeAccess.ownCredentials')}
      </h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeAccess.ownCredentialsHint')}
      </p>
      <label className="block space-y-1">
        <span>{t('knowledgeAccess.credentialDomain')}</span>
        <Input
          value={domain}
          autoComplete="off"
          disabled={command.busy}
          onChange={(event) => {
            clear();
            setDomain(event.target.value);
          }}
        />
      </label>
      <Button
        variant="outline"
        disabled={busy || !accessSchemaValid('ID', domain)}
        onClick={() => {
          void list();
        }}
      >
        {t('knowledgeAccess.listCredentials')}
      </Button>
      <AccessFailure error={error} />
      <CredentialFeedback command={command} />
      <ul className="space-y-3">
        {items.map((item) => (
          <li
            className="space-y-1 rounded border border-border-button p-3"
            key={item.credential_id}
          >
            <p>
              {item.client_label} —{' '}
              <code className="break-all">{item.credential_id}</code>
            </p>
            <p>
              {t('knowledgeAccess.grantScope', {
                domain: item.domain_id,
                module: item.module_id,
                version: item.module_version,
              })}
            </p>
            <p>
              {t(
                item.mode === 'read'
                  ? 'knowledgeAccess.readOnly'
                  : 'knowledgeAccess.readSearch',
              )}
            </p>
            <p>{t('knowledgeAccess.expires', { time: item.expires_at })}</p>
            <p>
              {t('knowledgeAccess.credentialLastUse')}:{' '}
              {item.last_used_at ?? '—'}
            </p>
            {item.revoked_at ? (
              <p>
                {t('knowledgeAccess.credentialRevoked')}: {item.revoked_at}
              </p>
            ) : (
              <>
                <label className="block space-x-2">
                  <input
                    type="checkbox"
                    disabled={command.locked}
                    checked={confirmation === item.credential_id}
                    onChange={(event) =>
                      setConfirmation(
                        event.target.checked ? item.credential_id : '',
                      )
                    }
                  />
                  <span>{t('knowledgeAccess.confirmRevokeCredential')}</span>
                </label>
                <Button
                  variant="danger"
                  disabled={
                    command.locked || confirmation !== item.credential_id
                  }
                  onClick={() => {
                    void revoke(item);
                  }}
                >
                  {t('knowledgeAccess.revokeCredential')}
                </Button>
              </>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

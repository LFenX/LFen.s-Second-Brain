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
import dayjs from 'dayjs';
import {
  MCPClientConfigKinds,
  mcpClientConfig,
  type MCPClientConfigKind,
} from '@/utils/mcp-client-config';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  CheckRow,
  CopyValue,
  Field,
  FieldSelect,
  Panel,
  StatusText,
} from '@/components/form-fields';
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

const MCPToolNames = [
  'knowledge_access_inspect',
  'knowledge_access_search',
  'knowledge_access_read',
] as const;
const TokenPlaceholder = '<token>';

export function formatAccessTime(value: string | null | undefined): string {
  if (!value) return '—';
  const time = dayjs(value);
  return time.isValid() ? time.format('YYYY-MM-DD HH:mm') : value;
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
    [copied, setCopied] = useState(false),
    [copiedConfig, setCopiedConfig] = useState<MCPClientConfigKind | null>(
      null,
    );
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
    setCopiedConfig(null);
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
        setCopiedConfig(null);
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
  async function copyClientConfig(kind: MCPClientConfigKind) {
    const value = secret.current;
    if (
      !value ||
      !endpoint ||
      value.boundary !== boundary ||
      nativeSessionRevision() !== actor.session
    )
      return;
    try {
      await navigator.clipboard.writeText(
        mcpClientConfig(kind, endpoint, value.token),
      );
      if (secret.current === value && lifetime.current.mounted)
        setCopiedConfig(kind);
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
    return (
      <Panel title={t('knowledgeAccess.temporaryMCP')}>
        <StatusText>{t('knowledgeAccess.noMCPGrant')}</StatusText>
      </Panel>
    );
  return (
    <Panel
      title={t('knowledgeAccess.temporaryMCP')}
      description={t('knowledgeAccess.mcpClientHint')}
      aria-label={t('knowledgeAccess.temporaryMCP')}
    >
      <dl className="grid gap-3 text-sm sm:grid-cols-[auto_1fr] sm:gap-x-4">
        <dt className="text-text-secondary">
          {t('knowledgeAccess.mcpEndpoint')}
        </dt>
        <dd className="min-w-0">
          {endpoint ? (
            <div className="space-y-1">
              <CopyValue value={endpoint} />
              <p className="text-xs leading-relaxed text-text-secondary">
                {t('knowledgeAccess.mcpEndpointHint')}
              </p>
            </div>
          ) : (
            <StatusText tone="warning">
              {t('knowledgeAccess.unavailable')}
            </StatusText>
          )}
        </dd>
        <dt className="text-text-secondary">{t('knowledgeAccess.mcpTools')}</dt>
        <dd className="flex flex-wrap gap-1.5">
          {MCPToolNames.map((name) => (
            <code
              key={name}
              className="rounded-md bg-bg-card px-1.5 py-0.5 font-mono text-xs text-text-primary"
            >
              {name}
            </code>
          ))}
        </dd>
        <dt className="text-text-secondary">
          {t('knowledgeAccess.credentialOwner')}
        </dt>
        <dd className="min-w-0 break-all font-mono text-xs leading-6 text-text-secondary">
          {actor.user}
        </dd>
      </dl>

      <ol className="space-y-5 border-t-0.5 border-border-button pt-4">
        <MCPStep index={1} title={t('knowledgeAccess.mcpStepScope')}>
          <Field label={t('knowledgeAccess.mode')}>
            <FieldSelect
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
            </FieldSelect>
          </Field>
          {catalog.length > 0 && (
            <fieldset disabled={command.locked || busy} className="space-y-2">
              <legend className="mb-1.5 text-sm font-medium text-text-primary">
                {t('knowledgeAccess.credentialSources')}
              </legend>
              {catalog.map(({ target }) => (
                <CheckRow
                  key={target.source.source_id}
                  checked={sources.includes(target.source.source_id)}
                  title={target.source.source_sha256}
                  onChange={(event) => {
                    changeScope();
                    setSources((values) =>
                      event.target.checked
                        ? [...values, target.source.source_id]
                        : values.filter((id) => id !== target.source.source_id),
                    );
                  }}
                >
                  <code className="break-all font-mono text-xs text-text-secondary">
                    {target.source.source_id} /{' '}
                    {target.source.source_revision_id} /{' '}
                    {target.source.source_sha256}
                  </code>
                </CheckRow>
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
            loading={busy}
            onClick={() => {
              void prepare();
            }}
          >
            {t('knowledgeAccess.prepareMCP')}
          </Button>
          <AccessFailure error={error} />
          {view && (
            <div
              className="space-y-1 rounded-xl bg-bg-card px-3 py-2.5 text-sm"
              role="group"
              aria-label={t('knowledgeAccess.credentialReview')}
            >
              <p className="text-text-primary">
                {t('knowledgeAccess.credentialScopeCount', {
                  sources: view.source_count,
                  units: view.unit_count,
                })}
              </p>
              <p className="break-all text-xs text-text-secondary">
                {t('knowledgeAccess.grantScope', {
                  domain: view.domain_id,
                  module: view.module_id,
                  version: view.module_version,
                })}
              </p>
              <p className="text-xs text-text-secondary">
                {t('knowledgeAccess.expires', {
                  time: formatAccessTime(view.expires_at),
                })}
              </p>
            </div>
          )}
        </MCPStep>

        <MCPStep index={2} title={t('knowledgeAccess.mcpStepCredential')}>
          <form onSubmit={mint} className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label={t('knowledgeAccess.clientLabel')}
                hint={t('knowledgeAccess.clientLabelHint')}
              >
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
              </Field>
              <Field
                label={t('knowledgeAccess.credentialTTL')}
                hint={t('knowledgeAccess.credentialTTLHint')}
              >
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
              </Field>
            </div>
            <CheckRow
              tone="confirm"
              checked={confirmed}
              disabled={!view || command.locked || busy}
              onChange={(event) => setConfirmed(event.target.checked)}
            >
              {t('knowledgeAccess.confirmCredential')}
            </CheckRow>
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
            <StatusText tone="success">
              {t('knowledgeAccess.credentialID')}:{' '}
              <code className="break-all font-mono text-xs">
                {command.receipt.credential_id}
              </code>{' '}
              · {formatAccessTime(command.receipt.expires_at)}
            </StatusText>
          )}
        </MCPStep>

        <MCPStep index={3} title={t('knowledgeAccess.mcpStepConnect')}>
          {command.delivery !== null && !hasToken && (
            <StatusText role="status" tone="warning">
              {t('knowledgeAccess.tokenUnavailable')}
            </StatusText>
          )}
          {!(hasToken && currentSecret) && command.delivery === null && (
            <StatusText>{t('knowledgeAccess.mcpConnectPending')}</StatusText>
          )}
          {hasToken && currentSecret && endpoint && (
            <div className="space-y-4">
              <div className="space-y-2 rounded-xl border-0.5 border-state-warning/40 bg-state-warning/5 p-3">
                <StatusText tone="warning">
                  {t('knowledgeAccess.tokenOnce')}
                </StatusText>
                {revealed && (
                  <code
                    className="block break-all rounded-lg bg-bg-component px-2.5 py-2 font-mono text-xs"
                    data-testid="one-time-token"
                  >
                    {currentSecret.token}
                  </code>
                )}
                <div className="flex flex-wrap gap-2">
                  {!revealed && (
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
                  <Button variant="ghost" onClick={closeDelivery}>
                    {t('knowledgeAccess.closeToken')}
                  </Button>
                </div>
                {copied && (
                  <StatusText role="status" tone="success">
                    {t('knowledgeAccess.tokenCopied')}
                  </StatusText>
                )}
              </div>
              {MCPClientConfigKinds.map((kind) => (
                <div key={kind} className="space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium text-text-primary">
                      {t(`knowledgeAccess.mcpConfig.${kind}`)}
                    </span>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        void copyClientConfig(kind);
                      }}
                    >
                      {copiedConfig === kind
                        ? t('knowledgeAccess.configCopied')
                        : t('knowledgeAccess.copyConfig')}
                    </Button>
                  </div>
                  <pre className="overflow-x-auto rounded-xl border-0.5 border-border-button bg-bg-card px-3 py-2.5 font-mono text-[12px] leading-relaxed text-text-primary">
                    {mcpClientConfig(kind, endpoint, TokenPlaceholder)}
                  </pre>
                </div>
              ))}
              <StatusText>{t('knowledgeAccess.mcpConfigHint')}</StatusText>
            </div>
          )}
        </MCPStep>
      </ol>
    </Panel>
  );
}

function MCPStep({
  index,
  title,
  children,
}: {
  index: number;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <li className="grid grid-cols-[24px_1fr] gap-x-3">
      <span className="mt-px flex size-6 items-center justify-center rounded-full bg-bg-card text-xs font-semibold text-text-primary">
        {index}
      </span>
      <div className="min-w-0 space-y-3">
        <h3 className="text-sm font-semibold leading-6 text-text-primary">
          {title}
        </h3>
        {children}
      </div>
    </li>
  );
}

export function OwnMCPCredentials({
  actor,
  domainChoices,
}: {
  actor: AccessActor;
  /** Knowledge domains the viewer can pick from instead of typing an ID. */
  domainChoices?: { id: string; label: string }[];
}) {
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
    <Panel
      title={t('knowledgeAccess.ownCredentials')}
      description={t('knowledgeAccess.ownCredentialsHint')}
      aria-label={t('knowledgeAccess.ownCredentials')}
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
        <Field
          label={t('knowledgeAccess.credentialDomain')}
          className="min-w-0 flex-1"
        >
          {domainChoices && domainChoices.length > 0 ? (
            <FieldSelect
              value={domain}
              disabled={command.busy}
              onChange={(event) => {
                clear();
                setDomain(event.target.value);
              }}
            >
              <option value="">
                {t('knowledgeAccess.chooseCredentialDomain')}
              </option>
              {domainChoices.map((choice) => (
                <option key={choice.id} value={choice.id}>
                  {choice.label}
                </option>
              ))}
            </FieldSelect>
          ) : (
            <Input
              value={domain}
              autoComplete="off"
              disabled={command.busy}
              onChange={(event) => {
                clear();
                setDomain(event.target.value);
              }}
            />
          )}
        </Field>
        <Button
          variant="outline"
          disabled={busy || !accessSchemaValid('ID', domain)}
          loading={busy}
          onClick={() => {
            void list();
          }}
        >
          {t('knowledgeAccess.listCredentials')}
        </Button>
      </div>
      <AccessFailure error={error} />
      <CredentialFeedback command={command} />
      {items.length > 0 && (
        <ul className="divide-y-0.5 divide-border-button overflow-hidden rounded-2xl border-0.5 border-border-button">
          {items.map((item) => (
            <li className="space-y-2 px-4 py-3" key={item.credential_id}>
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-text-primary">
                  {item.client_label}
                </span>
                <span
                  className={
                    item.revoked_at
                      ? 'rounded-full bg-bg-card px-2 py-0.5 text-xs text-text-secondary'
                      : 'rounded-full bg-state-success/10 px-2 py-0.5 text-xs text-state-success'
                  }
                >
                  {item.revoked_at
                    ? t('knowledgeAccess.credentialRevoked')
                    : t(
                        item.mode === 'read'
                          ? 'knowledgeAccess.readOnly'
                          : 'knowledgeAccess.readSearch',
                      )}
                </span>
                <code className="ms-auto break-all font-mono text-xs text-text-secondary">
                  {item.credential_id}
                </code>
              </div>
              <p className="break-all text-xs text-text-secondary">
                {t('knowledgeAccess.grantScope', {
                  domain: item.domain_id,
                  module: item.module_id,
                  version: item.module_version,
                })}
              </p>
              <p className="text-xs text-text-secondary">
                {t('knowledgeAccess.expires', {
                  time: formatAccessTime(item.expires_at),
                })}{' '}
                · {t('knowledgeAccess.credentialLastUse')}:{' '}
                {formatAccessTime(item.last_used_at)}
                {item.revoked_at &&
                  ' · ' +
                    t('knowledgeAccess.credentialRevoked') +
                    ': ' +
                    formatAccessTime(item.revoked_at)}
              </p>
              {!item.revoked_at && (
                <div className="flex flex-wrap items-center gap-3">
                  <CheckRow
                    disabled={command.locked}
                    checked={confirmation === item.credential_id}
                    onChange={(event) =>
                      setConfirmation(
                        event.target.checked ? item.credential_id : '',
                      )
                    }
                  >
                    {t('knowledgeAccess.confirmRevokeCredential')}
                  </CheckRow>
                  <Button
                    size="sm"
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
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

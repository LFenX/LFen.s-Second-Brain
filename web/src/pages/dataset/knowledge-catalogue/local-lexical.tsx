import { AccessPublicationLink } from '../knowledge-access/publication-link';
import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useContentPolicy } from '@/hooks/use-knowledge-consumption';
import type {
  ContentPolicy,
  ContentNode,
} from '@/interfaces/database/knowledge-consumption';
import { ContentRequestError } from '@/services/knowledge-consumption-service';
import { contentFixedLink } from '@/utils/knowledge-consumption';
import { getKnowledgeSessionScope } from '@/hooks/use-knowledge-foundation';
import {
  LocalLexicalKeys,
  useLocalLexicalActions,
  useLocalLexicalJob,
  useLocalLexicalPolicy,
  useLocalLexicalReadiness,
  type LocalLexicalActions,
} from '@/hooks/use-knowledge-local-lexical';
import type {
  LocalLexicalHit,
  LocalLexicalPolicy,
  LocalLexicalReadData,
  LocalLexicalCoverage,
  LocalLexicalReadiness,
  LocalLexicalSearchData,
  LocalLexicalSelection,
} from '@/interfaces/database/knowledge-local-lexical';
import type { LocalLexicalScope } from '@/interfaces/request/knowledge-local-lexical';
import { LocalLexicalRequestError } from '@/services/knowledge-local-lexical-service';
import {
  checkLocalLexicalSelection,
  localLexicalBytes,
  localLexicalCanonical,
  localLexicalSelectionKey,
  LocalLexicalVersion,
  LocalLexicalPolicyVersion,
} from '@/utils/knowledge-local-lexical';
import { CatalogueLoading } from './state';
import {
  ContentAttributesView,
  LocalLexicalFailure,
  LocalLexicalPicker,
  type LocalLexicalChoice,
} from './local-lexical-selection';

function PolicyPanel({
  scope,
  policy,
  actions,
}: {
  scope: LocalLexicalScope;
  policy: LocalLexicalPolicy;
  actions: LocalLexicalActions;
}) {
  const { t } = useTranslation(),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  useEffect(() => {
    setAck(false);
  }, [
    policy.local_policy_version,
    policy.foundation_policy_version,
    policy.enabled,
  ]);
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
      setAck(event.target.checked),
    refresh = () => {
      setAck(false);
      void actions.refresh();
    };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!ack || actions.pending || actions.busy || !actions.storageReady)
      return;
    setError(null);
    setAck(false);
    try {
      await actions.submit({
        operation: 'changeLocalPolicy',
        body: {
          contract_version: LocalLexicalPolicyVersion,
          idempotency_key: crypto.randomUUID(),
          expected_local_policy_version: policy.local_policy_version,
          expected_foundation_policy_version: policy.foundation_policy_version,
          enabled: !policy.enabled,
        },
      });
    } catch (issue) {
      setError(issue);
    }
  };
  return (
    <section
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t('knowledgeLocalLexical.policy')}
    >
      <h3 className="font-semibold">{t('knowledgeLocalLexical.policy')}</h3>
      <p>{t('knowledgeLocalLexical.policyRules')}</p>
      <p>
        {t(
          policy.enabled
            ? 'knowledgeLocalLexical.enabled'
            : 'knowledgeLocalLexical.disabled',
        )}{' '}
        ·{' '}
        {t('knowledgeLocalLexical.versions', {
          foundation: policy.foundation_policy_version,
          local: policy.local_policy_version,
        })}
      </p>
      <form onSubmit={submit} className="space-y-3">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={ack} onChange={acknowledge} />
          {t(
            policy.enabled
              ? 'knowledgeLocalLexical.confirmDisable'
              : 'knowledgeLocalLexical.confirmEnable',
          )}
        </label>
        <div className="flex flex-wrap gap-2">
          <Button
            type="submit"
            disabled={
              !ack || actions.busy || !!actions.pending || !actions.storageReady
            }
          >
            {t(
              policy.enabled
                ? 'knowledgeLocalLexical.disable'
                : 'knowledgeLocalLexical.enable',
            )}
          </Button>
          <Button type="button" variant="outline" onClick={refresh}>
            {t('knowledgeCatalogue.refresh')}
          </Button>
        </div>
      </form>
      {!!error && <LocalLexicalFailure error={error} />}
      <p className="break-all text-xs text-text-secondary">{scope.domain}</p>
    </section>
  );
}
function ContentPolicyPanel({
  policy,
  actions,
}: {
  policy: ContentPolicy;
  actions: LocalLexicalActions;
}) {
  const { t } = useTranslation(),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  useEffect(
    () => setAck(false),
    [
      policy.content_policy_version,
      policy.foundation_policy_version,
      policy.lexical_enabled,
      policy.parse_enabled,
    ],
  );
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!ack || actions.pending || actions.busy || !actions.storageReady)
      return;
    setAck(false);
    setError(null);
    try {
      await actions.submit({
        operation: 'changeContentPolicy',
        body: {
          contract_version: LocalLexicalVersion,
          request: {
            expected_policy_version: policy.foundation_policy_version,
            expected_content_policy_version: policy.content_policy_version,
            parse_enabled: policy.parse_enabled,
            lexical_enabled: !policy.lexical_enabled,
            confirmed: true,
            idempotency_key: crypto.randomUUID(),
          },
        },
      });
    } catch (issue) {
      setError(issue);
    }
  };
  return (
    <section
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t('knowledgeLocalLexical.contentPolicy')}
    >
      <h3 className="font-semibold">
        {t('knowledgeLocalLexical.contentPolicy')}
      </h3>
      <p>{t('knowledgeLocalLexical.contentPolicyRules')}</p>
      <p>
        {t(
          policy.lexical_enabled
            ? 'knowledgeLocalLexical.enabled'
            : 'knowledgeLocalLexical.disabled',
        )}{' '}
        ·{' '}
        {t('knowledgeLocalLexical.contentVersions', {
          foundation: policy.foundation_policy_version,
          content: policy.content_policy_version,
        })}
      </p>
      <p>
        {t('knowledgeLocalLexical.parseUnchanged', {
          state: t(
            policy.parse_enabled
              ? 'knowledgeLocalLexical.enabled'
              : 'knowledgeLocalLexical.disabled',
          ),
        })}
      </p>
      <form onSubmit={submit} className="space-y-3">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={ack} onChange={acknowledge} />
          {t(
            policy.lexical_enabled
              ? 'knowledgeLocalLexical.confirmContentDisable'
              : 'knowledgeLocalLexical.confirmContentEnable',
          )}
        </label>
        <Button
          type="submit"
          disabled={
            !ack || actions.busy || !!actions.pending || !actions.storageReady
          }
        >
          {t(
            policy.lexical_enabled
              ? 'knowledgeLocalLexical.disableContent'
              : 'knowledgeLocalLexical.enableContent',
          )}
        </Button>
      </form>
      {!!error && <LocalLexicalFailure error={error} />}
    </section>
  );
}

function PendingWrite({ actions }: { actions: LocalLexicalActions }) {
  const { t } = useTranslation(),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const retry = async () => {
    if (!ack || actions.busy) return;
    setAck(false);
    setError(null);
    try {
      await actions.resend();
    } catch (issue) {
      setError(issue);
    }
  };
  if (!actions.pending) return null;
  const issue = actions.error;
  const allowed =
    !issue ||
    (issue instanceof LocalLexicalRequestError &&
      issue.retrySameKey &&
      issue.code !== 'idempotency_conflict') ||
    (issue instanceof ContentRequestError &&
      issue.outcomeKnown !== true &&
      issue.code !== 'idempotency_conflict');
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeLocalLexical.pending')}</h3>
      <p>{t('knowledgeLocalLexical.pendingHint')}</p>
      <p className="break-all text-xs">
        {actions.pending.operation} ·{' '}
        {actions.pending.operation === 'changeContentPolicy'
          ? actions.pending.body.request.idempotency_key
          : actions.pending.body.idempotency_key}
      </p>
      <details>
        <summary>{t('knowledgeLocalLexical.fixedIdentity')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(actions.pending.body, null, 2)}
        </pre>
      </details>
      <label className="flex items-center gap-2">
        <input type="checkbox" checked={ack} onChange={acknowledge} />
        {t('knowledgeLocalLexical.confirmResend')}
      </label>
      <Button
        type="button"
        variant="outline"
        onClick={retry}
        disabled={!ack || !allowed || actions.busy}
      >
        {t('knowledgeLocalLexical.resend')}
      </Button>
      {!!(error || issue) && <LocalLexicalFailure error={error || issue} />}
    </section>
  );
}
function SelectedRow({
  choice,
  onRemove,
  disabled,
}: {
  choice: LocalLexicalChoice;
  onRemove: (source: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation(),
    remove = () => onRemove(choice.target.source_ref.source_id);
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p className="break-all font-medium">{choice.path}</p>
      <p className="text-xs">
        {t('knowledgeLocalLexical.currentness_' + choice.currentness)} ·{' '}
        {choice.target.representation_ref.kind}
      </p>
      <details>
        <summary>{t('knowledgeLocalLexical.fixedIdentity')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(choice.target, null, 2)}
        </pre>
      </details>
      <Button
        type="button"
        variant="outline"
        onClick={remove}
        disabled={disabled}
      >
        {t('knowledgeLocalLexical.remove')}
      </Button>
    </li>
  );
}
function CoverageState({ coverage }: { coverage: LocalLexicalCoverage[] }) {
  const { t } = useTranslation();
  if (!coverage.length)
    return <p>{t('knowledgeLocalLexical.coveragePending')}</p>;
  return (
    <ul className="space-y-3">
      {coverage.map((item) => (
        <li
          className="space-y-1 break-all rounded border border-border-button p-3"
          key={localLexicalCanonical(item.target)}
        >
          <p className="text-xs">
            {item.target.source_ref.source_id} ·{' '}
            {item.target.representation_ref.kind} ·{' '}
            {item.target.source_ref.source_revision_id}
          </p>
          <p>
            {t('knowledgeLocalLexical.readingCoverage', {
              covered: item.covered_reading_bytes,
              expected: item.expected_reading_bytes,
              units: item.indexed_unit_count,
            })}
          </p>
          <p>
            {t('knowledgeConsumption.space_' + item.coordinate_space)} ·{' '}
            {t(
              'knowledgeLocalLexical.representation_' +
                item.representation_state,
            )}{' '}
            · {t('knowledgeLocalLexical.textIndex_' + item.text_index_state)}
          </p>
          <p>
            {t(
              'knowledgeLocalLexical.originalCoverage_' +
                item.original_coverage,
            )}
          </p>
          <p>
            {t('knowledgeLocalLexical.unsearchableNodes', {
              count: item.unsearchable_node_count,
            })}
          </p>
        </li>
      ))}
    </ul>
  );
}
function IndexState({ readiness }: { readiness: LocalLexicalReadiness }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" role="status">
      <h4 className="font-semibold">
        {t('knowledgeLocalLexical.readiness_' + readiness.kind)}
      </h4>
      <p>
        {t('knowledgeLocalLexical.snapshot_' + readiness.snapshot_currentness)}
      </p>
      <CoverageState coverage={readiness.coverage} />
      <p className="text-xs text-text-secondary">
        {t('knowledgeLocalLexical.coverageBoundary')}
      </p>
      {!!readiness.diagnostic_codes.length && (
        <ul>
          {readiness.diagnostic_codes.map((code) => (
            <li key={code}>
              {t('knowledgeLocalLexical.diagnostic_' + code, {
                defaultValue: code,
              })}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
function EvidenceNode({ node }: { node: ContentNode }) {
  const { t } = useTranslation(),
    range = node.text.range;
  return (
    <div className="space-y-2">
      <p className="font-medium">
        {node.name || t('knowledgeConsumption.unnamed')} ·{' '}
        {t('knowledgeConsumption.kind_' + node.kind, {
          defaultValue: node.kind,
        })}
      </p>
      {node.name_truncated && <p>{t('knowledgeConsumption.nameTruncated')}</p>}
      <p>
        {t('knowledgeConsumption.text_' + node.text.status)}
        {range ? ' · [' + range.start_byte + ', ' + range.end_byte + ')' : ''}
      </p>
      <ContentAttributesView node={node} />
      <details>
        <summary>{t('knowledgeConsumption.fixedIdentity')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(node, null, 2)}
        </pre>
      </details>
    </div>
  );
}
export function LocalLexicalEvidence({ read }: { read: LocalLexicalReadData }) {
  const { t } = useTranslation(),
    evidence = read.evidence_ref,
    context = read.context;
  return (
    <section
      className="space-y-3 rounded border border-border-button p-4"
      aria-label={t('knowledgeLocalLexical.fixedUnit')}
    >
      <h4 className="font-semibold">{t('knowledgeLocalLexical.fixedUnit')}</h4>
      <p>
        {t('knowledgeLocalLexical.currentness_' + read.source_currentness)} ·{' '}
        {read.byte_length} B · {t('knowledgeLocalLexical.completeUnit')}
      </p>
      <p>
        {t('knowledgeConsumption.space_' + evidence.coordinate_space)} [
        {evidence.range.start_byte}, {evidence.range.end_byte})
      </p>
      <pre className="whitespace-pre-wrap break-all">{read.text}</pre>
      <a
        className="text-sm underline"
        href={contentFixedLink(evidence.address)}
      >
        {t('knowledgeConsumption.fixedLink')}
      </a>
      <details>
        <summary>{t('knowledgeLocalLexical.fixedIdentity')}</summary>
        <pre className="whitespace-pre-wrap break-all text-xs">
          {JSON.stringify(evidence, null, 2)}
        </pre>
      </details>
      <section
        className="space-y-3"
        aria-label={t('knowledgeLocalLexical.context')}
      >
        <h5 className="font-semibold">{t('knowledgeLocalLexical.context')}</h5>
        <p className="text-xs">{t('knowledgeLocalLexical.contextBoundary')}</p>
        <p>
          {t(
            'knowledgeLocalLexical.representation_' +
              context.representation_state,
          )}{' '}
          · {context.metadata_byte_length} B
        </p>
        {context.node ? (
          <EvidenceNode node={context.node} />
        ) : (
          <p>{t('knowledgeLocalLexical.nodeOmitted')}</p>
        )}
        <p>
          {t(
            context.typed_members_complete
              ? 'knowledgeLocalLexical.membersComplete'
              : 'knowledgeLocalLexical.membersIncomplete',
            { count: context.omitted_member_count },
          )}
        </p>
        <ul className="space-y-3">
          {context.typed_members.map((member) => (
            <li
              className="rounded border border-border-button p-3"
              key={localLexicalCanonical(member.node.ref)}
            >
              <EvidenceNode node={member.node} />
              <details>
                <summary>{t('knowledgeLocalLexical.memberIdentity')}</summary>
                <pre className="whitespace-pre-wrap break-all text-xs">
                  {JSON.stringify(
                    {
                      payload_sha256: member.payload_sha256,
                      text_assembly: member.text_assembly,
                    },
                    null,
                    2,
                  )}
                </pre>
              </details>
            </li>
          ))}
        </ul>
        {!!context.omission_reasons.length && (
          <ul>
            {context.omission_reasons.map((reason) => (
              <li key={reason}>
                {t('knowledgeLocalLexical.omission_' + reason, {
                  defaultValue: reason,
                })}
              </li>
            ))}
          </ul>
        )}
        <details>
          <summary>{t('knowledgeLocalLexical.contextIdentity')}</summary>
          <pre className="whitespace-pre-wrap break-all text-xs">
            {JSON.stringify(
              {
                membership: context.membership,
                manifest_sha256: context.manifest_sha256,
                typed_members_scope: context.typed_members_scope,
                parent_expansion: context.parent_expansion,
              },
              null,
              2,
            )}
          </pre>
        </details>
      </section>
    </section>
  );
}

function JobTracker({
  scope,
  job,
  onFinished,
}: {
  scope: LocalLexicalScope;
  job: string;
  onFinished: () => void;
}) {
  const { t } = useTranslation(),
    query = useLocalLexicalJob(scope, job),
    reported = useRef('');
  useEffect(() => {
    if (
      query.data &&
      ['published', 'failed'].includes(query.data.state) &&
      reported.current !== query.data.job_id + ':' + query.data.state
    ) {
      reported.current = query.data.job_id + ':' + query.data.state;
      onFinished();
    }
  }, [query.data, onFinished]);
  if (query.isError)
    return <LocalLexicalFailure error={query.error} onRetry={query.resume} />;
  if (!query.data) return <CatalogueLoading />;
  return (
    <section className="space-y-2">
      <h4>{t('knowledgeLocalLexical.job_' + query.data.state)}</h4>
      <p className="break-all text-xs">
        {query.data.job_id} ·{' '}
        {dayjs(query.data.updated_at).format('YYYY-MM-DD HH:mm:ss Z')}
      </p>
      {!query.waiting &&
        !['published', 'failed'].includes(query.data.state) && (
          <p>{t('knowledgeLocalLexical.stillProcessing')}</p>
        )}
      <Button type="button" variant="outline" onClick={query.resume}>
        {t('knowledgeCatalogue.refresh')}
      </Button>
    </section>
  );
}
function HitRow({
  hit,
  onRead,
  busy,
}: {
  hit: LocalLexicalHit;
  onRead: (hit: LocalLexicalHit) => void;
  busy: boolean;
}) {
  const { t } = useTranslation(),
    read = () => onRead(hit),
    evidence = hit.evidence_ref;
  return (
    <li className="space-y-2 rounded border border-border-button p-3">
      <p>
        {t('knowledgeLocalLexical.currentness_' + hit.source_currentness)} ·{' '}
        {t('knowledgeLocalLexical.lexicalScore', { score: hit.score })}
      </p>
      <p className="break-all text-xs">
        {evidence.address.target.source_ref.source_revision_id} ·{' '}
        {evidence.coordinate_space} [{evidence.range.start_byte},{' '}
        {evidence.range.end_byte})
      </p>
      <pre className="whitespace-pre-wrap break-all">{hit.text}</pre>
      <Button type="button" variant="outline" onClick={read} disabled={busy}>
        {t('knowledgeLocalLexical.readUnit')}
      </Button>
    </li>
  );
}
function SearchPanel({
  scope,
  selection,
  readiness,
  actions,
}: {
  scope: LocalLexicalScope;
  selection: LocalLexicalSelection;
  readiness: LocalLexicalReadiness;
  actions: LocalLexicalActions;
}) {
  const { t } = useTranslation(),
    client = useQueryClient(),
    active = useRef(true),
    request = useRef(0),
    readRequest = useRef(0),
    [query, setQuery] = useState(''),
    [limit, setLimit] = useState(8),
    [busy, setBusy] = useState(false),
    [reading, setReading] = useState(false),
    [result, setResult] = useState<LocalLexicalSearchData | null>(null),
    [read, setRead] = useState<LocalLexicalReadData | null>(null),
    [error, setError] = useState<unknown>(null),
    [readError, setReadError] = useState<unknown>(null);
  const { owner, domain } = scope;
  const current = () =>
    active.current && actions.session === getKnowledgeSessionScope();
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      void client.cancelQueries({
        queryKey: LocalLexicalKeys.searches(actions.session, { owner, domain }),
      });
      void client.cancelQueries({
        queryKey: LocalLexicalKeys.reads(actions.session, { owner, domain }),
      });
      client.removeQueries({
        queryKey: LocalLexicalKeys.searches(actions.session, { owner, domain }),
      });
      client.removeQueries({
        queryKey: LocalLexicalKeys.reads(actions.session, { owner, domain }),
      });
    };
  }, [client, actions.session, owner, domain]);
  const editQuery = (event: ChangeEvent<HTMLInputElement>) =>
      setQuery(event.target.value),
    editLimit = (event: ChangeEvent<HTMLInputElement>) =>
      setLimit(Number(event.target.value));
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (
      busy ||
      reading ||
      !query.trim() ||
      localLexicalBytes(query) > 4096 ||
      !Number.isInteger(limit) ||
      limit < 1 ||
      limit > 20 ||
      readiness.kind !== 'ready' ||
      !readiness.index_ref
    )
      return;
    const cycle = ++request.current;
    readRequest.current += 1;
    setBusy(true);
    setResult(null);
    setReading(false);
    setRead(null);
    setError(null);
    setReadError(null);
    try {
      const value = await actions.search({
        contract_version: LocalLexicalVersion,
        selection,
        index_ref: readiness.index_ref,
        query,
        limit,
      });
      if (current() && cycle === request.current) setResult(value);
    } catch (issue) {
      if (current() && cycle === request.current) {
        setError(issue);
        void actions.refresh();
      }
    } finally {
      if (current() && cycle === request.current) setBusy(false);
    }
  };
  const readHit = async (hit: LocalLexicalHit) => {
    if (reading || busy) return;
    const cycle = ++readRequest.current;
    setReading(true);
    setRead(null);
    setReadError(null);
    try {
      const value = await actions.read({
        contract_version: LocalLexicalVersion,
        evidence_ref: hit.evidence_ref,
      });
      if (current() && cycle === readRequest.current) setRead(value);
    } catch (issue) {
      if (current() && cycle === readRequest.current) setReadError(issue);
    } finally {
      if (current() && cycle === readRequest.current) setReading(false);
    }
  };
  return (
    <section
      className="space-y-4"
      aria-label={t('knowledgeLocalLexical.search')}
    >
      <h3 className="font-semibold">{t('knowledgeLocalLexical.search')}</h3>
      <form onSubmit={submit} className="space-y-3">
        <label className="block space-y-1">
          {t('knowledgeLocalLexical.query')}
          <Input value={query} onChange={editQuery} />
        </label>
        <p className="text-xs text-text-secondary">
          {localLexicalBytes(query)} / 4096 B
        </p>
        <label className="block space-y-1">
          {t('knowledgeLocalLexical.limit')}
          <Input
            type="number"
            min={1}
            max={20}
            step={1}
            value={limit}
            onChange={editLimit}
          />
        </label>
        <Button
          type="submit"
          disabled={
            busy ||
            reading ||
            !query.trim() ||
            localLexicalBytes(query) > 4096 ||
            !Number.isInteger(limit) ||
            limit < 1 ||
            limit > 20
          }
        >
          {t('knowledgeLocalLexical.search')}
        </Button>
      </form>
      {busy && <CatalogueLoading />}
      {!!error && <LocalLexicalFailure error={error} />}
      {result && (
        <>
          <p role="status">
            {result.hits.length
              ? t('knowledgeLocalLexical.returned', {
                  count: result.returned_count,
                })
              : t('knowledgeLocalLexical.emptyHits')}
          </p>
          <p className="text-xs text-text-secondary">
            {t('knowledgeLocalLexical.rankingBoundary')}
          </p>
          <ul className="space-y-3">
            {result.hits.map((hit) => (
              <HitRow
                key={hit.evidence_ref.index_row_id}
                hit={hit}
                onRead={readHit}
                busy={busy || reading}
              />
            ))}
          </ul>
        </>
      )}
      {reading && <CatalogueLoading />}
      {!!readError && <LocalLexicalFailure error={readError} />}
      {read && <LocalLexicalEvidence read={read} />}
    </section>
  );
}
function FixedSelection({
  scope,
  selection,
  names,
  policy,
  contentPolicy,
  actions,
}: {
  scope: LocalLexicalScope;
  selection: LocalLexicalSelection;
  names: Record<string, string>;
  contentPolicy: ContentPolicy;
  policy: LocalLexicalPolicy;
  actions: LocalLexicalActions;
}) {
  const { t } = useTranslation(),
    query = useLocalLexicalReadiness(scope, selection),
    active = useRef(true),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  const job =
    actions.lastJob &&
    localLexicalSelectionKey(actions.lastJob.selection) ===
      localLexicalSelectionKey(selection)
      ? actions.lastJob.job_id
      : '';
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
      setAck(event.target.checked),
    refresh = () => {
      setAck(false);
      void query.refetch();
    };
  const versionsMatch =
    query.data?.effective_policy.local_policy_version ===
      policy.local_policy_version &&
    query.data?.effective_policy.foundation_policy_version ===
      policy.foundation_policy_version &&
    query.data?.effective_policy.content_policy_version ===
      contentPolicy.content_policy_version &&
    query.data?.effective_policy.foundation_policy_version ===
      contentPolicy.foundation_policy_version;
  const available =
    policy.enabled &&
    contentPolicy.lexical_enabled &&
    query.data?.effective_policy.enabled === true &&
    versionsMatch &&
    !query.isFetching &&
    !query.isError;
  useEffect(
    () => setAck(false),
    [
      query.data?.effective_policy.local_policy_version,
      query.data?.effective_policy.content_policy_version,
      query.data?.effective_policy.foundation_policy_version,
      query.data?.profile.profile_sha256,
    ],
  );
  const create = async (event: FormEvent) => {
    event.preventDefault();
    if (
      !available ||
      !query.data ||
      !ack ||
      actions.busy ||
      actions.pending ||
      !actions.storageReady ||
      query.data?.readiness.kind === 'building'
    )
      return;
    setAck(false);
    setError(null);
    try {
      await actions.submit({
        operation: 'createIndex',
        body: {
          contract_version: LocalLexicalVersion,
          idempotency_key: crypto.randomUUID(),
          expected_local_policy_version:
            query.data.effective_policy.local_policy_version,
          expected_content_policy_version:
            query.data.effective_policy.content_policy_version,
          expected_foundation_policy_version:
            query.data.effective_policy.foundation_policy_version,
          expected_profile_sha256: query.data.profile.profile_sha256,
          selection,
        },
      });
    } catch (issue) {
      if (active.current) setError(issue);
    }
  };
  return (
    <section className="space-y-4 rounded border border-border-button p-4">
      <h3 className="font-semibold">{t('knowledgeLocalLexical.index')}</h3>
      {query.isFetching && <CatalogueLoading />}
      {query.isError ? (
        <LocalLexicalFailure error={query.error} onRetry={refresh} />
      ) : (
        query.data && (
          <>
            <IndexState readiness={query.data.readiness} />
            <p className="text-xs">
              {t('knowledgeLocalLexical.profileBoundary', {
                language: query.data.profile.language,
                bytes: query.data.profile.max_unit_bytes,
              })}
            </p>
            <details>
              <summary>{t('knowledgeLocalLexical.profileIdentity')}</summary>
              <pre className="whitespace-pre-wrap break-all text-xs">
                {JSON.stringify(query.data.profile, null, 2)}
              </pre>
            </details>
          </>
        )
      )}
      <Button type="button" variant="outline" onClick={refresh}>
        {t('knowledgeLocalLexical.inspect')}
      </Button>
      <form onSubmit={create} className="space-y-2">
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={ack}
            onChange={acknowledge}
            disabled={
              !available ||
              actions.busy ||
              !!actions.pending ||
              !actions.storageReady ||
              query.data?.readiness.kind === 'building'
            }
          />
          {t('knowledgeLocalLexical.confirmIndex')}
        </label>
        <Button
          type="submit"
          disabled={
            !available ||
            !ack ||
            actions.busy ||
            !!actions.pending ||
            !actions.storageReady ||
            query.data?.readiness.kind === 'building'
          }
        >
          {t('knowledgeLocalLexical.createIndex')}
        </Button>
      </form>
      {!!error && <LocalLexicalFailure error={error} />}
      {available &&
        query.data?.readiness.kind === 'ready' &&
        query.data.readiness.index_ref && (
          <AccessPublicationLink
            domain={scope.domain}
            selection={selection}
            publication={query.data.readiness.index_ref}
            names={names}
          />
        )}
      {job && (
        <JobTracker key={job} scope={scope} job={job} onFinished={refresh} />
      )}
      {available &&
      query.data?.readiness.kind === 'ready' &&
      query.data.readiness.index_ref ? (
        <SearchPanel
          key={
            query.data.readiness.index_ref.index_id +
            ':' +
            query.data.readiness.index_ref.manifest_sha256
          }
          scope={scope}
          selection={selection}
          readiness={query.data.readiness}
          actions={actions}
        />
      ) : (
        <p>{t('knowledgeLocalLexical.searchWhenReady')}</p>
      )}
    </section>
  );
}
function LocalLexicalWorkspace({
  scope,
  group,
  initialChoice,
}: {
  scope: LocalLexicalScope;
  group?: string;
  initialChoice?: LocalLexicalChoice;
}) {
  const { t } = useTranslation(),
    policy = useLocalLexicalPolicy(scope),
    contentPolicy = useContentPolicy(scope),
    actions = useLocalLexicalActions(scope),
    [choices, setChoices] = useState<LocalLexicalChoice[]>(() =>
      initialChoice ? [initialChoice] : [],
    ),
    [replacement, setReplacement] = useState<LocalLexicalChoice | null>(null),
    [selectionError, setSelectionError] = useState<unknown>(null);
  useEffect(() => {
    if (
      actions.pending?.operation === 'createIndex' &&
      localLexicalSelectionKey({
        kind: 'fixed_representations',
        targets: choices.map((choice) => choice.target),
      }) !== localLexicalSelectionKey(actions.pending.body.selection)
    )
      setChoices(
        actions.pending.body.selection.targets.map((target) => ({
          target,
          path: target.source_ref.source_id,
          currentness: 'unknown',
        })),
      );
  }, [actions.pending, choices]);
  const select = (choice: LocalLexicalChoice) => {
    if (actions.busy || actions.pending) return;
    setSelectionError(null);
    try {
      const previous = choices.find(
        (item) =>
          item.target.source_ref.source_id ===
          choice.target.source_ref.source_id,
      );
      if (previous) {
        if (
          localLexicalCanonical(previous.target) !==
          localLexicalCanonical(choice.target)
        )
          setReplacement(choice);
        return;
      }
      const next = [...choices, choice];
      checkLocalLexicalSelection(
        {
          kind: 'fixed_representations',
          targets: next.map((item) => item.target),
        },
        scope.domain,
      );
      setChoices(next);
    } catch (issue) {
      setSelectionError(issue);
    }
  };
  const replace = () => {
      if (replacement && !actions.busy && !actions.pending) {
        setChoices((items) =>
          items.map((item) =>
            item.target.source_ref.source_id ===
            replacement.target.source_ref.source_id
              ? replacement
              : item,
          ),
        );
        setReplacement(null);
      }
    },
    cancelReplace = () => setReplacement(null),
    remove = (source: string) => {
      if (!actions.busy && !actions.pending) {
        setChoices((items) =>
          items.filter((item) => item.target.source_ref.source_id !== source),
        );
        setReplacement(null);
      }
    };
  const refreshPolicy = () => {
    void policy.refetch();
    void contentPolicy.refetch();
  };
  if (policy.isError)
    return <LocalLexicalFailure error={policy.error} onRetry={refreshPolicy} />;
  if (contentPolicy.isError)
    return (
      <LocalLexicalFailure
        error={contentPolicy.error}
        onRetry={refreshPolicy}
      />
    );
  if (!policy.data || !contentPolicy.data) return <CatalogueLoading />;
  const selection: LocalLexicalSelection = {
    kind: 'fixed_representations',
    targets: choices.map((item) => item.target),
  };
  return (
    <div className="space-y-5">
      <h2 className="text-xl font-semibold">
        {t('knowledgeLocalLexical.title')}
      </h2>
      <p className="text-sm text-text-secondary">
        {t('knowledgeLocalLexical.intro')}
      </p>
      <PolicyPanel scope={scope} policy={policy.data} actions={actions} />
      <ContentPolicyPanel policy={contentPolicy.data} actions={actions} />
      {!!actions.storageError && (
        <LocalLexicalFailure error={actions.storageError} />
      )}
      <PendingWrite actions={actions} />
      <section className="space-y-3">
        <h3 className="font-semibold">
          {t('knowledgeLocalLexical.chooseSources')} · {choices.length} / 32
        </h3>
        <p className="text-sm text-text-secondary">
          {t('knowledgeLocalLexical.fixedHint')}
        </p>
        <LocalLexicalPicker
          scope={scope}
          group={group}
          onAdd={select}
          disabled={actions.busy || !!actions.pending}
        />
        <ul className="space-y-2">
          {choices.map((choice) => (
            <SelectedRow
              key={choice.target.source_ref.source_id}
              choice={choice}
              onRemove={remove}
              disabled={actions.busy || !!actions.pending}
            />
          ))}
        </ul>
        {replacement && (
          <div className="space-y-2 rounded border border-border-button p-3">
            <p>{t('knowledgeLocalLexical.replaceHint')}</p>
            <p className="break-all text-xs">
              {replacement.target.source_ref.source_revision_id}
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={replace}
              disabled={actions.busy || !!actions.pending}
            >
              {t('knowledgeLocalLexical.replace')}
            </Button>
            <Button type="button" variant="ghost" onClick={cancelReplace}>
              {t('common.cancel')}
            </Button>
          </div>
        )}
        {!!selectionError && <LocalLexicalFailure error={selectionError} />}
      </section>
      {choices.length > 0 && (
        <FixedSelection
          key={
            localLexicalSelectionKey(selection) +
            ':' +
            policy.data.local_policy_version +
            ':' +
            policy.data.foundation_policy_version +
            ':' +
            policy.data.enabled +
            ':' +
            contentPolicy.data.content_policy_version +
            ':' +
            contentPolicy.data.lexical_enabled
          }
          scope={scope}
          selection={selection}
          policy={policy.data}
          names={Object.fromEntries(
            choices.map((choice) => [
              choice.target.source_ref.source_id,
              choice.path,
            ]),
          )}
          contentPolicy={contentPolicy.data}
          actions={actions}
        />
      )}
    </div>
  );
}
export default function LocalLexicalBrowser({
  domain,
  owner,
  group,
  initialChoice,
}: {
  domain: string;
  owner: string;
  group?: string;
  initialChoice?: LocalLexicalChoice;
}) {
  return (
    <LocalLexicalWorkspace
      key={getKnowledgeSessionScope() + ':' + owner + ':' + domain}
      scope={{ owner, domain }}
      group={group}
      initialChoice={initialChoice}
    />
  );
}

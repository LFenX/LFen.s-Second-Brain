import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react';
import { Link } from 'react-router';
import dayjs from 'dayjs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useFetchDefaultModels } from '@/hooks/use-llm-request';
import { useKnowledgeAnswer } from '@/hooks/use-knowledge-answer';
import { useKnowledgeSource } from '@/hooks/use-knowledge-foundation';
import { useLocalLexicalReadiness } from '@/hooks/use-knowledge-local-lexical';
import {
  KnowledgeAnswerVersion,
  type AnswerCitation,
  type AnswerInspection,
  type AnswerPolicy,
  type AnswerResult,
} from '@/interfaces/database/knowledge-answer';
import type { AnswerRequest } from '@/interfaces/request/knowledge-answer';
import type { LocalLexicalScope } from '@/interfaces/request/knowledge-local-lexical';
import type { LocalLexicalSelection } from '@/interfaces/database/knowledge-local-lexical';
import { AnswerRequestError } from '@/services/knowledge-answer-service';
import {
  contentBytes,
  contentFixedLink,
  contentSameTarget,
} from '@/utils/knowledge-consumption';
import {
  LocalLexicalPicker,
  type LocalLexicalChoice,
} from './local-lexical-selection';
import { CatalogueLoading } from './state';

const ErrorMessages: Record<string, string> = {
  model_use_denied: '问答已关闭。请先明确确认资料外发范围。',
  unsupported_model:
    '当前默认模型不受此问答功能支持，请在模型设置中选择已配置的 DeepSeek deepseek-flash。',
  scope_changed: '资料范围或模型配置已变化，请刷新并重新确认。',
  index_stale:
    '资料尚未发布或索引版本已变化，请先在本地检索中发布同一资料范围。',
  version_conflict: '设置已在其他页面改变，请刷新后继续。',
  idempotency_conflict: '原请求已过期或内容不一致，请明确发起新问题。',
  model_output_invalid: '模型返回的答案或引用未通过校验，本次不显示答案。',
  provider_outcome_unknown:
    '本次请求结果尚未确定。查询原请求不会再次调用模型；明确发起新问题可能再次计费。',
  rate_limited: '已有问题正在处理或提问过快，请稍后再试。',
  not_found: '资料或模型当前不可访问。',
  unauthenticated: '登录状态已失效，请重新登录。',
};
function AnswerFailure({ error }: { error: unknown }) {
  return (
    <p
      role="alert"
      className="rounded border border-border-button p-3 text-text-secondary"
    >
      {error instanceof AnswerRequestError
        ? ErrorMessages[error.code] || '请求未完成，请刷新状态后重试。'
        : '请求未完成，请刷新状态后重试。'}
    </p>
  );
}
function Citation({ item, domain }: { item: AnswerCitation; domain: string }) {
  const source = useKnowledgeSource(
    domain,
    item.evidence_ref.address.target.source_ref.source_revision_id,
  );
  return (
    <details className="rounded border border-border-button p-3">
      <summary>
        {item.id} · {source.data?.relative_path || '固定来源'}
      </summary>
      <p className="my-2 whitespace-pre-wrap break-words">{item.text}</p>
      <Link
        className="underline"
        to={contentFixedLink(item.evidence_ref.address)}
      >
        查看引用的固定版本
      </Link>
    </details>
  );
}

type Flow = ReturnType<typeof useKnowledgeAnswer>;
function ConfirmSnapshot({
  scope,
  selection,
  flow,
  modelId,
}: {
  scope: LocalLexicalScope;
  selection: LocalLexicalSelection;
  flow: Flow;
  modelId: string;
}) {
  const readiness = useLocalLexicalReadiness(scope, selection);
  const [inspection, setInspection] = useState<AnswerInspection | null>(null),
    [ack, setAck] = useState(false),
    [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setInspection(null);
    setAck(false);
  }, [selection, modelId]);
  const inspect = async () => {
    const index = readiness.data?.readiness.index_ref;
    if (!index || !modelId) return;
    setError(null);
    setAck(false);
    setInspection(null);
    setBusy(true);
    try {
      setInspection(
        await flow.inspect.mutateAsync({
          contract_version: KnowledgeAnswerVersion,
          scope: { index_ref: index, selection },
          model_id: modelId,
        }),
      );
    } catch (issue) {
      setError(issue);
    } finally {
      setBusy(false);
    }
  };
  const acknowledge = (event: ChangeEvent<HTMLInputElement>) =>
    setAck(event.target.checked);
  const enable = async () => {
    if (!inspection || !ack) return;
    setError(null);
    setAck(false);
    setBusy(true);
    try {
      await flow.setPolicy.mutateAsync({
        contract_version: KnowledgeAnswerVersion,
        expected_version: inspection.policy_version,
        enabled: true,
        scope: inspection.scope,
        model_ref: inspection.model_ref,
        confirmed: true,
        external_send_confirmed: true,
        idempotency_key: crypto.randomUUID(),
      });
    } catch (issue) {
      setInspection(null);
      setError(issue);
    } finally {
      setBusy(false);
    }
  };
  const refresh = () => {
    setInspection(null);
    setAck(false);
    void readiness.refetch();
    void flow.policy.refetch();
  };
  const published =
    readiness.data?.readiness.publication === 'published' &&
    readiness.data.readiness.kind === 'ready';
  return (
    <section className="space-y-3 rounded border border-border-button p-4">
      <p>
        已选 {selection.targets.length}{' '}
        份固定资料。首次只使用已发布的本地检索索引。
      </p>
      {readiness.isPending ? (
        <CatalogueLoading />
      ) : (
        !published && (
          <p>请先在“本地检索”中为同一资料范围发布索引，再返回刷新。</p>
        )
      )}
      {!modelId && <p>请先在模型设置中选择已配置的默认 DeepSeek 问答模型。</p>}
      <div className="flex flex-wrap gap-2">
        <Button
          onClick={inspect}
          disabled={!readiness.data?.readiness.index_ref || !modelId || busy}
        >
          预览资料外发范围
        </Button>
        <Button variant="outline" onClick={refresh} disabled={busy}>
          刷新发布状态
        </Button>
      </div>
      {inspection && (
        <div className="space-y-3">
          <p>
            接收方：{inspection.destination_origin} ·{' '}
            {inspection.model_display_name}
          </p>
          <p>
            每次只发送你的问题和最多 5 段检索证据（合计不超过 32
            KiB）。资料范围或模型接入配置变化后需要重新确认。
          </p>
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              checked={ack}
              onChange={acknowledge}
              disabled={busy}
            />
            <span>
              我确认向以上模型发送所选固定版本中的证据。请勿发送未经许可外发的真实
              AICS 内容。
            </span>
          </label>
          <Button onClick={enable} disabled={!ack || busy}>
            确认并开启此资料范围的问答
          </Button>
        </div>
      )}
      {!!error && <AnswerFailure error={error} />}
    </section>
  );
}

function AnswerQuestions({
  scope,
  flow,
  permit,
}: {
  scope: LocalLexicalScope;
  flow: Flow;
  permit: AnswerPolicy;
}) {
  const [question, setQuestion] = useState(''),
    [askedQuestion, setAskedQuestion] = useState(''),
    [keywords, setKeywords] = useState(''),
    [pending, setPending] = useState<AnswerRequest | null>(null);
  const [result, setResult] = useState<AnswerResult | null>(null),
    [error, setError] = useState<unknown>(null),
    [busy, setBusy] = useState(false);
  const generation = useRef(0),
    permitRef = useRef(permit);
  permitRef.current = permit;
  useEffect(() => {
    generation.current++;
    setResult(null);
    setPending(null);
    setError(null);
  }, [permit.version, permit.enabled]);
  useEffect(() => {
    const invalidate = () => {
      generation.current++;
    };
    const hide = () => {
      if (document.hidden) {
        invalidate();
        setResult(null);
      }
    };
    document.addEventListener('visibilitychange', hide);
    return () => {
      invalidate();
      document.removeEventListener('visibilitychange', hide);
    };
  }, []);
  const changeQuestion = (event: ChangeEvent<HTMLTextAreaElement>) =>
    setQuestion(event.target.value);
  const changeKeywords = (event: ChangeEvent<HTMLInputElement>) =>
    setKeywords(event.target.value);
  const run = async (request: AnswerRequest) => {
    const captured = generation.current;
    setResult(null);
    setError(null);
    setAskedQuestion(request.question);
    setPending(request);
    setBusy(true);
    try {
      const answer = await flow.ask.mutateAsync({ body: request, permit });
      const current = await flow.policy.refetch();
      if (current.isError) {
        setResult(null);
        setError(current.error);
        return;
      }
      if (
        captured !== generation.current ||
        !current.data?.enabled ||
        current.data.version !== request.expected_policy_version ||
        !permitRef.current.enabled
      )
        return;
      setResult(answer);
      if (answer.state === 'completed') setPending(null);
    } catch (issue) {
      if (captured === generation.current) setError(issue);
    } finally {
      setBusy(false);
    }
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (
      busy ||
      !question.trim() ||
      contentBytes(question) > 4096 ||
      contentBytes(keywords) > 4096
    )
      return;
    void run({
      contract_version: KnowledgeAnswerVersion,
      expected_policy_version: permit.version,
      question: question.trim(),
      retrieval_query: keywords.trim() || null,
      idempotency_key: crypto.randomUUID(),
    });
  };
  const poll = () => {
    if (pending && !busy) void run(pending);
  };
  const disable = async () => {
    generation.current++;
    setResult(null);
    setPending(null);
    setError(null);
    try {
      await flow.setPolicy.mutateAsync({
        contract_version: KnowledgeAnswerVersion,
        expected_version: permit.version,
        enabled: false,
        scope: null,
        model_ref: null,
        confirmed: true,
        external_send_confirmed: false,
        idempotency_key: crypto.randomUUID(),
      });
    } catch (issue) {
      setError(issue);
    }
  };
  return (
    <div className="space-y-4">
      <div className="space-y-2 rounded border border-border-button p-4">
        <p>
          问答已开启 · 固定资料 {permit.scope?.selection.targets.length} 份
          {permit.consent_time &&
            ` · 确认于 ${dayjs(permit.consent_time).format('YYYY-MM-DD HH:mm')}`}
        </p>
        <p className="text-sm text-text-secondary">
          关闭后不再返回答案或缓存结果；此前已经发送给模型的文字无法撤回。
        </p>
        <Button
          variant="outline"
          onClick={disable}
          disabled={flow.setPolicy.isPending}
        >
          关闭模型问答
        </Button>
      </div>
      <form onSubmit={submit} className="space-y-3">
        <label className="block space-y-2">
          <span>你的问题</span>
          <textarea
            aria-label="你的问题"
            value={question}
            onChange={changeQuestion}
            disabled={busy}
            rows={4}
            className="w-full rounded border border-border-button bg-bg-base p-3"
          />
        </label>
        <label className="block space-y-2">
          <span>检索关键词（可选）</span>
          <Input
            aria-label="检索关键词（可选）"
            value={keywords}
            onChange={changeKeywords}
            disabled={busy}
            placeholder="留空时直接使用问题检索"
          />
        </label>
        <p className="text-sm text-text-secondary">
          答案只使用这个固定范围的证据。资料不足时会拒答；引用可展开并回到原文核对。
        </p>
        <Button
          type="submit"
          disabled={
            busy ||
            flow.setPolicy.isPending ||
            !question.trim() ||
            contentBytes(question) > 4096 ||
            contentBytes(keywords) > 4096
          }
        >
          {busy ? '正在检索和核对答案…' : pending ? '明确发起新问题' : '提问'}
        </Button>
      </form>
      {!!pending && !busy && (
        <Button variant="outline" onClick={poll}>
          查询本次结果（不重复调用模型）
        </Button>
      )}
      {!!error && <AnswerFailure error={error} />}
      {result?.state === 'running' && (
        <p role="status">原问题仍在处理，请稍后查询本次结果。</p>
      )}
      {result?.state === 'completed' && (
        <section
          aria-label="问答结果"
          className="space-y-3 rounded border border-border-button p-4"
        >
          <p className="text-sm text-text-secondary">问题：{askedQuestion}</p>
          <p className="whitespace-pre-wrap break-words">{result.answer}</p>
          {result.insufficient_evidence && (
            <p className="text-sm text-text-secondary">
              所选资料未提供足够证据。
            </p>
          )}
          {result.citations.map((item) => (
            <Citation key={item.id} item={item} domain={scope.domain} />
          ))}
        </section>
      )}
    </div>
  );
}

export default function KnowledgeAnswer({
  domain,
  owner,
  group,
}: {
  domain: string;
  owner: string;
  group?: string;
}) {
  const scope = { domain, owner },
    flow = useKnowledgeAnswer(scope),
    defaults = useFetchDefaultModels();
  const [choices, setChoices] = useState<LocalLexicalChoice[]>([]);
  const selection = useMemo<LocalLexicalSelection>(
    () => ({
      kind: 'fixed_representations',
      targets: choices.map((item) => item.target),
    }),
    [choices],
  );
  const add = (choice: LocalLexicalChoice) => {
    setChoices((current) =>
      current.length >= 32 ||
      current.some(
        (item) =>
          contentSameTarget(item.target, choice.target) ||
          item.target.source_ref.source_id ===
            choice.target.source_ref.source_id,
      )
        ? current
        : [...current, choice],
    );
  };
  const clear = () => setChoices([]),
    refresh = () => {
      void flow.policy.refetch();
    };
  const modelId =
    defaults.data.find((model) => model.model_type === 'chat' && model.enable)
      ?.model_id || '';
  if (flow.policy.isPending) return <CatalogueLoading />;
  if (flow.policy.isError)
    return (
      <div className="space-y-3">
        <AnswerFailure error={flow.policy.error} />
        <Button onClick={refresh}>刷新问答状态</Button>
      </div>
    );
  const permit = flow.policy.data;
  return (
    <section className="space-y-4" aria-label="知识库问答">
      <h2 className="text-lg font-semibold">知识库问答</h2>
      <p className="text-text-secondary">
        仅资料所有者可用。首次默认关闭，开启前明确确认固定资料版本和模型接收方。
      </p>
      {permit.enabled ? (
        <AnswerQuestions scope={scope} flow={flow} permit={permit} />
      ) : (
        <>
          <p>当前已关闭，不会发送问题或资料给模型。</p>
          <LocalLexicalPicker
            scope={scope}
            group={group}
            onAdd={add}
            disabled={flow.setPolicy.isPending}
          />
          {!!choices.length && (
            <div className="space-y-2">
              <ul>
                {choices.map((item) => (
                  <li key={item.target.source_ref.source_id}>{item.path}</li>
                ))}
              </ul>
              <Button variant="outline" onClick={clear}>
                清空所选资料
              </Button>
              <ConfirmSnapshot
                key={choices
                  .map(
                    (item) =>
                      item.target.source_ref.source_revision_id +
                      item.target.representation_ref.id,
                  )
                  .join(':')}
                scope={scope}
                selection={selection}
                flow={flow}
                modelId={modelId}
              />
            </div>
          )}
        </>
      )}
    </section>
  );
}

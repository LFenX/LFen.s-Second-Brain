import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import service, {
  AnswerRequestError,
} from '@/services/knowledge-answer-service';
import { getKnowledgeSessionScope } from './use-knowledge-foundation';
import type { LocalLexicalScope } from '@/interfaces/request/knowledge-local-lexical';
import type { AnswerPolicy } from '@/interfaces/database/knowledge-answer';
import type {
  AnswerInspectRequest,
  AnswerSetPolicyRequest,
  AnswerRequest,
} from '@/interfaces/request/knowledge-answer';

export const AnswerKeys = {
  root: () => ['knowledge-answer', '0.1.0-candidate'] as const,
  scope: (session: number, scope: LocalLexicalScope) =>
    [...AnswerKeys.root(), session, scope.owner, scope.domain] as const,
  policy: (session: number, scope: LocalLexicalScope) =>
    [...AnswerKeys.scope(session, scope), 'policy'] as const,
  action: (session: number, scope: LocalLexicalScope, operation: string) =>
    [...AnswerKeys.scope(session, scope), operation] as const,
};
export function useKnowledgeAnswer(scope: LocalLexicalScope) {
  const client = useQueryClient(),
    session = getKnowledgeSessionScope();
  const scoped = async <T>(work: () => Promise<T>) => {
    const check = () => {
      if (session !== getKnowledgeSessionScope())
        throw new AnswerRequestError(401, 'unauthenticated');
    };
    check();
    try {
      const result = await work();
      check();
      return result;
    } catch (error) {
      check();
      throw error;
    }
  };
  const policy = useQuery({
    queryKey: AnswerKeys.policy(session, scope),
    queryFn: ({ signal }) => scoped(() => service.policy(scope, signal)),
    retry: false,
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
  });
  const inspect = useMutation({
    mutationKey: AnswerKeys.action(session, scope, 'inspect'),
    retry: false,
    mutationFn: (body: AnswerInspectRequest) =>
      scoped(() => service.inspect(scope, body)),
  });
  const setPolicy = useMutation({
    mutationKey: AnswerKeys.action(session, scope, 'setPolicy'),
    retry: false,
    mutationFn: (body: AnswerSetPolicyRequest) =>
      scoped(() => service.setPolicy(scope, body)),
    onSuccess: (value) =>
      client.setQueryData(AnswerKeys.policy(session, scope), value),
    onError: () =>
      client.invalidateQueries({ queryKey: AnswerKeys.policy(session, scope) }),
  });
  const ask = useMutation({
    mutationKey: AnswerKeys.action(session, scope, 'answer'),
    retry: false,
    mutationFn: ({
      body,
      permit,
    }: {
      body: AnswerRequest;
      permit: AnswerPolicy;
    }) => scoped(() => service.answer(scope, body, permit)),
  });
  return { policy, inspect, setPolicy, ask };
}

import type {
  LocalLexicalIndexRef,
  LocalLexicalSelection,
  LocalLexicalEvidenceRef,
} from './knowledge-local-lexical';

export const KnowledgeAnswerVersion = '0.1.0-candidate' as const;
export type AnswerScope = {
  index_ref: LocalLexicalIndexRef;
  selection: LocalLexicalSelection;
};
export type AnswerModelRef = { model_id: string; configuration_sha256: string };
export type AnswerPolicy = {
  domain_id: string;
  version: number;
  enabled: boolean;
  scope: AnswerScope | null;
  scope_sha256: string | null;
  model_ref: AnswerModelRef | null;
  consent_actor: string | null;
  consent_time: string | null;
};
export type AnswerInspection = {
  policy_version: number;
  scope: AnswerScope;
  scope_sha256: string;
  model_ref: AnswerModelRef;
  model_display_name: string;
  destination_origin: string;
  readiness: 'ready';
  model_calls: 0;
  limits: {
    max_targets: 32;
    retrieval_limit: 5;
    max_context_utf8_bytes: 32768;
    max_question_utf8_bytes: 4096;
    max_output_tokens: 1024;
    max_answer_utf8_bytes: 8192;
  };
};
export type AnswerCitation = {
  id: string;
  evidence_ref: LocalLexicalEvidenceRef;
  text: string;
  byte_length: number;
  sha256: string;
};
export type AnswerRunning = {
  answer_id: string;
  state: 'running';
  policy_version: number;
  scope_sha256: string;
  model_ref: AnswerModelRef;
  provider_calls: 0 | 1;
  replayed: boolean;
};
export type AnswerCompleted = Omit<AnswerRunning, 'state'> & {
  state: 'completed';
  answer: string;
  insufficient_evidence: boolean;
  citations: AnswerCitation[];
  usage: {
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
  } | null;
};
export type AnswerResult = AnswerRunning | AnswerCompleted;
export type AnswerData = {
  inspect: AnswerInspection;
  getPolicy: AnswerPolicy;
  setPolicy: AnswerPolicy;
  answer: AnswerResult;
};
export type AnswerOperation = keyof AnswerData;

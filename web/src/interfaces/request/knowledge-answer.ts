import type {
  AnswerModelRef,
  AnswerScope,
  KnowledgeAnswerVersion,
} from '../database/knowledge-answer';

type Version = typeof KnowledgeAnswerVersion;
export type AnswerInspectRequest = {
  contract_version: Version;
  scope: AnswerScope;
  model_id: string;
};
export type AnswerSetPolicyRequest = {
  contract_version: Version;
  expected_version: number;
  enabled: boolean;
  scope: AnswerScope | null;
  model_ref: AnswerModelRef | null;
  confirmed: true;
  external_send_confirmed: boolean;
  idempotency_key: string;
};
export type AnswerRequest = {
  contract_version: Version;
  expected_policy_version: number;
  question: string;
  retrieval_query: string | null;
  idempotency_key: string;
};
export type AnswerRequests = {
  inspect: AnswerInspectRequest;
  getPolicy: { contract_version: Version };
  setPolicy: AnswerSetPolicyRequest;
  answer: AnswerRequest;
};

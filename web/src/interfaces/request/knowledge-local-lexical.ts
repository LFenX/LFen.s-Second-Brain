import type {
  LocalLexicalSelection,
  LocalLexicalIndexRef,
  LocalLexicalEvidenceRef,
} from '../database/knowledge-local-lexical';

export type LocalLexicalScope = { owner: string; domain: string };

export type LocalLexicalPolicyBody = {
  contract_version: '0.1.0-candidate';
  idempotency_key: string;
  expected_local_policy_version: number;
  expected_foundation_policy_version: number;
  enabled: boolean;
};

export type LocalLexicalInspectBody = {
  contract_version: '0.2.0-candidate';
  selection: LocalLexicalSelection;
};

export type LocalLexicalCreateBody = {
  contract_version: '0.2.0-candidate';
  idempotency_key: string;
  expected_local_policy_version: number;
  expected_content_policy_version: number;
  expected_foundation_policy_version: number;
  expected_profile_sha256: string;
  selection: LocalLexicalSelection;
};

export type LocalLexicalJobBody = {
  contract_version: '0.2.0-candidate';
  job_id: string;
};

export type LocalLexicalSearchBody = {
  contract_version: '0.2.0-candidate';
  index_ref: LocalLexicalIndexRef;
  selection: LocalLexicalSelection;
  query: string;
  limit: number;
};

export type LocalLexicalReadBody = {
  contract_version: '0.2.0-candidate';
  evidence_ref: LocalLexicalEvidenceRef;
};

export type LocalLexicalBodies = {
  getLocalPolicy: undefined;
  changeLocalPolicy: LocalLexicalPolicyBody;
  inspectIndex: LocalLexicalInspectBody;
  createIndex: LocalLexicalCreateBody;
  getIndexJob: LocalLexicalJobBody;
  search: LocalLexicalSearchBody;
  read: LocalLexicalReadBody;
};
export type LocalLexicalWrite =
  | { operation: 'changeLocalPolicy'; body: LocalLexicalPolicyBody }
  | { operation: 'createIndex'; body: LocalLexicalCreateBody }
  | {
      operation: 'changeContentPolicy';
      body: import('./knowledge-consumption').ContentRequests['content-policy-change'];
    };

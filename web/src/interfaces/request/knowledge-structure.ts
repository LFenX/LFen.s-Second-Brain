import type {
  StructureVersion,
  StructureKey,
  StructureID,
  StructureHash,
  StructureMember,
  StructureAlias,
} from '@/interfaces/database/knowledge-structure';

export type StructurePolicyChangeRequest = {
  expected_policy_version: StructureVersion;
  expected_structure_policy_version: StructureVersion;
  enabled: boolean;
  confirmed: true;
  idempotency_key: StructureKey;
};

export type StructureTaskRequest = {
  project_id: StructureID;
  manifest_id: StructureID;
  expected_active_view_id: StructureHash | null;
  expected_policy_version: StructureVersion;
  expected_structure_policy_version: StructureVersion;
  members: StructureMember[];
  aliases: StructureAlias[];
  confirmed: true;
  idempotency_key: StructureKey;
};

export type StructureCancelRequest = {
  expected_task_revision: StructureVersion;
  confirmed: true;
  idempotency_key: StructureKey;
};

export type StructurePageQuery = { limit?: number; cursor?: string };
export type StructureTaskQuery = StructurePageQuery & { project_id: string };
export type StructureNodeQuery = StructurePageQuery & {
  source_id: string;
  parent_id?: string;
};
export type StructureRelationQuery = StructurePageQuery & {
  node_id: string;
  direction?: 'in' | 'out' | 'both';
  kind?: import('@/interfaces/database/knowledge-structure').StructureRelationKind;
};
export type StructureContentQuery = { start_byte?: number; end_byte?: number };

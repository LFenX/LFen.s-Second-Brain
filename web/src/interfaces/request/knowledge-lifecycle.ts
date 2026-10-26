import type {
  LifecycleID,
  LifecycleVersion,
  LifecycleClassification,
} from '@/interfaces/database/knowledge-lifecycle';

export type LifecycleAction = 'reclassify' | 'move' | 'archive' | 'restore';
export type LifecycleCommand =
  | LifecycleStateCommand
  | LifecycleReclassifyCommand
  | LifecycleMoveCommand;
export type LifecycleListQuery = {
  state?: 'active' | 'archived';
  group_scope?: 'all' | 'ungrouped' | 'exact';
  group_id?: string;
  component_side?: LifecycleClassification['component_side'];
  language?: string;
  limit?: number;
  cursor?: string;
};
export type LifecycleTargetQuery = Pick<
  LifecycleMoveCommand,
  | 'relative_path'
  | 'expected_source_revision_id'
  | 'expected_lifecycle_version'
  | 'expected_policy_version'
>;

export type LifecycleStateCommand = {
  idempotency_key: string;
  expected_source_revision_id: LifecycleID;
  expected_lifecycle_version: LifecycleVersion;
  expected_policy_version: LifecycleVersion;
  confirmed: true;
};

export type LifecycleReclassifyCommand = {
  idempotency_key: string;
  expected_source_revision_id: LifecycleID;
  expected_lifecycle_version: LifecycleVersion;
  expected_policy_version: LifecycleVersion;
  confirmed: true;
  classification: LifecycleClassification;
};

export type LifecycleMoveCommand = {
  idempotency_key: string;
  expected_source_revision_id: LifecycleID;
  expected_lifecycle_version: LifecycleVersion;
  expected_policy_version: LifecycleVersion;
  confirmed: true;
  relative_path: string;
};

import type {
  AccessDelivery,
  AccessEvidenceReadSelector,
  AccessGeneration,
  AccessID,
  AccessKey,
  AccessMode,
  AccessModuleScope,
  AccessSubject,
  AccessTime,
  AccessUnitReadSelector,
  AccessVersion,
} from '@/interfaces/database/knowledge-access';

export type AccessSetDepartmentRequest = {
  contract_version: AccessVersion;
  tenant_id: AccessID;
  department_id: AccessID | null;
  expected_version: AccessGeneration;
  name: string;
  enabled: boolean;
  user_ids: AccessID[];
  confirmed: true;
  idempotency_key: AccessKey;
};
export type AccessListDepartmentsRequest = {
  contract_version: AccessVersion;
  tenant_id: AccessID;
};
export type AccessSetPolicyRequest = {
  contract_version: AccessVersion;
  domain_id: AccessID;
  expected_version: AccessGeneration;
  enabled: boolean;
  confirmed: true;
  idempotency_key: AccessKey;
};

export type AccessSetModuleRequest = {
  contract_version: AccessVersion;
  domain_id: AccessID;
  module_id: AccessID | null;
  expected_version: AccessGeneration;
  name: string;
  enabled: boolean;
  scope: AccessModuleScope;
  confirmed: true;
  idempotency_key: AccessKey;
};

export type AccessGetModuleAdminRequest = {
  contract_version: AccessVersion;
  domain_id: AccessID;
  module_id: AccessID;
};

export type AccessListModulesRequest = {
  contract_version: AccessVersion;
  tenant_id: AccessID;
  domain_id: AccessID | null;
};

export type AccessCreateGrantRequest = {
  contract_version: AccessVersion;
  domain_id: AccessID;
  module_id: AccessID;
  expected_module_version: number;
  subject: AccessSubject;
  mode: AccessMode;
  delivery: AccessDelivery;
  expires_at: AccessTime;
  confirmed: true;
  idempotency_key: AccessKey;
};

export type AccessRevokeGrantRequest = {
  contract_version: AccessVersion;
  domain_id: AccessID;
  grant_id: AccessID;
  confirmed: true;
  idempotency_key: AccessKey;
};

export type AccessInspectRequest = {
  contract_version: AccessVersion;
  domain_id: AccessID;
  module_id: AccessID;
  mode: AccessMode;
  delivery: AccessDelivery;
  source_ids: AccessID[] | null;
};

export type AccessSearchRequest = {
  contract_version: AccessVersion;
  view_id: AccessID;
  query: string;
  limit: number;
};

export type AccessReadRequest = {
  contract_version: AccessVersion;
  selector: AccessUnitReadSelector | AccessEvidenceReadSelector;
};

export interface AccessRequests {
  setDepartment: AccessSetDepartmentRequest;
  listDepartments: AccessListDepartmentsRequest;
  setPolicy: AccessSetPolicyRequest;
  setModule: AccessSetModuleRequest;
  getModuleAdmin: AccessGetModuleAdminRequest;
  listModules: AccessListModulesRequest;
  createGrant: AccessCreateGrantRequest;
  revokeGrant: AccessRevokeGrantRequest;
  inspect: AccessInspectRequest;
  search: AccessSearchRequest;
  read: AccessReadRequest;
}
export type AccessOperation = keyof AccessRequests;
export type AccessWriteOperation =
  | 'setDepartment'
  | 'setPolicy'
  | 'setModule'
  | 'createGrant'
  | 'revokeGrant';

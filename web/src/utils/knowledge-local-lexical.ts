import Ajv from 'ajv';
import Ajv2020 from 'ajv/dist/2020';
import type { ValidateFunction } from 'ajv';
import addFormats from 'ajv-formats';
import dayjs from 'dayjs';
import PolicyContract from '../../../docs/knowledge-core-local-lexical-v0.1.candidate.schema.json';
import ContentContract from '../../../docs/knowledge-core-content-v0.2.parsed.candidate.schema.json';
import type {
  LocalLexicalCoverage,
  LocalLexicalData,
  LocalLexicalEnvelope,
  LocalLexicalEvidenceRef,
  LocalLexicalJob,
  LocalLexicalOperation,
  LocalLexicalPolicy,
  LocalLexicalPolicyReceipt,
  LocalLexicalReadData,
  LocalLexicalReadiness,
  LocalLexicalReadinessData,
  LocalLexicalSearchData,
  LocalLexicalSelection,
  LocalLexicalIndexRef,
  LocalLexicalProfile,
} from '@/interfaces/database/knowledge-local-lexical';
import type {
  LocalLexicalBodies,
  LocalLexicalCreateBody,
  LocalLexicalInspectBody,
  LocalLexicalPolicyBody,
  LocalLexicalReadBody,
  LocalLexicalScope,
  LocalLexicalSearchBody,
  LocalLexicalJobBody,
} from '@/interfaces/request/knowledge-local-lexical';
import type { ContentTarget } from '@/interfaces/database/knowledge-consumption';
import {
  contentBytes,
  contentCanonical,
  contentDigest,
  contentFixedTarget,
  contentBoundNode,
  contentSameTarget,
  contentNodeKey,
  checkContentNode,
  contentTableText,
  contentWireBytes,
  contentTimestamp,
} from '@/utils/knowledge-consumption';
import { webSha256 } from '@/utils/knowledge-web';

// Exact new definitions from the assigned schema03; the 82 shared definitions
// are byte-equivalent in meaning and reuse the same 0.2 content schema.
const LexicalDefinitions = {
  LexicalBinding: {
    type: 'object',
    additionalProperties: false,
    required: [
      'index_ref',
      'selection',
      'selection_sha256',
      'local_policy_version',
      'content_policy_version',
      'foundation_policy_version',
      'profile_sha256',
      'tokenizer_producer_sha256',
      'engine',
      'execution_profile',
      'parent_expansion',
      'published_at',
    ],
    properties: {
      index_ref: {
        $ref: '#/$defs/LexicalIndexRef',
      },
      selection: {
        $ref: '#/$defs/LexicalSelection',
      },
      selection_sha256: {
        $ref: '#/$defs/Hash64',
      },
      local_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      content_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      foundation_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      profile_sha256: {
        $ref: '#/$defs/Hash64',
      },
      tokenizer_producer_sha256: {
        $ref: '#/$defs/Hash64',
      },
      engine: {
        const: 'elasticsearch',
      },
      execution_profile: {
        const: 'local_lexical_no_models/0.2.0',
      },
      parent_expansion: {
        const: 'disabled',
      },
      published_at: {
        type: 'string',
        pattern:
          '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]{1,9})?Z$',
      },
    },
  },
  LexicalContext: {
    type: 'object',
    additionalProperties: false,
    required: [
      'node',
      'node_omission_reason',
      'typed_members',
      'typed_members_scope',
      'typed_members_complete',
      'omitted_member_count',
      'omission_reasons',
      'manifest_sha256',
      'membership',
      'representation_state',
      'parent_expansion',
      'metadata_byte_length',
    ],
    properties: {
      node: {
        anyOf: [
          {
            $ref: '#/$defs/BoundNode',
          },
          {
            type: 'null',
          },
        ],
      },
      node_omission_reason: {
        anyOf: [
          {
            const: 'metadata_budget',
          },
          {
            type: 'null',
          },
        ],
      },
      typed_members: {
        type: 'array',
        minItems: 0,
        maxItems: 32,
        items: {
          $ref: '#/$defs/LexicalTypedMember',
        },
      },
      typed_members_scope: {
        const: 'whole_nodes_contained_in_evidence_range',
      },
      typed_members_complete: {
        type: 'boolean',
      },
      omitted_member_count: {
        type: 'integer',
        minimum: 0,
        maximum: 32768,
      },
      omission_reasons: {
        type: 'array',
        minItems: 0,
        maxItems: 7,
        items: {
          enum: [
            'typed_members_not_defined_for_source_family',
            'representation_contains_unsearchable_nodes',
            'whole_member_outside_evidence',
            'unsearchable_member',
            'empty_member_not_anchored',
            'member_limit',
            'metadata_budget',
          ],
        },
      },
      manifest_sha256: {
        anyOf: [
          {
            $ref: '#/$defs/Hash64',
          },
          {
            type: 'null',
          },
        ],
      },
      membership: {
        enum: [
          'stored_c1_root',
          'validated_r1_passage',
          'validated_document_manifest',
          'validated_parsed_manifest',
        ],
      },
      representation_state: {
        enum: ['ready', 'partial', 'empty'],
      },
      parent_expansion: {
        const: 'disabled',
      },
      metadata_byte_length: {
        type: 'integer',
        minimum: 1,
        maximum: 65536,
      },
    },
  },
  LexicalCoverage: {
    type: 'object',
    additionalProperties: false,
    required: [
      'target',
      'scope',
      'coordinate_space',
      'reading_text_sha256',
      'expected_reading_bytes',
      'covered_reading_bytes',
      'indexed_unit_count',
      'reading_text_complete',
      'representation_state',
      'unsearchable_node_count',
      'original_coverage',
      'text_index_state',
    ],
    properties: {
      target: {
        $ref: '#/$defs/Target',
      },
      scope: {
        const: 'selected_representation_reading_utf8',
      },
      coordinate_space: {
        enum: ['source_utf8', 'derived_utf8'],
      },
      reading_text_sha256: {
        $ref: '#/$defs/Hash64',
      },
      expected_reading_bytes: {
        type: 'integer',
        minimum: 0,
        maximum: 4194304,
      },
      covered_reading_bytes: {
        type: 'integer',
        minimum: 0,
        maximum: 4194304,
      },
      indexed_unit_count: {
        type: 'integer',
        minimum: 0,
        maximum: 32768,
      },
      reading_text_complete: {
        const: true,
      },
      representation_state: {
        enum: ['ready', 'partial', 'empty'],
      },
      unsearchable_node_count: {
        type: 'integer',
        minimum: 0,
        maximum: 32768,
      },
      original_coverage: {
        enum: ['source_utf8_exact', 'not_asserted'],
      },
      text_index_state: {
        enum: ['empty', 'available'],
      },
    },
    allOf: [
      {
        if: {
          properties: {
            coordinate_space: {
              const: 'source_utf8',
            },
          },
        },
        then: {
          properties: {
            original_coverage: {
              const: 'source_utf8_exact',
            },
            expected_reading_bytes: {
              type: 'integer',
              minimum: 0,
              maximum: 2097152,
            },
            covered_reading_bytes: {
              type: 'integer',
              minimum: 0,
              maximum: 2097152,
            },
          },
        },
        else: {
          properties: {
            original_coverage: {
              const: 'not_asserted',
            },
          },
        },
      },
    ],
  },
  LexicalCreateRequest: {
    type: 'object',
    additionalProperties: false,
    required: [
      'contract_version',
      'idempotency_key',
      'expected_local_policy_version',
      'expected_content_policy_version',
      'expected_foundation_policy_version',
      'expected_profile_sha256',
      'selection',
    ],
    properties: {
      contract_version: {
        $ref: '#/$defs/Version',
      },
      idempotency_key: {
        type: 'string',
        minLength: 1,
        maxLength: 128,
        pattern: '^[A-Za-z0-9_-]{1,128}$',
      },
      expected_local_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      expected_content_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      expected_foundation_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      expected_profile_sha256: {
        $ref: '#/$defs/Hash64',
      },
      selection: {
        $ref: '#/$defs/LexicalSelection',
      },
    },
  },
  LexicalError: {
    type: 'object',
    additionalProperties: false,
    required: [
      'operation',
      'contract_version',
      'request_id',
      'data',
      'code',
      'message',
      'outcome_known',
      'retry_same_key',
    ],
    properties: {
      operation: {
        enum: ['inspectIndex', 'createIndex', 'getIndexJob', 'search', 'read'],
      },
      contract_version: {
        $ref: '#/$defs/Version',
      },
      request_id: {
        $ref: '#/$defs/ID32',
      },
      data: {
        type: 'null',
      },
      code: {
        enum: [
          'invalid_request',
          'unauthenticated',
          'not_found',
          'contract_mismatch',
          'policy_disabled',
          'policy_changed',
          'profile_changed',
          'source_changed',
          'idempotency_conflict',
          'index_unindexed',
          'index_building',
          'index_coverage_gap',
          'index_stale',
          'index_failed',
          'coverage_gap',
          'unit_oversize',
          'unsupported_representation',
          'source_unavailable',
          'integrity_error',
          'capacity_exceeded',
          'empty_content',
          'unavailable',
        ],
      },
      message: {
        const: 'Content lexical operation could not be completed',
      },
      outcome_known: {
        type: 'boolean',
      },
      retry_same_key: {
        type: 'boolean',
      },
    },
  },
  LexicalEvidence: {
    type: 'object',
    additionalProperties: false,
    required: [
      'evidence_version',
      'index_ref',
      'index_row_id',
      'address',
      'coordinate_space',
      'reading_text_sha256',
      'range',
      'locator',
      'original_position',
      'coordinate_precision',
      'assembly',
    ],
    properties: {
      evidence_version: {
        $ref: '#/$defs/Version',
      },
      index_ref: {
        $ref: '#/$defs/LexicalIndexRef',
      },
      index_row_id: {
        $ref: '#/$defs/Hash64',
      },
      address: {
        $ref: '#/$defs/Address',
      },
      coordinate_space: {
        enum: ['source_utf8', 'derived_utf8'],
      },
      reading_text_sha256: {
        $ref: '#/$defs/Hash64',
      },
      range: {
        $ref: '#/$defs/LexicalRange',
      },
      locator: {
        $ref: '#/$defs/Locator',
      },
      original_position: {
        anyOf: [
          {
            $ref: '#/$defs/R2_OriginalPosition',
          },
          {
            type: 'null',
          },
        ],
      },
      coordinate_precision: {
        enum: ['source_bytes', 'derived_range'],
      },
      assembly: {
        enum: ['c1_root_subrange_exact_bytes', 'existing_passage_full_range'],
      },
    },
    allOf: [
      {
        if: {
          properties: {
            address: {
              properties: {
                target: {
                  properties: {
                    representation_ref: {
                      properties: {
                        kind: {
                          enum: ['source_revision', 'structure_view'],
                        },
                      },
                    },
                  },
                },
              },
            },
          },
        },
        then: {
          properties: {
            coordinate_space: {
              const: 'source_utf8',
            },
            coordinate_precision: {
              const: 'source_bytes',
            },
            original_position: {
              type: 'null',
            },
            locator: {
              properties: {
                kind: {
                  const: 'source_utf8',
                },
              },
            },
            range: {
              properties: {
                end_byte: {
                  type: 'integer',
                  minimum: 1,
                  maximum: 2097152,
                },
              },
            },
          },
        },
        else: {
          properties: {
            coordinate_space: {
              const: 'derived_utf8',
            },
            coordinate_precision: {
              const: 'derived_range',
            },
            locator: {
              properties: {
                kind: {
                  const: 'document',
                },
                document: {
                  properties: {
                    scheme: {
                      const: 'derived_passage_v1',
                    },
                  },
                },
              },
            },
          },
        },
      },
      {
        if: {
          properties: {
            address: {
              properties: {
                target: {
                  properties: {
                    representation_ref: {
                      properties: {
                        kind: {
                          const: 'source_revision',
                        },
                      },
                    },
                  },
                },
              },
            },
          },
        },
        then: {
          properties: {
            assembly: {
              const: 'c1_root_subrange_exact_bytes',
            },
          },
        },
        else: {
          properties: {
            assembly: {
              const: 'existing_passage_full_range',
            },
          },
        },
      },
    ],
    $comment:
      'Go additionally enforces positive exact byte range <=8192, range digest, source/representation/node consistency, fixed publication membership and UTF-8 byte boundaries.',
  },
  LexicalHit: {
    type: 'object',
    additionalProperties: false,
    required: [
      'evidence_ref',
      'text',
      'score',
      'match_type',
      'source_currentness',
    ],
    properties: {
      evidence_ref: {
        $ref: '#/$defs/LexicalEvidence',
      },
      text: {
        type: 'string',
        maxLength: 8192,
      },
      score: {
        type: 'number',
      },
      match_type: {
        const: 'lexical',
      },
      source_currentness: {
        enum: ['current', 'historical'],
      },
    },
  },
  LexicalIndexRef: {
    type: 'object',
    additionalProperties: false,
    required: ['index_id', 'manifest_sha256'],
    properties: {
      index_id: {
        $ref: '#/$defs/Hash64',
      },
      manifest_sha256: {
        $ref: '#/$defs/Hash64',
      },
    },
  },
  LexicalInspectData: {
    type: 'object',
    additionalProperties: false,
    required: ['effective_policy', 'profile', 'readiness'],
    properties: {
      effective_policy: {
        $ref: '#/$defs/LexicalPolicy',
      },
      profile: {
        $ref: '#/$defs/LexicalProfile',
      },
      readiness: {
        $ref: '#/$defs/LexicalReadiness',
      },
    },
  },
  LexicalInspectRequest: {
    type: 'object',
    additionalProperties: false,
    required: ['contract_version', 'selection'],
    properties: {
      contract_version: {
        $ref: '#/$defs/Version',
      },
      selection: {
        $ref: '#/$defs/LexicalSelection',
      },
    },
  },
  LexicalJob: {
    type: 'object',
    additionalProperties: false,
    required: [
      'job_id',
      'idempotency_key',
      'request_sha256',
      'state',
      'selection',
      'captured_local_policy_version',
      'captured_content_policy_version',
      'captured_foundation_policy_version',
      'captured_profile_sha256',
      'captured_tokenizer_producer_sha256',
      'index_ref',
      'readiness',
      'created_at',
      'updated_at',
    ],
    properties: {
      job_id: {
        $ref: '#/$defs/ID32',
      },
      idempotency_key: {
        type: 'string',
        minLength: 1,
        maxLength: 128,
        pattern: '^[A-Za-z0-9_-]{1,128}$',
      },
      request_sha256: {
        $ref: '#/$defs/Hash64',
      },
      state: {
        enum: ['queued', 'building', 'published', 'failed'],
      },
      selection: {
        $ref: '#/$defs/LexicalSelection',
      },
      captured_local_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      captured_content_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      captured_foundation_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      captured_profile_sha256: {
        $ref: '#/$defs/Hash64',
      },
      captured_tokenizer_producer_sha256: {
        $ref: '#/$defs/Hash64',
      },
      index_ref: {
        anyOf: [
          {
            $ref: '#/$defs/LexicalIndexRef',
          },
          {
            type: 'null',
          },
        ],
      },
      readiness: {
        $ref: '#/$defs/LexicalReadiness',
      },
      created_at: {
        type: 'string',
        pattern:
          '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]{1,9})?Z$',
      },
      updated_at: {
        type: 'string',
        pattern:
          '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]{1,9})?Z$',
      },
    },
  },
  LexicalJobRequest: {
    type: 'object',
    additionalProperties: false,
    required: ['contract_version', 'job_id'],
    properties: {
      contract_version: {
        $ref: '#/$defs/Version',
      },
      job_id: {
        $ref: '#/$defs/ID32',
      },
    },
  },
  LexicalPolicy: {
    type: 'object',
    additionalProperties: false,
    required: [
      'domain_id',
      'local_policy_version',
      'content_policy_version',
      'foundation_policy_version',
      'local_lexical_enabled',
      'content_lexical_enabled',
      'enabled',
      'mode',
    ],
    properties: {
      domain_id: {
        $ref: '#/$defs/ID32',
      },
      local_policy_version: {
        type: 'integer',
        minimum: 0,
        maximum: 9007199254740991,
      },
      content_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      foundation_policy_version: {
        type: 'integer',
        minimum: 1,
        maximum: 9007199254740991,
      },
      local_lexical_enabled: {
        type: 'boolean',
      },
      content_lexical_enabled: {
        type: 'boolean',
      },
      enabled: {
        type: 'boolean',
      },
      mode: {
        const: 'owner_only_local_lexical',
      },
    },
  },
  LexicalProfile: {
    type: 'object',
    additionalProperties: false,
    required: [
      'profile_sha256',
      'tokenizer_producer_sha256',
      'language',
      'strategies',
      'engine',
      'execution_profile',
      'max_unit_bytes',
      'parent_expansion',
      'models_enabled',
      'remote_fetches',
    ],
    properties: {
      profile_sha256: {
        $ref: '#/$defs/Hash64',
      },
      tokenizer_producer_sha256: {
        $ref: '#/$defs/Hash64',
      },
      language: {
        type: 'string',
        maxLength: 4096,
      },
      strategies: {
        const: [
          'c1_exact_utf8_ranges',
          'r1_existing_passage',
          'r2_existing_passage',
          'parsed_existing_passage',
        ],
      },
      engine: {
        const: 'elasticsearch',
      },
      execution_profile: {
        const: 'local_lexical_no_models/0.2.0',
      },
      max_unit_bytes: {
        const: 8192,
      },
      parent_expansion: {
        const: 'disabled',
      },
      models_enabled: {
        const: false,
      },
      remote_fetches: {
        const: false,
      },
    },
  },
  LexicalRange: {
    type: 'object',
    additionalProperties: false,
    required: ['start_byte', 'end_byte', 'sha256'],
    properties: {
      start_byte: {
        type: 'integer',
        minimum: 0,
        maximum: 4194304,
      },
      end_byte: {
        type: 'integer',
        minimum: 1,
        maximum: 4194304,
      },
      sha256: {
        $ref: '#/$defs/Hash64',
      },
    },
  },
  LexicalReadData: {
    type: 'object',
    additionalProperties: false,
    required: [
      'evidence_ref',
      'text',
      'encoding',
      'byte_length',
      'source_currentness',
      'complete',
      'context',
    ],
    properties: {
      evidence_ref: {
        $ref: '#/$defs/LexicalEvidence',
      },
      text: {
        type: 'string',
        minLength: 1,
        maxLength: 8192,
      },
      encoding: {
        const: 'utf-8',
      },
      byte_length: {
        type: 'integer',
        minimum: 1,
        maximum: 8192,
      },
      source_currentness: {
        enum: ['current', 'historical'],
      },
      complete: {
        const: true,
      },
      context: {
        $ref: '#/$defs/LexicalContext',
      },
    },
  },
  LexicalReadRequest: {
    type: 'object',
    additionalProperties: false,
    required: ['contract_version', 'evidence_ref'],
    properties: {
      contract_version: {
        $ref: '#/$defs/Version',
      },
      evidence_ref: {
        $ref: '#/$defs/LexicalEvidence',
      },
    },
  },
  LexicalReadiness: {
    type: 'object',
    additionalProperties: false,
    required: [
      'kind',
      'publication',
      'selection',
      'selection_sha256',
      'snapshot_currentness',
      'index_ref',
      'coverage',
      'diagnostic_codes',
    ],
    properties: {
      kind: {
        enum: ['unindexed', 'empty', 'building', 'failed', 'ready', 'stale'],
      },
      publication: {
        enum: ['none', 'published'],
      },
      selection: {
        $ref: '#/$defs/LexicalSelection',
      },
      selection_sha256: {
        $ref: '#/$defs/Hash64',
      },
      snapshot_currentness: {
        enum: ['current', 'historical', 'mixed'],
      },
      index_ref: {
        anyOf: [
          {
            $ref: '#/$defs/LexicalIndexRef',
          },
          {
            type: 'null',
          },
        ],
      },
      coverage: {
        type: 'array',
        minItems: 0,
        maxItems: 32,
        items: {
          $ref: '#/$defs/LexicalCoverage',
        },
      },
      diagnostic_codes: {
        type: 'array',
        minItems: 0,
        maxItems: 8,
        items: {
          enum: [
            'none',
            'building',
            'policy_disabled',
            'policy_changed',
            'profile_changed',
            'source_changed',
            'empty_content',
            'unit_oversize',
            'unsupported_representation',
            'source_unavailable',
            'integrity_error',
            'coverage_gap',
            'index_failed',
          ],
        },
      },
    },
  },
  LexicalSearchData: {
    type: 'object',
    additionalProperties: false,
    required: [
      'binding',
      'coverage',
      'query',
      'requested_limit',
      'returned_count',
      'hits',
      'total_matches',
      'pagination',
      'ranking',
      'parent_expansion',
    ],
    properties: {
      binding: {
        $ref: '#/$defs/LexicalBinding',
      },
      coverage: {
        type: 'array',
        minItems: 1,
        maxItems: 32,
        items: {
          $ref: '#/$defs/LexicalCoverage',
        },
      },
      query: {
        type: 'string',
        minLength: 1,
        maxLength: 4096,
      },
      requested_limit: {
        type: 'integer',
        minimum: 1,
        maximum: 20,
      },
      returned_count: {
        type: 'integer',
        minimum: 0,
        maximum: 20,
      },
      hits: {
        type: 'array',
        minItems: 0,
        maxItems: 20,
        items: {
          $ref: '#/$defs/LexicalHit',
        },
      },
      total_matches: {
        type: 'null',
      },
      pagination: {
        const: 'none',
      },
      ranking: {
        const: 'native_lexical_only',
      },
      parent_expansion: {
        const: 'disabled',
      },
    },
  },
  LexicalSearchRequest: {
    type: 'object',
    additionalProperties: false,
    required: ['contract_version', 'index_ref', 'selection', 'query', 'limit'],
    properties: {
      contract_version: {
        $ref: '#/$defs/Version',
      },
      index_ref: {
        $ref: '#/$defs/LexicalIndexRef',
      },
      selection: {
        $ref: '#/$defs/LexicalSelection',
      },
      query: {
        type: 'string',
        minLength: 1,
        maxLength: 4096,
      },
      limit: {
        type: 'integer',
        minimum: 1,
        maximum: 20,
      },
    },
  },
  LexicalSelection: {
    type: 'object',
    additionalProperties: false,
    required: ['kind', 'targets'],
    properties: {
      kind: {
        const: 'fixed_representations',
      },
      targets: {
        type: 'array',
        minItems: 1,
        maxItems: 32,
        items: {
          $ref: '#/$defs/Target',
        },
      },
    },
  },
  LexicalTypedMember: {
    type: 'object',
    additionalProperties: false,
    required: ['node', 'payload_sha256', 'text_assembly'],
    properties: {
      node: {
        $ref: '#/$defs/BoundNode',
      },
      payload_sha256: {
        $ref: '#/$defs/Hash64',
      },
      text_assembly: {
        enum: ['existing_reading_range', 'row_major_uncovered_cells_tab_lf_v1'],
      },
    },
  },
  LexicalcreateIndexResponse: {
    oneOf: [
      {
        type: 'object',
        additionalProperties: false,
        required: ['operation', 'contract_version', 'request_id', 'data'],
        properties: {
          operation: {
            const: 'createIndex',
          },
          contract_version: {
            $ref: '#/$defs/Version',
          },
          request_id: {
            $ref: '#/$defs/ID32',
          },
          data: {
            $ref: '#/$defs/LexicalJob',
          },
        },
      },
      {
        allOf: [
          {
            $ref: '#/$defs/LexicalError',
          },
          {
            properties: {
              operation: {
                const: 'createIndex',
              },
            },
          },
        ],
      },
    ],
  },
  LexicalgetIndexJobResponse: {
    oneOf: [
      {
        type: 'object',
        additionalProperties: false,
        required: ['operation', 'contract_version', 'request_id', 'data'],
        properties: {
          operation: {
            const: 'getIndexJob',
          },
          contract_version: {
            $ref: '#/$defs/Version',
          },
          request_id: {
            $ref: '#/$defs/ID32',
          },
          data: {
            $ref: '#/$defs/LexicalJob',
          },
        },
      },
      {
        allOf: [
          {
            $ref: '#/$defs/LexicalError',
          },
          {
            properties: {
              operation: {
                const: 'getIndexJob',
              },
            },
          },
        ],
      },
    ],
  },
  LexicalinspectIndexResponse: {
    oneOf: [
      {
        type: 'object',
        additionalProperties: false,
        required: ['operation', 'contract_version', 'request_id', 'data'],
        properties: {
          operation: {
            const: 'inspectIndex',
          },
          contract_version: {
            $ref: '#/$defs/Version',
          },
          request_id: {
            $ref: '#/$defs/ID32',
          },
          data: {
            $ref: '#/$defs/LexicalInspectData',
          },
        },
      },
      {
        allOf: [
          {
            $ref: '#/$defs/LexicalError',
          },
          {
            properties: {
              operation: {
                const: 'inspectIndex',
              },
            },
          },
        ],
      },
    ],
  },
  LexicalreadResponse: {
    oneOf: [
      {
        type: 'object',
        additionalProperties: false,
        required: ['operation', 'contract_version', 'request_id', 'data'],
        properties: {
          operation: {
            const: 'read',
          },
          contract_version: {
            $ref: '#/$defs/Version',
          },
          request_id: {
            $ref: '#/$defs/ID32',
          },
          data: {
            $ref: '#/$defs/LexicalReadData',
          },
        },
      },
      {
        allOf: [
          {
            $ref: '#/$defs/LexicalError',
          },
          {
            properties: {
              operation: {
                const: 'read',
              },
            },
          },
        ],
      },
    ],
  },
  LexicalsearchResponse: {
    oneOf: [
      {
        type: 'object',
        additionalProperties: false,
        required: ['operation', 'contract_version', 'request_id', 'data'],
        properties: {
          operation: {
            const: 'search',
          },
          contract_version: {
            $ref: '#/$defs/Version',
          },
          request_id: {
            $ref: '#/$defs/ID32',
          },
          data: {
            $ref: '#/$defs/LexicalSearchData',
          },
        },
      },
      {
        allOf: [
          {
            $ref: '#/$defs/LexicalError',
          },
          {
            properties: {
              operation: {
                const: 'search',
              },
            },
          },
        ],
      },
    ],
  },
};
const Contract = {
  $schema: 'https://json-schema.org/draft/2020-12/schema',
  $id: 'urn:lfen:knowledge-content-lexical:0.2:candidate:03',
  $defs: { ...ContentContract.$defs, ...LexicalDefinitions },
};

export const LocalLexicalVersion = '0.2.0-candidate' as const;
export const LocalLexicalPolicyVersion = '0.1.0-candidate' as const;
export const LocalLexicalId = /^[a-f0-9]{32}$/;
export const LocalLexicalHash = /^[a-f0-9]{64}$/;
export const LocalLexicalOperations = {
  getLocalPolicy: ['GetLocalPolicy', 'get', '/local-lexical-policy'],
  changeLocalPolicy: [
    'ChangeLocalPolicy',
    'post',
    '/local-lexical-policy/changes',
  ],
  inspectIndex: ['LexicalinspectIndex', 'post', '/content/lexical/inspect'],
  createIndex: ['LexicalcreateIndex', 'post', '/content/lexical/indexes'],
  getIndexJob: ['LexicalgetIndexJob', 'post', '/content/lexical/jobs/get'],
  search: ['Lexicalsearch', 'post', '/content/lexical/search'],
  read: ['Lexicalread', 'post', '/content/lexical/evidence/read'],
} as const;
const PolicyOperations = new Set<LocalLexicalOperation>([
  'getLocalPolicy',
  'changeLocalPolicy',
]);
export const localLexicalBytes = contentBytes;
export const localLexicalCanonical = contentCanonical;
function requireValid(value: unknown): asserts value {
  if (!value) throw Error('local_lexical_invalid_data');
}
export function localLexicalScalar(value: unknown): boolean {
  if (typeof value === 'string')
    return (
      new TextDecoder('utf-8', { fatal: true }).decode(
        new TextEncoder().encode(value),
      ) === value
    );
  if (Array.isArray(value)) return value.every(localLexicalScalar);
  if (value && typeof value === 'object')
    return Object.entries(value).every(
      ([key, item]) => localLexicalScalar(key) && localLexicalScalar(item),
    );
  return typeof value !== 'number' || Number.isFinite(value);
}
const PolicyValidator = new Ajv({ strict: false, allErrors: false });
addFormats(PolicyValidator);
PolicyValidator.addSchema(PolicyContract);
const Validator = new Ajv2020({
  strict: false,
  allErrors: false,
  validateFormats: true,
  strictNumbers: true,
});
addFormats(Validator);
Validator.addKeyword({
  keyword: 'x-max-utf8-bytes',
  type: 'string',
  schemaType: 'number',
  validate: (limit: number, value: string) => contentBytes(value) <= limit,
});
Validator.addKeyword({
  keyword: 'x-unicode-scalar',
  type: 'string',
  schemaType: 'boolean',
  validate: (enabled: boolean, value: string) =>
    !enabled || localLexicalScalar(value),
});
Validator.addKeyword({
  keyword: 'x-xml-local-name',
  type: 'string',
  schemaType: 'boolean',
  validate: (enabled: boolean, value: string) =>
    !enabled || /^[_\p{L}][_\p{L}\p{N}\p{M}.\-\u00b7]*$/u.test(value),
});
Validator.addSchema(Contract);
const Schemas = new Map<string, ValidateFunction>();
export function validLocalLexicalSchema(name: string, value: unknown): boolean {
  const policy = [
    'GetLocalPolicyRequest',
    'GetLocalPolicyResponse',
    'ChangeLocalPolicyRequest',
    'ChangeLocalPolicyResponse',
    'ErrorResponse',
  ].includes(name);
  const key = (policy ? 'policy:' : 'content:') + name;
  if (
    !Object.hasOwn(policy ? PolicyContract.definitions : Contract.$defs, name)
  )
    return false;
  let check = Schemas.get(key);
  if (!check) {
    check = (policy ? PolicyValidator : Validator).compile({
      $ref:
        (policy
          ? PolicyContract.$id + '#/definitions/'
          : Contract.$id + '#/$defs/') + name,
    });
    Schemas.set(key, check);
  }
  return !!check(value) && localLexicalScalar(value);
}
function orderedTarget(target: ContentTarget): ContentTarget {
  const s = target.source_ref,
    r = target.representation_ref;
  return {
    source_ref: {
      domain_id: s.domain_id,
      source_id: s.source_id,
      source_revision_id: s.source_revision_id,
      source_sha256: s.source_sha256,
    },
    representation_ref: {
      kind: r.kind,
      id: r.id,
      producer_sha256: r.producer_sha256,
    },
  };
}
function orderedSelection(
  selection: LocalLexicalSelection,
): LocalLexicalSelection {
  return {
    kind: selection.kind,
    targets: [...selection.targets]
      .sort((a, b) =>
        a.source_ref.source_id < b.source_ref.source_id
          ? -1
          : a.source_ref.source_id > b.source_ref.source_id
            ? 1
            : 0,
      )
      .map(orderedTarget),
  };
}
export const localLexicalSelectionKey = (selection: LocalLexicalSelection) =>
  contentCanonical(orderedSelection(selection));
const goJSON = (value: unknown) =>
  JSON.stringify(value).replace(
    /[<>&\u2028\u2029]/g,
    (value) => '\\u' + value.charCodeAt(0).toString(16).padStart(4, '0'),
  );
async function selectionDigest(selection: LocalLexicalSelection) {
  return contentDigest(goJSON(orderedSelection(selection)));
}
function identityBytes(value: unknown): Uint8Array {
  const join = (parts: Uint8Array[]) => {
    const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let at = 0;
    for (const part of parts) {
      out.set(part, at);
      at += part.length;
    }
    return out;
  };
  const length = (value: number) => {
    requireValid(Number.isSafeInteger(value) && value >= 0);
    const out = new Uint8Array(8);
    new DataView(out.buffer).setBigUint64(0, BigInt(value));
    return out;
  };
  if (value === null) return new Uint8Array([0x4e]);
  if (typeof value === 'string') {
    requireValid(localLexicalScalar(value));
    const bytes = new TextEncoder().encode(value);
    return join([new Uint8Array([0x53]), length(bytes.length), bytes]);
  }
  if (typeof value === 'number')
    return join([new Uint8Array([0x55]), length(value)]);
  requireValid(Array.isArray(value));
  return join([
    new Uint8Array([0x41]),
    length(value.length),
    ...value.map(identityBytes),
  ]);
}
const identityDigest = (...values: unknown[]) =>
  webSha256(new Uint8Array(identityBytes(values)).buffer);
export function checkLocalLexicalSelection(
  selection: LocalLexicalSelection,
  domain: string,
) {
  requireValid(validLocalLexicalSchema('LexicalSelection', selection));
  const sources = new Set<string>();
  for (const target of selection.targets) {
    requireValid(
      contentFixedTarget(domain, target) &&
        !sources.has(target.source_ref.source_id),
    );
    sources.add(target.source_ref.source_id);
  }
}
async function checkIndex(
  index: LocalLexicalIndexRef,
  scope: LocalLexicalScope,
) {
  requireValid(
    index.index_id ===
      (await identityDigest(
        'knowledge-content-index-publication/0.2.0',
        scope.owner,
        scope.domain,
        index.manifest_sha256,
      )),
  );
}
export function checkLocalLexicalEvidence(
  evidence: LocalLexicalEvidenceRef,
  domain: string,
  selection?: LocalLexicalSelection,
) {
  requireValid(validLocalLexicalSchema('LexicalEvidence', evidence));
  const { address, range, locator } = evidence,
    target = address.target,
    kind = target.representation_ref.kind,
    source = kind === 'source_revision' || kind === 'structure_view';
  requireValid(
    contentFixedTarget(domain, target) &&
      contentBoundNode(target, address.node_ref) &&
      range.end_byte > range.start_byte &&
      range.end_byte - range.start_byte <= 8192,
  );
  if (selection)
    requireValid(
      selection.targets.some((item) => contentSameTarget(item, target)),
    );
  if (source) {
    const original = locator.source_utf8;
    requireValid(
      evidence.coordinate_space === 'source_utf8' &&
        evidence.reading_text_sha256 === target.source_ref.source_sha256 &&
        range.end_byte <= 2097152 &&
        locator.kind === 'source_utf8' &&
        !!original &&
        original.start_byte <= range.start_byte &&
        original.end_byte >= range.end_byte &&
        evidence.original_position === null &&
        evidence.coordinate_precision === 'source_bytes',
    );
    if (kind === 'structure_view')
      requireValid(
        original.start_byte === range.start_byte &&
          original.end_byte === range.end_byte,
      );
  } else
    requireValid(
      evidence.coordinate_space === 'derived_utf8' &&
        range.end_byte <= 4194304 &&
        locator.kind === 'document' &&
        locator.document?.scheme === 'derived_passage_v1' &&
        evidence.coordinate_precision === 'derived_range',
    );
  requireValid(
    evidence.assembly ===
      (kind === 'source_revision'
        ? 'c1_root_subrange_exact_bytes'
        : 'existing_passage_full_range'),
  );
}
async function checkUnit(evidence: LocalLexicalEvidenceRef, text: string) {
  requireValid(
    contentBytes(text) ===
      evidence.range.end_byte - evidence.range.start_byte &&
      (await contentDigest(text)) === evidence.range.sha256,
  );
}
function checkCoverage(
  coverage: LocalLexicalCoverage[],
  selection: LocalLexicalSelection,
  complete = true,
) {
  if (complete) requireValid(coverage.length === selection.targets.length);
  const seen = new Set<string>();
  for (const row of coverage) {
    const id = row.target.source_ref.source_id;
    requireValid(
      !seen.has(id) &&
        selection.targets.some((target) =>
          contentSameTarget(target, row.target),
        ),
    );
    seen.add(id);
    const source = ['source_revision', 'structure_view'].includes(
      row.target.representation_ref.kind,
    );
    requireValid(
      row.coordinate_space === (source ? 'source_utf8' : 'derived_utf8') &&
        row.original_coverage ===
          (source ? 'source_utf8_exact' : 'not_asserted') &&
        row.expected_reading_bytes === row.covered_reading_bytes &&
        row.reading_text_complete &&
        row.text_index_state ===
          (row.expected_reading_bytes ? 'available' : 'empty'),
    );
    if (source)
      requireValid(
        row.reading_text_sha256 === row.target.source_ref.source_sha256 &&
          row.expected_reading_bytes <= 2097152,
      );
  }
}
async function checkReadiness(
  readiness: LocalLexicalReadiness,
  scope: LocalLexicalScope,
  selection: LocalLexicalSelection,
) {
  checkLocalLexicalSelection(readiness.selection, scope.domain);
  requireValid(
    localLexicalSelectionKey(readiness.selection) ===
      localLexicalSelectionKey(selection) &&
      readiness.selection_sha256 === (await selectionDigest(selection)),
  );
  const published = ['ready', 'stale'].includes(readiness.kind);
  requireValid(
    readiness.publication === (published ? 'published' : 'none') &&
      !!readiness.index_ref === published,
  );
  if (readiness.index_ref) await checkIndex(readiness.index_ref, scope);
  checkCoverage(
    readiness.coverage,
    selection,
    published || ['unindexed', 'empty'].includes(readiness.kind),
  );
}
async function checkProfile(profile: LocalLexicalProfile) {
  requireValid(
    profile.profile_sha256 ===
      (await identityDigest(
        'knowledge-content-index-plan/0.2.0',
        profile.strategies,
        'typed_content_ranges/0.2.0',
        8192,
        'exact_bytes_no_trim_no_merge_no_parent',
        profile.language,
        profile.tokenizer_producer_sha256,
        'full_text',
      )),
  );
}
function createProjection(body: LocalLexicalCreateBody) {
  return {
    contract_version: body.contract_version,
    idempotency_key: body.idempotency_key,
    expected_local_policy_version: body.expected_local_policy_version,
    expected_content_policy_version: body.expected_content_policy_version,
    expected_foundation_policy_version: body.expected_foundation_policy_version,
    expected_profile_sha256: body.expected_profile_sha256,
    selection: orderedSelection(body.selection),
  };
}
async function checkRow(evidence: LocalLexicalEvidenceRef, profile: string) {
  const binding = await identityDigest(
    'knowledge-index-binding/0.2.0',
    await contentDigest(goJSON(orderedTarget(evidence.address.target))),
    profile,
    evidence.coordinate_space,
    evidence.reading_text_sha256,
  );
  const node = evidence.address.node_ref;
  const ref = {
    projection_version: LocalLexicalVersion,
    address: {
      target: orderedTarget(evidence.address.target),
      node_ref: { kind: node.kind, id: node.id, item_index: node.item_index },
    },
    coordinate_space: evidence.coordinate_space,
    reading_text_sha256: evidence.reading_text_sha256,
    range: {
      start_byte: evidence.range.start_byte,
      end_byte: evidence.range.end_byte,
      sha256: evidence.range.sha256,
    },
    locator: evidence.locator,
    original_position: evidence.original_position,
    coordinate_precision: evidence.coordinate_precision,
    assembly: evidence.assembly,
  };
  requireValid(
    evidence.index_row_id ===
      (await identityDigest(
        'knowledge-index-row/0.2.0',
        binding,
        await contentDigest(goJSON(ref)),
        evidence.range.end_byte - evidence.range.start_byte,
        evidence.range.sha256,
      )),
  );
}
async function checkContext(result: LocalLexicalReadData) {
  const evidence = result.evidence_ref,
    context = result.context,
    target = evidence.address.target,
    kind = target.representation_ref.kind;
  requireValid(
    contentWireBytes(context) === context.metadata_byte_length &&
      context.metadata_byte_length <= 65536,
  );
  const membership = {
    source_revision: 'stored_c1_root',
    structure_view: 'validated_r1_passage',
    document_representation: 'validated_document_manifest',
    parsed_representation: 'validated_parsed_manifest',
    parser_result: '',
  };
  requireValid(
    context.membership === membership[kind] &&
      context.typed_members_complete ===
        (context.omission_reasons.length === 0 &&
          context.omitted_member_count === 0),
  );
  if (context.node) {
    requireValid(
      context.node_omission_reason === null &&
        contentNodeKey(context.node.ref) ===
          contentNodeKey(evidence.address.node_ref) &&
        contentCanonical(context.node.locator) ===
          contentCanonical(evidence.locator) &&
        contentCanonical(context.node.original_position) ===
          contentCanonical(evidence.original_position) &&
        context.node.coordinate_precision === evidence.coordinate_precision,
    );
    await checkContentNode(target, context.node);
    const range = context.node.text.range;
    requireValid(
      range &&
        range.start_byte <= evidence.range.start_byte &&
        range.end_byte >= evidence.range.end_byte,
    );
    if (kind !== 'source_revision')
      requireValid(
        range.start_byte === evidence.range.start_byte &&
          range.end_byte === evidence.range.end_byte,
      );
    if (kind === 'structure_view') {
      if (range.sha256 !== null)
        requireValid(range.sha256 === evidence.range.sha256);
    } else if (kind !== 'source_revision')
      requireValid(range.sha256 === evidence.range.sha256);
  } else requireValid(context.node_omission_reason === 'metadata_budget');
  const seen = new Set<string>(),
    bytes = new TextEncoder().encode(result.text);
  let previous: [number, number, string] | null = null;
  for (const member of context.typed_members) {
    const node = member.node,
      range = node.text.range,
      id = node.ref.id;
    requireValid(
      id &&
        range &&
        range.sha256 &&
        !seen.has(id) &&
        range.start_byte >= evidence.range.start_byte &&
        range.end_byte <= evidence.range.end_byte &&
        !['document', 'section', 'page', 'slide', 'passage'].includes(
          node.kind,
        ),
    );
    seen.add(id);
    const order: [number, number, string] = [
      range.start_byte,
      range.end_byte,
      id,
    ];
    if (previous)
      requireValid(
        order[0] > previous[0] ||
          (order[0] === previous[0] &&
            (order[1] > previous[1] ||
              (order[1] === previous[1] && order[2] > previous[2]))),
      );
    previous = order;
    await checkContentNode(target, node);
    const text = new TextDecoder('utf-8', { fatal: true }).decode(
      bytes.subarray(
        range.start_byte - evidence.range.start_byte,
        range.end_byte - evidence.range.start_byte,
      ),
    );
    requireValid((await contentDigest(text)) === range.sha256);
    if (node.attributes.parser?.table)
      requireValid(
        member.text_assembly === 'row_major_uncovered_cells_tab_lf_v1' &&
          contentTableText(node.attributes.parser.table) === text,
      );
    else requireValid(member.text_assembly === 'existing_reading_range');
    if (range.start_byte === range.end_byte)
      requireValid(node.text.status === 'empty' && text === '');
  }
  if (kind === 'source_revision' || kind === 'structure_view')
    requireValid(
      context.manifest_sha256 === null &&
        context.typed_members.length === 0 &&
        context.omission_reasons.includes(
          'typed_members_not_defined_for_source_family',
        ),
    );
  else requireValid(context.manifest_sha256 !== null);
}
export function checkLocalLexicalRequest<O extends LocalLexicalOperation>(
  operation: O,
  scope: LocalLexicalScope,
  body: LocalLexicalBodies[O],
) {
  requireValid(
    LocalLexicalId.test(scope.owner) && LocalLexicalId.test(scope.domain),
  );
  if (PolicyOperations.has(operation))
    requireValid(
      validLocalLexicalSchema(
        LocalLexicalOperations[operation][0] + 'Request',
        {
          operation,
          path: { domain_id: scope.domain },
          ...(body === undefined ? {} : { body }),
        },
      ),
    );
  else {
    const name = {
      inspectIndex: 'LexicalInspectRequest',
      createIndex: 'LexicalCreateRequest',
      getIndexJob: 'LexicalJobRequest',
      search: 'LexicalSearchRequest',
      read: 'LexicalReadRequest',
    }[
      operation as Exclude<
        LocalLexicalOperation,
        'getLocalPolicy' | 'changeLocalPolicy'
      >
    ];
    requireValid(
      validLocalLexicalSchema(name, body) &&
        contentBytes(JSON.stringify(body)) <= 32768,
    );
  }
  if (body && 'selection' in body)
    checkLocalLexicalSelection(body.selection, scope.domain);
  if (body && 'evidence_ref' in body)
    checkLocalLexicalEvidence(body.evidence_ref, scope.domain);
  if (body && 'query' in body) requireValid(contentBytes(body.query) <= 4096);
}
export async function checkLocalLexicalResponse<
  O extends LocalLexicalOperation,
>(
  operation: O,
  raw: unknown,
  scope: LocalLexicalScope,
  body: LocalLexicalBodies[O],
): Promise<LocalLexicalData[O]> {
  requireValid(
    validLocalLexicalSchema(
      LocalLexicalOperations[operation][0] + 'Response',
      raw,
    ),
  );
  const envelope = raw as LocalLexicalEnvelope<O>;
  requireValid(envelope.operation === operation && envelope.data !== null);
  const data = envelope.data;
  if (operation === 'getLocalPolicy')
    requireValid((data as LocalLexicalPolicy).domain_id === scope.domain);
  if (operation === 'changeLocalPolicy') {
    const receipt = data as LocalLexicalPolicyReceipt,
      request = body as LocalLexicalPolicyBody;
    requireValid(
      receipt.policy.domain_id === scope.domain &&
        receipt.idempotency_key === request.idempotency_key &&
        receipt.policy.enabled === request.enabled &&
        (receipt.outcome === 'changed'
          ? receipt.policy.local_policy_version >
            request.expected_local_policy_version
          : receipt.policy.local_policy_version ===
            request.expected_local_policy_version),
    );
  }
  if (operation === 'inspectIndex') {
    const result = data as LocalLexicalReadinessData,
      p = result.effective_policy;
    requireValid(
      p.domain_id === scope.domain &&
        p.enabled === (p.local_lexical_enabled && p.content_lexical_enabled),
    );
    await checkProfile(result.profile);
    await checkReadiness(
      result.readiness,
      scope,
      (body as LocalLexicalInspectBody).selection,
    );
  }
  if (operation === 'createIndex' || operation === 'getIndexJob') {
    const result = data as LocalLexicalJob;
    await checkReadiness(result.readiness, scope, result.selection);
    requireValid(
      contentCanonical(result.index_ref) ===
        contentCanonical(result.readiness.index_ref) &&
        !!result.index_ref === (result.state === 'published') &&
        contentTimestamp(result.created_at) &&
        contentTimestamp(result.updated_at) &&
        !dayjs(result.updated_at).isBefore(dayjs(result.created_at)),
    );
    const request: LocalLexicalCreateBody = {
      contract_version: LocalLexicalVersion,
      idempotency_key: result.idempotency_key,
      expected_local_policy_version: result.captured_local_policy_version,
      expected_content_policy_version: result.captured_content_policy_version,
      expected_foundation_policy_version:
        result.captured_foundation_policy_version,
      expected_profile_sha256: result.captured_profile_sha256,
      selection: result.selection,
    };
    requireValid(
      result.request_sha256 ===
        (await contentDigest(goJSON(createProjection(request)))),
    );
    if (operation === 'getIndexJob')
      requireValid(result.job_id === (body as LocalLexicalJobBody).job_id);
    else
      requireValid(
        contentCanonical(createProjection(request)) ===
          contentCanonical(createProjection(body as LocalLexicalCreateBody)),
      );
  }
  if (operation === 'search') {
    const result = data as LocalLexicalSearchData,
      request = body as LocalLexicalSearchBody,
      binding = result.binding;
    requireValid(contentTimestamp(binding.published_at));
    checkLocalLexicalSelection(binding.selection, scope.domain);
    requireValid(
      localLexicalSelectionKey(binding.selection) ===
        localLexicalSelectionKey(request.selection) &&
        binding.selection_sha256 ===
          (await selectionDigest(binding.selection)) &&
        contentCanonical(binding.index_ref) ===
          contentCanonical(request.index_ref) &&
        result.query === request.query &&
        result.requested_limit === request.limit &&
        result.returned_count === result.hits.length &&
        result.returned_count <= request.limit,
    );
    await checkIndex(binding.index_ref, scope);
    checkCoverage(result.coverage, request.selection);
    const seen = new Set<string>();
    for (const hit of result.hits) {
      checkLocalLexicalEvidence(
        hit.evidence_ref,
        scope.domain,
        request.selection,
      );
      requireValid(
        !seen.has(hit.evidence_ref.index_row_id) &&
          contentCanonical(hit.evidence_ref.index_ref) ===
            contentCanonical(request.index_ref),
      );
      seen.add(hit.evidence_ref.index_row_id);
      await checkUnit(hit.evidence_ref, hit.text);
      await checkRow(hit.evidence_ref, binding.profile_sha256);
    }
  }
  if (operation === 'read') {
    const result = data as LocalLexicalReadData;
    checkLocalLexicalEvidence(result.evidence_ref, scope.domain);
    await checkIndex(result.evidence_ref.index_ref, scope);
    requireValid(
      contentCanonical(result.evidence_ref) ===
        contentCanonical((body as LocalLexicalReadBody).evidence_ref) &&
        result.byte_length === contentBytes(result.text),
    );
    await checkUnit(result.evidence_ref, result.text);
    await checkContext(result);
  }
  return data;
}

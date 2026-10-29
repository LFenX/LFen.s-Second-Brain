import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { webcrypto } from 'node:crypto';
import { TextEncoder, TextDecoder } from 'node:util';
import request from '@/utils/next-request';
import service, {
  LocalLexicalRequestError,
} from '@/services/knowledge-local-lexical-service';
import contentService, {
  ContentRequestError,
} from '@/services/knowledge-consumption-service';
import {
  getKnowledgeSessionScope,
  useKnowledgeSources,
  useKnowledgeSource,
} from '@/hooks/use-knowledge-foundation';
import {
  useStructureHead,
  useStructureView,
} from '@/hooks/use-knowledge-structure';
import {
  useDocumentHead,
  useDocumentRepresentation,
} from '@/hooks/use-knowledge-document';
import { useLocalLexicalActions } from '@/hooks/use-knowledge-local-lexical';
import {
  checkLocalLexicalRequest,
  localLexicalSelectionKey,
} from '@/utils/knowledge-local-lexical';
import {
  parseContentJSON,
  contentFixedLink,
} from '@/utils/knowledge-consumption';
import LocalLexicalBrowser, { LocalLexicalEvidence } from '../local-lexical';
type LocalLexicalSelection =
  import('@/interfaces/database/knowledge-local-lexical').LocalLexicalSelection;
type LocalLexicalReadinessData =
  import('@/interfaces/database/knowledge-local-lexical').LocalLexicalReadinessData;
type LocalLexicalJob =
  import('@/interfaces/database/knowledge-local-lexical').LocalLexicalJob;
type LocalLexicalSearchData =
  import('@/interfaces/database/knowledge-local-lexical').LocalLexicalSearchData;
type LocalLexicalReadData =
  import('@/interfaces/database/knowledge-local-lexical').LocalLexicalReadData;
type LocalLexicalPolicy =
  import('@/interfaces/database/knowledge-local-lexical').LocalLexicalPolicy;
type LocalLexicalCreateBody =
  import('@/interfaces/request/knowledge-local-lexical').LocalLexicalCreateBody;
type LocalLexicalSearchBody =
  import('@/interfaces/request/knowledge-local-lexical').LocalLexicalSearchBody;
type ContentPolicy =
  import('@/interfaces/database/knowledge-consumption').ContentPolicy;
type ReactNode = import('react').ReactNode;

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, args?: Record<string, unknown>) =>
      key +
      (args && ('covered' in args || 'count' in args)
        ? ' ' + JSON.stringify(args)
        : ''),
  }),
}));
jest.mock('@/hooks/use-knowledge-foundation', () => ({
  ...jest.requireActual('@/hooks/use-knowledge-foundation'),
  getKnowledgeSessionScope: jest.fn(),
  useKnowledgeSources: jest.fn(),
  useKnowledgeSource: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-structure', () => ({
  useStructureHead: jest.fn(),
  useStructureView: jest.fn(),
}));
jest.mock('@/hooks/use-knowledge-document', () => ({
  useDocumentHead: jest.fn(),
  useDocumentRepresentation: jest.fn(),
}));
jest.mock('../../knowledge-import/batch-manifest', () => ({
  ProjectPicker: ({ onChoose }: { onChoose: (id: string) => void }) => {
    const choose = () => onChoose('7'.repeat(32));
    return <button onClick={choose}>fixture-project</button>;
  },
}));
jest.mock('../document-prepare', () => ({
  DocumentOriginalList: ({
    onSelect,
  }: {
    onSelect: (value: unknown) => void;
  }) => {
    const choose = () =>
      onSelect(
        (globalThis as unknown as { lexicalOriginal: unknown }).lexicalOriginal,
      );
    return <button onClick={choose}>fixture-original</button>;
  },
}));

// Fixed synthetic fixtures copied from registered interop01; component mocks are not native integration.
const Fixtures: {
  inspect: LocalLexicalReadinessData;
  ready: LocalLexicalReadinessData;
  created: LocalLexicalJob;
  published: LocalLexicalJob;
  search: LocalLexicalSearchData;
  reads: LocalLexicalReadData[];
  emptyDocx: LocalLexicalReadData;
  createBody: LocalLexicalCreateBody;
  searchBody: LocalLexicalSearchBody;
} = {
  inspect: {
    effective_policy: {
      domain_id: '11111111111111111111111111111111',
      local_policy_version: 1,
      content_policy_version: 3,
      foundation_policy_version: 1,
      local_lexical_enabled: true,
      content_lexical_enabled: true,
      enabled: true,
      mode: 'owner_only_local_lexical',
    },
    profile: {
      profile_sha256:
        '9fbd55ea73d9ce90cdde82c00c912afb50303433f8ffe5356de3edca626eb063',
      tokenizer_producer_sha256:
        '8a1f56a810dbba1461126fcdb48b5a8a60f3787bfb41ba85e31f58b2047e1a1f',
      language: 'English',
      strategies: [
        'c1_exact_utf8_ranges',
        'r1_existing_passage',
        'r2_existing_passage',
        'parsed_existing_passage',
      ],
      engine: 'elasticsearch',
      execution_profile: 'local_lexical_no_models/0.2.0',
      max_unit_bytes: 8192,
      parent_expansion: 'disabled',
      models_enabled: false,
      remote_fetches: false,
    },
    readiness: {
      kind: 'unindexed',
      publication: 'none',
      selection: {
        kind: 'fixed_representations',
        targets: [
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
        ],
      },
      selection_sha256:
        '869246967462657d703f219065b36ba5a2eea1a5bc1f38628bd421aa8eef922f',
      snapshot_currentness: 'current',
      index_ref: null,
      coverage: [
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          expected_reading_bytes: 19200,
          covered_reading_bytes: 19200,
          indexed_unit_count: 3,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'source_utf8_exact',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          expected_reading_bytes: 52,
          covered_reading_bytes: 52,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'source_utf8_exact',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
          expected_reading_bytes: 21,
          covered_reading_bytes: 21,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'not_asserted',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
          expected_reading_bytes: 24,
          covered_reading_bytes: 24,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'not_asserted',
          text_index_state: 'available',
        },
      ],
      diagnostic_codes: ['none'],
    },
  },
  ready: {
    effective_policy: {
      domain_id: '11111111111111111111111111111111',
      local_policy_version: 1,
      content_policy_version: 3,
      foundation_policy_version: 1,
      local_lexical_enabled: true,
      content_lexical_enabled: true,
      enabled: true,
      mode: 'owner_only_local_lexical',
    },
    profile: {
      profile_sha256:
        '9fbd55ea73d9ce90cdde82c00c912afb50303433f8ffe5356de3edca626eb063',
      tokenizer_producer_sha256:
        '8a1f56a810dbba1461126fcdb48b5a8a60f3787bfb41ba85e31f58b2047e1a1f',
      language: 'English',
      strategies: [
        'c1_exact_utf8_ranges',
        'r1_existing_passage',
        'r2_existing_passage',
        'parsed_existing_passage',
      ],
      engine: 'elasticsearch',
      execution_profile: 'local_lexical_no_models/0.2.0',
      max_unit_bytes: 8192,
      parent_expansion: 'disabled',
      models_enabled: false,
      remote_fetches: false,
    },
    readiness: {
      kind: 'ready',
      publication: 'published',
      selection: {
        kind: 'fixed_representations',
        targets: [
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
        ],
      },
      selection_sha256:
        '869246967462657d703f219065b36ba5a2eea1a5bc1f38628bd421aa8eef922f',
      snapshot_currentness: 'current',
      index_ref: {
        index_id:
          'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
        manifest_sha256:
          '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
      },
      coverage: [
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          expected_reading_bytes: 19200,
          covered_reading_bytes: 19200,
          indexed_unit_count: 3,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'source_utf8_exact',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          expected_reading_bytes: 52,
          covered_reading_bytes: 52,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'source_utf8_exact',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
          expected_reading_bytes: 21,
          covered_reading_bytes: 21,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'not_asserted',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
          expected_reading_bytes: 24,
          covered_reading_bytes: 24,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'not_asserted',
          text_index_state: 'available',
        },
      ],
      diagnostic_codes: [],
    },
  },
  created: {
    job_id: 'd3733491dfc948f9b604750554b9605a',
    idempotency_key: 'one_registered_mixed_job',
    request_sha256:
      'f20e4099de32c247260af0f7b70b9779a8a578cb8a470965f74444fa1e51920f',
    state: 'queued',
    selection: {
      kind: 'fixed_representations',
      targets: [
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '22222222222222222222222222222222',
            source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
            source_sha256:
              'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          },
          representation_ref: {
            kind: 'source_revision',
            id: '690bc04dba3e40f6bdc92143b4cd9f25',
            producer_sha256: null,
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '33333333333333333333333333333333',
            source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
            source_sha256:
              '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          },
          representation_ref: {
            kind: 'structure_view',
            id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
            producer_sha256:
              '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '55555555555555555555555555555555',
            source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
            source_sha256:
              '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
          },
          representation_ref: {
            kind: 'parsed_representation',
            id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
            producer_sha256:
              '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
            source_revision_id: '8a9578432e394df78b8fa017487ccf69',
            source_sha256:
              '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
          },
          representation_ref: {
            kind: 'document_representation',
            id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
            producer_sha256:
              'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
          },
        },
      ],
    },
    captured_local_policy_version: 1,
    captured_content_policy_version: 3,
    captured_foundation_policy_version: 1,
    captured_profile_sha256:
      '9fbd55ea73d9ce90cdde82c00c912afb50303433f8ffe5356de3edca626eb063',
    captured_tokenizer_producer_sha256:
      '8a1f56a810dbba1461126fcdb48b5a8a60f3787bfb41ba85e31f58b2047e1a1f',
    index_ref: null,
    readiness: {
      kind: 'building',
      publication: 'none',
      selection: {
        kind: 'fixed_representations',
        targets: [
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
        ],
      },
      selection_sha256:
        '869246967462657d703f219065b36ba5a2eea1a5bc1f38628bd421aa8eef922f',
      snapshot_currentness: 'current',
      index_ref: null,
      coverage: [],
      diagnostic_codes: ['building'],
    },
    created_at: '2026-10-29T05:52:13.262940296Z',
    updated_at: '2026-10-29T05:52:13.262940296Z',
  },
  published: {
    job_id: 'd3733491dfc948f9b604750554b9605a',
    idempotency_key: 'one_registered_mixed_job',
    request_sha256:
      'f20e4099de32c247260af0f7b70b9779a8a578cb8a470965f74444fa1e51920f',
    state: 'published',
    selection: {
      kind: 'fixed_representations',
      targets: [
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '22222222222222222222222222222222',
            source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
            source_sha256:
              'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          },
          representation_ref: {
            kind: 'source_revision',
            id: '690bc04dba3e40f6bdc92143b4cd9f25',
            producer_sha256: null,
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '33333333333333333333333333333333',
            source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
            source_sha256:
              '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          },
          representation_ref: {
            kind: 'structure_view',
            id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
            producer_sha256:
              '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '55555555555555555555555555555555',
            source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
            source_sha256:
              '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
          },
          representation_ref: {
            kind: 'parsed_representation',
            id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
            producer_sha256:
              '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
            source_revision_id: '8a9578432e394df78b8fa017487ccf69',
            source_sha256:
              '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
          },
          representation_ref: {
            kind: 'document_representation',
            id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
            producer_sha256:
              'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
          },
        },
      ],
    },
    captured_local_policy_version: 1,
    captured_content_policy_version: 3,
    captured_foundation_policy_version: 1,
    captured_profile_sha256:
      '9fbd55ea73d9ce90cdde82c00c912afb50303433f8ffe5356de3edca626eb063',
    captured_tokenizer_producer_sha256:
      '8a1f56a810dbba1461126fcdb48b5a8a60f3787bfb41ba85e31f58b2047e1a1f',
    index_ref: {
      index_id:
        'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
      manifest_sha256:
        '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
    },
    readiness: {
      kind: 'ready',
      publication: 'published',
      selection: {
        kind: 'fixed_representations',
        targets: [
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
        ],
      },
      selection_sha256:
        '869246967462657d703f219065b36ba5a2eea1a5bc1f38628bd421aa8eef922f',
      snapshot_currentness: 'current',
      index_ref: {
        index_id:
          'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
        manifest_sha256:
          '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
      },
      coverage: [
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          expected_reading_bytes: 19200,
          covered_reading_bytes: 19200,
          indexed_unit_count: 3,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'source_utf8_exact',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          expected_reading_bytes: 52,
          covered_reading_bytes: 52,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'source_utf8_exact',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
          expected_reading_bytes: 21,
          covered_reading_bytes: 21,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'not_asserted',
          text_index_state: 'available',
        },
        {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
          scope: 'selected_representation_reading_utf8',
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
          expected_reading_bytes: 24,
          covered_reading_bytes: 24,
          indexed_unit_count: 1,
          reading_text_complete: true,
          representation_state: 'ready',
          unsearchable_node_count: 0,
          original_coverage: 'not_asserted',
          text_index_state: 'available',
        },
      ],
      diagnostic_codes: [],
    },
    created_at: '2026-10-29T05:52:13.262940296Z',
    updated_at: '2026-10-29T05:52:14.428345002Z',
  },
  search: {
    binding: {
      index_ref: {
        index_id:
          'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
        manifest_sha256:
          '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
      },
      selection: {
        kind: 'fixed_representations',
        targets: [
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
        ],
      },
      selection_sha256:
        '869246967462657d703f219065b36ba5a2eea1a5bc1f38628bd421aa8eef922f',
      local_policy_version: 1,
      content_policy_version: 3,
      foundation_policy_version: 1,
      profile_sha256:
        '9fbd55ea73d9ce90cdde82c00c912afb50303433f8ffe5356de3edca626eb063',
      tokenizer_producer_sha256:
        '8a1f56a810dbba1461126fcdb48b5a8a60f3787bfb41ba85e31f58b2047e1a1f',
      engine: 'elasticsearch',
      execution_profile: 'local_lexical_no_models/0.2.0',
      parent_expansion: 'disabled',
      published_at: '2026-10-29T05:52:14.427693069Z',
    },
    coverage: [
      {
        target: {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '22222222222222222222222222222222',
            source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
            source_sha256:
              'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          },
          representation_ref: {
            kind: 'source_revision',
            id: '690bc04dba3e40f6bdc92143b4cd9f25',
            producer_sha256: null,
          },
        },
        scope: 'selected_representation_reading_utf8',
        coordinate_space: 'source_utf8',
        reading_text_sha256:
          'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
        expected_reading_bytes: 19200,
        covered_reading_bytes: 19200,
        indexed_unit_count: 3,
        reading_text_complete: true,
        representation_state: 'ready',
        unsearchable_node_count: 0,
        original_coverage: 'source_utf8_exact',
        text_index_state: 'available',
      },
      {
        target: {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '33333333333333333333333333333333',
            source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
            source_sha256:
              '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          },
          representation_ref: {
            kind: 'structure_view',
            id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
            producer_sha256:
              '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
          },
        },
        scope: 'selected_representation_reading_utf8',
        coordinate_space: 'source_utf8',
        reading_text_sha256:
          '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
        expected_reading_bytes: 52,
        covered_reading_bytes: 52,
        indexed_unit_count: 1,
        reading_text_complete: true,
        representation_state: 'ready',
        unsearchable_node_count: 0,
        original_coverage: 'source_utf8_exact',
        text_index_state: 'available',
      },
      {
        target: {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '55555555555555555555555555555555',
            source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
            source_sha256:
              '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
          },
          representation_ref: {
            kind: 'parsed_representation',
            id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
            producer_sha256:
              '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
          },
        },
        scope: 'selected_representation_reading_utf8',
        coordinate_space: 'derived_utf8',
        reading_text_sha256:
          '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
        expected_reading_bytes: 21,
        covered_reading_bytes: 21,
        indexed_unit_count: 1,
        reading_text_complete: true,
        representation_state: 'ready',
        unsearchable_node_count: 0,
        original_coverage: 'not_asserted',
        text_index_state: 'available',
      },
      {
        target: {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
            source_revision_id: '8a9578432e394df78b8fa017487ccf69',
            source_sha256:
              '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
          },
          representation_ref: {
            kind: 'document_representation',
            id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
            producer_sha256:
              'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
          },
        },
        scope: 'selected_representation_reading_utf8',
        coordinate_space: 'derived_utf8',
        reading_text_sha256:
          'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
        expected_reading_bytes: 24,
        covered_reading_bytes: 24,
        indexed_unit_count: 1,
        reading_text_complete: true,
        representation_state: 'ready',
        unsearchable_node_count: 0,
        original_coverage: 'not_asserted',
        text_index_state: 'available',
      },
    ],
    query: '合成',
    requested_limit: 20,
    returned_count: 6,
    hits: [
      {
        evidence_ref: {
          evidence_version: '0.2.0-candidate',
          index_ref: {
            index_id:
              'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
            manifest_sha256:
              '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
          },
          index_row_id:
            '9d1201050bade20db806049770c0e80dbd8655ef62087ea56733b29777693e75',
          address: {
            target: {
              source_ref: {
                domain_id: '11111111111111111111111111111111',
                source_id: '55555555555555555555555555555555',
                source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
                source_sha256:
                  '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
              },
              representation_ref: {
                kind: 'parsed_representation',
                id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
                producer_sha256:
                  '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
              },
            },
            node_ref: {
              kind: 'parsed_node',
              id: '416d6756f85d70f53128ec36d014fe19ff160dbe23754b3a17f99c5e7113aaa0',
              item_index: null,
            },
          },
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
          range: {
            start_byte: 0,
            end_byte: 21,
            sha256:
              '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
          },
          locator: {
            kind: 'document',
            source_utf8: null,
            document: {
              chunk_index: 0,
              parent_locator: {
                scheme: 'document_v1',
              },
              scheme: 'derived_passage_v1',
            },
            item_index: null,
          },
          original_position: {
            kind: 'document_only',
          },
          coordinate_precision: 'derived_range',
          assembly: 'existing_passage_full_range',
        },
        text: 'A\tB\n合成\t表格\n1\t2',
        score: 3.5,
        match_type: 'lexical',
        source_currentness: 'current',
      },
      {
        evidence_ref: {
          evidence_version: '0.2.0-candidate',
          index_ref: {
            index_id:
              'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
            manifest_sha256:
              '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
          },
          index_row_id:
            'a52a86fcade5887b398fdf1ee8800a0a7990881cc71dee1b22997bc162017b37',
          address: {
            target: {
              source_ref: {
                domain_id: '11111111111111111111111111111111',
                source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
                source_revision_id: '8a9578432e394df78b8fa017487ccf69',
                source_sha256:
                  '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
              },
              representation_ref: {
                kind: 'document_representation',
                id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
                producer_sha256:
                  'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
              },
            },
            node_ref: {
              kind: 'r2_node',
              id: '45f8352df08c77c7b79da10bfa13cae3a6fcc4c41e548ad91f3cafcc2172e574',
              item_index: null,
            },
          },
          coordinate_space: 'derived_utf8',
          reading_text_sha256:
            'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
          range: {
            start_byte: 0,
            end_byte: 24,
            sha256:
              'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
          },
          locator: {
            kind: 'document',
            source_utf8: null,
            document: {
              chunk_index: 0,
              parent_locator: {
                page_number: 1,
                scheme: 'pdf_page_v1',
              },
              scheme: 'derived_passage_v1',
            },
            item_index: null,
          },
          original_position: {
            kind: 'pdf_page',
            page_number: 1,
          },
          coordinate_precision: 'derived_range',
          assembly: 'existing_passage_full_range',
        },
        text: '合成 R2 派生文字\r\n',
        score: 3.5,
        match_type: 'lexical',
        source_currentness: 'current',
      },
      {
        evidence_ref: {
          evidence_version: '0.2.0-candidate',
          index_ref: {
            index_id:
              'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
            manifest_sha256:
              '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
          },
          index_row_id:
            'b4bd3f81bf4daa0f6797f2d9b5a81a91bfef20e6beb8985eab67072c2c33758e',
          address: {
            target: {
              source_ref: {
                domain_id: '11111111111111111111111111111111',
                source_id: '22222222222222222222222222222222',
                source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
                source_sha256:
                  'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
              },
              representation_ref: {
                kind: 'source_revision',
                id: '690bc04dba3e40f6bdc92143b4cd9f25',
                producer_sha256: null,
              },
            },
            node_ref: {
              kind: 'c1_node',
              id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
              item_index: null,
            },
          },
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          range: {
            start_byte: 8184,
            end_byte: 16368,
            sha256:
              'a6bba8b3e04727d97ed76808a3a55686f58b181016af2834e20f1752e5f05107',
          },
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 19200,
              start_line: 1,
              end_line: 800,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          assembly: 'c1_root_subrange_exact_bytes',
        },
        text: '合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n',
        score: 3.5,
        match_type: 'lexical',
        source_currentness: 'current',
      },
      {
        evidence_ref: {
          evidence_version: '0.2.0-candidate',
          index_ref: {
            index_id:
              'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
            manifest_sha256:
              '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
          },
          index_row_id:
            '08048290440d6a9167933938cf403a34d08a2765dc6328b25a5ec3ccb8fac64e',
          address: {
            target: {
              source_ref: {
                domain_id: '11111111111111111111111111111111',
                source_id: '33333333333333333333333333333333',
                source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
                source_sha256:
                  '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
              },
              representation_ref: {
                kind: 'structure_view',
                id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
                producer_sha256:
                  '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
              },
            },
            node_ref: {
              kind: 'r1_node',
              id: '338307af91e97ab633a7a387b36651492927d0a9b5a5df07f5c6ce6a253000b5',
              item_index: null,
            },
          },
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          range: {
            start_byte: 0,
            end_byte: 52,
            sha256:
              '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          },
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 52,
              start_line: 1,
              end_line: 3,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          assembly: 'existing_passage_full_range',
        },
        text: '# 合成 R1 标题\n\n合成固定 Markdown 段落。\n',
        score: 3.5,
        match_type: 'lexical',
        source_currentness: 'current',
      },
      {
        evidence_ref: {
          evidence_version: '0.2.0-candidate',
          index_ref: {
            index_id:
              'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
            manifest_sha256:
              '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
          },
          index_row_id:
            '137c79def7271d34650b2a59a682c7be84411f30207a299a4771e01dd2dd58f2',
          address: {
            target: {
              source_ref: {
                domain_id: '11111111111111111111111111111111',
                source_id: '22222222222222222222222222222222',
                source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
                source_sha256:
                  'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
              },
              representation_ref: {
                kind: 'source_revision',
                id: '690bc04dba3e40f6bdc92143b4cd9f25',
                producer_sha256: null,
              },
            },
            node_ref: {
              kind: 'c1_node',
              id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
              item_index: null,
            },
          },
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          range: {
            start_byte: 16368,
            end_byte: 19200,
            sha256:
              '3b73281e7e5d5decc1bf90e5abe350322cf18290e469ea224ffbe49b5e064890',
          },
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 19200,
              start_line: 1,
              end_line: 800,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          assembly: 'c1_root_subrange_exact_bytes',
        },
        text: '合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n',
        score: 3.5,
        match_type: 'lexical',
        source_currentness: 'current',
      },
      {
        evidence_ref: {
          evidence_version: '0.2.0-candidate',
          index_ref: {
            index_id:
              'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
            manifest_sha256:
              '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
          },
          index_row_id:
            '7afee5f7f4a850447127546e0748379e25e9f46d57a636b1f5bb1ce6a77a38ad',
          address: {
            target: {
              source_ref: {
                domain_id: '11111111111111111111111111111111',
                source_id: '22222222222222222222222222222222',
                source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
                source_sha256:
                  'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
              },
              representation_ref: {
                kind: 'source_revision',
                id: '690bc04dba3e40f6bdc92143b4cd9f25',
                producer_sha256: null,
              },
            },
            node_ref: {
              kind: 'c1_node',
              id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
              item_index: null,
            },
          },
          coordinate_space: 'source_utf8',
          reading_text_sha256:
            'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          range: {
            start_byte: 0,
            end_byte: 8184,
            sha256:
              'a6bba8b3e04727d97ed76808a3a55686f58b181016af2834e20f1752e5f05107',
          },
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 19200,
              start_line: 1,
              end_line: 800,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          assembly: 'c1_root_subrange_exact_bytes',
        },
        text: '合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n',
        score: 3.5,
        match_type: 'lexical',
        source_currentness: 'current',
      },
    ],
    total_matches: null,
    pagination: 'none',
    ranking: 'native_lexical_only',
    parent_expansion: 'disabled',
  },
  reads: [
    {
      evidence_ref: {
        evidence_version: '0.2.0-candidate',
        index_ref: {
          index_id:
            'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
          manifest_sha256:
            '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
        },
        index_row_id:
          '9d1201050bade20db806049770c0e80dbd8655ef62087ea56733b29777693e75',
        address: {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '55555555555555555555555555555555',
              source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
              source_sha256:
                '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
            },
            representation_ref: {
              kind: 'parsed_representation',
              id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
              producer_sha256:
                '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
            },
          },
          node_ref: {
            kind: 'parsed_node',
            id: '416d6756f85d70f53128ec36d014fe19ff160dbe23754b3a17f99c5e7113aaa0',
            item_index: null,
          },
        },
        coordinate_space: 'derived_utf8',
        reading_text_sha256:
          '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
        range: {
          start_byte: 0,
          end_byte: 21,
          sha256:
            '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
        },
        locator: {
          kind: 'document',
          source_utf8: null,
          document: {
            chunk_index: 0,
            parent_locator: {
              scheme: 'document_v1',
            },
            scheme: 'derived_passage_v1',
          },
          item_index: null,
        },
        original_position: {
          kind: 'document_only',
        },
        coordinate_precision: 'derived_range',
        assembly: 'existing_passage_full_range',
      },
      text: 'A\tB\n合成\t表格\n1\t2',
      encoding: 'utf-8',
      byte_length: 21,
      source_currentness: 'current',
      complete: true,
      context: {
        node: {
          ref: {
            kind: 'parsed_node',
            id: '416d6756f85d70f53128ec36d014fe19ff160dbe23754b3a17f99c5e7113aaa0',
            item_index: null,
          },
          parent_ref: {
            kind: 'parsed_node',
            id: 'c2c3e095583b17c92af5007dd3f212c554773a92079e093632cd3e6855cebbcf',
            item_index: null,
          },
          kind: 'passage',
          name: '',
          name_truncated: false,
          ordinal: 1,
          locator: {
            kind: 'document',
            source_utf8: null,
            document: {
              chunk_index: 0,
              parent_locator: {
                scheme: 'document_v1',
              },
              scheme: 'derived_passage_v1',
            },
            item_index: null,
          },
          original_position: {
            kind: 'document_only',
          },
          coordinate_precision: 'derived_range',
          text: {
            coordinate_space: 'derived_utf8',
            range: {
              start_byte: 0,
              end_byte: 21,
              sha256:
                '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
            },
            own_range: {
              start_byte: 0,
              end_byte: 21,
              sha256:
                '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
            },
            status: 'available',
            reason: null,
            value: null,
          },
          attributes: {
            source: null,
            structure: null,
            document: {
              empty: null,
              page: null,
              slide: null,
              heading: null,
              table: null,
              row: null,
              cell: null,
              list: null,
              list_item: null,
              media: null,
              passage: {
                chunk_index: 0,
              },
              unsupported_block: null,
            },
            parser: null,
          },
          attachment_ref: null,
        },
        node_omission_reason: null,
        typed_members: [
          {
            node: {
              ref: {
                kind: 'parsed_node',
                id: '1df86954350649e5e58dde54fb6ec47d15942eb9e1587984f40d8729bdd85e1e',
                item_index: null,
              },
              parent_ref: {
                kind: 'parsed_node',
                id: 'c2c3e095583b17c92af5007dd3f212c554773a92079e093632cd3e6855cebbcf',
                item_index: null,
              },
              kind: 'table',
              name: '',
              name_truncated: false,
              ordinal: 0,
              locator: {
                kind: 'parsed_item',
                source_utf8: null,
                document: null,
                item_index: 0,
              },
              original_position: null,
              coordinate_precision: 'item_only',
              text: {
                coordinate_space: 'derived_utf8',
                range: {
                  start_byte: 0,
                  end_byte: 21,
                  sha256:
                    '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
                },
                own_range: {
                  start_byte: 0,
                  end_byte: 21,
                  sha256:
                    '54dd76eaed959653edacd823abe3223a8e2659f7aa8e1112a3f1ebf362e3bac0',
                },
                status: 'available',
                reason: null,
                value: null,
              },
              attributes: {
                source: null,
                structure: null,
                document: null,
                parser: {
                  item_type: 'table',
                  ck_type: 'table',
                  table: {
                    grid: [
                      [
                        {
                          text: 'A',
                        },
                        {
                          text: 'B',
                        },
                      ],
                      [
                        {
                          text: '合成',
                        },
                        {
                          text: '表格',
                        },
                      ],
                      [
                        {
                          text: '1',
                        },
                        {
                          text: '2',
                        },
                      ],
                    ],
                    header_rows: 1,
                    caption: 'Data',
                  },
                  sheet: 'Data',
                  positions: [
                    [1, 1, 1, 1, 2],
                    [1, 2, 2, 1, 2],
                    [1, 3, 3, 1, 2],
                  ],
                  alt_text: null,
                  payload_present: false,
                  text_assembly: 'row_major_uncovered_cells_tab_lf_v1',
                },
              },
              attachment_ref: null,
            },
            payload_sha256:
              'fde14bc87446479fdac2ebf9a0ce800b2f239c732a4278e8f4497da9320484da',
            text_assembly: 'row_major_uncovered_cells_tab_lf_v1',
          },
        ],
        typed_members_scope: 'whole_nodes_contained_in_evidence_range',
        typed_members_complete: true,
        omitted_member_count: 0,
        omission_reasons: [],
        manifest_sha256:
          'bee9bea466c4c7fcb42b210ef53240fda8a0f97d2dfa1911965a57bafb5e86e0',
        membership: 'validated_parsed_manifest',
        representation_state: 'ready',
        parent_expansion: 'disabled',
        metadata_byte_length: 2935,
      },
    },
    {
      evidence_ref: {
        evidence_version: '0.2.0-candidate',
        index_ref: {
          index_id:
            'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
          manifest_sha256:
            '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
        },
        index_row_id:
          'a52a86fcade5887b398fdf1ee8800a0a7990881cc71dee1b22997bc162017b37',
        address: {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
              source_revision_id: '8a9578432e394df78b8fa017487ccf69',
              source_sha256:
                '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
            },
            representation_ref: {
              kind: 'document_representation',
              id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
              producer_sha256:
                'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
            },
          },
          node_ref: {
            kind: 'r2_node',
            id: '45f8352df08c77c7b79da10bfa13cae3a6fcc4c41e548ad91f3cafcc2172e574',
            item_index: null,
          },
        },
        coordinate_space: 'derived_utf8',
        reading_text_sha256:
          'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
        range: {
          start_byte: 0,
          end_byte: 24,
          sha256:
            'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
        },
        locator: {
          kind: 'document',
          source_utf8: null,
          document: {
            chunk_index: 0,
            parent_locator: {
              page_number: 1,
              scheme: 'pdf_page_v1',
            },
            scheme: 'derived_passage_v1',
          },
          item_index: null,
        },
        original_position: {
          kind: 'pdf_page',
          page_number: 1,
        },
        coordinate_precision: 'derived_range',
        assembly: 'existing_passage_full_range',
      },
      text: '合成 R2 派生文字\r\n',
      encoding: 'utf-8',
      byte_length: 24,
      source_currentness: 'current',
      complete: true,
      context: {
        node: {
          ref: {
            kind: 'r2_node',
            id: '45f8352df08c77c7b79da10bfa13cae3a6fcc4c41e548ad91f3cafcc2172e574',
            item_index: null,
          },
          parent_ref: {
            kind: 'r2_node',
            id: '82f2b4dda63bc10efd97de874440afb2cbcbc455f29e22496fe69846aeae1350',
            item_index: null,
          },
          kind: 'passage',
          name: '',
          name_truncated: false,
          ordinal: 0,
          locator: {
            kind: 'document',
            source_utf8: null,
            document: {
              chunk_index: 0,
              parent_locator: {
                page_number: 1,
                scheme: 'pdf_page_v1',
              },
              scheme: 'derived_passage_v1',
            },
            item_index: null,
          },
          original_position: {
            kind: 'pdf_page',
            page_number: 1,
          },
          coordinate_precision: 'derived_range',
          text: {
            coordinate_space: 'derived_utf8',
            range: {
              start_byte: 0,
              end_byte: 24,
              sha256:
                'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
            },
            own_range: {
              start_byte: 0,
              end_byte: 24,
              sha256:
                'ba3750df334f98b252a68e46c0be877a50f0a7dd05a335d9b4cfb4f447c020f7',
            },
            status: 'available',
            reason: null,
            value: null,
          },
          attributes: {
            source: null,
            structure: null,
            document: {
              empty: null,
              page: null,
              slide: null,
              heading: null,
              table: null,
              row: null,
              cell: null,
              list: null,
              list_item: null,
              media: null,
              passage: {
                chunk_index: 0,
              },
              unsupported_block: null,
            },
            parser: null,
          },
          attachment_ref: null,
        },
        node_omission_reason: null,
        typed_members: [],
        typed_members_scope: 'whole_nodes_contained_in_evidence_range',
        typed_members_complete: true,
        omitted_member_count: 0,
        omission_reasons: [],
        manifest_sha256:
          '95f959cba2dca6ceaab1e8a0c65f7f5ca09fb8728497ac2aca1e26f021deae79',
        membership: 'validated_document_manifest',
        representation_state: 'ready',
        parent_expansion: 'disabled',
        metadata_byte_length: 1597,
      },
    },
    {
      evidence_ref: {
        evidence_version: '0.2.0-candidate',
        index_ref: {
          index_id:
            'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
          manifest_sha256:
            '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
        },
        index_row_id:
          'b4bd3f81bf4daa0f6797f2d9b5a81a91bfef20e6beb8985eab67072c2c33758e',
        address: {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          node_ref: {
            kind: 'c1_node',
            id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
            item_index: null,
          },
        },
        coordinate_space: 'source_utf8',
        reading_text_sha256:
          'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
        range: {
          start_byte: 8184,
          end_byte: 16368,
          sha256:
            'a6bba8b3e04727d97ed76808a3a55686f58b181016af2834e20f1752e5f05107',
        },
        locator: {
          kind: 'source_utf8',
          source_utf8: {
            start_byte: 0,
            end_byte: 19200,
            start_line: 1,
            end_line: 800,
          },
          document: null,
          item_index: null,
        },
        original_position: null,
        coordinate_precision: 'source_bytes',
        assembly: 'c1_root_subrange_exact_bytes',
      },
      text: '合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n',
      encoding: 'utf-8',
      byte_length: 8184,
      source_currentness: 'current',
      complete: true,
      context: {
        node: {
          ref: {
            kind: 'c1_node',
            id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
            item_index: null,
          },
          parent_ref: null,
          kind: 'file',
          name: 'long-source.txt',
          name_truncated: false,
          ordinal: null,
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 19200,
              start_line: 1,
              end_line: 800,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          text: {
            coordinate_space: 'source_utf8',
            range: {
              start_byte: 0,
              end_byte: 19200,
              sha256: null,
            },
            own_range: null,
            status: 'available',
            reason: null,
            value: null,
          },
          attributes: {
            source: {
              logical_id:
                '349b490930f67d901d9a685c2c8808a05e33f5f3814b4d308119891f37a4b29b',
              logical_key: '.',
              structure_version: 'source-v1',
            },
            structure: null,
            document: null,
            parser: null,
          },
          attachment_ref: null,
        },
        node_omission_reason: null,
        typed_members: [],
        typed_members_scope: 'whole_nodes_contained_in_evidence_range',
        typed_members_complete: false,
        omitted_member_count: 0,
        omission_reasons: ['typed_members_not_defined_for_source_family'],
        manifest_sha256: null,
        membership: 'stored_c1_root',
        representation_state: 'ready',
        parent_expansion: 'disabled',
        metadata_byte_length: 1192,
      },
    },
    {
      evidence_ref: {
        evidence_version: '0.2.0-candidate',
        index_ref: {
          index_id:
            'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
          manifest_sha256:
            '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
        },
        index_row_id:
          '08048290440d6a9167933938cf403a34d08a2765dc6328b25a5ec3ccb8fac64e',
        address: {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '33333333333333333333333333333333',
              source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
              source_sha256:
                '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
            },
            representation_ref: {
              kind: 'structure_view',
              id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
              producer_sha256:
                '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
            },
          },
          node_ref: {
            kind: 'r1_node',
            id: '338307af91e97ab633a7a387b36651492927d0a9b5a5df07f5c6ce6a253000b5',
            item_index: null,
          },
        },
        coordinate_space: 'source_utf8',
        reading_text_sha256:
          '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
        range: {
          start_byte: 0,
          end_byte: 52,
          sha256:
            '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
        },
        locator: {
          kind: 'source_utf8',
          source_utf8: {
            start_byte: 0,
            end_byte: 52,
            start_line: 1,
            end_line: 3,
          },
          document: null,
          item_index: null,
        },
        original_position: null,
        coordinate_precision: 'source_bytes',
        assembly: 'existing_passage_full_range',
      },
      text: '# 合成 R1 标题\n\n合成固定 Markdown 段落。\n',
      encoding: 'utf-8',
      byte_length: 52,
      source_currentness: 'current',
      complete: true,
      context: {
        node: {
          ref: {
            kind: 'r1_node',
            id: '338307af91e97ab633a7a387b36651492927d0a9b5a5df07f5c6ce6a253000b5',
            item_index: null,
          },
          parent_ref: {
            kind: 'r1_node',
            id: 'ba6626fc6838747416bcfcd88f7706761d28e8210e9ca98860875f0ba8310d79',
            item_index: null,
          },
          kind: 'passage',
          name: '',
          name_truncated: false,
          ordinal: null,
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 52,
              start_line: 1,
              end_line: 3,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          text: {
            coordinate_space: 'source_utf8',
            range: {
              start_byte: 0,
              end_byte: 52,
              sha256: null,
            },
            own_range: null,
            status: 'available',
            reason: null,
            value: null,
          },
          attributes: {
            source: null,
            structure: {
              logical_node_id:
                'e09e4eef0900cc119598971708fb2b869e2ba4a3e474bb7088d1b57b7818ec6e',
              logical_key: 'file/passage/0-52',
              identity_stability: 'range_based',
              parser: 'markdown',
              classification: {
                group_id: null,
                component_side: 'non_code',
                language: '',
              },
            },
            document: null,
            parser: null,
          },
          attachment_ref: null,
        },
        node_omission_reason: null,
        typed_members: [],
        typed_members_scope: 'whole_nodes_contained_in_evidence_range',
        typed_members_complete: false,
        omitted_member_count: 0,
        omission_reasons: ['typed_members_not_defined_for_source_family'],
        manifest_sha256: null,
        membership: 'validated_r1_passage',
        representation_state: 'ready',
        parent_expansion: 'disabled',
        metadata_byte_length: 1403,
      },
    },
    {
      evidence_ref: {
        evidence_version: '0.2.0-candidate',
        index_ref: {
          index_id:
            'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
          manifest_sha256:
            '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
        },
        index_row_id:
          '137c79def7271d34650b2a59a682c7be84411f30207a299a4771e01dd2dd58f2',
        address: {
          target: {
            source_ref: {
              domain_id: '11111111111111111111111111111111',
              source_id: '22222222222222222222222222222222',
              source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
              source_sha256:
                'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
            },
            representation_ref: {
              kind: 'source_revision',
              id: '690bc04dba3e40f6bdc92143b4cd9f25',
              producer_sha256: null,
            },
          },
          node_ref: {
            kind: 'c1_node',
            id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
            item_index: null,
          },
        },
        coordinate_space: 'source_utf8',
        reading_text_sha256:
          'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
        range: {
          start_byte: 16368,
          end_byte: 19200,
          sha256:
            '3b73281e7e5d5decc1bf90e5abe350322cf18290e469ea224ffbe49b5e064890',
        },
        locator: {
          kind: 'source_utf8',
          source_utf8: {
            start_byte: 0,
            end_byte: 19200,
            start_line: 1,
            end_line: 800,
          },
          document: null,
          item_index: null,
        },
        original_position: null,
        coordinate_precision: 'source_bytes',
        assembly: 'c1_root_subrange_exact_bytes',
      },
      text: '合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n合成 C1 原文区间\r\n',
      encoding: 'utf-8',
      byte_length: 2832,
      source_currentness: 'current',
      complete: true,
      context: {
        node: {
          ref: {
            kind: 'c1_node',
            id: '6daf66840a442c2280733d8b253b16e8ded2bfa9e7e9d4865342f989d3eeeae8',
            item_index: null,
          },
          parent_ref: null,
          kind: 'file',
          name: 'long-source.txt',
          name_truncated: false,
          ordinal: null,
          locator: {
            kind: 'source_utf8',
            source_utf8: {
              start_byte: 0,
              end_byte: 19200,
              start_line: 1,
              end_line: 800,
            },
            document: null,
            item_index: null,
          },
          original_position: null,
          coordinate_precision: 'source_bytes',
          text: {
            coordinate_space: 'source_utf8',
            range: {
              start_byte: 0,
              end_byte: 19200,
              sha256: null,
            },
            own_range: null,
            status: 'available',
            reason: null,
            value: null,
          },
          attributes: {
            source: {
              logical_id:
                '349b490930f67d901d9a685c2c8808a05e33f5f3814b4d308119891f37a4b29b',
              logical_key: '.',
              structure_version: 'source-v1',
            },
            structure: null,
            document: null,
            parser: null,
          },
          attachment_ref: null,
        },
        node_omission_reason: null,
        typed_members: [],
        typed_members_scope: 'whole_nodes_contained_in_evidence_range',
        typed_members_complete: false,
        omitted_member_count: 0,
        omission_reasons: ['typed_members_not_defined_for_source_family'],
        manifest_sha256: null,
        membership: 'stored_c1_root',
        representation_state: 'ready',
        parent_expansion: 'disabled',
        metadata_byte_length: 1192,
      },
    },
  ],
  emptyDocx: {
    evidence_ref: {
      evidence_version: '0.2.0-candidate',
      index_ref: {
        index_id:
          '4929ad1b76a2327466e1f82bf2e89f3d50c5c9e0fc1c65ea44dee35baca157aa',
        manifest_sha256:
          '5a90c33ece5d6488bc54841a2d7cece53c5c68e31393e07ecd21bc5b1791c49a',
      },
      index_row_id:
        '63c2176373d043be22327d2d8788984de860876a0da4133b296800e1d1654bbd',
      address: {
        target: {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '45b6ae9e4ad54ec4af62c48a868a6461',
            source_revision_id: '5bd1f94ea2ad4c8892515f2cd224dea2',
            source_sha256:
              '74da54f68e73e815a95d8ecdb943d0b5260262c2fb2fcc8b2ab18f5449bbb264',
          },
          representation_ref: {
            kind: 'document_representation',
            id: 'b1bc78a8ed05c3a51b19a79ef3694482c36ff440d77e0cc818017a404144ff50',
            producer_sha256:
              'c9af07a68527cfc70dda428c741fd5865c415feedce891cf304088bc4d004830',
          },
        },
        node_ref: {
          kind: 'r2_node',
          id: '432227e51c707556253145aafb665ae806c2beb0f56719460405ec164e3eb5cf',
          item_index: null,
        },
      },
      coordinate_space: 'derived_utf8',
      reading_text_sha256:
        '73830ade66cf431974e13d7b61d6a5679a0a1b703ea32168a2e8a2cf9d4a9262',
      range: {
        start_byte: 0,
        end_byte: 16,
        sha256:
          '73830ade66cf431974e13d7b61d6a5679a0a1b703ea32168a2e8a2cf9d4a9262',
      },
      locator: {
        kind: 'document',
        source_utf8: null,
        document: {
          chunk_index: 0,
          parent_locator: {
            part: 'word/document.xml',
            scheme: 'office_package_v1',
            segments: [
              {
                namespace:
                  'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                name: 'document',
                index: 0,
              },
              {
                namespace:
                  'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                name: 'body',
                index: 0,
              },
            ],
          },
          scheme: 'derived_passage_v1',
        },
        item_index: null,
      },
      original_position: {
        kind: 'document_only',
      },
      coordinate_precision: 'derived_range',
      assembly: 'existing_passage_full_range',
    },
    text: '合成 DOCX\nA\t\tB',
    encoding: 'utf-8',
    byte_length: 16,
    source_currentness: 'current',
    complete: true,
    context: {
      node: {
        ref: {
          kind: 'r2_node',
          id: '432227e51c707556253145aafb665ae806c2beb0f56719460405ec164e3eb5cf',
          item_index: null,
        },
        parent_ref: {
          kind: 'r2_node',
          id: '4a158de09e435a222b1f32922e35935b39b7c44570c195a136ea9eba36add2ef',
          item_index: null,
        },
        kind: 'passage',
        name: '',
        name_truncated: false,
        ordinal: 2,
        locator: {
          kind: 'document',
          source_utf8: null,
          document: {
            chunk_index: 0,
            parent_locator: {
              part: 'word/document.xml',
              scheme: 'office_package_v1',
              segments: [
                {
                  namespace:
                    'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                  name: 'document',
                  index: 0,
                },
                {
                  namespace:
                    'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                  name: 'body',
                  index: 0,
                },
              ],
            },
            scheme: 'derived_passage_v1',
          },
          item_index: null,
        },
        original_position: {
          kind: 'document_only',
        },
        coordinate_precision: 'derived_range',
        text: {
          coordinate_space: 'derived_utf8',
          range: {
            start_byte: 0,
            end_byte: 16,
            sha256:
              '73830ade66cf431974e13d7b61d6a5679a0a1b703ea32168a2e8a2cf9d4a9262',
          },
          own_range: {
            start_byte: 0,
            end_byte: 16,
            sha256:
              '73830ade66cf431974e13d7b61d6a5679a0a1b703ea32168a2e8a2cf9d4a9262',
          },
          status: 'available',
          reason: null,
          value: null,
        },
        attributes: {
          source: null,
          structure: null,
          document: {
            empty: null,
            page: null,
            slide: null,
            heading: null,
            table: null,
            row: null,
            cell: null,
            list: null,
            list_item: null,
            media: null,
            passage: {
              chunk_index: 0,
            },
            unsupported_block: null,
          },
          parser: null,
        },
        attachment_ref: null,
      },
      node_omission_reason: null,
      typed_members: [
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: 'd1c5cbf5eb9f4196eb28e32b53e066d895d0564dc751884997f52740c2e5649a',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: '4a158de09e435a222b1f32922e35935b39b7c44570c195a136ea9eba36add2ef',
              item_index: null,
            },
            kind: 'paragraph',
            name: '合成 DOCX',
            name_truncated: false,
            ordinal: 0,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'p',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 0,
                end_byte: 11,
                sha256:
                  '544a526799eeed6ce8b36937e79dde7861dfbacf98da0a179dfde9afca2c5155',
              },
              own_range: {
                start_byte: 0,
                end_byte: 11,
                sha256:
                  '544a526799eeed6ce8b36937e79dde7861dfbacf98da0a179dfde9afca2c5155',
              },
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: {},
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: null,
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            'da616ac69f7a537a8d7db9df41d5c71ab97e66eb0822bbdec0491cacd8983e64',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: '06b432677d8d88c54f8d031303c9174bfccef91686e96dd15529268a44f81a7c',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: 'c41980f90927b7caa045fae92500f635547a378b024ecde3ee908ff729e633a5',
              item_index: null,
            },
            kind: 'cell',
            name: '',
            name_truncated: false,
            ordinal: 0,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tc',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 12,
                end_byte: 13,
                sha256:
                  '559aead08264d5795d3909718cdd05abd49572e84fe55590eef31a88a08fdffd',
              },
              own_range: null,
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: null,
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: {
                  row_index: 0,
                  cell_index: 0,
                  row_span: null,
                  column_span: null,
                  is_header: null,
                },
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            '58083566964b604fa0906430f4a610aee782262dbc247d52c1a6349ffb2674cc',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: 'a8331095da67885e1732c689c6b53ed52ffcd8aa6f48de7f4bbe8d458ade0161',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: '06b432677d8d88c54f8d031303c9174bfccef91686e96dd15529268a44f81a7c',
              item_index: null,
            },
            kind: 'paragraph',
            name: 'A',
            name_truncated: false,
            ordinal: 0,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tc',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'p',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 12,
                end_byte: 13,
                sha256:
                  '559aead08264d5795d3909718cdd05abd49572e84fe55590eef31a88a08fdffd',
              },
              own_range: {
                start_byte: 12,
                end_byte: 13,
                sha256:
                  '559aead08264d5795d3909718cdd05abd49572e84fe55590eef31a88a08fdffd',
              },
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: {},
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: null,
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            'f14b7e9e5a3c0df14bd25ff2399fb8ede84d2e1931b7e9aa01429aa6d07f5f67',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: '380bece72ace9c41ff8728bb28dac4fa58e23892dc7d7fbab187d8dbe2a0c4c9',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: '4a158de09e435a222b1f32922e35935b39b7c44570c195a136ea9eba36add2ef',
              item_index: null,
            },
            kind: 'table',
            name: '',
            name_truncated: false,
            ordinal: 1,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 12,
                end_byte: 16,
                sha256:
                  'fff4cf69e39b63ddf0400cd6388cf374f6df1f1892efdfe8875bf0b60c64c3ad',
              },
              own_range: null,
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: null,
                page: null,
                slide: null,
                heading: null,
                table: {
                  row_count: 1,
                  max_cells_per_row: 3,
                  layout_precision: 'source_array',
                },
                row: null,
                cell: null,
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            'b19f1d9ee6d9dbdf9bdfd9d353db07a7467759837646decd9556f21cc0fc7c67',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: 'c41980f90927b7caa045fae92500f635547a378b024ecde3ee908ff729e633a5',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: '380bece72ace9c41ff8728bb28dac4fa58e23892dc7d7fbab187d8dbe2a0c4c9',
              item_index: null,
            },
            kind: 'row',
            name: '',
            name_truncated: false,
            ordinal: 0,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 12,
                end_byte: 16,
                sha256:
                  'fff4cf69e39b63ddf0400cd6388cf374f6df1f1892efdfe8875bf0b60c64c3ad',
              },
              own_range: null,
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: null,
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: {
                  row_index: 0,
                  cell_count: 3,
                },
                cell: null,
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            '711263a149d0bfe0c1b20e756fcd304293afdd04b93f6ff7541314c1118b1453',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: '6806c17522a8005790d49b71c64656b58a1df60f002071fd938f619872535cd6',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: '97c32c46be39874cfe9fba4a5783f95e2986177f4909a93614fe5e659a001df2',
              item_index: null,
            },
            kind: 'paragraph',
            name: '',
            name_truncated: false,
            ordinal: 0,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tc',
                    index: 1,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'p',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 14,
                end_byte: 14,
                sha256:
                  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
              },
              own_range: {
                start_byte: 14,
                end_byte: 14,
                sha256:
                  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
              },
              status: 'empty',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: {},
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: null,
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            '36c4e631107e980667fda5c449371aa0c4d0d74c5a91c52f410bef0d9a83f57d',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: '97c32c46be39874cfe9fba4a5783f95e2986177f4909a93614fe5e659a001df2',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: 'c41980f90927b7caa045fae92500f635547a378b024ecde3ee908ff729e633a5',
              item_index: null,
            },
            kind: 'cell',
            name: '',
            name_truncated: false,
            ordinal: 1,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tc',
                    index: 1,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 14,
                end_byte: 14,
                sha256:
                  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
              },
              own_range: null,
              status: 'empty',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: null,
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: {
                  row_index: 0,
                  cell_index: 1,
                  row_span: null,
                  column_span: null,
                  is_header: null,
                },
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            'fe92a6463f32513b6bec6f9f98d3f0bb775fb101621b9780c2907b8aaed8888a',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: '72f9a0235015f20f03c8966a8dd794cced9bd6e15cc9eabaa873d2d28f09b6bc',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: 'c41980f90927b7caa045fae92500f635547a378b024ecde3ee908ff729e633a5',
              item_index: null,
            },
            kind: 'cell',
            name: '',
            name_truncated: false,
            ordinal: 2,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tc',
                    index: 2,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 15,
                end_byte: 16,
                sha256:
                  'df7e70e5021544f4834bbee64a9e3789febc4be81470df629cad6ddb03320a5c',
              },
              own_range: null,
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: null,
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: {
                  row_index: 0,
                  cell_index: 2,
                  row_span: null,
                  column_span: null,
                  is_header: null,
                },
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            '7aff7bdd04fafdb1d240863e089cc06ead10003b455b08796ef138b1e0a50a6f',
          text_assembly: 'existing_reading_range',
        },
        {
          node: {
            ref: {
              kind: 'r2_node',
              id: '8ae954215dc75a3b9a92cd68261f81d97efd0b5b7283842f553b3114824248a1',
              item_index: null,
            },
            parent_ref: {
              kind: 'r2_node',
              id: '72f9a0235015f20f03c8966a8dd794cced9bd6e15cc9eabaa873d2d28f09b6bc',
              item_index: null,
            },
            kind: 'paragraph',
            name: 'B',
            name_truncated: false,
            ordinal: 0,
            locator: {
              kind: 'document',
              source_utf8: null,
              document: {
                part: 'word/document.xml',
                scheme: 'office_package_v1',
                segments: [
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'document',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'body',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tbl',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tr',
                    index: 0,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'tc',
                    index: 2,
                  },
                  {
                    namespace:
                      'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
                    name: 'p',
                    index: 0,
                  },
                ],
              },
              item_index: null,
            },
            original_position: {
              kind: 'document_only',
            },
            coordinate_precision: 'source_path',
            text: {
              coordinate_space: 'derived_utf8',
              range: {
                start_byte: 15,
                end_byte: 16,
                sha256:
                  'df7e70e5021544f4834bbee64a9e3789febc4be81470df629cad6ddb03320a5c',
              },
              own_range: {
                start_byte: 15,
                end_byte: 16,
                sha256:
                  'df7e70e5021544f4834bbee64a9e3789febc4be81470df629cad6ddb03320a5c',
              },
              status: 'available',
              reason: null,
              value: null,
            },
            attributes: {
              source: null,
              structure: null,
              document: {
                empty: {},
                page: null,
                slide: null,
                heading: null,
                table: null,
                row: null,
                cell: null,
                list: null,
                list_item: null,
                media: null,
                passage: null,
                unsupported_block: null,
              },
              parser: null,
            },
            attachment_ref: null,
          },
          payload_sha256:
            'a44d459735c81dcf1cc7246e53ab2a4be3169fb0288fe075796edfca486bc089',
          text_assembly: 'existing_reading_range',
        },
      ],
      typed_members_scope: 'whole_nodes_contained_in_evidence_range',
      typed_members_complete: true,
      omitted_member_count: 0,
      omission_reasons: [],
      manifest_sha256:
        '9c4738d4dea2120801948e83369e7a88de1c4b4e1b47bd58d0af176d5bbed7d3',
      membership: 'validated_document_manifest',
      representation_state: 'ready',
      parent_expansion: 'disabled',
      metadata_byte_length: 17286,
    },
  },
  createBody: {
    contract_version: '0.2.0-candidate',
    idempotency_key: 'one_registered_mixed_job',
    expected_local_policy_version: 1,
    expected_content_policy_version: 3,
    expected_foundation_policy_version: 1,
    expected_profile_sha256:
      '9fbd55ea73d9ce90cdde82c00c912afb50303433f8ffe5356de3edca626eb063',
    selection: {
      kind: 'fixed_representations',
      targets: [
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '22222222222222222222222222222222',
            source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
            source_sha256:
              'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          },
          representation_ref: {
            kind: 'source_revision',
            id: '690bc04dba3e40f6bdc92143b4cd9f25',
            producer_sha256: null,
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '33333333333333333333333333333333',
            source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
            source_sha256:
              '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          },
          representation_ref: {
            kind: 'structure_view',
            id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
            producer_sha256:
              '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
            source_revision_id: '8a9578432e394df78b8fa017487ccf69',
            source_sha256:
              '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
          },
          representation_ref: {
            kind: 'document_representation',
            id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
            producer_sha256:
              'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '55555555555555555555555555555555',
            source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
            source_sha256:
              '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
          },
          representation_ref: {
            kind: 'parsed_representation',
            id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
            producer_sha256:
              '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
          },
        },
      ],
    },
  },
  searchBody: {
    contract_version: '0.2.0-candidate',
    index_ref: {
      index_id:
        'd266ac1e50d37b601b4135e6ed0f92f892f1754eafae3bc9451e78647f2e5b6c',
      manifest_sha256:
        '50087717902bb02e36ceb16db53ce3983a96c782e66a3f7be508c31dae304162',
    },
    selection: {
      kind: 'fixed_representations',
      targets: [
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '22222222222222222222222222222222',
            source_revision_id: '690bc04dba3e40f6bdc92143b4cd9f25',
            source_sha256:
              'ac668ce68cbdb35a61b3cbcf47b9a77099b6d3a51e5b4a6d7a3ee19b2ebb780f',
          },
          representation_ref: {
            kind: 'source_revision',
            id: '690bc04dba3e40f6bdc92143b4cd9f25',
            producer_sha256: null,
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '33333333333333333333333333333333',
            source_revision_id: 'c04d9331cf6548d7a7dfbd9f2dbba623',
            source_sha256:
              '7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce',
          },
          representation_ref: {
            kind: 'structure_view',
            id: '183e14ee583d54a4081e03ba0178cb0bba3a0826f09d7fffd433715929522697',
            producer_sha256:
              '736069464e2fc9a92448fa9a12ec1d8ae64d790765162f8b2eb46c3399e24986',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '81d8929e6b83428c8da6e5d0d8e94b3d',
            source_revision_id: '8a9578432e394df78b8fa017487ccf69',
            source_sha256:
              '5d2b53d88f2c052b5cbfd099910f02b69c7ce23a78891069e409424d68bc7ab3',
          },
          representation_ref: {
            kind: 'document_representation',
            id: 'f6feb8fcb74b66e5d87abf3c7478693ee500992e1c6c04cafacf0a62d47ab89f',
            producer_sha256:
              'db897742edeb03a07b0f04df3cd94c2d9ed3869637ae2f94652b5dfc6d3ef6a3',
          },
        },
        {
          source_ref: {
            domain_id: '11111111111111111111111111111111',
            source_id: '55555555555555555555555555555555',
            source_revision_id: '47b2753e8af643f9bc630032a06f9ad8',
            source_sha256:
              '76e2d6a34ceadd3d5e0e00f9a5c77bcbdd332c4af63fa15ed8270356acb3ca0f',
          },
          representation_ref: {
            kind: 'parsed_representation',
            id: '89728488ca4379a2db5da1010174683719bd8cc203bd75492c30165bda6e4a5b',
            producer_sha256:
              '8e47d3ab0af47734bfe525281c31473962cacf1034ada56c5f96c99b5af54ec1',
          },
        },
      ],
    },
    query: '合成',
    limit: 20,
  },
};
const PolicyFixture: LocalLexicalPolicy = {
  domain_id: '11111111111111111111111111111111',
  local_policy_version: 1,
  foundation_policy_version: 1,
  enabled: true,
  mode: 'owner_only_lexical',
  read_authority: 'existing_foundation_owner',
  accepted_inputs: ['c1_text_root_at_most_8192_bytes', 'r1_published_passage'],
  allowed_model_uses: [],
  external_models: false,
  local_inference: false,
  remote_fetches: false,
  automatic_ingestion: false,
  vector_index: 'not_created',
};

const Wire = { ...service },
  Http = request as unknown as jest.Mock,
  Session = getKnowledgeSessionScope as jest.Mock;
const Scope = { owner: 'a'.repeat(32), domain: '1'.repeat(32) },
  Targets = Fixtures.ready.readiness.selection.targets,
  Selection = Fixtures.ready.readiness.selection;
const C1 = Targets.find(
    (target) => target.representation_ref.kind === 'source_revision',
  )!,
  R1 = Targets.find(
    (target) => target.representation_ref.kind === 'structure_view',
  )!,
  R2 = Targets.find(
    (target) => target.representation_ref.kind === 'document_representation',
  )!,
  Parsed = Targets.find(
    (target) => target.representation_ref.kind === 'parsed_representation',
  )!;
const clients: Array<{ clear: () => void }> = [];
let localEnabled = false,
  contentEnabled = false,
  parseEnabled = false,
  indexed = false,
  localVersion = 1,
  contentVersion = 4,
  lastJob: LocalLexicalJob | null = null;
const fresh = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
const policy = (): LocalLexicalPolicy => ({
  ...fresh(PolicyFixture),
  domain_id: Scope.domain,
  enabled: localEnabled,
  local_policy_version: localVersion,
  foundation_policy_version: 1,
});
const contentPolicy = (): ContentPolicy => ({
  domain_id: Scope.domain,
  foundation_policy_version: 1,
  content_policy_version: contentVersion,
  parse_enabled: parseEnabled,
  lexical_enabled: contentEnabled,
});
function inspect(selection: LocalLexicalSelection): LocalLexicalReadinessData {
  const value = fresh(indexed ? Fixtures.ready : Fixtures.inspect);
  value.readiness.selection = selection;
  value.readiness.coverage = value.readiness.coverage.filter((row) =>
    selection.targets.some(
      (target) =>
        target.source_ref.source_id === row.target.source_ref.source_id,
    ),
  );
  Object.assign(value.effective_policy, {
    local_policy_version: localVersion,
    content_policy_version: contentVersion,
    local_lexical_enabled: localEnabled,
    content_lexical_enabled: contentEnabled,
    enabled: localEnabled && contentEnabled,
  });
  return value;
}
function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
function response(
  config: { transformResponse: Array<(raw: unknown) => unknown> },
  operation: string,
  data: unknown,
  status = 200,
  version = '0.2.0-candidate',
) {
  const envelope = {
    operation,
    contract_version: version,
    request_id: 'b'.repeat(32),
    data,
  };
  return {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
      'x-knowledge-content-version': version,
      'x-knowledge-content-lexical-protocol': 'content_lexical/0.2.0',
      'x-request-id': envelope.request_id,
    },
    data: config.transformResponse[0](
      new TextEncoder().encode(JSON.stringify(envelope)),
    ),
  };
}
beforeAll(() => {
  Object.assign(globalThis, { TextEncoder, TextDecoder });
  Object.defineProperty(globalThis, 'crypto', {
    value: webcrypto,
    configurable: true,
  });
});
beforeEach(() => {
  localEnabled = false;
  contentEnabled = false;
  parseEnabled = false;
  indexed = false;
  localVersion = 1;
  contentVersion = 4;
  lastJob = null;
  Http.mockReset();
  Session.mockReturnValue(1);
  sessionStorage.clear();
  jest.spyOn(service, 'policy').mockImplementation(async () => policy());
  jest
    .spyOn(service, 'changePolicy')
    .mockImplementation(async (_scope, body) => {
      localEnabled = body.enabled;
      localVersion++;
      return {
        idempotency_key: body.idempotency_key,
        request_sha256: 'c'.repeat(64),
        outcome: 'changed',
        policy: policy(),
      };
    });
  jest
    .spyOn(contentService, 'policy')
    .mockImplementation(async () => contentPolicy());
  jest
    .spyOn(contentService, 'changePolicy')
    .mockImplementation(async (_scope, body) => {
      contentEnabled = body.lexical_enabled;
      parseEnabled = body.parse_enabled;
      contentVersion++;
      return contentPolicy();
    });
  jest
    .spyOn(service, 'readiness')
    .mockImplementation(async (_scope, body) => inspect(body.selection));
  jest.spyOn(service, 'createJob').mockImplementation(async (_scope, body) => {
    indexed = true;
    lastJob = {
      ...fresh(Fixtures.created),
      idempotency_key: body.idempotency_key,
      selection: body.selection,
      captured_local_policy_version: body.expected_local_policy_version,
      captured_content_policy_version: body.expected_content_policy_version,
      captured_foundation_policy_version:
        body.expected_foundation_policy_version,
      captured_profile_sha256: body.expected_profile_sha256,
    };
    return lastJob;
  });
  jest.spyOn(service, 'job').mockImplementation(async () => ({
    ...fresh(Fixtures.published),
    selection: lastJob?.selection ?? Selection,
  }));
  jest.spyOn(service, 'search').mockImplementation(async (_scope, body) => ({
    ...fresh(Fixtures.search),
    query: body.query,
    requested_limit: body.limit,
    ...(body.query === 'empty' ? { hits: [], returned_count: 0 } : {}),
  }));
  jest
    .spyOn(service, 'read')
    .mockImplementation(async (_scope, body) =>
      fresh(
        Fixtures.reads.find(
          (value) =>
            value.evidence_ref.index_row_id === body.evidence_ref.index_row_id,
        )!,
      ),
    );
  const source = {
    domain_id: Scope.domain,
    source_id: C1.source_ref.source_id,
    source_revision_id: C1.source_ref.source_revision_id,
    sha256: C1.source_ref.source_sha256,
    relative_path: 'synthetic-long.txt',
    byte_length: 20000,
    is_current: true,
  };
  (useKnowledgeSources as jest.Mock).mockReturnValue({
    data: { pages: [{ items: [source] }] },
    hasNextPage: false,
    isFetching: false,
  });
  (useKnowledgeSource as jest.Mock).mockReturnValue({ data: source });
  (useStructureHead as jest.Mock).mockReturnValue({
    data: { view_id: R1.representation_ref.id },
  });
  (useStructureView as jest.Mock).mockReturnValue({
    data: {
      domain_id: Scope.domain,
      view_id: R1.representation_ref.id,
      producer_sha256: R1.representation_ref.producer_sha256,
      members: [
        {
          source_id: R1.source_ref.source_id,
          source_revision_id: R1.source_ref.source_revision_id,
          sha256: R1.source_ref.source_sha256,
          relative_path: 'synthetic-structure.ts',
        },
      ],
    },
  });
  const original = {
    domain_id: Scope.domain,
    source_id: R2.source_ref.source_id,
    source_revision_id: R2.source_ref.source_revision_id,
    sha256: R2.source_ref.source_sha256,
    original_filename: 'synthetic.docx',
  };
  (globalThis as unknown as { lexicalOriginal: unknown }).lexicalOriginal =
    original;
  (useDocumentHead as jest.Mock).mockReturnValue({
    data: {
      domain_id: Scope.domain,
      source_id: original.source_id,
      active_source_revision_id: original.source_revision_id,
      active_representation_id: R2.representation_ref.id,
    },
  });
  (useDocumentRepresentation as jest.Mock).mockReturnValue({
    data: {
      domain_id: Scope.domain,
      representation_id: R2.representation_ref.id,
      producer_sha256: R2.representation_ref.producer_sha256,
      original,
    },
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  sessionStorage.clear();
  jest.restoreAllMocks();
});

test('默认两项词法许可关闭；读取和固定选择均不写策略或创建索引', async () => {
  render(<LocalLexicalBrowser owner={Scope.owner} domain={Scope.domain} />, {
    wrapper,
  });
  await screen.findByRole('region', {
    name: 'knowledgeLocalLexical.contentPolicy',
  });
  fireEvent.click(screen.getByText('knowledgeLocalLexical.addC1'));
  await screen.findByText('knowledgeLocalLexical.readiness_unindexed');
  expect(service.changePolicy).not.toHaveBeenCalled();
  expect(contentService.changePolicy).not.toHaveBeenCalled();
  expect(service.createJob).not.toHaveBeenCalled();
  expect(
    screen.getByRole('button', { name: 'knowledgeLocalLexical.createIndex' }),
  ).toBeDisabled();
  expect(
    screen.getByRole('button', { name: 'knowledgeLocalLexical.enable' }),
  ).toBeDisabled();
  expect(
    screen.getByRole('button', { name: 'knowledgeLocalLexical.enableContent' }),
  ).toBeDisabled();
});
test('两项开关独立确认；开启内容词法保留实际解析开关且不自动索引', async () => {
  parseEnabled = true;
  render(<LocalLexicalBrowser owner={Scope.owner} domain={Scope.domain} />, {
    wrapper,
  });
  await screen.findByRole('region', {
    name: 'knowledgeLocalLexical.contentPolicy',
  });
  fireEvent.click(screen.getByLabelText('knowledgeLocalLexical.confirmEnable'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeLocalLexical.enable' }),
  );
  await waitFor(() => expect(service.changePolicy).toHaveBeenCalledTimes(1));
  await waitFor(() =>
    expect(
      screen.getByRole('button', {
        name: 'knowledgeLocalLexical.enableContent',
      }),
    ).toBeDisabled(),
  );
  fireEvent.click(
    screen.getByLabelText('knowledgeLocalLexical.confirmContentEnable'),
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeLocalLexical.enableContent' }),
  );
  await waitFor(() =>
    expect(contentService.changePolicy).toHaveBeenCalledTimes(1),
  );
  expect(
    jest.mocked(contentService.changePolicy).mock.calls[0][1],
  ).toMatchObject({
    expected_policy_version: 1,
    expected_content_policy_version: 4,
    parse_enabled: true,
    lexical_enabled: true,
    confirmed: true,
  });
  expect(jest.mocked(service.changePolicy).mock.calls[0][1]).toMatchObject({
    contract_version: '0.1.0-candidate',
    expected_local_policy_version: 1,
    enabled: true,
  });
  expect(service.createJob).not.toHaveBeenCalled();
});
test('解析结果加入后可复用实际R1/R2选择器形成四类混合集合；显式创建携带检查所得版本和profile', async () => {
  localEnabled = true;
  contentEnabled = true;
  render(
    <LocalLexicalBrowser
      owner={Scope.owner}
      domain={Scope.domain}
      initialChoice={{
        path: 'synthetic.parsed',
        target: Parsed,
        currentness: 'unknown',
      }}
    />,
    { wrapper },
  );
  await screen.findByRole('region', {
    name: 'knowledgeLocalLexical.contentPolicy',
  });
  fireEvent.click(screen.getByText('knowledgeLocalLexical.addC1'));
  fireEvent.click(screen.getByText('knowledgeLocalLexical.structureView'));
  fireEvent.click(await screen.findByText('fixture-project'));
  fireEvent.click(
    await screen.findByText('knowledgeConsumption.fixCurrentView'),
  );
  fireEvent.click(await screen.findByText('synthetic-structure.ts'));
  fireEvent.click(screen.getByText('knowledgeLocalLexical.documentView'));
  fireEvent.click(await screen.findByText('fixture-original'));
  fireEvent.click(await screen.findByText('synthetic.docx'));
  await waitFor(() =>
    expect(
      jest.mocked(service.readiness).mock.calls.at(-1)?.[1].selection.targets,
    ).toHaveLength(4),
  );
  await waitFor(() =>
    expect(
      screen.getByLabelText('knowledgeLocalLexical.confirmIndex'),
    ).toBeEnabled(),
  );
  fireEvent.click(screen.getByLabelText('knowledgeLocalLexical.confirmIndex'));
  fireEvent.click(
    screen.getByRole('button', { name: 'knowledgeLocalLexical.createIndex' }),
  );
  await waitFor(() => expect(service.createJob).toHaveBeenCalledTimes(1));
  const body = jest.mocked(service.createJob).mock.calls[0][1];
  expect(body).toMatchObject({
    contract_version: '0.2.0-candidate',
    expected_local_policy_version: 1,
    expected_content_policy_version: 4,
    expected_foundation_policy_version: 1,
    expected_profile_sha256: Fixtures.ready.profile.profile_sha256,
  });
  expect(localLexicalSelectionKey(body.selection)).toBe(
    localLexicalSelectionKey(Selection),
  );
  await screen.findByText('knowledgeLocalLexical.readiness_ready');
});
test('固定表示切换需显式替换；重复添加同一固定表示不会增加来源', async () => {
  localEnabled = true;
  contentEnabled = true;
  render(
    <LocalLexicalBrowser
      owner={Scope.owner}
      domain={Scope.domain}
      initialChoice={{
        path: 'old-synthetic.txt',
        target: {
          ...C1,
          source_ref: { ...C1.source_ref, source_revision_id: '9'.repeat(32) },
          representation_ref: { ...C1.representation_ref, id: '9'.repeat(32) },
        },
        currentness: 'unknown',
      }}
    />,
    { wrapper },
  );
  await screen.findByText('knowledgeLocalLexical.addC1');
  fireEvent.click(screen.getByText('knowledgeLocalLexical.addC1'));
  await screen.findByText('knowledgeLocalLexical.replaceHint');
  expect(
    jest.mocked(service.readiness).mock.calls.at(-1)?.[1].selection.targets[0]
      .source_ref.source_revision_id,
  ).toBe('9'.repeat(32));
  fireEvent.click(screen.getByText('knowledgeLocalLexical.replace'));
  await waitFor(() =>
    expect(
      jest.mocked(service.readiness).mock.calls.at(-1)?.[1].selection
        .targets[0],
    ).toEqual(C1),
  );
  fireEvent.click(screen.getByText('knowledgeLocalLexical.addC1'));
  expect(
    screen.queryByText('knowledgeLocalLexical.replaceHint'),
  ).not.toBeInTheDocument();
  expect(
    jest.mocked(service.readiness).mock.calls.at(-1)?.[1].selection.targets,
  ).toHaveLength(1);
});
test('搜索保留引擎命中顺序，固定证据回读带精确地址；新查询先清除旧读并允许成功零命中', async () => {
  localEnabled = true;
  contentEnabled = true;
  indexed = true;
  render(
    <LocalLexicalBrowser
      owner={Scope.owner}
      domain={Scope.domain}
      initialChoice={{
        path: 'synthetic.parsed',
        target: Parsed,
        currentness: 'unknown',
      }}
    />,
    { wrapper },
  );
  const form = await screen.findByRole('region', {
    name: 'knowledgeLocalLexical.search',
  });
  fireEvent.change(within(form).getByLabelText('knowledgeLocalLexical.query'), {
    target: { value: '合成' },
  });
  fireEvent.change(within(form).getByLabelText('knowledgeLocalLexical.limit'), {
    target: { value: '20' },
  });
  fireEvent.click(
    within(form).getByRole('button', { name: 'knowledgeLocalLexical.search' }),
  );
  await screen.findByText(/knowledgeLocalLexical.returned/);
  const reads = screen.getAllByRole('button', {
    name: 'knowledgeLocalLexical.readUnit',
  });
  expect(reads).toHaveLength(Fixtures.search.hits.length);
  fireEvent.click(reads[0]);
  const fixed = await screen.findByRole('region', {
    name: 'knowledgeLocalLexical.fixedUnit',
  });
  expect(jest.mocked(service.read).mock.calls[0][1].evidence_ref).toEqual(
    Fixtures.search.hits[0].evidence_ref,
  );
  expect(
    within(fixed).getByRole('link', { name: 'knowledgeConsumption.fixedLink' }),
  ).toHaveAttribute(
    'href',
    contentFixedLink(Fixtures.search.hits[0].evidence_ref.address),
  );
  expect(jest.mocked(service.search).mock.calls[0][1].limit).toBe(20);
  fireEvent.change(within(form).getByLabelText('knowledgeLocalLexical.query'), {
    target: { value: 'empty' },
  });
  fireEvent.click(
    within(form).getByRole('button', { name: 'knowledgeLocalLexical.search' }),
  );
  await screen.findByText('knowledgeLocalLexical.emptyHits');
  expect(
    screen.queryByRole('region', { name: 'knowledgeLocalLexical.fixedUnit' }),
  ).not.toBeInTheDocument();
});
test('真实合成DOCX owning context 的空单元格与空段落均保留，展示结构属性和明确缺省边界', () => {
  render(<LocalLexicalEvidence read={Fixtures.emptyDocx} />, { wrapper });
  const context = screen.getByRole('region', {
      name: 'knowledgeLocalLexical.context',
    }),
    members = within(context).getAllByText(
      'knowledgeLocalLexical.memberIdentity',
    );
  expect(members).toHaveLength(Fixtures.emptyDocx.context.typed_members.length);
  const empty = Fixtures.emptyDocx.context.typed_members.filter(
    (member) =>
      member.node.text.range?.start_byte === member.node.text.range?.end_byte,
  );
  expect(empty.some((member) => member.node.kind === 'cell')).toBe(true);
  expect(empty.some((member) => member.node.kind === 'paragraph')).toBe(true);
  expect(
    within(context).getAllByText(/knowledgeConsumption.text_empty/),
  ).toHaveLength(empty.length);
  expect(
    within(context).getByText('knowledgeLocalLexical.contextBoundary'),
  ).toBeInTheDocument();
  expect(
    within(context).getByText(/^knowledgeLocalLexical.membersComplete/),
  ).toBeInTheDocument();
});
test('未知索引写结果保留精确body和幂等键，重新挂载恢复后只显式重发原请求', async () => {
  const body = fresh(Fixtures.createBody);
  const create = jest.mocked(service.createJob);
  create.mockRejectedValueOnce(
    new LocalLexicalRequestError(
      0,
      'local_lexical_result_unknown',
      '',
      false,
      true,
    ),
  );
  const first = renderHook(() => useLocalLexicalActions(Scope), { wrapper });
  await waitFor(() => expect(first.result.current.storageReady).toBe(true));
  await act(async () => {
    await expect(
      first.result.current.submit({ operation: 'createIndex', body }),
    ).rejects.toMatchObject({ code: 'local_lexical_result_unknown' });
  });
  expect(first.result.current.pending?.body).toEqual(body);
  first.unmount();
  const next = renderHook(() => useLocalLexicalActions(Scope), { wrapper });
  await waitFor(() => expect(next.result.current.pending?.body).toEqual(body));
  await act(async () => {
    await expect(
      next.result.current.submit({
        operation: 'createIndex',
        body: { ...body, idempotency_key: 'different-key' },
      }),
    ).rejects.toMatchObject({ code: 'local_lexical_pending_request' });
  });
  await act(async () => {
    await next.result.current.resend();
  });
  expect(create.mock.calls[1][1]).toEqual(create.mock.calls[0][1]);
  expect(next.result.current.pending).toBeNull();
});
test('内容策略的未知写结果同样保存完整解析状态和原键；明确拒绝释放请求', async () => {
  const change = jest.mocked(contentService.changePolicy);
  change.mockRejectedValueOnce(new ContentRequestError(0, 'unavailable'));
  const body = {
    contract_version: '0.2.0-candidate' as const,
    request: {
      expected_policy_version: 1,
      expected_content_policy_version: 4,
      parse_enabled: false,
      lexical_enabled: true,
      confirmed: true as const,
      idempotency_key: 'content-lexical-fixed-key',
    },
  };
  const { result } = renderHook(() => useLocalLexicalActions(Scope), {
    wrapper,
  });
  await waitFor(() => expect(result.current.storageReady).toBe(true));
  await act(async () => {
    await expect(
      result.current.submit({ operation: 'changeContentPolicy', body }),
    ).rejects.toMatchObject({ code: 'unavailable' });
  });
  expect(result.current.pending).toEqual({
    operation: 'changeContentPolicy',
    body,
  });
  change.mockRejectedValueOnce(
    new ContentRequestError(409, 'content_policy_changed', '', true),
  );
  await act(async () => {
    await expect(result.current.resend()).rejects.toMatchObject({
      code: 'content_policy_changed',
    });
  });
  expect(result.current.pending).toBeNull();
  expect(change.mock.calls[1][1]).toEqual(body.request);
});
test('会话变化后的旧搜索结果拒绝回填，缓存键包含会话、所有者和域', async () => {
  let finish: ((value: LocalLexicalSearchData) => void) | undefined;
  jest.mocked(service.search).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result } = renderHook(() => useLocalLexicalActions(Scope), {
    wrapper,
  });
  const search = result.current.search(Fixtures.searchBody);
  Session.mockReturnValue(2);
  finish?.(fresh(Fixtures.search));
  await expect(search).rejects.toMatchObject({
    status: 401,
    code: 'unauthenticated',
  });
});
test('出站关闭模型字段、混域、重复来源和旧flat选择；合法选择顺序不影响固定集合键', () => {
  const body = Fixtures.searchBody;
  for (const invalid of [
    { ...body, llm_id: 'forbidden' },
    { ...body, selection: { kind: 'fixed_revisions', targets: Targets } },
    {
      ...body,
      selection: { kind: 'fixed_representations', targets: [C1, C1] },
    },
    {
      ...body,
      selection: {
        kind: 'fixed_representations',
        targets: [
          {
            ...C1,
            source_ref: { ...C1.source_ref, domain_id: 'f'.repeat(32) },
          },
        ],
      },
    },
  ])
    expect(() =>
      checkLocalLexicalRequest(
        'search',
        Scope,
        invalid as LocalLexicalSearchBody,
      ),
    ).toThrow();
  expect(
    localLexicalSelectionKey({ ...Selection, targets: [...Targets].reverse() }),
  ).toBe(localLexicalSelectionKey(Selection));
});
test.each([
  'version',
  'header',
  'request_id',
  'bytes',
  'context_budget',
  'row_binding',
] as const)('实际service拒绝被篡改的 %s', async (field) => {
  const read = fresh(Fixtures.reads[0]),
    body = {
      contract_version: '0.2.0-candidate' as const,
      evidence_ref: fresh(read.evidence_ref),
    };
  Http.mockImplementation(async (config) => {
    if (field === 'bytes') read.text += 'x';
    if (field === 'context_budget') read.context.metadata_byte_length++;
    if (field === 'row_binding')
      read.evidence_ref.address.target.source_ref.source_id = 'e'.repeat(32);
    const value = response(config, 'read', read);
    if (field === 'version')
      value.data = {
        ...(value.data as object),
        contract_version: '0.1.0-candidate',
      };
    if (field === 'header')
      value.headers['x-knowledge-content-lexical-protocol'] = 'wrong';
    if (field === 'request_id') value.headers['x-request-id'] = 'c'.repeat(32);
    return value;
  });
  await expect(Wire.read(Scope, body)).rejects.toMatchObject({
    code: 'local_lexical_invalid_response',
  });
});
test('严格解析拒绝重复字段与非score小数；只允许命中score的有限小数', () => {
  expect(() => parseContentJSON('{"data":{"x":1,"x":2}}', true)).toThrow();
  expect(() =>
    parseContentJSON('{"data":{"context":{"metadata_byte_length":1.5}}}', true),
  ).toThrow();
  expect(parseContentJSON('{"data":{"hits":[{"score":0.125}]}}', true)).toEqual(
    { data: { hits: [{ score: 0.125 }] } },
  );
  expect(() =>
    parseContentJSON('{"data":{"hits":[{"score":1e999}]}}', true),
  ).toThrow();
});
test('旧local策略仅保留实际总开关，新的job操作固定为POST与32位job_id', async () => {
  Http.mockImplementation(async (config) => {
    expect(config.method).toBe('post');
    expect(config.url).toBe(
      '/api/v1/datasets/' +
        Scope.domain +
        '/knowledge/content/lexical/jobs/get',
    );
    expect(JSON.parse(config.data)).toEqual({
      contract_version: '0.2.0-candidate',
      job_id: Fixtures.published.job_id,
    });
    return response(config, 'getIndexJob', Fixtures.published);
  });
  await expect(Wire.job(Scope, Fixtures.published.job_id)).resolves.toEqual(
    Fixtures.published,
  );
  await expect(Wire.job(Scope, 'f'.repeat(64))).rejects.toMatchObject({
    code: 'invalid_request',
  });
});
test.each(['created_at', 'updated_at'] as const)(
  '实际service拒绝无效日历日期 %s',
  async (field) => {
    const job = fresh(Fixtures.published);
    job[field] = '2026-02-31T01:00:00Z';
    Http.mockImplementation(async (config) =>
      response(config, 'getIndexJob', job),
    );
    await expect(Wire.job(Scope, job.job_id)).rejects.toMatchObject({
      code: 'local_lexical_invalid_response',
    });
  },
);
test('实际消费者从复现的owning DOCX上下文保留两个零长成员并核验回读', async () => {
  Http.mockImplementation(async (config) =>
    response(config, 'read', Fixtures.emptyDocx),
  );
  const result = await Wire.read(Scope, {
    contract_version: '0.2.0-candidate',
    evidence_ref: Fixtures.emptyDocx.evidence_ref,
  });
  expect(
    result.context.typed_members.filter(
      (member) => member.node.text.status === 'empty',
    ),
  ).toHaveLength(2);
  expect(result).toEqual(Fixtures.emptyDocx);
});

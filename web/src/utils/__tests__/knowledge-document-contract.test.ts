import Examples from '../../../../docs/knowledge-document-structure-v1.examples.json';
import '@/pages/dataset/knowledge-catalogue/__tests__/document-fixture';
import {
  validDocumentData,
  documentDigest,
  documentSha,
  documentBytes,
  parseDocumentJson,
  parseDocumentQuery,
} from '../knowledge-document-schema';
import { documentRequestDigest } from '../knowledge-document';
import { createHash } from 'node:crypto';

it.each(Examples.positive)('accepts frozen $name', async ({ schema, value }) =>
  expect(await validDocumentData(schema, value)).toBe(true),
);
it.each(Examples.negative)('rejects frozen $name', async ({ schema, value }) =>
  expect(await validDocumentData(schema, value)).toBe(false),
);
it.each(Object.entries(Examples.digest_vectors))(
  'matches frozen identity %s',
  async (_name, v) => expect(await documentDigest(v.projection)).toBe(v.sha256),
);
it.each(Examples.reading_vectors)('matches frozen UTF-8 $name', async (v) => {
  expect(documentBytes(v.text)).toBe(v.utf8_byte_length);
  expect(await documentSha(v.text)).toBe(v.sha256);
  expect(v.members.join(v.separator)).toBe(v.text);
});
it.each(Examples.raw_json_cases)('parses raw JSON $name', (v) => {
  if (v.accepted) expect(() => parseDocumentJson(v.text)).not.toThrow();
  else expect(() => parseDocumentJson(v.text)).toThrow();
});
it.each(Examples.query_cases)('parses query $name', async (v) => {
  if (v.accepted)
    await expect(parseDocumentQuery(v.schema, v.query)).resolves.toBeDefined();
  else await expect(parseDocumentQuery(v.schema, v.query)).rejects.toThrow();
});
it('constructs all three request projections without including keys', async () => {
  const s = {
    owner: Examples.fixtures.owner_id,
    domain: Examples.fixtures.domain_id,
  };
  for (const [format, tree] of Object.entries(Examples.fixtures.trees)) {
    expect(
      await documentRequestDigest(
        'task',
        s,
        tree.task_request as import('@/interfaces/request/knowledge-document').DocumentTaskRequest,
      ),
    ).toBe(
      Examples.digest_vectors[('task_request_' + format) as 'task_request_pdf']
        .sha256,
    );
  }
  expect(
    await documentRequestDigest(
      'policy',
      s,
      Examples.fixtures
        .policy_request as import('@/interfaces/request/knowledge-document').DocumentPolicyChangeRequest,
    ),
  ).toBe(Examples.digest_vectors.policy_request.sha256);
  expect(
    await documentRequestDigest(
      'cancel',
      { ...s, task: Examples.fixtures.cancel_receipt.target.task_id },
      Examples.fixtures
        .cancel_request as import('@/interfaces/request/knowledge-document').DocumentCancelRequest,
    ),
  ).toBe(Examples.digest_vectors.cancel_request.sha256);
});
it.each(Examples.semantic_cases)(
  'consumes semantic rule $name within client scope',
  async (rule) => {
    const trees = Object.values(Examples.fixtures.trees);
    switch (rule.name) {
      case 'no_root_id_in_representation_projection':
        for (const format of ['pdf', 'pptx', 'docx'] as const)
          expect(
            Examples.digest_vectors[
              ('representation_' + format) as 'representation_pdf'
            ].projection,
          ).not.toHaveProperty('root_document_node_id');
        break;
      case 'manifest_final_preorder_no_derived_ids':
        for (const tree of trees) {
          expect(tree.manifest.nodes).toEqual(
            tree.nodes.map((n) => ({
              natural_ref: { kind: n.payload.kind, locator: n.payload.locator },
              node_payload_sha256: n.node_payload_sha256,
            })),
          );
          for (const entry of tree.manifest.nodes)
            expect(Object.keys(entry).sort()).toEqual([
              'natural_ref',
              'node_payload_sha256',
            ]);
        }
        break;
      case 'payload_parent_ref_and_ordinal_tree':
        for (const tree of trees)
          for (const n of tree.nodes) {
            if (n.parent_id === null) expect(n.payload.parent_ref).toBeNull();
            else {
              const parent = tree.nodes.find(
                (p) => p.document_node_id === n.parent_id,
              )!;
              expect(n.payload.parent_ref).toEqual({
                kind: parent.payload.kind,
                locator: parent.payload.locator,
              });
              const siblings = tree.nodes.filter(
                (p) => p.parent_id === n.parent_id,
              );
              expect(n.payload.ordinal).toBe(siblings.indexOf(n));
            }
          }
        break;
      case 'all_ranges_and_fragments_match_utf8':
        for (const tree of trees) {
          const bytes = Buffer.from(tree.reading_text, 'utf8');
          for (const node of tree.nodes)
            for (const range of [
              node.payload.reading_range,
              node.payload.own_reading_range,
            ])
              if (range)
                expect(
                  createHash('sha256')
                    .update(bytes.subarray(range.start_byte, range.end_byte))
                    .digest('hex'),
                ).toBe(range.text_sha256);
        }
        break;
      case 'passages_cover_all_reading_bytes_once':
        for (const tree of trees) {
          const passages = tree.nodes
            .filter((n) => n.payload.kind === 'passage')
            .sort(
              (a, b) =>
                a.payload.reading_range.start_byte -
                b.payload.reading_range.start_byte,
            );
          let end = 0;
          for (const n of passages) {
            expect(n.payload.reading_range.start_byte).toBe(end);
            end = n.payload.reading_range.end_byte;
          }
          expect(end).toBe(Buffer.byteLength(tree.reading_text));
        }
        break;
      case 'source_and_revision_isolate_empty_nodes': {
        const nodes = Examples.fixtures.trees.pdf.nodes.filter(
          (n) =>
            n.payload.kind === 'page' &&
            n.payload.reading_range.start_byte ===
              n.payload.reading_range.end_byte,
        );
        expect(nodes.length).toBeGreaterThan(0);
        expect(
          new Set(
            Examples.fixtures.trees.pdf.nodes.map((n) => n.document_node_id),
          ).size,
        ).toBe(Examples.fixtures.trees.pdf.nodes.length);
        expect(
          new Set(trees.map((t) => t.representation.representation_id)).size,
        ).toBe(3);
        break;
      }
      case 'policy_cas_before_noop': {
        const receipt = JSON.parse(
          JSON.stringify(Examples.fixtures.policy_receipt),
        );
        receipt.before.document_policy_version++;
        expect(await validDocumentData('PolicyReceipt', receipt)).toBe(false);
        break;
      }
      case 'full_rejection_required_to_abandon_slot':
        expect(
          await validDocumentData('OperationRecord', {
            outcome_known: true,
            rejection_id: 'a'.repeat(32),
          }),
        ).toBe(false);
        break;
      default:
        throw Error('Unconsumed semantic rule ' + rule.name);
    }
  },
);

import Ajv from 'ajv/dist/2020';
import Contract from '../../../docs/knowledge-foundation-v1.openapi.json';

// Consume A's frozen schema directly instead of maintaining a second contract.
const Validator = new Ajv({ strict: false, validateFormats: false });
const Schemas = Object.fromEntries(
  Object.keys(Contract.components.schemas).map((name) => [
    name,
    Validator.compile({
      components: Contract.components,
      $ref: `#/components/schemas/${name}`,
    }),
  ]),
);

export function validKnowledgeData(schema: string, data: unknown): boolean {
  return !!Schemas[schema]?.(data);
}

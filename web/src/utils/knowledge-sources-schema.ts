import Ajv from 'ajv/dist/2020';
import addFormats from 'ajv-formats';
import Contract from '../../../docs/knowledge-sources-v1.openapi.json';

const Validator = new Ajv({ strict: false });
addFormats(Validator);
const Schemas = Object.fromEntries(
  Object.keys(Contract.components.schemas).map((name) => [
    name,
    Validator.compile({
      components: Contract.components,
      $ref: `#/components/schemas/${name}`,
    }),
  ]),
);

export function validKnowledgeSourceData(schema: string, data: unknown) {
  return !!Schemas[schema]?.(data);
}

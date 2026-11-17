import { en } from '../i18n/locales/en';
import {
  createFieldSchema,
  getArrayItemsSchema,
  removeObjectProperty,
} from '../lib/schema-editor';
import { buildValidationTree, validateSchemaByType } from '../types/validation';
import { validateJson } from '../utils/json-validator';

describe('schema editor validation', () => {
  it('accepts one-sided bounds and rejects reversed bounds', () => {
    expect(
      validateSchemaByType({ exclusiveMinimum: 2 }, 'number', en).success,
    ).toBe(true);
    expect(
      validateSchemaByType({ minimum: 8, maximum: 2 }, 'number', en).success,
    ).toBe(false);
    expect(
      validateSchemaByType({ minLength: 8, maxLength: 2 }, 'string', en)
        .success,
    ).toBe(false);
  });

  it('validates boolean and undeclared schemas without inventing a type', () => {
    expect(buildValidationTree(true, en).cumulativeChildrenErrors).toBe(0);
    expect(buildValidationTree(false, en).cumulativeChildrenErrors).toBe(1);
    expect(buildValidationTree({}, en).validation.success).toBe(true);
    expect(
      buildValidationTree({ type: 'object', properties: { denied: false } }, en)
        .cumulativeChildrenErrors,
    ).toBe(1);
  });

  it('creates a typed field when no validation settings are supplied', () => {
    expect(
      createFieldSchema({
        name: 'title',
        type: 'string',
        description: 'A title',
        required: false,
      }),
    ).toEqual({ type: 'string', description: 'A title' });
  });

  it('preserves false array items and does not mutate source properties', () => {
    expect(getArrayItemsSchema({ type: 'array', items: false })).toBe(false);
    const source = { type: 'object' as const, properties: { keep: true } };
    expect(removeObjectProperty(source, 'keep').properties).toEqual({});
    expect(source.properties).toEqual({ keep: true });
  });

  it('always returns an error collection for valid and invalid JSON', () => {
    expect(validateJson('{}', true)).toEqual({ valid: true, errors: [] });
    expect(validateJson('{}', false).errors.length).toBeGreaterThan(0);
    expect(validateJson('{', true).errors[0].path).toBe('/');
  });
});

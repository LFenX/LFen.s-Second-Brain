import type { ComponentParams } from '../../interface';
import { editorValue, rowText } from './value';

function params(key: string, value: unknown): ComponentParams {
  return {
    debug_inputs: {},
    delay_after_error: 0,
    description: '',
    exception_default_value: null,
    exception_goto: null,
    exception_method: null,
    inputs: {},
    field_name: '',
    max_retries: 0,
    message_history_window_size: 0,
    persist_logs: false,
    timeout: 0,
    outputs: {
      _created_time: {},
      _elapsed_time: {},
      name: {},
      output_format: { type: 'string', value: key },
      json: { type: 'string', value: '' },
      [key]: { type: Array.isArray(value) ? 'array' : 'string', value },
    },
  };
}

describe('parser editor values', () => {
  it.each(['text', 'html', 'markdown'])(
    'preserves %s string content and parameters',
    (key) => {
      const input = params(key, 'first\nsecond');
      expect(editorValue(input)).toEqual({
        key,
        type: 'string',
        value: 'first\nsecond',
        params: input,
      });
    },
  );

  it.each(['json', 'chunks'])(
    'preserves %s rows including numeric and structured metadata',
    (key) => {
      const rows = [{ text: 'content', score: 0.7, positions: [[1, 2]] }];
      const input = params(key, rows);
      expect(editorValue(input)).toEqual({
        key,
        type: 'array',
        value: rows,
        params: input,
      });
      expect(rows[0].positions).toEqual([[1, 2]]);
    },
  );

  it.each([null, 'wrong shape', [null], ['wrong row'], [[]]])(
    'rejects malformed row content %j',
    (value) => {
      expect(editorValue(params('chunks', value))).toBeNull();
    },
  );

  it('rejects an unsupported format and renders only text or numbers from a row', () => {
    expect(editorValue(params('other', 'content'))).toBeNull();
    expect(editorValue(params('text', []))).toBeNull();
    expect(rowText({ text: 'content' }, 'text')).toBe('content');
    expect(rowText({ text: 0 }, 'text')).toBe('0');
    expect(rowText({ text: { nested: true } }, 'text')).toBe('');
    expect(rowText({}, 'text')).toBe('');
  });
});

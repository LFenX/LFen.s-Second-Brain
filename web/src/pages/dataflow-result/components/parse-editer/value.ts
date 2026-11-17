import type { ComponentParams } from '../../interface';
import type { FormatPreserveEditorProps } from './interface';

function isRows(value: unknown): value is Record<string, unknown>[] {
  return (
    Array.isArray(value) &&
    value.every(
      (row) => row !== null && typeof row === 'object' && !Array.isArray(row),
    )
  );
}

export function editorValue(
  params: ComponentParams,
): FormatPreserveEditorProps['initialValue'] | null {
  const outputs = params.outputs;
  const key = outputs?.output_format?.value;
  if (!outputs || !key) return null;
  const output: unknown = outputs[key as keyof typeof outputs];
  if (!output || typeof output !== 'object' || !('value' in output))
    return null;
  const type =
    'type' in output && typeof output.type === 'string' ? output.type : '';
  if ((key === 'json' || key === 'chunks') && isRows(output.value)) {
    return { key, type, value: output.value, params };
  }
  if (
    (key === 'text' || key === 'html' || key === 'markdown') &&
    typeof output.value === 'string'
  ) {
    return { key, type, value: output.value, params };
  }
  return null;
}

export function rowText(row: Record<string, unknown>, key: string): string {
  const value = row[key];
  return typeof value === 'string' || typeof value === 'number'
    ? String(value)
    : '';
}

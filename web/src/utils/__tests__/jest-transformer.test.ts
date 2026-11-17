/** @jest-environment node */

import path from 'node:path';
import { runInNewContext } from 'node:vm';

const transformer =
  require('../../../jest-esbuild-transformer.cjs').createTransformer();

function execute(source: string) {
  const events: string[] = [];
  const mocks = new Map<string, unknown>();
  const jestObject = {
    mock(id: string, factory: () => unknown) {
      events.push(`mock:${id}`);
      mocks.set(id, factory());
    },
  };
  const options = {
    config: { cwd: process.cwd(), rootDir: process.cwd() },
    configString: '{}',
    instrument: false,
    cacheFS: new Map(),
    supportsStaticESM: false,
    supportsDynamicImport: false,
    supportsTopLevelAwait: false,
  };
  const { code }: { code: string } = transformer.process(
    source,
    path.join(process.cwd(), 'synthetic-transform.fixture.tsx'),
    options,
  );
  const module = { exports: {} as Record<string, unknown> };
  runInNewContext(code, {
    module,
    exports: module.exports,
    jest: jestObject,
    require: (id: string) => {
      if (id === '@jest/globals') return { jest: jestObject };
      events.push(`load:${id}`);
      if (!mocks.has(id)) throw new Error(`Unexpected module: ${id}`);
      return mocks.get(id);
    },
    React: {
      createElement: (
        type: string,
        props: unknown,
        ...children: unknown[]
      ) => ({
        type,
        props,
        children,
      }),
    },
  });
  return { exports: module.exports, events };
}

it('erases imported types, hoists mocks before imports, and defines Vite environment values', () => {
  const output = execute(`
    import type { ReactNode } from 'react';
    import { value } from 'synthetic-module';
    jest.mock('synthetic-module', () => ({ value: 'mocked' }));
    export const result: ReactNode = value;
    export const environment = import.meta.env;
  `);
  expect(output.events).toEqual([
    'mock:synthetic-module',
    'load:synthetic-module',
  ]);
  expect(output.exports.result).toBe('mocked');
  expect(output.exports.environment).toEqual({});
});

it('preserves JSX inside typed mock factories until mock hoisting finishes', () => {
  const output = execute(`
    import type { PropsWithChildren } from 'react';
    import { view } from 'synthetic-view';
    jest.mock('synthetic-view', () => ({
      view: ({ children }: PropsWithChildren) => <span>{children}</span>,
    }));
    export const result = view({ children: 'kept' });
  `);
  expect(output.events).toEqual(['mock:synthetic-view', 'load:synthetic-view']);
  expect(output.exports.result).toMatchObject({
    type: 'span',
    children: ['kept'],
  });
});

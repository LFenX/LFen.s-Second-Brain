// Jest transformer wrapping esbuild with the same options esbuild-jest was
// configured with, plus `define: { 'import.meta.env': '{}' }` — esbuild-jest@0.5
// does not forward esbuild's `define` option, and source files read Vite's
// import.meta.env at module scope, which crashes under jest's cjs runtime.
// Erase TypeScript before Babel rewrites modules for mock hoisting. Keep JSX
// intact until hoisting so JSX factories do not become out-of-scope bindings.
const path = require('node:path');
const esbuild = require('esbuild');
const esbuildJest = require('esbuild-jest');

const esbuildJestTransformer = esbuildJest.createTransformer({
  sourcemap: true,
});

const supportedLoaders = ['js', 'jsx', 'ts', 'tsx', 'json'];

module.exports = {
  createTransformer() {
    return {
      process(content, filename, config, opts) {
        const ext = path.extname(filename).slice(1);
        const loader = supportedLoaders.includes(ext) ? ext : 'text';
        const define = {
          'import.meta.env': '{}',
          'import.meta.glob': 'jestImportMetaGlob',
        };
        if (content.indexOf('ock(') >= 0) {
          const prepared = esbuild.transformSync(content, {
            loader,
            jsx: 'preserve',
            target: 'es2018',
            sourcemap: 'inline',
            sourcesContent: false,
            sourcefile: filename,
            define,
          });
          return esbuildJestTransformer.process(
            prepared.code,
            filename,
            config,
            opts,
          );
        }
        const result = esbuild.transformSync(content, {
          loader,
          format: 'cjs',
          target: 'es2018',
          sourcemap: true,
          sourcesContent: false,
          sourcefile: filename,
          define,
        });
        return { code: result.code, map: JSON.parse(result.map) };
      },
    };
  },
};

// Synthetic-only C2 verification. Uses the existing API; never starts ingestion or models.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const args = process.argv.slice(2);
if (args.includes('--help')) {
  console.log('LFEN_POC_AUTH_FILE=<ignored JSON with authorization> LFEN_POC_OUTPUT=<new ignored directory> node tools/scripts/knowledge_import_poc.mjs --mode smoke|concurrency|boundaries|performance');
  process.exit(0);
}
const mode = args[0] === '--mode' ? args[1] : 'smoke';
assert.ok(['smoke', 'concurrency', 'boundaries', 'performance'].includes(mode), 'Unsupported mode');
assert.equal(process.env.LFEN_POC_SYNTHETIC_ONLY, '1', 'Set LFEN_POC_SYNTHETIC_ONLY=1 explicitly');
const origin = new URL(process.env.LFEN_POC_BASE_URL || 'http://127.0.0.1:18080');
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(origin.hostname), 'Only loopback endpoints are allowed');
assert.ok(['http:', 'https:'].includes(origin.protocol) && !origin.username && !origin.password && origin.pathname === '/' && !origin.search && !origin.hash, 'Use a loopback origin without credentials or paths');
assert.ok(process.env.LFEN_POC_AUTH_FILE, 'LFEN_POC_AUTH_FILE is required');
const auth = JSON.parse(fs.readFileSync(process.env.LFEN_POC_AUTH_FILE, 'utf8'));
assert.ok(typeof auth.authorization === 'string' && auth.authorization.length > 0, 'Missing authorization in private file');
const output = path.resolve(process.env.LFEN_POC_OUTPUT || path.join(root, '.local-data', `c2-${mode}-${Date.now()}`));
const localRoot = path.join(root, '.local-data');
assert.ok(output.startsWith(localRoot + path.sep), 'Evidence must stay under ignored .local-data');
assert.ok(!fs.existsSync(output), 'Use a new evidence directory for each run');
fs.mkdirSync(output, { recursive: true, mode: 0o700 });

const require = createRequire(path.join(root, 'web/package.json'));
const Ajv = require('ajv');
const formats = require('ajv-formats');
const specBytes = fs.readFileSync(path.join(root, 'docs/knowledge-sources-v1.openapi.json'));
const spec = JSON.parse(specBytes);
const ajv = new Ajv({ strict: true, allErrors: true });
formats(ajv);
ajv.addSchema({ $id: 'urn:lfen:c2-poc', $defs: JSON.parse(JSON.stringify(spec.components.schemas).replaceAll('#/components/schemas/', '#/$defs/')) });
const validators = Object.fromEntries(Object.keys(spec.components.schemas).map(name => [name, ajv.getSchema(`urn:lfen:c2-poc#/$defs/${name}`)]));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const started = Date.now();
const checks = [];
const state = { mode, synthetic_only: true, contract_version: '0.2.0', contract_sha256: sha(specBytes), source_commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(), started_at: new Date().toISOString(), origin: origin.origin, requests: 0, model_endpoints_allowed: false, checks };
const save = () => fs.writeFileSync(path.join(output, 'result.json'), JSON.stringify(state, null, 2) + '\n');
const memory = [];
const sampleMemory = () => memory.push({ at: Date.now(), ...process.memoryUsage() });
sampleMemory();
const timer = setInterval(sampleMemory, 250);
timer.unref();

async function call(label, method, route, body, expected = 200, schema, options = {}) {
  assert.ok(Date.now() - started < 20 * 60 * 1000 && state.requests < 1800, 'Run budget exceeded');
  assert.ok(route === '/datasets' || route.startsWith('/datasets/') || route === '/knowledge/contract' || route === '/knowledge/import-contract', 'Unexpected endpoint');
  assert.ok(!/parse|chat|completion|embedding|rerank|agent|memory|ingestion/i.test(route), 'Model/ingestion endpoint prohibited');
  state.requests++;
  const headers = { 'X-Knowledge-Contract-Version': '1.0.0', 'X-Knowledge-Source-Contract-Version': '0.2.0' };
  if (!options.anonymous) headers.Authorization = auth.authorization;
  if (options.sourceVersion === null) delete headers['X-Knowledge-Source-Contract-Version'];
  let payload;
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = Buffer.isBuffer(body) ? body : JSON.stringify(body);
  }
  const start = performance.now();
  const response = await fetch(new URL('/api/v1' + route, origin), { method, headers, body: payload, redirect: 'error', signal: AbortSignal.timeout(30000) });
  const raw = await response.text();
  const duration = performance.now() - start;
  fs.appendFileSync(path.join(output, 'http.jsonl'), JSON.stringify({ label, method, path: route, status: response.status, duration_ms: duration, response_bytes: Buffer.byteLength(raw), request_bytes: payload === undefined ? 0 : Buffer.byteLength(payload) }) + '\n');
  assert.ok((Array.isArray(expected) ? expected : [expected]).includes(response.status), `${label}: HTTP ${response.status}, expected ${expected}`);
  const value = JSON.parse(raw);
  const schemaName = typeof schema === 'function' ? schema(response.status) : schema;
  if (schemaName) assert.ok(validators[schemaName](value), `${label}: response does not match ${schemaName}: ${ajv.errorsText(validators[schemaName].errors)}`);
  if (response.ok) assert.equal(value.code, 0, `${label}: unsuccessful envelope`);
  return { data: value.data, status: response.status, duration_ms: duration };
}

const checked = (name, actual) => { checks.push({ name, status: 'Passed', actual }); save(); };
const fixtures = () => [
  { relative_path: 'web/card.tsx', component_side: 'frontend', language: 'tsx', text: '// 合成前端\r\nexport const Card = () => "你好 🧠";\r\n' },
  { relative_path: 'backend/answer.go', component_side: 'backend', language: 'go', text: 'package synthetic\nfunc Answer() int { return 42 }\n' },
  { relative_path: 'shared/types.ts', component_side: 'shared', language: 'typescript', text: 'export type SyntheticID = string;\n' },
  { relative_path: 'cli/main.go', component_side: 'cli', language: 'go', text: 'package main\nfunc main() {}\n' },
  { relative_path: 'docs/note.md', component_side: 'non_code', language: 'markdown', text: '# 合成资料\n\n只有一句话。\n' },
  { relative_path: 'docs/empty.md', component_side: 'non_code', language: 'markdown', text: '' },
  { relative_path: 'docs/bom.txt', component_side: 'non_code', language: 'text', text: '\uFEFF合成 BOM 文本\r\n' },
  { relative_path: 'docs/large.txt', component_side: 'non_code', language: 'text', text: 'a'.repeat(65535) + '中🧠\n' + 'x'.repeat(4096) }
].map(f => ({ ...f, group_id: null }));

async function createDomain() {
  const dataset = (await call('synthetic-dataset', 'POST', '/datasets', { name: `Synthetic-C2-${mode}-${randomUUID().slice(0, 8)}`, permission: 'me', parse_type: 1, parser_id: 'general' })).data;
  state.domain_id = dataset.id;
  save();
  await call('register-domain', 'PUT', `/datasets/${dataset.id}/knowledge/domain`, { category: 'project' });
  return dataset.id;
}
async function register(domain, paths, key = randomUUID().replaceAll('-', '')) {
  const body = { registration_key: key, name: '合成验证来源', source_kind: 'text_file', input_mode: 'inline_text', include_paths: paths, group_id: null, component_side: 'shared', language: '' };
  return (await call('registration', 'POST', `/datasets/${domain}/knowledge/registrations`, body, 201, 'RegistrationResponse')).data;
}
const prefix = (domain, reg) => `/datasets/${domain}/knowledge/registrations/${reg.registration_id}`;
const preview = async (domain, reg, fixture) => (await call('preview', 'POST', prefix(domain, reg) + '/previews', fixture, 201, 'PreviewResponse')).data;
async function commit(domain, reg, p, key = randomUUID().replaceAll('-', ''), expected = 201) {
  return call('commit', 'POST', prefix(domain, reg) + '/imports', { preview_id: p.preview_id, manifest_sha256: p.manifest_sha256, idempotency_key: key }, expected, status => status >= 400 ? 'Error' : 'ReceiptResponse');
}
function saved(receipt) {
  assert.equal(receipt.status, 'succeeded');
  assert.equal(receipt.model_calls, 0);
  assert.equal(receipt.items.length, 1);
  const item = receipt.items[0];
  for (const [key, value] of Object.entries({ catalogue_status: 'ready', structure_status: 'deferred', full_text_status: 'disabled', vector_status: 'disabled' })) assert.equal(item[key], value);
  assert.match(item.source_id, /^[a-f0-9]{32}$/);
  return item;
}
async function readExact(domain, item, text) {
  const bytes = Buffer.from(text);
  const pieces = [];
  let offset = 0;
  do {
    let end = Math.min(offset + 65536, bytes.length);
    while (end < bytes.length && end > offset && (bytes[end] & 0xc0) === 0x80) end--;
    const result = await call('C1-fixed-read', 'GET', `/datasets/${domain}/knowledge/nodes/${item.root_node_id}/content?start_byte=${offset}&end_byte=${end}`);
    assert.equal(result.data.source_revision_id, item.source_revision_id);
    assert.equal(result.data.source_id, item.source_id);
    assert.equal(result.data.start_byte, offset);
    assert.equal(result.data.end_byte, end);
    pieces.push(Buffer.from(result.data.text));
    offset = end;
  } while (offset < bytes.length);
  assert.deepEqual(Buffer.concat(pieces), bytes, 'Original UTF-8 bytes changed');
  assert.equal(sha(Buffer.concat(pieces)), item.sha256);
}

async function smoke() {
  const cap = (await call('C2-capabilities', 'GET', '/knowledge/import-contract', undefined, 200, 'CapabilitiesResponse')).data;
  assert.equal(cap.status, 'Frozen');
  assert.equal(cap.processing.external_models, false);
  await call('missing-version', 'GET', '/knowledge/import-contract', undefined, 409, 'Error', { sourceVersion: null });
  await call('anonymous', 'GET', '/knowledge/import-contract', undefined, 401, 'NativeUnauthorized', { anonymous: true });
  const domain = await createDomain();
  const samples = fixtures();
  const reg = await register(domain, samples.map(f => f.relative_path));
  state.registration_id = reg.registration_id;
  state.samples = [];
  for (const sample of samples) {
    const p = await preview(domain, reg, sample);
    const item = saved((await commit(domain, reg, p)).data);
    assert.equal(item.component_side, sample.component_side);
    await readExact(domain, item, sample.text);
    state.samples.push({ path: sample.relative_path, bytes: Buffer.byteLength(sample.text), sha256: sha(sample.text), ...item });
    checked('UTF-8/' + sample.relative_path, '原字节、组件标签、固定修订和禁索引状态一致');
  }
  const first = state.samples[0];
  const same = saved((await commit(domain, reg, await preview(domain, reg, samples[0]))).data);
  assert.equal(same.save_status, 'unchanged');
  assert.equal(same.source_revision_id, first.source_revision_id);
  const updated = { ...samples[0], text: samples[0].text + '// 合成更新\n' };
  const item = saved((await commit(domain, reg, await preview(domain, reg, updated))).data);
  assert.equal(item.source_id, first.source_id);
  assert.notEqual(item.source_revision_id, first.source_revision_id);
  await readExact(domain, first, samples[0].text);
  await readExact(domain, item, updated.text);
  checked('unchanged-update-history', '未变不发布，更新保持来源ID，新旧版本字节均可读');
}

async function concurrency() {
  const domain = await createDomain();
  const paths = ['web/card.tsx'];
  const reg = await register(domain, paths);
  const body = { registration_key: reg.registration_key, name: reg.name, source_kind: reg.source_kind, input_mode: reg.input_mode, include_paths: paths, group_id: null, component_side: 'shared', language: '' };
  const registrations = await Promise.all(Array.from({ length: 5 }, () => call('parallel-registration-replay', 'POST', `/datasets/${domain}/knowledge/registrations`, body, 200, 'RegistrationResponse')));
  assert.ok(registrations.every(r => r.data.registration_id === reg.registration_id));
  const sample = fixtures()[0];
  const p = await preview(domain, reg, sample);
  const key = randomUUID().replaceAll('-', '');
  const attempts = await Promise.all(Array.from({ length: 5 }, () => commit(domain, reg, p, key, [200, 201])));
  assert.equal(attempts.filter(r => r.status === 201).length, 1);
  assert.equal(new Set(attempts.map(r => r.data.import_task_id)).size, 1);
  const first = saved(attempts[0].data);
  const competitors = await Promise.all(['left', 'right'].map(suffix => preview(domain, reg, { ...sample, text: sample.text + suffix })));
  const raced = await Promise.all(competitors.map(p => commit(domain, reg, p, randomUUID().replaceAll('-', ''), [201, 409])));
  assert.deepEqual(raced.map(r => r.status).sort(), [201, 409]);
  const latest = saved(raced.find(r => r.status === 201).data);
  assert.equal(latest.source_id, first.source_id);
  await readExact(domain, first, sample.text);
  checked('parallel-registration-submit-update', '5路登记回放、5路同键一次发布、两不同更新一成功一冲突；真实MySQL来源计数另由Go集成测试验证');
}

async function boundaries() {
  const domain = await createDomain();
  const reg = await register(domain, ['docs/limit.txt', 'web/card.tsx', 'docs/unsupported.pdf']);
  const sample = { relative_path: 'docs/limit.txt', text: 'x'.repeat(2 * 1024 * 1024), group_id: null, component_side: 'non_code', language: 'text' };
  const item = saved((await commit(domain, reg, await preview(domain, reg, sample))).data);
  await readExact(domain, item, sample.text);
  for (const [label, fixture, status] of [
    ['one-byte-over', { ...sample, text: sample.text + 'x' }, 413],
    ['NUL', { ...sample, text: 'x\0' }, 415],
    ['binary-extension', { ...sample, relative_path: 'docs/unsupported.pdf', text: '%PDF-synthetic' }, 415],
    ['private-key-marker', { ...sample, text: '-----BEGIN PRIVATE KEY-----\nsynthetic' }, 415],
    ['unknown-model-opt-in', { ...sample, text: 'synthetic', external_models: true }, 400]
  ]) await call(label, 'POST', prefix(domain, reg) + '/previews', fixture, status, 'Error');
  for (const relative_path of ['../escape.txt', '/tmp/x.txt', 'C:/x.txt', '\\\\host\\share\\x.txt', 'web/../x.txt', '.env.local', '.git/config', 'node_modules/x.js', 'docs/a.key']) {
    await call('path-boundary', 'POST', prefix(domain, reg) + '/previews', { ...sample, text: 'synthetic', relative_path }, 400, 'Error');
  }
  const invalidJSON = Buffer.concat([Buffer.from('{"relative_path":"docs/limit.txt","text":"'), Buffer.from([255]), Buffer.from('","group_id":null,"component_side":"non_code","language":"text"}')]);
  await call('invalid-UTF8-JSON', 'POST', prefix(domain, reg) + '/previews', invalidJSON, 400, 'Error');
  checked('input-boundaries', '2MiB精确分段回读；超限、路径、格式、非法UTF-8与模型放行字段拒绝');
}

async function performanceRun() {
  const domain = await createDomain();
  state.performance = [];
  const sizes = [1024, 65536, 512 * 1024, 2 * 1024 * 1024];
  for (const size of sizes) {
    for (const parallel of [1, 3]) {
      const rows = { preview: [], create: [], unchanged: [], read_64k_or_less: [] };
      const run = async index => {
        const reg = await register(domain, ['docs/sample.txt']);
        const text = 'x'.repeat(size);
        const sample = { relative_path: 'docs/sample.txt', text, group_id: null, component_side: 'non_code', language: 'text' };
        const p = await call('perf-preview', 'POST', prefix(domain, reg) + '/previews', sample, 201, 'PreviewResponse');
        const created = await commit(domain, reg, p.data);
        const item = saved(created.data);
        const unchanged = await commit(domain, reg, await preview(domain, reg, sample));
        const read = await call('perf-read', 'GET', `/datasets/${domain}/knowledge/nodes/${item.root_node_id}/content?start_byte=0&end_byte=${Math.min(65536, size)}`);
        assert.equal(unchanged.data.items[0].save_status, 'unchanged');
        assert.equal(read.data.text, text.slice(0, Math.min(65536, size)));
        if (index >= 0) {
          rows.preview.push(p.duration_ms); rows.create.push(created.duration_ms); rows.unchanged.push(unchanged.duration_ms); rows.read_64k_or_less.push(read.duration_ms);
        }
      };
      await run(-1); // Exclude warm-up, registration, and prerequisite requests from timing series.
      for (let i = 0; i < 20; i += parallel) await Promise.all(Array.from({ length: Math.min(parallel, 20 - i) }, (_, j) => run(i + j)));
      for (const [operation, times] of Object.entries(rows)) {
        times.sort((a, b) => a - b);
        state.performance.push({ source_bytes: size, concurrency: parallel, scope: 'same_domain_different_registrations', operation, n: times.length, successes: times.length, median_ms: (times[9] + times[10]) / 2, min_ms: times[0], max_ms: times.at(-1), mean_ms: times.reduce((a, b) => a + b, 0) / times.length, p95_sla_claim: false });
      }
      checked(`performance-${size}-${parallel}`, '每操作20次，预热剔除；单域锁竞争属于实测负载，未宣称p95达标');
    }
  }
}

try {
  save();
  await ({ smoke, concurrency, boundaries, performance: performanceRun })[mode]();
  state.status = 'Passed';
} catch (error) {
  state.status = 'Failed';
  // Do not echo a server response or a credential-bearing fetch exception.
  state.failure = error instanceof assert.AssertionError ? error.message : `Execution failed (${error.name || 'Error'}); inspect local service health`;
  process.exitCode = 1;
} finally {
  clearInterval(timer);
  sampleMemory();
  state.finished_at = new Date().toISOString();
  state.elapsed_ms = Date.now() - started;
  state.client_memory = { peak_rss_bytes: Math.max(...memory.map(x => x.rss)), peak_heap_used_bytes: Math.max(...memory.map(x => x.heapUsed)), scope: 'Node verification client, not backend' };
  fs.writeFileSync(path.join(output, 'client-memory.json'), JSON.stringify(memory) + '\n');
  save();
  console.log(JSON.stringify({ mode, status: state.status, requests: state.requests, checks: checks.length, evidence: path.relative(root, output), ...(state.failure ? { failure: state.failure } : {}) }));
}

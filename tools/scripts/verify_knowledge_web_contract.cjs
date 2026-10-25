// Contract-only validation. This script does not import application code or make network requests.
// Requires the repository's existing web/node_modules (Ajv 2020 and ajv-formats).
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const cp = require('node:child_process');
const assert = require('node:assert/strict');
const Ajv = require('../../web/node_modules/ajv/dist/2020.js').default;
const addFormats = require('../../web/node_modules/ajv-formats').default;

const root = path.resolve(__dirname, '../..');
const base = 'docs/knowledge-web-import-v0.6.candidate';
const read = name => fs.readFileSync(path.join(root, name));
const api = JSON.parse(read(base + '.openapi.json'));
const examples = JSON.parse(read(base + '.examples.json'));
const sha = value => crypto.createHash('sha256').update(value).digest('hex');
const results = {schema: [], structure: [], semantic: [], digests: [], fingerprint: [], endpoints: [], protection: [], scope: []};
const record = (group, name, passed, detail) => results[group].push({name, passed: !!passed, ...(detail ? {detail} : {})});
const rewrite = value => JSON.parse(JSON.stringify(value).replaceAll('#/components/schemas/', '#/$defs/'));
const ajv = new Ajv({strict: true, strictTypes: false, allErrors: true});
addFormats(ajv);
const validators = {};
for (const [name, schema] of Object.entries(api.components.schemas)) {
  validators[name] = ajv.compile({...rewrite(schema), $defs: rewrite(api.components.schemas)});
  record('schema', name, true);
}
for (const item of examples.structure_cases) {
  const valid = validators[item.schema](item.value);
  record('structure', item.name, valid === item.valid, valid !== item.valid ? validators[item.schema].errors : undefined);
}

// Independent string-token encoder; the fixture producer uses byte-buffer recursion.
function tokens(value) {
  if (value === null) return ['n'];
  if (typeof value === 'string') return ['s', value];
  if (typeof value === 'boolean') return ['b', value ? '1' : '0'];
  if (typeof value === 'number' && Number.isSafeInteger(value)) return ['i', String(value)];
  if (Array.isArray(value)) return ['a', String(value.length), ...value.flatMap(tokens)];
  assert.ok(value && Object.getPrototypeOf(value) === Object.prototype, 'unsupported typed-LP object');
  const keys = Object.keys(value).sort((a, b) => Buffer.compare(Buffer.from(a), Buffer.from(b)));
  return ['o', String(keys.length), ...keys.flatMap(key => [key, ...tokens(value[key])])];
}
const tokenBytes = values => Buffer.from(values.map(s => Buffer.byteLength(s, 'utf8') + ':' + s).join(''), 'utf8');
function digest(kind, input) {
  const spec = api['x-digest-projection'][kind];
  const projection = {};
  for (const field of spec.fields) {
    assert.ok(Object.hasOwn(input, field), 'missing projection ' + kind + '.' + field);
    projection[field] = input[field];
  }
  const bytes = tokenBytes([spec.prefix, ...tokens(projection)]);
  return {sha256: sha(bytes), encoded_hex: bytes.toString('hex')};
}
for (const item of examples.typed_encoding_vectors) {
  const bytes = tokenBytes(tokens(item.input));
  record('digests', item.name, item.encoded_hex === bytes.toString('hex') && item.sha256 === sha(bytes));
}
record('digests', 'null与空字符串不混淆', tokenBytes(tokens(null)).toString('hex') !== tokenBytes(tokens('')).toString('hex'));
record('digests', '长度前缀消除数组拼接歧义', tokenBytes(tokens(['a', 'bc'])).toString('hex') !== tokenBytes(tokens(['ab', 'c'])).toString('hex'));
for (const item of examples.digest_vectors) {
  const actual = digest(item.kind, item.input);
  record('digests', item.name, actual.sha256 === item.sha256 && actual.encoded_hex === item.encoded_hex);
  for (const field of api['x-digest-projection'][item.kind].fields) {
    const altered = structuredClone(item.input);
    altered[field] = altered[field] === null ? '' : [altered[field], 'mutation'];
    record('digests', item.kind + '绑定:' + field, digest(item.kind, altered).sha256 !== item.sha256);
  }
}
for (const item of examples.fingerprint_vectors) {
  const original = digest(item.kind, item.original).sha256;
  const changed = digest(item.kind, item.changed).sha256;
  record('fingerprint', item.name, original === item.original_sha256 && changed === item.changed_sha256 && (original === changed) === item.expected_equal);
}
function evaluate(kind, v) {
  switch (kind) {
    case 'capture_action':
      if (v.action === 'acquire') {
        if (v.mode !== 'explicit_fetch') return 'mode_conflict';
        if (v.has_attempt === false && ['expired', 'cancelled'].includes(v.state)) return 'expired_or_cancelled';
        return v.state === 'awaiting_fetch' ? 'start' : 'replay_no_fetch';
      }
      if (v.mode !== 'local_snapshot') return 'mode_conflict';
      return ['awaiting_bytes', 'interrupted'].includes(v.state) ? 'start' : 'state_conflict';
    case 'upstream_status': return v.status === 200 ? 200 : 502;
    case 'encoding': return v.encoding === '' || v.encoding === 'identity';
    case 'origin': {
      let u;
      try { u = new URL(v.url); } catch { return false; }
      return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password && !u.port && v.origins.includes(u.origin);
    }
    case 'expiry': return {observation: v.capture_created + 1800, preview: Math.min(v.capture_created + 1800, v.preview_created + 600)};
    case 'retention': return [...v.before];
    case 'confirm':
      return v.authorized && v.policy_equal && v.capture_ready && v.unexpired && v.manifest_equal && v.confirmed && v.heads_equal.every(Boolean) && JSON.stringify([...v.selected].sort()) === JSON.stringify([...v.acknowledged].sort());
    case 'completeness':
      if (v.external) return 'external_reference';
      return v.format === 'rss' && !v.has_content && v.has_summary ? 'summary' : 'unknown';
    case 'window': {
      const bytes = Buffer.from(v.text, 'utf8');
      if (v.start < 0 || v.end < v.start || v.end > bytes.length || v.end - v.start > 4096) return false;
      if (bytes.length === 0) return v.start === 0 && v.end === 0;
      if (v.start === v.end) return false;
      try {
        new TextDecoder('utf-8', {fatal: true}).decode(bytes.subarray(0, v.start));
        new TextDecoder('utf-8', {fatal: true}).decode(bytes.subarray(v.start, v.end));
        return true;
      } catch { return false; }
    }
    default: throw new Error('unknown rule ' + kind);
  }
}
for (const item of examples.semantic_cases) record('semantic', item.name, JSON.stringify(evaluate(item.kind, item.input)) === JSON.stringify(item.expected));

function resolve(ref) {
  assert.ok(ref.startsWith('#/'), 'external ref not allowed');
  return ref.slice(2).split('/').reduce((node, key) => {
    assert.ok(node && Object.hasOwn(node, key), 'missing ref ' + ref);
    return node[key];
  }, api);
}
function walk(value) {
  if (!value || typeof value !== 'object') return;
  if (value.$ref) resolve(value.$ref);
  Object.values(value).forEach(walk);
}
walk(api);
record('endpoints', '全部本地$ref可解析', true);
const operationIds = [];
for (const [route, verbs] of Object.entries(api.paths)) {
  for (const [method, op] of Object.entries(verbs)) {
    operationIds.push(op.operationId);
    const expectedPaths = [...route.matchAll(/\{([^}]+)\}/g)].map(m => m[1]).sort();
    const paths = op.parameters.filter(p => p.in === 'path').map(p => p.name).sort();
    const versions = op.parameters.filter(p => p.in === 'header').map(p => p.schema.const).sort();
    const responses = Object.values(op.responses).map(r => r.$ref ? resolve(r.$ref) : r);
    record('endpoints', method.toUpperCase() + ' ' + route, JSON.stringify(paths) === JSON.stringify(expectedPaths) && JSON.stringify(versions) === JSON.stringify(['0.6.0', '1.0.0']) && responses.every(r => r.headers['Cache-Control'].schema.const === 'no-store'));
  }
}
record('endpoints', 'operationId唯一', new Set(operationIds).size === operationIds.length);
record('endpoints', '创建Capture不会取得／GET不取得／不prune', api['x-policies'].create_capture_fetches === false && api['x-policies'].get_fetches === false && api['x-policies'].missing_entries_action === 'retain' && api['x-policies'].capture_fetch_attempts === 1);
record('endpoints', '确认全原子', api['x-policies'].confirmation === 'all_or_nothing');
const captureReady = examples.samples.local_capture_ready;
const fetchReady = examples.samples.fetch_capture_ready;
record('semantic', '本地来源证明不伪造HTTP', captureReady.provenance.kind === 'user_supplied' && captureReady.provenance.upstream_http_status === null && captureReady.provenance.fetch_started_at === null && captureReady.provenance.response_content_type === null);
record('semantic', '在线来源证明区分服务取得', fetchReady.provenance.kind === 'server_http' && fetchReady.provenance.upstream_http_status === 200 && fetchReady.provenance.fetch_started_at !== null);
record('semantic', '观察钉选版本和截止', examples.samples.observation.expires_at === captureReady.expires_at && JSON.stringify(examples.samples.observation.producer_versions) === JSON.stringify(captureReady.producer_versions));
record('semantic', '跨登记不合并来源身份', JSON.stringify(['c'.repeat(32), examples.samples.entry.entry_key]) !== JSON.stringify(['9'.repeat(32), examples.samples.entry.entry_key]));
const rawWindow = examples.samples.window;
record('semantic', '窗口与完整表示摘要和长度一致', Buffer.byteLength(rawWindow.text) === rawWindow.end_byte - rawWindow.start_byte && sha(Buffer.from(rawWindow.text)) === rawWindow.representation.sha256 && rawWindow.end_byte === rawWindow.representation.byte_length);
record('semantic', '恢复槽绑定三原键且不含原文／令牌', examples.samples.recovery_slot.capture_create_key === examples.samples.local_capture_request.idempotency_key && examples.samples.recovery_slot.preview_create_key === examples.samples.preview_request.idempotency_key && examples.samples.recovery_slot.confirmation_request.idempotency_key === examples.samples.confirm.idempotency_key && Buffer.byteLength(JSON.stringify(examples.samples.recovery_slot)) <= 32768);

for (const [file, expected] of Object.entries(examples.protected_artifact_hashes)) record('protection', file, sha(read(file)) === expected);
const allowed = new Set([base + '.md', base + '.openapi.json', base + '.examples.json', 'tools/scripts/verify_knowledge_web_contract.cjs', 'docs/stage2r5-web-contract-verification.json', 'docs/stage2r5-web-handoff.md', 'docs/stage2-local-coordination.md', 'README.md', '会话接续说明.md', '本地第二大脑知识库技术方案.md']);
const git = args => cp.execFileSync('git', args, {cwd: root, encoding: 'utf8'}).split('\0').filter(Boolean);
const changed = [...new Set([...git(['diff', '--name-only', '-z', '7c564a738597e924e411f3b191aaa02d14ae42d8']), ...git(['ls-files', '--others', '--exclude-standard', '-z'])])].sort();
record('scope', '仅候选／核验脚本／接续文档，无业务改动', changed.every(file => allowed.has(file)), changed);
record('scope', '候选诚实标记未冻结／未接入／模型0', api['x-lifecycle'] === 'ReviewCandidate' && api['x-scope'].business_code_connected === false && api['x-scope'].runtime_loaded === false && api['x-scope'].model_calls === 0 && api['x-scope'].real_source_fetches === 0 && api['x-scope'].rsshub_execution === false);
record('scope', '结构样本单JSON不越32KiB', examples.structure_cases.filter(c => c.valid).every(c => Buffer.byteLength(JSON.stringify(c.value)) <= 32768));
const all = Object.values(results).flat();
const failed = all.filter(item => !item.passed);
const report = {recorded_at: new Date().toISOString(),notice:'仅候选结构／参考规则验证，不是应用、SSRF、网络、RSSHub或性能测试',passed: failed.length === 0,counts: Object.fromEntries(Object.entries(results).map(([name, rows]) => [name, rows.length])),operation_count:operationIds.length,schema_count:Object.keys(validators).length,failed,results};
const outputDir = path.join(root, '.local-data/stage2r5-20261026');
fs.mkdirSync(outputDir, {recursive: true});
fs.writeFileSync(path.join(outputDir, 'candidate-validation.json'), JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify({passed: report.passed,counts: report.counts,operations:report.operation_count,schemas:report.schema_count,failed}));
if (failed.length) process.exitCode = 1;

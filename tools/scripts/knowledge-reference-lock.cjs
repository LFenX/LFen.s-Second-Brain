'use strict';

// Read-only reference validation and selection for the knowledge-core task.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');

const manifestPath = 'docs/knowledge-core-reference-lock.json';
const sourceScopes = [
  'cmd', 'internal', 'web', 'docker', 'tools/scripts', 'ragflow_deps',
  'sdk/python', 'test', '.github/workflows',
  'build.sh', 'Dockerfile', 'go.mod', 'go.sum', 'lefthook.yml',
];
const codeExtension = /\.(?:go|mod|sum|tsx?|jsx?|cjs|mjs|py|sh|c|cc|cpp|h|hpp|sql|json|ya?ml|toml)$/i;
const sha256 = value => crypto.createHash('sha256').update(value).digest('hex');

function canonical(value) {
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']';
  if (value && typeof value === 'object') {
    return '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + canonical(value[key])).join(',') + '}';
  }
  return JSON.stringify(value);
}

function digestManifest(manifest) {
  const payload = { ...manifest };
  delete payload.manifest_payload_sha256;
  return sha256(canonical(payload));
}

function safeFile(root, name) {
  if (typeof name !== 'string' || !name || name.includes('\\') || path.isAbsolute(name) ||
      name.split('/').some(segment => !segment || segment === '.' || segment === '..') ||
      /(^|\/)\.env(?:\.|$)/i.test(name) || name.startsWith('.local-data/') || /\.local\./i.test(name)) {
    throw new Error('不允许的引用路径');
  }
  const resolvedRoot = fs.realpathSync(root);
  const resolved = fs.realpathSync(path.resolve(resolvedRoot, name));
  const relative = path.relative(resolvedRoot, resolved);
  if (!relative || relative === '..' || relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) {
    throw new Error('引用路径超出仓库');
  }
  if (!fs.statSync(resolved).isFile()) throw new Error('引用不是文件');
  return resolved;
}

function pointerValue(value, pointer) {
  if (pointer === '') return value;
  if (typeof pointer !== 'string' || !pointer.startsWith('/')) throw new Error('非法JSON指针');
  for (const token of pointer.slice(1).split('/')) {
    const key = token.replace(/~1/g, '/').replace(/~0/g, '~');
    if (value === null || typeof value !== 'object' || !Object.prototype.hasOwnProperty.call(value, key)) {
      throw new Error('JSON选择字段不存在');
    }
    value = value[key];
  }
  return value;
}

function selectedText(raw, selector) {
  const text = new TextDecoder('utf-8', { fatal: true }).decode(raw);
  if (selector.type === 'whole') return text;
  if (selector.type === 'prefix_before') {
    const marker = selector.marker;
    const start = text.indexOf(marker);
    if (start < 0 || text.indexOf(marker, start + marker.length) >= 0) throw new Error('前缀边界缺失或不唯一');
    return text.slice(0, start);
  }
  if (selector.type === 'markdown_section') {
    const lines = text.split(/\r?\n/);
    const starts = lines.flatMap((line, index) => line === selector.heading ? [index] : []);
    if (starts.length !== 1) throw new Error('Markdown章节缺失或不唯一');
    const start = starts[0];
    const level = selector.heading.match(/^#+/)[0].length;
    let end = lines.length;
    for (let index = start + 1; index < lines.length; index++) {
      const heading = lines[index].match(/^(#+) /);
      if (heading && heading[1].length <= level) { end = index; break; }
    }
    return lines.slice(start, end).join('\n');
  }
  const value = JSON.parse(text);
  if (selector.type === 'json_pointer') return JSON.stringify(pointerValue(value, selector.pointer), null, 2) + '\n';
  if (selector.type === 'json_fields') {
    const selected = {};
    for (const field of selector.fields) selected[field] = pointerValue(value, '/' + field.replace(/~/g, '~0').replace(/\//g, '~1'));
    return JSON.stringify(selected, null, 2) + '\n';
  }
  throw new Error('不支持的选择器');
}

function verifyEntry(root, entry) {
  const raw = fs.readFileSync(safeFile(root, entry.path));
  const selected = selectedText(raw, entry.selector);
  if (!['file', 'selection'].includes(entry.integrity.mode)) throw new Error('摘要范围非法');
  const observed = sha256(entry.integrity.mode === 'file' ? raw : Buffer.from(selected, 'utf8'));
  if (observed !== entry.integrity.sha256 || sha256(Buffer.from(selected, 'utf8')) !== entry.selection_sha256) {
    throw new Error('引用摘要漂移: ' + entry.id);
  }
  return selected;
}

function getSourceSnapshot(root) {
  const names = execFileSync('git', ['ls-files', '--cached', '--others', '--exclude-standard', '-z', '--', ...sourceScopes], {
    cwd: root, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024,
  }).split('\0');
  const files = [...new Set(names)].filter(name => name &&
    (codeExtension.test(name) || path.posix.basename(name) === 'Dockerfile') &&
    !/(^|\/)\.env(?:\.|$)/i.test(name) && !/\.local\./i.test(name)).sort();
  const rows = [];
  for (const name of files) {
    const absolute = path.resolve(root, name);
    const stat = fs.lstatSync(absolute);
    const content = stat.isSymbolicLink() ? Buffer.from(fs.readlinkSync(absolute), 'utf8') : fs.readFileSync(absolute);
    rows.push([name, stat.isSymbolicLink() ? 'symlink' : 'file', sha256(content)]);
  }
  return { algorithm: 'sha256-canonical-path-kind-content-sha256-v1', file_count: rows.length, sha256: sha256(canonical(rows)) };
}

function validateLock(root, manifest, expected) {
  if (manifest.schema_version !== 1 || manifest.lock_id !== expected && expected !== undefined) throw new Error('引用锁ID或结构版本不符');
  if (manifest.manifest_payload_sha256 !== digestManifest(manifest)) throw new Error('引用锁内容摘要不符');
  const ids = new Set();
  for (const entry of manifest.entries) {
    if (ids.has(entry.id)) throw new Error('重复引用条目ID');
    ids.add(entry.id);
    verifyEntry(root, entry);
    if (entry.role === 'candidate_design' && entry.allowed_uses.some(use => use.includes('contract'))) {
      throw new Error('候选被错误提升为契约');
    }
  }
  const protectedEntries = Object.entries(manifest.protected_files);
  if (protectedEntries.length !== 47) throw new Error('47保护件清单不完整');
  for (const [name, digest] of protectedEntries) {
    if (sha256(fs.readFileSync(safeFile(root, name))) !== digest) throw new Error('保护件摘要改变: ' + name);
  }
  for (const request of manifest.request_checks) {
    const value = JSON.parse(fs.readFileSync(safeFile(root, request.path), 'utf8'));
    const original = pointerValue(value, request.pointer);
    if (typeof original !== 'string' || sha256(Buffer.from(original, 'utf8')) !== request.sha256) throw new Error('真人原话摘要漂移');
  }
  for (const item of manifest.archive.archived) {
    if (fs.existsSync(path.resolve(root, item.source)) ||
        sha256(fs.readFileSync(safeFile(root, item.destination))) !== item.archive_sha256) throw new Error('归档位置或摘要改变');
  }
  for (const item of manifest.archive.retained_historical) {
    if (sha256(fs.readFileSync(safeFile(root, item.source))) !== item.original_sha256) throw new Error('原路径历史证据改变');
  }
  const formal = manifest.entries.filter(entry => entry.role === 'existing_contract');
  if (formal.length !== 26 || formal.some(entry => /\.candidate\./.test(entry.path))) throw new Error('正式契约与旧候选分类错误');
  const source = getSourceSnapshot(root);
  if (canonical(source) !== canonical(manifest.source_snapshot)) throw new Error('源码事实基线漂移；请主控复核受影响资料');
  return { source, readable_entries: manifest.entries.length, formal_contract_files: formal.length,
    protected_files: protectedEntries.length, archived_documents: manifest.archive.archived.length,
    retained_historical_documents: manifest.archive.retained_historical.length };
}

function readEntry(root, manifest, id, use) {
  const entry = manifest.entries.find(item => item.id === id);
  if (!entry) throw new Error('条目未登记为可读资料；历史/未复核资料需主控指定');
  if (!entry.allowed_uses.includes(use)) throw new Error('用途不允许: ' + entry.id);
  return {
    reference: { lock_id: manifest.lock_id, id: entry.id, path: entry.path, role: entry.role, use,
      selection_sha256: entry.selection_sha256, limitation: entry.limitation },
    content: verifyEntry(root, entry),
  };
}

function argumentsOf(args) {
  const command = args[0];
  if (!['check', 'list', 'read'].includes(command)) throw new Error('用法: check|list|read [--id ID --use USE] [--expect-sha256 SHA256] [--expect-lock ID] [--json]');
  const options = {};
  for (let index = 1; index < args.length; index++) {
    const key = args[index];
    if (key === '--json') { if (options.json) throw new Error('参数重复'); options.json = true; continue; }
    if (!['--id', '--use', '--expect-sha256', '--expect-lock'].includes(key) || index + 1 >= args.length) throw new Error('参数非法或缺值');
    if (Object.prototype.hasOwnProperty.call(options, key)) throw new Error('参数重复');
    options[key] = args[++index];
  }
  if (command === 'read' && (!options['--id'] || !options['--use'])) throw new Error('read必须指定条目ID和用途');
  return { command, options };
}

function main(args) {
  const root = path.resolve(__dirname, '../..');
  const { command, options } = argumentsOf(args);
  const raw = fs.readFileSync(safeFile(root, manifestPath));
  const fileSha = sha256(raw);
  if (options['--expect-sha256'] !== undefined && options['--expect-sha256'] !== fileSha) throw new Error('与主控派发的引用锁SHA-256不符');
  const manifest = JSON.parse(raw.toString('utf8'));
  const observed = validateLock(root, manifest, options['--expect-lock']);
  if (command === 'check') {
    console.log(JSON.stringify({ status: 'Passed_ReferenceLockOnly', lock_id: manifest.lock_id,
      manifest_file_sha256: fileSha, manifest_payload_sha256: manifest.manifest_payload_sha256,
      ...observed, unlisted_policy: manifest.unlisted_policy }, null, 2));
  } else if (command === 'list') {
    console.log(JSON.stringify(manifest.entries.map(({ id, path: file, role, allowed_uses, selector }) => ({ id, path: file, role, allowed_uses, selector })), null, 2));
  } else {
    const selected = readEntry(root, manifest, options['--id'], options['--use']);
    if (options.json) console.log(JSON.stringify(selected, null, 2));
    else process.stdout.write(JSON.stringify(selected.reference, null, 2) + '\n\n' + selected.content + '\n');
  }
}

if (require.main === module) {
  try { main(process.argv.slice(2)); }
  catch (error) { console.error('REFUSED: ' + error.message); process.exitCode = 1; }
}

module.exports = { canonical, sha256, digestManifest, safeFile, selectedText, verifyEntry, getSourceSnapshot, validateLock, readEntry, argumentsOf };

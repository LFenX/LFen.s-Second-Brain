// Documentation-only validation. No application imports, HTTP, SQL, models or migrations.
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const Ajv = require("../../web/node_modules/ajv/dist/2020.js").default;
const addFormats = require("../../web/node_modules/ajv-formats").default;

const root = path.resolve(__dirname, "../..");
const base = "docs/knowledge-source-lifecycle-v0.7.candidate";
const read = (name) => fs.readFileSync(path.join(root, name));
const api = JSON.parse(read(base + ".openapi.json"));
const examples = JSON.parse(read(base + ".examples.json"));
const sha = (bytes) => crypto.createHash("sha256").update(bytes).digest("hex");
const groups = {
  schemas: [],
  structure: [],
  digests: [],
  decisions: [],
  fences: [],
  projects: [],
  paths: [],
  queries: [],
  errors: [],
  endpoints: [],
  protection: [],
};
const record = (group, name, passed, detail) =>
  groups[group].push({ name, passed: !!passed, ...(detail ? { detail } : {}) });
const rewrite = (value) =>
  JSON.parse(JSON.stringify(value).replaceAll("#/components/schemas/", "#/$defs/"));
const ajv = new Ajv({ strict: true, strictTypes: false, allErrors: true });
addFormats(ajv);
const validators = {};
for (const [name, schema] of Object.entries(api.components.schemas)) {
  try {
    validators[name] = ajv.compile({ ...rewrite(schema), $defs: rewrite(api.components.schemas) });
    record("schemas", name, true);
  } catch (error) {
    record("schemas", name, false, error.message);
  }
}
for (const item of examples.structure_cases) {
  const validate = validators[item.schema];
  const valid = !!validate && validate(item.value);
  record(
    "structure",
    item.name,
    valid === item.valid,
    valid !== item.valid ? validate?.errors : undefined,
  );
}

// Independent string-token encoder; the fixture producer encodes recursive buffers.
function tokens(value) {
  if (value === null) return ["n"];
  if (typeof value === "string") return ["s", value];
  if (typeof value === "boolean") return ["b", value ? "1" : "0"];
  if (typeof value === "number" && Number.isSafeInteger(value)) return ["i", String(value)];
  if (Array.isArray(value)) return ["a", String(value.length), ...value.flatMap(tokens)];
  if (!value || Object.getPrototypeOf(value) !== Object.prototype)
    throw new Error("invalid typed value");
  const keys = Object.keys(value).sort((a, b) => Buffer.compare(Buffer.from(a), Buffer.from(b)));
  return ["o", String(keys.length), ...keys.flatMap((key) => [key, ...tokens(value[key])])];
}
const encoded = (value) =>
  Buffer.from(
    tokens(value)
      .map((token) => Buffer.byteLength(token, "utf8") + ":" + token)
      .join(""),
    "utf8",
  );
for (const item of examples.digest_vectors) {
  const actual = encoded(item.value);
  record(
    "digests",
    item.name,
    actual.toString("hex") === item.encoded_hex && sha(actual) === item.sha256,
  );
}
const scalarString = (value) =>
  typeof value === "string" &&
  [...value].every(
    (char) => char.length !== 1 || char.charCodeAt(0) < 0xd800 || char.charCodeAt(0) > 0xdfff,
  );
const goWhitespace = new Set([
  9,
  10,
  11,
  12,
  13,
  32,
  0x85,
  0xa0,
  0x1680,
  ...Array.from({ length: 11 }, (_, index) => 0x2000 + index),
  0x2028,
  0x2029,
  0x202f,
  0x205f,
  0x3000,
]);
const goEdgeSpace = (value) => {
  const characters = [...value];
  return (
    characters.length > 0 &&
    (goWhitespace.has(characters[0].codePointAt(0)) ||
      goWhitespace.has(characters.at(-1).codePointAt(0)))
  );
};
function validClassification(value) {
  return (
    validators.Classification(value) &&
    scalarString(value.language) &&
    !value.language.includes("\0") &&
    !goEdgeSpace(value.language)
  );
}
function validTarget(value) {
  return (
    scalarString(value) &&
    [...value].length >= 1 &&
    [...value].length <= 512 &&
    !/[\\:\p{Cc}]/u.test(value) &&
    !value.startsWith("/") &&
    !value.split("/").some((part) => !part || part === "." || part === "..")
  );
}
function decision(item) {
  if (!item.authorised) return "knowledge_not_found";
  if (item.receipt) return item.receipt === "same" ? "replay" : "idempotency_conflict";
  if (item.request.expected_policy_version !== item.policy_version) return "policy_changed";
  if (
    item.request.expected_source_revision_id !== item.source.source_revision_id ||
    item.request.expected_lifecycle_version !== item.source.lifecycle_version
  )
    return "revision_changed";
  if (item.source.state === "archived" && ["move", "reclassify"].includes(item.action))
    return "lifecycle_source_archived";
  let changed;
  if (item.action === "reclassify") {
    if (item.group_valid === false) return "knowledge_not_found";
    if (!validClassification(item.request.classification)) return "knowledge_invalid_request";
    const a = item.request.classification,
      b = item.source.classification;
    changed =
      a.group_id !== b.group_id ||
      a.component_side !== b.component_side ||
      a.language !== b.language;
  } else if (item.action === "move") {
    if (!item.registration_id || !["text_file", "original_document"].includes(item.source_kind))
      return "lifecycle_action_unsupported";
    if (
      !validTarget(item.request.relative_path) ||
      !item.target_paths.includes(item.request.relative_path) ||
      item.media_matches === false
    )
      return "knowledge_invalid_request";
    if (item.target_owner && item.target_owner !== "d".repeat(32))
      return "lifecycle_position_occupied";
    changed = item.source.relative_path !== item.request.relative_path;
  } else changed = item.source.state !== (item.action === "archive" ? "archived" : "active");
  if (!changed) return "unchanged";
  if (item.source.lifecycle_version === Number.MAX_SAFE_INTEGER)
    return "lifecycle_version_exhausted";
  return "applied";
}
for (const item of examples.decision_cases)
  record("decisions", item.name, decision(item) === item.expected);
function fence(item) {
  if (item.authorised === false) return "knowledge_not_found";
  if (item.fixed_success) return "fixed_success";
  if (!item.observed) return "revision_changed";
  if (item.observed.source_id === null) return item.current === null ? "allow" : "revision_changed";
  return item.current?.state === "active" &&
    item.observed.source_id === item.current.source_id &&
    item.observed.version === item.current.version
    ? "allow"
    : "revision_changed";
}
for (const item of examples.fence_cases) record("fences", item.name, fence(item) === item.expected);
for (const item of examples.project_cases) {
  const result = !item.authorised
    ? "knowledge_not_found"
    : item.already_published
      ? "fixed_payload"
      : item.observed_version !== null &&
          item.observed_version === item.current_version &&
          item.active &&
          item.organisation_matches
        ? "allow"
        : "revision_changed";
  record("projects", item.name, result === item.expected);
}
for (const item of examples.path_cases)
  record(
    "paths",
    item.name,
    Buffer.from(item.left).toString("hex") === item.left_utf8 &&
      Buffer.from(item.right).toString("hex") === item.right_utf8 &&
      Buffer.from(item.left).equals(Buffer.from(item.right)) === item.equal,
  );
for (const [name, value, expected] of [
  ["unicode_scalar", "组件/🧠.tsx", true],
  ["unpaired_surrogate", "src/\uD800.tsx", false],
  ["parent_path", "src/../x.tsx", false],
  ["absolute_path", "/x.tsx", false],
  ["backslash", "src\\x.tsx", false],
  ["control", "src/\u0001.tsx", false],
])
  record("paths", name, validTarget(value) === expected);
function query(value) {
  const allowed = [
    "state",
    "group_scope",
    "group_id",
    "component_side",
    "language",
    "limit",
    "cursor",
  ];
  if (Object.keys(value).some((key) => !allowed.includes(key))) return false;
  if (value.state !== undefined && !["active", "archived"].includes(value.state)) return false;
  const group = value.group_scope || "all";
  if (
    !["all", "ungrouped", "exact"].includes(group) ||
    (group === "exact" ? !validators.ID(value.group_id) : value.group_id !== undefined)
  )
    return false;
  if (
    value.component_side !== undefined &&
    !["frontend", "backend", "shared", "cli", "non_code"].includes(value.component_side)
  )
    return false;
  if (
    value.language !== undefined &&
    !validClassification({ group_id: null, component_side: "non_code", language: value.language })
  )
    return false;
  if (
    value.limit !== undefined &&
    (!Number.isInteger(value.limit) || value.limit < 1 || value.limit > 20)
  )
    return false;
  return value.cursor === undefined || validators.ID(value.cursor);
}
for (const item of examples.query_cases)
  record("queries", item.name, query(item.query) === item.valid);
for (const item of examples.structure_cases.filter((x) => x.schema === "Error" && x.valid)) {
  const map = api["x-error-mapping"][item.value.code];
  record(
    "errors",
    item.name,
    !!map && [400, 401, 404, 409, 413, 503].includes(map[0]) && map[1] === item.value.outcome_known,
  );
}
for (const item of examples.operations) {
  const actual = api.paths[item.path]?.[item.method.toLowerCase()];
  record(
    "endpoints",
    item.operation_id,
    actual?.operationId === item.operation_id &&
      !!actual.responses["200"] &&
      (!item.request || !!actual.responses["201"]) &&
      actual.parameters.some(
        (x) => x.name === "X-Knowledge-Source-Contract-Version" && x.schema.const === "0.7.0",
      ),
  );
}
record(
  "endpoints",
  "exactly_ten_operations",
  examples.operations.length === 10 &&
    Object.values(api.paths).reduce((count, item) => count + Object.keys(item).length, 0) === 10,
);
for (const [name, expected] of Object.entries(examples.protected_files))
  record("protection", name, sha(read(name)) === expected);
record(
  "protection",
  "twenty_nine_frozen_resources",
  Object.keys(examples.protected_files).length === 29,
);

const output = path.join(root, ".local-data/stage2r6-20261026");
fs.mkdirSync(output, { recursive: true });
const result = {
  recorded_at: new Date().toISOString(),
  scope:
    "Candidate documentation/reference validation only; no Go, API, migration, browser, model or site execution",
  passed: Object.values(groups)
    .flat()
    .every((x) => x.passed),
  counts: Object.fromEntries(Object.entries(groups).map(([name, items]) => [name, items.length])),
  groups,
  input_sha256: {
    markdown: sha(read(base + ".md")),
    openapi: sha(read(base + ".openapi.json")),
    examples: sha(read(base + ".examples.json")),
  },
};
const resultName = "contract-check-" + Date.now() + ".json";
fs.writeFileSync(path.join(output, resultName), JSON.stringify(result, null, 2) + "\n");
fs.writeFileSync(
  path.join(output, "latest-contract-check.json"),
  JSON.stringify(
    { file: resultName, sha256: sha(fs.readFileSync(path.join(output, resultName))) },
    null,
    2,
  ) + "\n",
);
console.log(
  JSON.stringify({
    passed: result.passed,
    counts: result.counts,
    result: ".local-data/stage2r6-20261026/" + resultName,
    failures: Object.values(groups)
      .flat()
      .filter((x) => !x.passed),
  }),
);
if (!result.passed) process.exitCode = 1;

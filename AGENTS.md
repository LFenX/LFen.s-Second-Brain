# LFen's Second Brain Development Instructions

Use this file as the local operating guide for the current codebase. Prefer the code and the current AGENTS.md over any older convention or remembered project shape.

## Current task reference boundary
- Before using project documents, read `docs/knowledge-core-reference-authority.md` and run `node tools/scripts/knowledge-reference-lock.cjs check`. Every delegated task must carry the reference lock ID, the coordinator-provided manifest SHA-256, permitted entry IDs/uses, and its write window. Verify that exact manifest with `--expect-sha256` before proceeding.
- Read assigned documents through `node tools/scripts/knowledge-reference-lock.cjs read --id <entry> --use <use>`. Only the registered chapters/JSON fields are current references. Links, search hits, old handoffs, filenames, dates, and previous chat messages do not grant authority or write permission. Unregistered documents require coordinator review before use as current guidance.
- Current plan, static facts, existing frozen contracts, candidate designs, synthetic inputs, and historical evidence have different uses. `Draft_NotFreezeReady` is never an implementation contract; old approved contracts do not approve new interfaces or renew model/deployment scope. A lock failure or source drift stops only affected work; report it to the coordinator and never refresh hashes to force a pass.
- The latest human scope takes precedence over all historical plans and upstream migration advice, including the Go-only target below. Do not bulk-delete Python/native resources, restore old A/B write windows, expand complex formats/Wiki, or start Agent personality/memory work from historical text. Root layout changes still require the user's explicit confirmation.
- `docs/archive/` and the old documents listed as retained historical references in its index are not formal development guidance. Preserve archived evidence and any path retained for contract/evidence dependencies; do not restore archived plans into the active document list.

## Core Stance
- Treat legacy code as liability, not as a compatibility target.
- Prefer deletion over shims, deprecated branches, wrapper APIs, and dual-track migration notes.
- If old and new implementations coexist, converge to one path unless an external contract forces compatibility.
- Remove dead tests, commented-out code, stale docs, and "move later" notes instead of preserving them.
- Reduce public surface area when a helper can be made private or internal.
- Keep refactors centered on the owning abstraction, not on adjacent compatibility layers.

## Current stack
- Backend: Go implements the API, admin, ingestor, and syncer modes in `cmd/ragflow_server.go`; the CLI runs from `cmd/ragflow-cli.go`. The Docker entrypoint starts the Go services according to its command-line flags, and the frontend development server uses fixed Go backend proxy routes. Root-level `api/`, `rag/`, `deepdoc/`, and `agent/` are absent from the current tree; the similarly named Go packages under `internal/` remain active.
- Frontend: React + TypeScript + Vite in `web/`. When working under `web/`, read and follow `web/AGENTS.md` for frontend conventions.
- The Go module contains the server, ingestion, parsing, agent runtime, CLI, and supporting services.
- Runtime services commonly include MySQL/PostgreSQL, Kvrocks, NATS JetStream, MinIO, ClickHouse, and Elasticsearch/Infinity/OpenSearch depending on configuration.
## Go-only target
- Remove all Python code and Python-specific dependencies, scripts, containers, CI jobs, documentation, SDKs, and tests from the repository. Do not add new Python code while this removal is in progress. Replace Python-based preparation of Go native libraries, DeepDoc models, and the BPE table before removing the existing download scripts.
- Move any still-required behavior and test coverage into the Go backend or the frontend before deleting the old implementation. Do not preserve a Python server, compatibility layer, fallback, or dual-backend path.
- Preserve Go DeepDoc `.ort` models and `ocr.res` under `internal/rag/res/deepdoc/`. Any resource relocation requires coordinated model discovery, dependency preparation, Docker packaging, tests, and explicit approval for changing the directory structure.
- The frontend development proxy uses fixed Go API/Admin routes. Do not restore Go/Python backend switching; outdated dual-backend notes do not describe the current frontend.
- Update Docker's default entrypoint, dependency image, and CI workflows when removing their remaining Python paths. Keep the frontend development proxy routes aligned with the Go API and Admin ports in `web/vite.config.ts`.
- Keep this guide aligned with the checked-in tree. Remaining Python downloaders, SDK, HTTP tests, hooks, and parity tools require separate dependency checks before removal; absent application directories are not a pending deletion list.

## Code Layout to Expect
- `cmd/ragflow_server.go`: Go entrypoint for the API, admin, ingestor, and syncer modes (`--api`, `--admin`, `--ingestor`, `--syncer`).
- `cmd/ragflow-cli.go`: Go CLI entrypoint.
- Root `api/`, `rag/`, `deepdoc/`, and `agent/` are absent. Do not recreate a Python application backend.
- `internal/`: main Go application code. Important subtrees:
- `internal/agent/`: Go agent runtime, canvas execution, components, tool bindings, workflow helpers.
- `internal/admin/`: Go admin routes, handlers, and services.
- `internal/binding/cpp/`: C++ tokenizer binding built by `build.sh` for the Go server.
- `internal/cli/`: CLI parsing, HTTP transport, command execution, response formatting.
- `internal/dao/`: Go data-access layer and persistence-facing helpers.
- `internal/deepdoc/`: Go DeepDOC integrations, especially native-backed PDF/DOCX parsing.
- `internal/engine/`: search/index backends such as Elasticsearch and Infinity.
- `internal/entity/`: shared Go entities and model definitions.
- `internal/handler/`: HTTP handlers and route-facing request logic.
- `internal/ingestion/`: Go ingestion pipeline, canvas adapter, components, wiring, service orchestration.
- `internal/ingestion/component/`: stage implementations such as file/parser/chunker/tokenizer/extractor.
- `internal/ingestion/pipeline/`: DSL translation, canvas-driven execution, checkpoints, resume/run logic.
- `internal/parser/`: parser and chunk libraries used by ingestion and other Go paths.
- `internal/parser/parser/`: typed parse-result parsers for markdown/html/pdf/docx/xlsx/text and related families.
- `internal/parser/chunk/`: chunk operator library and DSL/typed execution helpers.
- `internal/service/`: higher-level business services used by handlers and server flows.
- `internal/storage/`: storage backends and in-memory test doubles.
- `internal/router/`: HTTP route registration.
- `internal/server/`: server bootstrap/config wiring.
- `internal/syncer/`: Go file synchronization service and connectors.
- `internal/rag/`: Go retrieval and RAG logic.
- `internal/harness/`: Go runtime harness and supporting execution code.
- `web/`: frontend application.
- `docker/`: local and production compose files.
- `sdk/python/`: legacy Python client SDK pending deletion.
- `tools/scripts/download_dev_deps.py` and `ragflow_deps/download_deps.py`: Python downloaders for the Go build's dependencies. `download_dev_deps.py` pulls the native static libraries (pdfium, pdf_oxide, office_oxide, onnxruntime) that `build.sh` links; `download_deps.py` pulls the runtime resources (cl100k BPE table, DeepDoc models, embedding tokenizer assets) and installs the stagehand server into the SDK cache. Versions and URLs are defined inline in each downloader (`tools/scripts/download_dev_deps.py` for native static libraries, `ragflow_deps/download_deps.py` for runtime resources); the tokenizer assets are inlined as `TOKENIZER_ASSETS` in each downloader. Replace these with non-Python preparation before removing Python dependencies.
- `test/`: includes Python HTTP tests against the server and SDK-dependent tests. Some parity tools elsewhere still refer to absent Python application modules. Check each consumer before retiring coverage; do not classify all Python tests as direct imports of the removed backend.

## Go-Specific Rules
- Treat `internal/ingestion`, `internal/parser`, and `internal/deepdoc` as actively refactored code. Prefer collapsing duplicate paths over preserving transitional wrappers.
- Do not add or preserve deprecated Go APIs just to ease migration inside the repo.
- Remove commented-out Go code instead of leaving recovery notes in place.
- Keep package comments and doc comments aligned with the current runtime path, not with migration history.

## GORM query traps
- **Never pass a bare string ID as the second argument to `Take`/`First`** (e.g. `Take(&x, id)`). Under GORM v1.25.7 a non-numeric string is treated as a raw SQL condition and concatenated into the `WHERE` clause: `Take(&x, "88b17c7a...")` emits `WHERE 88b17c7a... LIMIT 1`, which MySQL rejects with `Error 1054 (42S22): Unknown column '...' in 'where clause'`. Only strings that parse via `strconv.Atoi` are treated as primary-key values, so this bug is invisible for integer IDs and catastrophic for UUID primary keys. Always bind explicitly: `Take(&x, "id = ?", id)` or `Where("id = ?", id).Take(&x)`.
- When you change a query form across many files, verify the generated SQL with a `Session(&gorm.Session{DryRun: true})` probe before relying on it. A passing unit test with no rows and a broken `WHERE` clause look identical unless the SQL is inspected.

## Shared database schema (Go + Python)
The following Go/Peewee notes describe the upstream shared-schema history, not a Python API running from this checkout. The current application implementation is Go. Inspect the actual database and Go migration before changing existing constraints; do not recreate absent Python models to satisfy historical guidance.

Each ORM derives index names differently:
- Go (`internal/entity/`): GORM derives `idx_<table>_<column>` for indexes and `uni_<table>_<column>` for unique constraints. Tags in this repo name unique indexes explicitly, e.g. `uniqueIndex:idx_commit_file`.
- Python (`api/db/db_models.py`): Peewee derives `<db_table>_<col1>_<col2>` (truncated with an md5 suffix past 64 chars, see `playhouse.migrate.make_index_name`). There is no `idx_` prefix.

Both sides reconcile by **column set, not by name**: Go's `hasUniqueIndex` (`internal/dao/migration.go`) accepts an existing index only when its full column set equals the requested one, and Python's `ensure_model_indexes` (`api/db/db_models.py`) keys `DB.get_indexes()` on the column tuple. Consequences:

- Divergent names covering the same columns are expected, not a defect. Whichever runtime reaches the database first decides the name. Do not "fix" this by renaming inside a migration; it rewrites live databases for no functional gain.
- Add current indexes in the Go entity and migration that own them. Where an external historical deployment still shares the schema, verify its column-set requirements before migration. Do not add a second index over a column set an existing one already covers.
- In Go, declare single-column uniqueness with a named `uniqueIndex:`, not a bare `unique` tag. A bare `unique` becomes a table-level constraint in DDL, which SQLite materializes as an implicit index that cannot be dropped by name, and which GORM's `MigrateColumnUnique` then tries to drop under its own `uni_<table>_<column>` name — MySQL answers 1091 every startup. `namedIndexMigrator` (`internal/dao/database.go`) suppresses exactly that phantom drop; see `TestNamedIndexMigratorSkipsPhantomUniqueDrop`.
- A Python model existing does not mean Python owns that table's constraints. `pipeline_operation_log` has no `run_count` column and no unique index in `db_models.py`; both are added only by `internal/dao/migration.go`.

## Go Test Tiers
Go tests are classified by build tag. Tag a test file with `//go:build <tier>` placed before the `package` clause. Run them through `build.sh` so the required CGO configuration and native libraries are available.

| Tier | Build tag | Runs by default? | Needs |
|---|---|---|---|
| Unit | (none) | Yes (`bash build.sh --test`) | Native CGO static libs; no external services — uses in-memory SQLite, miniredis, or `httptest` stubs. |
| Integration | `integration` | No (`-tags integration`) | A real service: MySQL/MinIO/Elasticsearch/Infinity/LLM. Single component, reasonably fast. |
| E2E | `e2e` | No (`-tags e2e`) | Full cross-component pipeline (ingest → index → retrieve) against real services; heavy/slow. |
| Manual | `manual` | No (`-tags manual`) | Very slow/expensive (deepdoc render/parity/snapshot/bench). **Local opt-in ONLY — never run in CI.** |
| Native (orthogonal) | `cgo` / `!cgo` | `cgo` auto-satisfies under CGO_ENABLED=1 | Native static libs (`office_oxide`/`pdfium`/`pdf_oxide`). Combine with tiers, e.g. `//go:build cgo && integration`. |

Run tiers locally via `build.sh`:
```bash
bash build.sh --test                      # unit tier (no tags)
bash build.sh --test-integration ./...    # integration tier
bash build.sh --test-e2e                  # e2e tier
bash build.sh --test-manual               # manual tier (very slow)
bash build.sh --test-all                  # integration + e2e (never includes manual)
```
Rules:
- New tests that touch a real external service MUST carry `integration`/`e2e`/`manual` — do not rely on `t.Skip` + env vars to soft-isolate them in the default unit run. Keep an env guard as a harmless secondary safety net if desired.
- `manual` is never wired into CI or any automated pipeline.
- `unit` (no tag) must stay free of external-service dependencies so `bash build.sh --test` runs without MySQL/MinIO/ES/Infinity/LLM. The native CGO static libraries (`office_oxide`/`pdfium`/`pdf_oxide`) are still required at build time and are wired by `build.sh --test`; that is expected, not an external service.

## Working Rules
- When reviewing documentation or code, inspect the full affected path and report all verifiable findings in one review; do not return after only a few findings and expose further issues in later rounds.
- When handling review comments, independently verify each substantive claim against the current code or tests before accepting, rejecting, or acting on it.
- Before editing, inspect the nearest code path that actually owns the behavior.
- For server changes, work in the Go implementation. If a Python behavior is still needed, port it to Go and test the Go path before removing the Python code.
- Keep changes small and local unless the task is explicitly a broader refactor.
- Prefer one implementation path instead of preserving old and new versions side by side.
- Preserve behavior with focused tests when the behavior is still valid; do not keep tests that protect obsolete behavior.
- If a surface is only there for compatibility, remove it unless the user asks to keep it.
- Do not add new compatibility wording in comments or docs.
- When a maintainer takes over a community PR, a new commit generated by rewriting history (e.g. `merge`, `rebase -i`) must preserve the original author and add the maintainer as co-author (via a `Co-authored-by:` trailer) instead of overwriting the author with the maintainer alone.

## Commands
### Go backend
```bash
bash build.sh --all                         # build native code and Go server
bash build.sh --go                          # build Go server with native library already built
bash build.sh --test ./internal/handler/... # targeted unit tests
bash build.sh --test                        # all unit tests
bash build.sh --test-integration ./...      # integration tests; requires real services
bash build.sh --test-e2e                    # full-pipeline tests; requires real services
bash build.sh --test-manual                 # local opt-in only
bash build.sh --test-all                    # integration + e2e; excludes manual
```

Native libraries, Go DeepDoc models, tokenizer assets, and `ragflow_deps/cl100k_base.tiktoken` come from `tools/scripts/download_dev_deps.py` (native static libs) and `ragflow_deps/download_deps.py` (models, cl100k, tokenizer assets); run them with `uv run`. Build the Go resource image with `cd ragflow_deps && docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .` after downloading the resources.

### Frontend
```bash
cd web
npm install
npm run dev
npm run build
npm run lint
npm run test
npm run type-check
```

## Validation Preference
- Run the narrowest relevant test, lint, or build command after a change.
- For backend changes, first run package-scoped `bash build.sh --test ./path/to/package/...`. Run the integration or e2e tier when the changed behavior needs real services or a full pipeline.
- For frontend changes, prefer the touched-package lint, type-check, or test command.
- Do not default to raw `go test`, `go build`, or IDE Run/Debug for Go in this repo. They often miss the required CGO flags and native static libraries (`office_oxide`, `pdfium-static`, `pdf_oxide`) that `build.sh` wires correctly.
- After a Go build, execute the resulting binary's help command for each changed server mode; a successful link alone does not prove the binary starts. On this host, the original `build.sh` selected default LLD 18 and produced binaries that crashed before Go `main`; linking with LLD 20 started successfully. Verify the linker actually selected on Linux before trusting a new binary.
- If Go native builds fail, inspect `build.sh` and `internal/development.md` before changing code. Common environment issues are missing downloaded native dependencies or an incompatible `ld.lld` on Linux.

## Default review checklist
- Remove instead of retaining `deprecated`, `legacy`, or compatibility-only code.
- Collapse duplicate implementations to one path.
- Drop stale comments and documentation that describe a superseded design.
- Keep exported APIs only when the current code actually needs them.


## LFen Second Brain project rules (take precedence over upstream migration advice)
- 默认中文，表达简短、清晰；每轮开头标注使用的 skills。
- Primary repository: https://github.com/LFenX/LFen.s-Second-Brain. Develop in this repository root. The sibling ragflow checkout is a rollback reference, not the active development location.
- Upstream baseline: infiniflow/ragflow v1.0.0-rc2, commit ac29b4b29928ca65d3efca19b6cfae3f86069b81. Preserve LICENSE and upstream attribution. The existing repository history remains intact; this is a source snapshot import, not a rewritten upstream history.
- Keep the upstream cmd/internal/web/docker layout and existing Chinese planning files in place. Do not rename or restructure existing directories without the user's explicit request and confirmation.
- Read current material through the reference lock: README/会话接续说明 approved prefixes, 本地第二大脑知识库技术方案 chapter 41, and `docs/knowledge-core-realignment-plan.md`. Earlier chapters and old handoffs are historical evidence, not default current instructions. Preserve earlier acceptance scopes and original evidence.
- Prioritize a unified knowledge model and the native ingestion/retrieval chain before more format expansion or a general Agent runtime. Continue with synthetic material only. Do not send real AICS content to remote models or start new model calls without the relevant user scope. Department/module permissions, temporary MCP keys and code reuse packages remain planned, not completed; new shared interfaces require a specific candidate decision before business integration.
- Real model credentials are in ignored root .env.local. Runtime credentials/config/logs are ignored under docker/. Never print, commit, include in screenshots, or copy them into Docker build contexts. Public POC examples contain variable references, not live values.
- Use the existing lfen-ragflow-poc Compose project and volumes; never recreate credentials or delete volumes during routine development. The public minimal runtime recipe is docker/docker-compose.poc.example.json; this machine uses docker/docker-compose.poc.local.json.
- Source sync imports existing upstream Python files unchanged. Do not treat the upstream Go-only direction as permission to bulk-delete inherited code during unrelated work; no new Python application logic.
- Upstream Actions jobs are guarded to run only for infiniflow/ragflow. Do not enable upstream release/scheduled/self-hosted pipelines for this project as a side effect of code sync. Project CI is a separate task.
- 产品品牌只由web/src/conf.json统一；当前透明手绘Logo在web/public/logo-mascot.png。照片参考原件不入库。

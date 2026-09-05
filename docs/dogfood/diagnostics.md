# Muster Diagnostics — How It Works, What Breaks, The Right Way

> The "explained lessons" record from the 2026-08-07 dogfood run: how the
> system is built, why it behaves the way it does, the errors hit (mine and
> the project's own history), and the right way to do things. Not raw logs —
> explained knowledge.

## Architecture in one screen

Muster is a Go module (`github.com/wojons/muster`, go 1.26) with three entry
points sharing ~22 public packages under `pkg/`:

```
cmd/openapi-cli/    → bin/openapi-cli     (discover/request/generate/dsl/modules/daemon/config/auth...)
cmd/mcp-server/     → bin/openapi-mcp     (stdio JSON-RPC MCP server; every spec op = a tool)
cmd/daemon/         → bin/openapi-daemon  (Unix-socket HTTP proxy; start/proxy/stop/status/restart)
pkg/openapi/        OpenAPI 3.x parser (kin-openapi based)
pkg/generator/      spec → []*cobra.Command (NewGenerator(Config) + GenerateCommands(doc))
pkg/client/         traced HTTP client (retry, rate-limit, headers; Get/Post/Put/Delete)
pkg/mcp/            MCP protocol (NewInitializeHandler names the server)
pkg/daemon/         proxy Controller (Start/Stop/Status/StartWithSocket)
pkg/dsl/            Starlark interpreter (http_get/post/put/delete builtins)
pkg/wasm/           wazero sandbox (modules run, --wasm-plugin transform hooks)
pkg/cache, rate, auth, config, inventory, completion, prompt, ...  16+ more
```

Data flow for every surface: **spec bytes → `pkg/openapi` parse →
`pkg/generator` commands (or `pkg/mcp` tools) → `pkg/client` executes HTTP.**
One pipeline, seven front doors. This is why "it works" is credible: the CLI,
MCP, and library all proved the same pipeline against the same backend during
the dogfood run.

## Verified behaviors and their root causes

### 1. Daemon lifecycle lies (DF-001) — root cause found

`pkg/daemon/proxy.go` `Controller` keeps `running bool` and `startTime` as
**in-process state only** (proxy.go:76-118). `status`/`stop` run as *separate
processes* of the binary, construct a fresh Controller, and therefore always
see `running=false`:

- `status` → `Running: false`, `Start time: 0001-01-01` (zero value) — while
  the proxy serves requests.
- `stop` → `if !c.running { return fmt.Errorf("daemon is not running") }` —
  the live daemon is never signaled, socket never removed.
- `cmd/daemon` `start` prints "PID file: <path>" but the file is not written
  (or is written then lost) — `ls` shows only the socket. The code comment at
  cmd/daemon/main.go:138 even admits `Controller.Start() alone only flips the
  running flag`.

The **right way** already exists elsewhere in the repo:
`internal/builtin/daemon.go:211-250` (the `openapi-cli daemon stop` builtin)
reads the PID file, checks `isProcessRunning(pid)`, SIGTERMs, waits, and
cleans the socket. The `openapi-daemon` binary should reuse that pattern:
persist a real PID file on start, and have Status/Stop read it + signal the
process instead of consulting in-process state. Fix direction is in DF-001.

### 2. `generate` + the persisted command store (DF-003)

`openapi-cli generate <spec>` writes `~/.local/share/openapi-cli/commands.json`
(single-store, keyed by nothing — it holds ONE spec's commands at a time) and
the CLI loads that store at startup, registering every command into the cobra
root. Consequences, all observed:

- Generating for API B silently destroys the store for API A (no warning, no
  `spec_hash` check — the file has a `spec_hash` field that is never
  compared).
- A stray `list-pets` command in `--help` of a freshly built binary was a
  leftover from someone's earlier `generate` — persisted commands are
  indistinguishable from builtins in the help surface.
- After my taskforge generate, help showed `create-project`, `delete-comment`,
  etc.; after regenerating from petstore, help showed `add-pet`,
  `find-pets-by-status`, ...

The right way: warn/confirm when overwriting a store with a different
`spec_hash`; add `generate --list` / `generate --clear`; and decide whether
persisted commands should appear in the root help at all (a namespaced
subcommand or an opt-in flag would keep the surface honest).

### 3. Doc drift (DF-002) — how it happened and how to audit

`docs/guides/commands.md` and `docs/guides/quickstart.md` describe a command
surface (`spec validate/info/convert`, `init --output`, `inventory list
<spec>`, `doctor`) that the binary never had (or no longer has). The
integration guide (docs/integration-guide.md) and README, by contrast, are
accurate — every example I ran verbatim worked. The drift lives in the
"reference" guides, not the "integration" guide. Quick audit recipe for the
foreman: for every `openapi-cli <cmd>` code block in docs/guides/*.md, run it
with `--help` or a dry invocation and grep the binary's actual command list
(`openapi-cli --help`). The real surface: `discover request generate dsl
modules daemon config auth inventory tenant webhook audit completion version`.

### 4. Private repo vs public promises (DF-004)

`go get github.com/wojons/muster@v0.1.0` from a fresh module fails with
`could not read Username for 'https://github.com': terminal prompts disabled`
— the wojons org is private. The v0.1.0 tag exists and is push-verified, but
only credentialed consumers (GOPRIVATE + git credentials) can fetch it. The
documented `replace` to a local checkout is the correct pattern for this
fleet. Note the sequence: GAP-006 (Aug 7) "published" v0.1.0; it unblocks
musterflow (same org, credentials present) but does NOT make the module
publicly consumable — the README quickstart (`git clone
https://github.com/wojons/muster.git`) still fails for outsiders.

### 5. MCP identity + tool generation (verified fixed)

`pkg/mcp/server.go` initialize handler previously hardcoded
`"openapi-cli-mcp"` (GAP-010, fixed f8ac65c6 Aug 7): live handshake now
returns `serverInfo.name == "openapi-mcp"`. Tool generation: each path+method
becomes `snake_case` tools (`POST /projects` → `post_projects`, `GET
/projects/{project_id}/tasks` → `get_projects_{project_id}_tasks`). Path
params become required integer/string args. 22 tools from the Task Forge
spec, 19 from petstore — matches `tools/list` counts.

### 6. DSL interpreter (verified limits)

`pkg/dsl` is a Starlark interpreter with exactly four HTTP builtins returning
`(code, body)` tuples. Observed limits: no `import` (parse error "got illegal
token"), print() goes to structured logs, and the runner dumps all top-level
bindings at script end. Keep scripts to the builtins + core expressions; use
string concat for URLs; don't expect print() to be the output channel (the
final binding dump is).

### 7. Version reporting (GAP-007 verified, one gap left)

Makefile ldflags now target `github.com/wojons/muster/internal/version.*`
correctly — `openapi-cli version` prints `v0.1.0-8-g0d790342`. Gap: the other
two binaries have no `--version` flag at all (`flag provided but not defined:
-version`) → DF-005.

## Errors hit during the run (and what they meant)

| Error | Meaning | Resolution |
|-------|---------|------------|
| `unknown command "spec"` / `"doctor"` | Docs fiction, not a bug | Use discover/request/generate; DF-002 |
| `unknown flag: --output` (init/generate) | Docs fiction | generate persists to its own store; DF-002 |
| `No inventories configured.` (inventory list <spec>) | Command ignores the path arg; expects pre-configured inventories | Not usable as documented; DF-002 |
| `unsupported protocol scheme ""` (MCP tools/call) | Relative `servers[].url` in spec; `-base-url` required | Add `-base-url` (guide §2.1 documents this; confirmed) |
| `Daemon is not running` while it serves | In-process state vs separate process (see §1) | kill manually until DF-001 lands |
| `could not read Username for 'https://github.com'` | Private repo; go get needs credentials | Use replace directive; DF-004 |
| DSL `got illegal token, want primary expression` | `import` unsupported | Remove import; builtins only |
| `flag provided but not defined: -version` (mcp/daemon) | No version flag on those binaries | DF-005 |
| 500 HTML from POST /projects without name | Demo service lacks input validation (KeyError → Flask 500) | Muster relays faithfully; demo-service issue, DF-006c |

## The right way (TL;DR)

1. **To use it:** `make build` → `openapi-cli discover|request` against your
   spec; `openapi-mcp -spec <file> -base-url <base>` for agents; import
   `pkg/openapi` + `pkg/generator` + `pkg/client` for Go; `replace` directive
   for the private repo. All verified working.
2. **To stop the daemon:** `pkill -f openapi-daemon` until DF-001 is fixed —
   the documented `stop` does nothing.
3. **To change the CLI surface:** remember `generate` clobbers the one global
   command store; re-generate after switching specs or the help mixes surfaces.
4. **To test the docs:** every command in the integration guide runs verbatim
   (that guide is trustworthy); the reference guides need the DF-002 audit.

---

## 2026-08-17 deep run — addendum (post DF-001..006, GAP-001..031 fixes)

Status of the 08-07 findings: **DF-001 (daemon control) FIXED** (start detaches
in ~0.05s via setsid re-exec, status shows Running/PID, stop cleans files —
GAP-022), **DF-002 docs fiction mostly FIXED** (`init` implemented GAP-029;
`spec`/`doctor` still absent), **DF-003 command-store clobber FIXED-ish**
(generate now persists per-spec to `~/.local/share/openapi-cli/commands.json`
and the runtime loads it; the store is still global/single, but no silent
re-overwrite observed), **DF-005 version FIXED** on all three binaries
(GAP-020). `openapi-daemon restart` = stop-only (new DF-011).

### How the generate fallback path works (root cause of DF-007/DF-008)

`pkg/generator.GenerateCommands` groups spec operations by resource and builds
one cobra command per operation via `getCommandName` (uses `operationId` when
present) and `buildUsage`/`createRunHandler`. **When `operationId` is absent**
(the demo's own task-forge spec has zero), the fallback derives names from
path segments + summaries — and that fallback has three defects, all observed
in the generated `commands.json`:

1. **List-path builder appends "s" to an already-plural path**: `GET /projects`
   → `/projectss`, `/tasks` → `/taskss`, `/tags` → `/tagss`, `/health` →
   `/healths`. (Ops whose paths came from the operationId path are unaffected.)
2. **Resource names > 8 chars are truncated**: `milestones` → `milestons`,
   `delete-milestone` → `delete-mileston`, param `milestone_id` →
   `milestonId`.
3. **Fallback commands default to `https://api.example.com`** instead of the
   spec's `servers[].url` (13 of 22 task-forge commands got
   `http://localhost:9801`; the 9 fallback ones got api.example.com; top-level
   `base_url` = api.example.com). The operationId path uses the spec URL
   correctly.

Name collisions (DF-008): `create-project` ×3 / `get-project` ×2 / `get-task`
×2 — cobra allows duplicate names, the first registered wins, the rest are
shadowed and unreachable. `create-task` is worse: the store records
`POST /tasks` but the runtime command executes add-comment — the loader and
the persisted store disagree on the same name.

### How the daemon works (root cause of DF-009)

`cmd/daemon` has two modes: `start` (daemonizes via `MUSTER_DAEMON_CHILD=1`
re-exec; the child runs `StartWithSocket` and then serves an internal
management handler that answers every path with `{"status":"ok","message":
"Muster daemon is running"}`) and `proxy` (foreground; runs the real
`httputil.ReverseProxy` to `--backend-url`). `start --backend-url` accepts the
flag, prints "Proxying requests to…", but never wires the reverse proxy —
hence canned responses. **Right way: use `proxy` for traffic, `start` only for
lifecycle.** (GAP-022's acceptance criteria only checked start→status→stop
timing, which is why this slipped through.)

### Right way (updated 2026-08-17)

1. **Use:** `make build` → scratch `HOME` → `generate` only for specs WITH
   operationIds; generic `request` or the MCP server for operationId-less
   specs; `openapi-mcp -spec <f> -base-url <b>` for agents; library pattern in
   `/tmp/dogfood-muster/consumer` (replace directive; `NewGenerator` +
   `GenerateCommands` + `pkg/client`).
2. **Daemon:** `proxy` for proxying, `start`/`status`/`stop` for lifecycle,
   `restart` = stop (re-`start` after).
3. **Typed command args:** path params are positional (`get-project 1`), body
   fields are flags, `--base-url`/`MUSTER_BASE_URL` override everywhere.
4. **Tests:** `go test -short ./...` = 0.36s warm (GAP-026); full suite still
   the e2e gate.
5. **MCP is the most complete typed surface** — 22/22 tools incl. nested
   resources, no collisions, no truncation. Prefer it for full-coverage
   workflows until DF-007/DF-008 land.

## 2026-09-04 addendum — fix-verification run + bunker install leg

**Status change:** DF-007/DF-008 (generator) and DF-009/DF-011 (daemon) verified FIXED by
real use on HEAD 01849718. The "right way" above is now simpler: `generate` is safe on
operationId-less specs, and `start --backend-url` really proxies.

### Why the generator fix works (shape)

Nested same-verb operations now get path-qualified command names
(`create-project-task` vs `create-project`), so a name is unique per (verb, path
template) rather than per (verb, last-segment). The 08-17 mangling class
(`/projectss`, `delete-mileston`) is gone because name derivation no longer
falls back to stem-mangling when operationIds are absent — verified by the
programmatic 22-ops → 21-unique-commands diff (no collisions possible without
drops).

### New root cause found this run: tests clobber the real user store (DF-013)

`demo/test-suites/100-cli-real-integration_test.go` builds the real CLI and exec()s
`generate` via `exec.CommandContext` **without setting HOME/XDG_DATA_HOME for the
child**. The child resolves the store path from the environment → writes the
INVOKING USER's `$XDG_DATA_HOME/openapi-cli/commands.json`, overwriting it with
fixtures. Caught forensically: store `generated_at` (23:32:40) matched the
`go test -short` window exactly, and the file's `spec_path` pointed at
`/tmp/TestRealCLIIntegration3574462870/002/petstore.yaml` — a temp dir only the
test creates. **Lesson:** any test that exec()s the project's own binary must
`t.Setenv("HOME", t.TempDir())` first; otherwise "all tests green" is quietly
destroying user state. This is the third independent instance of the
persisted-store-overwrite family (DF-003 → GAP fix → now the test suite itself).

### Relative server URLs: the last `unsupported protocol scheme` trap (DF-015)

Petstore's `servers[].url` is `/api/v3` (relative). Resolution against the spec's
fetch URL does not happen for typed commands, so execution builds
`Get "/api/v3/pet/1"` → `unsupported protocol scheme ""`. The MCP surface got the
`--spec`-alone resolution fix (DF-MUSTER-1); the CLI surface still needs either
the same resolution or a doc'd `--base-url` note in the quickstart. Until fixed:
**always pass `--base-url`/`MUSTER_BASE_URL` for relative-server specs.**

### Bunker install leg — how a fresh machine actually goes (and what broke)

Order that worked on a bare Debian 13 agent:
1. Toolchain: `curl -LO go.dev/dl/go1.26.5.linux-amd64.tar.gz` → extract to
   `~/toolchain` → PATH. (Docs assume this exists; they shouldn't.)
2. Code: documented `git clone` fails without credentials (private repo — correct
   per access policy; tar-staging from an authorized machine is the sanctioned
   fallback). Without git metadata, `version` prints `dev (unknown)` — cosmetic.
3. Build: `make build-cli`. **Broke once**: `link: mapping output file failed:
   no space left on device` — the 64MB Go tarball still in HOME plus module cache
   hit a per-agent ~1.5G budget at peak. Fix: delete the tarball, rebuild. Docs
   should warn container/CI users about peak build footprint (~1.2G).
4. Smoke: discover+generate on petstore3 (works), typed `get-pet-by-id 1` dies
   with `unsupported protocol scheme ""` (DF-015 — spec has relative server URL),
   with `--base-url` reaches petstore (500 is petstore's own documented breakage),
   generic `request GET` against a local file server returns 200 with intact body.

### Right way (2026-09-04)

1. Build (`make build`), scratch HOME, `generate` any spec — operationIds or not.
2. Relative `servers[].url`? Pass `--base-url` or `MUSTER_BASE_URL`. Always.
3. Run tests ONLY with a scratch HOME until DF-013 is fixed; or re-`generate` after.
4. Daemon: `start --backend-url` proxies, `restart` restarts, `status`/`stop` as
   before. `proxy` mode remains for foreground debugging.
5. MCP unchanged: `-spec` + `-base-url` covers every op, 22/22 tools.

# Muster Diagnostics — How It Works, What Breaks, The Right Way

> The "explained lessons" record from the 2026-08-07 dogfood run: how the
> system is built, why it behaves the way it does, the errors hit (mine and
> the project's own history), and the right way to do things. Not raw logs —
> explained knowledge.

## Architecture in one screen

Muster is a Go module (`github.com/wojons/muster`, go 1.26) with three entry
points sharing ~26 public packages under `pkg/`:

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

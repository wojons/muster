# Dogfood Integration Report — 2026-09-25 (run 4): today's fixes + the new protocol seams

**HEAD at run time:** 3d0be0da · **Verdict:** ✅ SHIPPABLE (core surfaces) with the
install leg explicitly SKIPPED (infra down) · **Time-to-first-success:** < 2 min
(`init` → first successful `request`).

This run is a **fix-verification + new-surface run**. Runs 1-3 covered CLI CRUD,
MCP, daemon, Go lib, Docker, WASM, DSL, reload, and the management plane. This run
used the three DF fixes merged earlier today and the two protocol adapters merged
today (PROTO-004 JSON-RPC, PROTO-002 gRPC) against **real servers I built for the
purpose** — not test fixtures.

## What a real user does, end to end (all verified live)

### 1. Fresh user bootstrap (isolated HOME)

```bash
HOME=/tmp/df-home ./bin/openapi-cli init      # creates ~/.config/muster/config.yaml
HOME=/tmp/df-home ./bin/openapi-cli config show
```

### 2. DF-021 fix verified — tenants no longer brick when profiles exist

Before the fix (commit ec67cf2a), defining any `profiles:` block in config.yaml
made `tenant set/switch` reject every known tenant. Now:

```bash
# with profiles: {production:…, development:…} present in config.yaml:
openapi-cli tenant set acme-corp      # -> Set active tenant to acme-corp
openapi-cli tenant switch acme-corp   # -> Switched to tenant acme-corp
openapi-cli tenant list               # -> * acme-corp [free]
```

### 3. DF-022 fix verified — profile switch persists; precedence chain is real

The docs' path (hand-edit `~/.config/muster/config.yaml`, add a `profiles:` block)
then:

```bash
openapi-cli config profile switch production
# -> Switched to profile: production
# writes `default_profile: production` into config.yaml (persists across runs)

openapi-cli config show
# -> Active Profile: production (source: persisted)

MUSTER_PROFILE=development openapi-cli config show
# -> Active Profile: development (source: env)

openapi-cli --profile development config show
# -> Active Profile: development (source: flag)
```

The flag > env > persisted precedence documented in configuration.md works, and
`config show` names the source — genuinely useful.

### 4. DF-023 fix verified — stored credentials attach to requests (negative-tested)

Against a bearer-requiring mock (the repo's `demo/services/inkwell`, :9803):

```bash
openapi-cli auth add --name ink-admin --type bearer --value admin-token
openapi-cli request GET http://127.0.0.1:9803/admin/stats          # -> 401 (no cred)
openapi-cli request GET http://127.0.0.1:9803/admin/stats --auth prod-key  # -> 401 (wrong)
openapi-cli request GET http://127.0.0.1:9803/posts --auth ink-admin       # -> 200 + JSON
```

Generated typed commands carry the same `--auth` flag (parity confirmed by the
merge's `authflag_parity_test.go`, confirmed live with `list-tasks --auth …`).

### 5. PROTO-004 JSON-RPC seam — used against a live OpenRPC server

A real JSON-RPC 2.0 service (`/rpc` on :9911) serving an OpenRPC discovery doc
with `add(a,b)` and `echo(msg)`:

```bash
openapi-cli protocol jsonrpc --doc http://127.0.0.1:9911/rpc
# -> neutral operation list: id, kind=unary, parameters with types, source.server

openapi-cli protocol jsonrpc --doc http://127.0.0.1:9911/rpc --method add --args '{"a":2,"b":40}'
# -> 42

openapi-cli protocol jsonrpc --doc http://127.0.0.1:9911/rpc --method nope
# -> Error: method "nope" not found in the discovery document (discovered: add, echo)   [exit 1]
```

Friction found: `--endpoint` cannot be used *without* `--doc` even though its help
text implies it ("default: the document's servers[0].url") — filed as DF-025.

### 6. PROTO-002 gRPC seam — .proto ingestion AND server reflection, real server

A real gRPC server with reflection on :9912 (`calc.v1.Calculator`, unary `Add`
+ server-streaming `StreamSum`):

```bash
# discovery from a .proto file:
openapi-cli grpc discover "/tmp/df=calc.proto"
# unary            calc.v1.Calculator-Add
# server-streaming calc.v1.Calculator-StreamSum

# discovery from server reflection (no files needed):
openapi-cli grpc discover grpc://127.0.0.1:9912     # same output

# invocation via reflection:
openapi-cli grpc call calc.v1.Calculator-Add --source grpc://127.0.0.1:9912 --arg a=20 --arg b=22
# -> { "sum": "42" }

# invocation via the .proto + explicit target:
openapi-cli grpc call calc.v1.Calculator-Add --source /tmp/df/calc.proto --target 127.0.0.1:9912 --arg a=1 --arg b=2
# -> { "sum": "3" }

# streaming is cleanly rejected (documented PROTO-003 boundary):
openapi-cli grpc call calc.v1.Calculator-StreamSum --source grpc://127.0.0.1:9912 --arg n=3
# -> Error: VALIDATION_ERROR: streaming methods are out of scope for the gRPC unary gate (PROTO-003)

# bad argument types produce a clean typed error:
# -> VALIDATION_ERROR: arguments do not match message calc.v1.AddRequest … strconv.ParseInt …
```

Friction found: the file half of `dir=file.proto` resolves against the **current
working directory**, not the declared `dir=` — works only if cwd equals the
declared dir, or with an absolute file path. Filed as DF-026.

### 7. MCP server still good after the merges

`openapi-mcp -spec http://127.0.0.1:9801/openapi.json` → initialize OK, 22 tools
listed, `tools/call get_tasks` returns real rows. (`tools/list` without `-spec`
returns "Method not found" — load a spec first.)

## What did NOT get exercised (honest gaps)

- **Fresh-machine install** — every bunker agent was down (see SKIPPED-install-bunker
  board row). go.mod gained gRPC deps today, so install-at-HEAD is unproven.
- gRPC client-streaming / bidi (out of scope by design, PROTO-003).
- JSON-RPC batches (explicitly unimplemented, documented as such).

## Performance (measured, warm, hyperfine 20 runs)

| operation | mean ± σ |
|---|---|
| typed command `list-tasks` | 11.3 ms ± 0.7 ms |
| `grpc call` end-to-end (reflection discovery + call) | 11.6 ms ± 0.9 ms |
| `protocol jsonrpc` discover+call | ~10 ms |
| cold `generate <spec-url>` | 0.019 s |

Nothing a user would notice. No PERF rows filed — deliberately.

## Errors hit while using (and their fixes/meanings)

| error | meaning |
|---|---|
| `auth add prod-key --type …` → `--name is required` | name is flag-only; docs consistently use `--name` — correct behavior, wrong guess on my part |
| `--doc is required` on endpoint-only jsonrpc call | real gap, DF-025 |
| `NOT_FOUND: read proto file calc.proto` with `dir=file.proto` | real gap, DF-026 |
| `VALIDATION_ERROR: streaming methods are out of scope…` | correct, documented PROTO-003 boundary |

# Muster ↔ a live OpenAPI API: the proven recipe (2026-09-26 dogfood, run 11)

This is the "bring your own live spec" record CONSENSUS-INT-001 asked for. It was
proven against a **real foreign service** — the consensus agent runtime
(60-path OpenAPI spec, bearer-token auth, live SQLite-backed server) — not a
mock. The CLI leg worked end-to-end in under two minutes from a clean HOME.
The MCP and DSL legs hit a shared wall (no auth surface), documented below.

## What was proven (CLI, all green against the live server)

```bash
export HOME=/tmp/df-muster-home            # any clean HOME works
openapi-cli init

# 1. Discover — works from a served URL or a spec file alike:
openapi-cli discover https://host/openapi.json        # 60 paths listed, 28ms warm
openapi-cli discover /path/to/bundled.yaml            # 85 operations listed

# 2. Generate typed commands, pinning the live base URL:
openapi-cli generate /path/to/openapi.json --base-url http://127.0.0.1:8124
#   -> 70 commands registered and persisted, zero name collisions

# 3. Store a credential once (keychain-backed):
openapi-cli auth add --name consensus-admin --type bearer --value cs_ak_...

# 4. Real workflow against the live API:
openapi-cli health-check --base-url http://127.0.0.1:8124              # 200
openapi-cli create-session --base-url http://127.0.0.1:8124 \
    --auth consensus-admin --agent_name df-agent --goal "smoke"        # 201
openapi-cli send-message <session-id> --base-url http://127.0.0.1:8124 \
    --auth consensus-admin --content "ping"                            # 200
openapi-cli list-sessions --base-url http://127.0.0.1:8124 \
    --auth consensus-admin --output json                               # 200

# 5. Error paths behave (exit 1, one-line typed error):
openapi-cli get-session 00000000-... --auth consensus-admin   # 404 NOT_FOUND
openapi-cli list-sessions (no --auth)                         # 401 UNAUTHENTICATED
```

Env-variable path (script-friendly): `MUSTER_BASE_URL` substitutes for
`--base-url` and `MUSTER_AUTH_TOKEN` attaches a bearer token for generated
commands.

## Where the promise currently stops

1. **The MCP server has no auth surface (DF-036, P1).** `openapi-mcp` accepts
   `--spec` and `--base-url` and nothing else; `pkg/mcp/openapi_integration.go`
   sets only `Content-Type`. `MUSTER_AUTH_TOKEN`, which the CLI path honors, is
   ignored here. Every authenticated `tools/call` against a bearer API returns
   the upstream 401 as an error-text blob. tools/list works fine — the failure
   only appears once you actually invoke a protected tool.
2. **The Starlark HTTP builtins carry no headers (DF-037, P2).**
   `http_get/post/put/delete` (internal/builtin/dsl.go) accept only url/body, so
   an authed create→message→poll workflow is impossible; the 401 comes back as
   response body text.
3. **MCP error rendering is flat text (DF-038, P2).** Both successes (raw JSON)
   and failures ("Error: HTTP 401: {…}") arrive as a single text content blob,
   giving an agent consumer no structured signal about whether muster (wiring)
   or the upstream (real error) is at fault.
4. **`create-session` echoes the returned session key in cleartext (DF-039, P3).**

Until 1 and 2 are fixed, the honest summary is: **use the CLI for authenticated
APIs; the MCP/DSL surfaces are usable only against public endpoints.**

## Measurements (2026-09-26, HEAD d144fc70)

- `hyperfine --warmup 3 --runs 20 'openapi-cli discover <60-path spec>'`
  → 26.1 ms ± 2.1 ms warm
- cold `generate` (fresh store): 27 ms
- full authed `list-sessions` round-trip vs the live server: 106 ms
Nothing user-noticeable is slow.

## Integration notes / gotchas

- Consensus' bootstrap prints the admin key (`cs_ak_…`) **once**, on stdout, at
  first serve (or `consensus init`); it is never recoverable afterwards. Capture
  it from the serve log at bootstrap time.
- The generated command set (70 here) comes from operationIds; consensus'
  spec has full operationId coverage, so zero collisions — unlike older specs
  (see DF-007 history).
- `--auth` names a stored credential; it must be passed on **every** typed
  command invocation (there is no per-profile default credential yet).
- 401s surface the upstream body verbatim; when you see
  `UNAUTHENTICATED missing API key` it means muster sent no credential at all —
  check that `--auth` (CLI) was actually passed; on MCP there is currently no
  way to pass one (DF-036).

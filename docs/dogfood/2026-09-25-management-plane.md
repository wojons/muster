# Dogfood Integration — 2026-09-25 (2nd run): the management plane

Second dogfood run on 2026-09-25. Prior runs (see `2026-08-07`, `2026-08-17`,
`2026-09-04`, and the second-session run logged in `.coding-hermes/dogfood-log.md`)
covered the data plane: discover/generate, typed CRUD, MCP, daemon, Go library,
Docker, WASM, Starlark, and the persisted-store reload. This run took the surface
no previous run had touched: **everything a user configures rather than requests**
— config profiles, credentials, tenants, completion, inventories, audit, webhooks,
mTLS.

## How it was run

```bash
export HOME=/tmp/df-muster-3-home          # isolated state, throwaway
make build-cli                              # v0.1.1-98-g5f24a1af
python3 .venv/bin/python3 demo/services/task-forge/server.py 9801 &   # CRUD mock
python3 .venv/bin/python3 demo/auth-service/server.py 9803 &          # bearer-auth mock
```

Then, in order: `init` → `config show` → `tenant set/switch/list/current` →
`config profile switch` (docs example) → hand-edit `config.yaml` with the
`profiles:`/`aliases:` blocks from `docs/guides/configuration.md` →
`auth add --type api-key` and `--type bearer` → `request` against both mocks →
`inventory add/list` → `webhook create/list/test/delete` →
`MUSTER_AUDIT_ENABLED=1` activity + `audit log` → `completion bash` →
`auth mtls generate/info`.

## What works (verified live)

| Command | Result |
|---|---|
| `openapi-cli init` | writes a sane default config.yaml |
| `config show` | correct dirs/version |
| `tenant set/switch/list/current` (fresh HOME, no profiles) | works, persists to tenants.json, active marker survives processes |
| `auth add/list` | stores credential metadata (credentials.json) |
| `inventory add/list` | works, persists inventories.json |
| `webhook create/list/delete/test` (by wh-<id>) | works; `test` probes the URL and reports unreachable cleanly |
| `completion bash` | 1627-line script, loads and sources cleanly |
| `auth mtls generate/info` | generates a valid self-signed cert pair |
| `request -H 'Authorization: Bearer <tok>'` | correct request/exit-code behavior |

## What does NOT work (board rows DF-021..024)

1. **DF-021 (P1) — profiles brick tenants.** After adding ANY `profiles:` block
   to config.yaml, `tenant set/switch <id>` fails with `unknown tenant "<id>"`
   for tenants `tenant list` still shows. Root cause:
   `internal/builtin/extended.go:322-329` (`setActiveTenant`) validates the id
   against `cfg.Profiles` once any profile exists, while `tenant list` reads the
   tenants store. Lesson: when two commands share a concept, they must share a
   validation source.
2. **DF-022 (P2) — profile switch is a no-op that says it worked.**
   `config profile switch development` prints success, changes no file, and
   `config show` never reflects it. The documented global `--profile` flag does
   not exist; `MUSTER_PROFILE` is ignored. There is currently NO working way to
   activate a profile.
3. **DF-023 (P2) — stored credentials are never used.** `auth add` succeeds, then
   the same request 401s with no way to attach the credential (no flag, no env,
   no auto-wiring). The troubleshooting doc's 401 remedy dead-ends on
   nonexistent `config show auth`. Only manual `-H` works.
4. **DF-024 (P3) — drift bundle.** Documented `aliases:` never resolve;
   `config validate/check/hierarchy` print generic help; webhook
   delete/test reject the NAME that create/list accept; audit logging enabled
   writes zero events; `auth mtls generate` drops cert/key material into CWD
   (untracked, not gitignored — delete it if you ran this from a repo).

## Right way (until fixed)

- Tenants: do not define config profiles if you rely on tenant switching.
- Auth: skip `auth add` for actual requests; use `-H` directly.
- Profiles: they are inert documentation right now.
- Webhooks: capture the `wh-<id>` from create; delete/test by ID only.

## Diagnostic trail

- Method: isolated `HOME`, live mocks on loopback ports, every claim executed
  as a command with its exit code captured; file-level evidence (mtimes, file
  contents) for every persistence claim; source read only to locate the DF-021
  root cause after behavior was established.
- Same-day second run is deliberate: the skill's angle-change doctrine — the
  first 2026-09-25 run took the reload surface, this one took the management
  plane. Different surface, zero overlap.
- Perf: `hyperfine --warmup 3 --runs 20 './bin/openapi-cli tenant current'`
  → 31.9 ms ± 21.3 ms warm / 27.9 ms cold. Nothing user-noticeable is slow;
  no PERF rows (third consecutive run with that result).

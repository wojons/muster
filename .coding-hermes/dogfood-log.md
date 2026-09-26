# Dogfood Log — Muster

| Date | Verdict | Promise | Time-to-first-success | Top findings |
|------|---------|---------|----------------------|--------------|
| 2026-08-07 | 🟡 PROMISING-BUT-ROUGH | Turn any OpenAPI spec into 7 surfaces instantly (CLI, MCP, daemon, Go lib, Docker, WASM, Starlark) | ~6 min (fresh `make build` + petstore discover/request) | 1. Daemon start/status/stop control channel broken (DF-001, P1). 2. Guides document nonexistent commands — `spec`, `doctor`, `init --output` (DF-002, P2). 3. `generate` silently overwrites persisted command store; stale commands leak into `--help` (DF-003, P2). |
| 2026-08-17 | 🟡 PROMISING-BUT-ROUGH | Turn any OpenAPI spec into a CLI, MCP server, and Go library instantly | ~4 min (build + version/init/discover + first typed create against local mock) | 1. `generate` on operationId-less specs (incl. demo's own task-forge) mangles paths (/projectss, /milestons) + defaults to api.example.com (DF-007, P1). 2. Command name collisions shadow ops — create-task runs add-comment, POST /tasks unreachable (DF-008, P1). 3. daemon `start --backend-url` serves canned status instead of proxying; `restart` = stop-only (DF-009/011, P1/P2). |

## 2026-08-07 — Deep run (all 7 surfaces)

**Promise statement:** *"Muster claims that a user can turn any OpenAPI spec into seven working surfaces instantly — CLI (`discover`/`request`), MCP server (every operation = a tool), Unix-socket proxy daemon, Go library, Docker image, WASM plugins, Starlark DSL — no codegen, minutes from spec to usable tooling."*

**What was done (real use, not tests):**
1. Fresh `make build` → 3 binaries; `openapi-cli version` prints `v0.1.0-8-g0d790342` (GAP-007 verified).
2. CLI: full CRUD session against local Task Forge mock (list → create → read → add task → patch → 404/500 error paths) + README quickstart verbatim against public petstore (both work).
3. MCP: raw JSON-RPC handshake over stdio — initialize (serverInfo.name=openapi-mcp, GAP-010 verified), tools/list (22 tools), tools/call post_projects + get_projects (created real project).
4. Daemon: proxy mode serves GET/POST through Unix socket; **start/status/stop control channel broken** (DF-001).
5. Go library: fresh consumer in /tmp with `replace` — parsed spec (pkg/openapi), generated 6 CLI commands (pkg/generator), executed GET/POST via pkg/client. Success.
6. Docker: `docker build` + CLI discover/request + MCP tools/list in container (19 tools).
7. WASM: `modules run hello.wasm --fn hello` → [42]; add.wasm 2+3 → [5].
8. Starlark DSL: 6-step workflow (create project → query → create task → list → delete) all executed; `import` unsupported (documented in diagnostics).

**Findings written to board:** DF-001 (P1), DF-002 (P2), DF-003 (P2), DF-004 (P2), DF-005 (P3), DF-006 (P3) in `.coding-hermes/board/tasks.jsonl`.

**Foreman:** NOT paused (CooldownS=7200, Enabled=true, ticking) — no wake needed; tasks picked up on next tick.

## 2026-08-17 — Deep run (2nd, post GAP-001..031)

**Promise statement:** *"Muster claims a user can turn any OpenAPI spec into
typed CLI commands, an MCP tool surface, a Unix-socket proxy daemon, a Go
library, WASM plugins and Starlark workflows — instantly, no codegen."*

**What was done (real use, not tests):** fresh `make build`; isolated HOME;
two task-forge mocks (:9801/:9811). CLI: version/init/discover/generate +
typed CRUD (create-project 201, get-project 1 200, --base-url switch to :9811
verified). MCP: initialize + tools/list (22 tools) + full CRUD round-trip +
error surfacing. Daemon: --version/--help/start (0.054s detach)/status/stop
all fixed-verified; `proxy` mode real traffic; start-mode proxy broken.
WASM auto-export-select verified; Starlark 6-step workflow; Go library
consumer (/tmp/dogfood-muster/consumer) parsed + generated + executed the
generated tree + direct pkg/client POST; `go test -short ./...` 0.36s warm.

**Findings written to board:** DF-007 (P1), DF-008 (P1), DF-009 (P1),
DF-010 (P2), DF-011 (P2), DF-012 (P3) in `.coding-hermes/board/tasks.jsonl`.

**Foreman:** NOT woken — fleet.toml operator pin cooldown_s=21600 (6h);
prior stand-in cycles established the convention "never PUT below operator
pin" (GAP-029/030/031 reasoning G5). Foreman is enabled and ticking
(last tick 2026-08-17 12:46); it will pick up DF-007..012 on the next
natural tick.
2026-09-01 | PROMISING-BUT-ROUGH | 5s t2fs | friction 11 | 5 findings

## 2026-09-04 — Fix-verification run + first bunker install leg

**Promise statement:** *"Muster claims a user can turn any OpenAPI spec into a
CLI, an MCP server, and a Go library instantly — no codegen."*

**What was done (real use, not tests):** `./demo/run-all.sh` 23/23 PASS (~163s);
fresh `make build-cli` (4s, v0.1.1-57-g01849718); full CLI CRUD round-trip vs
live task-forge mock incl. the formerly-shadowed nested `create-project-task`;
programmatic generator inventory check (22 spec ops → 21 unique commands, zero
missing, zero collisions); daemon start-mode proxies REAL traffic, restart
restarts (DF-009/011 verified); MCP 22/22 tools; `go test -short` 70s cold
45 pkgs 0 FAIL. Bunker leg: fresh agent on las-bunker-03, manual Go toolchain
install, documented `make build-cli` (broke once: transient "no space left on
device" — tarball cleanup fixed), smoke PASS (discover/generate/typed
--base-url/request 200).

**Fix verification:** DF-007, DF-008, DF-009, DF-010, DF-011, DF-012 — ALL six
confirmed actually fixed on HEAD 01849718.

**Findings written to board:** DF-013 (P1, tests clobber real user command
store — caught live), DF-015 (P2, relative-server specs die client-side and
docs don't mention --base-url) in `.coding-hermes/board/tasks.jsonl`;
SPAWN-REG-001 (P1, bunkerd in-memory registry loss / orphaned port ranges) on
the bunker board, cross-referenced as DF-014 here.

**Foreman:** NOT woken — operator pin cooldown_s=604800 in ~/.hermes/fleet.toml;
standing convention "never PUT below operator pin" (GAP-029/030/031 reasoning
G5). muster foreman enabled, last tick 2026-09-02 23:26. New rows surface at
the next natural tick.

2026-09-04 | SHIPPABLE | install_seconds=67 | bunker=las-bunker-03 agent=6ce1acfc | smoke=ok | t2fs ~5min local / ~12min bunker | friction 3 | 3 findings (DF-013, DF-014→bunker, DF-015)
2026-09-25 | PROMISING-BUT-ROUGH | install_seconds=51 | bunker=las-bunker-03 agent=bf29dbe1 (hand-run legs; standard battery blocked by QA-HARNESS-DF-020) | smoke=ok | t2fs ~5min local (build+discover+typed request), reload-format defect found in session 2 | friction 3 | 4 rows (DF-017, DF-018, DF-019, QA-HARNESS-DF-020)

## 2026-09-25 — Second-session surface (persisted store reload) + fresh-install leg

**Promise statement:** "Muster claims a user can turn any OpenAPI spec into a CLI
instantly; typed commands persist to storage and reload on subsequent CLI
invocations with the same behavior."

**What was done (real use, not tests):** angle change per the skill (runs 1-5
covered CLI/CRUD/MCP/daemon/Go-lib/Docker/WASM/DSL + install) — this run took
the SECOND-SESSION surface nobody had used: `generate` → new process → reload
from the persisted store. Isolated HOME (/tmp/dogfood-muster-home), live
task-forge mock on :9801, a logging reverse proxy on :9802 for wire evidence.
Verified: discover 0.04s cold, generate 0.03s cold, 22 commands persisted,
typed CRUD (create 201, get 200, 404 exit 1), `generate --clear` +
`unknown command` after clear, regenerate restores. hyperfine 20 runs:
35.1ms ± 16.4ms warm. Found the reload path ignores -o/--output (raw body
dumped for table/yaml) and `--raw` leaks as ?raw=true on the wire —
DF-016's claim, now proven with a captured request line (DF-017).

**Install leg:** standard bunker-qa battery emitted a 0-byte generated script
(harness bug: unquoted heredoc executes backticked `docker pull` from a
comment; DETECT_UP_PREV_DIR unbound for Go repos under set -u) — filed as
QA-HARNESS-DF-020, battery leg declared SKIPPED-with-reason. Manual install
leg run instead on the same class of fresh agent (Debian 13, bf29dbe1,
las-bunker-03): Go bootstrap 4s, documented `make build-cli` 51s,
discover/generate/simple-test smoke PASS; DF-017's -o yaml defect reproduced
on the fresh install. Agent destroyed, key removed.

**Findings written to board:** DF-017 (P2), DF-018 (P3), DF-019 (P3),
QA-HARNESS-DF-020 (P2, harness) in `.coding-hermes/board/tasks.jsonl`,
committed 466f0d1f, findings section 58ad05d7, pushed to origin/main.

**Foreman:** NOT woken — operator pin cooldown_s=21600 in fleet.toml; standing
convention "never PUT below operator pin" (GAP-029/030/031 G5). Foreman
enabled; rows surface at the next natural tick.

**No PERF rows filed:** nothing user-noticeable is slow (sub-40ms warm CLI,
sub-1s cold generate); install time recorded above as a number.

## 2026-09-25 (2nd run, same day) — management-plane surface

**Promise statement:** "Muster's management plane works as documented: config
profiles, keychain-backed credentials, multi-tenant context, shell completion,
spec inventories, audit logging, webhook subscriptions, mTLS."

**What was done (real use, not tests):** angle change per the skill (prior runs
covered CLI CRUD/MCP/daemon/Go-lib/Docker/WASM/DSL/reload + install). Isolated
HOME (/tmp/df-muster-3-home), live task-forge mock (:9801) and the repo's own
auth-service mock (:9803, bearer). Worked the surface a real user works after
the first successful request: `init` → config show → tenant set/switch/list/
current → config profile (docs example 'production') → hand-edited profiles +
aliases per configuration.md → auth add (api-key + bearer) → request vs auth-
protected endpoint → inventory add/list → webhook create/list/test/delete →
audit with MUSTER_AUDIT_ENABLED=1 → completion bash → auth mtls generate/info.
READ SOURCE when stuck (allowed): located the tenant-validation bug at
internal/builtin/extended.go:322-329.

**Findings written to board:** DF-021 (P1, profile definition bricks tenant
switching), DF-022 (P2, profile switch persists nothing; --profile flag and
MUSTER_PROFILE env nonexistent/ignored), DF-023 (P2, stored credentials never
attached to requests; troubleshooting 401 path dead-ends on nonexistent
`config show auth`), DF-024 (P3, drift bundle: aliases, config
validate/check/hierarchy, webhook name-vs-id, audit writes zero events, mtls
key material in CWD). Works as promised: init, config show, inventory,
webhook lifecycle, auth add/list storage, completion (1627-line bash script
loads), mTLS cert generation.

**Install leg:** not re-run — same-day run on the same HEAD; installability
proven ~15h ago (commit 466f0d1f/5f24a1af, manual fresh-agent leg PASS).
No new install surface changed since.

**No PERF rows filed:** tenant/config-store command 31.9ms ± 21.3ms warm,
27.9ms cold — nothing a user would notice.

## 2026-09-25 (3rd run, same day) — fix-verification + new protocol seams

**Promise statement:** "Today's merged fixes (DF-021/022/023) work as a real
user would experience them, and the two new protocol seams (PROTO-004
JSON-RPC, PROTO-002 gRPC) can be driven end-to-end against real servers a
user would plausibly have."

**Angle change per skill:** runs 1-2 today covered the entry surface and
management plane; this run targeted the fixes merged after them (HEAD moved
5f24a1af → 3d0be0da: ec67cf2a, dc36ca9a, 290b94c8, fecee3c4, 5375a106) plus
the never-dogfooded protocol seams.

**What was done (real use, not tests):** isolated HOME (/tmp/df-muster-4),
live task-forge mock (:9801), repo inkwell auth mock (:9803), plus two
servers built for the run: a real JSON-RPC 2.0 service with OpenRPC doc
(:9911, add/echo) and a real gRPC server with reflection (:9912,
calc.v1.Calculator, unary Add + server-streaming StreamSum).
- DF-021 VERIFIED: tenant set/switch works with profiles defined (was the P1 brick).
- DF-022 VERIFIED: `config profile switch` persists default_profile; `config
  show` reports source (flag > env > persisted chain proven live).
- DF-023 VERIFIED with negative tests: no cred → 401, wrong cred → 401,
  stored `--auth` cred → 200; typed generated commands carry --auth parity.
- PROTO-004 VERIFIED: discover (typed params) + calls (add → 42, echo round-trip)
  + clean unknown-method error. Friction: --endpoint-only call rejected (--doc
  mandatory) → DF-025.
- PROTO-002 VERIFIED: discovery via .proto AND reflection agree; unary calls
  via both sources ({"sum":"42"} / {"sum":"3"}); streaming cleanly rejected per
  PROTO-003; bad arg type → clean typed error. Friction: `dir=file.proto`
  resolves file against CWD not dir= → DF-026.
- MCP re-check post-merges: initialize OK, 22 tools, tools/call returns real rows.
- Generated-command path re-run: discover + generate + typed list-tasks OK.

**Findings written to board (commit 27205f30):** DF-025 (P2, jsonrpc
endpoint-only rejected vs help text), DF-026 (P2, grpc dir= resolution),
DF-027 (P3, no `config profile create` subcommand — first profile requires
hand-editing YAML), SKIPPED-install-bunker (P2).

**Install leg: SKIPPED-install-bunker with evidence.** las-03 offline
(ssh timeout ×4, tailscale offline 32m+); las-02 reachable but bunkerd
crash-looping at restart counter 22897 ("refusing to bind non-loopback
plaintext listener :10002,:10001: set tls.enabled" + "audit chain-head
recovery skipped: record hash mismatch (tampered)"); las-01 offline 3h,
las-04 offline 8h. Install surface DID change today (go.mod/go.sum gained
gRPC deps in 5375a106), so installability is UNPROVEN at current HEAD —
next dogfood must re-run the leg.

**No PERF rows filed:** warm typed command 11.3ms ± 0.7ms, grpc call
end-to-end 11.6ms ± 0.9ms, jsonrpc call ~10ms, cold generate 0.019s
(hyperfine 20 runs) — nothing a user would notice.

**Time-to-first-success:** < 2 min (init → first successful request).

| 2026-09-25 (run 4) | PROMISING-BUT-ROUGH | install_seconds=3 (tarball path, warm deps; 96s cold incl. toolchain+modules; DF-035 breaks the git-init'd-tarball variant) | bunker=bunker-mvp agent=b430959d | smoke=ok | t2fs ~3min (build→discover→typed request; MCP client connect works pre-SDK-crash-fix only for lenient clients) | friction 6 | 5 rows (DF-031, DF-032, DF-033, DF-034, DF-035) |
| 2026-09-26 (run 11) | PROMISING-BUT-ROUGH | CLI chain vs LIVE consensus proven end-to-end (discover→generate→auth→201/200/404/401); MCP+DSL cannot reach authed APIs at all | ~2min (clean HOME → first 201 create-session) | 1. DF-036 P1 openapi-mcp has no auth surface — every authed tools/call 401s. 2. DF-037 P2 DSL http builtins carry no headers — authed workflow impossible. 3. DF-038 P2 MCP errors are flat isError text blobs. | friction 3 | 5 rows (DF-036..040); install leg SKIPPED-install-bunker (all bunkers down/activating; install surface drifted via d9d38a9b) |


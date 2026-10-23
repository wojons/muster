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

| 2026-09-27 (run 12) | SHIPPABLE | Fix-verification run: MCP auth (DF-036) live-proven (201s via token-file AND env vs 401 without), DSL headers (DF-037) 201/401/401, integration-guide §8 (DF-040) re-proved on 3 backends, middleware survives SetHTTPClient (DF-042, external consumer) — but DF-039 masking landed on ONE of TWO render paths (DF-043 P1: reload path leaks cs_*/Bearer in table+json) | ~4min (build→discover→generate→first MCP 201) | friction 5 | 3 rows (DF-043 P1, DF-044 P3, DF-045 P3) + las-03 SKIPPED-install-bunker (evidence: ssh timeout; sibling bunker-mvp PROVED install: bundle→clone→bootstrap 90s→make build-cli 120s cold→discover smoke rc=0); no PERF rows (warm 10.9ms ± 0.7ms / cold 35.5ms typed command, 4th fast run) |

## 2026-10-02 (run 13) — fix-verification + install-leg re-check

**Promise statement:** "REVIEW-MUSTER-3 fix (bare openapi-mcp refusal) works as documented, fresh clone builds clean, and install leg status unchanged."

**Angle change per skill:** last run (2026-09-27) verified MCP auth + DSL headers; this run targets the REVIEW-MUSTER-3 fix merged 5 days ago (commit dc51bf58) plus a fresh-clone build verification.

**What was done (real use, not tests):**
- REVIEW-MUSTER-3 VERIFIED: bare `openapi-mcp` (no -spec) exits 1 with "no MCP tools registered: ... Pass -spec <path|url>" on stderr, no JSON-RPC served. With `-spec demo/openapi-specs/petstore.yaml`, initialize + tools/list returns 5 tools (getPet, updatePet, deletePet, listPets, createPet) with correct inputSchema and annotations.
- Fresh clone from GitHub (depth 1, no auth) builds clean with Go 1.26.6 + CGO_ENABLED=1. README still claims "PRIVATE + GOPRIVATE required" (REVIEW-MUSTER-4 still pending, not reproduced — anon HTTPS clone works).
- Bunker install leg: las-02 and las-03 reachable via ssh, but `systemctl --user status bunkerd` returns "Unit bunkerd.service could not be found" — install leg still blocked, same as last run.

**Findings written to board:** none (all findings already on board as REVIEW-MUSTER-2/4/5, all P2/P3 polish).

**Install leg: SKIPPED-install-bunker with evidence.** las-02/03 reachable but bunkerd not running as systemd --user service. Install surface unchanged since last run (no new deps in go.mod since 2026-09-27).

**No PERF rows filed:** MCP initialize + tools/list < 50ms end-to-end (stdio, no network), fresh-clone build ~8s (warm deps). Nothing a user would notice.

**Time-to-first-success:** < 1 min (fresh clone → build → MCP tools/list).

**Verdict:** SHIPPABLE. REVIEW-MUSTER-3 fix verified live. Remaining pending rows are P2/P3 polish (version stamp, README cleanup, demo hints). Install leg still blocked (bunkerd not running on reachable hosts).

## 2026-10-10 (run 14) — SOAP adapter live interop (post PROTO-008 landing)

**Promise statement:** "The freshly landed SOAP/WSDL adapter (PROTO-008, merged 2026-10-08) works against a real WSDL document and a live SOAP service, and the existing CLI/MCP surface still works."

**Angle change per skill:** PROTO-008 landed 2 days ago; last run (10-02) predated it. This run targets the new `protocol soap` surface against a LIVE public service, not loopback fixtures.

**What was done (real use, not tests):**
- Built all binaries (make build, clean).
- Ingested a real public WSDL 1.1 (NumberConversion, dataaccess.com, fetched over HTTPS): `protocol soap --wsdl` discovers 2 operations (NumberToWords, NumberToDollars) with correct typed parts, endpoint, soap_version 1.1 — ingestion WORKS.
- Live call FAILED: `Invalid namespace '' expected 'http://www.dataaccess.com/webservicesserver/'`. --document shows the payload element is NOT namespace-qualified; hand-fixed envelope via curl returns the correct result → service is fine, adapter envelope is the defect. Filed DF-048 P1.
- Secondary: integer arg renders as `1.234567e+06` in the envelope (not a valid xsd:integer); service rejection mapped to INTERNAL_ERROR instead of BAD_REQUEST/client-error. Filed DF-049 P2.

**Perf (Step 2b):** soap call round-trip 0.30s / 0.27s warm (network-bound, failing fast); `discover` 0.01s; `generate` 0.01s; MCP initialize+tools/list <10ms stdio. Nothing slow enough that a user would notice → no PERF rows filed.

**Install leg: SKIPPED-install-bunker.** Not attempted this run: previous two runs (09-26, 10-02) found bunkerd not running as a systemd --user service on all reachable hosts (las-02/las-03: "Unit bunkerd.service could not be found"); no evidence that changed. Install surface unchanged (no new deps in go.mod affecting bootstrap since 09-27 run's proven install on bunker-mvp).

**Findings written to board:** DF-048 (P1), DF-049 (P2) — committed f4c13c2e, pushed to origin/main.

**Time-to-first-success:** ingestion < 1 min from clone of a public WSDL; first SUCCESSFUL live SOAP call: not achievable (DF-048).

**Verdict:** PROMISING-BUT-ROUGH. Ingestion side of PROTO-008 is good (discovery, typed parts, help/docs quality high — out-of-scope WS-Security documented, not hidden). Invocation side fails on the first real service a user would try: envelope not namespace-qualified. Recommend the foreman add a live-WSDL e2e leg to demo/21-soap-adapter.sh, not just loopback fixtures.

## 2026-10-13 (run 15) — DF-048 fix verification (strict live server) + install leg actually run

**Promise statement:** "A user can ingest any WSDL 1.1 with a targetNamespace and call its operations successfully (DF-048 fixed), installable from scratch on a fresh box following the README."

**Angle change per skill:** last run (10-10) FILED DF-048/DF-049; foremen closed both with 50aea846 (namespace-qualified payload root + plain integer rendering). This run re-proves the fix on fresh evidence — a STRICT live server that rejects unqualified payload roots, i.e. the exact failure mode of run 14 — instead of re-filing. Secondary: re-proof of pending REVIEW2-MUSTER-3 (version stamp) on a new artifact shape (tarball install).

**What was done (real use, not tests):**
- Built CLI at HEAD 63d65e7f (`make build-cli`, stamp v0.1.1-452-g63d65e7f, 0.95s warm ± 25ms).
- Ran a strict SOAP 1.1 echo server on loopback: 500 + `Invalid namespace` fault UNLESS the payload root carries `xmlns="http://example.com/echo"` (the DF-048 guard), 200 + EchoResponse when qualified. WSDL fixture: document/literal, targetNamespace http://example.com/echo, required string + optional int + unbounded string[] parts.
- Discovery (no --operation): both operations listed as typed JSON with soap_action/endpoint/parts incl. required+nillable+repeated flags — correct.
- Live call `--operation EchoService-Echo` with {"message","count":42,"tags":[a,b]}: HTTP 200, JSON `{"message":"ok"}`, rc=0. Server's strict ACCEPTED branch matched ⇒ payload root WAS namespace-qualified (`<EchoRequest xmlns="http://example.com/echo">`, confirmed via --document too). DF-048 FIX VERIFIED LIVE.
- UX check: unknown `--operation Echo` (vs discovered id `EchoService-Echo`) errors with usage — exit code is 1 (earlier rc=0 reading was a pipeline artifact; direct check verified 1). No row.
- README smoke path verbatim on fresh box (install leg below): discover → generate → typed `get-pet-by-id 1` → 200 Pet1. Relative-server error message (--base-url hint) fired exactly as documented when run without --base-url; with --base-url it succeeded. docs-lie check: none.

**Perf (Step 2b):** SOAP ingest+call 13.5ms ± 0.6ms warm / ~60ms cold (drop_caches) — build stamp in binary, call round-trip loopback; discovery-only 13.1ms ± 0.7ms; generate 0.7s cold interpreter. Nothing a user would feel → no PERF rows filed (numbers recorded here).

**Install leg: RUN (not skipped) on bunker-mvp** (las-01/02/03/04 all offline in tailscale, 9d–16h; bunker-mvp active). Fresh 2h agent dogfood-muster-t4; tar-over-ssh transfer (git archive HEAD, 3.5MB — private repo, no credentials minted; note git bundle was 234MB — heavy history, cf. REVIEW-MUSTER-10). Toolchain: agent image carried go/make/git (go auto-downloaded toolchain 1.26.6 on first build). `make build-cli` cold: 1m32s. Smoke path verbatim (discover/generate/typed command vs petstore3.swagger.io): all rc=0, typed command returned 200 Pet1. Agent destroyed, verified gone via agents list (count 0). Install premise PROVEN: tarball → build → discover → generate → typed call on a fresh box, ~2min wall after transfer.

**Re-proof of REVIEW2-MUSTER-3 (fresh evidence, no new row):** tarball/git-archive build (--version) reports `dev (unknown)` — same stamp gap as bare go build, now shown to affect the archive-install shape too; make-stamped builds remain correct. Premise holds.

**Findings written to board:** none (all observations already covered: DF-048 verified, REVIEW2-MUSTER-3 re-proofed, DF-045 covers clone dead-end, REVIEW-MUSTER-10 covers pack size).

**Time-to-first-success:** fresh box → first successful typed API call ≈ 2min (post-transfer); SOAP strict-server interop first try after fix.

**Verdict:** SHIPPABLE (upgrade from run 14's PROMISING-BUT-ROUGH). The DF-048 blocker is gone: document/literal with targetNamespace now accepted by a strict validator. Install leg proven on a real fresh box this run (previous three runs skipped/blocked). Remaining pending rows are P2/P3 polish.

## 2026-10-13 (run 16) — live external-service re-proof of DF-048/DF-049 + audit surface (DF-050)

**Promise statement:** "The closed DF-048/DF-049 SOAP fixes hold against the same live service at HEAD, a fresh machine can follow the documented install verbatim, and the audit surface changed by ee8fe3ab (REVIEW2-MUSTER-1) works for a real user."

**Angle change per skill:** runs 13-14 filed DF-048/49; the foreman closed both (50aea846, b8ba0232). SIBLING NOTE: a parallel tick landed run 15 in this log mid-flight (strict loopback echo server + bunker-mvp install, agent dogfood-muster-t4). Per the no-duplicate rule this entry keeps only what run 15 did NOT cover: re-proof against a LIVE EXTERNAL service over the internet (run 15 used loopback), the DF-049 large-integer leg, and the first drive of the audit surface. My independent install leg (agent dogfood-muster-1013, same host, tar-over-ssh, 169s cold build, fresh-box live-SOAP feature smoke) stands as re-proof of run 15's install premise; its claims are credited to run 15, not re-filed.

**What was done (real use, not tests):**
- DF-048 RE-PROVEN LIVE: `protocol soap --wsdl nc.wsdl --operation NumberConversion-NumberToWords --args '{"ubiNum":1234567}'` at HEAD 63d65e7f returns "one million two hundred thirty four thousand five hundred and sixty seven" (0.28-0.43s round-trip). --document shows payload root carries xmlns="http://www.dataaccess.com/webservicesserver/". Large-int leg: NumberToDollars 987654321 -> "nine hundred and eighty seven million six hundred and fifty four thousand three hundred and twenty one dollars" (plain decimal, no scientific notation). Both closed rows hold; no new rows.
- Fresh-box feature smoke on the ephemeral agent too: both live calls green from the cold install (install pass is not feature pass).
- Install leg EXECUTED (prior SKIPPED premise stale): bunker-mvp reachable, bunkerd active. Spawned agent dogfood-muster-1013 (TTL 2h), tar-over-ssh transfer (repo private, anon clone 404 — documented in README), `make build-cli` verbatim = 169s cold incl. module downloads, discover smoke exit 0. Destroyed; residue clean after 95s reconciler recheck.
- Audit surface (ee8fe3ab): MUSTER_AUDIT_ENABLED=1 events serialize correctly — snake_case top-level keys, result.Success nested true (the old always-false top-level Success is gone). But `audit log --limit 1` returns the OLDEST event on a command documented "Show recent audit events", and the table renders no action name (both middle columns blank). Filed DF-050 (P2).

**Perf (Step 2b):** live SOAP call 0.28s warm / 0.43s (network-bound), WSDL ingest list 0.013s, discover 0.01s, cold fresh-box build 169s (cold start incl. deps). Nothing user-noticeable beyond the documented cold build -> no PERF rows filed.

**Findings written to board:** DF-050 (P2) — committed with this log.

**Install leg: PASSED** — install_seconds=169 (cold, fresh agent on bunker-mvp), smoke=ok (discover exit 0 + two live SOAP calls from the fresh box), bunker=bunker-mvp agent=dogfood-muster-1013 (destroyed, residue recheck clean).

**Time-to-first-success:** < 2 min on a fresh machine (tar transfer + 169s build + smoke).

**Verdict:** SHIPPABLE. The headline protocol surface now works end-to-end against a real external service from a clean machine. Remaining pending rows are release/CI-billing infrastructure (RELEASE-001/003, CI-BILLING-002/003) and P2-P4 polish.

## 2026-10-17 (run 17) — audit wire-shape bisect on DF-050's fix (board row DF-051)

**Promise statement:** "The DF-050 fix (68f7d936, newest-first audit log) holds: `audit log` is newest-first, --limit returns the N most recent, and the render carries action+level."

**Angle change per skill:** run 16 already printed `audit log` at this same surface (and filed DF-050); this run re-proves the fix at a DIFFERENT layer — the raw JSONL the audit system itself writes at HEAD d95acc02 (not the render, not the test event shape), where the tag regressions actually live.

**What was done (real use, not tests):**
- Built at HEAD d95acc02 (`make build-cli`, v0.1.1-471-gd95acc02).
- Drove the whole audit surface end-to-end with MUSTER_AUDIT_ENABLED=1 and MUSTER_AUDIT_LOG under /tmp (scratch, no real state touched): `version`, `audit status`, `tenant set trial-tenant-1`, `webhook test` (flag check), `audit log/show` renders.
- DF-050 FIX HOLDS at HEAD d95acc02: `audit log --limit 4` prints the most recent event first (04:22:07Z tenant.set rows above the 04:20:17Z rows); level/action columns render (that part of the fix is real).
- NEW FINDING (DF-051, P2, filed): every HOOK-written row renders a BLANK event_type column, and the raw audit.jsonl the hook writes is camelCase (`"actor":{"ID":...}`, `"result":{"Success":true,"Code":"OK"}`, `event_type:""`) — the exact shapes ee8fe3ab (REVIEW2-MUSTER-1) claims were replaced by snake_case. Root cause: AuditEvent's nested structs carry NO struct tags at all (`ID   string` internal/audit/audit_types.go:33-53); the commit's `json:"event_type"` / `json:"actor.id"` changes never landed, so `event_type` is empty on the wire and the render's 3rd column (extended.go:690) is blank for all hook rows. Bisect: only the emitAuditEvent path (tenant/webhook mutations) sets EventType on the wire; the hook path (every CLI command, cmd/openapi-cli/main.go:31 → internal/audit/hooks.go:77 LogCommand) never sets it and has no tags to default it. Run 16's re-proof graded the RENDER, not the wire shape — the byte-level probe is what caught this.
- Corroborating probe: a hand-written legacy line with event_type set renders both columns correctly (tenant.set + pet.created on one row) — the render code is fine; the emitter never populates what it displays. Fix direction: correct the tag syntax (single backtick literals) and add a formatting-contract regression — write one event, read the jsonl bytes back, fail unless event_type/actor.id/actor.type/result.code are snake_case.

**Perf (Step 2b):** audit log --limit 4: 0.02s warm, ~0.04s first run. Discover/version infraops all <0.05s. Nothing user-noticeable — no PERF rows filed. Install leg: NOT re-run this tick — no install-path delta since 63d65e7f (19 commits since run 16 touch board/docs/pyc/tests/version-stamp only); run 16's fresh-box install pass (169s cold on bunker-mvp, live SOAP smoke) stands as the current install evidence.

**Findings written to board:** DF-051 (P2) + event id 833 (dogfood_reproof).

**Time on surface:** ~14 min (bisect + probes).

**Verdict at HEAD:** SHIPPABLE core+SOAP surfaces (run 16's verdict stands); the audit surface now has an honest P2 filed against its wire format instead of a green "fix verified" that graded the wrong layer.

## 2026-10-17 (run 18) — fix-reproofs of the 63d65e7f..3632f0dc delta + integration-gate leg (DF-052 + QA-MUSTER-18-R2)

**Promise statement:** "The fixes foremen closed against run-16-era findings hold at HEAD: bare builds stamp versions (e6f4538a / REVIEW2-MUSTER-3), `protocol soap --endpoint` is honored after dead-code removal (38729bab / REVIEW2-MUSTER-2), the demo auth fixture's /protected contract is callable end-to-end (dcf766b4 / REVIEW-MUSTER-12); and the run gate (make test-integration) holds."

**Angle change per skill:** delta-scope re-proof (19 commits, all four legs the delta touched that no dogfood entry names); SIBLING COLLISION HANDLED: run 17 (audit wire-shape, DF-051) landed as 3632f0dc mid-tick on this same lane before this run appended — my planned audit-log R4 leg is that surface, so it collapsed into a no-op re-proof rather than a duplicate run (nothing to add at the render layer run 17 did not bisect at the wire layer).

**What was done (real use, not tests):**
- R1 VERSION (e6f4538a PASS): /tmp bare `go build` CLI + MCP both report `v0.1.2-0.20261016205408-d95acc02e66f (d95acc02e66f)` — the run 15/16 shape (`dev (unknown)` from tarball/bare builds) is gone; go version -m cross-checks vcs.revision=d95acc02e66f. MCP binary also fails LOUD (no MCP tools registered: pass -spec) when launched without a spec — a fresh user gets the flag spelled out, not a hang.
- R2 --endpoint (38729bab PASS): echo WSDL pointing at dead :9999; no flag → `connection refused` (default from wsdl:service, adapter's own resolution); --endpoint http://127.0.0.1:9999/echo (live loopback echo, /tmp/mf17/soap_echo.py) → 200 `"message":"hi"`, flask log shows POST /echo; no flag second call lands POST /never on the logs — the flag IS honored end-to-end, not silenced by the dead-code removal. Contract ladder: port closed → transport failure rc 1; routed-404 → NOT_FOUND fault mapping (rc 1, fault preserved); wrong-media → VALIDATION_ERROR naming the empty XML root — all honest, machine-graded.
- R3 /protected fixture (dcf766b4 PASS): live fixture server 9803 (run verbatim), HTTP ladder /health 200, /protected unauth 401 {"error":"unauthorized"}, Bearer 200 {"user":"demo"}, /token issues the bearer. Through the CLI: discover shows `{GET /protected protectedEndpoint}` → generate (NOTE: `protected` was the printed wrong command; suggest-tool arg on stderr offers `protected-endpoint` and typed call works) → 401 → -H ...Bearer → 200. Fixture contract + CLI auth-leg both prove out.
- R4 audit-wire: COLLAPSED into sibling run 17 (3632f0dc): rendered DF-050 fix verified in passing this run too (audit log --limit 3 newest-first, action column populated — the ordering leg run 17 did not contradict).
- R5 INTEGRATION GATE — RED, bisected (findings DF-052 + QA-MUSTER-18-R2, P1): `make test-integration` fails with 8 TestKeychainIntegration subtest FAILs, all `credential not found: search items: dbus.Store: length mismatch`, at HEAD 3632f0dc. THIS BOX'S secret service is LIVE and UNLOCKED (gnome-keyring 50.0, Locked=false verified via dbus props; standalone /tmp/mf17/dbusprobe CreateItem SUCCEEDS on the login collection, then the SAME SearchItems Store() dies). Root cause: internal/platform/linux/keyring_secret_service.go:294 `Store(&results, &locked)` but SearchItems' result signature is a SINGLE `ao` (busctl introspect) — godbus Store with one src element and two dests is a length mismatch by construction (godbus dbus.go:47). So Set→Get is write-only against a live keyring; the keychain probe skip (c6eca0fd) only fires on Set-failure boxes and never on this failure ladder — the closed QA-MUSTER-18 premise 'dbus broken = headless-only' does not survive a live-host run. Repro'd with 0 repo code (standalone godbus program) and through the repo's own path (make test-integration, /tmp/mf17/intkey.log).
- go test -short ./... ALSO green this run (73 pkgs, re-checked to validate the red is integration-targeted).

**Perf (Step 2b):** typed CLI call 0.0025s median (hyperfine 20 runs warm, protected-endpoint vs local flask); protocol soap --wsdl ingest+list 0.0018s median warm; make build-cli 1.2s warm. All smaller than any user would notice — numbers recorded here, no PERF rows filed. Cold build on the ephemeral bunker covered by run 16 (169s) — no install-path delta on this branch (delta touches audit/tests/version-stamp/board only).

**Install leg: NOT re-run this tick** — no install-path change in the delta (verified per-file: 19 commits = board/docs/pyc-untrack/test fixes/version fallback; zero Makefile/Dockerfile/go.mod/README-install changes). Run 16's fresh-box pass (bunker-mvp, 169s cold, live SOAP smoke) stands as the current install evidence. Explicit skip with citation, not a silent pass.

**Findings written to board:** DF-052 (P1, keyring decode contract) + QA-MUSTER-18-R2 (P1, recurrence of a closed row, cites DF-052); events 834/835.

**Time-to-first-success:** unchanged (< 2 min fresh box per run 15/16). R5 gate: integration suite red for every user whose keyring service runs — first-succes stays available but the advertised integration gate lies green-adjacent.

**Verdict:** PROMISING-BUT-ROUGH at HEAD (downgrade from run 16/17's SHIPPABLE on the strength of the red integration gate + the write-only keyring defect it exposes for CLI auth storage on keyring-backed hosts). Core/SOAP/audit surfaces all re-proven green; the P1s are single-site fixes (one Store target) the foreman can close quickly; billing/CI wall unchanged (noted, not re-filed).

*(run-18 scratch evidence copied to `.coding-hermes/dogfood-evidence/run18/` in the follow-up commit — the rows' /tmp paths are backed by this durable copy.)*

## 2026-10-20 (run 19) — delta re-proofs of the 3632f0dc..69253114 window (audit wire/identity, keyring gate, toolchain bump) + fresh-box install leg

**Promise statement:** *"The findings run 18 filed are fixed at HEAD: the integration gate is green again (DF-052), audit rows carry the real success bit exactly once (DF-053), the wire shape is snake_case (DF-051) — and a fresh user can still install, build and use the CLI on a clean machine after the go.mod 1.26.9 bump (449e57a5)."*

**Angle note:** delta re-proof on the 26 commits since run 18's HEAD. Code delta = 5 commits (449e57a5 go 1.26.6→1.26.9 + x/net v0.60.0, a795cc28 gofmt, fe07a783 audit snake_case wire shape, f803684e audit single-log + real success bit, a9c7f6e9/530c58db/71bfe915 keyring Store() contract), everything else board/PM bookkeeping. Board read first: 8 pending rows, all owner-blocked (CI-BILLING-002/003, INT-CI-001, RELEASE-001/003, REVIEW2-MUSTER-5 = billing+authorization; GAP-032 module reachability; NEVER-DONE perpetual fixture) — nothing dispatchable, so no foreman wake and no scheduler mutation (read-only lane).

**R1 — audit surface (DF-051 + DF-053 re-proof: PASS, + one new defect):** fresh sink, path from docs/guides/configuration.md:121 (`export MUSTER_AUDIT_ENABLED=1`). Succeeding commands now write exactly ONE row each (`audit status` → 1 row, `webhook list` → 2nd row, no duplicate), every row carries `event_type=command.execute`, snake_case `actor.id/type/name` + `result.success/code`, and a FAILING invocation writes `result.success=false code=ERROR` — so the DF-051 wire shape and both halves of DF-053 (single write, real success bit) are VERIFIED FIXED at HEAD. NEW DEFECT filed as **DF-054**: the failure path loses the command identity and stamps a fake actor (`cmd/openapi-cli/main.go:56` passes `rootCmd.CommandPath()` — always `openapi-cli` — and the literal `"cli"` instead of the failing `cmd.CommandPath()` + `cliActorID()`); all six failing invocations (arg-count error, missing wasm file, unknown flag, connection refused, userinfo URL, unknown command) collapse to `action="command.openapi-cli" / actor.id="cli"`, and `audit log --limit 5` renders `command.openapi-cli\tcli` with no way to tell what failed. Also filed **DF-055** (`user` field declared at `internal/audit/audit_types.go:73` and never assigned anywhere in production code) and **DOC-13** (specs/EVENTS-AUDIT.md promises a `previous_hash` chain + MANDATORY redaction; the shipped event has neither, `HashAlgorithm: "sha256"` has no consumer, `RedactSensitive` is unreachable — no flag/env/config sets it — and `audit info` prints "Sensitive fields: not redacted"). POSITIVE control from the same sink: 88 accumulated rows including an authenticated request and a `user:password@host` URL contain **zero** occurrences of the probe secret, and the error text shows the userinfo stripped — no live credential leak; DOC-13 is a contract gap, not a leak.

**R2 — integration gate (QA-MUSTER-18-R2 re-proof: PASS):** `make test-integration` at HEAD 69253114 → 0 FAIL, "✓ Integration tests complete"; uncached re-run `go test -tags=integration -run Integration -count=1 -v ./test/e2e/` → `TestKeychainIntegration` 9/9 subtests PASS (1.08s) on THIS box's live, unlocked gnome-keyring — the run-18 red (8 `dbus.Store: length mismatch` FAILs) is gone. The gate is honest again for keyring-backed users.

**R3 — headline path verbatim (PASS):** README Quick start with an isolated HOME: `discover` remote petstore 0.68s rc=0 → `generate` 0.38s rc=0 → typed `get-pet-by-id 1` 0.30s rc=0 (HTTP 200) → generic `request` rc=0. Machine-readable stdout stays CLEAN: `get-pet-by-id 1 -o json | jq .id` → `1` (118 stdout bytes; the structured `INF HTTP response received` line goes to stderr only).

**R4 — demo gate (PASS):** `./demo/run-all.sh` verbatim → **33/33 checks**, 14 demo scripts + 6 integration suites all ✓, `real 20.20s` wall (documented gate, one-shot).

**Perf (Step 2b):** box load 7.9–13.8 (16 cores, sibling fleet lanes) — quoted with that caveat. CLI startup floor `openapi-cli version`: **43.7ms mean / 18.5ms min** (hyperfine, 30 runs, warm). Local `discover <spec>` (parse+render, no network): **59.0ms mean / 22.4ms min** (20 runs). Typed remote call (petstore3): **603ms mean / 284ms min** — RTT-bound, σ 420ms from box load. Audit-hook cost A/B on the same command: OFF **14.8ms mean (min 11.4)** vs ON **24.4ms mean (min 10.9)** — the gap is outlier noise, so turning audit logging on costs nothing a user could feel. `make build-cli` warm 1.27s. **No PERF rows filed:** nothing on the user path crosses the notice threshold; the only slow number is the fresh-box cold build (121s including toolchain + 35 module downloads), which is install-leg evidence, not an interaction. Disk note: the CLI's own startup work is sub-millisecond against a ~19ms process floor, so there is no hot path worth profiling — saying that plainly is the result.

**Install leg — RUN THIS TICK (fresh box, PASS):** bunker-mvp agent `df-muster-20261020` (only healthy server: las-01/02/03/04 + cube-las-00 answered no agent listing). Repo is PRIVATE and the lane rules forbid minting credentials for a clone, so the source was delivered as a tar snapshot (40MB, excluding `.git/.venv/bin/tmp`) plus `git init && commit` inside so make's git-derived stamps resolve — the one way this leg is weaker than a clone, and it is recorded as such: `VERSION` stamps the synthetic snapshot sha `b0587f9`, not `69253114`. Box: Ubuntu 24.04.4, 8 cores, go1.22.2, gcc 13.3, make/git/curl/python3 present. `make build-cli` verbatim → **rc=0 in 121s**, and `GOTOOLCHAIN=auto` **auto-fetched go1.26.9** from the go.mod directive (the documented auto-fetch path works on a box older than the floor — README's claim holds). Smoke: `./bin/openapi-cli version` rc=0 (stamped), `discover <local spec>` rc=0, `go test -count=1 ./internal/version/` ok, and the audit delta surface **reproduced DF-054 on the second machine** with a different `$USER` (so the fake-actor shape is not a one-box artifact). Agent destroyed, no residue (only the unrelated `crier-lab` agent remains on that server).

**Findings written to board:** DF-054 (P2), DF-055 (P3), GAP-033-R2 (P3, RECURRENCE of the closed GAP-033 toolchain-doc drift — go.mod is 1.26.9 while 8 sites still say 1.26.6; carries a recurrence-preventer AC because this class has now re-emerged twice), DOC-13 (P3). Post-write `boardctl validate -C .` → `RESULT: OK`, **0 errors / 193 warnings — identical to the pre-write baseline**, so the four rows are canon-clean and sibling lines are byte-identical to the snapshot.

**Deliberately NOT filed:** the checkout's `.venv` has a stale script shebang (`/home/kara/muster/.venv/bin/python`, the pre-move path) so `.venv/bin/flask`/`pip` fail — but every documented path still works (`run-all.sh`'s `-x .venv/bin/python3` probe passes, Flask imports, the demo runs 33/33) and the documented venv command repairs it: local drift, not a product row. `audit log` prints tab-separated columns with no header (UX nit, sibling of the closed DF-050, no new AC). go's isolated-HOME toolchain download (`$HOME/sdk`) is stock Go behaviour, not a muster defect.

**Verdict: 🟡 PROMISING-BUT-ROUGH** at HEAD 69253114 (unchanged from run 18). The three run-18 P1s are now independently re-proven fixed (green integration gate, single-write audit rows with a real success bit, working keyring decode), the documented demo gate is 33/33, and a fresh box installs + builds + smokes clean. What keeps it out of SHIPPABLE is the audit trail's own honesty: the rows a reviewer reads first (failures) are anonymous and unattributed (DF-054), a declared field is always empty (DF-055), and the spec describes a tamper-evident, mandatory-redacting store that does not exist (DOC-13) — small, single-site fixes, all in `internal/audit` + one doc sweep. Billing/CI wall unchanged and owner-blocked; no foreman wake from this read-only lane.

*(run-19 scratch evidence copied to `.coding-hermes/dogfood-evidence/run19/` — r1-audit.txt, audit3.jsonl, audit-leak.jsonl, r3-quickstart.txt, r4-perf.txt, demo-timed.txt, integration.txt, e2e-keychain-uncached.txt, leg-install-agent.txt.)*

## 2026-10-20 (run 19b, same tick, second angle) — resumed-turn close-out: CLI credential round-trip at the user boundary + two surfaces run 19 did not drive

**Disclosure / attribution:** this entry is the RESUMED (context-compacted) turn of tick `muster-dogfood-2026-10-20-11-29-14`. Run 19's first pass landed `69c36580` (+ `2eb9f8f3` evidence follow-up) with DF-054 / DF-055 / GAP-033-R2 / DOC-13 and the run-19 log entry above. Before designing this turn's legs I attributed the live work: the process table showed my own earlier turn's agent `df-muster-20261020` mid-install-leg from the shared scratch dir `/tmp/mf19`, so I CEDED the install leg (its log was read, not re-run) and destroyed the redundant agent `df-muster-1020` that I had spawned 37s later and never staged a payload into. No sibling session was involved — one instance, two turns.

**Promise statement (this turn):** *"The credential path a real user drives works end to end: `auth add` stores a secret, `--auth` attaches it to a live request, wrong or missing credentials fail loudly, and none of it leaves residue in the user's real config — the run-18 write-only keyring defect is gone at the USER boundary, not just in the Go test."*

**R5 — CLI credential round-trip (NEW this turn; the only leg run 19 did not drive).** Live fixture `demo/services/inkwell/server.py` on :9803 (repo `.venv` python, `health=200`); script `legB_roundtrip.sh`:
- `auth add --name df052 --type bearer --value reader-token` → rc=0, `0.10s` wall, "Stored credential "df052" (type: bearer)"; `auth list` shows the metadata row.
- NEGATIVE CONTROL: `request GET http://localhost:9803/me` (no `--auth`) → rc=1, `401 {"error":"missing or invalid Authorization header"}`.
- ROUND TRIP: `request GET .../me --auth df052` → rc=0, `200 {"role":"reader"}` — the token existed ONLY in the OS keyring (gnome-keyring, unlocked), so this is the run-18 "Set works, Get dies with `dbus.Store: length mismatch`" defect re-proven fixed where a user feels it, and it re-proves the closed DF-023 ("stored credentials are never attached to any request").
- WRONG-VALUE CONTROL: a second credential holding a bogus token against the same route → rc=1, `401 {"error":"invalid token"}` — the 200 above is driven by the stored value, not a stub response.
- CLEANUP: both credentials removed; `~/.config/muster/credentials.json` is byte-identical to its pre-run content (`consensus`, `consensus-int` only). No residue in the real user config.

**Corroboration (not new evidence):** an independent uncached run of the keyring gate, `go test -count=1 -tags=integration -run TestKeychainIntegration ./test/e2e/ -v`, passed all 10 subtests in 2.20s on this box's live unlocked keyring — the same result run 19 recorded (1.08s) from a different invocation.

**Findings filed this turn (2):** **DF-056** (P5) `./demo/run-all.sh` Results summary prints RAW ANSI escapes — lines 159-160 use a plain `echo` with `${GREEN}`/`${RED}` while :161 in the same block uses `echo -e`, so the gate's headline line reads `\033[0;32mPassed: 33\033[0m` (byte-verified in the committed evidence `run19/demo-timed.txt`). **DOC-14** (P3) `specs/PLATFORM-LINUX.md:2893-2897` documents four env vars NO source file reads (`MUSTER_CONFIG_DIR` spec+one test only, `MUSTER_CACHE_DIR`, `MUSTER_DATA_DIR`, `MUSTER_NO_COLOR` — the real knob is the standard `NO_COLOR`, `pkg/cli/error_formatter.go:206,225`), where the same table's `MUSTER_LOG_LEVEL` has 26 tree hits as the control; sibling of the closed DOC-11 (that fix touched `docs/guides/configuration.md` only).

**Deliberately NOT filed:** `MUSTER_CONFIG`/`MUSTER_CONFIG_FILE` do not redirect credential metadata (`auth add` with `MUSTER_CONFIG=/tmp/.../config.yaml` still wrote `~/.config/muster/credentials.json`) — but `XDG_CONFIG_HOME` DOES (`pkg/auth/credential.go:19-33`), so isolation exists and the gap is a doc nuance, recorded as a related note inside DOC-14 instead of its own row. `auth list` enumerating keyring items regardless of the config dir is correct behaviour, not a defect. Tab-separated headerless `audit log` output and the stale `.venv` shebang were already declared non-rows by run 19 — not re-filed.

**Perf (Step 2b):** no PERF rows, deliberately. This turn's only new timings are sub-second (`auth add` 0.10s wall, authenticated `GET /me` 0.30s incl. process start) and run 19's headline numbers (CLI startup 43.7ms mean, local discover 59.0ms, remote typed call 603ms RTT-bound, demo gate 20.20s, cold fresh-box build 121s) are already recorded above with their load caveat. Nothing on the user path crosses the notice threshold, so profiling was not warranted — saying that plainly is the result.

**Integration/close-out:** no code touched, no board row mutated, no scheduler state changed (read-only lane; foreman NOT woken), no repo visibility or credential changes. `boardctl -C . validate` re-run post-write (result recorded in the commit); the two new rows were minted through `board_next_id.py` (DF-056, DOC-14) and the ids were grep-verified free before the append.

**Verdict: 🟡 PROMISING-BUT-ROUGH** at HEAD `2eb9f8f3` — unchanged from run 19. The credential path now behaves correctly at the user boundary (its own negative and wrong-value controls fail loudly), the three run-18 P1s stay independently re-proven fixed, and this turn's additions are one P5 cosmetic and one P3 spec-drift row. The blockers to SHIPPABLE are still the owner-gated billing/release wall and the audit-trail honesty rows (DF-054/DF-055/DOC-13) run 19 filed.

*(run-19b scratch evidence copied to `.coding-hermes/dogfood-evidence/run19b/` — legB log + script, the two row bodies, fixture health.)*

*Run 19 addendum (same tick, pre-close):* perf numbers re-measured after the tick's heavier fleet load landed (loadavg **18.05** vs 7.9 at first measurement) — `openapi-cli version` **13.0ms mean / 9.6ms min** and local `discover <spec>` **23.0ms mean / 10.3ms min** over 25 runs each. The first pair (43.7/18.5ms and 59.0/22.4ms) was therefore load-inflated; the CLI's real floor is ~10ms and a local spec parse+render ~10–23ms. Same conclusion, more margin: nothing on the user path is slow enough to file a PERF row. Closing state for the tick: HEAD 69253114 before the run, my run's commit **2eb9f8f3** pushed to origin/main (rows DF-054/DF-055/GAP-033-R2/DOC-13 + event 850 + this log + `.coding-hermes/dogfood-evidence/run19/`), `boardctl validate` 0 errors / 193 warnings (baseline unchanged), demo ports 9801–9805 free and no demo service process left behind, install-leg agent destroyed. A SIBLING dogfood instance on the same tick filed run 19b (DF-056 ANSI escapes in the demo summary, DOC-14 spec env vars no source reads) as commit 3e20b6c4 sitting one commit above mine and unpushed at my close — different findings, no duplicate of DF-054/DF-055/GAP-033-R2/DOC-13; not mine to push (fleet strand-push covers it). One observed-and-declined-for-mine item now carries a sibling row: the raw `\033[0;32m` escapes in `run-all.sh`'s Results block (I judged it a nit; DF-056 records it at P5).

## 2026-10-23 (run 20) — delta re-proof of the 2eb9f8f3..0bd841fe window (audit chain/redaction, failure identity, demo ANSI, toolchain bump) + first drive of the new audit chain at the user boundary

**Promise statement:** *"The audit work run 19 filed is now usable, not just present: a user can see, trust and check the audit trail (chain + redaction reachable from the CLI), the failure rows carry their real identity, the demo gate's summary renders as colour instead of escapes (DF-056), and a fresh box still installs and builds at HEAD 0bd841fe (go 1.26.10)."*

**Angle note.** Last run was 19/19b on 2026-10-20 (HEAD 2eb9f8f3). Since then 38 commits landed, of which the code-relevant ones are 2eac74f1/02f99543 (audit hash chain + redaction, DOC-13), 50f16e08 (failure-path identity, DF-054), de9f2f9b (audit User field, DF-055), f848499a (demo ANSI, DF-056), 6a316125 (go 1.26.9→1.26.10, QA-MUSTER-19), plus docs sweeps. So this tick re-proves those fixes **at the user boundary** instead of re-reading their greps — which is exactly where DOC-13's closure was thin (`grep -c previous_hash = 1`).

**Build provenance.** The shared checkout was **dirty and in use by a concurrent worker** (`gofmt`/`go test` on pkg/auth, pkg/mcp, pkg/daemon, internal/builtin at 07:39) and its `bin/openapi-cli` reports `v0.1.1-539-g99196361-dirty`. To attribute every observation to HEAD, all product legs ran from a **clean detached worktree of 0bd841fe** (`git worktree add --detach /tmp/mf20/src HEAD`), built there (`go build ./cmd/openapi-cli`, 1.79 s, `dev (unknown)` — no .git stamps in a worktree), with `HOME=/tmp/mf20/home` and `MUSTER_AUDIT_LOG` per leg. No file in the shared checkout was modified.

**R1 — audit chain + identity (re-proofs PASS):** fresh sink, `MUSTER_AUDIT_ENABLED=1`. Seven consecutive rows each carry `id` + `hash` + `previous_hash`, and **every `previous_hash` equals the previous row's `hash`** (chain links correct by hand inspection, run20/legA-audit-chain-redaction.txt). `user` is populated (`kara`) — **DF-055 verified fixed** at the user surface. A failing invocation writes `result.success=false` with `code=ERROR` while a succeeding one writes `true/OK` — DF-053's split (single write, real success bit) still holds. `audit info` now prints **`Sensitive fields: redacted`** (was `not redacted`). A request carrying a bearer token in `--header` left **zero** occurrences of the token in the sink, and a `user:password@host` URL is stripped from the error text. **DF-054's main fix verified**: `frobnicate`, `discover` (arg error / missing file) and `request` (connection refused) each record their own leaf (`command.openapi-cli frobnicate`, `... discover`, `... request`).

**R2 — the audit trail is NOT usable yet (4 new rows):**
- **DF-057 (P2)** — the sink persists a **live webhook credential in plaintext**. `webhook create --url 'https://hooks.slack.com/services/T…/B…/<token>' --events …` writes `context.metadata.url` verbatim (73 chars, byte-true check: `has_secret=True`, sha12 `4cf91d0d7d52`) and the same for `webhook test`/`webhook delete`; **`[REDACTED]` appears 0 times in any row the product wrote**. Redaction is keyed on field NAME only (`internal/audit/redaction.go:33-75`: password/token/secret/api_key/authorization/card/ssn…) and `url` is not in the vocabulary, so for a real user the mandatory-redaction machinery never fires. This is specs/EVENTS-AUDIT.md:230-241 invariant 2 ("No credential appears in plaintext in any log output") failing its own prescribed test 4 (grep the log for token=).
- **DF-050-R2 (P2)** — **RECURRENCE of DF-050 on the sibling verb**: on a 2,200-row sink `audit show --limit 5` returns the five **OLDEST** rows (12:44:39Z) while `audit log --limit 5` returns the five newest (12:45:09Z); both are documented "Show recent audit events". Mechanism: `show` passes its `--limit` into `readAuditEvents` (which breaks at the top of the append-ordered file) and only then reverses (`internal/builtin/extended.go:684-698,779-798`); `log` passes 0 and cuts after the reverse. DF-050 was fixed verb-locally.
- **DF-059 (P3)** — the chain is **tamper-evident in code, unverifiable by a user**: `VerifyChain` has **zero production call sites** and `audit --help` offers only info/log/show/status. Driving real tamper on a real sink: rewriting one record's action+success, and separately deleting a middle record, both render normally — `audit log` rc=0, `audit info` rc=0, `audit status` rc=0, no warning anywhere. The evidence DOC-13 was closed on cannot be checked by the person the trail is for.
- **DF-054-R2 (P3)** — the identity fix stops at the **first non-flag word** (`cmd/openapi-cli/main.go:66-95`), so multi-word leaves collapse: `webhook create --bogus-flag 1` **and** `webhook add --url x` both record `command.openapi-cli webhook`, i.e. a bad flag and a non-existent subcommand are indistinguishable in the trail.
- **DOC-15 (P3)** — retention is decorative: `audit info` prints `Retention: 90 days` (`internal/builtin/extended.go:742`, default 90 at `hooks.go:201`) while **no code prunes or rotates** (`grep -rniE 'func .*(rotate|prune|trim|retention)|os.Truncate' … | grep -v _test` → empty) and the spec's documented default is **365** (specs/EVENTS-AUDIT.md:25). Consequence measured: `audit log --limit 1` **11.7 ms @200 rows → 27.1 ms @2,200 rows** (7.7 µs/row, strictly linear — `os.ReadFile` + unmarshal of every line, `extended.go:779-798`), ~1.1 s at ~146k rows.

**R3 — documented paths, verbatim (PASS except the quickstart's payoff step):** README:66-90 driven as written: `discover` rc=0 363 ms → `generate` rc=0 361 ms ("Generated 3 command groups") → **`get-pet-by-id 1` rc=1 "Pet not found / HTTP 404", five repeats 278-383 ms** — README:82-84 claims "GET /pet/1 … work" and run 19 recorded the same call returning HTTP 200 three days ago, so the third-party fixture moved. Filed **README-058 (P3)**. The docs' other example, `request GET http://localhost:9801/tasks -o json`, works exactly as written against the demo service: rc=0, stdout `[]`, and the 404 path emits **valid JSON** `{"error": "task not found"}` on stdout with the human error on stderr — the `-o json` contract holds (a first attempt that appeared to break it was my fixture: wrong service path, then a 405 from a route the demo server does not accept — both discarded).

**R4 — demo gate + DF-056 (PASS):** `./demo/run-all.sh` verbatim → **33/33 checks, 14 scripts + 6 integration suites ✓, real 36.7 s** (after the documented `python3 -m venv .venv && uv pip install -r demo/requirements.txt`, 0.14 s on a warm uv cache). **DF-056 verified fixed**: the Results line is real colour — byte check `od -c` shows `033 [ 0 ; 3 2 m P a s s e d : 3 3 033 [ 0 m`, and `grep -c '\033'` (literal backslash form) = **0**. On a clean tree with no venv the gate exits 1 **before building** with the venv command, the `--bootstrap` hint and the `DEMO_PYTHON` escape hatch — the freshest possible failure mode is documented, not silent.

**R5 — toolchain doc drift came straight back (recurrence):** go.mod is `go 1.26.10` (6a316125, 2026-10-22) and **nine user-facing sites still say 1.26.9** — README:32 (which asserts "the `go` directive in `go.mod` is `1.26.9`"), README:45, README:271, docs/README.md:54, docs/guides/installation.md:16/134/138, docs/integration-guide.md:44, docs/contributing/setup.md:9; **zero** sites say 1.26.10. GAP-033-R2 was closed one day earlier with exactly these lines at 1.26.9, and no guard exists (no test/script compares a documented Go version with go.mod). Filed **GAP-033-R3 (P3)** carrying the guard AC this class has now needed three times. AGENTS.md untouched (Bane doctrine 2026-09-24).

**Perf (Step 2b) — measured, and no PERF row filed:** box loadavg 6.9 (16 cores, sibling fleet lanes) quoted with every number. Startup floor `version` **10.1 ms ± 1.0** (30 runs, warm); audit OFF vs ON on the same command **9.6 ± 0.7 vs 11.0 ± 3.0 ms** (ranges overlap — turning audit on is not a cost a user can feel); writer growth **13 ms/cmd on an empty sink vs 14 ms/cmd on a 2,000-row / 1.1 MB sink** (the 1 MiB tail-read for the chain link is free); local `discover` **14.1 ms ± 13.9** (load-noisy, floor ~10 ms); demo gate 36.7 s one-shot; fresh-worktree build **1.79 s**. The only growth-shaped number is the audit *reader* (11.7→27.1 ms over 10× rows), and at today's scale it is not something a user feels — so it is stated inside **DOC-15** as the retention gap's consequence instead of being dressed up as a PERF row.

**Install leg — bunker battery LAUNCHED, PARTIAL at close (not SKIPPED):** `bunker-qa.sh launch /home/kara/wojons/muster --keep` (primary checkout, per the worktree refusal) → `repo-head OK` (tested HEAD `0bd841fe`, `pull=clone dirty — pull skipped, testing recorded sha`), `launch OK` — agent **53097b60** on **bunker-mvp**, ttl 4h, script 135,508 bytes, evidence `/tmp/bunker-qa-evidence-20261023T124416Z-3104346.jsonl`. Host pre-check: ONLINE, 5/50 agents, disk 61% (91.7/149.9 GB), CPU 56%, residue 0 users / 0 homes / 20 keys. At close the battery was still in its first cell (agent 1.9 GB and growing; `collect` answers `RUNNING … agent left intact`), so **no cell is graded and the install leg is UNVERIFIED this tick — it is PARTIAL, not a skip**. Finish with `bunker-qa.sh collect --evidence /tmp/bunker-qa-evidence-20261023T124416Z-3104346.jsonl` (idempotent, destroys on CELLS-DONE), or `bunker destroy 53097b60 --server bunker-mvp` if the TTL is preferred as the reaper. Do NOT re-spawn the leg at the same HEAD to look busy.

**Findings written to board (7):** DF-057 (P2), DF-050-R2 (P2), DF-059 (P3), DF-054-R2 (P3), DOC-15 (P3), README-058 (P3), GAP-033-R3 (P3). Ids minted through `board_next_id.py` (DF/DOC/README families + `--recur DF-050`, `--recur DF-054`, `--recur GAP-033`), grep-verified free before the append. Post-write `boardctl -C . validate` → **RESULT: OK, 209 warnings — identical to the pre-write baseline**; the rows were re-serialized once (uncommitted) to drop two out-of-canon keys and a null `depends_on`, so the delta is 0 warnings, not +14.

**Deliberately NOT filed:** the webhook URL is *also* printed in full by `webhook create`/`webhook list` (`webhookLabel` does not mask; my first reading of "sk-liv…4321" in a tool echo was **output elision, not product masking** — the byte check proved the full URL on stdout), but echoing back a subscription URL is normal tool behaviour and the security surface is the sink, so DF-057 is scoped there and says so. `0-byte output`/`405` probes from the first quickstart attempt were **my** fixture errors (wrong service dir, a route the demo server refuses) and were discarded, not filed. The absence of a `docs/qa/use-cases.md` catalog (so no UC-/REG- ids exist for muster) is recorded here as a coverage observation for the QA lane rather than as a product row.

**Verdict: 🟡 PROMISING-BUT-ROUGH** at HEAD `0bd841fe` (unchanged from runs 18/19). The mechanical fixes all hold at the user boundary — chain links valid, `user` populated, real success bit, per-command identity, colour summary, 33/33 demo gate, 10 ms CLI floor — but the audit trail, which is the feature this window was about, still cannot be **trusted by its user**: the one credential-bearing field it records lands in cleartext (DF-057), the "recent events" verb shows the oldest events (DF-050-R2), and nobody can check the chain or notice a deleted record (DF-059). All seven findings are small, single-site fixes in `internal/audit`, `internal/builtin/extended.go`, `cmd/openapi-cli/main.go` and docs. The billing/CI wall (RELEASE-012, CI-BILLING-003, INT-CI-001, REVIEW2-MUSTER-5) and GAP-032 remain owner-blocked; nothing was dispatchable on the board besides those, so no foreman wake and no scheduler mutation from this read-only dogfood lane.

*(run-20 evidence committed under `.coding-hermes/dogfood-evidence/run20/`: legA-audit-chain-redaction.txt, legB-tamper-silent.txt, legC-webhook-credential-in-sink.txt, legC-bytes.txt, legC2-webhook-console-bytes.txt, legD-failure-identity-matrix.txt, legE-audit-show-vs-log.txt, legF-quickstart-verbatim.txt, legG-local-demo-generic-request.txt, legPerf-measurements.txt, demo-gate-33of33.txt, doc-drift-census.txt, install-leg-launch.txt. Probe credentials are fake and were replaced with `FAKE-PROBE-WEBHOOK-SECRET` before the commit; the byte-true facts (lengths, hashes, booleans) are unaffected.)*

*ch:trace row=DF-057 spec=specs/EVENTS-AUDIT.md#credential-redaction--pii-protection test=run20/legC-webhook-credential-in-sink.txt+legC-bytes.txt doc=none:no-doc-claims-redaction-of-urls evidence=.coding-hermes/dogfood-evidence/run20/ witness=dogfood-run20-driven-at-user-boundary verdict=P2-live commit=pending*
*ch:trace row=DF-050-R2 spec=none:df-050-regression test=run20/legE-audit-show-vs-log.txt doc=docs/guides/commands.md:497 evidence=.coding-hermes/dogfood-evidence/run20/legE-audit-show-vs-log.txt witness=dogfood-run20 verdict=P2-live commit=pending*
*ch:trace row=DF-059 spec=specs/EVENTS-AUDIT.md:5 test=run20/legB-tamper-silent.txt doc=docs/guides/commands.md:482-505 evidence=.coding-hermes/dogfood-evidence/run20/legB-tamper-silent.txt witness=dogfood-run20 verdict=P3-live commit=pending*
*ch:trace row=DF-054-R2 spec=none:df-054-regression test=run20/legD-failure-identity-matrix.txt doc=none evidence=.coding-hermes/dogfood-evidence/run20/legD-failure-identity-matrix.txt witness=dogfood-run20 verdict=P3-live commit=pending*
*ch:trace row=DOC-15 spec=specs/EVENTS-AUDIT.md:13,25 test=run20/legPerf-measurements.txt+legA-audit-chain-redaction.txt doc=docs/guides/commands.md:482-505 evidence=.coding-hermes/dogfood-evidence/run20/ witness=dogfood-run20 verdict=P3-live commit=pending*
*ch:trace row=README-058 spec=none:quickstart test=run20/legF-quickstart-verbatim.txt doc=README.md:66-90 evidence=.coding-hermes/dogfood-evidence/run20/legF-quickstart-verbatim.txt witness=dogfood-run20 verdict=P3-live commit=pending*
*ch:trace row=GAP-033-R3 spec=none:toolchain-floor test=run20/doc-drift-census.txt doc=README.md:32,45,271+docs/guides/installation.md:16,134,138 evidence=.coding-hermes/dogfood-evidence/run20/doc-drift-census.txt witness=dogfood-run20 verdict=P3-live commit=pending*

### run 20 install-leg addendum (same tick, after the rows were pushed)

The battery's own first cell graded **`fresh-install OK` 2.9 min after launch** (`~/qa-evidence.jsonl`, ts 12:47:54Z) — but its tree `~/qa-muster` has **2915 files / 20,037,497 bytes and an EMPTY `bin/`**, and `~/qa-logs-*/inst.log` contains **only 90 module-download lines, no build output**. So the cell's `OK` is an rc verdict on a module fetch, **not** installability evidence: the documented post-install smoke (`./bin/openapi-cli version`) cannot be graded from the cell. Recorded here as a **harness observation, not a product row** (the cell's contract is rc, not artifact).

Artifact-level install proof run by this tick on a **copy** of the harness tree (`cp -a ~/qa-muster ~/qa-smoke`, so the still-running battery was untouched), staged as `leg-artifact.sh` and run on agent `53097b60`:

```
SMOKE_HEAD=970720d FILES=2915 BYTES=20037497
BUILD_RC=0 BUILD_SECONDS=203          # make build-cli, CGO_ENABLED=1, warm module cache
bin/openapi-cli  28012809 bytes       # artifact exists and is plausible
./bin/openapi-cli version -> 970720d (970720d)
./bin/openapi-cli discover demo/openapi-specs/petstore.yaml -> "API: Pet Store API (v1.0.0) / Base URL: http://localhost:8080"  rc=0
audit smoke (MUSTER_AUDIT_ENABLED=1, fresh sink): audit_rows=1 audit_has_hash=1
```

So a **fresh box installs and builds MUSTER from a clean tree in 203 s** (Go bootstrapped to 1.27.3 by the harness's toolchain cell, gcc/make/compose/node present) and the built binary drives a spec end-to-end and writes a hash-chained audit row. The build log's `GoVersion=go1.26.10` stamp is the go.mod floor the Makefile reads, not the toolchain that compiled it — noted so the number is not misread.

Battery state at close: still in its `ci-pass` cell (`act` running; `qa-logs-*/ci.log` at "Run Main Build"), agent **53097b60** alive and **deliberately NOT destroyed** (`--keep`, ttl 4h) so the remaining cells stay collectable:

```
bash ~/.hermes/scripts/bunker-qa.sh collect --evidence /tmp/bunker-qa-evidence-20261023T124416Z-3104346.jsonl
# or, if the TTL is preferred as the reaper:  bunker destroy 53097b60 --server bunker-mvp
```

Evidence: `run20/install-leg-launch.txt`, `run20/install-artifact.log`. Nothing was spawned a second time to "look busy": same HEAD, same docs.

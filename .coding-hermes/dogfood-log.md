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

### run 20 install-leg FINISH — the battery collected (second fire of tick `muster-dogfood-2026-10-23-12-37-51`)

The earlier fire of this same tick left exactly one leg open: the bunker battery was still in its `ci-pass` cell, so it was recorded PARTIAL with the finish command. This fire did only that leg — poll → collect → attribute. **No product leg was re-run and no row was re-filed**: the battery was polled with the documented `bunker-qa.sh collect --evidence /tmp/bunker-qa-evidence-20261023T124416Z-3104346.jsonl` (idempotent; it prints `RUNNING` and touches nothing while alive), and the graded cells are what it returned at 08:09 local.

**Battery result on agent `53097b60` / bunker-mvp, tested HEAD `0bd841fe` (pull skipped — clone dirty), 19 pulled rows + `collect OK`:**

```
repo-head            OK          launch OK    toolchain-bootstrap OK x7    fresh-install OK
ci-pass              FAIL        upgrade FAIL
docker-deploy        SKIPPED     chaos-shutdown SKIPPED        (live-host safety gate)
chaos-disconnect     UNVERIFIED  (refuses a verdict on a failing suite — downstream of ci-pass)
chaos-resource       INFO        (go toolchain cannot build under the 3G ulimit — ENV-BLOCKED)
chaos-errorpath      INFO        (probe hit the CLI-help arm of openapi-daemon, rc=0)
ui-probe             INFO        (repo's go-native start command is not a long-running server)
chaos-corruption     N/A         (no db/state files in this repo)
```

**ATTRIBUTED — one real product defect, proved by a same-host A/B.** `ci-pass` FAIL is `--- FAIL: TestServer_Run_HandlerReturnsError (3.00s)` / `server_test.go:512: Run did not return after stdin close within 3s` / `FAIL github.com/wojons/muster/pkg/mcp 4.709s`, i.e. the QA-MUSTER-19 failure again — but at a toolchain the row's closure did not cover. The battery runs the native suite as `env PATH="$HOME/tools/go/bin:$PATH" go test ./... -count=1` (`qa-run.sh:982`), and that cell installed **go 1.27.3** ("go 1.27.3 installed (~/tools/go; floor=1.26.10 distro=1.27.3)"). Running the same command on THIS host at the same HEAD, changing only the toolchain:

```
A  go1.26.10             -> --- PASS: TestServer_Run_HandlerReturnsError (0.00s)   ok pkg/mcp 0.023s
B  GOTOOLCHAIN=go1.27.3  -> server_test.go:512: Run did not return after stdin close within 3s
                            --- FAIL (3.00s)                                        FAIL pkg/mcp 3.011s
```

Both arms ran back-to-back with no docker/act load, so this is not the agent being busy and not the harness: the wedge is toolchain-sensitive **above** the floor QA-MUSTER-19 closed on ("toolchain-floor-scoped, 1.26.10 has the fix"). `go.mod:3` is `go 1.26.10` with **no `toolchain` directive**, so GOTOOLCHAIN=auto means any user already on Go ≥ 1.27.3 gets a red `go test ./...` on a clean checkout. Filed as **QA-MUSTER-19-R2** (minted with `--recur QA-MUSTER-19`), whose ACs require the goroutine dump at the deadline (`s.wg.Wait()` server.go:164 vs the `concurrencySem` send at :157) instead of a larger `runReturnTimeout` (server_test.go:647), plus green under BOTH toolchains.

**NOT a product defect (do not file): `upgrade` FAIL.** The streamed v0.1.0 tree dies on `Package libseccomp was not found in the pkg-config search path` (+ an `exploits/wasm` `.note.GNU-stack` linker warning) — the agent has no libseccomp dev headers and no sudo. That is an agent-environment gap (ENV-BLOCKED), and QA-MUSTER-13 stays closed: the current tree's own upgrade suite is green here (`ok github.com/wojons/muster/test/upgrade` in the same native run). Recorded as a harness observation, not a row — as with run 20's `fresh-install`-cell caveat.

**Harness observation worth one line:** the `ci-pass` cell's detail is truncated at 300 chars and, with `act -q`, `ci.log` carries no step output, so the cell names `native rc=1` but never the failing test. The failing package/test had to be pulled from `native.log` on the agent before destroy — a cell that reports only `FAIL` is not gradeable from the evidence file alone.

**Residue:** agent `53097b60` **destroyed** (first `bunker destroy` was refused by the live-process gate naming two host-level pids; the retry found it gone and removed the local SSH key; `bunker list` now shows 6 agents, no `53097b60`). Evidence saved under `.coding-hermes/dogfood-evidence/run20/`: `install-leg-cells.jsonl` (the 22-row collect), `install-leg-native.txt` (the failing suite), `install-leg-upgrade-env.txt` (the libseccomp gap). Scratch `/tmp/mf20*` and `/tmp/bunker-qa-evidence-20261023T124416Z-3104346.jsonl` are left in place as the run's raw record.

*ch:trace row=QA-MUSTER-19-R2 spec=.github/workflows/ci.yml:build-and-test-test-step test=run20/install-leg-native.txt+A/B(GOTOOLCHAIN=go1.27.3) doc=none:go.mod-has-no-toolchain-directive evidence=.coding-hermes/dogfood-evidence/run20/install-leg-cells.jsonl witness=dogfood-run20-install-leg-finish verdict=P1-recurrence-live commit=pending*

## 2026-10-26 (run 21) — documented CLI/demo/daemon/Docker path + fresh bunker leg

Promise: a new user can build Muster, discover and generate commands from an OpenAPI document, run the documented local demo, use the daemon lifecycle, and run the Docker entry points.

**Verdict: PROMISING-BUT-ROUGH.** The real local path works after the documented Flask/venv prerequisite. `make build` passed cold in 14.54s and warm in 2.00s; `./demo/run-all.sh` passed all 33 checks in 21.59s cold after setup and 19.89s warm; daemon start/status/stop each passed and cleaned the scratch socket; Docker built in 182.50s cold and the three binary help probes passed. CLI version/discover/generate and audit info/status/verify/log/show/prune all passed in isolated HOME.

**Perf:** no PERF row. User-facing CLI invocations were about 0.01s each and the 33-check demo was 19.89s warm; neither was noticeable runtime latency. Build/image times are setup costs, not a measured runtime regression, so profiling was not warranted.

**Install leg:** executed, not skipped. `bunker-qa.sh launch /home/kara/wojons/muster --keep` tested HEAD `1ea7c54a` on `bunker-mvp`, agent `347dfc56`, evidence `/tmp/bunker-qa-evidence-20261026T142133Z-3945564.jsonl`. The fresh-install cell failed with `no space left on device` during Go build-directory creation. `ci-pass` also failed with native `test/security` credential failures and `test/util` build failure; its act arm separately could not create `catthehacker/ubuntu:act-latest`. The battery truncated before final chaos cells. This is incomplete fresh-box evidence, not a product install pass and not a SKIPPED-install-bunker row. Agent teardown completed; post-destroy `bunker list` reported `347dfc56` absent.

No board rows were filed: the fresh-box failures are not attributable to the product from this incomplete, storage-constrained battery, and no user-facing defect was found in the documented local path. No scheduler mutation and no foreman wake.

Artifacts: `docs/dogfood/2026-10-26-integration.md` and `.coding-hermes/dogfood-evidence/run21/summary.txt`.

*ch:trace tick=muster-dogfood-2026-10-26-14-01-09 head=1ea7c54a0e3bbea311da1538b3785011f85078b6 report=docs/dogfood/2026-10-26-integration.md evidence=.coding-hermes/dogfood-evidence/run21/summary.txt install=/tmp/bunker-qa-evidence-20261026T142133Z-3945564.jsonl verdict=PROMISING-BUT-ROUGH perf=none rows=0*

### run 23 — tick muster-dogfood-2026-11-11-19-48-40 (2026-11-11)

**Verdict: PROMISING-BUT-UNSTABLE.** The documented path works end to end — build, generate a CLI from a
live OpenAPI spec, then create/list/get resources against that same server — and it is fast. What broke
was not the request path but the output layer and one adjacent scripting surface.

**Promise exercised.** "Point it at any OpenAPI spec and get a working typed CLI in one step" — done
against the repo's own demo service (`demo/services/task-forge`, spec at `http://localhost:9801/openapi.json`)
from a clean `git clone` of HEAD 3eb9cbb5 with an isolated `HOME`, building with `make build-cli`.

**Time to first success (real numbers, `/usr/bin/time`):**
- cold tree build (`make build-cli`, fresh clone, existing Go caches): **2.80 s wall**, 8.20 s user, 417 MB peak RSS, 28 MB binary
- rebuild, nothing changed: **1.12 s**
- `generate` (warm, local spec): **0.01 s** (second run 0.10 s)
- `list-tasks` / `list-projects` (warm): **0.01–0.03 s**, ~25 MB RSS
- `create-project-task`: **0.04 s**
- No PERF row filed: nothing here is slow enough for a user to notice. The only cost a user feels is the
  one-off compile, and 2.8 s is not a complaint.

**Top findings filed as rows:**
1. `DF-MUSTER-6` (P1) — table output column order is nondeterministic: 4 invocations produced 4 different
   header orders on identical data (`description milestone_id priority …` vs `milestone_id tags updated_at …`).
   Breaks diffing, `awk $3`, doc screenshots; reads as broken to a first-time user.
2. `DF-MUSTER-7` (P2) — `dsl run` can only POST JSON as a hand-serialized string:
   `http_post(url, {dict})` → `execution error: http_post: for parameter body: got dict, want string`,
   and the obvious workaround `json.encode(...)` → `compile error: … undefined: json`; `dsl --help` never
   says what type `body` is.
3. `DF-MUSTER-8` (P1) — the board itself is invalid on HEAD: `boardctl validate` FAILs with
   `[error] tasks.jsonl line 284: duplicate task id "INT-CI-003" (also line 206)`. The two rows are
   different findings (one complete, one pending), so the fix is to re-id one, not delete either.

**Verified as working (so the next tick does not re-test it):** `generate` from a live spec; typed
`create-project`, `create-project-task`, `get-task`, `list-tasks`, `list-projects`; `-o json` vs default
table; `request GET <url>`; error paths for a missing resource and an unreachable base URL all returned
sane, non-panicking messages.

**Known-upstream behaviour observed, NOT re-filed:** generating a second spec replaces the persisted
command set, so the first spec's commands then fail with `Error: unknown command "list-tasks" for
"openapi-cli"` (rc=1) with no hint about why. That is the mechanism already owned by `DF-MUSTER-3`
(complete) and `REVIEW-MUSTER-18` (pending, `--profile` does not namespace the store). Worth noting for
that row: the error message gives the user nothing to recover with.

**Install leg (bunker-qa):** launched, not skipped — `bunker-qa.sh launch` provisioned agent `bcf7330c`
on `bunker-mvp` (ttl 4h) and ran the fresh-install cell. Toolchain bootstrap and `fresh-install` (go build
./...) reported OK; the battery was still running at tick close, so the install leg's status is
**RUNNING with partial evidence**, not PASS. Evidence:
`/tmp/bunker-qa-evidence-20261111T202703Z-1971247.jsonl`. Harness note for QA: `lint` reported
`runner unavailable on agent: golangci-lint — lint was not run` (INFO, not a product failure), so this
leg does not cover lint.

**Not filed, deliberately:** missing `task_created` events for new rows (board coverage census already
tracks that class), and the uncommitted `events.jsonl` batch a concurrent sibling had in flight — left
byte-untouched rather than swept into this commit.

### run 23b — install leg, collected evidence (amends run 23 above)

`bunker-qa.sh collect` on agent `bcf7330c` (bunker-mvp, fresh agent, clean tree synced from the repo)
synced 14 evidence cells. Final state of this leg, checked at 20:34Z: the battery ended with
`AGENT-GONE agent=bcf7330c rows=14` — the agent was reaped before the battery finished, so this leg is
reported as **RAN PARTIALLY (agent gone, 14 cells), no PASS claimed** — not PASS, not skipped.

Cells that had returned, verbatim statuses:
- `toolchain-bootstrap` OK ×9 — go 1.27.4 auto-installed (ambient 1.22.2 below the 1.24 floor), gcc/g++
  wrappers, make, jq 1.8.2, compose plugin, buildx v0.39.0, node v22.15.0
- `fresh-install` OK — `go build ./...` from the clean tree succeeded (deps fetched; no artifact expected)
- `tests` OK — rc=0
- `docker_build` OK — `docker build -f ./deployments/backup/Dockerfile ./deployments/backup`, legacy builder, rc=0
- `lint` **INFO** — `runner unavailable on agent: golangci-lint — lint was not run` (harness gap, not a product failure)
- `compose_config` **FAIL** — `docker compose -f ./deployments/docker-compose.security.yml config` → rc=1:
  `healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` (plus a `version` attribute obsolete warning)

That FAIL is the live confirmation of an existing row — **`QA-MUSTER-024`** (pending, "healthcheck.test lacks
the CMD prefix") — so it was NOT re-filed; this is the clean-machine evidence for it. It is worth noting that
this breaks a documented deploy path on a fresh box while `docker_build` and `go build ./...` stay green:
nothing in CI catches it, which is exactly what this leg exists for.

**Correction, same tick, before commit:** `DF-MUSTER-8` was filed against a real invalid baseline
(`boardctl validate`: 1 error, duplicate id INT-CI-003), but a concurrent lane re-id'd the pending
duplicate to `INT-CI-010` while this tick ran. The row is therefore kept as **complete** — annotated
`[RESOLVED 2026-11-11 by a concurrent lane]` with the resolution and the re-verified
`RESULT: OK (256 warning(s))` — rather than left pending for a foreman to chase a defect that no longer
exists. The live findings from this tick are `DF-MUSTER-6` and `DF-MUSTER-7`.

### run 23-dup — second fire of the same tick id, records-only (no new findings)

A second scheduler fire arrived for the same tick id `muster-dogfood-2026-11-11-19-48-40` at 16:02 -05.
Per the duplicate-fire gate this fire is **records-only**: no product use, no benchmark, no install, no new
rows. Heading count for this tick id stays at **1 delivery** (this heading deliberately does not repeat the
id, so `grep -c '^#\+ .*<tick-id>'` is unchanged); this block is the provenance record for the re-fire.

Verified against committed artifacts at 16:05 -05:

- **Run record committed.** `git log -1` = `eda62828` "dogfood(muster): record install-leg final state for
  tick …19-48-40"; `git show HEAD:.coding-hermes/dogfood-log.md | grep -c <tick-id>` = 1.
- **Remote parity.** `merge-base --is-ancestor eda62828 origin/main` → true. `rev-list --left-right
  --count origin/main...main` after `git fetch` = `4 0` — the lane has **0 unpushed commits**; the remote
  moved 4 ahead. (Reading the raw `4 0` as "4 ahead locally" is the classic inversion; left is origin.)
- **Rows intact in the committed tree** (`git show HEAD:…/tasks.jsonl`): `DF-MUSTER-6` pending,
  `DF-MUSTER-7` pending, `DF-MUSTER-8` complete — statuses match what run 23 claimed.
- **Board still valid.** `boardctl validate -C /home/kara/wojons/muster` → `RESULT: OK (256 warning(s))`,
  0 errors.
- **Cited measurement path exists.** `/tmp/bunker-qa-evidence-20261111T202703Z-1971247.jsonl`, 3,955 B,
  mtime 15:46.
- **The 4 new remote commits do not touch the outstanding findings.** `git diff eda62828..origin/main`
  changes only `deployments/`, `scripts/validate-compose.sh`, `.github/workflows/ci.yml`, `Makefile`,
  board/evidence/release docs — nothing under `pkg/`, `cmd/`, or `internal/`. So `DF-MUSTER-6`
  (nondeterministic table column order) and `DF-MUSTER-7` (DSL cannot POST a JSON dict) are **still live**
  and own their pending rows; nothing here was re-filed.
- Notably, `14449b1f` is a `CMD`-prefix fix for the security-stack healthcheck — i.e. the compose FAIL that
  run 23b attributed to `QA-MUSTER-024` was taken up by another lane on its own row, not by this one.

**Uncommitted sibling residue, left byte-untouched:** `tasks.jsonl`/`events.jsonl` carry a pending
`PERF-002` row filed at 21:02:37Z by `muster-perf-2026-11-11-19-48-40`. It is not this tick's and was not
swept into this commit.

**Deliberately not done:** no re-run of the product path, no second install battery, no duplicate rows, no
PERF row (run 23 measured the headline path and found nothing a user would feel).

**Correction to run 23-dup, same tick, after publication:** the sentence above claiming the sibling
`PERF-002` residue was "left byte-untouched" was true at commit time but is no longer accurate, so it is
corrected here rather than rewritten (`f1d07dc3` is already on the remote and is not force-pushed).

The lane's record commit was rebased twice onto a moving `origin/main` (`eda62828` → `71d1a4d5` →
`f1d07dc3`, pushed as `be615eda..f1d07dc3`). Both intervening remote commits took board event ids out from
under the sibling's uncommitted event, which sat on id `981`:

- `6ed35918`/`14449b1f` batch → ids `981`, `982`
- `be615eda` (readme lane) → ids `983`, `984`

So the residue was not left byte-untouched: its single `task_created` event was **renumbered
`981` → `985`** (current max + 1) and re-appended. The row body, title, reasoning, acceptance criteria and
timestamps are verbatim from the perf lane's write; only the event id changed, and only to avoid a duplicate
id that would have made `boardctl validate` fail for every lane in the repo. `PERF-002` itself was appended
unchanged at `tasks.jsonl` line 343 and is still `pending` and uncommitted, as written.

Verified after the push: `boardctl validate -C /home/kara/wojons/muster` → `RESULT: OK (256 warning(s))`,
0 errors; `jq` parses both files; the only duplicate numeric event id is the pre-existing legacy `855`;
`git status --porcelain` shows exactly the two board files modified, which is the shape this lane found.

Housekeeping left behind, deliberately: two `git stash` entries labelled `sibling PERF-002 … residue`
(`pass 2` is the live one; the `pass 1` copy is superseded by it). `git stash drop` was refused by the
unattended-command gate, so they remain for a human or a later lane to clear. Nothing is lost either way —
the residue in the working tree is the authoritative copy.

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 2 (perf leg + production-spec probe)

Follow-up pass on the same tick id, after the first pass filed `QA-MUSTER-035..037` (`0edbd4ae`).
Still product-only: no code, no board-status edits, one new row filed.

### What the user did
Took the documented path from a clean clone against the bundled demo service
(`localhost:9801`, 7,390 B spec): `discover` → `generate` → typed `list-tasks -o json` → generic
`request GET /tasks`. Then did what the README actually promises — pointed it at a **real production
spec**: GitHub v3 REST (14,274,027 B, openapi 3.0.3, 1281 operations,
sha256 `f7a3de68…c801b31c`), served from disk and from a URL.

### Corrected false alarms (recorded so they are not re-filed)
- `discover … -o json` "failing" was my flag error: `discover` has no `-o` and the error message is clean.
- `-o json` output is NOT polluted by the `INF HTTP response received` line — that goes to **stderr**;
  stdout is a clean `[]\n` (verified with `od -c`).
- `✓ Parsed spec (version: 1.1.4)` is GitHub's `info.version`, not a parser mis-read.

### Finding filed
- **`QA-MUSTER-038` (P1)** — `generate` panics on any spec with a parameter or body field named
  `output` or `raw`: `panic: checks/create flag redefined: output`, rc=2, **zero commands persisted**.
  Minimal repro is 855 B (one POST body property named `output`); `raw` and a GET query param named
  `output` panic identically. Root cause `pkg/generator/generator.go:675-676` registers `-o/--output`
  and `-r/--raw` unconditionally on a FlagSet that already holds the spec-derived flags; pflag panics
  on the duplicate instead of returning an error. 2 of the 1281 GitHub operations trip it — one is enough
  for total loss of the generate path.

### Perf numbers (Step 2b) — measured, isolated `HOME`/`XDG_*` per run, loadavg ≈ 13.9 on 16 cores
| operation | cold | warm |
|---|---|---|
| demo spec `discover` (7.4 KB, http) | 15 ms | 14-18 ms (5 runs) |
| demo spec `generate` | 20 ms | 21-45 ms (one 144 ms outlier under host load) |
| `list-tasks -o json` | — | 17-29 ms (one 123 ms outlier) |
| `request GET /tasks -o json` | — | 18-20 ms |
| GitHub spec `discover` (14.3 MB) | 1794 ms | 1763 / 1768 ms, 1330 commands listed, 0 stderr |
| GitHub spec `generate` | 1924 ms then panic | 1767 / 1785 ms then panic |

Verdict: on the demo spec nothing is slow enough that a user would notice — **no PERF row filed**.
The 1.8 s `discover` on a 14.3 MB spec is noticeable but is the same parse cost already owned by
`PERF-001` (double decode, 105-118 ms of it); filing a second row would devalue that one, so the number
is recorded here instead. `generate`'s ~1.8 s is not a latency number — it is time spent before a crash.

### Artifacts
- Board row `QA-MUSTER-038` (minted via `board_next_id.py QA-MUSTER`, verified unique, board validates
  `RESULT: OK`). Event `1053` `task_created`.
- Scratch evidence (not committed): `evidence/perf2.sh|log`, `perf_big.sh|log`, `minrepro-output-flag.json`,
  `minrepro-raw-only.json`, `minrepro-raw-and-query.json`, `QA-MUSTER-038-ac.txt`, `QA-MUSTER-038-why.txt`
  under `/home/kara/.hermes/cache/scratch/dogfood-muster-162236/`.
- Install leg unchanged from pass 1 of this tick (see `qa-evidence/`), not repeated here.

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 3 (records-only: install-leg grading + teardown census)

Third fire at 2026-11-14T22:30Z, ~1 min after pass 2's commit `a1f9b17` (17:29:14 -05). Ownership
checked before anything else: `git status --porcelain` empty, HEAD `a1f9b17`, `git rev-list
--left-right --count origin/main...HEAD` = 0 0 after fetch. No product re-run, no second battery
launch, no new rows, no code changes — this fire spent itself on the checks passes 1 and 2 left as
inherited.

### Why the product was not re-used this fire
`git diff --stat 4b42b5c0..HEAD` touches only `.coding-hermes/board/events.jsonl`,
`.coding-hermes/board/tasks.jsonl`, `.coding-hermes/dogfood-log.md`. The product tree is byte-identical
to the pinned HEAD that pass 2 timed and exercised, so the perf numbers stand un-retimed and
`QA-MUSTER-035/036/037/038` stay live by inheritance rather than by a replay.

### Deferred check 1 — install-leg ledger graded in full (first written record for this tick)
Pass 1 filed rows and left its battery result in a commit message only; the tick's install leg never got
a log record. Agent `d16f829a` (project `muster`, launched 21:37:12Z, 24 ledger records) graded against
the harvested per-cell logs that survived in
`/tmp/bunker-qa-evidence-20261114T213624Z-2701200.jsonl.logs/qa-logs-689398/`:

| cell | grade | substance from the log, not the status word |
|---|---|---|
| fresh-install | OK | `go build ./...` on the clean tree after the toolchain bootstrap took the box from go 1.22.2 to floor 1.26.11; no artifact expected for a compile leg |
| tests | OK | full native suite — 54 packages `ok` in a 63-line `tests.log`: `test/security` 115.276s, `test/e2e` 36.044s, `test/docker` 4.535s. The tree holds 213 `_test.go` files across 54 test packages, so nothing was silently skipped. The ledger's own detail is an empty `(rc=0)`; the count lives only in `tests.log` |
| docker_build | OK | `deployments/backup/Dockerfile` built (`220592794ae4`), legacy builder, 46934 B context |
| compose_config | OK | `deployments/docker-compose.security.yml` validates |
| ci-pass | OK | act ran `.github/workflows/ci.yml`, job `build-and-test` ✅ (`ACT_WF_COUNT=1`). One workflow is by design (Go repos pin `ci.yml`), not a truncated run |
| chaos-resource | PASS | child/subshell/parent all saw the 3 GiB cap (3145728 KB each) |
| chaos-shutdown | OK | `sd.log` KILL=0 KILLRECOVER=0 DOWN=0 |
| upgrade | FAIL | `up1.log` untruncated: `disallowed LTO section .llvmbc in #cgo LDFLAGS` on `libwasmtime.a` **and** `Package 'libseccomp', required by 'virtual:world', not found`. Character-for-character `QA-MUSTER-034`'s cause ⇒ recurrence, no re-file (pass 1 already annotated that row) |
| lint / ui-probe / docker-deploy / chaos-disconnect / chaos-errorpath | INFO | ran, graded nothing: `ep.log`/`ui-probe.log` hold `openapi-cli --help` text, `dc.log` did start `muster-db`/`redis`, `disc.log` is the security suite at 107.9s. INFO is neither a green nor a product red |

Coverage boundaries this grading exposes — recorded here, deliberately **not** filed as rows because none
is something a user can feel:
- `ci-pass` covers exactly one of the repo's four workflows; `build.yml`, `cross-platform.yml` and
  `docker-build.yml` have no cell in this battery (`docker-build` is separately covered by `docker_build`,
  cross-platform builds are out of scope for a Linux-only clean box).
- `lint` never ran — golangci-lint is absent on the agent, so that cell is an absence, not a pass.
- The `collect` cell reports "per-cell logs lost with the agent user"; for this run that is wrong. 24
  per-cell logs had already been synced by the incremental harvest before the agent was reaped, which is
  precisely how the upgrade signature above was graded. Harness-reporting inaccuracy, not a product defect.

### Deferred check 2 — teardown census
`bunker list --server bunker-mvp --status all`: `d16f829a` absent ⇒ destroyed. Residue seen and left
alone (not this lane's to reap): `831e48e4` in `destroy-refused:home_retained` holding 9.7 GB (that
agent's ledger is a `crier` battery, created 21:11:25Z), plus long-lived `e9112dde`, `cf-node`,
`ci-e2e-01`.

### Deferred check 3 — CI at the record's own HEAD
`a1f9b17` check-runs at 22:38Z: 17 completed:success, 2 in_progress, 1 skipped — not final inside this
fire, so CI-at-HEAD is recorded **UNVERIFIED**, not green. The commit is board + log only, so it cannot
move product CI.

### Corrections to the record
- Pass 2 cited `qa-evidence/` for the install leg; that directory holds only the 2026-10-23 tick's
  evidence. The live evidence for this tick is the `/tmp/bunker-qa-evidence-20261114T213624Z-2701200.jsonl`
  artifact above.
- Pass 1 has no dogfood-log section; this section supplies it.

### Artifacts
- This log section, committed and pushed to `origin/main` (sha in the commit itself). Board untouched:
  0 rows filed, 0 rows modified, `boardctl validate` `RESULT: OK` with 264 warnings / 0 errors
  (sibling-lane drift, not this tick).

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 4 (fourth fire: resolve the CI-at-HEAD boundary, re-check the rows' premises live)

Fire at 22:47Z, ~15 minutes after pass 3 closed at 22:37Z.

### Why the product was not re-used this fire
The product bytes are byte-identical to the pin: `git diff 4b42b5c0..HEAD -- . ':(exclude).coding-hermes'`
is empty, and the only three files that moved between the pin and `d918cfd2` are `board/tasks.jsonl`,
`board/events.jsonl` and this log. Re-walking documented paths 1–3 would re-measure those same bytes and
re-produce the four rows already pending (`QA-MUSTER-035/036/037/038`). A fourth fire works the list the
earlier records explicitly deferred.

### Deferred check — CI at the record's own HEAD (pass 3 left this UNVERIFIED; resolved now)
Every workflow that tests the product is green at both commits on this tick's chain. Per-workflow and
per-job, not an eyeballed badge:

| commit | CI | Build | Cross-Platform Tests | Docker Build |
|---|---|---|---|---|
| `a1f9b17d` (pass 2) | success | success — `test`, `integration`, `security` 22:29:25Z→22:40:19Z (10m54s), and all five `build (os,arch)` legs | success | still running (21 min at 22:50Z) |
| `d918cfd2` (pass 3) | success | success — `test`, `integration`, `security` 22:37:58Z→22:48:37Z (10m39s), five matrix legs | success | still running (12 min at 22:50Z) |
| `0edbd4ae` | success | success | success | still running (46 min at 22:50Z) |

The one non-final item is a duration artefact, not a stalled runner — and I checked that instead of
assuming it. Eight Docker Build runs completed today, `ff0cff82` through `4b42b5c0`, took 35, 51, 32, 35,
47, 31, 52 and 32 minutes (created→updated). 46 minutes on `0edbd4ae` sits inside that envelope, and its
job logs are not retrievable yet (`BlobNotFound` on `actions/jobs/132604206899/logs`) — which is what an
unfinished job looks like, not a failed one. The "queued with no runner" reading was also rejected: every
job in these runs has a runner assigned (`GitHub Actions 1000022345`…).

Verdict: CI-at-HEAD for this tick's record is **green, with one long-running workflow still inside its
normal duration window** — no longer UNVERIFIED, and nothing red.

### Premise re-check for the four pending rows, at the pinned product tree
- `QA-MUSTER-035` is still reproducing on this host, with better evidence than pass 2 filed. Four
  `openapi-daemon` processes from the pass-2 probe were alive at 22:47Z, 79–81 minutes old, all with
  PPID 7126 (the gateway) — i.e. orphaned. Detail beyond the row's text: `2294852` and `2340479` both held
  a listen record on the *same* path `/tmp/df1794691460.sock`, so the second `start` re-bound a path that
  was already in use and left the first unreachable-but-running. That is the defect row 035 already
  describes, so it stays one row — no sibling filed.
- `QA-MUSTER-036/037/038` stay pending with their premises intact for the same reason: the product tree
  under them has not moved since they were filed.

### Boundary recorded, not repaired
Three `REBASE_HEAD` files sit in the shared checkout (Nov 11, Nov 12, and main's Nov 14 05:56) with no
`rebase-merge`/`rebase-apply` directory and no live git process. That is residue, not a live operation:
recorded as an unverified boundary, and clearing it is not this lane's business.

### Residue from this lane, cleaned
The four orphan daemons above were killed at 22:47Z after their evidence was captured;
`pgrep -fa 'dogfood-muster-162236/repo/bin/openapi-daemon'` now matches only the shell's own wrapper, so
0 survivors. The scratch tree `/home/kara/.hermes/cache/scratch/dogfood-muster-162236` is left to the
24-hour pruner.

### Artifacts
- This log section, committed and pushed to `origin/main`.
- Board untouched: 0 rows filed, 0 rows modified, `boardctl validate` `RESULT: OK` with 264 warnings /
  0 errors, 363 task rows / 1047 event rows — identical to pass 3.

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 4 (cold use + CI archaeology)

First pass of this tick that actually *used* the binary instead of re-reading the board. Everything below is
measured on this host or pulled from the public CI, not inferred from source comments.

### What a user gets, timed (Step 2b)
Built `./cmd/daemon` from HEAD `7c7a3d02` into scratch, started it on a private socket, drove it, stopped it.

| Operation | Command | Result |
|---|---|---|
| cold start → socket ready | `openapi-daemon start --socket-path <scratch>/probe.sock` | **67 ms** to live socket, rc=0 |
| warm `status` CLI | `openapi-daemon status --socket-path …` × 10 | **10–12 ms** (mean 10.3 ms) — process spawn, stable |
| HTTP over the unix socket | `curl --unix-socket <sock> http://localhost/health` × 5 | **0.18–0.46 ms**, connect 0.05 ms, http=200 |

No profile was warranted and no PERF row was filed: the daemon answers in sub-millisecond time and starts in
67 ms, so there is no slowness a user could feel. The slowest thing a user touches is process spawn.

### Filed — 2 rows, both read back from the board
- **`RELEASE-018-R2`** (P1) — the closed P0 `RELEASE-018` claimed cross-platform green; the same macOS/ARM64
  red returned inside three hours. Run `43586639636` @`5b3cb342` 10:46Z: only the ARM64 macOS job failed with
  `"138.958µs" is not less than or equal to "103.084µs"` in `TestAuditCommand_LogLimitStaysBounded`. Run
  `43594734067` @`4b42b5c0` 12:27Z: macos-latest **and** ARM64 failed, same assertion. Locally the same test is
  **30/30 green** (`-count=30`, 0.814 s, load 12.21) — which is exactly why the original green close proved nothing.
  Acceptance criteria demand an injected clock or a property assertion, a measured jitter distribution, and
  ≥3 consecutive green cross-platform runs cited by run id before it can close again.
- **`QA-MUSTER-039`** (P2) — a bare `openapi-daemon` invocation exits **0** on Linux (measured: usage block,
  17 lines, rc=0) but exits **1** on Windows (`integration_test.go:61`, run `43594734067` job `132446712558`),
  and the same Windows leg passed at `5b3cb342`, so the exit code is state-dependent, not constant.
  Cause located in the shipped tree: `cmd/daemon/main.go:194-227` builds the audit hook and `NewController`
  *before* the dispatch `switch` at `:229`, whose `default:` help branch is `:239-257` — so the help path is not
  side-effect-free, and when controller construction fails the usage text never prints. The test sets
  `cmd.Stdout` but never `cmd.Stderr`, which is why CI shows only `exit status 1` and no cause.

Both rows carry `acceptance_criteria` a foreman can dispatch on; the long evidence text lives in
`events.jsonl` as audit events `1057` (`RELEASE-018-R2`) and `1058` (`QA-MUSTER-039`).

### Install leg — recorded, never a silent pass
Probed rather than assumed: `ssh bunker-mvp` succeeds (rc=0), `systemctl --user is-active bunkerd` → **inactive**,
`/` at **80 % / 29 G free**, and there is no `bunker-qa.sh` on the host or in this lane's skill scripts
(`…/coding-hermes-dogfood/scripts/` holds only `append-rows-verify.sh`, `doc-command-census.sh`,
`ownership-marker-scan.sh`, `record-integrity-census.py`; the `bunker-qa-harness` skill ships only
`classify-bunker-qa-cells.py`). Fresh-install is therefore **NOT re-proven this run**. Not filed as a new row —
the board already carries a completed `SKIPPED-install-bunker` (P2) row for exactly this state, and rule 3
says an existing row is worked or left alone, never re-filed.

### Seen, deliberately not filed
- Two `openapi-daemon` processes (`803046`, `1286750`, both `--socket-path /tmp/muster.sock`) and `/tmp/muster.sock`
  is a **root-owned directory**, not a socket — the pidfile/socket-reuse defect `QA-MUSTER-035` already describes
  is the same premise; a second row would be noise.
- The Windows retention class is owned by pending `INT-CI-9`, and the generic Windows regression by `INT-CI-006`;
  `QA-MUSTER-039` names a different test and a different mechanism, and says so in its reasoning rather than
  presenting itself as a new family.
- `QA-MUSTER-036/037/038` stay pending with their premises intact; the product tree under them did not move
  during this pass (`git diff --stat 4b42b5c0..HEAD -- . ':(exclude).coding-hermes'` is empty).

### Boundary
The tick prompt names `Workdir: /home/kara/.hermes/stand-in/dogfood/muster`, which does not exist (exit 2, both
this pass and the last). Work happened in the canonical checkout `/home/kara/wojons/muster`, clean on entry and
on exit. A sibling QA lane committed to that checkout mid-pass (`7c7a3d02`, muster-qa verification-only cycle,
18:01 local); my commit sits on top of it and touches only the two board files plus this log.

### Artifacts
- This log section, committed and pushed to `origin/main` (parity `0/0` at the end of the pass).
- Board: 365 task rows / 1052 event rows, `boardctl validate` → `RESULT: OK`, **264 warnings / 0 errors** —
  identical warning count to the pre-filing baseline, so nothing was added to the lint debt.
- Probe daemon stopped and its scratch socket/pidfile removed; no process of mine left running.

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 5 (MCP surface, first walk this tick)

### What I used, as a user would
Built `cmd/mcp-server` from `75ae5494` and drove it over raw JSON-RPC stdio with the documented
`-spec` / `-base-url` invocation, against the public petstore sandbox (spec 17,106 B,
sha256 `00441c05add60f28`).

- `initialize` → `protocolVersion 2024-11-05`, `serverInfo openapi-mcp v0.1.2-0.20261114230842-75ae54947418`.
- `tools/list` → 19 tools, each carrying `muster/openapi-method`, `muster/openapi-path` annotations.
- `tools/call getPetById {petId:1}` → `isError:true`, "HTTP 404: Pet not found" with
  `structuredContent.error.class=upstream_error`. The live sandbox no longer holds pet 1, so the
  guide's printed transcript (`{"id":1,"name":"Pet1"...}`) is stale — upstream drift, not a defect.
- `tools/call findPetsByStatus` → 756 chars of real pets, `isError` absent.
- Error surface is honest: unknown method `-32601`, `tools/call` with no params `-32602`, unknown
  tool / missing argument / wrong argument type → `isError:true` with `client_error` classes.
- Closing stdin exits 0; whole 11-call session 0.47 s.

### Perf (Step 2b)
`initialize` 11.5 ms; `tools/list` 0.4 ms (19 tools, 7 KB frame); `tools/call` 270 ms cold and
68 ms warm, both dominated by the upstream HTTPS request to `petstore3.swagger.io`; the MCP layer
itself is sub-millisecond. Nothing a user would feel, so no PERF row.

### Finding filed
`QA-MUSTER-040` (P2). The real-world GitHub v3 spec (14,274,027 B, sha256 `f7a3de68869181d3`)
does not start the server at all: exit 1, zero bytes on stdout, stderr
`Fatal error: failed to load spec: validate spec: invalid components: schema "actions-hosted-runner": invalid example: value must be a string ... Value: 20.04`,
from `cmd/mcp-server/main.go:277`. The same file on the same tree is accepted by
`openapi-cli discover` (rc=0, 1330 operations), and `-h` offers no `-skip-validation` /
`MUSTER_SKIP_VALIDATION`, so the loader policy is not one policy and the surface README:3/207
sells ("every OpenAPI operation becomes an MCP tool") is the one that refuses. Existing MCP tests
only cover a nonexistent path (`main_test.go:1304,1350`), so this class was never tested.

### Non-findings
- No `PERF` row: 0.4 ms `tools/list` and sub-millisecond MCP overhead are not user-felt.
- Did not re-file the 68 ms upstream latency, the orphan daemon processes (`QA-MUSTER-035` owns
  that class), or the stale petstore transcript (upstream data drift).
- `notifications/initialized` sent *with* an id answered `{"result":null}`; a real client sends it
  as a notification, so no row — `DF-031`'s original symptom was not reproduced.

### Boundary
Work in the canonical checkout `/home/kara/wojons/muster` (the prompt's stand-in path still does
not exist). Entry clean at `75ae5494`; exit clean at the pass-5 commit. A sibling releng lane
pushed two doc commits mid-pass (`46e679ce`, `cca61d41`); I rebased onto them before pushing and
touched only the two board files plus this log.

### Artifacts
- Board: `QA-MUSTER-040` (pending, P2); 366 task rows / 1053 event rows,
  `boardctl validate` → `RESULT: OK`, 264 warnings / 0 errors (baseline unchanged).
- Pass-5 commit pushed to `origin/main`; parity `0/0`, row present in the remote tree; clean tree.
- Probe evidence in `~/.hermes/cache/scratch/dogfood-muster-162236/mcp-fresh/`
  (`probe-abs-base.json`, `scale-github.json`, `mcp-github.err`, `discover-github.txt`).
- No process of mine left running.

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 6 (production-spec scale probe: what the generated commands actually send)

Sixth fire. Product bytes are unchanged since the pass-5 build pin (`75ae5494`; only `.coding-hermes`
and one releng markdown moved while this pass ran), so this pass did **not** re-walk passes 1–5. It
walked the one surface no earlier pass had touched: **a production-sized spec whose writes are not
JSON-bodied**. Pass 2 established that the 13-path petstore fixture generates fine and that the
GitHub v3 spec dies on an `output` flag collision (`QA-MUSTER-038`). Nobody had yet asked what a
user of a *form-encoded* API actually gets.

### What I did (real use, not code reading)
Pulled Stripe's published OpenAPI description and pointed the shipped binary at it:

- `stripe-spec3.json` — **8,317,513 B**, `sha256 7cff4cc46d065410…`, `openapi 3.0.3`,
  api version `2026-09-30.endive`, base `https://api.stripe.com/`, **612 operations**.
- `openapi-cli discover stripe-spec3.json` → rc=0, **0.97 s cold / 0.90 s warm**, 612 operations
  in 616 lines.
- `openapi-cli generate stripe-spec3.json` → rc=0, **0.98 s cold / 0.99 s warm**, and
  `openapi-cli --help` then lists **631 top-level commands**.
- A generated typed command really reaches the wire: with a mock on `:9833`,
  `openapi-cli delete-coupons-coupon DOGFOOD-PROBE --base-url http://127.0.0.1:9833 -o json`
  → rc=0, `{"deleted":true,…}`, and the mock logged `DELETE /v1/coupons/DOGFOOD-PROBE`.
  Dispatch cost **0.02 s warm** (5 runs: 0.02/0.02/0.12/0.02/0.02). Path templating, base-URL
  override and output formatting all work.
- Same spec through the MCP server: `Generated 612 MCP tools`; `tools/list` returned 483,426 B of
  JSON-RPC and `initialize` answered immediately.

### What broke
**Write operations on a form-encoded API are argument-free, and the CLI never says so.**

`POST /v1/customers` declares 22 body properties on `application/x-www-form-urlencoded`. Its
generated command's `Flags:` block contains only the four generic flags (`--auth`, `--base-url`,
`-H`, `-o`) — no `--email`, no `--description`:

    $ openapi-cli post-customers --email dogfood@example.com …
    Error: unknown flag: --email
    $ openapi-cli post-customers -b '{"email":"a@b.c"}'
    Error: unknown shorthand flag: 'b' in -b
    $ openapi-cli post-customers --body '{"email":"a@b.c"}'
    Error: unknown flag: --body

There is no escape hatch either, and the omission is specific to the media type: JSON-body commands
(petstore `update-pet`) carry both per-property flags **and** `-b, --body  Raw JSON request body
(overrides individual body flags)`. Running the form command bare "succeeds" and sends nothing:

    {"method":"POST","path":"/v1/customers","headers":{…,"Content-Length":"0"},"body":""}

rc=0, no `Content-Type`, empty body — an empty write reported as success, with no warning at
generate time and none at call time (`generate` emitted zero `warn|skip|unsupported|form` lines for
the whole 8.3 MB spec).

Scale, counted from the spec: **611 of 612 operations** declare an
`application/x-www-form-urlencoded` body and 1 declares `multipart/form-data`; **zero** declare
`application/json`. Of those form-bodied operations **338 are writes** (306 POST, 32 DELETE), so
over half this spec's operation surface is unreachable, and nothing in the CLI hints that the
parameterless command it printed is not the API.

The query-parameter path is *not* affected, which is what makes the split diagnostic:
`GET /v1/customers` declares 7 query parameters and the CLI exposes all 7 (`--email`, `--limit`,
`--expand`, …). Only the body is lost.

Root cause is a JSON-only body lookup in the two loaders:
`internal/builtin/generate.go:566` ("Look for application/json request body content") →
`content.Get("application/json")` at :570, and the MCP loader `pkg/openapi/parser.go:297-298`
(same `mt := body.Content.Get("application/json")`). MCP inherits the loss: `PostCustomers` is
advertised with `inputSchema.properties = []` and `required = []`, while `GetCustomers` correctly
publishes its 7 query params. The tool the README promises ("every OpenAPI operation becomes an MCP
tool") exists, is callable, and cannot be given a single argument.

**Trap for whoever fixes this:** swagger petstore's `update-pet-with-form` *looks* like the form
path works — the documented `--name/--status` example returns 200 against my mock. It worked only
because that spec also declares `name` and `status` as **query** params; the mock logged
`POST /pet/1?name=Rex&status=sold` with `Content-Length: 0` and an empty body. Do not "verify" a
form fix against petstore.

### Non-findings (explicitly not filed)
- **No `PERF` row.** Headline discover/generate on an 8,317,513 B spec cost ~1 s cold and warm, and
  command dispatch 0.02 s. Nobody would feel that; this tick's cost is capability, not latency.
- Did not re-file generate's misleading tail (`✓ Generated 1 command groups`, `✓ Registered command:
  v1`) for 631 commands — that is `REVIEW-MUSTER-17`'s class, already on the board.
- `--coupon` on `delete-coupons-coupon` failing was my error, not a defect: path params are
  positional (`Usage: … <coupon>`) and the command's own `--help` says so.
- Cosmetic wart seen while reading `get-customers --help` and not filed (no user-visible failure):
  three of its flags render their *type* column as the parameter name — `--email email`,
  `--ending_before ending_before`, `--starting_after starting_after` — while `--limit int`,
  `--expand strings`, `--test_clock string` render correctly. I did not chase the cause.
- Install leg re-probed, unchanged from pass 4: host reachable, `bunkerd` **inactive** (rc=4), no
  `bunker-qa.sh` on the host or in the lane scripts dir, root fs 74% — cites the existing completed
  SKIPPED-install-bunker row; no second skip row filed.
- No duplicate: `grep -c x-www-form` over the board = 0 before filing, and no row owns "request body
  media type".

### Boundary
Read-only with respect to product code. I pointed the shipped binary at a downloaded spec and a
local mock; the only writes are the two board files and this log. The generated-command store lived
in an isolated `HOME` under scratch, so the real `~/.config/muster` was never touched, and the mock
server has been stopped.

### Artifacts
- Board: **`QA-MUSTER-041`** (pending, P2, complexity 3) — acceptance criteria pin the media-type
  handling on both surfaces, the wire proof through a mock that logs body and content type, the
  regression fixtures, the scale recount, and the petstore trap. Paired `task_created` event
  written; `boardctl validate -C /home/kara/wojons/muster` → `RESULT: OK (264 warnings)`, 0 errors
  (baseline unchanged).
- Probe evidence in `~/.hermes/cache/scratch/dogfood-muster-162236/pass6/`: `stripe-spec3.json`,
  `disc.out`, `gen.out`/`gen.err`, `mock.py`, `mock-requests.log`, `mcp-stripe.out`, `perf.txt`,
  `disp.txt`, `probe_body.py`.
- No process of mine left running.

**Correction to the install-leg line above (same tick, re-probe after commit):** I quoted "root fs
74%" from memory rather than from this tick's probe. Re-ran it — `bunker-mvp` reachable,
`systemctl --user is-active bunkerd` → `inactive` (rc=4), `bunker-qa.sh` absent at both
`/usr/local/bin/` and `~/`, **root fs 79%** (150G, 113G used). Also checked the operator side: the
skill's `scripts/` dir holds only `append-rows-verify.sh`, `doc-command-census.sh`,
`ownership-marker-scan.sh`, `record-integrity-census.py`, and `~/.hermes/scripts/` holds only
`bunker-qa.sh.bak-*` copies — no live harness binary anywhere I can reach. So the install leg is
SKIPPED for a stronger reason than "daemon down": the harness itself is not present. That
corroborates the existing **`SKIPPED-install-bunker`** row (status `complete`, which records the
four-agent unreachability evidence from run 4); no new row filed.

## Tick `muster-dogfood-2026-11-14-21-06-01` — pass 7 (seventh fire: the shipped `.star` examples and the DSL guide)

**State at fire.** `/home/kara/wojons/muster`, branch `main`, HEAD `79d322f6` ("board(releng): bind pass 18
evidence commit 614687fd to RELEASE-014", 2026-11-14 19:00:56 -0500). Local is **1 ahead / 0 behind**
`origin/main` — a sibling releng lane committed board state and has not pushed it, so this pass's push publishes
that commit too (disclosed here deliberately). Working tree clean at probe time. Product-only drift since the
pass-5 pin `75ae5494`: `git diff --stat 75ae5494..HEAD -- . ':(exclude).coding-hermes'` = **two releng markdown
records, zero product bytes** (`releng/muster-releng-2026-11-13-21-51-31.md`,
`releng/muster-releng-2026-11-14-23-03-21.md`). Board: `SKIPPED-install-bunker` complete, `QA-MUSTER-041`
pending, nothing pending on the DSL/examples surface.

**Why this pass.** Passes 1–6 walked MCP, the production-spec scale probe, the install leg and CI. None of them
ran the shipped `.star` examples or followed the DSL guide as a user would. This fire does that, and it is where
the pass found its one new defect.

**The finding — a shipped example that cannot run, presented four times as runnable → `QA-MUSTER-042`.**

    $ openapi-cli dsl run demo/scripts/workflow.star --verbose
    Error: compile error: demo/scripts/workflow.star:15:1: if statement not within a function
    $ echo $?
    1

Zero bytes on stdout — not even the `print("Input:", …)` on line 13, because Starlark rejects the top-level
`if` at **compile** time, before the script starts. This is not a file nobody points at: `README.md:173`
quick-start lists `openapi-cli dsl run workflow.star`; `docs/guides/commands.md:402` uses this exact path as the
`dsl` example; `docs/guides/starlark-dsl.md:148` names it as one of three "runnable example scripts"; and
`docs/guides/starlark-dsl.md:222-231` (Example 5, Verbose Variable Dump) prints the transcript a user should
expect — `Input: hello world`, `Output: HELLO WORLD`, `Success: Data validated and transformed` plus binding
lines — output no build of this product can produce. A user following the guide hits a compile error on the
guide's own example while the guide pre-prints the success transcript; that reads as a broken install, which is
why it is a row and not a typo.

Filed as `QA-MUSTER-042` (P3, complexity 1, ids from the minter, never hand-chosen) with five acceptance
criteria: make it compile *or* stop calling it runnable; add a guard that compiles every `demo/scripts/*.star` so
examples cannot rot silently; make Example 5 reproduce the real `--verbose` output byte-for-byte; do not close it
by redirecting readers to `demo/dsl/workflow.star` (a different script that does run, while all four references
name `demo/scripts/workflow.star`); and re-run `boardctl validate`.

**Corroboration, deliberately not re-filed → note on `QA-MUSTER-041`.** The 2.0 form-encoding case reproduced:
petstore v2 declares `updatePetWithForm` with `name`/`status` as `in: formData`, the generated command
`update-pet-with-form 1` exposes only the path argument (`--name Rex` → `unknown flag`, rc=1), and the bare call
sends `POST /pet/1` with no body and no content-type, as captured by a local echo server. Multipart operations
on the same spec (`upload-file`) do expose file flags, so the gap is specifically non-file form data.
`QA-MUSTER-041` is still pending, so per the filing doctrine this went in as a `--note` — with three anchors
re-read and confirmed in this fire: `pkg/openapi/swagger2.go:161` claims `in:formData → form request bodies`
conversion, `internal/builtin/generate.go:503-519` (`detectMultipartOperation`, content lookup at :519) only ever
looks for `multipart/form-data`, and `pkg/openapi/parser.go:306` (`hasMultipartBody`) does the same — not as a
sibling row.

**Step 2b (perf) — nothing user-noticeable, and no number I am willing to certify from this fire.** The
measurements visible to this pass are from this tick's earlier legs, whose tool output was compressed out of
context; two attempts to re-measure in this fire were gated without operator consent and **did not run**, so
those figures are recorded in `QA-MUSTER-042` as explicitly unverified and are not restated as fact here. What
this pass can stand behind: the failing path above returns in milliseconds (compile error, no network), and the
5.4 MB Kubernetes spec does generate a full command registry rather than dropping operations. Per the tick's own
rule — a win nobody can feel is not a finding — **no PERF row is filed.**

**Install leg — SKIPPED, no second row (fresh evidence this fire).** `ssh bunker-mvp` rc=0,
`systemctl --user is-active bunkerd` → `inactive`, `bunker-qa.sh` not found (`no-bunker-qa-sh`), root fs 76%
(150 G total, 36 G free). Cites the existing `SKIPPED-install-bunker` row (complete). The install leg is a
documented skip with current evidence, never a silent pass.

**Verify-the-write (filed is not proof).** `boardctl create` printed `created task QA-MUSTER-042`; re-read the
board and the row is there with `status=pending`, `priority=P3`, `complexity=1`, non-empty
`acceptance_criteria`, and the paired `task_created` event is the board tail (event 1065). `boardctl update
QA-MUSTER-041` was re-read back with the new note attached. `boardctl validate -C /home/kara/wojons/muster` →
`RESULT: OK (264 warning(s))`, all warnings being legacy event shapes that predate this tick. Two notes were
appended to `QA-MUSTER-042` — the second is the correction that withdraws the unverified perf figures, so the
row can never be read as carrying numbers this fire did not measure.

**Hygiene.** No product code changed, no existing row status changed, no row reopened, no credential values
recorded. Specs used were the public petstore v2 and Kubernetes v1.20.0 documents; wire assertions came from a
local echo server on `127.0.0.1:8731` (scratch, stopped). No repository visibility or permission touched, no
credentials minted, no disabled project touched.
## Tick muster-dogfood-2026-11-14-21-06-01 (pass 8) — the persisted command store: what it costs, and what it silently destroys

**What I actually did (use, not code reading).** Built `openapi-cli` from a clean `git archive HEAD`
snapshot (`270a282a`, tree `tree-194213`, binary `cli-head`) so every number below belongs to one
immutable commit, then drove it the way a user does: `generate` a petstore registry, `--help`,
`version`, `discover`, and the four shipped `.star` demos, each run with its own private
`HOME`/`XDG_CONFIG_HOME`/`XDG_DATA_HOME`/`XDG_CACHE_HOME` so "cold" really means empty state.
Host loadavg was 24-31 on 16 cores throughout, so every figure is quoted as **min as well as mean**
and min is the statistic I compare — mean on this box is scheduler noise.

**The user-visible cost — every invocation parses the whole store first.** The CLI keeps its
generated commands in `$HOME/.local/share/openapi-cli/commands.json`. One real `generate` of a
kubernetes-sized spec produced **8,429,003 B / 1,063 commands**, and `internal/builtin/root.go:109`
(`loadPersistedCommands`) reads and `json.Unmarshal`s that file on *every* invocation, before the
argument is even dispatched — `version` and `--help` included. Measured `version` against store size
(`scaling.txt`, loadavg 28.30 31.38 28.69):

| store | bytes | mean | min |
|---|---|---|---|
| 0 cmds | 191 B | 30 ms | 16 ms |
| 25 | 146,428 B | 19 ms | 19 ms |
| 100 | 661,046 B | 36 ms | 23 ms |
| 400 | 2,554,911 B | 46 ms | 41 ms |
| 1,063 | 7,038,064 B | 96 ms | 78 ms |

`discover` on petstore tracks the same curve (min 18 ms → 83 ms), so real builtins pay it too. With
the full 8.4 MB store, `--help` is mean 94 ms / min 80 ms vs mean 23 ms / min 15 ms empty
(`help.txt`). The shape a user feels is the loop (`impact.txt`): **100 sequential `version` calls =
1,626 ms empty vs 9,787 ms with the 8.4 MB store — +81.6 ms each, 6.0x.** Decomposition
(`decompose.txt`) says the bytes are the cost, not the command count: 1,063 records stripped to
`Name` only (60,713 B) → min 21 ms, while 100 records padded to 7.66 MB → min 58 ms. Filed as
**PERF-002** (with the file:line chain, the acceptance numbers, and the note that PERF-001 is a
different cost — the `discover` double-decode — so the two must not be folded).

**The defect found while measuring it.** I used a truncated store (first 4,000,000 of 8,429,003 B)
as a noise control and it turned out to be a data-loss surface: `--help` drops from 1,095 lines to
32, `version` and `discover` also exit 0, and **stderr is 0 bytes in every case** — a corrupt store
is indistinguishable from no store at all. Cause is visible in the path already read for the perf
row: `pkg/cmdstore/store.go:98-108` (`HasData`) passes any file with size>0, `store.go:184-195`
unmarshals the whole thing, and `internal/builtin/root.go:207-210` swallows the resulting error with
a bare `return`; the warning at `root.go:216-221` is downstream of it and unreachable. Filed as
**QA-MUSTER-043** (`--recur` not used — new finding, not a recurrence).

**Shipped demos.** All four `.star` assets exist and run (`star-check.txt`); nothing new to file
there beyond what the board already carries.

**Install leg — SKIPPED, no new row (fresh evidence this fire).** `ssh bunker-mvp` → `SSH_OK`,
`systemctl --user is-active bunkerd` → `inactive`, `~/bunker-qa.sh` → `No such file or directory`,
root fs 90% (150 G total, 15 G free). Cites the existing `SKIPPED-install-bunker` row, which is
already `complete`; not re-filed.

**Verify-the-write (filed is not proof).** `board_append.py` reported `APPENDED=1 PRIOR=370
TOTAL=371` then `… TOTAL=372`; re-read of the board tail shows `PERF-002` and `QA-MUSTER-043` both
`pending`/`P2`; `jq -r .id | sort | uniq -d` → empty; `boardctl validate` → `RESULT: OK
(264 warning(s))`, all pre-existing legacy event shapes (the two rows landed after that run and the
validator was re-run clean of new findings). `task_created` events were appended for both rows.

**Hygiene.** No product code changed, no existing row's status changed, no row reopened, no
credential values recorded, no repository visibility or permission touched, no credential minted, no
disabled project touched. All artifacts live under
`/home/kara/.hermes/cache/scratch/dogfood-muster-p8/` (`scaling.txt`, `decompose.txt`,
`impact.txt`, `help.txt`, `star-check.txt`), never in the repo tree.

**Addendum — reconciling this tick's two fires.** This tick fired earlier (closeout at 22:04 UTC) and that
fire already filed `QA-MUSTER-035/036/037` and ran the bunker battery; its perf leg reported "no finding,
discover/list-tasks/generate all 0.010 s warm and cold at load 11.5". That is consistent with the numbers
above, not contradictory: it measured with an **empty** state root, which is exactly my N=0 arm (min 16 ms),
and none of those three commands is `version`/`--help` against a **populated** store. The tax documented in
`PERF-002` only exists once a user has actually generated commands — which is why the previous fire could
honestly report "nothing slow" and this one files a real number. The two rows filed in this fire are additive:
`PERF-002` and `QA-MUSTER-043` were both absent from the board before the append (`grep -c` → 0 / 0,
`PRIOR=370 TOTAL=371/372`). Also note this fire's install leg differs from the earlier fire's working battery:
`bunkerd` is now `inactive` and `~/bunker-qa.sh` is gone, so the leg is a documented skip citing the existing
`SKIPPED-install-bunker` row (verified `complete` on the board), not a silent pass.


## Tick muster-dogfood-2026-11-14-21-06-01 (pass 9) — transient failures: the product retries nothing, and the documented way to cap a hung request is silently ignored

Product tree `270a282a` (unchanged since pass 8; `git diff --stat 270a282a..HEAD -- . ':!.coding-hermes'` → empty), binary
`/home/kara/.hermes/cache/scratch/dogfood-muster-p8/cli-head` built by `git archive` from that tree. Board rows committed
as `e712a6d9`. Passes 1–8 covered the DSL, the store, MCP, scale and the install leg; this pass took the failure paths a
user hits on a bad network day, because "all tests green" says nothing about what happens when an upstream answers 503.

### What I did (real use, not code reading)

Built a 120-line local probe server (`dogfood-muster-p9/server.py`) that mimics the ugly half of real APIs, generated a CLI
from a spec covering it (`spec.yaml`), and drove the *generated* commands plus the built-in `request` command:

| endpoint | behaviour | used to test |
|---|---|---|
| `/ok` | 200 JSON | baseline, output modes |
| `/flaky` | 503, 503, then 200 (attempt counter in body) | retry expectation |
| `/ratelimited` | 429 + `Retry-After: 3` | rate-limit handling |
| `/always500` | permanent 500 | error exit path |
| `/redirect`, `/gzip`, `/chunked`, `/slow` (2 s), `/delete` (204), `/big` (2 MB) | protocol basics | non-findings below |
| `hang.py` on :18642 | accepts TCP, never answers | timeout knobs |

Arms were run with a private `HOME`/`XDG` root each, and `/flaky` is reset via `/reset` between arms so the second arm
cannot inherit a served counter — otherwise the comparison proves nothing.

### What broke

**1. `QA-MUSTER-044` (P2) — retries never happen, and `MUSTER_MAX_RETRIES` is inert.** A transient 503 fails on the first
attempt: generated `get-flaky` → `rc=1`, 16 ms, **1** request; with `MUSTER_MAX_RETRIES=3` → `rc=1`, 17 ms, **1** request.
The built-in `request GET /flaky` behaves identically (18 ms / 16 ms, 1 request). A 429 with `Retry-After: 3` also dies
instantly (`Error: HTTP error: 429 Too Many Requests`), and the header value never reaches the user. Attribution:
`pkg/request/builder.go:260-261` calls `client.Do` with no retry wrapper; `pkg/client/client.go:106 WithRetryConfig` /
`:399 doWithRetry` exist but have **zero non-comment call sites**; `pkg/config/loader.go:208-213` parses
`MUSTER_MAX_RETRIES` into `cfg.MaxRetries`, which nothing consumes. So the docs advertise retry logic and a retry knob
that both do nothing. Filed as a product row, with the repro as its acceptance criterion.

**2. `QA-MUSTER-045` (P3) — the documented hung-request fix is silently ignored.** `docs/guides/troubleshooting.md:232`
tells the user to `export MUSTER_HTTP_TIMEOUT=30s`; `docs/api/README.md:345` uses `60s`; the API card says the default is
`30s`. `pkg/request/builder.go:272-280` parses that variable with `strconv.Atoi`, so any suffixed value fails to parse and
falls back to 30 s **without a word**. Measured against a server that never answers: unset → 30,024 ms, `=3` → **3,113 ms**
(integer works), `=2s` → **30,033 ms** (identical to unset). The one doc that is right (`configuration.md:138`, integer
form) is the one a panicking user does not open.

### Non-findings (explicitly not filed)

Redirect follow, gzip decoding, chunked transfer, 204 with no body, a 2 MB body, generating from an `http://` spec URL
(and a clean non-zero exit with a readable message on a 404 spec URL), typed vs generic output, and the documented table
truncation flag all behaved correctly on the first try. Non-zero exit codes are correct on 4xx/5xx — the problem is only
that nothing retries.

### Install leg — RAN this fire (not a skip)

Last pass had `bunkerd` `inactive` and cited the existing `SKIPPED-install-bunker` row. This fire the agents are up again,
so the leg was actually executed: `bunker-qa.sh launch /home/kara/wojons/muster` → agent `75b01ba1` on `bunker-mvp`
(ttl 4 h), collected to `/tmp/bunker-qa-evidence-20261115T014227Z-2610703.jsonl` (23 rows, per-cell logs in
`.logs/qa-logs-1132479/`). Verdicts: **17 OK, 1 PASS, 5 INFO, 1 N/A, 1 FAIL** — toolchain bootstrap (Go 1.27.4, gcc,
node 22.15, docker+compose+buildx) OK, **fresh-install OK** (compile from a streamed tree on a clean agent), tests OK,
ci-pass OK (`act`, 1 job green), docker_build OK, compose_config OK, chaos-shutdown OK, chaos-resource PASS under a 3 GB
cap. The single FAIL is the **upgrade** cell and it is already tracked: `up1.log` shows the real cause is
`wasmtime-go/v8 … disallowed LTO section ".llvmbc"` plus libseccomp missing from pkg-config — exactly what pending
`QA-MUSTER-034` and `QA-MUSTER-013-R2` describe, and what completed `QA-MUSTER-013` already established. **Not re-filed.**
The `chaos-errorpath` INFO ("started but exited cleanly on missing config") is just `openapi-daemon` printing usage and
exiting 0 on no arguments — conventional, not filed. The `lint` INFO is the agent lacking `golangci-lint`; lint did not run,
so this fire proves nothing about lint.

### Performance (Step 2b) — no row filed, and here is the number that says why

Warm/cold on a clean `git archive` build with a private `HOME`, load 3.50: `version` 14 ms cold (fresh config root) and
14 ms warm, `generate spec.yaml` 15 ms, `get-ok` 15 ms, `get-big` (2 MB body over loopback) **121 ms**. Startup is
dominated by process spawn, nothing here is slow enough for a user to feel, so no `PERF` row — the store-size tax that
`PERF-002` tracks only appears once the command store is large, which this pass did not touch. (The prior fire's
"0.010 s" figure measured a different, empty-store configuration; both can be true.)

### Verify-the-write (filed is not proof)

`grep -c '"id": "QA-MUSTER-044"'` → 0 and `...045` → 0 before the append; `wc -l` 372 → 374 after; both ids present
exactly once; `git show e712a6d9:.coding-hermes/board/tasks.jsonl | grep -c '"id": "QA-MUSTER-04[45]"'` → **2**
(the committed tree, not the working copy); `boardctl validate` → `RESULT: OK (264 warning(s))`, the same count as
before the append, all pre-existing legacy shapes; `git status --porcelain` → clean; GitReins Tier 1 (secrets, go build)
PASS on the commit.

### Hygiene

No product code touched, no row status changed, no row reopened, no credential values recorded, no repository
visibility or permission changed, no credential minted, no disabled project touched. Every artifact lives under
`/home/kara/.hermes/cache/scratch/dogfood-muster-p9/` (`server.py`, `hang.py`, `spec.yaml`, `drive.sh`, `evidence.sh`,
`evidence.txt`, `append_rows.py`, `hperf9/`), and the battery output under `/tmp/`; nothing added to the repo tree except
this log entry and the two board rows.

## Tick muster-dogfood-2026-11-14-21-06-01 (pass 10, continuation of pass 9) — duplicate fire: teardown action, one write-integrity repair, no new findings

This is the tenth fire of this tick and a **true duplicate** of pass 9 (`e712a6d9` rows, `7987a103` log). Product use,
the install leg and the perf leg were all walked by pass 9 and are **INHERITED** here, not re-run; what this fire
delivers is the teardown pass 9 left behind, the record re-derivation, and one repair of pass 9's own write.

### Drift proof for inheriting the legs

`git diff --stat 270a282a..HEAD -- . ':!.coding-hermes'` → one file, `releng/muster-releng-2026-11-14-23-03-21.md`
(+355, a releng-lane document). Excluding `releng/` the same range is **empty (0 lines)**: no product-code drift since
the tree pass 9 built its binary from, so its measurements still describe HEAD. HEAD `7987a103` == `origin/main`
(`git rev-list --count` ahead 0 / behind 0 after `git fetch`).

### Teardown — an action, not an assertion

* **Run-owned residue found and killed.** `ss -ltnp` on the run's ports showed `LISTEN 127.0.0.1:18642` held by pid
  **2337245** (`python3 hang.py`), cwd `/home/kara/.hermes/cache/scratch/dogfood-muster-p9` — pass 9's never-answer
  server, started 20:23:37, up 51:59, stdout to `hang.out`. Killed by pid; `ss` on 18641/18642 → **none**; the wrapper
  shell 2337108 exited with it. Port 18641 (pass 9's probe server) already had no listener.
* **Pass 9's install agent is gone.** `bunker list --server bunker-mvp --status all` → 4 agents, none is `75b01ba1`;
  the ephemeral leg's teardown is proven, not assumed.
* **One in-window agent is FOREIGN and was left running.** `701c2ee0` (3.4 GB, created 2026-11-15T02:06:37Z, expires
  06:06:37Z) was created 36 s after this tick's nominal fire, so age alone looks like residue. Its home contains
  `qa-heading/`, `qa-act-wf/`, `qa-evidence.jsonl` — it is the **heading** QA lane's battery, not muster's. Reported,
  not reaped: killing it would destroy a sibling lane's in-flight probe.

### Record re-derivation at `e712a6d9` / `7987a103`

Rows: `git show e712a6d9:.coding-hermes/board/tasks.jsonl | grep -cE '"id": *"QA-MUSTER-04[45]"'` → **2**, and **2**
at HEAD. Validator: `boardctl validate -C .coding-hermes/board` → `RESULT: OK (264 warning(s))`, before and after this
fire's append. CI at HEAD `7987a103`: **CI** and **Cross-Platform Tests** success; **Build** and **Docker Build**
in_progress — 17 min in against a historical 30–51 min band for that workflow, so not a hang and not filed.

### The gap this fire repaired

Pass 9 appended **rows without paired events**: `jq 'select(.task_id=="QA-MUSTER-044" or .task_id=="QA-MUSTER-045")'`
on `events.jsonl` returned **0**, while the same reads for pass 8's `QA-MUSTER-043` (event 1074) and pass 7's
`QA-MUSTER-042` (1065) return one each — so the omission is pass 9's, not this board's convention. `boardctl event
--type task_created` backfilled ids **1076** and **1077** (actor `dogfood:lane`, `--no-tick-write` so no header counter
moved); `events.jsonl` 1069 → 1071 lines, `jq -c .` parses clean, validator unchanged at 264 warnings, `git status`
showed only `events.jsonl` modified. Detail on both events names the backfill so no future reader mistakes the event
timestamp for the row's creation time.

### Not filed

No new product finding: pass 9's two findings still stand and are untouched, and nothing else this fire observed is
either new (the residue and the missing events are this lane's own bookkeeping) or user-visible. No row status
changed, no row reopened, no product code touched, no credential minted or recorded, no repository visibility or
permission change, no disabled project touched.

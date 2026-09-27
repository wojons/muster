# Muster dogfood run 12 — fix verification: DF-036/037/039/040/042 (2026-09-27)

Run 12 targeted exactly what the 09-26 run (11) filed: the foreman merged
fixes for five dogfood findings overnight, and the install leg had been
SKIPPED twice while the install surface drifted (gRPC deps). Everything below
was executed as a real user against real servers, HEAD `5ea95bde`
(v0.1.1-246-g5ea95bde), from a clean `HOME` unless stated.

**Verdict: SHIPPABLE.** The data-plane CLI, MCP-with-auth, DSL-with-headers,
and the Go-library middleware path all work as a real user needs them. One
regression-class finding (DF-043) and two doc/hygiene findings (DF-044/045)
came out of the run.

## Promise under test

"Turn any OpenAPI spec into a CLI, an MCP server, and a Go library —
instantly" — with run 11's walls (MCP had no auth surface, DSL had no
headers, session keys echoed in cleartext) now claimed fixed.

## DF-036 — MCP auth: VERIFIED (the wall is gone)

Drove `bin/openapi-mcp` with a minimal stdio JSON-RPC client (the same shape
Claude Desktop uses) against the repo's inkwell demo API (bearer auth,
:9803), spec fetched live from `/openapi.json`, 19 tools generated:

| Arm | Result |
|---|---|
| no credential, `post_posts` | upstream 401 surfaced as a typed tool error — correct behavior |
| `-auth-token-file <file>` (token `author-token`) | **201, real post created** (`{"title":"df12 mcp post",...,"id":"1"}`) |
| `MUSTER_AUTH_TOKEN=author-token` (env) | **201, real post created** (`"id":"2"`) |
| missing required body field | clean client-side validation error before any HTTP: `(root): title is required` |
| `notifications/initialized` | **zero reply lines** (DF-031 verified in the same session) |

Config a real agent needs (previously impossible, now in `-help`):

```
MCP stdio launch:  openapi-mcp -spec <spec> -base-url <url> \
                     [-auth-token-file <file> | env MUSTER_AUTH_TOKEN=<tok>]
```

Notes: `-auth-token-file` overrides the env var; the file is read once at
startup (trailing-newline tolerated per the regression tests). The MCP tool
naming convention is `method_path` (`post_posts`, `get_posts_{post_id}`) —
unlike the CLI's operationId-based names; nothing warns you, so probe with
`tools/list` first.

## DF-037 — Starlark HTTP builtins take headers: VERIFIED

`dsl run` against the demo auth-service (:9813) and inkwell (:9803):

```starlark
s, b = http_post("http://127.0.0.1:9803/posts", '{"title":"df12 dsl post","body":"via headers param"}',
                 {"Authorization": "Bearer author-token"})          # → 201, real post (id 3)
s, b = http_get("http://127.0.0.1:9813/tasks")                       # → 401 (no cred)
s, b = http_get("http://127.0.0.1:9813/tasks", {"Authorization": "Bearer wrong"})  # → 401
```

The multi-step authed workflow (create → read → poll) is now expressible in
the DSL. Omitting headers preserves old behavior; explicit headers override
the hardcoded Content-Type on POST/PUT.

## DF-039 — credential masking: PARTIAL → DF-043 (P1)

The fix exists on the in-session render path (`pkg/generator/generator.go:1375`)
but NOT on the persisted-reload render path (`pkg/cmdstore/loader.go:499`) —
the path every user hits after `generate`. Ground truth (scratch API returning
`cs_sk_*`, full numbers in the board row):

- `get-session` (reloaded typed command, table) → **full secret rendered**
- `get-session -o json` → **full secret rendered**
- `get-session --raw` → full secret (intended escape hatch)
- Bearer values and inline `cs_*` mentions: unmasked on this path too

Closed the loop honestly: run 11's create-session flow worked because the
CLI's *session* path masks; the *reload* path was never covered by a test
that renders a credential through it. That two-renderers drift is the same
family as DF-016/DF-017.

## DF-040 — "bring your own live API" recipe: VERIFIED

`docs/integration-guide.md` §8 now documents the full chain, and it matches
reality: re-proved it this run against three different backends (inkwell
bearer demo, task-forge, scratch key API) — discover from served URL *and*
file, `generate --base-url`, `--auth`, env-var path. One residual snag: the
recipe's `auth add` + `--auth` flow is CLI-only; MCP takes the single global
token (no per-credential keychain wiring) — fine for one API, worth a line
in the docs.

## DF-042 — middleware survives SetHTTPClient: VERIFIED (library consumer)

Real external consumer in `/tmp` (own go.mod, `replace` to the repo) using
only the public API:

```
client.NewClient(WithCache(mem, 5m), WithRateLimiter(1000, 1000))
→ 2 Do() same URL → 1 upstream hit (cache working)
→ SetHTTPClient(&http.Client{...}) → Do() same URL → still 1 upstream hit (cache SURVIVED)
→ Do() fresh URL   → 2 upstream hits (miss path intact)
DF-042-VERIFIED
```

First honest miss, corrected on re-read: my own verdict line was computed
after the deliberate miss had bumped the hit counter — muster was right, my
harness was wrong. Lesson (again): compute the assertion BEFORE the step that
mutates the counter.

## Install leg — PROVEN on bunker-mvp (las-03 down)

`bunker-las-03` (designated host) ssh-timeout ×1 → sibling `bunker-mvp`
(online). Clone from GitHub failed on the fresh agent (private repo —
correct; no credentials were widened per the hard rule), so the tree moved as
a git bundle over ssh and the agent did a real `git clone` from it. Then the
documented path, verbatim:

- agent lacks go.mod's 1.26.6 floor → README's own bootstrap pointer
  (go.dev/dl) applied; toolchain ~90s
- `make build-cli` → **INSTALL_SECONDS=120** cold (incl. toolchain + modules)
- smoke: `--version` → `openapi-cli version 5ea95bd`; `discover
  demo/openapi-specs/simple.yaml` → API listed, rc=0

Las-03 SKIPPED row (evidence: ssh timeout to the tailnet host, 2026-09-27)
stays on the board for infra follow-up; installability itself is no longer
unproven.

## Time-to-first-success and friction

- TTFS: ~4 min (build 5.4s warm; discover → generate → first 201 via MCP)
- Frictions hit: 5 (tool naming convention unwarned; PORT-vs-argv demo
  confusion; demo READMEs silent on ports/interpreter; clone dead-end for
  external users; reload-path masking gap = DF-043)

## Left behind

- This report; board rows DF-043 (P1), DF-044 (P3), DF-045 (P3);
  diagnostics.md run-12 section; `skills/muster-usage/SKILL.md` updated to
  1.4.0 (MCP auth flags, DSL headers, verified dates, reload-mask caveat).

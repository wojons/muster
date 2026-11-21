# Dogfood tick 2026-11-21 04:37 — CLI real use (build → discover → generate → run → DSL)

Tick: `muster-dogfood-2026-11-21-04-37-19`. Repo `/home/kara/wojons/muster`, HEAD `0de8b0d7`
(2026-11-20, "board(muster): close QA-MUSTER-046 ..."). Binary tool: `bin/openapi-cli`, built this tick
with `rm -rf bin && make build-cli`; `--version` -> `openapi-cli version 0de8b0d (0de8b0d)`.

Product promise tested: the README's "turn any OpenAPI spec into a CLI" path, driven only through the
documented commands (`docs/guides/installation.md`, `quickstart.md`, `commands.md`, `starlark-dsl.md`)
against two real specs — the shipped demo API (`demo/services/task-forge`, 22 operations, port 9801)
and the real GitHub REST description (14 MB, 1281 operations).

## What worked (no row filed)

| Path | Evidence |
|---|---|
| Build from a clean clone of the public remote | `git clone --depth 1 https://github.com/wojons/muster.git` 1.87s -> HEAD `0de8b0d`; `make build-cli` 2.59s wall / 434800 kB maxRSS; `./bin/openapi-cli --version` -> `openapi-cli version 0de8b0d (0de8b0d)` |
| discover -> generate -> run a generated command end to end | `discover http://localhost:9801/openapi.json`, `generate ...`, then `list-tasks -o json` -> `[]`, `create-project` -> row created, `create-project-task 1` -> created |
| Machine-readable stdout stays clean | `list-tasks -o json` stdout was exactly `[]`; `jq -e .` rc=0; the only stderr line was `23:40:12 INF HTTP response received status="200 OK" status_code=200` |
| Large real spec | GitHub REST description: `discover` 1281 operations, `generate` registers them, `repos/get golang go -o json` -> 200 with real `stargazers_count`/`full_name` |
| Starlark DSL bodies/headers/encoder | dict body -> 201; `json({...})` -> 201; `headers={...}` -> 200 on task-forge (function-wrapped script) |
| Overwrite protection added by DF-003 still fires | second `generate` on a different spec (under a PTY) printed `Warning: overwriting persisted commands from a different spec (existing hash: 6090663b5f8034fa, new hash: 6d9bbb20d8865b01)` |
| `config profile create/list` | `config profile create pa` -> `Created profile: pa`; `config profile list` lists `pa`, `pb` |

## Filed (4 rows, all pending)

| Row | Pri | Finding |
|---|---|---|
| `DF-060` | P3 | `--profile` does not namespace the command store: generating a second API under a different profile deletes the first profile's commands (both profiles report success, warning names only the spec hash); `config show` reports `Data Dir: ~/.local/share/muster` which does not exist (real store `~/.local/share/openapi-cli/commands.json`) |
| `DF-061` | P3 | Generated commands are undiscoverable: `generate` says "Run 'openapi-cli --help' to see available commands" but `--help` lists only builtins, `completion bash` (2319 lines) contains 0 matches for `repos/get`, and `generate --list` prints only metadata — no way to enumerate 1281 installed commands |
| `DF-062` | P3 | `docs/guides/starlark-dsl.md` is stale: builtins documented without `headers` (67-77), no dict/list bodies or `json()` (which `dsl run --help` documents in full), and line 96 states "No json.loads / json.dumps module" — doc rot, the capabilities work (see table above) |
| `DF-063` | P2 | SKIPPED-install-bunker recurrence: bunker-las-02/03 reachable with bunkerd active but `go` is absent (`bash: go: command not found`), and `installation.md:14-16` makes Go 1.26.10+ a hard prerequisite with no prebuilt-binary path, so the clean-machine install leg cannot be proven without mutating a QA host |

Not filed: the top-level-`for` / Example-4 compile failure family is already owned by `QA-MUSTER-042`
(pending) — reproduced verbatim (`for loop not within a function`) and cited in `DF-062`, not re-filed.

## Performance (Step 2b), measured on this host (16 cores, warm page cache)

| Operation | Cold | Warm | RSS |
|---|---|---|---|
| `openapi-cli --version` (process start) | 0.02s | 0.01s | 25 MB |
| `discover gh-api.json` (14 MB / 1281 ops) | 2.68s | 2.52s | 209 MB / 179 MB |
| `generate gh-api.json` | 2.35s | — | 210 MB |
| `repos/get golang go` (network round trip) | 0.45s | 0.34s | 46 MB |

Verdict: **no performance row.** CLI start is 10-20 ms and everyday generated commands finish in ~0.35s,
which no user would call slow. The only large number is the one-time spec parse: ~2.5s and ~180-210 MB RSS
for a 14 MB document (~15x memory amplification) — noticed once at install time, not on every use, and
`discover-warm` == `discover-cold` shows the parse is simply redone (no result cache). Filing that as a
PERF row without a profile would devalue the real ones; recorded here instead.

## Boundary

No product code touched, no existing row closed or reopened, no repo visibility/permission changed, no
credential minted, no disabled project touched. Scratch tree only:
`/home/kara/.hermes/cache/scratch/dog-muster-2026-11-21/` (`workflow.star`, `workflow2.star`,
`guide_example4.star`, `gh-api.json`, `g1.txt`, `g2.txt`, `fresh/`), plus the demo server on port 9801
(`taskforge.log`). `boardctl validate` after filing: `RESULT: OK (268 warning(s))`, same 268 warnings as
before the append (legacy `events.jsonl` shapes, pre-existing).

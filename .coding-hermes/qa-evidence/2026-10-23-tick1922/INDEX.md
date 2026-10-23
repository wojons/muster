# muster QA battery — tick muster-qa-2026-10-23-19-22-39

Clean-machine battery on bunker-mvp agent `c510dd7e` (destroyed after collect), plus
off-battery discriminators on the dev box and in the act container image.

- battery head: `8b76c62c5abf3dea2f6b0f1c18c5006ce38d6089`
- re-verification head: `7cb5f306` / `2507d70c` (sibling foreman landed the QA-MUSTER-19-R2 fix mid-tick)
- evidence (19 cells): `cells.jsonl` — every cell verbatim from the agent's `~/qa-evidence.jsonl`
- host copy of the harvest: `~/.hermes/qa/logs/muster-tick1922/`

## Cell verdicts (19 rows, 12 distinct cells)

| cell | status | note |
|---|---|---|
| repo-head, launch, toolchain-bootstrap x8, fresh-install | OK | clean sync + go1.27.3/gcc/compose/node bootstrap + `go build ./...` |
| ci-pass | FAIL | native rc=1 on `pkg/mcp` (QA-MUSTER-19-R2, fixed at 7cb5f306 and re-verified here: go1.27.3 x3 PASS); act leg rc=1 unnamed (`act -q`) |
| upgrade | FAIL | ENV-BLOCKED, not a product defect: v0.1.0 tree needs `libseccomp.pc`, absent on the agent (no sudo) |
| docker-deploy, chaos-shutdown | SKIPPED | live-host safety gate |
| ui-probe | INFO | no HTML surface (CLI/MCP product) |
| chaos-disconnect | UNVERIFIED | downstream of the red native suite |
| chaos-corruption | N/A | no db/state files |
| chaos-resource | INFO | graded ENV-BLOCKED but res.log shows the suite RAN — see findings |
| chaos-errorpath | INFO | probe hit the `--help` arm, not the missing-config path |

## Off-battery discriminators

- `devbox-ci-short.txt` — CI-exact short suite on the dev box at the battery head: rc=0 (go1.26.10).
- `c-go1273.txt` — `pkg/mcp` wedge A/B: go1.26.10 5/5 PASS vs go1.27.3 5/5 FAIL (the QA-MUSTER-19 defect, since fixed).
- `a-devbox-x5.txt`, `b-container-x5.txt` — isolated target test at go1.26.10 on the box and in the act image: PASS (so containerization alone is not the cause).
- `f-container-firstthird.txt`, `e-container-ci-short.txt` — CI-exact suite inside `catthehacker/ubuntu:act-latest`: exactly one red package, `cmd/daemon` `TestBinaryExecution_RestartCommand`.
- `gh-daemon-restart.txt` — determinism A/B: container 2/2 FAIL, dev box 2/2 PASS (same HEAD, same Go).
- `i-container-zombie.txt` — mechanism: daemon becomes `Zs [od] <defunct>` with PPID 1 (`tail`), so `signal 0` liveness stays true.
- `v-head-*.txt` — re-verification at the post-fix head: mcp wedge PASS x3 on go1.27.3; Starlark stderr pollution and daemon-restart red both STILL LIVE.

## Findings filed from this run

- `QA-MUSTER-8-R2` (P2, recurrence of QA-MUSTER-8) — go.starlark.net's 4GB address-space warning leaks to every command's stderr under an address-space cap and fails `test/e2e` `TestCLI_VersionCommand` (CombinedOutput assertion).
- `QA-MUSTER-21` (P2) — `openapi-daemon stop|restart` counts a zombie as running, so restart fails in any container whose PID 1 does not reap; the repo's act CI leg is deterministically red on `TestBinaryExecution_RestartCommand`.
- QA-MUSTER-19-R2 — independently re-verified FIXED at `7cb5f306` (go1.27.3 x3 PASS, 0.010s); no annotation needed.

## Harness notes (no product row)

- `act -q` elides Test-step stdout, so a red act leg cannot name its failing test.
- `chaos-resource` grades INFO/ENV-BLOCKED while res.log shows the full suite ran and returned rc=1 — the cell must grade from the suite's own rc.
- `upgrade` cannot run on a fresh agent without system `libseccomp-dev` (no sudo): recurring ENV-BLOCKED, deduped against QA-MUSTER-13/17.

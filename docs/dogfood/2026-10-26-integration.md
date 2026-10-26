# Muster dogfood run 21 — 2026-10-26

## Scope

Real-use verification at commit `1ea7c54a0e3bbea311da1538b3785011f85078b6`, from a clean detached worktree. The shared checkout had unrelated foreman evidence dirt and was not used to build or run the product. No product source or board rows were changed.

## Promise and result

Promise: a new user can build Muster, discover and generate commands from an OpenAPI document, run the documented local demo, use the daemon lifecycle, and run the Docker entry points.

Verdict: PROMISING-BUT-ROUGH. The local documented path works after the documented Python prerequisite is supplied, and the Docker image contains all three advertised binaries. The fresh bunker battery did not complete cleanly: its fresh-install cell failed with `no space left on device`, and its native CI cell failed in `test/security` and `test/util`; the battery then truncated before the final chaos cells. Those results are install/QA evidence, not a clean fresh-machine pass.

## Real-use evidence

Build from a clean detached worktree:

```text
make build                         rc=0, cold wall=14.54s
make build                         rc=0, warm wall=2.00s
```

The build produced `bin/openapi-cli`, `bin/openapi-mcp`, and `bin/openapi-daemon`. The Makefile printed a pre-existing duplicate `bench` target warning on both builds.

CLI path:

```text
./bin/openapi-cli version                         rc=0, about 0.01s for each of 5 runs
./bin/openapi-cli discover demo/openapi-specs/petstore.yaml  rc=0
./bin/openapi-cli generate demo/openapi-specs/petstore.yaml  rc=0
```

The generated command store was isolated under a scratch HOME. `discover` printed the Pet Store API and five operations; `generate` persisted one command group.

Demo path:

```text
./demo/run-all.sh (before prerequisite)  rc=1 in 0.03s
python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt  rc=0
./demo/run-all.sh (cold after venv install) rc=0, wall=21.59s
./demo/run-all.sh (warm)                 rc=0, wall=19.89s
```

The demo reported `Passed: 33` and `All 33 checks passed!`. The first failure was actionable and named the exact venv/Flask remedy; it was not a silent pass.

Daemon path:

```text
openapi-daemon start --socket-path <scratch>/muster.sock   rc=0
openapi-daemon status --socket-path <scratch>/muster.sock  rc=0, Running: true
openapi-daemon stop --socket-path <scratch>/muster.sock    rc=0, socket and pid removed
```

Docker path:

```text
docker build -f Dockerfile -t muster-dogfood-1ea7c54a .  rc=0
cold wall=182.50s; warm rebuild wall=168.78s
```

The three documented container help probes for `openapi-cli`, `openapi-mcp`, and `openapi-daemon` each returned rc=0. The temporary image was removed after the probes.

Audit path:

```text
audit info                 rc=0
 audit verify              rc=0, "2 records, chain intact"
audit log --limit 5       rc=0, newest events shown
 audit show --limit 5      rc=0, newest events shown
audit prune --dry-run     rc=0, "Would prune 0 of 5 records..."
```

## Performance

No PERF row was filed. The user-facing CLI operations were about 10ms process-start wall time in the scratch worktree, and the local demo completed in 19.89s warm for 33 checks; neither was a latency a user would notice during ordinary use. The cold build (14.54s) and Docker build (182.50s) are setup costs, not a runtime regression. Profiling was therefore not warranted.

## Ephemeral install leg

The bunker leg was executed, not skipped. Launch succeeded on `bunker-mvp` as agent `347dfc56`, testing the exact HEAD above. Evidence: `/tmp/bunker-qa-evidence-20261026T142133Z-3945564.jsonl`.

Collected results before teardown:

```text
repo-head              OK
fresh-install          FAIL: no space left on device while creating Go build directories
ci-pass                FAIL: test/security credential tests failed; test/util build failed
act                    FAIL: catthehacker/ubuntu:act-latest was unavailable locally
collect                 FAIL: battery truncated before final chaos-errorpath cell
```

The agent was destroyed and a post-destroy `bunker list` census reported `347dfc56` absent. Because the leg ran and produced cells, it is not labeled `SKIPPED-install-bunker`. The fresh-install result is not a product install verdict: the exact failure was the agent's storage exhaustion during compilation. The CI result is not a single clean product verdict either: the evidence contains both native test failures and a separate missing act image failure, and the battery truncated.

## Friction and follow-up

Friction count: 1 user-facing prerequisite (Flask/venv) plus 2 environmental fresh-box blockers (agent storage exhaustion and unavailable act image). The local product workflow itself completed after the documented prerequisite.

The owning foreman should treat the bunker evidence as incomplete/blocked rather than as a green install receipt. The exact product-side native failures need a clean-agent rerun with adequate storage before attribution. This lane did not wake the foreman or mutate scheduler state.

Evidence summary: `.coding-hermes/dogfood-evidence/run21/summary.txt`.

# README-058 — quickstart probe (recorded run)

**Row:** README-058 (wave `muster-foreman-2026-10-24-09-38-12`)
**Scope:** documentation only — `README.md` Quick start block. No product code changed.
**Why:** the previous quickstart pointed `discover`/`generate` at
`petstore3.swagger.io` and then claimed `GET /pet/1` and `/pet/findByStatus` "work".
A fresh run repeatedly 404'd because that upstream fixture is mutable, so the
first typed-command example could fail for reasons outside this repository.

The quickstart now uses the repo-owned, deterministic Task Forge demo service
(`demo/services/task-forge/server.py`, port 9801) and its own `openapi.json`.

## Recorded probe

Machine: this worktree (`/home/kara/worktrees/muster-README-058`, branch
`wt/README-058`). Go 1.26.10.

Setup, exactly the commands the README now documents:

```bash
make build-cli
python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt
PORT=9801 .venv/bin/python3 demo/services/task-forge/server.py    # separate terminal
```

Probe harness: a throwaway `HOME` and `XDG_DATA_HOME` (`mktemp -d`) wrapped
around every command, so a persisted command store from the host or from an
earlier run cannot mask a 404. `$out=$(...)` captures stdout+stderr and each
step's exit status is recorded.

```
HOME=/var/tmp/hermes-scratch/tmp.pEQ2YpEPHp
XDG_DATA_HOME=/var/tmp/hermes-scratch/tmp.rS5P8k6vCq

=== step 1: discover
$ ./bin/openapi-cli discover http://localhost:9801/openapi.json
API: Task Forge API (v1.0.0)
Base URL: http://localhost:9801

Other
  DELETE /comments/{comment_id}
  GET    /health
  GET    /milestones/{milestone_id}
  DELETE /milestones/{milestone_id}
  GET    /projects
  POST   /projects
  GET    /projects/{project_id}
  PATCH  /projects/{project_id}
  DELETE /projects/{project_id}
  GET    /projects/{project_id}/milestones
  POST   /projects/{project_id}/milestones
  GET    /projects/{project_id}/tasks
  POST   /projects/{project_id}/tasks
  GET    /tags
  POST   /tags
  GET    /tasks
  GET    /tasks/{task_id}
  PUT    /tasks/{task_id}
  PATCH  /tasks/{task_id}
  DELETE /tasks/{task_id}
  GET    /tasks/{task_id}/comments
  POST   /tasks/{task_id}/comments
--- exit=0

=== step 2: generate
$ ./bin/openapi-cli generate http://localhost:9801/openapi.json
Loading spec from: http://localhost:9801/openapi.json
✓ Parsed spec (version: 1.0.0)
✓ Generated 6 command groups
✓ Commands persisted to storage
✓ Registered command: milestones
✓ Registered command: comments
✓ Registered command: projects
✓ Registered command: tags
✓ Registered command: tasks
✓ Registered command: health

✓ Success! Commands registered and persisted
Run 'openapi-cli --help' to see available commands
Commands will load automatically on next CLI start
--- exit=0

=== step 3: typed command
$ ./bin/openapi-cli list-tasks -o json
[]
05:12:36 INF HTTP response received status="200 OK" status_code=200
--- exit=0

=== step 4: generic request
$ ./bin/openapi-cli request GET http://localhost:9801/tasks -o json
[]
--- exit=0

=== overall exit=0
```

(`generate` prints its `✓ Registered command:` lines in map order, so the order
differs between runs; the set is stable.)

### What the steps prove

| Step | Command | Result |
|---|---|---|
| 1 | `discover http://localhost:9801/openapi.json` | exit 0 — parses `Task Forge API (v1.0.0)`, base URL `http://localhost:9801`, 22 operations |
| 2 | `generate http://localhost:9801/openapi.json` | exit 0 — 6 command groups persisted to a fresh store |
| 3 | `list-tasks -o json` (typed command) | **exit 0, JSON body `[]`** — HTTP 200 from the local service |
| 4 | `request GET http://localhost:9801/tasks -o json` | exit 0 — JSON body `[]`, same operation via the generic path |

`[]` is the correct body for a freshly started Task Forge (its store is
in-memory and seeded only by `./demo/run-all.sh`); the README says so in the
comment above the typed command so an empty list is not mistaken for a failure.

## Documentation checks

```
$ git diff --check                                        # clean (no whitespace errors)
$ grep -F -n 'localhost:9801/tasks' README.md             # present (board check)
87:./bin/openapi-cli request GET http://localhost:9801/tasks -o json
$ grep -F -c 'GET /pet/1 and /pet/findByStatus work' README.md
0                                                         # required removal
$ grep -F -c 'https://petstore3.swagger.io' README.md
0                                                         # no quickstart claim depends on the third-party fixture
$ grep -n 'petstore3' README.md
337:- **Public petstore3 demo server is unreliable** — some endpoints return HTTP 500
    server-side (measured 2026-10-18: `/store/inventory`); use the local mock
    services (`./demo/run-all.sh`) for deterministic requests.
```

The one remaining `petstore3` mention is the "Known pitfalls"/limitations note,
which describes the upstream demo as **unreliable** and redirects to the local
mock services — it makes no "works" claim and is no longer part of the
quickstart block.

## Contract-vs-README note

The board row also requires the quickstart to keep working end to end. The
typed command that quickstart exercises (`list-tasks`) is the Task Forge
operation `GET /tasks`; the deterministic replacement therefore exercises the
same class of generated command (a zero-argument GET that returns JSON) that the
old `get-pet-by-id` example did, without depending on third-party state.

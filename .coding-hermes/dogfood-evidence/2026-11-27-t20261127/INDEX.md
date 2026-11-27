# Dogfood tick muster-dogfood-2026-11-27-10-14-45 — evidence index

Product: /home/kara/wojons/muster (github.com/wojons/muster), HEAD 8577f120 at run start.
Binary under test: `make build` at 8577f120 (6.0 s wall, warm module cache, go1.26.11, CGO on).
Backend: repo-owned Task Forge demo Flask server, PORT=9801 (11 paths / 22 operations).
HOME isolated per run: scratch home so the persisted command store starts empty.

## What was used (real user path)

1. `openapi-cli discover http://localhost:9801/openapi.json` -> 22 operations listed, 54 ms.
2. `openapi-cli generate http://localhost:9801/openapi.json` -> 72 ms; new DF-061 hint:
   "Registered 22 generated commands:" + first 10 names + "Run 'openapi-cli generate --list'".
3. `openapi-cli generate --list` -> 33 lines: store header + "Commands (22):" grouped by
   METHOD, `path -> name` (GET/POST/PUT/PATCH/DELETE). 54 ms.
4. `openapi-cli create-project --name 'Dogfood 27'` -> HTTP 201, table row printed, rc=0.
5. `openapi-cli list-projects` -> row present, 30-40 ms per call (HTTP to local Flask).
6. Shell completion: `completion bash` = 95,889 bytes containing the generated commands;
   registered function `__start_openapi-cli`; completions for `create-` = 5 candidates
   (create-project, create-project-milestone, create-project-task, create-tag,
   create-task-comment). Works ONLY with /usr/share/bash-completion/bash_completion
   sourced; without it: `_get_comp_words_by_ref: command not found` and 0 replies.
7. CLI completion record (OBS-MUSTER-5):
   `MUSTER_CLI_METRICS_PATH=<dir>/muster_cli.prom openapi-cli version|list-projects|...`

## Findings filed

| id | prio | one line |
|---|---|---|
| DF-064 | P2 | concurrent invocations lose counts: 40 x -P16 -> 7 recorded; 20 x -P4 -> 13; 20 sequential -> 20 |
| DF-065 | P2 | record published mode 0600 (umask 000 still 0600); node_exporter cannot read it |
| DF-066 | P2 | every invocation writes /var/lib/node_exporter/textfile by default; non-root gets a stderr warning per command; no opt-out |
| DF-067 | P3 | the completion record is undocumented (no MUSTER_CLI_METRICS_PATH anywhere in docs/) |
| DF-068 | P4 | DF-061 shipped enumerate-by-method --list + completion, docs still describe a one-line summary |

Not filed (already on the board): the six "Registered command: <group>" lines for 22
commands = REVIEW-MUSTER-17 (P4, pending). --profile store namespacing = DF-060; the
undiscoverable-store symptom = DF-061.

## Raw artifacts

- `prom-single-run.prom` — 1 version + 1 list-projects + 1 create-project + 1 failure.
- `prom-sequential-20.prom` — 20 sequential `version` -> counter 20 (exact).
- `prom-parallel-P4-20-sent.prom` — 20 sent at -P4 -> counter 13.
- `prom-parallel-P16-40-sent.prom` — 40 sent at -P16 -> counter 7.
- `mode-0600-proof.txt` — `ls -l` of a record written under `umask 000`.
- `nonroot-warning.txt` — `sudo -u nobody openapi-cli version` stderr.
- `perf.txt` — warm/cold CLI timings and the record-size scaling arm.

## Performance (Step 2b) — measured, no PERF row filed

Host load average 24.8 at measurement time, so treat absolutes as upper bounds; the deltas
are the signal. All scratch paths on local disk, tmpfs arm on /dev/shm.

| arm | median |
|---|---|
| `version`, metrics path unwritable (baseline + failed write) | 26.9 ms |
| `version`, record on local disk (1.7 KB) | 36.2 ms |
| `version`, record on tmpfs | 32.7 ms |
| `version`, 55 KB record (real, accumulated) | 46.7 ms |
| `list-projects`, no record write vs 55 KB record | 41.7 ms vs 63.8 ms |

Record-size scaling (`version`, synthetic record, series = command labels):

| series | file | median |
|---|---|---|
| 22 | 1.5 KB | 32.6 ms |
| 612 | 41.1 KB | 36.1 ms |
| 3000 | 205.9 KB | 49.8 ms |
| 10000 | 691.3 KB | 99.9 ms |

Verdict: the completion record costs ~9 ms per invocation, ~20 ms with a 55 KB record, and
grows linearly with label count; a 612-command spec (GitHub v3 scale) costs ~3.5 ms over the
22-command record. Nothing here reaches "a user would notice" except pathologically large
records, so no PERF row was filed. The cost is not the defect; the lost increments and the
file mode are.

## Side effects disclosed

A plain `openapi-cli version` (no env var of mine) wrote the host's real
`/var/lib/node_exporter/textfile/muster_cli.prom` (55,606 B, mode 0600, mtime 2026-11-27
06:46) because the CLI's default metrics path is that directory and kara has group write on
it. The file was left in place (it is the product's own output; deleting another agent's
textfile could blank a dashboard). Task Forge mock server killed at end of run.

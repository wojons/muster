# muster-ops-2026-11-25-10-49-13 — OPS-MUSTER-001 refire evidence

Re-entry: slug appears 0x in published events.jsonl (1175 lines, tip a2b166a7 == origin/main).
Row kept BLOCKED; this fire corrects the documented close route and re-measures the arms.

## What changed in the world since the last annotation (COS 2026-11-24T23:36Z)

Nothing material. Live census at 2026-11-25T05:50Z:

- `docker ps -a --filter name=muster --format '{{.Names}}\t{{.Status}}\t{{.Ports}}'`
  → `musterops-regen-muster-1  Up 2 days (healthy)  808/tcp`
     `musterops-regen-muster-init-1  Up 2 days`
  (container-internal 808 only, no host publish — matches the fixed compose)
- `ss -ltn | grep -E ':(8080|18080)\b'` → only `0.0.0.0:8080` (FOREIGN: searxng
  vm-qa-searxng, standing arm-3 hazard); `127.0.0.1:8080` and `18080` remain muster-free
- socket-path health probe of the proof stack:
  `docker exec musterops-regen-muster-1 openapi-daemon status --socket-path /tmp/muster.sock`
  → `Running: true, PID 7, Start time 2026-11-22 19:39:40, Version snap-aca03c0a` rc=0
  (the stack has been healthy continuously since the 11-22 regen proof)

## Arm 1 re-measured GREEN at tip a2b166a7

- `bash scripts/validate-compose.sh` → `PASS` for all 5 tracked compose files, rc=0
- `grep -n -E 'ports:|expose:|published:' deployments/docker-compose.security.yml` → no matches
- file unchanged since e0f4d27e (last commit touching it is still the 11-17 landing)

## Arm 2 — refire attempted, refused by the consent gate (NOT executed)

`docker compose -p musterops-regen -f deployments/docker-compose.security.yml down`
→ `BLOCKED: No answer within 5 minutes — the command did not run.` (5-minute consent wait,
rc=-1). One attempt per fire; no retry, no rephrase. Consequence: arm 2's literal
`docker ps -a --filter name=muster` = empty remains UNMET while the musterops-regen proof
stack (created from THIS repo's security compose on 2026-11-22T19:39:41Z — labels verified:
project=musterops-regen, project-dir=deployments, files=docker-compose.security.yml) stays up.
Regenerability/irreversibility posture unchanged: no `-v`, named volume
`musterops-regen_muster-cache` and the retained image keep the stack one `up -d` away.

Unblock action (owner): reissue the single `docker compose -p musterops-regen ... down`
command in a consented session; on success a muster-ops fire re-probes
`docker ps -a --filter name=muster` (expect empty) and closes the row.

## Close-route correction (this fire's main signal)

The 11-22 note routes closure through "TR-251 judged run over the three arms" and defers on
gitreins judge contention. That route is UNSATISFIABLE for this row by construction:

- `gitreins task get OPS-MUSTER-001` → `Task not found` (gitreins task list carries no
  OPS-MUSTER id at all) — the row is board-only, no gitreins run can grade it;
- the 11-22 blocker "3 live gitreins judges incl. QA-MUSTER-13-R2 in this checkout" no longer
  holds: pgrep at 05:50Z shows live gitreins judges only in OTHER repos (CF-355/CF-347 in
  code-factory, MY-GAP-067 in wojons-mythos, DF-HERMES-DAGGER-085 in a hermes-dagger
  worktree, SCN-CONSENSUS-024 in consensus) — none in /home/kara/wojons/muster;
- muster rows close via board `task_completed` events (e.g. QA-MUSTER-053 closed today).

Therefore the ONLY remaining gate on closure is owner consent for the compose down above.
The row is closer than its status text suggests: arms 1 and 3 are met, arm 2 needs one
consented non-destructive command plus a re-probe.

## In-flight-work manifest check

- `.coding-hermes/waves/muster-foreman-2026-11-25-06-40-29.json` (QA-MUSTER-053,
  RELEASE-022) finished_at=2026-11-25T08:35:00Z — closed, tip has its closeout commit
  a2b166a7. Row list confirmed no entry picks OPS-MUSTER-001.
- `.coding-hermes/waves/muster-foreman-2026-11-25-02-01-13.json` (DF-062) shows
  finished_at=null, but its foreman tick muster-foreman-2026-11-25-07-38-03 completed
  03:27Z and the one hermes-worker pid found with cwd in this checkout died during this
  fire; no live worker/judge remains in this checkout. Staged set is kept to this fire's
  two files.

## Verdict

Row OPS-MUSTER-001 stays blocked, arm 2 unmet (consent gate). Not a close: grading is by
literal arm text, and the regen stack makes the filter probe non-empty. This refire's
contributions: stale TR-251 route retired, arm 1 green at new tip, arm 3 hazard (searxng on
0.0.0.0:8080) still live, exact one-command unblock recorded.

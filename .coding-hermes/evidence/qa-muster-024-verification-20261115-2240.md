# QA-MUSTER-024 verification-only closure — muster foreman tick 2026-11-15-22-40-11

Row: QA-MUSTER-024 — deployments/docker-compose.security.yml healthcheck.test lacks the CMD prefix
Decision: premise already satisfied on main — CLOSED verification-only, no worker, no source change.

Repo: /home/kara/wojons/muster
HEAD at probe: dfdc59a4656850e1b95e691f9fac892dd9f5260d (origin/main)
Host: load ~7 (16 cores), docker compose v? (see below)

## 1. The file at HEAD carries the CMD prefix

command: grep -n -A6 'healthcheck' deployments/docker-compose.security.yml

   57:    # the healthcheck interrogate THAT daemon instead of running a one-shot
   58:    # CLI print (QA-MUSTER-024's CMD-prefix rule is preserved).
   59:    entrypoint: ["/usr/local/bin/openapi-daemon"]
   60:    command: ["start", "--foreground", "--socket-path", "/tmp/muster.sock"]
   61:    healthcheck:
   62:      test: ["CMD", "/usr/local/bin/openapi-daemon", "status", "--socket-path", "/tmp/muster.sock"]
   63:      interval: 30s
   64:      timeout: 5s
   65:      retries: 3
   66:      start_period: 10s

## 2. compose config now parses (the row's failing gate)

command: docker compose -f deployments/docker-compose.security.yml config; echo rc=$?
result: rc=0, empty stderr (no "healthcheck.test must start either by CMD, CMD-SHELL or NONE")

The failure mode recorded in the row ("docker compose config exits 15") does not reproduce at HEAD.

## 3. History of the fix (so this closure is attributable, not assumed)

command: git log --oneline -5 -- deployments/docker-compose.security.yml

    9524f6f2 fix(deployments): hardened stack runs a long-lived daemon and a real healthcheck (QA-MUSTER-029 acceptance)
    14449b1f fix(deploy): CMD-prefix the security-stack healthcheck, gate compose parsing
    6ed35918 fix(ops): repair hardened compose healthcheck

## 4. What this closure does NOT claim

The row's live-container arm (up -d, container healthy) is still owned by the muster-ops lane: the published
127.0.0.1:8080 port is held by QA-MUSTER-029 residue and retiring it is operator-gated (recorded in the row's
ops_audit_20261113_2123 block). This closure covers the row's own failing gate: field syntax + compose config parse.

## Provenance

Probe executed by the foreman at tick muster-foreman-2026-11-15-22-40-11 on the controller, not from a battery
artifact and not from a prior tick's claim.

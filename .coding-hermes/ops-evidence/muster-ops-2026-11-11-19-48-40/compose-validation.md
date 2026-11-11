# muster-ops-2026-11-11-19-48-40 — QA-MUSTER-024 (compose security stack)

Lane: `muster-ops` · Workdir (registration): `/home/kara/.hermes/stand-in/pm-lane/muster`
Canonical checkout: `/home/kara/wojons/muster` · Remote alias: `origin` (GitHub, wojons/muster)
Base at tick start: `3b6de28153f7726a81ebc1b10fa70d382d48c410`
Base at publication: `3eb9cbb5cf3f30da85e7c088fcd406de4f01cc46` (sibling `muster-docs` tick
`muster-docs-2026-11-11-19-48-40` added 8 DOC rows to `tasks.jsonl`; no file overlap with this row)
Isolated worktree: `/home/kara/muster-wt-ops-20261111-1948` (branch `lane/muster/ops-20261111-1948`)
Host docker: client/daemon `29.1.3`, compose `v2.32.1`

Tick-event census before probing (first board operation after fetch):
`slug_count=0`, `iso_count=0`, `day_count=0` over `origin/main:.coding-hermes/board/events.jsonl`
→ fresh tick, one-row budget available.

## Defect (as filed by the QA battery)

`deployments/docker-compose.security.yml` declared
`test: ["/usr/local/bin/openapi-cli", "version"]` with no `CMD` prefix, so
`docker compose config` aborts and the hardened path documented in
`deployments/DOCKER-SECURITY.md:106` never reaches `up`.

## Arm 1 — `config` exits 0 (PASS)

Before (published base, unchanged file):

    $ docker compose -f /home/kara/wojons/muster/deployments/docker-compose.security.yml config
    healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"
    $ echo rc=$?
    rc=15

After (this worktree):

    $ bash scripts/validate-compose.sh
    PASS  deployments/docker-compose.security.yml
    PASS  deployments/logging/elk/docker-compose.yml
    PASS  deployments/logging/loki/docker-compose.yml
    PASS  deployments/monitoring/docker-compose.yml
    PASS  docker-compose.yml
    all 5 compose file(s) valid
    $ echo rc=$?
    rc=0

## Arm 3 — VERIFY: a gate iterates every tracked compose file (PASS)

Added `scripts/validate-compose.sh` (tracked-file sweep via
`git ls-files | grep -Ei '(^|/)(docker-)?compose(\.[^/]*)?\.ya?ml$'`), wired into
`make validate-compose` and as a CI step in `.github/workflows/ci.yml` after `Demo`.
Negative control — the same gate against the pre-fix file tree:

    $ bash scripts/validate-compose.sh -f /home/kara/wojons/muster/deployments/docker-compose.security.yml
    FAIL  /home/kara/wojons/muster/deployments/docker-compose.security.yml (rc=15)
          healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"
    compose validation FAILED
    $ echo rc=$?
    rc=1

The healthcheck still probes `openapi-cli version`; only the required `CMD` form was added.

## Arm 2 — `up -d` reaches healthy (FAILED, not caused by this row)

    $ MUSTER_SECURITY_SUBNET=172.29.0.0/16 MUSTER_SECURITY_GATEWAY=172.29.0.1 \
        docker compose -p musterqa24 -f deployments/docker-compose.security.yml up -d --build
    ...
    Container musterqa24-muster-1  Started
    up_rc=0

    $ docker ps --filter name=musterqa24 --format '{{.Names}}\t{{.Status}}'
    musterqa24-muster-1       Restarting (139) 7 seconds ago
    musterqa24-muster-init-1  Up About a minute

    $ docker inspect musterqa24-muster-1 --format '{{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}}'
    restarting exit=139 oom=false

    $ docker inspect musterqa24-muster-1 --format '{{json .State.Health}}'
    {"Status":"unhealthy","FailingStreak":0,"Log":[]}

    $ docker logs --tail 20 musterqa24-muster-1
    (empty)

Two independent findings, both outside this row's fix:

1. **Subnet collision (worked around, then fixed as an override).**
   `172.28.0.0/16` is already claimed on this host by `crier-ui-test-net`, and
   `172.30.0.0/16` / `172.31.0.0/16` by `dexdat-memory_memory-network` /
   `trouble_default`, so the hardcoded subnet made `up` fail on overlap.
   The subnet and gateway are now `MUSTER_SECURITY_SUBNET` /
   `MUSTER_SECURITY_GATEWAY` with the original `172.28.0.0/16` / `172.28.0.1`
   defaults, so behaviour is unchanged where the range is free. This host ran on
   `172.29.0.0/16`.

2. **The service container crash-loops with exit 139 under the hardened sandbox.**
   Not a healthcheck defect: `up` returned 0, the container starts and immediately
   dies with SIGSEGV, so the healthcheck is never evaluated (`FailingStreak: 0`,
   empty log). The same binary built from the same tree is healthy on the host, so
   the crash is introduced by the container's security options, not the code:

       $ CGO_ENABLED=0 go build -o /home/kara/.hermes/cache/scratch/muster-openapi-cli ./cmd/openapi-cli
       build_rc=0
       $ ./muster-openapi-cli --help >/dev/null; echo help_rc=$?
       help_rc=0
       $ ./muster-openapi-cli version
       dev (unknown)
       version_rc=0

   Container security context measured from the running container:
   `user=65532:65532 ro=true pid=container:<muster-init>`,
   `cap_drop=[ALL]`, `no-new-privileges:true`, `apparmor:docker-default`, and
   `seccomp=./seccomp-profile.json` — a bespoke allowlist whose
   `defaultAction` is `SCMP_ACT_ERRNO` (every syscall not named returns EPERM)
   instead of docker's default profile.

   Ordered ladder for the follow-up (no destructive step taken here):
   `deployments/docker-compose.security.yml` → measure the image with docker's
   default seccomp profile (`--security-opt seccomp=default`) to confirm the
   bespoke profile is the variable; if confirmed, extend
   `deployments/seccomp-profile.json` with the missing Go-runtime syscalls
   (start with `clone3`, `rseq` neighbours, `arch_prctl`, `getrandom` variants)
   and re-run `up -d` until `docker inspect` reports `healthy`. `read_only: true`
   and `pid: service:muster-init` are the next suspects in that order.

Residual state and cleanup: containers `musterqa24-muster-1` (restart-looping) and
`musterqa24-muster-init-1` are still present, plus image `muster:latest` from the
`--build`. Cleanup was attempted and refused at the approval gate
(`docker compose -p musterqa24 ... down -v` blocked, no consent in window), so it is
recorded as an operator action rather than retried:

    docker compose -p musterqa24 -f deployments/docker-compose.security.yml down -v --remove-orphans

## Family census (structural context, not lane health)

`python3 ~/.hermes/scripts/fleet_ops_check.py --json` → `census_rc=1` with structured
output (measurement verdict). Family `muster`: kind `service`, `gaps=[]`, 16 lanes
including `muster-ops`; registration `repo_url=local:/home/kara/.hermes/stand-in/pm/muster`,
workdir `/home/kara/.hermes/stand-in/pm-lane/muster`, `enabled=true`, `cooldown_s=86400`,
`namespace_id=ops`, `deliver=telegram:-1003310984808:4370`.

## Disposition

Row `QA-MUSTER-024` stays **pending**: arm 1 and arm 3 are proven, arm 2 is blocked by
the sandbox crash-loop above, which is a distinct defect this row's change set does not
fix. No second row was filed (one-row budget); the residual is carried in this artifact
and in the row's audit event.

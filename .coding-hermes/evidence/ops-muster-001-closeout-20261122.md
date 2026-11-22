# OPS-MUSTER-001 closeout — measured live at tip 9de7f38c (muster-ops-2026-11-22-19-33-17)

Tick: muster-ops-2026-11-22-19-33-17, lane muster-ops.
All probes read-only except the two explicitly flagged compose calls (up = regen proof arm 2; down = REFUSED, see refused_fires).

## Live census at tick time

```
$ docker ps -a --filter name=muster --format '{{.Names}} {{.Status}} {{.Ports}}'
(no lines — zero muster containers, exited or running)
$ ss -ltn | grep -E ':(8080|18080)\s'
LISTEN 0 4096  0.0.0.0:8080 ...
LISTEN 0 4096  [::]:8080 ...
(no 127.0.0.1:8080 and no 18080 binding)
```

The 0.0.0.0:8080 listener is not muster: it is Bane's searxng container `vm-qa-searxng`
(`0.0.0.0:8080->8080/tcp`, started 2026-11-22T12:24:45Z, restart=no, no compose labels).
This is the live-controller-host instance of arm 3's unattributable-port hazard: any probe of
127.0.0.1:8080 by a muster battery would be answered by searxng, exactly the
QA-MUSTER-033 mechanism. Host-infra-owned; filed for the owner, not mutated here.

## Arm 1 — validate-compose still green at tip

```
$ bash scripts/validate-compose.sh   (tip 9de7f38c)
PASS  deployments/docker-compose.security.yml
PASS  deployments/logging/elk/docker-compose.yml
PASS  deployments/logging/loki/docker-compose.yml
PASS  deployments/monitoring/docker-compose.yml
PASS  docker-compose.yml
all 5 compose file(s) valid   rc=0
$ grep -n 'ports\|127.0.0.1:8080' deployments/docker-compose.security.yml
33: - NET_BIND_SERVICE   (capability comment only)
100: comment referring to the removed dead publish
```

Fix commit unchanged: e0f4d27e (ops: drop dead 8080 publish).

## Arm 2 — reap premise resolved, regenerability proven

The five orphan containers the arm gated on are gone (COS pass 2026-11-21, commit 461c7471;
re-measured live this tick: `docker ps -a --filter name=muster` → empty). Nothing to reap;
the owner gate (blocked_on=OWNER-DOCKER-REAP, PM tick 2026-11-20-20-30-12) is moot.

Regenerability clause executed from the retained image:

```
$ docker compose -p musterops-regen -f deployments/docker-compose.security.yml up -d
up_rc=0, 2 containers started
$ docker ps --filter name=musterops-regen --format '{{.Names}} | {{.Status}} | {{.Ports}}'
musterops-regen-muster-1 | Up 5 minutes (healthy) | 8080/tcp   (internal only, no host publish)
musterops-regen-muster-init-1 | Up 5 minutes |
$ docker inspect musterops-regen-muster-1 --format '{{.State.Health.Status}}'
healthy checks=5; last log: Daemon Status: Running: true, PID 7, Socket: /tmp/muster.sock,
Version: snap-aca03c0a (aca03c0a)
```

## refused_fires / observed_fires

- refused_fires = 1: `docker compose -p musterops-regen -f deployments/docker-compose.security.yml down`
  refused by the unattended destructive-action gate after a 5-minute approval timeout; the command
  did NOT run. Consequence: the two musterops-regen scratch containers above are still running on
  the host awaiting sanctioned teardown (owner or hygiene cron). They publish no host port and are
  capped at 1 CPU / 512M. observed_fires for the standing gate = 1 (this refused fire).

## Arm 3 — no reintroduced publish

Tracked compose files carry no port publish for the socket-only service (validate PASS + grep
above). The remaining host-port risk is cross-project (searxng wildcard publish), recorded above.

Sha256 of raw probe captures (frozen under .gitreins/history/2026-11-22/, untracked):

```
ccec443dadfa1fdcb2ed5f5c0d62014aa49b882f3d9e6e08dae5c68dd4b50501  regen-up.out
9ad1a754c0b50a155e13aa632e041f103e01274775449e947bef7c3ee9d5b87b  validate-compose.out
36ab003267df1c80a4039efc0117385b3941a4752b4ecddc1191a84f5d4f4034  regen-proof.txt
```

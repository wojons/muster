# OPS-MUSTER-001 — dead 8080 publish + orphaned hardened compose stacks

Tick: muster-ops-2026-11-17-00-09-53 (workdir /home/kara/.hermes/stand-in/pm-lane/muster)
Host: Linux 7.0.0-38-generic (controller), measured 2026-11-17T00:09-05:00
Repo under test: /home/kara/wojons/muster @ 3a741dff (origin/main)

## 1. Finding — the hardened stack publishes a port its daemon never binds

deployments/docker-compose.security.yml ran the daemon in the foreground on a
unix socket and still declared a TCP publish:

    entrypoint: ["/usr/local/bin/openapi-daemon"]
    command: ["start", "--foreground", "--socket-path", "/tmp/muster.sock"]
    healthcheck: openapi-daemon status --socket-path /tmp/muster.sock
    expose: ["8080"]
    ports: ["127.0.0.1:8080:8080"]

Measured against a live stack (project muster-r2-seccomp, up 5 days):

    $ curl -sv --max-time 5 http://127.0.0.1:8080/health
    * Established connection to 127.0.0.1 (127.0.0.1 port 8080)
    > GET /health HTTP/1.1
    * Recv failure: Connection reset by peer

    $ curl -s -o /dev/null -w 'root_http=%{http_code}\n' http://127.0.0.1:18080/
    root_http=000                       # project musterops24, same reset

    $ docker logs --tail 8 muster-r2-seccomp-muster-1
    [INFO] proxy server started socket=/tmp/muster.sock
    Metrics: http://127.0.0.1:9110/metrics
    Daemon started successfully
    PID file: /tmp/muster.sock.pid
    Socket: /tmp/muster.sock

    $ docker inspect -f 'health={{.State.Health.Status}} fails={{.State.Health.FailingStreak}}' muster-r2-seccomp-muster-1
    health=healthy fails=0              # healthcheck probes the SOCKET, not the publish

Source fact (no TCP listener on the daemon path):

    $ grep -rn "net.Listen(" pkg/daemon/proxy.go
    pkg/daemon/proxy.go:455:  listener, err := net.Listen("unix", p.socketPath)

So the publish is a dead mapping: docker-proxy accepts on the host, forwards to
container:8080 where nothing listens, and the connection is reset. Two
consequences, both operational:

1. Port squat. 127.0.0.1:8080 has been held by a container that serves nothing
   for 5 days; 18080 likewise for 3 days. Nothing else on the host can bind them.
2. Unattributable probes. Any probe cell pointed at 127.0.0.1:8080 on this host
   reads a reset (rc=56) that looks like a failing service but is really an empty
   publish — the mirror image of QA-MUSTER-033 (a foreign listener answering 200
   on a bunker agent). Neither direction can be attributed to the compose
   project from the port alone.

## 2. Finding — three orphaned compose projects from finished probe runs

    $ docker ps -a --filter name=muster --format '{{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}'
    musterops24-muster-1            Up 3 days (healthy)  127.0.0.1:18080->8080/tcp  muster:ops24-probe
    musterops24-muster-init-1       Up 3 days                                      busybox:latest
    muster-r2-seccomp-muster-1      Up 5 days (healthy)  127.0.0.1:8080->8080/tcp   muster:latest
    muster-r2-seccomp-muster-init-1 Up 5 days                                      busybox:latest
    musterqa24-muster-init-1        Up 5 days                                      busybox:latest

Provenance (docker inspect labels):

    project=musterops24        workdir=/home/kara/.hermes/cache/scratch/muster-ops-20261113-2123/head-tree/deployments
                               cfg=.../docker-compose.security.yml,.../port-accommodation.yml   started 2026-11-13T22:05:22Z
    project=muster-r2-seccomp  workdir=/home/kara/proj-wt-muster-carter-r2/deployments            started 2026-11-11T22:48:50Z
    project=musterqa24         workdir=/home/kara/muster-wt-ops-20261111-1948/deployments          started 2026-11-11T20:06:47Z

    $ ls /home/kara/.hermes/cache/scratch/muster-ops-20261113-2123/
    No such file or directory   # scratch GC removed the tree the stack was launched from

musterqa24 is a half-torn stack: its muster service container is gone, only the
busybox init container survives. musterops24 can no longer be brought down with
`docker compose` from its project dir, because that dir (and its
port-accommodation overlay) no longer exists — the stack is now only addressable
by container name. All three stacks trace to this lane's own evidence runs
(.coding-hermes/evidence/ops-qa024-*.md), and no tick is using them:

    $ sqlite3 -readonly .../scheduler.db "select id,status from ticks where status='running'"
    ... muster-ops-2026-11-17-00-09-53 (this tick), plus unrelated projects
    $ ps -eo pid,args | grep -E 'docker(-compose| compose)'
    (none)

## 3. Change made (bounded, reversible, inside the worktree)

deployments/docker-compose.security.yml: removed the dead `expose: 8080` and
`ports: 127.0.0.1:8080:8080` pair, replaced with a comment naming the socket-only
reality, the operator command that actually works, and this evidence path.

Validated:

    $ docker compose -f docker-compose.security.yml config -q ; echo rc=$?
    rc=0

Behaviour change: none at runtime (the publish never carried traffic); the file
no longer advertises a TCP surface that does not exist, and future hardened
stacks stop squatting 8080.

## 4. Left alone deliberately — owner-gated reap

The five containers are still running. Reaping them is destructive and was NOT
executed unattended. They are regenerable, which is the precondition for the
reap:

- compose sources: deployments/docker-compose.security.yml parses (rc=0 above);
  the two surviving worktrees (/home/kara/proj-wt-muster-carter-r2,
  /home/kara/muster-wt-ops-20261111-1948) still contain their deployments dir;
- images retained: muster:latest (5 d), muster:ops24-probe (3 d) — a reap that
  removes containers only keeps them, so `up -d` restores the stack without a rebuild.

Owner-run reap (no image or volume deletion, fully reversible by `up -d`):

    docker rm -f musterops24-muster-1 musterops24-muster-init-1 \
                 muster-r2-seccomp-muster-1 muster-r2-seccomp-muster-init-1 \
                 musterqa24-muster-init-1
    docker network rm musterops24_muster-network muster-r2-seccomp_muster-network \
                      musterqa24_muster-network 2>/dev/null || true
    ss -ltn | grep -E ':(8080|18080)\b' || echo 'ports free'

## 5. Host context at tick time

    $ df -h / | tail -1
    /dev/nvme0n1p2  1.8T  1.4T  362G  80%  /        # 92% -> 80% since the disk shed
    $ cat /proc/loadavg
    6.32 11.14 14.21 2/1893 ...

No service restart was performed or needed (fault named: dead publish, not a
crash loop; both stacks report healthy via the socket healthcheck).

#!/usr/bin/env python3
"""Stage the three dogfood rows for muster through boardctl (argv lists, no shell quoting)."""
import subprocess
import sys

REPO = "/home/kara/wojons/muster"
BOARD = "/home/kara/wojons/muster/.coding-hermes/board"

rows = [
    {
        "id": "QA-MUSTER-052",
        "priority": "P2",
        "complexity": "2",
        "title": "P2 — daemon-operations.md still documents the pre-fix /health/ready: measured 503 not_ready, doc says unconditional 200",
        "type": "task",
        "tags": "dogfood,docs,daemon,health",
        "reasoning": (
            "Dogfood tick muster-dogfood-2026-11-17-23-48-14 at HEAD b3e50644, binaries built from this tree "
            "(go build ./cmd/daemon, ./cmd/openapi-cli). docs/guides/daemon-operations.md claims, under "
            "'GET /health/ready': 'Readiness. Currently identical in behavior to liveness - the handler does not "
            "consult backend connectivity or any subsystem state. A proxy daemon with a dead backend still answers "
            "ready/200 (verified: only non-health paths return 502 in that state)', and under 'Response when things "
            "are wrong': 'No known path returns a non-200 today. If you observe a non-200 health response, the daemon "
            "process itself is broken - treat it as down.' The Semantics-for-LB table likewise tells operators that "
            "/health/ready is 'not yet backend-aware'. "
            "MEASURED (start --backend-url http://127.0.0.1:19999, nothing listening): /health -> 200 "
            "{\"status\":\"healthy\"}; /health/live -> 200 {\"status\":\"alive\",...}; /health/ready -> 503 "
            "{\"status\":\"not_ready\",\"reason\":\"backend_unreachable\",\"error\":\"Get \\\"http://127.0.0.1:19999/health\\\": "
            "dial tcp 127.0.0.1:19999: connect: connection refused\",\"timestamp\":\"2026-11-17T23:57:46Z\"}; / and "
            "/some/path -> 502 text/plain 'Failed to execute proxy request'. Against a backend that accepts and never "
            "answers (blackhole listener), /health/ready -> 503 after 2.0020 / 2.0013 / 2.0009 s, while /health still "
            "answers in 0.2 ms. "
            "This is the documentation lag behind OBS-MUSTER-6 ('Make /health/ready honest'), which is closed: the code "
            "now probes the backend (cmd/daemon/main.go:565-590 backendReadinessHandler, reason=backend_unreachable / "
            "backend_unhealthy + backend_status). The doc was never updated, and its troubleshooting rule now inverts "
            "the risk: an operator who follows it restarts a healthy daemon on any backend outage, and an LB built on "
            "the 'not yet backend-aware' note mis-scopes its probe semantics. The 503 body schema and the 2 s probe "
            "budget appear nowhere in the docs."
        ),
        "ac": (
            "docs/guides/daemon-operations.md documents the post-OBS-MUSTER-6 behaviour: (1) /health/ready returns 503 "
            "with reason=backend_unreachable|backend_unhealthy, backend_status for the unhealthy case and the probe "
            "error string when the backend cannot be reached, plus the 2 s probe budget (measured 2.0013-2.0020 s "
            "against a hung backend); (2) the 'identical to liveness' and 'No known path returns a non-200' claims are "
            "gone and replaced with what /health and /health/live still guarantee; (3) the LB/systemd table no longer "
            "calls ready 'not yet backend-aware' and states what a 503 means for pool membership vs a daemon failure; "
            "(4) the doc's verification note is bound to a re-runnable command, not the word 'verified'. "
            "Acceptance is checked by grep: no remaining 'does not consult backend connectivity', 'still answers "
            "ready/200', 'No known path returns a non-200', or 'not yet backend-aware' in the file; and the quoted "
            "503 body matches a live daemon built from HEAD. Docs-only change: no product code touched."
        ),
    },
    {
        "id": "QA-MUSTER-053",
        "priority": "P3",
        "complexity": "2",
        "title": "P3 — metrics-listener bind failure is silent in the default daemonized mode: the documented [WARN] goes to /dev/null",
        "type": "task",
        "tags": "dogfood,daemon,observability,metrics",
        "reasoning": (
            "docs/guides/daemon-operations.md says of the Prometheus listener: 'A busy port is a WARNING, not a fatal: "
            "the daemon keeps serving the socket and prints [WARN] metrics listener not started on ...'. "
            "MEASURED at HEAD b3e50644 with 127.0.0.1:19103 already held by a listener: "
            "ARM A (start --foreground): stderr = '[WARN] metrics listener not started on 127.0.0.1:19103: metrics "
            "listen \"127.0.0.1:19103\": listen tcp 127.0.0.1:19103: bind: address already in use' - the claim holds. "
            "ARM B (start, i.e. the documented default, which daemonizes): rc=0, stdout = only 'Daemon started "
            "successfully' + PID/socket lines, stderr EMPTY, and the child's stdio is /dev/null on both fds "
            "(/proc/<pid>/fd/1 and /fd/2 -> /dev/null; cmd/daemon/main.go:448-450 sets cmd.Stdout/Stderr = devnull). "
            "No log file exists on disk to hold it: the shipped flag set has no --log-file and no MUSTER_LOG_FILE, and "
            "nothing was written anywhere. "
            "So in the shape the doc tells operators to run, losing the metrics endpoint is completely silent: the "
            "scrape target just stops answering and the only evidence of the bind error is in a fd that goes nowhere. "
            "The same swallow applies to the other [WARN] lines on that stderr (metrics shutdown, readiness response "
            "close)."
        ),
        "ac": (
            "Measured on both arms with a port already held: (a) `start --foreground` with a busy --metrics-addr, and "
            "(b) plain `start` (daemonized) with the same busy address, each produce an operator-visible warning naming "
            "the address AND the bind error - either on the parent's stdout/stderr or in a real log destination that the "
            "daemon documents. Evidence must be the two arms' captured stdout/stderr plus the child's fd targets. If the "
            "chosen fix is a log destination, docs/guides/daemon-operations.md states exactly where the warning lands; if "
            "the chosen fix is that an explicitly requested --metrics-addr must not fail silently, `start` exits non-zero "
            "with the bind error instead. Either way the doc sentence 'the daemon ... prints [WARN]' matches an arm an "
            "operator can run."
        ),
    },
    {
        "id": "QA-MUSTER-054",
        "priority": "P3",
        "complexity": "2",
        "title": "P3 — every proxied request carries X-Forwarded-For: \"@\" (the Unix socket pseudo-host), so backends cannot attribute or rate-limit callers",
        "type": "task",
        "tags": "dogfood,daemon,proxy,observability",
        "reasoning": (
            "Dogfood tick muster-dogfood-2026-11-17-23-48-14 at HEAD b3e50644. Real-use shape: `openapi-daemon start "
            "--backend-url http://127.0.0.1:19103` in front of an echo backend that records the headers it receives. "
            "Two clients against the same daemon: "
            "(1) plain request, no XFF sent -> backend receives x-forwarded-for: \"@\"; "
            "(2) client sends 'X-Forwarded-For: 203.0.113.9' and 'X-Real-IP: 203.0.113.9' -> backend receives "
            "x-forwarded-for: \"@\" and x-real-ip: \"203.0.113.9\". "
            "Stripping the client-supplied XFF is the right anti-spoofing choice, but the replacement value is Go's "
            "UnixConn RemoteAddr pseudo-host '@', not an address. Consequence for a user: every proxied request looks "
            "like it came from one caller named '@' in backend logs, per-client rate limits collapse into a single "
            "bucket, and an audit trail built on XFF records a value that is not an address at all. The daemon is "
            "socket-only, so this affects 100% of proxied traffic - there is no deployment where XFF carries a real "
            "client address today. daemon-operations.md does not mention XFF at all, so an operator cannot even learn "
            "that the value is synthetic."
        ),
        "ac": (
            "Proxied requests no longer carry a synthetic '@' in X-Forwarded-For. Choose and document one behaviour in "
            "docs/guides/daemon-operations.md: either the header is omitted for Unix-socket clients (and the doc says why "
            "and what a backend should use instead), or it carries a documented non-address marker that cannot be "
            "mistaken for a client IP. A client-supplied X-Forwarded-For must still not pass through unaltered. "
            "Evidence: the same two-client probe (with and without a client-supplied XFF) against an echo backend, "
            "before/after, with the received header set quoted."
        ),
    },
]


def run(argv):
    p = subprocess.run(argv, capture_output=True, text=True)
    print("rc=%d %s" % (p.returncode, " ".join(argv[:6])))
    if p.stdout.strip():
        print("  out: " + p.stdout.strip()[:400])
    if p.stderr.strip():
        print("  err: " + p.stderr.strip()[:400])
    return p.returncode


fails = 0
for r in rows:
    argv = [
        "boardctl", "-C", BOARD, "create",
        "--id", r["id"],
        "--title", r["title"],
        "--type", r["type"],
        "--status", "pending",
        "--priority", r["priority"],
        "--complexity", r["complexity"],
        "--reasoning", r["reasoning"],
        "--acceptance-criteria", r["ac"],
        "--capability-tags", r["tags"],
        "--meta", "source=dogfood",
        "--meta", "tick=muster-dogfood-2026-11-17-23-48-14",
    ]
    fails += run(argv) != 0
print("FAILED=%d" % fails)
sys.exit(1 if fails else 0)

#!/usr/bin/env python3
"""
seed.py — Creates 20 data points across 3 metrics (cpu, memory, disk).
Run:  python seed.py
Requires: the metrics-engine server running on http://localhost:9804
"""
import json
import sys
from datetime import datetime, timedelta, timezone

try:
    from urllib.request import Request, urlopen
    from urllib.error import URLError, HTTPError
except ImportError:
    print("FATAL: urllib not available (should not happen in Python stdlib)")
    sys.exit(1)

BASE = "http://localhost:9804"


def post_json(path, body):
    url = f"{BASE}{path}"
    data = json.dumps(body).encode("utf-8")
    req = Request(url, data=data, method="POST")
    req.add_header("Content-Type", "application/json")
    try:
        with urlopen(req, timeout=10) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except HTTPError as e:
        return e.code, json.loads(e.read().decode("utf-8"))
    except URLError as e:
        print(f"  Connection error: {e.reason}")
        return None, None


def main():
    print(f"Seeding metrics-engine at {BASE} ...")

    # Start from 2 hours ago
    base_ts = datetime.now(timezone.utc) - timedelta(hours=2)

    # Generate 20 points: 6 cpu + 8 memory + 6 disk = 20
    points = []

    for i in range(6):
        ts = (base_ts + timedelta(minutes=20 * i)).strftime("%Y-%m-%dT%H:%M:%SZ")
        points.append({
            "name": "cpu",
            "value": round(30.0 + 20.0 * (i % 3) + 5.0 * (i % 2), 2),
            "timestamp": ts,
        })

    for i in range(8):
        ts = (base_ts + timedelta(minutes=15 * i)).strftime("%Y-%m-%dT%H:%M:%SZ")
        points.append({
            "name": "memory",
            "value": round(60.0 + 10.0 * (i % 4) + 2.0 * i, 2),
            "timestamp": ts,
        })

    for i in range(6):
        ts = (base_ts + timedelta(minutes=25 * i)).strftime("%Y-%m-%dT%H:%M:%SZ")
        points.append({
            "name": "disk",
            "value": round(45.0 + 5.0 * (i % 5) + 1.0 * i, 2),
            "timestamp": ts,
        })

    assert len(points) == 20, f"Expected 20 points, got {len(points)}"
    print(f"  Prepared {len(points)} points: cpu=6, memory=8, disk=6")

    status, resp = post_json("/metrics", {"points": points})
    if status == 201:
        print(f"  OK  201  inserted={resp.get('inserted')}  errors={resp.get('total_errors')}")
    else:
        print(f"  FAIL {status} {resp}")
        sys.exit(1)

    # Verify with range queries
    print("Verifying seed data...")
    try:
        req = Request(f"{BASE}/metrics/cpu?from={base_ts.strftime('%Y-%m-%dT%H:%M:%SZ')}")
        with urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            print(f"  cpu points:  {body.get('count', 0)}")
    except Exception as e:
        print(f"  Verify error (cpu): {e}")

    try:
        req = Request(f"{BASE}/metrics/memory/latest")
        with urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            latest = body.get("data", {})
            print(f"  memory latest: value={latest.get('value')} @ {latest.get('timestamp')}")  # noqa: E501
    except Exception as e:
        print(f"  Verify error (memory/latest): {e}")

    print("Seeding complete.")


if __name__ == "__main__":
    main()

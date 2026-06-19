#!/usr/bin/env python3
"""Seed Pulse with metric series, data points, dashboards, and alerts."""

import json
import math
import random
import sys
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

BASE = f"http://localhost:{sys.argv[1]}" if len(sys.argv) > 1 else "http://localhost:9804"


def api(method, path, data=None):
    url = f"{BASE}{path}"
    req = urllib.request.Request(url, method=method)
    if data:
        req.add_header("Content-Type", "application/json")
        req.data = json.dumps(data).encode()
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())


# ── Generate data points ─────────────────────────────────────
# 50 points over 24h (one every ~28.8 minutes)
end_time = datetime.utcnow().replace(second=0, microsecond=0)
start_time = end_time - timedelta(hours=24)
interval = timedelta(hours=24) / 49  # 50 points → 49 gaps

# Seed for reproducibility
random.seed(42)


def generate_points(generator_fn):
    """Generate 50 data points with timestamps and values from a generator."""
    points = []
    for i in range(50):
        t = start_time + interval * i
        points.append({
            "timestamp": t.replace(tzinfo=timezone.utc).isoformat().replace("+00:00", "Z"),
            "value": round(generator_fn(i), 4)
        })
    return points


# ── Series 1: CPU Usage (0–100%, sinusoidal + noise) ─────────
def cpu_generator(i):
    # Base sine wave with 12h period, amplitude 40, center 50
    base = 50 + 35 * math.sin(2 * math.pi * i / 25)  # 25 = ~half the points for 12h period over 50 pts
    noise = random.uniform(-8, 8)
    # Add a small upward trend
    trend = i * 0.15
    return max(0.0, min(100.0, base + noise + trend))


cpu_points = generate_points(cpu_generator)

# ── Series 2: Request Latency (ms, log-normal-ish) ──────────
def latency_generator(i):
    # Base around 45ms with periodic spikes
    hour_angle = 2 * math.pi * i / 50  # one full cycle over all points
    base = 45 + 15 * math.sin(hour_angle * 2)  # two cycles
    # Spikes every ~10 points
    spike = 80 if i % 10 == 0 else 0
    noise = random.uniform(-10, 10)
    return max(1.0, base + spike + noise)


latency_points = generate_points(latency_generator)

# ── Series 3: Error Rate (%, low base with occasional spikes) ─
def error_generator(i):
    # Base around 0.5%, spikes to 5-8% every ~15 points
    base = 0.5
    if i % 15 == 0:
        base = random.uniform(5.0, 8.0)
    elif i % 7 == 0:
        base = random.uniform(1.5, 3.0)
    noise = random.uniform(-0.2, 0.2)
    return max(0.0, base + noise)


error_points = generate_points(error_generator)


# ── Create series ────────────────────────────────────────────
status, s_cpu = api("POST", "/series", {
    "name": "CPU Usage",
    "description": "Server CPU utilization percentage across all cores",
    "unit": "%"
})
status, s_latency = api("POST", "/series", {
    "name": "Request Latency",
    "description": "P95 HTTP request latency in milliseconds",
    "unit": "ms"
})
status, s_error = api("POST", "/series", {
    "name": "Error Rate",
    "description": "Percentage of requests returning 5xx status codes",
    "unit": "%"
})

sid_cpu = s_cpu["id"]
sid_latency = s_latency["id"]
sid_error = s_error["id"]

print(f"✅ Series: CPU Usage ({sid_cpu}), Request Latency ({sid_latency}), Error Rate ({sid_error})")

# ── Bulk-insert data points ──────────────────────────────────
for s_name, s_id, pts in [
    ("CPU Usage", sid_cpu, cpu_points),
    ("Request Latency", sid_latency, latency_points),
    ("Error Rate", sid_error, error_points)
]:
    status, result = api("POST", f"/series/{s_id}/data", {"points": pts})
    print(f"  ✅ {s_name}: {result['inserted']} data points inserted")


# ── Create dashboards ────────────────────────────────────────
# Dashboard 1: "Infrastructure Overview"
status, d1 = api("POST", "/dashboards", {
    "name": "Infrastructure Overview",
    "description": "High-level system health — CPU, latency, and error rates over time.",
    "panels": [
        {
            "title": "CPU Usage (avg)",
            "type": "line",
            "series_id": sid_cpu,
            "aggregate": "avg",
            "width": 6
        },
        {
            "title": "CPU Usage (max)",
            "type": "gauge",
            "series_id": sid_cpu,
            "aggregate": "max",
            "width": 3
        },
        {
            "title": "Error Rate",
            "type": "line",
            "series_id": sid_error,
            "aggregate": "avg",
            "width": 6
        }
    ]
})
print(f"✅ Dashboard: {d1['name']} ({d1['id']}) — {len(d1['panels'])} panels")

# Dashboard 2: "Latency Deep Dive"
status, d2 = api("POST", "/dashboards", {
    "name": "Latency Deep Dive",
    "description": "Request latency analysis — average, peak, and correlation with errors.",
    "panels": [
        {
            "title": "Request Latency (avg)",
            "type": "line",
            "series_id": sid_latency,
            "aggregate": "avg",
            "width": 6
        },
        {
            "title": "Request Latency (max spike)",
            "type": "stat",
            "series_id": sid_latency,
            "aggregate": "max",
            "width": 3
        },
        {
            "title": "Total Error Count",
            "type": "stat",
            "series_id": sid_error,
            "aggregate": "sum",
            "width": 3
        }
    ]
})
print(f"✅ Dashboard: {d2['name']} ({d2['id']}) — {len(d2['panels'])} panels")

# ── Create alerts ────────────────────────────────────────────
# Alert 1: CPU > 85% (avg)
status, a1 = api("POST", "/alerts", {
    "name": "High CPU Alert",
    "series_id": sid_cpu,
    "condition": "gt",
    "threshold": 85.0,
    "aggregate": "avg",
    "description": "Fire when average CPU usage exceeds 85%"
})
print(f"  ✅ Alert: {a1['name']} ({a1['id']}) — avg({sid_cpu}) > {a1['threshold']}%")

# Alert 2: Latency > 100ms (avg)
status, a2 = api("POST", "/alerts", {
    "name": "High Latency Alert",
    "series_id": sid_latency,
    "condition": "gt",
    "threshold": 100.0,
    "aggregate": "avg",
    "description": "Fire when average P95 latency exceeds 100ms"
})
print(f"  ✅ Alert: {a2['name']} ({a2['id']}) — avg({sid_latency}) > {a2['threshold']}ms")

# Alert 3: Error Rate > 2% (avg)
status, a3 = api("POST", "/alerts", {
    "name": "Elevated Error Rate Alert",
    "series_id": sid_error,
    "condition": "gt",
    "threshold": 2.0,
    "aggregate": "avg",
    "description": "Fire when average error rate exceeds 2%"
})
print(f"  ✅ Alert: {a3['name']} ({a3['id']}) — avg({sid_error}) > {a3['threshold']}%")

# ── Verify: evaluate alerts ──────────────────────────────────
yesterday = (datetime.utcnow() - timedelta(hours=24)).isoformat() + "Z"
for aid, a_name in [(a1["id"], a1["name"]), (a2["id"], a2["name"]), (a3["id"], a3["name"])]:
    status, ev = api("GET", f"/alerts/{aid}/evaluate?since={yesterday}")
    triggered = "🔥 TRIGGERED" if ev.get("triggered") else "✅ ok"
    print(f"  🔍 {a_name}: {triggered} — {ev.get('aggregate')}={ev.get('value')} vs threshold={ev.get('threshold')} ({ev.get('point_count')} points)")


print(f"\n✅ Seeded: 3 series, 150 data points, 2 dashboards, 3 alerts")

#!/usr/bin/env python3
"""
metrics-engine: Time-Series Analytics API
Listens on :9804
Endpoints:
  POST   /metrics              bulk insert points
  GET    /metrics/NAME         range query
  GET    /metrics/NAME/aggregate  aggregate (avg|min|max|sum) with interval
  GET    /metrics/NAME/latest  most recent point
  GET    /metrics              multi-series query by names
  GET    /dashboards           list dashboards
  POST   /dashboards           create dashboard
  GET    /dashboards/ID        get dashboard
  PUT    /dashboards/ID        update dashboard
  DELETE /dashboards/ID        delete dashboard
  GET    /health               health check
  GET    /openapi.json         OpenAPI spec
"""
from flask import Flask, request, jsonify, g
import logging
import sys
import time
import os
import json
from datetime import datetime, timezone

app = Flask(__name__)

# ── In-memory stores ───────────────────────────────────────────
metrics_store = {}       # metric_name -> list[{name, value, timestamp}]
dashboards = {}
_next_dashboard_id = 1

# ── Logging ────────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    stream=sys.stdout,
)
logger = logging.getLogger("metrics-engine")


@app.before_request
def log_request():
    g.start_time = time.time()
    logger.info("--> %s %s", request.method, request.path)


@app.after_request
def log_response(response):
    elapsed = time.time() - g.start_time
    logger.info(
        "<-- %s %s %d %.4fs",
        request.method,
        request.path,
        response.status_code,
        elapsed,
    )
    return response


# ── Health / Spec ──────────────────────────────────────────────

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/openapi.json", methods=["GET"])
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))


# ── Helpers ────────────────────────────────────────────────────

def _iso_to_ts(iso_str):
    """Parse ISO 8601 string to Unix timestamp (float). Returns None on failure."""
    if not iso_str:
        return None
    try:
        dt = datetime.fromisoformat(iso_str)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt.timestamp()
    except (ValueError, TypeError):
        return None


def _ts_to_iso(ts):
    """Convert Unix timestamp float to ISO 8601 string."""
    return datetime.fromtimestamp(ts, tz=timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _valid_point(p):
    """Validate a point dict has required keys."""
    return isinstance(p, dict) and "name" in p and "value" in p and "timestamp" in p


def _points_in_range(name, from_ts, to_ts):
    """Return points for a metric falling within [from_ts, to_ts] (inclusive, None = unbounded)."""
    points = metrics_store.get(name, [])
    if from_ts is not None:
        points = [p for p in points if p["timestamp"] >= from_ts]
    if to_ts is not None:
        points = [p for p in points if p["timestamp"] <= to_ts]
    return points


def _parse_interval(interval_str):
    """Parse interval like '1h', '30m', '3600s', '2d' -> seconds. Returns -1 on invalid."""
    if not interval_str:
        return 3600
    s = str(interval_str).strip().lower()
    try:
        if s.endswith("h"):
            return int(s[:-1]) * 3600
        elif s.endswith("m"):
            return int(s[:-1]) * 60
        elif s.endswith("s"):
            return int(s[:-1])
        elif s.endswith("d"):
            return int(s[:-1]) * 86400
        else:
            return int(s)
    except (ValueError, TypeError):
        return -1


# ═══════════════════════════════════════════════════════════════
#  METRICS — Bulk Insert
# ═══════════════════════════════════════════════════════════════

@app.route("/metrics", methods=["POST"])
def bulk_insert_metrics():
    """
    POST /metrics
    Body: { "points": [{ "name": "cpu", "value": 42.5, "timestamp": "2024-01-01T00:00:00Z" }, ...] }
    Returns 201 with inserted count and per-point errors.
    """
    data = request.get_json(force=True, silent=True)
    if not data or "points" not in data or not isinstance(data["points"], list):
        return jsonify({"error": "body must contain a points array"}), 400
    if not data["points"]:
        return jsonify({"error": "points array must not be empty"}), 400

    inserted = 0
    errors = []
    for i, point in enumerate(data["points"]):
        if not _valid_point(point):
            errors.append({"index": i, "point": point, "error": "each point must have name, value, timestamp"})
            continue
        name = point["name"]
        ts_raw = point["timestamp"]
        # Accept ISO string or numeric timestamp
        if isinstance(ts_raw, str):
            ts = _iso_to_ts(ts_raw)
            if ts is None:
                errors.append({"index": i, "point": point, "error": "invalid timestamp format (expected ISO 8601)"})
                continue
        else:
            try:
                ts = float(ts_raw)
            except (ValueError, TypeError):
                errors.append({"index": i, "point": point, "error": "timestamp must be ISO string or number"})
                continue

        entry = {
            "name": name,
            "value": float(point["value"]),
            "timestamp": ts,
        }
        if name not in metrics_store:
            metrics_store[name] = []
        metrics_store[name].append(entry)
        inserted += 1

    return jsonify({
        "inserted": inserted,
        "errors": errors,
        "total_errors": len(errors),
    }), 201


# ═══════════════════════════════════════════════════════════════
#  METRICS — Range Query  /metrics/NAME?from=ISO&to=ISO
# ═══════════════════════════════════════════════════════════════

@app.route("/metrics/<name>", methods=["GET"])
def query_metrics_range(name):
    """Return data points for a metric within the optional from/to time window."""
    from_ts = _iso_to_ts(request.args.get("from"))
    to_ts = _iso_to_ts(request.args.get("to"))

    if name not in metrics_store:
        return jsonify({"metric": name, "count": 0, "data": []})

    points = _points_in_range(name, from_ts, to_ts)
    points.sort(key=lambda p: p["timestamp"])

    return jsonify({
        "metric": name,
        "count": len(points),
        "from": request.args.get("from"),
        "to": request.args.get("to"),
        "data": [
            {
                "name": p["name"],
                "value": p["value"],
                "timestamp": _ts_to_iso(p["timestamp"]),
            }
            for p in points
        ],
    })


# ═══════════════════════════════════════════════════════════════
#  METRICS — Aggregate  /metrics/NAME/aggregate?fn=avg&interval=1h
# ═══════════════════════════════════════════════════════════════

@app.route("/metrics/<name>/aggregate", methods=["GET"])
def aggregate_metrics(name):
    """
    Aggregate a metric over time buckets.
    fn: avg|min|max|sum   (default: avg)
    interval: 1h, 30m, 3600s, 2d  (default: 1h)
    from / to: optional ISO range filter.
    """
    fn = request.args.get("fn", "avg").lower()
    interval = request.args.get("interval", "1h")
    from_ts = _iso_to_ts(request.args.get("from"))
    to_ts = _iso_to_ts(request.args.get("to"))

    if fn not in ("avg", "min", "max", "sum"):
        return jsonify({"error": "fn must be one of: avg, min, max, sum"}), 400

    interval_seconds = _parse_interval(interval)
    if interval_seconds <= 0:
        return jsonify({"error": "invalid interval format (e.g. 1h, 30m, 3600s, 2d)"}), 400

    if name not in metrics_store:
        return jsonify({"metric": name, "fn": fn, "interval": interval, "data": []})

    points = _points_in_range(name, from_ts, to_ts)
    if not points:
        return jsonify({"metric": name, "fn": fn, "interval": interval, "data": []})

    points.sort(key=lambda p: p["timestamp"])

    # Bucket by interval
    buckets = {}
    for p in points:
        bucket_key = int(p["timestamp"] // interval_seconds) * interval_seconds
        buckets.setdefault(bucket_key, []).append(p["value"])

    result = []
    for bucket_ts in sorted(buckets.keys()):
        values = buckets[bucket_ts]
        if fn == "avg":
            agg_value = sum(values) / len(values)
        elif fn == "min":
            agg_value = min(values)
        elif fn == "max":
            agg_value = max(values)
        elif fn == "sum":
            agg_value = sum(values)
        else:
            agg_value = 0.0

        result.append({
            "timestamp": _ts_to_iso(bucket_ts),
            "value": round(agg_value, 4),
            "count": len(values),
        })

    return jsonify({
        "metric": name,
        "fn": fn,
        "interval": interval,
        "data": result,
    })


# ═══════════════════════════════════════════════════════════════
#  METRICS — Latest  /metrics/NAME/latest
# ═══════════════════════════════════════════════════════════════

@app.route("/metrics/<name>/latest", methods=["GET"])
def latest_metric(name):
    """Return the most recent data point for a metric."""
    if name not in metrics_store or not metrics_store[name]:
        return jsonify({"metric": name, "data": None})

    points = metrics_store[name]
    latest = max(points, key=lambda p: p["timestamp"])
    return jsonify({
        "metric": name,
        "data": {
            "name": latest["name"],
            "value": latest["value"],
            "timestamp": _ts_to_iso(latest["timestamp"]),
        },
    })


# ═══════════════════════════════════════════════════════════════
#  METRICS — Multi-series  /metrics?names=cpu,memory&from=...&to=...
# ═══════════════════════════════════════════════════════════════

@app.route("/metrics", methods=["GET"])
def list_metrics():
    """Query multiple metrics by comma-separated names with optional time range."""
    names_str = request.args.get("names", "")
    if not names_str:
        return jsonify({"error": "names parameter is required (comma-separated)"}), 400

    names = [n.strip() for n in names_str.split(",") if n.strip()]
    from_ts = _iso_to_ts(request.args.get("from"))
    to_ts = _iso_to_ts(request.args.get("to"))

    result = {}
    for name in names:
        if name not in metrics_store:
            result[name] = {"data": []}
            continue
        points = _points_in_range(name, from_ts, to_ts)
        points.sort(key=lambda p: p["timestamp"])
        result[name] = {
            "data": [
                {"name": p["name"], "value": p["value"], "timestamp": _ts_to_iso(p["timestamp"])}
                for p in points
            ],
        }

    return jsonify({
        "metrics": result,
        "from": request.args.get("from"),
        "to": request.args.get("to"),
    })


# ═══════════════════════════════════════════════════════════════
#  DASHBOARDS CRUD
# ═══════════════════════════════════════════════════════════════

@app.route("/dashboards", methods=["GET"])
def list_dashboards():
    return jsonify({"data": sorted(dashboards.values(), key=lambda d: d["id"])})


@app.route("/dashboards", methods=["POST"])
def create_dashboard():
    data = request.get_json(force=True, silent=True)
    if not data or "name" not in data:
        return jsonify({"error": "name is required"}), 400
    global _next_dashboard_id
    did = _next_dashboard_id
    _next_dashboard_id += 1
    dashboard = {
        "id": did,
        "name": data["name"],
        "description": data.get("description", ""),
        "panels": data.get("panels", []),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    dashboards[did] = dashboard
    return jsonify(dashboard), 201


@app.route("/dashboards/<int:dashboard_id>", methods=["GET"])
def get_dashboard(dashboard_id):
    d = dashboards.get(dashboard_id)
    if d is None:
        return jsonify({"error": "dashboard not found"}), 404
    return jsonify(d)


@app.route("/dashboards/<int:dashboard_id>", methods=["PUT"])
def update_dashboard(dashboard_id):
    d = dashboards.get(dashboard_id)
    if d is None:
        return jsonify({"error": "dashboard not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("name", "description", "panels"):
        if field in data:
            d[field] = data[field]
    d["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(d)


@app.route("/dashboards/<int:dashboard_id>", methods=["DELETE"])
def delete_dashboard(dashboard_id):
    d = dashboards.pop(dashboard_id, None)
    if d is None:
        return jsonify({"error": "dashboard not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
#  MAIN
# ═══════════════════════════════════════════════════════════════

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9804))
    logger.info("Starting metrics-engine on :%d", port)
    app.run(host="0.0.0.0", port=port, debug=False)

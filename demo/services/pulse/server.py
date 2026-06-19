#!/usr/bin/env python3
"""Pulse — Metrics Dashboard Demo Service (AC-D04)."""

import json
import os
import uuid
from datetime import datetime
from flask import Flask, jsonify, request

app = Flask(__name__)

# ── In-memory store ──────────────────────────────────────────
series = {}
dashboards = {}
alerts = {}
next_ids = {"series": 1, "dashboards": 1, "alerts": 1}

# ── Helpers ───────────────────────────────────────────────────

def _parse_iso(ts):
    """Parse an ISO 8601 timestamp string to a float epoch for comparisons."""
    try:
        return datetime.fromisoformat(ts.replace("Z", "+00:00")).timestamp()
    except (ValueError, AttributeError):
        return 0.0


def _filter_range(points, since, until):
    """Filter data points by time range [since, until] (inclusive)."""
    if since is not None:
        since_epoch = _parse_iso(since)
        points = [p for p in points if _parse_iso(p["timestamp"]) >= since_epoch]
    if until is not None:
        until_epoch = _parse_iso(until)
        points = [p for p in points if _parse_iso(p["timestamp"]) <= until_epoch]
    return points


def _aggregate(points, agg_type):
    """Compute aggregation over a list of data-point dicts."""
    if not points:
        return None
    values = [p["value"] for p in points]
    if agg_type == "avg":
        return sum(values) / len(values)
    elif agg_type == "min":
        return min(values)
    elif agg_type == "max":
        return max(values)
    elif agg_type == "sum":
        return sum(values)
    return None


# ── Health & Spec ────────────────────────────────────────────

@app.route("/health")
def health():
    total_points = sum(len(s["data_points"]) for s in series.values())
    return jsonify({
        "status": "healthy",
        "series_count": len(series),
        "data_points_count": total_points,
        "dashboards_count": len(dashboards),
        "alerts_count": len(alerts)
    })


@app.route("/openapi.json")
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))


# ── Series ───────────────────────────────────────────────────

@app.route("/series", methods=["GET", "POST"])
def series_handler():
    if request.method == "GET":
        q = request.args.get("q")
        result = list(series.values())
        if q:
            ql = q.lower()
            result = [s for s in result if ql in s["name"].lower() or ql in s.get("description", "").lower()]
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 20))
        start = (page - 1) * limit
        return jsonify(result[start:start + limit])
    else:
        data = request.get_json(force=True)
        sid = str(next_ids["series"]); next_ids["series"] += 1
        s = {
            "id": sid,
            "name": data["name"],
            "description": data.get("description", ""),
            "unit": data.get("unit", ""),
            "data_points": [],
            "created_at": datetime.utcnow().isoformat() + "Z",
            "updated_at": datetime.utcnow().isoformat() + "Z"
        }
        series[sid] = s
        return jsonify(s), 201


@app.route("/series/<series_id>", methods=["GET", "PATCH", "DELETE"])
def single_series(series_id):
    if series_id not in series:
        return jsonify({"error": "series not found"}), 404
    if request.method == "GET":
        return jsonify(series[series_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        for k in ("name", "description", "unit"):
            if k in data:
                series[series_id][k] = data[k]
        series[series_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(series[series_id])
    else:  # DELETE
        del series[series_id]
        return jsonify({"deleted": True})


# ── Data Points ──────────────────────────────────────────────

@app.route("/series/<series_id>/data", methods=["GET", "POST"])
def data_points_handler(series_id):
    if series_id not in series:
        return jsonify({"error": "series not found"}), 404

    if request.method == "GET":
        points = list(series[series_id]["data_points"])

        # Time-window filters
        since = request.args.get("since")
        until = request.args.get("until")
        points = _filter_range(points, since, until)

        # Sort by timestamp ascending
        points.sort(key=lambda p: p["timestamp"])

        # Aggregation
        agg = request.args.get("aggregate")
        if agg:
            if agg not in ("avg", "min", "max", "sum"):
                return jsonify({"error": "aggregate must be one of: avg, min, max, sum"}), 400
            agg_value = _aggregate(points, agg)
            return jsonify({"series_id": series_id, "aggregate": agg, "value": agg_value, "count": len(points)})

        # Pagination for raw points
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 100))
        start = (page - 1) * limit
        return jsonify(points[start:start + limit])

    else:  # POST — bulk insert
        data = request.get_json(force=True)
        if isinstance(data, list):
            points = data
        elif isinstance(data, dict):
            points = data.get("points", data)
        else:
            return jsonify({"error": "expected an array of data points"}), 400
        if isinstance(points, dict):
            points = [points]
        if not isinstance(points, list):
            return jsonify({"error": "expected an array of data points"}), 400

        added = 0
        for pt in points:
            ts = pt.get("timestamp") or datetime.utcnow().isoformat() + "Z"
            dp = {
                "timestamp": ts,
                "value": float(pt["value"])
            }
            series[series_id]["data_points"].append(dp)
            added += 1

        series[series_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify({"series_id": series_id, "inserted": added}), 201


@app.route("/series/<series_id>/data/<int:dp_index>", methods=["GET", "DELETE", "PATCH"])
def single_data_point(series_id, dp_index):
    if series_id not in series:
        return jsonify({"error": "series not found"}), 404
    dps = series[series_id]["data_points"]
    if dp_index < 0 or dp_index >= len(dps):
        return jsonify({"error": "data point index out of range"}), 404

    if request.method == "GET":
        return jsonify({"index": dp_index, **dps[dp_index]})
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        if "value" in data:
            dps[dp_index]["value"] = float(data["value"])
        if "timestamp" in data:
            dps[dp_index]["timestamp"] = data["timestamp"]
        series[series_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify({"index": dp_index, **dps[dp_index]})
    else:  # DELETE
        removed = dps.pop(dp_index)
        series[series_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify({"deleted": True, "index": dp_index, "value": removed["value"]})


# ── Dashboards ───────────────────────────────────────────────

@app.route("/dashboards", methods=["GET", "POST"])
def dashboards_handler():
    if request.method == "GET":
        result = list(dashboards.values())
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 20))
        start = (page - 1) * limit
        return jsonify(result[start:start + limit])
    else:
        data = request.get_json(force=True)
        did = str(next_ids["dashboards"]); next_ids["dashboards"] += 1
        dash = {
            "id": did,
            "name": data["name"],
            "description": data.get("description", ""),
            "panels": data.get("panels", []),
            "created_at": datetime.utcnow().isoformat() + "Z",
            "updated_at": datetime.utcnow().isoformat() + "Z"
        }
        dashboards[did] = dash
        return jsonify(dash), 201


@app.route("/dashboards/<dashboard_id>", methods=["GET", "PATCH", "DELETE"])
def single_dashboard(dashboard_id):
    if dashboard_id not in dashboards:
        return jsonify({"error": "dashboard not found"}), 404
    if request.method == "GET":
        return jsonify(dashboards[dashboard_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        for k in ("name", "description"):
            if k in data:
                dashboards[dashboard_id][k] = data[k]
        if "panels" in data:
            dashboards[dashboard_id]["panels"] = data["panels"]
        dashboards[dashboard_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(dashboards[dashboard_id])
    else:  # DELETE
        del dashboards[dashboard_id]
        return jsonify({"deleted": True})


# ── Alerts ───────────────────────────────────────────────────

@app.route("/alerts", methods=["GET", "POST"])
def alerts_handler():
    if request.method == "GET":
        result = list(alerts.values())
        series_filter = request.args.get("series_id")
        if series_filter:
            result = [a for a in result if a["series_id"] == series_filter]
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 20))
        start = (page - 1) * limit
        return jsonify(result[start:start + limit])
    else:
        data = request.get_json(force=True)
        aid = str(next_ids["alerts"]); next_ids["alerts"] += 1
        alert = {
            "id": aid,
            "name": data["name"],
            "series_id": data["series_id"],
            "condition": data["condition"],   # "gt", "lt", "gte", "lte"
            "threshold": float(data["threshold"]),
            "aggregate": data.get("aggregate", "avg"),  # avg, max, min
            "description": data.get("description", ""),
            "created_at": datetime.utcnow().isoformat() + "Z",
            "updated_at": datetime.utcnow().isoformat() + "Z"
        }
        alerts[aid] = alert
        return jsonify(alert), 201


@app.route("/alerts/<alert_id>", methods=["GET", "PATCH", "DELETE"])
def single_alert(alert_id):
    if alert_id not in alerts:
        return jsonify({"error": "alert not found"}), 404
    if request.method == "GET":
        return jsonify(alerts[alert_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        for k in ("name", "description", "condition", "threshold", "aggregate", "series_id"):
            if k in data:
                alerts[alert_id][k] = data[k]
        alerts[alert_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(alerts[alert_id])
    else:  # DELETE
        del alerts[alert_id]
        return jsonify({"deleted": True})


# ── Alert evaluation ─────────────────────────────────────────

@app.route("/alerts/<alert_id>/evaluate")
def evaluate_alert(alert_id):
    if alert_id not in alerts:
        return jsonify({"error": "alert not found"}), 404
    alert = alerts[alert_id]
    sid = alert["series_id"]
    if sid not in series:
        return jsonify({"error": "series not found"}), 404

    points = list(series[sid]["data_points"])
    since = request.args.get("since")
    until = request.args.get("until")
    points = _filter_range(points, since, until)

    agg_type = alert.get("aggregate", "avg")
    agg_value = _aggregate(points, agg_type)

    condition = alert["condition"]
    threshold = alert["threshold"]
    triggered = False
    if agg_value is not None:
        if condition == "gt":
            triggered = agg_value > threshold
        elif condition == "lt":
            triggered = agg_value < threshold
        elif condition == "gte":
            triggered = agg_value >= threshold
        elif condition == "lte":
            triggered = agg_value <= threshold

    return jsonify({
        "alert_id": alert_id,
        "alert_name": alert["name"],
        "aggregate": agg_type,
        "value": agg_value,
        "condition": condition,
        "threshold": threshold,
        "triggered": triggered,
        "point_count": len(points)
    })


# ── Main ─────────────────────────────────────────────────────

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9804))
    print(f"Pulse starting on :{port}")
    app.run(host="0.0.0.0", port=port, debug=False)

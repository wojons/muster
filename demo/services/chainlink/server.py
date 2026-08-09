#!/usr/bin/env python3
"""ChainLink — Workflow Automation Demo Service (AC-D05)."""

import json
import os
import uuid
import time
import threading
import urllib.request
from datetime import datetime
from flask import Flask, jsonify, request

app = Flask(__name__)


def _require(data, *fields):
    """Return a (response, status) tuple if any required field is missing or
    empty, else return None. Use as: ``err = _require(data, "name"); if err:
    return err``."""
    for f in fields:
        if f not in data or not data[f]:
            return jsonify({"error": f"{f} is required"}), 400
    return None


# ── In-memory store ──────────────────────────────────────────
workflows = {}
steps = {}
executions = {}
webhooks = {}
next_ids = {"workflows": 1, "steps": 1, "executions": 1, "webhooks": 1}

# ── Health & Spec ────────────────────────────────────────────

@app.route("/health")
def health():
    return jsonify({
        "status": "healthy",
        "workflows_count": len(workflows),
        "steps_count": len(steps),
        "executions_count": len(executions),
        "webhooks_count": len(webhooks)
    })

@app.route("/openapi.json")
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))

# ── Workflows ─────────────────────────────────────────────────

@app.route("/workflows", methods=["GET", "POST"])
def workflows_handler():
    if request.method == "GET":
        q = request.args.get("q")
        result = list(workflows.values())
        if q:
            result = [w for w in result if q.lower() in w["name"].lower() or q.lower() in w.get("description", "").lower()]
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 20))
        start = (page - 1) * limit
        return jsonify(result[start:start + limit])
    else:
        data = request.get_json(force=True)
        err = _require(data, "name")
        if err:
            return err
        wid = str(next_ids["workflows"]); next_ids["workflows"] += 1
        workflow = {
            "id": wid, "name": data["name"], "description": data.get("description", ""),
            "status": data.get("status", "active"),
            "created_at": datetime.utcnow().isoformat() + "Z",
            "updated_at": datetime.utcnow().isoformat() + "Z"
        }
        workflows[wid] = workflow
        return jsonify(workflow), 201


@app.route("/workflows/<workflow_id>", methods=["GET", "PATCH", "DELETE"])
def workflow_handler(workflow_id):
    if workflow_id not in workflows:
        return jsonify({"error": "workflow not found"}), 404
    if request.method == "GET":
        return jsonify(workflows[workflow_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        for k in ("name", "description", "status"):
            if k in data:
                workflows[workflow_id][k] = data[k]
        workflows[workflow_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(workflows[workflow_id])
    else:  # DELETE
        # Cascade delete steps
        wf_steps = [s for s in steps.values() if s["workflow_id"] == workflow_id]
        for s in wf_steps:
            del steps[s["id"]]
        # Cascade delete executions
        wf_execs = [e for e in executions.values() if e["workflow_id"] == workflow_id]
        for e in wf_execs:
            del executions[e["id"]]
        del workflows[workflow_id]
        return jsonify({"deleted": True})


# ── Steps (nested under workflows) ────────────────────────────

@app.route("/workflows/<workflow_id>/steps", methods=["GET", "POST"])
def workflow_steps(workflow_id):
    if workflow_id not in workflows:
        return jsonify({"error": "workflow not found"}), 404
    if request.method == "GET":
        result = sorted(
            [s for s in steps.values() if s["workflow_id"] == workflow_id],
            key=lambda s: s["order"]
        )
        return jsonify(result)
    else:
        data = request.get_json(force=True)
        err = _require(data, "name")
        if err:
            return err
        sid = str(next_ids["steps"]); next_ids["steps"] += 1
        step = {
            "id": sid, "workflow_id": workflow_id,
            "name": data["name"], "description": data.get("description", ""),
            "order": data.get("order", len([s for s in steps.values() if s["workflow_id"] == workflow_id]) + 1),
            "depends_on": data.get("depends_on"),
            "retry_count": data.get("retry_count", 3),
            "timeout_seconds": data.get("timeout_seconds", 30),
            "created_at": datetime.utcnow().isoformat() + "Z"
        }
        steps[sid] = step
        return jsonify(step), 201


@app.route("/steps/<step_id>", methods=["GET", "PATCH", "DELETE"])
def step_handler(step_id):
    if step_id not in steps:
        return jsonify({"error": "step not found"}), 404
    if request.method == "GET":
        return jsonify(steps[step_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        for k in ("name", "description", "order", "depends_on", "retry_count", "timeout_seconds"):
            if k in data:
                steps[step_id][k] = data[k]
        return jsonify(steps[step_id])
    else:  # DELETE
        del steps[step_id]
        return jsonify({"deleted": True})


# ── Executions (async with threading) ─────────────────────────

def _execute_workflow(execution_id):
    """Run workflow steps sequentially in a background thread."""
    execution = executions.get(execution_id)
    if not execution:
        return

    workflow_id = execution["workflow_id"]
    execution["status"] = "running"
    execution["started_at"] = datetime.utcnow().isoformat() + "Z"

    # Get ordered steps for this workflow
    wf_steps = sorted(
        [s for s in steps.values() if s["workflow_id"] == workflow_id],
        key=lambda s: s["order"]
    )

    step_results = []
    all_success = True
    failed_step = None

    for step in wf_steps:
        step_start = datetime.utcnow().isoformat() + "Z"
        result = {"step_id": step["id"], "step_name": step["name"], "status": "running", "started_at": step_start}

        # Check dependency
        if step.get("depends_on"):
            dep_result = next((r for r in step_results if r["step_id"] == step["depends_on"]), None)
            if dep_result and dep_result["status"] != "done":
                result.update({
                    "status": "skipped",
                    "error": f"Dependency step {step['depends_on']} did not complete successfully",
                    "completed_at": datetime.utcnow().isoformat() + "Z"
                })
                step_results.append(result)
                all_success = False
                failed_step = step["id"]
                break

        # Execute with retry logic
        max_retries = step.get("retry_count", 3)
        step_success = False
        last_error = None
        attempt = 0

        for attempt in range(max_retries + 1):
            try:
                if attempt > 0:
                    # Brief delay before retry
                    time.sleep(0.3)
                # Simulate work
                time.sleep(0.5)
                step_success = True
                break
            except Exception as e:
                last_error = str(e)

        if step_success:
            result["status"] = "done"
            result["completed_at"] = datetime.utcnow().isoformat() + "Z"
            result["retries"] = attempt
        else:
            result["status"] = "failed"
            result["error"] = last_error or "Step failed after all retries"
            result["retries"] = max_retries
            result["completed_at"] = datetime.utcnow().isoformat() + "Z"
            all_success = False
            failed_step = step["id"]

        step_results.append(result)
        execution["current_step"] = step["id"]

        if not step_success:
            break

    execution["step_results"] = step_results
    execution["status"] = "done" if all_success else "failed"
    execution["completed_at"] = datetime.utcnow().isoformat() + "Z"

    # Fire callbacks
    _fire_callbacks(execution)


def _fire_callbacks(execution):
    """Fire registered webhooks with execution result."""
    payload = json.dumps({
        "event": f"execution.{execution['status']}",
        "execution_id": execution["id"],
        "workflow_id": execution["workflow_id"],
        "status": execution["status"],
        "completed_at": execution.get("completed_at")
    }).encode()

    for wh in webhooks.values():
        if "execution.*" in wh.get("events", []) or f"execution.{execution['status']}" in wh.get("events", []):
            try:
                req = urllib.request.Request(wh["url"], method="POST")
                req.add_header("Content-Type", "application/json")
                if wh.get("secret"):
                    req.add_header("X-Webhook-Secret", wh["secret"])
                # Fire and forget — don't block
                urllib.request.urlopen(req, timeout=5)
            except Exception:
                pass  # Silently ignore callback failures


@app.route("/workflows/<workflow_id>/execute", methods=["POST"])
def execute_workflow(workflow_id):
    if workflow_id not in workflows:
        return jsonify({"error": "workflow not found"}), 404

    wf_steps = [s for s in steps.values() if s["workflow_id"] == workflow_id]
    if not wf_steps:
        return jsonify({"error": "workflow has no steps"}), 400

    data = request.get_json(silent=True) or {}
    eid = str(next_ids["executions"]); next_ids["executions"] += 1

    execution = {
        "id": eid, "workflow_id": workflow_id,
        "status": "pending", "current_step": None,
        "started_at": None, "completed_at": None,
        "step_results": [],
        "callback_url": data.get("callback_url"),
        "payload": data.get("payload", {}),
        "created_at": datetime.utcnow().isoformat() + "Z"
    }
    executions[eid] = execution

    # Spawn background thread for async execution
    thread = threading.Thread(target=_execute_workflow, args=(eid,), daemon=True)
    thread.start()

    return jsonify({"execution_id": eid, "status": "pending"}), 202


@app.route("/executions", methods=["GET"])
def executions_handler():
    result = list(executions.values())
    workflow_id = request.args.get("workflow_id")
    if workflow_id:
        result = [e for e in result if e["workflow_id"] == workflow_id]
    status = request.args.get("status")
    if status:
        result = [e for e in result if e["status"] == status]
    page = int(request.args.get("page", 1))
    limit = int(request.args.get("limit", 20))
    start = (page - 1) * limit
    return jsonify(result[start:start + limit])


@app.route("/executions/<execution_id>", methods=["GET"])
def execution_handler(execution_id):
    if execution_id not in executions:
        return jsonify({"error": "execution not found"}), 404
    return jsonify(executions[execution_id])


@app.route("/executions/<execution_id>", methods=["DELETE"])
def delete_execution(execution_id):
    if execution_id not in executions:
        return jsonify({"error": "execution not found"}), 404
    del executions[execution_id]
    return jsonify({"deleted": True})


# ── Webhooks ─────────────────────────────────────────────────

@app.route("/webhooks", methods=["GET", "POST"])
def webhooks_handler():
    if request.method == "GET":
        return jsonify(list(webhooks.values()))
    else:
        data = request.get_json(force=True)
        err = _require(data, "name", "url")
        if err:
            return err
        wid = str(next_ids["webhooks"]); next_ids["webhooks"] += 1
        webhook = {
            "id": wid,
            "name": data["name"],
            "url": data["url"],
            "events": data.get("events", ["execution.*"]),
            "secret": data.get("secret", ""),
            "active": data.get("active", True),
            "created_at": datetime.utcnow().isoformat() + "Z"
        }
        webhooks[wid] = webhook
        return jsonify(webhook), 201


@app.route("/webhooks/<webhook_id>", methods=["GET", "DELETE"])
def webhook_handler(webhook_id):
    if webhook_id not in webhooks:
        return jsonify({"error": "webhook not found"}), 404
    if request.method == "GET":
        return jsonify(webhooks[webhook_id])
    else:  # DELETE
        del webhooks[webhook_id]
        return jsonify({"deleted": True})


# ── Main ─────────────────────────────────────────────────────

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9805))
    print(f"ChainLink starting on :{port}")
    app.run(host="0.0.0.0", port=port, debug=False)

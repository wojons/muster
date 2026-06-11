#!/usr/bin/env python3
"""
orchestrator: Workflow / Webhook API
Listens on :9805
Resources: workflows, executions, triggers (webhook)
Key patterns: state-machine execution, progress polling, webhook-triggered runs
"""
from flask import Flask, request, jsonify, g
import logging
import sys
import time
import os
import json
import uuid

app = Flask(__name__)

# ── In-memory stores ───────────────────────────────────────────
workflows = {}
executions = {}
triggers = {}          # name -> {name, workflow_id, created_at}
_next_ids = {"workflows": 1, "executions": 1, "triggers": 1}

# ── Logging ────────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    stream=sys.stdout,
)
logger = logging.getLogger("orchestrator")


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

def _get_body():
    data = request.get_json(force=True, silent=True)
    if data is None:
        return None
    return data


def _now():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def _build_execution_steps(workflow):
    """Create step-tracking entries from a workflow's step definitions."""
    return [
        {
            "step_id": s["id"],
            "name": s["name"],
            "type": s.get("type", "task"),
            "status": "pending",
            "started_at": None,
            "completed_at": None,
            "output": None,
            "error": None,
        }
        for s in workflow.get("steps", [])
    ]


def _advance_execution(exec_id):
    """Simple state-machine: advance one step at a time."""
    ex = executions.get(exec_id)
    if ex is None:
        return
    if ex["status"] != "running":
        return

    steps = ex["steps"]
    total = len(steps)
    if total == 0:
        ex["status"] = "completed"
        ex["progress"] = 100
        ex["current_step"] = None
        ex["completed_at"] = _now()
        return

    # Find the first pending step
    for idx, step in enumerate(steps):
        if step["status"] == "pending":
            step["status"] = "running"
            step["started_at"] = _now()
            ex["current_step"] = step["step_id"]
            ex["progress"] = int((idx / total) * 100)
            # Simulate async completion after a tiny delay — but in-memory,
            # we complete immediately for test predictability.
            step["status"] = "completed"
            step["completed_at"] = _now()
            step["output"] = {"result": f"step-{step['step_id']}-ok"}
            ex["progress"] = int(((idx + 1) / total) * 100)
            continue  # process ALL pending steps, not just one

    # Check if all done
    if all(s["status"] == "completed" for s in steps):
        ex["status"] = "completed"
        ex["progress"] = 100
        ex["current_step"] = None
        ex["completed_at"] = _now()


# ═══════════════════════════════════════════════════════════════
#  WORKFLOWS
# ═══════════════════════════════════════════════════════════════

@app.route("/workflows", methods=["GET"])
def list_workflows():
    result = list(workflows.values())
    # Filters
    name = request.args.get("name")
    if name:
        result = [w for w in result if name.lower() in w.get("name", "").lower()]
    status = request.args.get("status")
    if status:
        result = [w for w in result if w.get("status") == status]
    # Sort by id asc
    result.sort(key=lambda w: w["id"])
    return jsonify({
        "data": result,
        "total": len(result),
    })


@app.route("/workflows", methods=["POST"])
def create_workflow():
    data = _get_body()
    if not data or "name" not in data:
        return jsonify({"error": "name is required"}), 400
    wid = _next_ids["workflows"]
    _next_ids["workflows"] += 1
    workflow = {
        "id": wid,
        "name": data["name"],
        "description": data.get("description", ""),
        "status": data.get("status", "active"),
        "steps": data.get("steps", []),
        "created_at": _now(),
        "updated_at": _now(),
    }
    # Assign step ids if missing
    for idx, step in enumerate(workflow["steps"]):
        if "id" not in step:
            step["id"] = idx + 1
        if "name" not in step:
            step["name"] = f"Step {step['id']}"
        if "type" not in step:
            step["type"] = "task"
        if "order" not in step:
            step["order"] = idx + 1
    workflows[wid] = workflow
    return jsonify(workflow), 201


@app.route("/workflows/<int:workflow_id>", methods=["GET"])
def get_workflow(workflow_id):
    w = workflows.get(workflow_id)
    if w is None:
        return jsonify({"error": "workflow not found"}), 404
    return jsonify(w)


@app.route("/workflows/<int:workflow_id>", methods=["PUT"])
def update_workflow(workflow_id):
    w = workflows.get(workflow_id)
    if w is None:
        return jsonify({"error": "workflow not found"}), 404
    data = _get_body()
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        w["name"] = data["name"]
    if "description" in data:
        w["description"] = data["description"]
    if "status" in data:
        w["status"] = data["status"]
    if "steps" in data:
        w["steps"] = data["steps"]
        for idx, step in enumerate(w["steps"]):
            if "id" not in step:
                step["id"] = idx + 1
            if "name" not in step:
                step["name"] = f"Step {step['id']}"
            if "order" not in step:
                step["order"] = idx + 1
    w["updated_at"] = _now()
    return jsonify(w)


@app.route("/workflows/<int:workflow_id>", methods=["DELETE"])
def delete_workflow(workflow_id):
    w = workflows.pop(workflow_id, None)
    if w is None:
        return jsonify({"error": "workflow not found"}), 404
    # Also remove related executions
    to_remove = [eid for eid, ex in executions.items() if ex["workflow_id"] == workflow_id]
    for eid in to_remove:
        del executions[eid]
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
#  TRIGGERS  (webhook)
# ═══════════════════════════════════════════════════════════════

@app.route("/triggers", methods=["GET"])
def list_triggers():
    return jsonify({
        "data": list(triggers.values()),
        "total": len(triggers),
    })


@app.route("/triggers", methods=["POST"])
def create_trigger():
    data = _get_body()
    if not data or "name" not in data or "workflow_id" not in data:
        return jsonify({"error": "name and workflow_id are required"}), 400
    w = workflows.get(data["workflow_id"])
    if w is None:
        return jsonify({"error": "workflow not found"}), 404
    if data["name"] in triggers:
        return jsonify({"error": "trigger name already exists"}), 409
    tid = _next_ids["triggers"]
    _next_ids["triggers"] += 1
    trigger = {
        "id": tid,
        "name": data["name"],
        "workflow_id": data["workflow_id"],
        "created_at": _now(),
    }
    triggers[data["name"]] = trigger
    return jsonify(trigger), 201


@app.route("/triggers/<trigger_name>", methods=["GET"])
def get_trigger(trigger_name):
    t = triggers.get(trigger_name)
    if t is None:
        return jsonify({"error": "trigger not found"}), 404
    return jsonify(t)


@app.route("/triggers/<trigger_name>", methods=["DELETE"])
def delete_trigger(trigger_name):
    t = triggers.pop(trigger_name, None)
    if t is None:
        return jsonify({"error": "trigger not found"}), 404
    return jsonify({"deleted": True})


@app.route("/triggers/<trigger_name>/webhook", methods=["POST"])
def trigger_webhook(trigger_name):
    """Start a workflow execution via webhook trigger."""
    t = triggers.get(trigger_name)
    if t is None:
        return jsonify({"error": "trigger not found"}), 404
    w = workflows.get(t["workflow_id"])
    if w is None:
        return jsonify({"error": "workflow not found"}), 500

    body = _get_body() or {}

    eid = _next_ids["executions"]
    _next_ids["executions"] += 1

    step_records = _build_execution_steps(w)
    execution = {
        "id": eid,
        "workflow_id": w["id"],
        "workflow_name": w["name"],
        "status": "running",
        "progress": 0,
        "current_step": None,
        "steps": step_records,
        "triggered_by": trigger_name,
        "payload": body,
        "created_at": _now(),
        "started_at": _now(),
        "completed_at": None,
    }
    executions[eid] = execution

    # Advance the state machine (completes first step immediately)
    _advance_execution(eid)

    return jsonify({
        "execution_id": eid,
        "status": execution["status"],
        "progress": execution["progress"],
    }), 202


# ═══════════════════════════════════════════════════════════════
#  EXECUTIONS
# ═══════════════════════════════════════════════════════════════

@app.route("/executions", methods=["GET"])
def list_executions():
    result = list(executions.values())
    # Filters
    workflow_id = request.args.get("workflow_id")
    if workflow_id:
        try:
            wid = int(workflow_id)
            result = [e for e in result if e["workflow_id"] == wid]
        except (ValueError, TypeError):
            pass
    status = request.args.get("status")
    if status:
        result = [e for e in result if e["status"] == status]
    # Sort by id desc (most recent first)
    result.sort(key=lambda e: e["id"], reverse=True)
    return jsonify({
        "data": result,
        "total": len(result),
    })


@app.route("/executions/<int:execution_id>", methods=["GET"])
def get_execution(execution_id):
    ex = executions.get(execution_id)
    if ex is None:
        return jsonify({"error": "execution not found"}), 404
    return jsonify(ex)


# ═══════════════════════════════════════════════════════════════

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9805))
    logger.info("Starting orchestrator on :%d", port)
    app.run(host="0.0.0.0", port=port, debug=False)

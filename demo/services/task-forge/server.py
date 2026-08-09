#!/usr/bin/env python3
"""Task Forge — Project Management Demo Service (AC-D01)."""

import json
import os
import uuid
import time
from datetime import datetime
from flask import Flask, jsonify, request

app = Flask(__name__)

# ── Validation helper ───────────────────────────────────────


def _require(data, *fields):
    """Return a (response, status) tuple if any required field is missing or
    empty, else return None. Use as: ``err = _require(data, "name"); if err:
    return err``."""
    for f in fields:
        if f not in data or not data[f]:
            return jsonify({"error": f"{f} is required"}), 400
    return None


# ── In-memory store ──────────────────────────────────────────
projects = {}
tasks = {}
comments = {}
milestones = {}
tags = {}
next_ids = {"projects": 1, "tasks": 1, "comments": 1, "milestones": 1, "tags": 1}

# ── Health & Spec ────────────────────────────────────────────

@app.route("/health")
def health():
    return jsonify({"status": "healthy", "projects_count": len(projects), "tasks_count": len(tasks)})

@app.route("/openapi.json")
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))

# ── Projects ─────────────────────────────────────────────────

@app.route("/projects", methods=["GET", "POST"])
def projects_handler():
    if request.method == "GET":
        q = request.args.get("q")
        result = list(projects.values())
        if q:
            result = [p for p in result if q.lower() in p["name"].lower() or q.lower() in p.get("description", "").lower()]
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 20))
        start = (page - 1) * limit
        return jsonify(result[start:start + limit])
    else:
        data = request.get_json(force=True)
        err = _require(data, "name")
        if err:
            return err
        pid = str(next_ids["projects"]); next_ids["projects"] += 1
        project = {
            "id": pid, "name": data["name"], "description": data.get("description", ""),
            "status": "active", "tags": data.get("tags", []),
            "created_at": datetime.utcnow().isoformat() + "Z", "updated_at": datetime.utcnow().isoformat() + "Z"
        }
        projects[pid] = project
        return jsonify(project), 201

@app.route("/projects/<project_id>", methods=["GET", "PATCH", "DELETE"])
def project_handler(project_id):
    if project_id not in projects:
        return jsonify({"error": "project not found"}), 404
    if request.method == "GET":
        return jsonify(projects[project_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        for k in ("name", "description", "status"):
            if k in data:
                projects[project_id][k] = data[k]
        projects[project_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(projects[project_id])
    else:  # DELETE
        del projects[project_id]
        return jsonify({"deleted": True})

@app.route("/projects/<project_id>/tasks", methods=["GET", "POST"])
def project_tasks(project_id):
    if project_id not in projects:
        return jsonify({"error": "project not found"}), 404
    if request.method == "GET":
        result = [t for t in tasks.values() if t["project_id"] == project_id]
        status = request.args.get("status")
        if status: result = [t for t in result if t["status"] == status]
        tag = request.args.get("tag")
        if tag: result = [t for t in result if tag in t.get("tags", [])]
        return jsonify(result)
    else:
        data = request.get_json(force=True)
        err = _require(data, "title")
        if err:
            return err
        tid = str(next_ids["tasks"]); next_ids["tasks"] += 1
        task = {
            "id": tid, "project_id": project_id,
            "title": data["title"], "description": data.get("description", ""),
            "status": data.get("status", "todo"), "priority": data.get("priority", "medium"),
            "assignee": data.get("assignee", ""), "tags": data.get("tags", []),
            "milestone_id": data.get("milestone_id"),
            "created_at": datetime.utcnow().isoformat() + "Z", "updated_at": datetime.utcnow().isoformat() + "Z"
        }
        tasks[tid] = task
        return jsonify(task), 201

# ── Tasks ────────────────────────────────────────────────────

@app.route("/tasks", methods=["GET"])
def tasks_handler():
    result = list(tasks.values())
    page = int(request.args.get("page", 1))
    limit = int(request.args.get("limit", 20))
    start = (page - 1) * limit
    return jsonify(result[start:start + limit])

@app.route("/tasks/<task_id>", methods=["GET", "PATCH", "PUT", "DELETE"])
def task_handler(task_id):
    if task_id not in tasks:
        return jsonify({"error": "task not found"}), 404
    if request.method == "GET":
        return jsonify(tasks[task_id])
    elif request.method in ("PATCH", "PUT"):
        data = request.get_json(force=True)
        for k in ("title", "description", "status", "priority", "assignee", "milestone_id"):
            if k in data:
                tasks[task_id][k] = data[k]
        if "tags" in data: tasks[task_id]["tags"] = data["tags"]
        tasks[task_id]["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(tasks[task_id])
    else:
        del tasks[task_id]
        return jsonify({"deleted": True})

# ── Comments (nested under tasks) ────────────────────────────

@app.route("/tasks/<task_id>/comments", methods=["GET", "POST"])
def task_comments(task_id):
    if task_id not in tasks:
        return jsonify({"error": "task not found"}), 404
    if request.method == "GET":
        return jsonify([c for c in comments.values() if c["task_id"] == task_id])
    else:
        data = request.get_json(force=True)
        err = _require(data, "body")
        if err:
            return err
        cid = str(next_ids["comments"]); next_ids["comments"] += 1
        comment = {
            "id": cid, "task_id": task_id,
            "author": data.get("author", "anonymous"), "body": data["body"],
            "created_at": datetime.utcnow().isoformat() + "Z"
        }
        comments[cid] = comment
        return jsonify(comment), 201

@app.route("/comments/<comment_id>", methods=["DELETE"])
def comment_handler(comment_id):
    if comment_id not in comments:
        return jsonify({"error": "comment not found"}), 404
    del comments[comment_id]
    return jsonify({"deleted": True})

# ── Milestones ───────────────────────────────────────────────

@app.route("/projects/<project_id>/milestones", methods=["GET", "POST"])
def project_milestones(project_id):
    if project_id not in projects:
        return jsonify({"error": "project not found"}), 404
    if request.method == "GET":
        return jsonify([m for m in milestones.values() if m["project_id"] == project_id])
    else:
        data = request.get_json(force=True)
        err = _require(data, "name")
        if err:
            return err
        mid = str(next_ids["milestones"]); next_ids["milestones"] += 1
        milestone = {
            "id": mid, "project_id": project_id,
            "name": data["name"], "description": data.get("description", ""),
            "due_date": data.get("due_date", ""), "status": data.get("status", "planned"),
            "created_at": datetime.utcnow().isoformat() + "Z"
        }
        milestones[mid] = milestone
        return jsonify(milestone), 201

@app.route("/milestones/<milestone_id>", methods=["GET", "DELETE"])
def milestone_handler(milestone_id):
    if milestone_id not in milestones:
        return jsonify({"error": "milestone not found"}), 404
    if request.method == "GET":
        return jsonify(milestones[milestone_id])
    del milestones[milestone_id]
    return jsonify({"deleted": True})

# ── Tags ─────────────────────────────────────────────────────

@app.route("/tags", methods=["GET", "POST"])
def tags_handler():
    if request.method == "GET":
        return jsonify(list(tags.values()))
    else:
        data = request.get_json(force=True)
        err = _require(data, "name")
        if err:
            return err
        tid = str(next_ids["tags"]); next_ids["tags"] += 1
        tag = {"id": tid, "name": data["name"], "color": data.get("color", "#666666")}
        tags[tid] = tag
        return jsonify(tag), 201

# ── Main ─────────────────────────────────────────────────────

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9801))
    print(f"Task Forge starting on :{port}")
    app.run(host="0.0.0.0", port=port, debug=False)

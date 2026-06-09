#!/usr/bin/env python3
"""
task-forge: Project Management API
Listens on :9800
Resources: projects, tasks, comments, milestones, tags
Key patterns: nested resources, query filters, PATCH partial update, pagination, search
"""
from flask import Flask, request, jsonify, g
import logging
import sys
import time
import os
import json

app = Flask(__name__)

# ── In-memory stores ───────────────────────────────────────────
projects = {}
tasks = {}
comments = {}
milestones = {}
tags = {}
_next_ids = {"projects": 1, "tasks": 1, "comments": 1, "milestones": 1, "tags": 1}

# ── Logging ────────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    stream=sys.stdout,
)
logger = logging.getLogger("task-forge")


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

def _paginate(items, default_limit=20, max_limit=100):
    try:
        page = int(request.args.get("page", 1))
    except (ValueError, TypeError):
        page = 1
    try:
        limit = int(request.args.get("per_page", request.args.get("limit", default_limit)))
    except (ValueError, TypeError):
        limit = default_limit
    limit = max(1, min(limit, max_limit))
    offset = (page - 1) * limit
    result = items[offset:offset + limit]
    return {
        "data": result,
        "page": page,
        "per_page": limit,
        "limit": limit,
        "total": len(items),
        "pages": max(1, (len(items) + limit - 1) // limit),
    }


def _apply_sort(items, default_field="id", default_order="asc"):
    sort_field = request.args.get("sort", default_field)
    order = request.args.get("order", default_order).lower()
    reverse = order == "desc"
    try:
        items.sort(key=lambda x: (x.get(sort_field) is None, x.get(sort_field, "")), reverse=reverse)
    except (TypeError, KeyError):
        pass
    return items


# ═══════════════════════════════════════════════════════════════
#  PROJECTS
# ═══════════════════════════════════════════════════════════════

@app.route("/projects", methods=["GET"])
def list_projects():
    result = list(projects.values())
    # Filters
    status = request.args.get("status")
    if status:
        result = [p for p in result if p.get("status") == status]
    name = request.args.get("name")
    if name:
        result = [p for p in result if name.lower() in p.get("name", "").lower()]
    q = request.args.get("q")
    if q:
        result = [p for p in result if q.lower() in p.get("name", "").lower() or q.lower() in p.get("description", "").lower()]
    result = _apply_sort(result, "id")
    return jsonify(_paginate(result))


@app.route("/projects", methods=["POST"])
def create_project():
    data = request.get_json(force=True, silent=True)
    if not data or "name" not in data:
        return jsonify({"error": "name is required"}), 400
    pid = _next_ids["projects"]
    _next_ids["projects"] += 1
    project = {
        "id": pid,
        "name": data["name"],
        "description": data.get("description", ""),
        "status": data.get("status", "active"),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    projects[pid] = project
    return jsonify(project), 201


@app.route("/projects/<int:project_id>", methods=["GET"])
def get_project(project_id):
    p = projects.get(project_id)
    if p is None:
        return jsonify({"error": "project not found"}), 404
    return jsonify(p)


@app.route("/projects/<int:project_id>", methods=["PUT"])
def update_project(project_id):
    p = projects.get(project_id)
    if p is None:
        return jsonify({"error": "project not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        p["name"] = data["name"]
    if "description" in data:
        p["description"] = data["description"]
    if "status" in data:
        p["status"] = data["status"]
    p["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(p)


@app.route("/projects/<int:project_id>", methods=["PATCH"])
def patch_project(project_id):
    p = projects.get(project_id)
    if p is None:
        return jsonify({"error": "project not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        p["name"] = data["name"]
    if "description" in data:
        p["description"] = data["description"]
    if "status" in data:
        p["status"] = data["status"]
    p["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(p)


@app.route("/projects/<int:project_id>", methods=["DELETE"])
def delete_project(project_id):
    p = projects.pop(project_id, None)
    if p is None:
        return jsonify({"error": "project not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
#  PROJECTS → TASKS (nested)
# ═══════════════════════════════════════════════════════════════

@app.route("/projects/<int:project_id>/tasks", methods=["GET"])
def list_project_tasks(project_id):
    p = projects.get(project_id)
    if p is None:
        return jsonify({"error": "project not found"}), 404
    result = [t for t in tasks.values() if t["project_id"] == project_id]
    status = request.args.get("status")
    if status:
        result = [t for t in result if t.get("status") == status]
    priority = request.args.get("priority")
    if priority:
        result = [t for t in result if t.get("priority") == priority]
    tag = request.args.get("tag")
    if tag:
        result = [t for t in result if tag in t.get("tags", [])]
    result = _apply_sort(result, "id")
    return jsonify(_paginate(result))


@app.route("/projects/<int:project_id>/tasks", methods=["POST"])
def create_project_task(project_id):
    p = projects.get(project_id)
    if p is None:
        return jsonify({"error": "project not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data or "title" not in data:
        return jsonify({"error": "title is required"}), 400
    tid = _next_ids["tasks"]
    _next_ids["tasks"] += 1
    task = {
        "id": tid,
        "project_id": project_id,
        "title": data["title"],
        "description": data.get("description", ""),
        "status": data.get("status", "open"),
        "priority": data.get("priority", "medium"),
        "milestone_id": data.get("milestone_id"),
        "tags": data.get("tags", []),
        "assignee": data.get("assignee"),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    tasks[tid] = task
    return jsonify(task), 201


# ═══════════════════════════════════════════════════════════════
#  TASKS (top-level)
# ═══════════════════════════════════════════════════════════════

@app.route("/tasks", methods=["GET"])
def list_tasks():
    result = list(tasks.values())
    status = request.args.get("status")
    if status:
        result = [t for t in result if t.get("status") == status]
    priority = request.args.get("priority")
    if priority:
        result = [t for t in result if t.get("priority") == priority]
    tag = request.args.get("tag")
    if tag:
        result = [t for t in result if tag in t.get("tags", [])]
    project_id = request.args.get("project_id")
    if project_id:
        try:
            pid = int(project_id)
            result = [t for t in result if t.get("project_id") == pid]
        except (ValueError, TypeError):
            pass
    assignee = request.args.get("assignee")
    if assignee:
        result = [t for t in result if t.get("assignee") == assignee]
    q = request.args.get("q")
    if q:
        result = [t for t in result if q.lower() in t.get("title", "").lower() or q.lower() in t.get("description", "").lower()]
    result = _apply_sort(result, "id")
    return jsonify(_paginate(result))


@app.route("/tasks/search", methods=["GET"])
def search_tasks():
    """Full-text search across all task fields."""
    q = (request.args.get("q") or "").lower()
    if not q:
        return jsonify({"data": [], "total": 0, "query": q})
    result = []
    for t in tasks.values():
        # Search across title, description, status, priority, assignee, tags
        searchable = " ".join([
            t.get("title", ""),
            t.get("description", ""),
            t.get("status", ""),
            t.get("priority", ""),
            t.get("assignee", ""),
            " ".join(t.get("tags", [])),
        ]).lower()
        if q in searchable:
            result.append(t)
    result = _apply_sort(result, "created_at", "desc")
    return jsonify(_paginate(result))


@app.route("/tasks/<int:task_id>", methods=["GET"])
def get_task(task_id):
    t = tasks.get(task_id)
    if t is None:
        return jsonify({"error": "task not found"}), 404
    return jsonify(t)


@app.route("/tasks/<int:task_id>", methods=["PUT"])
def update_task(task_id):
    t = tasks.get(task_id)
    if t is None:
        return jsonify({"error": "task not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("title", "description", "status", "priority", "milestone_id", "tags", "assignee"):
        if field in data:
            t[field] = data[field]
    t["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(t)


@app.route("/tasks/<int:task_id>", methods=["PATCH"])
def patch_task(task_id):
    """Partial update on a task."""
    t = tasks.get(task_id)
    if t is None:
        return jsonify({"error": "task not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("title", "description", "status", "priority", "milestone_id", "tags", "assignee"):
        if field in data:
            t[field] = data[field]
    t["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(t)


@app.route("/tasks/<int:task_id>", methods=["DELETE"])
def delete_task(task_id):
    t = tasks.pop(task_id, None)
    if t is None:
        return jsonify({"error": "task not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
#  TASKS → COMMENTS (nested)
# ═══════════════════════════════════════════════════════════════

@app.route("/tasks/<int:task_id>/comments", methods=["GET"])
def list_task_comments(task_id):
    t = tasks.get(task_id)
    if t is None:
        return jsonify({"error": "task not found"}), 404
    result = sorted(
        [c for c in comments.values() if c["task_id"] == task_id],
        key=lambda c: c["id"],
    )
    return jsonify({"data": result, "total": len(result)})


@app.route("/tasks/<int:task_id>/comments", methods=["POST"])
def create_task_comment(task_id):
    t = tasks.get(task_id)
    if t is None:
        return jsonify({"error": "task not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data or "content" not in data:
        return jsonify({"error": "content is required"}), 400
    cid = _next_ids["comments"]
    _next_ids["comments"] += 1
    comment = {
        "id": cid,
        "task_id": task_id,
        "content": data["content"],
        "author": data.get("author", "anonymous"),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    comments[cid] = comment
    return jsonify(comment), 201


# ═══════════════════════════════════════════════════════════════
#  COMMENTS
# ═══════════════════════════════════════════════════════════════

@app.route("/comments/<int:comment_id>", methods=["GET"])
def get_comment(comment_id):
    c = comments.get(comment_id)
    if c is None:
        return jsonify({"error": "comment not found"}), 404
    return jsonify(c)


@app.route("/comments/<int:comment_id>", methods=["PUT"])
def update_comment(comment_id):
    c = comments.get(comment_id)
    if c is None:
        return jsonify({"error": "comment not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "content" in data:
        c["content"] = data["content"]
    if "author" in data:
        c["author"] = data["author"]
    return jsonify(c)


@app.route("/comments/<int:comment_id>", methods=["DELETE"])
def delete_comment(comment_id):
    c = comments.pop(comment_id, None)
    if c is None:
        return jsonify({"error": "comment not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
#  MILESTONES
# ═══════════════════════════════════════════════════════════════

@app.route("/milestones", methods=["GET"])
def list_milestones():
    result = list(milestones.values())
    project_id = request.args.get("project_id")
    if project_id:
        try:
            pid = int(project_id)
            result = [m for m in result if m.get("project_id") == pid]
        except (ValueError, TypeError):
            pass
    status = request.args.get("status")
    if status:
        result = [m for m in result if m.get("status") == status]
    result = _apply_sort(result, "due_date")
    return jsonify(_paginate(result))


@app.route("/milestones", methods=["POST"])
def create_milestone():
    data = request.get_json(force=True, silent=True)
    if not data or "name" not in data or "project_id" not in data:
        return jsonify({"error": "name and project_id are required"}), 400
    mid = _next_ids["milestones"]
    _next_ids["milestones"] += 1
    milestone = {
        "id": mid,
        "project_id": data["project_id"],
        "name": data["name"],
        "description": data.get("description", ""),
        "due_date": data.get("due_date"),
        "status": data.get("status", "pending"),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    milestones[mid] = milestone
    return jsonify(milestone), 201


@app.route("/milestones/<int:milestone_id>", methods=["GET"])
def get_milestone(milestone_id):
    m = milestones.get(milestone_id)
    if m is None:
        return jsonify({"error": "milestone not found"}), 404
    return jsonify(m)


@app.route("/milestones/<int:milestone_id>", methods=["PUT"])
def update_milestone(milestone_id):
    m = milestones.get(milestone_id)
    if m is None:
        return jsonify({"error": "milestone not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("name", "description", "due_date", "status", "project_id"):
        if field in data:
            m[field] = data[field]
    m["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(m)


@app.route("/milestones/<int:milestone_id>", methods=["PATCH"])
def patch_milestone(milestone_id):
    m = milestones.get(milestone_id)
    if m is None:
        return jsonify({"error": "milestone not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("name", "description", "due_date", "status", "project_id"):
        if field in data:
            m[field] = data[field]
    m["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(m)


@app.route("/milestones/<int:milestone_id>", methods=["DELETE"])
def delete_milestone(milestone_id):
    m = milestones.pop(milestone_id, None)
    if m is None:
        return jsonify({"error": "milestone not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
#  TAGS
# ═══════════════════════════════════════════════════════════════

@app.route("/tags", methods=["GET"])
def list_tags():
    result = sorted(tags.values(), key=lambda t: t["id"])
    return jsonify(_paginate(result))


@app.route("/tags", methods=["POST"])
def create_tag():
    data = request.get_json(force=True, silent=True)
    if not data or "name" not in data:
        return jsonify({"error": "name is required"}), 400
    tid = _next_ids["tags"]
    _next_ids["tags"] += 1
    tag = {
        "id": tid,
        "name": data["name"],
        "color": data.get("color", "#808080"),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    tags[tid] = tag
    return jsonify(tag), 201


@app.route("/tags/<int:tag_id>", methods=["GET"])
def get_tag(tag_id):
    t = tags.get(tag_id)
    if t is None:
        return jsonify({"error": "tag not found"}), 404
    return jsonify(t)


@app.route("/tags/<int:tag_id>", methods=["PUT"])
def update_tag(tag_id):
    t = tags.get(tag_id)
    if t is None:
        return jsonify({"error": "tag not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        t["name"] = data["name"]
    if "color" in data:
        t["color"] = data["color"]
    return jsonify(t)


@app.route("/tags/<int:tag_id>", methods=["DELETE"])
def delete_tag(tag_id):
    t = tags.pop(tag_id, None)
    if t is None:
        return jsonify({"error": "tag not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9800))
    logger.info("Starting task-forge on :%d", port)
    app.run(host="0.0.0.0", port=port, debug=False)

#!/usr/bin/env python3
"""
file-vault: File Storage API
Listens on :9802
Resources: files, folders, shares
Key patterns: multipart upload, binary download, folder hierarchy, soft delete (trash/recover)
"""
from flask import Flask, request, jsonify, g, send_file
import logging
import sys
import time
import os
import json
import io

app = Flask(__name__)

# ── In-memory stores ───────────────────────────────────────────
files = {}       # id -> {id, name, folder_id, size, content_type, content (bytes), deleted, ...}
folders = {}     # id -> {id, name, parent_id, deleted, ...}
shares = {}      # id -> {id, file_id, shared_with, permission, ...}
_next_ids = {"files": 1, "folders": 1, "shares": 1}
_trash = {"files": [], "folders": []}  # ids pending permanent deletion

# ── Logging ────────────────────────────────────────────────────
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s", stream=sys.stdout)
logger = logging.getLogger("file-vault")


@app.before_request
def log_request():
    g.start_time = time.time()
    logger.info("--> %s %s", request.method, request.path)


@app.after_request
def log_response(response):
    elapsed = time.time() - g.start_time
    logger.info("<-- %s %s %d %.4fs", request.method, request.path, response.status_code, elapsed)
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
    except ValueError:
        page = 1
    try:
        limit = int(request.args.get("limit", default_limit))
    except ValueError:
        limit = default_limit
    limit = max(1, min(limit, max_limit))
    offset = (page - 1) * limit
    result = items[offset:offset + limit]
    return {"data": result, "page": page, "limit": limit, "total": len(items), "pages": max(1, (len(items) + limit - 1) // limit)}


def _file_meta(f):
    """Return file metadata without binary content."""
    return {k: v for k, v in f.items() if k != "content"}


# ═══════════════════════════════════════════════════════════════
#  FILES
# ═══════════════════════════════════════════════════════════════

@app.route("/files", methods=["GET"])
def list_files():
    result = sorted(files.values(), key=lambda f: f["id"])
    # Filter out deleted (soft-deleted)
    if request.args.get("include_deleted") != "true":
        result = [f for f in result if not f.get("deleted")]
    folder_id = request.args.get("folder_id")
    if folder_id:
        try:
            fid = int(folder_id)
            result = [f for f in result if f.get("folder_id") == fid]
        except ValueError:
            pass
    name = request.args.get("name")
    if name:
        result = [f for f in result if name.lower() in f.get("name", "").lower()]
    return jsonify(_paginate([_file_meta(f) for f in result]))


@app.route("/files", methods=["POST"])
def upload_file():
    # Support both multipart/form-data and JSON metadata
    name = None
    content = b""
    content_type = "application/octet-stream"
    folder_id = None

    if request.content_type and "multipart/form-data" in request.content_type:
        if "file" in request.files:
            f = request.files["file"]
            name = f.filename
            content = f.read()
            content_type = f.content_type or "application/octet-stream"
        folder_id_str = request.form.get("folder_id")
        if folder_id_str:
            try:
                folder_id = int(folder_id_str)
            except ValueError:
                return jsonify({"error": "invalid folder_id"}), 400
    else:
        data = request.get_json(force=True, silent=True)
        if not data or "name" not in data:
            return jsonify({"error": "name is required"}), 400
        name = data["name"]
        content = (data.get("content") or "").encode("utf-8")
        content_type = data.get("content_type", "text/plain")
        folder_id = data.get("folder_id")

    if not name:
        return jsonify({"error": "name is required"}), 400

    fid = _next_ids["files"]
    _next_ids["files"] += 1
    file_obj = {
        "id": fid,
        "name": name,
        "folder_id": folder_id,
        "size": len(content),
        "content_type": content_type,
        "content": content,
        "deleted": False,
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    files[fid] = file_obj
    return jsonify(_file_meta(file_obj)), 201


@app.route("/files/<int:file_id>", methods=["GET"])
def get_file(file_id):
    f = files.get(file_id)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    return jsonify(_file_meta(f))


@app.route("/files/<int:file_id>/download", methods=["GET"])
def download_file(file_id):
    f = files.get(file_id)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    if f.get("deleted"):
        return jsonify({"error": "file is in trash — recover first"}), 404
    return send_file(
        io.BytesIO(f["content"]),
        mimetype=f.get("content_type", "application/octet-stream"),
        as_attachment=True,
        download_name=f["name"],
    )


@app.route("/files/<int:file_id>", methods=["PUT"])
def update_file(file_id):
    f = files.get(file_id)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        f["name"] = data["name"]
    if "content" in data:
        f["content"] = data["content"].encode("utf-8")
        f["size"] = len(f["content"])
    if "content_type" in data:
        f["content_type"] = data["content_type"]
    if "folder_id" in data:
        f["folder_id"] = data["folder_id"]
    f["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(_file_meta(f))


@app.route("/files/<int:file_id>", methods=["PATCH"])
def patch_file(file_id):
    f = files.get(file_id)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        f["name"] = data["name"]
    if "folder_id" in data:
        f["folder_id"] = data["folder_id"]
    f["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(_file_meta(f))


@app.route("/files/<int:file_id>", methods=["DELETE"])
def delete_file(file_id):
    f = files.get(file_id)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    f["deleted"] = True
    f["deleted_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    _trash["files"].append(file_id)
    return jsonify({"deleted": True, "trashed": True})


@app.route("/files/<int:file_id>/recover", methods=["POST"])
def recover_file(file_id):
    f = files.get(file_id)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    if not f.get("deleted"):
        return jsonify({"error": "file is not deleted"}), 400
    f["deleted"] = False
    f.pop("deleted_at", None)
    if file_id in _trash["files"]:
        _trash["files"].remove(file_id)
    f["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(_file_meta(f))


@app.route("/files/<int:file_id>/permanent", methods=["DELETE"])
def permanent_delete_file(file_id):
    f = files.pop(file_id, None)
    if f is None:
        return jsonify({"error": "file not found"}), 404
    if file_id in _trash["files"]:
        _trash["files"].remove(file_id)
    return jsonify({"deleted": True, "permanent": True})


# ═══════════════════════════════════════════════════════════════
#  FOLDERS
# ═══════════════════════════════════════════════════════════════

@app.route("/folders", methods=["GET"])
def list_folders():
    result = sorted(folders.values(), key=lambda f: f["id"])
    if request.args.get("include_deleted") != "true":
        result = [f for f in result if not f.get("deleted")]
    parent_id = request.args.get("parent_id")
    if parent_id is not None:
        try:
            pid = int(parent_id)
            result = [f for f in result if f.get("parent_id") == pid]
        except ValueError:
            pass
    return jsonify(_paginate(result))


@app.route("/folders", methods=["POST"])
def create_folder():
    data = request.get_json(force=True, silent=True)
    if not data or "name" not in data:
        return jsonify({"error": "name is required"}), 400
    parent_id = data.get("parent_id")
    fid = _next_ids["folders"]
    _next_ids["folders"] += 1
    folder = {
        "id": fid,
        "name": data["name"],
        "parent_id": parent_id,
        "deleted": False,
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    folders[fid] = folder
    return jsonify(folder), 201


@app.route("/folders/<int:folder_id>", methods=["GET"])
def get_folder(folder_id):
    f = folders.get(folder_id)
    if f is None:
        return jsonify({"error": "folder not found"}), 404
    # Include children counts
    children_files = sum(1 for fi in files.values() if fi.get("folder_id") == folder_id and not fi.get("deleted"))
    children_folders = sum(1 for fo in folders.values() if fo.get("parent_id") == folder_id and not fo.get("deleted"))
    result = dict(f)
    result["file_count"] = children_files
    result["folder_count"] = children_folders
    return jsonify(result)


@app.route("/folders/<int:folder_id>", methods=["PUT"])
def update_folder(folder_id):
    f = folders.get(folder_id)
    if f is None:
        return jsonify({"error": "folder not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "name" in data:
        f["name"] = data["name"]
    if "parent_id" in data:
        f["parent_id"] = data["parent_id"]
    f["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(f)


@app.route("/folders/<int:folder_id>", methods=["DELETE"])
def delete_folder(folder_id):
    f = folders.get(folder_id)
    if f is None:
        return jsonify({"error": "folder not found"}), 404
    f["deleted"] = True
    f["deleted_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    _trash["folders"].append(folder_id)
    return jsonify({"deleted": True, "trashed": True})


@app.route("/folders/<int:folder_id>/recover", methods=["POST"])
def recover_folder(folder_id):
    f = folders.get(folder_id)
    if f is None:
        return jsonify({"error": "folder not found"}), 404
    if not f.get("deleted"):
        return jsonify({"error": "folder is not deleted"}), 400
    f["deleted"] = False
    f.pop("deleted_at", None)
    if folder_id in _trash["folders"]:
        _trash["folders"].remove(folder_id)
    f["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(f)


# ═══════════════════════════════════════════════════════════════
#  SHARES
# ═══════════════════════════════════════════════════════════════

@app.route("/shares", methods=["GET"])
def list_shares():
    result = sorted(shares.values(), key=lambda s: s["id"])
    file_id = request.args.get("file_id")
    if file_id:
        try:
            fid = int(file_id)
            result = [s for s in result if s.get("file_id") == fid]
        except ValueError:
            pass
    return jsonify({"data": result, "total": len(result)})


@app.route("/shares", methods=["POST"])
def create_share():
    data = request.get_json(force=True, silent=True)
    if not data or "file_id" not in data or "shared_with" not in data:
        return jsonify({"error": "file_id and shared_with are required"}), 400
    sid = _next_ids["shares"]
    _next_ids["shares"] += 1
    share = {
        "id": sid,
        "file_id": data["file_id"],
        "shared_with": data["shared_with"],
        "permission": data.get("permission", "read"),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    shares[sid] = share
    return jsonify(share), 201


@app.route("/shares/<int:share_id>", methods=["GET"])
def get_share(share_id):
    s = shares.get(share_id)
    if s is None:
        return jsonify({"error": "share not found"}), 404
    return jsonify(s)


@app.route("/shares/<int:share_id>", methods=["PUT"])
def update_share(share_id):
    s = shares.get(share_id)
    if s is None:
        return jsonify({"error": "share not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "shared_with" in data:
        s["shared_with"] = data["shared_with"]
    if "permission" in data:
        s["permission"] = data["permission"]
    return jsonify(s)


@app.route("/shares/<int:share_id>", methods=["DELETE"])
def delete_share(share_id):
    s = shares.pop(share_id, None)
    if s is None:
        return jsonify({"error": "share not found"}), 404
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9802))
    logger.info("Starting file-vault on :%d", port)
    app.run(host="0.0.0.0", port=port, debug=False)

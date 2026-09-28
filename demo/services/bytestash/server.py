#!/usr/bin/env python3
"""ByteStash — Cloud File Storage Demo Service (AC-D02)."""

import json
import os
import sys
import uuid
import shutil
import secrets
from datetime import datetime
from flask import Flask, jsonify, request, send_file, Response

app = Flask(__name__)


def _require(data, *fields):
    """Return a (response, status) tuple if any required field is missing or
    empty, else return None."""
    for f in fields:
        if f not in data or not data[f]:
            return jsonify({"error": f"{f} is required"}), 400
    return None


# ── Storage directory ─────────────────────────────────────────
STORAGE_DIR = os.path.join(os.path.dirname(__file__), "_storage")
os.makedirs(STORAGE_DIR, exist_ok=True)

# ── In-memory store ──────────────────────────────────────────
folders = {}
files = {}
trash_items = {}
share_links = {}
multipart_uploads = {}
next_ids = {"folders": 1, "files": 1, "trash": 1, "shares": 1, "uploads": 1}

def _now():
    return datetime.utcnow().isoformat() + "Z"

def _content_path(file_id):
    return os.path.join(STORAGE_DIR, file_id)

def _part_path(upload_id, part_number):
    return os.path.join(STORAGE_DIR, f"._upload_{upload_id}_part_{part_number}")

# ── Health & Spec ────────────────────────────────────────────

@app.route("/health")
def health():
    return jsonify({
        "status": "healthy",
        "folders_count": len(folders),
        "files_count": len(files),
        "trash_count": len(trash_items),
        "shares_count": len(share_links),
        "uploads_count": len(multipart_uploads),
    })

@app.route("/openapi.json")
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))

# ── Folders ──────────────────────────────────────────────────

@app.route("/folders", methods=["GET", "POST"])
def folders_handler():
    if request.method == "GET":
        parent_id = request.args.get("parent_id")
        result = list(folders.values())
        if parent_id is not None:
            result = [f for f in result if f.get("parent_id") == parent_id]
        else:
            # root-level only unless parent_id specified
            result = [f for f in result if f.get("parent_id") is None]
        return jsonify(result)
    else:
        data = request.get_json(force=True)
        err = _require(data, "name")
        if err:
            return err
        fid = str(next_ids["folders"]); next_ids["folders"] += 1
        parent_id = data.get("parent_id")
        if parent_id and parent_id not in folders:
            return jsonify({"error": "parent folder not found"}), 404
        folder = {
            "id": fid,
            "name": data["name"],
            "parent_id": parent_id,
            "created_at": _now(),
            "updated_at": _now(),
        }
        folders[fid] = folder
        return jsonify(folder), 201

@app.route("/folders/<folder_id>", methods=["GET", "PATCH", "DELETE"])
def folder_handler(folder_id):
    if folder_id not in folders:
        return jsonify({"error": "folder not found"}), 404
    if request.method == "GET":
        folder_data = dict(folders[folder_id])
        folder_data["children"] = [
            f for f in folders.values() if f.get("parent_id") == folder_id
        ]
        folder_data["files"] = [
            f for f in files.values() if f.get("folder_id") == folder_id
        ]
        return jsonify(folder_data)
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        if "name" in data:
            folders[folder_id]["name"] = data["name"]
        if "parent_id" in data:
            new_parent = data["parent_id"]
            if new_parent and new_parent not in folders:
                return jsonify({"error": "parent folder not found"}), 404
            # prevent circular nesting
            if new_parent == folder_id:
                return jsonify({"error": "cannot move folder into itself"}), 400
            folders[folder_id]["parent_id"] = new_parent
        folders[folder_id]["updated_at"] = _now()
        return jsonify(folders[folder_id])
    else:  # DELETE — soft delete
        return _soft_delete("folder", folder_id)

# ── Files ────────────────────────────────────────────────────

@app.route("/folders/<folder_id>/files", methods=["GET", "POST"])
def folder_files(folder_id):
    if folder_id not in folders:
        return jsonify({"error": "folder not found"}), 404
    if request.method == "GET":
        result = [f for f in files.values() if f.get("folder_id") == folder_id]
        return jsonify(result)
    else:
        # Upload file via multipart/form-data
        if "file" not in request.files:
            return jsonify({"error": "no file field in request"}), 400
        uploaded = request.files["file"]
        if uploaded.filename == "" or uploaded.filename is None:
            return jsonify({"error": "empty filename"}), 400
        file_id = str(next_ids["files"]); next_ids["files"] += 1
        filename = uploaded.filename
        # Write content to disk
        content_path = _content_path(file_id)
        uploaded.save(content_path)
        file_size = os.path.getsize(content_path)
        file_record = {
            "id": file_id,
            "folder_id": folder_id,
            "name": filename,
            "size": file_size,
            "mime_type": uploaded.content_type or "application/octet-stream",
            "created_at": _now(),
            "updated_at": _now(),
        }
        files[file_id] = file_record
        return jsonify(file_record), 201

@app.route("/files/<file_id>", methods=["GET", "PATCH", "DELETE"])
def file_handler(file_id):
    if file_id not in files:
        return jsonify({"error": "file not found"}), 404
    if request.method == "GET":
        return jsonify(files[file_id])
    elif request.method == "PATCH":
        data = request.get_json(force=True)
        if "name" in data:
            files[file_id]["name"] = data["name"]
        if "folder_id" in data:
            new_folder = data["folder_id"]
            if new_folder not in folders:
                return jsonify({"error": "target folder not found"}), 404
            files[file_id]["folder_id"] = new_folder
        files[file_id]["updated_at"] = _now()
        return jsonify(files[file_id])
    else:  # DELETE — soft delete
        return _soft_delete("file", file_id)

@app.route("/files/<file_id>/download", methods=["GET"])
def file_download(file_id):
    if file_id not in files:
        return jsonify({"error": "file not found"}), 404
    content_path = _content_path(file_id)
    if not os.path.exists(content_path):
        return jsonify({"error": "file content missing on disk"}), 500
    f = files[file_id]
    return send_file(
        content_path,
        mimetype=f.get("mime_type", "application/octet-stream"),
        as_attachment=True,
        download_name=f["name"],
    )

# ── Multipart Upload ─────────────────────────────────────────

@app.route("/uploads", methods=["POST"])
def initiate_upload():
    data = request.get_json(force=True)
    err = _require(data, "filename")
    if err:
        return err
    folder_id = data.get("folder_id")
    if folder_id and folder_id not in folders:
        return jsonify({"error": "folder not found"}), 404
    upload_id = str(next_ids["uploads"]); next_ids["uploads"] += 1
    upload = {
        "id": upload_id,
        "filename": data["filename"],
        "folder_id": folder_id,
        "total_parts": int(data.get("total_parts", 0)),
        "mime_type": data.get("mime_type", "application/octet-stream"),
        "parts_received": [],
        "status": "in_progress",
        "created_at": _now(),
    }
    multipart_uploads[upload_id] = upload
    return jsonify(upload), 201

@app.route("/uploads/<upload_id>/parts", methods=["PUT"])
def upload_part(upload_id):
    if upload_id not in multipart_uploads:
        return jsonify({"error": "upload not found"}), 404
    upload = multipart_uploads[upload_id]
    if upload["status"] != "in_progress":
        return jsonify({"error": "upload is not in progress"}), 400
    part_number = request.args.get("part_number")
    if not part_number:
        return jsonify({"error": "part_number query parameter required"}), 400
    try:
        part_number = int(part_number)
    except ValueError:
        return jsonify({"error": "part_number must be an integer"}), 400
    if part_number < 1:
        return jsonify({"error": "part_number must be >= 1"}), 400

    # Save part binary to temp storage
    part_path = _part_path(upload_id, part_number)
    with open(part_path, "wb") as f:
        f.write(request.get_data())
    part_size = os.path.getsize(part_path)

    if part_number not in upload["parts_received"]:
        upload["parts_received"].append(part_number)

    return jsonify({
        "upload_id": upload_id,
        "part_number": part_number,
        "size": part_size,
        "parts_received": sorted(upload["parts_received"]),
    })

@app.route("/uploads/<upload_id>/complete", methods=["POST"])
def complete_upload(upload_id):
    if upload_id not in multipart_uploads:
        return jsonify({"error": "upload not found"}), 404
    upload = multipart_uploads[upload_id]
    if upload["status"] != "in_progress":
        return jsonify({"error": "upload is not in progress"}), 400
    if upload["total_parts"] > 0 and len(upload["parts_received"]) != upload["total_parts"]:
        return jsonify({
            "error": f"expected {upload['total_parts']} parts, got {len(upload['parts_received'])}"
        }), 400

    # Assemble parts
    file_id = str(next_ids["files"]); next_ids["files"] += 1
    content_path = _content_path(file_id)
    total_size = 0
    with open(content_path, "wb") as out:
        for pn in sorted(upload["parts_received"]):
            part_path = _part_path(upload_id, pn)
            with open(part_path, "rb") as inp:
                shutil.copyfileobj(inp, out)
            total_size += os.path.getsize(part_path)
            os.remove(part_path)  # clean up part

    file_record = {
        "id": file_id,
        "folder_id": upload.get("folder_id"),
        "name": upload["filename"],
        "size": total_size,
        "mime_type": upload["mime_type"],
        "created_at": _now(),
        "updated_at": _now(),
    }
    files[file_id] = file_record
    upload["status"] = "completed"
    upload["file_id"] = file_id
    return jsonify(file_record), 201

@app.route("/uploads/<upload_id>", methods=["DELETE"])
def abort_upload(upload_id):
    if upload_id not in multipart_uploads:
        return jsonify({"error": "upload not found"}), 404
    upload = multipart_uploads[upload_id]
    if upload["status"] != "in_progress":
        return jsonify({"error": "upload is not in progress"}), 400
    # Clean up any uploaded parts
    for pn in upload.get("parts_received", []):
        part_path = _part_path(upload_id, pn)
        if os.path.exists(part_path):
            os.remove(part_path)
    upload["status"] = "aborted"
    return jsonify({"aborted": True, "upload_id": upload_id})

@app.route("/uploads/<upload_id>", methods=["GET"])
def get_upload(upload_id):
    """Get upload status — used by demo to extract file_id after completion."""
    if upload_id not in multipart_uploads:
        return jsonify({"error": "upload not found"}), 404
    return jsonify(multipart_uploads[upload_id])

@app.route("/uploads/<upload_id>/parts/<int:part_number>", methods=["PUT"])
def upload_part_path(upload_id, part_number):
    """Upload a part with part_number as a path segment (demo compatibility)."""
    if upload_id not in multipart_uploads:
        return jsonify({"error": "upload not found"}), 404
    upload = multipart_uploads[upload_id]
    if upload["status"] != "in_progress":
        return jsonify({"error": "upload not in progress"}), 400
    data = request.get_data()
    part_path = _part_path(upload_id, part_number)
    with open(part_path, "wb") as f:
        f.write(data)
    if part_number not in upload["parts_received"]:
        upload["parts_received"].append(part_number)
    upload["total_parts"] = max(upload.get("total_parts", 0), part_number)
    return jsonify({"status": "received", "part": part_number, "size": len(data)})

# ── Trash (Soft Delete / Recover / Permanent Delete) ────────

def _soft_delete(item_type, item_id):
    """Move a folder or file to the trash."""
    store = folders if item_type == "folder" else files
    if item_id not in store:
        return jsonify({"error": f"{item_type} not found"}), 404

    # If folder, recursively trash children
    if item_type == "folder":
        child_folders = [f["id"] for f in folders.values() if f.get("parent_id") == item_id]
        child_files_list = [f["id"] for f in files.values() if f.get("folder_id") == item_id]
        for cf in child_folders:
            _soft_delete("folder", cf)
        for cf in child_files_list:
            _soft_delete("file", cf)

    tid = str(next_ids["trash"]); next_ids["trash"] += 1
    trash_item = {
        "id": tid,
        "item_type": item_type,
        "item_id": item_id,
        "item_name": store[item_id]["name"],
        "deleted_at": _now(),
        "original_data": dict(store[item_id]),
    }
    trash_items[tid] = trash_item
    del store[item_id]
    return jsonify({"deleted": True, "trash_id": tid}), 200

@app.route("/trash", methods=["GET"])
def trash_list():
    return jsonify(list(trash_items.values()))

@app.route("/trash/<trash_id>/recover", methods=["POST"])
def trash_recover(trash_id):
    if trash_id not in trash_items:
        return jsonify({"error": "trash item not found"}), 404
    item = trash_items[trash_id]
    item_type = item["item_type"]
    item_id = item["item_id"]
    store = folders if item_type == "folder" else files
    store[item_id] = item["original_data"]
    del trash_items[trash_id]
    return jsonify({"recovered": True, "item_type": item_type, "item_id": item_id})

@app.route("/trash/<trash_id>", methods=["DELETE"])
def trash_permanent_delete(trash_id):
    if trash_id not in trash_items:
        return jsonify({"error": "trash item not found"}), 404
    item = trash_items[trash_id]
    file_id = item["item_id"]
    if item["item_type"] == "file":
        content_path = _content_path(file_id)
        if os.path.exists(content_path):
            os.remove(content_path)
    # Also remove any associated share links
    to_remove = [k for k, v in share_links.items() if v.get("target_id") == file_id]
    for k in to_remove:
        del share_links[k]
    del trash_items[trash_id]
    return jsonify({"permanently_deleted": True})

# ── Share Links ──────────────────────────────────────────────

@app.route("/shares", methods=["GET"])
def shares_list():
    return jsonify(list(share_links.values()))

@app.route("/files/<file_id>/share", methods=["POST"])
def share_file(file_id):
    if file_id not in files:
        return jsonify({"error": "file not found"}), 404
    data = request.get_json(force=True) if request.is_json else {}
    return _create_share("file", file_id, data)

@app.route("/folders/<folder_id>/share", methods=["POST"])
def share_folder(folder_id):
    if folder_id not in folders:
        return jsonify({"error": "folder not found"}), 404
    data = request.get_json(force=True) if request.is_json else {}
    return _create_share("folder", folder_id, data)

def _create_share(target_type, target_id, data):
    sid = str(next_ids["shares"]); next_ids["shares"] += 1
    token = secrets.token_urlsafe(24)
    share = {
        "id": sid,
        "target_type": target_type,
        "target_id": target_id,
        "target_name": (folders if target_type == "folder" else files)[target_id]["name"],
        "token": token,
        "password": data.get("password"),
        "expires_at": data.get("expires_at"),
        "created_at": _now(),
    }
    share_links[sid] = share
    return jsonify(share), 201

@app.route("/shares/<share_id>", methods=["DELETE"])
def revoke_share(share_id):
    if share_id not in share_links:
        return jsonify({"error": "share link not found"}), 404
    del share_links[share_id]
    return jsonify({"revoked": True})

@app.route("/share/<token>", methods=["GET"])
def access_share(token):
    # Find share by token
    share = None
    for s in share_links.values():
        if s["token"] == token:
            share = s
            break
    if not share:
        return jsonify({"error": "invalid or expired share link"}), 404

    # Check expiration
    if share.get("expires_at") and share["expires_at"] < _now():
        return jsonify({"error": "share link has expired"}), 410

    # Check password if set
    if share.get("password"):
        provided = request.headers.get("X-Share-Password") or request.args.get("password")
        if provided != share["password"]:
            return jsonify({"error": "password required"}), 403

    target_type = share["target_type"]
    target_id = share["target_id"]

    if target_type == "folder":
        folder = folders.get(target_id)
        if not folder:
            return jsonify({"error": "folder no longer exists"}), 404
        result = dict(folder)
        result["files"] = [f for f in files.values() if f.get("folder_id") == target_id]
        result["children"] = [f for f in folders.values() if f.get("parent_id") == target_id]
        return jsonify(result)

    elif target_type == "file":
        f = files.get(target_id)
        if not f:
            return jsonify({"error": "file no longer exists"}), 404
        content_path = _content_path(target_id)
        if not os.path.exists(content_path):
            return jsonify({"error": "file content missing"}), 500
        return send_file(
            content_path,
            mimetype=f.get("mime_type", "application/octet-stream"),
            as_attachment=True,
            download_name=f["name"],
        )

# ── Main ─────────────────────────────────────────────────────

if __name__ == "__main__":
    usage = "usage: server.py [port]   (default: 9802; PORT environment variable also honored)"
    if len(sys.argv) > 2:
        sys.stderr.write(f"error: unexpected arguments {' '.join(sys.argv[2:])}\n{usage}\n")
        raise SystemExit(2)
    if len(sys.argv) == 2:
        try:
            port = int(sys.argv[1])
        except ValueError:
            sys.stderr.write(f"error: invalid port {sys.argv[1]!r}\n{usage}\n")
            raise SystemExit(2)
    else:
        port = int(os.environ.get("PORT", 9802))
    print(f"ByteStash starting on :{port}")
    app.run(host="0.0.0.0", port=port, debug=False)

#!/usr/bin/env python3
"""Inkwell — Blog/CMS Demo Service (AC-D03)."""

import json
import os
from datetime import datetime
from flask import Flask, jsonify, request, g

app = Flask(__name__)

# ═══════════════════════════════════════════════════════════════
# Auth
# ═══════════════════════════════════════════════════════════════

TOKENS = {
    "reader-token": {"role": "reader"},
    "author-token": {"role": "author", "author_id": "1"},
    "editor-token": {"role": "editor"},
    "admin-token": {"role": "admin"},
}

UNPROTECTED = {"/health", "/openapi.json"}
UNPROTECTED_GET = {"/posts"}


@app.before_request
def auth_middleware():
    g.token_info = None
    if request.path in UNPROTECTED:
        return
    if request.method == "GET" and request.path in UNPROTECTED_GET:
        return
    auth = request.headers.get("Authorization", "")
    if not auth.startswith("Bearer "):
        return jsonify({"error": "missing or invalid Authorization header"}), 401
    token = auth[7:]
    if token not in TOKENS:
        return jsonify({"error": "invalid token"}), 401
    g.token_info = TOKENS[token]


def role():
    if g.token_info is None:
        return "reader"
    return g.token_info["role"]


def author_id():
    return g.token_info.get("author_id")


def require_role(*roles):
    if role() not in roles:
        return jsonify({"error": "forbidden"}), 403
    return None


# ═══════════════════════════════════════════════════════════════
# In-memory store
# ═══════════════════════════════════════════════════════════════

authors = {}
posts = {}
comments = {}
next_ids = {"authors": 1, "posts": 1, "comments": 1}

# ═══════════════════════════════════════════════════════════════
# Health & Spec
# ═══════════════════════════════════════════════════════════════


@app.route("/health")
def health():
    return jsonify({
        "status": "healthy",
        "posts_count": len(posts),
        "comments_count": len(comments),
        "authors_count": len(authors),
    })


@app.route("/openapi.json")
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))


# ═══════════════════════════════════════════════════════════════
# Auth info
# ═══════════════════════════════════════════════════════════════


@app.route("/me")
def me():
    info = {"role": role()}
    if author_id():
        info["author_id"] = author_id()
        if author_id() in authors:
            info["username"] = authors[author_id()]["username"]
    return jsonify(info)


# ═══════════════════════════════════════════════════════════════
# Authors
# ═══════════════════════════════════════════════════════════════


@app.route("/authors", methods=["GET", "POST"])
def authors_handler():
    if request.method == "GET":
        return jsonify(list(authors.values()))
    # POST — create author (admin only)
    err = require_role("admin")
    if err:
        return err
    data = request.get_json(force=True)
    if not data.get("username"):
        return jsonify({"error": "username is required"}), 400
    aid = str(next_ids["authors"])
    next_ids["authors"] += 1
    author = {
        "id": aid,
        "username": data["username"],
        "display_name": data.get("display_name", data["username"]),
        "bio": data.get("bio", ""),
        "avatar_url": data.get("avatar_url", ""),
        "created_at": datetime.utcnow().isoformat() + "Z",
    }
    authors[aid] = author
    return jsonify(author), 201


@app.route("/authors/<author_id>", methods=["GET"])
def author_handler(author_id):
    if author_id not in authors:
        return jsonify({"error": "author not found"}), 404
    return jsonify(authors[author_id])


# ═══════════════════════════════════════════════════════════════
# Posts
# ═══════════════════════════════════════════════════════════════


@app.route("/posts", methods=["GET", "POST"])
def posts_handler():
    if request.method == "GET":
        # Readers see only published; editors/admins see all
        if role() == "reader":
            result = [p for p in posts.values() if p["status"] == "published"]
        else:
            result = list(posts.values())
        # Filters
        status = request.args.get("status")
        if status and role() != "reader":
            result = [p for p in result if p["status"] == status]
        author = request.args.get("author_id")
        if author:
            result = [p for p in result if p["author_id"] == author]
        q = request.args.get("q")
        if q:
            ql = q.lower()
            result = [p for p in result if ql in p["title"].lower() or ql in p.get("excerpt", "").lower()]
        # Pagination
        page = int(request.args.get("page", 1))
        limit = int(request.args.get("limit", 20))
        start = (page - 1) * limit
        return jsonify(result[start:start + limit])

    # POST — create post (author+)
    err = require_role("author", "editor", "admin")
    if err:
        return err
    data = request.get_json(force=True)
    if not data.get("title"):
        return jsonify({"error": "title is required"}), 400
    pid = str(next_ids["posts"])
    next_ids["posts"] += 1
    aid = author_id() or data.get("author_id", "1")
    now = datetime.utcnow().isoformat() + "Z"
    post = {
        "id": pid,
        "author_id": aid,
        "title": data["title"],
        "slug": data.get("slug", _slugify(data["title"])),
        "excerpt": data.get("excerpt", ""),
        "body": data.get("body", ""),
        "tags": data.get("tags", []),
        "status": "draft",
        "created_at": now,
        "updated_at": now,
        "published_at": None,
    }
    posts[pid] = post
    return jsonify(post), 201


@app.route("/posts/<post_id>", methods=["GET", "PATCH", "DELETE"])
def post_handler(post_id):
    if post_id not in posts:
        return jsonify({"error": "post not found"}), 404
    post = posts[post_id]

    if request.method == "GET":
        if post["status"] != "published" and role() == "reader":
            return jsonify({"error": "post not found"}), 404
        return jsonify(post)

    elif request.method == "PATCH":
        err = _can_edit_post(post)
        if err:
            return err
        data = request.get_json(force=True)
        for k in ("title", "slug", "excerpt", "body", "tags", "status"):
            if k in data:
                post[k] = data[k]
        post["updated_at"] = datetime.utcnow().isoformat() + "Z"
        return jsonify(post)

    else:  # DELETE
        err = require_role("admin")
        if err:
            return err
        del posts[post_id]
        # also delete associated comments
        for cid in list(comments.keys()):
            if comments[cid]["post_id"] == post_id:
                del comments[cid]
        return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
# Post workflow actions
# ═══════════════════════════════════════════════════════════════


@app.route("/posts/<post_id>/submit", methods=["POST"])
def post_submit(post_id):
    """Submit a draft for review (draft → review)."""
    if post_id not in posts:
        return jsonify({"error": "post not found"}), 404
    post = posts[post_id]
    err = _can_edit_post(post)
    if err:
        return err
    if post["status"] not in ("draft", "rejected"):
        return jsonify({"error": f"cannot submit post with status '{post['status']}'"}), 400
    post["status"] = "review"
    post["updated_at"] = datetime.utcnow().isoformat() + "Z"
    return jsonify(post)


@app.route("/posts/<post_id>/publish", methods=["POST"])
def post_publish(post_id):
    """Publish a post (review → published). Editor or admin."""
    if post_id not in posts:
        return jsonify({"error": "post not found"}), 404
    err = require_role("editor", "admin")
    if err:
        return err
    post = posts[post_id]
    if post["status"] != "review":
        return jsonify({"error": f"cannot publish post with status '{post['status']}'"}), 400
    post["status"] = "published"
    post["published_at"] = datetime.utcnow().isoformat() + "Z"
    post["updated_at"] = post["published_at"]
    return jsonify(post)


@app.route("/posts/<post_id>/reject", methods=["POST"])
def post_reject(post_id):
    """Reject a post back to draft (review → draft). Editor or admin."""
    if post_id not in posts:
        return jsonify({"error": "post not found"}), 404
    err = require_role("editor", "admin")
    if err:
        return err
    post = posts[post_id]
    if post["status"] != "review":
        return jsonify({"error": f"cannot reject post with status '{post['status']}'"}), 400
    post["status"] = "draft"
    post["updated_at"] = datetime.utcnow().isoformat() + "Z"
    return jsonify(post)


# ═══════════════════════════════════════════════════════════════
# Comments
# ═══════════════════════════════════════════════════════════════


@app.route("/posts/<post_id>/comments", methods=["GET", "POST"])
def post_comments(post_id):
    if post_id not in posts:
        return jsonify({"error": "post not found"}), 404

    if request.method == "GET":
        post = posts[post_id]
        if post["status"] != "published" and role() == "reader":
            return jsonify({"error": "post not found"}), 404
        result = [c for c in comments.values() if c["post_id"] == post_id]
        # Readers see only approved; editors/admins see all
        if role() == "reader":
            result = [c for c in result if c["status"] == "approved"]
        status_filter = request.args.get("status")
        if status_filter and role() != "reader":
            result = [c for c in result if c["status"] == status_filter]
        return jsonify(result)

    else:  # POST
        data = request.get_json(force=True)
        if not data.get("body"):
            return jsonify({"error": "body is required"}), 400
        cid = str(next_ids["comments"])
        next_ids["comments"] += 1
        # Auto-approve comments from author/editor/admin; readers go to pending
        auto_status = "approved" if role() in ("author", "editor", "admin") else "pending"
        comment = {
            "id": cid,
            "post_id": post_id,
            "parent_id": data.get("parent_id"),
            "author_name": data.get("author_name", "Anonymous"),
            "author_role": role(),
            "body": data["body"],
            "status": data.get("status", auto_status),
            "created_at": datetime.utcnow().isoformat() + "Z",
        }
        comments[cid] = comment
        return jsonify(comment), 201


@app.route("/comments/<comment_id>", methods=["GET", "PATCH", "DELETE"])
def comment_handler(comment_id):
    if comment_id not in comments:
        return jsonify({"error": "comment not found"}), 404

    if request.method == "GET":
        c = comments[comment_id]
        if c["status"] != "approved" and role() == "reader":
            return jsonify({"error": "comment not found"}), 404
        return jsonify(c)

    elif request.method == "PATCH":
        err = require_role("editor", "admin")
        if err:
            return err
        data = request.get_json(force=True)
        if "status" in data:
            if data["status"] not in ("pending", "approved", "rejected"):
                return jsonify({"error": "invalid status"}), 400
            comments[comment_id]["status"] = data["status"]
        if "body" in data:
            comments[comment_id]["body"] = data["body"]
        return jsonify(comments[comment_id])

    else:  # DELETE
        err = require_role("admin")
        if err:
            return err
        # Also delete child comments
        for cid in list(comments.keys()):
            if comments[cid].get("parent_id") == comment_id:
                del comments[cid]
        del comments[comment_id]
        return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════
# Helpers
# ═══════════════════════════════════════════════════════════════


def _can_edit_post(post):
    """Check if the current user can edit a post."""
    r = role()
    if r == "admin" or r == "editor":
        return None
    if r == "author" and post.get("author_id") == author_id():
        return None
    return jsonify({"error": "forbidden"}), 403


def _slugify(text):
    import re
    s = text.lower().strip()
    s = re.sub(r"[^\w\s-]", "", s)
    s = re.sub(r"[\s_]+", "-", s)
    s = re.sub(r"-+", "-", s)
    return s.strip("-")


# ═══════════════════════════════════════════════════════════════
# Main
# ═══════════════════════════════════════════════════════════════

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9803))
    print(f"Inkwell starting on :{port}")
    app.run(host="0.0.0.0", port=port, debug=False)

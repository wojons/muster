#!/usr/bin/env python3
"""
content-hub: Blog/CMS API with Bearer token auth
Listens on :9803
Resources: posts (draft/publish), comments, authors
Auth: 4 tokens — reader-token, author-token, editor-token, admin-token
"""
from flask import Flask, request, jsonify, g
import logging
import sys
import time
import os
import json

app = Flask(__name__)

# ── Auth tokens ──────────────────────────────────────────────────
TOKENS = {
    "reader-token":  {"role": "reader", "user": "reader@hub.local"},
    "author-token":  {"role": "author", "user": "author@hub.local"},
    "editor-token":  {"role": "editor", "user": "editor@hub.local"},
    "admin-token":   {"role": "admin",  "user": "admin@hub.local"},
}

ROLE_HIERARCHY = {"reader": 0, "author": 1, "editor": 2, "admin": 3}

# ── In-memory stores ────────────────────────────────────────────
posts = {}       # id -> {id, title, content, author_id, author_name, tags, status, ...}
comments = {}    # id -> {id, post_id, content, author_name, ...}
authors = {}     # id -> {id, name, email, bio, ...}
_next_ids = {"posts": 1, "comments": 1, "authors": 1}

# ── Logging ─────────────────────────────────────────────────────
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s", stream=sys.stdout)
logger = logging.getLogger("content-hub")


@app.before_request
def authenticate():
    g.start_time = time.time()
    logger.info("--> %s %s", request.method, request.path)

    # No auth for health and spec
    if request.path in ("/health", "/openapi.json"):
        g.role = None
        g.user = None
        return

    auth_header = request.headers.get("Authorization", "")
    if not auth_header.startswith("Bearer "):
        return jsonify({"error": "Missing or invalid Authorization header"}), 401

    token = auth_header[7:]
    token_info = TOKENS.get(token)
    if not token_info:
        return jsonify({"error": "Invalid token"}), 401

    g.role = token_info["role"]
    g.user = token_info["user"]


@app.after_request
def log_response(response):
    elapsed = time.time() - g.get("start_time", time.time())
    logger.info("<-- %s %s %d %.4fs", request.method, request.path, response.status_code, elapsed)
    return response


# ── Helpers ─────────────────────────────────────────────────────

def _paginate(items, default_limit=20, max_limit=100):
    try:
        page = int(request.args.get("page", 1))
    except ValueError:
        page = 1
    try:
        limit = int(request.args.get("per_page", request.args.get("limit", default_limit)))
    except ValueError:
        limit = default_limit
    limit = max(1, min(limit, max_limit))
    offset = (page - 1) * limit
    result = items[offset:offset + limit]
    return {"data": result, "page": page, "per_page": limit, "total": len(items), "pages": max(1, (len(items) + limit - 1) // limit)}


def _require_role(min_role):
    if g.role is None:
        return jsonify({"error": "Authentication required"}), 401
    if ROLE_HIERARCHY.get(g.role, -1) < ROLE_HIERARCHY.get(min_role, 999):
        return jsonify({"error": "Role '{}' required; you have '{}'".format(min_role, g.role)}), 403
    return None


# ═══════════════════════════════════════════════════════════════
#  HEALTH / SPEC
# ═══════════════════════════════════════════════════════════════

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})


@app.route("/openapi.json", methods=["GET"])
def openapi_spec():
    with open(os.path.join(os.path.dirname(__file__), "openapi.json"), "r") as f:
        return jsonify(json.load(f))


# ═══════════════════════════════════════════════════════════════
#  AUTHORS
# ═══════════════════════════════════════════════════════════════

@app.route("/authors", methods=["GET"])
def list_authors():
    err = _require_role("reader")
    if err:
        return err
    result = sorted(authors.values(), key=lambda a: a["id"])
    # Include post_count for each author
    enriched = []
    for a in result:
        author_posts = [p for p in posts.values() if p.get("author_id") == a["id"]]
        entry = dict(a)
        entry["post_count"] = len(author_posts)
        enriched.append(entry)
    return jsonify(_paginate(enriched))


@app.route("/authors", methods=["POST"])
def create_author():
    err = _require_role("admin")
    if err:
        return err
    data = request.get_json(force=True, silent=True)
    if not data or "name" not in data:
        return jsonify({"error": "name is required"}), 400
    aid = _next_ids["authors"]
    _next_ids["authors"] += 1
    author = {
        "id": aid,
        "name": data["name"],
        "email": data.get("email", ""),
        "bio": data.get("bio", ""),
        "avatar_url": data.get("avatar_url", ""),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    authors[aid] = author
    return jsonify(author), 201


@app.route("/authors/<int:author_id>", methods=["GET"])
def get_author(author_id):
    err = _require_role("reader")
    if err:
        return err
    a = authors.get(author_id)
    if a is None:
        return jsonify({"error": "author not found"}), 404
    author_posts = [p for p in posts.values() if p.get("author_id") == author_id and p.get("status") == "published"]
    result = dict(a)
    result["post_count"] = len(author_posts)
    result["posts"] = [{"id": p["id"], "title": p["title"], "status": p["status"], "created_at": p["created_at"]} for p in author_posts]
    return jsonify(result)


@app.route("/authors/<int:author_id>", methods=["PUT"])
def update_author(author_id):
    err = _require_role("admin")
    if err:
        return err
    a = authors.get(author_id)
    if a is None:
        return jsonify({"error": "author not found"}), 404
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("name", "email", "bio", "avatar_url"):
        if field in data:
            a[field] = data[field]
    return jsonify(a)


# ═══════════════════════════════════════════════════════════════
#  POSTS
# ═══════════════════════════════════════════════════════════════

@app.route("/posts", methods=["GET"])
def list_posts():
    err = _require_role("reader")
    if err:
        return err
    result = sorted(posts.values(), key=lambda p: p["id"], reverse=True)

    # Readers only see published; authors see their drafts too; editors/admin see all
    if g.role == "reader":
        result = [p for p in result if p.get("status") == "published"]
    elif g.role == "author":
        result = [p for p in result if p.get("status") == "published" or p.get("author_name") == g.user]

    # Filters
    status = request.args.get("status")
    if status:
        result = [p for p in result if p.get("status") == status]
    author_id = request.args.get("author_id")
    if author_id:
        try:
            aid = int(author_id)
            result = [p for p in result if p.get("author_id") == aid]
        except ValueError:
            pass
    tag = request.args.get("tag")
    if tag:
        result = [p for p in result if tag in p.get("tags", [])]
    q = request.args.get("q")
    if q:
        ql = q.lower()
        result = [p for p in result if ql in p.get("title", "").lower() or ql in p.get("content", "").lower() or ql in " ".join(p.get("tags", [])).lower()]

    return jsonify(_paginate(result))


@app.route("/posts/search", methods=["GET"])
def search_posts():
    """Full-text search across posts."""
    err = _require_role("reader")
    if err:
        return err
    q = (request.args.get("q") or "").lower()
    if not q:
        return jsonify({"data": [], "total": 0, "query": q})
    result = []
    for p in posts.values():
        if p.get("status") != "published" and g.role == "reader":
            continue
        if p.get("status") != "published" and g.role == "author" and p.get("author_name") != g.user:
            continue
        searchable = " ".join([
            p.get("title", ""),
            p.get("content", ""),
            " ".join(p.get("tags", [])),
        ]).lower()
        if q in searchable:
            result.append(p)
    result = sorted(result, key=lambda p: p["id"], reverse=True)
    return jsonify(_paginate(result))


@app.route("/posts", methods=["POST"])
def create_post():
    err = _require_role("author")
    if err:
        return err
    data = request.get_json(force=True, silent=True)
    if not data or "title" not in data:
        return jsonify({"error": "title is required"}), 400
    pid = _next_ids["posts"]
    _next_ids["posts"] += 1
    # Only editor/admin can create as published; author creates as draft
    status = data.get("status", "draft")
    if status == "published" and g.role not in ("editor", "admin"):
        status = "draft"
    post = {
        "id": pid,
        "title": data["title"],
        "content": data.get("content", ""),
        "author_id": data.get("author_id"),
        "author_name": g.user,
        "tags": data.get("tags", []),
        "status": status,
        "excerpt": data.get("excerpt", ""),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    posts[pid] = post
    return jsonify(post), 201


@app.route("/posts/<int:post_id>", methods=["GET"])
def get_post(post_id):
    err = _require_role("reader")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    if p.get("status") != "published" and g.role == "reader":
        return jsonify({"error": "post not found"}), 404
    if p.get("status") != "published" and g.role == "author" and p.get("author_name") != g.user:
        return jsonify({"error": "post not found"}), 404
    return jsonify(p)


@app.route("/posts/<int:post_id>", methods=["PUT"])
def update_post(post_id):
    err = _require_role("author")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    if g.role == "author" and p.get("author_name") != g.user:
        return jsonify({"error": "You can only update your own posts"}), 403
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("title", "content", "author_id", "tags", "excerpt"):
        if field in data:
            p[field] = data[field]
    if "status" in data:
        new_status = data["status"]
        if new_status == "published" and g.role not in ("editor", "admin"):
            return jsonify({"error": "Only editors or admins can publish posts"}), 403
        p["status"] = new_status
    p["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(p)


@app.route("/posts/<int:post_id>", methods=["PATCH"])
def patch_post(post_id):
    err = _require_role("author")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    if g.role == "author" and p.get("author_name") != g.user:
        return jsonify({"error": "You can only update your own posts"}), 403
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    for field in ("title", "content", "author_id", "tags", "excerpt"):
        if field in data:
            p[field] = data[field]
    if "status" in data:
        new_status = data["status"]
        if new_status == "published" and g.role not in ("editor", "admin"):
            return jsonify({"error": "Only editors or admins can publish posts"}), 403
        p["status"] = new_status
    p["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return jsonify(p)


@app.route("/posts/<int:post_id>/publish", methods=["PUT"])
def publish_post(post_id):
    """Dedicated endpoint: transition a post from draft to published (editor/admin only)."""
    err = _require_role("editor")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    if p.get("status") == "published":
        return jsonify({"error": "post is already published"}), 409
    p["status"] = "published"
    p["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    logger.info("Post %d published by %s", post_id, g.user)
    return jsonify(p)


@app.route("/posts/<int:post_id>", methods=["DELETE"])
def delete_post(post_id):
    err = _require_role("author")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    if g.role == "author" and p.get("author_name") != g.user:
        return jsonify({"error": "You can only delete your own posts"}), 403
    cids = [cid for cid, c in comments.items() if c.get("post_id") == post_id]
    for cid in cids:
        del comments[cid]
    del posts[post_id]
    return jsonify({"deleted": True, "comments_deleted": len(cids)})


# ═══════════════════════════════════════════════════════════════
#  COMMENTS (nested under posts)
# ═══════════════════════════════════════════════════════════════

@app.route("/posts/<int:post_id>/comments", methods=["GET"])
def list_post_comments(post_id):
    err = _require_role("reader")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    result = sorted(
        [c for c in comments.values() if c.get("post_id") == post_id],
        key=lambda c: c["id"]
    )
    return jsonify({"data": result, "total": len(result)})


@app.route("/posts/<int:post_id>/comments", methods=["POST"])
def create_comment(post_id):
    err = _require_role("reader")
    if err:
        return err
    p = posts.get(post_id)
    if p is None:
        return jsonify({"error": "post not found"}), 404
    if p.get("status") != "published":
        return jsonify({"error": "cannot comment on unpublished posts"}), 400
    data = request.get_json(force=True, silent=True)
    if not data or "content" not in data:
        return jsonify({"error": "content is required"}), 400
    cid = _next_ids["comments"]
    _next_ids["comments"] += 1
    comment = {
        "id": cid,
        "post_id": post_id,
        "content": data["content"],
        "author_name": data.get("author_name", g.user),
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    comments[cid] = comment
    return jsonify(comment), 201


@app.route("/comments/<int:comment_id>", methods=["GET"])
def get_comment(comment_id):
    err = _require_role("reader")
    if err:
        return err
    c = comments.get(comment_id)
    if c is None:
        return jsonify({"error": "comment not found"}), 404
    return jsonify(c)


@app.route("/comments/<int:comment_id>", methods=["PUT"])
def update_comment(comment_id):
    err = _require_role("reader")
    if err:
        return err
    c = comments.get(comment_id)
    if c is None:
        return jsonify({"error": "comment not found"}), 404
    if g.role == "reader" and c.get("author_name") != g.user:
        return jsonify({"error": "You can only update your own comments"}), 403
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "invalid JSON body"}), 400
    if "content" in data:
        c["content"] = data["content"]
    if "author_name" in data:
        c["author_name"] = data["author_name"]
    return jsonify(c)


@app.route("/comments/<int:comment_id>", methods=["DELETE"])
def delete_comment(comment_id):
    err = _require_role("reader")
    if err:
        return err
    c = comments.get(comment_id)
    if c is None:
        return jsonify({"error": "comment not found"}), 404
    if g.role == "reader" and c.get("author_name") != g.user:
        return jsonify({"error": "You can only delete your own comments"}), 403
    del comments[comment_id]
    return jsonify({"deleted": True})


# ═══════════════════════════════════════════════════════════════

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 9803))
    logger.info("Starting content-hub on :%d", port)
    app.run(host="0.0.0.0", port=port, debug=False)

#!/usr/bin/env python3
"""
Seed script for Content Hub — populates authors, posts, and comments.
Run standalone: python3 seed.py
"""
import json
import sys
import urllib.request

BASE = "http://localhost:9803"
AUTH = "admin-token"  # Seed with admin privileges


def request(method, path, body=None):
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {AUTH}",
    }
    data = json.dumps(body).encode("utf-8") if body else None
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        resp = urllib.request.urlopen(req)
        if method == "DELETE":
            return {"deleted": True}
        return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body_text = e.read().decode() if e.fp else ""
        print(f"  ERROR {method} {path}: HTTP {e.code} — {body_text}", file=sys.stderr)
        return None
    except urllib.error.URLError as e:
        print(f"  ERROR {method} {path}: {e.reason}", file=sys.stderr)
        return None


def post(path, body):
    return request("POST", path, body)


def main():
    print("🌱 Seeding Content Hub...")

    # ── Create authors ──
    print("  Creating authors...")
    alice = post("/authors", {"name": "Alice Chen", "email": "alice@blog.dev", "bio": "Senior backend engineer writing about APIs and Go."})
    if not alice:
        print("ERROR: Could not create first author — is content-hub running?", file=sys.stderr)
        sys.exit(1)
    bob = post("/authors", {"name": "Bob Martinez", "email": "bob@blog.dev", "bio": "Frontend architect exploring design systems."})

    # ── Create posts ──
    print("  Creating posts...")

    # Post 1: Draft by Alice
    p1 = post("/posts", {
        "title": "Building Resilient APIs with Circuit Breakers",
        "content": "Circuit breakers prevent cascading failures in distributed systems. This post explores patterns for Go services using the net/http package with exponential backoff and jitter.",
        "author_id": alice["id"],
        "status": "draft"
    })

    # Post 2: Published by Alice
    p2 = post("/posts", {
        "title": "Why Go Interfaces Are Different",
        "content": "Go interfaces are satisfied implicitly. This design decision has profound implications for how we structure Go programs. Let's explore the philosophy and practical patterns.",
        "author_id": alice["id"],
        "status": "published"
    })

    # Post 3: Draft by Bob
    p3 = post("/posts", {
        "title": "CSS Container Queries in Practice",
        "content": "Container queries finally let us write component-level responsive styles. Here's how to migrate from media queries and what patterns work best.",
        "author_id": bob["id"] if bob else None,
        "status": "draft"
    })

    # Post 4: Published by Bob
    p4 = post("/posts", {
        "title": "Design Tokens for Multi-Platform Consistency",
        "content": "Design tokens bridge the gap between design tools and production code. This guide covers token naming, organization, and automation across web, iOS, and Android.",
        "author_id": bob["id"] if bob else None,
        "status": "published"
    })

    # ── Create comments ──
    print("  Creating comments...")
    if p2:
        post(f"/posts/{p2['id']}/comments", {
            "content": "Great explanation of implicit interfaces. This is the cleanest write-up I've seen.",
            "author_name": "reader@hub.local"
        })
        post(f"/posts/{p2['id']}/comments", {
            "content": "Could you add a section on interface pollution? I see that a lot in production.",
            "author_name": "reader2@email.com"
        })
    if p4:
        post(f"/posts/{p4['id']}/comments", {
            "content": "Design tokens have been a game-changer for our team. We use Style Dictionary for the pipeline.",
            "author_name": "reader3@email.com"
        })

    print(f"  ✅ Seeded {2 if bob else 1} authors, 4 posts, and 3 comments.")
    print(f"     Server: {BASE}")
    print(f"     Tokens: reader-token, author-token, editor-token, admin-token")


if __name__ == "__main__":
    main()

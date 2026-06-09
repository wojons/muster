#!/usr/bin/env python3
"""
Seed script for Task Forge — populates realistic project management data.
Run standalone: python3 seed.py
"""
import json
import sys
import time
import urllib.request

BASE = "http://localhost:9800"


def post(path, body):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(body).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        resp = urllib.request.urlopen(req)
        return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        print(f"  ERROR {path}: HTTP {e.code} — {body}", file=sys.stderr)
        return None
    except urllib.error.URLError as e:
        print(f"  ERROR {path}: {e.reason}", file=sys.stderr)
        return None


def main():
    print("🌱 Seeding Task Forge...")

    # ── Tags ──
    print("  Creating tags...")
    tag_bug = post("/tags", {"name": "bug", "color": "#e74c3c"})
    tag_feature = post("/tags", {"name": "feature", "color": "#2ecc71"})
    tag_ux = post("/tags", {"name": "ux", "color": "#3498db"})
    tag_docs = post("/tags", {"name": "docs", "color": "#9b59b6"})
    tag_perf = post("/tags", {"name": "performance", "color": "#e67e22"})
    tag_security = post("/tags", {"name": "security", "color": "#1abc9c"})
    tag_blocker = post("/tags", {"name": "blocker", "color": "#c0392b"})

    # ── Project 1: SaaS Platform ──
    print("  Creating projects...")
    p1 = post("/projects", {"name": "SaaS Platform v2", "description": "Complete rewrite of the SaaS platform with multi-tenant support and billing integration.", "status": "active"})
    if not p1:
        print("ERROR: Could not create project 1 — is the server running?", file=sys.stderr)
        sys.exit(1)

    # Milestones for p1
    m1_1 = post("/milestones", {"name": "M1: Core Auth", "project_id": p1["id"], "status": "completed", "due_date": "2026-03-15"})
    m1_2 = post("/milestones", {"name": "M2: Billing Integration", "project_id": p1["id"], "status": "active", "due_date": "2026-05-01"})
    m1_3 = post("/milestones", {"name": "M3: Multi-Tenant", "project_id": p1["id"], "status": "pending", "due_date": "2026-07-15"})
    m1_4 = post("/milestones", {"name": "M4: Public Launch", "project_id": p1["id"], "status": "pending", "due_date": "2026-09-01"})

    # Tasks for p1
    t_titles = [
        ("Implement OAuth2 SSO with Google/GitHub", "open", "high", "alice", [tag_feature["id"], tag_security["id"]]),
        ("Design billing database schema", "in_progress", "high", "bob", [tag_feature["id"]]),
        ("Build invoice PDF generation", "open", "medium", "alice", [tag_feature["id"]]),
        ("Fix token refresh race condition", "review", "critical", "charlie", [tag_bug["id"], tag_blocker["id"]]),
        ("Add rate limiting to billing endpoints", "in_progress", "high", "alice", [tag_feature["id"], tag_security["id"]]),
        ("Write API docs for tenant provisioning", "open", "low", "bob", [tag_docs["id"]]),
        ("Optimize tenant isolation query performance", "open", "high", "charlie", [tag_perf["id"]]),
        ("Implement tenant onboarding wizard UI", "open", "medium", "dana", [tag_ux["id"], tag_feature["id"]]),
        ("Audit PII handling for GDPR compliance", "open", "critical", "alice", [tag_security["id"]]),
        ("Set up staging environment for multi-tenant testing", "closed", "medium", "bob", [tag_feature["id"]]),
    ]
    for title, status, priority, assignee, t_tags in t_titles:
        task = post(f"/projects/{p1['id']}/tasks", {
            "title": title, "status": status, "priority": priority,
            "assignee": assignee, "tags": t_tags,
            "milestone_id": m1_2["id"] if "Billing" in title or "invoice" in title.lower() else (m1_1["id"] if "Auth" in title else (m1_3["id"] if "tenant" in title.lower() or "multi-tenant" in title.lower() else None))
        })
        if task:
            post(f"/tasks/{task['id']}/comments", {"content": f"Started working on this — will update EOD.", "author": assignee})
            post(f"/tasks/{task['id']}/comments", {"content": f"Need to sync with team on approach.", "author": "alice" if assignee != "alice" else "bob"})

    # ── Project 2: Mobile App ──
    p2 = post("/projects", {"name": "Mobile App Redesign", "description": "Complete UX overhaul of the iOS and Android apps with dark mode and offline-first architecture.", "status": "active"})
    m2_1 = post("/milestones", {"name": "M1: Design System", "project_id": p2["id"], "status": "completed", "due_date": "2026-04-01"})
    m2_2 = post("/milestones", {"name": "M2: Offline Sync", "project_id": p2["id"], "status": "active", "due_date": "2026-06-01"})

    mobile_tasks = [
        ("Design dark mode color palette", "closed", "medium", "dana", [tag_ux["id"]]),
        ("Implement offline-first data sync layer", "in_progress", "high", "charlie", [tag_feature["id"], tag_perf["id"]]),
        ("Build push notification bridge for Android", "open", "high", "bob", [tag_feature["id"]]),
        ("Fix crash on iOS 18 rotation", "open", "critical", "alice", [tag_bug["id"], tag_blocker["id"]]),
        ("Add haptic feedback to core actions", "open", "low", "dana", [tag_ux["id"]]),
        ("Profile app startup time — target < 2s", "open", "medium", "charlie", [tag_perf["id"]]),
    ]
    for title, status, priority, assignee, t_tags in mobile_tasks:
        post(f"/projects/{p2['id']}/tasks", {"title": title, "status": status, "priority": priority, "assignee": assignee, "tags": t_tags, "milestone_id": m2_1["id"] if "Design" in title or "dark" in title.lower() else (m2_2["id"] if "offline" in title.lower() else None)})

    # ── Project 3: Internal Tools (archived) ──
    p3 = post("/projects", {"name": "Internal Dashboard v1", "description": "First version of the internal analytics dashboard. Completed Q1 2026.", "status": "archived"})
    post("/milestones", {"name": "M1: MVP", "project_id": p3["id"], "status": "completed"})
    post(f"/projects/{p3['id']}/tasks", {"title": "Wire up data sources", "status": "closed", "priority": "medium", "assignee": "bob", "tags": [tag_feature["id"]]})

    print(f"  ✅ Seeded {len(projects := [p1,p2,p3])} projects, tags, milestones, and tasks.")
    print(f"     Server: {BASE}")


if __name__ == "__main__":
    main()

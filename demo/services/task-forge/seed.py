#!/usr/bin/env python3
"""Seed Task Forge with "Acme Corp Website Redesign" project data."""

import json
import urllib.request
import sys

BASE = f"http://localhost:{sys.argv[1]}" if len(sys.argv) > 1 else "http://localhost:9801"

def api(method, path, data=None):
    url = f"{BASE}{path}"
    req = urllib.request.Request(url, method=method)
    if data:
        req.add_header("Content-Type", "application/json")
        req.data = json.dumps(data).encode()
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())

# Create tags
status, ux = api("POST", "/tags", {"name": "ux", "color": "#4A90D9"})
status, backend = api("POST", "/tags", {"name": "backend", "color": "#7ED321"})
status, frontend = api("POST", "/tags", {"name": "frontend", "color": "#F5A623"})
status, bug = api("POST", "/tags", {"name": "bug", "color": "#D0021B"})
status, performance = api("POST", "/tags", {"name": "performance", "color": "#BD10E0"})
print(f"✅ Tags: ux({ux['id']}), backend({backend['id']}), frontend({frontend['id']}), bug({bug['id']}), performance({performance['id']})")

# Create project
status, proj = api("POST", "/projects", {
    "name": "Acme Corp Website Redesign",
    "description": "Complete overhaul of the corporate website with new branding, improved UX, and modern tech stack.",
    "tags": [ux["id"], frontend["id"], backend["id"]]
})
pid = proj["id"]
print(f"✅ Project: {proj['name']} ({pid})")

# Create milestones
status, m1 = api("POST", f"/projects/{pid}/milestones", {
    "name": "Phase 1: Design & Prototypes", "due_date": "2026-07-01", "status": "completed"
})
status, m2 = api("POST", f"/projects/{pid}/milestones", {
    "name": "Phase 2: Core Development", "due_date": "2026-08-15", "status": "in_progress"
})
status, m3 = api("POST", f"/projects/{pid}/milestones", {
    "name": "Phase 3: Polish & Launch", "due_date": "2026-09-30", "status": "planned"
})
print(f"✅ Milestones: {m1['name']}, {m2['name']}, {m3['name']}")

# Tasks with real-sounding names
tasks_data = [
    # Phase 1 — done
    {"title": "Stakeholder interviews", "description": "Interview 5 department heads about needs and pain points with current site.", "status": "done", "assignee": "alice@acme.com", "milestone_id": m1["id"], "tags": [ux["id"]]},
    {"title": "Competitive analysis", "description": "Review 3 competitor sites: Tesla, Apple, Stripe. Document UX patterns, IA, and tech stack clues.", "status": "done", "assignee": "bob@acme.com", "milestone_id": m1["id"], "tags": [ux["id"], frontend["id"]]},
    {"title": "Wireframes — homepage", "description": "Desktop and mobile wireframes for new homepage layout. 3 variants for A/B consideration.", "status": "done", "assignee": "alice@acme.com", "milestone_id": m1["id"], "tags": [ux["id"]]},
    {"title": "Design system audit", "description": "Audit current CSS/components. Identify what to keep, refactor, or replace.", "status": "done", "assignee": "carol@acme.com", "milestone_id": m1["id"], "tags": [frontend["id"]]},
    {"title": "Tech stack evaluation", "description": "Evaluate Next.js vs Astro vs plain React for rebuild. Document pros/cons with perf benchmarks.", "status": "done", "assignee": "bob@acme.com", "milestone_id": m1["id"], "tags": [backend["id"], frontend["id"]]},
    # Phase 2 — in progress
    {"title": "Set up CI/CD pipeline", "description": "GitHub Actions: lint, test, build, deploy preview on PR. Notify Slack on failure.", "status": "in_progress", "assignee": "bob@acme.com", "milestone_id": m2["id"], "tags": [backend["id"]]},
    {"title": "Homepage hero section", "description": "Build new hero with animated gradient background and headline CMS integration.", "status": "in_progress", "assignee": "carol@acme.com", "milestone_id": m2["id"], "tags": [frontend["id"]]},
    {"title": "Navigation redesign", "description": "New mega-menu navigation with search. Must work on mobile (hamburger) and desktop.", "status": "in_progress", "assignee": "carol@acme.com", "milestone_id": m2["id"], "tags": [ux["id"], frontend["id"]]},
    {"title": "API endpoints for CMS", "description": "Build content API: /pages, /sections, /media. Pagination, draft/preview support.", "status": "todo", "assignee": "bob@acme.com", "milestone_id": m2["id"], "tags": [backend["id"]]},
    {"title": "Blog migration script", "description": "Export WordPress → new CMS. Handle 200+ posts with images, tags, and redirects.", "status": "todo", "assignee": "alice@acme.com", "milestone_id": m2["id"], "tags": [backend["id"]]},
    {"title": "Search indexing pipeline", "description": "Set up Meilisearch/Elasticsearch for site-wide search. Index content, products, docs.", "status": "todo", "assignee": "bob@acme.com", "milestone_id": m2["id"], "tags": [backend["id"], performance["id"]]},
    {"title": "Footer component", "description": "New footer with sitemap links, social icons, newsletter signup, copyright.", "status": "todo", "assignee": "carol@acme.com", "milestone_id": m2["id"], "tags": [frontend["id"]]},
    # Phase 3 — planned
    {"title": "Cross-browser testing", "description": "Test on Chrome, Firefox, Safari, Edge. Document rendering differences. Fix P0 issues.", "status": "todo", "assignee": "alice@acme.com", "milestone_id": m3["id"], "tags": [frontend["id"], bug["id"]]},
    {"title": "Lighthouse audit", "description": "Target scores: Performance 90+, Accessibility 95+, SEO 100. Fix flagged issues.", "status": "todo", "assignee": "bob@acme.com", "milestone_id": m3["id"], "tags": [performance["id"], frontend["id"]]},
    {"title": "Launch checklist & go/no-go", "description": "Final review: DNS, SSL, redirects, monitoring, rollback plan. Sign-off from VP.", "status": "todo", "assignee": "alice@acme.com", "milestone_id": m3["id"], "tags": [ux["id"]]},
]

for t in tasks_data:
    status, task = api("POST", f"/projects/{pid}/tasks", t)
    print(f"  ✅ {task['title']} ({task['id']}) [{task['status']} → {task['assignee']}]")

# Comments on the in-progress tasks
comments_data = [
    ("CI/CD pipeline task", "Pipeline is green on first run. Need to add secrets for staging deploy."),
    ("CI/CD pipeline task", "Added staging deploy. Waiting on @carol to confirm preview URL convention."),
    ("CI/CD pipeline task", "Using preview-{branch}.acme.dev pattern. Pipeline updated."),
    ("CI/CD pipeline task", "LGTM. Merging."),
    ("Homepage hero section", "First pass is up on preview. Check the gradient — it banding on Chrome."),
    ("Homepage hero section", "Fixed Chrome banding. Added dithering layer. Preview updated."),
    ("Homepage hero section", "Much better. Can we add the CMS admin preview toggle?"),
    ("Homepage hero section", "Added toggle. Works but need to wire CMS save button — separate task."),
    ("Navigation redesign", "Mega-menu looks great on desktop. Mobile hamburger animation is janky."),
    ("Navigation redesign", "Changed to CSS transform instead of JS animation. Smooth now."),
]

# Get in-progress tasks for commenting
status, task_list = api("GET", f"/projects/{pid}/tasks?status=in_progress")
task_map = {t["id"]: t["title"] for t in task_list}

for task_name_prefix, body in comments_data:
    for tid, title in task_map.items():
        if title.startswith(task_name_prefix.split(" ")[0]):
            api("POST", f"/tasks/{tid}/comments", {"author": "alice@acme.com", "body": body})

print(f"\n✅ Seeded: 1 project, 3 milestones, {len(tasks_data)} tasks, {len(comments_data)} comments")

#!/usr/bin/env python3
"""Seed ChainLink with "User Onboarding" and "Data Pipeline" workflows."""

import json
import urllib.request
import sys

BASE = f"http://localhost:{sys.argv[1]}" if len(sys.argv) > 1 else "http://localhost:9805"


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


# ── Pre-register webhook callbacks ────────────────────────────

status, slack_wh = api("POST", "/webhooks", {
    "name": "Slack #eng-notify",
    "url": "https://hooks.slack.com/services/demo/chainlink/execution-done",
    "events": ["execution.done", "execution.failed"],
    "secret": "slack-demo-secret"
})
print(f"✅ Webhook: {slack_wh['name']} ({slack_wh['id']})")

status, audit_wh = api("POST", "/webhooks", {
    "name": "Audit Logger",
    "url": "https://audit.internal.example.com/chainlink-events",
    "events": ["execution.*"],
    "secret": "audit-demo-secret"
})
print(f"✅ Webhook: {audit_wh['name']} ({audit_wh['id']})")

status, retry_wh = api("POST", "/webhooks", {
    "name": "On-Call PagerDuty",
    "url": "https://events.pagerduty.com/v2/enqueue",
    "events": ["execution.failed"],
    "secret": "pd-demo-secret"
})
print(f"✅ Webhook: {retry_wh['name']} ({retry_wh['id']})")

# ── Workflow 1: User Onboarding (4 steps) ─────────────────────

status, wf1 = api("POST", "/workflows", {
    "name": "User Onboarding",
    "description": "Provision new employee accounts across all internal systems: email, HR, GitHub, and Slack."
})
wf1_id = wf1["id"]
print(f"✅ Workflow: {wf1['name']} ({wf1_id})")

steps_wf1 = [
    {
        "name": "Create Email Account",
        "description": "Provision Gmail inbox via GSuite Admin API. Set up forwarding rules and signature template.",
        "order": 1,
        "retry_count": 3,
        "timeout_seconds": 30
    },
    {
        "name": "Create HR Profile",
        "description": "Register employee in BambooHR: personal info, department, manager, start date. Trigger background check workflow.",
        "order": 2,
        "depends_on": None,  # Parallel with step 1 conceptually, but sequential in chain
        "retry_count": 2,
        "timeout_seconds": 45
    },
    {
        "name": "Provision GitHub Access",
        "description": "Add user to Acme GitHub org, assign team membership based on department, grant repo permissions.",
        "order": 3,
        "retry_count": 3,
        "timeout_seconds": 30
    },
    {
        "name": "Invite to Slack & Channels",
        "description": "Send Slack invite, auto-join #general, #team-{department}, #new-hires. Set display name and timezone.",
        "order": 4,
        "retry_count": 2,
        "timeout_seconds": 20
    }
]

for s in steps_wf1:
    status, step = api("POST", f"/workflows/{wf1_id}/steps", s)
    print(f"  ✅ Step {step['order']}: {step['name']} ({step['id']}) [retry={step['retry_count']}]")

# ── Workflow 2: Data Pipeline (3 steps) ───────────────────────

status, wf2 = api("POST", "/workflows", {
    "name": "Data Pipeline",
    "description": "Nightly ETL pipeline: extract from Postgres, transform with dbt, load into analytics warehouse."
})
wf2_id = wf2["id"]
print(f"✅ Workflow: {wf2['name']} ({wf2_id})")

steps_wf2 = [
    {
        "name": "Extract from Production DB",
        "description": "Run pg_dump queries against production replica. Extract orders, users, events tables. Compress output.",
        "order": 1,
        "retry_count": 2,
        "timeout_seconds": 120
    },
    {
        "name": "Transform with dbt",
        "description": "Run dbt models: staging, intermediate, marts. Materialize fact_sales, dim_customers, dim_products.",
        "order": 2,
        "retry_count": 3,
        "timeout_seconds": 300
    },
    {
        "name": "Load into Analytics Warehouse",
        "description": "INSERT into ClickHouse analytics cluster. Refresh materialized views. Validate row counts vs source.",
        "order": 3,
        "retry_count": 2,
        "timeout_seconds": 180
    }
]

for s in steps_wf2:
    status, step = api("POST", f"/workflows/{wf2_id}/steps", s)
    print(f"  ✅ Step {step['order']}: {step['name']} ({step['id']}) [retry={step['retry_count']}]")

print(f"\n✅ Seeded: 2 workflows, {len(steps_wf1) + len(steps_wf2)} steps, 3 webhooks")
print(f"   User Onboarding: {wf1_id}")
print(f"   Data Pipeline:   {wf2_id}")

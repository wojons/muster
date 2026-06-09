#!/usr/bin/env python3
"""
Seed data for orchestrator (port 9805).

Creates:
  - 1 workflow with 3 steps
  - 1 completed execution for that workflow

Usage:
  python seed.py                  # dry-run, shows seed data structure
  python seed.py --http           # POST to http://localhost:9805
  python seed.py --http URL       # POST to custom URL
"""
import sys
import json
import time
import os

# Allow direct in-process seeding when run as `python seed.py` (no --http)
# This lets tests import seed_data() and call it before app startup.

WORKFLOW = {
    "name": "User Onboarding",
    "description": "New user registration pipeline: validate, provision, notify",
    "status": "active",
    "steps": [
        {
            "name": "Validate Email",
            "type": "task",
            "config": {
                "action": "email_validation",
                "timeout_seconds": 30,
            },
        },
        {
            "name": "Provision Account",
            "type": "task",
            "config": {
                "action": "account_provisioning",
                "region": "us-east-1",
            },
        },
        {
            "name": "Send Welcome",
            "type": "notification",
            "config": {
                "channel": "email",
                "template": "welcome_v2",
            },
        },
    ],
}

TRIGGER = {
    "name": "user-signup",
    "workflow_id": 1,
}

WEBHOOK_PAYLOAD = {
    "user_id": "usr_a1b2c3d4",
    "email": "newuser@example.com",
    "plan": "pro",
}


def seed_data(workflows_store, executions_store, triggers_store, next_ids):
    """Populate in-memory stores with seed data.

    Call this before app.run() when running in-process.
    """
    # ── Workflow ────────────────────────────────────────────
    wid = next_ids["workflows"]
    next_ids["workflows"] += 1

    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    steps = []
    for idx, s in enumerate(WORKFLOW["steps"]):
        steps.append({
            "id": idx + 1,
            "name": s["name"],
            "type": s.get("type", "task"),
            "config": s.get("config", {}),
            "order": idx + 1,
        })

    workflow = {
        "id": wid,
        "name": WORKFLOW["name"],
        "description": WORKFLOW["description"],
        "status": WORKFLOW["status"],
        "steps": steps,
        "created_at": now,
        "updated_at": now,
    }
    workflows_store[wid] = workflow

    # ── Trigger ─────────────────────────────────────────────
    tid = next_ids["triggers"]
    next_ids["triggers"] += 1
    trigger = {
        "id": tid,
        "name": TRIGGER["name"],
        "workflow_id": TRIGGER["workflow_id"],
        "created_at": now,
    }
    triggers_store[TRIGGER["name"]] = trigger

    # ── Completed Execution ─────────────────────────────────
    eid = next_ids["executions"]
    next_ids["executions"] += 1

    step_results = [
        {
            "step_id": 1,
            "name": "Validate Email",
            "type": "task",
            "status": "completed",
            "started_at": now,
            "completed_at": now,
            "output": {"result": "email_valid", "email": "newuser@example.com"},
            "error": None,
        },
        {
            "step_id": 2,
            "name": "Provision Account",
            "type": "task",
            "status": "completed",
            "started_at": now,
            "completed_at": now,
            "output": {"result": "account_created", "account_id": "acc_9876"},
            "error": None,
        },
        {
            "step_id": 3,
            "name": "Send Welcome",
            "type": "notification",
            "status": "completed",
            "started_at": now,
            "completed_at": now,
            "output": {"result": "notification_sent", "recipient": "newuser@example.com"},
            "error": None,
        },
    ]

    execution = {
        "id": eid,
        "workflow_id": wid,
        "workflow_name": WORKFLOW["name"],
        "status": "completed",
        "progress": 100,
        "current_step": None,
        "steps": step_results,
        "triggered_by": TRIGGER["name"],
        "payload": WEBHOOK_PAYLOAD,
        "created_at": now,
        "started_at": now,
        "completed_at": now,
    }
    executions_store[eid] = execution

    return {
        "workflow_id": wid,
        "execution_id": eid,
        "trigger_name": TRIGGER["name"],
    }


def seed_via_http(base_url="http://localhost:9805"):
    """POST seed data to a running orchestrator instance."""
    import urllib.request
    import urllib.error

    def _post(path, body):
        url = f"{base_url}{path}"
        data = json.dumps(body).encode("utf-8")
        req = urllib.request.Request(
            url, data=data, method="POST",
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req) as resp:
                return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            print("  HTTP {} from {}: {}".format(e.code, url, e.read().decode()))
            return None

    print("Seeding orchestrator at {} ...".format(base_url))

    # Create workflow
    w = _post("/workflows", WORKFLOW)
    if not w:
        print("  FAILED to create workflow")
        return None
    wid = w["id"]
    print("  Created workflow id={}: {}".format(wid, w["name"]))

    # Create trigger
    trigger_body = {"name": TRIGGER["name"], "workflow_id": wid}
    t = _post("/triggers", trigger_body)
    if not t:
        print("  FAILED to create trigger")
        return None
    print("  Created trigger: {} -> workflow {}".format(t["name"], t["workflow_id"]))

    # Trigger webhook to create execution
    ex = _post("/triggers/{}/webhook".format(TRIGGER["name"]), WEBHOOK_PAYLOAD)
    if not ex:
        print("  FAILED to trigger execution")
        return None
    eid = ex.get("execution_id")
    print("  Created execution id={}: status={}, progress={}".format(
        eid, ex.get("status"), ex.get("progress")))

    print("  Seed complete.")
    return {"workflow_id": wid, "execution_id": eid, "trigger_name": TRIGGER["name"]}


if __name__ == "__main__":
    if "--http" in sys.argv:
        idx = sys.argv.index("--http")
        if idx + 1 < len(sys.argv) and not sys.argv[idx + 1].startswith("--"):
            url = sys.argv[idx + 1]
        else:
            url = "http://localhost:9805"
        result = seed_via_http(url)
        if result:
            print(json.dumps(result, indent=2))
        else:
            sys.exit(1)
    else:
        # In-process seeding — demonstrate usage
        sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
        print("In-process seed mode: call seed_data(workflows, executions, triggers, next_ids)")
        print("from Python to populate stores before app.run().")
        print()
        print("Example:")
        print("  from server import workflows, executions, triggers, _next_ids")
        print("  from seed import seed_data")
        print("  seed_data(workflows, executions, triggers, _next_ids)")
        print()
        # Dry-run: simulate
        w_store = {}
        e_store = {}
        t_store = {}
        nids = {"workflows": 1, "executions": 1, "triggers": 1}
        result = seed_data(w_store, e_store, t_store, nids)
        print("Dry-run result:")
        print(json.dumps(result, indent=2))
        print("  workflows: {}, executions: {}, triggers: {}".format(
            len(w_store), len(e_store), len(t_store)))

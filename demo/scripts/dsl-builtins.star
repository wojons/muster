# demo/scripts/dsl-builtins.star — DSL builtins against the Task Forge demo
# service (docs/guides/starlark-dsl.md, "Runnable Example" section).
#
# Exercises the three builtin call shapes:
#   1. a dict body, marshalled to JSON by the http_post builtin,
#   2. the json() encoder used to pre-encode the payload (string body), and
#   3. an optional headers dict passed to http_get.
#
# Prerequisites — the seed creates project 1, whose task endpoint receives the
# POSTs below (unseeded, they would return 404):
#   python3 demo/services/task-forge/server.py 9801        # in another shell
#   python3 demo/services/task-forge/seed.py 9801
#
# Task Forge POST /projects/1/tasks requires a non-empty "title"; "priority"
# is optional (server default: "medium"). POST returns 201, GET /tasks 200.
#
# Note: each result gets a distinct global name — the DSL interpreter does not
# allow reassigning an existing top-level binding with a second `=`.
#
# Run with:
#   openapi-cli dsl run demo/scripts/dsl-builtins.star

base = "http://localhost:9801"

# 1. dict body — the builtin marshals this to {"title": "...", "priority": "low"}
dict_status, dict_body = http_post(base + "/projects/1/tasks", {"title": "Wire the DSL builtins example", "priority": "low"})
print("dict body           -> status", dict_status)

# 2. json() encoder — pre-encode the payload, then send the resulting string
payload = json({"title": "Document the json() encoder", "priority": "low"})
json_status, json_body = http_post(base + "/projects/1/tasks", payload)
print("json() encoded body -> status", json_status)

# 3. headers dict — optional headers on http_get
hdr_status, hdr_body = http_get(base + "/tasks", {"X-Dsl-Example": "headers"})
print("headers dict        -> status", hdr_status)

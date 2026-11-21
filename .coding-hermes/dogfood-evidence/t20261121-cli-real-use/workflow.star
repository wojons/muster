base = "http://localhost:9801"

# 1. Health check first (what any real operator script does)
status, body = http_get(base + "/health")
print("health:", status)

# 2. Create a task with a DICT body -- the shape DF-MUSTER-7 just added
new_task = {"title": "DSL created this", "priority": "high", "description": "from starlark"}
status, body = http_post(base + "/projects/1/tasks", new_task)
print("create:", status, body[:80])

# 3. Same thing with the json() encoder, explicitly
payload = json({"title": "encoded by json()", "priority": "low"})
print("encoded:", payload)
status, body = http_post(base + "/projects/1/tasks", payload)
print("create2:", status)

# 4. Send a headers dict (DF-037 surface)
status, body = http_get(base + "/tasks", {"Accept": "application/json", "X-Dogfood": "yes"})
print("list with headers:", status)

# 5. A list body should marshal to a JSON array
status, body = http_post(base + "/tags", ["alpha", "beta"])
print("list body:", status, body[:80])

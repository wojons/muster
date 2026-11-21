base = "http://localhost:9801"

def main():
    status, body = http_get(base + "/health")
    print("health:", status)

    new_task = {"title": "DSL created this", "priority": "high", "description": "from starlark"}
    status, body = http_post(base + "/projects/1/tasks", new_task)
    print("create:", status, body[:70])

    payload = json({"title": "encoded by json()", "priority": "low"})
    print("encoded:", payload)
    status, body = http_post(base + "/projects/1/tasks", payload)
    print("create2:", status)

    status, body = http_get(base + "/tasks", {"Accept": "application/json", "X-Dogfood": "yes"})
    print("list with headers:", status)

    status, body = http_post(base + "/tags", ["alpha", "beta"])
    print("list body:", status, body[:70])

main()

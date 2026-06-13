# Demo DSL script: Task Forge workflow chaining via Starlark
# Uses HTTP builtins: http_get, http_post, http_put, http_delete
# Each returns (status_code, response_body)

BASE = "http://localhost:9801"

# Step 1: Create a project
c1, proj = http_post(BASE + "/projects", '{"name":"DSL Project","description":"Created via Starlark DSL"}')
print("CREATE PROJECT:", c1, proj)

# Step 2: Create tasks under the project
c2, t1 = http_post(BASE + "/projects/1/tasks", '{"title":"Design DSL pipeline"}')
print("CREATE TASK 1:", c2, t1)

c3, t2 = http_post(BASE + "/projects/1/tasks", '{"title":"Implement HTTP builtins"}')
print("CREATE TASK 2:", c3, t2)

# Step 3: List all tasks
c4, tasks = http_get(BASE + "/tasks")
print("LIST TASKS:", c4, tasks)

# Step 4: Update a task
c5, upd = http_put(BASE + "/tasks/1", '{"title":"Design DSL pipeline","done":true}')
print("UPDATE TASK:", c5, upd)

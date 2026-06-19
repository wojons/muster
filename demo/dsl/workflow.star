<<<<<<< Updated upstream
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
=======
# workflow.star — Starlark DSL workflow for Task Forge API
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1

def main():
    # Step 1: Create a project
    project_body = '{"name": "DSL Workflow Project", "description": "Created via Starlark DSL"}'
    project = http_post("http://127.0.0.1:9801/projects", body = project_body)
    print("CREATE PROJECT:", project)

    # Step 2: Create a task in the project (extract project id from response)
    # The response is JSON; we use a simple approach: hardcode the first project id
    task_body = '{"title": "DSL-created task", "description": "Chained via Starlark DSL", "priority": "high"}'
    task = http_post("http://127.0.0.1:9801/projects/1/tasks", body = task_body)
    print("CREATE TASK:", task)

    # Step 3: List all tasks
    tasks = http_get("http://127.0.0.1:9801/tasks")
    print("LIST TASKS:", tasks)

main()
>>>>>>> Stashed changes

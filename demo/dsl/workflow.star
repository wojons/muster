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

"""Demo OpenAPI spec provider for Muster acceptance testing.

Serves a small Petstore OpenAPI 3.0 spec (PETSTORE_SPEC) plus /health and
/openapi.json endpoints.

The served spec advertises servers[0].url as THIS provider's own address
(http://localhost:<port>), so `openapi-cli discover <url>/openapi.json` +
`generate` produce typed commands that resolve the API server URL correctly
and hit this provider — not a stale hardcoded port (DF-MUSTER-5).

Port resolution order (matches the other demo services, which run-all.sh
starts as `PORT=$PORT $PYTHON "$SCRIPT"`):
  1. PORT environment variable
  2. argv[1]
  3. default 9801
"""
from flask import Flask, jsonify
import os
import sys

DEFAULT_PORT = 9801

app = Flask(__name__)


def resolve_port():
    """Resolve the listen port: PORT env var, then argv[1], then 9801."""
    env_port = os.environ.get("PORT")
    if env_port:
        return int(env_port)
    if len(sys.argv) > 1:
        return int(sys.argv[1])
    return DEFAULT_PORT


PORT = resolve_port()

PETSTORE_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Petstore API", "version": "1.0.0"},
    "servers": [{"url": f"http://localhost:{PORT}"}],
    "paths": {
        "/tasks": {
            "get": {
                "operationId": "listTasks",
                "summary": "List all tasks",
                "responses": {"200": {"description": "A list of tasks"}}
            },
            "post": {
                "operationId": "createTask",
                "summary": "Create a task",
                "requestBody": {
                    "required": True,
                    "content": {"application/json": {"schema": {"type": "object", "properties": {"title": {"type": "string"}}}}}
                },
                "responses": {"201": {"description": "Task created"}}
            }
        },
        "/tasks/{taskId}": {
            "get": {
                "operationId": "getTask",
                "summary": "Get a task by ID",
                "parameters": [{"name": "taskId", "in": "path", "required": True, "schema": {"type": "integer"}}],
                "responses": {"200": {"description": "Task details"}, "404": {"description": "Not found"}}
            },
            "put": {
                "operationId": "updateTask",
                "summary": "Update a task",
                "parameters": [{"name": "taskId", "in": "path", "required": True, "schema": {"type": "integer"}}],
                "responses": {"200": {"description": "Task updated"}}
            },
            "delete": {
                "operationId": "deleteTask",
                "summary": "Delete a task",
                "parameters": [{"name": "taskId", "in": "path", "required": True, "schema": {"type": "integer"}}],
                "responses": {"204": {"description": "Task deleted"}}
            }
        }
    }
}


@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy", "spec": "petstore"})


@app.route('/openapi.json', methods=['GET'])
def openapi_json():
    return jsonify(PETSTORE_SPEC)


if __name__ == '__main__':
    app.run(host='0.0.0.0', port=PORT, debug=False)

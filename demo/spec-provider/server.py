"""Demo OpenAPI spec provider for Muster acceptance testing — port 9801."""
from flask import Flask, jsonify
import sys

app = Flask(__name__)

PETSTORE_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Petstore API", "version": "1.0.0"},
    "servers": [{"url": "http://localhost:9800"}],
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
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9801
    app.run(host='0.0.0.0', port=port, debug=False)

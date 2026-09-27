"""Demo CRUD service for Muster acceptance testing — default port 9800.

Port resolution order (matches the other demo services):
  1. argv[1]
  2. PORT environment variable
  3. default 9800

An invalid or extra argument exits with status 2 and a usage line.
"""
from flask import Flask, request, jsonify
import os
import sys

DEFAULT_PORT = 9800

app = Flask(__name__)
tasks = []
task_id_counter = [1]

@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy", "tasks_count": len(tasks)})

@app.route('/tasks', methods=['GET'])
def list_tasks():
    return jsonify(tasks)

@app.route('/tasks', methods=['POST'])
def create_task():
    data = request.get_json(force=True)
    task = {"id": task_id_counter[0], "title": data.get("title", "untitled"), "status": "pending"}
    task_id_counter[0] += 1
    tasks.append(task)
    return jsonify(task), 201

@app.route('/tasks/<int:task_id>', methods=['GET'])
def get_task(task_id):
    for t in tasks:
        if t['id'] == task_id:
            return jsonify(t)
    return jsonify({"error": "not found"}), 404

@app.route('/tasks/<int:task_id>', methods=['PUT'])
def update_task(task_id):
    data = request.get_json(force=True)
    for t in tasks:
        if t['id'] == task_id:
            t['title'] = data.get('title', t['title'])
            t['status'] = data.get('status', t['status'])
            return jsonify(t)
    return jsonify({"error": "not found"}), 404

@app.route('/tasks/<int:task_id>', methods=['DELETE'])
def delete_task(task_id):
    global tasks
    tasks = [t for t in tasks if t['id'] != task_id]
    return '', 204

if __name__ == '__main__':
    usage = "usage: server.py [port]   (default: 9800; PORT environment variable also honored)"
    if len(sys.argv) > 2:
        sys.stderr.write(f"error: unexpected arguments {' '.join(sys.argv[2:])}\n{usage}\n")
        raise SystemExit(2)
    if len(sys.argv) == 2:
        try:
            port = int(sys.argv[1])
        except ValueError:
            sys.stderr.write(f"error: invalid port {sys.argv[1]!r}\n{usage}\n")
            raise SystemExit(2)
    else:
        port = int(os.environ.get('PORT', DEFAULT_PORT))
    app.run(host='0.0.0.0', port=port, debug=False)

"""Demo CRUD service for Muster acceptance testing — port 9800."""
from flask import Flask, request, jsonify
import sys

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
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9800
    app.run(host='0.0.0.0', port=port, debug=False)

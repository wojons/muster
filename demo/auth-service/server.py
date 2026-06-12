"""Demo auth service for Muster acceptance testing — port 9803."""
from flask import Flask, jsonify, request
import sys

app = Flask(__name__)
VALID_TOKEN = "demo-token-12345"

@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy"})

@app.route('/token', methods=['POST'])
def token():
    return jsonify({"access_token": VALID_TOKEN, "token_type": "bearer"})

@app.route('/tasks', methods=['GET'])
def list_tasks():
    auth = request.headers.get('Authorization', '')
    if not auth.startswith('Bearer '):
        return jsonify({"error": "unauthorized"}), 401
    token = auth.split(' ')[1]
    if token != VALID_TOKEN:
        return jsonify({"error": "invalid token"}), 401
    return jsonify([{"id": 1, "title": "secure task"}])

if __name__ == '__main__':
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9803
    app.run(host='0.0.0.0', port=port, debug=False)

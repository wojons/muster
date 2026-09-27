"""Demo auth service for Muster acceptance testing — default port 9803.

Port resolution order (matches the other demo services):
  1. argv[1]
  2. PORT environment variable
  3. default 9803

An invalid or extra argument exits with status 2 and a usage line.
"""
from flask import Flask, jsonify, request
import os
import sys

DEFAULT_PORT = 9803

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
    usage = "usage: server.py [port]   (default: 9803; PORT environment variable also honored)"
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

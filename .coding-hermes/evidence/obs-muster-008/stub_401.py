#!/usr/bin/env python3
"""401 stub upstream for the OBS-MUSTER-008 live .prom probe."""
import http.server

class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = b'{"error":"unauthorized"}'
        self.send_response(401)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass

http.server.HTTPServer(("127.0.0.1", 40181), H).serve_forever()

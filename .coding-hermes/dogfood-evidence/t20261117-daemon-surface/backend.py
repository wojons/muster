"""Echo backend for muster daemon dogfood: reports exactly what it received."""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Echo(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _reply(self):
        body = json.dumps(
            {
                "method": self.command,
                "path": self.path,
                "authorization": self.headers.get("Authorization", "<absent>"),
                "x_api_key": self.headers.get("X-API-Key", "<absent>"),
                "content_length": self.headers.get("Content-Length", "<absent>"),
                "n_headers": len(self.headers),
            },
            sort_keys=True,
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    do_GET = _reply
    do_POST = _reply
    do_HEAD = _reply

    def log_message(self, fmt, *args):  # keep stderr clean
        sys.stderr.write("backend: " + fmt % args + "\n")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 19101
    ThreadingHTTPServer(("127.0.0.1", port), Echo).serve_forever()

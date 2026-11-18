"""Echo backend v2: reports body bytes and every header it received."""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Echo(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _reply(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n) if n else b""
        payload = json.dumps(
            {
                "method": self.command,
                "path": self.path,
                "body_len": len(body),
                "body_text": body[:200].decode("utf-8", "replace"),
                "headers": {k.lower(): v for k, v in self.headers.items()},
            },
            sort_keys=True,
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(payload)

    do_GET = _reply
    do_POST = _reply
    do_PUT = _reply
    do_PATCH = _reply
    do_DELETE = _reply
    do_HEAD = _reply

    def log_message(self, fmt, *args):
        sys.stderr.write("backend2: " + fmt % args + "\n")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 19102
    ThreadingHTTPServer(("127.0.0.1", port), Echo).serve_forever()

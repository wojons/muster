"""Echo backend v3: correct body accounting for Content-Length AND chunked requests."""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def read_exact(rfile, n):
    return rfile.read(n) if n else b""


def read_chunked(rfile):
    out = b""
    while True:
        line = rfile.readline(1024).strip()
        if not line:
            break
        size = int(line.split(b";")[0], 16)
        if size == 0:
            # consume trailers up to blank line
            while True:
                t = rfile.readline(1024)
                if t in (b"\r\n", b"\n", b""):
                    break
            break
        out += rfile.read(size)
        rfile.read(2)  # CRLF
    return out


class Echo(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _reply(self):
        hdrs = {k.lower(): v for k, v in self.headers.items()}
        te = hdrs.get("transfer-encoding", "").lower()
        if "chunked" in te:
            body = read_chunked(self.rfile)
            mode = "chunked"
        else:
            n = int(hdrs.get("content-length") or 0)
            body = read_exact(self.rfile, n)
            mode = "content-length" if n else "none"
        payload = json.dumps(
            {
                "method": self.command,
                "path": self.path,
                "body_mode": mode,
                "body_len": len(body),
                "body_text": body[:120].decode("utf-8", "replace"),
                "headers": hdrs,
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
        sys.stderr.write("backend3: " + fmt % args + "\n")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 19103
    ThreadingHTTPServer(("127.0.0.1", port), Echo).serve_forever()

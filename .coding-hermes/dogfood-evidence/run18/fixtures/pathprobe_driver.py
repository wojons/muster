"""Drive the path-shape probe end to end: spawn the recorder server as a thread,
fire curl and the muster CLI at 'http://127.0.0.1:PORT/ ' (trailing space),
record the request paths each arrived at, print the JSON evidence."""
import http.server
import threading
import json
import subprocess

HITS = []

class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        ln = int(self.headers.get("Content-Length", 0))
        self.rfile.read(ln)
        HITS.append({"path": self.path})
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')
    def log_message(self, *a):
        pass

srv = http.server.HTTPServer(("127.0.0.1", 9871), H)
th = threading.Thread(target=srv.serve_forever, daemon=True)
th.start()

subprocess.run([
    "curl", "-s", "-m", "3", "-o", "/dev/null",
    "-X", "POST", "http://127.0.0.1:9871/ ", "-d", "x",
], capture_output=True)

cli = subprocess.run([
    "/home/kara/wojons/muster/bin/openapi-cli",
    "protocol", "soap",
    "--wsdl", "/tmp/mf17/echo.wsdl",
    "--operation", "EchoService-Echo",
    "--args", '{"message":"pathprobe"}',
    "--endpoint", "http://127.0.0.1:9871/ ",
], capture_output=True, text=True, timeout=30)

print(json.dumps({
    "hits": HITS,
    "cli_rc": cli.returncode,
    "cli_err_tail": (cli.stderr or cli.stdout).strip().splitlines()[-1:] ,
}, indent=2))
srv.shutdown()

"""Readiness probe cost: a backend that accepts connections and never answers."""
import socket
import sys
import time

port = int(sys.argv[1]) if len(sys.argv) > 1 else 19140
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", port))
s.listen(64)
sys.stderr.write("blackhole listening on %d\n" % port)
conns = []
while True:
    c, _ = s.accept()
    conns.append(c)  # accept, never read, never answer
    time.sleep(0.01)

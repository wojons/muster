"""Minimal raw HTTP/1.1 backend: TCP_NODELAY on, whole response written in ONE send.

Removes the Nagle/delayed-ACK pair that a header-then-body two-write server creates,
so any remaining latency through the proxy is the daemon's own cost.
"""
import socket
import sys
import threading

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 19104


def handle(conn):
    conn.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    buf = b""
    try:
        while True:
            while b"\r\n\r\n" not in buf:
                chunk = conn.recv(65536)
                if not chunk:
                    return
                buf += chunk
            head, _, rest = buf.partition(b"\r\n\r\n")
            lines = head.split(b"\r\n")
            hdrs = {}
            for line in lines[1:]:
                if b":" in line:
                    k, _, v = line.partition(b":")
                    hdrs[k.strip().lower()] = v.strip()
            n = int(hdrs.get(b"content-length", b"0") or 0)
            if b"chunked" in hdrs.get(b"transfer-encoding", b"").lower():
                # consume until terminating chunk
                while b"0\r\n\r\n" not in rest:
                    chunk = conn.recv(65536)
                    if not chunk:
                        return
                    rest += chunk
                body = rest
                rest = b""
            else:
                while len(rest) < n:
                    chunk = conn.recv(65536)
                    if not chunk:
                        return
                    rest += chunk
                body, rest = rest[:n], rest[n:]
            payload = b'{"ok":true,"method":"%s","len":%d}' % (lines[0].split(b" ")[0], len(body))
            conn.sendall(
                b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: "
                + str(len(payload)).encode()
                + b"\r\n\r\n"
                + payload
            )
            buf = rest
    except OSError:
        return
    finally:
        try:
            conn.close()
        except OSError:
            pass


def main():
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(("127.0.0.1", PORT))
    s.listen(128)
    sys.stderr.write("backend4 (nodelay, single-write) on %d\n" % PORT)
    while True:
        c, _ = s.accept()
        threading.Thread(target=handle, args=(c,), daemon=True).start()


if __name__ == "__main__":
    main()

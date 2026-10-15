#!/usr/bin/env python3
"""Logging proxy for Maestri's CLI socket.

Listens on a (relative) Unix socket path, forwards each connection to the
real Maestri socket, and appends both directions to a log with timestamps.
Secrets in headers (X-Maestri-Token) are redacted in the log.

Usage: sockproxy.py <listen-path> <real-socket> <log-file>
"""
import os
import re
import socket
import sys
import threading
import time

LISTEN, REAL, LOG = sys.argv[1], sys.argv[2], sys.argv[3]
lock = threading.Lock()
counter = [0]


def log(conn_id, direction, data):
    text = data.decode("utf-8", errors="replace")
    text = re.sub(r"(?im)^(x-maestri-token:).*$", r"\1 <redacted>", text)
    with lock, open(LOG, "a") as fh:
        fh.write("[%.3f] #%d %s %d bytes\n%s\n---\n" % (time.time(), conn_id, direction, len(data), text))


def pump(src, dst, conn_id, direction):
    try:
        while True:
            data = src.recv(65536)
            if not data:
                break
            log(conn_id, direction, data)
            dst.sendall(data)
    except OSError:
        pass
    finally:
        try:
            dst.shutdown(socket.SHUT_WR)
        except OSError:
            pass


def handle(client):
    with lock:
        counter[0] += 1
        conn_id = counter[0]
    upstream = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    upstream.connect(REAL)
    t1 = threading.Thread(target=pump, args=(client, upstream, conn_id, "CLI->APP"))
    t2 = threading.Thread(target=pump, args=(upstream, client, conn_id, "APP->CLI"))
    t1.start(); t2.start(); t1.join(); t2.join()
    client.close(); upstream.close()


def main():
    if os.path.exists(LISTEN):
        os.unlink(LISTEN)
    server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    server.bind(LISTEN)
    os.chmod(LISTEN, 0o600)
    server.listen(8)
    while True:
        client, _ = server.accept()
        threading.Thread(target=handle, args=(client,), daemon=True).start()


if __name__ == "__main__":
    main()

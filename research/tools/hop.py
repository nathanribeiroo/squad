#!/usr/bin/env python3
"""Client for hop_bridge.py. Reads Python code from argv[1] or stdin, runs it
inside Hopper, and prints captured stdout plus `result` (as JSON).

Usage: hop.py 'result = len(H.procs())'
       hop.py - < snippet.py
"""
import json
import os
import socket
import sys

BASE = "/private/tmp/claude-501/-Users-nathan-www-rea-maestri/2f0883ea-0d91-4fbd-b89f-57ff644e7f04/scratchpad/hopper"


def main():
    code = sys.stdin.read() if len(sys.argv) < 2 or sys.argv[1] == "-" else sys.argv[1]
    os.chdir(BASE)
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.settimeout(1800)
    sock.connect("bridge.sock")
    sock.sendall(json.dumps({"code": code}).encode("utf-8"))
    sock.shutdown(socket.SHUT_WR)
    chunks = []
    while True:
        data = sock.recv(1 << 20)
        if not data:
            break
        chunks.append(data)
    reply = json.loads(b"".join(chunks).decode("utf-8"))
    if reply.get("stdout"):
        sys.stdout.write(reply["stdout"])
    if not reply.get("ok"):
        sys.stdout.write(reply.get("error", "error") + "\n")
        sys.exit(1)
    if reply.get("result") is not None:
        print(json.dumps(reply["result"], indent=1, ensure_ascii=False))


if __name__ == "__main__":
    main()

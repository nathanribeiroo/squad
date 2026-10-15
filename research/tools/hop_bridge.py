# Minimal Hopper bridge, run with `hopper -e <bin> -Y hop_bridge.py`.
#
# Hopper only allows its API on its dedicated Python thread, so this script
# blocks that thread with a single-client-at-a-time Unix socket server.
# Each request is JSON {"code": "<python>"}; the code runs with `doc` and the
# helper object `H` in scope and may set `result`. The reply is JSON
# {"ok", "stdout", "result"|"error"}. Send {"code": "QUIT"} to stop.
import contextlib
import io
import json
import os
import socket
import traceback

BASE = "/private/tmp/claude-501/-Users-nathan-www-rea-maestri/2f0883ea-0d91-4fbd-b89f-57ff644e7f04/scratchpad/hopper"
SOCK = "bridge.sock"  # relative: the absolute path exceeds sun_path's 104 bytes
LOG = os.path.join(BASE, "bridge.log")


def log(msg):
    with open(LOG, "a") as fh:
        fh.write(msg + "\n")


class Helpers(object):
    def __init__(self, document):
        self.doc = document
        self._procs = None
        self._strings = None
        self._pseudo = {}

    def hx(self, value):
        return "0x%x" % value

    def seg(self, addr):
        return self.doc.getSegmentAtAddress(addr)

    def name(self, addr):
        segment = self.seg(addr)
        return segment.getNameAtAddress(addr) if segment else None

    def procs(self):
        if self._procs is None:
            out = {}
            for segment in self.doc.getSegmentsList():
                for i in range(segment.getProcedureCount()):
                    proc = segment.getProcedureAtIndex(i)
                    entry = proc.getEntryPoint()
                    out[entry] = segment.getNameAtAddress(entry) or ("sub_%x" % entry)
            self._procs = out
        return self._procs

    def strings(self):
        if self._strings is None:
            out = []
            for segment in self.doc.getSegmentsList():
                for value, address in segment.getStringsList():
                    out.append((address, value))
            self._strings = out
        return self._strings

    def find_strings(self, needle, limit=50):
        hits = [(a, v) for a, v in self.strings() if needle in v]
        return hits[:limit]

    def xrefs(self, addr):
        segment = self.seg(addr)
        return list(segment.getReferencesOfAddress(addr)) if segment else []

    def proc_at(self, addr):
        segment = self.seg(addr)
        proc = segment.getProcedureAtAddress(addr) if segment else None
        return proc

    def entry_of(self, addr):
        proc = self.proc_at(addr)
        return proc.getEntryPoint() if proc else None

    def decompile(self, addr):
        entry = self.entry_of(addr)
        if entry is None:
            return None
        if entry not in self._pseudo:
            self._pseudo[entry] = self.proc_at(entry).decompile()
        return self._pseudo[entry]

    def dump(self, addr, subdir="pseudo"):
        """Decompile the procedure containing addr into BASE/<subdir>/<entry>.c."""
        entry = self.entry_of(addr)
        if entry is None:
            return None
        code = self.decompile(entry) or ""
        folder = os.path.join(BASE, subdir)
        if not os.path.isdir(folder):
            os.makedirs(folder)
        path = os.path.join(folder, "%s.c" % self.hx(entry))
        with open(path, "w") as fh:
            fh.write("// %s %s\n" % (self.hx(entry), self.procs().get(entry, "")))
            fh.write(code)
        return {"entry": self.hx(entry), "name": self.procs().get(entry), "lines": code.count("\n"), "path": path}

    def callers(self, addr):
        proc = self.proc_at(addr)
        return [p.getEntryPoint() for p in proc.getAllCallerProcedures()] if proc else []

    def callees(self, addr):
        proc = self.proc_at(addr)
        return [p.getEntryPoint() for p in proc.getAllCalleeProcedures()] if proc else []

    def string_users(self, needle, limit=20):
        """Strings containing needle -> referencing procedures."""
        out = []
        for address, value in self.find_strings(needle, limit):
            users = []
            for src in self.xrefs(address):
                entry = self.entry_of(src)
                users.append({"from": self.hx(src),
                              "proc": self.hx(entry) if entry is not None else None,
                              "name": self.procs().get(entry) if entry is not None else None})
            out.append({"addr": self.hx(address), "value": value[:160], "users": users})
        return out


def handle(request, namespace):
    code = request.get("code", "")
    if code == "QUIT":
        return None
    buf = io.StringIO()
    namespace.pop("result", None)
    try:
        with contextlib.redirect_stdout(buf):
            exec(code, namespace)
        res = namespace.get("result")
        try:
            json.dumps(res)
        except (TypeError, ValueError):
            res = repr(res)
        return {"ok": True, "stdout": buf.getvalue(), "result": res}
    except Exception:
        return {"ok": False, "stdout": buf.getvalue(), "error": traceback.format_exc()}


def serve():
    document = Document.getCurrentDocument()
    document.waitForBackgroundProcessToEnd()
    helpers = Helpers(document)
    namespace = {"doc": document, "H": helpers, "json": json, "os": os, "BASE": BASE}
    os.chdir(BASE)
    if os.path.exists(SOCK):
        os.unlink(SOCK)
    server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    server.bind(SOCK)
    os.chmod(SOCK, 0o600)
    server.listen(1)
    log("ready: %s" % document.getDocumentName())
    try:
        while True:
            conn, _ = server.accept()
            chunks = []
            while True:
                data = conn.recv(65536)
                if not data:
                    break
                chunks.append(data)
            reply = handle(json.loads(b"".join(chunks).decode("utf-8")), namespace)
            if reply is None:
                conn.sendall(b'{"ok": true, "result": "bye"}')
                conn.close()
                break
            conn.sendall(json.dumps(reply).encode("utf-8"))
            conn.close()
    finally:
        server.close()
        if os.path.exists(SOCK):
            os.unlink(SOCK)
        log("stopped")


try:
    serve()
except Exception:
    log(traceback.format_exc())

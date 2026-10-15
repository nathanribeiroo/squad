#!/usr/bin/env python3
"""Run inside a Maestri terminal to record exactly what the app writes to it.

Enables bracketed paste (DECSET 2004), switches the tty to raw mode and logs
every input chunk (hex + repr) with a timestamp. After each submitted line
(CR or LF) it imitates an agent: prints a progress line every second for
WORK seconds, then goes quiet. Exits after TOTAL seconds or on Ctrl-C.

Usage: ptylog.py <log-file> [WORK=6] [TOTAL=180] [paste=1]
"""
import os
import select
import sys
import termios
import time
import tty

LOG = sys.argv[1]
WORK = float(sys.argv[2]) if len(sys.argv) > 2 else 6.0
TOTAL = float(sys.argv[3]) if len(sys.argv) > 3 else 180.0
PASTE = (sys.argv[4] if len(sys.argv) > 4 else "1") == "1"


def log(msg):
    with open(LOG, "a") as fh:
        fh.write("[%.3f] %s\n" % (time.time(), msg))


def out(text):
    os.write(1, text.encode())
    log("OUT %r" % text)


fd = 0
old = termios.tcgetattr(fd)
start = time.time()
work_until = None
next_tick = None
tick = 0
try:
    tty.setraw(fd)
    if PASTE:
        os.write(1, b"\x1b[?2004h")
    log("START paste=%s work=%s" % (PASTE, WORK))
    out("ptylog ready (paste=%s)\r\n" % PASTE)
    while time.time() - start < TOTAL:
        now = time.time()
        if work_until is not None and now >= next_tick:
            if now < work_until:
                tick += 1
                out("working %d\r\n" % tick)
                next_tick = now + 1.0
            else:
                out("DONE\r\n")
                log("WORK_END")
                work_until = None
        r, _, _ = select.select([fd], [], [], 0.05)
        if not r:
            continue
        data = os.read(fd, 65536)
        log("IN %d bytes hex=%s repr=%r" % (len(data), data.hex(), data))
        if b"\x03" in data:
            break
        if b"\r" in data or b"\n" in data:
            if work_until is None:
                log("SUBMIT_SEEN")
                tick = 0
                work_until = time.time() + WORK
                next_tick = time.time() + 0.2
finally:
    if PASTE:
        os.write(1, b"\x1b[?2004l")
    termios.tcsetattr(fd, termios.TCSADRAIN, old)
    log("EXIT")
    os.write(1, b"ptylog exit\r\n")

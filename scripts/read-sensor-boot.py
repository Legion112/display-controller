#!/usr/bin/env python3
"""Read Arduino boot log (READY / ERR init) over serial after reset."""

import os
import sys
import time

try:
    import serial
except ImportError:
    print("install: pip install pyserial", file=sys.stderr)
    sys.exit(1)

PORT = os.environ.get("LUX_PORT", os.environ.get("RAVEDUDE_PORT", "/dev/ttyUSB0"))
BAUD = 57600
TIMEOUT = 3.0


def main() -> int:
    port = serial.Serial(PORT, BAUD, timeout=0.2)
    port.reset_input_buffer()
    # Toggle DTR to reset Arduino (CH340/FTDI)
    port.dtr = False
    time.sleep(0.05)
    port.dtr = True
    time.sleep(0.05)

    deadline = time.time() + TIMEOUT
    lines = []
    while time.time() < deadline:
        raw = port.readline()
        if not raw:
            continue
        line = raw.decode("utf-8", errors="replace").strip()
        if line:
            print(line)
            lines.append(line)
        if line.startswith("ERR init") or line == "READY":
            time.sleep(0.15)
            while True:
                extra = port.readline()
                if not extra:
                    break
                el = extra.decode("utf-8", errors="replace").strip()
                if el:
                    print(el)
                    lines.append(el)
            break

    port.close()
    if not lines:
        print(f"no output from {PORT} (timeout)", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

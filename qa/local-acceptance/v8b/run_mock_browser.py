#!/usr/bin/env python3
"""Run mock browser QA against a Vite server owned by this process."""

import http.client
import os
from pathlib import Path
import socket
import subprocess
import sys
import time


ROOT = Path(__file__).resolve().parents[3]
HOST = "127.0.0.1"
PORT = 5173


def port_is_open() -> bool:
    with socket.socket() as sock:
        sock.settimeout(0.5)
        return sock.connect_ex((HOST, PORT)) == 0


def vite_is_ready() -> bool:
    try:
        connection = http.client.HTTPConnection(HOST, PORT, timeout=1)
        connection.request("GET", "/")
        response = connection.getresponse()
        response.read()
        connection.close()
        return response.status == 200
    except (OSError, http.client.HTTPException):
        return False


def main() -> int:
    if port_is_open():
        print(f"BLOCKED: {HOST}:{PORT} is already in use; refusing to test another server", file=sys.stderr)
        return 2

    vite = ROOT / "web" / "node_modules" / "vite" / "bin" / "vite.js"
    if not vite.is_file():
        print("BLOCKED: Vite is missing; run npm ci in web first", file=sys.stderr)
        return 2

    with subprocess.Popen(
        ["node", str(vite), "--host", HOST, "--port", str(PORT), "--strictPort"],
        cwd=ROOT / "web",
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    ) as server:
        try:
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline:
                if server.poll() is not None:
                    print("Vite exited before browser QA started", file=sys.stderr)
                    return 1
                if vite_is_ready():
                    break
                time.sleep(0.25)
            else:
                print("Vite did not become ready within 30 seconds", file=sys.stderr)
                return 1

            test_env = os.environ.copy()
            test_env["PYTHONDONTWRITEBYTECODE"] = "1"
            result = subprocess.run(
                [sys.executable, "-m", "pytest", "qa/browser/test_console.py", "-q"],
                cwd=ROOT,
                env=test_env,
                check=False,
            )
            return result.returncode
        finally:
            server.terminate()
            try:
                output, _ = server.communicate(timeout=10)
            except subprocess.TimeoutExpired:
                server.kill()
                output, _ = server.communicate()
            if output:
                print(output[-4000:])


if __name__ == "__main__":
    raise SystemExit(main())

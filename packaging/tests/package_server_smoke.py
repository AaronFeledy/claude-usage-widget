#!/usr/bin/env python3
"""Exercise one packaged usage-server without provider discovery or credentials."""

import argparse
import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def request(url, token=None):
    headers = {"Authorization": "Bearer " + token} if token else {}
    try:
        with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=2) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", required=True)
    args = parser.parse_args()
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        port = reservation.getsockname()[1]
    with tempfile.TemporaryDirectory(prefix="headroom-packaged-server-") as root:
        config = os.path.join(root, "config.yaml")
        with open(config, "w", encoding="utf-8") as output:
            output.write("providers:\n  claude: {enabled: false}\n  codex: {enabled: false}\n  cursor: {enabled: false}\n  grok: {enabled: false}\n")
        environment = dict(os.environ)
        for key in list(environment):
            if key.startswith("USAGE_") or key.startswith("CLAUDE_") or key.startswith("CODEX_") or key.startswith("CURSOR_") or key.startswith("GROK_"):
                environment.pop(key)
        environment.update({
            "USAGE_CONFIG": config,
            "USAGE_AUTH_TOKEN": "package-smoke-token",
            "USAGE_PROVIDER_CLAUDE_ENABLED": "false",
            "USAGE_PROVIDER_CODEX_ENABLED": "false",
            "USAGE_PROVIDER_CURSOR_ENABLED": "false",
            "USAGE_PROVIDER_GROK_ENABLED": "false",
            "HOME": root,
        })
        creationflags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
        process = subprocess.Popen([args.server, "--listen-addr", f"127.0.0.1:{port}"], env=environment,
                                   stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                   stderr=subprocess.DEVNULL, creationflags=creationflags)
        try:
            deadline = time.monotonic() + 20
            while True:
                if process.poll() is not None:
                    raise RuntimeError(f"packaged server exited {process.returncode}")
                try:
                    status, health = request(f"http://127.0.0.1:{port}/api/v1/health", "package-smoke-token")
                    if status == 200:
                        break
                except OSError:
                    pass
                if time.monotonic() >= deadline:
                    raise TimeoutError("packaged server readiness timed out")
                time.sleep(0.05)
            assert health["status"] in ("ok", "degraded") and health["providers"] == [] and health["version"]
            status, usage = request(f"http://127.0.0.1:{port}/api/v1/usage", "package-smoke-token")
            assert status == 200 and usage == []
            status, _ = request(f"http://127.0.0.1:{port}/api/v1/health")
            assert status == 401
        finally:
            process.terminate()
            try:
                process.wait(5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(5)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Exercise an installed native CLI against a disposable provider-disabled server.

Only health and usage GET requests are permitted here. Never add reset actions:
executing the reset button, endpoint, or triggering code can consume a valuable
banked reset. This fixture must never use a live account or saved credentials.
"""
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--manager", type=Path)
    parser.add_argument("--release-manifest", type=Path, help="Use the public installer instead of the manager directly.")
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()
    if bool(args.manager) == bool(args.release_manifest):
        parser.error("supply exactly one of --manager or --release-manifest")
    with tempfile.TemporaryDirectory(prefix="headroom cli smoke ") as directory:
        root = Path(directory)
        executable = root / "entries with spaces" / ("headroom.exe" if os.name == "nt" else "headroom")
        env = {key: value for key, value in os.environ.items()
               if not key.startswith(("HEADROOM_", "USAGE_"))}
        env.update(HOME=str(root), LOCALAPPDATA=str(root / "local app data"),
                   APPDATA=str(root / "roaming app data"), XDG_DATA_HOME=str(root / "user data"))
        install = root / "install with spaces"
        if args.release_manifest:
            repository = Path(__file__).resolve().parents[2]
            if os.name == "nt":
                command = ["powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(repository / "install.ps1"),
                    "-CLI", "-NoLaunch", "-PackagePath", str(args.archive.resolve()),
                    "-ReleaseManifestPath", str(args.release_manifest.resolve()), "-InstallRoot", str(install),
                    "-EntryPath", str(executable)]
            else:
                command = ["sh", str(repository / "install.sh"), "--cli", "--no-launch", "--package", str(args.archive.resolve()),
                    "--release-manifest", str(args.release_manifest.resolve()), "--install-root", str(install), "--entry-path", str(executable)]
        else:
            command = [str(args.manager.resolve()), "install", "--archive", str(args.archive.resolve()),
                "--install-root", str(install), "--entry-path", str(executable), "--cli-entry-path", str(executable)]
        subprocess.run(command, env=env, check=True, timeout=90, stdout=subprocess.PIPE)
        def run(*arguments):
            return subprocess.check_output([str(executable), *arguments], env=env, text=True, timeout=20, stderr=subprocess.STDOUT)
        assert args.version in run("version")
        assert "serve" in run("help")
        if os.name != "nt":
            legacy = executable.with_name("usage-server")
            assert "listen-addr" in subprocess.check_output([str(legacy), "--help"], env=env, text=True, timeout=20, stderr=subprocess.STDOUT)
        config = root / "server.yaml"
        config.write_text("providers:\n" + "".join(f"  {name}:\n    enabled: false\n"
            for name in ("claude", "codex", "cursor", "grok")))
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        address = f"http://127.0.0.1:{port}"
        # Start the installed payload directly so this fixture owns the exact
        # child it terminates on every OS, including Windows console launchers.
        install = root / "install with spaces"
        state = json.loads((install / "install-state.json").read_text())
        runtime = install / state["version_path"] / "bin" / executable.name
        server_env = env | {"HEADROOM_INSTALL_ROOT": str(install), "HEADROOM_PACKAGE_VERSION": args.version}
        server = subprocess.Popen([str(runtime), "serve", "--config", str(config),
            "--listen-addr", f"127.0.0.1:{port}"], env=server_env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 15
            while True:
                if server.poll() is not None:
                    raise AssertionError("provider-disabled server exited before readiness")
                try:
                    with urllib.request.urlopen(address + "/api/v1/health", timeout=1) as response:
                        if response.status == 200:
                            break
                except (urllib.error.URLError, TimeoutError):
                    pass
                if time.monotonic() >= deadline:
                    raise AssertionError("provider-disabled server did not become ready")
                time.sleep(0.1)
            assert json.loads(run("--once", "--json", "--url", address)) == []
            assert "Headroom" in run("--once", "--plain", "--url", address)
        finally:
            server.terminate()
            try:
                server.wait(timeout=15)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait(timeout=5)
        print("Native CLI install, routing, version, and provider-disabled serve/usage passed.")


if __name__ == "__main__":
    main()

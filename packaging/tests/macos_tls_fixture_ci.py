#!/usr/bin/env python3
"""Run native Qt TLS fixtures with disposable key storage on hosted macOS CI."""

import os
from pathlib import Path
import re
import secrets
import shlex
import subprocess
import sys
import tempfile


def security(*arguments):
    return subprocess.check_output(["/usr/bin/security", *map(str, arguments)], text=True).strip()


def main():
    if (sys.platform != "darwin" or os.environ.get("GITHUB_ACTIONS") != "true"
            or os.environ.get("RUNNER_ENVIRONMENT") != "github-hosted"):
        raise SystemExit("Disposable TLS keychain setup is restricted to hosted macOS CI.")
    if len(sys.argv) < 2:
        raise SystemExit("Supply the non-reset test command to run.")
    previous_default = shlex.split(security("default-keychain", "-d", "user"))
    previous_search = shlex.split(security("list-keychains", "-d", "user"))
    if len(previous_default) != 1:
        raise SystemExit("Cannot preserve the runner's default keychain.")
    fixture = (Path(__file__).parents[2] / "clients/desktop/tests/tls_fixture.h").read_text()
    identities = dict(re.findall(r'inline constexpr char (\w+)\[\] = R"PEM\((.*?)\)PEM";', fixture, re.DOTALL))
    with tempfile.TemporaryDirectory(prefix="headroom-tls-fixtures-") as temporary:
        root = Path(temporary)
        keychain = root / "fixtures.keychain-db"
        password = secrets.token_hex(24)
        try:
            security("create-keychain", "-p", password, keychain)
            security("set-keychain-settings", "-lut", "1800", keychain)
            security("unlock-keychain", "-p", password, keychain)
            # Qt 6.8.3's official SDK-14 build cannot request memory-only key
            # import on macOS 15, which ignores its temporary-keychain option.
            # Only public synthetic fixture keys are imported here. They are
            # not trusted roots, and peer verification/pinning stay enabled.
            for prefix in ("", "replacement"):
                cert_name = prefix + ("CertificatePem" if prefix else "certificatePem")
                key_name = prefix + ("PrivateKeyPem" if prefix else "privateKeyPem")
                certificate, private_key, archive = root / "cert.pem", root / "key.pem", root / "identity.p12"
                certificate.write_text(identities[cert_name])
                private_key.write_text(identities[key_name])
                subprocess.run(["/usr/bin/openssl", "pkcs12", "-export", "-in", str(certificate),
                                "-inkey", str(private_key), "-out", str(archive), "-passout", "pass:fixture"],
                               check=True, timeout=30, stdout=subprocess.DEVNULL)
                security("import", archive, "-k", keychain, "-P", "fixture", "-A")
            security("default-keychain", "-d", "user", "-s", keychain)
            security("list-keychains", "-d", "user", "-s", keychain, *previous_search)
            result = subprocess.run(sys.argv[1:], timeout=850)
            return result.returncode
        finally:
            security("default-keychain", "-d", "user", "-s", *previous_default)
            security("list-keychains", "-d", "user", "-s", *previous_search)
            if keychain.exists():
                security("delete-keychain", keychain)


if __name__ == "__main__":
    sys.exit(main())

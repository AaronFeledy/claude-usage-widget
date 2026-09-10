#!/usr/bin/env python3
"""Validate the single release version consumed by every Headroom component."""

import argparse
import re

STABLE_SEMVER = re.compile(
    r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$"
)


def validate(value: str) -> tuple[str, str]:
    match = STABLE_SEMVER.fullmatch(value)
    if not match:
        raise ValueError("version must be stable SemVer without a leading v")
    numeric = tuple(int(match.group(index)) for index in range(1, 4))
    if any(component > 65534 for component in numeric):
        raise ValueError("numeric version components must fit .NET and Windows version resources")
    return value, ".".join(str(component) for component in numeric)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("--github-output")
    args = parser.parse_args()
    try:
        version, numeric = validate(args.version)
    except ValueError as error:
        parser.error(str(error))
    if args.github_output:
        with open(args.github_output, "a", encoding="utf-8", newline="\n") as output:
            output.write(f"version={version}\nnumeric_version={numeric}\n")
    else:
        print(version)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

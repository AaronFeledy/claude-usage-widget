#!/usr/bin/env python3
"""Check native Mach-O slices and deployment targets before signing a Mac package."""

import argparse
from pathlib import Path
import re
import subprocess


MACHO_MAGIC = {b"\xfe\xed\xfa\xce", b"\xce\xfa\xed\xfe", b"\xfe\xed\xfa\xcf", b"\xcf\xfa\xed\xfe",
               b"\xca\xfe\xba\xbe", b"\xbe\xba\xfe\xca", b"\xca\xfe\xba\xbf", b"\xbf\xba\xfe\xca"}


def check(path, architecture):
    slices = subprocess.check_output(["lipo", "-archs", str(path)], text=True).split()
    if architecture not in slices:
        raise ValueError(f"{path.name}: missing {architecture} Mach-O slice")
    headers = subprocess.check_output(["otool", "-arch", architecture, "-l", str(path)], text=True)
    targets = re.findall(r"\b(?:minos|version)\s+(\d+)\.(\d+)(?:\.(\d+))?", "\n".join(
        block for block in headers.split("Load command") if "LC_BUILD_VERSION" in block or "LC_VERSION_MIN_MACOSX" in block))
    if len(targets) != 1 or tuple(int(part or 0) for part in targets[0]) > (12, 0, 0):
        raise ValueError(f"{path.name}: {architecture} slice does not support the macOS 12 baseline")


def check_tree(root, architecture):
    count = 0
    paths = (root,) if root.is_file() else root.rglob("*")
    for path in paths:
        if path.is_symlink() or not path.is_file():
            continue
        with path.open("rb") as source:
            magic = source.read(4)
        if magic in MACHO_MAGIC:
            check(path, architecture)
            count += 1
    if not count:
        raise ValueError("no Mach-O payloads were found")
    return count


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("path", type=Path)
    parser.add_argument("architecture", choices=("arm64", "x86_64"))
    args = parser.parse_args()
    print(f"Verified {check_tree(args.path, args.architecture)} Mach-O payloads for {args.architecture} and macOS 12")

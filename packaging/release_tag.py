#!/usr/bin/env python3
"""Find a stable release tag already attached to an exact commit."""

import argparse
import pathlib
import secrets
import subprocess

from validate_version import validate


def git(*args: str, cwd: pathlib.Path) -> str:
    return subprocess.run(
        ["git", *args], cwd=cwd, check=True, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    ).stdout.rstrip("\n")


def read_tag(repo: pathlib.Path, commit: str, tag: str) -> tuple[str, str, str]:
    exact = git("rev-parse", f"{commit}^{{commit}}", cwd=repo)
    if not tag.startswith("v"):
        raise ValueError("release tag must start with v")
    version, _ = validate(tag[1:])
    if git("rev-parse", f"{tag}^{{commit}}", cwd=repo) != exact:
        raise ValueError("release tag does not point to the initiating commit")
    changelog = ""
    if git("cat-file", "-t", f"refs/tags/{tag}", cwd=repo) == "tag":
        changelog = git("for-each-ref", "--format=%(contents)", f"refs/tags/{tag}", cwd=repo)
    return tag, version, changelog


def find_existing(repo: pathlib.Path, commit: str) -> tuple[str, str, str] | None:
    exact = git("rev-parse", f"{commit}^{{commit}}", cwd=repo)
    tags = []
    for tag in git("tag", "--points-at", exact, "--list", "v*", cwd=repo).splitlines():
        try:
            result = read_tag(repo, exact, tag)
        except ValueError:
            continue
        tags.append(result)
    if len(tags) > 1:
        raise ValueError("more than one stable release tag points to the initiating commit")
    if not tags:
        return None
    return tags[0]


def append_output(path: pathlib.Path, result: tuple[str, str, str] | None) -> None:
    with path.open("a", encoding="utf-8", newline="\n") as output:
        if result is None:
            output.write("reused=false\n")
            return
        tag, version, changelog = result
        delimiter = "HEADROOM_CHANGELOG_" + secrets.token_hex(16)
        output.write(f"reused=true\ntag={tag}\nversion={version}\n")
        output.write(f"changelog<<{delimiter}\n{changelog}\n{delimiter}\n")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", default=".")
    parser.add_argument("--commit", required=True)
    parser.add_argument("--tag")
    parser.add_argument("--github-output", required=True)
    args = parser.parse_args()
    repo = pathlib.Path(args.repo)
    result = read_tag(repo, args.commit, args.tag) if args.tag else find_existing(repo, args.commit)
    append_output(pathlib.Path(args.github_output), result)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

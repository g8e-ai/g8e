#!/usr/bin/env python3

from __future__ import annotations

import argparse
import hashlib
import os
import shutil
import tempfile
from pathlib import Path

import generate_readme as readme


class PromotionError(Exception):
    pass


def candidate_tree_sha256(root: Path) -> str:
    if not root.is_dir():
        raise PromotionError(f"candidate is not a directory: {root}")
    digest = hashlib.sha256()
    files = sorted(path for path in root.rglob("*") if path.is_file() or path.is_symlink())
    if not files:
        raise PromotionError("candidate contains no files")
    for path in files:
        if path.is_symlink():
            raise PromotionError(f"candidate contains a symbolic link: {path.relative_to(root)}")
        relative = path.relative_to(root).as_posix()
        content = path.read_bytes()
        digest.update(relative.encode())
        digest.update(b"\0")
        digest.update(str(len(content)).encode())
        digest.update(b"\0")
        digest.update(hashlib.sha256(content).hexdigest().encode())
        digest.update(b"\n")
    return digest.hexdigest()


def promote(candidate: Path, current: Path, approved_digest: str) -> None:
    actual_digest = candidate_tree_sha256(candidate)
    if actual_digest != approved_digest:
        raise PromotionError(f"approved candidate tree digest does not match: {approved_digest} != {actual_digest}")
    try:
        readme.load_snapshot(candidate)
    except readme.ReadmeError as exc:
        raise PromotionError(f"candidate validation failed: {exc}") from exc
    if not current.parent.is_dir():
        raise PromotionError(f"current snapshot parent does not exist: {current.parent}")
    with tempfile.TemporaryDirectory(prefix="readme-promotion-", dir=current.parent) as temporary:
        staged = Path(temporary) / "staged"
        shutil.copytree(candidate, staged)
        if candidate_tree_sha256(staged) != approved_digest:
            raise PromotionError("staged candidate tree digest does not match approval")
        backup = Path(temporary) / "previous"
        had_current = current.exists()
        if had_current:
            os.replace(current, backup)
        try:
            os.replace(staged, current)
        except OSError:
            if had_current and backup.exists():
                os.replace(backup, current)
            raise
        if candidate_tree_sha256(current) != approved_digest:
            failed = Path(temporary) / "failed"
            os.replace(current, failed)
            if had_current and backup.exists():
                os.replace(backup, current)
            raise PromotionError("installed candidate tree digest does not match approval")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("candidate", type=Path)
    parser.add_argument("current", type=Path)
    parser.add_argument("--approved-tree-sha256", required=True)
    parser.add_argument("--print-tree-sha256", action="store_true")
    args = parser.parse_args(argv)
    try:
        digest = candidate_tree_sha256(args.candidate)
        if args.print_tree_sha256:
            print(digest)
            return 0
        promote(args.candidate, args.current, args.approved_tree_sha256)
    except (PromotionError, OSError) as exc:
        print(f"error: {exc}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

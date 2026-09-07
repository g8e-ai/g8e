#!/usr/bin/env python3

from __future__ import annotations

import argparse
import hashlib
import json
import os
import tempfile
from pathlib import Path
from typing import Any


class ProvenanceError(Exception):
    pass


def _canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n"


def _sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _require_sha256(value: Any, label: str) -> str:
    if not isinstance(value, str) or len(value) != 64 or any(character not in "0123456789abcdef" for character in value):
        raise ProvenanceError(f"{label} must be a lowercase SHA-256 digest")
    return value


def _validate_digests(rows: list[dict[str, Any]], identity_field: str, label: str) -> list[dict[str, Any]]:
    normalized: list[dict[str, Any]] = []
    seen: set[str] = set()
    for row in rows:
        identity = row.get(identity_field)
        if not isinstance(identity, str) or not identity:
            raise ProvenanceError(f"{label} requires non-empty {identity_field}")
        if identity in seen:
            raise ProvenanceError(f"duplicate {label} identity: {identity}")
        seen.add(identity)
        normalized.append(dict(sorted({**row, "sha256": _require_sha256(row.get("sha256"), f"{label} {identity} sha256")}.items())))
    return sorted(normalized, key=lambda row: str(row[identity_field]))


def collect(source_root: Path, source_paths: list[str], campaign_profile: dict[str, Any], components: list[dict[str, Any]], images: list[dict[str, Any]], environment: dict[str, str]) -> dict[str, Any]:
    root = source_root.resolve()
    if not root.is_dir():
        raise ProvenanceError(f"source root is not a directory: {source_root}")
    profile_id = campaign_profile.get("profile_id")
    release_version = campaign_profile.get("release_version")
    if campaign_profile.get("schema_version") != "1.0.0" or not isinstance(profile_id, str) or not profile_id or not isinstance(release_version, str) or not release_version:
        raise ProvenanceError("campaign profile requires schema_version 1.0.0, profile_id, and release_version")
    required_environment = {"os", "arch", "hardware", "python", "container_runtime"}
    if set(environment) != required_environment or not all(isinstance(value, str) and value for value in environment.values()):
        raise ProvenanceError("environment requires non-empty os, arch, hardware, python, and container_runtime")
    files: list[dict[str, Any]] = []
    seen_paths: set[str] = set()
    for raw_path in sorted(source_paths):
        relative = Path(raw_path)
        if relative.is_absolute() or relative == Path() or ".." in relative.parts:
            raise ProvenanceError(f"source entry must be a relative source path: {raw_path}")
        normalized = relative.as_posix()
        if normalized in seen_paths:
            raise ProvenanceError(f"duplicate source path: {normalized}")
        seen_paths.add(normalized)
        resolved = (root / relative).resolve()
        try:
            resolved.relative_to(root)
        except ValueError as exc:
            raise ProvenanceError(f"source entry escapes declared root: {raw_path}") from exc
        if not resolved.is_file() or (root / relative).is_symlink():
            raise ProvenanceError(f"source entry is not a regular file: {raw_path}")
        content = resolved.read_bytes()
        files.append({"path": normalized, "byte_length": len(content), "sha256": _sha256_bytes(content)})
    if not files:
        raise ProvenanceError("source inclusion manifest must contain at least one file")
    encoded_files = "".join(_canonical_json(row) for row in files).encode()
    profile = json.loads(_canonical_json(campaign_profile))
    return {
        "schema_version": "1.0.0",
        "campaign_profile": profile,
        "campaign_profile_sha256": _sha256_bytes(_canonical_json(profile).encode()),
        "source_tree": {
            "algorithm": "sha256-canonical-jsonl-v1",
            "files": files,
            "state_sha256": _sha256_bytes(encoded_files),
        },
        "components": _validate_digests(components, "name", "component"),
        "images": _validate_digests(images, "service", "image"),
        "environment": dict(sorted(environment.items())),
    }


def _load_object(path: Path, label: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise ProvenanceError(f"cannot read {label}: {exc}") from exc
    if not isinstance(value, dict):
        raise ProvenanceError(f"{label} must contain an object")
    return value


def _load_rows(path: Path, label: str) -> list[dict[str, Any]]:
    value = _load_object(path, label)
    rows = value.get(label)
    if not isinstance(rows, list) or not all(isinstance(row, dict) for row in rows):
        raise ProvenanceError(f"{label} file must contain a {label} list")
    return rows


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("source_root", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--source-path", action="append", required=True)
    parser.add_argument("--campaign-profile", type=Path, required=True)
    parser.add_argument("--components", type=Path, required=True)
    parser.add_argument("--images", type=Path, required=True)
    parser.add_argument("--environment", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        if args.output.exists():
            raise ProvenanceError(f"output already exists: {args.output}")
        result = collect(args.source_root, args.source_path, _load_object(args.campaign_profile, "campaign profile"), _load_rows(args.components, "components"), _load_rows(args.images, "images"), _load_object(args.environment, "environment"))
        args.output.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile("w", dir=args.output.parent, delete=False) as handle:
            temporary = Path(handle.name)
            handle.write(_canonical_json(result))
        os.replace(temporary, args.output)
    except ProvenanceError as exc:
        print(f"error: {exc}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

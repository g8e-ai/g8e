#!/usr/bin/env python3

"""Generate or verify the canonical Python protobuf modules."""

from __future__ import annotations

import argparse
import filecmp
import tempfile
from importlib.resources import files
from pathlib import Path

from grpc_tools import protoc

PYTHON_ROOT = Path(__file__).resolve().parents[1]
PROTO_ROOT = PYTHON_ROOT.parent / "proto"
PROTO_FILES = (
    Path("g8e/common/v1/common.proto"),
    Path("g8e/operator/v1/operator.proto"),
    Path("g8e/pubsub/v1/pubsub.proto"),
)
GENERATED_FILES = tuple(
    generated
    for path in PROTO_FILES
    for generated in (
        path.with_name(f"{path.stem}_pb2.py"),
        path.with_name(f"{path.stem}_pb2.pyi"),
    )
)


def generate(output_root: Path) -> None:
    args = [
        "grpc_tools.protoc",
        f"--proto_path={PROTO_ROOT}",
        f"--proto_path={files('grpc_tools') / '_proto'}",
        f"--python_out={output_root}",
        f"--pyi_out={output_root}",
        *(str(path) for path in PROTO_FILES),
    ]
    result = protoc.main(args)
    if result != 0:
        raise SystemExit(result)


def check() -> None:
    with tempfile.TemporaryDirectory() as temp_dir:
        generated_root = Path(temp_dir)
        generate(generated_root)
        changed = [
            str(path)
            for path in GENERATED_FILES
            if not (PYTHON_ROOT / path).is_file()
            or not filecmp.cmp(generated_root / path, PYTHON_ROOT / path, shallow=False)
        ]
    if changed:
        raise SystemExit("generated protobuf modules are stale: " + ", ".join(changed))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if args.check:
        check()
    else:
        generate(PYTHON_ROOT)


if __name__ == "__main__":
    main()

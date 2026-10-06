#!/bin/sh
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Build a runtime-only Operator image on an explicit Docker context using an
# allowlisted temporary context. No repository scratch or credential paths are sent.
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
    echo "usage: $0 <docker-context> <image-tag> [linux-amd64-binary]" >&2
    exit 2
fi

docker_context=$1
image_tag=$2
binary=${3:-bin/g8e-linux-amd64}
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

case "$(file -b "$binary")" in
    *ELF*64-bit*x86-64*) ;;
    *) echo "operator image requires a Linux amd64 ELF binary: $binary" >&2; exit 1 ;;
esac

build_context=$(mktemp -d)
trap 'rm -rf "$build_context"' EXIT HUP INT TERM
mkdir -p "$build_context/protocol" "$build_context/docs"
cp "$binary" "$build_context/g8e"
cp "$repo_root/scripts/docker-entrypoint.sh" "$build_context/entrypoint.sh"
cp "$repo_root/deploy/operator-runtime/gateway-preflight.sh" "$build_context/gateway-preflight.sh"
cp -R "$repo_root/protocol/constants" "$build_context/protocol/constants"
cp -R "$repo_root/docs/reference" "$build_context/docs/reference"
cp "$repo_root/deploy/operator-runtime/Dockerfile" "$build_context/Dockerfile"

docker --context "$docker_context" build --pull=false --tag "$image_tag" "$build_context"
docker --context "$docker_context" image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}' "$image_tag"

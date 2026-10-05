# syntax=docker/dockerfile:1
# Runtime-only Operator image. The build context is produced by
# scripts/build-operator-image.sh and contains no repository credentials/state.
FROM debian@sha256:30482e873082e906a4908c10529180aefb6f77620aea7404b909829fadc5d168

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY g8e /g8e
COPY entrypoint.sh /entrypoint.sh
COPY gateway-preflight.sh /gateway-preflight.sh
COPY protocol/constants /protocol/constants
COPY docs/reference /docs/reference
RUN chmod 0755 /g8e /entrypoint.sh /gateway-preflight.sh
ENTRYPOINT ["/entrypoint.sh"]

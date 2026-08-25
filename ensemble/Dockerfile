# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# =============================================================================
# Multi-stage Dockerfile for the g8e Ensemble (g8ee, Python/FastAPI)
# Build context: g8e repo root (so the Dockerfile can COPY protocol/python
# and ensemble/ the way the monorepo layout requires).
# =============================================================================

# Stage 1: Builder — install the in-tree protocol package, then the ensemble
FROM python:3.12-slim AS builder

WORKDIR /build

# Copy the in-tree g8e Python protocol package and install it first
# (non-editable so it survives the multi-stage copy into the runtime image).
# This makes the `g8e` dependency resolvable from the local package instead
# of PyPI, matching the [tool.uv.sources] path override in ensemble/pyproject.toml.
COPY protocol/python /build/protocol/python

# Copy protocol/constants/ and sync into the bundled _data/ directory so changes
# to the JSON files invalidate the Docker layer cache (the COPY protocol/python
# layer only hashes protocol/python/ content). This also ensures the build
# always uses fresh constants even if _data/ was not re-synced via make
# python-build before the Docker build (E.9 staleness guard).
COPY protocol/constants /build/protocol/constants
RUN cp /build/protocol/constants/*.json /build/protocol/python/g8e/_data/ \
 && rm -rf /build/protocol/python/g8e/_data/doctrine \
 && cp -r /build/protocol/constants/doctrine /build/protocol/python/g8e/_data/

RUN pip install --no-cache-dir /build/protocol/python

# Build-time startup smoke test: import g8e.constants and resolve STATUS["status"]
# so a broken or stale constants bundle fails the image build, not the container
# runtime. This guards against the E.1 KeyError: 'status' crash that only
# surfaced at container startup.
RUN python3 -c "from g8e.constants import STATUS; assert 'status' in STATUS and STATUS['status'], 'broken constants bundle: STATUS[\"status\"] missing or empty'"

# Copy the ensemble and install runtime dependencies only (no dev/test extras).
COPY ensemble /build/ensemble
RUN pip install --no-cache-dir /build/ensemble

# Stage 2: Runtime — slim image with only runtime dependencies
FROM python:3.12-slim

# Copy installed site-packages and console scripts from the builder
COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin

# Copy the ensemble application source
COPY --from=builder /build/ensemble /app

WORKDIR /app

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]

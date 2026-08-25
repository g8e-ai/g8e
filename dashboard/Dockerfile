# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Dashboard (g8ed) Dockerfile — Monorepo Layout
# Build context: repo root (so the Dockerfile can COPY dashboard/ and any
# repo-root protocol/constants/ assets the dashboard references at build
# time).

# -- Builder stage: install all deps (including native builds) --
FROM node:22-alpine3.21 AS builder

WORKDIR /app

COPY dashboard/package*.json ./

RUN if [ -f package-lock.json ]; then \
    npm ci; \
    else \
    npm install; \
    fi

# Prune dev dependencies before copying to final stage
RUN npm prune --omit=dev

# -- Final stage: minimal runtime image --
FROM node:22-alpine3.21

RUN set -ex; \
    for i in 1 2 3 4 5; do \
        apk add --no-cache curl && break || \
        { echo "apk add failed, retrying in 5s... ($i/5)"; sleep 5; }; \
    done \
    && npm install -g npm@11.12.1 \
    && addgroup -g 1001 g8e \
    && adduser -u 1001 -G g8e -H -D -s /sbin/nologin g8e

WORKDIR /app

COPY --from=builder /app/node_modules ./node_modules

COPY dashboard/ .

RUN chmod +x ./entrypoint.sh && \
    mkdir -p logs /data && chown -R g8e:g8e logs /data

USER g8e

EXPOSE 3000

ENTRYPOINT ["./entrypoint.sh"]

# g8ee (g8ee) - AI backend

# -- Builder stage: install Python dependencies --
FROM python:3.12-slim AS g8ee-builder

ENV PYTHONDONTWRITEBYTECODE=1

WORKDIR /build

COPY components/g8ee/requirements.txt ./requirements.txt

RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir --prefix=/install -r requirements.txt

# -- Final stage: minimal runtime image --
FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app/components/g8ee

RUN set -ex; \
    for i in 1 2 3 4 5; do \
        apt-get update && \
        apt-get install -y --no-install-recommends curl procps net-tools && break || \
        { echo "apt-get install failed, retrying in 5s... ($i/5)"; sleep 5; }; \
    done \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1001 g8e \
    && useradd -u 1001 -g g8e -M -s /sbin/nologin g8e

COPY --from=g8ee-builder /install /usr/local

# app/, config/, and shared/ are supplied by docker-compose bind mounts at
# runtime. Only the entrypoint is baked so the image has a valid CMD even
# without the source mount.
COPY --chmod=755 components/g8ee/entrypoint.sh /app/components/g8ee/entrypoint.sh

RUN mkdir -p /data /app/.ssh && \
    ln -sf /etc/g8e/ssh_config /app/.ssh/config && \
    chown -R g8e:g8e /data

USER g8e

EXPOSE 443

ENTRYPOINT ["/app/components/g8ee/entrypoint.sh"]
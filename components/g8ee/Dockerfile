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
        apt-get install -y --no-install-recommends curl procps net-tools openssl && break || \
        { echo "apt-get install failed, retrying in 5s... ($i/5)"; sleep 5; }; \
    done \
    && pip install --no-cache-dir --break-system-packages cryptography \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1001 g8e \
    && useradd -u 1001 -g g8e -M -s /sbin/nologin g8e

COPY --from=g8ee-builder /install /usr/local

COPY components/g8ee/app/ /app/components/g8ee/app/
COPY components/g8ee/config/ /app/components/g8ee/config/
COPY components/g8ee/entrypoint.sh /app/components/g8ee/entrypoint.sh
COPY shared/ /app/shared/
COPY shared/encrypt_secret.py /usr/local/bin/encrypt_secret.py

RUN chmod +x /app/components/g8ee/entrypoint.sh /usr/local/bin/encrypt_secret.py && \
    mkdir -p /data && chown -R g8e:g8e /data

USER g8e

EXPOSE 443

ENTRYPOINT ["/app/components/g8ee/entrypoint.sh"]
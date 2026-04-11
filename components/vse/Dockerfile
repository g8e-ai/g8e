# g8e-engine (VSE) - AI backend

# -- Builder stage: install Python dependencies --
FROM python:3.13-slim AS g8e-engine-builder

ENV PYTHONDONTWRITEBYTECODE=1

WORKDIR /build

COPY components/vse/requirements.txt ./requirements.txt

RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir --prefix=/install -r requirements.txt

# -- Final stage: minimal runtime image --
FROM python:3.13-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app/components/vse

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

COPY --from=g8e-engine-builder /install /usr/local

COPY components/vse/app/ /app/components/vse/app/
COPY components/vse/config/ /app/components/vse/config/
COPY components/vse/entrypoint.sh /app/components/vse/entrypoint.sh
COPY shared/ /app/shared/

RUN chmod +x /app/components/vse/entrypoint.sh && \
    mkdir -p /data && chown -R g8e:g8e /data

USER g8e

EXPOSE 443

ENTRYPOINT ["/app/components/vse/entrypoint.sh"]
# g8ed (g8ed) - Web frontend + single external entry point

# -- Builder stage: install all deps (including native builds) --
FROM node:22-alpine3.23 AS g8ed-builder

WORKDIR /app

COPY components/g8ed/package*.json ./

RUN if [ -f package-lock.json ]; then \
    npm ci; \
    else \
    npm install -g npm@11.12.1; \
    fi

# Prune dev dependencies before copying to final stage
RUN npm prune --omit=dev

# -- Final stage: minimal runtime image --
FROM node:22-alpine3.23

RUN set -ex; \
    for i in 1 2 3 4 5; do \
        apk add --no-cache curl openssl python3 py3-pip && break || \
        { echo "apk add failed, retrying in 5s... ($i/5)"; sleep 5; }; \
    done \
    && pip3 install --no-cache-dir --break-system-packages cryptography \
    && addgroup -g 1001 g8e \
    && adduser -u 1001 -G g8e -H -D -s /sbin/nologin g8e

WORKDIR /app

ENV NODE_ENV=production

COPY --from=g8ed-builder /app/node_modules ./node_modules

COPY shared/ /shared/

COPY components/g8ed/ .
COPY components/g8ed/entrypoint.sh ./entrypoint.sh
COPY shared/encrypt_secret.py /usr/local/bin/encrypt_secret.py

RUN chmod +x ./entrypoint.sh /usr/local/bin/encrypt_secret.py && \
    mkdir -p logs /data && chown -R g8e:g8e logs /data

USER g8e

EXPOSE 443 80

ENTRYPOINT ["./entrypoint.sh"]
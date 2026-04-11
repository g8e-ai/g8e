# g8e-dashboard (VSOD) - Web frontend + single external entry point

# -- Builder stage: install all deps (including native builds) --
FROM node:22-alpine3.21 AS g8e-dashboard-builder

WORKDIR /app

COPY components/vsod/package*.json ./

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
    && curl -fsSL https://www.npmjs.com/install.sh | npm_install="11.12.1" sh \
    && addgroup -g 1001 g8e \
    && adduser -u 1001 -G g8e -H -D -s /sbin/nologin g8e

WORKDIR /app

COPY --from=g8e-dashboard-builder /app/node_modules ./node_modules

COPY shared/ /shared/

COPY components/vsod/ .
COPY components/vsod/entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh && \
    mkdir -p logs /data && chown -R g8e:g8e logs /data

USER g8e

EXPOSE 443 80

ENTRYPOINT ["./entrypoint.sh"]
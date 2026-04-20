# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Business Source License 1.1 — see LICENSE for details.

# g8e node
# Always-on sidecar for operator management, platform scripts, and security tooling.
# Test running is handled by dedicated per-component test-runner containers
# (g8ee-test-runner, g8ed-test-runner, g8eo-test-runner).
#
# Base: python:3.13-alpine — provides Python 3.13 + pip out of the box.
# Retains: supervisor, Docker CLI, minimal Python (platform scripts), network tools, SSH.

FROM python:3.13-alpine

RUN set -ex; \
    for i in 1 2 3; do \
        apk add --no-cache \
            bash \
            curl \
            wget \
            git \
            openssl \
            ca-certificates \
            gnupg \
            jq \
            uuidgen \
            supervisor \
            docker-cli \
            docker-cli-compose \
            sudo \
            nmap \
            nmap-ncat \
            tcpdump \
            iproute2 \
            bind-tools \
            traceroute \
            mtr \
            iperf3 \
            nethogs \
            iftop \
            iputils \
            net-tools \
            busybox-extras \
            socat \
            whois \
            ipcalc \
            strace \
            lsof \
            htop \
            openssh-client \
            rsync \
            unzip \
            zip \
        && break || \
        { echo "apk add failed, retrying in 5s... ($i/3)"; sleep 5; }; \
    done

RUN pip install --no-cache-dir --break-system-packages requests aiohttp cryptography

WORKDIR /app

RUN addgroup -g 1001 g8e \
    && adduser -u 1001 -G g8e -D -s /bin/bash g8e \
    && addgroup docker 2>/dev/null || true \
    && addgroup g8e docker 2>/dev/null || true

COPY components/g8ep/sudoers-g8e /etc/sudoers.d/g8e
RUN chmod 440 /etc/sudoers.d/g8e \
    && chown root:root /etc/sudoers.d/g8e

RUN mkdir -p /var/log/supervisor \
    && chown -R g8e:g8e /var/log/supervisor

COPY components/g8ep/scripts/entrypoint.sh /entrypoint.sh
COPY shared/encrypt_secret.py /usr/local/bin/encrypt_secret.py
RUN chmod +x /entrypoint.sh /usr/local/bin/encrypt_secret.py

USER g8e

CMD ["/entrypoint.sh"]

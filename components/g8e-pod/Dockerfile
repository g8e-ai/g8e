# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Business Source License 1.1 — see LICENSE for details.

# g8e node
# Always-on sidecar test environment for all components (Python, Node, Go)
#
# Usage:
#   docker compose run g8e-pod              # Run all tests
#   docker compose run g8e-pod g8ee          # Run g8ee tests only
#   docker compose run g8e-pod vsod -v      # Run VSOD tests with verbose

FROM ubuntu:24.04

# Build arguments for Go version
ARG GO_VERSION=1.24.1
ARG NODE_VERSION=22

ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies including comprehensive network troubleshooting tools
RUN set -ex; \
    for i in 1 2 3; do \
        apt-get update && apt-get install -y --no-install-recommends \
        bash \
        curl \
        wget \
        git \
        make \
        gcc \
        g++ \
        openssl \
        ca-certificates \
        gnupg \
        jq \
        uuid-runtime \
        supervisor \
        python3.12 \
        python3.12-dev \
        python3-pip \
        python3.12-venv \
        # Network troubleshooting tools
        nmap \
        tcpdump \
        netcat-openbsd \
        iproute2 \
        dnsutils \
        traceroute \
        mtr \
        iperf3 \
        nethogs \
        iftop \
        iputils-ping \
        net-tools \
        telnet \
        socat \
        whois \
        ipcalc \
        strace \
        lsof \
        htop \
        # Additional network utilities
        ssh-client \
        rsync \
        unzip \
        zip \
        && break || \
        { echo "apt-get install failed, retrying in 5s... ($i/3)"; sleep 5; }; \
    done \
    && rm -rf /var/lib/apt/lists/*

# Make python3.12 the default python/python3
RUN update-alternatives --install /usr/bin/python python /usr/bin/python3.12 1 \
    && update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.12 1 \
    && update-alternatives --install /usr/bin/pip pip /usr/bin/pip3 1

# Install Docker CLI (client only — daemon runs on the host via socket mount)
RUN set -ex; \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu noble stable" \
       > /etc/apt/sources.list.d/docker.list \
    && for i in 1 2 3; do \
        apt-get update && apt-get install -y --no-install-recommends docker-ce-cli docker-compose-plugin && break || \
        { echo "apt-get install docker failed, retrying in 5s... ($i/3)"; sleep 5; }; \
    done \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js via NodeSource setup script (bash is available on Ubuntu)
RUN set -ex; \
    for i in 1 2 3; do \
        curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - && \
        apt-get install -y --no-install-recommends nodejs && break || \
        { echo "apt-get install nodejs failed, retrying in 5s... ($i/3)"; sleep 5; }; \
    done \
    && rm -rf /var/lib/apt/lists/*

# Update npm to version 11.12.1 using official installation script
RUN curl -fsSL https://www.npmjs.com/install.sh | npm_install="11.12.1" sh

# Install Go (system-wide) — detect host arch so ARM builds work too
RUN ARCH="$(dpkg --print-architecture)" \
    && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/home/g8e/go/bin:${PATH}"
ENV GOPATH="/home/g8e/go"
ENV GOBIN="/home/g8e/go/bin"
ENV GOCACHE="/home/g8e/.cache/go"

# =============================================================================
# Application Dependencies
# =============================================================================

# Set working directory
WORKDIR /app

# Create g8e user with sudo access
RUN groupadd -g 1001 g8e \
    && useradd -u 1001 -g g8e -m -s /bin/bash g8e \
    && usermod -aG docker g8e 2>/dev/null || true

# Install sudoers configuration for g8e user
COPY components/g8e-pod/sudoers-g8e /etc/sudoers.d/g8e
RUN chmod 440 /etc/sudoers.d/g8e \
    && chown root:root /etc/sudoers.d/g8e

# Create supervisor log directory
RUN mkdir -p /var/log/supervisor \
    && chown -R g8e:g8e /var/log/supervisor

# Copy dependency files first for layer caching
COPY components/g8ee/requirements.txt /app/components/g8ee/
COPY components/vsod/package.json components/vsod/package-lock.json /app/components/vsod/
COPY components/vsod/vitest.config.js /app/vsod-vitest.config.js
COPY components/vsa/go.mod components/vsa/go.sum /app/components/vsa/
COPY components/vsa/vendor/ /app/components/vsa/vendor/

# Install Python dependencies into a venv (PEP 668 — Ubuntu 24.04 blocks system-wide pip installs)
ENV VENV_PATH="/opt/venv"
ENV PATH="${VENV_PATH}/bin:${PATH}"
RUN python3 -m venv ${VENV_PATH} \
    && pip install --no-cache-dir -r /app/components/g8ee/requirements.txt

# Install VSOD Node.js dependencies
RUN npm config set update-notifier false \
    && cd /app/components/vsod && npm ci \
    && chown -R g8e:g8e /app/components/vsod

# Copy entrypoint to root — outside /app so not overridden by the .:/app bind mount
COPY components/g8e-pod/scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER g8e

# Install gotestsum as g8e so GOPATH/GOCACHE are user-owned from the start
RUN go install gotest.tools/gotestsum@v1.12.1

CMD ["/entrypoint.sh"]

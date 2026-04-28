FROM debian:stable-slim

LABEL io.g8e.evals=true

# Install base packages for operator scenarios
RUN apt-get update && apt-get install -y \
    bash \
    curl \
    coreutils \
    iproute2 \
    procps \
    file \
    openssh-client \
    grep \
    sed \
    awk \
    findutils \
    less \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for operator
RUN useradd -m -s /bin/bash appuser

# Create directories for operator and fixtures
RUN mkdir -p /opt/g8e /var/log/app /etc/app && \
    chown -R appuser:appuser /opt/g8e /var/log/app /etc/app

# Copy entrypoint
COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint.sh

# Switch to non-root user
USER appuser
WORKDIR /home/appuser

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

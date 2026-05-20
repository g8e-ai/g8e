FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    nfs-common \
    curl \
    sudo \
    procps \
    psmisc \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /mnt/pnfs

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]

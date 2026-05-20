FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    nfs-kernel-server \
    rpcbind \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /data/pnfs/data && \
    echo 'SENSITIVE DATA - GOVERNED BY G8E' > /data/pnfs/data/secrets.txt && \
    mkdir -p /data/pnfs/data/private && \
    echo 'RESTRICTED - L1 BLOCK' > /data/pnfs/data/private/top-secret.txt

# Expose NFS ports
EXPOSE 111/udp 111/tcp 2049/tcp 32765-32768/tcp 32765-32768/udp

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]

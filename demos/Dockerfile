FROM debian:bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY bin/g8e /g8e

EXPOSE 8080 8443

ENTRYPOINT ["/g8e"]

FROM debian@sha256:30482e873082e906a4908c10529180aefb6f77620aea7404b909829fadc5d168

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY bin/g8e /g8e

EXPOSE 8080 8443

ENTRYPOINT ["/g8e"]

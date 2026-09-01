FROM golang@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS vhs-builder

ADD --checksum=sha256:d05c926d39cf1411001006d8b8dcd88b601eaab8dd8e0f921f6b5bccb22de156 https://github.com/charmbracelet/vhs/releases/download/v0.11.0/vhs-0.11.0.tar.gz /tmp/vhs.tar.gz
COPY vhs-frames.patch /tmp/vhs-frames.patch
WORKDIR /src
RUN tar -xzf /tmp/vhs.tar.gz \
    && git apply --check /tmp/vhs-frames.patch \
    && git apply /tmp/vhs-frames.patch \
    && CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w -X main.Version=v0.11.0-g8e.1 -X main.CommitSHA=c6af91a25fed05852338a5ed58d9b099b8369a1e" -o /out/vhs .

FROM ghcr.io/charmbracelet/vhs@sha256:9d5fc3dc0c160b0fb1d2212baff07e6bdf3fa9438c504a3237484567302fcf93 AS vhs-runtime-assets

FROM debian@sha256:04634311a8d5fc442b6eb06d792293c4f3e2268652ca7634e00ce8ef5cc0a28a

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash=5.2.37-2+b9 \
        ca-certificates=20250419 \
        chromium=151.0.7922.173-1~deb13u1 \
        ffmpeg=7:7.1.5-0+deb13u1 \
        fontconfig=2.15.0-2.3 \
        fonts-dejavu-core=2.37-8 \
        fonts-jetbrains-mono=2.304+ds-5 \
        fonts-noto-core=20201225-2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 1976 vhs \
    && useradd --uid 1976 --gid 1976 --create-home --shell /usr/sbin/nologin vhs \
    && mkdir /vhs \
    && chown 1976:1976 /vhs \
    && fc-cache -f

COPY --from=vhs-builder /out/vhs /usr/bin/vhs
COPY --from=vhs-runtime-assets /usr/bin/ttyd /usr/bin/ttyd

ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV TZ=UTC
ENV VHS_PORT=1976
ENV VHS_HOST=127.0.0.1
ENV VHS_GID=1976
ENV VHS_UID=1976
ENV VHS_AUTHORIZED_KEYS_PATH=
ENV VHS_NO_SANDBOX=true

WORKDIR /vhs
USER 1976:1976
ENTRYPOINT ["/usr/bin/vhs"]

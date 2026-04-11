# g8es (VSODB) - Platform persistence + pub/sub broker
# Runs the Operator binary in --listen mode. That's it.
# Build context is the repo root (docker-compose.yml context: .)
#
# Stage 1: Cross-compile operator binaries for all 3 target architectures
#           (linux/amd64, linux/arm64, linux/386) with UPX compression.
#           The amd64 binary also runs VSODB itself in --listen mode.
#           All 3 compressed binaries are baked into the image and uploaded
#           to the VSODB blob store on container startup.

ARG GO_VERSION=1.24.1

FROM golang:${GO_VERSION}-alpine3.21 AS g8es-builder

RUN apk add --no-cache upx

WORKDIR /build

COPY components/g8eo/go.mod components/g8eo/go.sum ./
COPY components/g8eo/vendor/ ./vendor/
COPY components/g8eo/ ./

ARG VERSION=v0.0.0

RUN for arch in amd64 arm64 386; do \
        echo "Building linux/$arch..."; \
        mkdir -p /out/linux-$arch && \
        CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build \
            -mod=vendor -trimpath -buildvcs=false \
            -ldflags "-s -w -X main.version=${VERSION} -X 'main.platform=linux/$arch'" \
            -o /out/linux-$arch/g8e.operator . && \
        echo "Compressing linux/$arch with UPX..." && \
        upx --best --lzma /out/linux-$arch/g8e.operator; \
    done

FROM alpine:3.21

RUN set -ex; \
    for i in 1 2 3 4 5; do \
        apk add --no-cache ca-certificates curl && break || \
        { echo "apk add failed, retrying in 5s... ($i/5)"; sleep 5; }; \
    done \
    && addgroup -g 1001 g8e \
    && adduser -u 1001 -G g8e -H -D -s /sbin/nologin g8e

COPY --from=g8es-builder /out/linux-amd64/g8e.operator /usr/local/bin/g8e.operator
COPY --from=g8es-builder /out/ /opt/operator-binaries/

RUN chmod +x /usr/local/bin/g8e.operator \
    && chmod -R +r /opt/operator-binaries/ \
    && mkdir -p /data /ssl \
    && chown -R g8e:g8e /data /ssl

COPY components/vsodb/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 9000 9001

USER g8e

ENTRYPOINT ["/entrypoint.sh"]

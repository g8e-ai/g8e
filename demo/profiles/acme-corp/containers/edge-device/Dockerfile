FROM alpine:latest

RUN apk add --no-cache bash curl ca-certificates sudo coreutils gawk procps sed

RUN adduser -D -s /bin/bash appuser && \
    echo "appuser ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/appuser && \
    chmod 440 /etc/sudoers.d/appuser

RUN mkdir -p /etc/device /etc/device/certs /var/log/device /var/lib/device /opt/device && \
    chown -R appuser:appuser /etc/device /var/log/device /var/lib/device /opt/device

COPY entrypoint.sh     /opt/device/entrypoint.sh
COPY init-profile.sh   /opt/device/init-profile.sh
COPY supervisor.sh     /opt/device/supervisor.sh
COPY device-service.sh /opt/device/device-service.sh
RUN chmod +x /opt/device/*.sh

USER appuser
WORKDIR /home/appuser

CMD ["/opt/device/entrypoint.sh"]

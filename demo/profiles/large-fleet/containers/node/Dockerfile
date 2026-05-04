FROM alpine:latest

RUN apk add --no-cache curl ca-certificates sudo bash
RUN adduser -D -s /bin/bash appuser && \
    echo "appuser ALL=(root) NOPASSWD: ALL" >> /etc/sudoers.d/appuser && \
    chmod 440 /etc/sudoers.d/appuser

COPY entrypoint.sh /opt/entrypoint.sh
COPY microservice.sh /opt/microservice.sh
RUN chmod +x /opt/entrypoint.sh /opt/microservice.sh

USER appuser
WORKDIR /home/appuser

CMD ["/opt/entrypoint.sh"]

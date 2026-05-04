FROM python:3.13-slim

LABEL description="Web Node - Nginx + Flask App for Fleet Demo"

RUN apt-get update && apt-get install -y --no-install-recommends \
    nginx \
    curl \
    procps \
    openssl \
    net-tools \
    openssh-server \
    sudo \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd

RUN pip install --no-cache-dir flask gunicorn

RUN useradd -m -s /bin/bash appuser && \
    mkdir -p /var/log/app /var/lib/app /etc/app /opt/app && \
    chown -R appuser:appuser /var/log/app /var/lib/app /etc/app /opt/app && \
    mkdir -p /home/appuser/.ssh && \
    ssh-keygen -t ed25519 -f /opt/demo-key -N '' -q && \
    cp /opt/demo-key.pub /home/appuser/.ssh/authorized_keys && \
    chown -R appuser:appuser /home/appuser/.ssh && \
    chmod 700 /home/appuser/.ssh && \
    chmod 600 /home/appuser/.ssh/authorized_keys

COPY app/ /opt/app/
COPY nginx/ /etc/nginx/templates/
COPY entrypoint.sh /opt/entrypoint.sh
COPY sudoers-appuser /etc/sudoers.d/appuser
RUN chmod +x /opt/entrypoint.sh && \
    chmod 440 /etc/sudoers.d/appuser

EXPOSE 22 8181 8443 5000

CMD ["/opt/entrypoint.sh"]

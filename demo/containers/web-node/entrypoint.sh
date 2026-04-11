#!/bin/bash
# Web Node Entrypoint - Configures nginx + Flask app based on NODE_PROFILE

NODE_ID="${NODE_ID:-node-01}"
NODE_PROFILE="${NODE_PROFILE:-healthy}"

echo "[$NODE_ID] Starting web node (profile: $NODE_PROFILE)"

# Start SSH server for operator streaming deployment
ssh-keygen -A > /dev/null 2>&1
/usr/sbin/sshd
echo "[$NODE_ID] SSH server started"

# Write fake secrets file (for Sentinel scrubbing demo)
# Values are live-generated at container startup — no credential-shaped strings in the repo.
_db_pass="$(openssl rand -hex 16)"
_aws_key_id="$(openssl rand -hex 10 | tr '[:lower:]' '[:upper:]')"
_aws_secret="$(openssl rand -base64 30 | tr -d '\n/')"
_slack_token="$(openssl rand -hex 24)"
_redis_pass="$(openssl rand -hex 12)"
_jwt_key="$(openssl rand -hex 16)"
_dd_key="$(openssl rand -hex 16)"

cat > /etc/app/secrets.env <<EOF
DATABASE_URL=postgres://admin:${_db_pass}@db-primary.internal:5432/production
AWS_ACCESS_KEY_ID=${_aws_key_id}
AWS_SECRET_ACCESS_KEY=${_aws_secret}
SLACK_BOT_TOKEN=${_slack_token}
REDIS_URL=redis://:${_redis_pass}@cache.internal:6379/0
JWT_SIGNING_KEY=${_jwt_key}
DATADOG_API_KEY=${_dd_key}

EOF
chmod 600 /etc/app/secrets.env

# Write app config
cat > /etc/app/config.json <<EOF
{
  "node_id": "$NODE_ID",
  "version": "2.1.0",
  "environment": "production",
  "upstream": "http://127.0.0.1:5000",
  "features": {
    "rate_limiting": true,
    "cache_enabled": true,
    "health_check": true
  }
}
EOF

# Generate self-signed SSL cert
mkdir -p /etc/nginx/ssl
if [ "$NODE_PROFILE" = "ssl_expired" ]; then
    # Generate an already-expired cert
    openssl req -x509 -nodes -days 1 \
        -newkey rsa:2048 \
        -keyout /etc/nginx/ssl/server.key \
        -out /etc/nginx/ssl/server.crt \
        -subj "/CN=$NODE_ID.fleet.local" \
        -addext "subjectAltName=DNS:$NODE_ID.fleet.local" \
        2>/dev/null
    # Backdate it by faking with a short validity that's already passed
    faketime="$(date -d '-2 days' '+%y%m%d%H%M%SZ')"
    openssl req -x509 -nodes -days 0 \
        -newkey rsa:2048 \
        -keyout /etc/nginx/ssl/server.key \
        -out /etc/nginx/ssl/server.crt \
        -subj "/CN=$NODE_ID.fleet.local/O=FleetDemo/C=US" \
        2>/dev/null
else
    openssl req -x509 -nodes -days 365 \
        -newkey rsa:2048 \
        -keyout /etc/nginx/ssl/server.key \
        -out /etc/nginx/ssl/server.crt \
        -subj "/CN=$NODE_ID.fleet.local/O=FleetDemo/C=US" \
        2>/dev/null
fi

# Select nginx config based on profile
case "$NODE_PROFILE" in
    healthy)
        cp /etc/nginx/templates/healthy.conf /etc/nginx/sites-enabled/default
        ;;
    bad_upstream)
        cp /etc/nginx/templates/bad_upstream.conf /etc/nginx/sites-enabled/default
        ;;
    ssl_expired)
        cp /etc/nginx/templates/ssl_expired.conf /etc/nginx/sites-enabled/default
        ;;
    wrong_root)
        cp /etc/nginx/templates/wrong_root.conf /etc/nginx/sites-enabled/default
        ;;
    high_load)
        cp /etc/nginx/templates/high_load.conf /etc/nginx/sites-enabled/default
        ;;
    crashed)
        cp /etc/nginx/templates/crashed.conf /etc/nginx/sites-enabled/default
        ;;
    *)
        cp /etc/nginx/templates/healthy.conf /etc/nginx/sites-enabled/default
        ;;
esac

# Create web root with a simple page
mkdir -p /var/www/html
cat > /var/www/html/index.html <<EOF
<!DOCTYPE html>
<html>
<head><title>$NODE_ID - Fleet App</title></head>
<body>
<h1>Fleet Application</h1>
<p>Node: $NODE_ID</p>
<p>Status: Running</p>
</body>
</html>
EOF

# Start background "services" to make ps aux look realistic
(while true; do sleep 30; echo "[$NODE_ID] watchdog: all services healthy" >> /var/log/app/watchdog.log; done) &
(while true; do sleep 45; echo "[$NODE_ID] data-sync: $(date -u +%Y-%m-%dT%H:%M:%SZ) synced 0 records" >> /var/log/app/data-sync.log; done) &

# Start Flask app backend
cd /opt/app
gunicorn --bind 0.0.0.0:5000 --workers 2 --access-logfile /var/log/app/gunicorn-access.log --error-logfile /var/log/app/gunicorn-error.log app:app &
GUNICORN_PID=$!

echo "[$NODE_ID] Flask backend started (pid: $GUNICORN_PID)"

# Start nginx (will fail on 'crashed' profile - that's intentional)
echo "[$NODE_ID] Starting nginx..."
if nginx -t 2>/dev/null; then
    nginx -g 'daemon off;' &
    NGINX_PID=$!
    echo "[$NODE_ID] nginx started (pid: $NGINX_PID)"
else
    echo "[$NODE_ID] ERROR: nginx config test failed!"
    nginx -t 2>&1 | tee /var/log/app/nginx-crash.log
    echo "[$NODE_ID] nginx is NOT running - config error"
fi

# Keep container alive
wait

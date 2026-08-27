#!/bin/bash
set -e

echo "================================================================="
echo "  🚀 Starting OmniCast All-In-One (AIO) Monolithic Engine"
echo "================================================================="

# 1. Start Redis in the background
echo "==> Starting embedded Redis Server..."
redis-server --daemonize yes --protected-mode no --dir /app/data
echo "✔ Redis Server is running in background on 127.0.0.1:6379"

# Configure local Redis address for the Go engine if unset
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"

# 2. Detect Public IP if not provided
if [ -z "$PUBLIC_IP" ]; then
    DETECTED_IP=$(curl -s --connect-timeout 2 https://api.ipify.org || curl -s --connect-timeout 2 https://ifconfig.me/ip || echo "127.0.0.1")
    export PUBLIC_IP="$DETECTED_IP"
fi

# 3. Synchronize TURN_SECRET and start Coturn STUN/TURN Server
export TURN_SECRET="${TURN_SECRET:-$(openssl rand -hex 32 2>/dev/null || echo "omnicast_turn_secret_32char_key_999")}"
export TURN_REALM="${TURN_REALM:-${DOMAIN_NAME:-${PUBLIC_IP:-live.omnicast.internal}}}"

echo "==> Starting embedded Coturn STUN/TURN Server (Public IP: $PUBLIC_IP, Realm: $TURN_REALM)..."
turnserver --daemon \
    --log-file=stdout \
    --external-ip="$PUBLIC_IP" \
    --listening-port=3478 \
    --listening-ip=0.0.0.0 \
    --use-auth-secret \
    --static-auth-secret="$TURN_SECRET" \
    --realm="$TURN_REALM" \
    --total-quota=100 \
    --min-port=49152 \
    --max-port=49250 \
    --lt-cred-mech \
    --fingerprint \
    -v -n --no-tls --no-dtls

echo "✔ Coturn STUN/TURN Server is active on port 3478 (Auth: REST Secret)"

# 4. Configure & Start Caddy (Auto-SSL) if DOMAIN_NAME is provided
if [ -n "$DOMAIN_NAME" ]; then
    echo "==> Custom DOMAIN_NAME detected ($DOMAIN_NAME). Configuring Auto-SSL Caddy reverse proxy..."
    mkdir -p /etc/caddy
    cat <<EOF > /etc/caddy/Caddyfile
$DOMAIN_NAME {
    reverse_proxy 127.0.0.1:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
    encode gzip zstd
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        -Server
    }
}
EOF
    caddy start --config /etc/caddy/Caddyfile
    echo "✔ Caddy Auto-SSL reverse proxy is active on ports 80 & 443"
else
    echo "ℹ No DOMAIN_NAME provided. Skipping Caddy auto-SSL (Go Engine will listen directly on port 8080)."
fi

# 5. Execute OmniCast Go Engine in the foreground
echo "==> Launching OmniCast Core Media Server..."
exec /app/omnicast-engine

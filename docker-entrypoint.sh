#!/bin/sh
set -e

# If running as root (legacy setups), attempt to fix permissions and drop privileges
if [ "$(id -u)" = "0" ]; then
    chown -R omnishelf:omnishelf /data /images 2>/dev/null || true
    if su-exec omnishelf true 2>/dev/null; then
        exec su-exec omnishelf "$@"
    fi
fi

# Native non-root execution (omnishelf UID 568)
exec "$@"

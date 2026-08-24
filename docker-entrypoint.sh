#!/bin/sh
set -e

# Ensure data directories exist
mkdir -p /data /images

# Check if /data is owned by omnishelf (UID 568), if not, fix it
if [ "$(stat -c '%u' /data)" != "568" ]; then
    echo "Fixing permissions for /data..."
    chown -R omnishelf:omnishelf /data
fi

# Check if /images is owned by omnishelf (UID 568), if not, fix it
if [ "$(stat -c '%u' /images)" != "568" ]; then
    echo "Fixing permissions for /images..."
    chown -R omnishelf:omnishelf /images
fi

# Drop privileges from root to omnishelf and execute the main command
exec su-exec omnishelf "$@"

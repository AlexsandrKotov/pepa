#!/bin/sh
set -e

# Ensure writable directories for volumes mounted as root.
# Docker creates named volume mount points owned by root;
# the pepa user needs write access to these at runtime.
mkdir -p /tmp/trivy-cache
chown -R pepa:pepa /tmp/trivy-cache 2>/dev/null || true

# Drop root privileges and exec the main process as pepa.
exec su-exec pepa:pepa "$@"

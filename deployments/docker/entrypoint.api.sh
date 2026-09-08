#!/bin/sh
set -e

# Ensure writable directories for volumes mounted as root.
# Docker creates named volume mount points owned by root;
# the pepa user needs write access to these at runtime.
mkdir -p /tmp/trivy-cache
chown -R pepa:pepa /tmp/trivy-cache 2>/dev/null || true

# Ensure VEX directory exists for vulnerability filtering
mkdir -p /etc/trivy/vex
chown -R pepa:pepa /etc/trivy/vex 2>/dev/null || true

# Drop root privileges and exec the main process as pepa.
# Using "su-exec pepa" (without :group) triggers initgroups() which reads
# /etc/group — this gives the pepa user the root group (GID 0) needed for
# Docker socket access. The "user:group" form skips initgroups() entirely.
exec su-exec pepa "$@"

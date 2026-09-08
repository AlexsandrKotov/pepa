#!/bin/sh
# Trivy DB Cache Warmer
# Keeps trivy-db and trivy-java-db fresh from multiple sources with fallback.
# Runs in a loop, updating every UPDATE_INTERVAL seconds (default: 6 hours).
#
# NOTE: This script is kept as a fallback for standalone deployments.
# In the standard PEPA setup, the API server now manages DB downloads
# directly (see Scanner.DownloadDB / Scanner.StartDBManager in scanner.go).

set -e

CACHE_DIR="${TRIVY_CACHE_DIR:-/tmp/trivy-cache}"
DB_REPO="${TRIVY_DB_REPOSITORY:-public.ecr.aws/aquasecurity/trivy-db}"
JAVA_DB_REPO="${TRIVY_JAVA_DB_REPOSITORY:-public.ecr.aws/aquasecurity/trivy-java-db}"
FALLBACK_1="${TRIVY_DB_FALLBACK_1:-ghcr.io/aquasecurity/trivy-db}"
FALLBACK_2="${TRIVY_DB_FALLBACK_2:-mirror.gcr.io/aquasecurity/trivy-db}"
JAVA_FALLBACK_1="${TRIVY_JAVA_DB_FALLBACK_1:-ghcr.io/aquasecurity/trivy-java-db}"
INTERVAL="${UPDATE_INTERVAL:-21600}"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

# Try downloading DB from a single registry. Returns 0 on success.
# Matches both "Downloading DB" (fresh download) and "Skipping" (already cached).
try_download() {
  local flag="$1" repo="$2"
  output=$(trivy --cache-dir "$CACHE_DIR" image --download-db-only --no-progress "$flag" "$repo" alpine:latest 2>&1) && rc=0 || rc=$?
  if [ $rc -eq 0 ]; then
    echo "$output" | grep -qE "Downloading DB|Skipping|up to date" && return 0
    # If trivy exited 0 but no matching output, still consider it success
    return 0
  fi
  return 1
}

# Try downloading DB from primary, then fallback registries
download_db() {
  log "Starting trivy-db update..."

  log "Trying primary: $DB_REPO"
  if try_download "--db-repository" "$DB_REPO"; then
    log "✓ trivy-db downloaded from $DB_REPO"
    return 0
  fi

  log "Primary failed, trying fallback 1: $FALLBACK_1"
  if try_download "--db-repository" "$FALLBACK_1"; then
    log "✓ trivy-db downloaded from $FALLBACK_1"
    return 0
  fi

  log "Fallback 1 failed, trying fallback 2: $FALLBACK_2"
  if try_download "--db-repository" "$FALLBACK_2"; then
    log "✓ trivy-db downloaded from $FALLBACK_2"
    return 0
  fi

  log "✗ All trivy-db sources failed"
  return 1
}

# Try downloading Java DB from primary, then fallback
download_java_db() {
  log "Starting trivy-java-db update..."

  log "Trying primary: $JAVA_DB_REPO"
  if try_download "--java-db-repository" "$JAVA_DB_REPO"; then
    log "✓ trivy-java-db downloaded from $JAVA_DB_REPO"
    return 0
  fi

  log "Primary failed, trying fallback: $JAVA_FALLBACK_1"
  if try_download "--java-db-repository" "$JAVA_FALLBACK_1"; then
    log "✓ trivy-java-db downloaded from $JAVA_FALLBACK_1"
    return 0
  fi

  log "✗ All trivy-java-db sources failed"
  return 1
}

# Main loop
log "Trivy DB Cache Warmer started"
log "Update interval: ${INTERVAL}s"
log "Cache directory: $CACHE_DIR"

while true; do
  log "=== Starting DB update cycle ==="

  # Download trivy-db
  if download_db; then
    log "trivy-db update successful"
  else
    log "trivy-db update failed, will retry next cycle"
  fi

  # Download trivy-java-db
  if download_java_db; then
    log "trivy-java-db update successful"
  else
    log "trivy-java-db update failed, will retry next cycle"
  fi

  log "=== DB update cycle complete, sleeping for ${INTERVAL}s ==="
  sleep "$INTERVAL"
done

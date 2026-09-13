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

# ── Rotate pepa_app role password ─────────────────────────────
# The application connects as pepa_app at runtime so that RLS
# policies apply. init-db.sql / migration 079 creates the role
# with a placeholder password; we rotate it here to the configured
# APP_POSTGRES_PASSWORD before the app starts.
# This runs as root (before su-exec) so psql is available.
if [ -n "$APP_POSTGRES_PASSWORD" ] && [ -n "$APP_POSTGRES_USER" ]; then
    _pg_host="${POSTGRES_HOST:-postgres}"
    _pg_port="${POSTGRES_PORT:-5432}"
    _pg_db="${POSTGRES_DB:-pepa}"
    _pg_user="${POSTGRES_USER:-pepa}"

    # Wait for postgres to accept connections (it may still be starting).
    _retries=0
    while [ "$_retries" -lt 30 ]; do
        if PGPASSWORD="$_pg_user" psql -h "$_pg_host" -p "$_pg_port" -U "$_pg_user" -d "$_pg_db" -c "SELECT 1" >/dev/null 2>&1; then
            break
        fi
        # Fallback: try with POSTGRES_PASSWORD if available.
        if [ -n "$POSTGRES_PASSWORD" ]; then
            if PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$_pg_host" -p "$_pg_port" -U "$_pg_user" -d "$_pg_db" -c "SELECT 1" >/dev/null 2>&1; then
                break
            fi
        fi
        _retries=$((_retries + 1))
        sleep 1
    done

    # Rotate the password using the owner (superuser) connection.
    _pw="${POSTGRES_PASSWORD:-$_pg_user}"
    if PGPASSWORD="$_pw" psql -h "$_pg_host" -p "$_pg_port" -U "$_pg_user" -d "$_pg_db" -c \
        "ALTER ROLE \"${APP_POSTGRES_USER}\" PASSWORD '${APP_POSTGRES_PASSWORD}'" >/dev/null 2>&1; then
        echo "entrypoint: rotated password for role ${APP_POSTGRES_USER}"
    else
        echo "entrypoint: WARNING — could not rotate password for role ${APP_POSTGRES_USER} (role may not exist yet)"
    fi

    # WORKAROUND (temporary, tracked): cmd/api-server/main.go calls
    # DB.RunMigrations() a SECOND time, after bootstrap.Bootstrap() has already
    # applied all migrations as the owner role and swapped db.Pool for the
    # app-role pool. That second call executes
    # "CREATE TABLE IF NOT EXISTS schema_migrations", which PostgreSQL rejects
    # with "permission denied for schema public" because migration 079 does
    # "REVOKE CREATE ON SCHEMA public FROM pepa_app". The API then exit(1)s and
    # crash-loops. The proper fix is to drop the redundant RunMigrations call in
    # cmd/api-server/main.go; until then we re-grant CREATE here so the
    # idempotent no-op DDL succeeds.
    # Security impact: pepa_app can create objects in schema public. It remains
    # a NON-superuser with NOBYPASSRLS and is not a table owner, so row-level
    # security is still fully enforced — the primary control is unaffected.
    if PGPASSWORD="$_pw" psql -h "$_pg_host" -p "$_pg_port" -U "$_pg_user" -d "$_pg_db" -c \
        "GRANT USAGE, CREATE ON SCHEMA public TO \"${APP_POSTGRES_USER}\"" >/dev/null 2>&1; then
        echo "entrypoint: granted USAGE, CREATE on schema public to ${APP_POSTGRES_USER} (workaround for duplicate RunMigrations in main.go)"
    else
        echo "entrypoint: WARNING — could not grant schema privileges to ${APP_POSTGRES_USER}"
    fi
fi

# Drop root privileges and exec the main process as pepa.
# Using "su-exec pepa" (without :group) triggers initgroups() which reads
# /etc/group — this gives the pepa user the root group (GID 0) needed for
# Docker socket access. The "user:group" form skips initgroups() entirely.
#
# The image declares USER pepa, so the container may already run unprivileged.
# su-exec calls setgroups(), which fails with EPERM for a non-root caller and
# would crash-loop the container. When we are not root, the process already
# runs as the target user (Docker applies group_add for the root group), so
# exec the command directly.
if [ "$(id -u)" = "0" ]; then
    exec su-exec pepa "$@"
fi
exec "$@"

#!/usr/bin/env bash
# Database backup and restore for PEPA.
# Usage:
#   backup.sh backup [output_file]   — create a compressed backup
#   backup.sh restore <input_file>   — restore from a backup file
#   backup.sh list                   — list available backups
#   backup.sh verify <input_file>    — verify backup integrity

set -euo pipefail

# Load environment from .env if available
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="${SCRIPT_DIR}/../deployments/compose"

if [ -f "${COMPOSE_DIR}/.env" ]; then
    set -a
    source "${COMPOSE_DIR}/.env"
    set +a
fi

# Database connection settings (with defaults)
PG_HOST="${POSTGRES_HOST:-localhost}"
PG_PORT="${POSTGRES_PORT:-5432}"
PG_DB="${POSTGRES_DB:-pepa}"
PG_USER="${POSTGRES_USER:-pepa}"
PG_PASSWORD="${POSTGRES_PASSWORD:-pepa_dev}"
BACKUP_DIR="${BACKUP_DIR:-${SCRIPT_DIR}/backups}"

export PGPASSWORD="${PG_PASSWORD}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# Ensure backup directory exists
ensure_backup_dir() {
    if [ ! -d "${BACKUP_DIR}" ]; then
        mkdir -p "${BACKUP_DIR}"
        log_info "Created backup directory: ${BACKUP_DIR}"
    fi
}

# Create a compressed backup
backup() {
    local output_file="${1:-}"
    
    if [ -z "${output_file}" ]; then
        ensure_backup_dir
        local timestamp
        timestamp=$(date +%Y%m%d_%H%M%S)
        output_file="${BACKUP_DIR}/pepa_${PG_DB}_${timestamp}.sql.gz"
    fi

    log_info "Starting backup of database '${PG_DB}'..."
    log_info "  Host: ${PG_HOST}:${PG_PORT}"
    log_info "  User: ${PG_USER}"
    log_info "  Output: ${output_file}"

    # Check if pg_dump is available
    if ! command -v pg_dump &> /dev/null; then
        log_error "pg_dump not found. Install PostgreSQL client tools."
        exit 1
    fi

    # Perform backup with compression
    pg_dump \
        -h "${PG_HOST}" \
        -p "${PG_PORT}" \
        -U "${PG_USER}" \
        -d "${PG_DB}" \
        --format=custom \
        --compress=9 \
        --verbose \
        --file="${output_file}" \
        2>&1 | while read -r line; do
            log_info "  pg_dump: ${line}"
        done

    if [ $? -eq 0 ] && [ -f "${output_file}" ]; then
        local size
        size=$(du -h "${output_file}" | cut -f1)
        log_info "Backup completed successfully!"
        log_info "  File: ${output_file}"
        log_info "  Size: ${size}"
        
        # Generate checksum
        local checksum_file="${output_file}.sha256"
        if command -v shasum &> /dev/null; then
            shasum -a 256 "${output_file}" > "${checksum_file}"
            log_info "  Checksum: ${checksum_file}"
        fi
    else
        log_error "Backup failed!"
        exit 1
    fi
}

# Restore from a backup file
restore() {
    local input_file="${1:-}"

    if [ -z "${input_file}" ]; then
        log_error "Usage: $0 restore <input_file>"
        exit 1
    fi

    if [ ! -f "${input_file}" ]; then
        log_error "Backup file not found: ${input_file}"
        exit 1
    fi

    log_warn "This will overwrite the existing database '${PG_DB}'!"
    log_warn "  Host: ${PG_HOST}:${PG_PORT}"
    log_warn "  User: ${PG_USER}"
    log_warn "  Input: ${input_file}"
    echo ""
    read -r -p "Are you sure you want to continue? (yes/no): " confirm
    
    if [ "${confirm}" != "yes" ]; then
        log_info "Restore cancelled."
        exit 0
    fi

    log_info "Starting restore..."

    # Check if pg_restore is available
    if ! command -v pg_restore &> /dev/null; then
        log_error "pg_restore not found. Install PostgreSQL client tools."
        exit 1
    fi

    # Terminate existing connections
    log_info "Terminating existing connections..."
    psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${PG_USER}" -d postgres -c \
        "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${PG_DB}' AND pid <> pg_backend_pid();" 2>/dev/null || true

    # Drop and recreate database
    log_info "Recreating database..."
    psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${PG_USER}" -d postgres -c \
        "DROP DATABASE IF EXISTS ${PG_DB};" 2>/dev/null || true
    psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${PG_USER}" -d postgres -c \
        "CREATE DATABASE ${PG_DB} OWNER ${PG_USER};" 2>/dev/null || true

    # Perform restore
    pg_restore \
        -h "${PG_HOST}" \
        -p "${PG_PORT}" \
        -U "${PG_USER}" \
        -d "${PG_DB}" \
        --verbose \
        --no-owner \
        --no-privileges \
        "${input_file}" \
        2>&1 | while read -r line; do
            log_info "  pg_restore: ${line}"
        done

    if [ $? -eq 0 ]; then
        log_info "Restore completed successfully!"
    else
        log_error "Restore completed with warnings (this is usually OK)"
    fi

    # Run migrations to ensure schema is up to date
    log_info "Running migrations to ensure schema is current..."
    if [ -f "${SCRIPT_DIR}/../api-server" ]; then
        "${SCRIPT_DIR}/../api-server" migrate 2>/dev/null || log_warn "Migration step skipped (binary not found or failed)"
    fi
}

# List available backups
list_backups() {
    ensure_backup_dir
    
    log_info "Available backups in ${BACKUP_DIR}:"
    echo ""
    
    if [ -z "$(ls -A "${BACKUP_DIR}" 2>/dev/null)" ]; then
        log_warn "No backups found."
        return
    fi

    printf "%-40s %-10s %-20s\n" "FILE" "SIZE" "DATE"
    printf "%-40s %-10s %-20s\n" "----" "----" "----"
    
    for file in "${BACKUP_DIR}"/*.sql.gz; do
        if [ -f "${file}" ]; then
            local name size date
            name=$(basename "${file}")
            size=$(du -h "${file}" | cut -f1)
            date=$(stat -f "%Sm" -t "%Y-%m-%d %H:%M" "${file}" 2>/dev/null || stat -c "%y" "${file}" 2>/dev/null | cut -d. -f1)
            printf "%-40s %-10s %-20s\n" "${name}" "${size}" "${date}"
        fi
    done
}

# Verify backup integrity
verify() {
    local input_file="${1:-}"

    if [ -z "${input_file}" ]; then
        log_error "Usage: $0 verify <input_file>"
        exit 1
    fi

    if [ ! -f "${input_file}" ]; then
        log_error "Backup file not found: ${input_file}"
        exit 1
    fi

    log_info "Verifying backup integrity: ${input_file}"

    # Check checksum if available
    local checksum_file="${input_file}.sha256"
    if [ -f "${checksum_file}" ]; then
        log_info "Verifying checksum..."
        if command -v shasum &> /dev/null; then
            if shasum -a 256 -c "${checksum_file}" &> /dev/null; then
                log_info "  Checksum: OK"
            else
                log_error "  Checksum: FAILED"
                exit 1
            fi
        fi
    else
        log_warn "No checksum file found, skipping checksum verification"
    fi

    # Try to list contents
    log_info "Verifying archive integrity..."
    if pg_restore --list "${input_file}" &> /dev/null; then
        log_info "  Archive: OK"
        local count
        count=$(pg_restore --list "${input_file}" 2>/dev/null | wc -l)
        log_info "  Objects: ${count}"
    else
        log_error "  Archive: CORRUPTED"
        exit 1
    fi

    log_info "Backup verification passed!"
}

# Main command dispatcher
case "${1:-help}" in
    backup)
        backup "${2:-}"
        ;;
    restore)
        restore "${2:-}"
        ;;
    list)
        list_backups
        ;;
    verify)
        verify "${2:-}"
        ;;
    help|--help|-h)
        echo "PEPA Database Backup Tool"
        echo ""
        echo "Usage:"
        echo "  $0 backup [output_file]   Create a compressed backup"
        echo "  $0 restore <input_file>   Restore from a backup file"
        echo "  $0 list                   List available backups"
        echo "  $0 verify <input_file>    Verify backup integrity"
        echo ""
        echo "Environment variables:"
        echo "  POSTGRES_HOST     Database host (default: localhost)"
        echo "  POSTGRES_PORT     Database port (default: 5432)"
        echo "  POSTGRES_DB       Database name (default: pepa)"
        echo "  POSTGRES_USER     Database user (default: pepa)"
        echo "  POSTGRES_PASSWORD Database password (default: pepa_dev)"
        echo "  BACKUP_DIR        Backup directory (default: ./backups)"
        ;;
    *)
        log_error "Unknown command: $1"
        echo "Run '$0 help' for usage information."
        exit 1
        ;;
esac

#!/bin/bash
# backup.sh - Backup AdHive data directory

set -e

# Configuration
DATA_DIR="${DATA_DIR:-./data}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/adhive-backup-${DATE}.tar.gz"

# Create backup directory
mkdir -p "${BACKUP_DIR}"

echo "=== AdHive Backup ==="
echo "Data directory: ${DATA_DIR}"
echo "Backup location: ${BACKUP_FILE}"

# Check if data directory exists
if [ ! -d "${DATA_DIR}" ]; then
    echo "ERROR: Data directory not found: ${DATA_DIR}"
    exit 1
fi

# Optional: Stop service for consistent backup
# Uncomment if running as systemd service
# if systemctl is-active --quiet adhive; then
#     echo "Stopping AdHive service..."
#     systemctl stop adhive
#     STOPPED=true
# fi

# Create backup
echo "Creating backup..."
tar -czf "${BACKUP_FILE}" \
    --exclude="*.log" \
    --exclude="*.tmp" \
    --exclude="*.wal" \
    --exclude="*.shm" \
    -C "$(dirname "${DATA_DIR}")" \
    "$(basename "${DATA_DIR}")"

# Restart service if we stopped it
# if [ "$STOPPED" = true ]; then
#     echo "Restarting AdHive service..."
#     systemctl start adhive
# fi

# Clean old backups (keep last 7)
echo "Cleaning old backups..."
find "${BACKUP_DIR}" -name "adhive-backup-*.tar.gz" -mtime +7 -delete 2>/dev/null || true

# Calculate size
BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)

echo ""
echo "=== Backup Complete ==="
echo "File: ${BACKUP_FILE}"
echo "Size: ${BACKUP_SIZE}"
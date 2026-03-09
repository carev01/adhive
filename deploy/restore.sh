#!/bin/bash
# restore.sh - Restore AdHive from a backup

set -e

# Configuration
DATA_DIR="${DATA_DIR:-./data}"
BACKUP_FILE="${1:?Usage: restore.sh <backup-file>}"

echo "=== AdHive Restore ==="
echo "Backup file: ${BACKUP_FILE}"
echo "Data directory: ${DATA_DIR}"

# Check if backup file exists
if [ ! -f "${BACKUP_FILE}" ]; then
    echo "ERROR: Backup file not found: ${BACKUP_FILE}"
    exit 1
fi

# Confirm restore
read -p "This will replace existing data. Continue? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

# Optional: Stop service
# Uncomment if running as systemd service
# if systemctl is-active --quiet adhive; then
#     echo "Stopping AdHive service..."
#     systemctl stop adhive
#     STOPPED=true
# fi

# Backup current data
if [ -d "${DATA_DIR}" ]; then
    echo "Backing up current data..."
    mv "${DATA_DIR}" "${DATA_DIR}.old.$(date +%Y%m%d_%H%M%S)"
fi

# Restore
echo "Restoring from backup..."
tar -xzf "${BACKUP_FILE}" -C /

# Set permissions if running as systemd
# chown -R adhive:adhive "${DATA_DIR}"

# Restart service if we stopped it
# if [ "$STOPPED" = true ]; then
#     echo "Starting AdHive service..."
#     systemctl start adhive
# fi

echo ""
echo "=== Restore Complete ==="
echo "Data restored to: ${DATA_DIR}"
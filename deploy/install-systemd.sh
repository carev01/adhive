#!/bin/bash
# install-systemd.sh - Install AdHive as a systemd service

set -e

echo "=== AdHive Systemd Installation ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root: sudo ./deploy/install-systemd.sh"
    exit 1
fi

# Create user and group
echo "Creating adhive user..."
useradd --system --home-dir /var/lib/adhive --shell /usr/sbin/nologin adhive 2>/dev/null || true

# Create directories
echo "Creating directories..."
mkdir -p /opt/adhive/bin
mkdir -p /var/lib/adhive/data
mkdir -p /var/lib/adhive/logs
mkdir -p /etc/adhive

# Copy binary
echo "Installing binary..."
if [ -f "./bin/ad-catalog" ]; then
    cp bin/ad-catalog /opt/adhive/bin/
else
    echo "ERROR: Binary not found at ./bin/ad-catalog"
    echo "Please run 'make build' first."
    exit 1
fi
chmod +x /opt/adhive/bin/ad-catalog

# Copy systemd unit
echo "Installing systemd service..."
cp deploy/adhive.service /etc/systemd/system/

# Create config file if not exists
if [ ! -f /etc/adhive/config.env ]; then
    echo "Creating configuration file..."
    cat > /etc/adhive/config.env << 'EOF'
# AdHive Configuration
# Edit this file with your settings

# REQUIRED: Session secret (generate with: openssl rand -hex 32)
SESSION_SECRET=change-me-to-a-secure-random-string

# Environment
GO_ENV=production
LOG_LEVEL=info

# Security
HTTPS_ENABLED=false
CORS_ALLOWED_ORIGINS=

# Rate limiting
RATE_LIMIT_GLOBAL=100
RATE_LIMIT_AUTH=5
EOF
    chmod 600 /etc/adhive/config.env
fi

# Set permissions
echo "Setting permissions..."
chown -R adhive:adhive /var/lib/adhive
chmod 750 /var/lib/adhive
chmod 750 /var/lib/adhive/data

# Enable and start
echo "Enabling service..."
systemctl daemon-reload
systemctl enable adhive

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Edit /etc/adhive/config.env with your configuration"
echo "   - Set SESSION_SECRET to a secure random string"
echo "   - Configure CORS_ALLOWED_ORIGINS if needed"
echo ""
echo "2. Start the service:"
echo "   sudo systemctl start adhive"
echo ""
echo "3. Check status:"
echo "   sudo systemctl status adhive"
echo ""
echo "4. View logs:"
echo "   sudo journalctl -u adhive -f"
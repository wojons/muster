#!/bin/bash
# install-launchagent.sh
# Install Muster LaunchAgent for current user

set -e

LAUNCH_AGENT="com.muster.app.plist"
LAUNCH_AGENT_DIR="$HOME/Library/LaunchAgents"
LAUNCH_AGENT_PATH="$LAUNCH_AGENT_DIR/$LAUNCH_AGENT"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing Muster LaunchAgent..."

# Create LaunchAgents directory if needed
mkdir -p "$LAUNCH_AGENT_DIR"

# Create log directory
LOG_DIR="$HOME/Library/Logs/com.muster.app"
mkdir -p "$LOG_DIR"

# Stop existing agent if loaded
if launchctl list | grep -q "com.muster.app"; then
    echo "Stopping existing LaunchAgent..."
    launchctl stop com.muster.app 2>/dev/null || true
    launchctl unload "$LAUNCH_AGENT_PATH" 2>/dev/null || true
fi

# Install the plist
cp "$SCRIPT_DIR/../LaunchAgents/$LAUNCH_AGENT" "$LAUNCH_AGENT_PATH"

# Set proper permissions
chmod 644 "$LAUNCH_AGENT_PATH"

# Load the agent
echo "Loading LaunchAgent..."
launchctl load "$LAUNCH_AGENT_PATH"

# Start the agent
launchctl start com.muster.app

echo "LaunchAgent installed and started successfully!"
echo ""
echo "Logs available at:"
echo "  stdout: $LOG_DIR/stdout.log"
echo "  stderr: $LOG_DIR/stderr.log"
echo ""
echo "To manage the service:"
echo "  launchctl stop com.muster.app    # Stop"
echo "  launchctl start com.muster.app   # Start"
echo "  launchctl unload $LAUNCH_AGENT_PATH  # Unload"

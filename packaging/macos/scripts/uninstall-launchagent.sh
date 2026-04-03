#!/bin/bash
# uninstall-launchagent.sh
# Uninstall Muster LaunchAgent for current user

set -e

LAUNCH_AGENT="com.muster.app.plist"
LAUNCH_AGENT_DIR="$HOME/Library/LaunchAgents"
LAUNCH_AGENT_PATH="$LAUNCH_AGENT_DIR/$LAUNCH_AGENT"

echo "Uninstalling Muster LaunchAgent..."

# Stop and unload agent if running
if launchctl list | grep -q "com.muster.app"; then
    echo "Stopping LaunchAgent..."
    launchctl stop com.muster.app 2>/dev/null || true
    
    if [ -f "$LAUNCH_AGENT_PATH" ]; then
        echo "Unloading LaunchAgent..."
        launchctl unload "$LAUNCH_AGENT_PATH" 2>/dev/null || true
    fi
fi

# Remove plist file
if [ -f "$LAUNCH_AGENT_PATH" ]; then
    echo "Removing plist file..."
    rm -f "$LAUNCH_AGENT_PATH"
fi

echo "Muster LaunchAgent uninstalled successfully!"

#!/usr/bin/env python3
"""
Ralph Loop Monitor - Watch loop progress and send notifications
"""

import os
import sys
import json
import time
import argparse
from pathlib import Path
from datetime import datetime


def tail_log(log_file, n=20):
    """Tail last N lines of log file"""
    try:
        with open(log_file) as f:
            lines = f.readlines()
            return lines[-n:]
    except:
        return []


def parse_state(state_file):
    """Parse current loop state"""
    try:
        with open(state_file) as f:
            return json.load(f)
    except:
        return None


def format_duration(start_time):
    """Format duration since start"""
    try:
        start = datetime.fromisoformat(start_time)
        delta = datetime.now() - start
        hours = delta.seconds // 3600
        minutes = (delta.seconds % 3600) // 60
        return f"{hours}h {minutes}m"
    except:
        return "unknown"


def show_dashboard(project_dir):
    """Show live dashboard"""
    state_file = Path(project_dir) / ".ralph_state.json"
    log_file = Path(project_dir) / ".ralph.log"
    
    # Clear screen
    os.system('clear' if os.name != 'nt' else 'cls')
    
    print("="*70)
    print("🤖 RALPH WIGGUM LOOP - MONITOR")
    print("="*70)
    print()
    
    # Show state
    state = parse_state(state_file)
    if state:
        print(f"📊 STATUS: {state.get('status', 'unknown').upper()}")
        print(f"🔄 Iterations: {state.get('iteration', 0)}")
        print(f"✅ Tasks Completed: {state.get('tasks_completed', 0)}")
        print(f"❌ Tasks Failed: {state.get('tasks_failed', 0)}")
        print(f"⏱️  Running for: {format_duration(state.get('last_run', ''))}")
        if state.get('current_task'):
            print(f"📝 Current Task: {state.get('current_task')}")
    else:
        print("⚠️  No state file found. Loop may not be running.")
    
    print()
    print("-"*70)
    print("📜 RECENT LOG ENTRIES:")
    print("-"*70)
    
    # Show recent logs
    logs = tail_log(log_file, 15)
    for line in logs:
        print(line.rstrip())
    
    print()
    print("="*70)
    print("Press Ctrl+C to exit monitor")
    print("="*70)


def watch_mode(project_dir, interval=5):
    """Watch mode - continuously update dashboard"""
    try:
        while True:
            show_dashboard(project_dir)
            time.sleep(interval)
    except KeyboardInterrupt:
        print("\n👋 Monitor stopped")


def main():
    parser = argparse.ArgumentParser(description='Ralph Loop Monitor')
    parser.add_argument('--project-dir', default='.', help='Project directory')
    parser.add_argument('--watch', '-w', action='store_true', help='Watch mode')
    parser.add_argument('--interval', '-i', type=int, default=5, help='Update interval (seconds)')
    
    args = parser.parse_args()
    
    if args.watch:
        watch_mode(args.project_dir, args.interval)
    else:
        show_dashboard(args.project_dir)


if __name__ == '__main__':
    main()

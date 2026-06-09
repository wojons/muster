#!/bin/bash
# Start the Ralph Wiggum Loop
# Usage: ./start_ralph.sh [options]

cd "$(dirname "$0")"

echo "🤖 Starting Ralph Wiggum Loop..."
echo ""

# Check if Python is available
if ! command -v python3 &> /dev/null; then
    echo "❌ Error: Python 3 is required but not installed"
    exit 1
fi

# Install dependencies if needed
if [ ! -d "venv" ]; then
    echo "📦 Creating virtual environment..."
    python3 -m venv venv
fi

# Check for requirements
if [ -f "requirements.txt" ]; then
    echo "📥 Installing dependencies..."
    source venv/bin/activate
    pip install -q -r requirements.txt
fi

# Default settings
export RALPH_LOG_LEVEL=${RALPH_LOG_LEVEL:-INFO}
export RALPH_DELAY_SECONDS=${RALPH_DELAY_SECONDS:-5}
export RALPH_MAX_ITERATIONS=${RALPH_MAX_ITERATIONS:-1000}

echo ""
echo "⚙️  Configuration:"
echo "   Project directory: $(pwd)"
echo "   Log level: $RALPH_LOG_LEVEL"
echo "   Delay between tasks: ${RALPH_DELAY_SECONDS}s"
echo "   Max iterations: $RALPH_MAX_ITERATIONS"
echo ""

# Parse command line arguments
DRY_RUN=""
TASK_FILTER=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN="--dry-run"
            echo "🔍 Dry run mode (no actual execution)"
            shift
            ;;
        --filter)
            TASK_FILTER="--task-filter $2"
            echo "🔍 Task filter: $2"
            shift 2
            ;;
        --phase)
            TASK_FILTER="--task-filter \"Phase $2\""
            echo "🔍 Phase filter: Phase $2"
            shift 2
            ;;
        --help)
            echo ""
            echo "Usage: ./start_ralph.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --dry-run           Simulate without executing"
            echo "  --filter REGEX      Filter tasks by regex pattern"
            echo "  --phase N           Run only tasks from Phase N"
            echo "  --help              Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  RALPH_LOG_LEVEL      Logging level (DEBUG, INFO, WARNING, ERROR)"
            echo "  RALPH_DELAY_SECONDS  Delay between iterations (default: 5)"
            echo "  RALPH_MAX_ITERATIONS Max iterations (default: 1000)"
            echo ""
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo ""
echo "🚀 Starting loop..."
echo "   Press Ctrl+C to stop"
echo ""

# Run the loop
source venv/bin/activate
exec python3 ralph_loop.py \
    --project-dir . \
    $DRY_RUN \
    $TASK_FILTER \
    --max-iterations $RALPH_MAX_ITERATIONS

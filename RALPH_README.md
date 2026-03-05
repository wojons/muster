# 🤖 Ralph Wiggum Loop

Autonomous AI development loop for implementing the OpenAPI CLI project from Bible-standard specifications.

## Overview

Ralph Wiggum Loop is an autonomous coding agent that:
- Reads tasks from `TODO.md`
- Follows specifications from `specs/` directory
- Generates production-ready Go code
- Runs tests and validates implementation
- Continues until all tasks are complete

**Based on:**
- [frankbria/ralph-claude-code](https://github.com/frankbria/ralph-claude-code)
- [addyosmani.com - Self-Improving Agents](https://addyosmani.com/blog/self-improving-agents/)

## Quick Start

### 1. Start the Loop

```bash
# Run all tasks
./start_ralph.sh

# Dry run (simulate without executing)
./start_ralph.sh --dry-run

# Run only Phase 1 tasks
./start_ralph.sh --phase 1

# Filter tasks by keyword
./start_ralph.sh --filter "config"
```

### 2. Monitor Progress

```bash
# Show current status
python3 ralph_monitor.py

# Watch mode (continuous updates)
python3 ralph_monitor.py --watch

# Custom update interval
python3 ralph_monitor.py --watch --interval 10
```

### 3. Python Direct Usage

```bash
# Check status
python3 ralph_loop.py --status

# Run with options
python3 ralph_loop.py \
    --max-iterations 100 \
    --task-filter "Phase 1" \
    --dry-run
```

## How It Works

### 1. Task Parsing
Ralph reads `TODO.md` and extracts 150+ tasks across 15 phases:
- Phase 1: Project Setup & Core Infrastructure
- Phase 2: WASM Runtime System
- Phase 3: Authentication & Security
- Phase 4: MCP Server
- Phase 5: Starlark DSL
- Phase 6: CLI Framework
- ...etc

### 2. Specification Mapping
Each task maps to a specification file:
- `CONFIG-LOADER.md` → Configuration system
- `WASM-RUNTIME.md` → WASM runtime
- `OAUTH2-IMPLEMENTATION.md` → OAuth2 authentication
- etc.

### 3. Implementation Loop
For each task:
1. Read specification
2. Generate prompt for AI
3. Execute implementation
4. Run tests
5. Mark complete or retry
6. Move to next task

### 4. State Management
Ralph maintains state in `.ralph_state.json`:
- Current iteration
- Tasks completed/failed
- Current task
- Last run timestamp

## Files

| File | Purpose |
|------|---------|
| `ralph_loop.py` | Main loop implementation |
| `start_ralph.sh` | Easy startup script |
| `ralph_monitor.py` | Progress monitoring |
| `TODO.md` | Task list (279 tasks) |
| `PROMPT.md` | AI agent instructions |
| `.ralph_state.json` | Loop state (auto-generated) |
| `.ralph.log` | Execution log (auto-generated) |

## Configuration

### Environment Variables

```bash
# AI Model to use (e.g., gpt-4, claude-3-opus, gpt-4-turbo)
export RALPH_MODEL=gpt-4

# Logging level (DEBUG, INFO, WARNING, ERROR)
export RALPH_LOG_LEVEL=INFO

# Delay between iterations (seconds)
export RALPH_DELAY_SECONDS=5

# Maximum iterations before stopping
export RALPH_MAX_ITERATIONS=1000
```

### Command Line Options

```bash
./ralph [OPTIONS] [COMMAND]

Options:
  --model MODEL            AI model to use (default: from RALPH_MODEL env var)
  --no-commit             Disable auto-commit after each phase
  -h, --help              Show help

Commands:
  loop                    Run the development loop (default)
  status                  Show current status
  logs [COUNT]            Show recent logs (default: 20)
  clean                   Clean old log files
  reset                   Reset loop state

Examples:
  ./ralph loop --model gpt-4          # Use GPT-4
  ./ralph loop --model claude-3-opus  # Use Claude
  ./ralph status                      # Check progress
```

## Workflow

```
┌─────────────────────────────────────────┐
│           Ralph Wiggum Loop             │
├─────────────────────────────────────────┤
│  1. Parse TODO.md → Task Queue          │
│  2. Get Next Task → Execute             │
│  3. Read Spec → Build Prompt            │
│  4. AI Generates Code                   │
│  5. Run Tests → Validate                │
│  6. Mark Complete/Failed                │
│  7. Save State → Repeat                 │
└─────────────────────────────────────────┘
```

## Safety Features

- **Max Iterations**: Prevents infinite loops
- **State Persistence**: Can resume after interruption
- **Error Logging**: Tracks failures for review
- **Dry Run Mode**: Test without executing
- **Task Filtering**: Run specific subsets

## Monitoring

### Live Dashboard
```bash
python3 ralph_monitor.py --watch
```

Shows:
- Current status
- Iteration count
- Tasks completed/failed
- Recent log entries
- Current task

### Log Files
```bash
# View full log
tail -f .ralph.log

# View state
cat .ralph_state.json | jq

# View structured events
cat .ralph_log.jsonl | jq
```

## Expected Runtime

- **Phase 1** (Setup): ~30 minutes
- **Phase 2** (WASM): ~2 hours
- **Phase 3** (Auth): ~3 hours
- **Phase 4** (MCP): ~2 hours
- **Phase 5** (DSL): ~2 hours
- **Phase 6** (CLI): ~3 hours
- **Phase 7-15**: ~5 hours

**Total: ~3-5 days** of continuous operation

## Tips

1. **Start Small**: Run Phase 1 first to verify setup
   ```bash
   ./start_ralph.sh --phase 1
   ```

2. **Use Dry Run**: Test without making changes
   ```bash
   ./start_ralph.sh --dry-run
   ```

3. **Monitor Continuously**: Keep an eye on progress
   ```bash
   python3 ralph_monitor.py --watch
   ```

4. **Resume After Stop**: Ralph saves state automatically
   ```bash
   # Stop with Ctrl+C
   # Restart - continues where left off
   ./start_ralph.sh
   ```

5. **Check Logs**: Review failures
   ```bash
   grep "ERROR" .ralph.log
   ```

## Troubleshooting

### Loop Not Starting
```bash
# Check Python
python3 --version

# Check permissions
ls -la ralph_loop.py start_ralph.sh

# Run directly
python3 ralph_loop.py --status
```

### Tasks Failing
- Check specification files exist in `specs/`
- Review `.ralph.log` for errors
- Ensure Go is installed for testing

### Want to Reset?
```bash
# Remove state files
rm .ralph_state.json .ralph.log .ralph_log.jsonl

# Start fresh
./start_ralph.sh
```

## Success Criteria

Ralph completes when:
- ✅ All 279 TODO items done
- ✅ All tests passing
- ✅ Code coverage >80%
- ✅ Zero linting errors
- ✅ Security audit passed
- ✅ Documentation complete

## Architecture

See `specs/` directory for:
- `IMPLEMENTATION-MAPPING.md` - Task-to-file mapping
- `REFERENCE-IMPLEMENTATION.md` - Code patterns
- `master-specification.md` - System overview

## License

MIT - Same as OpenAPI CLI project

---

**Ready to let Ralph code while you sleep?** 🚀

```bash
# Run with default model
./ralph loop

# Use specific model
./ralph loop --model gpt-4
./ralph loop --model claude-3-opus

# Dry run without committing
./ralph loop --no-commit

# Check status
./ralph status
```

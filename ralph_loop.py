#!/usr/bin/env python3
"""
Meta Ralph Loop for Hivemind
Runs builder and verify loops alternately until all tasks complete.

This Python version provides:
- Better error handling and logging
- Extensible workflow engine
- State machine for complex transitions
- Plugin architecture for custom behaviors
- Metrics and monitoring hooks
"""

import argparse
import logging
import os
import re
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Callable, Tuple
import yaml


# Configuration
MAX_AGENT_TIME = 7200  # 2 hours
MAX_FAILED_ATTEMPTS = 3
SLEEP_BETWEEN_ITERATIONS = 2


# Paths
SCRIPT_DIR = Path(__file__).parent.resolve()
PROJECT_ROOT = SCRIPT_DIR
RALPH_DIR = PROJECT_ROOT / ".ralph"
MEMORY_BANK = PROJECT_ROOT / ".memory-bank"
TODO_FILE = PROJECT_ROOT / ".agents" / "skills" / "ralph-wiggum-loop" / "TODO.md"
LOOP_STATE = RALPH_DIR / "loop-state.yaml"
LOGS_DIR = RALPH_DIR / "logs"


# ANSI Colors
class Colors:
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    CYAN = '\033[0;36m'
    NC = '\033[0m'


@dataclass
class LoopState:
    """Current state of the Ralph loop."""
    iteration: int = 0
    phase: str = "idle"
    last_task: Optional[str] = None
    completed_tasks: List[str] = field(default_factory=list)
    failed_attempts: int = 0
    started_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
    
    def to_dict(self) -> Dict:
        return {
            "iteration": self.iteration,
            "phase": self.phase,
            "last_task": self.last_task,
            "completed_tasks": self.completed_tasks,
            "failed_attempts": self.failed_attempts,
            "started_at": self.started_at
        }
    
    @classmethod
    def from_dict(cls, data: Dict) -> "LoopState":
        return cls(
            iteration=data.get("iteration", 0),
            phase=data.get("phase", "idle"),
            last_task=data.get("last_task"),
            completed_tasks=data.get("completed_tasks", []),
            failed_attempts=data.get("failed_attempts", 0),
            started_at=data.get("started_at", datetime.now(timezone.utc).isoformat())
        )


class RalphLogger:
    """Structured logging for the Ralph loop."""
    
    def __init__(self, name: str = "ralph"):
        self.logger = logging.getLogger(name)
        self.logger.setLevel(logging.DEBUG)
        self.logger.handlers = []
    
    def info(self, msg: str):
        print(f"{Colors.BLUE}[RALPH]{Colors.NC} {msg}")
        self.logger.info(msg)
    
    def success(self, msg: str):
        print(f"{Colors.GREEN}[RALPH]{Colors.NC} {msg}")
        self.logger.info(f"SUCCESS: {msg}")
    
    def warning(self, msg: str):
        print(f"{Colors.YELLOW}[RALPH]{Colors.NC} {msg}")
        self.logger.warning(msg)
    
    def error(self, msg: str):
        print(f"{Colors.RED}[RALPH]{Colors.NC} {msg}")
        self.logger.error(msg)
    
    def phase(self, msg: str):
        print(f"{Colors.CYAN}[RALPH]{Colors.NC} {msg}")
        self.logger.info(f"PHASE: {msg}")


class StateManager:
    """Manages loop state persistence."""
    
    def __init__(self, state_file: Path):
        self.state_file = state_file
        self.state = LoopState()
        self._load()
    
    def _load(self):
        """Load state from file."""
        if self.state_file.exists():
            try:
                with open(self.state_file, 'r') as f:
                    data = yaml.safe_load(f) or {}
                    self.state = LoopState.from_dict(data)
            except Exception as e:
                print(f"Warning: Could not load state: {e}")
                self.state = LoopState()
    
    def save(self):
        """Save state to file."""
        try:
            with open(self.state_file, 'w') as f:
                yaml.dump(self.state.to_dict(), f, default_flow_style=False)
        except Exception as e:
            print(f"Warning: Could not save state: {e}")
    
    def increment_iteration(self) -> int:
        """Increment iteration counter and return new value."""
        self.state.iteration += 1
        self.save()
        return self.state.iteration
    
    def set_phase(self, phase: str):
        """Set current phase."""
        self.state.phase = phase
        self.save()
    
    def record_failure(self) -> int:
        """Record a failure and return new count."""
        self.state.failed_attempts += 1
        self.save()
        return self.state.failed_attempts
    
    def reset_failures(self):
        """Reset failure counter."""
        self.state.failed_attempts = 0
        self.save()
    
    def reset(self):
        """Reset state to defaults."""
        self.state = LoopState()
        self.save()


class TaskManager:
    """Manages task tracking from TODO.md."""
    
    def __init__(self, todo_file: Path):
        self.todo_file = todo_file
    
    def has_pending_tasks(self) -> bool:
        """Check if there are incomplete tasks."""
        if not self.todo_file.exists():
            return False
        content = self.todo_file.read_text()
        return bool(re.search(r'^- \[ \]', content, re.MULTILINE))
    
    def count_pending(self) -> int:
        """Count number of pending tasks."""
        if not self.todo_file.exists():
            return 0
        content = self.todo_file.read_text()
        return len(re.findall(r'^- \[ \]', content, re.MULTILINE))
    
    def get_next_task(self) -> Optional[str]:
        """Get the next pending task description."""
        if not self.todo_file.exists():
            return None
        content = self.todo_file.read_text()
        match = re.search(r'^- \[ \] (.+)$', content, re.MULTILINE)
        return match.group(1) if match else None


class LogManager:
    """Manages log file operations."""
    
    def __init__(self, logs_dir: Path):
        self.logs_dir = logs_dir
        self.logs_dir.mkdir(parents=True, exist_ok=True)
    
    def get_log_file(self, phase: str) -> Path:
        """Generate unique log filename."""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        return self.logs_dir / f"{phase}_{timestamp}.log"
    
    def list_logs(self, count: int = 10) -> List[Path]:
        """List recent log files."""
        if not self.logs_dir.exists():
            return []
        logs = sorted(
            [f for f in self.logs_dir.glob("*.log")],
            key=lambda x: x.stat().st_mtime,
            reverse=True
        )
        return logs[:count]
    
    def clean_old_logs(self, keep: int = 100):
        """Remove old logs, keeping only the most recent."""
        logs = self.list_logs(10000)
        if len(logs) > keep:
            for log in logs[keep:]:
                log.unlink()


class PhaseRunner:
    """Runs individual phases (build, verify, plan)."""
    
    def __init__(
        self,
        logger: RalphLogger,
        log_manager: LogManager,
        max_agent_time: int = MAX_AGENT_TIME,
        model: Optional[str] = None
    ):
        self.logger = logger
        self.log_manager = log_manager
        self.max_agent_time = max_agent_time
        self.model = model
        self.last_output: str = ""
    
    def run_opencode(
        self,
        prompt: str,
        phase_name: str,
        working_dir: Path = PROJECT_ROOT,
        capture_output: bool = True
    ) -> Tuple[bool, str]:
        """Run OpenCode with a prompt.
        
        Returns:
            Tuple of (success, output)
        """
        log_file = self.log_manager.get_log_file(phase_name)
        output_lines: List[str] = []
        
        self.logger.phase("═" * 60)
        self.logger.phase(f"Starting {phase_name} phase...")
        self.logger.phase(f"Log file: {log_file}")
        self.logger.phase("═" * 60)
        
        try:
            # Build command with optional model
            cmd = ["opencode", "run"]
            if self.model:
                cmd.extend(["--model", self.model])
            cmd.append(prompt)
            
            if self.model:
                self.logger.info(f"Using model: {self.model}")
            
            # Run with timeout
            with open(log_file, 'w') as log_fh:
                process = subprocess.Popen(
                    cmd,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    cwd=working_dir,
                    text=True
                )
                
                start_time = time.time()
                while True:
                    if time.time() - start_time > self.max_agent_time:
                        process.kill()
                        self.logger.error(f"{phase_name} phase TIMED OUT")
                        return False, ""
                    
                    if process.stdout:
                        line = process.stdout.readline()
                        if not line and process.poll() is not None:
                            break
                        
                        if line:
                            print(line, end='')
                            log_fh.write(line)
                            log_fh.flush()
                            if capture_output:
                                output_lines.append(line.rstrip())
                    else:
                        if process.poll() is not None:
                            break
                        time.sleep(0.1)
                
                exit_code = process.wait()
                self.last_output = "\n".join(output_lines)
                
                if exit_code == 0:
                    self.logger.success(f"{phase_name} phase completed")
                    return True, self.last_output
                else:
                    self.logger.error(f"{phase_name} phase failed")
                    return False, self.last_output
                    
        except FileNotFoundError:
            self.logger.error("OpenCode not found. Please install opencode.")
            return False, ""
        except Exception as e:
            self.logger.error(f"Error running {phase_name}: {e}")
            return False, ""
    
    def commit_changes(self, message: str, working_dir: Path = PROJECT_ROOT) -> bool:
        """Commit changes with given message."""
        if not message or message.strip() == "":
            self.logger.warning("No commit message provided, skipping commit")
            return False
        
        # Extract meaningful commit message
        commit_msg = self._extract_commit_message(message)
        
        if not commit_msg or commit_msg.strip() == "":
            self.logger.warning("Could not extract commit message, skipping")
            return False
        
        try:
            self.logger.info("Staging all changes...")
            result = subprocess.run(
                ["git", "add", "-A"],
                cwd=working_dir,
                capture_output=True,
                text=True
            )
            
            # Check if there are changes
            result = subprocess.run(
                ["git", "status", "--porcelain"],
                cwd=working_dir,
                capture_output=True,
                text=True
            )
            if not result.stdout.strip():
                self.logger.info("No changes to commit")
                return True
            
            self.logger.info(f"Committing: {commit_msg[:80]}...")
            result = subprocess.run(
                ["git", "commit", "-m", commit_msg],
                cwd=working_dir,
                capture_output=True,
                text=True
            )
            
            if result.returncode == 0:
                self.logger.success(f"Committed: {result.stdout.strip()}")
                return True
            else:
                self.logger.error(f"Commit failed: {result.stderr}")
                return False
                
        except Exception as e:
            self.logger.error(f"Error during commit: {e}")
            return False
    
    def _extract_commit_message(self, output: str) -> str:
        """Extract meaningful commit message from output."""
        lines = output.strip().split('\n')
        
        meaningful_lines = []
        for line in lines:
            line = line.strip()
            if not line:
                continue
            if any(prefix in line.lower() for prefix in [
                'info:', 'debug:', 'warning:', 'error:', 'trace:',
                'running', 'starting', 'completed', 'finished'
            ]):
                continue
            if len(line) < 3:
                continue
            meaningful_lines.append(line)
        
        if not meaningful_lines:
            return lines[-1] if lines else output
        
        last_line = meaningful_lines[-1]
        
        if len(last_line) < 50 and len(meaningful_lines) > 1:
            context_lines = []
            for i in range(max(0, len(meaningful_lines) - 4), len(meaningful_lines)):
                context_lines.append(meaningful_lines[i])
            last_line = " | ".join(context_lines)
        
        if len(last_line) > 500:
            last_line = last_line[:497] + "..."
        
        return last_line
    
    def get_last_output(self) -> str:
        """Get the last captured output."""
        return self.last_output


class WorkflowEngine:
    """Extensible workflow engine for Ralph loops."""
    
    def __init__(
        self,
        logger: RalphLogger,
        state_manager: StateManager,
        task_manager: TaskManager,
        log_manager: LogManager,
        phase_runner: PhaseRunner
    ):
        self.logger = logger
        self.state = state_manager
        self.tasks = task_manager
        self.logs = log_manager
        self.runner = phase_runner
        
        self.pre_phase_hooks: Dict[str, List[Callable]] = {}
        self.post_phase_hooks: Dict[str, List[Callable]] = {}
        self.on_failure_hooks: List[Callable] = []
        self.on_success_hooks: List[Callable] = []
    
    def run_phase(self, phase_name: str, prompt_file: Path, auto_commit: bool = False) -> bool:
        """Run a phase."""
        self.state.set_phase(phase_name)
        
        # Read prompt
        if not prompt_file.exists():
            self.logger.error(f"Prompt file not found: {prompt_file}")
            return False
        
        prompt = prompt_file.read_text()
        success, output = self.runner.run_opencode(prompt, phase_name)
        
        if auto_commit and success and output:
            self.logger.info("Auto-committing changes...")
            self.runner.commit_changes(output)
        
        return success
    
    def run_standard_loop(self, auto_commit: bool = False):
        """Run the standard build-verify loop."""
        prompt_dir = PROJECT_ROOT / ".agents" / "skills" / "ralph-wiggum-loop"
        
        while self.tasks.has_pending_tasks():
            iteration = self.state.increment_iteration()
            pending = self.tasks.count_pending()
            
            self.logger.info("═" * 60)
            self.logger.info(f"Iteration #{iteration} | Pending tasks: {pending}")
            self.logger.info("═" * 60)
            
            # Build phase
            success = self.run_phase(
                "build",
                prompt_dir / "PROMPT.md",
                auto_commit=auto_commit
            )
            
            if not success:
                failures = self.state.record_failure()
                
                if failures >= MAX_FAILED_ATTEMPTS:
                    self.logger.error(f"Too many failures ({failures}). Stopping.")
                    return False
                
                self.logger.warning("Build failed, retrying...")
                continue
            
            # Verify phase (if you have a verify prompt)
            verify_prompt = prompt_dir / "PROMPT-VERIFY.md"
            if verify_prompt.exists():
                success = self.run_phase("verify", verify_prompt, auto_commit=auto_commit)
                
                if not success:
                    self.logger.warning("Verification failed, fixes needed")
                    self.state.reset_failures()
                    continue
            
            self.state.reset_failures()
            time.sleep(SLEEP_BETWEEN_ITERATIONS)
        
        self.logger.success("All tasks complete!")
        return True


class RalphCLI:
    """Command-line interface for the Ralph loop."""
    
    def __init__(self, model: Optional[str] = None):
        self.logger = RalphLogger()
        self.state = StateManager(LOOP_STATE)
        self.tasks = TaskManager(TODO_FILE)
        self.logs = LogManager(LOGS_DIR)
        self.model = model
        self.runner = PhaseRunner(self.logger, self.logs, model=model)
        self.engine = WorkflowEngine(
            self.logger, self.state, self.tasks, self.logs, self.runner
        )
        self.auto_commit = True
    
    def cmd_loop(self):
        """Run full loop."""
        self.logger.info("Starting Ralph loop...")
        self.engine.run_standard_loop(auto_commit=self.auto_commit)
    
    def cmd_status(self):
        """Show current status."""
        self.logger.info("Loop status:")
        print(f"  Iteration: {self.state.state.iteration}")
        print(f"  Phase: {self.state.state.phase}")
        print(f"  Pending tasks: {self.tasks.count_pending()}")
        print(f"  Failed attempts: {self.state.state.failed_attempts}")
        print(f"  Started at: {self.state.state.started_at}")
        print("")
        self.cmd_logs(5)
    
    def cmd_logs(self, count: int = 20):
        """Show recent logs."""
        self.logger.info(f"Recent log files (last {count}):")
        logs = self.logs.list_logs(count)
        if logs:
            for log in logs:
                mtime = datetime.fromtimestamp(log.stat().st_mtime)
                print(f"  {log.name} ({mtime.strftime('%Y-%m-%d %H:%M:%S')})")
        else:
            print("  No logs found")
    
    def cmd_clean(self):
        """Clean old logs."""
        self.logger.info("Cleaning old logs (keeping last 100)...")
        self.logs.clean_old_logs(100)
        self.logger.success("Log cleanup complete")
    
    def cmd_reset(self):
        """Reset loop state."""
        self.state.reset()
        self.logger.success("Loop state reset")
    
    def run(self, args: Optional[List[str]] = None):
        """Main entry point."""
        parser = argparse.ArgumentParser(
            description="Meta Ralph Loop for OpenAPI CLI",
            formatter_class=argparse.RawDescriptionHelpFormatter,
            epilog="""
Examples:
  python ralph_loop.py loop                           # Run full loop
  python ralph_loop.py loop --no-commit               # Run without auto-commit
  python ralph_loop.py loop --model gpt-4             # Use GPT-4 model
  python ralph_loop.py status                         # Show status
  python ralph_loop.py logs 10                        # Show last 10 logs
  python ralph_loop.py clean                          # Clean old logs

Environment Variables:
  RALPH_MODEL              # Default AI model to use
  RALPH_LOG_LEVEL          # Logging level (DEBUG, INFO, WARNING, ERROR)
  RALPH_DELAY_SECONDS      # Delay between iterations

Configuration:
  Max agent time: 2 hours
  Max failed attempts: 3
  Logs directory: .ralph/logs/
            """
        )
        
        parser.add_argument(
            "command",
            choices=["loop", "status", "logs", "clean", "reset"],
            nargs="?",
            default="loop",
            help="Command to run (default: loop)"
        )
        parser.add_argument(
            "count",
            type=int,
            nargs="?",
            default=20,
            help="Number of logs to show (for 'logs' command)"
        )
        parser.add_argument(
            "--no-commit",
            action="store_true",
            help="Disable auto-commit after each phase"
        )
        parser.add_argument(
            "--model",
            type=str,
            default=os.getenv("RALPH_MODEL"),
            help="AI model to use (e.g., gpt-4, claude-3-opus). Can also set RALPH_MODEL env var"
        )
        
        parsed = parser.parse_args(args)
        
        self.auto_commit = not parsed.no_commit
        
        # Update model if provided via CLI
        if parsed.model:
            self.model = parsed.model
            self.runner.model = parsed.model
        
        # Ensure directories exist
        RALPH_DIR.mkdir(parents=True, exist_ok=True)
        LOGS_DIR.mkdir(parents=True, exist_ok=True)
        
        commands = {
            "loop": self.cmd_loop,
            "status": self.cmd_status,
            "logs": lambda: self.cmd_logs(parsed.count),
            "clean": self.cmd_clean,
            "reset": self.cmd_reset,
        }
        
        command_func = commands.get(parsed.command)
        if command_func:
            try:
                command_func()
            except KeyboardInterrupt:
                print("\n")
                self.logger.warning("Interrupted by user")
                sys.exit(130)
            except Exception as e:
                self.logger.error(f"Command failed: {e}")
                sys.exit(1)
        else:
            parser.print_help()
            sys.exit(1)


def main():
    """Entry point."""
    cli = RalphCLI()
    cli.run()


if __name__ == "__main__":
    main()

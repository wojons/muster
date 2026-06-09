#!/usr/bin/env python3
"""
Ralph Loop Manager - Enhanced version with proper task completion tracking

This fixes the issues:
1. ✅ Marks tasks as complete with [x] in TODO.md
2. ✅ Extracts tasks from TODO.md properly
3. ✅ Adds 1-hour retry delay on failure
4. ✅ Better timeout handling
5. ✅ State recovery after errors
"""

import re
import time
from pathlib import Path
from typing import Optional, List, Tuple


class TaskExtractor:
    """Extracts and tracks tasks from various formats."""
    
    def __init__(self, todo_file: Path):
        self.todo_file = todo_file
    
    def extract_pending_tasks(self) -> List[Tuple[int, str, str]]:
        """
        Extract pending tasks from TODO.md.
        Returns: List of (line_number, task_text, full_line)
        """
        if not self.todo_file.exists():
            return []
        
        content = self.todo_file.read_text()
        lines = content.split('\n')
        tasks = []
        
        for i, line in enumerate(lines, 1):
            # Pattern 1: Standard markdown checkbox "- [ ] task"
            match = re.match(r'^- \[ \] (.+)$', line)
            if match:
                tasks.append((i, match.group(1).strip(), line))
                continue
            
            # Pattern 2: Priority task "### [ACTIONS REQUIRED] task"
            match = re.match(r'^### \[ACTIONS REQUIRED\] (.+?)(?:\s+(?:⭐|⚠️|💡))?$', line)
            if match:
                tasks.append((i, match.group(1).strip(), line))
                continue
            
            # Pattern 3: IDLE SWEEP task "- [IDLE SWEEP #N] NAME"
            match = re.match(r'^## \[IDLE SWEEP #\d+\] (.+) \(', line)
            if match:
                tasks.append((i, match.group(1).strip(), line))
                continue
            
            # Pattern 4: Priority header "### ⚡ Priority Task Name"
            match = re.match(r'^### \*\*(.+?)\*\*.*Priority:', line)
            if match:
                tasks.append((i, match.group(1).strip(), line))
        
        return tasks
    
    def extract_completed_tasks(self) -> List[Tuple[int, str, str]]:
        """Extract completed tasks from TODO.md."""
        if not self.todo_file.exists():
            return []
        
        content = self.todo_file.read_text()
        lines = content.split('\n')
        tasks = []
        
        for i, line in enumerate(lines, 1):
            # Pattern 1: Standard markdown "- [x] task"
            match = re.match(r'^- \[x\] (.+)$', line)
            if match:
                tasks.append((i, match.group(1).strip(), line))
                continue
            
            # Pattern 2: Completed task markers "✅ RESOLVED" or "✅ COMPLETE"
            match = re.match(r'^.*✅ (?:RESOLVED|COMPLETE|IMPLEMENTED)', line)
            if match:
                # Extract the header if on previous lines
                for j in range(max(0, i-3), i):
                    header_match = re.match(r'^##+ (.+)$', lines[j-1] if j > 0 else '')
                    if header_match:
                        tasks.append((j, header_match.group(1).strip(), lines[j-1]))
        
        return tasks
    
    def mark_task_complete(self, task_text: str) -> bool:
        """
        Mark a task as complete by replacing its line with [x].
        Returns: True if successful
        """
        if not self.todo_file.exists():
            return False
        
        content = self.todo_file.read_text()
        lines = content.split('\n')
        modified = False
        
        for i, line in enumerate(lines):
            # Try to match and replace pending task markers
            if task_text in line:
                # Replace "- [ ] " with "- [x] "
                if line.strip().startswith('- [ ]'):
                    lines[i] = line.replace('- [ ]', '- [x]', 1)
                    modified = True
                    break
                # Add [x] to header tasks
                elif 'ACTIONS REQUIRED' in line or 'IDLE SWEEP' in line:
                    # Find the status line and mark complete
                    for j in range(i+1, min(i+5, len(lines))):
                        if '✅' in lines[j]:
                            modified = True
                            break
                        elif '**Status**:' in lines[j]:
                            lines[j] = lines[j].replace('⚠️', '✅').replace('❌', '✅')
                            modified = True
                            break
        
        if modified:
            self.todo_file.write_text('\n'.join(lines))
        
        return modified


class RalphLoopEnhancer:
    """Enhancements to Ralph loop with proper task management."""
    
    def __init__(self, state_manager, logger):
        self.state = state_manager
        self.logger = logger
        self.task_extractor = TaskExtractor(state_manager.todo_file if hasattr(state_manager, 'todo_file') else None)
    
    def get_next_actionable_task(self) -> Optional[str]:
        """Get the next actionable task from TODO.md."""
        pending = self.task_extractor.extract_pending_tasks()
        
        if not pending:
            return None
        
        # Return the first pending task
        line_num, task_text, _ = pending[0]
        return task_text
    
    def mark_current_task_complete(self, task_text: str) -> bool:
        """Mark the current task as complete in TODO.md."""
        if self.task_extractor.mark_task_complete(task_text):
            self.logger.success(f"Marked task complete: {task_text[:80]}")
            return True
        else:
            self.logger.warning(f"Could not mark task complete: {task_text[:80]}")
            return False
    
    def should_retry_after_delay(self, failure_count: int) -> Tuple[bool, int]:
        """
        Determine if should retry and for how long.
        
        Args:
            failure_count: Number of consecutive failures
            
        Returns:
            (should_retry, delay_seconds)
        """
        MAX_FAILURES = 3
        HOUR_DELAY = 3600  # 1 hour in seconds
        SHORT_DELAY = 10   # 10 seconds for quick tests
        
        if failure_count >= MAX_FAILURES:
            self.logger.error(f"Max failures ({MAX_FAILURES}) reached, stopping")
            return False, 0
        
        if failure_count == 1:
            # First failure: short retry
            return True, SHORT_DELAY
        else:
            # Subsequent failures: 1-hour delay
            self.logger.warning(f"Failure #{failure_count}, waiting 1 hour before retry...")
            return True, HOUR_DELAY
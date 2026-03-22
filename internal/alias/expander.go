package alias

import (
	"context"
	"fmt"
)

// Expander handles alias expansion with circular reference detection and depth limiting.
// It maintains an expansion history to prevent infinite loops when aliases reference
// each other recursively.
type Expander struct {
	manager *Manager
	history []string // Track expanded aliases to detect circular references
}

// MaxExpansionDepth is the maximum allowed depth for alias expansion.
// This prevents stack overflow from deeply nested alias chains.
const MaxExpansionDepth = 10

// NewExpander creates a new alias expander with the given manager.
// The expander maintains state for recursive expansion detection.
func NewExpander(manager *Manager) *Expander {
	return &Expander{
		manager: manager,
		history: make([]string, 0),
	}
}

// ExpandCommand expands aliases in command arguments with circular detection.
// It recursively expands aliases until no more expansions are possible or
// a circular reference is detected.
//
// Parameters:
//   - ctx: Context for cancellation support
//   - args: Command arguments where args[0] is the command/alias name
//
// Returns:
//   - Expanded arguments
//   - Error if expansion depth is exceeded
func (e *Expander) ExpandCommand(ctx context.Context, args []string) ([]string, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if len(args) == 0 {
		return args, nil
	}

	aliasName := args[0]

	// Check for circular reference
	if e.isInHistory(aliasName) {
		// Circular reference detected - return args without expanding
		return args, nil
	}

	// Attempt to expand using the manager
	expanded, wasExpanded := e.manager.Expand(args)
	if !wasExpanded {
		return args, nil
	}

	// Track this expansion in history
	e.history = append(e.history, aliasName)
	defer e.popHistory()

	// Check depth limit
	if len(e.history) > MaxExpansionDepth {
		return nil, fmt.Errorf("alias expansion depth exceeded (max %d)", MaxExpansionDepth)
	}

	// Recursively expand the result
	return e.ExpandCommand(ctx, expanded)
}

// isInHistory checks if an alias name has already been expanded in the current chain.
// This prevents infinite loops from circular alias references.
func (e *Expander) isInHistory(name string) bool {
	for _, h := range e.history {
		if h == name {
			return true
		}
	}
	return false
}

// popHistory removes the most recently expanded alias from history.
// This is called via defer to maintain proper history stack during recursion.
func (e *Expander) popHistory() {
	if len(e.history) > 0 {
		e.history = e.history[:len(e.history)-1]
	}
}

// Expand performs full recursive alias expansion with circular detection.
// This is a convenience method that uses a background context.
// For context-aware expansion (with cancellation support), use ExpandCommand.
//
// Parameters:
//   - args: Command arguments where args[0] is the command/alias name
//
// Returns:
//   - Fully expanded arguments
//   - Error if expansion depth is exceeded
func (e *Expander) Expand(args []string) ([]string, error) {
	return e.ExpandCommand(context.Background(), args)
}

// ExpandOnce performs a single level of alias expansion without recursion.
// This is useful when only one level of expansion is desired.
//
// Parameters:
//   - args: Command arguments where args[0] is the command/alias name
//
// Returns:
//   - Expanded arguments (single level only)
//   - true if expansion occurred, false otherwise
func (e *Expander) ExpandOnce(args []string) ([]string, bool) {
	if len(args) == 0 {
		return args, false
	}

	return e.manager.Expand(args)
}

// History returns a copy of the current expansion history.
// This is primarily useful for debugging and testing.
func (e *Expander) History() []string {
	result := make([]string, len(e.history))
	copy(result, e.history)
	return result
}

// Depth returns the current expansion depth.
func (e *Expander) Depth() int {
	return len(e.history)
}

// Reset clears the expansion history.
// This can be used to start fresh with the same expander instance.
func (e *Expander) Reset() {
	e.history = make([]string, 0)
}

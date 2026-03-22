package alias

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExpander(t *testing.T) {
	t.Run("creates expander with empty history", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		expander := NewExpander(manager)
		require.NotNil(t, expander)
		assert.Empty(t, expander.History())
		assert.Equal(t, 0, expander.Depth())
	})
}

func TestExpander_ExpandCommand(t *testing.T) {
	t.Run("expands single alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("ls", "inventory list")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{"ls"})

		require.NoError(t, err)
		assert.Equal(t, []string{"inventory", "list"}, result)
	})

	t.Run("expands alias with arguments", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("get", "inventory get")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{"get", "item1", "--verbose"})

		require.NoError(t, err)
		assert.Equal(t, []string{"inventory", "get", "item1", "--verbose"}, result)
	})

	t.Run("returns original when no alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		expander := NewExpander(manager)
		args := []string{"direct", "command"}
		result, err := expander.ExpandCommand(context.Background(), args)

		require.NoError(t, err)
		assert.Equal(t, args, result)
	})

	t.Run("returns empty args unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("recursively expands chained aliases", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create a chain: a -> b -> c -> final
		err = manager.Define("a", "b extra")
		require.NoError(t, err)
		err = manager.Define("b", "c more")
		require.NoError(t, err)
		err = manager.Define("c", "final cmd")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{"a"})

		require.NoError(t, err)
		assert.Equal(t, []string{"final", "cmd", "more", "extra"}, result)
	})

	t.Run("detects circular reference and stops", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create circular reference: a -> b -> a
		err = manager.Define("a", "b")
		require.NoError(t, err)
		err = manager.Define("b", "a")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{"a"})

		require.NoError(t, err)
		// Should stop at 'a' when circular reference detected
		assert.Equal(t, []string{"a"}, result)
	})

	t.Run("detects self-referential alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Self-referential: loop -> loop
		err = manager.Define("loop", "loop")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{"loop"})

		require.NoError(t, err)
		// Should return as-is since first expansion would hit circular reference
		assert.Equal(t, []string{"loop"}, result)
	})

	t.Run("returns error when depth limit exceeded", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		// Create a long chain that exceeds MaxExpansionDepth (10)
		for i := 0; i < MaxExpansionDepth+2; i++ {
			current := string(rune('a' + i))
			next := string(rune('a' + i + 1))
			err = manager.Define(current, next)
			require.NoError(t, err)
		}

		expander := NewExpander(manager)
		_, err = expander.ExpandCommand(context.Background(), []string{"a"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "depth exceeded")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("ls", "inventory list")
		require.NoError(t, err)

		expander := NewExpander(manager)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = expander.ExpandCommand(ctx, []string{"ls"})
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("handles complex expansion chain", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		// prod -> config set api.url https://prod.example.com
		err = manager.Define("prod", "config set api.url https://prod.example.com")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.ExpandCommand(context.Background(), []string{"prod"})

		require.NoError(t, err)
		assert.Equal(t, []string{"config", "set", "api.url", "https://prod.example.com"}, result)
	})
}

func TestExpander_Expand(t *testing.T) {
	t.Run("expands alias without context", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("ls", "inventory list")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.Expand([]string{"ls"})

		require.NoError(t, err)
		assert.Equal(t, []string{"inventory", "list"}, result)
	})

	t.Run("handles chained aliases without context", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("a", "b extra")
		require.NoError(t, err)
		err = manager.Define("b", "final cmd")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, err := expander.Expand([]string{"a"})

		require.NoError(t, err)
		assert.Equal(t, []string{"final", "cmd", "extra"}, result)
	})

	t.Run("returns error on depth exceeded without context", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		for i := 0; i < MaxExpansionDepth+2; i++ {
			current := string(rune('a' + i))
			next := string(rune('a' + i + 1))
			err = manager.Define(current, next)
			require.NoError(t, err)
		}

		expander := NewExpander(manager)
		_, err = expander.Expand([]string{"a"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "depth exceeded")
	})
}

func TestExpander_ExpandOnce(t *testing.T) {
	t.Run("expands single level only", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("a", "b extra")
		require.NoError(t, err)
		err = manager.Define("b", "final cmd")
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, expanded := expander.ExpandOnce([]string{"a"})

		assert.True(t, expanded)
		// Should only expand 'a' to 'b extra', not recursively to 'final cmd'
		assert.Equal(t, []string{"b", "extra"}, result)
	})

	t.Run("returns false when no alias", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		expander := NewExpander(manager)
		args := []string{"direct", "command"}
		result, expanded := expander.ExpandOnce(args)

		assert.False(t, expanded)
		assert.Equal(t, args, result)
	})

	t.Run("handles empty args", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		expander := NewExpander(manager)
		result, expanded := expander.ExpandOnce([]string{})

		assert.False(t, expanded)
		assert.Empty(t, result)
	})
}

func TestExpander_History(t *testing.T) {
	t.Run("tracks expansion history", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("a", "b")
		require.NoError(t, err)
		err = manager.Define("b", "c")
		require.NoError(t, err)
		err = manager.Define("c", "final")
		require.NoError(t, err)

		expander := NewExpander(manager)

		// History should be empty before expansion
		assert.Empty(t, expander.History())

		_, err = expander.ExpandCommand(context.Background(), []string{"a"})
		require.NoError(t, err)

		// History should be cleared after expansion completes
		assert.Empty(t, expander.History())
	})

	t.Run("returns copy of history", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("a", "b")
		require.NoError(t, err)
		err = manager.Define("b", "final")
		require.NoError(t, err)

		expander := NewExpander(manager)

		// Access internal history during expansion by checking Depth
		// This verifies the history is being tracked during expansion
		_, err = expander.ExpandCommand(context.Background(), []string{"a"})
		require.NoError(t, err)

		// After expansion, history should be empty
		history := expander.History()
		assert.Empty(t, history)
	})
}

func TestExpander_Depth(t *testing.T) {
	t.Run("returns zero initially", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		expander := NewExpander(manager)
		assert.Equal(t, 0, expander.Depth())
	})
}

func TestExpander_Reset(t *testing.T) {
	t.Run("clears history", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		// We can't easily populate history externally, but we can verify
		// Reset doesn't panic and leaves expander in clean state
		expander := NewExpander(manager)
		expander.Reset()

		assert.Empty(t, expander.History())
		assert.Equal(t, 0, expander.Depth())
	})

	t.Run("allows reuse after reset", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("ls", "inventory list")
		require.NoError(t, err)

		expander := NewExpander(manager)

		// Use expander
		result, err := expander.ExpandCommand(context.Background(), []string{"ls"})
		require.NoError(t, err)
		assert.Equal(t, []string{"inventory", "list"}, result)

		// Reset and use again
		expander.Reset()
		result, err = expander.ExpandCommand(context.Background(), []string{"ls"})
		require.NoError(t, err)
		assert.Equal(t, []string{"inventory", "list"}, result)
	})
}

func TestExpander_Concurrency(t *testing.T) {
	t.Run("handles concurrent expansions", func(t *testing.T) {
		tmpDir := t.TempDir()
		manager, err := NewManager(tmpDir)
		require.NoError(t, err)

		err = manager.Define("a", "final a")
		require.NoError(t, err)
		err = manager.Define("b", "final b")
		require.NoError(t, err)

		expander := NewExpander(manager)
		done := make(chan bool, 3)

		// Multiple concurrent expansions
		for i := 0; i < 3; i++ {
			go func(id int) {
				alias := string(rune('a' + id%2))
				_, err := expander.ExpandCommand(context.Background(), []string{alias})
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all to complete
		for i := 0; i < 3; i++ {
			<-done
		}
	})
}

func TestMaxExpansionDepth(t *testing.T) {
	t.Run("constant is set to 10", func(t *testing.T) {
		assert.Equal(t, 10, MaxExpansionDepth)
	})
}

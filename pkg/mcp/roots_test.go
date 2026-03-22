package mcp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootManager(t *testing.T) {
	tests := []struct {
		name        string
		listChanged bool
		wantLen     int
		wantCap     bool
	}{
		{
			name:        "with listChanged support",
			listChanged: true,
			wantLen:     0,
			wantCap:     true,
		},
		{
			name:        "without listChanged support",
			listChanged: false,
			wantLen:     0,
			wantCap:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewRootManager(tt.listChanged)
			require.NotNil(t, rm)
			assert.Equal(t, tt.wantLen, rm.Count())
			assert.Equal(t, tt.wantCap, rm.SupportsListChanged())
		})
	}
}

func TestRootManager_SetRoots(t *testing.T) {
	rm := NewRootManager(true)

	tests := []struct {
		name      string
		roots     []Root
		wantCount int
		wantRoots []Root
	}{
		{
			name:      "set empty roots",
			roots:     []Root{},
			wantCount: 0,
			wantRoots: []Root{},
		},
		{
			name: "set single root",
			roots: []Root{
				{URI: "file:///home/user/project", Name: "project"},
			},
			wantCount: 1,
			wantRoots: []Root{
				{URI: "file:///home/user/project", Name: "project"},
			},
		},
		{
			name: "set multiple roots",
			roots: []Root{
				{URI: "file:///home/user/project", Name: "project"},
				{URI: "file:///home/user/docs", Name: "docs"},
				{URI: "https://example.com/repo", Name: "remote"},
			},
			wantCount: 3,
			wantRoots: []Root{
				{URI: "file:///home/user/project", Name: "project"},
				{URI: "file:///home/user/docs", Name: "docs"},
				{URI: "https://example.com/repo", Name: "remote"},
			},
		},
		{
			name: "replace existing roots",
			roots: []Root{
				{URI: "file:///new/path", Name: "new"},
			},
			wantCount: 1,
			wantRoots: []Root{
				{URI: "file:///new/path", Name: "new"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm.SetRoots(tt.roots)
			assert.Equal(t, tt.wantCount, rm.Count())
			assert.Equal(t, tt.wantRoots, rm.GetRoots())
		})
	}
}

func TestRootManager_GetRoots(t *testing.T) {
	rm := NewRootManager(false)

	// Set initial roots
	original := []Root{
		{URI: "file:///home/user/project", Name: "project"},
		{URI: "file:///home/user/docs", Name: "docs"},
	}
	rm.SetRoots(original)

	// Get roots
	got := rm.GetRoots()

	// Verify we got the expected roots
	assert.Equal(t, original, got)

	// Verify defensive copy - modifying returned slice should not affect internal state
	got[0].Name = "modified"
	_ = append(got, Root{URI: "file:///extra", Name: "extra"})

	// Original should be unchanged
	roots := rm.GetRoots()
	assert.Equal(t, "project", roots[0].Name)
	assert.Equal(t, 2, len(roots))
}

func TestRootManager_AddRoot(t *testing.T) {
	rm := NewRootManager(false)

	// Add first root
	rm.AddRoot(Root{URI: "file:///home/user/project", Name: "project"})
	assert.Equal(t, 1, rm.Count())

	// Add second root
	rm.AddRoot(Root{URI: "file:///home/user/docs", Name: "docs"})
	assert.Equal(t, 2, rm.Count())

	// Add root without name
	rm.AddRoot(Root{URI: "file:///home/user/unnamed"})
	assert.Equal(t, 3, rm.Count())

	// Verify all roots
	roots := rm.GetRoots()
	require.Len(t, roots, 3)
	assert.Equal(t, "project", roots[0].Name)
	assert.Equal(t, "docs", roots[1].Name)
	assert.Empty(t, roots[2].Name)
}

func TestRootManager_RemoveRoot(t *testing.T) {
	rm := NewRootManager(false)

	// Set up initial roots
	rm.SetRoots([]Root{
		{URI: "file:///home/user/project", Name: "project"},
		{URI: "file:///home/user/docs", Name: "docs"},
		{URI: "file:///home/user/other", Name: "other"},
	})
	assert.Equal(t, 3, rm.Count())

	// Remove existing root
	rm.RemoveRoot("file:///home/user/docs")
	assert.Equal(t, 2, rm.Count())

	roots := rm.GetRoots()
	require.Len(t, roots, 2)
	assert.Equal(t, "project", roots[0].Name)
	assert.Equal(t, "other", roots[1].Name)

	// Try to remove non-existent root (should be no-op)
	rm.RemoveRoot("file:///does/not/exist")
	assert.Equal(t, 2, rm.Count())

	// Remove all roots one by one
	rm.RemoveRoot("file:///home/user/project")
	rm.RemoveRoot("file:///home/user/other")
	assert.Equal(t, 0, rm.Count())
}

func TestRootManager_OnChange(t *testing.T) {
	rm := NewRootManager(true)

	// Channel to receive notifications
	ch := make(chan []Root, 10)

	// Register handler
	rm.OnChange(func(roots []Root) {
		ch <- roots
	})

	// Test SetRoots triggers notification
	rm.SetRoots([]Root{
		{URI: "file:///home/user/project", Name: "project"},
	})

	select {
	case roots := <-ch:
		assert.Len(t, roots, 1)
		assert.Equal(t, "project", roots[0].Name)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for SetRoots notification")
	}

	// Test AddRoot triggers notification
	rm.AddRoot(Root{URI: "file:///home/user/docs", Name: "docs"})

	select {
	case roots := <-ch:
		assert.Len(t, roots, 2)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for AddRoot notification")
	}

	// Test RemoveRoot triggers notification
	rm.RemoveRoot("file:///home/user/project")

	select {
	case roots := <-ch:
		assert.Len(t, roots, 1)
		assert.Equal(t, "docs", roots[0].Name)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for RemoveRoot notification")
	}

	// Test multiple handlers
	ch2 := make(chan []Root, 10)
	rm.OnChange(func(roots []Root) {
		ch2 <- roots
	})

	rm.Clear()

	// Both handlers should be called
	select {
	case <-ch:
		// Good
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for first handler")
	}

	select {
	case <-ch2:
		// Good
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for second handler")
	}
}

func TestRootManager_IsWithinRoots(t *testing.T) {
	rm := NewRootManager(false)

	rm.SetRoots([]Root{
		{URI: "file:///home/user/project", Name: "project"},
		{URI: "file:///home/user/docs", Name: "docs"},
		{URI: "https://example.com", Name: "web"},
	})

	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{
			name: "within first root",
			uri:  "file:///home/user/project/src/main.go",
			want: true,
		},
		{
			name: "within second root",
			uri:  "file:///home/user/docs/readme.md",
			want: true,
		},
		{
			name: "within web root",
			uri:  "https://example.com/api/v1/users",
			want: true,
		},
		{
			name: "exact root match",
			uri:  "file:///home/user/project",
			want: true,
		},
		{
			name: "outside all roots",
			uri:  "file:///home/user/other/file.txt",
			want: false,
		},
		{
			name: "similar but different path",
			uri:  "file:///home/user/project2/file.txt",
			want: false,
		},
		{
			name: "empty uri",
			uri:  "",
			want: false,
		},
		{
			name: "partial match only",
			uri:  "file:///home/user/proj",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rm.IsWithinRoots(tt.uri)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRootManager_IsWithinRoots_EmptyRoots(t *testing.T) {
	rm := NewRootManager(false)
	// No roots set

	assert.False(t, rm.IsWithinRoots("file:///any/path"))
}

func TestRootManager_Clear(t *testing.T) {
	rm := NewRootManager(false)

	// Add some roots
	rm.SetRoots([]Root{
		{URI: "file:///home/user/project", Name: "project"},
		{URI: "file:///home/user/docs", Name: "docs"},
	})
	assert.Equal(t, 2, rm.Count())

	// Clear all
	rm.Clear()
	assert.Equal(t, 0, rm.Count())
	assert.Empty(t, rm.GetRoots())
}

func TestRootManager_GetRootByURI(t *testing.T) {
	rm := NewRootManager(false)

	rm.SetRoots([]Root{
		{URI: "file:///home/user/project", Name: "project"},
		{URI: "file:///home/user/docs", Name: "docs"},
	})

	// Get existing root
	root, found := rm.GetRootByURI("file:///home/user/project")
	assert.True(t, found)
	assert.Equal(t, "project", root.Name)

	// Get non-existent root
	root, found = rm.GetRootByURI("file:///does/not/exist")
	assert.False(t, found)
	assert.Empty(t, root.URI)
}

func TestRootManager_Count(t *testing.T) {
	rm := NewRootManager(false)
	assert.Equal(t, 0, rm.Count())

	rm.AddRoot(Root{URI: "file:///a", Name: "a"})
	assert.Equal(t, 1, rm.Count())

	rm.AddRoot(Root{URI: "file:///b", Name: "b"})
	assert.Equal(t, 2, rm.Count())

	rm.RemoveRoot("file:///a")
	assert.Equal(t, 1, rm.Count())
}

func TestRootManager_SupportsListChanged(t *testing.T) {
	rm1 := NewRootManager(true)
	assert.True(t, rm1.SupportsListChanged())

	rm2 := NewRootManager(false)
	assert.False(t, rm2.SupportsListChanged())
}

func TestRootManager_Concurrency(t *testing.T) {
	rm := NewRootManager(true)

	// Run concurrent operations
	done := make(chan bool, 3)

	// Goroutine 1: Add roots
	go func() {
		for i := 0; i < 100; i++ {
			rm.AddRoot(Root{URI: "file:///" + string(rune(i)), Name: "root"})
		}
		done <- true
	}()

	// Goroutine 2: Read roots
	go func() {
		for i := 0; i < 100; i++ {
			_ = rm.GetRoots()
			_ = rm.Count()
			_ = rm.IsWithinRoots("file:///test")
		}
		done <- true
	}()

	// Goroutine 3: Remove roots
	go func() {
		for i := 0; i < 50; i++ {
			rm.RemoveRoot("file:///" + string(rune(i)))
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		select {
		case <-done:
			// Good
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	// Just verify no panic occurred and state is consistent
	_ = rm.GetRoots()
	_ = rm.Count()
}

func TestRoot_Struct(t *testing.T) {
	tests := []struct {
		name     string
		root     Root
		wantURI  string
		wantName string
	}{
		{
			name:     "with name",
			root:     Root{URI: "file:///path", Name: "myroot"},
			wantURI:  "file:///path",
			wantName: "myroot",
		},
		{
			name:     "without name",
			root:     Root{URI: "file:///path"},
			wantURI:  "file:///path",
			wantName: "",
		},
		{
			name:     "empty",
			root:     Root{},
			wantURI:  "",
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantURI, tt.root.URI)
			assert.Equal(t, tt.wantName, tt.root.Name)
		})
	}
}

func TestRootsList_Struct(t *testing.T) {
	rl := RootsList{
		Roots: []Root{
			{URI: "file:///a", Name: "a"},
			{URI: "file:///b", Name: "b"},
		},
	}

	assert.Len(t, rl.Roots, 2)
	assert.Equal(t, "a", rl.Roots[0].Name)
	assert.Equal(t, "b", rl.Roots[1].Name)
}

func TestListRootsResult_Struct(t *testing.T) {
	result := ListRootsResult{
		Roots: []Root{
			{URI: "file:///project", Name: "project"},
		},
	}

	assert.Len(t, result.Roots, 1)
	assert.Equal(t, "project", result.Roots[0].Name)
}

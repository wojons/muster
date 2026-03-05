package progress

import (
	"bytes"
	"testing"
)

func TestProgressBar_Render(t *testing.T) {
	pb := NewProgressBar(100, "Processing")

	output := pb.Render()

	if !contains(output, "Processing") {
		t.Errorf("Expected output to contain 'Processing', got: %s", output)
	}

	if !contains(output, "0%") {
		t.Errorf("Expected output to contain '0%%', got: %s", output)
	}
}

func TestProgressBar_Set(t *testing.T) {
	pb := NewProgressBar(100, "Processing")

	pb.Set(50)

	if pb.Current != 50 {
		t.Errorf("Expected Current to be 50, got %d", pb.Current)
	}
}

func TestProgressBar_Add(t *testing.T) {
	pb := NewProgressBar(100, "Processing")

	pb.Add(25)

	if pb.Current != 25 {
		t.Errorf("Expected Current to be 25, got %d", pb.Current)
	}

	pb.Add(200)

	if pb.Current != 100 {
		t.Errorf("Expected Current to be capped at 100, got %d", pb.Current)
	}
}

func TestProgressBar_Incr(t *testing.T) {
	pb := NewProgressBar(10, "Processing")

	pb.Incr()
	pb.Incr()
	pb.Incr()

	if pb.Current != 3 {
		t.Errorf("Expected Current to be 3, got %d", pb.Current)
	}
}

func TestProgressBar_Finish(t *testing.T) {
	pb := NewProgressBar(100, "Processing")

	pb.Finish()

	if pb.Current != 100 {
		t.Errorf("Expected Current to be 100 after Finish, got %d", pb.Current)
	}

	output := pb.Render()
	if !contains(output, "100%") {
		t.Errorf("Expected output to contain '100%%', got: %s", output)
	}
}

func TestProgressBar_ShowCount(t *testing.T) {
	pb := NewProgressBar(100, "Processing")
	pb.ShowCount = true
	pb.Set(50)

	output := pb.Render()

	if !contains(output, "50/100") {
		t.Errorf("Expected output to contain '50/100', got: %s", output)
	}
}

func TestProgressBar_Display(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar(100, "Processing")
	pb.Set(50)

	pb.Draw(&buf)

	if buf.Len() == 0 {
		t.Error("Expected buffer to contain output")
	}
}

func TestProgressBar_Clear(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar(100, "Processing")

	pb.Clear(&buf)

	if buf.Len() == 0 {
		t.Error("Expected buffer to contain clear sequence")
	}
}

func TestProgressBar_ZeroTotal(t *testing.T) {
	pb := NewProgressBar(0, "Processing")

	output := pb.Render()

	if output != "Processing" {
		t.Errorf("Expected 'Processing' for zero total, got: %s", output)
	}
}

func TestNewProgressBar(t *testing.T) {
	pb := NewProgressBar(100, "Test")

	if pb.Total != 100 {
		t.Errorf("Expected Total to be 100, got %d", pb.Total)
	}

	if pb.Description != "Test" {
		t.Errorf("Expected Description to be 'Test', got %s", pb.Description)
	}

	if pb.Width != 40 {
		t.Errorf("Expected Width to be 40, got %d", pb.Width)
	}
}

func TestProgressBar_ThreadSafety(t *testing.T) {
	pb := NewProgressBar(100, "Processing")

	done := make(chan bool)

	// Concurrent Set calls
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				pb.Set(int64(j))
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

package progress

import (
	"testing"
	"time"
)

func TestNewSpinner(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: "Loading...",
		Speed:   100 * time.Millisecond,
	})

	if spinner.Message != "Loading..." {
		t.Errorf("Expected message 'Loading...', got '%s'", spinner.Message)
	}

	if len(spinner.Frames) == 0 {
		t.Errorf("Expected non-empty frames")
	}
}

func TestSpinner_SetMessage(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: "Loading...",
	})

	spinner.SetMessage("New message")

	if spinner.Message != "New message" {
		t.Errorf("Expected message to be updated to 'New message', got '%s'", spinner.Message)
	}
}

func TestSpinner_Stop(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: "Loading...",
	})

	// Stop can be called without panicking
	spinner.Stop()

	// Should not panic
}

func TestSpinner_StyleFallback(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   "invalid",
		Message: "Loading...",
	})

	if len(spinner.Frames) == 0 {
		t.Errorf("Expected fallback to StyleDots for invalid style")
	}
}

func TestSpinner_DefaultSpeed(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: "Loading...",
	})

	if spinner.Interval != 100*time.Millisecond {
		t.Errorf("Expected default speed of 100ms, got %v", spinner.Interval)
	}
}

func TestSpinnerStyle_Constants(t *testing.T) {
	if StyleDots != "dots" {
		t.Errorf("Expected StyleDots to be 'dots', got %s", StyleDots)
	}

	if StyleLine != "line" {
		t.Errorf("Expected StyleLine to be 'line', got %s", StyleLine)
	}

	if StyleSquare != "square" {
		t.Errorf("Expected StyleSquare to be 'square', got %s", StyleSquare)
	}
}

func TestSpinnerFrames(t *testing.T) {
	dotsFrames := SpinnerFrames[StyleDots]
	if len(dotsFrames) == 0 {
		t.Errorf("Expected non-empty frames for StyleDots")
	}

	lineFrames := SpinnerFrames[StyleLine]
	if len(lineFrames) == 0 {
		t.Errorf("Expected non-empty frames for StyleLine")
	}

	squareFrames := SpinnerFrames[StyleSquare]
	if len(squareFrames) == 0 {
		t.Errorf("Expected non-empty frames for StyleSquare")
	}
}

func TestNewSpinner_ZeroSpeed(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: "Loading...",
		Speed:   0,
	})

	if spinner.Interval != 100*time.Millisecond {
		t.Errorf("Expected default speed when Speed is 0, got %v", spinner.Interval)
	}
}

func TestSpinner_ThreadSafety(t *testing.T) {
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: "Loading...",
	})

	done := make(chan bool)

	// Concurrent SetMessage calls
	for i := 0; i < 10; i++ {
		go func(n int) {
			spinner.SetMessage(string(rune('A' + n%26)))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic
}

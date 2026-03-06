package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// SpinnerStyle defines the animation style
type SpinnerStyle string

const (
	StyleDots   SpinnerStyle = "dots"
	StyleLine   SpinnerStyle = "line"
	StyleSquare SpinnerStyle = "square"
)

// SpinnerFrames defines frame sets for different styles
var SpinnerFrames = map[SpinnerStyle][]string{
	StyleDots:   {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	StyleLine:   {"-", "\\", "|", "/"},
	StyleSquare: {"◰", "◳", "◲", "◱"},
}

// SpinnerOptions configures a spinner
type SpinnerOptions struct {
	Style   SpinnerStyle
	Message string
	Speed   time.Duration
}

// Spinner displays an indeterminate progress indicator
type Spinner struct {
	Frames   []string
	Interval time.Duration
	Message  string
	current  int
	mu       sync.Mutex
	stopChan chan struct{}
	running  bool
}

// NewSpinner creates a new spinner
func NewSpinner(opts SpinnerOptions) *Spinner {
	frames := SpinnerFrames[opts.Style]
	if frames == nil {
		frames = SpinnerFrames[StyleDots]
	}

	speed := opts.Speed
	if speed == 0 {
		speed = 100 * time.Millisecond
	}

	return &Spinner{
		Frames:   frames,
		Interval: speed,
		Message:  opts.Message,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start(w io.Writer) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.draw(w)
		case <-func() chan struct{} {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.stopChan
		}():
			s.clear(w)
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		}
	}
}

// StartAsync starts the spinner in a goroutine
func (s *Spinner) StartAsync(w io.Writer) {
	go s.Start(w)
}

// draw renders the current frame
func (s *Spinner) draw(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	frame := s.Frames[s.current%len(s.Frames)]
	s.current++

	if s.Message != "" {
		fmt.Fprintf(w, "\r%s %s", frame, s.Message)
	} else {
		fmt.Fprintf(w, "\r%s", frame)
	}
}

// Stop halts the spinner
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopChan)
		s.stopChan = make(chan struct{})
	}
}

// clear clears the spinner line
func (s *Spinner) clear(w io.Writer) {
	fmt.Fprint(w, "\r\033[K")
}

// SetMessage updates the spinner message
func (s *Spinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Message = message
}

// DoWithSpinner executes a function while showing a spinner
func DoWithSpinner(message string, fn func() error) error {
	var w io.Writer = os.Stdout
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: message,
		Speed:   100 * time.Millisecond,
	})

	spinner.StartAsync(w)

	err := fn()

	spinner.Stop()

	return err
}

// DoWithSpinnerContext executes a function with context while showing a spinner
func DoWithSpinnerContext(ctx context.Context, message string, fn func(context.Context) error) error {
	var w io.Writer = os.Stdout
	spinner := NewSpinner(SpinnerOptions{
		Style:   StyleDots,
		Message: message,
		Speed:   100 * time.Millisecond,
	})

	// Start spinner in goroutine
	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	// Run spinner until context canceled or function completes
	for {
		select {
		case <-ctx.Done():
			spinner.Stop()
			return ctx.Err()
		case err := <-done:
			spinner.Stop()
			return err
		default:
			spinner.draw(w)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

# CLI Progress Indicators Specification

**Status**: ✅ COMPLETE  
**Date**: 2026-02-28  
**Purpose**: Progress bars, spinners, and status messages for long-running operations  
**Language**: Go 1.21+  

---

## Overview

Progress indicators provide visual feedback for long-running operations, including progress bars, spinners, status messages, and multi-progress displays.

---

## Progress Bars

### Basic Progress Bar

```go
// internal/cli/progress/bar.go
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProgressBar displays a visual progress bar
type ProgressBar struct {
	writer      io.Writer
	width       int
	current     int64
	total       int64
	startTime   time.Time
	prefix      string
	suffix      string
	showPercent bool
	showRate    bool
	mu          sync.RWMutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int64) *ProgressBar {
	return &ProgressBar{
		writer:      os.Stdout,
		width:       40,
		total:       total,
		startTime:   time.Now(),
		showPercent: true,
		showRate:    true,
	}
}

// SetWriter sets the output writer
func (pb *ProgressBar) SetWriter(w io.Writer) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.writer = w
}

// SetPrefix sets the prefix text
func (pb *ProgressBar) SetPrefix(prefix string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.prefix = prefix
}

// SetSuffix sets the suffix text
func (pb *ProgressBar) SetSuffix(suffix string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.suffix = suffix
}

// Add increments progress by n
func (pb *ProgressBar) Add(n int64) {
	atomic.AddInt64(&pb.current, n)
	pb.Render()
}

// Set sets current progress
func (pb *ProgressBar) Set(n int64) {
	atomic.StoreInt64(&pb.current, n)
	pb.Render()
}

// Inc increments by 1
func (pb *ProgressBar) Inc() {
	pb.Add(1)
}

// Render draws the progress bar
func (pb *ProgressBar) Render() {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	current := atomic.LoadInt64(&pb.current)
	if pb.total == 0 {
		return
	}

	percent := float64(current) / float64(pb.total)
	if percent > 1.0 {
		percent = 1.0
	}

	// Build bar
	filled := int(percent * float64(pb.width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", pb.width-filled)

	var parts []string

	if pb.prefix != "" {
		parts = append(parts, pb.prefix)
	}

	if pb.showPercent {
		parts = append(parts, fmt.Sprintf("%3.0f%%", percent*100))
	}

	parts = append(parts, "["+bar+"]")
	parts = append(parts, fmt.Sprintf("%d/%d", current, pb.total))

	if pb.showRate {
		elapsed := time.Since(pb.startTime)
		if elapsed > 0 {
			rate := float64(current) / elapsed.Seconds()
			parts = append(parts, fmt.Sprintf("%.1f/s", rate))
		}
	}

	if pb.suffix != "" {
		parts = append(parts, pb.suffix)
	}

	// Clear line and print
	fmt.Fprintf(pb.writer, "\r\033[K%s", strings.Join(parts, " "))
}

// Finish completes the progress bar
func (pb *ProgressBar) Finish() {
	pb.Set(pb.total)
	fmt.Fprintln(pb.writer)
}

// Close implements io.Closer
func (pb *ProgressBar) Close() error {
	pb.Finish()
	return nil
}
```

---

## Spinners

### Spinner Implementation

```go
// internal/cli/progress/spinner.go
package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Spinner displays a loading animation
type Spinner struct {
	writer   io.Writer
	frames   []string
	prefix   string
	suffix   string
	interval time.Duration
	current  int
	active   bool
	stopChan chan struct{}
	mu       sync.RWMutex
}

// Spinner styles
var (
	SpinnerDots = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	SpinnerLine = []string{"-", "\\", "|", "/"}
	SpinnerPulse = []string{"◐", "◓", "◑", "◒"}
)

// NewSpinner creates a new spinner
func NewSpinner() *Spinner {
	return &Spinner{
		writer:   os.Stdout,
		frames:   SpinnerDots,
		interval: 100 * time.Millisecond,
		stopChan: make(chan struct{}),
	}
}

// SetFrames sets the animation frames
func (s *Spinner) SetFrames(frames []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames = frames
}

// SetPrefix sets the prefix text
func (s *Spinner) SetPrefix(prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefix = prefix
}

// SetSuffix sets the suffix text
func (s *Spinner) SetSuffix(suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suffix = suffix
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.animate()
}

// Stop ends the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		s.active = false
		close(s.stopChan)
		fmt.Fprintf(s.writer, "\r\033[K")
	}
}

func (s *Spinner) animate() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.render()
			s.current = (s.current + 1) % len(s.frames)
		case <-s.stopChan:
			return
		}
	}
}

func (s *Spinner) render() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	frame := s.frames[s.current]
	var parts []string

	if s.prefix != "" {
		parts = append(parts, s.prefix)
	}

	parts = append(parts, frame)

	if s.suffix != "" {
		parts = append(parts, s.suffix)
	}

	fmt.Fprintf(s.writer, "\r\033[K%s", strings.Join(parts, " "))
}
```

---

## Status Messages

### Status Display

```go
// internal/cli/progress/status.go
package progress

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
)

// StatusLevel represents message importance
type StatusLevel int

const (
	StatusInfo StatusLevel = iota
	StatusSuccess
	StatusWarning
	StatusError
	StatusDebug
)

// StatusMessage represents a status message
type StatusMessage struct {
	Level   StatusLevel
	Message string
	Time    time.Time
}

// StatusPrinter prints status messages
type StatusPrinter struct {
	writer    io.Writer
	colors    bool
	showTime  bool
	indent    int
}

// NewStatusPrinter creates a new status printer
func NewStatusPrinter() *StatusPrinter {
	return &StatusPrinter{
		writer:   os.Stdout,
		colors:   true,
		showTime: false,
	}
}

// Print prints a status message
func (sp *StatusPrinter) Print(level StatusLevel, message string) {
	prefix := sp.prefixFor(level)
	indent := strings.Repeat("  ", sp.indent)

	var timeStr string
	if sp.showTime {
		timeStr = time.Now().Format("15:04:05 ")
	}

	if sp.colors {
		colored := sp.colorize(level, prefix)
		fmt.Fprintf(sp.writer, "%s%s%s %s\n", timeStr, indent, colored, message)
	} else {
		fmt.Fprintf(sp.writer, "%s%s%s %s\n", timeStr, indent, prefix, message)
	}
}

// Info prints an info message
func (sp *StatusPrinter) Info(message string) {
	sp.Print(StatusInfo, message)
}

// Success prints a success message
func (sp *StatusPrinter) Success(message string) {
	sp.Print(StatusSuccess, message)
}

// Warning prints a warning message
func (sp *StatusPrinter) Warning(message string) {
	sp.Print(StatusWarning, message)
}

// Error prints an error message
func (sp *StatusPrinter) Error(message string) {
	sp.Print(StatusError, message)
}

// Debug prints a debug message
func (sp *StatusPrinter) Debug(message string) {
	sp.Print(StatusDebug, message)
}

func (sp *StatusPrinter) prefixFor(level StatusLevel) string {
	switch level {
	case StatusInfo:
		return "ℹ"
	case StatusSuccess:
		return "✓"
	case StatusWarning:
		return "⚠"
	case StatusError:
		return "✗"
	case StatusDebug:
		return "◆"
	default:
		return "•"
	}
}

func (sp *StatusPrinter) colorize(level StatusLevel, text string) string {
	switch level {
	case StatusSuccess:
		return color.GreenString(text)
	case StatusWarning:
		return color.YellowString(text)
	case StatusError:
		return color.RedString(text)
	case StatusDebug:
		return color.CyanString(text)
	default:
		return color.BlueString(text)
	}
}

// Indent increases indentation
func (sp *StatusPrinter) Indent() {
	sp.indent++
}

// Dedent decreases indentation
func (sp *StatusPrinter) Dedent() {
	if sp.indent > 0 {
		sp.indent--
	}
}
```

---

## Multi-Progress

### Multiple Concurrent Progress

```go
// internal/cli/progress/multi.go
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// MultiProgress manages multiple progress bars
type MultiProgress struct {
	writer io.Writer
	bars   []*ProgressBar
	mu     sync.RWMutex
}

// NewMultiProgress creates a multi-progress manager
func NewMultiProgress() *MultiProgress {
	return &MultiProgress{
		writer: os.Stdout,
		bars:   make([]*ProgressBar, 0),
	}
}

// AddBar adds a new progress bar
func (mp *MultiProgress) AddBar(total int64, prefix string) *ProgressBar {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	bar := &ProgressBar{
		writer:      io.Discard, // Write to buffer
		total:       total,
		prefix:      prefix,
		showPercent: true,
	}

	mp.bars = append(mp.bars, bar)
	return bar
}

// Render draws all progress bars
func (mp *MultiProgress) Render() {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Clear previous output
	for i := 0; i < len(mp.bars); i++ {
		fmt.Fprint(mp.writer, "\033[A\033[K")
	}

	// Render all bars
	for _, bar := range mp.bars {
		bar.Render()
		fmt.Fprintln(mp.writer)
	}
}

// Wait waits for all bars to complete
func (mp *MultiProgress) Wait() {
	for _, bar := range mp.bars {
		for !bar.IsComplete() {
			time.Sleep(100 * time.Millisecond)
			mp.Render()
		}
	}
}

// IsComplete checks if all bars are complete
func (pb *ProgressBar) IsComplete() bool {
	return atomic.LoadInt64(&pb.current) >= pb.total
}
```

---

## Usage Example

```go
// Progress bar
bar := progress.NewProgressBar(100)
bar.SetPrefix("Uploading")
for i := 0; i < 100; i++ {
    bar.Inc()
    time.Sleep(50 * time.Millisecond)
}
bar.Finish()

// Spinner
spinner := progress.NewSpinner()
spinner.SetSuffix("Loading...")
spinner.Start()
time.Sleep(3 * time.Second)
spinner.Stop()

// Status messages
status := progress.NewStatusPrinter()
status.Info("Starting operation")
status.Success("Complete")
status.Warning("Something might be wrong")
status.Error("Failed")
```

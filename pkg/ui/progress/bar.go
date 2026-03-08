package progress

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ProgressBar displays operation progress
type ProgressBar struct {
	Total       int64
	Current     int64
	Width       int
	Description string
	StartTime   time.Time
	ShowCount   bool
	ShowETA     bool
	mu          sync.RWMutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int64, description string) *ProgressBar {
	return &ProgressBar{
		Total:       total,
		Width:       40,
		Description: description,
		StartTime:   time.Now(),
	}
}

// Render generates the progress bar string
func (pb *ProgressBar) Render() string {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	if pb.Total == 0 {
		return pb.Description
	}

	percent := float64(pb.Current) / float64(pb.Total)
	filled := int(percent * float64(pb.Width))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", pb.Width-filled)

	var parts []string
	parts = append(parts, pb.Description)
	parts = append(parts, fmt.Sprintf("[%s]", bar))
	parts = append(parts, fmt.Sprintf("%3.0f%%", percent*100))

	if pb.ShowCount {
		parts = append(parts, fmt.Sprintf("(%d/%d)", pb.Current, pb.Total))
	}

	if pb.ShowETA && pb.Current > 0 {
		eta := pb.calculateETA()
		parts = append(parts, fmt.Sprintf("ETA: %s", eta))
	}

	return strings.Join(parts, " ")
}

// calculateETA estimates time remaining
func (pb *ProgressBar) calculateETA() string {
	elapsed := time.Since(pb.StartTime)
	rate := float64(pb.Current) / elapsed.Seconds()
	if rate == 0 {
		return "--:--"
	}
	remaining := float64(pb.Total-pb.Current) / rate
	duration := time.Duration(remaining * float64(time.Second))
	seconds := int(duration.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	return fmt.Sprintf("%dh", hours)
}

// Set updates the current progress
func (pb *ProgressBar) Set(current int64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.Current = current
	if pb.Current > pb.Total {
		pb.Current = pb.Total
	}
}

// Add increments the current progress
func (pb *ProgressBar) Add(n int64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.Current += n
	if pb.Current > pb.Total {
		pb.Current = pb.Total
	}
}

// Finish completes the progress bar
func (pb *ProgressBar) Finish() {
	pb.Set(pb.Total)
}

// Draw renders the progress bar to writer
func (pb *ProgressBar) Draw(w io.Writer) {
	_, _ = fmt.Fprint(w, "\r"+pb.Render())
}

// Clear clears the progress bar line
func (pb *ProgressBar) Clear(w io.Writer) {
	_, _ = fmt.Fprint(w, "\r\033[K")
}

// Incr increments progress by 1
func (pb *ProgressBar) Incr() {
	pb.Add(1)
}

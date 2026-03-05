# Progress Bars Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **progress bars** for long-running operations.

## 1. Progress Bar Structure

```go
type ProgressBar struct {
    Total       int64
    Current     int64
    Width       int
    Description string
    StartTime   time.Time
    mu          sync.RWMutex
}

type ProgressBarOptions struct {
    Width       int
    Description string
    ShowPercent bool
    ShowCount   bool
    ShowETA     bool
}
```

## 2. Rendering

```go
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

func (pb *ProgressBar) calculateETA() string {
    elapsed := time.Since(pb.StartTime)
    rate := float64(pb.Current) / elapsed.Seconds()
    remaining := float64(pb.Total-pb.Current) / rate
    
    return time.Duration(remaining * float64(time.Second)).String()
}
```

## 3. Update Methods

```go
func (pb *ProgressBar) Set(current int64) {
    pb.mu.Lock()
    defer pb.mu.Unlock()
    pb.Current = current
    if pb.Current > pb.Total {
        pb.Current = pb.Total
    }
}

func (pb *ProgressBar) Add(n int64) {
    pb.mu.Lock()
    defer pb.mu.Unlock()
    pb.Current += n
    if pb.Current > pb.Total {
        pb.Current = pb.Total
    }
}

func (pb *ProgressBar) Finish() {
    pb.Set(pb.Total)
}
```

## 4. Terminal Integration

```go
func (pb *ProgressBar) Draw(w io.Writer) {
    fmt.Fprint(w, "\r"+pb.Render())
}

func (pb *ProgressBar) Clear(w io.Writer) {
    // Clear the line
    fmt.Fprint(w, "\r\033[K")
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

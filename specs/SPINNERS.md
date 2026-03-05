# Spinners Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **spinners** for indeterminate progress.

## 1. Spinner Structure

```go
type Spinner struct {
    Frames      []string
    Interval    time.Duration
    Message     string
    current     int
    mu          sync.Mutex
    stopChan    chan struct{}
}

type SpinnerOptions struct {
    Style   SpinnerStyle
    Message string
    Speed   time.Duration
}

type SpinnerStyle string

const (
    StyleDots   SpinnerStyle = "dots"
    StyleLine   SpinnerStyle = "line"
    StyleSquare SpinnerStyle = "square"
)
```

## 2. Spinner Styles

```go
var SpinnerFrames = map[SpinnerStyle][]string{
    StyleDots:   {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
    StyleLine:   {"-", "\\", "|", "/"},
    StyleSquare: {"◰", "◳", "◲", "◱"},
}
```

## 3. Operations

```go
func NewSpinner(opts SpinnerOptions) *Spinner {
    frames := SpinnerFrames[opts.Style]
    if frames == nil {
        frames = SpinnerFrames[StyleDots]
    }
    
    return &Spinner{
        Frames:   frames,
        Interval: opts.Speed,
        Message:  opts.Message,
        stopChan: make(chan struct{}),
    }
}

func (s *Spinner) Start(w io.Writer) {
    ticker := time.NewTicker(s.Interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            s.draw(w)
        case <-s.stopChan:
            s.clear(w)
            return
        }
    }
}

func (s *Spinner) draw(w io.Writer) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    frame := s.Frames[s.current%len(s.Frames)]
    s.current++
    
    if s.Message != "" {
        fmt.Fprintf(w, "\r%s %s", frame, s.Message)
    } else {
        fmt.Fprintf(w, "\r%s", frame)
    }
}

func (s *Spinner) Stop() {
    close(s.stopChan)
}

func (s *Spinner) clear(w io.Writer) {
    fmt.Fprint(w, "\r\033[K")
}
```

## 4. Usage

```go
func DoWithSpinner(ctx context.Context, message string, fn func() error) error {
    spinner := NewSpinner(SpinnerOptions{
        Style:   StyleDots,
        Message: message,
        Speed:   100 * time.Millisecond,
    })
    
    go spinner.Start(os.Stderr)
    
    err := fn()
    
    spinner.Stop()
    
    return err
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

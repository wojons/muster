# OS Signals Specification

Complete specification for signal handling and graceful shutdown across platforms.

## Overview

Cross-platform signal handling with graceful shutdown support, signal mapping, and context cancellation integration.

## Signal Constants

```go
package signalutil

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// ShutdownSignals are signals that trigger graceful shutdown
var ShutdownSignals = []os.Signal{
	syscall.SIGINT,
	syscall.SIGTERM,
}

// ReloadSignals are signals that trigger configuration reload
var ReloadSignals = []os.Signal{
	syscall.SIGHUP,
}

// Platform-specific signals
func init() {
	// Add platform-specific signals
	if isWindows() {
		// Windows doesn't have SIGUSR1/SIGUSR2
	} else {
		ShutdownSignals = append(ShutdownSignals, syscall.SIGUSR1)
	}
}
```

## Signal Handler

```go
// Handler manages signal handling
type Handler struct {
	shutdownSignals []os.Signal
	reloadSignals   []os.Signal
	shutdownCh      chan struct{}
	reloadCh        chan struct{}
}

// NewHandler creates a signal handler
func NewHandler() *Handler {
	return &Handler{
		shutdownSignals: ShutdownSignals,
		reloadSignals:   ReloadSignals,
		shutdownCh:      make(chan struct{}),
		reloadCh:        make(chan struct{}),
	}
}

// Setup installs signal handlers
func (h *Handler) Setup() {
	// Combine all signals
	allSignals := append(h.shutdownSignals, h.reloadSignals...)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, allSignals...)

	go func() {
		for sig := range sigChan {
			for _, s := range h.shutdownSignals {
				if sig == s {
					close(h.shutdownCh)
					return
				}
			}
			for _, s := range h.reloadSignals {
				if sig == s {
					h.reloadCh <- struct{}{}
				}
			}
		}
	}()
}

// Shutdown returns channel for shutdown signals
func (h *Handler) Shutdown() <-chan struct{} {
	return h.shutdownCh
}

// Reload returns channel for reload signals
func (h *Handler) Reload() <-chan struct{} {
	return h.reloadCh
}
```

## Context Integration

```go
// WithShutdown returns context that cancels on shutdown signal
func WithShutdown(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, ShutdownSignals...)
	
	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()
	
	return ctx, cancel
}

// WaitForShutdown blocks until shutdown signal received
func WaitForShutdown() os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, ShutdownSignals...)
	return <-sigChan
}
```

## Platform Mapping

```go
// SignalName returns human-readable signal name
func SignalName(sig os.Signal) string {
	if s, ok := sig.(syscall.Signal); ok {
		name := syscall.SignalName(s)
		if name != "" {
			return name
		}
	}
	return sig.String()
}

// SendSignal sends signal to process
func SendSignal(pid int, sig os.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(sig)
}
```

## Usage Example

```go
func RunServer() error {
	ctx, cancel := signalutil.WithShutdown(context.Background())
	defer cancel()

	server := &http.Server{Addr: ":8080"}
	
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()
	
	return server.ListenAndServe()
}
```

## Version

**1.0.0** - 2024-01-15

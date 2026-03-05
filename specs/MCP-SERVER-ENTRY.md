# MCP Server Entry Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Entry Point

## Overview

Complete specification of the **MCP server entry point**, handling CLI invocation, configuration, and stdio setup.

## 1. Entry Point

```go
// cmd/mcp-server/main.go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Handle signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigChan
        cancel()
    }()
    
    // Create and run server
    server := NewMCPServer(os.Stdin, os.Stdout, os.Stderr)
    
    if err := server.Run(ctx); err != nil && err != context.Canceled {
        os.Exit(1)
    }
}
```

## 2. Configuration

```go
type MCPServerConfig struct {
    // Logging
    LogLevel  string `json:"log_level"`
    LogOutput string `json:"log_output"`
    
    // Features
    EnableTools     bool `json:"enable_tools"`
    EnableResources bool `json:"enable_resources"`
    EnablePrompts   bool `json:"enable_prompts"`
    
    // Command execution
    MaxConcurrentCommands int           `json:"max_concurrent_commands"`
    CommandTimeout        time.Duration `json:"command_timeout"`
}

func loadConfig() *MCPServerConfig {
    return &MCPServerConfig{
        LogLevel:              "info",
        EnableTools:           true,
        EnableResources:       true,
        EnablePrompts:         true,
        MaxConcurrentCommands: 10,
        CommandTimeout:        30 * time.Second,
    }
}
```

## 3. Stdio Setup

```go
func setupStdio() (stdin io.Reader, stdout, stderr io.Writer) {
    // Ensure binary mode on Windows
    if runtime.GOOS == "windows" {
        enableBinaryMode()
    }
    
    // Set up line buffering
    stdin = bufio.NewReader(os.Stdin)
    stdout = bufio.NewWriter(os.Stdout)
    stderr = os.Stderr
    
    return
}
```

## 4. Error Handling

```go
func handleFatal(err error) {
    fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
    os.Exit(1)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

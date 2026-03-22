// Package config provides daemon configuration management with hot reload support.
//
// The config package manages daemon configuration from JSON files with support for
// live reloading when configuration files change. It provides thread-safe access
// to configuration values and notification mechanisms for configuration updates.
//
// Basic Usage:
//
//	mgr := config.NewManager("/etc/muster/daemon.json")
//	if err := mgr.Load(); err != nil {
//	    log.Fatal(err)
//	}
//
// Configuration Structure:
//
// The Config struct holds all daemon configuration values:
//
//	socket_path       - Unix socket path for daemon communication
//	pid_file          - Path to PID file for process management
//	log_file          - Path to daemon log file
//	target_url        - Backend URL for proxy requests
//	socket_mode       - File permissions for Unix socket (default: 0660)
//	max_connections   - Maximum concurrent connections (default: 100)
//	keep_alive        - Enable HTTP keep-alive (default: true)
//	keep_alive_period - Keep-alive duration (default: 30s)
//	read_timeout      - HTTP read timeout (default: 30s)
//	write_timeout     - HTTP write timeout (default: 30s)
//	shutdown_timeout  - Graceful shutdown timeout (default: 30s)
//	log_level         - Log level: debug, info, warn, error (default: info)
//
// Hot Reload:
//
// Use FileWatcher to monitor configuration file changes:
//
//	watcher := config.NewFileWatcher("/etc/muster/daemon.json", mgr)
//	if err := watcher.Initialize(); err != nil {
//	    log.Fatal(err)
//	}
//	go watcher.Start(5 * time.Second)
//
// Watch for configuration changes:
//
//	ch := mgr.Watch()
//	go func() {
//	    for cfg := range ch {
//	        log.Printf("Config reloaded: socket=%s", cfg.SocketPath)
//	    }
//	}()
//
// Manual Reload:
//
//	mgr.SignalReload() // Signal that configuration should be reloaded
//
// Thread Safety:
//
// All Manager methods are thread-safe. Configuration values can be read
// concurrently while updates are processed safely.
//
// File Format:
//
// Configuration files use JSON format:
//
//	{
//	  "socket_path": "/var/run/muster/daemon.sock",
//	  "pid_file": "/var/run/muster/daemon.pid",
//	  "log_file": "/var/log/muster/daemon.log",
//	  "target_url": "http://localhost:8080",
//	  "socket_mode": 416,
//	  "max_connections": 100,
//	  "keep_alive": true,
//	  "keep_alive_period": 30000000000,
//	  "read_timeout": 30000000000,
//	  "write_timeout": 30000000000,
//	  "shutdown_timeout": 30000000000,
//	  "log_level": "info"
//	}
//
// Validation:
//
// Configuration values are validated on load and update:
//   - socket_path is required
//   - target_url is required
//   - max_connections must be positive
//   - read_timeout must be positive
//   - write_timeout must be positive
//
// Default Configuration:
//
// If no configuration file exists, sensible defaults are used:
//
//	cfg := config.Default()
//
// Package Components:
//
//   - Config: Configuration struct with all daemon settings
//   - Manager: Thread-safe configuration management
//   - FileWatcher: File system monitoring for hot reload
//   - Default(): Returns default configuration values
//
// Example: Complete Setup
//
//	package main
//
//	import (
//	    "context"
//	    "log"
//	    "time"
//
//	    "github.com/anomalyco/muster/internal/daemon/config"
//	)
//
//	func main() {
//	    // Create manager with config file path
//	    mgr := config.NewManager("/etc/muster/daemon.json")
//
//	    // Load existing config or use defaults
//	    if err := mgr.Load(); err != nil {
//	        log.Fatalf("Failed to load config: %v", err)
//	    }
//
//	    // Start file watcher for hot reload
//	    watcher := config.NewFileWatcher("/etc/muster/daemon.json", mgr)
//	    if err := watcher.Initialize(); err != nil {
//	        log.Fatalf("Failed to initialize watcher: %v", err)
//	    }
//	    defer watcher.Stop()
//
//	    go watcher.Start(5 * time.Second)
//
//	    // Watch for config updates
//	    ch := mgr.Watch()
//	    go func() {
//	        for cfg := range ch {
//	            log.Printf("Configuration updated: target_url=%s", cfg.TargetURL)
//	        }
//	    }()
//
//	    // Get current config
//	    cfg := mgr.Get()
//	    log.Printf("Daemon configured: socket=%s, target=%s", cfg.SocketPath, cfg.TargetURL)
//
//	    // Block forever
//	    select {}
//	}
//
// Error Handling:
//
// All methods return descriptive errors. Common errors include:
//   - File read errors (permissions, not found)
//   - JSON parse errors (invalid syntax)
//   - Validation errors (missing required fields)
//   - File write errors (permissions, disk full)
//
// The Manager handles missing configuration files gracefully by using
// default values and creating the file on first Save().
package config

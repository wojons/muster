# Daemon Controller Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Daemon

## Overview

Complete specification of **daemon lifecycle controller**.

## 1. Controller Structure

```go
type DaemonController struct {
    pidFile    string
    socketPath string
    logger     *log.Logger
}

func NewDaemonController() *DaemonController {
    return &DaemonController{
        pidFile:    filepath.Join(getDataDir(), "daemon.pid"),
        socketPath: filepath.Join(getDataDir(), "daemon.sock"),
    }
}
```

## 2. Lifecycle Operations

```go
func (dc *DaemonController) Start() error {
    // Check if already running
    if dc.IsRunning() {
        return errors.New("daemon already running")
    }
    
    // Fork process
    cmd := exec.Command(os.Args[0], "daemon", "run")
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setsid: true,
    }
    
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("failed to start daemon: %w", err)
    }
    
    // Write PID file
    if err := dc.writePID(cmd.Process.Pid); err != nil {
        cmd.Process.Kill()
        return err
    }
    
    return nil
}

func (dc *DaemonController) Stop() error {
    pid, err := dc.readPID()
    if err != nil {
        return err
    }
    
    process, err := os.FindProcess(pid)
    if err != nil {
        return err
    }
    
    // Graceful shutdown
    if err := process.Signal(syscall.SIGTERM); err != nil {
        return err
    }
    
    // Wait for shutdown
    time.Sleep(2 * time.Second)
    
    // Force kill if still running
    if dc.IsRunning() {
        process.Kill()
    }
    
    // Clean up
    dc.cleanup()
    
    return nil
}

func (dc *DaemonController) Status() (DaemonStatus, error) {
    if !dc.IsRunning() {
        return DaemonStatusStopped, nil
    }
    
    // Check socket
    if _, err := os.Stat(dc.socketPath); err != nil {
        return DaemonStatusStarting, nil
    }
    
    return DaemonStatusRunning, nil
}
```

## 3. PID File Management

```go
func (dc *DaemonController) writePID(pid int) error {
    return os.WriteFile(dc.pidFile, []byte(strconv.Itoa(pid)), 0644)
}

func (dc *DaemonController) readPID() (int, error) {
    data, err := os.ReadFile(dc.pidFile)
    if err != nil {
        return 0, err
    }
    
    return strconv.Atoi(strings.TrimSpace(string(data)))
}

func (dc *DaemonController) IsRunning() bool {
    pid, err := dc.readPID()
    if err != nil {
        return false
    }
    
    process, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    
    // Check if process exists
    err = process.Signal(syscall.Signal(0))
    return err == nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

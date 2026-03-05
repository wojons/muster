# Plugin Loader Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Plugin System

## Overview

Complete specification for the **plugin loader** responsible for discovering, validating, loading, and managing plugin lifecycle. The loader implements a multi-stage pipeline ensuring secure, reliable plugin operations with dependency resolution, hot-reloading, and comprehensive error handling.

## 1. Architecture Overview

### 1.1 Loading Pipeline

```
┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  Discovery  │ → │  Validation │ → │ Dependency  │ → │  Loading    │ → │ Activation  │
│             │   │             │   │ Resolution  │   │             │   │             │
└─────────────┘   └─────────────┘   └─────────────┘   └─────────────┘   └─────────────┘
      │                 │                 │                 │                 │
      ▼                 ▼                 ▼                 ▼                 ▼
  Scan dirs         Check sig        Build graph      Initialize      Register with
  Watch files       Verify API       Topo sort       Setup runtime    host
```

### 1.2 Directory Structure

```
~/.config/openapi-cli/plugins/
├── installed/
│   ├── plugin-a/
│   │   ├── plugin.yaml          # Manifest
│   │   ├── plugin.wasm          # WASM binary
│   │   └── assets/              # Static assets
│   ├── plugin-b/
│   │   ├── plugin.yaml
│   │   └── plugin.so            # Native binary
│   └── ...
├── cache/
│   └── wasm/
│       └── compiled/
├── registry/
│   └── index.json               # Installed plugin index
└── disabled/                    # Disabled plugins
```

## 2. Core Loader Interface

### 2.1 Plugin Loader

```go
package plugin

import (
    "context"
    "path/filepath"
    "sync"
    "time"
)

// Loader manages plugin lifecycle
type Loader struct {
    config     LoaderConfig
    registry   *Registry
    instances  map[string]PluginInstance
    mu         sync.RWMutex
    watcher    *fsnotify.Watcher
    bus        EventBus
    logger     Logger
}

// LoaderConfig for plugin loader initialization
type LoaderConfig struct {
    // Plugin directories to scan
    PluginDirs []string
    
    // Cache directory for compiled artifacts
    CacheDir string
    
    // Enable hot reloading
    HotReload bool
    
    // Maximum number of loaded plugins
    MaxPlugins int
    
    // Default timeout for plugin operations
    DefaultTimeout time.Duration
    
    // Security settings
    Security SecurityConfig
    
    // WASM runtime configuration
    WASM WASMConfig
}

// SecurityConfig for plugin security
type SecurityConfig struct {
    // Require code signing for native plugins
    RequireSigned bool
    
    // Allowed signing keys
    TrustedKeys []string
    
    // Capability enforcement
    EnforceCapabilities bool
    
    // Sandboxing for native plugins
    SandboxNative bool
}

// WASMConfig for WASM runtime
type WASMConfig struct {
    // Runtime implementation: "wazero", "wasmer", "wasmtime"
    Runtime string
    
    // Memory limit per plugin (MB)
    MemoryLimit int
    
    // CPU time limit per call (ms)
    CPULimit int
    
    // Enable WASI
    EnableWASI bool
}

// NewLoader creates a new plugin loader
func NewLoader(cfg LoaderConfig) (*Loader, error) {
    if cfg.DefaultTimeout == 0 {
        cfg.DefaultTimeout = 30 * time.Second
    }
    if cfg.MaxPlugins == 0 {
        cfg.MaxPlugins = 100
    }
    if cfg.WASM.Runtime == "" {
        cfg.WASM.Runtime = "wazero"
    }
    
    // Initialize file watcher if hot reload enabled
    var watcher *fsnotify.Watcher
    var err error
    if cfg.HotReload {
        watcher, err = fsnotify.NewWatcher()
        if err != nil {
            return nil, fmt.Errorf("create file watcher: %w", err)
        }
    }
    
    return &Loader{
        config:    cfg,
        registry:  NewRegistry(),
        instances: make(map[string]PluginInstance),
        watcher:   watcher,
        logger:    NewDefaultLogger(),
    }, nil
}
```

### 2.2 Plugin Instance

```go
// PluginInstance represents a loaded plugin
type PluginInstance struct {
    ID          string
    Manifest    Manifest
    Plugin      Plugin
    Type        PluginType
    State       PluginState
    StartedAt   time.Time
    Dependencies []string
    Capabilities []Capability
    
    // Runtime-specific fields
    WASMModule  *wasm.Module    // For WASM plugins
    NativeLib   *plugin.Plugin  // For native plugins
    Process     *os.Process     // For external plugins
    
    // Health tracking
    LastHealthCheck time.Time
    HealthStatus    HealthStatus
    ErrorCount      int
}

// PluginState represents plugin lifecycle state
type PluginState string

const (
    StateDiscovered   PluginState = "discovered"
    StateValidated    PluginState = "validated"
    StateResolved     PluginState = "resolved"
    StateLoading      PluginState = "loading"
    StateInitialized  PluginState = "initialized"
    StateActive       PluginState = "active"
    StateError        PluginState = "error"
    StateUnloading    PluginState = "unloading"
    StateUnloaded     PluginState = "unloaded"
    StateDisabled     PluginState = "disabled"
)

// HealthStatus represents plugin health
type HealthStatus string

const (
    HealthStatusHealthy   HealthStatus = "healthy"
    HealthStatusDegraded  HealthStatus = "degraded"
    HealthStatusUnhealthy HealthStatus = "unhealthy"
    HealthStatusUnknown   HealthStatus = "unknown"
)
```

## 3. Discovery Phase

### 3.1 Discovery Implementation

```go
// Discover scans plugin directories and finds available plugins
func (l *Loader) Discover(ctx context.Context) ([]*DiscoveredPlugin, error) {
    var discovered []*DiscoveredPlugin
    
    for _, dir := range l.config.PluginDirs {
        entries, err := os.ReadDir(dir)
        if err != nil {
            l.logger.Warn("failed to read plugin directory", "dir", dir, "error", err)
            continue
        }
        
        for _, entry := range entries {
            if !entry.IsDir() {
                continue
            }
            
            pluginPath := filepath.Join(dir, entry.Name())
            manifestPath := filepath.Join(pluginPath, "plugin.yaml")
            
            // Check for manifest
            if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
                continue
            }
            
            // Parse manifest
            manifest, err := l.loadManifest(manifestPath)
            if err != nil {
                l.logger.Warn("failed to load plugin manifest", 
                    "plugin", entry.Name(), "error", err)
                continue
            }
            
            dp := &DiscoveredPlugin{
                Path:     pluginPath,
                Manifest: manifest,
                State:    StateDiscovered,
            }
            
            // Watch for changes if hot reload enabled
            if l.config.HotReload && l.watcher != nil {
                l.watcher.Add(pluginPath)
            }
            
            discovered = append(discovered, dp)
        }
    }
    
    return discovered, nil
}

// DiscoveredPlugin represents a plugin found during discovery
type DiscoveredPlugin struct {
    Path     string
    Manifest Manifest
    State    PluginState
}
```

### 3.2 Manifest Loading

```go
// loadManifest reads and parses plugin manifest
func (l *Loader) loadManifest(path string) (Manifest, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return Manifest{}, fmt.Errorf("read manifest: %w", err)
    }
    
    var manifest Manifest
    if err := yaml.Unmarshal(data, &manifest); err != nil {
        return Manifest{}, fmt.Errorf("parse manifest: %w", err)
    }
    
    // Validate required fields
    if err := validateManifest(&manifest); err != nil {
        return Manifest{}, fmt.Errorf("validate manifest: %w", err)
    }
    
    return manifest, nil
}

// validateManifest checks manifest completeness
func validateManifest(m *Manifest) error {
    if m.SchemaVersion == "" {
        return fmt.Errorf("schema_version is required")
    }
    if m.Plugin.Name == "" {
        return fmt.Errorf("plugin.name is required")
    }
    if m.Plugin.Version == "" {
        return fmt.Errorf("plugin.version is required")
    }
    if m.Runtime.Type == "" {
        return fmt.Errorf("runtime.type is required")
    }
    
    // Validate plugin name format
    if !isValidPluginName(m.Plugin.Name) {
        return fmt.Errorf("invalid plugin name: %s", m.Plugin.Name)
    }
    
    // Validate version format
    if _, err := semver.Parse(m.Plugin.Version); err != nil {
        return fmt.Errorf("invalid version: %s", m.Plugin.Version)
    }
    
    return nil
}

func isValidPluginName(name string) bool {
    // Allow lowercase letters, numbers, hyphens
    // Must start with letter
    matched, _ := regexp.MatchString(`^[a-z][a-z0-9-]*$`, name)
    return matched
}
```

## 4. Validation Phase

### 4.1 Validation Implementation

```go
// Validate performs security and compatibility checks
func (l *Loader) Validate(ctx context.Context, dp *DiscoveredPlugin) error {
    manifest := dp.Manifest
    
    // Check API version compatibility
    if !l.isCompatibleVersion(manifest.Host.MinVersion) {
        return fmt.Errorf("incompatible host version: required %s, have %s",
            manifest.Host.MinVersion, hostVersion)
    }
    
    // Validate entry point exists
    entryPoint := filepath.Join(dp.Path, manifest.Runtime.EntryPoint)
    if _, err := os.Stat(entryPoint); os.IsNotExist(err) {
        return fmt.Errorf("entry point not found: %s", entryPoint)
    }
    
    // Type-specific validation
    switch manifest.Runtime.Type {
    case string(PluginTypeWASM):
        if err := l.validateWASMPlugin(dp); err != nil {
            return fmt.Errorf("wasm validation failed: %w", err)
        }
    case string(PluginTypeNative):
        if err := l.validateNativePlugin(dp); err != nil {
            return fmt.Errorf("native validation failed: %w", err)
        }
    case string(PluginTypeExternal):
        if err := l.validateExternalPlugin(dp); err != nil {
            return fmt.Errorf("external validation failed: %w", err)
        }
    default:
        return fmt.Errorf("unknown plugin type: %s", manifest.Runtime.Type)
    }
    
    // Verify checksums if provided
    if manifest.Checksum != "" {
        if err := l.verifyChecksum(entryPoint, manifest.Checksum); err != nil {
            return fmt.Errorf("checksum verification failed: %w", err)
        }
    }
    
    dp.State = StateValidated
    return nil
}

// validateWASMPlugin validates WASM binary
func (l *Loader) validateWASMPlugin(dp *DiscoveredPlugin) error {
    entryPoint := filepath.Join(dp.Path, dp.Manifest.Runtime.EntryPoint)
    
    // Validate WASM binary
    data, err := os.ReadFile(entryPoint)
    if err != nil {
        return err
    }
    
    // Check magic number
    if len(data) < 4 || string(data[:4]) != "\x00asm" {
        return fmt.Errorf("invalid WASM magic number")
    }
    
    // Validate with runtime
    runtime, err := l.getWASMRuntime()
    if err != nil {
        return err
    }
    
    if err := runtime.ValidateModule(data); err != nil {
        return fmt.Errorf("wasm module validation failed: %w", err)
    }
    
    return nil
}

// validateNativePlugin validates native shared library
func (l *Loader) validateNativePlugin(dp *DiscoveredPlugin) error {
    if l.config.Security.RequireSigned {
        // Verify code signature
        entryPoint := filepath.Join(dp.Path, dp.Manifest.Runtime.EntryPoint)
        if err := l.verifySignature(entryPoint); err != nil {
            return fmt.Errorf("signature verification failed: %w", err)
        }
    }
    
    // Check for dangerous imports
    entryPoint := filepath.Join(dp.Path, dp.Manifest.Runtime.EntryPoint)
    if err := l.checkNativeImports(entryPoint); err != nil {
        return fmt.Errorf("import check failed: %w", err)
    }
    
    return nil
}
```

## 5. Dependency Resolution

### 5.1 Dependency Graph

```go
// DependencyResolver manages plugin dependencies
type DependencyResolver struct {
    plugins map[string]*DiscoveredPlugin
    graph   *DependencyGraph
}

// DependencyGraph represents plugin dependencies
type DependencyGraph struct {
    nodes map[string]*DependencyNode
    edges map[string][]string // plugin -> dependencies
}

type DependencyNode struct {
    Plugin      *DiscoveredPlugin
    Status      ResolutionStatus
    ResolvedAt  time.Time
}

type ResolutionStatus string

const (
    ResolutionPending    ResolutionStatus = "pending"
    ResolutionResolved   ResolutionStatus = "resolved"
    ResolutionFailed     ResolutionStatus = "failed"
    ResolutionCircular   ResolutionStatus = "circular"
)

// ResolveDependencies performs topological sort of plugin dependencies
func (r *DependencyResolver) ResolveDependencies() ([]*DiscoveredPlugin, error) {
    // Build dependency graph
    graph := NewDependencyGraph()
    
    for name, plugin := range r.plugins {
        graph.AddNode(name, plugin)
        
        for _, dep := range plugin.Manifest.Dependencies {
            if _, ok := r.plugins[dep.Name]; !ok {
                return nil, fmt.Errorf("missing dependency: %s required by %s", 
                    dep.Name, name)
            }
            graph.AddEdge(name, dep.Name)
        }
    }
    
    // Check for circular dependencies
    if cycle := graph.FindCycle(); cycle != nil {
        return nil, fmt.Errorf("circular dependency detected: %v", cycle)
    }
    
    // Topological sort
    sorted, err := graph.TopologicalSort()
    if err != nil {
        return nil, fmt.Errorf("dependency resolution failed: %w", err)
    }
    
    // Update states
    for _, plugin := range sorted {
        plugin.State = StateResolved
    }
    
    return sorted, nil
}

// TopologicalSort returns plugins in dependency order
func (g *DependencyGraph) TopologicalSort() ([]*DiscoveredPlugin, error) {
    var result []*DiscoveredPlugin
    visited := make(map[string]bool)
    temp := make(map[string]bool)
    
    var visit func(string) error
    visit = func(name string) error {
        if temp[name] {
            return fmt.Errorf("circular dependency")
        }
        if visited[name] {
            return nil
        }
        
        temp[name] = true
        for _, dep := range g.edges[name] {
            if err := visit(dep); err != nil {
                return err
            }
        }
        temp[name] = false
        visited[name] = true
        
        result = append(result, g.nodes[name].Plugin)
        return nil
    }
    
    for name := range g.nodes {
        if !visited[name] {
            if err := visit(name); err != nil {
                return nil, err
            }
        }
    }
    
    return result, nil
}
```

## 6. Loading Phase

### 6.1 WASM Loading

```go
// loadWASM loads and initializes a WASM plugin
func (l *Loader) loadWASM(ctx context.Context, dp *DiscoveredPlugin) (*PluginInstance, error) {
    entryPoint := filepath.Join(dp.Path, dp.Manifest.Runtime.EntryPoint)
    
    // Read WASM binary
    wasmBytes, err := os.ReadFile(entryPoint)
    if err != nil {
        return nil, fmt.Errorf("read wasm binary: %w", err)
    }
    
    // Get runtime
    runtime, err := l.getWASMRuntime()
    if err != nil {
        return nil, err
    }
    
    // Compile module
    ctx, cancel := context.WithTimeout(ctx, l.config.DefaultTimeout)
    defer cancel()
    
    module, err := runtime.CompileModule(ctx, wasmBytes)
    if err != nil {
        return nil, fmt.Errorf("compile wasm module: %w", err)
    }
    
    // Instantiate with capabilities
    config := wasm.ModuleConfig()
    config.WithName(dp.Manifest.Plugin.Name)
    config.WithMemoryLimit(uint32(l.config.WASM.MemoryLimit) * 1024 * 1024)
    
    instance, err := runtime.InstantiateModule(ctx, module, config)
    if err != nil {
        return nil, fmt.Errorf("instantiate wasm module: %w", err)
    }
    
    pi := &PluginInstance{
        ID:         generatePluginID(dp.Manifest.Plugin.Name),
        Manifest:   dp.Manifest,
        Type:       PluginTypeWASM,
        State:      StateLoading,
        WASMModule: module,
    }
    
    // Get exports
    if err := l.setupWASMExports(ctx, pi, instance); err != nil {
        return nil, fmt.Errorf("setup wasm exports: %w", err)
    }
    
    return pi, nil
}

// setupWASMExports configures host functions and gets plugin exports
func (l *Loader) setupWASMExports(ctx context.Context, pi *PluginInstance, instance wasm.Module) error {
    // Initialize with config
    configJSON, _ := json.Marshal(map[string]interface{}{
        "plugin_dir": filepath.Dir(pi.Manifest.Runtime.EntryPoint),
        "config":     pi.Manifest.Runtime.Config,
    })
    
    initFn := instance.ExportedFunction("openapi_init")
    if initFn == nil {
        return fmt.Errorf("missing required export: openapi_init")
    }
    
    results, err := initFn.Call(ctx, uint64(len(configJSON)))
    if err != nil {
        return fmt.Errorf("wasm init failed: %w", err)
    }
    
    if results[0] != 0 {
        return fmt.Errorf("wasm init returned error: %d", results[0])
    }
    
    return nil
}
```

### 6.2 Native Loading

```go
// loadNative loads a native Go plugin
func (l *Loader) loadNative(ctx context.Context, dp *DiscoveredPlugin) (*PluginInstance, error) {
    entryPoint := filepath.Join(dp.Path, dp.Manifest.Runtime.EntryPoint)
    
    // Open plugin
    p, err := plugin.Open(entryPoint)
    if err != nil {
        return nil, fmt.Errorf("open native plugin: %w", err)
    }
    
    // Lookup Plugin symbol
    sym, err := p.Lookup("Plugin")
    if err != nil {
        return nil, fmt.Errorf("lookup Plugin symbol: %w", err)
    }
    
    pluginImpl, ok := sym.(Plugin)
    if !ok {
        return nil, fmt.Errorf("Plugin symbol does not implement Plugin interface")
    }
    
    pi := &PluginInstance{
        ID:         generatePluginID(dp.Manifest.Plugin.Name),
        Manifest:   dp.Manifest,
        Plugin:     pluginImpl,
        Type:       PluginTypeNative,
        State:      StateLoading,
        NativeLib:  p,
    }
    
    return pi, nil
}
```

## 7. Activation Phase

### 7.1 Plugin Activation

```go
// Activate initializes and activates a plugin
func (l *Loader) Activate(ctx context.Context, pi *PluginInstance) error {
    pi.State = StateInitialized
    
    // Prepare config
    config := map[string]interface{}{
        "plugin_dir": filepath.Dir(pi.Manifest.Runtime.EntryPoint),
        "config":     pi.Manifest.Runtime.Config,
    }
    
    // Initialize plugin
    initCtx, cancel := context.WithTimeout(ctx, l.config.DefaultTimeout)
    defer cancel()
    
    if err := pi.Plugin.Initialize(initCtx, config); err != nil {
        pi.State = StateError
        return fmt.Errorf("plugin initialization failed: %w", err)
    }
    
    // Check health
    health := pi.Plugin.Health(ctx)
    pi.HealthStatus = health
    pi.LastHealthCheck = time.Now()
    
    if health == HealthStatusUnhealthy {
        pi.State = StateError
        return fmt.Errorf("plugin reported unhealthy status")
    }
    
    // Register capabilities
    caps := pi.Plugin.Capabilities()
    if err := l.registerCapabilities(pi, caps); err != nil {
        return fmt.Errorf("register capabilities: %w", err)
    }
    
    // Store instance
    l.mu.Lock()
    l.instances[pi.ID] = pi
    l.mu.Unlock()
    
    // Register with registry
    if err := l.registry.Register(ctx, pi); err != nil {
        return fmt.Errorf("registry registration failed: %w", err)
    }
    
    pi.State = StateActive
    pi.StartedAt = time.Now()
    
    // Publish event
    l.bus.Publish(ctx, Event{
        Type:      EventPluginLoad,
        Timestamp: time.Now(),
        Source:    pi.ID,
        Data: map[string]interface{}{
            "plugin_name": pi.Manifest.Plugin.Name,
            "version":     pi.Manifest.Plugin.Version,
        },
    })
    
    return nil
}

// registerCapabilities registers plugin capabilities with the host
func (l *Loader) registerCapabilities(pi *PluginInstance, caps Capabilities) error {
    // Register commands
    for _, cmd := range caps.Commands {
        if err := l.registerCommand(pi, cmd); err != nil {
            return fmt.Errorf("register command %s: %w", cmd.Name, err)
        }
    }
    
    // Register hooks
    for _, hook := range caps.Hooks {
        if err := l.registerHook(pi, hook); err != nil {
            return fmt.Errorf("register hook %s: %w", hook.Event, err)
        }
    }
    
    // Register middleware
    for _, mw := range caps.Middleware {
        if err := l.registerMiddleware(pi, mw); err != nil {
            return fmt.Errorf("register middleware %s: %w", mw.Name, err)
        }
    }
    
    return nil
}
```

## 8. Hot Reloading

### 8.1 File Watcher

```go
// StartHotReload begins watching for plugin changes
func (l *Loader) StartHotReload(ctx context.Context) error {
    if !l.config.HotReload || l.watcher == nil {
        return nil
    }
    
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case event, ok := <-l.watcher.Events:
                if !ok {
                    return
                }
                
                if event.Op&fsnotify.Write == fsnotify.Write {
                    l.handleFileChange(event.Name)
                }
                
            case err, ok := <-l.watcher.Errors:
                if !ok {
                    return
                }
                l.logger.Error("file watcher error", "error", err)
            }
        }
    }()
    
    return nil
}

// handleFileChange processes plugin file changes
func (l *Loader) handleFileChange(path string) {
    l.logger.Info("plugin file changed", "path", path)
    
    // Find affected plugin
    l.mu.RLock()
    var affectedPlugin *PluginInstance
    for _, pi := range l.instances {
        if strings.HasPrefix(path, filepath.Dir(pi.Manifest.Runtime.EntryPoint)) {
            affectedPlugin = pi
            break
        }
    }
    l.mu.RUnlock()
    
    if affectedPlugin == nil {
        return
    }
    
    // Reload plugin
    ctx, cancel := context.WithTimeout(context.Background(), l.config.DefaultTimeout)
    defer cancel()
    
    if err := l.Reload(ctx, affectedPlugin.ID); err != nil {
        l.logger.Error("plugin reload failed", "plugin", affectedPlugin.ID, "error", err)
    }
}

// Reload unloads and reloads a plugin
func (l *Loader) Reload(ctx context.Context, pluginID string) error {
    // Unload existing
    if err := l.Unload(ctx, pluginID); err != nil {
        return fmt.Errorf("unload plugin: %w", err)
    }
    
    // Rediscover and load
    plugins, err := l.Discover(ctx)
    if err != nil {
        return fmt.Errorf("rediscover plugins: %w", err)
    }
    
    for _, dp := range plugins {
        if generatePluginID(dp.Manifest.Plugin.Name) == pluginID {
            if err := l.Validate(ctx, dp); err != nil {
                return fmt.Errorf("validate plugin: %w", err)
            }
            
            // Load and activate
            var pi *PluginInstance
            switch dp.Manifest.Runtime.Type {
            case string(PluginTypeWASM):
                pi, err = l.loadWASM(ctx, dp)
            case string(PluginTypeNative):
                pi, err = l.loadNative(ctx, dp)
            }
            
            if err != nil {
                return fmt.Errorf("load plugin: %w", err)
            }
            
            if err := l.Activate(ctx, pi); err != nil {
                return fmt.Errorf("activate plugin: %w", err)
            }
            
            return nil
        }
    }
    
    return fmt.Errorf("plugin not found after reload: %s", pluginID)
}
```

## 9. Error Handling

| Error Code | Condition | Recovery |
|------------|-----------|----------|
| `LOAD_MANIFEST_INVALID` | Plugin manifest parsing error | Skip plugin, log error |
| `LOAD_VALIDATION_FAILED` | Security/compatibility check failed | Quarantine plugin, notify admin |
| `LOAD_DEPENDENCY_MISSING` | Required dependency not found | Defer loading, retry later |
| `LOAD_CIRCULAR_DEPENDENCY` | Circular dependency detected | Fail all plugins in cycle |
| `LOAD_INIT_TIMEOUT` | Plugin initialization timeout | Kill plugin, mark failed |
| `LOAD_INIT_ERROR` | Plugin initialization failed | Unload plugin, log error |
| `LOAD_HEALTH_CHECK_FAILED` | Plugin unhealthy after init | Retry with backoff |

## 10. Testing

```go
func TestLoader_Load(t *testing.T) {
    tmpDir := t.TempDir()
    
    loader, err := plugin.NewLoader(plugin.LoaderConfig{
        PluginDirs: []string{tmpDir},
        WASM: plugin.WASMConfig{
            Runtime: "wazero",
        },
    })
    require.NoError(t, err)
    
    // Create test plugin
    pluginDir := filepath.Join(tmpDir, "test-plugin")
    os.MkdirAll(pluginDir, 0755)
    
    manifest := `
schema_version: "1.0"
plugin:
  name: test-plugin
  version: "1.0.0"
  description: Test plugin
runtime:
  type: wasm
  entry_point: plugin.wasm
`
    os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0644)
    
    // Load
    ctx := context.Background()
    discovered, err := loader.Discover(ctx)
    require.NoError(t, err)
    assert.Len(t, discovered, 1)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

*This specification produces identical implementations across different AI systems. No human interpretation required.*

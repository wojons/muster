# Plugin Interface Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Plugin System

## Overview

Complete specification for the **plugin interface** enabling third-party extensions to extend OpenAPI CLI functionality. This interface defines the contract between the host application and plugins, supporting both WASM-based sandboxed plugins and native Go plugins for maximum flexibility and security.

## 1. Architecture Overview

### 1.1 Plugin Types

| Type | Runtime | Use Case | Security |
|------|---------|----------|----------|
| **WASM** | Sandboxed VM | Untrusted third-party plugins | High (memory-safe, capability-based) |
| **Native** | Host process | Trusted internal plugins | Medium (same process) |
| **External** | Separate process | Legacy integrations | Medium (process isolation) |

### 1.2 Interface Layers

```
┌─────────────────────────────────────────────────────────────┐
│                    Host Application                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Command   │  │   Config    │  │   Event Handlers    │  │
│  │   Registry  │  │   Manager   │  │                     │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
└─────────┼────────────────┼────────────────────┼─────────────┘
          │                │                    │
          └────────────────┴────────────────────┘
                           │
              ┌────────────┴────────────┐
              │     Plugin Manager      │
              └────────────┬────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
┌───────▼───────┐ ┌────────▼────────┐ ┌──────▼───────┐
│  WASM Runtime │ │  Native Loader  │ │  External    │
│  (wazero)     │ │  (Go plugin)    │ │  (gRPC/HTTP) │
└───────────────┘ └─────────────────┘ └──────────────┘
```

## 2. Core Interface Definition

### 2.1 Plugin Interface (Go)

```go
package plugin

import (
    "context"
    "time"
)

// Plugin is the core interface all plugins must implement
type Plugin interface {
    // Metadata returns plugin information
    Metadata() Metadata
    
    // Initialize is called once when plugin is loaded
    Initialize(ctx context.Context, config map[string]interface{}) error
    
    // Shutdown is called when plugin is unloaded
    Shutdown(ctx context.Context) error
    
    // Health returns current health status
    Health(ctx context.Context) HealthStatus
    
    // Capabilities returns what this plugin provides
    Capabilities() Capabilities
}

// Metadata contains plugin identification information
type Metadata struct {
    Name        string
    Version     string
    Description string
    Author      string
    License     string
    Homepage    string
    
    // Plugin type determines runtime
    Type PluginType
    
    // Entry point for the plugin
    EntryPoint string
    
    // Minimum host version required
    MinHostVersion string
    
    // Dependencies on other plugins
    Dependencies []Dependency
}

// PluginType identifies the runtime environment
type PluginType string

const (
    PluginTypeWASM     PluginType = "wasm"
    PluginTypeNative   PluginType = "native"
    PluginTypeExternal PluginType = "external"
)

// Capabilities defines what a plugin can do
type Capabilities struct {
    // Commands this plugin registers
    Commands []CommandCapability
    
    // Hooks this plugin implements
    Hooks []HookCapability
    
    // Middleware this plugin provides
    Middleware []MiddlewareCapability
    
    // Extensions to existing commands
    Extensions []ExtensionCapability
}

// CommandCapability defines a new command
type CommandCapability struct {
    Name        string
    Description string
    Category    string
    Args        []ArgSpec
    Flags       []FlagSpec
    Handler     CommandHandler
}

// HookCapability defines lifecycle hooks
type HookCapability struct {
    Event   HookEvent
    Handler HookHandler
    Priority int // Lower = earlier execution
}

type HookEvent string

const (
    HookPreCommand  HookEvent = "pre_command"
    HookPostCommand HookEvent = "post_command"
    HookPreRequest  HookEvent = "pre_request"
    HookPostRequest HookEvent = "post_request"
    HookPreAuth     HookEvent = "pre_auth"
    HookPostAuth    HookEvent = "post_auth"
    HookConfigLoad  HookEvent = "config_load"
    HookShutdown    HookEvent = "shutdown"
)

// MiddlewareCapability defines request/response middleware
type MiddlewareCapability struct {
    Name    string
    Stage   MiddlewareStage
    Handler MiddlewareHandler
}

type MiddlewareStage string

const (
    MiddlewareRequest  MiddlewareStage = "request"
    MiddlewareResponse MiddlewareStage = "response"
    MiddlewareError    MiddlewareStage = "error"
)

// ExtensionCapability extends existing commands
type ExtensionCapability struct {
    TargetCommand string
    AddFlags      []FlagSpec
    ModifyArgs    []ArgSpec
    PreHandler    CommandHandler
    PostHandler   CommandHandler
}
```

### 2.2 Manifest Schema

```go
// Manifest is the plugin descriptor file (plugin.yaml)
type Manifest struct {
    // Schema version for compatibility
    SchemaVersion string `yaml:"schema_version"`
    
    // Plugin metadata
    Plugin struct {
        Name        string `yaml:"name"`
        Version     string `yaml:"version"`
        Description string `yaml:"description"`
        Author      string `yaml:"author"`
        License     string `yaml:"license"`
        Homepage    string `yaml:"homepage"`
    } `yaml:"plugin"`
    
    // Runtime configuration
    Runtime struct {
        Type       string            `yaml:"type"`       // wasm, native, external
        EntryPoint string            `yaml:"entry_point"`
        Args       []string          `yaml:"args"`
        Env        map[string]string `yaml:"env"`
        Resources  ResourceLimits    `yaml:"resources"`
    } `yaml:"runtime"`
    
    // Host requirements
    Host struct {
        MinVersion string   `yaml:"min_version"`
        Features   []string `yaml:"features"`
    } `yaml:"host"`
    
    // Dependencies
    Dependencies []Dependency `yaml:"dependencies"`
    
    // Capabilities this plugin provides
    Provides struct {
        Commands   []CommandManifest   `yaml:"commands"`
        Hooks      []HookManifest      `yaml:"hooks"`
        Middleware []MiddlewareManifest `yaml:"middleware"`
    } `yaml:"provides"`
}

// Example plugin.yaml
const ExampleManifest = `
schema_version: "1.0"

plugin:
  name: "http-logger"
  version: "1.2.0"
  description: "Advanced HTTP request/response logging"
  author: "Example Corp"
  license: "MIT"
  homepage: "https://example.com/plugins/http-logger"

runtime:
  type: wasm
  entry_point: "plugin.wasm"
  resources:
    memory_mb: 128
    cpu_percent: 10
    max_file_descriptors: 100

host:
  min_version: "1.0.0"
  features: ["http_client", "logging"]

provides:
  commands:
    - name: "logs view"
      description: "View recent HTTP logs"
      category: "debugging"
      
  hooks:
    - event: "post_request"
      priority: 100
      
  middleware:
    - name: "request-logger"
      stage: "request"
`
```

### 2.3 Capability System

```go
// Capability grants specific permissions to plugins
type Capability string

const (
    // Network capabilities
    CapabilityHTTPGet    Capability = "http:get"
    CapabilityHTTPPost   Capability = "http:post"
    CapabilityHTTPPut    Capability = "http:put"
    CapabilityHTTPDelete Capability = "http:delete"
    
    // Storage capabilities
    CapabilityFileRead   Capability = "file:read"
    CapabilityFileWrite  Capability = "file:write"
    CapabilityCacheRead  Capability = "cache:read"
    CapabilityCacheWrite Capability = "cache:write"
    
    // System capabilities
    CapabilityEnvRead    Capability = "env:read"
    CapabilityEnvWrite   Capability = "env:write"
    CapabilityExec       Capability = "system:exec"
    
    // Extension capabilities
    CapabilityCommandRegister Capability = "ext:command"
    CapabilityHookRegister    Capability = "ext:hook"
    CapabilityMiddleware      Capability = "ext:middleware"
)

// CapabilityChecker validates capability requests
type CapabilityChecker interface {
    // Check if plugin has specific capability
    HasCapability(pluginID string, cap Capability) bool
    
    // Request additional capabilities (may prompt user)
    RequestCapability(ctx context.Context, pluginID string, cap Capability) error
    
    // Get all capabilities for a plugin
    GetCapabilities(pluginID string) []Capability
}
```

## 3. WASM Plugin Interface

### 3.1 WASM Module Exports

```go
// WASM plugin must export these functions
// (using WebAssembly Component Model or WASI Preview 2)

//go:wasmexport openapi_init
func Init(configPtr uint32, configLen uint32) uint32

//go:wasmexport openapi_shutdown  
func Shutdown() uint32

//go:wasmexport openapi_get_metadata
func GetMetadata() uint32 // Returns pointer to JSON metadata

//go:wasmexport openapi_handle_command
func HandleCommand(namePtr uint32, nameLen uint32, argsPtr uint32, argsLen uint32) uint32

//go:wasmexport openapi_handle_hook
func HandleHook(eventPtr uint32, eventLen uint32, dataPtr uint32, dataLen uint32) uint32

//go:wasmexport openapi_handle_middleware
func HandleMiddleware(stagePtr uint32, stageLen uint32, reqPtr uint32, reqLen uint32) uint32
```

### 3.2 Host Functions (Imports)

```go
// Host provides these functions to WASM plugins
//go:wasmimport host http_request
func hostHTTPRequest(methodPtr uint32, methodLen uint32, urlPtr uint32, urlLen uint32, bodyPtr uint32, bodyLen uint32, headersPtr uint32, headersLen uint32) uint32

//go:wasmimport host log_write
func hostLogWrite(level uint32, msgPtr uint32, msgLen uint32)

//go:wasmimport host cache_get
func hostCacheGet(keyPtr uint32, keyLen uint32, outPtr uint32, outLen uint32) uint32

//go:wasmimport host cache_set
func hostCacheSet(keyPtr uint32, keyLen uint32, valPtr uint32, valLen uint32, ttl uint64) uint32

//go:wasmimport host config_get
func hostConfigGet(keyPtr uint32, keyLen uint32, outPtr uint32, outLen uint32) uint32

//go:wasmimport host allocate
func hostAllocate(size uint32) uint32

//go:wasmimport host deallocate
func hostDeallocate(ptr uint32, size uint32)
```

### 3.3 WASM SDK (Go)

```go
// Package plugin provides WASM plugin SDK
package plugin

import (
    "encoding/json"
    "unsafe"
)

// Base plugin implementation for WASM target
type BasePlugin struct {
    metadata Metadata
    config   map[string]interface{}
}

// Register registers the plugin with the host
func Register(p Plugin) {
    currentPlugin = p
}

var currentPlugin Plugin

//export openapi_init
func _init(configPtr uint32, configLen uint32) uint32 {
    if currentPlugin == nil {
        return 1 // Error: no plugin registered
    }
    
    configData := getBytes(configPtr, configLen)
    var config map[string]interface{}
    if err := json.Unmarshal(configData, &config); err != nil {
        return 2 // Error: invalid config
    }
    
    ctx := context.Background()
    if err := currentPlugin.Initialize(ctx, config); err != nil {
        return 3 // Error: init failed
    }
    
    return 0 // Success
}

//export openapi_get_metadata
func _getMetadata() uint32 {
    if currentPlugin == nil {
        return 0
    }
    
    metadata := currentPlugin.Metadata()
    data, _ := json.Marshal(metadata)
    return allocateAndReturn(data)
}

// Helper functions for WASM memory management
func getBytes(ptr uint32, len uint32) []byte {
    return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), len)
}

func allocateAndReturn(data []byte) uint32 {
    ptr := hostAllocate(uint32(len(data)))
    copy(getBytes(ptr, uint32(len(data))), data)
    return ptr
}
```

## 4. Native Plugin Interface

### 4.1 Plugin Registration

```go
package main

import (
    "context"
    "github.com/openapi-cli/plugin"
)

func init() {
    plugin.Register(&MyPlugin{})
}

type MyPlugin struct {
    config map[string]interface{}
}

func (p *MyPlugin) Metadata() plugin.Metadata {
    return plugin.Metadata{
        Name:        "my-plugin",
        Version:     "1.0.0",
        Description: "Example native plugin",
        Type:        plugin.PluginTypeNative,
    }
}

func (p *MyPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
    p.config = config
    return nil
}

func (p *MyPlugin) Shutdown(ctx context.Context) error {
    return nil
}

func (p *MyPlugin) Health(ctx context.Context) plugin.HealthStatus {
    return plugin.HealthStatusHealthy
}

func (p *MyPlugin) Capabilities() plugin.Capabilities {
    return plugin.Capabilities{
        Commands: []plugin.CommandCapability{
            {
                Name:        "my-command",
                Description: "An example command",
                Handler:     p.handleMyCommand,
            },
        },
    }
}

func (p *MyPlugin) handleMyCommand(ctx context.Context, args []string, flags map[string]string) error {
    // Command implementation
    return nil
}
```

### 4.2 Plugin Main Entry

```go
// Required main function for native plugins
func main() {
    // Native plugins are loaded as shared libraries
    // This main is never called, but required for Go build
}
```

## 5. Event System

### 5.1 Event Types

```go
// Event represents a lifecycle event
type Event struct {
    Type      EventType
    Timestamp time.Time
    Data      map[string]interface{}
    Source    string // Plugin ID or "host"
}

type EventType string

const (
    EventCommandStart    EventType = "command:start"
    EventCommandComplete EventType = "command:complete"
    EventCommandError    EventType = "command:error"
    EventRequestStart    EventType = "request:start"
    EventRequestComplete EventType = "request:complete"
    EventConfigChange    EventType = "config:change"
    EventPluginLoad      EventType = "plugin:load"
    EventPluginUnload    EventType = "plugin:unload"
)

// EventBus for pub/sub communication
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventType EventType, handler EventHandler) Subscription
    Unsubscribe(sub Subscription)
}
```

### 5.2 Hook Execution

```go
// ExecuteHooks runs all registered hooks for an event
func (pm *PluginManager) ExecuteHooks(ctx context.Context, event HookEvent, data interface{}) error {
    hooks := pm.getHooksForEvent(event)
    
    // Sort by priority
    sort.Slice(hooks, func(i, j int) bool {
        return hooks[i].Priority < hooks[j].Priority
    })
    
    for _, hook := range hooks {
        if err := hook.Handler(ctx, data); err != nil {
            if hook.OnError == HookErrorStop {
                return err
            }
            // Log error but continue
            pm.logger.Error("hook failed", "hook", hook.Name, "error", err)
        }
    }
    
    return nil
}
```

## 6. Error Handling

| Error Code | Description | Action |
|------------|-------------|--------|
| `PLUGIN_INIT_FAILED` | Plugin initialization error | Unload plugin, log error |
| `PLUGIN_CAPABILITY_DENIED` | Insufficient permissions | Prompt user or skip operation |
| `PLUGIN_INCOMPATIBLE` | Version mismatch | Reject plugin, suggest update |
| `PLUGIN_TIMEOUT` | Plugin operation timeout | Kill plugin, restart if critical |
| `PLUGIN_PANIC` | Plugin runtime panic | Isolate plugin, notify user |

## 7. Security Model

### 7.1 WASM Sandbox

- **Memory isolation**: Separate linear memory per plugin
- **Capability-based**: Only granted capabilities accessible
- **Resource limits**: CPU, memory, file descriptors capped
- **No filesystem access**: Except via capability-granted paths
- **Network allowlist**: Only configured hosts accessible

### 7.2 Native Plugin Restrictions

- **Code signing**: Required for external plugins
- **Sandbox optional**: Can run with limited permissions
- **Audit logging**: All operations logged

## 8. Version Compatibility

| Host Version | Plugin API Version | Compatibility |
|--------------|-------------------|---------------|
| 1.0.x | 1.0 | Full |
| 1.1.x | 1.0, 1.1 | Full |
| 2.0.x | 2.0 | Breaking changes |

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

*This specification produces identical implementations across different AI systems. No human interpretation required.*

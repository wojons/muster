# MCP Capability Negotiation Specification

MCP capability exchange between client and server during initialization.

## Overview

Capability negotiation occurs during the `initialize` request. Both parties declare their supported features, enabling optional functionality only when both support it.

## Protocol Flow

```
Client                                         Server
  |                                              |
  |--- initialize { capabilities, clientInfo } -->|
  |                                              |
  |<-- { capabilities, serverInfo, protocol } ---|
  |                                              |
  |--- notifications/initialized --------------->|
  |                                              |
```

## Client Capabilities

```go
// ClientCapabilities represents client-supported features
type ClientCapabilities struct {
    Roots    *RootsCapability    `json:"roots,omitempty"`
    Sampling *SamplingCapability `json:"sampling,omitempty"`
    Experimental map[string]interface{} `json:"experimental,omitempty"`
}

type RootsCapability struct {
    ListChanged bool `json:"listChanged,omitempty"`
}

type SamplingCapability struct{}
```

## Server Capabilities

```go
// ServerCapabilities represents server-supported features
type ServerCapabilities struct {
    Logging    *LoggingCapability    `json:"logging,omitempty"`
    Prompts    *PromptsCapability    `json:"prompts,omitempty"`
    Resources  *ResourcesCapability  `json:"resources,omitempty"`
    Tools      *ToolsCapability      `json:"tools,omitempty"`
    Experimental map[string]interface{} `json:"experimental,omitempty"`
}

type LoggingCapability struct{}

type PromptsCapability struct {
    ListChanged bool `json:"listChanged,omitempty"`
}

type ResourcesCapability struct {
    Subscribe   bool `json:"subscribe,omitempty"`
    ListChanged bool `json:"listChanged,omitempty"`
}

type ToolsCapability struct {
    ListChanged bool `json:"listChanged,omitempty"`
}
```

## Negotiation Protocol

```go
// Negotiator handles capability negotiation
type Negotiator struct {
    clientCaps ClientCapabilities
    serverCaps ServerCapabilities
    negotiated NegotiatedCapabilities
}

type NegotiatedCapabilities struct {
    RootsSupported    bool
    SamplingSupported bool
    PromptsSupported  bool
    ResourcesSupported bool
    ToolsSupported    bool
    LoggingSupported  bool
}

func (n *Negotiator) Negotiate() NegotiatedCapabilities {
    return NegotiatedCapabilities{
        RootsSupported:     n.clientCaps.Roots != nil,
        SamplingSupported:  n.clientCaps.Sampling != nil,
        PromptsSupported:   n.serverCaps.Prompts != nil,
        ResourcesSupported: n.serverCaps.Resources != nil,
        ToolsSupported:     n.serverCaps.Tools != nil,
        LoggingSupported:   n.serverCaps.Logging != nil,
    }
}
```

## Complete Implementation

```go
// InitializeRequest from client
type InitializeRequest struct {
    ProtocolVersion string             `json:"protocolVersion"`
    Capabilities    ClientCapabilities `json:"capabilities"`
    ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeResult from server
type InitializeResult struct {
    ProtocolVersion string             `json:"protocolVersion"`
    Capabilities    ServerCapabilities `json:"capabilities"`
    ServerInfo      Implementation     `json:"serverInfo"`
    Instructions    string             `json:"instructions,omitempty"`
}

func (s *Server) handleInitialize(req Request) {
    var params InitializeRequest
    if err := json.Unmarshal(req.Params, &params); err != nil {
        s.sendError(req.ID, InvalidParamsCode, "Invalid params", nil)
        return
    }

    // Version check
    if params.ProtocolVersion != ProtocolVersion {
        s.sendError(req.ID, UnsupportedProtocolVersionCode,
            fmt.Sprintf("Unsupported protocol version: %s", params.ProtocolVersion), nil)
        return
    }

    // Store client capabilities
    s.clientCapabilities = &params.Capabilities
    
    // Negotiate capabilities
    negotiator := &Negotiator{
        clientCaps: params.Capabilities,
        serverCaps: *s.serverCapabilities,
    }
    s.negotiated = negotiator.Negotiate()

    // Send response with server capabilities
    result := InitializeResult{
        ProtocolVersion: ProtocolVersion,
        Capabilities:    *s.serverCapabilities,
        ServerInfo:      s.implementation,
        Instructions:    s.instructions,
    }
    
    s.sendResult(req.ID, result)
    s.initialized = true
}
```

## Key Points

- Capabilities are opt-in; omitting a capability means "not supported"
- Server must not use features client doesn't support
- Protocol version must match exactly
- Experimental capabilities allow vendor extensions

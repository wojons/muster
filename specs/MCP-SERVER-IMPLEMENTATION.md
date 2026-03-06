# MCP Server Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: MCP Server

## Overview

Complete specification of the **MCP server implementation**, managing stdio communication, request routing, and lifecycle.

## Architecture

```mermaid
graph TB
    subgraph Server["MCPServer"]
        IO["I/O Layer"]
        STDIN["stdin<br/>(JSON-RPC Lines)"]
        STDOUT["stdout<br/>(JSON-RPC Responses)"]
        STDERR["stderr<br/>(Logs)"]
        
        Handlers["Handler Registry"]
        Init["InitializeHandler"]
        InitDone["InitializedHandler"]
        ListTools["ListToolsHandler"]
        CallTool["CallToolHandler"]
        ListRes["ListResourcesHandler"]
        ReadRes["ReadResourceHandler"]
        ListPrompts["ListPromptsHandler"]
        GetPrompt["GetPromptHandler"]
        
        State["State"]
        CmdRegistry["Command Registry"]
        Executor["Command Executor"]
        Shutdown["Shutdown Channel"]
        
        STDIN -->|"Scanner<br/>Line by Line"| IO
        IO -->|"Parse JSON-RPC"| Handlers
        STDOUT <---|"Write Responses"| IO
        STDERR <---|"Logging"| IO
        Handlers -->|"Registered"| Init
        Handlers -->|"Registered"| InitDone
        Handlers -->|"Registered"| ListTools
        Handlers -->|"Registered"| CallTool
        Handlers -->|"Registered"| ListRes
        Handlers -->|"Registered"| ReadRes
        Handlers -->|"Registered"| ListPrompts
        Handlers -->|"Registered"| GetPrompt
        ListTools -->|"Registry"| CmdRegistry
        CallTool -->|"Execute"| Executor
        State -->|"Control"| Shutdown
        Shutdown -->|"Trigger"| IO
    end
    
    External["External System"] -.->|"stdin/stdout I/O"| Server
```

## Request Processing Flow

```mermaid
flowchart TD
    START([Start Run]) --> SCANNER[Create Scanner]
    SCANNER --> SCAN{Scanner Scan}
    SCAN -->|Line| CHECK_EMPTY{Empty Line?}
    CHECK_EMPTY -->|Yes| RESTART[Continue Loop]
    CHECK_EMPTY -->|No| PARSE[Parse JSON-RPC Request]
    
    PARSE -->{Parse Error?}
    {Parse Error?} -->|Yes| ERROR_MSG[Send Error -32700<br/>Parse Error]
    ERROR_MSG --> RESTART
    
    {Parse Error?} -->|No| CHECK_CTX{Context Done?}
    CHECK_CTX -->|Yes| ERROR_CTX[Return Context Error]
    CHECK_CTX -->|No| CHECK_SHUTDOWN{Shutdown Signal?}
    
    CHECK_SHUTDOWN -->|Yes| RETURN_DONE[Return nil]
    CHECK_SHUTDOWN -->|No| HANDLE[handleRequest Call]
    
    HANDLE --> HANDLE_REQ[handlers.Handle Method]
    HANDLE_REQ -->{Handler Error?}
    {Handler Error?} -->|Yes| SEND_ERROR[sendError]
    {Handler Error?} -->|No| SEND_RESULT[sendResponse]
    
    SEND_ERROR --> WRITE[writeResponse]
    SEND_RESULT --> WRITE
    WRITE --> STDOUT[Write to stdout]
    STDOUT --> LINEBREAK[Add Line Break]
    LINEBREAK --> RESTART
    
    RESTART --> SCAN
    
    ERROR_CTX -.-> END([End])
    RETURN_DONE -.-> END
    
    style {Parse Error?} fill:#ff9,stroke:#333
    style {Context Done?} fill:#ff9,stroke:#333
    style {Shutdown Signal?} fill:#ff9,stroke:#333
    style {Handler Error?} fill:#ff9,stroke:#333
    style START fill:#9f9,stroke:#333
    style END fill:#f99,stroke:#333
```

## Handler Registration Flow

```mermaid
flowchart LR
    START([registerHandlers]) --> REGISTRY[Get Handler Registry]
    REGISTRY --> REG_INIT[Register InitializeHandler]
    REG_INIT --> REG_INIT_DONE[Register InitializedHandler]
    REG_INIT_DONE --> REG_LIST_TOOLS[Register ListToolsHandler]
    REG_LIST_TOOLS --> REG_CALL_TOOL[Register CallToolHandler]
    REG_CALL_TOOL --> REG_LIST_RES[Register ListResourcesHandler]
    REG_LIST_RES --> REG_READ_RES[Register ReadResourceHandler]
    REG_READ_RES --> REG_LIST_PROMPTS[Register ListPromptsHandler]
    REG_LIST_PROMPTS --> REG_GET_PROMPT[Register GetPromptHandler]
    REG_GET_PROMPT --> DONE([Complete])
    
    REG_LIST_TOOLS -.->|References| CMD_REG[Command Registry]
    REG_CALL_TOOL -.->|References| EXECUTOR[Command Executor]
    
    REG_INIT -.-> HANDLERS[Handler Registry]
    REG_INIT_DONE -.-> HANDLERS
    REG_LIST_TOOLS -.-> HANDLERS
    REG_CALL_TOOL -.-> HANDLERS
    REG_LIST_RES -.-> HANDLERS
    REG_READ_RES -.-> HANDLERS
    REG_LIST_PROMPTS -.-> HANDLERS
    REG_GET_PROMPT -.-> HANDLERS
```

## Response Flow

```mermaid
sequenceDiagram
    participant Client as Claude Desktop
    participant Srv as MCPServer
    participant Handler as Handler
    participant Registry as Handler Registry
    participant Writer as stdout
    
    Client->>Srv: Send JSON-RPC Request Line
    Srv->>Srv: Scanner Scan Line
    Srv->>Srv: Parse JSON-RPC
    
    alt Parse Error
        Srv->>Writer: Send Error -32700 "Parse Error"
        Writer->>Client: JSON-RPC Error Response
        Client->>Client: Handle Error
    else Valid Request
        Srv->>Registry: Handle Method
        Registry->>Handler: Find Handler
        Handler->>Handler: Execute Request
        
        alt Handler Error
            Handler-->>Registry: Return Error
            Registry-->>Srv: Error
            Srv->>Writer: sendError ID Code Message
            Writer->>Client: JSON-RPC Error Response
        else Handler Success
            Handler-->>Registry: Return Result
            Registry-->>Srv: Result
            Srv->>Writer: sendResponse ID Result
            Writer->>Client: JSON-RPC Response
        end
    end
    
    Note over Writer: Response marshaled to JSON
    Note over Writer: Line break added for stdio
```

## Lifecycle Flow

```mermaid
stateDiagram-v2
    [*] --> New: NewMCPServer
    New --> Registering: registerHandlers
    Registering --> Ready: All Handlers Registered
    Ready --> Running: Run Context
    Running --> Processing: Receive JSON-RPC Line
    
    Processing --> Parse: Read Line
    Parse --> Valid: Parsed Successfully
    Parse --> Error: Parse Error
    
    Valid --> Handle: Find Handler
    Handle --> Responding: Send Response
    Handle --> ErrorHandle: Handler Error
    
    Error --> Running: Continue Loop
    ErrorHandle --> Running: Continue Loop
    Responding --> Running: Continue Loop
    
    Running --> Shutdown: Context Done / Signal
    Shutdown --> Stopped: Close Shutdown Channel
    Stopped --> [*]: Cleanup Complete
    
    note right of Ready: Ready to accept JSON-RPC
    note right of Processing: Main request loop
    note right of Shutdown: Graceful shutdown
```

## 1. Server Structure

```go
type MCPServer struct {
    // I/O
    stdin  io.Reader
    stdout io.Writer
    stderr io.Writer
    
    // Handlers
    handlers *HandlerRegistry
    
    // State
    initialized   bool
    shutdownChan  chan struct{}
    
    // Components
    commandRegistry *CommandRegistry
    executor        *CommandExecutor
}

func NewMCPServer(stdin io.Reader, stdout, stderr io.Writer) *MCPServer {
    s := &MCPServer{
        stdin:      stdin,
        stdout:     stdout,
        stderr:     stderr,
        handlers:   NewHandlerRegistry(),
        shutdownChan: make(chan struct{}),
    }
    
    s.registerHandlers()
    return s
}
```

## 2. Request Processing

```go
func (s *MCPServer) Run(ctx context.Context) error {
    scanner := bufio.NewScanner(s.stdin)
    
    for scanner.Scan() {
        select {
        case <-s.shutdownChan:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        
        line := scanner.Text()
        if line == "" {
            continue
        }
        
        var req JSONRPCRequest
        if err := json.Unmarshal([]byte(line), &req); err != nil {
            s.sendError(nil, -32700, "Parse error")
            continue
        }
        
        go s.handleRequest(ctx, &req)
    }
    
    return scanner.Err()
}

func (s *MCPServer) handleRequest(ctx context.Context, req *JSONRPCRequest) {
    result, err := s.handlers.Handle(ctx, req.Method, req.Params)
    
    if err != nil {
        s.sendError(req.ID, err)
    } else {
        s.sendResponse(req.ID, result)
    }
}
```

## 3. Response Handling

```go
func (s *MCPServer) sendResponse(id interface{}, result interface{}) {
    resp := JSONRPCResponse{
        JSONRPC: "2.0",
        ID:      id,
        Result:  mustMarshal(result),
    }
    
    s.writeResponse(resp)
}

func (s *MCPServer) sendError(id interface{}, code int, message string) {
    resp := JSONRPCResponse{
        JSONRPC: "2.0",
        ID:      id,
        Error: &JSONRPCError{
            Code:    code,
            Message: message,
        },
    }
    
    s.writeResponse(resp)
}

func (s *MCPServer) writeResponse(resp JSONRPCResponse) {
    data, _ := json.Marshal(resp)
    fmt.Fprintln(s.stdout, string(data))
}
```

## 4. Initialization Flow

```go
func (s *MCPServer) registerHandlers() {
    // Lifecycle
    s.handlers.Register(&InitializeHandler{server: s})
    s.handlers.Register(&InitializedHandler{server: s})
    
    // Tools
    s.handlers.Register(&ListToolsHandler{registry: s.commandRegistry})
    s.handlers.Register(&CallToolHandler{executor: s.executor})
    
    // Resources
    s.handlers.Register(&ListResourcesHandler{})
    s.handlers.Register(&ReadResourceHandler{})
    
    // Prompts
    s.handlers.Register(&ListPromptsHandler{})
    s.handlers.Register(&GetPromptHandler{})
}
```

## 5. Shutdown

```go
func (s *MCPServer) Shutdown() error {
    close(s.shutdownChan)
    return nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

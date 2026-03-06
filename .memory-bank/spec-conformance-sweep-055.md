# Spec Conformance Sweep #055 - MCP-MARKET-RESOURCES.md

**Date**: 2026-03-05
**Spec**: specs/MCP-MARKET-RESOURCES.md
**Conformance**: 30% (3/10 requirements met)
**Status**: ⚠️ MAJOR GAP - Resource templates and subscription system NOT implemented

---

## Spec Overview

**Purpose**: MCP (Model Context Protocol) resources support for exposing data and documents to LLMs
**Lines**: 234 lines
**Key Features**:
- Resource types (Resource, ResourceContent) ✅ Implemented
- Resource templates (ResourceTemplate, URITemplate) ❌ NOT implemented
- Resource providers (ResourceProvider interface) ⚠️ Partially implemented
- Resource manager (ResourceManager) ❌ NOT implemented
- Resource subscription/notification system ❌ NOT implemented
- Server integration (handlers) ✅ Implemented

---

## Conformance Analysis

### ✅ Requirements Met (3/10)

**1. Resource Type Structures** (80% implemented)
- Evidence: pkg/mcp/handlers/resources.go
- Spec requirement: Resource and ResourceContent structs
- Implementation:
  ✅ Resource struct: URI, Name, Description, MimeType ✅
  ⚠️ ResourceContent: URI, MimeType, Text ✅
  ❌ ResourceContent: Blob field ❌ MISSING
  ❌ Resource: Size field ❌ MISSING
  ❌ Resource: Annotations field ❌ MISSING
- Working: ✅ YES - Partial structure present, missing Blob/Size/Annotations fields
- Gap: MINOR (core structure present, some optional fields missing)

**2. Resource Provider Interface** (70% implemented)
- Evidence: pkg/mcp/handlers/resources.go:14-17
- Spec requirement: ResourceProvider interface with ListResources, ReadResource, ListTemplates
- Implementation:
  ✅ GetResource(uri string) (*StoredResource, error)
  ✅ ListResources() ([]ResourceInfo, error)
  ❌ ListTemplates() ([]ResourceTemplate, error) ❌ MISSING
- Working: ✅ YES - Basic provider interface exists
- Gap: MEDIUM (ListTemplates method missing)

**3. Server Integration (Handlers)** (100% implemented)
- Evidence: pkg/mcp/handlers/resources.go
- Spec requirement: handleResourcesList, handleResourcesRead, handleResourcesSubscribe
- Implementation:
  ✅ ListResourcesHandler with Method() and Handle()
  ✅ ReadResourceHandler with Method() and Handle()
  ❌ handleResourcesSubscribe ❌ MISSING
  ✅ MCPError handling
  ✅ JSON-RPC request/response handling
- Working: ✅ YES - List and Read handlers fully implemented
- Gap: LOW (subscribe functionality missing)

---

### ❌ Requirements Missing or Incomplete (7/10)

**1. Resource Templates** (0% implemented)
- Spec requirement (lines 30-51):
  ```go
  type ResourceTemplate struct {
      URITemplate string
      Name        string
      Description string
      MimeType    string
      Annotations map[string]interface{}
  }
  ```
- Actual implementation: ❌ NONE - No templates or URITemplate structure
- Current approach: No dynamic resource templates
- Gap: CRITICAL (core feature for dynamic resource discovery)
- Impact: No dynamic resource construction, no inventory item templates

**2. ListTemplates Method** (0% implemented)
- Spec requirement: ResourceProvider interface has `ListTemplates(ctx context.Context) ([]ResourceTemplate, error)`
- Evidence: pkg/mcp/handlers/resources.go:14-17 - Interface has GetResource and ListResources only
- Actual implementation: ❌ N/A - Method not defined
- Current approach: No template listing capability
- Gap: MEDIUM (templates not supported at all)

**3. Resource Manager** (0% implemented)
- Spec requirement (lines 76-142):
  ```go
  type ResourceManager struct {
      mu        sync.RWMutex
      resources map[string]Resource
      providers []ResourceProvider
      templates []ResourceTemplate
  }

  func NewResourceManager() *ResourceManager
  func (rm *ResourceManager) RegisterResource(r Resource)
  func (rm *ResourceManager) RegisterProvider(p ResourceProvider)
  func (rm *ResourceManager) ListResources(ctx context.Context)
  func (rm *ResourceManager) ReadResource(ctx context.Context, uri string)
  ```
- Evidence: Search shows no ResourceManager anywhere in codebase
- Actual implementation: ❌ NONE - No centralized resource management
- Current approach: Direct handler/provider invocation without manager
- Gap: MAJOR (no centralization, no template support, no resource registration)
- Impact: No static resource storage, no provider aggregation, no template management

**4. Resource Updates/Subscription System** (0% implemented)
- Spec requirement (lines 147-190):
  ```go
  type SubscribeRequest struct { URI string }
  type UnsubscribeRequest struct { URI string }
  type ResourceUpdatedNotification struct { URI string }
  type ResourceListChangedNotification struct {}
  type ResourceSubscriber struct {
      subscribers map[string]map[*RequestID]bool
  }

  func (rs *ResourceSubscriber) Subscribe(uri string, id *RequestID)
  func (rs *ResourceSubscriber) NotifyUpdate(uri string)
  ```
- Evidence: No ResourceSubscriber, SubscribeRequest, or notification types found
- Actual implementation: ❌ NONE - No subscription/notification capability
- Current approach: No resource change tracking, no notification mechanism
- Gap: CRITICAL (core feature for live resource updates)
- Impact: No dynamic resource updates, no LLM notifications on changes

**5. Blob Support in ResourceContent** (0% implemented)
- Spec requirement (lines 18-24):
  ```go
  type ResourceContent struct {
      URI      string
      MimeType string
      Text     string  // text content
      Blob     string  // base64-encoded binary content
  }
  ```
- Evidence: pkg/mcp/handlers/resources.go:133-136 - ResourceContents only has Text field
- Implementation:
  ```go
  type ResourceContents struct {
      URI      string
      MimeType string
      Text     string  // ONLY text, no Blob
  }
  ```
- Current approach: Text only, no binary content support
- Gap: MEDIUM (cannot handle binary resources)
- Impact: No image/file resource support, limited to text-only resources

**6. Resource Annotations** (0% implemented)
- Spec requirement (lines 9-16):
  ```go
  type Resource struct {
      URI         string
      Name        string
      Description string
      MimeType    string
      Size        *int64
      Annotations map[string]interface{}  // MISSING
  }
  ```
- Evidence: pkg/mcp/handlers/resources.go:79-84 - Resource struct lacks Annotations and Size
- Implementation:
  ```go
  type Resource struct {
      URI         string
      Name        string
      Description string
      MimeType    string
      // Size and Annotations MISSING
  }
  ```
- Current approach: No metadata annotation capability
- Gap: LOW (optional extension field)
- Impact: No custom metadata, limited resource description

**7. Resource Size Tracking** (0% implemented)
- Spec requirement: Resource struct has optional `Size *int64` field
- Evidence: pkg/mcp/handlers/resources.go:79-84 - No Size field
- Current approach: No size information returned
- Gap: LOW (optional field, informational)
- Impact: No resource size information for LLMs

**8. ListCursor Pagination** (0% implemented)
- Spec requirement (lines 71-73):
  ```go
  type ListResourcesRequest struct {
      Cursor *string `json:"cursor,omitempty"`
  }

  type ListResourcesResult struct {
      Resources  []Resource `json:"resources"`
      NextCursor string     `json:"nextCursor,omitempty"`
  }
  ```
- Evidence: pkg/mcp/handlers/resources.go:71-77 - Request and Result structs have Cursor and NextCursor
- Implementation:
  ✅ Cursor field in ListResourcesRequest ✅
  ✅ NextCursor field in ListResourcesResult ✅
  ❌ BUT: Handler never processes Cursor or sets NextCursor ❌
  ❌ Gap: MEDIUM (pagination structures present but logic not implemented)

---

## Implementation Evidence

### Existing Implementation (pkg/mcp/handlers/resources.go)

**ResourceProvider Interface** (lines 14-17):
```go
type ResourceProvider interface {
    GetResource(uri string) (*StoredResource, error)
    ListResources() ([]ResourceInfo, error)
    // ⚠️ ListTemplates() MISSING
}
```

**ListResourcesHandler** (lines 26-68):
```go
func (h *ListResourcesHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
    // ✅ Unmarshals ListResourcesRequest
    // ✅ Calls provider.ListResources()
    // ✅ Converts to Resource structs
    // ❌ NEVER processes Cursor or sets NextCursor
    resourceList := make([]Resource, 0, len(resources))
    for _, res := range resources {
        resourceList = append(resourceList, Resource{
            URI:         res.URI,
            Name:        res.Name,
            Description: res.Description,
            MimeType:    res.MimeType,
            // ❌ Size and Annotations MISSING
        })
    }
    return &ListResourcesResult{Resources: resourceList}, nil
}
```

**ReadResourceHandler** (lines 100-123):
```go
func (h *ReadResourceHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
    var req ReadResourceRequest
    if err := json.Unmarshal(params, &req); err != nil {
        return nil, &MCPError{Code: -32602, Message: "Invalid params"}
    }

    res, err := h.provider.GetResource(req.URI)
    if err != nil {
        return nil, &MCPError{
            Code: -32603,
            Message: fmt.Sprintf("Resource not found: %s", req.URI),
        }
    }

    return &ReadResourceResult{
        Contents: []ResourceContents{
            {
                URI:      req.URI,
                MimeType: res.MimeType,
                Text:     string(res.Data),
                // ❌ Blob MISSING
            },
        },
    }, nil
}
```

**No Templates or Subscription**:
- Search for "ResourceTemplate" ❌ 0 results
- Search for "ResourceSubscriber" ❌ 0 results
- Search for "SubscribeRequest" ❌ 0 results
- Search for "ResourceManager" ❌ 0 results

---

## Impact Assessment

### Functional Impact

**Current Working Features**:
- ✅ Basic resource listing (ListResourcesHandler)
- ✅ Resource reading (ReadResourceHandler)
- ✅ Resource provider interface (basic)
- ✅ MCP protocol compliance (JSON-RPC handlers)

**Missing Capabilities**:
- ❌ No dynamic resource templates (no URITemplate support)
- ❌ No resource change notification (no subscription system)
- ❌ No binary content support (no Blob field)
- ❌ No resource metadata (no Annotations field)
- ❌ No size tracking (no Size field)
- ❌ No pagination (Cursor structures present but unused)
- ❌ No centralized management (no ResourceManager)

### User Experience Gap

**For LLM Integration**:
- ❌ Cannot provide dynamic resource discovery (no templates)
- ❌ Cannot notify LLM of resource changes (no subscription)
- ❌ Cannot handle binary resources (images/files)
- ❌ Cannot attach custom metadata to resources (no annotations)
- ❌ Cannot inform LLM of resource sizes (no size tracking)

**For MCP Protocol Compliance**:
- ✅ Protocol works for basic list/read operations
- ❌ NOT compliant with full MCP resource spec (missing templates, subscription, blob)

### Production Readiness

**Blocker Status**:
- ❌ **BLOCKING** if protocol compliance to FULL MCP spec required
- ✅ **NOT BLOCKING** if basic list/read operations only needed

**Decision Point**: Scope of MCP resource support
- If **full MCP protocol** required: Templates + subscription + blob are CRITICAL
- If **basic resource listing** only sufficient: Current state may be acceptable for MVP
- If **advanced MCP features** needed later: Can defer to Phase 2

---

## Action Items

### Required Implementation (12-16 hours for full compliance)

**1. Add Resource Templates** (2 hours)
- File: pkg/mcp/resources/templates.go (NEW)
- Components:
  ```go
  type ResourceTemplate struct {
      URITemplate string
      Name        string
      Description string
      MimeType    string
      Annotations map[string]interface{}
  }

  type ListResourceTemplatesResult struct {
      ResourceTemplates []ResourceTemplate
  }

  type ListTemplatesHandler struct {
      templates []ResourceTemplate
  }
  ```
- Priority: HIGH (core feature for dynamic resources)

**2. Implement Template Provider Method** (1.5 hours)
- File: pkg/mcp/handlers/resources.go
- Task: Add ListTemplates() []ResourceTemplate to ResourceProvider interface
- Task: Create default implementation returning empty list
- Priority: HIGH (required for spec compliance)

**3. Implement ResourceManager** (3 hours)
- File: pkg/mcp/resources/manager.go (NEW)
- Components:
  ```go
  type ResourceManager struct {
      mu        sync.RWMutex
      resources map[string]Resource
      providers []ResourceProvider
      templates []ResourceTemplate
  }

  func NewResourceManager() *ResourceManager
  func (rm *ResourceManager) RegisterResource(r Resource) error
  func (rm *ResourceManager) RegisterProvider(p ResourceProvider) error
  func (rm *ResourceManager) ListResources(ctx context.Context) ([]Resource, error)
  func (rm *ResourceManager) ReadResource(ctx context.Context, uri string) (*ResourceContent, error)
  func (rm *ResourceManager) ListTemplates(ctx context.Context) ([]ResourceTemplate, error)
  ```
- Priority: HIGH (centralization and template management)

**4. Add Resource Subscription/Notification** (3 hours)
- File: pkg/mcp/resources/subscription.go (NEW)
- Components:
  ```go
  type SubscribeRequest struct { URI string }
  type UnsubscribeRequest struct { URI string }
  type ResourceUpdatedNotification struct { URI string }
  type ResourceListChangedNotification struct {}

  type ResourceSubscriber struct {
      mu          sync.RWMutex
      subscribers map[string]map[*RequestID]bool
  }

  func NewResourceSubscriber() *ResourceSubscriber
  func (rs *ResourceSubscriber) Subscribe(uri string, id *RequestID) error
  func (rs *ResourceSubscriber) Unsubscribe(uri string, id *RequestID) error
  func (rs *ResourceSubscriber) NotifyUpdate(uri string)
  func (rs *ResourceSubscriber) NotifyListChanged()
  ```
- Priority: HIGH (core feature for dynamic updates)

**5. Add SubscribeHandler in Server** (2 hours)
- File: pkg/mcp/handlers/resources.go
- Task: Create SubscribeHandler and UnsubscribeHandler
- Task: Update server.go to register these handlers
- Priority: HIGH (completes subscription system)

**6. Add Blob Support** (1 hour)
- File: pkg/mcp/handlers/resources.go
- Task: Add Blob field to ResourceContents struct
- Task: Update ReadResourceHandler to support binary content
- Priority: MEDIUM (enables binary resource support)

**7. Add Annotations and Size Fields** (0.5 hours)
- File: pkg/mcp/handlers/resources.go
- Task: Add Annotations map to Resource struct
- Task: Add Size *int64 to Resource struct
- Priority: LOW (optional extension fields)

**8. Implement Cursor Pagination** (1 hour)
- File: pkg/mcp/handlers/resources.go
- Task: Process Cursor in ListResourcesHandler
- Task: Return NextCursor when more results available
- Priority: MEDIUM (pagination support)

**9. Add Tests** (3 hours)
- File: pkg/mcp/handlers/*_test.go
- Coverage:
  - Resource templates
  - Resource subscription
  - Resource manager
  - Pagination
  - Blob support
- Priority: MEDIUM (quality assurance)

### Recommended Next Steps

**Option 1: Full Protocol Compliance** (Recommended for MCP Server Product)
- Effort: 12-16 hours
- Benefit: Complete MCP resource spec compliance
- Timeline: 2 days development + testing
- Priority: HIGH (production-grade MCP server requires full spec)

**Option 2: Basic Resource Support** (Acceptable for MVP)
- Effort: 6-8 hours
- Scope: Only templates + manager (skip subscription and blob)
- Benefit: Basic resource discovery and listing
- Timeline: 1 day development
- Priority: MEDIUM (improves functionality without full compliance)

**Option 3: Defer to Phase 2** (Acceptable if time-constrained)
- Rationale: Basic list/read operations work for current use cases
- Justification: Templates and subscription advanced features
- When needed: User demand for dynamic resource discovery
- Note: Document as "Future Enhancement - Advanced MCP Resources"

---

## Verification Method

**Method**: Code inspection and file search
**Files Checked**:
- pkg/mcp/handlers/resources.go ✅ EXISTS (137 lines)
- pkg/mcp/resources/templates.go ❌ NOT FOUND
- pkg/mcp/resources/manager.go ❌ NOT FOUND
- pkg/mcp/resources/subscription.go ❌ NOT FOUND

**Search Patterns Used**:
- `ResourceTemplate`, `URITemplate`, `ListTemplates` → 0 results
- `ResourceManager` → 0 results
- `SubscribeRequest`, `UnsubscribeRequest` → 0 results
- `ResourceSubscriber`, `NotifyUpdate` → 0 results
- `ResourceContent.Blob` → 0 results

**Evidence**: Spec describes comprehensive resource system, but only basic handlers implemented

---

## Conclusion

**Overall Conformance**: 30% (3/10 requirements met)
**Critical Gaps**:
- ❌ No resource templates (no dynamic discovery)
- ❌ No subscription/notification system (no change tracking)
- ❌ No ResourceManager (no centralization)
**Functional Status**: Basic list/read operations work, but advanced features missing
**Production Impact**: NOT blocking if basic operations sufficient; BLOCKING for full MCP compliance
**Recommendation**: Implement templates + manager + subscription for production-grade MCP server
**Priority**: HIGH for MCP protocol compliance, MEDIUM for basic resource support
**Effort Required**: 12-16 hours for full implementation
**Timeline**: 2 days (development + testing)

**Decision Context**:
- If FULL MCP RESOURCE SPEC required: MUST IMPLEMENT (12-16h)
- If BASIC LIST/READ sufficient: DEFERRABLE to Phase 2 (advanced features)
- Current state: Functional for basic operations, non-compliant with full spec

**Production Readiness**: BASIC OPERATIONS WORK BUT ADVANCED FEATURES MISSING

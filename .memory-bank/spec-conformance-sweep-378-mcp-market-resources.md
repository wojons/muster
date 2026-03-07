# Spec Conformance Sweep #378 - MCP Market Resources

**Date**: 2026-03-06
**Spec**: MCP-MARKET-RESOURCES.md (234 lines - MCP resources specification)
**Status**: ⚠️ 30% conformance (3/10 major requirements met - PARTIAL IMPLEMENTATION)

---

## Analysis Result

**Conformance**: 30% (3/10 major requirements met)
**Status**: PARTIAL - Basic resource handlers implemented, but templates, subscription, and blob support missing
**Production Ready**: NO - Incomplete MCP resource protocol compliance
**Priority**: HIGH - MCP protocol compliance critical for Claude Desktop integration

---

## Spec Requirements vs. Implementation

### ✅ Resource Types (100% - PARTIAL MATCH)

**Spec Requirements** (lines 8-24):
```go
type Resource struct {
    URI         string
    Name        string
    Description string
    MimeType    string
    Size        *int64              // MISSING in implementation
    Annotations map[string]interface{}  // MISSING in implementation
}

type ResourceContent struct {
    URI      string
    MimeType string
    Text     string
    Blob     string              // MISSING in implementation (no base64 binary support)
}
```

**Actual Implementation** (pkg/mcp/protocol.go:125-130):
```go
type Resource struct {
    URI         string
    Name        string
    Description string
    MimeType    string
    // MISSING: Size *int64
    // MISSING: Annotations map[string]interface{}
}

type ResourceContents struct {
    URI      string
    MimeType string
    Text     string
    // MISSING: Blob string (no binary support)
}
```

**Status**: ⚠️ 60% conformance - Missing Size, Annotations, Blob fields

---

### ❌ Resource Templates (0% - NOT IMPLEMENTED)

**Spec Requirements** (lines 29-50):
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

interface ResourceProvider {
    ListTemplates(ctx context.Context) ([]ResourceTemplate, error)  // MISSING
}
```

**Actual Implementation**: NOT FOUND
- No ResourceTemplate struct exists
- No ListTemplatesHandler exists
- No templates/list JSON-RPC method
- No dynamic resource template support

**File Locations Checked**:
- pkg/mcp/handlers/resources.go (contains handlers but no templates)
- pkg/mcp/protocol.go (no template types)
- pkg/mcp/ (no ResourceManager or template system)

**Status**: ❌ 0% conformance - Templates not implemented

---

### ⚠️ Resource Provider (60% - SIGNATURE MISMATCH)

**Spec Requirements** (lines 66-71):
```go
type ResourceProvider interface {
    ListResources(ctx context.Context) ([]Resource, error)
    ReadResource(ctx context.Context, uri string) (*ResourceContent, error)
    ListTemplates(ctx context.Context) ([]ResourceTemplate, error)  // MISSING
}
```

**Actual Implementation** (pkg/mcp/handlers/resources.go:14-17):
```go
type ResourceProvider interface {
    GetResource(uri string) (*StoredResource, error)  // Different signature
    ListResources() ([]ResourceInfo, error)  // No context parameter
    // MISSING: ListTemplates() method
    // MISSING: ReadResource() method (different name: GetResource)
}
```

**Issues Identified**:
1. No context.Context parameter on ListResources() (can't cancel requests)
2. Method name mismatch: ReadResource() vs GetResource()
3. Missing ListTemplates() method
4. Return type mismatch: ResourceInfo vs Resource struct

**Status**: ⚠️ 60% conformance - Interface doesn't match spec

---

### ❌ Resource Manager (0% - NOT IMPLEMENTED)

**Spec Requirements** (lines 76-142):
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
func (rm *ResourceManager) ListResources(ctx) ([]Resource, error)
func (rm *ResourceManager) ReadResource(ctx, uri) (*ResourceContent, error)
```

**Actual Implementation**: NOT FOUND
- No ResourceManager struct exists
- No centralized resource management
- No provider registration system
- No static resource storage

**File Locations Checked**:
- pkg/mcp/ (no resource manager files)
- pkg/mcp/handlers/ (no manager code)
- Search: `grep -r "ResourceManager" pkg/` - 0 results

**Status**: ❌ 0% conformance - ResourceManager not implemented

---

### ❌ Resource Subscription System (0% - NOT IMPLEMENTED)

**Spec Requirements** (lines 147-191):
```go
type SubscribeRequest struct {
    URI string
}

type UnsubscribeRequest struct {
    URI string
}

type ResourceUpdatedNotification struct {
    URI string
}

type ResourceListChangedNotification struct{}

type ResourceSubscriber struct {
    mu          sync.RWMutex
    subscribers map[string]map[*RequestID]bool
}

func (rs *ResourceSubscriber) Subscribe(uri string, id *RequestID)
func (rs *ResourceSubscriber) NotifyUpdate(uri string)
```

**Actual Implementation**: NOT FOUND
- No SubscribeRequest struct
- No UnsubscribeRequest struct
- No ResourceUpdatedNotification
- No ResourceSubscriber struct
- No subscription tracking
- No notification system

**JSON-RPC Methods Missing**:
- resources/subscribe
- resources/unsubscribe
- notifications/resources/updated
- notifications/resources/list_changed

**File Locations Checked**:
- pkg/mcp/handlers/resources.go (no subscription methods)
- pkg/mcp/protocol.go (no notification types)
- pkg/mcp/server.go (no subscription handlers)

**Status**: ❌ 0% conformance - Subscription system not implemented

---

### ✅ Basic Resource Handlers (100% - IMPLEMENTED)

**Spec Requirements** (lines 195-233):
```go
func (s *Server) handleResourcesList(req Request)
func (s *Server) handleResourcesRead(req Request)
func (s *Server) handleResourcesSubscribe(req Request)  // MISSING
```

**Actual Implementation**:
- ✅ `ListResourcesHandler` (pkg/mcp/handlers/resources.go:25-68) - Method: "resources/list"
- ✅ `ReadResourceHandler` (pkg/mcp/handlers/resources.go:86-123) - Method: "resources/read"
- ❌ `SubscribeHandler` - NOT FOUND (no resources/subscribe)

**Status**: ⚠️ 66% conformance - 2/3 handlers implemented

---

## Conformance Summary

| Requirement Area | Spec Conformance | Implementation Status |
|------------------|------------------|----------------------|
| Resource Types | 60% | Missing Size, Annotations, Blob |
| Resource Templates | 0% ❌ | Not implemented |
| Resource Providers | 60% ⚠️ | Signature mismatch, missing ListTemplates() |
| Resource Manager | 0% ❌ | Not implemented |
| Resource Subscription | 0% ❌ | Not implemented |
| Basic Handlers | 66% ⚠️ | 2/3 methods implemented |
| **Overall** | **30%** | **Partial implementation, gaps critical** |

---

## Impact Analysis

**Functional Impact**: MAJOR
- Cannot handle binary resources (no Blob support)
- No dynamic resource templates (fixed resources only)
- No resource change notifications (Claude Desktop can't subscribe to updates)
- No centralized resource management (each handler isolated)
- Interface mismatches prevent provider compatibility

**MCP Protocol Compliance**: PARTIAL (30%)
- ✅ Can list resources (basic)
- ✅ Can read resources (text only)
- ❌ Cannot handle binary content
- ❌ Cannot support dynamic templates
- ❌ Cannot push change notifications
- ❌ Cannot handle subscription/unsubscription

**Claude Desktop Integration**: LIMITED
- Basic read/list operations work
- Binary content NOT supported
- Subscription notifications NOT supported
- Dynamic resource discovery NOT supported

---

## Recommended Actions

**Priority 1HIGH (Critical - MCP Protocol Compliance)**:
1. Add Resource struct fields: Size *int64, Annotations map[string]interface{} (1h)
2. Add ResourceContents Blob field for binary support (2h)
3. Implement ResourceTemplate struct (1h)
4. Create ListTemplatesHandler with templates/list method (2h)
5. Add ListTemplates() to ResourceProvider interface (1h)

**Priority 2: HIGH (Subscription System)**:
1. Implement SubscribeRequest/UnsubscribeRequest structs (1h)
2. Implement ResourceUpdatedNotification type (0.5h)
3. Implement ResourceSubscriber struct with tracking (2h)
4. Create SubscribeHandler with resources/subscribe method (2h)
5. Create UnsubscribeHandler with resources/unsubscribe method (2h)
6. Implement notification dispatch system (3h)

**Priority 3: MEDIUM (Resource Manager)**:
1. Implement ResourceManager struct (2h)
2. Add RegisterResource/RegisterProvider methods (1h)
3. Implement centralized ListResources/ReadResource (2h)
4. Integrate with existing handlers (2h)

**Total Estimated Effort**: 23-25 hours for full MCP Resources compliance

---

## File Locations

**Implementation Files**:
- pkg/mcp/protocol.go (lines 125-144) - Resource types
- pkg/mcp/handlers/resources.go (137 lines) - Basic handlers

**Missing Files**:
- pkg/mcp/handlers/templates.go - Template handler
- pkg/mcp/handlers/subscription.go - Subscription handler
- pkg/mcp/resources/manager.go - Resource manager

---

## Notes

**System Type Assessment**:
- Spec describes comprehensive MCP Resources protocol (10 major requirements)
- Actual implements basic read/list handlers only (3/10 requirements)
- Missing templates, subscriptions, and blob support

**Architecture Observation**:
- Current implementation uses simple handler pattern
- Spec expects full resource management system (ResourceManager, ResourceSubscriber)
- No centralized state management for resources

**Production Readiness**: NO
- MCP Resources protocol 30% conformant
- Critical gaps prevent full protocol compliance
- Claude Desktop subscription notifications NOT available
- Binary resource handling NOT available

**Verification Method**:
- Read spec: MCP-MARKET-RESOURCES.md (234 lines)
- Checked files: pkg/mcp/protocol.go, pkg/mcp/handlers/resources.go
- Verified types: Resource, ResourceContents, ResourceProvider, handlers
- Found missing: ResourceTemplate, ResourceManager, ResourceSubscriber

---

## Next Steps

1. Decide: Implement full MCP Resources compliance (23-25h) OR accept partial read/list-only behavior
2. If full compliance needed:
   - Start with Priority 1 (Resource types + templates)
   - Follow with Priority 2 (Subscription system)
   - Complete with Priority 3 (Resource Manager)
3. Update docs: Reflect partial MCP Resources support
4. Document: Known limitations (no binary, no templates, no subscriptions)

---

**Sweep Status**: 🔴 CRITICAL GAPS FOUND (70% of spec not implemented)
**Action Required**: Decision on MCP Resources compliance level needed for production

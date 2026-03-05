# Domain: API Operation

## Overview

Operation entities define API endpoints, their parameters, request/response schemas, and groupings for organized API management.

## Entities

### Operation

Core API operation entity representing an endpoint.

```go
type Operation struct {
    ID            string             `json:"id" validate:"required,uuid4"`
    InventoryID   string             `json:"inventory_id" validate:"required,uuid4"`
    GroupID       *string            `json:"group_id,omitempty" validate:"omitempty,uuid4"`
    Name          string             `json:"name" validate:"required,min=3,max=100,operation_name"`
    Summary       string             `json:"summary" validate:"max=200"`
    Description   string             `json:"description" validate:"max=2000"`
    Method        HTTPMethod         `json:"method" validate:"required,http_method"`
    Path          string             `json:"path" validate:"required,path_pattern,max=500"`
    Tags          []string           `json:"tags" validate:"dive,min=1,max=50"`
    Deprecated    bool               `json:"deprecated"`
    Security      []SecurityScheme   `json:"security,omitempty"`
    Parameters    []Parameter        `json:"parameters" validate:"dive"`
    RequestBody   *RequestBody       `json:"request_body,omitempty"`
    Responses     []Response         `json:"responses" validate:"required,min=1,dive"`
    Callbacks     []Callback         `json:"callbacks,omitempty" validate:"dive"`
    Servers       []Server           `json:"servers,omitempty" validate:"dive"`
    ExternalDocs  *ExternalDocs      `json:"external_docs,omitempty"`
    Extensions    map[string]any     `json:"extensions,omitempty"`
    Metadata      OperationMetadata  `json:"metadata"`
    Status        OperationStatus    `json:"status" validate:"required"`
    CreatedAt     time.Time          `json:"created_at"`
    UpdatedAt     time.Time          `json:"updated_at"`
}

type HTTPMethod string

const (
    MethodGET     HTTPMethod = "GET"
    MethodPOST    HTTPMethod = "POST"
    MethodPUT     HTTPMethod = "PUT"
    MethodPATCH   HTTPMethod = "PATCH"
    MethodDELETE  HTTPMethod = "DELETE"
    MethodHEAD    HTTPMethod = "HEAD"
    MethodOPTIONS HTTPMethod = "OPTIONS"
    MethodTRACE   HTTPMethod = "TRACE"
)

type OperationStatus string

const (
    OperationStatusDraft     OperationStatus = "draft"
    OperationStatusActive    OperationStatus = "active"
    OperationStatusDeprecated OperationStatus = "deprecated"
    OperationStatusRemoved   OperationStatus = "removed"
)
```

### Parameter

Operation parameter definitions.

```go
type Parameter struct {
    ID             string           `json:"id" validate:"required,uuid4"`
    OperationID    string           `json:"operation_id" validate:"required,uuid4"`
    Name           string           `json:"name" validate:"required,max=100"`
    In             ParameterIn      `json:"in" validate:"required,oneof=query header path cookie"`
    Description    string           `json:"description" validate:"max=500"`
    Required       bool             `json:"required"`
    Deprecated     bool             `json:"deprecated"`
    AllowEmpty     bool             `json:"allow_empty"`
    Style          ParameterStyle   `json:"style,omitempty" validate:"omitempty,oneof=matrix label form simple spaceDelimited pipeDelimited deepObject"`
    Explode        bool             `json:"explode"`
    Schema         Schema           `json:"schema" validate:"required"`
    Example        any              `json:"example,omitempty"`
    Examples       map[string]Example `json:"examples,omitempty"`
    AllowReserved  bool             `json:"allow_reserved"`
}

type ParameterIn string

const (
    ParameterInQuery  ParameterIn = "query"
    ParameterInHeader ParameterIn = "header"
    ParameterInPath   ParameterIn = "path"
    ParameterInCookie ParameterIn = "cookie"
)

type ParameterStyle string

const (
    ParameterStyleMatrix          ParameterStyle = "matrix"
    ParameterStyleLabel           ParameterStyle = "label"
    ParameterStyleForm            ParameterStyle = "form"
    ParameterStyleSimple          ParameterStyle = "simple"
    ParameterStyleSpaceDelimited  ParameterStyle = "spaceDelimited"
    ParameterStylePipeDelimited   ParameterStyle = "pipeDelimited"
    ParameterStyleDeepObject      ParameterStyle = "deepObject"
)
```

### Request Body

Request body definition.

```go
type RequestBody struct {
    ID          string           `json:"id" validate:"required,uuid4"`
    OperationID string           `json:"operation_id" validate:"required,uuid4"`
    Description string           `json:"description" validate:"max=500"`
    Required    bool             `json:"required"`
    Content     []ContentType    `json:"content" validate:"required,min=1,dive"`
}

type ContentType struct {
    ID             string           `json:"id" validate:"required,uuid4"`
    MimeType       string           `json:"mime_type" validate:"required,mime_type"`
    Schema         Schema           `json:"schema"`
    Example        any              `json:"example,omitempty"`
    Examples       map[string]Example `json:"examples,omitempty"`
    Encoding       map[string]Encoding `json:"encoding,omitempty"`
}

type Encoding struct {
    ContentType   string            `json:"content_type" validate:"mime_type"`
    Headers       map[string]Header `json:"headers,omitempty"`
    Style         string            `json:"style,omitempty"`
    Explode       bool              `json:"explode"`
    AllowReserved bool              `json:"allow_reserved"`
}
```

### Response

Response definition.

```go
type Response struct {
    ID          string            `json:"id" validate:"required,uuid4"`
    OperationID string            `json:"operation_id" validate:"required,uuid4"`
    Code        string            `json:"code" validate:"required,status_code"`
    Description string            `json:"description" validate:"required,max=500"`
    Headers     []Header          `json:"headers,omitempty" validate:"dive"`
    Content     []ContentType     `json:"content,omitempty" validate:"dive"`
    Links       []Link            `json:"links,omitempty" validate:"dive"`
}

type Header struct {
    ID          string            `json:"id" validate:"required,uuid4"`
    Name        string            `json:"name" validate:"required,max=100"`
    Description string            `json:"description" validate:"max=500"`
    Required    bool              `json:"required"`
    Deprecated  bool              `json:"deprecated"`
    Schema      Schema            `json:"schema"`
}

type Link struct {
    ID          string            `json:"id" validate:"required,uuid4"`
    Name        string            `json:"name" validate:"required,max=100"`
    OperationID string            `json:"operation_id,omitempty"`
    OperationRef string           `json:"operation_ref,omitempty"`
    Parameters  map[string]any    `json:"parameters,omitempty"`
    RequestBody any               `json:"request_body,omitempty"`
    Description string            `json:"description,omitempty"`
    Server      *Server           `json:"server,omitempty"`
}
```

### Schema

Schema definition for data validation.

```go
type Schema struct {
    ID             string            `json:"id,omitempty" validate:"omitempty,uuid4"`
    Type           string            `json:"type,omitempty" validate:"omitempty,oneof=object array string number integer boolean null"`
    Format         string            `json:"format,omitempty"`
    Title          string            `json:"title,omitempty" validate:"max=200"`
    Description    string            `json:"description,omitempty" validate:"max=2000"`
    Default        any               `json:"default,omitempty"`
    MultipleOf     *float64          `json:"multiple_of,omitempty"`
    Maximum        *float64          `json:"maximum,omitempty"`
    ExclusiveMax   bool              `json:"exclusive_max,omitempty"`
    Minimum        *float64          `json:"minimum,omitempty"`
    ExclusiveMin   bool              `json:"exclusive_min,omitempty"`
    MaxLength      *int              `json:"max_length,omitempty" validate:"omitempty,gte=0"`
    MinLength      *int              `json:"min_length,omitempty" validate:"omitempty,gte=0"`
    Pattern        string            `json:"pattern,omitempty"`
    MaxItems       *int              `json:"max_items,omitempty" validate:"omitempty,gte=0"`
    MinItems       *int              `json:"min_items,omitempty" validate:"omitempty,gte=0"`
    UniqueItems    bool              `json:"unique_items,omitempty"`
    MaxProperties  *int              `json:"max_properties,omitempty" validate:"omitempty,gte=0"`
    MinProperties  *int              `json:"min_properties,omitempty" validate:"omitempty,gte=0"`
    Required       []string          `json:"required,omitempty" validate:"dive,max=100"`
    Enum           []any             `json:"enum,omitempty"`
    Properties     map[string]Schema `json:"properties,omitempty"`
    Items          *Schema           `json:"items,omitempty"`
    AllOf          []Schema          `json:"all_of,omitempty"`
    OneOf          []Schema          `json:"one_of,omitempty"`
    AnyOf          []Schema          `json:"any_of,omitempty"`
    Not            *Schema           `json:"not,omitempty"`
    AdditionalProperties any         `json:"additional_properties,omitempty"`
    Nullable       bool              `json:"nullable,omitempty"`
    ReadOnly       bool              `json:"read_only,omitempty"`
    WriteOnly      bool              `json:"write_only,omitempty"`
    Deprecated     bool              `json:"deprecated,omitempty"`
    Example        any               `json:"example,omitempty"`
    ExternalDocs   *ExternalDocs     `json:"external_docs,omitempty"`
}
```

### OperationGroup

Grouping entity for organizing operations.

```go
type OperationGroup struct {
    ID          string            `json:"id" validate:"required,uuid4"`
    InventoryID string            `json:"inventory_id" validate:"required,uuid4"`
    Name        string            `json:"name" validate:"required,min=3,max=100,alphanum_underscore"`
    Description string            `json:"description" validate:"max=500"`
    BasePath    string            `json:"base_path,omitempty" validate:"omitempty,path_prefix"`
    Tags        []string          `json:"tags,omitempty" validate:"dive,max=50"`
    Metadata    GroupMetadata     `json:"metadata"`
    Order       int               `json:"order"`
    Operations  []string          `json:"operations,omitempty" validate:"dive,uuid4"`
}

type GroupMetadata struct {
    DisplayName   string            `json:"display_name,omitempty" validate:"max=200"`
    Icon          string            `json:"icon,omitempty" validate:"max=50"`
    Color         string            `json:"color,omitempty" validate:"omitempty,hexcolor"`
    Documentation string            `json:"documentation,omitempty" validate:"omitempty,url"`
}
```

## Validation Methods

```go
func (o *Operation) Validate() error {
    if err := validator.Validate(o); err != nil {
        return fmt.Errorf("operation validation failed: %w", err)
    }
    
    if o.RequestBody != nil && !o.hasBodyMethod() {
        return errors.New("request body only allowed for POST, PUT, PATCH, DELETE methods")
    }
    
    pathParams := o.extractPathParams()
    for _, param := range o.Parameters {
        if param.In == ParameterInPath {
            if !contains(pathParams, param.Name) {
                return fmt.Errorf("path parameter %s not found in path template", param.Name)
            }
        }
    }
    
    for _, required := range pathParams {
        found := false
        for _, param := range o.Parameters {
            if param.Name == required && param.In == ParameterInPath {
                found = true
                if !param.Required {
                    return fmt.Errorf("path parameter %s must be required", param.Name)
                }
                break
            }
        }
        if !found {
            return fmt.Errorf("required path parameter %s not defined", required)
        }
    }
    
    hasSuccess := false
    for _, resp := range o.Responses {
        if strings.HasPrefix(resp.Code, "2") {
            hasSuccess = true
            break
        }
    }
    if !hasSuccess {
        return errors.New("operation must define at least one success (2xx) response")
    }
    
    return nil
}

func (o *Operation) hasBodyMethod() bool {
    switch o.Method {
    case MethodPOST, MethodPUT, MethodPATCH, MethodDELETE:
        return true
    default:
        return false
    }
}
```

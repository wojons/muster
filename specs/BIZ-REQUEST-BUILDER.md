# Business Request Builder Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: HTTP Client

## Overview

Complete specification for **HTTP request construction** from OpenAPI operations. The request builder handles parameter serialization, body construction, header management, and URL templating for API requests.

## 1. Builder Architecture

```
RequestBuilder
├── Base Configuration (URL, Method)
├── Path Parameters
├── Query Parameters
├── Headers
├── Body
└── Execute
```

## 2. Core Components

### 2.1 Request Builder

```go
package request

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
)

type Builder struct {
    baseURL      string
    path         string
    method       string
    pathParams   map[string]string
    queryParams  url.Values
    headers      http.Header
    body         io.Reader
    contentType  string
    timeout      time.Duration
}

func NewBuilder(baseURL, path, method string) *Builder {
    return &Builder{
        baseURL:     strings.TrimRight(baseURL, "/"),
        path:        path,
        method:      strings.ToUpper(method),
        pathParams:  make(map[string]string),
        queryParams: make(url.Values),
        headers:     make(http.Header),
        timeout:     30 * time.Second,
    }
}

// Path parameters
func (b *Builder) WithPathParam(name, value string) *Builder {
    b.pathParams[name] = value
    return b
}

// Query parameters
func (b *Builder) WithQueryParam(name, value string) *Builder {
    if value != "" {
        b.queryParams.Add(name, value)
    }
    return b
}

// Headers
func (b *Builder) WithHeader(key, value string) *Builder {
    b.headers.Set(key, value)
    return b
}

// Body with interface{}
func (b *Builder) WithBody(body interface{}) *Builder {
    switch v := body.(type) {
    case io.Reader:
        b.body = v
    case []byte:
        b.body = bytes.NewReader(v)
    case string:
        b.body = strings.NewReader(v)
    default:
        data, err := json.Marshal(v)
        if err != nil {
            panic(fmt.Sprintf("marshal body: %v", err))
        }
        b.body = bytes.NewReader(data)
        b.contentType = "application/json"
    }
    return b
}

// Authentication
func (b *Builder) WithAuth(token string) *Builder {
    b.headers.Set("Authorization", "Bearer "+token)
    return b
}

func (b *Builder) WithBasicAuth(username, password string) *Builder {
    b.headers.Set("Authorization", "Basic "+basicAuth(username, password))
    return b
}

func (b *Builder) WithAPIKey(key, value string) *Builder {
    b.headers.Set(key, value)
    return b
}

// Build the request
func (b *Builder) Build() (*http.Request, error) {
    // Build URL with path params
    finalPath := b.buildPath()
    fullURL := b.baseURL + finalPath
    
    // Add query params
    if len(b.queryParams) > 0 {
        fullURL += "?" + b.queryParams.Encode()
    }
    
    // Create request
    req, err := http.NewRequest(b.method, fullURL, b.body)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    
    // Set headers
    for key, values := range b.headers {
        for _, value := range values {
            req.Header.Add(key, value)
        }
    }
    
    // Set content type if body exists
    if b.body != nil && b.contentType != "" {
        req.Header.Set("Content-Type", b.contentType)
    }
    
    // Set default accept header
    if req.Header.Get("Accept") == "" {
        req.Header.Set("Accept", "application/json")
    }
    
    return req, nil
}

func (b *Builder) buildPath() string {
    path := b.path
    for key, value := range b.pathParams {
        placeholder := "{" + key + "}"
        path = strings.ReplaceAll(path, placeholder, url.PathEscape(value))
    }
    return path
}

func (b *Builder) Execute(ctx context.Context) (*http.Response, error) {
    req, err := b.Build()
    if err != nil {
        return nil, err
    }
    
    req = req.WithContext(ctx)
    
    client := &http.Client{
        Timeout: b.timeout,
    }
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    return resp, nil
}
```

## 3. Parameter Serialization

### 3.1 Query Parameter Serialization

```go
type ParamSerializer struct {
    style   string
    explode bool
}

func NewParamSerializer(style string, explode bool) *ParamSerializer {
    return &ParamSerializer{
        style:   style,
        explode: explode,
    }
}

func (ps *ParamSerializer) Serialize(name string, value interface{}) url.Values {
    result := make(url.Values)
    
    switch v := value.(type) {
    case string:
        result.Set(name, v)
    case int, int64:
        result.Set(name, fmt.Sprintf("%d", v))
    case float64:
        result.Set(name, fmt.Sprintf("%g", v))
    case bool:
        result.Set(name, fmt.Sprintf("%t", v))
    case []string:
        ps.serializeArray(result, name, v)
    case []int:
        var strs []string
        for _, i := range v {
            strs = append(strs, fmt.Sprintf("%d", i))
        }
        ps.serializeArray(result, name, strs)
    case time.Time:
        result.Set(name, v.Format(time.RFC3339))
    default:
        result.Set(name, fmt.Sprintf("%v", v))
    }
    
    return result
}

func (ps *ParamSerializer) serializeArray(result url.Values, name string, values []string) {
    switch ps.style {
    case "form":
        if ps.explode {
            for _, item := range values {
                result.Add(name, item)
            }
        } else {
            result.Set(name, strings.Join(values, ","))
        }
    case "spaceDelimited":
        result.Set(name, strings.Join(values, " "))
    case "pipeDelimited":
        result.Set(name, strings.Join(values, "|"))
    default:
        result.Set(name, strings.Join(values, ","))
    }
}

func (b *Builder) WithTypedQueryParam(name string, value interface{}) *Builder {
    serializer := NewParamSerializer("form", true)
    values := serializer.Serialize(name, value)
    
    for key, vals := range values {
        for _, val := range vals {
            b.queryParams.Add(key, val)
        }
    }
    return b
}
```

### 3.2 Deep Object Serialization

```go
func (b *Builder) WithDeepObjectParam(prefix string, obj interface{}) *Builder {
    values := serializeDeepObject(prefix, obj)
    
    for key, vals := range values {
        for _, val := range vals {
            b.queryParams.Add(key, val)
        }
    }
    return b
}

func serializeDeepObject(prefix string, obj interface{}) url.Values {
    result := make(url.Values)
    v := reflect.ValueOf(obj)
    t := v.Type()
    
    // Handle pointer
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
        t = v.Type()
    }
    
    if v.Kind() != reflect.Struct {
        result.Set(prefix, fmt.Sprintf("%v", obj))
        return result
    }
    
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := t.Field(i)
        
        // Skip unexported fields
        if !field.CanInterface() {
            continue
        }
        
        // Get JSON tag or field name
        name := fieldType.Name
        if tag := fieldType.Tag.Get("json"); tag != "" {
            parts := strings.Split(tag, ",")
            if parts[0] != "-" {
                name = parts[0]
            }
        }
        
        key := prefix + "[" + name + "]"
        
        if field.Kind() == reflect.Struct {
            nested := serializeDeepObject(key, field.Interface())
            for k, v := range nested {
                result[k] = v
            }
        } else if field.Kind() == reflect.Slice {
            for j := 0; j < field.Len(); j++ {
                result.Add(key, fmt.Sprintf("%v", field.Index(j).Interface()))
            }
        } else {
            result.Set(key, fmt.Sprintf("%v", field.Interface()))
        }
    }
    
    return result
}
```

## 4. Body Building

### 4.1 Content Type Builders

```go
func (b *Builder) WithJSONBody(data interface{}) *Builder {
    jsonData, err := json.Marshal(data)
    if err != nil {
        panic(fmt.Sprintf("marshal JSON: %v", err))
    }
    
    b.body = bytes.NewReader(jsonData)
    b.contentType = "application/json"
    return b
}

func (b *Builder) WithFormBody(data map[string]string) *Builder {
    form := url.Values{}
    for key, value := range data {
        form.Set(key, value)
    }
    
    b.body = strings.NewReader(form.Encode())
    b.contentType = "application/x-www-form-urlencoded"
    return b
}

func (b *Builder) WithMultipartBody(fields map[string]string, files map[string]string) (*Builder, error) {
    var buf bytes.Buffer
    writer := multipart.NewWriter(&buf)
    
    // Add form fields
    for key, value := range fields {
        if err := writer.WriteField(key, value); err != nil {
            return nil, fmt.Errorf("write field %s: %w", key, err)
        }
    }
    
    // Add files
    for key, path := range files {
        file, err := os.Open(path)
        if err != nil {
            return nil, fmt.Errorf("open file %s: %w", path, err)
        }
        
        part, err := writer.CreateFormFile(key, filepath.Base(path))
        if err != nil {
            file.Close()
            return nil, fmt.Errorf("create form file: %w", err)
        }
        
        if _, err := io.Copy(part, file); err != nil {
            file.Close()
            return nil, fmt.Errorf("copy file: %w", err)
        }
        file.Close()
    }
    
    if err := writer.Close(); err != nil {
        return nil, fmt.Errorf("close writer: %w", err)
    }
    
    b.body = &buf
    b.contentType = writer.FormDataContentType()
    return b, nil
}

func (b *Builder) WithXMLBody(data interface{}) *Builder {
    xmlData, err := xml.Marshal(data)
    if err != nil {
        panic(fmt.Sprintf("marshal XML: %v", err))
    }
    
    b.body = bytes.NewReader(xmlData)
    b.contentType = "application/xml"
    return b
}

func (b *Builder) WithPlainTextBody(text string) *Builder {
    b.body = strings.NewReader(text)
    b.contentType = "text/plain"
    return b
}
```

## 5. Header Management

```go
func (b *Builder) WithStandardHeaders(version, fingerprint string) *Builder {
    b.headers.Set("Accept", "application/json")
    b.headers.Set("X-Client-Version", version)
    b.headers.Set("X-Client-Fingerprint", fingerprint)
    b.headers.Set("User-Agent", fmt.Sprintf("muster/%s (%s; %s)", version, runtime.GOOS, runtime.GOARCH))
    return b
}

func (b *Builder) WithAcceptFormat(format string) *Builder {
    switch format {
    case "json":
        b.headers.Set("Accept", "application/json")
    case "yaml", "yml":
        b.headers.Set("Accept", "application/yaml")
    case "xml":
        b.headers.Set("Accept", "application/xml")
    case "html":
        b.headers.Set("Accept", "text/html")
    case "text", "plain":
        b.headers.Set("Accept", "text/plain")
    default:
        b.headers.Set("Accept", "application/json")
    }
    return b
}

func (b *Builder) WithConditionalRequest(etag string) *Builder {
    if etag != "" {
        b.headers.Set("If-None-Match", etag)
    }
    return b
}

func (b *Builder) WithIdempotencyKey(key string) *Builder {
    if key == "" {
        key = generateIdempotencyKey()
    }
    b.headers.Set("Idempotency-Key", key)
    return b
}

func (b *Builder) WithRequestID(id string) *Builder {
    if id == "" {
        id = generateRequestID()
    }
    b.headers.Set("X-Request-ID", id)
    return b
}

func generateIdempotencyKey() string {
    return uuid.New().String()
}

func generateRequestID() string {
    return fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
}
```

## 6. URL Templating

```go
type URITemplate struct {
    template string
}

func NewURITemplate(template string) *URITemplate {
    return &URITemplate{template: template}
}

func (t *URITemplate) Expand(values map[string]string) (string, error) {
    result := t.template
    
    matches := templateRegex.FindAllStringSubmatch(t.template, -1)
    for _, match := range matches {
        fullMatch := match[0]
        varSpec := match[1]
        
        // Parse variable specification
        varName, modifier := parseVarSpec(varSpec)
        
        value, ok := values[varName]
        if !ok {
            return "", fmt.Errorf("missing template variable: %s", varName)
        }
        
        var replacement string
        switch modifier {
        case "+":
            // Reserved expansion
            replacement = value
        case "/":
            // Path segment expansion
            replacement = "/" + value
        case "?":
            // Query expansion
            replacement = "?" + varName + "=" + url.QueryEscape(value)
        case "&":
            // Query continuation
            replacement = "&" + varName + "=" + url.QueryEscape(value)
        case "#":
            // Fragment expansion
            replacement = "#" + value
        default:
            // Simple string expansion
            replacement = url.PathEscape(value)
        }
        
        result = strings.Replace(result, fullMatch, replacement, 1)
    }
    
    return result, nil
}

func parseVarSpec(spec string) (name, modifier string) {
    // Check for modifier prefix
    if len(spec) > 0 {
        switch spec[0] {
        case '+', '/', '?', '&', '#':
            return spec[1:], string(spec[0])
        }
    }
    return spec, ""
}

var templateRegex = regexp.MustCompile(`\{([^}]+)\}`)
```

## 7. Execution Context

```go
type ExecutionResult struct {
    StatusCode int
    Headers    http.Header
    Body       []byte
    Duration   time.Duration
}

func (b *Builder) ExecuteWithResult(ctx context.Context) (*ExecutionResult, error) {
    start := time.Now()
    
    req, err := b.Build()
    if err != nil {
        return nil, err
    }
    
    req = req.WithContext(ctx)
    
    client := &http.Client{
        Timeout: b.timeout,
    }
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read body: %w", err)
    }
    
    return &ExecutionResult{
        StatusCode: resp.StatusCode,
        Headers:    resp.Header,
        Body:       body,
        Duration:   time.Since(start),
    }, nil
}

func (b *Builder) WithTimeout(timeout time.Duration) *Builder {
    b.timeout = timeout
    return b
}

func (b *Builder) WithRetry(retries int, backoff time.Duration) *Builder {
    b.retries = retries
    b.backoff = backoff
    return b
}
```

## 8. Complete Example

```go
func ExampleRequestBuilder() {
    builder := NewBuilder(
        "https://api.example.com",
        "/api/v1/namespaces/{namespace}/pods/{name}",
        "GET",
    )
    
    result, err := builder.
        WithPathParam("namespace", "production").
        WithPathParam("name", "web-server").
        WithQueryParam("pretty", "true").
        WithQueryParam("limit", "10").
        WithAuth("token-123").
        WithStandardHeaders("1.0.0", "abc123").
        WithAcceptFormat("json").
        WithRequestID("").
        WithTimeout(30 * time.Second).
        ExecuteWithResult(context.Background())
    
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Status: %d\n", result.StatusCode)
    fmt.Printf("Duration: %v\n", result.Duration)
    fmt.Printf("Body: %s\n", string(result.Body))
}
```

## 9. Testing

```go
func TestRequestBuilder(t *testing.T) {
    t.Run("build_request", func(t *testing.T) {
        builder := NewBuilder("https://api.example.com", "/pods/{name}", "GET")
        
        req, err := builder.
            WithPathParam("name", "test-pod").
            WithQueryParam("namespace", "default").
            WithAuth("token123").
            Build()
        
        require.NoError(t, err)
        assert.Equal(t, "https://api.example.com/pods/test-pod?namespace=default", req.URL.String())
        assert.Equal(t, "Bearer token123", req.Header.Get("Authorization"))
    })
    
    t.Run("serialize_deep_object", func(t *testing.T) {
        type Filter struct {
            Name   string `json:"name"`
            Status string `json:"status"`
        }
        
        builder := NewBuilder("https://api.example.com", "/pods", "GET")
        builder.WithDeepObjectParam("filter", Filter{Name: "web", Status: "running"})
        
        req, err := builder.Build()
        require.NoError(t, err)
        
        query := req.URL.Query()
        assert.Equal(t, "web", query.Get("filter[name]"))
        assert.Equal(t, "running", query.Get("filter[status]"))
    })
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC

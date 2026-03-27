package request

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type Builder struct {
	baseURL         string
	path            string
	method          string
	pathParams      map[string]string
	queryParams     url.Values
	headers         http.Header
	body            io.Reader
	contentType     string
	timeout         time.Duration
	maxResponseBody int64
	buildError      error
}

const defaultMaxResponseBody = 10 << 20 // 10 MB

func NewBuilder(baseURL, path, method string) *Builder {
	return &Builder{
		baseURL:         strings.TrimRight(baseURL, "/"),
		path:            path,
		method:          strings.ToUpper(method),
		pathParams:      make(map[string]string),
		queryParams:     make(url.Values),
		headers:         make(http.Header),
		timeout:         30 * time.Second,
		maxResponseBody: defaultMaxResponseBody,
	}
}

func (b *Builder) WithPathParam(name, value string) *Builder {
	b.pathParams[name] = value
	return b
}

func (b *Builder) WithQueryParam(name, value string) *Builder {
	if value != "" {
		b.queryParams.Add(name, value)
	}
	return b
}

func (b *Builder) WithHeader(key, value string) *Builder {
	b.headers.Set(key, value)
	return b
}

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
			b.buildError = fmt.Errorf("marshal body: %w", err)
			return b
		}
		b.body = bytes.NewReader(data)
		b.contentType = "application/json"
	}
	return b
}

func (b *Builder) WithAuth(token string) *Builder {
	b.headers.Set("Authorization", "Bearer "+token)
	return b
}

func (b *Builder) WithBasicAuth(username, password string) *Builder {
	b.headers.Set("Authorization", "Basic "+b.basicAuth(username, password))
	return b
}

func (b *Builder) WithAPIKey(key, value string) *Builder {
	b.headers.Set(key, value)
	return b
}

func (b *Builder) WithMaxResponseBody(maxBytes int64) *Builder {
	b.maxResponseBody = maxBytes
	return b
}

func (b *Builder) basicAuth(username, password string) string {
	credentials := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(credentials))
}

func (b *Builder) JSONBody(data interface{}) *Builder {
	jsonData, err := json.Marshal(data)
	if err != nil {
		b.buildError = fmt.Errorf("marshal JSON: %w", err)
		return b
	}

	b.body = bytes.NewReader(jsonData)
	b.contentType = "application/json"
	return b
}

func (b *Builder) FormBody(data map[string]string) *Builder {
	form := url.Values{}
	for key, value := range data {
		form.Set(key, value)
	}

	b.body = strings.NewReader(form.Encode())
	b.contentType = "application/x-www-form-urlencoded"
	return b
}

func (b *Builder) MultipartBody(fields map[string]string, files map[string]string) (*Builder, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, fmt.Errorf("write field %s: %w", key, err)
		}
	}

	for key, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open file %s: %w", path, err)
		}

		part, err := writer.CreateFormFile(key, filepath.Base(path))
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("create form file: %w", err)
		}

		if _, err := io.Copy(part, file); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("copy file: %w", err)
		}
		_ = file.Close()
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	b.body = &buf
	b.contentType = writer.FormDataContentType()
	return b, nil
}

func (b *Builder) XMLBody(data interface{}) *Builder {
	xmlData, err := xml.Marshal(data)
	if err != nil {
		b.buildError = fmt.Errorf("marshal XML: %w", err)
		return b
	}

	b.body = bytes.NewReader(xmlData)
	b.contentType = "application/xml"
	return b
}

func (b *Builder) PlainTextBody(text string) *Builder {
	b.body = strings.NewReader(text)
	b.contentType = "text/plain"
	return b
}

func (b *Builder) WithTimeout(timeout time.Duration) *Builder {
	b.timeout = timeout
	return b
}

func (b *Builder) Build() (*http.Request, error) {
	finalPath := b.buildPath()
	fullURL := b.baseURL + finalPath

	if len(b.queryParams) > 0 {
		fullURL += "?" + b.queryParams.Encode()
	}

	req, err := http.NewRequest(b.method, fullURL, b.body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	for key, values := range b.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	if b.body != nil && b.contentType != "" {
		req.Header.Set("Content-Type", b.contentType)
	}

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
	if b.buildError != nil {
		return nil, b.buildError
	}

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

type ExecutionResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
}

func (b *Builder) ExecuteWithResult(ctx context.Context) (*ExecutionResult, error) {
	if b.buildError != nil {
		return nil, b.buildError
	}

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
	defer func() { _ = resp.Body.Close() }()

	limitedReader := io.LimitReader(resp.Body, b.maxResponseBody)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if int64(len(body)) >= b.maxResponseBody {
		buffer := make([]byte, 1)
		n, _ := resp.Body.Read(buffer)
		if n > 0 {
			return nil, fmt.Errorf("response body exceeds max size of %d bytes", b.maxResponseBody)
		}
	}

	return &ExecutionResult{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
		Duration:   time.Since(start),
	}, nil
}

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
		result.Set(name, fmt.Sprintf("%v", value))
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

		if !field.CanInterface() {
			continue
		}

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

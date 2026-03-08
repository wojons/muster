package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestResource_Size(t *testing.T) {
	size := int64(1024)
	res := Resource{
		URI:      "test://resource",
		Name:     "Test",
		MimeType: "text/plain",
		Size:     &size,
	}

	if res.Size == nil || *res.Size != 1024 {
		t.Errorf("expected size 1024, got %v", res.Size)
	}
}

func TestResource_Annotations(t *testing.T) {
	annotations := map[string]interface{}{
		"priority": "high",
		"owner":    "test-user",
	}
	res := Resource{
		URI:         "test://resource",
		Name:        "Test",
		Annotations: annotations,
	}

	if res.Annotations == nil {
		t.Fatal("expected annotations, got nil")
	}
	if res.Annotations["priority"] != "high" {
		t.Errorf("expected priority high, got %v", res.Annotations["priority"])
	}
}

func TestResourceContents_Blob(t *testing.T) {
	data := []byte("binary data")
	rc := ResourceContents{
		URI:      "test://binary",
		MimeType: "application/octet-stream",
		Blob:     base64.StdEncoding.EncodeToString(data),
	}

	if rc.Blob == "" {
		t.Error("expected blob data, got empty")
	}
	if rc.Text != "" {
		t.Error("expected no text for binary data")
	}

	decoded, err := base64.StdEncoding.DecodeString(rc.Blob)
	if err != nil {
		t.Fatalf("failed to decode blob: %v", err)
	}
	if string(decoded) != "binary data" {
		t.Errorf("expected 'binary data', got %s", decoded)
	}
}

func TestResourceTemplate(t *testing.T) {
	template := ResourceTemplate{
		URITemplate: "market://inventory/{module}/{operation}",
		Name:        "Inventory Operation",
		Description: "Details for a specific inventory operation",
		MimeType:    "application/json",
		Annotations: map[string]interface{}{
			"category": "inventory",
		},
	}

	if template.URITemplate != "market://inventory/{module}/{operation}" {
		t.Errorf("unexpected URI template: %s", template.URITemplate)
	}
}

func TestListTemplatesHandler(t *testing.T) {
	templates := []ResourceTemplate{
		{
			URITemplate: "test://item/{id}",
			Name:        "Test Item",
			MimeType:    "application/json",
		},
	}

	handler := NewListTemplatesHandler(templates)
	if handler.Method() != "resources/templates/list" {
		t.Errorf("expected method 'resources/templates/list', got %s", handler.Method())
	}

	result, err := handler.Handle(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listResult, ok := result.(*ListResourceTemplatesResult)
	if !ok {
		t.Fatal("expected ListResourceTemplatesResult")
	}

	if len(listResult.ResourceTemplates) != 1 {
		t.Errorf("expected 1 template, got %d", len(listResult.ResourceTemplates))
	}
}

func TestResourceManager_RegisterResource(t *testing.T) {
	rm := NewResourceManager()

	res := Resource{
		URI:      "test://resource/1",
		Name:     "Test Resource",
		MimeType: "text/plain",
	}

	rm.RegisterResource(res)

	if len(rm.resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(rm.resources))
	}

	if rm.resources["test://resource/1"].Name != "Test Resource" {
		t.Error("resource not registered correctly")
	}
}

func TestResourceManager_RegisterProvider(t *testing.T) {
	rm := NewResourceManager()

	provider := &testResourceProvider{
		resources: []ResourceInfo{
			{URI: "provider://resource/1", Name: "Provider Resource"},
		},
	}

	rm.RegisterProvider(provider)

	if len(rm.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(rm.providers))
	}
}

func TestResourceManager_ListResources(t *testing.T) {
	rm := NewResourceManager()

	rm.RegisterResource(Resource{
		URI:  "test://static/1",
		Name: "Static Resource",
	})

	rm.RegisterProvider(&testResourceProvider{
		resources: []ResourceInfo{
			{URI: "provider://dynamic/1", Name: "Dynamic Resource"},
		},
	})

	resources, err := rm.ListResources(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}
}

func TestResourceManager_ReadResource(t *testing.T) {
	rm := NewResourceManager()

	rm.RegisterProvider(&testResourceProvider{
		resources: []ResourceInfo{
			{URI: "provider://resource/1", Name: "Test"},
		},
		data: map[string]*StoredResource{
			"provider://resource/1": {
				Data:     []byte("test content"),
				MimeType: "text/plain",
			},
		},
	})

	content, err := rm.ReadResource(context.Background(), "provider://resource/1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if content.Text != "test content" {
		t.Errorf("expected 'test content', got %s", content.Text)
	}
}

func TestResourceManager_ReadResource_Binary(t *testing.T) {
	rm := NewResourceManager()

	binaryData := []byte{0x00, 0x01, 0x02, 0x03}
	rm.RegisterProvider(&testResourceProvider{
		resources: []ResourceInfo{
			{URI: "provider://binary/1"},
		},
		data: map[string]*StoredResource{
			"provider://binary/1": {
				Data:     binaryData,
				MimeType: "application/octet-stream",
			},
		},
	})

	content, err := rm.ReadResource(context.Background(), "provider://binary/1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if content.Text != "" {
		t.Error("expected no text for binary resource")
	}
	if content.Blob == "" {
		t.Error("expected base64 blob for binary resource")
	}
}

func TestResourceManager_ListTemplates(t *testing.T) {
	rm := NewResourceManager()

	template := ResourceTemplate{
		URITemplate: "test://item/{id}",
		Name:        "Item Template",
	}

	rm.RegisterTemplate(template)

	templates := rm.ListTemplates()
	if len(templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(templates))
	}
}

func TestResourceSubscriber_Subscribe(t *testing.T) {
	rs := NewResourceSubscriber(nil)

	id1 := &RequestID{Value: "client-1"}
	rs.Subscribe("test://resource/1", id1)

	if len(rs.subscribers["test://resource/1"]) != 1 {
		t.Errorf("expected 1 subscriber, got %d", len(rs.subscribers["test://resource/1"]))
	}
}

func TestResourceSubscriber_Unsubscribe(t *testing.T) {
	rs := NewResourceSubscriber(nil)

	id1 := &RequestID{Value: "client-1"}
	rs.Subscribe("test://resource/1", id1)
	rs.Unsubscribe("test://resource/1", id1)

	if len(rs.subscribers["test://resource/1"]) != 0 {
		t.Errorf("expected 0 subscribers, got %d", len(rs.subscribers["test://resource/1"]))
	}
}

func TestResourceSubscriber_NotifyUpdate(t *testing.T) {
	var notifications []struct {
		method string
		uri    string
	}

	rs := NewResourceSubscriber(func(method string, params interface{}, id *RequestID) {
		if notification, ok := params.(ResourceUpdatedNotification); ok {
			notifications = append(notifications, struct {
				method string
				uri    string
			}{method: method, uri: notification.URI})
		}
	})

	id1 := &RequestID{Value: "client-1"}
	rs.Subscribe("test://resource/1", id1)
	rs.NotifyUpdate("test://resource/1")

	if len(notifications) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].method != "notifications/resources/updated" {
		t.Errorf("expected method 'notifications/resources/updated', got %s", notifications[0].method)
	}
}

func TestResourceSubscriber_NotifyListChanged(t *testing.T) {
	var notifications []string

	rs := NewResourceSubscriber(func(method string, params interface{}, id *RequestID) {
		notifications = append(notifications, method)
	})

	id1 := &RequestID{Value: "client-1"}
	id2 := &RequestID{Value: "client-2"}
	rs.Subscribe("test://resource/1", id1)
	rs.Subscribe("test://resource/2", id2)

	rs.NotifyListChanged()

	if len(notifications) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifications))
	}
}

func TestSubscribeHandler_Handle(t *testing.T) {
	rs := NewResourceSubscriber(nil)
	handler := NewSubscribeHandler(rs)

	id := &RequestID{Value: "client-1"}
	params := SubscribeRequest{
		URI: "test://resource/1",
		ID:  id,
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}

	_, err = handler.Handle(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUnsubscribeHandler_Handle(t *testing.T) {
	rs := NewResourceSubscriber(nil)
	handler := NewUnsubscribeHandler(rs)

	id := &RequestID{Value: "client-1"}
	params := UnsubscribeRequest{
		URI: "test://resource/1",
		ID:  id,
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}

	_, err = handler.Handle(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestIsBinaryMimeType(t *testing.T) {
	tests := []struct {
		mimeType string
		expected bool
	}{
		{"application/octet-stream", true},
		{"application/pdf", true},
		{"application/zip", true},
		{"image/png", true},
		{"image/jpeg", true},
		{"video/mp4", true},
		{"audio/mp3", true},
		{"text/plain", false},
		{"application/json", false},
		{"text/html", false},
	}

	for _, tt := range tests {
		result := isBinaryMimeType(tt.mimeType)
		if result != tt.expected {
			t.Errorf("isBinaryMimeType(%s) = %v, expected %v", tt.mimeType, result, tt.expected)
		}
	}
}

type testResourceProvider struct {
	resources []ResourceInfo
	data      map[string]*StoredResource
}

func (m *testResourceProvider) GetResource(uri string) (*StoredResource, error) {
	if m.data == nil {
		return nil, nil
	}
	return m.data[uri], nil
}

func (m *testResourceProvider) ListResources() ([]ResourceInfo, error) {
	return m.resources, nil
}

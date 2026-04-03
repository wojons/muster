package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PagerdutyWebhook handles PagerDuty webhook notifications
type PagerdutyWebhook struct {
	config     PagerdutyConfig
	httpClient *http.Client
}

// PagerdutyEvent represents a PagerDuty v2 event
type PagerdutyEvent struct {
	RoutingKey  string                `json:"routing_key"`
	EventAction string                `json:"event_action"`
	DedupKey    string                `json:"dedup_key,omitempty"`
	Payload     PagerdutyEventPayload `json:"payload"`
	Images      []PagerdutyImage      `json:"images,omitempty"`
	Links       []PagerdutyLink       `json:"links,omitempty"`
}

// PagerdutyEventPayload represents the payload of a PagerDuty event
type PagerdutyEventPayload struct {
	Summary       string                 `json:"summary"`
	Source        string                 `json:"source"`
	Severity      string                 `json:"severity"`
	Component     string                 `json:"component,omitempty"`
	Group         string                 `json:"group,omitempty"`
	Class         string                 `json:"class,omitempty"`
	Timestamp     time.Time              `json:"timestamp,omitempty"`
	CustomDetails map[string]interface{} `json:"custom_details,omitempty"`
}

// PagerdutyResponse represents a PagerDuty API response
type PagerdutyResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	DedupKey string `json:"dedup_key"`
}

// NewPagerdutyWebhook creates a new PagerDuty webhook handler
func NewPagerdutyWebhook(config PagerdutyConfig) *PagerdutyWebhook {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &PagerdutyWebhook{
		config:     config,
		httpClient: client,
	}
}

// NewPagerdutyWebhookWithClient creates a new PagerDuty webhook handler with custom HTTP client
func NewPagerdutyWebhookWithClient(config PagerdutyConfig, client *http.Client) *PagerdutyWebhook {
	return &PagerdutyWebhook{
		config:     config,
		httpClient: client,
	}
}

// Send sends a notification for the given alert group
func (p *PagerdutyWebhook) Send(payload *AlertManagerWebhookPayload) error {
	if !p.shouldSend(payload) {
		return nil
	}

	for _, alert := range payload.Alerts {
		event := p.buildEvent(alert, payload)
		if err := p.sendEvent(event); err != nil {
			return fmt.Errorf("failed to send PagerDuty event: %w", err)
		}
	}

	return nil
}

// shouldSend determines if the notification should be sent based on the configuration
func (p *PagerdutyWebhook) shouldSend(payload *AlertManagerWebhookPayload) bool {
	// Don't send resolved notifications unless configured to do so
	if payload.Status == AlertStatusResolved && !p.config.SendResolved {
		return false
	}
	return true
}

// buildEvent builds a PagerDuty event from an alert
func (p *PagerdutyWebhook) buildEvent(alert Alert, payload *AlertManagerWebhookPayload) *PagerdutyEvent {
	// Build custom details
	customDetails := make(map[string]interface{})
	customDetails["group_key"] = payload.GroupKey
	customDetails["receiver"] = payload.Receiver

	// Add all labels as custom details
	for key, value := range alert.Labels {
		if key != "alertname" && key != "severity" {
			customDetails[key] = value
		}
	}

	// Add annotations
	for key, value := range alert.Annotations {
		customDetails[key] = value
	}

	// Determine event action
	var eventAction string
	var dedupKey string
	if alert.Status == AlertStatusFiring {
		eventAction = "trigger"
		// Generate dedup key from alert labels
		data, _ := json.Marshal(alert.Labels)
		dedupKey = string(data)
	} else {
		eventAction = "resolve"
		// Use the same dedup key that was used for the trigger
		data, _ := json.Marshal(alert.Labels)
		dedupKey = string(data)
	}

	// Determine severity
	severity := p.getSeverity(alert)

	// Build routing key
	routingKey := p.config.RoutingKey
	if routingKey == "" {
		routingKey = p.config.ServiceKey
	}

	event := &PagerdutyEvent{
		RoutingKey:  routingKey,
		EventAction: eventAction,
		DedupKey:    dedupKey,
		Payload: PagerdutyEventPayload{
			Summary:       p.getSummary(alert),
			Source:        p.getSource(alert),
			Severity:      severity,
			Component:     p.getComponent(alert),
			Group:         p.getGroup(alert),
			Class:         p.getClass(alert),
			Timestamp:     alert.StartsAt,
			CustomDetails: customDetails,
		},
	}

	// Add images
	if len(p.config.Images) > 0 {
		event.Images = p.config.Images
	}

	// Add links
	if len(p.config.Links) > 0 {
		event.Links = p.config.Links
	}

	// Add external URL if available
	if payload.ExternalURL != "" {
		event.Links = append(event.Links, PagerdutyLink{
			Href: payload.ExternalURL,
			Text: "AlertManager",
		})
	}

	return event
}

// getSeverity returns the PagerDuty severity for an alert
func (p *PagerdutyWebhook) getSeverity(alert Alert) string {
	// Use configured severity if set
	if p.config.Severity != "" {
		return p.config.Severity
	}

	// Map AlertManager severity to PagerDuty severity
	severity := alert.Labels["severity"]
	switch severity {
	case "critical":
		return "critical"
	case "high":
		return "error"
	case "warning":
		return "warning"
	case "info":
		return "info"
	default:
		return "error"
	}
}

// getSummary returns the summary for the alert
func (p *PagerdutyWebhook) getSummary(alert Alert) string {
	if p.config.Description != "" {
		return p.config.Description
	}
	return alert.GetSummary()
}

// getSource returns the source for the alert
func (p *PagerdutyWebhook) getSource(alert Alert) string {
	if p.config.Source != "" {
		return p.config.Source
	}

	// Try to get source from labels
	if source, ok := alert.Labels["instance"]; ok {
		return source
	}
	if source, ok := alert.Labels["job"]; ok {
		return source
	}

	return "AlertManager"
}

// getComponent returns the component for the alert
func (p *PagerdutyWebhook) getComponent(alert Alert) string {
	if p.config.Component != "" {
		return p.config.Component
	}

	if component, ok := alert.Labels["component"]; ok {
		return component
	}

	return alert.Labels["alertname"]
}

// getGroup returns the group for the alert
func (p *PagerdutyWebhook) getGroup(alert Alert) string {
	if p.config.Group != "" {
		return p.config.Group
	}

	if group, ok := alert.Labels["group"]; ok {
		return group
	}

	return ""
}

// getClass returns the class for the alert
func (p *PagerdutyWebhook) getClass(alert Alert) string {
	if p.config.Class != "" {
		return p.config.Class
	}

	if class, ok := alert.Labels["class"]; ok {
		return class
	}

	return ""
}

// sendEvent sends a single event to PagerDuty
func (p *PagerdutyWebhook) sendEvent(event *PagerdutyEvent) error {
	url := p.config.URL
	if url == "" {
		url = "https://events.pagerduty.com/v2/enqueue"
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal PagerDuty event: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create PagerDuty request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send PagerDuty event: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("PagerDuty returned non-OK status: %d", resp.StatusCode)
	}

	// Parse response
	var pdResp PagerdutyResponse
	if err := json.NewDecoder(resp.Body).Decode(&pdResp); err != nil {
		// Not critical, just log it
		return nil
	}

	if pdResp.Status != "success" {
		return fmt.Errorf("PagerDuty returned error status: %s - %s", pdResp.Status, pdResp.Message)
	}

	return nil
}

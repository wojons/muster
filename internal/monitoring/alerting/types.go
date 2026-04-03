package alerting

import (
	"encoding/json"
	"time"
)

// AlertStatus represents the status of an alert
type AlertStatus string

const (
	// AlertStatusFiring means the alert is active
	AlertStatusFiring AlertStatus = "firing"
	// AlertStatusResolved means the alert is resolved
	AlertStatusResolved AlertStatus = "resolved"
)

// Severity represents the severity level of an alert
type Severity string

const (
	// SeverityCritical requires immediate action
	SeverityCritical Severity = "critical"
	// SeverityHigh requires attention within a few hours
	SeverityHigh Severity = "high"
	// SeverityWarning requires monitoring
	SeverityWarning Severity = "warning"
	// SeverityInfo is informational
	SeverityInfo Severity = "info"
)

// Alert represents a single alert in the AlertManager format
type Alert struct {
	Status       AlertStatus       `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
	Fingerprint  string            `json:"fingerprint,omitempty"`
}

// AlertGroup represents a group of alerts sent together
type AlertGroup struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            AlertStatus       `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL,omitempty"`
	Alerts            []Alert           `json:"alerts"`
}

// AlertManagerWebhookPayload represents the payload sent to webhook receivers
type AlertManagerWebhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            AlertStatus       `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// NewAlert creates a new alert with the given parameters
func NewAlert(name string, severity Severity, summary, description string, labels map[string]string) *Alert {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["alertname"] = name
	labels["severity"] = string(severity)

	return &Alert{
		Status: AlertStatusFiring,
		Labels: labels,
		Annotations: map[string]string{
			"summary":     summary,
			"description": description,
		},
		StartsAt: time.Now(),
	}
}

// Resolved marks the alert as resolved
func (a *Alert) Resolved() *Alert {
	a.Status = AlertStatusResolved
	a.EndsAt = time.Now()
	return a
}

// GetFingerprint calculates a unique fingerprint for the alert
func (a *Alert) GetFingerprint() string {
	if a.Fingerprint != "" {
		return a.Fingerprint
	}

	// Simple hash of labels
	data, _ := json.Marshal(a.Labels)
	return string(data)
}

// IsCritical returns true if the alert is critical severity
func (a *Alert) IsCritical() bool {
	return a.Labels["severity"] == string(SeverityCritical)
}

// IsHigh returns true if the alert is high severity
func (a *Alert) IsHigh() bool {
	return a.Labels["severity"] == string(SeverityHigh)
}

// GetSummary returns the summary annotation or a default
func (a *Alert) GetSummary() string {
	if summary, ok := a.Annotations["summary"]; ok {
		return summary
	}
	return "Alert: " + a.Labels["alertname"]
}

// GetDescription returns the description annotation or empty string
func (a *Alert) GetDescription() string {
	return a.Annotations["description"]
}

// GetDuration returns how long the alert has been active
func (a *Alert) GetDuration() time.Duration {
	if a.Status == AlertStatusResolved && !a.EndsAt.IsZero() {
		return a.EndsAt.Sub(a.StartsAt)
	}
	return time.Since(a.StartsAt)
}

// AlertGroupBuilder helps build alert groups
type AlertGroupBuilder struct {
	group *AlertGroup
}

// NewAlertGroupBuilder creates a new AlertGroupBuilder
func NewAlertGroupBuilder(receiver string) *AlertGroupBuilder {
	return &AlertGroupBuilder{
		group: &AlertGroup{
			Version:           "4",
			Receiver:          receiver,
			GroupLabels:       make(map[string]string),
			CommonLabels:      make(map[string]string),
			CommonAnnotations: make(map[string]string),
			Alerts:            make([]Alert, 0),
		},
	}
}

// WithGroupKey sets the group key
func (b *AlertGroupBuilder) WithGroupKey(key string) *AlertGroupBuilder {
	b.group.GroupKey = key
	return b
}

// WithGroupLabels sets group labels
func (b *AlertGroupBuilder) WithGroupLabels(labels map[string]string) *AlertGroupBuilder {
	b.group.GroupLabels = labels
	return b
}

// WithCommonLabels sets common labels
func (b *AlertGroupBuilder) WithCommonLabels(labels map[string]string) *AlertGroupBuilder {
	b.group.CommonLabels = labels
	return b
}

// WithCommonAnnotations sets common annotations
func (b *AlertGroupBuilder) WithCommonAnnotations(annotations map[string]string) *AlertGroupBuilder {
	b.group.CommonAnnotations = annotations
	return b
}

// WithExternalURL sets the external URL
func (b *AlertGroupBuilder) WithExternalURL(url string) *AlertGroupBuilder {
	b.group.ExternalURL = url
	return b
}

// AddAlert adds an alert to the group
func (b *AlertGroupBuilder) AddAlert(alert Alert) *AlertGroupBuilder {
	b.group.Alerts = append(b.group.Alerts, alert)

	// Update status based on alerts
	hasFiring := false
	hasResolved := false
	for _, a := range b.group.Alerts {
		if a.Status == AlertStatusFiring {
			hasFiring = true
		} else {
			hasResolved = true
		}
	}

	if hasFiring {
		b.group.Status = AlertStatusFiring
	} else if hasResolved {
		b.group.Status = AlertStatusResolved
	}

	return b
}

// Build returns the built AlertGroup
func (b *AlertGroupBuilder) Build() *AlertGroup {
	return b.group
}

// CountBySeverity counts alerts by severity
func (g *AlertGroup) CountBySeverity() map[Severity]int {
	counts := make(map[Severity]int)
	for _, alert := range g.Alerts {
		severity := Severity(alert.Labels["severity"])
		counts[severity]++
	}
	return counts
}

// HasCritical returns true if any alert is critical
func (g *AlertGroup) HasCritical() bool {
	for _, alert := range g.Alerts {
		if alert.IsCritical() {
			return true
		}
	}
	return false
}

// GetCriticalAlerts returns only critical alerts
func (g *AlertGroup) GetCriticalAlerts() []Alert {
	var critical []Alert
	for _, alert := range g.Alerts {
		if alert.IsCritical() {
			critical = append(critical, alert)
		}
	}
	return critical
}

// ToWebhookPayload converts the alert group to a webhook payload
func (g *AlertGroup) ToWebhookPayload() *AlertManagerWebhookPayload {
	return &AlertManagerWebhookPayload{
		Version:           g.Version,
		GroupKey:          g.GroupKey,
		Status:            g.Status,
		Receiver:          g.Receiver,
		GroupLabels:       g.GroupLabels,
		CommonLabels:      g.CommonLabels,
		CommonAnnotations: g.CommonAnnotations,
		ExternalURL:       g.ExternalURL,
		Alerts:            g.Alerts,
	}
}

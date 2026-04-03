package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackWebhook handles Slack webhook notifications
type SlackWebhook struct {
	config     SlackConfig
	httpClient *http.Client
}

// NewSlackWebhook creates a new Slack webhook handler
func NewSlackWebhook(config SlackConfig) *SlackWebhook {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &SlackWebhook{
		config:     config,
		httpClient: client,
	}
}

// NewSlackWebhookWithClient creates a new Slack webhook handler with custom HTTP client
func NewSlackWebhookWithClient(config SlackConfig, client *http.Client) *SlackWebhook {
	return &SlackWebhook{
		config:     config,
		httpClient: client,
	}
}

// Send sends a notification for the given alert group
func (s *SlackWebhook) Send(payload *AlertManagerWebhookPayload) error {
	if !s.shouldSend(payload) {
		return nil
	}

	message := s.buildMessage(payload)
	return s.sendToSlack(message)
}

// shouldSend determines if the notification should be sent based on the configuration
func (s *SlackWebhook) shouldSend(payload *AlertManagerWebhookPayload) bool {
	// Don't send resolved notifications unless configured to do so
	if payload.Status == AlertStatusResolved && !s.config.SendResolved {
		return false
	}
	return true
}

// SlackMessage represents a Slack message payload
type SlackMessage struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	IconURL     string            `json:"icon_url,omitempty"`
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment represents a Slack message attachment
type SlackAttachment struct {
	Color      string        `json:"color,omitempty"`
	Title      string        `json:"title,omitempty"`
	TitleLink  string        `json:"title_link,omitempty"`
	Pretext    string        `json:"pretext,omitempty"`
	Text       string        `json:"text,omitempty"`
	Fields     []SlackField  `json:"fields,omitempty"`
	Actions    []SlackAction `json:"actions,omitempty"`
	Footer     string        `json:"footer,omitempty"`
	Ts         int64         `json:"ts,omitempty"`
	MarkdownIn []string      `json:"mrkdwn_in,omitempty"`
}

// SlackField represents a field in a Slack attachment
type SlackField struct {
	Title string `json:"title,omitempty"`
	Value string `json:"value,omitempty"`
	Short bool   `json:"short,omitempty"`
}

// buildMessage builds a Slack message from the alert payload
func (s *SlackWebhook) buildMessage(payload *AlertManagerWebhookPayload) *SlackMessage {
	msg := &SlackMessage{
		Channel:   s.config.Channel,
		Username:  s.config.Username,
		IconEmoji: s.config.IconEmoji,
		IconURL:   s.config.IconURL,
	}

	if payload.Status == AlertStatusFiring {
		msg.Text = fmt.Sprintf("*%d firing alerts* in group %s", len(payload.Alerts), payload.GroupKey)
	} else {
		msg.Text = fmt.Sprintf("*%d resolved alerts* in group %s", len(payload.Alerts), payload.GroupKey)
	}

	attachment := s.buildAttachment(payload)
	msg.Attachments = []SlackAttachment{attachment}

	return msg
}

// buildAttachment builds the main attachment for the alert
func (s *SlackWebhook) buildAttachment(payload *AlertManagerWebhookPayload) SlackAttachment {
	var color string
	if payload.Status == AlertStatusFiring {
		color = s.getSeverityColor(payload.Alerts)
	} else {
		color = "#28a745" // Green for resolved
	}

	attachment := SlackAttachment{
		Color:      color,
		Title:      s.getTitle(payload),
		TitleLink:  s.config.TitleLink,
		Pretext:    s.config.Pretext,
		Ts:         time.Now().Unix(),
		MarkdownIn: []string{"text", "fields"},
	}

	// Build text content
	if s.config.Text != "" {
		attachment.Text = s.config.Text
	} else {
		attachment.Text = s.buildDefaultText(payload)
	}

	// Add fields
	attachment.Fields = s.buildFields(payload)

	return attachment
}

// getSeverityColor returns the color based on the highest severity alert
func (s *SlackWebhook) getSeverityColor(alerts []Alert) string {
	hasCritical := false
	hasHigh := false
	hasWarning := false

	for _, alert := range alerts {
		severity := alert.Labels["severity"]
		switch severity {
		case "critical":
			hasCritical = true
		case "high":
			hasHigh = true
		case "warning":
			hasWarning = true
		}
	}

	if s.config.Color != "" {
		return s.config.Color
	}

	switch {
	case hasCritical:
		return "#dc3545" // Red
	case hasHigh:
		return "#fd7e14" // Orange
	case hasWarning:
		return "#ffc107" // Yellow
	default:
		return "#17a2b8" // Blue
	}
}

// getTitle returns the title for the attachment
func (s *SlackWebhook) getTitle(payload *AlertManagerWebhookPayload) string {
	if s.config.Title != "" {
		return s.config.Title
	}

	if payload.Status == AlertStatusFiring {
		return fmt.Sprintf("%s: Alerts Firing", payload.Receiver)
	}
	return fmt.Sprintf("%s: Alerts Resolved", payload.Receiver)
}

// buildDefaultText builds the default text for the attachment
func (s *SlackWebhook) buildDefaultText(payload *AlertManagerWebhookPayload) string {
	var text string

	for _, alert := range payload.Alerts {
		summary := alert.GetSummary()
		if alert.Status == AlertStatusFiring {
			duration := alert.GetDuration()
			text += fmt.Sprintf("• %s (firing for %s)\n", summary, duration.Round(time.Second))
		} else {
			duration := alert.GetDuration()
			text += fmt.Sprintf("• %s (resolved after %s)\n", summary, duration.Round(time.Second))
		}
	}

	return text
}

// buildFields builds the fields for the attachment
func (s *SlackWebhook) buildFields(payload *AlertManagerWebhookPayload) []SlackField {
	fields := []SlackField{}

	// Severity counts
	severityCounts := make(map[string]int)
	for _, alert := range payload.Alerts {
		severity := alert.Labels["severity"]
		severityCounts[severity]++
	}

	for severity, count := range severityCounts {
		fields = append(fields, SlackField{
			Title: fmt.Sprintf("%s Alerts", severity),
			Value: fmt.Sprintf("%d", count),
			Short: true,
		})
	}

	// Common labels
	if len(payload.CommonLabels) > 0 {
		var commonText string
		for key, value := range payload.CommonLabels {
			if key != "alertname" && key != "severity" {
				commonText += fmt.Sprintf("%s: %s\n", key, value)
			}
		}
		if commonText != "" {
			fields = append(fields, SlackField{
				Title: "Common Labels",
				Value: commonText,
				Short: false,
			})
		}
	}

	// External URL
	if payload.ExternalURL != "" {
		fields = append(fields, SlackField{
			Title: "AlertManager",
			Value: fmt.Sprintf("\u003c%s|View in AlertManager\u003e", payload.ExternalURL),
			Short: false,
		})
	}

	return fields
}

// sendToSlack sends the message to Slack
func (s *SlackWebhook) sendToSlack(message *SlackMessage) error {
	url := s.config.APIURL
	if url == "" {
		return fmt.Errorf("slack API URL is required")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack message: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

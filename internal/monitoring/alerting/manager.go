package alerting

import (
	"fmt"
	"sync"
	"time"
)

// AlertManager manages alert routing and notification
type AlertManager struct {
	config      *AlertManagerConfig
	receivers   map[string]*Receiver
	webhooks    map[string]NotificationHandler
	mu          sync.RWMutex
	sendHistory []NotificationRecord
	maxHistory  int
	logger      Logger
}

// Logger interface for logging alert events
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, err error, keysAndValues ...interface{})
}

// NotificationHandler is the interface for notification handlers
type NotificationHandler interface {
	Send(payload *AlertManagerWebhookPayload) error
}

// NotificationRecord tracks a sent notification
type NotificationRecord struct {
	Timestamp  time.Time
	Receiver   string
	Status     AlertStatus
	AlertCount int
	Error      string
}

// defaultLogger implements a basic logger
type defaultLogger struct{}

func (l *defaultLogger) Debug(msg string, keysAndValues ...interface{})            {}
func (l *defaultLogger) Info(msg string, keysAndValues ...interface{})             {}
func (l *defaultLogger) Warn(msg string, keysAndValues ...interface{})             {}
func (l *defaultLogger) Error(msg string, err error, keysAndValues ...interface{}) {}

// NewAlertManager creates a new AlertManager with the given configuration
func NewAlertManager(config *AlertManagerConfig) (*AlertManager, error) {
	return NewAlertManagerWithLogger(config, nil)
}

// NewAlertManagerWithLogger creates a new AlertManager with custom logger
func NewAlertManagerWithLogger(config *AlertManagerConfig, logger Logger) (*AlertManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = &defaultLogger{}
	}

	am := &AlertManager{
		config:     config,
		receivers:  make(map[string]*Receiver),
		webhooks:   make(map[string]NotificationHandler),
		maxHistory: 1000,
		logger:     logger,
	}

	// Register receivers
	for _, receiver := range config.Receivers {
		am.receivers[receiver.Name] = &receiver

		// Initialize notification handlers
		for _, slackConfig := range receiver.SlackConfigs {
			am.webhooks[fmt.Sprintf("%s-slack", receiver.Name)] = NewSlackWebhook(slackConfig)
		}

		for _, pdConfig := range receiver.PagerdutyConfigs {
			am.webhooks[fmt.Sprintf("%s-pagerduty", receiver.Name)] = NewPagerdutyWebhook(pdConfig)
		}

		for _, whConfig := range receiver.WebhookConfigs {
			am.webhooks[fmt.Sprintf("%s-webhook", receiver.Name)] = NewWebhookHandler(whConfig)
		}
	}

	return am, nil
}

// ProcessAlertGroup processes a group of alerts and sends notifications
func (am *AlertManager) ProcessAlertGroup(group *AlertGroup) error {
	payload := group.ToWebhookPayload()

	// Find matching receivers through the routing tree
	receivers := am.findReceivers(am.config.Route, group)

	for _, receiverName := range receivers {
		if err := am.sendToReceiver(receiverName, payload); err != nil {
			am.logger.Error("Failed to send notification",
				err,
				"receiver", receiverName,
				"group", group.GroupKey)
			// Continue with other receivers
		}
	}

	return nil
}

// SendAlert sends a single alert
func (am *AlertManager) SendAlert(alert *Alert, receiver string) error {
	group := NewAlertGroupBuilder(receiver).
		WithGroupKey(alert.GetFingerprint()).
		AddAlert(*alert).
		Build()

	return am.ProcessAlertGroup(group)
}

// findReceivers recursively finds all receivers that match the alert group
func (am *AlertManager) findReceivers(route *Route, group *AlertGroup) []string {
	var receivers []string

	// Check if this route matches
	if am.matches(route, group) {
		receivers = append(receivers, route.Receiver)

		// Check children only if continue is true
		if route.Continue {
			for _, child := range route.Routes {
				childReceivers := am.findReceivers(child, group)
				receivers = append(receivers, childReceivers...)
			}
		}
	}

	// Always check children for matching routes
	for _, child := range route.Routes {
		childReceivers := am.findReceivers(child, group)
		receivers = append(receivers, childReceivers...)
	}

	return receivers
}

// matches checks if a route matches an alert group
func (am *AlertManager) matches(route *Route, group *AlertGroup) bool {
	// Check matchers
	for key, value := range route.Match {
		if groupValue, ok := group.GroupLabels[key]; !ok || groupValue != value {
			return false
		}
	}

	// Check matchers with regex (simplified - just direct match for now)
	for key, pattern := range route.MatchRE {
		if groupValue, ok := group.GroupLabels[key]; !ok || groupValue != pattern {
			return false
		}
	}

	return true
}

// sendToReceiver sends the payload to a specific receiver
func (am *AlertManager) sendToReceiver(receiverName string, payload *AlertManagerWebhookPayload) error {
	_, ok := am.receivers[receiverName]
	if !ok {
		return fmt.Errorf("receiver not found: %s", receiverName)
	}

	// Try each notification handler type
	var lastErr error
	sent := false

	for handlerName, handler := range am.webhooks {
		// Only use handlers for this receiver
		if !startsWith(handlerName, receiverName+"-") {
			continue
		}

		if err := handler.Send(payload); err != nil {
			lastErr = err
			am.logger.Error("Failed to send notification via handler",
				err,
				"receiver", receiverName,
				"handler", handlerName)
		} else {
			sent = true
			am.logger.Info("Notification sent",
				"receiver", receiverName,
				"handler", handlerName,
				"alerts", len(payload.Alerts),
				"status", payload.Status)
		}
	}

	// Record the notification
	record := NotificationRecord{
		Timestamp:  time.Now(),
		Receiver:   receiverName,
		Status:     payload.Status,
		AlertCount: len(payload.Alerts),
	}
	if lastErr != nil {
		record.Error = lastErr.Error()
	}
	am.recordNotification(record)

	if !sent {
		return fmt.Errorf("no notification sent for receiver %s: %w", receiverName, lastErr)
	}

	return nil
}

// recordNotification records a notification in the history
func (am *AlertManager) recordNotification(record NotificationRecord) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.sendHistory = append(am.sendHistory, record)

	// Trim history if too large
	if len(am.sendHistory) > am.maxHistory {
		am.sendHistory = am.sendHistory[len(am.sendHistory)-am.maxHistory:]
	}
}

// GetSendHistory returns the notification history
func (am *AlertManager) GetSendHistory() []NotificationRecord {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]NotificationRecord, len(am.sendHistory))
	copy(result, am.sendHistory)
	return result
}

// GetStats returns alerting statistics
func (am *AlertManager) GetStats() map[string]interface{} {
	am.mu.RLock()
	defer am.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_notifications"] = len(am.sendHistory)

	// Count by receiver
	receiverCounts := make(map[string]int)
	receiverErrors := make(map[string]int)

	for _, record := range am.sendHistory {
		receiverCounts[record.Receiver]++
		if record.Error != "" {
			receiverErrors[record.Receiver]++
		}
	}

	stats["notifications_by_receiver"] = receiverCounts
	stats["errors_by_receiver"] = receiverErrors

	// Count by status
	firingCount := 0
	resolvedCount := 0
	for _, record := range am.sendHistory {
		if record.Status == AlertStatusFiring {
			firingCount++
		} else {
			resolvedCount++
		}
	}
	stats["firing_notifications"] = firingCount
	stats["resolved_notifications"] = resolvedCount

	return stats
}

// GetReceiver returns a receiver configuration by name
func (am *AlertManager) GetReceiver(name string) (*Receiver, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	receiver, ok := am.receivers[name]
	return receiver, ok
}

// ReloadConfig reloads the alert manager configuration
func (am *AlertManager) ReloadConfig(config *AlertManagerConfig) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	// Clear and rebuild
	am.config = config
	am.receivers = make(map[string]*Receiver)
	am.webhooks = make(map[string]NotificationHandler)

	// Register receivers
	for _, receiver := range config.Receivers {
		am.receivers[receiver.Name] = &receiver

		// Initialize notification handlers
		for _, slackConfig := range receiver.SlackConfigs {
			am.webhooks[fmt.Sprintf("%s-slack", receiver.Name)] = NewSlackWebhook(slackConfig)
		}

		for _, pdConfig := range receiver.PagerdutyConfigs {
			am.webhooks[fmt.Sprintf("%s-pagerduty", receiver.Name)] = NewPagerdutyWebhook(pdConfig)
		}

		for _, whConfig := range receiver.WebhookConfigs {
			am.webhooks[fmt.Sprintf("%s-webhook", receiver.Name)] = NewWebhookHandler(whConfig)
		}
	}

	am.logger.Info("AlertManager configuration reloaded",
		"receivers", len(config.Receivers))

	return nil
}

// startsWith checks if s starts with prefix
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

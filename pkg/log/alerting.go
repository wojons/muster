package log

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LogAlertRule defines a rule for triggering alerts based on log patterns
type LogAlertRule struct {
	ID            string
	Name          string
	Description   string
	Severity      AlertSeverity
	Condition     AlertCondition
	Actions       []AlertAction
	Enabled       bool
	Cooldown      time.Duration // Minimum time between alerts
	CreatedAt     time.Time
	LastTriggered time.Time
}

// AlertSeverity defines the severity of an alert
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityError    AlertSeverity = "error"
	SeverityCritical AlertSeverity = "critical"
)

// AlertCondition defines when an alert should trigger
type AlertCondition struct {
	Level           string            // Match log level (error, warn, etc.)
	MessageContains []string          // Match message containing any of these strings
	FieldMatch      map[string]string // Match field values (field > value)
	Threshold       int               // Trigger after N matching logs
	Window          time.Duration     // Time window for threshold
	RateThreshold   float64           // Trigger if rate > X per minute
}

// AlertAction defines what to do when an alert triggers
type AlertAction struct {
	Type   AlertActionType
	Config map[string]string
}

// AlertActionType defines the type of alert action
type AlertActionType string

const (
	ActionLog       AlertActionType = "log"       // Log the alert
	ActionWebhook   AlertActionType = "webhook"   // POST to webhook
	ActionSlack     AlertActionType = "slack"     // Send Slack message
	ActionPagerDuty AlertActionType = "pagerduty" // Create PagerDuty incident
	ActionEmail     AlertActionType = "email"     // Send email
)

// LogAlertManager manages log-based alerting
type LogAlertManager struct {
	rules     map[string]*LogAlertRule
	ruleMu    sync.RWMutex
	logCh     chan LogEntry
	stopCh    chan struct{}
	wg        sync.WaitGroup
	isRunning bool
	handlers  map[AlertActionType]AlertHandler
	handlerMu sync.RWMutex
}

// LogEntry represents a single log entry for alerting
type LogEntry struct {
	Timestamp     time.Time
	Level         string
	Message       string
	Fields        map[string]interface{}
	Service       string
	CorrelationID string
}

// AlertHandler handles alert actions
type AlertHandler func(ctx context.Context, alert Alert) error

// Alert represents a triggered alert
type Alert struct {
	RuleID      string
	RuleName    string
	Severity    AlertSeverity
	Message     string
	Details     map[string]interface{}
	TriggeredAt time.Time
	Count       int // Number of matching logs
}

// NewLogAlertManager creates a new log alert manager
func NewLogAlertManager() *LogAlertManager {
	return &LogAlertManager{
		rules:    make(map[string]*LogAlertRule),
		logCh:    make(chan LogEntry, 1000),
		stopCh:   make(chan struct{}),
		handlers: make(map[AlertActionType]AlertHandler),
	}
}

// Start begins the alert manager
func (m *LogAlertManager) Start() error {
	m.ruleMu.Lock()
	defer m.ruleMu.Unlock()

	if m.isRunning {
		return fmt.Errorf("alert manager already running")
	}

	m.isRunning = true
	m.wg.Add(1)
	go m.processLoop()

	return nil
}

// Stop stops the alert manager
func (m *LogAlertManager) Stop() error {
	m.ruleMu.Lock()
	if !m.isRunning {
		m.ruleMu.Unlock()
		return nil
	}
	m.isRunning = false
	close(m.stopCh)
	m.ruleMu.Unlock()

	m.wg.Wait()
	return nil
}

// RegisterHandler registers an alert handler
func (m *LogAlertManager) RegisterHandler(actionType AlertActionType, handler AlertHandler) {
	m.handlerMu.Lock()
	defer m.handlerMu.Unlock()
	m.handlers[actionType] = handler
}

// AddRule adds a new alert rule
func (m *LogAlertManager) AddRule(rule LogAlertRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if rule.Cooldown == 0 {
		rule.Cooldown = 5 * time.Minute
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}

	m.ruleMu.Lock()
	defer m.ruleMu.Unlock()

	m.rules[rule.ID] = &rule
	return nil
}

// RemoveRule removes an alert rule
func (m *LogAlertManager) RemoveRule(ruleID string) {
	m.ruleMu.Lock()
	defer m.ruleMu.Unlock()
	delete(m.rules, ruleID)
}

// GetRule returns an alert rule by ID
func (m *LogAlertManager) GetRule(ruleID string) (*LogAlertRule, bool) {
	m.ruleMu.RLock()
	defer m.ruleMu.RUnlock()
	rule, ok := m.rules[ruleID]
	return rule, ok
}

// GetRules returns all alert rules
func (m *LogAlertManager) GetRules() []*LogAlertRule {
	m.ruleMu.RLock()
	defer m.ruleMu.RUnlock()

	rules := make([]*LogAlertRule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}
	return rules
}

// SubmitLog submits a log entry for alert evaluation
func (m *LogAlertManager) SubmitLog(entry LogEntry) {
	select {
	case m.logCh <- entry:
	default:
		// Channel full, drop log
	}
}

// processLoop processes log entries
func (m *LogAlertManager) processLoop() {
	defer m.wg.Done()

	for {
		select {
		case entry := <-m.logCh:
			m.evaluateLog(entry)
		case <-m.stopCh:
			return
		}
	}
}

// evaluateLog evaluates a log entry against all rules
func (m *LogAlertManager) evaluateLog(entry LogEntry) {
	m.ruleMu.RLock()
	rules := make([]*LogAlertRule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}
	m.ruleMu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if m.matchesCondition(entry, rule.Condition) {
			m.triggerAlert(rule, entry)
		}
	}
}

// matchesCondition checks if a log entry matches the alert condition
func (m *LogAlertManager) matchesCondition(entry LogEntry, condition AlertCondition) bool {
	// Check level
	if condition.Level != "" && entry.Level != condition.Level {
		return false
	}

	// Check message contains
	if len(condition.MessageContains) > 0 {
		matched := false
		for _, substr := range condition.MessageContains {
			if contains(entry.Message, substr) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check field matches
	for field, value := range condition.FieldMatch {
		entryValue, ok := entry.Fields[field]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", entryValue) != value {
			return false
		}
	}

	return true
}

// triggerAlert triggers an alert
func (m *LogAlertManager) triggerAlert(rule *LogAlertRule, entry LogEntry) {
	// Check cooldown
	if !rule.LastTriggered.IsZero() && time.Since(rule.LastTriggered) < rule.Cooldown {
		return
	}

	// Update last triggered time
	rule.LastTriggered = time.Now()

	// Create alert
	alert := Alert{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Severity:    rule.Severity,
		Message:     entry.Message,
		Details:     entry.Fields,
		TriggeredAt: time.Now(),
		Count:       1,
	}

	// Execute actions
	for _, action := range rule.Actions {
		m.executeAction(action, alert)
	}
}

// executeAction executes an alert action
func (m *LogAlertManager) executeAction(action AlertAction, alert Alert) {
	m.handlerMu.RLock()
	handler, ok := m.handlers[action.Type]
	m.handlerMu.RUnlock()

	if !ok {
		// No handler registered, just log
		fmt.Printf("[ALERT] %s: %s (severity: %s)\n", alert.RuleName, alert.Message, alert.Severity)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := handler(ctx, alert); err != nil {
		// Log handler error
		fmt.Printf("[ALERT ERROR] Failed to execute action %s: %v\n", action.Type, err)
	}
}

// Common alert rules
func DefaultAlertRules() []LogAlertRule {
	return []LogAlertRule{
		{
			ID:          "high-error-rate",
			Name:        "High Error Rate",
			Description: "Triggers when error logs exceed threshold",
			Severity:    SeverityCritical,
			Condition: AlertCondition{
				Level:     "error",
				Threshold: 10,
				Window:    5 * time.Minute,
			},
			Actions: []AlertAction{
				{Type: ActionLog},
				{Type: ActionSlack, Config: map[string]string{"channel": "#alerts"}},
			},
			Enabled:  true,
			Cooldown: 5 * time.Minute,
		},
		{
			ID:          "authentication-failure",
			Name:        "Authentication Failure",
			Description: "Triggers on authentication failures",
			Severity:    SeverityWarning,
			Condition: AlertCondition{
				Level:           "error",
				MessageContains: []string{"auth", "login", "authentication"},
			},
			Actions: []AlertAction{
				{Type: ActionLog},
			},
			Enabled:  true,
			Cooldown: 1 * time.Minute,
		},
		{
			ID:          "database-error",
			Name:        "Database Error",
			Description: "Triggers on database errors",
			Severity:    SeverityCritical,
			Condition: AlertCondition{
				Level:           "error",
				MessageContains: []string{"database", "db", "sql"},
			},
			Actions: []AlertAction{
				{Type: ActionLog},
				{Type: ActionPagerDuty, Config: map[string]string{"service_key": "database"}},
			},
			Enabled:  true,
			Cooldown: 1 * time.Minute,
		},
	}
}

// contains checks if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		stringsContains(s, substr))
}

func stringsContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// AlertMetrics holds metrics for alert processing
type AlertMetrics struct {
	TotalLogs       int64
	MatchedLogs     int64
	TriggeredAlerts int64
	ActiveRules     int
}

// GetMetrics returns current alert metrics
func (m *LogAlertManager) GetMetrics() AlertMetrics {
	m.ruleMu.RLock()
	activeRules := len(m.rules)
	m.ruleMu.RUnlock()

	return AlertMetrics{
		ActiveRules: activeRules,
	}
}

// LogAlertConfig holds configuration for log alerting
type LogAlertConfig struct {
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	BufferSize      int           `json:"buffer_size" yaml:"buffer_size"`
	DefaultCooldown time.Duration `json:"default_cooldown" yaml:"default_cooldown"`
}

// DefaultLogAlertConfig returns default configuration
func DefaultLogAlertConfig() LogAlertConfig {
	return LogAlertConfig{
		Enabled:         true,
		BufferSize:      1000,
		DefaultCooldown: 5 * time.Minute,
	}
}

// LogAlertSummary provides a summary of alert activity
type LogAlertSummary struct {
	Period        time.Duration
	TotalAlerts   int
	BySeverity    map[AlertSeverity]int
	ByRule        map[string]int
	TopAlertRules []string
}

// GenerateSummary generates a summary of alert activity
func (m *LogAlertManager) GenerateSummary(period time.Duration) LogAlertSummary {
	// This would typically query a database of alert history
	// For now, return empty summary
	return LogAlertSummary{
		Period:     period,
		BySeverity: make(map[AlertSeverity]int),
		ByRule:     make(map[string]int),
	}
}

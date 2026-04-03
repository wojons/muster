package alerting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewAlert(t *testing.T) {
	labels := map[string]string{
		"instance": "localhost:9090",
		"job":      "muster",
	}

	alert := NewAlert("HighErrorRate", SeverityHigh, "High error rate detected", "Error rate above threshold", labels)

	assert.Equal(t, AlertStatusFiring, alert.Status)
	assert.Equal(t, "HighErrorRate", alert.Labels["alertname"])
	assert.Equal(t, "high", alert.Labels["severity"])
	assert.Equal(t, "localhost:9090", alert.Labels["instance"])
	assert.Equal(t, "muster", alert.Labels["job"])
	assert.Equal(t, "High error rate detected", alert.Annotations["summary"])
	assert.Equal(t, "Error rate above threshold", alert.Annotations["description"])
	assert.True(t, alert.StartsAt.After(time.Now().Add(-time.Second)))
	assert.True(t, alert.EndsAt.IsZero())
}

func TestNewAlert_NilLabels(t *testing.T) {
	alert := NewAlert("Test", SeverityWarning, "Test summary", "Test description", nil)

	assert.NotNil(t, alert.Labels)
	assert.Equal(t, "Test", alert.Labels["alertname"])
	assert.Equal(t, "warning", alert.Labels["severity"])
}

func TestAlert_Resolved(t *testing.T) {
	alert := NewAlert("Test", SeverityInfo, "Test summary", "Test description", nil)

	// Should be firing
	assert.Equal(t, AlertStatusFiring, alert.Status)
	assert.True(t, alert.EndsAt.IsZero())

	// Resolve it
	resolved := alert.Resolved()

	assert.Equal(t, AlertStatusResolved, resolved.Status)
	assert.False(t, resolved.EndsAt.IsZero())
}

func TestAlert_GetFingerprint(t *testing.T) {
	alert := NewAlert("Test", SeverityCritical, "Test", "Test", map[string]string{
		"instance": "localhost",
		"job":      "test",
	})

	fingerprint := alert.GetFingerprint()
	assert.NotEmpty(t, fingerprint)

	// Same labels should give same fingerprint
	alert2 := NewAlert("Test", SeverityCritical, "Test", "Test", map[string]string{
		"instance": "localhost",
		"job":      "test",
	})
	assert.Equal(t, fingerprint, alert2.GetFingerprint())

	// Different labels should give different fingerprint
	alert3 := NewAlert("Test", SeverityCritical, "Test", "Test", map[string]string{
		"instance": "other",
		"job":      "test",
	})
	assert.NotEqual(t, fingerprint, alert3.GetFingerprint())
}

func TestAlert_IsCritical(t *testing.T) {
	tests := []struct {
		severity   Severity
		isCritical bool
	}{
		{SeverityCritical, true},
		{SeverityHigh, false},
		{SeverityWarning, false},
		{SeverityInfo, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			alert := NewAlert("Test", tt.severity, "Test", "Test", nil)
			assert.Equal(t, tt.isCritical, alert.IsCritical())
		})
	}
}

func TestAlert_IsHigh(t *testing.T) {
	tests := []struct {
		severity Severity
		isHigh   bool
	}{
		{SeverityCritical, false},
		{SeverityHigh, true},
		{SeverityWarning, false},
		{SeverityInfo, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			alert := NewAlert("Test", tt.severity, "Test", "Test", nil)
			assert.Equal(t, tt.isHigh, alert.IsHigh())
		})
	}
}

func TestAlert_GetSummary(t *testing.T) {
	// With summary annotation
	alert := NewAlert("Test", SeverityWarning, "My Summary", "My Description", nil)
	assert.Equal(t, "My Summary", alert.GetSummary())

	// Without summary annotation
	alert.Labels["alertname"] = "NoSummary"
	delete(alert.Annotations, "summary")
	assert.Equal(t, "Alert: NoSummary", alert.GetSummary())
}

func TestAlert_GetDescription(t *testing.T) {
	alert := NewAlert("Test", SeverityWarning, "Summary", "My Description", nil)
	assert.Equal(t, "My Description", alert.GetDescription())

	delete(alert.Annotations, "description")
	assert.Equal(t, "", alert.GetDescription())
}

func TestAlert_GetDuration(t *testing.T) {
	alert := NewAlert("Test", SeverityWarning, "Test", "Test", nil)

	// Firing alert should have duration > 0
	duration := alert.GetDuration()
	assert.True(t, duration >= 0)
	assert.True(t, duration < time.Second)

	// Resolved alert
	alert = alert.Resolved()
	duration = alert.GetDuration()
	assert.True(t, duration >= 0)
	assert.True(t, duration < time.Second)
}

func TestAlertGroupBuilder(t *testing.T) {
	builder := NewAlertGroupBuilder("test-receiver")

	group := builder.
		WithGroupKey("test-group").
		WithGroupLabels(map[string]string{"alertname": "Test"}).
		WithCommonLabels(map[string]string{"instance": "localhost"}).
		WithCommonAnnotations(map[string]string{"summary": "Test summary"}).
		WithExternalURL("http://localhost:9093").
		AddAlert(*NewAlert("Test1", SeverityCritical, "Test1", "Test1", nil)).
		AddAlert(*NewAlert("Test2", SeverityHigh, "Test2", "Test2", nil)).
		Build()

	assert.Equal(t, "4", group.Version)
	assert.Equal(t, "test-group", group.GroupKey)
	assert.Equal(t, "test-receiver", group.Receiver)
	assert.Equal(t, AlertStatusFiring, group.Status) // Has firing alerts
	assert.Len(t, group.Alerts, 2)
}

func TestAlertGroupBuilder_Resolved(t *testing.T) {
	builder := NewAlertGroupBuilder("test-receiver")

	alert1 := NewAlert("Test1", SeverityCritical, "Test1", "Test1", nil)
	alert1 = alert1.Resolved()

	group := builder.
		AddAlert(*alert1).
		Build()

	assert.Equal(t, AlertStatusResolved, group.Status)
}

func TestAlertGroupBuilder_MixedStatus(t *testing.T) {
	builder := NewAlertGroupBuilder("test-receiver")

	alert1 := NewAlert("Test1", SeverityCritical, "Test1", "Test1", nil)
	alert2 := NewAlert("Test2", SeverityHigh, "Test2", "Test2", nil)
	alert2 = alert2.Resolved()

	group := builder.
		AddAlert(*alert1).
		AddAlert(*alert2).
		Build()

	// Has at least one firing, so status is firing
	assert.Equal(t, AlertStatusFiring, group.Status)
}

func TestAlertGroup_CountBySeverity(t *testing.T) {
	builder := NewAlertGroupBuilder("test-receiver")

	group := builder.
		AddAlert(*NewAlert("Alert1", SeverityCritical, "", "", nil)).
		AddAlert(*NewAlert("Alert2", SeverityCritical, "", "", nil)).
		AddAlert(*NewAlert("Alert3", SeverityHigh, "", "", nil)).
		AddAlert(*NewAlert("Alert4", SeverityWarning, "", "", nil)).
		Build()

	counts := group.CountBySeverity()

	assert.Equal(t, 2, counts[SeverityCritical])
	assert.Equal(t, 1, counts[SeverityHigh])
	assert.Equal(t, 1, counts[SeverityWarning])
	assert.Equal(t, 0, counts[SeverityInfo])
}

func TestAlertGroup_HasCritical(t *testing.T) {
	// No critical
	builder := NewAlertGroupBuilder("test-receiver")
	group := builder.
		AddAlert(*NewAlert("Alert1", SeverityHigh, "", "", nil)).
		Build()
	assert.False(t, group.HasCritical())

	// Has critical
	builder = NewAlertGroupBuilder("test-receiver")
	group = builder.
		AddAlert(*NewAlert("Alert1", SeverityCritical, "", "", nil)).
		Build()
	assert.True(t, group.HasCritical())
}

func TestAlertGroup_GetCriticalAlerts(t *testing.T) {
	builder := NewAlertGroupBuilder("test-receiver")

	group := builder.
		AddAlert(*NewAlert("Critical1", SeverityCritical, "", "", nil)).
		AddAlert(*NewAlert("Critical2", SeverityCritical, "", "", nil)).
		AddAlert(*NewAlert("High1", SeverityHigh, "", "", nil)).
		Build()

	criticalAlerts := group.GetCriticalAlerts()

	assert.Len(t, criticalAlerts, 2)
	for _, alert := range criticalAlerts {
		assert.Equal(t, string(SeverityCritical), alert.Labels["severity"])
	}
}

func TestAlertGroup_ToWebhookPayload(t *testing.T) {
	builder := NewAlertGroupBuilder("test-receiver")

	group := builder.
		WithGroupKey("test-group").
		WithGroupLabels(map[string]string{"alertname": "Test"}).
		WithCommonLabels(map[string]string{"instance": "localhost"}).
		WithCommonAnnotations(map[string]string{"summary": "Test summary"}).
		WithExternalURL("http://localhost:9093").
		AddAlert(*NewAlert("Test", SeverityCritical, "Test summary", "Test description", nil)).
		Build()

	payload := group.ToWebhookPayload()

	assert.Equal(t, "4", payload.Version)
	assert.Equal(t, "test-group", payload.GroupKey)
	assert.Equal(t, "test-receiver", payload.Receiver)
	assert.Equal(t, "localhost", payload.CommonLabels["instance"])
	assert.Equal(t, "Test", payload.GroupLabels["alertname"])
	assert.Equal(t, "Test summary", payload.CommonAnnotations["summary"])
	assert.Equal(t, "http://localhost:9093", payload.ExternalURL)
	assert.Len(t, payload.Alerts, 1)
}

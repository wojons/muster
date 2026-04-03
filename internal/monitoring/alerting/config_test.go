package alerting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.NotNil(t, config.Route)
	assert.Len(t, config.Receivers, 3)
	assert.Equal(t, "default", config.Route.Receiver)

	// Validate the config
	err := config.Validate()
	assert.NoError(t, err)
}

func TestAlertManagerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *AlertManagerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &AlertManagerConfig{
				Route: &Route{Receiver: "default"},
				Receivers: []Receiver{
					{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil route",
			config:  &AlertManagerConfig{Receivers: []Receiver{{Name: "default"}}},
			wantErr: true,
		},
		{
			name: "empty receivers",
			config: &AlertManagerConfig{
				Route:     &Route{Receiver: "default"},
				Receivers: []Receiver{},
			},
			wantErr: true,
		},
		{
			name: "unknown receiver reference",
			config: &AlertManagerConfig{
				Route: &Route{Receiver: "unknown"},
				Receivers: []Receiver{
					{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate receiver name",
			config: &AlertManagerConfig{
				Route: &Route{Receiver: "default"},
				Receivers: []Receiver{
					{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
					{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "empty receiver name",
			config: &AlertManagerConfig{
				Route: &Route{Receiver: "default"},
				Receivers: []Receiver{
					{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
					{Name: "", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
				},
			},
			wantErr: true,
		},
		{
			name: "receiver with no config",
			config: &AlertManagerConfig{
				Route: &Route{Receiver: "default"},
				Receivers: []Receiver{
					{Name: "default"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAlertManagerConfig_Validate_Defaults(t *testing.T) {
	config := &AlertManagerConfig{
		Route: &Route{Receiver: "default"},
		Receivers: []Receiver{
			{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
		},
	}

	// Before validation
	assert.Equal(t, time.Duration(0), config.Global.ResolveTimeout)
	assert.Equal(t, time.Duration(0), config.Global.GroupWait)
	assert.Equal(t, time.Duration(0), config.Global.GroupInterval)
	assert.Equal(t, time.Duration(0), config.Global.RepeatInterval)

	// After validation
	err := config.Validate()
	require.NoError(t, err)

	assert.Equal(t, 5*time.Minute, config.Global.ResolveTimeout)
	assert.Equal(t, 30*time.Second, config.Global.GroupWait)
	assert.Equal(t, 5*time.Minute, config.Global.GroupInterval)
	assert.Equal(t, 4*time.Hour, config.Global.RepeatInterval)
}

func TestRoute_Validate(t *testing.T) {
	tests := []struct {
		name    string
		route   *Route
		wantErr bool
	}{
		{
			name:    "valid route",
			route:   &Route{Receiver: "default"},
			wantErr: false,
		},
		{
			name:    "empty receiver",
			route:   &Route{Receiver: ""},
			wantErr: true,
		},
		{
			name: "nested invalid child route",
			route: &Route{
				Receiver: "default",
				Routes:   []*Route{{Receiver: ""}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.route.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReceiver_Validate(t *testing.T) {
	tests := []struct {
		name     string
		receiver Receiver
		wantErr  bool
	}{
		{
			name:     "valid receiver with webhook",
			receiver: Receiver{Name: "default", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
			wantErr:  false,
		},
		{
			name:     "valid receiver with slack",
			receiver: Receiver{Name: "slack", SlackConfigs: []SlackConfig{{Channel: "#alerts"}}},
			wantErr:  false,
		},
		{
			name:     "valid receiver with pagerduty",
			receiver: Receiver{Name: "pagerduty", PagerdutyConfigs: []PagerdutyConfig{{ServiceKey: "key"}}},
			wantErr:  false,
		},
		{
			name:     "empty name",
			receiver: Receiver{Name: "", WebhookConfigs: []WebhookConfig{{URL: "http://localhost"}}},
			wantErr:  true,
		},
		{
			name:     "no notification config",
			receiver: Receiver{Name: "default"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.receiver.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

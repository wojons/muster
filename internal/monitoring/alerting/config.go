// Package alerting provides AlertManager-compatible alerting configuration
// and webhook handlers for PagerDuty, Slack, and other notification channels.
package alerting

import (
	"fmt"
	"time"
)

// AlertManagerConfig represents the top-level AlertManager configuration
type AlertManagerConfig struct {
	Global       GlobalConfig  `yaml:"global" json:"global"`
	Route        *Route        `yaml:"route" json:"route"`
	InhibitRules []InhibitRule `yaml:"inhibit_rules,omitempty" json:"inhibit_rules,omitempty"`
	Receivers    []Receiver    `yaml:"receivers" json:"receivers"`
	Templates    []string      `yaml:"templates,omitempty" json:"templates,omitempty"`
}

// GlobalConfig contains globally applicable configuration parameters
type GlobalConfig struct {
	SMTPFrom         string            `yaml:"smtp_from,omitempty" json:"smtp_from,omitempty"`
	SMTPHello        string            `yaml:"smtp_hello,omitempty" json:"smtp_hello,omitempty"`
	SMTPSmarthost    string            `yaml:"smtp_smarthost,omitempty" json:"smtp_smarthost,omitempty"`
	SMTPAuthUsername string            `yaml:"smtp_auth_username,omitempty" json:"smtp_auth_username,omitempty"`
	SMTPAuthPassword string            `yaml:"smtp_auth_password,omitempty" json:"smtp_auth_password,omitempty"`
	SlackAPIURL      string            `yaml:"slack_api_url,omitempty" json:"slack_api_url,omitempty"`
	PagerdutyURL     string            `yaml:"pagerduty_url,omitempty" json:"pagerduty_url,omitempty"`
	HipchatURL       string            `yaml:"hipchat_url,omitempty" json:"hipchat_url,omitempty"`
	HipchatAuthToken string            `yaml:"hipchat_auth_token,omitempty" json:"hipchat_auth_token,omitempty"`
	OpsGenieAPIURL   string            `yaml:"opsgenie_api_url,omitempty" json:"opsgenie_api_url,omitempty"`
	VictorOpsURL     string            `yaml:"victorops_url,omitempty" json:"victorops_url,omitempty"`
	WeChatAPIURL     string            `yaml:"wechat_api_url,omitempty" json:"wechat_api_url,omitempty"`
	WebexAPIURL      string            `yaml:"webex_api_url,omitempty" json:"webex_api_url,omitempty"`
	HTTPConfig       *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	ResolveTimeout   time.Duration     `yaml:"resolve_timeout,omitempty" json:"resolve_timeout,omitempty"`
	GroupBy          []string          `yaml:"group_by,omitempty" json:"group_by,omitempty"`
	GroupWait        time.Duration     `yaml:"group_wait,omitempty" json:"group_wait,omitempty"`
	GroupInterval    time.Duration     `yaml:"group_interval,omitempty" json:"group_interval,omitempty"`
	RepeatInterval   time.Duration     `yaml:"repeat_interval,omitempty" json:"repeat_interval,omitempty"`
}

// HTTPClientConfig configures an HTTP client for webhook requests
type HTTPClientConfig struct {
	ProxyURL        string         `yaml:"proxy_url,omitempty" json:"proxy_url,omitempty"`
	TLSConfig       *TLSConfig     `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
	BasicAuth       *BasicAuth     `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
	Authorization   *Authorization `yaml:"authorization,omitempty" json:"authorization,omitempty"`
	OAuth2          *OAuth2Config  `yaml:"oauth2,omitempty" json:"oauth2,omitempty"`
	FollowRedirects bool           `yaml:"follow_redirects,omitempty" json:"follow_redirects,omitempty"`
	EnableHTTP2     bool           `yaml:"enable_http2,omitempty" json:"enable_http2,omitempty"`
}

// TLSConfig configures TLS settings
type TLSConfig struct {
	CAFile             string `yaml:"ca_file,omitempty" json:"ca_file,omitempty"`
	CertFile           string `yaml:"cert_file,omitempty" json:"cert_file,omitempty"`
	KeyFile            string `yaml:"key_file,omitempty" json:"key_file,omitempty"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
}

// BasicAuth configures basic authentication
type BasicAuth struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// Authorization configures generic authorization
type Authorization struct {
	Type            string `yaml:"type,omitempty" json:"type,omitempty"`
	Credentials     string `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	CredentialsFile string `yaml:"credentials_file,omitempty" json:"credentials_file,omitempty"`
}

// OAuth2Config configures OAuth2 authentication
type OAuth2Config struct {
	ClientID       string            `yaml:"client_id" json:"client_id"`
	ClientSecret   string            `yaml:"client_secret" json:"client_secret"`
	TokenURL       string            `yaml:"token_url" json:"token_url"`
	Scopes         []string          `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	EndpointParams map[string]string `yaml:"endpoint_params,omitempty" json:"endpoint_params,omitempty"`
}

// Route defines a node in the routing tree
type Route struct {
	Receiver            string            `yaml:"receiver" json:"receiver"`
	GroupBy             []string          `yaml:"group_by,omitempty" json:"group_by,omitempty"`
	GroupWait           time.Duration     `yaml:"group_wait,omitempty" json:"group_wait,omitempty"`
	GroupInterval       time.Duration     `yaml:"group_interval,omitempty" json:"group_interval,omitempty"`
	RepeatInterval      time.Duration     `yaml:"repeat_interval,omitempty" json:"repeat_interval,omitempty"`
	Continue            bool              `yaml:"continue,omitempty" json:"continue,omitempty"`
	Match               map[string]string `yaml:"match,omitempty" json:"match,omitempty"`
	MatchRE             map[string]string `yaml:"match_re,omitempty" json:"match_re,omitempty"`
	Matchers            []string          `yaml:"matchers,omitempty" json:"matchers,omitempty"`
	Routes              []*Route          `yaml:"routes,omitempty" json:"routes,omitempty"`
	ActiveTimeIntervals []string          `yaml:"active_time_intervals,omitempty" json:"active_time_intervals,omitempty"`
}

// InhibitRule defines when to inhibit alerts
type InhibitRule struct {
	SourceMatch   map[string]string `yaml:"source_match,omitempty" json:"source_match,omitempty"`
	SourceMatchRE map[string]string `yaml:"source_match_re,omitempty" json:"source_match_re,omitempty"`
	TargetMatch   map[string]string `yaml:"target_match,omitempty" json:"target_match,omitempty"`
	TargetMatchRE map[string]string `yaml:"target_match_re,omitempty" json:"target_match_re,omitempty"`
	Equal         []string          `yaml:"equal,omitempty" json:"equal,omitempty"`
}

// Receiver represents a named receiver configuration
type Receiver struct {
	Name             string            `yaml:"name" json:"name"`
	SlackConfigs     []SlackConfig     `yaml:"slack_configs,omitempty" json:"slack_configs,omitempty"`
	PagerdutyConfigs []PagerdutyConfig `yaml:"pagerduty_configs,omitempty" json:"pagerduty_configs,omitempty"`
	WebhookConfigs   []WebhookConfig   `yaml:"webhook_configs,omitempty" json:"webhook_configs,omitempty"`
	EmailConfigs     []EmailConfig     `yaml:"email_configs,omitempty" json:"email_configs,omitempty"`
	OpsGenieConfigs  []OpsGenieConfig  `yaml:"opsgenie_configs,omitempty" json:"opsgenie_configs,omitempty"`
	VictorOpsConfigs []VictorOpsConfig `yaml:"victorops_configs,omitempty" json:"victorops_configs,omitempty"`
	WeChatConfigs    []WeChatConfig    `yaml:"wechat_configs,omitempty" json:"wechat_configs,omitempty"`
	WebexConfigs     []WebexConfig     `yaml:"webex_configs,omitempty" json:"webex_configs,omitempty"`
}

// SlackConfig configures Slack notifications
type SlackConfig struct {
	SendResolved bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig   *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APIURL       string            `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	Channel      string            `yaml:"channel,omitempty" json:"channel,omitempty"`
	Username     string            `yaml:"username,omitempty" json:"username,omitempty"`
	IconEmoji    string            `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
	IconURL      string            `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	Title        string            `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink    string            `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Pretext      string            `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	Text         string            `yaml:"text,omitempty" json:"text,omitempty"`
	Color        string            `yaml:"color,omitempty" json:"color,omitempty"`
	Actions      []SlackAction     `yaml:"actions,omitempty" json:"actions,omitempty"`
	ShortFields  bool              `yaml:"short_fields,omitempty" json:"short_fields,omitempty"`
}

// SlackAction represents a Slack action button
type SlackAction struct {
	Type  string `yaml:"type" json:"type"`
	Name  string `yaml:"name" json:"name"`
	Text  string `yaml:"text" json:"text"`
	URL   string `yaml:"url,omitempty" json:"url,omitempty"`
	Style string `yaml:"style,omitempty" json:"style,omitempty"`
}

// PagerdutyConfig configures PagerDuty notifications
type PagerdutyConfig struct {
	SendResolved bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig   *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	ServiceKey   string            `yaml:"service_key,omitempty" json:"service_key,omitempty"`
	RoutingKey   string            `yaml:"routing_key,omitempty" json:"routing_key,omitempty"`
	URL          string            `yaml:"url,omitempty" json:"url,omitempty"`
	Client       string            `yaml:"client,omitempty" json:"client,omitempty"`
	ClientURL    string            `yaml:"client_url,omitempty" json:"client_url,omitempty"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	Details      map[string]string `yaml:"details,omitempty" json:"details,omitempty"`
	Severity     string            `yaml:"severity,omitempty" json:"severity,omitempty"`
	Class        string            `yaml:"class,omitempty" json:"class,omitempty"`
	Component    string            `yaml:"component,omitempty" json:"component,omitempty"`
	Group        string            `yaml:"group,omitempty" json:"group,omitempty"`
	Source       string            `yaml:"source,omitempty" json:"source,omitempty"`
	Images       []PagerdutyImage  `yaml:"images,omitempty" json:"images,omitempty"`
	Links        []PagerdutyLink   `yaml:"links,omitempty" json:"links,omitempty"`
}

// PagerdutyImage represents a PagerDuty image
type PagerdutyImage struct {
	Src  string `yaml:"src" json:"src"`
	Alt  string `yaml:"alt,omitempty" json:"alt,omitempty"`
	Href string `yaml:"href,omitempty" json:"href,omitempty"`
}

// PagerdutyLink represents a PagerDuty link
type PagerdutyLink struct {
	Href string `yaml:"href" json:"href"`
	Text string `yaml:"text" json:"text"`
}

// WebhookConfig configures generic webhook notifications
type WebhookConfig struct {
	SendResolved bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig   *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	URL          string            `yaml:"url" json:"url"`
	MaxAlerts    int               `yaml:"max_alerts,omitempty" json:"max_alerts,omitempty"`
}

// EmailConfig configures email notifications
type EmailConfig struct {
	SendResolved bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	To           string            `yaml:"to" json:"to"`
	From         string            `yaml:"from,omitempty" json:"from,omitempty"`
	Hello        string            `yaml:"hello,omitempty" json:"hello,omitempty"`
	Smarthost    string            `yaml:"smarthost,omitempty" json:"smarthost,omitempty"`
	AuthUsername string            `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	AuthPassword string            `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	HTML         string            `yaml:"html,omitempty" json:"html,omitempty"`
	Text         string            `yaml:"text,omitempty" json:"text,omitempty"`
	RequireTLS   bool              `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
	TLSConfig    *TLSConfig        `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
}

// OpsGenieConfig configures OpsGenie notifications
type OpsGenieConfig struct {
	SendResolved bool                `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig   *HTTPClientConfig   `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APIKey       string              `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIURL       string              `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	Message      string              `yaml:"message,omitempty" json:"message,omitempty"`
	Description  string              `yaml:"description,omitempty" json:"description,omitempty"`
	Source       string              `yaml:"source,omitempty" json:"source,omitempty"`
	Tags         string              `yaml:"tags,omitempty" json:"tags,omitempty"`
	Note         string              `yaml:"note,omitempty" json:"note,omitempty"`
	Priority     string              `yaml:"priority,omitempty" json:"priority,omitempty"`
	Details      map[string]string   `yaml:"details,omitempty" json:"details,omitempty"`
	Responders   []OpsGenieResponder `yaml:"responders,omitempty" json:"responders,omitempty"`
	Entity       string              `yaml:"entity,omitempty" json:"entity,omitempty"`
	Actions      string              `yaml:"actions,omitempty" json:"actions,omitempty"`
}

// OpsGenieResponder represents an OpsGenie responder
type OpsGenieResponder struct {
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
}

// VictorOpsConfig configures VictorOps (Splunk On-Call) notifications
type VictorOpsConfig struct {
	SendResolved      bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig        *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APIKey            string            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIURL            string            `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	RoutingKey        string            `yaml:"routing_key,omitempty" json:"routing_key,omitempty"`
	MessageType       string            `yaml:"message_type,omitempty" json:"message_type,omitempty"`
	EntityDisplayName string            `yaml:"entity_display_name,omitempty" json:"entity_display_name,omitempty"`
	StateMessage      string            `yaml:"state_message,omitempty" json:"state_message,omitempty"`
}

// WeChatConfig configures WeChat notifications
type WeChatConfig struct {
	SendResolved bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig   *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APISecret    string            `yaml:"api_secret,omitempty" json:"api_secret,omitempty"`
	APIURL       string            `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	CorpID       string            `yaml:"corp_id,omitempty" json:"corp_id,omitempty"`
	AgentID      string            `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
	ToUser       string            `yaml:"to_user,omitempty" json:"to_user,omitempty"`
	ToParty      string            `yaml:"to_party,omitempty" json:"to_party,omitempty"`
	ToTag        string            `yaml:"to_tag,omitempty" json:"to_tag,omitempty"`
	Message      string            `yaml:"message,omitempty" json:"message,omitempty"`
}

// WebexConfig configures Cisco Webex notifications
type WebexConfig struct {
	SendResolved bool              `yaml:"send_resolved,omitempty" json:"send_resolved,omitempty"`
	HTTPConfig   *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APIURL       string            `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	RoomID       string            `yaml:"room_id,omitempty" json:"room_id,omitempty"`
	Message      string            `yaml:"message,omitempty" json:"message,omitempty"`
}

// Validate checks if the configuration is valid
func (c *AlertManagerConfig) Validate() error {
	if c.Route == nil {
		return fmt.Errorf("route is required")
	}
	if len(c.Receivers) == 0 {
		return fmt.Errorf("at least one receiver is required")
	}

	// Validate global settings
	if c.Global.ResolveTimeout == 0 {
		c.Global.ResolveTimeout = 5 * time.Minute
	}
	if c.Global.GroupWait == 0 {
		c.Global.GroupWait = 30 * time.Second
	}
	if c.Global.GroupInterval == 0 {
		c.Global.GroupInterval = 5 * time.Minute
	}
	if c.Global.RepeatInterval == 0 {
		c.Global.RepeatInterval = 4 * time.Hour
	}

	// Validate route
	if err := c.Route.Validate(); err != nil {
		return fmt.Errorf("invalid route: %w", err)
	}

	// Validate receivers
	receiverNames := make(map[string]bool)
	for _, r := range c.Receivers {
		if r.Name == "" {
			return fmt.Errorf("receiver name cannot be empty")
		}
		if receiverNames[r.Name] {
			return fmt.Errorf("duplicate receiver name: %s", r.Name)
		}
		receiverNames[r.Name] = true

		if err := r.Validate(); err != nil {
			return fmt.Errorf("invalid receiver %s: %w", r.Name, err)
		}
	}

	// Validate route receiver exists
	if !receiverNames[c.Route.Receiver] {
		return fmt.Errorf("route references unknown receiver: %s", c.Route.Receiver)
	}

	return nil
}

// Validate checks if the route is valid
func (r *Route) Validate() error {
	if r.Receiver == "" {
		return fmt.Errorf("receiver is required")
	}

	for _, child := range r.Routes {
		if err := child.Validate(); err != nil {
			return fmt.Errorf("invalid child route: %w", err)
		}
	}

	return nil
}

// Validate checks if the receiver is valid
func (r *Receiver) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate at least one config exists
	configCount := len(r.SlackConfigs) + len(r.PagerdutyConfigs) + len(r.WebhookConfigs) +
		len(r.EmailConfigs) + len(r.OpsGenieConfigs) + len(r.VictorOpsConfigs) +
		len(r.WeChatConfigs) + len(r.WebexConfigs)

	if configCount == 0 {
		return fmt.Errorf("at least one notification config is required")
	}

	return nil
}

// DefaultConfig returns a default AlertManager configuration
func DefaultConfig() *AlertManagerConfig {
	return &AlertManagerConfig{
		Global: GlobalConfig{
			ResolveTimeout: 5 * time.Minute,
			GroupBy:        []string{"alertname", "severity", "tenant_id"},
			GroupWait:      30 * time.Second,
			GroupInterval:  5 * time.Minute,
			RepeatInterval: 4 * time.Hour,
		},
		Route: &Route{
			Receiver:       "default",
			GroupBy:        []string{"alertname", "severity", "tenant_id"},
			GroupWait:      30 * time.Second,
			GroupInterval:  5 * time.Minute,
			RepeatInterval: 4 * time.Hour,
			Routes: []*Route{
				{
					Receiver: "critical",
					Match:    map[string]string{"severity": "critical"},
				},
				{
					Receiver: "high",
					Match:    map[string]string{"severity": "high"},
				},
			},
		},
		Receivers: []Receiver{
			{
				Name: "default",
				WebhookConfigs: []WebhookConfig{
					{URL: "http://localhost:5001/webhook"},
				},
			},
			{
				Name: "critical",
				PagerdutyConfigs: []PagerdutyConfig{
					{
						SendResolved: true,
						Severity:     "critical",
					},
				},
			},
			{
				Name: "high",
				SlackConfigs: []SlackConfig{
					{
						SendResolved: true,
						Channel:      "#alerts",
					},
				},
			},
		},
	}
}

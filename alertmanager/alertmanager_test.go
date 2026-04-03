package alertmanager

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAlertmanagerConfigExists verifies the AlertManager configuration file exists
func TestAlertmanagerConfigExists(t *testing.T) {
	configPath := "../alertmanager/alertmanager.yml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("AlertManager config not found at %s", configPath)
	}
}

// TestAlertmanagerConfigIsValidYAML verifies the AlertManager config is valid YAML
func TestAlertmanagerConfigIsValidYAML(t *testing.T) {
	configPath := "../alertmanager/alertmanager.yml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read AlertManager config: %v", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("AlertManager config is not valid YAML: %v", err)
	}

	// Verify required sections exist
	if _, ok := config["global"]; !ok {
		t.Error("AlertManager config missing 'global' section")
	}
	if _, ok := config["route"]; !ok {
		t.Error("AlertManager config missing 'route' section")
	}
	if _, ok := config["receivers"]; !ok {
		t.Error("AlertManager config missing 'receivers' section")
	}
}

// TestAlertmanagerRoutingRules verifies routing configuration
func TestAlertmanagerRoutingRules(t *testing.T) {
	configPath := "../alertmanager/alertmanager.yml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read AlertManager config: %v", err)
	}

	type Route struct {
		Receiver string   `yaml:"receiver"`
		GroupBy  []string `yaml:"group_by"`
		Routes   []Route  `yaml:"routes"`
	}

	type Config struct {
		Route Route `yaml:"route"`
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse AlertManager config: %v", err)
	}

	// Verify default receiver
	if config.Route.Receiver != "default" {
		t.Errorf("Expected default receiver 'default', got '%s'", config.Route.Receiver)
	}

	// Verify group_by labels
	if len(config.Route.GroupBy) == 0 {
		t.Error("Routing missing 'group_by' configuration")
	}

	// Verify sub-routes exist for severity-based routing
	if len(config.Route.Routes) == 0 {
		t.Error("Missing severity-based routing sub-routes")
	}
}

// TestAlertmanagerReceivers verifies receivers are configured
func TestAlertmanagerReceivers(t *testing.T) {
	configPath := "../alertmanager/alertmanager.yml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read AlertManager config: %v", err)
	}

	type Receiver struct {
		Name string `yaml:"name"`
	}

	type Config struct {
		Receivers []Receiver `yaml:"receivers"`
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse AlertManager config: %v", err)
	}

	// Verify required receivers exist
	receiverNames := make(map[string]bool)
	for _, r := range config.Receivers {
		receiverNames[r.Name] = true
	}

	required := []string{"default", "slack", "pagerduty"}
	for _, name := range required {
		if !receiverNames[name] {
			t.Errorf("Missing required receiver: %s", name)
		}
	}
}

// TestAlertRulesExist verifies alert rules file exists
func TestAlertRulesExist(t *testing.T) {
	rulesPath := "../prometheus/rules/alerts.yml"
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		t.Fatalf("Alert rules not found at %s", rulesPath)
	}
}

// TestAlertRulesAreValidYAML verifies alert rules are valid YAML
func TestAlertRulesAreValidYAML(t *testing.T) {
	rulesPath := "../prometheus/rules/alerts.yml"
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("Failed to read alert rules: %v", err)
	}

	var rules map[string]interface{}
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatalf("Alert rules are not valid YAML: %v", err)
	}

	// Verify 'groups' section exists
	if _, ok := rules["groups"]; !ok {
		t.Error("Alert rules missing 'groups' section")
	}
}

// TestAlertRulesContainCriticalAlerts verifies critical alert rules exist
func TestAlertRulesContainCriticalAlerts(t *testing.T) {
	rulesPath := "../prometheus/rules/alerts.yml"
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("Failed to read alert rules: %v", err)
	}

	type Rule struct {
		Alert       string            `yaml:"alert"`
		Expr        string            `yaml:"expr"`
		For         string            `yaml:"for"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	}

	type Group struct {
		Name  string `yaml:"name"`
		Rules []Rule `yaml:"rules"`
	}

	type Rules struct {
		Groups []Group `yaml:"groups"`
	}

	var rules Rules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatalf("Failed to parse alert rules: %v", err)
	}

	// Collect all alert names
	alertNames := make(map[string]bool)
	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			alertNames[rule.Alert] = true
		}
	}

	// Verify expected critical alerts
	expected := []string{
		"HighErrorRate",
		"HighLatency",
		"LowDiskSpace",
		"HighMemoryUsage",
		"DatabaseConnectionFailures",
		"ServiceDown",
	}

	for _, alert := range expected {
		if !alertNames[alert] {
			t.Errorf("Missing expected alert: %s", alert)
		}
	}
}

// TestAlertRulesHaveRequiredFields verifies alerts have required fields
func TestAlertRulesHaveRequiredFields(t *testing.T) {
	rulesPath := "../prometheus/rules/alerts.yml"
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("Failed to read alert rules: %v", err)
	}

	type Rule struct {
		Alert       string            `yaml:"alert"`
		Expr        string            `yaml:"expr"`
		For         string            `yaml:"for"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	}

	type Group struct {
		Name  string `yaml:"name"`
		Rules []Rule `yaml:"rules"`
	}

	type Rules struct {
		Groups []Group `yaml:"groups"`
	}

	var rules Rules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatalf("Failed to parse alert rules: %v", err)
	}

	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			if rule.Alert == "" {
				t.Error("Alert rule missing 'alert' name")
			}
			if rule.Expr == "" {
				t.Errorf("Alert '%s' missing 'expr'", rule.Alert)
			}
			if rule.Labels["severity"] == "" {
				t.Errorf("Alert '%s' missing severity label", rule.Alert)
			}
			if rule.Annotations["summary"] == "" {
				t.Errorf("Alert '%s' missing summary annotation", rule.Alert)
			}
			if rule.Annotations["description"] == "" {
				t.Errorf("Alert '%s' missing description annotation", rule.Alert)
			}
		}
	}
}

// TestTemplatesExist verifies notification templates exist
func TestTemplatesExist(t *testing.T) {
	templatePath := "../alertmanager/templates/default.tmpl"
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		t.Fatalf("AlertManager templates not found at %s", templatePath)
	}

	// Verify template has content
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("Failed to read templates: %v", err)
	}

	if len(data) == 0 {
		t.Error("Template file is empty")
	}
}

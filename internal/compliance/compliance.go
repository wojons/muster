// Package compliance provides comprehensive compliance verification for the Muster system.
// This includes Bible Standard, OpenAPI 3.0, MCP Protocol, OAuth2, TLS, GDPR, HIPAA,
// PCI DSS, and FIPS 140-3 compliance checking.
package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Framework represents a compliance framework (e.g., GDPR, HIPAA)
type Framework string

const (
	FrameworkBible       Framework = "BIBLE_STANDARD"
	FrameworkOpenAPI     Framework = "OPENAPI_3.0"
	FrameworkMCP         Framework = "MCP_PROTOCOL"
	FrameworkOAuth2      Framework = "OAUTH2"
	FrameworkTLS         Framework = "TLS_1.3"
	FrameworkGDPR        Framework = "GDPR"
	FrameworkHIPAA       Framework = "HIPAA"
	FrameworkPCI         Framework = "PCI_DSS"
	FrameworkFIPS1403    Framework = "FIPS_140-3"
	FrameworkLinuxFHS    Framework = "LINUX_FHS"
	FrameworkMacSandbox  Framework = "MACOS_SANDBOX"
	FrameworkWindowsCert Framework = "WINDOWS_CERT"
	FrameworkDocker      Framework = "DOCKER_BEST_PRACTICES"
	FrameworkK8s         Framework = "KUBERNETES_HELM"
)

// Severity represents the severity of a compliance issue
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// Issue represents a compliance issue
type Issue struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Location string   `json:"location,omitempty"`
	Control  string   `json:"control,omitempty"` // e.g., "GDPR-17", "PCI-3.4"
	Evidence Evidence `json:"evidence,omitempty"`
}

// Evidence represents collected evidence for a compliance check
type Evidence struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`              // "code", "config", "test", "log", "scan"
	Source    string                 `json:"source"`            // file path or system component
	Content   string                 `json:"content,omitempty"` // content hash or summary
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Control represents a compliance control with implementation mapping
type Control struct {
	ID             string         `json:"id"` // e.g., "GDPR-17-1"
	Framework      Framework      `json:"framework"`
	Category       string         `json:"category"` // e.g., "data-retention"
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Requirement    string         `json:"requirement"` // The actual requirement text
	Status         string         `json:"status"`      // "implemented", "partial", "not-implemented"
	Implementation Implementation `json:"implementation"`
	Evidence       []Evidence     `json:"evidence,omitempty"`
	LastUpdated    time.Time      `json:"last_updated"`
}

// Implementation maps a control to actual implementation
type Implementation struct {
	Files      []string  `json:"files"`  // Implementation files
	Tests      []string  `json:"tests"`  // Test files verifying compliance
	Docs       []string  `json:"docs"`   // Documentation
	Status     string    `json:"status"` // "complete", "partial", "missing"
	VerifiedAt time.Time `json:"verified_at"`
	VerifiedBy string    `json:"verified_by"`
}

// ValidationResult represents the result of a compliance check
type ValidationResult struct {
	CheckName string        `json:"check_name"`
	Framework Framework     `json:"framework"`
	ControlID string        `json:"control_id,omitempty"`
	Passed    bool          `json:"passed"`
	Issues    []Issue       `json:"issues,omitempty"`
	Score     float64       `json:"score"`
	Evidence  []Evidence    `json:"evidence,omitempty"`
	Duration  time.Duration `json:"duration_ms,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// Report represents a full compliance report
type Report struct {
	ID            string             `json:"id"`
	Timestamp     time.Time          `json:"timestamp"`
	Frameworks    []Framework        `json:"frameworks"`
	Results       []ValidationResult `json:"results"`
	TotalScore    float64            `json:"total_score"`
	Passed        bool               `json:"passed"`
	Summary       ReportSummary      `json:"summary"`
	Controls      []Control          `json:"controls,omitempty"`
	EvidenceCount int                `json:"evidence_count"`
}

// ReportSummary provides a high-level summary of compliance
type ReportSummary struct {
	TotalControls  int `json:"total_controls"`
	Implemented    int `json:"implemented"`
	Partial        int `json:"partial"`
	NotImplemented int `json:"not_implemented"`
	CriticalIssues int `json:"critical_issues"`
	ErrorIssues    int `json:"error_issues"`
	WarningIssues  int `json:"warning_issues"`
	InfoIssues     int `json:"info_issues"`
}

// Check represents a compliance check interface
type Check interface {
	Name() string
	Framework() Framework
	Validate() ValidationResult
}

// DeterminismCheck validates that specifications use deterministic language
// DEPRECATED: Use BibleStandardCheck for full compliance checking
type DeterminismCheck struct {
	SpecContent string
}

func (c *DeterminismCheck) Name() string {
	return "Determinism"
}

func (c *DeterminismCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Passed:    true,
	}

	ambiguousTerms := []string{
		"should", "could", "might", "consider",
		"appropriate", "reasonable", "sufficient",
	}

	lowerContent := strings.ToLower(c.SpecContent)
	for _, term := range ambiguousTerms {
		if strings.Contains(lowerContent, term) {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityWarning,
				Message:  "Ambiguous term found: '" + term + "'",
			})
			result.Passed = false
		}
	}

	if len(result.Issues) == 0 {
		result.Score = 1.0
	} else {
		result.Score = 1.0 - (float64(len(result.Issues)) * 0.1)
		if result.Score < 0 {
			result.Score = 0
		}
	}

	return result
}

// CompletenessCheck validates that specifications have all required sections
// DEPRECATED: Use BibleStandardCheck for full compliance checking
type CompletenessCheck struct {
	Sections map[string]string
}

func (c *CompletenessCheck) Name() string {
	return "Completeness"
}

func (c *CompletenessCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Passed:    true,
	}

	requiredSections := []string{
		"Overview",
		"Architecture",
		"Implementation",
		"Error Handling",
		"Testing",
	}

	foundCount := 0
	for _, section := range requiredSections {
		if _, has := c.Sections[section]; has {
			foundCount++
		} else {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				Message:  "Missing required section: '" + section + "'",
			})
			result.Passed = false
		}
	}

	result.Score = float64(foundCount) / float64(len(requiredSections))

	return result
}

// SARIF types for exporting to SARIF format (Static Analysis Results Interchange Format)
type SARIFLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
	} `json:"physicalLocation"`
}

type SARIFResult struct {
	RuleID  string `json:"ruleId"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Level     string          `json:"level"`
	Locations []SARIFLocation `json:"locations,omitempty"`
}

type SARIFRun struct {
	Tool struct {
		Driver struct {
			Name           string `json:"name"`
			Version        string `json:"version"`
			InformationURI string `json:"informationUri"`
		} `json:"driver"`
	} `json:"tool"`
	Results []SARIFResult `json:"results"`
}

type SARIFReport struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []SARIFRun `json:"runs"`
}

// ComplianceMatrix maintains the control → implementation mapping
type ComplianceMatrix struct {
	mu       sync.RWMutex
	controls map[string]*Control
	evidence map[string][]Evidence
}

// NewComplianceMatrix creates a new compliance matrix
func NewComplianceMatrix() *ComplianceMatrix {
	return &ComplianceMatrix{
		controls: make(map[string]*Control),
		evidence: make(map[string][]Evidence),
	}
}

// RegisterControl adds a control to the matrix
func (m *ComplianceMatrix) RegisterControl(control *Control) {
	m.mu.Lock()
	defer m.mu.Unlock()
	control.LastUpdated = time.Now()
	m.controls[control.ID] = control
}

// GetControl retrieves a control by ID
func (m *ComplianceMatrix) GetControl(id string) (*Control, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.controls[id]
	return c, ok
}

// AddEvidence adds evidence for a control
func (m *ComplianceMatrix) AddEvidence(controlID string, evidence Evidence) {
	m.mu.Lock()
	defer m.mu.Unlock()
	evidence.Timestamp = time.Now()
	m.evidence[controlID] = append(m.evidence[controlID], evidence)
	if c, ok := m.controls[controlID]; ok {
		c.Evidence = append(c.Evidence, evidence)
	}
}

// GetEvidence retrieves evidence for a control
func (m *ComplianceMatrix) GetEvidence(controlID string) []Evidence {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.evidence[controlID]
}

// GetAllControls returns all registered controls
func (m *ComplianceMatrix) GetAllControls() []*Control {
	m.mu.RLock()
	defer m.mu.RUnlock()

	controls := make([]*Control, 0, len(m.controls))
	for _, c := range m.controls {
		controls = append(controls, c)
	}

	// Sort by ID for consistent ordering
	sort.Slice(controls, func(i, j int) bool {
		return controls[i].ID < controls[j].ID
	})

	return controls
}

// GetControlsByFramework returns controls for a specific framework
func (m *ComplianceMatrix) GetControlsByFramework(fw Framework) []*Control {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var controls []*Control
	for _, c := range m.controls {
		if c.Framework == fw {
			controls = append(controls, c)
		}
	}

	sort.Slice(controls, func(i, j int) bool {
		return controls[i].ID < controls[j].ID
	})

	return controls
}

// GenerateReport creates a comprehensive compliance report
func (m *ComplianceMatrix) GenerateReport(frameworks []Framework) (*Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := &Report{
		ID:         fmt.Sprintf("COMPLIANCE-%d", time.Now().Unix()),
		Timestamp:  time.Now(),
		Frameworks: frameworks,
		Results:    []ValidationResult{},
	}

	// Run all registered checks
	for _, check := range m.getChecks() {
		for _, fw := range frameworks {
			if check.Framework() == fw {
				start := time.Now()
				result := check.Validate()
				result.Duration = time.Since(start)
				result.Timestamp = time.Now()
				report.Results = append(report.Results, result)
			}
		}
	}

	// Calculate scores and summaries
	var totalScore float64
	passed := true

	for _, result := range report.Results {
		totalScore += result.Score
		if !result.Passed {
			passed = false
		}
		for _, issue := range result.Issues {
			switch issue.Severity {
			case SeverityCritical:
				report.Summary.CriticalIssues++
			case SeverityError:
				report.Summary.ErrorIssues++
			case SeverityWarning:
				report.Summary.WarningIssues++
			case SeverityInfo:
				report.Summary.InfoIssues++
			}
		}
	}

	if len(report.Results) > 0 {
		report.TotalScore = totalScore / float64(len(report.Results))
	} else {
		report.TotalScore = 1.0
	}
	report.Passed = passed && report.TotalScore >= 0.8

	// Add controls from matrix
	for _, c := range m.controls {
		report.Controls = append(report.Controls, *c)
		report.EvidenceCount += len(c.Evidence)

		switch c.Status {
		case "implemented":
			report.Summary.Implemented++
		case "partial":
			report.Summary.Partial++
		default:
			report.Summary.NotImplemented++
		}
	}
	report.Summary.TotalControls = len(m.controls)

	return report, nil
}

// ExportReport exports the report to various formats
func ExportReport(report *Report, format string, path string) error {
	switch strings.ToLower(format) {
	case "json":
		return exportJSON(report, path)
	case "markdown", "md":
		return exportMarkdown(report, path)
	case "csv":
		return exportCSV(report, path)
	case "sarif":
		return exportSARIF(report, path)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

func exportJSON(report *Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func exportMarkdown(report *Report, path string) error {
	var sb strings.Builder

	sb.WriteString("# Compliance Report\n\n")
	sb.WriteString(fmt.Sprintf("**Report ID:** %s\n\n", report.ID))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", report.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Overall Score:** %.1f%%\n\n", report.TotalScore*100))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", func() string {
		if report.Passed {
			return "✅ PASS"
		}
		return "❌ FAIL"
	}()))

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- Total Controls: %d\n", report.Summary.TotalControls))
	sb.WriteString(fmt.Sprintf("- Implemented: %d\n", report.Summary.Implemented))
	sb.WriteString(fmt.Sprintf("- Partial: %d\n", report.Summary.Partial))
	sb.WriteString(fmt.Sprintf("- Not Implemented: %d\n\n", report.Summary.NotImplemented))

	sb.WriteString("### Issues by Severity\n\n")
	sb.WriteString(fmt.Sprintf("- 🔴 Critical: %d\n", report.Summary.CriticalIssues))
	sb.WriteString(fmt.Sprintf("- 🟠 Error: %d\n", report.Summary.ErrorIssues))
	sb.WriteString(fmt.Sprintf("- 🟡 Warning: %d\n", report.Summary.WarningIssues))
	sb.WriteString(fmt.Sprintf("- 🔵 Info: %d\n\n", report.Summary.InfoIssues))

	// Results by framework
	sb.WriteString("## Results by Framework\n\n")
	for _, fw := range report.Frameworks {
		sb.WriteString(fmt.Sprintf("### %s\n\n", fw))
		for _, result := range report.Results {
			if result.Framework == fw {
				status := "✅"
				if !result.Passed {
					status = "❌"
				}
				sb.WriteString(fmt.Sprintf("- %s **%s** (%.0f%%)\n", status, result.CheckName, result.Score*100))
			}
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func exportCSV(report *Report, path string) error {
	var sb strings.Builder
	sb.WriteString("CheckName,Framework,ControlID,Passed,Score,Issues\n")

	for _, result := range report.Results {
		issueCount := len(result.Issues)
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%t,%.2f,%d\n",
			result.CheckName,
			result.Framework,
			result.ControlID,
			result.Passed,
			result.Score,
			issueCount,
		))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func exportSARIF(report *Report, path string) error {
	// SARIF format for integration with CI/CD systems
	sarif := SARIFReport{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
	}

	run := SARIFRun{}
	run.Tool.Driver.Name = "Muster Compliance"
	run.Tool.Driver.Version = "1.0.0"
	run.Tool.Driver.InformationURI = "https://github.com/anomalyco/muster"

	for _, result := range report.Results {
		for _, issue := range result.Issues {
			level := "warning"
			switch issue.Severity {
			case SeverityCritical, SeverityError:
				level = "error"
			case SeverityInfo:
				level = "note"
			}

			sarifResult := SARIFResult{
				RuleID: result.ControlID,
			}
			sarifResult.Message.Text = issue.Message
			sarifResult.Level = level
			if issue.Location != "" {
				loc := SARIFLocation{}
				loc.PhysicalLocation.ArtifactLocation.URI = issue.Location
				sarifResult.Locations = append(sarifResult.Locations, loc)
			}
			run.Results = append(run.Results, sarifResult)
		}
	}

	sarif.Runs = append(sarif.Runs, run)

	data, err := json.MarshalIndent(sarif, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// getChecks returns all registered compliance checks
func (m *ComplianceMatrix) getChecks() []Check {
	return []Check{
		&BibleStandardCheck{},
		&OpenAPIComplianceCheck{},
		&MCPComplianceCheck{},
		&OAuth2ComplianceCheck{},
		&TLSComplianceCheck{},
		&GDPRComplianceCheck{},
	}
}

// CollectEvidenceFromFile collects evidence from a file
func CollectEvidenceFromFile(filePath string) (Evidence, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return Evidence{}, fmt.Errorf("failed to stat file: %w", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return Evidence{}, fmt.Errorf("failed to read file: %w", err)
	}

	return Evidence{
		Timestamp: time.Now(),
		Type:      "code",
		Source:    filePath,
		Content:   fmt.Sprintf("%x", hashContent(content)),
		Metadata: map[string]interface{}{
			"size":        info.Size(),
			"mod_time":    info.ModTime(),
			"permissions": info.Mode().String(),
		},
	}, nil
}

// hashContent creates a simple hash of content
func hashContent(content []byte) []byte {
	// Simple hash - in production, use crypto/sha256
	if len(content) == 0 {
		return []byte{}
	}
	return content[:min(32, len(content))]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

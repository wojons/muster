package compliance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestComplianceMatrix tests the compliance matrix functionality
func TestComplianceMatrix(t *testing.T) {
	t.Run("Register and Get Control", func(t *testing.T) {
		matrix := NewComplianceMatrix()

		control := &Control{
			ID:          "TEST-001",
			Framework:   FrameworkGDPR,
			Category:    "test-category",
			Title:       "Test Control",
			Description: "A test control",
			Requirement: "Must test",
			Status:      "implemented",
		}

		matrix.RegisterControl(control)

		retrieved, ok := matrix.GetControl("TEST-001")
		if !ok {
			t.Fatal("Expected to retrieve control")
		}

		if retrieved.ID != "TEST-001" {
			t.Errorf("Expected ID TEST-001, got %s", retrieved.ID)
		}

		if retrieved.Title != "Test Control" {
			t.Errorf("Expected title 'Test Control', got %s", retrieved.Title)
		}
	})

	t.Run("Get All Controls", func(t *testing.T) {
		matrix := NewComplianceMatrix()

		matrix.RegisterControl(&Control{
			ID:        "TEST-001",
			Framework: FrameworkGDPR,
		})
		matrix.RegisterControl(&Control{
			ID:        "TEST-002",
			Framework: FrameworkGDPR,
		})

		controls := matrix.GetAllControls()

		if len(controls) != 2 {
			t.Errorf("Expected 2 controls, got %d", len(controls))
		}
	})

	t.Run("Get Controls By Framework", func(t *testing.T) {
		matrix := NewComplianceMatrix()

		matrix.RegisterControl(&Control{
			ID:        "GDPR-001",
			Framework: FrameworkGDPR,
		})
		matrix.RegisterControl(&Control{
			ID:        "HIPAA-001",
			Framework: FrameworkHIPAA,
		})

		gdprControls := matrix.GetControlsByFramework(FrameworkGDPR)

		if len(gdprControls) != 1 {
			t.Errorf("Expected 1 GDPR control, got %d", len(gdprControls))
		}

		if gdprControls[0].ID != "GDPR-001" {
			t.Errorf("Expected GDPR-001, got %s", gdprControls[0].ID)
		}
	})

	t.Run("Add and Get Evidence", func(t *testing.T) {
		matrix := NewComplianceMatrix()

		control := &Control{
			ID:        "TEST-001",
			Framework: FrameworkGDPR,
		}
		matrix.RegisterControl(control)

		evidence := Evidence{
			Type:   "test",
			Source: "test-file.go",
		}

		matrix.AddEvidence("TEST-001", evidence)

		evidenceList := matrix.GetEvidence("TEST-001")

		if len(evidenceList) != 1 {
			t.Errorf("Expected 1 evidence, got %d", len(evidenceList))
		}

		if evidenceList[0].Type != "test" {
			t.Errorf("Expected type 'test', got %s", evidenceList[0].Type)
		}
	})
}

// TestGenerateReport tests compliance report generation
func TestGenerateReport(t *testing.T) {
	t.Run("Generate Report with All Checks", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()

		report, err := matrix.GenerateReport([]Framework{
			FrameworkBible,
			FrameworkOpenAPI,
			FrameworkGDPR,
		})

		if err != nil {
			t.Fatalf("Failed to generate report: %v", err)
		}

		if report.ID == "" {
			t.Error("Expected non-empty report ID")
		}

		if report.Timestamp.IsZero() {
			t.Error("Expected non-zero timestamp")
		}

		// Should have results from all check types
		if len(report.Results) == 0 {
			t.Error("Expected results from compliance checks")
		}

		// Should have controls from the matrix
		if len(report.Controls) == 0 {
			t.Error("Expected controls in report")
		}

		// Score should be calculated
		if report.TotalScore < 0 || report.TotalScore > 1 {
			t.Errorf("Expected score between 0 and 1, got %f", report.TotalScore)
		}

		t.Logf("Generated report with score: %.2f%%", report.TotalScore*100)
	})

	t.Run("Report with Summary Statistics", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()

		// Add some test controls
		matrix.RegisterControl(&Control{
			ID:          "TEST-001",
			Framework:   FrameworkGDPR,
			Status:      "implemented",
			Title:       "Test 1",
			Description: "Test",
		})
		matrix.RegisterControl(&Control{
			ID:          "TEST-002",
			Framework:   FrameworkGDPR,
			Status:      "partial",
			Title:       "Test 2",
			Description: "Test",
		})
		matrix.RegisterControl(&Control{
			ID:          "TEST-003",
			Framework:   FrameworkGDPR,
			Status:      "not-implemented",
			Title:       "Test 3",
			Description: "Test",
		})

		report, err := matrix.GenerateReport([]Framework{FrameworkBible})
		if err != nil {
			t.Fatalf("Failed to generate report: %v", err)
		}

		if report.Summary.TotalControls == 0 {
			t.Error("Expected total controls count")
		}

		t.Logf("Controls - Total: %d, Implemented: %d, Partial: %d, Not Implemented: %d",
			report.Summary.TotalControls,
			report.Summary.Implemented,
			report.Summary.Partial,
			report.Summary.NotImplemented)
	})
}

// TestReportExport tests report export functionality
func TestReportExport(t *testing.T) {
	t.Run("Export to JSON", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()
		report, _ := matrix.GenerateReport([]Framework{FrameworkBible})

		tempDir := t.TempDir()
		jsonPath := filepath.Join(tempDir, "report.json")

		err := ExportReport(report, "json", jsonPath)
		if err != nil {
			t.Fatalf("Failed to export JSON: %v", err)
		}

		// Verify file exists and is valid JSON
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			t.Fatalf("Failed to read exported JSON: %v", err)
		}

		var exportedReport Report
		if err := json.Unmarshal(data, &exportedReport); err != nil {
			t.Fatalf("Exported JSON is invalid: %v", err)
		}

		if exportedReport.ID != report.ID {
			t.Error("Exported report ID mismatch")
		}
	})

	t.Run("Export to Markdown", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()
		report, _ := matrix.GenerateReport([]Framework{FrameworkBible})

		tempDir := t.TempDir()
		mdPath := filepath.Join(tempDir, "report.md")

		err := ExportReport(report, "markdown", mdPath)
		if err != nil {
			t.Fatalf("Failed to export Markdown: %v", err)
		}

		// Verify file exists and contains expected content
		data, err := os.ReadFile(mdPath)
		if err != nil {
			t.Fatalf("Failed to read exported Markdown: %v", err)
		}

		content := string(data)
		if !strings.Contains(content, "# Compliance Report") {
			t.Error("Markdown missing title")
		}

		if !strings.Contains(content, "Compliance Report") {
			t.Error("Markdown missing report header")
		}
	})

	t.Run("Export to CSV", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()
		report, _ := matrix.GenerateReport([]Framework{FrameworkBible})

		tempDir := t.TempDir()
		csvPath := filepath.Join(tempDir, "report.csv")

		err := ExportReport(report, "csv", csvPath)
		if err != nil {
			t.Fatalf("Failed to export CSV: %v", err)
		}

		// Verify file exists and contains CSV header
		data, err := os.ReadFile(csvPath)
		if err != nil {
			t.Fatalf("Failed to read exported CSV: %v", err)
		}

		content := string(data)
		if !strings.Contains(content, "CheckName,Framework,ControlID") {
			t.Error("CSV missing expected header")
		}
	})

	t.Run("Export to SARIF", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()
		report, _ := matrix.GenerateReport([]Framework{FrameworkBible})

		tempDir := t.TempDir()
		sarifPath := filepath.Join(tempDir, "report.sarif")

		err := ExportReport(report, "sarif", sarifPath)
		if err != nil {
			t.Fatalf("Failed to export SARIF: %v", err)
		}

		// Verify file exists and is valid SARIF JSON
		data, err := os.ReadFile(sarifPath)
		if err != nil {
			t.Fatalf("Failed to read exported SARIF: %v", err)
		}

		var sarif SARIFReport
		if err := json.Unmarshal(data, &sarif); err != nil {
			t.Fatalf("Exported SARIF is invalid: %v", err)
		}

		if sarif.Version != "2.1.0" {
			t.Errorf("Expected SARIF version 2.1.0, got %s", sarif.Version)
		}

		if len(sarif.Runs) == 0 {
			t.Error("SARIF missing runs")
		}
	})

	t.Run("Export Unsupported Format", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()
		report, _ := matrix.GenerateReport([]Framework{FrameworkBible})

		err := ExportReport(report, "xml", "/tmp/test.xml")
		if err == nil {
			t.Error("Expected error for unsupported format")
		}
	})
}

// TestEvidenceCollection tests evidence collection functionality
func TestEvidenceCollection(t *testing.T) {
	t.Run("Collect Evidence from File", func(t *testing.T) {
		// Create a temporary file
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.go")
		content := []byte("package main\n\nfunc main() {}\n")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		evidence, err := CollectEvidenceFromFile(testFile)
		if err != nil {
			t.Fatalf("Failed to collect evidence: %v", err)
		}

		if evidence.Type != "code" {
			t.Errorf("Expected type 'code', got %s", evidence.Type)
		}

		if evidence.Source != testFile {
			t.Errorf("Expected source %s, got %s", testFile, evidence.Source)
		}

		if evidence.Timestamp.IsZero() {
			t.Error("Expected non-zero timestamp")
		}
	})

	t.Run("Collect Evidence from Non-existent File", func(t *testing.T) {
		_, err := CollectEvidenceFromFile("/nonexistent/path/file.go")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

// TestBibleStandardCheck tests Bible Standard compliance check
func TestBibleStandardCheck(t *testing.T) {
	t.Run("Validate Bible Standard", func(t *testing.T) {
		check := &BibleStandardCheck{}
		result := check.Validate()

		// Should return a result with framework info
		if result.Framework != FrameworkBible {
			t.Errorf("Expected framework %s, got %s", FrameworkBible, result.Framework)
		}

		if result.CheckName != "Bible Standard Compliance" {
			t.Errorf("Expected check name 'Bible Standard Compliance', got %s", result.CheckName)
		}

		// Score should be between 0 and 1
		if result.Score < 0 || result.Score > 1 {
			t.Errorf("Expected score between 0 and 1, got %f", result.Score)
		}

		t.Logf("Bible Standard Check: Passed=%v, Score=%.2f%%", result.Passed, result.Score*100)
	})
}

// TestOpenAPIComplianceCheck tests OpenAPI compliance check
func TestOpenAPIComplianceCheck(t *testing.T) {
	t.Run("Validate OpenAPI Compliance", func(t *testing.T) {
		check := &OpenAPIComplianceCheck{}
		result := check.Validate()

		if result.Framework != FrameworkOpenAPI {
			t.Errorf("Expected framework %s, got %s", FrameworkOpenAPI, result.Framework)
		}

		t.Logf("OpenAPI Check: Passed=%v, Score=%.2f%%", result.Passed, result.Score*100)
	})
}

// TestMCPComplianceCheck tests MCP Protocol compliance check
func TestMCPComplianceCheck(t *testing.T) {
	t.Run("Validate MCP Compliance", func(t *testing.T) {
		check := &MCPComplianceCheck{}
		result := check.Validate()

		if result.Framework != FrameworkMCP {
			t.Errorf("Expected framework %s, got %s", FrameworkMCP, result.Framework)
		}

		t.Logf("MCP Check: Passed=%v, Score=%.2f%%", result.Passed, result.Score*100)
	})
}

// TestOAuth2ComplianceCheck tests OAuth2 compliance check
func TestOAuth2ComplianceCheck(t *testing.T) {
	t.Run("Validate OAuth2 Compliance", func(t *testing.T) {
		check := &OAuth2ComplianceCheck{}
		result := check.Validate()

		if result.Framework != FrameworkOAuth2 {
			t.Errorf("Expected framework %s, got %s", FrameworkOAuth2, result.Framework)
		}

		t.Logf("OAuth2 Check: Passed=%v, Score=%.2f%%", result.Passed, result.Score*100)
	})
}

// TestGDPRComplianceCheck tests GDPR compliance check
func TestGDPRComplianceCheck(t *testing.T) {
	t.Run("Validate GDPR Compliance", func(t *testing.T) {
		check := &GDPRComplianceCheck{}
		result := check.Validate()

		if result.Framework != FrameworkGDPR {
			t.Errorf("Expected framework %s, got %s", FrameworkGDPR, result.Framework)
		}

		t.Logf("GDPR Check: Passed=%v, Score=%.2f%%", result.Passed, result.Score*100)
	})
}

// TestComplianceMatrixJSON tests JSON serialization
func TestComplianceMatrixJSON(t *testing.T) {
	t.Run("Save and Load Matrix from JSON", func(t *testing.T) {
		matrix := InitializeComplianceMatrix()

		// Add test control
		matrix.RegisterControl(&Control{
			ID:          "JSON-001",
			Framework:   FrameworkGDPR,
			Category:    "test",
			Title:       "JSON Test",
			Description: "Test control for JSON",
			Requirement: "Must test JSON",
			Status:      "implemented",
			Implementation: Implementation{
				Files:      []string{"test.go"},
				Tests:      []string{"test_test.go"},
				Status:     "complete",
				VerifiedAt: time.Now(),
				VerifiedBy: "test",
			},
		})

		// Get all controls and save to JSON
		controls := matrix.GetAllControls()
		data, err := json.MarshalIndent(controls, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal controls: %v", err)
		}

		tempDir := t.TempDir()
		jsonPath := filepath.Join(tempDir, "controls.json")
		if err := os.WriteFile(jsonPath, data, 0644); err != nil {
			t.Fatalf("Failed to write controls file: %v", err)
		}

		// Load from JSON
		loadedMatrix, err := LoadComplianceMatrixFromJSON(jsonPath)
		if err != nil {
			t.Fatalf("Failed to load matrix from JSON: %v", err)
		}

		// Verify loaded data
		control, ok := loadedMatrix.GetControl("JSON-001")
		if !ok {
			t.Fatal("Expected to retrieve loaded control")
		}

		if control.Title != "JSON Test" {
			t.Errorf("Expected title 'JSON Test', got %s", control.Title)
		}
	})
}

// TestSeverity tests severity levels
func TestSeverity(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityInfo, "INFO"},
		{SeverityWarning, "WARNING"},
		{SeverityError, "ERROR"},
		{SeverityCritical, "CRITICAL"},
		{Severity(99), "UNKNOWN"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.severity.String()
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

// TestFrameworkConstants tests framework constants
func TestFrameworkConstants(t *testing.T) {
	frameworks := []Framework{
		FrameworkBible,
		FrameworkOpenAPI,
		FrameworkMCP,
		FrameworkOAuth2,
		FrameworkTLS,
		FrameworkGDPR,
		FrameworkHIPAA,
		FrameworkPCI,
		FrameworkFIPS1403,
		FrameworkLinuxFHS,
		FrameworkMacSandbox,
		FrameworkWindowsCert,
		FrameworkDocker,
		FrameworkK8s,
	}

	for _, fw := range frameworks {
		t.Run(string(fw), func(t *testing.T) {
			if fw == "" {
				t.Error("Framework should not be empty")
			}
		})
	}
}

// TestInitializeComplianceMatrix tests matrix initialization
func TestInitializeComplianceMatrix(t *testing.T) {
	matrix := InitializeComplianceMatrix()

	// Should have some controls registered
	controls := matrix.GetAllControls()
	if len(controls) == 0 {
		t.Error("Expected initialized matrix to have controls")
	}

	// Should have Bible Standard controls
	bibleControls := matrix.GetControlsByFramework(FrameworkBible)
	if len(bibleControls) == 0 {
		t.Error("Expected Bible Standard controls")
	}

	// Should have GDPR controls
	gdprControls := matrix.GetControlsByFramework(FrameworkGDPR)
	if len(gdprControls) == 0 {
		t.Error("Expected GDPR controls")
	}

	// Should have OpenAPI controls
	openapiControls := matrix.GetControlsByFramework(FrameworkOpenAPI)
	if len(openapiControls) == 0 {
		t.Error("Expected OpenAPI controls")
	}

	t.Logf("Initialized matrix with %d controls", len(controls))
	for _, c := range controls {
		t.Logf("  - %s: %s", c.ID, c.Title)
	}
}

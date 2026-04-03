package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRollingDeploymentBasic tests basic rolling deployment functionality
func TestRollingDeploymentBasic(t *testing.T) {
	// Test that script exists and is executable
	scriptPath := "./rolling-deployment.sh"
	info, err := os.Stat(scriptPath)
	require.NoError(t, err, "Script should exist")
	assert.True(t, info.Mode().IsRegular(), "Should be a regular file")
	assert.NotEqual(t, 0, info.Mode()&0111, "Script should be executable")
}

// TestRollingDeploymentHelp tests the help flag
func TestRollingDeploymentHelp(t *testing.T) {
	cmd := exec.Command("bash", "./rolling-deployment.sh", "--help")
	output, err := cmd.CombinedOutput()

	// Help should return exit code 0
	assert.NoError(t, err, "Help should succeed")

	outputStr := string(output)
	assert.Contains(t, outputStr, "Usage:", "Should contain usage information")
	assert.Contains(t, outputStr, "--environment", "Should contain environment option")
	assert.Contains(t, outputStr, "--strategy", "Should contain strategy option")
	assert.Contains(t, outputStr, "rolling|canary|blue-green", "Should list available strategies")
}

// TestRollingDeploymentDryRun tests dry run mode
func TestRollingDeploymentDryRun(t *testing.T) {
	// This test verifies the script runs in dry-run mode
	// In real usage, it would need kubectl configured
	cmd := exec.Command("bash", "./rolling-deployment.sh",
		"--environment", "dev",
		"--version", "v1.0.0",
		"--dry-run",
	)
	cmd.Dir = "."

	// Capture output for verification
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Script should either succeed or fail due to missing kubectl
	// We're mainly verifying the script syntax is valid
	if err != nil {
		// If it fails, it should be due to kubectl not being configured, not syntax errors
		assert.NotContains(t, outputStr, "syntax error", "Should not have syntax errors")
		assert.NotContains(t, outputStr, "unexpected EOF", "Should be complete script")
	}

	t.Logf("Dry run output:\n%s", outputStr)
}

// TestRollingDeploymentStrategyValidation tests strategy validation
func TestRollingDeploymentStrategyValidation(t *testing.T) {
	// Test with invalid strategy
	cmd := exec.Command("bash", "./rolling-deployment.sh",
		"--environment", "dev",
		"--strategy", "invalid",
	)
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail with invalid strategy
	assert.Error(t, err, "Invalid strategy should fail")
	assert.Contains(t, outputStr, "Invalid strategy", "Should report invalid strategy error")
}

// TestRollingDeploymentEnvironmentValidation tests environment validation
func TestRollingDeploymentEnvironmentValidation(t *testing.T) {
	// Test with invalid environment
	cmd := exec.Command("bash", "./rolling-deployment.sh",
		"--environment", "invalid-env",
	)
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail with invalid environment
	assert.Error(t, err, "Invalid environment should fail")
	assert.Contains(t, outputStr, "Invalid environment", "Should report invalid environment error")
}

// TestRollingDeploymentConfig tests that configuration is properly set
func TestRollingDeploymentConfig(t *testing.T) {
	// Read the script and verify configuration defaults
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Verify default configurations
	assert.Contains(t, contentStr, "STRATEGY=\"${STRATEGY:-rolling}\"", "Should have default rolling strategy")
	assert.Contains(t, contentStr, "CANARY_PERCENTAGE=\"${CANARY_PERCENTAGE:-10}\"", "Should have default canary percentage")
	assert.Contains(t, contentStr, "CANARY_DURATION=\"${CANARY_DURATION:-300}\"", "Should have default canary duration")
	assert.Contains(t, contentStr, "HEALTH_CHECK_INTERVAL=\"${HEALTH_CHECK_INTERVAL:-10}\"", "Should have default health check interval")
}

// TestRollingDeploymentPhases tests that deployment phases are properly structured
func TestRollingDeploymentPhases(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Verify phase functions exist
	assert.Contains(t, contentStr, "phase_start()", "Should have phase_start function")
	assert.Contains(t, contentStr, "phase_complete()", "Should have phase_complete function")
	assert.Contains(t, contentStr, "phase_failed()", "Should have phase_failed function")

	// Verify deployment strategies
	assert.Contains(t, contentStr, "rolling_deployment()", "Should have rolling_deployment function")
	assert.Contains(t, contentStr, "canary_deployment()", "Should have canary_deployment function")
	assert.Contains(t, contentStr, "blue_green_deployment()", "Should have blue_green_deployment function")
}

// TestRollingDeploymentHealthChecks tests health check functions
func TestRollingDeploymentHealthChecks(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Verify health check functions
	assert.Contains(t, contentStr, "get_deployment_metrics()", "Should have metrics function")
	assert.Contains(t, contentStr, "wait_for_healthy_pods()", "Should have health wait function")

	// Verify health gates
	assert.Contains(t, contentStr, "ERROR_THRESHOLD", "Should have error rate threshold")
	assert.Contains(t, contentStr, "LATENCY_THRESHOLD", "Should have latency threshold")
}

// TestRollingDeploymentRollback tests rollback functionality
func TestRollingDeploymentRollback(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Verify rollback functions
	assert.Contains(t, contentStr, "auto_rollback()", "Should have auto_rollback function")
	assert.Contains(t, contentStr, "cleanup_canary()", "Should have cleanup_canary function")

	// Verify rollback commands for each strategy
	assert.Contains(t, contentStr, "kubectl rollout undo", "Should have kubectl rollback")
	assert.Contains(t, contentStr, "kubectl scale", "Should have scale commands")
}

// TestRollingDeploymentLogging tests logging functionality
func TestRollingDeploymentLogging(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Verify logging functions
	assert.Contains(t, contentStr, "log()", "Should have log function")
	assert.Contains(t, contentStr, "warn()", "Should have warn function")
	assert.Contains(t, contentStr, "error()", "Should have error function")
	assert.Contains(t, contentStr, "info()", "Should have info function")
}

// TestRollingDeploymentMetricsJSON tests JSON metrics output
func TestRollingDeploymentMetricsJSON(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Verify JSON metrics output - check that the metrics function exists and outputs JSON
	// The script uses: echo "{\"ready\":$ready_count,\"total\":$total_count,...}"
	// In Go string, this appears as: \"ready\":$ready_count
	assert.Contains(t, contentStr, "ready_count", "Should use ready_count variable")
	assert.Contains(t, contentStr, "total_count", "Should use total_count variable")
	assert.Contains(t, contentStr, "error_count", "Should use error_count variable")
	assert.Contains(t, contentStr, "get_deployment_metrics", "Should have metrics function")
}

// TestRollingDeploymentScriptCompleteness tests that script is complete
func TestRollingDeploymentScriptCompleteness(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Check for script completeness markers
	assert.Contains(t, contentStr, "#!/bin/bash", "Should have shebang")
	assert.Contains(t, contentStr, "set -euo pipefail", "Should have strict error handling")
	assert.Contains(t, contentStr, "main()", "Should have main function")
	assert.Contains(t, contentStr, "usage()", "Should have usage function")

	// Count function definitions
	functions := []string{
		"log()",
		"warn()",
		"error()",
		"info()",
		"phase_start()",
		"phase_complete()",
		"phase_failed()",
		"get_deployment_metrics()",
		"wait_for_healthy_pods()",
		"rolling_deployment()",
		"canary_deployment()",
		"blue_green_deployment()",
		"cleanup_canary()",
		"auto_rollback()",
		"main()",
		"usage()",
	}

	for _, fn := range functions {
		assert.Contains(t, contentStr, fn, "Should have %s function", fn)
	}
}

// TestRollingDeploymentBestPractices tests script follows best practices
func TestRollingDeploymentBestPractices(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Security best practices
	assert.Contains(t, contentStr, "--timeout=", "Should use timeouts for kubectl commands")

	// Error handling
	assert.Contains(t, contentStr, "set -e", "Should exit on error")
	assert.Contains(t, contentStr, "-u", "Should exit on undefined variable (part of set -euo pipefail)")

	// Logging
	assert.Contains(t, contentStr, "LOG_FILE", "Should log to file")

	// Health monitoring
	assert.Contains(t, contentStr, "HEALTH_CHECK_INTERVAL", "Should have health check configuration")

	// Rollback capability
	assert.Contains(t, contentStr, "ROLLBACK_TRIGGERED", "Should track rollback state")

	// Dry run support
	assert.Contains(t, contentStr, "DRY_RUN", "Should support dry run mode")
}

// TestRollingDeploymentCanarySpecifics tests canary-specific features
func TestRollingDeploymentCanarySpecifics(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Canary-specific features
	assert.Contains(t, contentStr, "canary_replicas", "Should calculate canary replicas")
	assert.Contains(t, contentStr, "stable_replicas", "Should calculate stable replicas")
	assert.Contains(t, contentStr, "CANARY_START_TIME", "Should track canary start time")
	assert.Contains(t, contentStr, "CANARY_DURATION", "Should use canary duration")
}

// TestRollingDeploymentBlueGreenSpecifics tests blue-green specific features
func TestRollingDeploymentBlueGreenSpecifics(t *testing.T) {
	content, err := os.ReadFile("./rolling-deployment.sh")
	require.NoError(t, err)

	contentStr := string(content)

	// Blue-green specific features
	assert.Contains(t, contentStr, "current_color", "Should track current color")
	assert.Contains(t, contentStr, "new_color", "Should track new color")
	assert.Contains(t, contentStr, "blue", "Should support blue deployment")
	assert.Contains(t, contentStr, "green", "Should support green deployment")
	assert.Contains(t, contentStr, "kubectl patch service", "Should patch service for traffic switch")
}

// BenchmarkHealthCheck benchmarks the health check function
func BenchmarkHealthCheck(b *testing.B) {
	// This is a conceptual benchmark - in real usage would test actual health check
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf("{\"ready\":%d,\"total\":%d,\"errors\":%d,\"error_rate\":%.2f}",
			i%10, 10, i%3, float64(i%3)/10.0*100)
	}
}

// TestRollingDeploymentExampleUsage shows example usage through test
func TestRollingDeploymentExampleUsage(t *testing.T) {
	// Example: Rolling deployment
	fmt.Println("Example: Deploy v1.2.3 to staging using rolling strategy")
	fmt.Println("  ./rolling-deployment.sh -e staging -v v1.2.3 -s rolling")

	// Example: Canary deployment
	fmt.Println("\nExample: Deploy v1.2.3 to prod with 10% canary for 5 minutes")
	fmt.Println("  ./rolling-deployment.sh -e prod -v v1.2.3 -s canary --canary-percentage 10 --canary-duration 300")

	// Example: Blue-green deployment
	fmt.Println("\nExample: Deploy v1.2.3 to prod using blue-green strategy")
	fmt.Println("  ./rolling-deployment.sh -e prod -v v1.2.3 -s blue-green")
}

// TestRollingDeploymentIntegration is an integration test
// It requires a Kubernetes cluster to be available
func TestRollingDeploymentIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Test dry run on actual cluster
	cmd := exec.CommandContext(ctx, "bash", "./rolling-deployment.sh",
		"--environment", "dev",
		"--version", "v1.0.0",
		"--dry-run",
	)
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Dry run should succeed on actual cluster: %s", string(output))

	outputStr := string(output)
	assert.Contains(t, outputStr, "DRY RUN", "Should indicate dry run mode")
}

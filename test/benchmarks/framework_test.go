// Package benchmarks provides tests for the benchmark framework
//
// Tests:
//
//   - TestBenchmarkConfig: Benchmark configuration
//   - TestBenchmarkResult: Result collection
//   - TestPerformanceBudget: Budget validation
//   - TestBudgetRegistry: Budget management
package benchmarks

import (
	"testing"
	"time"
)

// TestBenchmarkConfig validates default configuration
func TestBenchmarkConfig(t *testing.T) {
	config := DefaultBenchmarkConfig()

	if config.MinIterations != 1000 {
		t.Errorf("MinIterations = %d, want 1000", config.MinIterations)
	}

	if config.MaxDuration != 30*time.Second {
		t.Errorf("MaxDuration = %v, want 30s", config.MaxDuration)
	}

	if config.WarmupDuration != 100*time.Millisecond {
		t.Errorf("WarmupDuration = %v, want 100ms", config.WarmupDuration)
	}

	if config.EnableProfiling {
		t.Error("EnableProfiling should be false by default")
	}

	if config.SaveResults {
		t.Error("SaveResults should be false by default")
	}
}

// TestBenchmarkConfig_Custom validates custom configuration
func TestBenchmarkConfig_Custom(t *testing.T) {
	config := &BenchmarkConfig{
		MinIterations:   500,
		MaxDuration:     10 * time.Second,
		WarmupDuration:  50 * time.Millisecond,
		EnableProfiling: true,
		SaveResults:     true,
		ResultsPath:     "/custom/path",
	}

	if config.MinIterations != 500 {
		t.Errorf("MinIterations = %d, want 500", config.MinIterations)
	}

	if config.MaxDuration != 10*time.Second {
		t.Errorf("MaxDuration = %v, want 10s", config.MaxDuration)
	}
}

// TestBenchmarkResult validates result creation
func TestBenchmarkResult(t *testing.T) {
	result := BenchmarkResult{
		Name:        "test-benchmark",
		Iterations:  1000,
		Duration:    1 * time.Second,
		NsPerOp:     1000000, // 1ms per operation
		BytesPerOp:  100,
		AllocsPerOp: 2,
	}

	if result.Name != "test-benchmark" {
		t.Errorf("Name = %s, want test-benchmark", result.Name)
	}

	if result.Iterations != 1000 {
		t.Errorf("Iterations = %d, want 1000", result.Iterations)
	}
}

// TestPerformanceBudget_Validation validates budget checking
func TestPerformanceBudget_Validation(t *testing.T) {
	budget := &PerformanceBudget{
		MaxLatency: 100 * time.Millisecond,
		MaxMemory:  1024,
		MaxAllocs:  10,
	}

	// Test within budget
	result := &BenchmarkResult{
		NsPerOp:     50 * 1000000, // 50ms
		BytesPerOp:  512,
		AllocsPerOp: 5,
	}

	if err := budget.Check(result); err != nil {
		t.Errorf("Expected within budget, got error: %v", err)
	}

	// Test latency exceeded
	result.NsPerOp = 200 * 1000000 // 200ms
	if err := budget.Check(result); err == nil {
		t.Error("Expected latency budget exceeded error")
	}

	// Test memory exceeded
	result.NsPerOp = 50 * 1000000
	result.BytesPerOp = 2048
	if err := budget.Check(result); err == nil {
		t.Error("Expected memory budget exceeded error")
	}

	// Test allocations exceeded
	result.BytesPerOp = 512
	result.AllocsPerOp = 20
	if err := budget.Check(result); err == nil {
		t.Error("Expected allocation budget exceeded error")
	}
}

// TestPerformanceBudget_Unlimited validates unlimited budgets
func TestPerformanceBudget_Unlimited(t *testing.T) {
	// Budget with no limits
	budget := &PerformanceBudget{
		MaxLatency: 0,
		MaxMemory:  0,
		MaxAllocs:  0,
	}

	result := &BenchmarkResult{
		NsPerOp:     1000 * 1000000000, // 1000 seconds
		BytesPerOp:  1000000,
		AllocsPerOp: 10000,
	}

	if err := budget.Check(result); err != nil {
		t.Errorf("Unlimited budget should not fail: %v", err)
	}
}

// TestBudgetRegistry_GetBudget validates budget retrieval
func TestBudgetRegistry_GetBudget(t *testing.T) {
	// Test existing budget
	budget := GetBudget("Cache_Set")
	if budget == nil {
		t.Error("Expected budget for Cache_Set, got nil")
	}

	// Test non-existent budget
	budget = GetBudget("NonExistent")
	if budget != nil {
		t.Error("Expected nil for non-existent budget")
	}
}

// TestBudgetRegistry_GetBudgetOrDefault validates default fallback
func TestBudgetRegistry_GetBudgetOrDefault(t *testing.T) {
	// Test existing budget
	budget := GetBudgetOrDefault("Cache_Set")
	if budget == nil {
		t.Fatal("Expected budget for Cache_Set")
	}

	// Test default budget
	budget = GetBudgetOrDefault("NonExistent")
	if budget.MaxLatency != 1*time.Second {
		t.Errorf("Default MaxLatency = %v, want 1s", budget.MaxLatency)
	}

	if budget.MaxMemory != 65536 {
		t.Errorf("Default MaxMemory = %d, want 65536", budget.MaxMemory)
	}
}

// TestBudgetRegistry_AddBudget validates adding budgets
func TestBudgetRegistry_AddBudget(t *testing.T) {
	newBudget := &PerformanceBudget{
		MaxLatency: 500 * time.Millisecond,
		MaxMemory:  2048,
		MaxAllocs:  20,
	}

	AddBudget("Custom_Budget", newBudget)

	budget := GetBudget("Custom_Budget")
	if budget == nil {
		t.Fatal("Expected to find Custom_Budget")
	}

	if budget.MaxLatency != 500*time.Millisecond {
		t.Errorf("MaxLatency = %v, want 500ms", budget.MaxLatency)
	}

	// Cleanup
	RemoveBudget("Custom_Budget")
}

// TestBudgetRegistry_ListBudgets validates budget listing
func TestBudgetRegistry_ListBudgets(t *testing.T) {
	budgets := ListBudgets()

	// Should have many budgets defined
	if len(budgets) == 0 {
		t.Error("Expected budgets to be defined")
	}

	// Check for known budgets
	hasCacheSet := false
	for _, name := range budgets {
		if name == "Cache_Set" {
			hasCacheSet = true
			break
		}
	}

	if !hasCacheSet {
		t.Error("Expected Cache_Set budget to exist")
	}
}

// TestBudgetCategories validates category organization
func TestBudgetCategories(t *testing.T) {
	// Check that categories are defined
	if len(BudgetCategories) == 0 {
		t.Error("Expected budget categories to be defined")
	}

	// Check first category
	category := BudgetCategories[0]
	if category.Name != "Authentication" {
		t.Errorf("First category = %s, want Authentication", category.Name)
	}

	if category.Priority != 1 {
		t.Errorf("Authentication priority = %d, want 1", category.Priority)
	}
}

// TestPerformanceTiers validates tier definitions
func TestPerformanceTiers(t *testing.T) {
	// Test strict tier
	strict := PerformanceTiers["strict"]
	if strict == nil {
		t.Fatal("Expected strict tier")
	}

	if strict.LatencyMultiplier != 1.0 {
		t.Errorf("Strict latency multiplier = %f, want 1.0", strict.LatencyMultiplier)
	}

	// Test lenient tier
	lenient := PerformanceTiers["lenient"]
	if lenient == nil {
		t.Fatal("Expected lenient tier")
	}

	if lenient.LatencyMultiplier != 2.0 {
		t.Errorf("Lenient latency multiplier = %f, want 2.0", lenient.LatencyMultiplier)
	}
}

// TestGetTieredBudget validates tiered budget calculation
func TestGetTieredBudget(t *testing.T) {
	base := GetBudget("Cache_Set")
	if base == nil {
		t.Skip("Cache_Set budget not found")
	}

	// Test strict tier
	strictBudget := GetTieredBudget("Cache_Set", "strict")
	if strictBudget == nil {
		t.Fatal("Expected strict budget")
	}

	// Should be same as base
	if strictBudget.MaxLatency != base.MaxLatency {
		t.Error("Strict budget should equal base budget")
	}

	// Test lenient tier
	lenientBudget := GetTieredBudget("Cache_Set", "lenient")
	if lenientBudget == nil {
		t.Fatal("Expected lenient budget")
	}

	// Should be 2x base
	expectedLatency := time.Duration(float64(base.MaxLatency) * 2.0)
	if lenientBudget.MaxLatency != expectedLatency {
		t.Errorf("Lenient latency = %v, want %v", lenientBudget.MaxLatency, expectedLatency)
	}

	// Test non-existent tier
	unknownBudget := GetTieredBudget("Cache_Set", "nonexistent")
	if unknownBudget.MaxLatency != base.MaxLatency {
		t.Error("Unknown tier should return base budget")
	}
}

// TestBenchmarkRunner validates runner creation
func TestBenchmarkRunner(t *testing.T) {
	config := DefaultBenchmarkConfig()
	runner := NewBenchmarkRunner(config)

	if runner == nil {
		t.Fatal("Expected runner to be created")
	}

	if runner.config != config {
		t.Error("Runner should use provided config")
	}

	results := runner.GetResults()
	if len(results) != 0 {
		t.Error("New runner should have empty results")
	}
}

// TestBenchmarkRunner_NilConfig validates runner with nil config
func TestBenchmarkRunner_NilConfig(t *testing.T) {
	runner := NewBenchmarkRunner(nil)

	if runner == nil {
		t.Fatal("Expected runner to be created with default config")
	}

	if runner.config == nil {
		t.Error("Runner should have default config")
	}

	// Should have default values
	if runner.config.MinIterations != 1000 {
		t.Error("Default config should have 1000 min iterations")
	}
}

// TestBenchmarkRunner_SaveResults validates results saving
func TestBenchmarkRunner_SaveResults(t *testing.T) {
	config := &BenchmarkConfig{
		SaveResults: true,
		ResultsPath: t.TempDir() + "/results",
	}

	runner := NewBenchmarkRunner(config)

	// Save should succeed even with no results
	err := runner.SaveResults()
	if err != nil {
		t.Errorf("SaveResults should succeed with no results: %v", err)
	}
}

// TestBenchmarkHelper validates helper functions
func TestBenchmarkHelper(t *testing.T) {
	runner := NewBenchmarkRunner(nil)
	helper := NewBenchmarkHelper(runner)

	if helper == nil {
		t.Fatal("Expected helper to be created")
	}

	if helper.runner != runner {
		t.Error("Helper should reference runner")
	}
}

// TestCriticalPathBudgets validates critical path budget definitions
func TestCriticalPathBudgets(t *testing.T) {
	// Test that all critical paths have reasonable budgets
	criticalPaths := []string{
		"Auth_OAuth2TokenGeneration",
		"Auth_OAuth2TokenRefresh",
		"Auth_MTLSHandshake",
		"Cache_Set",
		"Cache_Get",
		"Cache_Delete",
		"Request_BuilderCreate",
		"Request_WithHeader",
		"Request_WithBody",
		"Request_Execute",
	}

	for _, path := range criticalPaths {
		budget := GetBudget(path)
		if budget == nil {
			t.Errorf("Missing budget for critical path: %s", path)
			continue
		}

		// Validate budget values are reasonable
		if budget.MaxLatency <= 0 {
			t.Errorf("%s: MaxLatency should be positive", path)
		}

		if budget.MaxMemory <= 0 {
			t.Errorf("%s: MaxMemory should be positive", path)
		}

		if budget.MaxAllocs <= 0 {
			t.Errorf("%s: MaxAllocs should be positive", path)
		}
	}
}

// BenchmarkFramework_BudgetLookup measures budget lookup performance
func BenchmarkFramework_BudgetLookup(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = GetBudget("Cache_Set")
	}
}

// BenchmarkFramework_BudgetCheck measures budget check performance
func BenchmarkFramework_BudgetCheck(b *testing.B) {
	budget := GetBudgetOrDefault("Cache_Set")
	result := &BenchmarkResult{
		NsPerOp:     500000, // 500µs
		BytesPerOp:  200,
		AllocsPerOp: 2,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = budget.Check(result)
	}
}

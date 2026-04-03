// Package benchmarks provides performance budgets for critical paths
//
// Performance Budgets:
//
// Performance budgets define maximum acceptable resource usage for critical
// operations. Benchmarks exceeding these budgets will fail, ensuring
// performance regressions are caught during development.
//
// Budget Categories:
//
//   - P50Latency: 50th percentile latency (median)
//   - P95Latency: 95th percentile latency
//   - P99Latency: 99th percentile latency (tail latency)
//   - Memory: Memory allocations per operation
//   - Allocations: Number of allocations per operation
//
// Critical Paths:
//
// The following critical paths have defined performance budgets:
//   - Authentication: OAuth2 token operations
//   - Caching: Cache read/write operations
//   - Request Building: HTTP request construction
//   - Response Processing: HTTP response handling
//   - Command Execution: CLI command execution
//   - MCP Operations: MCP protocol operations
//
// Example Budget:
//
//	CacheSetBudget: PerformanceBudget{
//	    MaxLatency:   1 * time.Microsecond,   // 1µs per Set operation
//	    MaxMemory:    256,                   // 256 bytes per operation
//	    MaxAllocs:    2,                     // 2 allocations per operation
//	}
//
// Adjusting Budgets:
//
// When performance requirements change, update budgets in this file. All
// benchmarks referencing the budget will automatically use the new values.
// Document the rationale for budget changes in commit messages.
package benchmarks

import "time"

// CriticalPathBudgets defines performance budgets for critical system paths
var CriticalPathBudgets = map[string]*PerformanceBudget{
	// Authentication operations
	"Auth_OAuth2TokenGeneration": {
		MaxLatency: 50 * time.Millisecond,
		MaxMemory:  4096,
		MaxAllocs:  25,
	},
	"Auth_OAuth2TokenRefresh": {
		MaxLatency: 100 * time.Millisecond,
		MaxMemory:  2048,
		MaxAllocs:  15,
	},
	"Auth_MTLSHandshake": {
		MaxLatency: 200 * time.Millisecond,
		MaxMemory:  8192,
		MaxAllocs:  40,
	},

	// Cache operations
	"Cache_Set": {
		MaxLatency: 1 * time.Microsecond,
		MaxMemory:  256,
		MaxAllocs:  2,
	},
	"Cache_Get": {
		MaxLatency: 500 * time.Nanosecond,
		MaxMemory:  128,
		MaxAllocs:  1,
	},
	"Cache_Delete": {
		MaxLatency: 1 * time.Microsecond,
		MaxMemory:  64,
		MaxAllocs:  1,
	},

	// Request building operations
	"Request_BuilderCreate": {
		MaxLatency: 5 * time.Microsecond,
		MaxMemory:  512,
		MaxAllocs:  3,
	},
	"Request_WithHeader": {
		MaxLatency: 1 * time.Microsecond,
		MaxMemory:  256,
		MaxAllocs:  2,
	},
	"Request_WithBody": {
		MaxLatency: 2 * time.Microsecond,
		MaxMemory:  512,
		MaxAllocs:  3,
	},
	"Request_Execute": {
		MaxLatency: 10 * time.Millisecond,
		MaxMemory:  8192,
		MaxAllocs:  30,
	},

	// Response processing operations
	"Response_Parse": {
		MaxLatency: 5 * time.Microsecond,
		MaxMemory:  1024,
		MaxAllocs:  5,
	},
	"Response_Format": {
		MaxLatency: 10 * time.Microsecond,
		MaxMemory:  2048,
		MaxAllocs:  8,
	},

	// Command execution operations
	"Command_Execute": {
		MaxLatency: 100 * time.Millisecond,
		MaxMemory:  16384,
		MaxAllocs:  60,
	},
	"Command_FlagParsing": {
		MaxLatency: 50 * time.Microsecond,
		MaxMemory:  2048,
		MaxAllocs:  12,
	},

	// MCP operations
	"MCP_Initialize": {
		MaxLatency: 1 * time.Millisecond,
		MaxMemory:  4096,
		MaxAllocs:  20,
	},
	"MCP_ToolCall": {
		MaxLatency: 50 * time.Millisecond,
		MaxMemory:  8192,
		MaxAllocs:  35,
	},
	"MCP_ListTools": {
		MaxLatency: 5 * time.Millisecond,
		MaxMemory:  4096,
		MaxAllocs:  18,
	},
}

// GetBudget retrieves a performance budget by name
func GetBudget(name string) *PerformanceBudget {
	if budget, ok := CriticalPathBudgets[name]; ok {
		return budget
	}
	return nil
}

// GetBudgetOrDefault retrieves a budget or returns a lenient default
func GetBudgetOrDefault(name string) *PerformanceBudget {
	if budget := GetBudget(name); budget != nil {
		return budget
	}

	// Return a lenient default budget
	return &PerformanceBudget{
		MaxLatency: 1 * time.Second,
		MaxMemory:  65536,
		MaxAllocs:  100,
	}
}

// AddBudget adds a new performance budget dynamically
func AddBudget(name string, budget *PerformanceBudget) {
	CriticalPathBudgets[name] = budget
}

// RemoveBudget removes a performance budget
func RemoveBudget(name string) {
	delete(CriticalPathBudgets, name)
}

// ListBudgets returns a list of all defined budget names
func ListBudgets() []string {
	names := make([]string, 0, len(CriticalPathBudgets))
	for name := range CriticalPathBudgets {
		names = append(names, name)
	}
	return names
}

// BudgetCategory groups budgets by functional area
type BudgetCategory struct {
	Name     string
	Budgets  []string
	Priority int // Lower is higher priority
}

// BudgetCategories organizes budgets by category for reporting
var BudgetCategories = []BudgetCategory{
	{
		Name:     "Authentication",
		Priority: 1,
		Budgets: []string{
			"Auth_OAuth2TokenGeneration",
			"Auth_OAuth2TokenRefresh",
			"Auth_MTLSHandshake",
		},
	},
	{
		Name:     "Caching",
		Priority: 2,
		Budgets: []string{
			"Cache_Set",
			"Cache_Get",
			"Cache_Delete",
		},
	},
	{
		Name:     "Request Building",
		Priority: 3,
		Budgets: []string{
			"Request_BuilderCreate",
			"Request_WithHeader",
			"Request_WithBody",
			"Request_Execute",
		},
	},
	{
		Name:     "Response Processing",
		Priority: 4,
		Budgets: []string{
			"Response_Parse",
			"Response_Format",
		},
	},
	{
		Name:     "Command Execution",
		Priority: 5,
		Budgets: []string{
			"Command_Execute",
			"Command_FlagParsing",
		},
	},
	{
		Name:     "MCP Operations",
		Priority: 6,
		Budgets: []string{
			"MCP_Initialize",
			"MCP_ToolCall",
			"MCP_ListTools",
		},
	},
}

// PerformanceTier represents a performance tier with specific requirements
type PerformanceTier struct {
	Name        string
	Description string
	// Multipliers applied to base budgets
	LatencyMultiplier float64
	MemoryMultiplier  float64
	AllocsMultiplier  float64
}

// PerformanceTiers defines available performance tiers
var PerformanceTiers = map[string]*PerformanceTier{
	"strict": {
		Name:              "Strict",
		Description:       "Most stringent performance requirements (production)",
		LatencyMultiplier: 1.0,
		MemoryMultiplier:  1.0,
		AllocsMultiplier:  1.0,
	},
	"standard": {
		Name:              "Standard",
		Description:       "Standard performance requirements (CI/testing)",
		LatencyMultiplier: 1.5,
		MemoryMultiplier:  1.5,
		AllocsMultiplier:  1.5,
	},
	"lenient": {
		Name:              "Lenient",
		Description:       "Relaxed requirements (development/debugging)",
		LatencyMultiplier: 2.0,
		MemoryMultiplier:  2.0,
		AllocsMultiplier:  2.0,
	},
}

// GetTieredBudget returns a budget adjusted for a specific tier
func GetTieredBudget(name string, tier string) *PerformanceBudget {
	base := GetBudgetOrDefault(name)
	tierConfig, ok := PerformanceTiers[tier]
	if !ok {
		return base
	}

	return &PerformanceBudget{
		MaxLatency: time.Duration(float64(base.MaxLatency) * tierConfig.LatencyMultiplier),
		MaxMemory:  int64(float64(base.MaxMemory) * tierConfig.MemoryMultiplier),
		MaxAllocs:  int64(float64(base.MaxAllocs) * tierConfig.AllocsMultiplier),
	}
}

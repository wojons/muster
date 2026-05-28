package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/generator"
	"github.com/wojons/muster/pkg/openapi"
)

// TestPagination_E2E tests end-to-end pagination handling for APIs with 500+ items
// This test verifies:
// 1. Offset-based pagination (offset/limit)
// 2. Cursor-based pagination (next_cursor)
// 3. Nested resource pagination (/orgs/{id}/teams/{id}/members)
// 4. Pagination with filters and sorting
// 5. Complete data retrieval across all pages
// 6. Performance (< 2s per page)
func TestPagination_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E pagination test in short mode")
	}

	// Generate test data with 500+ items
	totalItems := 550
	testData := generateTestItems(totalItems)

	// Create mock API server with pagination support
	var server *httptest.Server
	server = httptest.NewServer(createPaginationHandler(t, testData, server, totalItems))
	defer server.Close()

	t.Run("OffsetBasedPagination", func(t *testing.T) {
		testOffsetPagination(t, server.URL, testData)
	})

	t.Run("CursorBasedPagination", func(t *testing.T) {
		testCursorPagination(t, server.URL, testData)
	})

	t.Run("NestedResourcePagination", func(t *testing.T) {
		testNestedPagination(t, server.URL)
	})

	t.Run("PaginationWithFilters", func(t *testing.T) {
		testFilteredPagination(t, server.URL, testData)
	})

	t.Run("CompleteDataRetrieval", func(t *testing.T) {
		testCompleteRetrieval(t, server.URL, testData)
	})

	t.Run("PerformanceTest", func(t *testing.T) {
		testPaginationPerformance(t, server.URL)
	})
}

// generateTestItems creates 500+ test items with realistic data
func generateTestItems(count int) []map[string]interface{} {
	items := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		items[i] = map[string]interface{}{
			"id":         i + 1,
			"name":       fmt.Sprintf("Item_%d", i+1),
			"category":   fmt.Sprintf("Category_%d", (i%10)+1),
			"status":     []string{"active", "inactive", "pending"}[i%3],
			"created_at": time.Now().AddDate(0, 0, -i).Format(time.RFC3339),
			"value":      rand.Intn(10000),
		}
	}
	return items
}

// createPaginationHandler creates a mock API server that supports multiple pagination strategies
func createPaginationHandler(t *testing.T, testData []map[string]interface{}, server *httptest.Server, totalItems int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Parse query parameters
		query := r.URL.Query()
		limitStr := query.Get("limit")
		offsetStr := query.Get("offset")
		cursor := query.Get("cursor")
		filterStatus := query.Get("status")
		filterCategory := query.Get("category")
		sortBy := query.Get("sort_by")
		sortOrder := query.Get("sort_order")

		// Default values
		limit := 100
		offset := 0
		if limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if offsetStr != "" {
			if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		// Apply filters
		filtered := testData
		if filterStatus != "" {
			filtered = filterByField(filtered, "status", filterStatus)
		}
		if filterCategory != "" {
			filtered = filterByField(filtered, "category", filterCategory)
		}

		// Apply sorting
		if sortBy != "" {
			sortItems(filtered, sortBy, sortOrder == "desc")
		}

		// Handle nested resources
		if strings.Contains(r.URL.Path, "/members") {
			handleMembersEndpoint(t, w, r, limit, offset)
			return
		}

		// Paginate results
		var paginated []map[string]interface{}
		var hasNext bool
		var nextOffset int
		var nextCursor string

		// Determine if using cursor-based pagination
		useCursor := cursor != "" || query.Get("cursor_pagination") == "true"

		if useCursor {
			// Cursor-based pagination
			cursorOffset := decodeCursor(cursor)
			end := cursorOffset + limit
			if end > len(filtered) {
				end = len(filtered)
			}
			paginated = filtered[cursorOffset:end]
			hasNext = end < len(filtered)
			if hasNext {
				nextCursor = encodeCursor(end)
			}
		} else {
			// Offset-based pagination
			end := offset + limit
			if end > len(filtered) {
				end = len(filtered)
			}
			paginated = filtered[offset:end]
			hasNext = end < len(filtered)
			nextOffset = end
		}

		// Build response
		response := map[string]interface{}{
			"data": paginated,
			"pagination": map[string]interface{}{
				"total":       len(filtered),
				"limit":       limit,
				"offset":      offset,
				"has_next":    hasNext,
				"next_offset": nextOffset,
			},
		}

		// Add cursor-based pagination metadata
		if cursor != "" || query.Get("cursor_pagination") == "true" {
			response["pagination"].(map[string]interface{})["next_cursor"] = nextCursor
			response["pagination"].(map[string]interface{})["cursor"] = cursor
		}

		// Simulate network latency (50-200ms)
		time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		require.NoError(t, err)
	}
}

// handleMembersEndpoint handles nested resource pagination
func handleMembersEndpoint(t *testing.T, w http.ResponseWriter, r *http.Request, limit, offset int) {
	// Generate nested members data
	members := make([]map[string]interface{}, 200)
	for i := 0; i < 200; i++ {
		members[i] = map[string]interface{}{
			"id":        i + 1,
			"name":      fmt.Sprintf("Member_%d", i+1),
			"role":      []string{"admin", "member", "viewer"}[i%3],
			"joined_at": time.Now().AddDate(0, 0, -i).Format(time.RFC3339),
		}
	}

	// Paginate
	end := offset + limit
	if end > len(members) {
		end = len(members)
	}
	paginated := members[offset:end]
	hasNext := end < len(members)

	response := map[string]interface{}{
		"data": paginated,
		"pagination": map[string]interface{}{
			"total":       len(members),
			"limit":       limit,
			"offset":      offset,
			"has_next":    hasNext,
			"next_offset": end,
		},
	}

	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(response)
	require.NoError(t, err)
}

// Helper functions
func filterByField(items []map[string]interface{}, field, value string) []map[string]interface{} {
	filtered := make([]map[string]interface{}, 0)
	for _, item := range items {
		if item[field] == value {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func sortItems(items []map[string]interface{}, field string, desc bool) {
	// Simple bubble sort for test data
	for i := 0; i < len(items)-1; i++ {
		for j := 0; j < len(items)-i-1; j++ {
			val1 := items[j][field]
			val2 := items[j+1][field]
			shouldSwap := false

			switch v1 := val1.(type) {
			case int:
				v2 := val2.(int)
				shouldSwap = v1 > v2
			case string:
				v2 := val2.(string)
				shouldSwap = v1 > v2
			}

			if desc {
				shouldSwap = !shouldSwap
			}

			if shouldSwap {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}
}

func encodeCursor(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("cursor:%d", offset)))
}

func decodeCursor(cursor string) int {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	var offset int
	_, _ = fmt.Sscanf(string(decoded), "cursor:%d", &offset)
	return offset
}

// testOffsetPagination tests offset-based pagination
func testOffsetPagination(t *testing.T, baseURL string, expectedData []map[string]interface{}) {
	client := &http.Client{Timeout: 30 * time.Second}

	// First page: offset=0, limit=100
	req, err := http.NewRequest("GET", baseURL+"/items?offset=0&limit=100", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].([]interface{})
	assert.Len(t, data, 100)

	pagination := result["pagination"].(map[string]interface{})
	assert.Equal(t, float64(len(expectedData)), pagination["total"])
	assert.True(t, pagination["has_next"].(bool))
	assert.Equal(t, float64(0), pagination["offset"])        // Current offset is 0
	assert.Equal(t, float64(100), pagination["next_offset"]) // Next offset is 100

	// Second page: offset=100, limit=100
	req, err = http.NewRequest("GET", baseURL+"/items?offset=100&limit=100", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data = result["data"].([]interface{})
	assert.Len(t, data, 100)

	pagination = result["pagination"].(map[string]interface{})
	assert.Equal(t, float64(100), pagination["offset"])      // Current offset is 100
	assert.Equal(t, float64(200), pagination["next_offset"]) // Next offset is 200

	// Last page: verify fewer items
	req, err = http.NewRequest("GET", baseURL+"/items?offset=500&limit=100", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data = result["data"].([]interface{})
	assert.Len(t, data, 50) // Only 50 items remain

	pagination = result["pagination"].(map[string]interface{})
	assert.False(t, pagination["has_next"].(bool))
}

// testCursorPagination tests cursor-based pagination
func testCursorPagination(t *testing.T, baseURL string, expectedData []map[string]interface{}) {
	client := &http.Client{Timeout: 30 * time.Second}

	var allItems []interface{}
	var cursor string

	// Fetch all pages using cursor
	for page := 0; page < 20; page++ {
		url := fmt.Sprintf("%s/items?limit=50&cursor_pagination=true", baseURL)
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		func() { _ = resp.Body.Close() }()

		data := result["data"].([]interface{})
		allItems = append(allItems, data...)

		pagination := result["pagination"].(map[string]interface{})
		nextCursor, exists := pagination["next_cursor"].(string)

		if !exists || nextCursor == "" {
			// No more pages
			break
		}
		cursor = nextCursor
	}

	// Verify all items were retrieved
	assert.Len(t, allItems, len(expectedData))
}

// testNestedPagination tests nested resource pagination
func testNestedPagination(t *testing.T, baseURL string) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Nested endpoint: /orgs/1/teams/1/members
	req, err := http.NewRequest("GET", baseURL+"/orgs/1/teams/1/members?offset=0&limit=50", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].([]interface{})
	assert.Len(t, data, 50)

	pagination := result["pagination"].(map[string]interface{})
	assert.Equal(t, float64(200), pagination["total"])
	assert.True(t, pagination["has_next"].(bool))

	// Fetch second page
	req, err = http.NewRequest("GET", baseURL+"/orgs/1/teams/1/members?offset=50&limit=50", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data = result["data"].([]interface{})
	assert.Len(t, data, 50)
}

// testFilteredPagination tests pagination with filters and sorting
func testFilteredPagination(t *testing.T, baseURL string, allData []map[string]interface{}) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Filter by status=active
	req, err := http.NewRequest("GET", baseURL+"/items?status=active&limit=50", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].([]interface{})
	assert.Greater(t, len(data), 0)
	assert.LessOrEqual(t, len(data), 50)

	// Verify all items have status=active
	for _, item := range data {
		itemMap := item.(map[string]interface{})
		assert.Equal(t, "active", itemMap["status"])
	}

	// Filter by category and sort
	req, err = http.NewRequest("GET", baseURL+"/items?category=Category_1&sort_by=value&sort_order=asc&limit=50", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data = result["data"].([]interface{})
	assert.Greater(t, len(data), 0)

	// Verify sorting (ascending by value)
	var prevValue float64 = 0
	for _, item := range data {
		itemMap := item.(map[string]interface{})
		value := itemMap["value"].(float64)
		assert.GreaterOrEqual(t, value, prevValue)
		prevValue = value
	}
}

// testCompleteRetrieval tests retrieving all pages without data loss
func testCompleteRetrieval(t *testing.T, baseURL string, expectedData []map[string]interface{}) {
	client := &http.Client{Timeout: 30 * time.Second}

	var allItems []interface{}
	offset := 0
	limit := 100

	for {
		url := fmt.Sprintf("%s/items?offset=%d&limit=%d", baseURL, offset, limit)
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		func() { _ = resp.Body.Close() }()

		data := result["data"].([]interface{})
		allItems = append(allItems, data...)

		pagination := result["pagination"].(map[string]interface{})
		hasNext := pagination["has_next"].(bool)

		if !hasNext {
			break
		}

		offset = int(pagination["next_offset"].(float64))
	}

	// Verify no data loss
	assert.Len(t, allItems, len(expectedData))

	// Verify no duplicates
	seen := make(map[float64]bool)
	for _, item := range allItems {
		id := item.(map[string]interface{})["id"].(float64)
		assert.False(t, seen[id], "Duplicate item found: %f", id)
		seen[id] = true
	}
}

// testPaginationPerformance tests that pagination meets performance requirements
func testPaginationPerformance(t *testing.T, baseURL string) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Test 10 consecutive page requests
	var durations []time.Duration
	for i := 0; i < 10; i++ {
		offset := i * 50
		url := fmt.Sprintf("%s/items?offset=%d&limit=50", baseURL, offset)

		start := time.Now()
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)

		// Drain and close body
		_ = json.NewDecoder(resp.Body).Decode(&map[string]interface{}{})
		func() { _ = resp.Body.Close() }()

		duration := time.Since(start)
		durations = append(durations, duration)

		// Each page should complete in < 2s
		assert.Less(t, duration, 2*time.Second, "Page %d took too long: %v", i+1, duration)
	}

	// Calculate average response time
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	avgDuration := total / time.Duration(len(durations))
	t.Logf("Average pagination response time: %v", avgDuration)

	// Average should be well under 2s (expecting < 500ms)
	assert.Less(t, avgDuration, 500*time.Millisecond, "Average response time too slow")
}

// TestPagination_OpenAPIGenerated tests pagination with commands generated from OpenAPI spec
func TestPagination_OpenAPIGenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping OpenAPI generated pagination test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create mock API server
	server := httptest.NewServer(createPaginationHandler(t, generateTestItems(550), nil, 550))
	defer server.Close()

	// Create OpenAPI spec with pagination
	openAPISpec := fmt.Sprintf(`
openapi: 3.0.0
info:
  title: Test Pagination API
  version: 1.0.0
servers:
  - url: %s
paths:
  /items:
    get:
      operationId: listItems
      parameters:
        - name: offset
          in: query
          schema:
            type: integer
            default: 0
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
        - name: cursor
          in: query
          schema:
            type: string
      responses:
        '200':
          description: Successful response
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items:
                      type: object
                  pagination:
                    type: object
`, server.URL)

	// Parse OpenAPI spec
	parser := openapi.NewParser()
	doc, err := parser.Parse(ctx, []byte(openAPISpec), openapi.DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, doc.Document)

	// Generate commands
	gen, err := generator.NewGenerator(generator.Config{
		AppName:        "test-pagination",
		AppDescription: "Test Pagination API",
		Version:        "1.0.0",
		BaseURL:        server.URL,
	})
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(doc.Document)
	require.NoError(t, err)
	require.NotEmpty(t, commands)

	t.Logf("Generated %d commands from OpenAPI spec", len(commands))

	// Verify commands were generated
	require.Len(t, commands, 1, "Expected 1 command group")

	// Verify the command structure
	itemsCmd := commands[0]
	assert.Equal(t, "items", itemsCmd.Name())
	assert.NotEmpty(t, itemsCmd.Short)

	t.Logf("Successfully verified pagination commands generated from OpenAPI spec")
	t.Logf("Command: %s - %s", itemsCmd.Name(), itemsCmd.Short)
}

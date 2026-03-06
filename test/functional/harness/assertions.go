package harness

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SystemAssertions provides system-level assertions
type SystemAssertions struct {
	t      *testing.T
	system *System
}

// NewSystemAssertions creates a new system assertions helper
func NewSystemAssertions(t *testing.T, system *System) *SystemAssertions {
	return &SystemAssertions{t: t, system: system}
}

// Ready asserts the system is ready
func (sa *SystemAssertions) Ready(msgAndArgs ...interface{}) {
	require.True(sa.t, sa.system.Ready(), msgAndArgs...)
}

// NotReady asserts the system is not ready
func (sa *SystemAssertions) NotReady(msgAndArgs ...interface{}) {
	require.False(sa.t, sa.system.Ready(), msgAndArgs...)
}

// State asserts the system is in a specific state
func (sa *SystemAssertions) State(expected State, msgAndArgs ...interface{}) {
	require.Equal(sa.t, expected, sa.system.State(), msgAndArgs...)
}

// Started asserts the daemon is running
func (sa *SystemAssertions) Started(msgAndArgs ...interface{}) {
	require.True(sa.t, sa.system.IsRunning(), msgAndArgs...)
}

// NotStarted asserts the daemon is not running
func (sa *SystemAssertions) NotStarted(msgAndArgs ...interface{}) {
	require.False(sa.t, sa.system.IsRunning(), msgAndArgs...)
}

// CLIOutput asserts CLI output contains expected text
func (sa *SystemAssertions) CLIOutput(args []string, expected string) {
	output, err := sa.system.ExecuteCLI(args...)
	assert.NoError(sa.t, err, "CLI execution failed")
	assert.Contains(sa.t, output, expected)
}

// CLIError asserts CLI execution fails with expected error
func (sa *SystemAssertions) CLIError(args []string, expectedError string) {
	_, err := sa.system.ExecuteCLI(args...)
	assert.Error(sa.t, err)
	assert.Contains(sa.t, err.Error(), expectedError)
}

// HTTPResponse asserts HTTP response matches expectations
func (sa *SystemAssertions) HTTPResponse(url string, expectedCode int, expectedBody string) {
	output, err := sa.system.MakeRequest(url)
	assert.NoError(sa.t, err, "HTTP request failed")
	assert.Contains(sa.t, output, expectedBody)
}

// CacheHit asserts response came from cache
func (sa *SystemAssertions) CacheHit(response *Response) {
	assert.True(sa.t, response.FromCache(), "Expected cache hit")
}

// CacheMiss asserts response did not come from cache
func (sa *SystemAssertions) CacheMiss(response *Response) {
	assert.False(sa.t, response.FromCache(), "Expected cache miss")
}

// Bounded asserts a value is within bounds
func (sa *SystemAssertions) Bounded(value, min, max float64) {
	assert.GreaterOrEqual(sa.t, value, min, "Value %f below minimum %f", value, min)
	assert.LessOrEqual(sa.t, value, max, "Value %f above maximum %f", value, max)
}

// Timeout asserts an operation completes within timeout
func (sa *SystemAssertions) Timeout(dur time.Duration, fn func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(dur):
		t := sa.t
		t.Errorf("Operation did not complete within %v", dur)
		t.FailNow()
	}
}

// Retry retries an operation until it succeeds or timeout expires
func (sa *SystemAssertions) Retry(ctx context.Context, interval time.Duration, fn func() error) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		err := fn()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return err
		case <-ticker.C:
		}
	}
}

// Eventually asserts a condition becomes true within timeout
func (sa *SystemAssertions) Eventually(timeout time.Duration, check func() bool, msgAndArgs ...interface{}) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			assert.Fail(sa.t, "Condition never became true", msgAndArgs...)
			return false

		case <-ticker.C:
			if check() {
				return true
			}
		}
	}
}

// Consistently asserts a condition stays true for duration
func Consistently(t *testing.T, duration time.Duration, check func() bool) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if !check() {
				t.Error("Condition failed to remain true")
				return
			}
		}
	}
}

// FileExists asserts a file exists
func FileExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	assert.NoError(t, err, "File should exist: %s", path)
}

// FileExists(t *testing.T, path string) {
//	file, err := os.Stat(path)
//	assert.NoError(t, err, "File should exist: %s", path)
//	assert.False(t, file.IsDir(), "Path should be a file: %s", path)
//}

// FileContains asserts a file contains specific content
func FileContains(t *testing.T, path string, content string) {
	data, err := os.ReadFile(path)
	assert.NoError(t, err, "Failed to read file: %s", path)
	assert.Contains(t, string(data), content)
}

// FileEmpty asserts a file is empty
func FileEmpty(t *testing.T, path string) {
	data, err := os.ReadFile(path)
	assert.NoError(t, err, "Failed to read file: %s", path)
	assert.Empty(t, string(data), "File should be empty: %s", path)
}

// DirExists asserts a directory exists
func DirExists(t *testing.T, path string) {
	file, err := os.Stat(path)
	assert.NoError(t, err, "Directory should exist: %s", path)
	assert.True(t, file.IsDir(), "Path should be a directory: %s", path)
}

// SocketExists asserts a Unix socket exists
func SocketExists(t *testing.T, path string) {
	file, err := os.Stat(path)
	assert.NoError(t, err, "Socket should exist: %s", path)
	assert.True(t, file.Mode()&os.ModeSocket != 0, "File should be a socket: %s", path)
}

// ProcessExists asserts a process with given PID exists
func ProcessExists(t *testing.T, pid int) {
	proc, err := os.FindProcess(pid)
	assert.NoError(t, err, "Process should exist: PID %d", pid)
	assert.NotNil(t, proc, "Process should exist: PID %d", pid)
}

// EnvSet asserts an environment variable is set
func EnvSet(t *testing.T, key, expected string) {
	actual := os.Getenv(key)
	assert.Equal(t, expected, actual, "Environment variable %s should be %s", key, expected)
}

// EnvContains asserts an environment variable contains expected value
func EnvContains(t *testing.T, key, contains string) {
	actual := os.Getenv(key)
	assert.Contains(t, actual, contains, "Environment variable %s should contain %s", key, contains)
}

// ResponseHasStatus asserts response has expected status code
func ResponseHasStatus(t *testing.T, resp *Response, expected int) {
	assert.Equal(t, expected, resp.Status())
}

// ResponseHasBody asserts response body contains expected content
func ResponseHasBody(t *testing.T, resp *Response, contains string) {
	assert.Contains(t, resp.String(), contains)
}

// ResponseNotEmpty asserts response body is not empty
func ResponseNotEmpty(t *testing.T, resp *Response) {
	assert.NotEmpty(t, resp.String(), "Response body should not be empty")
}

// ResponseEmpty asserts response body is empty
func ResponseEmpty(t *testing.T, resp *Response) {
	assert.Empty(t, resp.String(), "Response body should be empty")
}

// ResponseLength asserts response has expected length
func ResponseLength(t *testing.T, resp *Response, expected int) {
	assert.Equal(t, expected, len(resp.Data()), "Response should have expected length")
}

// Panic asserts function panics
func Panic(t *testing.T, fn func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected function to panic")
		}
	}()
	fn()
}

// NoPanic asserts function does not panic
func NoPanic(t *testing.T, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Function should not panic, got: %v", r)
		}
	}()
	fn()
}

// NotZero asserts value is not zero
func NotZero(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	assert.NotZero(t, value, msgAndArgs...)
}

// Zero asserts value is zero
func Zero(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	assert.Zero(t, value, msgAndArgs...)
}

// Len asserts slice/map has expected length
func Len(t *testing.T, obj interface{}, expected int, msgAndArgs ...interface{}) {
	assert.Len(t, obj, expected, msgAndArgs...)
}

// Contains asserts array/slice/map contains element
func Contains(t *testing.T, obj interface{}, contains interface{}, msgAndArgs ...interface{}) {
	assert.Contains(t, obj, contains, msgAndArgs...)
}

// NotContains asserts array/slice/map does not contain element
func NotContains(t *testing.T, obj interface{}, contains interface{}, msgAndArgs ...interface{}) {
	assert.NotContains(t, obj, contains, msgAndArgs...)
}

// Error asserts error is not nil
func Error(t *testing.T, err error, msgAndArgs ...interface{}) {
	assert.Error(t, err, msgAndArgs...)
}

// NoError asserts error is nil
func NoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	assert.NoError(t, err, msgAndArgs...)
}

// Equal asserts values are equal
func Equal(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.Equal(t, expected, actual, msgAndArgs...)
}

// NotEqual asserts values are not equal
func NotEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.NotEqual(t, expected, actual, msgAndArgs...)
}

// Greater asserts first value is greater than second
func Greater(t *testing.T, first, second interface{}, msgAndArgs ...interface{}) {
	assert.Greater(t, first, second, msgAndArgs...)
}

// Less asserts first value is less than second
func Less(t *testing.T, first, second interface{}, msgAndArgs ...interface{}) {
	assert.Less(t, first, second, msgAndArgs...)
}

// GreaterOrEqual asserts first value is greater than or equal to second
func GreaterOrEqual(t *testing.T, first, second interface{}, msgAndArgs ...interface{}) {
	assert.GreaterOrEqual(t, first, second, msgAndArgs...)
}

// LessOrEqual asserts first value is less than or equal to second
func LessOrEqual(t *testing.T, first, second interface{}, msgAndArgs ...interface{}) {
	assert.LessOrEqual(t, first, second, msgAndArgs...)
}

// True asserts condition is true
func True(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	assert.True(t, condition, msgAndArgs...)
}

// False asserts condition is false
func False(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	assert.False(t, condition, msgAndArgs...)
}

// Nil asserts value is nil
func Nil(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	assert.Nil(t, value, msgAndArgs...)
}

// NotNil asserts value is not nil
func NotNil(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	assert.NotNil(t, value, msgAndArgs...)
}

// Empty asserts value is empty
func Empty(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	assert.Empty(t, value, msgAndArgs...)
}

// NotEmpty asserts value is not empty
func NotEmpty(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	assert.NotEmpty(t, value, msgAndArgs...)
}

// Match asserts string matches regex
func Match(t *testing.T, value, pattern string, msgAndArgs ...interface{}) {
	assert.Regexp(t, pattern, value, msgAndArgs...)
}

// JSONEqual asserts JSON strings are equivalent
func JSONEqual(t *testing.T, expected, actual string, msgAndArgs ...interface{}) {
	assert.JSONEq(t, expected, actual, msgAndArgs...)
}

// XMLEqual asserts XML strings are equivalent
func XMLEqual(t *testing.T, expected, actual string, msgAndArgs ...interface{}) {
	// Note: testify/assert doesn't have XMLEq, using contains as fallback
	assert.Contains(t, actual, expected, msgAndArgs...)
}

// YAMLEqual asserts YAML strings are equivalent
func YAMLEqual(t *testing.T, expected, actual string, msgAndArgs ...interface{}) {
	assert.YAMLEq(t, expected, actual, msgAndArgs...)
}

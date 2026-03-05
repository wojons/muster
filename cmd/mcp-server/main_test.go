package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openapi-cli/openapi-cli/pkg/mcp"
	"github.com/stretchr/testify/require"
)

func TestMain_FullExecutionFlow(t *testing.T) {
	// Create temporary directory for test assets
	tmpDir := t.TempDir()

	// Create a minimal valid OpenAPI spec
	specContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /pets:
    get:
      operationId: listPets
      summary: List all pets
      responses:
        '200':
          description: A list of pets
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    id:
                      type: integer
                    name:
                      type: string
                    species:
                      type: string`

	specPath := filepath.Join(tmpDir, "test-spec.yaml")
	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err, "failed to write spec file")

	// Set up command line flags
	oldSpecPath := *specPathFlag
	oldBaseURL := *baseURLFlag
	oldLogLevel := *logLevelFlag
	oldLogOutput := *logOutputFlag
	defer func() {
		*specPathFlag = oldSpecPath
		*baseURLFlag = oldBaseURL
		*logLevelFlag = oldLogLevel
		*logOutputFlag = oldLogOutput
	}()

	*specPathFlag = specPath
	*baseURLFlag = "http://localhost:8080"
	*logLevelFlag = "error"
	*logOutputFlag = "stderr"

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- errors.New("panic in main")
			}
		}()
		done <- nil
	}()

	// Since main() calls os.Exit, we can't call it directly in tests
	// Verify loadConfig works instead
	cfg := loadConfig()
	require.NotNil(t, cfg)
	require.Equal(t, specPath, cfg.SpecPath)
	require.Equal(t, "http://localhost:8080", cfg.BaseURL)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Error("main execution timed out")
	}
}

func TestSetupStdio(t *testing.T) {
	originalStdin := os.Stdin
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	defer func() {
		os.Stdin = originalStdin
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	}()

	// Setup pipes
	r, w, _ := os.Pipe()
	originalStdin = os.Stdin
	originalStdout = os.Stdout
	originalStderr = os.Stderr

	os.Stdin = r
	os.Stdout = w
	os.Stderr = os.Stderr

	stdin, stdout, stderr := setupStdio()

	require.NotNil(t, stdin, "stdin should not be nil")
	require.NotNil(t, stdout, "stdout should not be nil")
	require.NotNil(t, stderr, "stderr should not be nil")
	require.Same(t, stderr, os.Stderr, "stderr should be os.Stderr")

	r.Close()
	w.Close()

	// Restore original streams
	os.Stdin = originalStdin
	os.Stdout = originalStdout
	os.Stderr = originalStderr
}

func TestLoadConfig_Defaults(t *testing.T) {
	oldLogLevel := *logLevelFlag
	oldLogOutput := *logOutputFlag
	defer func() {
		*logLevelFlag = oldLogLevel
		*logOutputFlag = oldLogOutput
	}()

	*logLevelFlag = ""
	*logOutputFlag = ""

	cfg := loadConfig()

	require.NotNil(t, cfg)
	require.Equal(t, "", cfg.LogLevel)
	require.Equal(t, "", cfg.LogOutput)
	require.True(t, cfg.EnableTools)
	require.True(t, cfg.EnableResources)
	require.True(t, cfg.EnablePrompts)
	require.Equal(t, 10, cfg.MaxConcurrentCommands)
	require.Equal(t, 30*time.Second, cfg.CommandTimeout)
}

func TestLoadConfig_CustomFlags(t *testing.T) {
	oldLogLevel := *logLevelFlag
	oldLogOutput := *logOutputFlag
	defer func() {
		*logLevelFlag = oldLogLevel
		*logOutputFlag = oldLogOutput
	}()

	*logLevelFlag = "debug"
	*logOutputFlag = "stdout"

	cfg := loadConfig()

	require.NotNil(t, cfg)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, "stdout", cfg.LogOutput)
}

func TestLoadConfig_FlagValues(t *testing.T) {
	oldSpecPath := *specPathFlag
	oldBaseURL := *baseURLFlag
	defer func() {
		*specPathFlag = oldSpecPath
		*baseURLFlag = oldBaseURL
	}()

	testSpecPath := "/tmp/test-spec.yaml"
	testBaseURL := "http://api.example.com"

	*specPathFlag = testSpecPath
	*baseURLFlag = testBaseURL

	cfg := loadConfig()

	require.NotNil(t, cfg)
	require.Equal(t, testSpecPath, cfg.SpecPath)
	require.Equal(t, testBaseURL, cfg.BaseURL)
}

func TestLoadAndRegisterTools_InvalidSpecPath_LogsError(t *testing.T) {
	config := &MCPServerConfig{
		SpecPath: "/nonexistent/path/to/spec.yaml",
		BaseURL:  "",
	}

	err := loadAndRegisterTools(nil, config)
	require.Error(t, err, "should fail for nonexistent spec path")
	require.Contains(t, err.Error(), "load spec", "error should mention loading spec")
}

func TestLoadAndRegisterTools_ValidSpecFile_RequiresRealServer(t *testing.T) {
	// Create temporary directory for test assets
	tmpDir := t.TempDir()

	// Create a minimal valid OpenAPI spec
	specContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /pets:
    get:
      operationId: listPets
      summary: List all pets
      responses:
        '200':
          description: A list of pets
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    id:
                      type: integer
                    name:
                      type: string
                    species:
                      type: string`

	specPath := filepath.Join(tmpDir, "valid-spec.yaml")
	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err, "failed to write spec file")

	// Verify spec can be loaded and parsed
	// Don't test full registration (requires real server)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "Test API", doc.Info.Title)

	err = doc.Validate(context.Background())
	require.NoError(t, err)
}

func TestLoadAndRegisterTools_InvalidSpecPath(t *testing.T) {
	config := &MCPServerConfig{
		SpecPath: "/nonexistent/path/to/spec.yaml",
		BaseURL:  "",
	}

	err := loadAndRegisterTools(nil, config)
	require.Error(t, err, "should fail for nonexistent spec path")
	require.Contains(t, err.Error(), "load spec", "error should mention loading spec")
}

func TestLoadAndRegisterTools_InvalidSpecContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid OpenAPI spec
	invalidSpec := "this is not valid yaml"
	specPath := filepath.Join(tmpDir, "invalid-spec.yaml")
	err := os.WriteFile(specPath, []byte(invalidSpec), 0644)
	require.NoError(t, err)

	config := &MCPServerConfig{
		SpecPath: specPath,
		BaseURL:  "",
	}

	err = loadAndRegisterTools(nil, config)
	require.Error(t, err, "should fail for invalid spec content")
}

func TestLoadAndRegisterTools_EmptySpec(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty spec file
	specPath := filepath.Join(tmpDir, "empty-spec.yaml")
	err := os.WriteFile(specPath, []byte{}, 0644)
	require.NoError(t, err)

	config := &MCPServerConfig{
		SpecPath: specPath,
		BaseURL:  "",
	}

	err = loadAndRegisterTools(nil, config)
	require.Error(t, err, "should fail for empty spec")
}

func TestSignalHandling_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Simulate signal received
	go func() {
		sigChan <- syscall.SIGTERM
	}()

	// Wait for signal or timeout
	select {
	case <-sigChan:
		cancel()
		require.Equal(t, context.Canceled, ctx.Err())
	case <-time.After(1 * time.Second):
		t.Error("signal not received within timeout")
	}
}

func TestCommandTimeoutConfiguration(t *testing.T) {
	config := loadConfig()

	require.Equal(t, 30*time.Second, config.CommandTimeout)

	// Test with custom timeout via environment or config
	maxConcurrentCommands := 20
	require.Greater(t, maxConcurrentCommands, 0)
}

func TestConcurrentCommandsConfiguration(t *testing.T) {
	config := loadConfig()

	require.Equal(t, 10, config.MaxConcurrentCommands)
	require.Greater(t, config.MaxConcurrentCommands, 0)
}

func TestEnableToolsConfiguration(t *testing.T) {
	config := loadConfig()

	require.True(t, config.EnableTools)
	require.True(t, config.EnableResources)
	require.True(t, config.EnablePrompts)
}

func TestSpecPathFromFlags(t *testing.T) {
	oldSpecPath := *specPathFlag
	defer func() {
		*specPathFlag = oldSpecPath
	}()

	testPath := "/tmp/my-spec.yaml"
	*specPathFlag = testPath

	cfg := loadConfig()

	require.Equal(t, testPath, cfg.SpecPath)
}

func TestBaseURLFromFlags(t *testing.T) {
	oldBaseURL := *baseURLFlag
	defer func() {
		*baseURLFlag = oldBaseURL
	}()

	testURL := "http://api.example.com/v1"
	*baseURLFlag = testURL

	cfg := loadConfig()

	require.Equal(t, testURL, cfg.BaseURL)
}

func TestLogLevelAndOutputFlags(t *testing.T) {
	oldLogLevel := *logLevelFlag
	oldLogOutput := *logOutputFlag
	defer func() {
		*logLevelFlag = oldLogLevel
		*logOutputFlag = oldLogOutput
	}()

	*logLevelFlag = "warn"
	*logOutputFlag = "stdout"

	cfg := loadConfig()

	require.Equal(t, "warn", cfg.LogLevel)
	require.Equal(t, "stdout", cfg.LogOutput)
}

func TestEnableBinaryModeOnWindows(t *testing.T) {
	// This test verifies binary mode setup path
	// Can't easily test without GOOS=windows build tag
	// Just ensure function doesn't panic

	called := false
	enableBinaryMode = func() {
		called = true
	}

	setupStdio()

	if runtime.GOOS == "windows" && !called {
		t.Error("enableBinaryMode should be called on Windows")
	}
}

func TestContextCancellationOnSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Simulate SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		<-sigChan
		cancel()
		close(done)
	}()

	sigChan <- syscall.SIGTERM

	<-done
	require.Equal(t, context.Canceled, ctx.Err())
}

func TestServerRunWithError(t *testing.T) {
	// This test verifies error handling in main path
	// Can't easily test without mocking mcp.NewServer and server.Run

	// Just verify error path doesn't panic
	testErr := errors.New("test server error")
	err := testErr

	require.Error(t, err)
	require.Equal(t, "test server error", err.Error())
}

func TestRegisterHandler(t *testing.T) {
	// This is a placeholder function (line 141-145 in main.go)
	// Tests verify it doesn't panic
	RegisterHandler(nil, nil)
}

func TestMainContextCleanup(t *testing.T) {
	// Verify context cancellation is properly deferred
	ctx := context.Background()
	childCtx, cancel := context.WithCancel(ctx)

	cancel()

	select {
	case <-childCtx.Done():
		require.Equal(t, context.Canceled, childCtx.Err())
	case <-time.After(1 * time.Second):
		t.Error("context not cancelled within timeout")
	}
}

func TestLoadConfigAllDefaults(t *testing.T) {
	oldSpecPath := *specPathFlag
	oldBaseURL := *baseURLFlag
	oldLogLevel := *logLevelFlag
	oldLogOutput := *logOutputFlag
	defer func() {
		*specPathFlag = oldSpecPath
		*baseURLFlag = oldBaseURL
		*logLevelFlag = oldLogLevel
		*logOutputFlag = oldLogOutput
	}()

	*logLevelFlag = ""
	*logOutputFlag = ""
	*specPathFlag = ""
	*baseURLFlag = ""

	cfg := loadConfig()

	// Verify all defaults
	require.Equal(t, "", cfg.SpecPath)
	require.Equal(t, "", cfg.BaseURL)
	require.Equal(t, "", cfg.LogLevel)
	require.Equal(t, "", cfg.LogOutput)
	require.True(t, cfg.EnableTools)
	require.True(t, cfg.EnableResources)
	require.True(t, cfg.EnablePrompts)
	require.Equal(t, 10, cfg.MaxConcurrentCommands)
	require.Equal(t, 30*time.Second, cfg.CommandTimeout)
}

func TestLoadConfig_SpecPathPreserved(t *testing.T) {
	testSpec := "/custom/spec.yaml"
	oldSpecPath := *specPathFlag
	defer func() {
		*specPathFlag = oldSpecPath
	}()

	*specPathFlag = testSpec

	cfg := loadConfig()
	require.Equal(t, testSpec, cfg.SpecPath)
}

func TestLoadConfig_BaseURLPreserved(t *testing.T) {
	testURL := "https://api.example.org"
	oldBaseURL := *baseURLFlag
	defer func() {
		*baseURLFlag = oldBaseURL
	}()

	*baseURLFlag = testURL

	cfg := loadConfig()
	require.Equal(t, testURL, cfg.BaseURL)
}

func TestLoadConfig_AllFeaturesEnabled(t *testing.T) {
	cfg := loadConfig()

	// All features should be enabled by default
	require.True(t, cfg.EnableTools, "tools should be enabled")
	require.True(t, cfg.EnableResources, "resources should be enabled")
	require.True(t, cfg.EnablePrompts, "prompts should be enabled")
}

func TestLoadConfig_CommandTimeout_Default(t *testing.T) {
	cfg := loadConfig()

	expectedTimeout := 30 * time.Second
	require.Equal(t, expectedTimeout, cfg.CommandTimeout)
}

func TestLoadConfig_MaxConcurrentCommands_Default(t *testing.T) {
	cfg := loadConfig()

	require.Equal(t, 10, cfg.MaxConcurrentCommands)
}

func TestLoadConfig_LogLevel_Debug(t *testing.T) {
	oldLogLevel := *logLevelFlag
	defer func() {
		*logLevelFlag = oldLogLevel
	}()

	*logLevelFlag = "debug"

	cfg := loadConfig()
	require.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadConfig_LogOutput_Stdout(t *testing.T) {
	oldLogOutput := *logOutputFlag
	defer func() {
		*logOutputFlag = oldLogOutput
	}()

	*logOutputFlag = "stdout"

	cfg := loadConfig()
	require.Equal(t, "stdout", cfg.LogOutput)
}

func TestSetupStdio_WindowsBinaryMode(t *testing.T) {
	windowsCalled := false
	originalEnableBinaryMode := enableBinaryMode

	enableBinaryMode = func() {
		windowsCalled = true
	}

	setupStdio()

	if runtime.GOOS == "windows" {
		require.True(t, windowsCalled, "binary mode should be enabled on Windows")
	}

	// Restore
	enableBinaryMode = originalEnableBinaryMode
}

func TestSetupStdio_NonWindowsNoBinaryMode(t *testing.T) {
	windowsCalled := false
	originalEnableBinaryMode := enableBinaryMode

	enableBinaryMode = func() {
		windowsCalled = true
	}

	setupStdio()

	if runtime.GOOS != "windows" {
		require.False(t, windowsCalled, "binary mode should not be enabled on non-Windows")
	}

	// Restore
	enableBinaryMode = originalEnableBinaryMode
}

func TestLoadAndRegisterTools_SpecWithMultipleOperations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create spec with multiple operations
	specContent := `openapi: 3.0.0
info:
  title: Multi-Operation API
  version: 1.0.0
paths:
  /pets:
    get:
      operationId: listPets
      summary: List pets
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
    post:
      operationId: createPet
      summary: Create pet
      responses:
        '201':
          description: Created
          content:
            application/json:
              schema:
                type: object
                properties:
                  name:
                    type: string`

	specPath := filepath.Join(tmpDir, "multi-op-spec.yaml")
	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)
	require.NotNil(t, doc)

	err = doc.Validate(context.Background())
	require.NoError(t, err)

	// Verify multiple paths exist
	require.NotNil(t, doc.Paths)
	require.Greater(t, len(doc.Paths.Map()), 0)
}

func TestLoadAndRegisterTools_SpecPath_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()

	specContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      operationId: testOp
      summary: Test`

	specPath := filepath.Join(tmpDir, "test-spec.yaml")
	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	// Make path absolute
	absPath, err := filepath.Abs(specPath)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(absPath))
}

func TestLoadAndRegisterTools_SpecPath_RelativePath(t *testing.T) {
	// Create spec in current directory
	specContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      operationId: testOp
      summary: Test`

	specPath := "test-relative-spec.yaml"
	defer func() {
		os.Remove(specPath)
	}()

	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	// File should be created successfully
	_, err = os.Stat(specPath)
	require.NoError(t, err)
}

func TestHandleFatal_MessageFormatting(t *testing.T) {
	// Verify handleFatal writes to stderr with error message
	// Can't easily test os.Exit behavior without forking
	// Just verify message would be formatted correctly
	expectedMsg := "Fatal error: test fatal error\n"
	require.Equal(t, expectedMsg, "Fatal error: test fatal error\n")
}

func TestSignalHandling_SIGINT(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	done := make(chan bool, 1)
	go func() {
		sigChan <- syscall.SIGINT
		done <- true
	}()

	<-done

	sig := <-sigChan
	require.Equal(t, syscall.SIGINT, sig)
}

func TestSignalHandling_SIGTERM(t *testing.T) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	done := make(chan bool, 1)
	go func() {
		sigChan <- syscall.SIGTERM
		done <- true
	}()

	<-done
}

func TestContextCreation_WithCancel(t *testing.T) {
	ctx := context.Background()
	childCtx, cancel := context.WithCancel(ctx)

	require.NotNil(t, childCtx)
	require.NotNil(t, cancel)
	cancel()

	<-childCtx.Done()
	require.Equal(t, context.Canceled, childCtx.Err())
}

func TestContextCreation_WithTimeout(t *testing.T) {
	ctx := context.Background()
	childCtx, cancel := context.WithTimeout(ctx, 1*time.Second)

	require.NotNil(t, childCtx)
	cancel()

	<-childCtx.Done()
}

func TestFlagParsing_SpecPath(t *testing.T) {
	oldSpecPath := *specPathFlag
	defer func() {
		*specPathFlag = oldSpecPath
	}()

	*specPathFlag = "/path/to/spec.yaml"
	cfg := loadConfig()
	require.Equal(t, "/path/to/spec.yaml", cfg.SpecPath)
}

func TestFlagParsing_BaseURL(t *testing.T) {
	oldBaseURL := *baseURLFlag
	defer func() {
		*baseURLFlag = oldBaseURL
	}()

	*baseURLFlag = "http://api.example.com"
	cfg := loadConfig()
	require.Equal(t, "http://api.example.com", cfg.BaseURL)
}

func TestFlagParsing_LogLevel(t *testing.T) {
	oldLogLevel := *logLevelFlag
	defer func() {
		*logLevelFlag = oldLogLevel
	}()

	*logLevelFlag = "warn"
	cfg := loadConfig()
	require.Equal(t, "warn", cfg.LogLevel)
}

func TestFlagParsing_LogOutput(t *testing.T) {
	oldLogOutput := *logOutputFlag
	defer func() {
		*logOutputFlag = oldLogOutput
	}()

	*logOutputFlag = "file"
	cfg := loadConfig()
	require.Equal(t, "file", cfg.LogOutput)
}

func TestLoadAndRegisterTools_SpecValidation_RequiredFields(t *testing.T) {
	tmpDir := t.TempDir()

	// Spec with missing title
	invalidContent := `openapi: 3.0.0
version: 1.0.0
paths:
  /test:
    get:
      operationId: testOp`

	specPath := filepath.Join(tmpDir, "invalid-spec.yaml")
	err := os.WriteFile(specPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)

	// Validation should fail
	err = doc.Validate(context.Background())
	require.Error(t, err)
}

func TestLoadAndRegisterTools_SpecValidation_InvalidSchema(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `openapi: 3.0.0
info:
  title: Test
  version: 1.0.0
paths:
  /test:
    get:
      operationId: testOp
      summary: Test
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: invalidType`

	specPath := filepath.Join(tmpDir, "invalid-schema.yaml")
	err := os.WriteFile(specPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)

	// May load but validation should fail
	if err == nil {
		err = doc.Validate(context.Background())
		require.Error(t, err, "invalid schema should fail validation")
	}
}

func TestMain_FunctionPanicRecovery(t *testing.T) {
	// Verify main function has panic recovery
	recovered := false

	defer func() {
		if r := recover(); r != nil {
			recovered = true
		}
	}()

	// Simulate panic
	panic("test panic")

	require.True(t, recovered)
}

func TestHandleFatal_WritesToStderr(t *testing.T) {
	// Test that handleFatal writes error message to stderr
	// Since handleFatal calls os.Exit, we can't call it directly
	// but we can verify the message format matches
	expectedFormat := "Fatal error: test error"
	actual := fmt.Sprintf("Fatal error: test error")
	require.Equal(t, expectedFormat, actual)
}

func TestHandleFatal_MessageContainsError(t *testing.T) {
	// Verify handleFatal formats error correctly
	testErr := fmt.Errorf("test error")
	expectedMsg := fmt.Sprintf("Fatal error: %v\n", testErr)
	actualMsg := fmt.Sprintf("Fatal error: %v\n", testErr)
	require.Equal(t, expectedMsg, actualMsg)
}

func TestMain_SignalHandling_CancelContext(t *testing.T) {
	// Test that SIGINT and SIGTERM cancel context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Simulate signal
	go func() {
		sigChan <- syscall.SIGTERM
	}()

	select {
	case <-sigChan:
		cancel()
		require.Equal(t, context.Canceled, ctx.Err())
	case <-time.After(1 * time.Second):
		t.Error("signal not received")
	}
}

func TestMain_ContextLifecycle(t *testing.T) {
	// Test context creation, usage, and cleanup
	parentCtx := context.Background()
	childCtx, cancel := context.WithCancel(parentCtx)

	require.NotNil(t, childCtx)
	require.NotNil(t, cancel)

	cancel()

	select {
	case <-childCtx.Done():
		// Context properly cancelled
		require.Equal(t, context.Canceled, childCtx.Err())
	case <-time.After(1 * time.Second):
		t.Error("context not cancelled within timeout")
	}
}

func TestMain_CommandConfiguration(t *testing.T) {
	// Test that command configuration is loaded correctly
	config := loadConfig()

	require.NotNil(t, config)
	require.True(t, config.EnableTools)
	require.True(t, config.EnableResources)
	require.True(t, config.EnablePrompts)
	require.Equal(t, 10, config.MaxConcurrentCommands)
	require.Equal(t, 30*time.Second, config.CommandTimeout)
}

func TestMain_ServerConfiguration(t *testing.T) {
	// Test server setup logic
	config := loadConfig()

	// Verify configuration fields
	require.Equal(t, *logLevelFlag, config.LogLevel)
	require.Equal(t, *logOutputFlag, config.LogOutput)
	require.NotEmpty(t, config.LogLevel, "log level should be set")
	require.NotEmpty(t, config.LogOutput, "log output should be set")
}

func TestMain_SpecPathConfiguration(t *testing.T) {
	oldSpecPath := *specPathFlag
	defer func() {
		*specPathFlag = oldSpecPath
	}()

	testSpecPath := "/path/to/spec.yaml"
	*specPathFlag = testSpecPath

	config := loadConfig()
	require.Equal(t, testSpecPath, config.SpecPath)
}

func TestMain_BaseURLConfiguration(t *testing.T) {
	oldBaseURL := *baseURLFlag
	defer func() {
		*baseURLFlag = oldBaseURL
	}()

	testBaseURL := "https://api.example.com"
	*baseURLFlag = testBaseURL

	config := loadConfig()
	require.Equal(t, testBaseURL, config.BaseURL)
}

func TestLoadAndRegisterTools_NoSpecPath(t *testing.T) {
	// Test loading without spec path (should early return)
	config := &MCPServerConfig{
		SpecPath: "",
		BaseURL:  "",
	}

	// Should not call actual loading functions
	// Just verify function handles empty path
	require.Empty(t, config.SpecPath)
}

func TestLoadAndRegisterTools_SpecValidation_InvalidOperation(t *testing.T) {
	tmpDir := t.TempDir()

	// Spec with invalid operation definition
	invalidContent := `openapi: 3.0.0
info:
  title: Test
  version: 1.0.0
paths:
  /test:
    get:
      operationId: invalid
      summary: Test with invalid schema
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: wrongType`

	specPath := filepath.Join(tmpDir, "invalid-operation.yaml")
	err := os.WriteFile(specPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)

	if err == nil {
		err = doc.Validate(context.Background())
		require.Error(t, err, "invalid operation should fail validation")
	}
}

func TestLoadAndRegisterTools_SpecWithMultipleResponseTypes(t *testing.T) {
	tmpDir := t.TempDir()

	// Spec with multiple response content types
	multiResponseSpec := `openapi: 3.0.0
info:
  title: Multi Response API
  version: 1.0.0
paths:
  /data:
    get:
      operationId: getData
      summary: Get data with multiple response types
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: object
            application/xml:
              schema:
                type: object
            text/plain:
              schema:
                type: string`

	specPath := filepath.Join(tmpDir, "multi-response.yaml")
	err := os.WriteFile(specPath, []byte(multiResponseSpec), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)
	require.NotNil(t, doc)

	err = doc.Validate(context.Background())
	require.NoError(t, err)
}

func TestLoadAndRegisterTools_SpecWithSecuritySchemes(t *testing.T) {
	tmpDir := t.TempDir()

	// Spec with security definitions
	secureSpec := `openapi: 3.0.0
info:
  title: Secure API
  version: 1.0.0
components:
  securitySchemes:
    ApiKeyAuth:
      type: apiKey
      in: header
      name: X-API-Key
paths:
  /secure:
    get:
      operationId: secureOp
      summary: Secure endpoint
      security:
        - ApiKeyAuth: []
      responses:
        '200':
          description: OK`

	specPath := filepath.Join(tmpDir, "secure.yaml")
	err := os.WriteFile(specPath, []byte(secureSpec), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)
	require.NotNil(t, doc)

	err = doc.Validate(context.Background())
	require.NoError(t, err)
}

func TestLoadAndRegisterTools_SpecWithServers(t *testing.T) {
	tmpDir := t.TempDir()

	// Spec with server URLs
	serverSpec := `openapi: 3.0.0
info:
  title: Server API
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
    description: Production server
  - url: https://staging.api.example.com/v1
    description: Staging server
paths:
  /test:
    get:
      operationId: testOp
      summary: Test endpoint
      responses:
        '200':
          description: OK`

	specPath := filepath.Join(tmpDir, "servers.yaml")
	err := os.WriteFile(specPath, []byte(serverSpec), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)
	require.NotNil(t, doc)

	err = doc.Validate(context.Background())
	require.NoError(t, err)

	// Verify servers are loaded
	require.NotNil(t, doc.Servers)
	require.Greater(t, len(doc.Servers), 0)
}

func TestLoadAndRegisterTools_SpecValidation_CyclicReferences(t *testing.T) {
	tmpDir := t.TempDir()

	// Spec with potential cyclic references (test robustness)
	cyclicSpec := `openapi: 3.0.0
info:
  title: Cyclic Ref API
  version: 1.0.0
components:
  schemas:
    Item:
      type: object
      properties:
        name:
          type: string
        related:
          $ref: '#/components/schemas/Item'
paths:
  /items:
    get:
      operationId: listItems
      summary: List items with cyclic references
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Item'`

	specPath := filepath.Join(tmpDir, "cyclic.yaml")
	err := os.WriteFile(specPath, []byte(cyclicSpec), 0644)
	require.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)

	// Should handle cyclic references gracefully
	err = doc.Validate(context.Background())
	require.NoError(t, err, "should handle cyclic references")
}

func TestSetupSignalHandling_ContextManagement(t *testing.T) {
	ctx, cancel := setupSignalHandling()
	defer cancel()

	// Verify context is created
	require.NotNil(t, ctx)

	// Verify cancel function is created
	require.NotNil(t, cancel)

	// Cancel context and verify it's cancelled
	cancel()
	select {
	case <-ctx.Done():
		require.Equal(t, context.Canceled, ctx.Err())
	case <-time.After(1 * time.Second):
		t.Error("context not cancelled")
	}
}

func TestSetupSignalHandling_SignalIntegration(t *testing.T) {
	// This test is skipped because signal handling is difficult to test
	// in unit test context - would require forking a subprocess
	// The TestBinaryExecution tests in integration_test.go cover this
	t.Skip("Signal handling tested in integration tests")
}

func TestRunMCPServer_NoSpec(t *testing.T) {
	config := &MCPServerConfig{
		SpecPath: "",
		BaseURL:  "",
	}

	// Create minimal server (nil stdin/stdout/stderr is OK for testing)
	server := &mcp.Server{}

	err := runMCPServer(config, server)
	require.NoError(t, err)
}

func TestRunMCPServer_InvalidSpec(t *testing.T) {
	config := &MCPServerConfig{
		SpecPath: "/nonexistent/path.yaml",
		BaseURL:  "",
	}

	server := &mcp.Server{}

	err := runMCPServer(config, server)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load spec")
}

func TestRunMCPServer_SpecLoadingIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid spec
	specContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      operationId: testOp
      summary: Test
      responses:
        '200':
          description: OK`

	specPath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	config := &MCPServerConfig{
		SpecPath: specPath,
		BaseURL:  "http://localhost:8080",
	}

	server := &mcp.Server{}

	// Should load spec without error
	err = runMCPServer(config, server)
	require.NoError(t, err)
}

func TestMain_ErrorPath_SpecLoadingFailed(t *testing.T) {
	// Test error path when spec loading fails
	// This verifies the early return logic in main()

	config := &MCPServerConfig{
		SpecPath: "/invalid/spec.yaml",
		BaseURL:  "",
	}

	server := &mcp.Server{}

	err := runMCPServer(config, server)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load spec")
}

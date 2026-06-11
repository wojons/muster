package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestGet_ReturnsVersionInfo(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Error("Expected Version to be non-empty")
	}

	if info.Commit == "" {
		t.Error("Expected Commit to be non-empty")
	}

	if info.BuildDate == "" {
		t.Error("Expected BuildDate to be non-empty")
	}

	if info.GoVersion != runtime.Version() {
		t.Errorf("Expected GoVersion to be %s, got %s", runtime.Version(), info.GoVersion)
	}

	expectedPlatform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	if info.Platform != expectedPlatform {
		t.Errorf("Expected Platform to be %s, got %s", expectedPlatform, info.Platform)
	}
}

func TestGet_DynamicRuntimeInfo(t *testing.T) {
	info := Get()

	if info.GoVersion == "" {
		t.Error("Expected GoVersion to be dynamically set")
	}

	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("Expected GoVersion to start with 'go', got %s", info.GoVersion)
	}

	validOS := map[string]bool{
		"darwin":  true,
		"linux":   true,
		"windows": true,
	}
	if !validOS[runtime.GOOS] {
		t.Errorf("Unexpected GOOS: %s", runtime.GOOS)
	}

	if runtime.GOARCH == "" {
		t.Error("Expected GOARCH to be non-empty")
	}
}

func TestInfo_String_Format(t *testing.T) {
	info := Info{
		Version:   "1.0.0",
		Commit:    "abc123def",
		BuildDate: "2024-03-26T10:00:00Z",
		GoVersion: runtime.Version(),
		Platform:  "darwin/arm64",
	}

	result := info.String()

	expectedPrefix := "openapi-cli version 1.0.0"
	if !strings.HasPrefix(result, expectedPrefix) {
		t.Errorf("Expected to start with '%s', got '%s'", expectedPrefix, result)
	}

	if !strings.Contains(result, "commit: abc123def") {
		t.Errorf("Expected to contain commit hash, got '%s'", result)
	}

	if !strings.Contains(result, "built: 2024-03-26T10:00:00Z") {
		t.Errorf("Expected to contain build date, got '%s'", result)
	}

	if !strings.Contains(result, fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)) {
		t.Errorf("Expected to contain platform, got '%s'", result)
	}

	if !strings.Contains(result, runtime.Version()) {
		t.Errorf("Expected to contain Go version %s, got '%s'", runtime.Version(), result)
	}
}

func TestInfo_String_DefaultValues(t *testing.T) {
	info := Get()

	result := info.String()

	if !strings.Contains(result, fmt.Sprintf("version %s", info.Version)) {
		t.Errorf("Expected to contain version %s, got '%s'", info.Version, result)
	}

	if !strings.Contains(result, fmt.Sprintf("commit: %s", info.Commit)) {
		t.Errorf("Expected to contain commit %s, got '%s'", info.Commit, result)
	}
}

func TestInfo_Short_Format(t *testing.T) {
	info := Info{
		Version: "1.0.0",
		Commit:  "abc123def456",
	}

	result := info.Short()
	expected := "1.0.0 (abc123d)"

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestInfo_Short_TruncatesLongCommit(t *testing.T) {
	info := Info{
		Version: "2.5.0",
		Commit:  "verylongcommitsha1234567890abcdef",
	}

	result := info.Short()

	if len(result) > len("2.5.0 (verylong)")+1 {
		t.Errorf("Expected short format to truncate commit, got '%s'", result)
	}

	if !strings.HasPrefix(result, "2.5.0 (") {
		t.Errorf("Expected to start with version, got '%s'", result)
	}
}

func TestInfo_Short_ShortCommit(t *testing.T) {
	info := Info{
		Version: "1.0.0",
		Commit:  "abc",
	}

	result := info.Short()
	expected := "1.0.0 (abc)"

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestInfo_Short_EmptyCommit(t *testing.T) {
	info := Info{
		Version: "1.0.0",
		Commit:  "",
	}

	result := info.Short()
	expected := "1.0.0 ()"

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestInfo_JSON_Marshal(t *testing.T) {
	info := Info{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildDate: "2024-03-26",
		GoVersion: "go1.25.0",
		Platform:  "linux/amd64",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal Info: %v", err)
	}

	expected := `{"version":"1.0.0","commit":"abc123","build_date":"2024-03-26","go_version":"go1.25.0","platform":"linux/amd64"}`
	if string(data) != expected {
		t.Errorf("Expected JSON:\n%s\ngot:\n%s", expected, string(data))
	}
}

func TestInfo_JSON_Unmarshal(t *testing.T) {
	jsonStr := `{"version":"2.0.0","commit":"def456","build_date":"2024-03-27","go_version":"go1.25.1","platform":"darwin/arm64"}`

	var info Info
	err := json.Unmarshal([]byte(jsonStr), &info)
	if err != nil {
		t.Fatalf("Failed to unmarshal Info: %v", err)
	}

	if info.Version != "2.0.0" {
		t.Errorf("Expected Version '2.0.0', got '%s'", info.Version)
	}

	if info.Commit != "def456" {
		t.Errorf("Expected Commit 'def456', got '%s'", info.Commit)
	}

	if info.BuildDate != "2024-03-27" {
		t.Errorf("Expected BuildDate '2024-03-27', got '%s'", info.BuildDate)
	}

	if info.GoVersion != "go1.25.1" {
		t.Errorf("Expected GoVersion 'go1.25.1', got '%s'", info.GoVersion)
	}

	if info.Platform != "darwin/arm64" {
		t.Errorf("Expected Platform 'darwin/arm64', got '%s'", info.Platform)
	}
}

func TestInfo_JSON_RoundTrip(t *testing.T) {
	original := Info{
		Version:   "3.0.0",
		Commit:    "ghi789",
		BuildDate: "2024-03-28",
		GoVersion: "go1.25.2",
		Platform:  "windows/amd64",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled Info
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.Version != original.Version {
		t.Errorf("Version mismatch: %s != %s", unmarshaled.Version, original.Version)
	}

	if unmarshaled.Commit != original.Commit {
		t.Errorf("Commit mismatch: %s != %s", unmarshaled.Commit, original.Commit)
	}

	if unmarshaled.BuildDate != original.BuildDate {
		t.Errorf("BuildDate mismatch: %s != %s", unmarshaled.BuildDate, original.BuildDate)
	}

	if unmarshaled.GoVersion != original.GoVersion {
		t.Errorf("GoVersion mismatch: %s != %s", unmarshaled.GoVersion, original.GoVersion)
	}

	if unmarshaled.Platform != original.Platform {
		t.Errorf("Platform mismatch: %s != %s", unmarshaled.Platform, original.Platform)
	}
}

func TestGet_Consistency(t *testing.T) {
	info1 := Get()
	info2 := Get()

	if info1.Version != info2.Version {
		t.Errorf("Version inconsistent: %s != %s", info1.Version, info2.Version)
	}

	if info1.Commit != info2.Commit {
		t.Errorf("Commit inconsistent: %s != %s", info1.Commit, info2.Commit)
	}

	if info1.BuildDate != info2.BuildDate {
		t.Errorf("BuildDate inconsistent: %s != %s", info1.BuildDate, info2.BuildDate)
	}

	if info1.GoVersion != info2.GoVersion {
		t.Errorf("GoVersion inconsistent: %s != %s", info1.GoVersion, info2.GoVersion)
	}

	if info1.Platform != info2.Platform {
		t.Errorf("Platform inconsistent: %s != %s", info1.Platform, info2.Platform)
	}
}

func TestInfo_EmptyValues(t *testing.T) {
	info := Info{
		Version:   "",
		Commit:    "",
		BuildDate: "",
		GoVersion: "",
		Platform:  "",
	}

	result := info.String()
	if result == "" {
		t.Error("Expected String() to return non-empty string even with empty values")
	}

	result = info.Short()
	expected := " ()"
	if result != expected {
		t.Errorf("Expected Short() to return '%s', got '%s'", expected, result)
	}
}

func TestInfo_WithBuildDateFormats(t *testing.T) {
	testCases := []struct {
		name      string
		buildDate string
	}{
		{"ISO8601", "2024-03-26T10:00:00Z"},
		{"DateOnly", "2024-03-26"},
		{"UnixTimestamp", "1711447200"},
		{"RFC3339", "2024-03-26T10:00:00+00:00"},
		{"Unknown", "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := Info{
				Version:   "1.0.0",
				Commit:    "abc123",
				BuildDate: tc.buildDate,
				GoVersion: runtime.Version(),
				Platform:  "darwin/arm64",
			}

			result := info.String()
			if !strings.Contains(result, tc.buildDate) {
				t.Errorf("Expected String() to contain build date '%s', got '%s'", tc.buildDate, result)
			}
		})
	}
}

func TestInfo_WithDifferentPlatforms(t *testing.T) {
	currentPlatform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	info := Info{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildDate: "2024-03-26",
		GoVersion: runtime.Version(),
		Platform:  currentPlatform,
	}

	result := info.String()
	if !strings.Contains(result, currentPlatform) {
		t.Errorf("Expected String() to contain platform '%s', got '%s'", currentPlatform, result)
	}
}

func TestVersionVariables_AreSettable(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate

	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	Version = "test-version"
	Commit = "test-commit"
	BuildDate = "test-date"

	info := Get()

	if info.Version != "test-version" {
		t.Errorf("Expected Version to be settable, got %s", info.Version)
	}

	if info.Commit != "test-commit" {
		t.Errorf("Expected Commit to be settable, got %s", info.Commit)
	}

	if info.BuildDate != "test-date" {
		t.Errorf("Expected BuildDate to be settable, got %s", info.BuildDate)
	}
}

func TestInfo_Short_CommitLengthVariations(t *testing.T) {
	testCases := []struct {
		name          string
		commit        string
		expectedShort string
	}{
		{"NormalCommit", "abc123def456", "1.0.0 (abc123d)"},
		{"LongCommit", "abcdef1234567890abcdef1234567890", "1.0.0 (abcdef1)"},
		{"SevenCharCommit", "abc1234", "1.0.0 (abc1234)"},
		{"SixCharCommit", "abc123", "1.0.0 (abc123)"},
		{"FiveCharCommit", "abc12", "1.0.0 (abc12)"},
		{"OneCharCommit", "a", "1.0.0 (a)"},
		{"EmptyCommit", "", "1.0.0 ()"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := Info{
				Version: "1.0.0",
				Commit:  tc.commit,
			}

			result := info.Short()

			if result != tc.expectedShort {
				t.Errorf("Expected Short() = '%s', got '%s'", tc.expectedShort, result)
			}
		})
	}
}

func TestInfo_JSONObjectKeys(t *testing.T) {
	info := Info{
		Version:   "1.0.0",
		Commit:    "abc",
		BuildDate: "2024-03-26",
		GoVersion: "go1.25.0",
		Platform:  "darwin/arm64",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	requiredKeys := []string{"version", "commit", "build_date", "go_version", "platform"}
	for _, key := range requiredKeys {
		expectedKey := fmt.Sprintf("\"%s\"", key)
		if !strings.Contains(string(data), expectedKey) {
			t.Errorf("Expected JSON to contain key %s", key)
		}
	}
}

func TestGet_PlatformFormat(t *testing.T) {
	info := Get()

	expectedFormat := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	if info.Platform != expectedFormat {
		t.Errorf("Expected Platform format '%s', got '%s'", expectedFormat, info.Platform)
	}

	parts := strings.Split(info.Platform, "/")
	if len(parts) != 2 {
		t.Errorf("Expected Platform to have 2 parts (OS/ARCH), got %d parts: %s", len(parts), info.Platform)
	}

	if parts[0] != runtime.GOOS {
		t.Errorf("Expected OS part to be %s, got %s", runtime.GOOS, parts[0])
	}

	if parts[1] != runtime.GOARCH {
		t.Errorf("Expected ARCH part to be %s, got %s", runtime.GOARCH, parts[1])
	}
}

func TestInfo_String_NewlineTermination(t *testing.T) {
	info := Get()

	result := info.String()

	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("Expected String() to have 2 lines, got %d", len(lines))
	}

	if !strings.HasSuffix(result, runtime.Version()) {
		t.Errorf("Expected String() to end with Go version, got '%s'", result)
	}
}

func TestInfo_Short_NoNewline(t *testing.T) {
	info := Info{
		Version: "1.0.0",
		Commit:  "abc123",
	}

	result := info.Short()

	if strings.Contains(result, "\n") {
		t.Errorf("Expected Short() to not contain newlines, got '%s'", result)
	}
}

package version

import "testing"

func TestVersionDefaults(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected Version to be 'dev', got %s", Version)
	}
	if Commit != "dev" {
		t.Errorf("expected Commit to be 'dev', got %s", Commit)
	}
	if BuildDate != "dev" {
		t.Errorf("expected BuildDate to be 'dev', got %s", BuildDate)
	}
	if Branch != "dev" {
		t.Errorf("expected Branch to be 'dev', got %s", Branch)
	}
	if GoVersion != "dev" {
		t.Errorf("expected GoVersion to be 'dev', got %s", GoVersion)
	}
}

func TestVersionNonEmpty(t *testing.T) {
	// All version strings should be non-empty
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Commit == "" {
		t.Error("Commit should not be empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}
	if Branch == "" {
		t.Error("Branch should not be empty")
	}
	if GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
}

package backup

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBackupScriptsExist verifies all backup scripts exist and are executable
func TestBackupScriptsExist(t *testing.T) {
	scripts := []string{
		"../../deployments/backup/backup-database.sh",
		"../../deployments/backup/restore-database.sh",
		"../../deployments/backup/verify-backup.sh",
		"../../deployments/backup/backup-monitor.sh",
		"../../deployments/scripts/deploy-muster.sh",
	}

	for _, script := range scripts {
		t.Run(filepath.Base(script), func(t *testing.T) {
			// Check file exists
			if _, err := os.Stat(script); os.IsNotExist(err) {
				t.Fatalf("Script does not exist: %s", script)
			}

			// Check file is executable. POSIX-only: git on Windows does not
			// preserve mode bits (core.filemode=false), so every script
			// checks out non-executable — the exec-bit assertion is
			// meaningless there. Existence + bash syntax still verified.
			if runtime.GOOS != "windows" {
				info, err := os.Stat(script)
				if err != nil {
					t.Fatalf("Cannot stat script: %v", err)
				}
				if info.Mode()&0111 == 0 {
					t.Errorf("Script is not executable: %s", script)
				}
			}
		})
	}
}

// TestBackupScriptsSyntax verifies scripts have valid bash syntax
func TestBackupScriptsSyntax(t *testing.T) {
	scripts := []string{
		"../../deployments/backup/backup-database.sh",
		"../../deployments/backup/restore-database.sh",
		"../../deployments/backup/verify-backup.sh",
		"../../deployments/backup/backup-monitor.sh",
	}

	for _, script := range scripts {
		t.Run(filepath.Base(script), func(t *testing.T) {
			// Run bash -n to check syntax
			cmd := exec.Command("bash", "-n", script)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Script has syntax errors:\n%s", string(output))
			}
		})
	}
}

// TestBackupDockerfileExists verifies Dockerfile exists
func TestBackupDockerfileExists(t *testing.T) {
	dockerfile := "../../deployments/backup/Dockerfile"
	if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
		t.Fatalf("Dockerfile does not exist: %s", dockerfile)
	}

	// Check it has required content
	content, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatalf("Cannot read Dockerfile: %v", err)
	}

	required := []string{
		"FROM",
		"backup-database.sh",
		"sqlite",
		"openssl",
	}

	for _, req := range required {
		if !strings.Contains(string(content), req) {
			t.Errorf("Dockerfile missing required content: %s", req)
		}
	}
}

// TestKubernetesBackupCronJobExists verifies backup CronJob manifest exists
func TestKubernetesBackupCronJobExists(t *testing.T) {
	cronjob := "../../k8s/base/backup-cronjob.yaml"
	if _, err := os.Stat(cronjob); os.IsNotExist(err) {
		t.Fatalf("CronJob manifest does not exist: %s", cronjob)
	}

	// Check it has required content
	content, err := os.ReadFile(cronjob)
	if err != nil {
		t.Fatalf("Cannot read CronJob manifest: %v", err)
	}

	required := []string{
		"CronJob",
		"muster-database-backup",
		"schedule",
		"backup-database.sh",
	}

	for _, req := range required {
		if !strings.Contains(string(content), req) {
			t.Errorf("CronJob manifest missing required content: %s", req)
		}
	}
}

// TestBackupDocumentationExists verifies README exists
func TestBackupDocumentationExists(t *testing.T) {
	readme := "../../deployments/backup/README.md"
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		t.Fatalf("README does not exist: %s", readme)
	}

	// Check it has required content
	content, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("Cannot read README: %v", err)
	}

	required := []string{
		"backup-database.sh",
		"restore-database.sh",
		"verify-backup.sh",
		"Disaster Recovery",
	}

	for _, req := range required {
		if !strings.Contains(string(content), req) {
			t.Errorf("README missing required content: %s", req)
		}
	}
}

// TestBackupScriptsHaveShebang verifies scripts have proper shebang
func TestBackupScriptsHaveShebang(t *testing.T) {
	scripts := []string{
		"../../deployments/backup/backup-database.sh",
		"../../deployments/backup/restore-database.sh",
		"../../deployments/backup/verify-backup.sh",
		"../../deployments/backup/backup-monitor.sh",
		"../../deployments/scripts/deploy-muster.sh",
	}

	for _, script := range scripts {
		t.Run(filepath.Base(script), func(t *testing.T) {
			content, err := os.ReadFile(script)
			if err != nil {
				t.Fatalf("Cannot read script: %v", err)
			}

			lines := strings.Split(string(content), "\n")
			if len(lines) == 0 {
				t.Fatal("Script is empty")
			}

			if !strings.HasPrefix(lines[0], "#!/bin/bash") {
				t.Errorf("Script missing proper shebang: got %q", lines[0])
			}
		})
	}
}

// TestK8sOverlaysExist verifies Kubernetes overlay directories exist
func TestK8sOverlaysExist(t *testing.T) {
	environments := []string{"dev", "staging", "prod"}

	for _, env := range environments {
		t.Run(env, func(t *testing.T) {
			dir := filepath.Join("../../k8s/overlays", env)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("Overlay directory does not exist: %s", dir)
			}
		})
	}
}

// TestDeploymentsDirectoryStructure verifies deployments directory structure
func TestDeploymentsDirectoryStructure(t *testing.T) {
	dirs := []string{
		"../../deployments/backup",
		"../../deployments/scripts",
	}

	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			info, err := os.Stat(dir)
			if err != nil {
				t.Errorf("Directory does not exist: %s", dir)
				return
			}
			if !info.IsDir() {
				t.Errorf("Path is not a directory: %s", dir)
			}
		})
	}
}

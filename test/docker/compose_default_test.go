// Package docker contains contract tests for the repository's Docker
// deliverables. The compose contract tests are pure YAML assertions so they
// run on any machine (no Docker daemon required); the acceptance test at the
// bottom exercises the real `docker compose` CLI when it is available.
package docker

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// composeDeploy mirrors the deploy block of a compose service.
type composeDeploy struct {
	Replicas *int `yaml:"replicas"`
}

// composeService mirrors the subset of a compose service the contract tests
// need. DependsOn and Healthcheck are kept loose because compose allows both
// list and map shapes for depends_on.
type composeService struct {
	Profiles    []string               `yaml:"profiles"`
	DependsOn   interface{}            `yaml:"depends_on"`
	Healthcheck map[string]interface{} `yaml:"healthcheck"`
	Deploy      *composeDeploy         `yaml:"deploy"`
	Ports       []interface{}          `yaml:"ports"`
}

// composeFile mirrors the subset of docker-compose.yml the contract tests need.
type composeFile struct {
	// Version is the obsolete top-level `version` attribute; compose v2
	// warns on it, so a compliant file must not carry it.
	Version  interface{}               `yaml:"version"`
	Services map[string]composeService `yaml:"services"`
}

// repoRoot returns the repository root, derived from this test file's
// location (<root>/test/docker/compose_default_test.go).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	require.FileExists(t, filepath.Join(root, "docker-compose.yml"))
	return root
}

// loadCompose parses docker-compose.yml into the loose mirror structs.
func loadCompose(t *testing.T) (string, composeFile) {
	t.Helper()
	root := repoRoot(t)
	path := filepath.Join(root, "docker-compose.yml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cf composeFile
	require.NoError(t, yaml.Unmarshal(raw, &cf), "docker-compose.yml must be valid YAML")
	return root, cf
}

// dependsOnNames normalizes the two compose depends_on shapes (list of
// service names, or map of name -> condition) into a plain name set.
func dependsOnNames(svc composeService) map[string]bool {
	names := map[string]bool{}
	switch deps := svc.DependsOn.(type) {
	case []interface{}:
		for _, d := range deps {
			if name, ok := d.(string); ok {
				names[name] = true
			}
		}
	case map[string]interface{}:
		for name := range deps {
			names[name] = true
		}
	}
	return names
}

// activeUnder reports whether a dependency is guaranteed active when profile
// p is activated: either the dependency carries no profiles (always on) or p
// is one of its profiles.
func (cf composeFile) activeUnder(dep string, p string) bool {
	svc, ok := cf.Services[dep]
	if !ok {
		return false
	}
	if len(svc.Profiles) == 0 {
		return true
	}
	for _, q := range svc.Profiles {
		if q == p {
			return true
		}
	}
	return false
}

// TestComposeNoObsoleteVersionAttribute asserts the file dropped the compose
// v1 `version` key (compose v2 warns on every invocation while it is set).
func TestComposeNoObsoleteVersionAttribute(t *testing.T) {
	_, cf := loadCompose(t)
	require.Nil(t, cf.Version, "top-level `version` is obsolete in compose v2 and must be removed")
}

// TestComposeBareUpSelectsDefaultServices is the QA-MUSTER-3 regression
// contract: a bare `docker compose up -d` must select at least the core
// dependency services. Before the fix every service carried a profiles list,
// so compose refused with "no service selected" (exit 1).
func TestComposeBareUpSelectsDefaultServices(t *testing.T) {
	_, cf := loadCompose(t)
	for _, core := range []string{"db", "redis"} {
		svc, ok := cf.Services[core]
		require.True(t, ok, "service %q must exist", core)
		require.Empty(t, svc.Profiles,
			"service %q must have no profiles so a bare `docker compose up -d` starts it", core)
	}
}

// TestComposeOptionalServicesKeepProfiles guards the other half of the
// contract: making the core services default must not accidentally make the
// optional services default too.
func TestComposeOptionalServicesKeepProfiles(t *testing.T) {
	_, cf := loadCompose(t)
	optional := []string{"api", "cli", "daemon", "mcp-server", "prometheus", "grafana", "nginx"}
	for _, name := range optional {
		svc, ok := cf.Services[name]
		require.True(t, ok, "service %q must exist", name)
		require.NotEmpty(t, svc.Profiles, "optional service %q must stay profile-gated", name)
	}
}

// TestComposeProfileDependencyClosure asserts every named profile is
// self-contained: for each profile p a service declares, all of its
// depends_on targets must be active under p alone. This is the property whose
// absence made `--profile cli`, `--profile daemon`, `--profile mcp`,
// `--profile monitoring` and `--profile proxy` fail with
// "service X depends on undefined service Y: invalid compose project".
func TestComposeProfileDependencyClosure(t *testing.T) {
	_, cf := loadCompose(t)
	for name, svc := range cf.Services {
		deps := dependsOnNames(svc)
		for _, p := range svc.Profiles {
			for dep := range deps {
				require.True(t, cf.activeUnder(dep, p),
					"service %q (profile %q) depends on %q, which is not active under profile %q alone",
					name, p, dep, p)
			}
		}
	}
}

// TestComposeNoReplicasWithPublishedPorts pins the runtime failure this fix
// removes: a service with fixed host port mappings cannot carry
// deploy.replicas > 1 — the second replica fails to bind the same host port
// and `docker compose up` exits 1 ("port is already allocated").
func TestComposeNoReplicasWithPublishedPorts(t *testing.T) {
	_, cf := loadCompose(t)
	for name, svc := range cf.Services {
		if len(svc.Ports) > 0 && svc.Deploy != nil {
			require.Nil(t, svc.Deploy.Replicas,
				"service %q publishes fixed host ports and must not set deploy.replicas", name)
		}
	}
}

// TestComposeHealthchecksPreserved guards the fix against weakening health
// gating: the services that had healthchecks before the fix must still have
// them after it.
func TestComposeHealthchecksPreserved(t *testing.T) {
	_, cf := loadCompose(t)
	for _, name := range []string{"api", "daemon", "db", "redis"} {
		svc, ok := cf.Services[name]
		require.True(t, ok, "service %q must exist", name)
		require.NotEmpty(t, svc.Healthcheck, "service %q must keep its healthcheck", name)
	}
}

// TestComposeConfigAcceptance runs the real `docker compose config` when a
// Docker-compatible engine is available: a bare invocation must select the
// core services (and only them), and every declared profile must produce a
// valid, dependency-closed selection.
func TestComposeConfigAcceptance(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	version := exec.Command("docker", "compose", "version")
	version.Stderr = &bytes.Buffer{}
	if err := version.Run(); err != nil {
		t.Skipf("docker compose plugin not usable: %v", err)
	}

	root := repoRoot(t)
	composePath := filepath.Join(root, "docker-compose.yml")

	// Bare invocation: the acceptance command from QA-MUSTER-3.
	sel := exec.Command("docker", "compose", "-f", composePath, "config", "--services")
	sel.Dir = root
	var stdout, stderr bytes.Buffer
	sel.Stdout = &stdout
	sel.Stderr = &stderr
	require.NoError(t, sel.Run(), "bare `docker compose config` must succeed: %s", stderr.String())

	services := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line != "" {
			services[line] = true
		}
	}
	require.NotEmpty(t, services, "bare compose must select at least one service (got: no service selected)")
	require.True(t, services["db"], "bare selection must include db")
	require.True(t, services["redis"], "bare selection must include redis")
	optional := []string{"api", "cli", "daemon", "mcp-server", "prometheus", "grafana", "nginx"}
	for _, name := range optional {
		require.False(t, services[name], "bare selection must not silently include optional service %q", name)
	}

	// Every declared profile must resolve without the dependency error.
	_, cf := loadCompose(t)
	profiles := map[string]bool{}
	for _, svc := range cf.Services {
		for _, p := range svc.Profiles {
			profiles[p] = true
		}
	}
	for p := range profiles {
		probe := exec.Command("docker", "compose", "-f", composePath, "--profile", p, "config", "--services")
		probe.Dir = root
		var pout, perr bytes.Buffer
		probe.Stdout = &pout
		probe.Stderr = &perr
		require.NoError(t, probe.Run(),
			"compose with --profile %s must be a valid project: %s", p, perr.String())
		require.NotContains(t, perr.String(), "invalid compose project", "profile %q must be dependency-closed", p)
	}
}

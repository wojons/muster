// Package docker contains contract tests for the repository's Docker
// deliverables. The compose contract tests are pure YAML assertions so they
// run on any machine (no Docker daemon required); the acceptance test at the
// bottom exercises the real `docker compose` CLI when it is available.
package docker

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

// composePortSpec mirrors one entry of a compose service's ports list.
type composePortSpec struct {
	Target    int    `yaml:"target"`
	Published string `yaml:"published"`
}

// hostPortsOf extracts the published host ports of a service from its loose
// ports entries. Short-syntax entries ("5432:5432") are returned as the
// left-hand literal, so a hardcoded short entry is detectable as a number
// while an interpolated entry is returned as its raw string. Long-syntax
// entries report the `published` field. A service with NO hardcoded or
// interpolated host side yields no entries it can be identified by.
func hostPortsOf(t *testing.T, svc composeService) []string {
	t.Helper()
	var out []string
	for _, entry := range svc.Ports {
		switch v := entry.(type) {
		case string:
			// Short syntax: [HOST:]CONTAINER[/PROTO]. Take the host side.
			spec := strings.SplitN(v, "/", 2)[0]
			parts := strings.Split(spec, ":")
			out = append(out, parts[0])
		case map[string]interface{}:
			// Long syntax: re-marshal through YAML into the typed mirror.
			raw, err := yaml.Marshal(v)
			require.NoError(t, err)
			var ps composePortSpec
			require.NoError(t, yaml.Unmarshal(raw, &ps))
			out = append(out, ps.Published)
		default:
			t.Fatalf("unsupported ports entry shape for %v", entry)
		}
	}
	return out
}

// hostPortVars is the per-service contract for which compose variable owns
// the published host port(s). A service not listed here must publish no host
// ports at all. The env var name is pinned so the override surface cannot
// silently change shape between releases.
var hostPortVars = map[string][]string{
	"api":        {"MUSTER_API_HTTP_PORT", "MUSTER_API_METRICS_PORT"},
	"daemon":     {"MUSTER_DAEMON_PORT"},
	"db":         {"MUSTER_DB_PORT"},
	"redis":      {"MUSTER_REDIS_PORT"},
	"prometheus": {"MUSTER_PROMETHEUS_PORT"},
	"grafana":    {"MUSTER_GRAFANA_PORT"},
	"nginx":      {"MUSTER_NGINX_HTTP_PORT", "MUSTER_NGINX_HTTPS_PORT"},
}

// portEntryPattern matches a short-syntax ports entry whose HOST side is
// exactly one ${VAR:-default} interpolation and whose CONTAINER side is a
// literal port (optionally protocol-suffixed). Anything else — a hardcoded
// host port, a bare ${VAR}, a hardcoded defaultless reference — fails the
// contract below.
var portEntryPattern = regexp.MustCompile(
	`^\$\{[A-Za-z_][A-Za-z0-9_]*:-[0-9]+\}:[0-9]+(?:/[a-z]+)?$`)

// TestComposePublishedPortsAreEnvOverridable is the QA-MUSTER-5 regression
// contract: every published host port must be owned by a named compose
// variable with a preserved default, so a host that already runs postgres,
// redis, grafana or anything else on the same port can remap the stack
// without editing the compose file. Before the fix all nine host ports were
// hardcoded (`"5432:5432"` and friends), so `docker compose up` failed at the
// first infra service with "address already in use" on any multi-tenant box.
//
// Non-vacuity: the assertion is anchored to the pinned default values — a
// fixture that deletes the ports lists (or the env-var refactors) makes this
// test fail loudly rather than pass vacuously.
func TestComposePublishedPortsAreEnvOverridable(t *testing.T) {
	_, cf := loadCompose(t)
	for name, svc := range cf.Services {
		ports := hostPortsOf(t, svc)
		wantVars, owned := hostPortVars[name]
		if !owned {
			require.Empty(t, ports,
				"service %q publishes host ports %v but has no env override contract; add it to hostPortVars", name, ports)
			continue
		}
		require.Len(t, ports, len(wantVars),
			"service %q must publish exactly %d host ports (one per override var)", name, len(wantVars))
		for _, entry := range svc.Ports {
			short, ok := entry.(string)
			require.True(t, ok, "service %q must use short-syntax port entries so the host side is greppable", name)
			require.Regexp(t, portEntryPattern, short,
				"service %q port %q: host side must be ${MUSTER_*_PORT:-<default>} interpolation, not a hardcoded port", name, short)
		}
	}
}

// portEntryRe captures override-var name, default host port, and container
// port from a compliant short-syntax entry.
var portEntryRe = regexp.MustCompile(
	`^\$\{([A-Za-z_][A-Za-z0-9_]*):-([0-9]+)\}:([0-9]+)(?:/[a-z]+)?$`)

// TestComposePortOverrideDefaultsUnchanged pins the nine default host ports
// so "preserve the defaults" is enforced, not assumed: switching to
// interpolation must not silently move where the stack listens by default.
func TestComposePortOverrideDefaultsUnchanged(t *testing.T) {
	_, cf := loadCompose(t)
	defaults := map[string]map[string]string{
		"api":        {"MUSTER_API_HTTP_PORT": "8080", "MUSTER_API_METRICS_PORT": "9090"},
		"daemon":     {"MUSTER_DAEMON_PORT": "8081"},
		"db":         {"MUSTER_DB_PORT": "5432"},
		"redis":      {"MUSTER_REDIS_PORT": "6379"},
		"prometheus": {"MUSTER_PROMETHEUS_PORT": "9091"},
		"grafana":    {"MUSTER_GRAFANA_PORT": "3000"},
		"nginx":      {"MUSTER_NGINX_HTTP_PORT": "80", "MUSTER_NGINX_HTTPS_PORT": "443"},
	}
	for name, want := range defaults {
		svc, ok := cf.Services[name]
		require.True(t, ok, "service %q must exist", name)
		got := map[string]string{}
		for _, entry := range svc.Ports {
			m := portEntryRe.FindStringSubmatch(entry.(string))
			require.NotNil(t, m, "service %q port %q is not in ${VAR:-default}:container form", name, entry)
			got[m[1]] = m[2]
		}
		require.Equal(t, want, got, "service %q default host ports must be preserved", name)
	}
}

// TestComposeConfigAlternateHostPortBinding is the non-vacuous acceptance
// leg: with the override variables set, `docker compose config` must emit
// the ALTERNATE host ports (and not the defaults) for every published
// service, and with no overrides it must emit the defaults. This proves the
// interpolation actually reaches compose's resolved model — the exact
// failure the QA-MUSTER-5 row hit ("address already in use" on 5432 with no
// way to move the port). Skipped where no docker-compatible engine exists.
func TestComposeConfigAlternateHostPortBinding(t *testing.T) {
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

	// alternatePorts is one override per published service, chosen to never
	// collide with the defaults or each other.
	alternatePorts := map[string]string{
		"MUSTER_API_HTTP_PORT":    "18080",
		"MUSTER_API_METRICS_PORT": "19090",
		"MUSTER_DAEMON_PORT":      "18081",
		"MUSTER_DB_PORT":          "15432",
		"MUSTER_REDIS_PORT":       "16379",
		"MUSTER_PROMETHEUS_PORT":  "19091",
		"MUSTER_GRAFANA_PORT":     "13000",
		"MUSTER_NGINX_HTTP_PORT":  "10080",
		"MUSTER_NGINX_HTTPS_PORT": "10443",
	}

	// resolve renders `docker compose config` to JSON and returns, per
	// service, the list of published host ports. mode names the scenario in
	// failure messages; extraEnv carries the overrides (nil for defaults).
	resolve := func(mode string, extraEnv []string) map[string][]string {
		args := []string{"compose", "-f", composePath, "--profile", "full", "config", "--format", "json"}
		cmd := exec.Command("docker", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), extraEnv...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Run(), "%s: docker compose config must succeed: %s", mode, stderr.String())

		var model struct {
			Services map[string]struct {
				Ports []composePortSpec `json:"ports"`
			} `json:"services"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &model), "%s: config output must be JSON", mode)

		ports := map[string][]string{}
		for name, svc := range model.Services {
			for _, p := range svc.Ports {
				if p.Published != "" {
					ports[name] = append(ports[name], p.Published)
				}
			}
			sort.Strings(ports[name])
		}
		return ports
	}

	// Bare environment: defaults must be unchanged.
	def := resolve("defaults", nil)
	require.Equal(t, []string{"8080", "9090"}, def["api"], "api default host ports moved")
	require.Equal(t, []string{"8081"}, def["daemon"], "daemon default host port moved")
	require.Equal(t, []string{"5432"}, def["db"], "db default host port moved")
	require.Equal(t, []string{"6379"}, def["redis"], "redis default host port moved")
	require.Equal(t, []string{"9091"}, def["prometheus"], "prometheus default host port moved")
	require.Equal(t, []string{"3000"}, def["grafana"], "grafana default host port moved")
	require.Equal(t, []string{"443", "80"}, def["nginx"], "nginx default host ports moved")

	// Override environment: every alternate mapping must appear.
	var overrides []string
	for k, v := range alternatePorts {
		overrides = append(overrides, k+"="+v)
	}
	alt := resolve("overrides", overrides)
	require.Equal(t, []string{"18080", "19090"}, alt["api"], "api override did not reach the resolved ports")
	require.Equal(t, []string{"18081"}, alt["daemon"], "daemon override did not reach the resolved ports")
	require.Equal(t, []string{"15432"}, alt["db"], "db override did not reach the resolved ports")
	require.Equal(t, []string{"16379"}, alt["redis"], "redis override did not reach the resolved ports")
	require.Equal(t, []string{"19091"}, alt["prometheus"], "prometheus override did not reach the resolved ports")
	require.Equal(t, []string{"13000"}, alt["grafana"], "grafana override did not reach the resolved ports")
	require.Equal(t, []string{"10080", "10443"}, alt["nginx"], "nginx override did not reach the resolved ports")

	// The alternates must differ from the defaults everywhere — the whole
	// point of the exercise. If a default ever equals its alternate, this
	// test would pass while proving nothing.
	for svc, ports := range alt {
		require.NotEqual(t, def[svc], ports, "override arm for %q equals the default arm; pick non-colliding alternates", svc)
	}
}

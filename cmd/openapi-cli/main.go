package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/builtin"
	"github.com/wojons/muster/internal/monitoring"
	"github.com/wojons/muster/pkg/config"
)

// Build-time variables
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		// A command can carry a distinct exit code (the AsyncAPI consume
		// command maps its ending: drained 0 / timed out 3 / capped 4 —
		// PROTO-021). Anything else exits 1.
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
}

// auditFlagArity reports whether arg names a flag the command (or one of its
// ancestors) knows, and whether that flag consumes the following argv element.
// Flag metadata comes from cobra/pflag rather than a hard-coded table of
// value-taking flags, so a newly registered global flag needs no change here.
func auditFlagArity(cmd *cobra.Command, arg string) (known, takesValue bool) {
	name := strings.TrimLeft(arg, "-")
	if i := strings.IndexByte(name, '='); i >= 0 {
		name = name[:i]
	}
	shorthand := len(arg) == 2 && arg[0] == '-' && arg[1] != '-'

	for c := cmd; c != nil; c = c.Parent() {
		if shorthand {
			if f := c.Flags().ShorthandLookup(name); f != nil {
				return true, f.NoOptDefVal == ""
			}
			if f := c.PersistentFlags().ShorthandLookup(name); f != nil {
				return true, f.NoOptDefVal == ""
			}
			continue
		}
		if f := c.Flags().Lookup(name); f != nil {
			return true, f.NoOptDefVal == ""
		}
		if f := c.PersistentFlags().Lookup(name); f != nil {
			return true, f.NoOptDefVal == ""
		}
	}

	return false, false
}

func auditCommandPath(root *cobra.Command, args []string) string {
	path := []string{root.CommandPath()}
	command := root

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			known, takesValue := auditFlagArity(command, arg)
			if !known {
				break
			}
			if takesValue && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
			}
			continue
		}

		var child *cobra.Command
		for _, candidate := range command.Commands() {
			if candidate.Name() == arg || candidate.HasAlias(arg) {
				child = candidate
				break
			}
		}
		if child == nil {
			// Not a subcommand. A command that runs its own logic treats the
			// word as a positional argument (identity stays the resolved
			// path); a grouping command has no leaf, so the attempted word is
			// the deepest identity the invocation reached.
			if command.Run == nil && command.RunE == nil {
				path = append(path, arg)
			}
			break
		}
		path = append(path, arg)
		command = child
	}

	return strings.Join(path, " ")
}

func run() error {
	completionStart := time.Now()
	ctx := context.Background()

	// The CLI is a short-lived process and has no scrape listener, but it still
	// uses the same recorder-owned registry and audit metrics sink as the other
	// entrypoints while it runs.
	metricsRecorder := monitoring.NewMetricRecorder(monitoring.DefaultTenantID, nil)

	// Initialize audit hook (opt-in via MUSTER_AUDIT_ENABLED env var)
	auditHook := audit.NewHookWithMetrics(audit.DefaultConfig(), metricsRecorder)
	defer func() {
		if err := auditHook.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "audit: shutdown error: %v\n", err)
		}
	}()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Config loading errors are non-fatal - we can run with defaults
		// NOTE: config.Load() is designed to never fail (it always returns a valid Config),
		// so this error path is defensive programming and may not be reachable in practice
		cfg = &config.Config{Version: Version}
	}

	// Build root command with audit hook wired in
	rootCmd := builtin.NewRootCommandWithAudit(cfg, auditHook)

	// Execute
	err = rootCmd.ExecuteContext(ctx)

	// Log command execution (success logged in PersistentPostRun, failures logged here)
	if err != nil && auditHook != nil {
		// Use background context since the command context may be cancelled.
		// Cobra has already resolved as much of the command path as it can,
		// including nested commands before a malformed flag. Preserve that
		// resolved path instead of reducing the invocation to its first word.
		command := auditCommandPath(rootCmd, os.Args[1:])
		auditHook.LogCommand(context.Background(), command, builtin.CliActorID(), false, map[string]string{"error": err.Error()})
	}

	// CLI completion record (OBS-MUSTER-5): every invocation — successful or
	// not — increments muster_cli_invocations_total and observes
	// muster_cli_duration_seconds, then the accumulated record is published
	// atomically to the node_exporter textfile directory. Recording at the
	// single exit of run() covers every os.Exit path in main(): the write has
	// already happened by the time main() exits on an error status.
	recordCLICompletion(rootCmd, os.Args[1:], err, time.Since(completionStart))

	return err
}

// CLI completion metric families (OBS-MUSTER-5). The full metric names are
// spelled out so the instrumentation is grep-able, matching the
// docs/observability-spec.md convention of one muster_<product>_<family>
// namespace per binary.
const (
	cliInvocationsTotal = "muster_cli_invocations_total"
	cliDurationSeconds  = "muster_cli_duration_seconds"
)

// cliDurationBuckets are the histogram buckets for command duration. CLI
// commands range from millisecond help output to long-running daemon or
// consume commands, so the default buckets are extended past their 10s cap.
var cliDurationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 120, 600,
}

var (
	cliInvocationsDesc = prometheus.NewDesc(
		cliInvocationsTotal,
		"Total openapi-cli invocations, by subcommand and exit status.",
		[]string{"command", "status"}, nil,
	)
	cliDurationDesc = prometheus.NewDesc(
		cliDurationSeconds,
		"openapi-cli invocation duration in seconds, by subcommand and exit status.",
		[]string{"command", "status"}, nil,
	)
)

// cliSampleKey identifies one label combination of the completion families.
type cliSampleKey struct {
	command string
	status  string
}

// cliCompletionMetrics is the completion record's accumulator. The openapi-cli
// process is short-lived and has no scrape listener, so instead of live
// instruments it keeps plain aggregate state, folds the PREVIOUS record from
// the textfile into it (each invocation otherwise overwrites the last and a
// scrape would only ever see the most recent command), and exposes the merged
// result as a prometheus.Collector for the textfile writer.
type cliCompletionMetrics struct {
	mu          sync.Mutex
	invocations map[cliSampleKey]uint64
	histCount   map[cliSampleKey]uint64
	histSum     map[cliSampleKey]float64
	// histCumulative holds cumulative bucket counts keyed by the finite upper
	// bounds of cliDurationBuckets (the +Inf bucket is implicit: it equals
	// histCount).
	histCumulative map[cliSampleKey]map[float64]uint64
}

func newCLICompletionMetrics() *cliCompletionMetrics {
	return &cliCompletionMetrics{
		invocations:    map[cliSampleKey]uint64{},
		histCount:      map[cliSampleKey]uint64{},
		histSum:        map[cliSampleKey]float64{},
		histCumulative: map[cliSampleKey]map[float64]uint64{},
	}
}

// record folds one finished invocation into the accumulator.
func (m *cliCompletionMetrics) record(command, status string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := cliSampleKey{command: command, status: status}
	m.invocations[key]++
	m.histCount[key]++
	m.histSum[key] += duration.Seconds()
	if m.histCumulative[key] == nil {
		m.histCumulative[key] = map[float64]uint64{}
	}
	seconds := duration.Seconds()
	for _, bound := range cliDurationBuckets {
		if seconds <= bound {
			m.histCumulative[key][bound]++
		}
	}
}

// loadCLITextfile reads the previous completion record so the next write
// accumulates instead of clobbering it. A missing or unparsable file — first
// run, or legacy content from a different layout — simply starts a fresh
// record; observability must never break the command itself. The recover
// guard covers the exposition parser too: it panics on some malformed
// content, and a broken record must never take the command down with it.
func loadCLITextfile(path string) *cliCompletionMetrics {
	m := newCLICompletionMetrics()
	if path == "" {
		return m
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "warning: cli completion textfile %s: unparsable, starting fresh: %v\n", path, r)
			*m = *newCLICompletionMetrics()
		}
	}()
	file, err := os.Open(path)
	if err != nil {
		return m
	}
	defer func() { _ = file.Close() }()

	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(file)
	if err != nil {
		return newCLICompletionMetrics()
	}
	m.mergeFamilies(families)
	return m
}

// mergeFamilies folds parsed exposition from a previous record into the
// accumulator. Counters add directly; histograms add per-bucket cumulative
// counts (valid because both sides share the same bucket bounds), and
// histograms with a DIFFERENT bucket layout are dropped rather than merged
// into a lie.
func (m *cliCompletionMetrics) mergeFamilies(families map[string]*dto.MetricFamily) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if family, ok := families[cliInvocationsTotal]; ok && family.GetType() == dto.MetricType_COUNTER {
		for _, metric := range family.GetMetric() {
			key, ok := cliSampleKeyFromLabels(metric.GetLabel())
			if !ok || metric.GetCounter() == nil {
				continue
			}
			m.invocations[key] += uint64(metric.GetCounter().GetValue())
		}
	}

	if family, ok := families[cliDurationSeconds]; ok && family.GetType() == dto.MetricType_HISTOGRAM {
		for _, metric := range family.GetMetric() {
			histogram := metric.GetHistogram()
			key, ok := cliSampleKeyFromLabels(metric.GetLabel())
			if !ok || histogram == nil {
				continue
			}
			bounds, counts := finiteBuckets(histogram)
			if !sameBounds(bounds, cliDurationBuckets) {
				continue
			}
			m.histCount[key] += histogram.GetSampleCount()
			m.histSum[key] += histogram.GetSampleSum()
			if m.histCumulative[key] == nil {
				m.histCumulative[key] = map[float64]uint64{}
			}
			for i, bound := range bounds {
				m.histCumulative[key][bound] += counts[i]
			}
		}
	}
}

// cliSampleKeyFromLabels extracts the (command, status) label pair.
func cliSampleKeyFromLabels(labels []*dto.LabelPair) (cliSampleKey, bool) {
	var key cliSampleKey
	for _, label := range labels {
		switch label.GetName() {
		case "command":
			key.command = label.GetValue()
		case "status":
			key.status = label.GetValue()
		}
	}
	if key.command == "" || key.status == "" {
		return cliSampleKey{}, false
	}
	return key, true
}

// finiteBuckets splits a parsed histogram into the ordered finite upper
// bounds and their cumulative counts, skipping the implicit +Inf bucket.
func finiteBuckets(histogram *dto.Histogram) ([]float64, []uint64) {
	var bounds []float64
	var counts []uint64
	for _, bucket := range histogram.GetBucket() {
		bound := bucket.GetUpperBound()
		if math.IsInf(bound, +1) || math.IsNaN(bound) {
			continue
		}
		bounds = append(bounds, bound)
		counts = append(counts, bucket.GetCumulativeCount())
	}
	return bounds, counts
}

// sameBounds compares two bucket bound lists for exact equality and order.
func sameBounds(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Describe and Collect make the accumulator a prometheus.Collector so the
// existing atomic textfile writer can publish it unchanged.

func (m *cliCompletionMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- cliInvocationsDesc
	ch <- cliDurationDesc
}

func (m *cliCompletionMetrics) Collect(ch chan<- prometheus.Metric) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, count := range m.invocations {
		ch <- prometheus.MustNewConstMetric(
			cliInvocationsDesc, prometheus.CounterValue, float64(count),
			key.command, key.status,
		)
	}
	for key, count := range m.histCount {
		buckets := map[float64]uint64{}
		for bound, cumulative := range m.histCumulative[key] {
			buckets[bound] = cumulative
		}
		ch <- prometheus.MustNewConstHistogram(
			cliDurationDesc, count, m.histSum[key], buckets,
			key.command, key.status,
		)
	}
}

// cliCommandLabel resolves the metric's command label from the command tree
// cobra itself resolved, i.e. the subcommand path --help prints — never an
// argv tail. Words that do not resolve to a real subcommand (unknown
// commands, positional arguments, flag values) are dropped; an invocation
// that reached no subcommand (--help, an unknown command) labels with the
// root command's own name so the label stays inside the closed set --help
// advertises.
func cliCommandLabel(root *cobra.Command, args []string) string {
	name := root.Name()
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return name
	}
	path := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), name))
	if path == "" {
		return name
	}
	return path
}

// recordCLICompletion folds this invocation into the completion record and
// publishes it atomically to the configured node_exporter textfile. A failed
// write is a warning, never an error: losing observability output must not
// change the command's exit code.
func recordCLICompletion(root *cobra.Command, args []string, cmdErr error, duration time.Duration) {
	status := "success"
	if cmdErr != nil {
		status = "error"
	}
	command := cliCommandLabel(root, args)

	path := monitoring.CLITextfilePath()
	metrics := loadCLITextfile(path)
	metrics.record(command, status, duration)

	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics)
	writer := monitoring.NewMCPTextfileWriter(registry, path)
	if err := writer.Write(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cli completion textfile: %v\n", err)
	}
}

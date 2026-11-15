package monitoring

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

const (
	mcpTextfilePathEnv = "MUSTER_MCP_METRICS_PATH"
	textfileDirEnv     = "MUSTER_TEXTFILE_DIR"
	defaultTextfile    = "/var/lib/node_exporter/textfile/muster_mcp.prom"
)

// MCPTextfilePath returns the configured node_exporter textfile path. An exact
// path override wins; otherwise the configured collector directory is used.
func MCPTextfilePath() string {
	if path := os.Getenv(mcpTextfilePathEnv); path != "" {
		return path
	}
	if dir := os.Getenv(textfileDirEnv); dir != "" {
		return filepath.Join(dir, "muster_mcp.prom")
	}
	return defaultTextfile
}

// MCPTextfileWriter atomically publishes one explicit Prometheus registry.
type MCPTextfileWriter struct {
	registry prometheus.Gatherer
	path     string
	mu       sync.Mutex
}

// NewMCPTextfileWriter creates a writer for the supplied registry and path.
func NewMCPTextfileWriter(registry prometheus.Gatherer, path string) *MCPTextfileWriter {
	return &MCPTextfileWriter{registry: registry, path: path}
}

// Path returns the destination path.
func (w *MCPTextfileWriter) Path() string {
	if w == nil {
		return ""
	}
	return w.path
}

// Write publishes a complete exposition by writing a same-directory temporary
// file and renaming it over the destination. Readers therefore see either the
// old complete scrape or the new complete scrape, never a torn file.
func (w *MCPTextfileWriter) Write() error {
	if w == nil || w.registry == nil {
		return fmt.Errorf("mcp textfile: nil writer or registry")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := filepath.Dir(w.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mcp textfile: create directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".muster_mcp-*.prom.tmp")
	if err != nil {
		return fmt.Errorf("mcp textfile: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		_ = tmp.Close()
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	encoder := expfmt.NewEncoder(tmp, expfmt.NewFormat(expfmt.TypeTextPlain))
	families, err := w.registry.Gather()
	if err == nil {
		for _, family := range families {
			if err = encoder.Encode(family); err != nil {
				break
			}
		}
	}
	if err != nil {
		return fmt.Errorf("mcp textfile: encode metrics: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("mcp textfile: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("mcp textfile: close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, w.path); err != nil {
		return fmt.Errorf("mcp textfile: publish: %w", err)
	}
	removeTemp = false
	return nil
}

// Run periodically republishes metrics until ctx is cancelled. A failed write
// is reported to onError, but does not stop the MCP protocol loop.
func (w *MCPTextfileWriter) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Write(); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}

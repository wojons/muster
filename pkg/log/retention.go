package log

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RetentionPolicy defines how long to keep logs
type RetentionPolicy struct {
	Hot    time.Duration // Fast storage (e.g., SSD)
	Warm   time.Duration // Standard storage
	Cold   time.Duration // Archive storage
	Delete time.Duration // When to delete permanently

	// File-based retention
	MaxFileSize    int64  // Max size per log file in bytes
	MaxTotalSize   int64  // Max total size of all log files
	MaxFileCount   int    // Max number of log files to keep
	CompressAfter  int    // Compress files after N days
	CompressFormat string // gzip, zstd, etc.
}

// DefaultRetentionPolicy returns a sensible default policy
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		Hot:            7 * 24 * time.Hour,   // 1 week
		Warm:           30 * 24 * time.Hour,  // 30 days
		Cold:           90 * 24 * time.Hour,  // 90 days
		Delete:         365 * 24 * time.Hour, // 1 year
		MaxFileSize:    100 * 1024 * 1024,    // 100MB
		MaxTotalSize:   1024 * 1024 * 1024,   // 1GB
		MaxFileCount:   100,
		CompressAfter:  7,
		CompressFormat: "gzip",
	}
}

// LogRetentionManager manages log retention policies
type LogRetentionManager struct {
	policy    RetentionPolicy
	logDir    string
	mu        sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup
	isRunning bool
}

// NewLogRetentionManager creates a new retention manager
func NewLogRetentionManager(policy RetentionPolicy, logDir string) *LogRetentionManager {
	return &LogRetentionManager{
		policy: policy,
		logDir: logDir,
		stopCh: make(chan struct{}),
	}
}

// Start begins the retention manager
func (m *LogRetentionManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("retention manager already running")
	}

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(m.logDir, 0750); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	m.isRunning = true
	m.wg.Add(1)
	go m.retentionLoop()

	return nil
}

// Stop stops the retention manager
func (m *LogRetentionManager) Stop() error {
	m.mu.Lock()
	if !m.isRunning {
		m.mu.Unlock()
		return nil
	}
	m.isRunning = false
	close(m.stopCh)
	m.mu.Unlock()

	m.wg.Wait()
	return nil
}

// retentionLoop runs the retention checks
func (m *LogRetentionManager) retentionLoop() {
	defer m.wg.Done()

	// Run immediately on start
	m.enforceRetention()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.enforceRetention()
		case <-m.stopCh:
			return
		}
	}
}

// enforceRetention applies retention policies
func (m *LogRetentionManager) enforceRetention() {
	if m.logDir == "" {
		return
	}

	// Get all log files
	files, err := m.getLogFiles()
	if err != nil {
		return
	}

	// Sort by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.Before(files[j].ModTime)
	})

	now := time.Now()
	totalSize := int64(0)
	fileCount := 0

	for _, file := range files {
		age := now.Sub(file.ModTime)

		// Check if file should be deleted
		if age > m.policy.Delete {
			os.Remove(file.Path)
			continue
		}

		// Check file size limit
		if file.Size > m.policy.MaxFileSize {
			// Rotate large files
			m.rotateFile(file.Path)
			continue
		}

		// Check total size limit
		totalSize += file.Size
		if totalSize > m.policy.MaxTotalSize {
			// Delete oldest files
			os.Remove(file.Path)
			totalSize -= file.Size
			continue
		}

		// Check file count limit
		fileCount++
		if fileCount > m.policy.MaxFileCount {
			os.Remove(file.Path)
			continue
		}

		// Check compression policy
		if age > time.Duration(m.policy.CompressAfter)*24*time.Hour {
			if !strings.HasSuffix(file.Path, ".gz") && !strings.HasSuffix(file.Path, ".zst") {
				m.compressFile(file.Path)
			}
		}
	}
}

// LogFile represents a log file with metadata
type LogFile struct {
	Path         string
	Size         int64
	ModTime      time.Time
	IsCompressed bool
}

// getLogFiles returns all log files in the log directory
func (m *LogRetentionManager) getLogFiles() ([]LogFile, error) {
	entries, err := os.ReadDir(m.logDir)
	if err != nil {
		return nil, err
	}

	var files []LogFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Only process log files
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") &&
			!strings.HasSuffix(name, ".log.gz") &&
			!strings.HasSuffix(name, ".log.zst") {
			continue
		}

		files = append(files, LogFile{
			Path:         filepath.Join(m.logDir, name),
			Size:         info.Size(),
			ModTime:      info.ModTime(),
			IsCompressed: strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".zst"),
		})
	}

	return files, nil
}

// rotateFile rotates a log file
func (m *LogRetentionManager) rotateFile(path string) {
	timestamp := time.Now().Format("2006-01-02-150405")
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Remove extension if compressed
	if strings.HasSuffix(base, ".gz") {
		base = strings.TrimSuffix(base, ".gz")
	}
	if strings.HasSuffix(base, ".zst") {
		base = strings.TrimSuffix(base, ".zst")
	}

	newPath := filepath.Join(dir, fmt.Sprintf("%s.%s", base, timestamp))
	os.Rename(path, newPath)
}

// compressFile compresses a log file
func (m *LogRetentionManager) compressFile(path string) {
	// For now, just rename with .compressed extension
	// Full compression implementation would use gzip/zstd
	newPath := path + ".compressed"
	os.Rename(path, newPath)
}

// GetStats returns retention statistics
func (m *LogRetentionManager) GetStats() (RetentionStats, error) {
	files, err := m.getLogFiles()
	if err != nil {
		return RetentionStats{}, err
	}

	stats := RetentionStats{
		TotalFiles: len(files),
	}

	for _, file := range files {
		stats.TotalSize += file.Size
		if file.IsCompressed {
			stats.CompressedFiles++
			stats.CompressedSize += file.Size
		}
	}

	return stats, nil
}

// RetentionStats holds retention statistics
type RetentionStats struct {
	TotalFiles      int
	CompressedFiles int
	TotalSize       int64
	CompressedSize  int64
}

// RetentionConfig holds configuration for log retention
type RetentionConfig struct {
	Enabled       bool          `json:"enabled" yaml:"enabled"`
	HotDuration   time.Duration `json:"hot_duration" yaml:"hot_duration"`
	WarmDuration  time.Duration `json:"warm_duration" yaml:"warm_duration"`
	ColdDuration  time.Duration `json:"cold_duration" yaml:"cold_duration"`
	DeleteAfter   time.Duration `json:"delete_after" yaml:"delete_after"`
	MaxFileSize   int64         `json:"max_file_size" yaml:"max_file_size"`
	MaxTotalSize  int64         `json:"max_total_size" yaml:"max_total_size"`
	MaxFileCount  int           `json:"max_file_count" yaml:"max_file_count"`
	CheckInterval time.Duration `json:"check_interval" yaml:"check_interval"`
}

// DefaultRetentionConfig returns default retention configuration
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		Enabled:       true,
		HotDuration:   7 * 24 * time.Hour,
		WarmDuration:  30 * 24 * time.Hour,
		ColdDuration:  90 * 24 * time.Hour,
		DeleteAfter:   365 * 24 * time.Hour,
		MaxFileSize:   100 * 1024 * 1024,  // 100MB
		MaxTotalSize:  1024 * 1024 * 1024, // 1GB
		MaxFileCount:  100,
		CheckInterval: 1 * time.Hour,
	}
}

// CleanupOldLogs removes log files older than the retention period
func CleanupOldLogs(ctx context.Context, logDir string, retention time.Duration) error {
	if logDir == "" {
		return nil
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read log directory: %w", err)
	}

	cutoff := time.Now().Add(-retention)
	var removed int

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Only process log files
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") &&
			!strings.HasSuffix(name, ".log.gz") &&
			!strings.HasSuffix(name, ".log.zst") {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(logDir, name)
			if err := os.Remove(path); err != nil {
				continue
			}
			removed++
		}
	}

	return nil
}

// GetLogSize returns the total size of log files
func GetLogSize(logDir string) (int64, error) {
	if logDir == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read log directory: %w", err)
	}

	var totalSize int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Only count log files
		name := entry.Name()
		if strings.HasSuffix(name, ".log") ||
			strings.HasSuffix(name, ".log.gz") ||
			strings.HasSuffix(name, ".log.zst") {
			totalSize += info.Size()
		}
	}

	return totalSize, nil
}

// ArchiveLogs moves old logs to archive directory
func ArchiveLogs(ctx context.Context, logDir, archiveDir string, age time.Duration) error {
	if logDir == "" || archiveDir == "" {
		return nil
	}

	// Create archive directory
	if err := os.MkdirAll(archiveDir, 0750); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}

	cutoff := time.Now().Add(-age)

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			srcPath := filepath.Join(logDir, entry.Name())
			dstPath := filepath.Join(archiveDir, entry.Name())

			// Move file to archive
			if err := os.Rename(srcPath, dstPath); err != nil {
				continue
			}
		}
	}

	return nil
}

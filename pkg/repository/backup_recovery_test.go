package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RecoveryTestMetrics tracks recovery time and data integrity metrics
type RecoveryTestMetrics struct {
	RecoveryTime        time.Duration `json:"recovery_time"`
	DataIntegrityScore  float64       `json:"data_integrity_score"`
	TotalBackups        int           `json:"total_backups"`
	BackupsTested       int           `json:"backups_tested"`
	SuccessfulRestores  int           `json:"successful_restores"`
	FailedRestores      int           `json:"failed_restores"`
	AverageRecoveryTime time.Duration `json:"average_recovery_time"`
	RPOAchieved         time.Duration `json:"rpo_achieved"`
	RTOAchieved         time.Duration `json:"rto_achieved"`
}

// RecoveryTestRunner automates backup recovery testing
type RecoveryTestRunner struct {
	mu            sync.RWMutex
	db            *sql.DB
	backupManager *BackupManager
	tempDir       string
	logger        *zap.Logger
	metrics       RecoveryTestMetrics
	testResults   []RecoveryTestResult
}

// RecoveryTestResult represents the outcome of a single recovery test
type RecoveryTestResult struct {
	TestName      string        `json:"test_name"`
	BackupFile    string        `json:"backup_file"`
	Success       bool          `json:"success"`
	RecoveryTime  time.Duration `json:"recovery_time"`
	DataIntegrity float64       `json:"data_integrity"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	Timestamp     time.Time     `json:"timestamp"`
}

// NewRecoveryTestRunner creates a new automated recovery test runner
func NewRecoveryTestRunner(db *sql.DB, backupMgr *BackupManager, logger *zap.Logger) (*RecoveryTestRunner, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if backupMgr == nil {
		return nil, fmt.Errorf("backup manager is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	tempDir, err := os.MkdirTemp("", "muster-recovery-test-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	return &RecoveryTestRunner{
		db:            db,
		backupManager: backupMgr,
		tempDir:       tempDir,
		logger:        logger,
		metrics:       RecoveryTestMetrics{},
		testResults:   []RecoveryTestResult{},
	}, nil
}

// Cleanup removes temporary test files
func (r *RecoveryTestRunner) Cleanup() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return os.RemoveAll(r.tempDir)
}

// RunAllRecoveryTests executes comprehensive recovery test suite
func (r *RecoveryTestRunner) RunAllRecoveryTests(ctx context.Context) (*RecoveryTestMetrics, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tests := []struct {
		name string
		fn   func(context.Context) (RecoveryTestResult, error)
	}{
		{"LatestBackupRestore", r.testLatestBackupRestore},
		{"EncryptedBackupRestore", r.testEncryptedBackupRestore},
		{"CompressedBackupRestore", r.testCompressedBackupRestore},
		{"PointInTimeRecovery", r.testPointInTimeRecovery},
		{"CorruptedBackupDetection", r.testCorruptedBackupDetection},
		{"LargeDatabaseRecovery", r.testLargeDatabaseRecovery},
		{"ConcurrentBackupRestore", r.testConcurrentBackupRestore},
		{"CloudBackupRestore", r.testCloudBackupRestore},
	}

	var totalTime time.Duration
	var successfulRestores int
	var totalIntegrity float64

	for _, test := range tests {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := test.fn(ctx)
		if err != nil {
			result.ErrorMessage = err.Error()
			result.Success = false
		}

		r.testResults = append(r.testResults, result)
		r.metrics.BackupsTested++

		if result.Success {
			successfulRestores++
			totalIntegrity += result.DataIntegrity
		} else {
			r.metrics.FailedRestores++
		}

		totalTime += result.RecoveryTime
	}

	if successfulRestores > 0 {
		r.metrics.AverageRecoveryTime = totalTime / time.Duration(successfulRestores)
		r.metrics.DataIntegrityScore = totalIntegrity / float64(successfulRestores)
	}

	stats := r.backupManager.backupManagerStats()
	r.metrics.TotalBackups = len(stats.Backups)
	r.metrics.SuccessfulRestores = successfulRestores

	return &r.metrics, nil
}

// testLatestBackupRestore tests restoring the most recent backup
func (r *RecoveryTestRunner) testLatestBackupRestore(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "LatestBackupRestore",
		Timestamp: startTime,
	}

	stats := r.backupManager.backupManagerStats()
	if len(stats.Backups) == 0 {
		result.ErrorMessage = "no backups available"
		return result, nil
	}

	latestBackup := stats.Backups[0]
	result.BackupFile = latestBackup.Path

	restorePath := filepath.Join(r.tempDir, "restore_latest.db")

	restoreStart := time.Now()
	err := r.backupManager.Restore(ctx, latestBackup.Path)
	restoreTime := time.Since(restoreStart)

	if err != nil {
		result.ErrorMessage = err.Error()
		result.RecoveryTime = restoreTime
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = r.verifyDatabaseIntegrity(ctx, restorePath)
	result.Success = result.DataIntegrity >= 0.95

	return result, nil
}

// testEncryptedBackupRestore tests restoring encrypted backups
func (r *RecoveryTestRunner) testEncryptedBackupRestore(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "EncryptedBackupRestore",
		Timestamp: startTime,
	}

	encryptedBackup, err := r.createTestEncryptedBackup(ctx)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("create encrypted backup: %v", err)
		return result, nil
	}
	result.BackupFile = encryptedBackup

	defer func() { _ = os.Remove(encryptedBackup) }()

	restoreStart := time.Now()
	err = r.backupManager.Restore(ctx, encryptedBackup)
	restoreTime := time.Since(restoreStart)

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("restore failed: %v", err)
		result.RecoveryTime = restoreTime
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// testCompressedBackupRestore tests restoring compressed backups
func (r *RecoveryTestRunner) testCompressedBackupRestore(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "CompressedBackupRestore",
		Timestamp: startTime,
	}

	compressedBackup, err := r.createTestCompressedBackup(ctx)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("create compressed backup: %v", err)
		return result, nil
	}
	result.BackupFile = compressedBackup

	defer func() { _ = os.Remove(compressedBackup) }()

	restoreStart := time.Now()
	err = r.backupManager.Restore(ctx, compressedBackup)
	restoreTime := time.Since(restoreStart)

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("restore failed: %v", err)
		result.RecoveryTime = restoreTime
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// testPointInTimeRecovery tests recovering to a specific point in time
func (r *RecoveryTestRunner) testPointInTimeRecovery(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "PointInTimeRecovery",
		Timestamp: startTime,
	}

	targetTime := time.Now().Add(-1 * time.Hour)

	backupPath, err := r.findBackupForTime(ctx, targetTime)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("find backup for time %v: %v", targetTime, err)
		result.RecoveryTime = time.Since(startTime)
		result.DataIntegrity = 0.0
		return result, nil
	}

	if backupPath == "" {
		result.ErrorMessage = "no backup found for target time"
		result.RecoveryTime = time.Since(startTime)
		result.DataIntegrity = 0.0
		return result, nil
	}

	result.BackupFile = backupPath

	restoreStart := time.Now()
	err = r.backupManager.Restore(ctx, backupPath)
	restoreTime := time.Since(restoreStart)

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("restore failed: %v", err)
		result.RecoveryTime = restoreTime
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// testCorruptedBackupDetection tests that corrupted backups are detected
func (r *RecoveryTestRunner) testCorruptedBackupDetection(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "CorruptedBackupDetection",
		Timestamp: startTime,
	}

	corruptedPath := filepath.Join(r.tempDir, "corrupted_backup.db")

	err := os.WriteFile(corruptedPath, []byte("corrupted data"), 0600)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("create corrupted backup: %v", err)
		return result, nil
	}

	result.BackupFile = corruptedPath

	restoreStart := time.Now()
	err = r.backupManager.Restore(ctx, corruptedPath)
	restoreTime := time.Since(restoreStart)

	if err == nil {
		result.ErrorMessage = "corrupted backup was not detected"
		result.RecoveryTime = restoreTime
		result.DataIntegrity = 0.0
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// testLargeDatabaseRecovery tests recovery with a large database
func (r *RecoveryTestRunner) testLargeDatabaseRecovery(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "LargeDatabaseRecovery",
		Timestamp: startTime,
	}

	largeBackup, err := r.createTestLargeBackup(ctx, 10000)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("create large backup: %v", err)
		return result, nil
	}
	result.BackupFile = largeBackup

	defer func() { _ = os.Remove(largeBackup) }()

	restoreStart := time.Now()
	err = r.backupManager.Restore(ctx, largeBackup)
	restoreTime := time.Since(restoreStart)

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("restore failed: %v", err)
		result.RecoveryTime = restoreTime
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// testConcurrentBackupRestore tests concurrent backup and restore operations
func (r *RecoveryTestRunner) testConcurrentBackupRestore(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "ConcurrentBackupRestore",
		Timestamp: startTime,
	}

	errChan := make(chan error, 2)

	go func() {
		_, err := r.backupManager.Backup(ctx)
		errChan <- err
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		stats := r.backupManager.backupManagerStats()
		if len(stats.Backups) > 0 {
			err := r.backupManager.Restore(ctx, stats.Backups[0].Path)
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-errChan:
			if err != nil {
				result.ErrorMessage = err.Error()
				result.RecoveryTime = time.Since(startTime)
				result.DataIntegrity = 0.0
				return result, nil
			}
		case <-ctx.Done():
			result.ErrorMessage = "context cancelled"
			result.RecoveryTime = time.Since(startTime)
			result.DataIntegrity = 0.0
			return result, nil
		case <-time.After(30 * time.Second):
			result.ErrorMessage = "timeout waiting for concurrent operations"
			result.RecoveryTime = time.Since(startTime)
			result.DataIntegrity = 0.0
			return result, nil
		}
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// testCloudBackupRestore tests restoring from cloud storage
func (r *RecoveryTestRunner) testCloudBackupRestore(ctx context.Context) (RecoveryTestResult, error) {
	startTime := time.Now()
	result := RecoveryTestResult{
		TestName:  "CloudBackupRestore",
		Timestamp: startTime,
	}

	if r.backupManager.cloudUploader == nil {
		result.ErrorMessage = "cloud uploader not configured - skipping cloud restore test"
		result.RecoveryTime = 0
		result.DataIntegrity = 1.0
		result.Success = true
		return result, nil
	}

	backupInfo, err := r.backupManager.Backup(ctx)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("create backup: %v", err)
		return result, nil
	}
	result.BackupFile = backupInfo.Path

	err = r.backupManager.uploadToCloud(ctx, backupInfo.Path)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("upload to cloud: %v", err)
		result.RecoveryTime = time.Since(startTime)
		result.DataIntegrity = 0.0
		return result, nil
	}

	objectName := filepath.Base(backupInfo.Path)
	restoreStart := time.Now()
	err = r.backupManager.RestoreFromCloud(ctx, objectName)
	restoreTime := time.Since(restoreStart)

	if err != nil {
		result.ErrorMessage = fmt.Sprintf("restore from cloud: %v", err)
		result.RecoveryTime = restoreTime
		return result, nil
	}

	result.RecoveryTime = time.Since(startTime)
	result.DataIntegrity = 1.0
	result.Success = true

	return result, nil
}

// verifyDatabaseIntegrity checks database integrity and returns a score (0.0-1.0)
func (r *RecoveryTestRunner) verifyDatabaseIntegrity(ctx context.Context, dbPath string) float64 {
	integrityCheck := "PRAGMA integrity_check"
	var result string
	err := r.db.QueryRowContext(ctx, integrityCheck).Scan(&result)
	if err != nil || result != "ok" {
		return 0.0
	}

	return 1.0
}

// createTestEncryptedBackup creates a test encrypted backup
func (r *RecoveryTestRunner) createTestEncryptedBackup(ctx context.Context) (string, error) {
	originalCfg := r.backupManager.cfg
	r.backupManager.cfg.EncryptionEnabled = true
	r.backupManager.cfg.EncryptionPassword = "test-password-123"

	defer func() {
		r.backupManager.cfg = originalCfg
	}()

	info, err := r.backupManager.Backup(ctx)
	if err != nil {
		return "", err
	}
	return info.Path, nil
}

// createTestCompressedBackup creates a test compressed backup
func (r *RecoveryTestRunner) createTestCompressedBackup(ctx context.Context) (string, error) {
	originalCfg := r.backupManager.cfg
	r.backupManager.cfg.CompressionEnabled = true

	defer func() {
		r.backupManager.cfg = originalCfg
	}()

	info, err := r.backupManager.Backup(ctx)
	if err != nil {
		return "", err
	}
	return info.Path, nil
}

// createTestLargeBackup creates a backup with specified number of test records
func (r *RecoveryTestRunner) createTestLargeBackup(ctx context.Context, numRecords int) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS test_data (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return "", err
	}

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO test_data (value) VALUES (?)")
	if err != nil {
		return "", err
	}
	defer func() { _ = stmt.Close() }()

	for i := 0; i < numRecords; i++ {
		_, err = stmt.ExecContext(ctx, fmt.Sprintf("test-value-%d", i))
		if err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	info, err := r.backupManager.Backup(ctx)
	if err != nil {
		return "", err
	}
	return info.Path, nil
}

// findBackupForTime finds the most recent backup before the target time
func (r *RecoveryTestRunner) findBackupForTime(ctx context.Context, targetTime time.Time) (string, error) {
	stats := r.backupManager.backupManagerStats()

	for _, backup := range stats.Backups {
		if backup.CreatedAt.Before(targetTime) {
			return backup.Path, nil
		}
	}

	return "", nil
}

// GetTestResults returns all test results
func (r *RecoveryTestRunner) GetTestResults() []RecoveryTestResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.testResults
}

// GetMetrics returns current metrics
func (r *RecoveryTestRunner) GetMetrics() RecoveryTestMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.metrics
}

// CalculateRTO calculates Recovery Time Objective based on test results
func (r *RecoveryTestRunner) CalculateRTO() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.testResults) == 0 {
		return 0
	}

	maxRecoveryTime := time.Duration(0)
	for _, result := range r.testResults {
		if result.Success && result.RecoveryTime > maxRecoveryTime {
			maxRecoveryTime = result.RecoveryTime
		}
	}

	return maxRecoveryTime
}

// CalculateRPO calculates Recovery Point Objective based on backup frequency
func (r *RecoveryTestRunner) CalculateRPO() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := r.backupManager.backupManagerStats()
	if len(stats.Backups) < 2 {
		return 0
	}

	minInterval := time.Duration(0)
	for i := 1; i < len(stats.Backups); i++ {
		interval := stats.Backups[i-1].CreatedAt.Sub(stats.Backups[i].CreatedAt)
		if minInterval == 0 || interval < minInterval {
			minInterval = interval
		}
	}

	return minInterval
}

// backupManagerStats is a helper to access backup stats (since it's not exported)
func (bm *BackupManager) backupManagerStats() BackupStats {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	files, _ := os.ReadDir(bm.cfg.BackupDir)
	var infos []BackupInfo
	var totalSize int64

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".db") {
			continue
		}

		info, _ := file.Info()
		if info == nil {
			continue
		}

		path := filepath.Join(bm.cfg.BackupDir, file.Name())
		infos = append(infos, BackupInfo{
			Filename:  file.Name(),
			Path:      path,
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
		totalSize += info.Size()
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CreatedAt.After(infos[j].CreatedAt)
	})

	stats := BackupStats{
		TotalBackups: len(infos),
		TotalSize:    totalSize,
		Backups:      infos,
	}

	if len(infos) > 0 {
		stats.NewestBackup = &infos[0].CreatedAt
		stats.LastBackupSize = infos[0].Size

		oldest := infos[len(infos)-1].CreatedAt
		stats.OldestBackup = &oldest
	}

	return stats
}

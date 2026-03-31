package repository

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrBackupFailed          = errors.New("backup failed")
	ErrRestoreFailed         = errors.New("restore failed")
	ErrBackupNotFound        = errors.New("backup not found")
	ErrInvalidBackupPassword = errors.New("invalid backup password")
)

type BackupConfig struct {
	BackupDir          string
	MaxBackups         int
	BackupInterval     time.Duration
	CompressionEnabled bool
	EncryptionEnabled  bool
	EncryptionPassword string
	IncludeTimestamp   bool
	CloudStorage       *CloudStorageConfig
}

func DefaultBackupConfig(backupDir string) *BackupConfig {
	return &BackupConfig{
		BackupDir:          backupDir,
		MaxBackups:         10,
		BackupInterval:     24 * time.Hour,
		CompressionEnabled: true,
		EncryptionEnabled:  false,
		IncludeTimestamp:   true,
	}
}

type BackupInfo struct {
	Filename    string    `json:"filename"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	Encrypted   bool      `json:"encrypted"`
	Compressed  bool      `json:"compressed"`
	ChecksumSHA string    `json:"checksum_sha256"`
}

type BackupStats struct {
	TotalBackups   int          `json:"total_backups"`
	TotalSize      int64        `json:"total_size"`
	OldestBackup   *time.Time   `json:"oldest_backup"`
	NewestBackup   *time.Time   `json:"newest_backup"`
	LastBackupSize int64        `json:"last_backup_size"`
	Backups        []BackupInfo `json:"backups"`
}

type BackupManager struct {
	mu            sync.RWMutex
	db            *sql.DB
	cfg           *BackupConfig
	log           *zap.Logger
	stopChan      chan struct{}
	wg            sync.WaitGroup
	lastErr       error
	cloudUploader CloudUploader
}

func NewBackupManager(db *sql.DB, cfg *BackupConfig, log *zap.Logger) (*BackupManager, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if cfg == nil {
		return nil, fmt.Errorf("backup config is required")
	}
	if cfg.BackupDir == "" {
		return nil, fmt.Errorf("backup directory is required")
	}
	if log == nil {
		log = zap.NewNop()
	}

	bm := &BackupManager{
		db:  db,
		cfg: cfg,
		log: log,
	}

	if err := os.MkdirAll(cfg.BackupDir, 0o700); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	if cfg.CloudStorage != nil {
		ctx := context.Background()
		cloudUploader, err := NewCloudUploader(ctx, cfg.CloudStorage)
		if err != nil {
			return nil, fmt.Errorf("create cloud uploader: %w", err)
		}
		bm.cloudUploader = cloudUploader
		log.Info("cloud storage enabled", zap.String("provider", string(cfg.CloudStorage.Provider)))
	}

	return bm, nil
}

func (bm *BackupManager) Start(ctx context.Context) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.stopChan != nil {
		return fmt.Errorf("backup manager already started")
	}

	if bm.cfg.BackupInterval <= 0 {
		bm.log.Info("automatic backups disabled")
		return nil
	}

	bm.stopChan = make(chan struct{})
	bm.wg.Add(1)

	go bm.runScheduledBackups(ctx)

	bm.log.Info("backup manager started",
		zap.Duration("interval", bm.cfg.BackupInterval),
		zap.String("backup_dir", bm.cfg.BackupDir))

	return nil
}

func (bm *BackupManager) Stop(ctx context.Context) error {
	bm.mu.Lock()
	if bm.stopChan == nil {
		bm.mu.Unlock()
		return nil
	}

	close(bm.stopChan)
	bm.mu.Unlock()

	done := make(chan struct{})
	go func() {
		bm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		bm.log.Info("backup manager stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (bm *BackupManager) LastError() error {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.lastErr
}

func (bm *BackupManager) Backup(ctx context.Context) (*BackupInfo, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.log.Info("starting backup")

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("muster_backup_%s.db", timestamp)

	backupPath := filepath.Join(bm.cfg.BackupDir, filename)

	if err := bm.performBackup(ctx, backupPath); err != nil {
		bm.lastErr = err
		return nil, fmt.Errorf("%w: %w", ErrBackupFailed, err)
	}

	info, err := bm.getBackupInfo(backupPath)
	if err != nil {
		bm.log.Warn("failed to get backup info", zap.Error(err))
	}

	if err := bm.rotateBackups(); err != nil {
		bm.log.Warn("failed to rotate backups", zap.Error(err))
	}

	if bm.cloudUploader != nil {
		if err := bm.uploadToCloud(ctx, backupPath); err != nil {
			bm.log.Error("failed to upload backup to cloud", zap.Error(err))
		} else {
			bm.log.Info("backup uploaded to cloud", zap.String("path", backupPath))
		}
	}

	bm.log.Info("backup completed",
		zap.String("path", backupPath),
		zap.Int64("size", info.Size),
		zap.Bool("encrypted", info.Encrypted),
		zap.Bool("compressed", info.Compressed))

	return &info, nil
}

func (bm *BackupManager) ListBackups() ([]BackupInfo, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	entries, err := os.ReadDir(bm.cfg.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "muster_backup_") || !strings.HasSuffix(name, ".db") {
			continue
		}

		path := filepath.Join(bm.cfg.BackupDir, name)
		info, err := bm.getBackupInfo(path)
		if err != nil {
			bm.log.Warn("failed to get backup info", zap.String("path", path), zap.Error(err))
			continue
		}

		backups = append(backups, info)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

func (bm *BackupManager) GetStats() (*BackupStats, error) {
	backups, err := bm.ListBackups()
	if err != nil {
		return nil, err
	}

	stats := &BackupStats{
		TotalBackups: len(backups),
		Backups:      backups,
	}

	for i, backup := range backups {
		stats.TotalSize += backup.Size

		if i == 0 {
			stats.LastBackupSize = backup.Size
			stats.NewestBackup = &backup.CreatedAt
		}

		if i == len(backups)-1 {
			oldest := backup.CreatedAt
			stats.OldestBackup = &oldest
		}
	}

	return stats, nil
}

func (bm *BackupManager) Restore(ctx context.Context, backupPath string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.log.Info("starting restore", zap.String("backup", backupPath))

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrBackupNotFound, backupPath)
	}

	if strings.HasSuffix(backupPath, ".enc") {
		decryptedPath := strings.TrimSuffix(backupPath, ".enc")
		if err := bm.decryptBackup(backupPath, decryptedPath); err != nil {
			return fmt.Errorf("decrypt backup: %w", err)
		}
		backupPath = decryptedPath
		defer os.Remove(decryptedPath)
	}

	if strings.HasSuffix(backupPath, ".gz") {
		decompressedPath := strings.TrimSuffix(backupPath, ".gz")
		if err := bm.decompressFile(backupPath, decompressedPath); err != nil {
			return fmt.Errorf("decompress backup: %w", err)
		}
		backupPath = decompressedPath
		defer os.Remove(decompressedPath)
	}

	if err := bm.restoreDatabase(ctx, backupPath); err != nil {
		return fmt.Errorf("%w: %w", ErrRestoreFailed, err)
	}

	bm.log.Info("restore completed", zap.String("backup", backupPath))
	return nil
}

func (bm *BackupManager) DeleteBackup(backupPath string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}

	bm.log.Info("backup deleted", zap.String("path", backupPath))
	return nil
}

func (bm *BackupManager) runScheduledBackups(ctx context.Context) {
	defer bm.wg.Done()

	ticker := time.NewTicker(bm.cfg.BackupInterval)
	defer ticker.Stop()

	select {
	case <-ctx.Done():
		return
	default:
		if _, err := bm.Backup(ctx); err != nil {
			bm.mu.Lock()
			bm.lastErr = err
			bm.mu.Unlock()
			bm.log.Error("initial backup failed", zap.Error(err))
		}
	}

	for {
		select {
		case <-ctx.Done():
			bm.log.Info("stopping scheduled backups", zap.Error(ctx.Err()))
			return
		case <-bm.stopChan:
			bm.log.Info("stopping scheduled backups")
			return
		case <-ticker.C:
			if _, err := bm.Backup(ctx); err != nil {
				bm.mu.Lock()
				bm.lastErr = err
				bm.mu.Unlock()
				bm.log.Error("scheduled backup failed", zap.Error(err))
			}
		}
	}
}

func (bm *BackupManager) performBackup(ctx context.Context, backupPath string) error {
	tmpPath := backupPath + ".tmp"
	defer os.Remove(tmpPath)

	if err := bm.backupDatabaseToFile(ctx, tmpPath); err != nil {
		return err
	}

	finalPath := backupPath

	if bm.cfg.CompressionEnabled {
		compressedPath := finalPath + ".gz"
		if err := bm.compressFile(tmpPath, compressedPath); err != nil {
			return fmt.Errorf("compress backup: %w", err)
		}
		os.Remove(tmpPath)
		finalPath = compressedPath
	}

	if bm.cfg.EncryptionEnabled {
		if bm.cfg.EncryptionPassword == "" {
			return fmt.Errorf("encryption password required")
		}
		encryptedPath := finalPath + ".enc"
		if err := bm.encryptFile(finalPath, encryptedPath); err != nil {
			return fmt.Errorf("encrypt backup: %w", err)
		}
		os.Remove(finalPath)
		finalPath = encryptedPath
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		if processedExists, _ := os.Stat(finalPath); processedExists != nil {
			return nil
		}
		return fmt.Errorf("move backup to final location: %w", err)
	}

	return nil
}

func (bm *BackupManager) rotateBackups() error {
	if bm.cfg.MaxBackups <= 0 {
		return nil
	}

	backups, err := bm.ListBackups()
	if err != nil {
		return err
	}

	if len(backups) <= bm.cfg.MaxBackups {
		return nil
	}

	toRemove := len(backups) - bm.cfg.MaxBackups
	bm.log.Info("rotating backups", zap.Int("to_remove", toRemove))

	for i := 0; i < toRemove; i++ {
		backup := backups[len(backups)-1-i]
		if err := os.Remove(backup.Path); err != nil {
			bm.log.Warn("failed to remove old backup",
				zap.String("path", backup.Path),
				zap.Error(err))
		} else {
			bm.log.Info("removed old backup", zap.String("path", backup.Path))
		}
	}

	return nil
}

func (bm *BackupManager) getBackupInfo(path string) (BackupInfo, error) {
	info := BackupInfo{
		Filename:   filepath.Base(path),
		Path:       path,
		Encrypted:  strings.HasSuffix(path, ".enc"),
		Compressed: strings.HasSuffix(path, ".gz"),
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return info, err
	}

	info.Size = fileInfo.Size()
	info.CreatedAt = fileInfo.ModTime()

	checksum, err := bm.calculateChecksum(path)
	if err == nil {
		info.ChecksumSHA = checksum
	}

	return info, nil
}

func (bm *BackupManager) calculateChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (bm *BackupManager) compressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (bm *BackupManager) decompressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (bm *BackupManager) encryptFile(src, dst string) error {
	if bm.cfg.EncryptionPassword == "" {
		return fmt.Errorf("encryption password required")
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	if _, err := dstFile.Write(salt); err != nil {
		return err
	}

	key := pbkdf2.Key([]byte(bm.cfg.EncryptionPassword), salt, 100000, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	if _, err := dstFile.Write(nonce); err != nil {
		return err
	}

	plaintext, err := io.ReadAll(srcFile)
	if err != nil {
		return err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	_, err = dstFile.Write(ciphertext)
	return err
}

func (bm *BackupManager) decryptBackup(src, dst string) error {
	if bm.cfg.EncryptionPassword == "" {
		return fmt.Errorf("decryption password required")
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	salt := make([]byte, 16)
	if _, err := io.ReadFull(srcFile, salt); err != nil {
		return fmt.Errorf("read salt: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(srcFile, nonce); err != nil {
		return fmt.Errorf("read nonce: %w", err)
	}

	key := pbkdf2.Key([]byte(bm.cfg.EncryptionPassword), salt, 100000, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	ciphertext, err := io.ReadAll(srcFile)
	if err != nil {
		return err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBackupPassword, err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.Write(plaintext)
	return err
}

func (bm *BackupManager) backupDatabaseToFile(ctx context.Context, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer file.Close()

	_, err = bm.db.ExecContext(ctx, "VACUUM INTO ?", path)
	if err != nil {
		return fmt.Errorf("vacuum into backup: %w", err)
	}

	return nil
}

func (bm *BackupManager) restoreDatabase(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	_, err = bm.db.ExecContext(ctx, string(data))
	if err != nil {
		return fmt.Errorf("execute restore: %w", err)
	}

	return nil
}

func (bm *BackupManager) uploadToCloud(ctx context.Context, backupPath string) error {
	if bm.cloudUploader == nil {
		return fmt.Errorf("cloud uploader not configured")
	}

	objectName := filepath.Base(backupPath)
	if bm.cfg.CloudStorage.Prefix != "" {
		objectName = filepath.Join(bm.cfg.CloudStorage.Prefix, objectName)
	}

	return bm.cloudUploader.Upload(ctx, backupPath, objectName)
}

func (bm *BackupManager) downloadFromCloud(ctx context.Context, objectName string, localPath string) error {
	if bm.cloudUploader == nil {
		return fmt.Errorf("cloud uploader not configured")
	}

	return bm.cloudUploader.Download(ctx, objectName, localPath)
}

func (bm *BackupManager) RestoreFromCloud(ctx context.Context, objectName string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.cloudUploader == nil {
		return fmt.Errorf("cloud uploader not configured")
	}

	bm.log.Info("starting cloud restore", zap.String("object", objectName))

	tmpPath := filepath.Join(bm.cfg.BackupDir, "tmp_restore_"+objectName)
	defer os.Remove(tmpPath)

	if err := bm.downloadFromCloud(ctx, objectName, tmpPath); err != nil {
		return fmt.Errorf("download from cloud: %w", err)
	}

	if err := bm.Restore(ctx, tmpPath); err != nil {
		return fmt.Errorf("restore from downloaded backup: %w", err)
	}

	bm.log.Info("cloud restore completed", zap.String("object", objectName))
	return nil
}

// Package copy provides idempotent directory synchronization.
package copy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SmonSisay/winprovision/internal/logging"
	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/utils"
)

// SyncDirectory copies files from src to dst, skipping identical files.
func SyncDirectory(src, dst string, logger logging.Logger) (models.CopyStats, error) {
	stats := models.CopyStats{}
	if !utils.DirExists(src) {
		return stats, fmt.Errorf("source directory does not exist: %s", src)
	}
	if err := utils.EnsureDir(dst); err != nil {
		return stats, err
	}

	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			stats.Failed++
			logger.Error("walk", string(models.TaskStatusFailed), walkErr.Error(), 0, walkErr)
			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			stats.Failed++
			logger.Error("relative-path", string(models.TaskStatusFailed), err.Error(), 0, err)
			return nil
		}
		target := filepath.Join(dst, rel)

		if entry.IsDir() {
			if err := utils.EnsureDir(target); err != nil {
				stats.Failed++
				logger.Error("create-dir", string(models.TaskStatusFailed), err.Error(), 0, err)
			}
			return nil
		}

		same, err := utils.SameFile(path, target)
		if err == nil && same {
			stats.Skipped++
			return nil
		}

		if err := copyFile(path, target); err != nil {
			stats.Failed++
			logger.Error("copy-file", string(models.TaskStatusFailed), fmt.Sprintf("%s -> %s", path, target), 0, err)
			return nil
		}
		stats.Copied++
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("sync directory: %w", err)
	}
	return stats, nil
}

func copyFile(src, dst string) error {
	if err := utils.EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}
	// Flush OS write-back cache to physical storage. Without this, a power
	// loss during provisioning can leave installer files silently corrupted.
	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync destination file: %w", err)
	}

	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return fmt.Errorf("set destination timestamps: %w", err)
	}
	return nil
}

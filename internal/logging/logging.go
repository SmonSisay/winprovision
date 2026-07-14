// Package logging provides structured file logging for provisioning operations.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SmonSisay/winprovision/internal/utils"
)

// Logger defines structured logging operations.
type Logger interface {
	Info(action, status, message string, duration time.Duration, err error)
	Warn(action, status, message string, duration time.Duration, err error)
	Error(action, status, message string, duration time.Duration, err error)
	WithModule(module string) Logger
	Close() error
}

// FileLogger writes structured log entries to a file.
type FileLogger struct {
	mu     *sync.Mutex // shared across all instances derived from the same root
	file   *os.File
	module string
	level  string
}

// logPriority maps level names to numeric priorities. Defined at package level
// to avoid a heap allocation on every log call.
var logPriority = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
}

// NewFileLogger creates a logger that writes to path relative to baseDir.
func NewFileLogger(baseDir, relativePath, level string) (*FileLogger, error) {
	logPath := relativePath
	if !filepath.IsAbs(relativePath) {
		logPath = filepath.Join(baseDir, relativePath)
	}
	if err := utils.EnsureDir(filepath.Dir(logPath)); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	if level == "" {
		level = "info"
	}

	return &FileLogger{
		mu:    new(sync.Mutex),
		file:  file,
		level: strings.ToLower(level),
	}, nil
}

// WithModule returns a logger scoped to the given module name.
// The returned logger shares the same mutex and file handle as the parent.
func (l *FileLogger) WithModule(module string) Logger {
	return &FileLogger{
		mu:     l.mu,
		file:   l.file,
		module: module,
		level:  l.level,
	}
}

// Info writes an informational log entry.
func (l *FileLogger) Info(action, status, message string, duration time.Duration, err error) {
	l.write("INFO", action, status, message, duration, err)
}

// Warn writes a warning log entry.
func (l *FileLogger) Warn(action, status, message string, duration time.Duration, err error) {
	l.write("WARN", action, status, message, duration, err)
}

// Error writes an error log entry.
func (l *FileLogger) Error(action, status, message string, duration time.Duration, err error) {
	l.write("ERROR", action, status, message, duration, err)
}

// Close closes the underlying log file.
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *FileLogger) write(level, action, status, message string, duration time.Duration, err error) {
	if !l.shouldLog(level) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	module := l.module
	if module == "" {
		module = "setup"
	}

	errText := ""
	if err != nil {
		errText = err.Error()
	}

	line := fmt.Sprintf(
		"timestamp=%s level=%s module=%s action=%s duration=%s status=%s message=%q error=%q\n",
		timestamp,
		level,
		module,
		action,
		duration.Round(time.Millisecond),
		status,
		message,
		errText,
	)
	_, _ = l.file.WriteString(line)
}

func (l *FileLogger) shouldLog(level string) bool {
	current, ok := logPriority[l.level]
	if !ok {
		current = 1
	}
	incoming, ok := logPriority[strings.ToLower(level)]
	if !ok {
		incoming = 1
	}
	return incoming >= current
}

// NopLogger discards all log output.
type NopLogger struct{}

// Info is a no-op.
func (NopLogger) Info(string, string, string, time.Duration, error) {}

// Warn is a no-op.
func (NopLogger) Warn(string, string, string, time.Duration, error) {}

// Error is a no-op.
func (NopLogger) Error(string, string, string, time.Duration, error) {}

// WithModule returns the same nop logger.
func (NopLogger) WithModule(string) Logger { return NopLogger{} }

// Close is a no-op.
func (NopLogger) Close() error { return nil }

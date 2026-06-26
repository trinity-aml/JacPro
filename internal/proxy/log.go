package proxy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type logLevel int

const (
	levelDebug logLevel = iota
	levelInfo
	levelWarning
	levelError
	levelCritical
)

type Logger struct {
	mu    sync.RWMutex
	level logLevel
	file  *rotatingFile
}

type rotatingFile struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
	size    int64
}

func NewLogger(settings Settings) (*Logger, error) {
	level, _ := parseLogLevel(settings.LogLevel)
	logger := &Logger{level: level}
	if settings.LogFile != "" {
		file, err := newRotatingFile(settings.LogFile, 10*1024*1024, 5)
		if err != nil {
			return nil, err
		}
		logger.file = file
	}
	return logger, nil
}

func parseLogLevel(raw string) (logLevel, bool) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DEBUG":
		return levelDebug, true
	case "", "INFO":
		return levelInfo, true
	case "WARN", "WARNING":
		return levelWarning, true
	case "ERROR":
		return levelError, true
	case "CRITICAL":
		return levelCritical, true
	default:
		return levelInfo, false
	}
}

func (l *Logger) Reconfigure(settings Settings) error {
	level, _ := parseLogLevel(settings.LogLevel)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
	currentPath := ""
	if l.file != nil {
		currentPath = l.file.path
	}
	if settings.LogFile == currentPath {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	if settings.LogFile != "" {
		file, err := newRotatingFile(settings.LogFile, 10*1024*1024, 5)
		if err != nil {
			return err
		}
		l.file = file
	}
	return nil
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(levelDebug, "DEBUG", format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(levelInfo, "INFO", format, args...) }
func (l *Logger) Warningf(format string, args ...any) {
	l.logf(levelWarning, "WARNING", format, args...)
}
func (l *Logger) Errorf(format string, args ...any) { l.logf(levelError, "ERROR", format, args...) }
func (l *Logger) Criticalf(format string, args ...any) {
	l.logf(levelCritical, "CRITICAL", format, args...)
}

func (l *Logger) logf(level logLevel, name, format string, args ...any) {
	l.mu.RLock()
	minLevel := l.level
	file := l.file
	l.mu.RUnlock()
	if level < minLevel {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %-8s %s\n", time.Now().Format(time.RFC3339), name, msg)
	_, _ = io.WriteString(os.Stdout, line)
	if file != nil {
		_, _ = file.Write([]byte(line))
	}
}

func newRotatingFile(path string, maxSize int64, backups int) (*rotatingFile, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &rotatingFile{path: path, maxSize: maxSize, backups: backups, file: file, size: info.Size()}, nil
}

func (f *rotatingFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return 0, os.ErrClosed
	}
	if f.maxSize > 0 && f.size+int64(len(p)) > f.maxSize {
		if err := f.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := f.file.Write(p)
	f.size += int64(n)
	return n, err
}

func (f *rotatingFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return nil
	}
	err := f.file.Close()
	f.file = nil
	return err
}

func (f *rotatingFile) rotate() error {
	if f.file != nil {
		_ = f.file.Close()
		f.file = nil
	}
	for i := f.backups - 1; i >= 1; i-- {
		oldName := fmt.Sprintf("%s.%d", f.path, i)
		newName := fmt.Sprintf("%s.%d", f.path, i+1)
		_ = os.Rename(oldName, newName)
	}
	if f.backups > 0 {
		_ = os.Rename(f.path, fmt.Sprintf("%s.1", f.path))
	} else {
		_ = os.Remove(f.path)
	}
	file, err := os.OpenFile(f.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	f.file = file
	f.size = 0
	return nil
}

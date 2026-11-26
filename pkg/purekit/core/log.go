package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

// SetupCrashOutput configures Go's runtime crash output (unhandled panics and
// Windows native exceptions caught by Go's VEH) to append to crash.log inside dir.
func SetupCrashOutput(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "crash.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	err = debug.SetCrashOutput(f, debug.CrashOptions{})
	closeErr := f.Close()
	if err != nil {
		return path, err
	}
	if closeErr != nil {
		return path, closeErr
	}
	return path, nil
}

// Logger writes one file per day. The GUI has no console, so this is the
// only place diagnostics go.
type Logger struct {
	mu      sync.Mutex
	dir     string
	file    *os.File
	day     string
	verbose bool
}

// OpenLogger opens the log for today.
func OpenLogger(dir string, verbose bool) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Logger{dir: dir, verbose: verbose}
	if err := l.open(time.Now()); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) open(t time.Time) error {
	day := t.Format("20060102")
	if l.file != nil && l.day == day {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	path := filepath.Join(l.dir, "pureutils-"+day+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	l.file, l.day = f, day
	return nil
}

func (l *Logger) write(level, msg string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if err := l.open(now); err != nil || l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "%s %-5s %s\n", now.Format("2006-01-02 15:04:05.000"), level, msg)
	if level != "DEBUG" || l.verbose {
		_ = l.file.Sync()
	}
}

// Debugf writes a debug line.
func (l *Logger) Debugf(format string, args ...any) { l.write("DEBUG", fmt.Sprintf(format, args...)) }

// Infof writes an info line.
func (l *Logger) Infof(format string, args ...any) { l.write("INFO", fmt.Sprintf(format, args...)) }

// Warnf writes a warning.
func (l *Logger) Warnf(format string, args ...any) { l.write("WARN", fmt.Sprintf(format, args...)) }

// Errorf writes an error.
func (l *Logger) Errorf(format string, args ...any) { l.write("ERROR", fmt.Sprintf(format, args...)) }

// Close closes the file.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

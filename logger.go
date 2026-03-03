package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/untrustedmodders/go-plugify"
)

const (
	defaultDateFmt     = "02_01_2006"
	defaultTimeFmt     = "02.01.2006 15:04:05"
	defaultChanBufSize = 128
)

type Options struct {
	Folder      string
	DateFmt     string
	TimeFmt     string
	ChanBufSize int
}

func (o *Options) withDefaults() {
	if o.DateFmt == "" {
		o.DateFmt = defaultDateFmt
	}
	if o.TimeFmt == "" {
		o.TimeFmt = defaultTimeFmt
	}
	if o.ChanBufSize <= 0 {
		o.ChanBufSize = defaultChanBufSize
	}
}

type logEntry struct {
	level string
	msg   string
	ts    time.Time
}

type Logger struct {
	dir    string
	prefix string

	dateFmt string
	timeFmt string

	ch   chan logEntry
	done chan struct{}
	once sync.Once

	file     *os.File
	filePath string

	date string
}

// New creates a new Logger with default options.
func New() (*Logger, error) {
	return NewWithOptions(Options{})
}

// NewWithOptions creates a new Logger with the provided options.
// It initializes the log directory, opens the first log file, and starts the background writer.
func NewWithOptions(opts Options) (*Logger, error) {
	opts.withDefaults()
	dir := filepath.Join(plugify.LogsDir, opts.Folder)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("logger mkdir: %w", err)
	}

	var prefix string
	if opts.Folder == "" {
		prefix = plugify.Plugin.Name + "_"
	}

	l := &Logger{
		dir:     dir,
		prefix:  prefix,
		dateFmt: opts.DateFmt,
		timeFmt: opts.TimeFmt,
		ch:      make(chan logEntry, opts.ChanBufSize),
		done:    make(chan struct{}),
	}

	if err := l.rotate(); err != nil {
		return nil, err
	}

	go l.worker()

	return l, nil
}

func (l *Logger) worker() {
	defer close(l.done)

	for entry := range l.ch {
		_ = l.writeEntry(entry)
	}

	if l.file != nil {
		_ = l.file.Close()
	}
}

func (l *Logger) rotate() error {
	today := time.Now().Format(l.dateFmt)
	needRotate := l.file == nil || l.date != today

	if !needRotate {
		if _, err := os.Stat(l.filePath); os.IsNotExist(err) {
			needRotate = true
		}
	}

	if !needRotate {
		return nil
	}

	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}

	if err := os.MkdirAll(l.dir, 0755); err != nil {
		return fmt.Errorf("logger mkdir: %w", err)
	}

	path := filepath.Join(l.dir, l.prefix+today+".log")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	l.file = f
	l.date = today
	l.filePath = path

	return nil
}

func (l *Logger) writeEntry(e logEntry) error {
	if err := l.rotate(); err != nil {
		return err
	}

	data := fmt.Sprintf("%s [%s] [%s] %s\n", e.ts.Format(l.timeFmt), plugify.Plugin.Name, e.level, e.msg)

	fmt.Print(data)
	_, err := fmt.Fprint(l.file, data)

	return err
}

func (l *Logger) write(level, msg string) {
	l.ch <- logEntry{
		level: level,
		msg:   msg,
		ts:    time.Now(),
	}
}

// Close gracefully shuts down the logger, flushing all pending log entries and closing the log file.
// It is safe to call Close multiple times.
func (l *Logger) Close() {
	l.once.Do(func() {
		close(l.ch)
		<-l.done
	})
}

// Info writes an informational message to the log.
func (l *Logger) Info(msg string) {
	l.write("INFO", msg)
}

// Infof writes a formatted informational message to the log.
func (l *Logger) Infof(format string, args ...any) {
	l.Info(fmt.Sprintf(format, args...))
}

// Error writes an error message to the log.
func (l *Logger) Error(msg string) {
	l.write("ERROR", msg)
}

// Errorf writes a formatted error message to the log.
func (l *Logger) Errorf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
}

// Warn writes a warning message to the log.
func (l *Logger) Warn(msg string) {
	l.write("WARN", msg)
}

// Warnf writes a formatted warning message to the log.
func (l *Logger) Warnf(format string, args ...any) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Log writes a general log message.
func (l *Logger) Log(msg string) {
	l.write("LOG", msg)
}

// Logf writes a formatted general log message.
func (l *Logger) Logf(format string, args ...any) {
	l.Log(fmt.Sprintf(format, args...))
}

package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/untrustedmodders/go-plugify"
)

const (
	defaultPluginName  = "plugify"
	defaultDateFmt     = "02_01_2006"
	defaultTimeFmt     = "02.01.2006 15:04:05"
	defaultChanBufSize = 1024
)

const (
	// maxBatchBytes limits how much formatted output the writer accumulates before flushing it.
	maxBatchBytes = 64 << 10

	// fileCheckInterval limits how often the writer checks that the log file still exists
	// on disk and how often it retries opening the file after a failure.
	fileCheckInterval = time.Second
)

type Options struct {
	PluginName  string
	Folder      string
	DateFmt     string
	TimeFmt     string
	ChanBufSize int
}

func (o *Options) withDefaults() {
	if o.PluginName == "" {
		o.PluginName = defaultPluginName
	}
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

// Logger writes log entries to stdout and to a daily log file from a background goroutine.
// It is safe for concurrent use. Call Close to flush pending entries and stop the writer.
type Logger struct {
	pluginName string

	dir    string
	prefix string

	dateFmt string
	timeFmt string

	ch   chan logEntry
	done chan struct{}
	once sync.Once

	mu     sync.RWMutex
	closed bool

	dropped atomic.Uint64

	console  io.Writer
	file     *os.File
	filePath string

	date      string
	lastCheck time.Time
	lastErr   error
}

// New creates a new Logger with default options.
func New() (*Logger, error) {
	return NewWithOptions(Options{PluginName: defaultPluginName})
}

// NewWithOptions creates a new Logger with the provided options.
// It initializes the log directory, opens the first log file, and starts the background writer.
func NewWithOptions(opts Options) (*Logger, error) {
	opts.withDefaults()
	dir := filepath.Join(plugify.LogsDir(), opts.Folder)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("logger mkdir: %w", err)
	}

	var prefix string
	if opts.Folder == "" {
		prefix = opts.PluginName + "_"
	}

	l := &Logger{
		pluginName: opts.PluginName,
		dir:        dir,
		prefix:     prefix,
		dateFmt:    opts.DateFmt,
		timeFmt:    opts.TimeFmt,
		ch:         make(chan logEntry, opts.ChanBufSize),
		done:       make(chan struct{}),
		console:    os.Stdout,
	}

	if err := l.rotate(); err != nil {
		return nil, err
	}

	go l.worker()

	return l, nil
}

// worker drains the channel in batches: everything already queued is written
// with one call to the console and one to the file instead of two writes per entry.
func (l *Logger) worker() {
	defer close(l.done)

	buf := make([]byte, 0, maxBatchBytes)

	for e := range l.ch {
		buf = l.appendEntry(buf[:0], e)

	drain:
		for len(buf) < maxBatchBytes {
			select {
			case e, ok := <-l.ch:
				if !ok {
					break drain
				}
				buf = l.appendEntry(buf, e)
			default:
				break drain
			}
		}

		l.flush(l.appendDropped(buf))

		// Do not keep a buffer grown by a huge message alive forever.
		if cap(buf) > 2*maxBatchBytes {
			buf = make([]byte, 0, maxBatchBytes)
		}
	}

	if buf = l.appendDropped(buf[:0]); len(buf) > 0 {
		l.flush(buf)
	}

	if l.file != nil {
		_ = l.file.Close()
	}
}

func (l *Logger) rotate() error {
	now := time.Now()
	today := now.Format(l.dateFmt)

	if l.file != nil && l.date == today {
		if now.Sub(l.lastCheck) < fileCheckInterval {
			return nil
		}

		l.lastCheck = now

		if _, err := os.Stat(l.filePath); !os.IsNotExist(err) {
			return nil
		}
	}

	if l.file == nil && l.lastErr != nil && now.Sub(l.lastCheck) < fileCheckInterval {
		return l.lastErr
	}

	l.lastCheck = now

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

func (l *Logger) appendEntry(buf []byte, e logEntry) []byte {
	buf = e.ts.AppendFormat(buf, l.timeFmt)
	buf = append(buf, " ["...)
	buf = append(buf, l.pluginName...)
	buf = append(buf, "] ["...)
	buf = append(buf, e.level...)
	buf = append(buf, "] "...)
	buf = append(buf, e.msg...)

	return append(buf, '\n')
}

func (l *Logger) appendDropped(buf []byte) []byte {
	n := l.dropped.Swap(0)
	if n == 0 {
		return buf
	}

	return l.appendEntry(buf, logEntry{
		level: "WARN",
		msg:   "dropped " + strconv.FormatUint(n, 10) + " log messages: buffer is full",
		ts:    time.Now(),
	})
}

func (l *Logger) flush(buf []byte) {
	_, _ = l.console.Write(buf)

	err := l.rotate()
	if err == nil {
		_, err = l.file.Write(buf)
	}

	l.reportErr(err)
}

// reportErr prints log file errors to stderr once per distinct error,
// so a broken log file is visible without flooding the console.
func (l *Logger) reportErr(err error) {
	switch {
	case err != nil && (l.lastErr == nil || l.lastErr.Error() != err.Error()):
		fmt.Fprintf(os.Stderr, "[%s] logger: %v\n", l.pluginName, err)
	case err == nil && l.lastErr != nil:
		fmt.Fprintf(os.Stderr, "[%s] logger: log file recovered: %s\n", l.pluginName, l.filePath)
	}

	l.lastErr = err
}

// write queues an entry and reports whether it was accepted.
func (l *Logger) write(level, msg string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return false
	}

	select {
	case l.ch <- logEntry{level: level, msg: msg, ts: time.Now()}:
		return true
	default:
		l.dropped.Add(1)
		return false
	}
}

// Close gracefully shuts down the logger, flushing all pending log entries and closing the log file.
// It is safe to call Close multiple times.
func (l *Logger) Close() {
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		close(l.ch)
		l.mu.Unlock()

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

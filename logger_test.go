package logger

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func newTestLogger(t testing.TB, opts Options) *Logger {
	t.Helper()

	l, err := NewWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Tests check the log file; keep the console output out of the test log.
	l.console = io.Discard

	return l
}

func readLog(t *testing.T, l *Logger) string {
	t.Helper()

	data, err := os.ReadFile(l.filePath)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestOptions_withDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Options
		want Options
	}{
		{
			name: "empty options",
			in:   Options{},
			want: Options{PluginName: defaultPluginName, DateFmt: defaultDateFmt, TimeFmt: defaultTimeFmt, ChanBufSize: defaultChanBufSize},
		},
		{
			name: "custom values are kept",
			in:   Options{PluginName: "test", Folder: "f", DateFmt: "2006", TimeFmt: "15", ChanBufSize: 8},
			want: Options{PluginName: "test", Folder: "f", DateFmt: "2006", TimeFmt: "15", ChanBufSize: 8},
		},
		{
			name: "negative buffer size",
			in:   Options{ChanBufSize: -1},
			want: Options{PluginName: defaultPluginName, DateFmt: defaultDateFmt, TimeFmt: defaultTimeFmt, ChanBufSize: defaultChanBufSize},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.in
			got.withDefaults()
			if got != tt.want {
				t.Errorf("withDefaults() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestLogger_rotate_throttlesFileCheck: a deleted log file is noticed and recreated
// only once per fileCheckInterval, not on every write.
func TestLogger_rotate_throttlesFileCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	synctest.Test(t, func(t *testing.T) {
		l := newTestLogger(t, Options{Folder: dir})
		defer l.Close()

		l.Info("first")
		synctest.Wait()

		if err := os.Remove(l.filePath); err != nil {
			t.Fatal(err)
		}

		l.Info("lost")
		synctest.Wait()
		if _, err := os.Stat(l.filePath); !os.IsNotExist(err) {
			t.Fatal("log file recreated before fileCheckInterval elapsed")
		}

		time.Sleep(fileCheckInterval)
		l.Info("second")
		synctest.Wait()

		if got := readLog(t, l); !strings.Contains(got, "second") || strings.Contains(got, "lost") {
			t.Fatalf("recreated log file = %q, want only the entry written after the interval", got)
		}
	})
}

// TestLogger_flush_recoversFromFileError: when the log file cannot be reopened the logger
// keeps running, retries at most once per fileCheckInterval and recovers once the problem is gone.
func TestLogger_flush_recoversFromFileError(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "logs")

	synctest.Test(t, func(t *testing.T) {
		l := newTestLogger(t, Options{Folder: dir})
		defer l.Close()

		l.Info("first")
		synctest.Wait()

		// A regular file in place of the log directory makes reopening fail.
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir, nil, 0644); err != nil {
			t.Fatal(err)
		}

		time.Sleep(fileCheckInterval)
		l.Info("lost: directory is broken")
		synctest.Wait()

		// The directory is fixed, but the retry waits for the next interval.
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
		l.Info("lost: retry throttled")
		synctest.Wait()
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("logger retried before fileCheckInterval elapsed")
		}

		time.Sleep(fileCheckInterval)
		l.Info("saved")
		synctest.Wait()

		if got := readLog(t, l); !strings.Contains(got, "saved") || strings.Contains(got, "lost") {
			t.Fatalf("log after recovery = %q, want only the entry written after recovery", got)
		}
	})
}

func TestLogger_appendDropped(t *testing.T) {
	t.Parallel()

	l := newTestLogger(t, Options{Folder: t.TempDir()})
	l.dropped.Store(3)
	l.Close()

	if got := readLog(t, l); !strings.Contains(got, "[WARN] dropped 3 log messages") {
		t.Fatalf("log = %q, want a warning about 3 dropped messages", got)
	}
}

func TestLogger_write(t *testing.T) {
	t.Parallel()

	t.Run("drops instead of blocking when the buffer is full", func(t *testing.T) {
		t.Parallel()

		// No writer goroutine: the buffer holds exactly one entry.
		l := &Logger{ch: make(chan logEntry, 1)}

		done := make(chan []bool)
		go func() {
			done <- []bool{l.write("INFO", "1"), l.write("INFO", "2"), l.write("INFO", "3")}
		}()

		select {
		case got := <-done:
			if want := []bool{true, false, false}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
				t.Fatalf("accepted = %v, want %v", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("write blocked on a full buffer")
		}

		if got := l.dropped.Load(); got != 2 {
			t.Fatalf("dropped = %d, want 2", got)
		}
	})

	t.Run("rejected after Close", func(t *testing.T) {
		t.Parallel()

		l := newTestLogger(t, Options{Folder: t.TempDir()})
		l.Close()

		if l.write("INFO", "after close") {
			t.Fatal("write accepted after Close")
		}
	})
}

func TestLogger_Close(t *testing.T) {
	t.Parallel()

	t.Run("safe to call twice", func(t *testing.T) {
		t.Parallel()

		l := newTestLogger(t, Options{Folder: t.TempDir()})
		l.Close()
		l.Close()
	})

	// Every entry accepted by write must be in the log file after Close,
	// even when writes race with Close. Run with -race to also catch panics and data races.
	t.Run("no accepted entry is lost", func(t *testing.T) {
		t.Parallel()

		for range 200 {
			l := newTestLogger(t, Options{Folder: t.TempDir(), ChanBufSize: 1 << 16})

			var accepted atomic.Int64
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 200 {
						if l.write("INFO", "x") {
							accepted.Add(1)
						}
					}
				})
			}

			l.Close()
			wg.Wait()

			if got, want := int64(strings.Count(readLog(t, l), "\n")), accepted.Load(); got != want {
				t.Fatalf("lost entries: %d accepted, %d written", want, got)
			}
		}
	})
}

// newBenchWriter returns a Logger without a file writer: a goroutine drains the channel,
// so benchmarks measure only the cost paid by the calling goroutine.
func newBenchWriter(b *testing.B) *Logger {
	l := &Logger{ch: make(chan logEntry, defaultChanBufSize), done: make(chan struct{})}

	go func() {
		defer close(l.done)
		for range l.ch {
		}
	}()
	b.Cleanup(l.Close)

	return l
}

func BenchmarkLogger_Info(b *testing.B) {
	const msg = "player connected: steamid=76561198000000000 name=test"

	b.Run("serial", func(b *testing.B) {
		l := newBenchWriter(b)

		b.ReportAllocs()
		for b.Loop() {
			l.Info(msg)
		}
	})

	b.Run("parallel", func(b *testing.B) {
		l := newBenchWriter(b)

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				l.Info(msg)
			}
		})
	})
}

// BenchmarkLogger_worker measures how fast the writer formats entries and writes them
// to stdout and the log file.
func BenchmarkLogger_worker(b *testing.B) {
	l := newTestLogger(b, Options{Folder: b.TempDir()})
	defer l.Close()

	e := logEntry{level: "INFO", msg: "player connected: steamid=76561198000000000 name=test", ts: time.Now()}

	b.ReportAllocs()
	for b.Loop() {
		l.ch <- e
	}
}

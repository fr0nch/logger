package logger_test

import (
	"sync"
	"testing"

	"github.com/fr0nch/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()

	log, err := logger.NewWithOptions(logger.Options{Folder: t.TempDir(), ChanBufSize: 1})
	if err != nil {
		t.Fatal(err)
	}

	return log
}

func TestLogger_WriteAfterClose(t *testing.T) {
	log := newTestLogger(t)
	log.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("write after Close panicked: %v", r)
		}
	}()

	log.Info("hello world")
	log.Close()
}

func TestLogger_ConcurrentClose(t *testing.T) {
	for range 20 {
		log := newTestLogger(t)

		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 100 {
					log.Info("hello world")
				}
			})
		}

		log.Close()
		wg.Wait()
	}
}

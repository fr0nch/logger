package logger_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fr0nch/logger"
)

func TestLogger_Info(t *testing.T) {
	log, err := logger.NewWithOptions(logger.Options{Dir: t.TempDir()})
	require.NoError(t, err)

	t.Run("on closed ch", func(t *testing.T) {
		log.Close()
		require.NotPanics(t, func() {
			log.Info("hello world")
		})
	})
}

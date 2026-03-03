//go:build !debug

package logger

// Debug is a no-op when the "debug" build tag is not set.
func (l *Logger) Debug(msg string) {

}

// Debugf is a no-op when the "debug" build tag is not set.
func (l *Logger) Debugf(format string, args ...any) {

}

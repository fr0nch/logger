//go:build debug

package logger

// Debug writes a debug message to the log.
// This method is only available when the "debug" build tag is set.
func (l *Logger) Debug(msg string) {
	l.write("DEBUG", msg)
}

// Debugf writes a formatted debug message to the log.
// This method is only available when the "debug" build tag is set.
func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(fmt.Sprintf(format, args...))
}

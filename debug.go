//go:build debug

package logger

func (l *Logger) Debug(msg string) {
	l.write("DEBUG", msg)
}

func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(fmt.Sprintf(format, args...))
}

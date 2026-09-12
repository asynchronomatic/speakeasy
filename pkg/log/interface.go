package log

type Logger interface {
	Printf(s string, v ...any)
	Panicf(s string, v ...any)
	Fatalf(s string, v ...any)
	Infof(s string, v ...any)
	Eventf(s string, v ...any)
	Warnf(s string, v ...any)
	Debugf(s string, v ...any)
	Highlightf(s string, v ...any)
	Errorf(s string, v ...any)
}
